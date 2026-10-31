import Foundation
import LyrimuseCore

func runTelemetryTests() {
    do {
        print("\n== 遥测:日期、心跳判据、写法 ==")
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "Asia/Shanghai")!
        // 北京时间刚过零点、UTC 还是前一天:按本地日期算。
        let justAfterMidnight = Date(timeIntervalSince1970: 1_793_377_800)
        expectEqual(Telemetry.day(justAfterMidnight, calendar: calendar), "2026-10-31", "日期: 按本地自然日")
        expectEqual(Telemetry.heartbeatDue(today: "2026-10-31", lastSentDay: nil), true, "心跳: 从没发过要发")
        expectEqual(Telemetry.heartbeatDue(today: "2026-10-31", lastSentDay: "2026-10-30"), true, "心跳: 新的一天要发")
        expectEqual(Telemetry.heartbeatDue(today: "2026-10-31", lastSentDay: "2026-10-31"), false, "心跳: 今天发过不再发")
        expectEqual(Telemetry.noticePending(hasCompletedOnboarding: true, noticeSeen: false), true, "告知: 升级上来、没看过要挂蓝点")
        expectEqual(Telemetry.noticePending(hasCompletedOnboarding: true, noticeSeen: true), false, "告知: 看过不再挂")
        expectEqual(Telemetry.noticePending(hasCompletedOnboarding: false, noticeSeen: false), false, "告知: 还没走完引导的不挂(欢迎页有开关)")

        expectEqual(Telemetry.playerTag(forBundleID: PlaybackPlayer.appleMusic.bundleIdentifier),
                    PlaybackPlayer.appleMusic.rawValue, "播放器: 内置的用目录 id")
        expectEqual(Telemetry.playerTag(forBundleID: "com.example.SomethingElse"), "other", "播放器: 别的 App 一律 other")
        expectEqual(Telemetry.playerTag(forBundleID: nil), nil, "播放器: 没有 bundle id 不记")

        expectEqual(TelemetryEnvironment.languageOnly("zh-Hans-CN"), "zh-Hans", "语言: 去掉地区、留文字")
        expectEqual(TelemetryEnvironment.languageOnly("en-US"), "en", "语言: 去掉地区")
        expectEqual(TelemetryEnvironment.languageOnly("ja"), "ja", "语言: 没地区原样")
        expectEqual(TelemetryEnvironment.osVersionString(OperatingSystemVersion(majorVersion: 26, minorVersion: 1, patchVersion: 0)),
                    "26.1", "系统版本: 补丁号为 0 不写")
        expectEqual(TelemetryEnvironment.osVersionString(OperatingSystemVersion(majorVersion: 15, minorVersion: 6, patchVersion: 1)),
                    "15.6.1", "系统版本: 带补丁号")

        expectEqual(Telemetry.redactPaths("Library missing: /Users/alice/Library/Frameworks/X.framework/X (no such file)"),
                    "Library missing: <path> (no such file)", "脱敏: 绝对路径换掉")
        expectEqual(Telemetry.redactPaths("Referenced from: '~/Applications/Lyrimuse.app/Contents/MacOS/lyrimuse'"),
                    "Referenced from: '<path>'", "脱敏: ~ 开头的路径换掉")
        expectEqual(Telemetry.redactPaths("EXC_BAD_ACCESS / KERN_INVALID_ADDRESS at 0x0"),
                    "EXC_BAD_ACCESS / KERN_INVALID_ADDRESS at 0x0", "脱敏: 孤立的斜杠不动")
        expectEqual(Telemetry.redactPaths("Namespace SIGNAL, Code 11 Segmentation fault: 11"),
                    "Namespace SIGNAL, Code 11 Segmentation fault: 11", "脱敏: 没路径原样")
        expectEqual(Telemetry.redactPaths("swift/Array.swift"), "swift/Array.swift", "脱敏: 相对路径不当成绝对路径")
    }

    do {
        print("\n== 遥测:崩溃摘要 ==")
        var summary = CrashReportSummary(fileName: "lyrimuse-2026-10-31-120000.ips")
        summary.processName = "lyrimuse"
        summary.processPath = "/Users/alice/Applications/Lyrimuse.app/Contents/MacOS/lyrimuse"
        summary.version = "1.9.0"
        summary.osVersion = "macOS 26.1 (25B78)"
        summary.timestamp = "2026-10-31 12:00:00.00 +0800"
        summary.exceptionType = "EXC_BAD_ACCESS"
        summary.exceptionSignal = "SIGSEGV"
        summary.terminationIndicator = "Library missing"
        summary.terminationReasons = ["Library not loaded: /Users/alice/x.dylib", "a", "b", "c"]
        summary.frames = (0..<20).map { (i: Int) in
            CrashReportSummary.Frame(imageName: "lyrimuse", symbol: "f\(i)", imageOffset: i,
                                     sourceFile: "/Users/alice/src/Foo.swift", sourceLine: 10)
        }
        let env = TelemetryEnvironment(appVersion: "1.9.0", build: "1.9.0.1000", osVersion: "26.1", arch: "arm64",
                                       uiLanguage: "zh-hans", systemLanguage: "zh-Hans")
        let id = Telemetry.reportID(fileName: summary.fileName)
        expectEqual(id.count, 16, "去重键: 16 位十六进制")
        expectEqual(id == Telemetry.reportID(fileName: summary.fileName), true, "去重键: 同一文件名同一个键")
        let crash = TelemetryCrash(summary: summary, reportID: id, executableName: "lyrimuse", installID: "i", env: env)
        expectEqual(crash.process, "app", "进程: App 本体")
        expectEqual(crash.terminationReasons, ["Library not loaded: <path>", "a", "b"], "终止原因: 脱敏、最多三条")
        expectEqual(crash.frames.count, Telemetry.maxCrashFrames, "帧: 截到上限")
        expectEqual(crash.frames.first, TelemetryCrash.Frame(image: "lyrimuse", symbol: "f0", offset: 0), "帧: 只带镜像、符号、偏移")
        let json = (try? JSONEncoder().encode(crash)).flatMap { String(data: $0, encoding: .utf8) } ?? ""
        expectEqual(json.contains("/Users") || json.contains("alice") || json.contains("Foo.swift"), false,
                    "崩溃负载: 不带路径、用户名、源码文件")
        expectEqual(json.contains(summary.fileName), false, "崩溃负载: 不带报告文件名")
        var engine = summary
        engine.processName = "lyrimuse-engine"
        expectEqual(TelemetryCrash(summary: engine, reportID: id, executableName: "lyrimuse", installID: "i", env: env).process,
                    "engine", "进程: 引擎")
    }

    do {
        print("\n== 遥测:每日活动记账 ==")
        // 用本进程自己的偏好域、带 pid 的键:另开 suite 会在 ~/Library/Preferences 留下一个文件。
        let key = "selftest.telemetryActivity.\(getpid())"
        defer { UserDefaults.standard.removeObject(forKey: key) }
        let activity = TelemetryActivity(defaults: .standard, key: key)
        let day1 = Date(timeIntervalSince1970: 1_793_300_000)
        let day2 = day1.addingTimeInterval(86_400)
        let music = PlaybackPlayer.appleMusic.bundleIdentifier
        activity.notePlayer(bundleID: music, now: day1)
        activity.noteHang(now: day1)
        expectEqual(activity.completedDays(before: "9999-12-31"), [], "关着时什么都不记")
        activity.isRecording = true
        activity.notePlayer(bundleID: music, now: day1)
        activity.notePlayer(bundleID: "com.example.Other", now: day1)
        activity.notePlayer(bundleID: music, now: day1)
        activity.noteHang(now: day1)
        activity.noteHang(now: day1)
        activity.notePlayer(bundleID: music, now: day2)
        let d1 = Telemetry.day(day1), d2 = Telemetry.day(day2)
        expectEqual(activity.completedDays(before: d2),
                    [TelemetryActivityDay(day: d1, playersPlayed: [PlaybackPlayer.appleMusic.rawValue, "other"].sorted(), hangs: 2)],
                    "记账: 当天放过的播放器去重、卡顿计数")
        expectEqual(activity.completedDays(before: d1), [], "记账: 今天的不算完成")
        activity.remove(days: [d1])
        expectEqual(activity.completedDays(before: "9999-12-31").map(\.day), [d2], "记账: 发过的那天删掉")
        for i in 2..<7 { activity.noteHang(now: day1.addingTimeInterval(Double(i) * 86_400)) }
        expectEqual(activity.completedDays(before: "9999-12-31").count, TelemetryActivity.maxStoredDays, "记账: 最多留几天")
        activity.clear()
        expectEqual(activity.completedDays(before: "9999-12-31"), [], "记账: 关掉遥测时清空")
    }

    do {
        print("\n== 遥测:负载键名(跟 Worker 对得上)==")
        let env = TelemetryEnvironment(appVersion: "1", build: "1", osVersion: "26.1", arch: "arm64",
                                       uiLanguage: "en", systemLanguage: "en")
        let features = TelemetryFeatures(surfaces: ["notch", "overlay"], playersSelected: ["auto"], lyricsSourceMode: "smart",
                                         lyricsSourcesEnabled: 15, translation: true, romanization: false,
                                         machineTranslation: false, accounts: [], showInDock: true, launchAtLogin: true,
                                         betaUpdates: false)
        let beat = TelemetryHeartbeat(installID: "i", day: "2026-10-31", env: env, features: features,
                                      activity: [TelemetryActivityDay(day: "2026-10-30")])
        func keys(_ value: some Encodable) -> [String] {
            guard let data = try? JSONEncoder().encode(value),
                  let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else { return [] }
            return obj.keys.sorted()
        }
        let beatKeys = keys(beat)
        expectEqual(beatKeys, ["activity", "day", "env", "features", "install_id", "v"], "心跳: 顶层键")
        expectEqual(keys(env), ["app_version", "arch", "build", "os_version", "system_language", "ui_language"], "心跳: env 键")
        let root = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent()
        let worker = (try? String(contentsOfFile: root.appendingPathComponent("../../telemetry-worker/src/index.js").path,
                                  encoding: .utf8)) ?? ""
        let fields = keys(env) + keys(features) + ["install_id", "report_id", "players_played", "hangs",
                                                   "exception_type", "termination_indicator", "frames"]
        let missing = fields.filter { !worker.contains($0) }
        expectEqual(missing, [], "Worker: 认得 App 发的每个字段")
    }

    do {
        print("\n== 遥测:接线 ==")
        let root = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent()
        func sourceText(_ path: String) -> String {
            (try? String(contentsOfFile: root.appendingPathComponent(path).path, encoding: .utf8)) ?? ""
        }
        expectEqual(sourceHas(sourceText("lyrimuse/AppDelegate.swift"), "TelemetryService.shared.start()"), true,
                    "接线: 启动时开始")
        expectEqual(sourceHas(sourceText("LyrimuseCore/Local/LocalPlaybackSource.swift"),
                              "TelemetryActivity.shared.notePlayer(bundleID: bundleID)"), true, "接线: 换播放器记一笔")
        expectEqual(sourceHas(sourceText("LyrimuseCore/Diagnostics/MainThreadWatchdog.swift"),
                              "TelemetryActivity.shared.noteHang()"), true, "接线: 主线程卡住记一笔")
        expectEqual(sourceHas(sourceText("lyrimuse/SettingsView.swift"), "Toggle(\"\", isOn: $settings.telemetryEnabled)"), true,
                    "接线: 设置页开关")
        expectEqual(sourceHas(sourceText("lyrimuse/OnboardingView.swift"), "isOn: $settings.telemetryEnabled"), true,
                    "接线: 引导欢迎页开关")
        let settings = sourceText("lyrimuse/Settings/AppSettings.swift")
        expectEqual(sourceHas(settings, "telemetryEnabled = (defaults.object(forKey: Keys.telemetryEnabled) as? Bool) ?? true"),
                    true, "接线: 默认开")
        expectEqual(sourceHas(settings, "TelemetryService.shared.preferenceChanged(enabled: telemetryEnabled)"), true,
                    "接线: 改开关通知发送端")
        let service = sourceText("lyrimuse/Settings/TelemetryService.swift")
        expectEqual(sourceHas(sourceBody("private func apply(enabled: Bool)", in: service), "TelemetryActivity.shared.clear()"),
                    true, "接线: 关掉时清空活动")
        expectEqual(sourceHas(sourceBody("private func post<T: Encodable>", in: service),
                              "guard AppSettings.shared.telemetryEnabled else { return false }"), true, "接线: 关着不发")
        expectEqual(sourceHas(sourceText("lyrimuse/Settings/ConfigPortability.swift"), "TelemetryService.Keys.installID,"), true,
                    "接线: 安装 ID 不随配置搬家")
        expectEqual(sourceHas(sourceText("lyrimuse/Settings/ConfigPortability.swift"), "TelemetryNotice.seenKey,"), true,
                    "告知接线: 已告知标记不随配置搬家")
        let settingsView = sourceText("lyrimuse/SettingsView.swift")
        expectEqual(sourceHas(settingsView, "if tab == .about, telemetryNotice.pending { SettingsUnseenDot() }"), true,
                    "告知接线: 侧栏「关于」挂蓝点")
        expectEqual(sourceHas(settingsView, "unseenDot: telemetryNotice.pending")
                    && sourceHas(settingsView, ".onScrolledIntoView { telemetryNotice.rowShown() }"), true,
                    "告知接线: 那一行挂蓝点,滚进屏幕记为已告知")
        expectEqual(sourceHas(settingsView, ".onDisappear { telemetryNotice.leftAbout() }"), true,
                    "告知接线: 离开关于页撤掉蓝点")
        expectEqual(sourceHas(sourceText("lyrimuse/OnboardingView.swift"), ".onAppear { TelemetryNotice.shared.seenInOnboarding() }"),
                    true, "告知接线: 欢迎页出现即记为已告知")
        let notice = sourceBody("final class TelemetryNotice", in: service)
        expectEqual(sourceHas(sourceBody("func rowShown()", in: notice), "pending = false"), false,
                    "告知接线: 刚滚进屏幕不撤蓝点,留到离开关于页")
    }
}
