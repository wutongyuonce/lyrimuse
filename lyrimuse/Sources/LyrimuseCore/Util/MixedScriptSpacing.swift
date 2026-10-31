import Foundation

/// 把名字(播放器名、平台名)填进中文句子时的中英文间距:名字以拉丁字母或数字开头 / 结尾、紧挨着的是汉字时,
/// 在靠汉字的那一侧补一个半角空格(「提高 Apple Music 播放进度」「Telegram 拒收」);中文名(「网易云音乐」)、
/// 挨着标点或空格的一侧都不补。英文句子里没有汉字,原样返回。只处理填进去的参数两侧,模板自己的文字不动。
public enum MixedScriptSpacing {
    private static let open: Character = "\u{E000}"
    private static let close: Character = "\u{E001}"

    /// `String(format:)` 的版本:模板里的 `%@` / `%1$@` 照常按位置取 `args`。
    public static func format(_ template: String, _ args: [String]) -> String {
        let marked: [CVarArg] = args.map { "\(open)\($0)\(close)" }
        return resolve(String(format: template, arguments: marked))
    }

    private static func resolve(_ text: String) -> String {
        let chars = Array(text)
        var out: [Character] = []
        out.reserveCapacity(chars.count + 4)
        for (i, c) in chars.enumerated() {
            guard c == open || c == close else {
                out.append(c)
                continue
            }
            if let before = out.last, let after = chars[(i + 1)...].first(where: { $0 != open && $0 != close }),
               needsSpace(before, after) {
                out.append(" ")
            }
        }
        return String(out)
    }

    private static func needsSpace(_ a: Character, _ b: Character) -> Bool {
        (isHan(a) && isLatin(b)) || (isLatin(a) && isHan(b))
    }

    private static func isHan(_ c: Character) -> Bool {
        guard let v = c.unicodeScalars.first?.value else { return false }
        return (0x4E00...0x9FFF).contains(v) || (0x3400...0x4DBF).contains(v) || (0xF900...0xFAFF).contains(v)
            || (0x20000...0x2EBEF).contains(v)
    }

    private static func isLatin(_ c: Character) -> Bool {
        guard let v = c.unicodeScalars.first?.value else { return false }
        if c.isASCII { return c.isLetter || c.isNumber }
        return (0x00C0...0x024F).contains(v) && c.isLetter
    }
}
