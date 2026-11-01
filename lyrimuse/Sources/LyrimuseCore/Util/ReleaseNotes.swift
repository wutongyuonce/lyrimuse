import Foundation

/// 从 CHANGELOG.md(build.sh 拷进包里)取某一版的更新内容,拆成单一语言的「小节 + 条目」,给「本次更新」卡片画。
///
/// 格式跟 `.github/scripts/split_release_notes.py` 认的一致:节标题 `## v<版本>`;小节标题「英文 / 中文」(按最后一个
/// ` / ` 切);条目以 `- ` 开头,先若干行英文、后若干行中文,一行算不算中文按 CJK 字符占比(>0.25),从第一行中文起余下
/// 都归中文。表格、链接、HTML 注释这类两种语言共用的行不收。两处规则要一起改。见 14 章决策 155。
public enum ReleaseNotes {
    public struct Section: Equatable, Sendable {
        public var title: String
        public var items: [String]

        public init(title: String, items: [String]) {
            self.title = title
            self.items = items
        }
    }

    /// `## v<version>` 到下一个 `## v` 之间的正文;没有这一节为 nil。
    public static func section(version: String, in changelog: String) -> String? {
        let lines = changelog.components(separatedBy: "\n")
        guard let start = lines.firstIndex(where: { $0.trimmingCharacters(in: .whitespaces) == "## v\(version)" }) else {
            return nil
        }
        let rest = lines[(start + 1)...]
        let end = rest.firstIndex { $0.hasPrefix("## v") } ?? lines.endIndex
        return lines[(start + 1)..<end].joined(separator: "\n")
    }

    /// `baseline` 之后到 `current` 为止(含)各版的正文,按文件里的次序(新的在前);一节都没有时退回 `current` 那一节。
    /// 跳过几个版本升上来的用户看到中间每一版的内容。
    public static func sections(after baseline: String, through current: String, in changelog: String) -> [String] {
        let versions = changelog.components(separatedBy: "\n").compactMap { line -> String? in
            let trimmed = line.trimmingCharacters(in: .whitespaces)
            guard trimmed.hasPrefix("## v") else { return nil }
            return String(trimmed.dropFirst(4))
        }
        let inRange = versions.filter {
            WhatsNew.compare($0, baseline) == .orderedDescending && WhatsNew.compare($0, current) != .orderedDescending
        }
        let picked = inRange.isEmpty ? [current] : inRange
        return picked.compactMap { section(version: $0, in: changelog) }
    }

    /// 几版拆好的小节并成一份:同名小节合在一起(按先出现的次序),条目新版在前。
    public static func merge(_ versions: [[Section]]) -> [Section] {
        var merged: [Section] = []
        for sections in versions {
            for section in sections {
                if let i = merged.firstIndex(where: { $0.title == section.title }) {
                    merged[i].items += section.items
                } else {
                    merged.append(section)
                }
            }
        }
        return merged
    }

    /// 拆成单一语言。`chinese` 为假取英文;某一条没写中文时退回它的英文。
    public static func parse(_ section: String, chinese: Bool) -> [Section] {
        var sections: [Section] = []
        var item: [String] = []

        func flushItem() {
            guard !item.isEmpty else { return }
            let zhStart = item.firstIndex(where: isChinese)
            let en = zhStart.map { Array(item[..<$0]) } ?? item
            let zh = zhStart.map { Array(item[$0...]) } ?? []
            let text = join(chinese && !zh.isEmpty ? zh : en)
            if !text.isEmpty {
                if sections.isEmpty { sections.append(Section(title: "", items: [])) }
                sections[sections.count - 1].items.append(text)
            }
            item = []
        }

        for raw in section.components(separatedBy: "\n") {
            let line = raw.trimmingCharacters(in: .whitespaces)
            if line.isEmpty || isShared(line) {
                flushItem()
                continue
            }
            if item.isEmpty, let header = splitHeader(line) {
                flushItem()
                sections.append(Section(title: chinese ? header.zh : header.en, items: []))
            } else if line.hasPrefix("- ") {
                flushItem()
                item = [String(line.dropFirst(2))]
            } else if !item.isEmpty {
                item.append(line)
            }
        }
        flushItem()
        return sections.filter { !$0.items.isEmpty }
    }

    static func isChinese(_ line: String) -> Bool {
        let chars = line.unicodeScalars.filter { !CharacterSet.whitespaces.contains($0) }
        guard !chars.isEmpty else { return false }
        let cjk = chars.filter { (0x4E00...0x9FFF).contains($0.value) || (0x3000...0x303F).contains($0.value)
            || (0xFF00...0xFFEF).contains($0.value) }.count
        return Double(cjk) / Double(chars.count) > 0.25
    }

    /// 表格、裸链接、HTML 注释、Full Changelog:两种语言都不是条目正文。
    private static func isShared(_ line: String) -> Bool {
        line.hasPrefix("|") || line.hasPrefix("http") || line.hasPrefix("<!--") || line.hasPrefix("**Full Changelog**")
            || line.hasPrefix("#")
    }

    /// 「New / 新功能」→ (New, 新功能);右边不是中文或左边是中文时不算小节标题。
    private static func splitHeader(_ line: String) -> (en: String, zh: String)? {
        guard !line.hasPrefix("- "), let range = line.range(of: " / ", options: .backwards) else { return nil }
        let left = String(line[..<range.lowerBound]), right = String(line[range.upperBound...])
        guard isChinese(right), !isChinese(left) else { return nil }
        return (left, right)
    }

    /// 硬换行的几行拼回一句:两侧都不是 CJK 补一个空格;都是 CJK 直接接;一侧是 CJK 时,汉字挨着英文字母或数字补空格
    /// (「下发」+「DNS」→「下发 DNS」),挨着标点直接接(「；」+「ListenBrainz」、「/」+「剩余」)。
    /// 跟 `.github/scripts/split_release_notes.py` 的 `join_wrapped` 同一条规则。
    static func join(_ lines: [String]) -> String {
        var text = ""
        for line in lines where !line.isEmpty {
            if let last = text.unicodeScalars.last, let first = line.unicodeScalars.first, needsSpace(last, first) {
                text += " "
            }
            text += line
        }
        return text
    }

    private static func needsSpace(_ a: Unicode.Scalar, _ b: Unicode.Scalar) -> Bool {
        switch (isCJK(a), isCJK(b)) {
        case (false, false): return true
        case (true, true): return false
        case (true, false): return isIdeograph(a) && isASCIIAlphanumeric(b)
        case (false, true): return isASCIIAlphanumeric(a) && isIdeograph(b)
        }
    }

    private static func isCJK(_ scalar: Unicode.Scalar) -> Bool {
        isIdeograph(scalar) || (0x3000...0x303F).contains(scalar.value) || (0xFF00...0xFFEF).contains(scalar.value)
    }

    private static func isIdeograph(_ scalar: Unicode.Scalar) -> Bool { (0x4E00...0x9FFF).contains(scalar.value) }

    private static func isASCIIAlphanumeric(_ scalar: Unicode.Scalar) -> Bool {
        scalar.isASCII && CharacterSet.alphanumerics.contains(scalar)
    }
}
