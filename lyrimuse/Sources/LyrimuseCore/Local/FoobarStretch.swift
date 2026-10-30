import Foundation
import os

/// foobar2000 的播放速度。菜单里没有倍速,只有 DSP 里的 Audio-Stretch 能变速;开着时系统那份照报速率 1、播放中不再发锚点,
/// 按 1 倍外推越走越偏,到暂停、跳转它报回正确位置才纠回(锚点本身是准的)。在放时每拍读一次主窗口的进度条
/// (`FoobarSliders.progress`),只拿来量速度:变速时进度条比出声固定落后约 0.9 秒,开播头几秒这段落后还在涨,所以锚点之后
/// `settleSecs` 才取起点,起点不动,到最新一次读数隔 `windowSecs` 以上就算出速度(间隔越长越准),偏离 1 倍不到 `minDeviation`
/// 算 1 倍。位置照「锚点 + 锚点以来的时间 × 速度」走。量出来的速度记着(DSP 链换歌不变),新锚点照它走;锚点之后进度条
/// 开始走了(读数跟上一次不一样)才取粗量起点(冷启动接着放时它先停在存下的位置一秒多,再一下跳上来),到它隔
/// `quickWindowSecs` 以上先粗量一次,只用来改方向:记着变速、粗量是 1 倍就放掉,记着 1 倍、粗量偏离 `quickDeviation`
/// 以上就先用粗量的。同一份锚点下进度条往回跳(单曲循环绕回开头时它不发新锚点)、或离上一次读数隔了 `readGapSecs` 以上
/// (焦点被别的 App 占着那段不读进度条)就从那里重新等、重新量,可能跨圈的两次读数不拿来算。
/// 进度条读不到(没授「辅助功能」、主窗口没布局、`SourceKnockout` 关了 `foobar-slider`)时照记着的走。
/// 见 02 章决策 211、212。
public enum FoobarStretch {
    /// 两次读数至少隔多久才算速度:进度条读数跟出声差 ±0.1 秒,隔 4 秒误差在 5% 以内。
    public static let windowSecs: TimeInterval = 4
    /// 偏离 1 倍不到这么多算 1 倍。
    public static let minDeviation = 0.05
    /// 锚点之后多久才取量速度的起点。
    public static let settleSecs: TimeInterval = 3
    /// 粗量的最短间隔。
    public static let quickWindowSecs: TimeInterval = 2
    /// 记着 1 倍时,粗量偏离多少才先改用它(开播头几秒进度条的落后在涨,粗量偏低,只认明显的变速)。
    public static let quickDeviation = 0.15
    /// 进度条读数比上一次小这么多以上算往回跳(读数本身抖 ±0.1 秒)。
    public static let rewindSlackSecs = 0.5
    /// 两次读数隔这么久以上算中断过(在放时每拍都读)。
    public static let readGapSecs: TimeInterval = 8

    public struct Sample: Equatable, Sendable {
        public let value: Double
        public let at: Date
        public init(value: Double, at: Date) {
            self.value = value
            self.at = at
        }
    }

    /// 记着的东西:当前锚点和第一次见到它的时刻、粗量和细量的起点、上一次读数、量出来的速度。
    public struct Tracker: Equatable, Sendable {
        public var anchorKey: String?
        public var anchorSeenAt: Date?
        public var probe: Sample?
        public var first: Sample?
        public var last: Sample?
        public var rate: Double = 1
        public init() {}
    }

    /// 同一份锚点下的两次进度条读数 → 速度(保留两位);间隔不到 `window`、读数倒退或离谱时为 nil。纯函数,selftest 覆盖。
    public static func measuredRate(from first: Sample, to latest: Sample, window: TimeInterval = windowSecs) -> Double? {
        let span = latest.at.timeIntervalSince(first.at)
        guard span >= window else { return nil }
        let rate = (latest.value - first.value) / span
        guard rate > 0.25, rate < 4 else { return nil }
        return abs(rate - 1) < minDeviation ? 1 : (rate * 100).rounded() / 100
    }

    /// 按 1 倍外推的位置换成按 `rate` 外推:锚点加上锚点以来走过的那段乘速度。纯函数,selftest 覆盖。
    public static func position(anchor: Double, oneX: Double, rate: Double) -> Double {
        anchor + (oneX - anchor) * rate
    }

    /// 走一拍:换了锚点就重新量;停着不量、只清掉起点;读不到进度条时原样。纯函数,selftest 覆盖。
    public static func step(_ tracker: Tracker, anchorKey: String, playing: Bool, sample: Sample?, now: Date) -> Tracker {
        var next = tracker
        if next.anchorKey != anchorKey {
            next.anchorKey = anchorKey
            next.anchorSeenAt = now
            next.probe = nil
            next.first = nil
            next.last = nil
        }
        guard playing else {
            next.probe = nil
            next.first = nil
            next.last = nil
            return next
        }
        guard let sample else { return next }
        let previous = next.last
        if let previous,
           sample.value < previous.value - rewindSlackSecs || sample.at.timeIntervalSince(previous.at) > readGapSecs {
            next.anchorSeenAt = now
            next.probe = nil
            next.first = nil
        }
        next.last = sample
        if let probe = next.probe {
            if let quick = measuredRate(from: probe, to: sample, window: quickWindowSecs) {
                if next.rate == 1, abs(quick - 1) >= quickDeviation { next.rate = quick }
                if next.rate != 1, quick == 1 { next.rate = 1 }
            }
        } else if let previous, sample.value != previous.value {
            next.probe = sample
        }
        guard let first = next.first else {
            if sample.at.timeIntervalSince(next.anchorSeenAt ?? sample.at) >= settleSecs { next.first = sample }
            return next
        }
        if let rate = measuredRate(from: first, to: sample) { next.rate = rate }
        return next
    }

    private static let tracker = OSAllocatedUnfairLock(initialState: Tracker())
    private static let logger = Logger(subsystem: LyrimuseIdentity.logSubsystem, category: "foobar-stretch")

    /// 这一拍该按的速度。在放时读一次进度条,会阻塞跨进程调用,在轮询的后台线程上调。
    public static func rate(anchorKey: String, playing: Bool, duration: Double?, now: Date) -> Double {
        let value = playing && !SourceKnockout.isOut(.foobarSlider) ? FoobarSliders.progress(duration: duration) : nil
        let sample = value.map { Sample(value: $0, at: Date()) }
        let (before, after) = tracker.withLock { state -> (Double, Tracker) in
            let before = state.rate
            state = step(state, anchorKey: anchorKey, playing: playing, sample: sample, now: now)
            return (before, state)
        }
        if before != after.rate {
            logger.notice("foobar2000 speed: \(before, format: .fixed(precision: 2))x → \(after.rate, format: .fixed(precision: 2))x from its progress bar")
        }
        return after.rate
    }
}
