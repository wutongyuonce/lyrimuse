import Foundation

/// 更新之后让用户知道这一版多了什么:升级后第一次打开菜单栏面板时顶部一条「已更新到 X」,点开是一张「本次更新」卡片
/// (内容见 `ReleaseNotes`);菜单栏右键菜单里一直有「查看本次更新…」。这里是判据,状态与界面在 App 侧 `WhatsNewCenter`。
/// 见 14 章决策 155。
///
/// 「升级」按版本号算:记下上次启动的版本,当前版本比它新就把上次那一版当基准。新装的用户基准就是当前版本,不出。
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

    /// 面板顶部要不要出「已更新」:主版本或次版本比基准新(只修 bug 的补丁版不出),且这一版还没出过。
    public static func bannerPending(current: String, baseline: String, shownVersion: String?) -> Bool {
        guard shownVersion != current else { return false }
        return compare(featureVersion(current), featureVersion(baseline)) == .orderedDescending
    }

    /// `1.12.3` → `1.12`。
    public static func featureVersion(_ version: String) -> String {
        version.split(separator: ".").prefix(2).joined(separator: ".")
    }

    /// 按点分的数字段比(1.9.0 < 1.12.0);缺的段当 0,非数字的段(预发布后缀)当 0。
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
