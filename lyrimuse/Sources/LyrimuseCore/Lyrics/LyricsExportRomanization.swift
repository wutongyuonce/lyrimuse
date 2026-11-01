import Foundation

/// 导出用的最终读音:按屏上同一套逐行规则(`LyricsSyncEngine`,读音语言全开、日文用罗马字)算出每一行显示的读音,
/// 写成 `.roma.lrc`。缓存里的 `lyrics_roma` 是歌词源给的原样或预生成的那份,屏上那几道修正(源读音覆盖不到八成整份
/// 不用、残留原文、体系用错、按内容重新对行、去段落标签、形近字母行不标)只在显示时做;导出照原样写就跟屏上对不上。
/// 引擎经 `lyrics-romanize` 交来,算出跟原样逐行一致就回 `.sameAsStored`,只有真不同的那几百首才另存。见 10 章决策 66。
public enum LyricsExportRomanization {
    /// 算导出读音要的全部输入,跟 App 显示时喂给 `LyricsSyncEngine.load` 的同一组(译文不影响读音,不要)。
    public struct Input: Sendable {
        public var lyrics: String
        public var roma: String
        public var yrc: String
        public var bg: String
        public var kanaDonor: String
        public var title: String
        public var artist: String
        public var headerAliases: [LRCParser.HeaderIdentity]
        public var cantonese: Bool
        public var hokkien: Bool

        public init(lyrics: String, roma: String, yrc: String = "", bg: String = "", kanaDonor: String = "",
                    title: String = "", artist: String = "", headerAliases: [LRCParser.HeaderIdentity] = [],
                    cantonese: Bool = false, hokkien: Bool = false) {
            self.lyrics = lyrics
            self.roma = roma
            self.yrc = yrc
            self.bg = bg
            self.kanaDonor = kanaDonor
            self.title = title
            self.artist = artist
            self.headerAliases = headerAliases
            self.cantonese = cantonese
            self.hokkien = hokkien
        }
    }

    public enum Result: Equatable, Sendable {
        /// 每一行显示的读音都跟缓存里那份对得上:照原样导出。
        case sameAsStored
        /// 导出这一份;空串 = 整首屏上不显示读音,不该有 `.roma.lrc`。
        case final(String)
    }

    /// 规则版本:屏上怎么取读音(`LyricsSyncEngine` 的读音来源链、`Romanizer`、`LyricsRomanization` 的预处理)改了就 +1,
    /// 引擎据此把旧规则算的导出读音重算一遍。跟引擎的 migrationStaleGeneratedRomaVersion 是两回事:那个只管预生成的那份。
    public static let rulesVersion = 1

    /// 屏上「读音语言全开」的那一档。
    public static let allScripts: RomanizationScripts = RomanizationScripts.default.union(.world)

    public static func compute(_ input: Input) -> Result {
        guard !(input.lyrics.isEmpty && input.yrc.isEmpty) else {
            return input.roma.isEmpty ? .sameAsStored : .final("")
        }
        let engine = LyricsSyncEngine()
        _ = engine.load(lyrics: input.lyrics, lyricsTr: "", lyricsRoma: input.roma, lyricsYRC: input.yrc,
                        lyricsBG: input.bg, kanaDonor: input.kanaDonor,
                        trackTitle: input.title, trackArtist: input.artist, headerAliases: input.headerAliases,
                        romanizationScripts: allScripts, japaneseReading: .romaji,
                        songIsCantonese: input.cantonese, songIsHokkien: input.hokkien)
        let shown = engine.allLines(idPrefix: "").map { window -> (timeMs: Int, text: String, reading: String) in
            let line = window.line
            let text = line.mainText ?? line.words?.map(\.text).joined() ?? ""
            return (window.timeMs, text, collapsedWhitespace(line.romanization ?? ""))
        }
        if matchesStored(shown, lyrics: input.lyrics, yrc: input.yrc, roma: input.roma) { return .sameAsStored }
        let tags = lineTags(input.lyrics)
        let out = shown.compactMap { row -> String? in
            guard !row.reading.isEmpty else { return nil }
            return (tags[row.timeMs] ?? tag(row.timeMs)) + row.reading
        }
        return .final(out.joined(separator: "\n"))
    }

    /// 屏上每一行的读音跟缓存那份贴到同一行上的内容逐行比(同一时刻的那一行、按内容找、700ms 内就近,依次退):
    /// 屏上有读音、存的没有或不同,屏上没读音、存的这一行却有,都算对不上。只差空白不算。
    static func matchesStored(_ shown: [(timeMs: Int, text: String, reading: String)],
                              lyrics: String, yrc: String, roma: String) -> Bool {
        let stored = LRCParser.parse(roma)
        var byTime: [Int: String] = [:]
        for line in stored { byTime[line.timeMs] = line.text }
        func nearest(_ t: Int) -> String? {
            if let s = byTime[t] { return s }
            var best: (gap: Int, text: String)?
            for line in stored where abs(line.timeMs - t) <= 700 {
                let gap = abs(line.timeMs - t)
                if best == nil || gap < best!.gap { best = (gap, line.text) }
            }
            return best?.text
        }
        var byText: [String: String] = [:]
        for line in LRCParser.parse(lyrics) {
            if let r = nearest(line.timeMs) { byText[contentKey(line.text)] = r }
        }
        for line in YRCParser.parse(yrc) {
            let key = contentKey(line.words.map(\.text).joined())
            if byText[key] == nil, let r = nearest(line.timeMs) { byText[key] = r }
        }
        for row in shown {
            if row.reading.isEmpty {
                // 屏上不显示读音的行(英文行、形近字母行、署名行):原样文件里同一时刻有一行读音就是多出来的。只认同一时刻,
                // 按内容、就近找会把隔壁那句的读音算到它头上。
                if !collapsedWhitespace(byTime[row.timeMs] ?? "").isEmpty { return false }
                continue
            }
            // 先认同一时刻那一行:导出文件按时间逐行对齐,同一句唱两遍、两遍读音不同时按内容找只会拿到其中一遍。
            let storedReading = collapsedWhitespace(byTime[row.timeMs] ?? byText[contentKey(row.text)] ?? nearest(row.timeMs) ?? "")
            if storedReading != row.reading { return false }
        }
        return true
    }

    private static func contentKey(_ s: String) -> String {
        String(s.filter { !$0.isWhitespace && !$0.isPunctuation })
    }

    private static func collapsedWhitespace(_ s: String) -> String {
        s.split(whereSeparator: { $0.isWhitespace }).joined(separator: " ")
    }

    /// 正文每一行行首的时间标签原文,按毫秒查:导出的读音沿用正文那一行的写法,两份文件逐行对得齐。
    private static func lineTags(_ lyrics: String) -> [Int: String] {
        var out: [Int: String] = [:]
        for raw in lyrics.split(separator: "\n", omittingEmptySubsequences: true) {
            let line = String(raw)
            guard line.hasPrefix("["), let close = line.firstIndex(of: "]"),
                  let ms = LRCParser.firstTimestampMs(line) else { continue }
            if out[ms] == nil { out[ms] = String(line[...close]) }
        }
        return out
    }

    /// 正文里没有这一刻的标签(逐字那份的行起点)时现写一个:毫秒是 10 的整数倍写两位,否则写三位。
    public static func tag(_ ms: Int) -> String {
        let t = max(0, ms)
        let minutes = t / 60_000, seconds = (t / 1000) % 60, frac = t % 1000
        if frac % 10 == 0 { return String(format: "[%02d:%02d.%02d]", minutes, seconds, frac / 10) }
        return String(format: "[%02d:%02d.%03d]", minutes, seconds, frac)
    }
}
