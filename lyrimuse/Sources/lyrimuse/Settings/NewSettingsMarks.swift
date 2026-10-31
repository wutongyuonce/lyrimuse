import Foundation
import LyrimuseCore
import SwiftUI

/// 更新之后的提示状态(判据在 Core `WhatsNew`):
/// - 设置里这次更新新加的设置(`NewSettingsCatalog`)标题后挂小蓝点:`SettingsRow`、`SettingsCardHeader` 自己问 `isPending`,
///   设置窗口侧栏按 `pendingTabs` 给含新设置的分类挂点。那一行至少一半滚进屏幕(浮层里出现即算)就记为看过,蓝点淡出。
/// - 菜单栏面板顶部一条「已更新到 X」(`bannerVersion`),点「看看新功能」去「软件更新」页、或点 ✕ 后这一版不再出。
///
/// 启动时 `start()` 定这次的基准版本并记下当前版本。几个键都是机器专属的(ConfigPortability.machineLocalDefaultsKeys)。
/// 见 14 章决策 153。
@MainActor
final class NewSettingsMarks: ObservableObject {
    static let shared = NewSettingsMarks()

    enum Keys {
        static let lastLaunchedVersion = "np:lastLaunchedVersion"
        static let baseline = "np:newSettingsBaseline"
        /// 看过的新设置(titleKey)。
        static let seen = "np:newSettingsSeen"
        /// 面板顶部「已更新」关掉的是哪一版。
        static let bannerDismissed = "np:whatsNewDismissedVersion"
    }

    @Published private(set) var pendingKeys: Set<String> = []
    @Published private(set) var bannerVersion: String?

    private let defaults = UserDefaults.standard
    private var started = false

    private init() {}

    func start() {
        guard !started else { return }
        started = true
        let current = SparkleUpdaterManager.appVersionString
        let baseline = WhatsNew.baseline(current: current, lastLaunched: defaults.string(forKey: Keys.lastLaunchedVersion),
                                         stored: defaults.string(forKey: Keys.baseline),
                                         hasCompletedOnboarding: AppSettings.shared.hasCompletedOnboarding)
        defaults.set(baseline, forKey: Keys.baseline)
        defaults.set(current, forKey: Keys.lastLaunchedVersion)
        let seen = Set(defaults.stringArray(forKey: Keys.seen) ?? [])
        pendingKeys = WhatsNew.pendingKeys(releases: NewSettingsCatalog.releases, baseline: baseline, seen: seen)
        bannerVersion = WhatsNew.bannerPending(current: current, baseline: baseline,
                                               dismissedVersion: defaults.string(forKey: Keys.bannerDismissed)) ? current : nil
    }

    /// 标题为 `title`(当前界面语言)的设置还没看过。
    func isPending(title: String) -> Bool { key(for: title) != nil }

    /// 侧栏里含没看过的新设置的分类(`SettingsTab` 的 rawValue)。
    var pendingTabs: Set<String> {
        var tabs = Set<String>()
        for entry in SettingsSearchCatalog.entries where pendingKeys.contains(entry.titleKey) {
            if case .tab(let raw) = entry.destination { tabs.insert(raw) }
        }
        return tabs
    }

    /// 那一行出现在屏幕上了。
    func seen(title: String) {
        guard let key = key(for: title) else { return }
        var seen = Set(defaults.stringArray(forKey: Keys.seen) ?? [])
        seen.insert(key)
        defaults.set(Array(seen).sorted(), forKey: Keys.seen)
        withAnimation(.easeOut(duration: 0.25)) { _ = pendingKeys.remove(key) }
    }

    func dismissBanner() {
        guard let bannerVersion else { return }
        defaults.set(bannerVersion, forKey: Keys.bannerDismissed)
        self.bannerVersion = nil
    }

    private func key(for title: String) -> String? {
        pendingKeys.first { L10n.t($0) == title }
    }
}

/// 设置行、卡头标题后面的蓝点与「看过」判定,两处共用。
struct NewSettingDot: View {
    let title: String
    @ObservedObject private var marks = NewSettingsMarks.shared

    var body: some View {
        if marks.isPending(title: title) {
            SettingsUnseenDot()
                .transition(.opacity)
                .modifier(NewSettingSeen(title: title))
        }
    }
}

/// 滚动区里至少一半滚进屏幕才算看过;浮层里不在滚动区里,出现即算。
private struct NewSettingSeen: ViewModifier {
    let title: String
    @Environment(\.settingsInPopover) private var inPopover

    func body(content: Content) -> some View {
        if inPopover {
            content.onAppear { NewSettingsMarks.shared.seen(title: title) }
        } else {
            content.onScrolledIntoView { NewSettingsMarks.shared.seen(title: title) }
        }
    }
}
