import Foundation
import LyrimuseCore

func runWhatsNewTests() {
    do {
        print("\n== 更新后的新功能提示:基准版本 ==")
        typealias W = WhatsNew
        expectEqual(W.baseline(current: "1.12.0", lastLaunched: nil, stored: nil, hasCompletedOnboarding: true),
                    W.legacyBaseline, "基准: 老用户第一次跑到这套代码,按从上一个版本升上来算")
        expectEqual(W.baseline(current: "1.12.0", lastLaunched: nil, stored: nil, hasCompletedOnboarding: false),
                    "1.12.0", "基准: 新装的用户就是当前版本,什么都不挂")
        expectEqual(W.baseline(current: "1.13.0", lastLaunched: "1.12.0", stored: "1.11.0", hasCompletedOnboarding: true),
                    "1.12.0", "基准: 刚升级,换成升级前那一版")
        expectEqual(W.baseline(current: "1.12.0", lastLaunched: "1.12.0", stored: "1.11.0", hasCompletedOnboarding: true),
                    "1.11.0", "基准: 没升级,沿用存着的")
        expectEqual(W.compare("1.12.0", "1.9.0"), .orderedDescending, "版本: 按数字比,1.12 > 1.9")
        expectEqual(W.compare("1.12", "1.12.0"), .orderedSame, "版本: 缺的段当 0")
        expectEqual(W.featureVersion("1.12.3"), "1.12", "版本: 主次版本")
    }

    do {
        print("\n== 更新后的新功能提示:挂哪些点、出不出横条 ==")
        typealias W = WhatsNew
        let releases: [(version: String, titleKeys: [String])] = [("1.12.0", ["a", "b"]), ("1.13.0", ["c"])]
        expectEqual(W.pendingKeys(releases: releases, baseline: "1.11.0", seen: []), ["a", "b", "c"], "蓝点: 基准之后各版新加的")
        expectEqual(W.pendingKeys(releases: releases, baseline: "1.12.0", seen: []), ["c"], "蓝点: 基准那一版及以前的不挂")
        expectEqual(W.pendingKeys(releases: releases, baseline: "1.11.0", seen: ["a"]), ["b", "c"], "蓝点: 看过的不挂")
        expectEqual(W.bannerPending(current: "1.12.0", baseline: "1.11.0", dismissedVersion: nil), true, "横条: 次版本升级出")
        expectEqual(W.bannerPending(current: "1.12.1", baseline: "1.12.0", dismissedVersion: nil), false, "横条: 补丁版不出")
        expectEqual(W.bannerPending(current: "1.12.0", baseline: "1.11.0", dismissedVersion: "1.12.0"), false, "横条: 这一版关过不出")
        expectEqual(W.bannerPending(current: "1.12.0", baseline: "1.12.0", dismissedVersion: nil), false, "横条: 新装的不出")
        let catalogTitles = Set(SettingsSearchCatalog.entries.map(\.titleKey))
        let listed = NewSettingsCatalog.releases.flatMap(\.titleKeys)
        expectEqual(listed.filter { !catalogTitles.contains($0) }, [], "名单: 每一项都在设置搜索目录里")
        expectEqual(NewSettingsCatalog.releases.map(\.version) == NewSettingsCatalog.releases.map(\.version)
                        .sorted { W.compare($0, $1) == .orderedAscending }, true, "名单: 按版本从旧到新")
    }

    do {
        print("\n== 更新后的新功能提示:接线 ==")
        let root = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent()
        func sourceText(_ path: String) -> String {
            (try? String(contentsOfFile: root.appendingPathComponent(path).path, encoding: .utf8)) ?? ""
        }
        let design = sourceText("lyrimuse/Settings/SettingsDesignSystem.swift")
        expectEqual(sourceHas(sourceBody("struct SettingsRow<Trailing: View>: View", in: design), "NewSettingDot(title: title)")
                    && sourceHas(sourceBody("struct SettingsCardHeader<Trailing: View>: View", in: design), "NewSettingDot(title: title)"),
                    true, "接线: 设置行和卡头标题后挂蓝点")
        expectEqual(sourceHas(sourceText("lyrimuse/SettingsView.swift"),
                              "if newSettings.pendingTabs.contains(tab.rawValue) { SettingsUnseenDot() }"), true,
                    "接线: 侧栏给含新设置的分类挂点")
        let marks = sourceText("lyrimuse/Settings/NewSettingsMarks.swift")
        expectEqual(sourceHas(marks, "content.onScrolledIntoView { NewSettingsMarks.shared.seen(title: title) }")
                    && sourceHas(marks, "content.onAppear { NewSettingsMarks.shared.seen(title: title) }"), true,
                    "接线: 滚进屏幕(浮层里出现)就记为看过")
        expectEqual(sourceHas(sourceBody("func seen(title: String)", in: marks), "pendingKeys.remove(key)"), true,
                    "接线: 看到就撤,不等离开页面")
        expectEqual(sourceHas(sourceText("lyrimuse/AppDelegate.swift"), "NewSettingsMarks.shared.start()"), true,
                    "接线: 启动时定基准、记下当前版本")
        let panel = sourceText("lyrimuse/MenuBar/MenuBarPanel.swift")
        expectEqual(sourceHas(panel, "if let version = whatsNew.bannerVersion {")
                    && sourceHas(panel, "AppActions.shared.requestSettings(.softwareUpdate)"), true,
                    "接线: 面板顶部「已更新」,点进去是软件更新页")
        let portability = sourceText("lyrimuse/Settings/ConfigPortability.swift")
        let machineLocal = portability.range(of: "private static let machineLocalDefaultsKeys").flatMap { a in
            portability.range(of: "\n    ]", range: a.upperBound..<portability.endIndex).map { portability[a.upperBound..<$0.lowerBound] }
        } ?? ""
        expectEqual(sourceHas(machineLocal, "NewSettingsMarks.Keys.lastLaunchedVersion,")
                    && sourceHas(machineLocal, "NewSettingsMarks.Keys.seen,"), true, "接线: 这几个键是机器专属的")
    }
}
