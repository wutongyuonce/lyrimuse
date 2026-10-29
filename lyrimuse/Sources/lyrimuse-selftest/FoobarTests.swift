import Foundation
import LyrimuseCore

/// foobar2000:开播锚点随封面重报那一份不采信、按开着的文件认当前文件、光盘占位图、没封面时用匹配到的、接线契约。
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
    }
}
