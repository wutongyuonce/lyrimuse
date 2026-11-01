import Foundation
import LyrimuseCore

func runWhatsNewTests() {
    do {
        print("\n== 本次更新:基准版本 ==")
        typealias W = WhatsNew
        expectEqual(W.baseline(current: "1.12.0", lastLaunched: nil, stored: nil, hasCompletedOnboarding: true),
                    W.legacyBaseline, "基准: 老用户第一次跑到这套代码,按从上一个版本升上来算")
        expectEqual(W.baseline(current: "1.12.0", lastLaunched: nil, stored: nil, hasCompletedOnboarding: false),
                    "1.12.0", "基准: 新装的用户就是当前版本")
        expectEqual(W.baseline(current: "1.13.0", lastLaunched: "1.12.0", stored: "1.11.0", hasCompletedOnboarding: true),
                    "1.12.0", "基准: 刚升级,换成升级前那一版")
        expectEqual(W.baseline(current: "1.12.0", lastLaunched: "1.12.0", stored: "1.11.0", hasCompletedOnboarding: true),
                    "1.11.0", "基准: 没升级,沿用存着的")
        expectEqual(W.compare("1.12.0", "1.9.0"), .orderedDescending, "版本: 按数字比,1.12 > 1.9")
        expectEqual(W.compare("1.12", "1.12.0"), .orderedSame, "版本: 缺的段当 0")
        expectEqual(W.featureVersion("1.12.3"), "1.12", "版本: 主次版本")
    }

    do {
        print("\n== 本次更新:面板顶部出不出 ==")
        typealias W = WhatsNew
        expectEqual(W.bannerPending(current: "1.12.0", baseline: "1.11.0", shownVersion: nil), true, "横条: 次版本升级出")
        expectEqual(W.bannerPending(current: "2.0.0", baseline: "1.12.3", shownVersion: nil), true, "横条: 主版本升级出")
        expectEqual(W.bannerPending(current: "1.12.1", baseline: "1.12.0", shownVersion: nil), false, "横条: 补丁版不出")
        expectEqual(W.bannerPending(current: "1.12.0", baseline: "1.11.0", shownVersion: "1.12.0"), false, "横条: 这一版出过不再出")
        expectEqual(W.bannerPending(current: "1.13.0", baseline: "1.12.0", shownVersion: "1.12.0"), true, "横条: 出过的是旧版,新版照出")
        expectEqual(W.bannerPending(current: "1.12.0", baseline: "1.12.0", shownVersion: nil), false, "横条: 新装的不出")
    }

    do {
        print("\n== 本次更新:CHANGELOG 拆成单一语言 ==")
        let changelog = """
        # Changelog

        ## v1.13.0

        New / 新功能
        - Added a thing that wraps
          onto two lines
          新增一样东西，
          折成两行
        - English only
        - Fixed DNS
          修好 VPN 下发
          DNS
          时；
          ListenBrainz 也行

        Fixed / 修复
        - Fixed a 组曲 prefix
          修复「组曲」前缀
        | Chip / 芯片 | dmg |
        |---|---|
        Not sure which one? Check your chip
        不确定该下哪个？

        **Full Changelog**: https://example.com

        ## v1.12.0

        New / 新功能
        - Old item
          旧条目
        """
        let section = ReleaseNotes.section(version: "1.13.0", in: changelog) ?? ""
        expectEqual(section.contains("Old item"), false, "分节: 到下一个 ## v 为止")
        expectEqual(ReleaseNotes.section(version: "1.14.0", in: changelog), nil, "分节: 没有这一版为 nil")
        expectEqual(ReleaseNotes.parse(section, chinese: false), [
            .init(title: "New", items: ["Added a thing that wraps onto two lines", "English only", "Fixed DNS"]),
            .init(title: "Fixed", items: ["Fixed a 组曲 prefix"]),
        ], "英文: 小节标题取左边,硬换行补空格,夹着的中文词不算中文行,表格和散句不收")
        expectEqual(ReleaseNotes.parse(section, chinese: true), [
            .init(title: "新功能", items: ["新增一样东西，折成两行", "English only", "修好 VPN 下发 DNS 时；ListenBrainz 也行"]),
            .init(title: "修复", items: ["修复「组曲」前缀"]),
        ], "中文: 小节标题取右边,中文行直接接,汉字挨着英文字母补空格、挨着标点不补,没写中文的退回英文")
    }

    do {
        print("\n== 本次更新:仓库里的 CHANGELOG 每一版都拆得开 ==")
        let repo = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent()
        let text = (try? String(contentsOf: repo.appendingPathComponent("CHANGELOG.md"), encoding: .utf8)) ?? ""
        let versions = text.components(separatedBy: "\n").filter { $0.hasPrefix("## v") }.map { String($0.dropFirst(4)) }
        expectEqual(versions.isEmpty, false, "CHANGELOG: 读得到")
        func hasCJK(_ s: String) -> Bool { s.unicodeScalars.contains { (0x4E00...0x9FFF).contains($0.value) } }
        var broken: [String] = []
        for version in versions {
            let section = ReleaseNotes.section(version: version, in: text) ?? ""
            let en = ReleaseNotes.parse(section, chinese: false), zh = ReleaseNotes.parse(section, chinese: true)
            let zhItems = zh.flatMap(\.items)
            if en.isEmpty || en.map(\.items.count) != zh.map(\.items.count) || zhItems.contains(where: { !hasCJK($0) }) {
                broken.append(version)
            }
        }
        expectEqual(broken, [], "CHANGELOG: 每一版两种语言条目一一对应、中文条目都是中文")
    }

    do {
        print("\n== 本次更新:接线 ==")
        let root = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent()
        func sourceText(_ path: String) -> String {
            (try? String(contentsOfFile: root.appendingPathComponent(path).path, encoding: .utf8)) ?? ""
        }
        expectEqual(sourceHas(sourceText("lyrimuse/AppDelegate.swift"), "WhatsNewCenter.shared.start()"), true,
                    "接线: 启动时定基准、记下当前版本")
        let panel = sourceText("lyrimuse/MenuBar/MenuBarPanel.swift")
        expectEqual(sourceHas(panel, "@State private var whatsNewVersion = WhatsNewCenter.shared.bannerVersion")
                    && sourceHas(panel, "AppActions.shared.openWhatsNew?()"), true,
                    "接线: 面板顶部「已更新」开面板那一刻取一次,点进去开卡片")
        expectEqual(sourceHas(sourceBody("private struct WhatsNewBanner: View", in: panel),
                              ".onAppear { WhatsNewCenter.shared.bannerShown() }"), true,
                    "接线: 横条摆出来就记下,只出一次")
        expectEqual(sourceHas(sourceText("lyrimuse/MenuBar/MenuBarStatusMenu.swift"), "L10n.t(\"查看本次更新…\")"), true,
                    "接线: 右键菜单有「查看本次更新…」")
        expectEqual(sourceHas(sourceText("lyrimuse/MenuBar/MenuBarSceneActions.swift"),
                              "openWindowAction(id: WhatsNewWindow.sceneID)"), true, "接线: 卡片窗口由锚点视图开")
        expectEqual(sourceHas(sourceText("lyrimuse/App.swift"), "id: WhatsNewWindow.sceneID"), true, "接线: 卡片是一扇场景窗")
        let build = sourceText("../build.sh")
        expectEqual(sourceHas(build, "cp ../CHANGELOG.md \"$APP_DIR/Contents/Resources/CHANGELOG.md\"")
                    && sourceHas(build, "cp -p ../CHANGELOG.md \"$SNAPSHOT_ROOT/CHANGELOG.md\"")
                    && sourceHas(sourceText("../package.sh"), "\"Resources/CHANGELOG.md\""), true,
                    "接线: CHANGELOG 打进包里(含快照),打包时查它在")
        let portability = sourceText("lyrimuse/Settings/ConfigPortability.swift")
        let machineLocal = portability.range(of: "private static let machineLocalDefaultsKeys").flatMap { a in
            portability.range(of: "\n    ]", range: a.upperBound..<portability.endIndex).map { portability[a.upperBound..<$0.lowerBound] }
        } ?? ""
        expectEqual(sourceHas(machineLocal, "WhatsNewCenter.Keys.lastLaunchedVersion,")
                    && sourceHas(machineLocal, "WhatsNewCenter.Keys.baseline,")
                    && sourceHas(machineLocal, "WhatsNewCenter.Keys.bannerShown,"), true, "接线: 这几个键是机器专属的")
    }
}
