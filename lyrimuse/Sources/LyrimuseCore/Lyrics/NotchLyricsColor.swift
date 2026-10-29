import Foundation

/// 灵动岛歌词行的颜色:跟着灵动岛那一套强调色走,或沿歌词流动的彩虹(`RainbowLyrics`)。见 05 章决策 120。
public enum NotchLyricsColor: String, CaseIterable, Sendable {
    case accent
    case rainbow

    /// 存着的值认不出(或没存过)时用 `fallback`。
    public static func resolved(stored: String?, fallback: NotchLyricsColor) -> NotchLyricsColor {
        stored.flatMap(NotchLyricsColor.init(rawValue:)) ?? fallback
    }
}
