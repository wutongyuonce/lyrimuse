import Foundation

/// 听歌报告用哪种语言写:App 的界面语言(`L10n.current`:zh-hans / zh-hant / en),写进 features.json 的
/// `report_language` 给引擎(digestlang.go)。认不得的按简体。
public enum ReportLanguage: String, CaseIterable, Sendable {
    case zhHans = "zh-hans", zhHant = "zh-hant", en

    /// features.json 里的键名,同引擎 featureFlagsFile.ReportLanguage 的 json 标签。
    public static let featuresKey = "report_language"

    public init(_ raw: String) {
        self = ReportLanguage(rawValue: raw.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()) ?? .zhHans
    }

    public var words: ReportWords { ReportWords(table: ReportWords.table[self] ?? [:]) }
}

/// 报告里的固定用语(标题、第一行、小节名、三套预设模板、各平台排版的标签),同引擎 digestWordTable,两边对得上靠共用样例
/// shared/testdata/notify/words.json。每一句里的 `{名字}` 由调用处换成值,换法同引擎 digestWords.f。见 15 章决策 98。
public struct ReportWords: Sendable {
    let table: [String: String]

    public subscript(_ key: String) -> String { table[key] ?? "" }

    /// 取 key 那一句,把 `{名字}` 换成值。
    public func f(_ key: String, _ values: [(String, String)] = []) -> String {
        values.reduce(self[key]) { $0.replacingOccurrences(of: "{\($1.0)}", with: $1.1) }
    }

    /// 带数量 `{n}` 的那一句;数量是 1 且表里有 `key_one` 时用它(英文单复数)。
    public func count(_ key: String, _ n: Int, _ values: [(String, String)] = []) -> String {
        f(n == 1 && table[key + "_one"] != nil ? key + "_one" : key, [("n", String(n))] + values)
    }

    /// 标题里的日期,同引擎 digestWords.date。
    public func date(_ date: Date, _ calendar: Calendar) -> String {
        let c = calendar.dateComponents([.month, .day], from: date)
        let month = c.month ?? 1, day = c.day ?? 1
        return f("date", [("mm", String(format: "%02d", month)), ("dd", String(format: "%02d", day)),
                          ("mon", String(Self.monthNames[month - 1].prefix(3))), ("d", String(day))])
    }

    /// 带年、月的那一句,同引擎 digestWords.month。
    public func month(_ key: String, year: Int, month: Int) -> String {
        f(key, [("year", String(year)), ("month", String(month)), ("month_name", Self.monthNames[month - 1])])
    }

    /// 英文月份名,同 Go time.Month.String()。
    static let monthNames = ["January", "February", "March", "April", "May", "June", "July", "August", "September",
                             "October", "November", "December"]

    /// 三种语言的用语。简体那份就是引擎一直推出去的原文。
    public static let table: [ReportLanguage: [String: String]] = [
        .zhHans: [
            "brief_line": "{label}：{name} · {n}",
            "col_albums": "专辑",
            "col_artists": "歌手",
            "col_tracks": "歌曲",
            "colon": "：",
            "compare": "（{label} {prev}，{change}）",
            "cover": "封面",
            "date": "{mm}-{dd}",
            "duration_hm": " · 约 {h} 小时 {m} 分",
            "duration_m": " · 约 {m} 分",
            "flat": "持平",
            "footer": "Lyrimuse · 数据来自 {source}",
            "head_albums": "Top 专辑",
            "head_artists": "Top 歌手",
            "head_tracks": "Top 歌曲",
            "item": "{i}. {name}（{n}）",
            "keyword": "听歌",
            "label_album": "专辑",
            "label_artist": "歌手",
            "label_track": "歌曲",
            "link": "在 {source} 上看完整榜单",
            "more": " 等",
            "most_played": "最常听",
            "name_sep": "、",
            "names_lead": "：",
            "new_artists": "新听了 {n} 位歌手",
            "pair": "{artist} - {name}",
            "plays": "共播放 {n} 次",
            "plays_truncated": "共播放 {n} 次以上（记录太多，只统计了最近这些）",
            "preset_brief": "{summary}\n最常听的歌手：{top_artist}\n最常听的专辑：{top_album}\n最常听的歌曲：{top_track}\n新听了 {new_artists} 位歌手：{new_artist_names}",
            "preset_full": "{summary}\n\nTop 歌手：\n{top_artists}\n\nTop 专辑：\n{top_albums}\n\nTop 歌曲：\n{top_tracks}\n\n新听了 {new_artists} 位歌手：{new_artist_names}",
            "preset_overview": "{summary}\n新听了 {new_artists} 位歌手",
            "prev_month": "{month} 月",
            "prev_month_year": "{year} 年 {month} 月",
            "prev_week": "前一周",
            "prev_year": "{year} 年",
            "title_day": "🎧 听歌报告（{date}）",
            "title_month": "📅 {year} 年 {month} 月听歌小结",
            "title_sample": "🎧 听歌报告示例",
            "title_today": "🎧 今日听歌报告（{date}）",
            "title_week": "🎵 上周听歌小结（{from}~{to}）",
            "title_year": "🏆 {year} 年度听歌小结",
            "title_yesterday": "🎧 昨日听歌报告（{date}）",
            "value_hm": "{h} 小时 {m} 分",
            "value_m": "{m} 分",
        ],
        .zhHant: [
            "brief_line": "{label}：{name} · {n}",
            "col_albums": "專輯",
            "col_artists": "歌手",
            "col_tracks": "歌曲",
            "colon": "：",
            "compare": "（{label} {prev}，{change}）",
            "cover": "封面",
            "date": "{mm}-{dd}",
            "duration_hm": " · 約 {h} 小時 {m} 分",
            "duration_m": " · 約 {m} 分",
            "flat": "持平",
            "footer": "Lyrimuse · 資料來自 {source}",
            "head_albums": "Top 專輯",
            "head_artists": "Top 歌手",
            "head_tracks": "Top 歌曲",
            "item": "{i}. {name}（{n}）",
            "keyword": "聽歌",
            "label_album": "專輯",
            "label_artist": "歌手",
            "label_track": "歌曲",
            "link": "在 {source} 上看完整排行",
            "more": " 等",
            "most_played": "最常聽",
            "name_sep": "、",
            "names_lead": "：",
            "new_artists": "新聽了 {n} 位歌手",
            "pair": "{artist} - {name}",
            "plays": "共播放 {n} 次",
            "plays_truncated": "共播放 {n} 次以上（記錄太多，只統計了最近這些）",
            "preset_brief": "{summary}\n最常聽的歌手：{top_artist}\n最常聽的專輯：{top_album}\n最常聽的歌曲：{top_track}\n新聽了 {new_artists} 位歌手：{new_artist_names}",
            "preset_full": "{summary}\n\nTop 歌手：\n{top_artists}\n\nTop 專輯：\n{top_albums}\n\nTop 歌曲：\n{top_tracks}\n\n新聽了 {new_artists} 位歌手：{new_artist_names}",
            "preset_overview": "{summary}\n新聽了 {new_artists} 位歌手",
            "prev_month": "{month} 月",
            "prev_month_year": "{year} 年 {month} 月",
            "prev_week": "前一週",
            "prev_year": "{year} 年",
            "title_day": "🎧 聽歌報告（{date}）",
            "title_month": "📅 {year} 年 {month} 月聽歌小結",
            "title_sample": "🎧 聽歌報告範例",
            "title_today": "🎧 今日聽歌報告（{date}）",
            "title_week": "🎵 上週聽歌小結（{from}~{to}）",
            "title_year": "🏆 {year} 年度聽歌小結",
            "title_yesterday": "🎧 昨日聽歌報告（{date}）",
            "value_hm": "{h} 小時 {m} 分",
            "value_m": "{m} 分",
        ],
        .en: [
            "brief_line": "{label}: {name} · {n}",
            "col_albums": "Albums",
            "col_artists": "Artists",
            "col_tracks": "Tracks",
            "colon": ":",
            "compare": " (vs. {prev} {label}, {change})",
            "cover": "Cover",
            "date": "{mon} {d}",
            "duration_hm": " · about {h} h {m} min",
            "duration_m": " · about {m} min",
            "flat": "no change",
            "footer": "Lyrimuse · Data from {source}",
            "head_albums": "Top albums",
            "head_artists": "Top artists",
            "head_tracks": "Top tracks",
            "item": "{i}. {name} ({n})",
            "keyword": "Listening",
            "label_album": "Album",
            "label_artist": "Artist",
            "label_track": "Track",
            "link": "See the full chart on {source}",
            "more": " and more",
            "most_played": "Most played",
            "name_sep": ", ",
            "names_lead": ": ",
            "new_artists": "{n} new artists",
            "pair": "{artist} - {name}",
            "plays": "{n} plays",
            "plays_truncated": "{n}+ plays (too many listens, only the most recent were counted)",
            "preset_brief": "{summary}\nTop artist: {top_artist}\nTop album: {top_album}\nTop track: {top_track}\nNew artists ({new_artists}): {new_artist_names}",
            "preset_full": "{summary}\n\nTop artists:\n{top_artists}\n\nTop albums:\n{top_albums}\n\nTop tracks:\n{top_tracks}\n\nNew artists ({new_artists}): {new_artist_names}",
            "preset_overview": "{summary}\nNew artists: {new_artists}",
            "prev_month": "in {month_name}",
            "prev_month_year": "in {month_name} {year}",
            "prev_week": "the week before",
            "prev_year": "in {year}",
            "title_day": "🎧 Listening Report ({date})",
            "title_month": "📅 {month_name} {year} Listening Digest",
            "title_sample": "🎧 Sample Listening Report",
            "title_today": "🎧 Today's Listening Report ({date})",
            "title_week": "🎵 Last Week's Listening Digest ({from} – {to})",
            "title_year": "🏆 {year} Listening Digest",
            "title_yesterday": "🎧 Yesterday's Listening Report ({date})",
            "value_hm": "{h} h {m} min",
            "value_m": "{m} min",
            "plays_one": "{n} play",
            "new_artists_one": "{n} new artist",
        ],
    ]
}
