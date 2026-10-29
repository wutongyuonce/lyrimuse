import AppKit
import LyrimuseCore
import QuartzCore

/// 灵动岛「悬浮淡化」(`AppSettings.notchFadeLyricRowOnHover`):稳态顶行以下那一截(歌词行、掉歌名条、横幅条)只显示、不接指针。
/// 指针压上来时整扇窗切成 `ignoresMouseEvents`,点击、滚动落到下层窗口,这一截整条淡下去露出底下的内容;指针离开这一截就恢复接收。
/// 见 05 章决策 121、124。
///
/// 穿透期间这扇窗收不到任何指针事件,SwiftUI 的悬停也就不来了:指针去哪由全局 / 本地监听器跟踪(只在穿透期间装着,
/// 平时不让全系统的指针移动经 IPC 送进本进程),再加一个低频兜底,监听器漏了事件也能及时退出。
///
/// 淡化挂在 hosting view 的 layer 上(`CALayer.mask`),只在穿透期间存在。别改成 SwiftUI 里常驻的 `.mask`:灵动岛的逐字填色
/// 一直在动,常驻遮罩让整卡每帧多一次离屏合成;按条件挂又会换掉卡片的视图身份、重置里面的状态。
@MainActor
final class NotchLyricRowPassThrough {
    struct Hit {
        var region: NotchHoverHit.Region
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
    /// 顶行那一块,一直不透明。
    private var maskTop: CALayer?
    /// 只显示那一截,透明度在 1 与 `NotchPassThroughFade.opacity` 之间渐变。
    private var maskBand: CALayer?

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
        unfade()
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
        fade(bandTop: bandTop, cardSize: hit.cardSize)
    }

    // MARK: - 淡化

    private func fade(bandTop: CGFloat, cardSize: CGSize) {
        guard let hostView, let layer = hostView.layer else { return }
        let opening = maskLayer == nil
        if opening { installMask(on: layer) }
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        let bounds = layer.bounds
        maskLayer?.frame = bounds
        // 卡片贴着窗口顶边,上沿那条线以上是顶行(整宽都算:卡片两侧本来就是透明区)。
        let lineY = hostView.isFlipped ? bandTop : bounds.height - bandTop
        let top = hostView.isFlipped
            ? CGRect(x: 0, y: 0, width: bounds.width, height: lineY)
            : CGRect(x: 0, y: lineY, width: bounds.width, height: bounds.height - lineY)
        let band = hostView.isFlipped
            ? CGRect(x: 0, y: lineY, width: bounds.width, height: bounds.height - lineY)
            : CGRect(x: 0, y: 0, width: bounds.width, height: lineY)
        maskTop?.frame = top
        maskBand?.frame = band
        CATransaction.commit()
        // 正在淡回(刚离开又压回来)时模型值已经是 1,接着从当前样子淡下去。
        if opening || maskBand?.opacity != NotchPassThroughFade.opacity { animateBand(to: NotchPassThroughFade.opacity) }
    }

    private func installMask(on layer: CALayer) {
        let container = CALayer()
        let top = CALayer()
        top.backgroundColor = NSColor.black.cgColor
        let band = CALayer()
        band.backgroundColor = NSColor.black.cgColor
        container.addSublayer(top)
        container.addSublayer(band)
        layer.mask = container
        maskLayer = container
        maskTop = top
        maskBand = band
    }

    private func animateBand(to opacity: Float, completion: (() -> Void)? = nil) {
        guard let band = maskBand else { completion?(); return }
        CATransaction.begin()
        CATransaction.setCompletionBlock(completion)
        let animation = CABasicAnimation(keyPath: "opacity")
        animation.fromValue = band.presentation()?.opacity ?? band.opacity
        animation.toValue = opacity
        animation.duration = NotchPassThroughFade.duration
        animation.timingFunction = CAMediaTimingFunction(name: .easeOut)
        band.opacity = opacity
        band.add(animation, forKey: "fade")
        CATransaction.commit()
    }

    /// 淡回不透明之后才摘遮罩:直接摘掉会一下子跳回来。淡回途中又穿透了就接着用这一份。
    private func unfade() {
        guard let container = maskLayer else { return }
        animateBand(to: 1) { [weak self] in
            MainActor.assumeIsolated {
                guard let self, !self.isActive, self.maskLayer === container else { return }
                self.hostView?.layer?.mask = nil
                self.maskLayer = nil
                self.maskTop = nil
                self.maskBand = nil
            }
        }
    }
}
