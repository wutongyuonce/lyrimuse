import Foundation

/// 触控栏「歌词颜色 › 彩虹」的配色和流动时钟。见 17 章决策 54。
///
/// 配色:OKLCH 里同一个亮度绕色相一圈,每个色相的彩度取 `maxChroma` 和 sRGB 色域上限里小的那个 —— 亮度一致,
/// 黑底上蓝、紫不比黄、绿暗;直接取饱和的纯色相,蓝紫那几个字看不清。
public enum RainbowLyrics {
    public static let lightness = 0.80
    /// 浅色底(浅色菜单栏)上用的亮度:0.80 那组压在近白的底上发虚。
    public static let lightBackgroundLightness = 0.55
    public static let maxChroma = 0.14
    /// 第一档的色相(度),从偏暖的红起。
    public static let startHue = 20.0
    /// 一圈里取几个色相,首尾不重复;渐变按这些点线性插值。
    public static let huesPerCycle = 9
    /// 颜色流过一圈要几秒。
    public static let cycleSeconds = 6.0
    /// 没唱到的字打的透明度(悬浮歌词、灵动岛、网页;触控栏按自己的「暗部亮度」)。
    public static let unsungOpacity = 0.45
    /// 一圈多宽(点)的下限:一圈按这一行字的宽算(`cycleWidth`),只有两三个字的句子不至于挤进一整圈。
    public static let minCycleWidth = 100.0

    /// 一圈多宽:这一行字的宽,夹在 `minCycleWidth` 和可视窗宽之间。短句也看得到整条彩虹;放不下要滚的长句按
    /// 可视窗宽一圈,窗里始终是一整圈。
    public static func cycleWidth(textWidth: Double, viewWidth: Double) -> Double {
        let upper = max(viewWidth, 1)
        return min(upper, max(textWidth, min(minCycleWidth, upper)))
    }

    public struct RGB: Equatable, Sendable {
        public var r: Double
        public var g: Double
        public var b: Double
    }

    /// 一圈的色标,`huesPerCycle + 1` 个:最后一个跟第一个是同一个颜色,一圈接一圈拼起来没有接缝。深色底用。
    public static let cycleStops: [RGB] = stops(lightness: lightness)
    /// 同上,浅色底用(`lightBackgroundLightness`)。
    public static let lightBackgroundCycleStops: [RGB] = stops(lightness: lightBackgroundLightness)

    /// 按 `lightness` 绕色相一圈取色标,首尾同色。
    public static func stops(lightness: Double) -> [RGB] {
        let step = 360.0 / Double(huesPerCycle)
        let ring = (0..<huesPerCycle).map { inGamut(hue: startHue + Double($0) * step, lightness: lightness) }
        return ring + [ring[0]]
    }

    /// 这个色相在 `lightness` 下、彩度不超过 `maxChroma` 时落在 sRGB 色域里的颜色(彩度二分到色域边界)。
    public static func inGamut(hue: Double, lightness: Double = RainbowLyrics.lightness) -> RGB {
        if let rgb = srgb(l: lightness, c: maxChroma, hue: hue) { return rgb }
        var lo = 0.0, hi = maxChroma
        for _ in 0..<24 {
            let mid = (lo + hi) / 2
            if srgb(l: lightness, c: mid, hue: hue) != nil { lo = mid } else { hi = mid }
        }
        return srgb(l: lightness, c: lo, hue: hue) ?? RGB(r: lightness, g: lightness, b: lightness)
    }

    /// OKLCH → sRGB(0~1,带 gamma);落在色域外返回 nil。
    public static func srgb(l: Double, c: Double, hue: Double) -> RGB? {
        let lin = linearSRGB(l: l, c: c, hue: hue)
        let eps = 1e-6
        guard [lin.r, lin.g, lin.b].allSatisfy({ $0 >= -eps && $0 <= 1 + eps }) else { return nil }
        func gamma(_ v: Double) -> Double {
            let x = min(max(v, 0), 1)
            return x <= 0.0031308 ? 12.92 * x : 1.055 * pow(x, 1 / 2.4) - 0.055
        }
        return RGB(r: gamma(lin.r), g: gamma(lin.g), b: gamma(lin.b))
    }

    /// OKLCH → 线性 sRGB(不夹、不做 gamma)。
    public static func linearSRGB(l: Double, c: Double, hue: Double) -> RGB {
        let a = c * cos(hue * .pi / 180)
        let b = c * sin(hue * .pi / 180)
        let l_ = l + 0.3963377774 * a + 0.2158037573 * b
        let m_ = l - 0.1055613458 * a - 0.0638541728 * b
        let s_ = l - 0.0894841775 * a - 1.2914855480 * b
        let (lc, mc, sc) = (l_ * l_ * l_, m_ * m_ * m_, s_ * s_ * s_)
        return RGB(r: 4.0767416621 * lc - 3.3077115913 * mc + 0.2309699292 * sc,
                   g: -1.2684380046 * lc + 2.6097574011 * mc - 0.3413193965 * sc,
                   b: -0.0041960863 * lc - 0.7034186147 * mc + 1.7076147010 * sc)
    }

    /// sRGB(带 gamma)→ OKLab 的 L,给测试核「一圈亮度一致」用。
    public static func oklabLightness(_ rgb: RGB) -> Double {
        func linear(_ v: Double) -> Double { v <= 0.04045 ? v / 12.92 : pow((v + 0.055) / 1.055, 2.4) }
        let (r, g, b) = (linear(rgb.r), linear(rgb.g), linear(rgb.b))
        let l = cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b)
        let m = cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b)
        let s = cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b)
        return 0.2104542553 * l + 0.7936177850 * m - 0.0040720468 * s
    }

    /// 流动的相位(0..<1):全进程从 `epoch` 起按墙钟走,从不冻住 —— 各展示面、各行、换句前后颜色始终一致;停着的行停在
    /// 此刻的全局相位上,再走时直接回到全局相位(暂停后继续那一下颜色跳一格)。
    public static func phase(at now: Double, epoch: Double, cycle: Double = cycleSeconds) -> Double {
        let c = cycle > 0 ? cycle : cycleSeconds
        let t = (now - epoch).truncatingRemainder(dividingBy: c)
        return (t < 0 ? t + c : t) / c
    }
}
