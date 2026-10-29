import AppKit
import LyrimuseCore
import QuartzCore

/// 灵动岛稳态顶行以下那一截(歌词行、掉歌名条、横幅条)只显示、不接指针:指针压上来时整扇窗切成 `ignoresMouseEvents`,
/// 点击、滚动落到下层窗口,光标周围挖一个渐隐的洞露出底下的内容;指针离开这一截就恢复接收。见 05 章决策 121。
///
/// 穿透期间这扇窗收不到任何指针事件,SwiftUI 的悬停也就不来了:指针去哪由全局 / 本地监听器跟踪(只在穿透期间装着,
/// 平时不让全系统的指针移动经 IPC 送进本进程),再加一个低频兜底,监听器漏了事件也能及时退出。
///
/// 洞挂在 hosting view 的 layer 上(`CALayer.mask`),只在穿透期间存在。别改成 SwiftUI 里常驻的 `.mask`:灵动岛的逐字填色
/// 一直在动,常驻遮罩让整卡每帧多一次离屏合成;按条件挂又会换掉卡片的视图身份、重置里面的状态。
@MainActor
final class NotchLyricRowPassThrough {
    struct Hit {
        var region: NotchHoverHit.Region
        /// 指针的卡片局部坐标(原点在卡片左上角)。
        var local: CGPoint
        var cardSize: CGSize
        /// 只显示那一截的上沿(卡片局部 y);nil = 此刻整张卡都接指针。
        var bandTop: CGFloat?
    }

    private weak var window: NSWindow?
    private weak var hostView: NSView?
    /// 给一个屏幕坐标,按此刻的卡片矩形和形态判它落在哪一块。
    private let locate: (NSPoint) -> Hit?
    /// 指针离开只显示那一截时调,参数是它现在所在的那一块。
    private let onExit: (NotchHoverHit.Region) -> Void

    private var monitors: [Any] = []
    private var fallbackTimer: Timer?
    private var maskLayer: CALayer?
    private var maskShape: CAShapeLayer?
    private var holeClip: CALayer?
    private var holeGradient: CAGradientLayer?

    /// 兜底复查的间隔:指针移到本 App 别的窗口上时两个监听器都可能收不到。
    private static let fallbackInterval: TimeInterval = 0.25

    init(window: NSWindow?, hostView: NSView?, locate: @escaping (NSPoint) -> Hit?,
         onExit: @escaping (NotchHoverHit.Region) -> Void) {
        self.window = window
        self.hostView = hostView
        self.locate = locate
        self.onExit = onExit
    }

    var isActive: Bool { !monitors.isEmpty }

    /// 设了就拿它代替真指针(屏幕坐标,`debug/notch-pass-through`)。
    var debugPointer: NSPoint?

    func begin() {
        guard !isActive, let window else { return }
        window.ignoresMouseEvents = true
        let mask: NSEvent.EventTypeMask = [.mouseMoved, .leftMouseDragged, .rightMouseDragged, .otherMouseDragged]
        if let global = NSEvent.addGlobalMonitorForEvents(matching: mask, handler: { [weak self] _ in
            MainActor.assumeIsolated { self?.track() }
        }) {
            monitors.append(global)
        }
        if let local = NSEvent.addLocalMonitorForEvents(matching: mask, handler: { [weak self] event in
            MainActor.assumeIsolated { self?.track() }
            return event
        }) {
            monitors.append(local)
        }
        let timer = Timer(timeInterval: Self.fallbackInterval, repeats: true) { [weak self] _ in
            MainActor.assumeIsolated { self?.track() }
        }
        RunLoop.main.add(timer, forMode: .common)
        fallbackTimer = timer
        NotchLyricsWindowController.hoverLog.notice("pass-through begin")
        track()
    }

    /// 卡片尺寸或形态变了(展开、广告开始、换行高)时复查一次:指针没动也可能已经不在只显示那一截上了。
    func refresh() {
        guard isActive else { return }
        track()
    }

    func end() {
        guard isActive else { return }
        monitors.forEach(NSEvent.removeMonitor)
        monitors.removeAll()
        fallbackTimer?.invalidate()
        fallbackTimer = nil
        window?.ignoresMouseEvents = false
        closeHole()
        NotchLyricsWindowController.hoverLog.notice("pass-through end")
    }

    private func track() {
        guard isActive else { return }
        guard window?.isVisible ?? false, let hit = locate(debugPointer ?? NSEvent.mouseLocation) else {
            end()
            onExit(.outside)
            return
        }
        guard hit.region == .passThrough, let bandTop = hit.bandTop else {
            end()
            onExit(hit.region)
            return
        }
        showHole(at: hit.local, bandTop: bandTop, cardSize: hit.cardSize)
    }

    // MARK: - 洞

    private func showHole(at pointer: CGPoint, bandTop: CGFloat, cardSize: CGSize) {
        guard let hostView, let layer = hostView.layer else { return }
        let opening = maskLayer == nil
        if opening { installMask(on: layer) }
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        let bounds = layer.bounds
        maskLayer?.frame = bounds
        maskShape?.frame = bounds
        let hole = NotchPassThroughHole.rect(pointer: pointer, bandTop: bandTop,
                                             cardWidth: cardSize.width, cardHeight: cardSize.height)
            .map { viewRect(cardRect: $0, cardWidth: cardSize.width, in: hostView) }
        let path = CGMutablePath()
        path.addRect(bounds)
        if let hole { path.addRect(hole) }
        maskShape?.path = path
        if let hole {
            holeClip?.isHidden = false
            holeClip?.frame = hole
            let r = NotchPassThroughHole.radius
            let center = viewPoint(cardPoint: pointer, cardWidth: cardSize.width, in: hostView)
            holeGradient?.frame = CGRect(x: center.x - r - hole.minX, y: center.y - r - hole.minY,
                                         width: r * 2, height: r * 2)
        } else {
            holeClip?.isHidden = true
        }
        CATransaction.commit()
        if opening { fadeHole(open: true) }
    }

    private func installMask(on layer: CALayer) {
        let container = CALayer()
        let shape = CAShapeLayer()
        shape.fillRule = .evenOdd
        shape.fillColor = NSColor.black.cgColor
        let clip = CALayer()
        clip.masksToBounds = true
        let gradient = CAGradientLayer()
        gradient.type = .radial
        gradient.startPoint = CGPoint(x: 0.5, y: 0.5)
        gradient.endPoint = CGPoint(x: 1, y: 1)
        gradient.locations = [0, NSNumber(value: Double(NotchPassThroughHole.clearFraction)), 1]
        gradient.colors = Self.closedColors
        clip.addSublayer(gradient)
        container.addSublayer(shape)
        container.addSublayer(clip)
        layer.mask = container
        maskLayer = container
        maskShape = shape
        holeClip = clip
        holeGradient = gradient
    }

    private static let closedColors = [NSColor.black.cgColor, NSColor.black.cgColor, NSColor.black.cgColor]
    private static let openColors = [NSColor.clear.cgColor, NSColor.clear.cgColor, NSColor.black.cgColor]

    private func fadeHole(open: Bool, completion: (() -> Void)? = nil) {
        guard let gradient = holeGradient else { completion?(); return }
        let target = open ? Self.openColors : Self.closedColors
        CATransaction.begin()
        CATransaction.setAnimationDuration(NotchPassThroughHole.fadeDuration)
        CATransaction.setCompletionBlock(completion)
        let animation = CABasicAnimation(keyPath: "colors")
        animation.fromValue = gradient.presentation()?.colors ?? gradient.colors
        animation.toValue = target
        animation.duration = NotchPassThroughHole.fadeDuration
        animation.timingFunction = CAMediaTimingFunction(name: .easeOut)
        gradient.colors = target
        gradient.add(animation, forKey: "fade")
        CATransaction.commit()
    }

    /// 合上之后才摘遮罩:直接摘掉洞会一下子跳没。合上途中又穿透了就接着用这一份。
    private func closeHole() {
        guard let container = maskLayer else { return }
        fadeHole(open: false) { [weak self] in
            MainActor.assumeIsolated {
                guard let self, !self.isActive, self.maskLayer === container else { return }
                self.hostView?.layer?.mask = nil
                self.maskLayer = nil
                self.maskShape = nil
                self.holeClip = nil
                self.holeGradient = nil
            }
        }
    }

    /// 卡片贴着窗口顶边、水平居中(同控制器 `cardScreenFrame`)。卡片局部坐标原点在左上角、y 向下。
    private func viewPoint(cardPoint: CGPoint, cardWidth: CGFloat, in view: NSView) -> CGPoint {
        let x = (view.bounds.width - cardWidth) / 2 + cardPoint.x
        let y = view.isFlipped ? cardPoint.y : view.bounds.height - cardPoint.y
        return CGPoint(x: x, y: y)
    }

    private func viewRect(cardRect: CGRect, cardWidth: CGFloat, in view: NSView) -> CGRect {
        let a = viewPoint(cardPoint: CGPoint(x: cardRect.minX, y: cardRect.minY), cardWidth: cardWidth, in: view)
        let b = viewPoint(cardPoint: CGPoint(x: cardRect.maxX, y: cardRect.maxY), cardWidth: cardWidth, in: view)
        return CGRect(x: min(a.x, b.x), y: min(a.y, b.y), width: abs(b.x - a.x), height: abs(b.y - a.y))
    }
}
