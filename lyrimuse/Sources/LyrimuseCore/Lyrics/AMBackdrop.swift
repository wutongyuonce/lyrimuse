import CoreGraphics
import CoreImage
import Foundation

/// 歌词窗口「封面」背景:照 Apple Music 播放页背景的做法(07 章决策 186)。
///
/// AM 的做法:封面缩到 128²,三份副本(半边长 1.4 / 0.7 / 0.7,长边一半为 1)各自整圈慢转,第三份跟着第一份公转;
/// 每份逐像素压暗、按 Rec.709 亮度权重提饱和;叠好后大半径高斯模糊、反预乘、混 10% 白,最后过一张压红 / 蓝的色表。
/// 这里把模糊挪到每份副本自己身上预先烘好(两张位图),转动交给 Core Animation:底层外延铺满不露边,
/// 上层两份带软边透明、按普通 over 叠。几何、周期取 AM 的值;「先叠后模糊」变成「先模糊后叠」会让明暗跨度收窄、
/// 彩度略高,所以饱和比 AM 低一点、再补一点对比度(这两个值是对着 AM 真值在训练集上拟合的,见 07 章决策 187)。
public enum AMBackdrop {
    /// 一份封面副本。单位:视图长边的一半 = 1,y 向上,原点在视图中心。
    public struct Layer: Equatable, Sendable {
        /// 副本(正方形)半边长。
        public let halfSide: Double
        /// 副本中心相对视图中心的偏移(父层公转前)。
        public let dx: Double
        public let dy: Double
        /// 自转一整圈的秒数。
        public let period: Double
        /// 跟着哪一层一起绕视图中心公转(nil = 不公转)。
        public let parent: Int?
        /// 逐像素压暗系数。
        public let darken: Double
    }

    public static let layers: [Layer] = [
        Layer(halfSide: 1.4, dx: 0, dy: 0, period: 240, parent: nil, darken: 0.65),
        Layer(halfSide: 0.7, dx: -0.25, dy: 0.15, period: 140, parent: nil, darken: 0.6425),
        Layer(halfSide: 0.7, dx: 0.7, dy: 0.7, period: 180, parent: 0, darken: 0.635),
    ]

    /// 提饱和倍数(AM 宽窗口那一档是 2.4)。
    public static let saturation = 2.3
    /// 对比度:颜色绕 `contrastPivot` 拉开的倍数(AM 没有这一步,补「先模糊后叠」收窄的明暗跨度)。
    public static let contrast = 1.07
    public static let contrastPivot = 0.45
    /// 模糊 σ(点)。AM 按绝对点数取,不随窗口缩放。
    public static let blurSigmaPoints = 120.0
    /// 模糊之后混进的白。
    public static let whiteMix = 0.1
    /// 烘焙时按这个视图宽度(点)把 σ 换成副本内的相对尺度。
    public static let referenceWidth = 1100.0
    /// 三份周期的最小公倍数:动画起点在这一段里按曲目取,同一首歌每次一样。
    public static let cycle = 5040.0
    /// 底层位图覆盖到的半边长(比副本大一圈,外延的边转到窗口角上也不露底)。
    public static let baseHalfExtent = 1.6

    // MARK: - 颜色

    /// 一份副本的颜色变换:out = M · rgb + bias(压暗、提饱和、混白、对比度合在一起),逐行给出。
    public static func colorMatrix(darken: Double, saturation: Double = saturation,
                                   contrast k: Double = contrast) -> (rows: [[Double]], bias: Double) {
        let lw = [0.2126, 0.7152, 0.0722]
        let keep = 1 - whiteMix
        let rows = (0..<3).map { r in
            (0..<3).map { c in k * keep * darken * ((r == c ? saturation : 0) + (1 - saturation) * lw[c]) }
        }
        return (rows, k * whiteMix + (1 - k) * contrastPivot)
    }

    /// 色表校正:压以红 / 蓝为主导的高饱和色,其余几乎不动。系数是对 AM 色表采样做最小二乘拟合得到的
    /// (样本按提饱和之后真实出现的颜色加权,色立方表面那一层多采),特征是各主导通道的超出量及其交叉项;
    /// 输入输出都是 sRGB 编码值,先夹到 0...1。
    public static func lutCorrect(_ r0: Double, _ g0: Double, _ b0: Double) -> (Double, Double, Double) {
        let r = min(1, max(0, r0)), g = min(1, max(0, g0)), b = min(1, max(0, b0))
        let er = max(0, r - max(g, b)), eb = max(0, b - max(r, g)), eg = max(0, g - max(r, b))
        let my = max(0, min(r, g) - b), mm = max(0, min(r, b) - g), mc = max(0, min(g, b) - r)
        let f = [er, eb, eg, er * er, eb * eb, er * g, er * b, eb * r, eb * g, er * r, eb * b,
                 my * er, mm * er, mm * eb, mc * eb, mm, my, mc, er * (1 - r), eb * (1 - b),
                 er * er * er, eb * eb * eb, er * er * g, eb * eb * r, er * g * g, eb * r * r,
                 mm * mm, my * my, mc * mc, er * mm * mm, eb * mm * mm]
        var out = [r, g, b]
        for (i, x) in f.enumerated() where x != 0 {
            for ch in 0..<3 { out[ch] += x * lutCoefficients[i][ch] }
        }
        return (min(1, max(0, out[0])), min(1, max(0, out[1])), min(1, max(0, out[2])))
    }

    static let lutCoefficients: [[Double]] = [
        [-0.22771, 0.00565, -0.02657],
        [-0.04578, 0.00048, -0.20979],
        [-0.00289, 0.00269, -0.00288],
        [-0.13404, 0.08816, 0.28593],
        [0.26152, -0.03129, -0.45514],
        [0.20817, -0.22188, 0.29190],
        [-0.04794, 0.12561, -0.16533],
        [-0.19971, 0.07769, -0.13302],
        [0.26529, -0.10607, 0.25788],
        [-0.16698, -0.01037, -0.04264],
        [-0.08173, -0.04371, -0.30464],
        [0.01501, -0.22414, -0.16324],
        [-0.24111, 0.12335, -0.62047],
        [-0.60855, 0.09365, -0.10738],
        [-0.14355, -0.09011, 0.28352],
        [0.05726, 0.00648, 0.03632],
        [-0.00973, -0.04158, 0.02144],
        [0.01037, -0.03776, -0.01498],
        [-0.06073, 0.01602, 0.01607],
        [0.03596, 0.04419, 0.09484],
        [0.24094, -0.02589, -0.15483],
        [-0.09902, 0.11643, 0.70673],
        [-0.29924, 0.07269, -0.10795],
        [0.14010, 0.18757, 0.55278],
        [0.12542, 0.16795, -0.04946],
        [-0.01369, 0.08555, 0.12032],
        [-0.06367, -0.00261, -0.03669],
        [0.03707, 0.07426, -0.03073],
        [-0.01635, 0.06452, 0.03399],
        [1.05746, -0.40696, 0.96151],
        [1.03010, -0.34521, 0.94932],
    ]

    /// `lutCorrect` 采样成的色立方(CIColorCube 的 RGBA Float32 排布,r 变化最快)。
    public static func colorCube(dimension n: Int) -> Data {
        var values = [Float](repeating: 1, count: n * n * n * 4)
        for b in 0..<n {
            for g in 0..<n {
                for r in 0..<n {
                    let d = Double(n - 1)
                    let (cr, cg, cb) = lutCorrect(Double(r) / d, Double(g) / d, Double(b) / d)
                    let o = ((b * n + g) * n + r) * 4
                    values[o] = Float(cr); values[o + 1] = Float(cg); values[o + 2] = Float(cb)
                }
            }
        }
        return values.withUnsafeBufferPointer { Data(buffer: $0) }
    }

    // MARK: - 几何与转动

    /// 第 i 份副本在时刻 t 的绕自身中心转角(弧度,y 向上坐标里正为逆时针;AM 是顺时针转,所以为负)。
    public static func spinAngle(layer i: Int, time t: Double) -> Double {
        -2 * Double.pi * t / layers[i].period
    }

    /// 第 i 份副本在时刻 t 摆在哪:中心(视图坐标,y 向上)、总转角(自转 + 父层公转)、副本半边长(点)。
    public static func placement(layer i: Int, viewSize: CGSize, time t: Double)
        -> (center: CGPoint, angle: Double, halfSide: Double) {
        let l = layers[i]
        let unit = Double(max(viewSize.width, viewSize.height)) / 2
        var dx = l.dx, dy = l.dy, angle = spinAngle(layer: i, time: t)
        if let p = l.parent {
            let a = spinAngle(layer: p, time: t)
            (dx, dy) = (dx * cos(a) - dy * sin(a), dx * sin(a) + dy * cos(a))
            angle += a
        }
        let c = CGPoint(x: Double(viewSize.width) / 2 + dx * unit, y: Double(viewSize.height) / 2 + dy * unit)
        return (c, angle, l.halfSide * unit)
    }

    /// 一整圈的离散关键帧角度(弧度),从时刻 `start` 那一档开始,`stepsPerSecond` 档每秒;首尾差一整圈,
    /// `repeatCount = .infinity` 循环时接得上。
    public static func turnAngles(period: Double, start: Double, stepsPerSecond: Double = 10) -> [Double] {
        let steps = max(2, Int((period * stepsPerSecond).rounded()))
        return (0...steps).map { k in -2 * Double.pi * (start / period + Double(k) / Double(steps)) }
    }

    /// 动画起点(秒):按曲目种子在一个完整周期里取,同一首歌每次从同一处开始。
    public static func startTime(seed: UInt64) -> Double {
        Double(seed % UInt64(cycle))
    }

    // MARK: - 烘焙

    public struct Baked {
        /// 底层:第一份副本(半边长 1.4)外延铺到 `baseHalfExtent`,不透明。
        public let base: CGImage
        /// 底层位图边长对应的半边长(单位同 `Layer`)。
        public let baseHalfExtent: Double
        /// 上层:半边长 0.7 的副本连软边,带透明;第二、三份共用。
        public let overlay: CGImage
        /// 上层位图边长对应的半边长。
        public let overlayHalfExtent: Double
        /// 底层可见区的均色(sRGB 0...1)。
        public let meanColor: (r: Double, g: Double, b: Double)
    }

    /// 颜色运算在 sRGB 编码值上做(跟 AM 一样),不经色彩管理;模糊前不截断,超出 0...1 的值留到色表才夹。
    public static let bakeContext = CIContext(options: [
        .workingColorSpace: NSNull(),
        .outputColorSpace: NSNull(),
        .workingFormat: CIFormat.RGBAh,
        .cacheIntermediates: false,
    ])

    private static let cube17 = colorCube(dimension: 17)

    /// 烘两张位图,边长 `storeSide` 像素。
    public static func bake(_ source: CGImage, storeSide: Int = 240) -> Baked? {
        let n = 128
        guard let srgb = CGColorSpace(name: CGColorSpace.sRGB),
              let ctx = CGContext(data: nil, width: n, height: n, bitsPerComponent: 8, bytesPerRow: n * 4, space: srgb,
                                  bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue) else { return nil }
        ctx.interpolationQuality = .high
        ctx.setFillColor(CGColor(srgbRed: 0, green: 0, blue: 0, alpha: 1))
        ctx.fill(CGRect(x: 0, y: 0, width: n, height: n))
        ctx.draw(source, in: CGRect(x: 0, y: 0, width: n, height: n))
        guard let art = ctx.makeImage() else { return nil }
        let artImage = CIImage(cgImage: art, options: [.colorSpace: NSNull()])
        let sigmaUnits = blurSigmaPoints / (referenceWidth / 2)

        // 带偏置的颜色矩阵输出范围是无限大,要外延就得在它之前对原图外延。
        func colored(_ darken: Double, extended: Bool) -> CIImage {
            let m = colorMatrix(darken: darken)
            func v(_ r: Int) -> CIVector { CIVector(x: m.rows[r][0], y: m.rows[r][1], z: m.rows[r][2], w: 0) }
            return (extended ? artImage.clampedToExtent() : artImage).applyingFilter("CIColorMatrix", parameters: [
                "inputRVector": v(0), "inputGVector": v(1), "inputBVector": v(2),
                "inputAVector": CIVector(x: 0, y: 0, z: 0, w: 1),
                "inputBiasVector": CIVector(x: m.bias, y: m.bias, z: m.bias, w: 0),
            ])
        }
        func graded(_ img: CIImage) -> CIImage {
            img.applyingFilter("CIColorCube", parameters: ["inputCubeDimension": 17, "inputCubeData": cube17])
        }
        func render(_ img: CIImage, halfExtentPixels h: Double) -> CGImage? {
            let c = Double(n) / 2
            let crop = CGRect(x: c - h, y: c - h, width: 2 * h, height: 2 * h)
            let s = Double(storeSide) / (2 * h)
            let out = img.cropped(to: crop)
                .transformed(by: CGAffineTransform(translationX: -crop.minX, y: -crop.minY))
                .transformed(by: CGAffineTransform(scaleX: s, y: s))
                .clampedToExtent()  // 裁剪框不在整像素上,不外延的话最外一圈半透明
            return bakeContext.createCGImage(out, from: CGRect(x: 0, y: 0, width: storeSide, height: storeSide),
                                             format: .RGBA8, colorSpace: srgb)
        }

        // 底层:像素 / 单位 = 128 / (2 × 1.4)。外延后模糊,不透明。
        let l0 = layers[0]
        let px0 = Double(n) / (2 * l0.halfSide)
        let base0 = graded(colored(l0.darken, extended: true).applyingGaussianBlur(sigma: sigmaUnits * px0))
        guard let base = render(base0, halfExtentPixels: baseHalfExtent * px0) else { return nil }

        // 上层:放在透明底上模糊出软边。色表滤镜自己按反预乘的颜色算,别再手动反预乘 / 预乘(会叠两次,软边发黑)。
        let l1 = layers[1]
        let px1 = Double(n) / (2 * l1.halfSide)
        let overlayHalfExtent = l1.halfSide + 3 * sigmaUnits
        let blurred = colored(l1.darken, extended: false).cropped(to: artImage.extent).applyingGaussianBlur(sigma: sigmaUnits * px1)
        let over1 = graded(blurred)
        guard let overlay = render(over1, halfExtentPixels: overlayHalfExtent * px1) else { return nil }

        // 均色取底层中间那块(宽窗口能看到的范围大致是半宽 1、半高 0.6)。
        let vis = CIVector(cgRect: CGRect(x: Double(n) / 2 - px0, y: Double(n) / 2 - 0.6 * px0, width: 2 * px0, height: 1.2 * px0))
        let avg = base0.applyingFilter("CIAreaAverage", parameters: [kCIInputExtentKey: vis])
        var px = [UInt8](repeating: 0, count: 4)
        bakeContext.render(avg, toBitmap: &px, rowBytes: 4, bounds: CGRect(x: 0, y: 0, width: 1, height: 1),
                           format: .RGBA8, colorSpace: nil)
        let mean = (r: Double(px[0]) / 255, g: Double(px[1]) / 255, b: Double(px[2]) / 255)
        return Baked(base: base, baseHalfExtent: baseHalfExtent, overlay: overlay,
                     overlayHalfExtent: overlayHalfExtent, meanColor: mean)
    }

    /// 时刻 t 的一帧画进 `ctx`(y 向上,`size` 为点尺寸;歌词窗口的图层树按同样的几何摆)。
    public static func drawFrame(_ baked: Baked, in ctx: CGContext, size: CGSize, time t: Double) {
        ctx.saveGState()
        ctx.setFillColor(CGColor(srgbRed: baked.meanColor.r, green: baked.meanColor.g, blue: baked.meanColor.b, alpha: 1))
        ctx.fill(CGRect(origin: .zero, size: size))
        ctx.interpolationQuality = .high
        let unit = Double(max(size.width, size.height)) / 2
        for i in layers.indices {
            let p = placement(layer: i, viewSize: size, time: t)
            let image = i == 0 ? baked.base : baked.overlay
            let half = (i == 0 ? baked.baseHalfExtent : baked.overlayHalfExtent) * unit
            ctx.saveGState()
            ctx.translateBy(x: p.center.x, y: p.center.y)
            ctx.rotate(by: CGFloat(p.angle))
            ctx.draw(image, in: CGRect(x: -half, y: -half, width: 2 * half, height: 2 * half))
            ctx.restoreGState()
        }
        ctx.restoreGState()
    }
}
