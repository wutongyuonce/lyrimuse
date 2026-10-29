import Foundation
import LyrimuseCore

/// foobar2000:开播锚点随封面重报那一份不采信、按开着的文件认当前文件、光盘占位图、没封面时用匹配到的、播放顺序与音量、
/// 失焦时的菜单、播放列表、没写标签的文件名、接线契约。
/// 锚点照真机录到的数(media-control `--micros`,咔哒文件 2 秒一声,第一份锚点跟出声差 0.03~0.06 秒)。
@MainActor
func runFoobarTests() {
    typealias MC = MediaControlClient
    let foobar = PlaybackPlayer.foobar2000.bundleIdentifier
    func micros(_ seconds: Double) -> String { MediaControlMicros.timestampPrefix + String(Int64((seconds * 1_000_000).rounded())) }

    // ---- 开播锚点随封面重报 ----
    do {
        let startTS = 1_793_281_213.250
        let start = MC.PlayingAnchor(track: "Lyrimuse格式测试|稻香 AAC", elapsed: 0, timestamp: micros(startTS),
                                     instant: Date(timeIntervalSince1970: startTS))
        let now = Date(timeIntervalSince1970: 1_793_281_213.9)
        func stale(_ elapsed: Double, at ts: Double, bundleID: String? = PlaybackPlayer.foobar2000.bundleIdentifier,
                   last: MC.PlayingAnchor? = nil, track: String = "Lyrimuse格式测试|稻香 AAC") -> Bool {
            MC.isStaleAnchorRepublish(last: last ?? start, track: track, elapsed: elapsed, timestamp: micros(ts),
                                      duration: 223.453, bundleID: bundleID, now: now)
        }
        expectEqual(stale(0.083117, at: 1_793_281_213.883), true, "foobar 重报: 封面到时位置 0.08 秒、起播点晚 0.55 秒的那份不采信(实测样本)")
        expectEqual(stale(0, at: 1_793_281_213.792), true, "foobar 重报: 位置报成 0 的那份按 republishesZeroAnchor 不采信(实测样本)")
        expectEqual(PlaybackPlayer.foobar2000.republishesZeroAnchor, true, "foobar 重报: 开播 0 锚点会重报")
        expectEqual(stale(0.083117, at: 1_793_281_213.883, bundleID: PlaybackPlayer.soda.bundleIdentifier), false,
                    "foobar 重报: 位置不为 0 的那条只对 foobar2000")
        expectEqual(stale(0.35, at: 1_793_281_214.1), false, "foobar 重报: 位置到 0.3 秒以上的是真锚点(拖动)")
        expectEqual(stale(0.2, at: 1_793_281_215.0), false, "foobar 重报: 推出的起播点晚过 1.5 秒的是真锚点")
        expectEqual(stale(0.2, at: 1_793_281_213.4), false, "foobar 重报: 推出的起播点没往后挪的是真锚点")
        let playing = MC.PlayingAnchor(track: "Lyrimuse格式测试|稻香 AAC", elapsed: 8.5, timestamp: micros(startTS),
                                       instant: Date(timeIntervalSince1970: startTS))
        expectEqual(stale(0.083117, at: 1_793_281_213.883, last: playing), false, "foobar 重报: 上一份不是开播锚点时不认")
        expectEqual(stale(0.083117, at: 1_793_281_213.883, track: "Lyrimuse格式测试|稻香 FLAC"), false, "foobar 重报: 换了歌不认")
        expectEqual(MC.isFoobarArtworkRepublish(last: start, elapsed: 0, timestamp: micros(1_793_281_213.792), bundleID: foobar),
                    false, "foobar 重报: 位置为 0 的不归这条(归 republishesZeroAnchor 那条)")
    }

    // ---- 单曲循环绕回开头 ----
    do {
        func loop(_ position: Double?, playing: Bool? = true, bundle: String? = PlaybackPlayer.foobar2000.bundleIdentifier,
                  duration: Double? = 12) -> Double? {
            MC.foobarLoopPosition(bundleID: bundle, playing: playing, position: position, duration: duration)
        }
        expectEqual(loop(26.5).map { ($0 * 1000).rounded() / 1000 }, 2.5, "foobar 循环: 第二圈起不再发锚点,越过曲长按曲长取余(实测每圈正好 12.000 秒)")
        expectEqual(loop(12.1), nil, "foobar 循环: 第一圈它自己会重发锚点(晚 0.07~0.13 秒),余量之内不动")
        expectEqual(loop(11.9), nil, "foobar 循环: 没越过曲长不动")
        expectEqual(loop(26.5, playing: false), nil, "foobar 循环: 暂停着不动")
        expectEqual(loop(26.5, bundle: PlaybackPlayer.pinePlayer.bundleIdentifier), nil, "foobar 循环: 只对 foobar2000")
        expectEqual(loop(26.5, duration: nil), nil, "foobar 循环: 没有时长(电台)不动")
        let t = Date(timeIntervalSince1970: 1_000)
        func check(_ position: Double) -> Double? {
            MC.foobarLoopCheckAt(bundleID: foobar, playing: true, position: position, duration: 12, now: t).map { $0.timeIntervalSince(t) }
        }
        expectEqual(check(11).map { ($0 * 1000).rounded() / 1000 }, 1.15, "foobar 循环: 离曲尾不到 3 秒时排一次补查,刚过绕回点那一刻")
        expectEqual([check(5), check(12.2)].map { $0 == nil }, [true, true], "foobar 循环: 离曲尾还远、已经过了绕回点都不排")
    }

    // ---- 当前文件、封面、轮询 ----
    do {
        expectEqual(PlayerOpenFile.bundleIDs.contains(foobar), true, "foobar 当前文件: 按它开着的文件认,路径交引擎读 .lrc")
        expectEqual(CoverArtReplacementGate.missingArtworkUsesMatchedCover(bundleID: foobar), true,
                    "foobar 封面: 文件没封面时用匹配到的")
        let disc = KnownPlaceholderArtwork.entries.filter { $0.player == foobar }
        expectEqual(disc.map(\.byteCount), [26589], "foobar 封面: 光盘占位图登记了一张")
        expectEqual(LocalPlaybackSource.pollsFastWhilePaused(bundleID: foobar, pausedFor: 30), true,
                    "foobar 轮询: 刚暂停那段按播放档轮询(恢复只有事件流一条即时信号)")
        expectEqual(LocalPlaybackSource.positionSourceTier(forBundleID: foobar), .precise, "foobar 位置: 锚点准,归 precise")
        expectEqual(TornTrackHold.arrivesWhole(bundle: foobar), true, "foobar 换歌: 一帧报齐,不按撕裂快照按住")
    }

    // ---- 播放顺序、音量、失焦时的菜单 ----
    do {
        typealias F = FoobarMenuControl
        expectEqual(["Default", "Repeat (playlist)", "Repeat (track)", "Random", "Shuffle (tracks)", "Shuffle (albums)",
                     "Shuffle (folders)", "Stop After Current"].map { F.mode(checkedTitle: $0)?.rawValue ?? "nil" },
                    ["list", "repeatAll", "repeatOne", "shuffle", "shuffle", "shuffle", "shuffle", "nil"],
                    "foobar 顺序: Order 七项对到四档,Random 和三种 Shuffle 都是随机")
        expectEqual(F.titleToPress(for: .shuffle, current: .list), "Shuffle (tracks)", "foobar 顺序: 开随机按 Shuffle (tracks)")
        expectEqual(F.titleToPress(for: .shuffle, current: .shuffle), nil, "foobar 顺序: 已经是随机(哪一种都算)不按")
        expectEqual(F.titleToPress(for: .list, current: .repeatOne), "Default", "foobar 顺序: 关循环按 Default")
        expectEqual(F.titleToPress(for: .repeatOne, current: nil), "Repeat (track)", "foobar 顺序: 读不出当前档也按")
        let t0 = Date(timeIntervalSince1970: 1_000)
        expectEqual(F.effective(read: .list, pending: (.repeatAll, t0), now: t0.addingTimeInterval(1)), .repeatAll,
                    "foobar 顺序: 刚按过 1 秒内以按下去的为准(勾 0.9 秒后才跟上)")
        expectEqual(F.effective(read: .list, pending: (.repeatAll, t0), now: t0.addingTimeInterval(3.5)), .list,
                    "foobar 顺序: 过了 3 秒照读到的勾")
        expectEqual(F.options, [.shuffle, .repeatAll, .repeatOne], "foobar 顺序: 随机、列表循环、单曲循环都有")
        let sliders = [PineVolume.Slider(min: 0, max: 223.453, value: 8), PineVolume.Slider(min: 0, max: 100, value: 10)]
        expectEqual(FoobarVolume.volumeIndex(sliders), 1, "foobar 音量: 取值 0~100 的那根")
        expectEqual(FoobarVolume.volumeIndex([PineVolume.Slider(min: 0, max: 100, value: 3), PineVolume.Slider(min: 0, max: 100, value: 10)]),
                    nil, "foobar 音量: 这首恰好 100 秒、两根都是 0~100 时不猜")
        expectEqual([FoobarVolume.percent(10.00000048932983), FoobarVolume.percent(-1), FoobarVolume.percent(130)], [10, 0, 100],
                    "foobar 音量: 四舍五入、越界夹住")
        expectEqual(MusicPlaybackController.supportsExtendedControls(.foobar2000), true, "foobar 顺序: 歌词窗口出随机、循环、音量")
        expectEqual(MusicPlaybackController.supportsRepeatOne(.foobar2000), true, "foobar 顺序: 有单曲循环")
        expectEqual(PlaybackPlayer.foobar2000.accessibilityUse == .pressesPlayMode, true, "foobar 权限: 跟 reAMP 同一条说明")
        typealias PMC = PlaybackMenuControl
        expectEqual(["Pause", "Play", "Next", "Previous", "Stop", "Random"].map { PMC.command(forMenuTitle: $0, player: .foobar2000)?.rawValue ?? "nil" },
                    ["toggle-play-pause", "nil", "next-track", "previous-track", "nil", "nil"],
                    "foobar 菜单: 播放 / 暂停只按 Pause(暂停着按也是接着放),Play 不认")
        expectEqual(PMC.players.contains(.foobar2000), true, "foobar 菜单: 失焦时能按菜单")
        expectEqual([PMC.startTitle(for: .foobar2000), PMC.startTitle(for: .pinePlayer), PMC.startTitle(for: .qqMusic)], ["Play", nil, nil],
                    "foobar 菜单: 停住 / 冷启动后开播按 Play(Pause 不理),别家照旧按播放 / 暂停")
        expectEqual(MusicPlaybackController.controlRoute(exclusivelyAppleMusic: false, focusFallback: nil, focusHeldElsewhere: true,
                                                         directQueryFallback: .foobar2000, mediaControlCommand: "next-track"),
                    .menuAccessibility(.foobar2000, .nextTrack), "foobar 菜单: 焦点被占时下一首按它的菜单")
    }

    // ---- 播放列表、没写标签的文件 ----
    do {
        typealias PL = FoobarPlaylist
        let text = "\u{FEFF}file:///Users/a/Music/Lyrimuse测试/reAMP/Click A.wav\nfile:///Users/a/Music/100%/b.flac\n\nhttp://x/y.mp3\n/Users/a/c.mp3\n"
        expectEqual(PL.paths(fplite: text), ["/Users/a/Music/Lyrimuse测试/reAMP/Click A.wav", "/Users/a/Music/100%/b.flac", "/Users/a/c.mp3"],
                    "foobar 列表: 去掉 BOM 和 file://,空格、中文、百分号原样留,网络地址跳过")
        let lists = [["/a/1.mp3", "/a/2.mp3"], ["/b/1.mp3", "/b/2.mp3", "/b/3.mp3"]]
        expectEqual(PL.pick(lists, current: "/b/2.mp3")?.count, 3, "foobar 列表: 挑含着当前文件的那一份")
        expectEqual(PL.pick(lists, current: "/c/9.mp3") == nil && PL.pick(lists, current: nil) == nil, true,
                    "foobar 列表: 哪份都不含、不知道当前文件时不交")
        let tagged = PL.track(path: "/m/x.mp3", tags: LocalAudioFileTags(title: "稻香", artist: "周杰伦", album: "魔杰座", duration: 223.4))
        expectEqual("\(tagged.title)|\(tagged.artist)|\(tagged.album)", "稻香|周杰伦|魔杰座", "foobar 列表: 有标签用标签")
        let untagged = PL.track(path: "/m/无标签 Mezzamo - Happy Boy.mp3", tags: LocalAudioFileTags(title: "", artist: "", album: "", duration: 40))
        expectEqual("\(untagged.title)|\(untagged.artist)|\(untagged.duration)", "Happy Boy|无标签 Mezzamo|40.0",
                    "foobar 列表: 没写标签的按文件名拆,跟真放到时同一份")
        expectEqual([PL.repeating(.shuffle), PL.repeating(.repeatAll), PL.repeating(.repeatOne), PL.repeating(.list), PL.repeating(nil)],
                    ["shuffle", "all", "one", "off", nil], "foobar 列表: 菜单的档交给引擎的写法")
        let id = PlayerOpenFile.fileNameIdentity(reportedArtist: "", reportedTitle: "无标签 Mezzamo - Happy Boy",
                                                 fileName: "无标签 Mezzamo - Happy Boy.mp3", duration: 40, acceptsBaseName: true)
        expectEqual("\(id?.artist ?? "nil")|\(id?.title ?? "nil")", "无标签 Mezzamo|Happy Boy", "foobar 没写标签: 不带扩展名的文件名也拆")
        expectEqual(PlayerOpenFile.fileNameIdentity(reportedArtist: "", reportedTitle: "无标签 Mezzamo - Happy Boy",
                                                    fileName: "无标签 Mezzamo - Happy Boy.mp3", duration: 40) == nil, true,
                    "foobar 没写标签: 别的播放器照旧只认带扩展名的")
        expectEqual(PlayerOpenFile.underscoringControls("回车检查\r\n第二行\t制表"), "回车检查__第二行_制表",
                    "foobar 标签里的控制字符: 各换成一个下划线,回车换行是两个")
        let multiline = LocalAudioFileTags(title: "换行检查\n第二行", artist: "测试\n歌手", album: "测试专辑", duration: 40)
        expectEqual(PlayerOpenFile.fileMatches(reportedArtist: "测试_歌手", reportedTitle: "换行检查_第二行", reportedDuration: 40,
                                               tags: multiline, fileName: "newline.mp3", underscoresControls: true), true,
                    "foobar 标签带换行: 它报成下划线也认得出是这个文件")
        expectEqual(PlayerOpenFile.fileMatches(reportedArtist: "测试_歌手", reportedTitle: "换行检查_第二行", reportedDuration: 40,
                                               tags: multiline, fileName: "newline.mp3"), false,
                    "foobar 标签带换行: 别的播放器照旧不把下划线当换行")
    }

    // ---- 接线契约 ----
    do {
        let root = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent()
        func sourceText(_ path: String) -> String {
            (try? String(contentsOfFile: root.appendingPathComponent(path).path, encoding: .utf8)) ?? ""
        }
        let client = sourceText("LyrimuseCore/Local/MediaControlClient.swift")
        expectEqual(sourceHas(client, "if isFoobarArtworkRepublish(last: last, elapsed: elapsed, timestamp: timestamp, bundleID: bundleID) { return true }"),
                    true, "foobar 接线: 重报判定排在「位置逐毫秒相等」那道之前")
        expectEqual(sourceHas(client, "if bundleID == PinePlayerInfo.bundleID || bundleID == PlaybackPlayer.foobar2000.bundleIdentifier,"),
                    true, "foobar 接线: 推的是光盘时先找同目录的图")
        let idle = sourceText("lyrimuse/UI/IdlePlaybackActions.swift")
        expectEqual(sourceHas(idle, "let outcome = PlaybackMenuControl.pressStart(for: player)"), true,
                    "foobar 接线: 停播页「继续播放」按开播那一项")
        expectEqual(sourceHas(idle, "return PlaybackMenuControl.isStopped(for: player) == false"), true,
                    "foobar 接线: 按了还停着(列表是空的)就算没放起来,退回打开它")
        let openFile = sourceText("LyrimuseCore/Local/PlayerOpenFile.swift")
        expectEqual(sourceHas(openFile, "foobar: bundleID == PlaybackPlayer.foobar2000.bundleIdentifier,"), true,
                    "foobar 接线: 按开着的文件认曲目时打开 foobar 那两条(不带扩展名的文件名、控制字符换下划线)")
        expectEqual(sourceHas(openFile, "fileName: fileName, underscoresControls: foobar)"), true,
                    "foobar 接线: 找文件时把控制字符那条交给比对")
        expectEqual(sourceHas(openFile, "guard let snapshot, !SourceKnockout.isOut(.playerFile),"), true,
                    "foobar 接线: 开着的文件那条腿能拔(player-file)")
        expectEqual(sourceHas(openFile, "guard !SourceKnockout.isOut(.playerFile), let bundleID, bundleIDs.contains(bundleID), let rawTitle"), true,
                    "foobar 接线: 拔了 player-file 不按开着的文件找同目录封面")
        expectEqual(sourceHas(openFile, "guard !SourceKnockout.isOut(.playerFile), let bundleID, bundleIDs.contains(bundleID), !title.isEmpty"), true,
                    "foobar 接线: 拔了 player-file 不写 local_file")
        expectEqual(sourceHas(openFile, "out.republishDueBy = snapshot.republishDueBy"), true,
                    "foobar 接线: 换成标签那份要带上补查时刻,不然曲尾那一查排不上")
        expectEqual(sourceHas(client, "elapsedTime: amazonPosition ?? radioPosition ?? pine?.position ?? foobarLoop ?? elapsed,"), true,
                    "foobar 接线: 解码那一拍位置换成取余后的")
        expectEqual(sourceHas(openFile, "acceptsBaseName: foobar)"), true,
                    "foobar 接线: 认当前文件时没写标签的按不带扩展名的文件名拆")
    }
}
