import AppKit
import Combine
import LyrimuseCore
import os

/// 「锁屏时显示」(`AppSettings.notchShowOnLockScreen`):锁屏期间在灵动岛那块屏上另摆一份灵动岛,窗口在锁屏之上的
/// Space 里(`LockScreenSpace`)。平时那份照旧在锁屏底下,看不见、按 occlusion 停表。
///
/// 锁屏那份按屏幕存、建了就不销毁(挪进那个 Space 的窗口被释放会让 SkyLight 崩),解锁、关掉开关时只藏起来。
/// 不碰 `.shared`(见 `NotchLyricsWindowController` 文件头那条不变量):目标屏用静态的 `targetScreen()` 算。
/// 解锁通知有时比真正解锁晚好几秒,锁屏那份露着时每 0.5 秒查一次会话的锁屏状态,先一步藏起来。见 05 章决策 122。
@MainActor
enum NotchLockScreenPresenter {
    private static var instances: [String: NotchLyricsWindowController] = [:]
    private static var cancellables = Set<AnyCancellable>()
    private static var locked = false
    private static var shownScreenID: String?
    private static var unlockPoll: Timer?
    private static var unlockWatch = LockScreenUnlockWatch()
    /// 挪不进锁屏那层(私有函数没了):这次运行不再试。
    private static var spaceUnavailable = false
    private static let log = Logger(subsystem: "me.yudaotor.lyrimuse", category: "notch-lock-screen")

    /// 锁屏那份此刻露着。锁屏时歌词那一拍要不要接着走看它(`AppDelegate.applyScreenHidden`)。
    static var isPresenting: Bool { shownScreenID != nil }

    /// App 启动时调一次。
    static func start() {
        guard cancellables.isEmpty else { return }
        let settings = AppSettings.shared
        // 两个值从参数拿:@Published 在 willSet 时机发布,回读拿到的是旧值。
        settings.$notchShowOnLockScreen
            .combineLatest(settings.$notchOverlayEnabled)
            .dropFirst()
            .sink { enabled, notchEnabled in apply(enabled: enabled && notchEnabled) }
            .store(in: &cancellables)
    }

    /// 锁屏 / 解锁(`AppDelegate` 按系统通知调)。
    static func setLocked(_ isLocked: Bool) {
        guard locked != isLocked else { return }
        locked = isLocked
        let settings = AppSettings.shared
        apply(enabled: settings.notchShowOnLockScreen && settings.notchOverlayEnabled)
    }

    private static func apply(enabled: Bool) {
        let wanted = locked && enabled ? targetScreenID() : nil
        guard wanted != shownScreenID else { return }
        if let shownScreenID { instances[shownScreenID]?.syncStateFromSettings(notchEnabled: false) }
        shownScreenID = nil
        if let wanted, let instance = instance(for: wanted) {
            instance.syncStateFromSettings(notchEnabled: true)
            shownScreenID = wanted
            log.notice("shown on lock screen")
        } else if wanted == nil {
            log.notice("hidden from lock screen")
        }
        syncUnlockPoll()
    }

    private static func targetScreenID() -> String? {
        NotchLyricsWindowController.targetScreen().flatMap(ScreenIdentity.id(of:))
    }

    /// 这块屏的锁屏那份:没有就建一个、挪进锁屏那层;挪不进去(私有函数没了)就拆掉、不显示。
    private static func instance(for screenID: String) -> NotchLyricsWindowController? {
        if let existing = instances[screenID] { return existing }
        guard !spaceUnavailable else { return nil }
        let instance = NotchLyricsWindowController(pinnedScreenID: screenID, presentsOnLockScreen: true)
        guard let window = instance.window, LockScreenSpace.adopt(window) else {
            // 没挪进去的窗口还是普通窗口,拆掉安全。
            instance.teardown()
            spaceUnavailable = true
            log.error("lock screen space unavailable; notch stays hidden while locked")
            return nil
        }
        instances[screenID] = instance
        return instance
    }

    private static func syncUnlockPoll() {
        guard isPresenting else {
            unlockPoll?.invalidate()
            unlockPoll = nil
            return
        }
        guard unlockPoll == nil else { return }
        unlockWatch = LockScreenUnlockWatch()
        unlockPoll = Timer.scheduledTimer(withTimeInterval: 0.5, repeats: true) { _ in
            MainActor.assumeIsolated {
                guard unlockWatch.observe(sessionLocked: sessionScreenIsLocked()) else { return }
                log.notice("session unlocked before the unlock notification")
                setLocked(false)
            }
        }
    }

    /// 会话此刻是不是锁着。没锁时字典里没有这个键。
    private static func sessionScreenIsLocked() -> Bool {
        let session = CGSessionCopyCurrentDictionary() as? [String: Any]
        return session?["CGSSessionScreenIsLocked"] as? Bool ?? false
    }
}
