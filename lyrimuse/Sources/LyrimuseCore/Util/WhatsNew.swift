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
    /// 当前版本比上次启动的新 = 刚升级,基准换成上次那一版;但上次那一版的横条该出却没出过(没打开过面板就又升了),
    /// 沿用存着的基准,不然那次大版本的提示就再也不出。没升级沿用存着的基准。
    public static func baseline(current: String, lastLaunched: String?, stored: String?, shownVersion: String?,
                                hasCompletedOnboarding: Bool) -> String {
        guard let lastLaunched else { return hasCompletedOnboarding ? legacyBaseline : current }
        guard compare(current, lastLaunched) == .orderedDescending else { return stored ?? lastLaunched }
        if let stored, pendingFeatureUpgrade(current: lastLaunched, baseline: stored, shownVersion: shownVersion) {
            return stored
        }
        return lastLaunched
    }

    /// 面板顶部要不要出「已更新」:主版本或次版本比基准新(只修 bug 的补丁版不出),且这一版还没出过。
    /// 测试版不出:CHANGELOG 里没有测试版的节,卡片是空的。
    public static func bannerPending(current: String, baseline: String, shownVersion: String?) -> Bool {
        !isPrerelease(current) && pendingFeatureUpgrade(current: current, baseline: baseline, shownVersion: shownVersion)
    }

    private static func pendingFeatureUpgrade(current: String, baseline: String, shownVersion: String?) -> Bool {
        if let shownVersion, featureVersion(shownVersion) == featureVersion(current),
           compare(shownVersion, current) != .orderedDescending || isPrerelease(current) {
            return false
        }
        return compare(featureVersion(current), featureVersion(baseline)) == .orderedDescending
    }

    /// `1.13.0-beta.2` 这类带预发布后缀的。
    public static func isPrerelease(_ version: String) -> Bool { version.contains("-") }

    /// `1.12.3` → `1.12`(预发布后缀不算)。
    public static func featureVersion(_ version: String) -> String {
        release(version).split(separator: ".").prefix(2).joined(separator: ".")
    }

    /// 先按 X.Y.Z 的数字段比(1.9.0 < 1.12.0,缺的段当 0);相同时测试版排在正式版前面(1.13.0-beta.2 < 1.13.0),
    /// 两个测试版按后缀里的数字段比(beta.1 < beta.2)。
    public static func compare(_ a: String, _ b: String) -> ComparisonResult {
        func numbers(_ v: Substring) -> [Int] {
            v.split(whereSeparator: { $0 == "." || $0 == "-" }).map { Int($0) ?? 0 }
        }
        func compareNumbers(_ pa: [Int], _ pb: [Int]) -> ComparisonResult {
            for i in 0..<max(pa.count, pb.count) {
                let x = i < pa.count ? pa[i] : 0, y = i < pb.count ? pb[i] : 0
                if x != y { return x < y ? .orderedAscending : .orderedDescending }
            }
            return .orderedSame
        }
        let main = compareNumbers(numbers(Substring(release(a))), numbers(Substring(release(b))))
        guard main == .orderedSame else { return main }
        switch (isPrerelease(a), isPrerelease(b)) {
        case (false, false): return .orderedSame
        case (true, false): return .orderedAscending
        case (false, true): return .orderedDescending
        case (true, true): return compareNumbers(numbers(suffix(a)), numbers(suffix(b)))
        }
    }

    private static func release(_ version: String) -> String {
        version.split(separator: "-", maxSplits: 1).first.map(String.init) ?? version
    }

    private static func suffix(_ version: String) -> Substring {
        version.split(separator: "-", maxSplits: 1).dropFirst().first ?? ""
    }
}
