import Foundation

/// 面板格子这类窄位置上的标题能不能折成两行。
/// 中日文标题没有空格时，折行会落在任意两个字之间（会把一个词拆开），这类标题只放一行、靠缩字放下；
/// 带空格或连字符的标题（英文、德文、韩文）按词折行。
public enum TileTitleWrap {
    public static func breaksInsideWords(_ title: String) -> Bool {
        if title.contains(where: { $0.isWhitespace }) { return false }
        return title.unicodeScalars.contains { s in
            switch s.value {
            case 0x3040...0x30FF, 0x31F0...0x31FF, 0x3400...0x4DBF, 0x4E00...0x9FFF, 0xF900...0xFAFF: return true
            default: return false
            }
        }
    }
}
