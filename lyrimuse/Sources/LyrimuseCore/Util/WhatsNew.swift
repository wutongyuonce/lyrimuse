import Foundation

/// 更新之后让用户知道多了什么:菜单栏面板顶部一条「已更新到 X」、设置里给这一版新加的设置挂小蓝点。这里是判据,
/// 状态与界面在 App 侧 `NewSettingsMarks`。见 14 章决策 153。
///
/// 「新」按这次升级算:记下上次启动的版本,升级后把升级前那一版当基准,只挂基准之后各版新加的设置(`NewSettingsCatalog`)。
/// 新装的用户基准就是当前版本,什么都不挂。
public enum WhatsNew {
    /// 有这套机制之前的最后一个版本:老用户第一次跑到这套代码时没有「上次启动的版本」,按从它升上来算。
    public static let legacyBaseline = "1.11.0"

    /// 这次启动的基准版本。`lastLaunched` 为 nil:老用户(走完过引导)按 `legacyBaseline`,新用户按当前版本。
    /// 当前版本比上次启动的新 = 刚升级,基准换成上次那一版;否则沿用存着的基准。
    public static func baseline(current: String, lastLaunched: String?, stored: String?,
                                hasCompletedOnboarding: Bool) -> String {
        guard let lastLaunched else { return hasCompletedOnboarding ? legacyBaseline : current }
        if compare(current, lastLaunched) == .orderedDescending { return lastLaunched }
        return stored ?? lastLaunched
    }

    /// 要挂蓝点的设置:基准之后各版新加的,去掉看过的。
    public static func pendingKeys(releases: [(version: String, titleKeys: [String])], baseline: String,
                                   seen: Set<String>) -> Set<String> {
        var keys = Set<String>()
        for release in releases where compare(release.version, baseline) == .orderedDescending {
            keys.formUnion(release.titleKeys)
        }
        return keys.subtracting(seen)
    }

    /// 面板顶部要不要出「已更新」:主版本或次版本比基准新(只修 bug 的补丁版不出),且这一版还没关过。
    public static func bannerPending(current: String, baseline: String, dismissedVersion: String?) -> Bool {
        guard dismissedVersion != current else { return false }
        return compare(featureVersion(current), featureVersion(baseline)) == .orderedDescending
    }

    /// `1.12.3` → `1.12`。
    public static func featureVersion(_ version: String) -> String {
        version.split(separator: ".").prefix(2).joined(separator: ".")
    }

    /// 按点分的数字段比(1.9.0 < 1.12.0);缺的段当 0,非数字的段(预发布后缀)忽略。
    public static func compare(_ a: String, _ b: String) -> ComparisonResult {
        func parts(_ v: String) -> [Int] {
            v.split(whereSeparator: { $0 == "." || $0 == "-" }).map { Int($0) ?? 0 }
        }
        let pa = parts(a), pb = parts(b)
        for i in 0..<max(pa.count, pb.count) {
            let x = i < pa.count ? pa[i] : 0, y = i < pb.count ? pb[i] : 0
            if x != y { return x < y ? .orderedAscending : .orderedDescending }
        }
        return .orderedSame
    }
}
