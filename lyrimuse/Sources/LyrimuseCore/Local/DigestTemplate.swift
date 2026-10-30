import Foundation

/// 听歌报告的自定义内容:标题、正文各一段模板,四档共用,用 `{占位符}` 取这一期的数。渠道「报告内容」选了自定义时用
/// 那个渠道的(`NotificationChannel.digestTemplate`);features.json 的 `digest_template` 是渠道还没选过时沿用的那份
/// (见 `DigestStyle`)。跟默认一样的那一段存成空串。真推送由引擎渲染(digesttemplate.go),
/// 这里是同一套规则,给编辑器和渠道预览用;两边对得上靠共用样例 shared/testdata/notify/template.json。
/// 用了自定义就按纯文本推,不套各平台的卡片排版。见 15 章决策 84。
public struct DigestTemplate: Equatable, Sendable {
    public var title: String
    public var body: String

    public init(title: String = "", body: String = "") {
        self.title = title
        self.body = body
    }

    public static let defaultTitle = "{title}"
    public static let defaultBody = "{body}"
    /// 每段最多留多少字(同引擎 digestTemplateMaxRunes)。
    public static let maxLength = 4000

    /// 认得的占位符,顺序就是编辑器里那一排的顺序(同引擎 digestPlaceholderNames)。
    public static let placeholders = [
        "title", "body", "summary", "plays", "prev_plays", "prev_label", "change", "duration",
        "top_artists", "top_albums", "top_tracks", "top_artist", "top_album", "top_track",
        "new_artists", "new_artist_names", "source", "from", "to", "link",
    ]

    /// 跟默认一样的那一段存成空;超长截断(同引擎 resolveDigestTemplate)。
    public static func resolved(title: String?, body: String?) -> DigestTemplate {
        func clean(_ raw: String?, _ fallback: String) -> String {
            let s = (raw ?? "").replacingOccurrences(of: "\r\n", with: "\n")
            if s.trimmingCharacters(in: .whitespacesAndNewlines) == fallback { return "" }
            return s.count > maxLength ? String(s.prefix(maxLength)) : s
        }
        return DigestTemplate(title: clean(title, defaultTitle), body: clean(body, defaultBody))
    }

    /// 有没有自定义(标题、正文任一段)。
    public var isCustom: Bool {
        !title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            || !body.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }

    /// 编辑器里显示的那一段:没自定义时显示默认的 `{title}` / `{body}`。
    public var editableTitle: String { title.isEmpty ? Self.defaultTitle : title }
    public var editableBody: String { body.isEmpty ? Self.defaultBody : body }

    /// 这一期每个占位符的值;取不到的是空串。
    public static func values(defaultTitle: String, defaultBody: String, stats s: NotificationPreview.Stats,
                              topN: Int, lang: ReportLanguage = .zhHans) -> [String: String] {
        let w = lang.words
        var v = Dictionary(uniqueKeysWithValues: placeholders.map { ($0, "") })
        v["title"] = defaultTitle
        v["body"] = defaultBody
        v["summary"] = NotificationPreview.summaryLine(s, lang: lang)
        v["plays"] = String(s.totalPlays)
        v["source"] = s.sourceName
        v["from"] = s.from
        v["to"] = s.to
        v["link"] = s.link
        if s.prevPlays > 0, !s.prevLabel.isEmpty, !s.truncated {
            v["prev_plays"] = String(s.prevPlays)
            v["prev_label"] = s.prevLabel
            v["change"] = NotificationPreview.change(s.totalPlays, s.prevPlays, lang: lang)
        }
        let minutes = s.totalDurationMs / 60000
        if minutes >= 60 {
            v["duration"] = w.f("value_hm", [("h", String(minutes / 60)), ("m", String(minutes % 60))])
        } else if minutes > 0 {
            v["duration"] = w.f("value_m", [("m", String(minutes))])
        }
        let n = DigestTopN.resolved(topN)
        func list(_ tallies: [NotificationPreview.Tally]) -> (lines: String, first: String) {
            let top = Array(tallies.prefix(n))
            let lines = top.enumerated().map { NotificationPreview.plainItem($0.offset + 1, $0.element, w) }
            return (lines.joined(separator: "\n"), top.first.map { NotificationPreview.plainName($0, w) } ?? "")
        }
        (v["top_artists"], v["top_artist"]) = list(s.artists)
        (v["top_albums"], v["top_album"]) = list(s.albums)
        (v["top_tracks"], v["top_track"]) = list(s.tracks)
        if s.newArtists > 0 {
            v["new_artists"] = String(s.newArtists)
            v["new_artist_names"] = s.newArtistNames.joined(separator: w["name_sep"])
        }
        return v
    }

    /// 按行替换:一行里认得的占位符全都取不到值时整行不写;认不得的 `{xxx}` 原样留着;首尾空行去掉(同引擎 renderDigestTemplateText)。
    public static func renderText(_ template: String, values: [String: String]) -> String {
        var out: [String] = []
        for line in template.replacingOccurrences(of: "\r\n", with: "\n").components(separatedBy: "\n") {
            var known = 0, filled = 0
            var rendered = ""
            var rest = Substring(line)
            while let open = rest.firstIndex(of: "{") {
                rendered += rest[..<open]
                let afterOpen = rest.index(after: open)
                guard let close = rest[afterOpen...].firstIndex(of: "}") else {
                    rendered += rest[open...]
                    rest = ""
                    break
                }
                let name = rest[afterOpen..<close]
                if !name.isEmpty, name.allSatisfy({ ("a"..."z").contains($0) || $0 == "_" }), let value = values[String(name)] {
                    known += 1
                    if !value.isEmpty { filled += 1 }
                    rendered += value
                    rest = rest[rest.index(after: close)...]
                } else {
                    // 不是占位符:只吐出这个「{」,从下一个字符接着找(同正则 \{([a-z_]+)\} 的匹配方式)。
                    rendered += "{"
                    rest = rest[afterOpen...]
                }
            }
            rendered += rest
            if known > 0 && filled == 0 { continue }
            while let last = rendered.last, last == " " || last == "\t" { rendered.removeLast() }
            out.append(rendered)
        }
        var text = out.joined(separator: "\n")
        while text.hasPrefix("\n") { text.removeFirst() }
        while text.hasSuffix("\n") { text.removeLast() }
        return text
    }

    /// 三套预设写成模板的样子(标题都是默认标题,存成空),编辑器照它显示、从它改起;精简、概况的纯文本正文就是渲染它。
    /// 正文按报告语言取(ReportWords 的 preset_*)。同引擎 digestPresetTemplate,共用样例 shared/testdata/notify/presets.json
    /// (简体)、words.json(三种语言)。自定义返回 nil。见 15 章决策 91。
    public static func preset(_ style: DigestStyle, lang: ReportLanguage = .zhHans) -> DigestTemplate? {
        style == .custom ? nil : DigestTemplate(body: lang.words["preset_" + style.rawValue])
    }

    /// 多行的占位符:插进正文时单独占一行,标题里不插。
    public static let multilinePlaceholders: Set<String> = ["body", "top_artists", "top_albums", "top_tracks"]

    /// 默认正文拆成各段的写法(编辑器「拆开默认正文」):换掉 `{body}` 后能逐段删改、调顺序。只有一条的榜单也照列,
    /// 新歌手那句不带「等」,其余跟默认正文一样。就是完整那套预设。
    public static func expandedDefaultBody(lang: ReportLanguage = .zhHans) -> String { lang.words["preset_full"] }

    /// 模板里 `{小写字母和下划线}` 的位置(UTF-16,给 NSTextView 上色用)和认不认得,同 renderText 的匹配方式。
    public struct Token: Equatable, Sendable {
        public let range: NSRange
        public let name: String
        public let known: Bool
    }

    public static func tokens(in text: String) -> [Token] {
        let units = Array(text.utf16)
        var out: [Token] = []
        var i = 0
        let open = UInt16(UInt8(ascii: "{")), close = UInt16(UInt8(ascii: "}"))
        let a = UInt16(UInt8(ascii: "a")), z = UInt16(UInt8(ascii: "z")), underscore = UInt16(UInt8(ascii: "_"))
        while i < units.count {
            guard units[i] == open else { i += 1; continue }
            var j = i + 1
            while j < units.count, (a...z).contains(units[j]) || units[j] == underscore { j += 1 }
            if j > i + 1, j < units.count, units[j] == close {
                let name = String(utf16CodeUnits: Array(units[(i + 1)..<j]), count: j - i - 1)
                out.append(Token(range: NSRange(location: i, length: j - i + 1), name: name, known: placeholders.contains(name)))
                i = j + 1
            } else {
                i += 1
            }
        }
        return out
    }

    /// 在光标处插 `{name}` 时实际插进去的文字。`before` / `after` 是光标前后紧挨着的那个字(没有就是 nil)。
    /// 多行的占位符单独占一行:前面不是行首就先换行,后面总带一个换行,插完光标在新的一行上;
    /// 其余的,前一个字是英文字母、数字或别的占位符时补一个空格,不然 `{title}{plays}` 会粘在一起。
    public static func insertionText(for name: String, before: Character?, after: Character?) -> String {
        var token = "{\(name)}"
        if multilinePlaceholders.contains(name) {
            if let before, before != "\n" { token = "\n" + token }
            if after != "\n" { token += "\n" }
        } else if let before, before == "}" || (before.isASCII && (before.isLetter || before.isNumber)) {
            token = " " + token
        }
        return token
    }

    /// 认不出的占位符(去重、按出现顺序);它们会原样发出去。
    public static func unknownPlaceholders(in text: String) -> [String] {
        var seen: [String] = []
        for t in tokens(in: text) where !t.known && !seen.contains(t.name) { seen.append(t.name) }
        return seen
    }

    /// 这一期实际推的标题和正文;哪一段没自定义、或者渲染出来是空的,那一段用默认(同引擎 renderDigestTemplate)。
    public func render(defaultTitle: String, defaultBody: String, stats: NotificationPreview.Stats,
                       topN: Int, lang: ReportLanguage = .zhHans) -> (title: String, body: String) {
        let v = Self.values(defaultTitle: defaultTitle, defaultBody: defaultBody, stats: stats, topN: topN, lang: lang)
        var outTitle = defaultTitle, outBody = defaultBody
        if !title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            let r = Self.renderText(title, values: v).replacingOccurrences(of: "\n", with: " ")
                .trimmingCharacters(in: .whitespacesAndNewlines)
            if !r.isEmpty { outTitle = r }
        }
        if !body.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            let r = Self.renderText(body, values: v)
            if !r.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty { outBody = r }
        }
        return (outTitle, outBody)
    }
}
