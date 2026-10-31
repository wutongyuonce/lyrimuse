import CoreGraphics

/// 编辑台工具栏一行按内容分宽(`EditorToolbarContentWidthRow`)的分法。见 04 章决策 80。
public enum EditorToolbarWidths {
    /// 每颗拿 min(理想宽度, max(底线, 水位)),水位取恰好把 `room` 填满的那个值:放得下时全拿理想宽度,
    /// 放不下时短的照旧拿满、亏空摊给最长的那几颗,每颗最多压到底线。底线加起来都放不下,才按底线的比例一起缩。
    /// 底线比理想宽度大、或没给底线的,按理想宽度算(不压)。
    public static func fill(ideals: [CGFloat], floors: [CGFloat], room: CGFloat) -> [CGFloat] {
        let room = max(room, 0)
        guard ideals.reduce(0, +) > room else { return ideals }
        let floors = ideals.indices.map { i in min(max(i < floors.count ? floors[i] : ideals[i], 0), ideals[i]) }
        let floorTotal = floors.reduce(0, +)
        guard floorTotal < room else {
            let scale = floorTotal > 0 ? room / floorTotal : 0
            return floors.map { $0 * scale }
        }
        func filled(_ level: CGFloat) -> [CGFloat] {
            zip(ideals, floors).map { min($0, max($1, level)) }
        }
        var low: CGFloat = 0
        var high = ideals.max() ?? 0
        for _ in 0..<40 {
            let mid = (low + high) / 2
            if filled(mid).reduce(0, +) > room { high = mid } else { low = mid }
        }
        return filled(low)
    }
}
