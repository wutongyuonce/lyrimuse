import ApplicationServices
import Foundation
import os

/// foobar2000 的播放顺序:菜单栏 Playback 里「Order」子菜单的七项恰好一项打勾(Default、Repeat (playlist)、Repeat (track)、
/// Random、Shuffle (tracks / albums / folders)),对那一项 `AXPress` 切过去。界面只有英文,按标题认;子菜单是同时有
/// Default、Repeat (playlist)、Repeat (track) 三项的那一个。系统会话里不报这一档。需要「辅助功能」权限,没有就读不到、
/// 按钮不显示。见 02 章决策 207。
///
/// 按完要过 0.9~1 秒勾才跟着变:刚按过的那一档在 `pendingWindow` 里以按下去的结果为准,不读勾,不然连点两下会按成反的。
public enum FoobarMenuControl {
    public static let bundleID = PlaybackPlayer.foobar2000.bundleIdentifier
    public static let options: MusicPlaybackController.PlaybackModeOptions = [.shuffle, .repeatAll, .repeatOne]
    /// 按过之后多久以内不信勾。
    public static let pendingWindow: TimeInterval = 3
    static let anchorTitles: Set<String> = ["Default", "Repeat (playlist)", "Repeat (track)"]

    /// 打勾的那一项 → 档位:Random 和三种 Shuffle 都算随机。认不出为 nil。纯函数,selftest 覆盖。
    public static func mode(checkedTitle: String) -> MusicPlaybackController.MusicPlaybackMode? {
        switch checkedTitle.trimmingCharacters(in: .whitespaces) {
        case "Default": return .list
        case "Repeat (playlist)": return .repeatAll
        case "Repeat (track)": return .repeatOne
        case "Random", "Shuffle (tracks)", "Shuffle (albums)", "Shuffle (folders)": return .shuffle
        default: return nil
        }
    }

    /// 切到某一档该按哪一项;已经是这一档(随机那几项里哪一项都算)不用按,为 nil。随机按 Shuffle (tracks)。纯函数,selftest 覆盖。
    public static func titleToPress(for mode: MusicPlaybackController.MusicPlaybackMode,
                                    current: MusicPlaybackController.MusicPlaybackMode?) -> String? {
        guard current != mode else { return nil }
        switch mode {
        case .list: return "Default"
        case .repeatAll: return "Repeat (playlist)"
        case .repeatOne: return "Repeat (track)"
        case .shuffle: return "Shuffle (tracks)"
        }
    }

    /// 读到的档叠上刚按过的那一档(还在 `pendingWindow` 里的)。纯函数,selftest 覆盖。
    public static func effective(read: MusicPlaybackController.MusicPlaybackMode?,
                                 pending: (mode: MusicPlaybackController.MusicPlaybackMode, at: Date)?,
                                 now: Date) -> MusicPlaybackController.MusicPlaybackMode? {
        guard let pending, now.timeIntervalSince(pending.at) < pendingWindow else { return read }
        return pending.mode
    }

    private static let pending = OSAllocatedUnfairLock<(mode: MusicPlaybackController.MusicPlaybackMode, at: Date)?>(initialState: nil)

    /// 当前档;foobar2000 没在跑、没有辅助功能权限、找不到子菜单或不是恰好一项打勾时为 nil。会阻塞跨进程查询,别在主线程调。
    public static func readMode(now: Date = Date()) -> MusicPlaybackController.MusicPlaybackMode? {
        guard let items = orderItems() else { return nil }
        return effective(read: checkedMode(items), pending: pending.withLock { $0 }, now: now)
    }

    /// 切到某一档,返回按下去了没有(已经是这一档也算)。会阻塞,别在主线程调。
    public static func setMode(_ mode: MusicPlaybackController.MusicPlaybackMode, now: Date = Date()) -> Bool {
        guard let items = orderItems() else { return false }
        let current = effective(read: checkedMode(items), pending: pending.withLock { $0 }, now: now)
        guard let title = titleToPress(for: mode, current: current) else { return true }
        guard let item = items.first(where: { $0.title == title }), MenuBarAX.press(item.element) else { return false }
        pending.withLock { $0 = (mode, now) }
        return true
    }

    private static func checkedMode(_ items: [(title: String, element: AXUIElement)]) -> MusicPlaybackController.MusicPlaybackMode? {
        let marked = items.filter { !(MenuBarAX.string($0.element, "AXMenuItemMarkChar") ?? "").isEmpty }
        return marked.count == 1 ? mode(checkedTitle: marked[0].title) : nil
    }

    /// 「Order」子菜单的各项(标题去掉首尾空白)。
    private static func orderItems() -> [(title: String, element: AXUIElement)]? {
        guard let bar = MenuBarAX.menuBar(bundleID: bundleID) else { return nil }
        for top in MenuBarAX.menuChildren(bar) {
            for entry in MenuBarAX.menuChildren(top) {
                let sub = MenuBarAX.menuChildren(entry).map {
                    (title: (MenuBarAX.string($0, kAXTitleAttribute) ?? "").trimmingCharacters(in: .whitespaces), element: $0)
                }
                if anchorTitles.isSubset(of: Set(sub.map(\.title))) { return sub }
            }
        }
        return nil
    }
}

/// foobar2000 的音量:主窗口里那根取值 0~100 的 `AXSlider`(另一根是进度条,最大值是这首的时长)。读写它的值直接改到
/// foobar2000 的音量;窗口隐藏时照样读写。要辅助功能权限。会阻塞跨进程调用,别在主线程调。
public enum FoobarVolume {
    /// 几根滑块里哪一根是音量:取值范围正好 0~100 的那一根;没有或不止一根(这首恰好 100 秒长时进度条也是 0~100)为 nil。
    /// 纯函数,selftest 覆盖。
    public static func volumeIndex(_ sliders: [PineVolume.Slider]) -> Int? {
        let hits = sliders.indices.filter { sliders[$0].min == 0 && sliders[$0].max == 100 }
        return hits.count == 1 ? hits[0] : nil
    }

    /// 滑块值 → 0~100 的音量(四舍五入,越界夹住)。纯函数,selftest 覆盖。
    public static func percent(_ value: Double) -> Int {
        Int((Swift.min(100, Swift.max(0, value))).rounded())
    }

    /// 当前音量 0~100;foobar2000 没在跑、没授权、找不到那根滑块为 nil。
    public static func read() -> Int? {
        guard let slider = volumeSlider(),
              let value = (FoobarSliders.attribute(slider, kAXValueAttribute) as? NSNumber)?.doubleValue else { return nil }
        return percent(value)
    }

    /// 设成 0~100(越界夹住)。写成了为 true。
    public static func set(_ percent: Int) -> Bool {
        guard let slider = volumeSlider() else { return false }
        let value = Double(Swift.min(100, Swift.max(0, percent)))
        return AXUIElementSetAttributeValue(slider, kAXValueAttribute as CFString, value as CFNumber) == .success
    }

    private static func volumeSlider() -> AXUIElement? {
        let found = FoobarSliders.all()
        return volumeIndex(found.map(\.slider)).map { found[$0].element }
    }
}

/// foobar2000 主窗口里的滑块:音量(0~100)和进度条(最大值是这首的时长,值是当前位置,带小数)。要辅助功能权限;窗口隐藏时
/// 照样读得到,被隐藏启动、主窗口没布局时读不到。会阻塞跨进程调用,别在主线程调。
public enum FoobarSliders {
    /// 几根滑块里哪一根是进度条:最大值跟时长差 1.5 秒以内的那一根;没有或不止一根(这首恰好 100 秒长时音量也对得上)为 nil。
    /// 纯函数,selftest 覆盖。
    public static func progressIndex(_ sliders: [PineVolume.Slider], duration: Double) -> Int? {
        let hits = sliders.indices.filter { sliders[$0].min == 0 && abs(sliders[$0].max - duration) <= 1.5 }
        return hits.count == 1 ? hits[0] : nil
    }

    /// 进度条此刻的值(秒);读不到为 nil。
    public static func progress(duration: Double?) -> Double? {
        guard let duration, duration > 0 else { return nil }
        let found = all()
        return progressIndex(found.map(\.slider), duration: duration).map { found[$0].slider.value }
    }

    static func all() -> [(element: AXUIElement, slider: PineVolume.Slider)] {
        guard AXIsProcessTrusted(), let pid = PlayerProcess.pid(bundleID: FoobarMenuControl.bundleID) else { return [] }
        let app = MenuBarAX.timed(AXUIElementCreateApplication(pid), timeout: PineVolume.axTimeout)
        var found: [(element: AXUIElement, slider: PineVolume.Slider)] = []
        for window in MenuBarAX.children(app, attribute: kAXWindowsAttribute, timeout: PineVolume.axTimeout) {
            for child in MenuBarAX.children(window, timeout: PineVolume.axTimeout) {
                for element in [child] + MenuBarAX.children(child, timeout: PineVolume.axTimeout) {
                    guard (attribute(element, kAXRoleAttribute) as? String) == kAXSliderRole,
                          let min = (attribute(element, kAXMinValueAttribute) as? NSNumber)?.doubleValue,
                          let max = (attribute(element, kAXMaxValueAttribute) as? NSNumber)?.doubleValue,
                          let value = (attribute(element, kAXValueAttribute) as? NSNumber)?.doubleValue else { continue }
                    found.append((element, PineVolume.Slider(min: min, max: max, value: value)))
                }
            }
            if !found.isEmpty { break }
        }
        return found
    }

    static func attribute(_ element: AXUIElement, _ name: String) -> AnyObject? {
        var value: AnyObject?
        return AXUIElementCopyAttributeValue(element, name as CFString, &value) == .success ? value : nil
    }
}
