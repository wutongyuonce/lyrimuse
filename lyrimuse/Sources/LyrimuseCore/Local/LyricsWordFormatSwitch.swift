import Foundation

/// 换逐字歌词格式的进度:引擎换格式时(lyricsdirswitch.go 的 `switchLyricsWordFormats`)写
/// `lyrimuse-lyrics-word-format-status.json`,设置页「逐字歌词格式」那一行按它显示进度和「已改存」。
///
/// 进度按有逐字的歌数(编好算半首、写出算整首);总数要等导出开始才数出来,之前是 0。引擎启动时删掉上次留下的那份。
public enum LyricsWordFormatSwitch {
    public struct Status: Decodable, Equatable {
        public let running: Bool
        /// 换成的格式,同 features.json 的 `lyrics_word_formats`(第一个是主格式)。
        public let formats: [String]
        public let done: Int
        public let total: Int
        public let startedAt: Int64
        public let updatedAt: Int64

        public init(running: Bool, formats: [String], done: Int, total: Int, startedAt: Int64, updatedAt: Int64) {
            self.running = running
            self.formats = formats
            self.done = done
            self.total = total
            self.startedAt = startedAt
            self.updatedAt = updatedAt
        }
    }

    public enum Phase: Equatable {
        /// 设置已写下,引擎还没接手(或在导入、还没数出总数)。
        case waiting
        case converting(done: Int, total: Int)
        /// total 是有逐字的歌数;文件夹本来就是这几种格式时是 0。
        case finished(total: Int)
        /// 等了 `pickupTimeout` 引擎也没接手,或跑到一半没了(引擎退出):下次启动时引擎会补做。
        case queued
    }

    /// 引擎几乎立刻热重读设置;过了这么久状态文件里还没有这一趟,就当它没在跑。
    public static let pickupTimeout: TimeInterval = 10
    /// 跑着时每 0.25 秒写一次;这么久没更新就当这一趟没了。
    public static let stallTimeout: TimeInterval = 30

    /// 纯函数,selftest 覆盖。状态文件里是别的一趟(格式不同,或比这次请求早开始)按还没接手算。
    public static func phase(formats: [String], status: Status?, requestedAt: Date, now: Date) -> Phase {
        guard let status, status.formats == formats,
              Double(status.startedAt) >= requestedAt.timeIntervalSince1970.rounded(.down) else {
            return now.timeIntervalSince(requestedAt) < pickupTimeout ? .waiting : .queued
        }
        guard status.running else { return .finished(total: status.total) }
        if now.timeIntervalSince1970 - Double(status.updatedAt) > stallTimeout { return .queued }
        return status.total > 0 ? .converting(done: min(status.done, status.total), total: status.total) : .waiting
    }

    static let statusURL = LyrimusePaths.configFile("lyrimuse-lyrics-word-format-status.json")

    /// 状态文件此刻的内容;不存在 / 解不开都是 nil。只在等结论时读,文件很小。
    public static var current: Status? {
        FileIO.decodeJSON(Status.self, from: statusURL)
    }
}
