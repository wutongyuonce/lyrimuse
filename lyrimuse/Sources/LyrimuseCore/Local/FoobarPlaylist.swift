import Foundation

/// foobar2000 的播放列表,给引擎预解析(`PlayerQueryServer` 的 `foobar_queue`,引擎侧 `foobarqueue.go`)。列表存在
/// `~/Library/foobar2000-v2/playlists-v2.0/playlist-<GUID>.fplite`(不带沙盒,不要权限):带 BOM 的 UTF-8,一行一个
/// `file://` 地址,路径原样写、空格和中文都不转义,列表一改就落盘。几个列表里挑含着它此刻开着的那个文件的那一份(它只开着
/// 正在放的那一个);每个文件的歌名歌手按认当前这首那一套换(`PlayerOpenFile`),循环 / 随机读「Order」菜单的勾
/// (`FoobarMenuControl`)。交出去的形状同 Pine 那份(`PinePlaylist.QueueReply`)。见 02 章决策 207。
public enum FoobarPlaylist {
    public static let maxTracks = 500

    public static var playlistsDirectory: URL {
        FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent("Library/foobar2000-v2/playlists-v2.0")
    }

    /// fplite 正文 → 本地文件路径(`file://` 之后原样取,不做百分号解码;空行和别的地址跳过),最多 `maxTracks` 条。
    /// 纯函数,selftest 覆盖。
    public static func paths(fplite: String) -> [String] {
        var out: [String] = []
        for raw in fplite.split(whereSeparator: \.isNewline) {
            let line = raw.trimmingCharacters(in: CharacterSet.whitespaces.union(CharacterSet(charactersIn: "\u{FEFF}")))
            if line.hasPrefix("file:///") {
                out.append(String(line.dropFirst("file://".count)))
            } else if line.hasPrefix("/") {
                out.append(line)
            } else {
                continue
            }
            if out.count == maxTracks { break }
        }
        return out
    }

    /// 几份列表里含着 `current` 的那一份;不知道当前文件、哪份都没有时为 nil。纯函数,selftest 覆盖。
    public static func pick(_ lists: [[String]], current: String?) -> [String]? {
        guard let current else { return nil }
        return lists.first { $0.contains(current) }
    }

    /// 一个文件 → 曲目:标签里有歌名的用标签(乱码还原);没有的照 foobar2000 报不带扩展名的文件名的样子拆
    /// (`PlayerOpenFile.fileNameIdentity`,跟真放到时写进播放状态的是同一份)。`tags` 为 nil = 读不出。纯函数,selftest 覆盖。
    public static func track(path: String, tags: LocalAudioFileTags?) -> PinePlaylist.Track {
        let fileName = URL(fileURLWithPath: path).lastPathComponent
        if let tags, let id = PlayerTitleSplit.identity(from: tags) {
            return PinePlaylist.Track(title: id.title, artist: id.artist, album: id.album, duration: id.duration ?? 0, file: path)
        }
        let base = PlayerTitleSplit.strippingAudioExtension(fileName)
        let id = PlayerOpenFile.fileNameIdentity(reportedArtist: "", reportedTitle: base, fileName: fileName,
                                                 duration: tags?.duration, acceptsBaseName: true)
        return PinePlaylist.Track(title: id?.title ?? base, artist: id?.artist ?? "", album: "", duration: tags?.duration ?? 0,
                                  file: path)
    }

    /// 菜单里的档 → 交给引擎的写法:随机 `shuffle`、列表循环 `all`、单曲循环 `one`、不循环 `off`;读不出为 nil。纯函数,selftest 覆盖。
    public static func repeating(_ mode: MusicPlaybackController.MusicPlaybackMode?) -> String? {
        switch mode {
        case .shuffle?: return "shuffle"
        case .repeatAll?: return "all"
        case .repeatOne?: return "one"
        case .list?: return "off"
        case nil: return nil
        }
    }

    /// 读列表与菜单里的档;认不出当前文件、哪份列表都不含它、一首都没有时为 nil。会读盘、阻塞跨进程查询,别在主线程调。
    public static func queueReply() -> PinePlaylist.QueueReply? {
        let current = PlayerProcess.pid(bundleID: FoobarMenuControl.bundleID).flatMap { pid in
            ProcessOpenFiles.paths(pid: pid).first {
                PlayerTitleSplit.audioExtensions.contains(URL(fileURLWithPath: $0).pathExtension.lowercased())
            }
        }
        let files = (try? FileManager.default.contentsOfDirectory(at: playlistsDirectory, includingPropertiesForKeys: nil)) ?? []
        let lists = files.filter { $0.pathExtension == "fplite" }.compactMap { FileIO.readString($0) }.map(paths(fplite:))
        guard let list = pick(lists, current: current), !list.isEmpty else { return nil }
        let tracks = list.map { track(path: $0, tags: LocalAudioFileTags.read(path: $0)) }
        return PinePlaylist.QueueReply(tracks: tracks, repeating: repeating(FoobarMenuControl.readMode()))
    }
}
