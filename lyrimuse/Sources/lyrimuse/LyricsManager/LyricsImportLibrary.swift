import AppKit
import Combine
import CoreServices
import LyrimuseCore
import SwiftUI
import os

private let logger = Logger(subsystem: "me.yudaotor.lyrimuse", category: "lyrics-import-library")

/// 导入库(引擎 importstore.go)的只读视图:读 index.json 和每份记录,给「导入库」面板列出来。删除走引擎改动请求
/// `drop_imported_lyrics`(`EnrichCacheStore.dropImportedLyrics`),App 不自己改引擎的文件。见 11 章决策 152。
enum LyricsImportLibrary {
    static let directoryName = "lyrimuse-imported-lyrics"
    static var directory: URL { LyrimusePaths.configFile(directoryName) }

    struct Record: Identifiable, Hashable {
        /// 记录的文件名,删除时交给引擎。
        let file: String
        /// 按歌曲存的那种(决策 150)是缓存 key;按身份存的(决策 151)是空串。
        let key: String
        let title: String
        let artist: String
        let files: [String]
        let importedAt: Date?
        let lineCount: Int
        let hasWordTiming: Bool
        let hasTranslation: Bool
        var id: String { file }
        var bySong: Bool { !key.isEmpty }
    }

    private struct Index: Decodable {
        let keys: [String: String]?
        let titles: [String: [String]]?
    }

    private struct RawRecord: Decodable {
        struct Identity: Decodable {
            let artist: String
            let title: String
        }
        let key: String?
        let lyrics: String?
        let tr: String?
        let yrc: String?
        let title: String?
        let artist: String?
        let files: [String]?
        let importedAt: Int64?
        let identities: [Identity]?

        enum CodingKeys: String, CodingKey {
            case key, lyrics, tr, yrc, title, artist, files, identities
            case importedAt = "imported_at"
        }
    }

    /// 读出导入库里的全部记录,新导入的在前。没有导入库时是空的。
    static func load() async -> [Record] {
        await Task.detached(priority: .userInitiated) { loadBlocking() }.value
    }

    private static func loadBlocking() -> [Record] {
        guard let data = FileIO.read(directory.appendingPathComponent("index.json")),
              let index = try? JSONDecoder().decode(Index.self, from: data) else { return [] }
        var names = Set(index.keys?.values.map { $0 } ?? [])
        for list in (index.titles ?? [:]).values { names.formUnion(list) }
        let records: [Record] = names.compactMap { name in
            guard let data = FileIO.read(directory.appendingPathComponent(name)),
                  let raw = try? JSONDecoder().decode(RawRecord.self, from: data) else { return nil }
            let key = raw.key ?? ""
            let parts = key.split(separator: "|", omittingEmptySubsequences: false).map(String.init)
            let identity = raw.identities?.first
            let title = [raw.title, identity?.title, parts.count > 1 ? parts[1] : nil].compactMap { $0 }.first { !$0.isEmpty } ?? ""
            let artist = [raw.artist, identity?.artist, parts.first].compactMap { $0 }.first { !$0.isEmpty } ?? ""
            return Record(
                file: name, key: key, title: title, artist: artist, files: raw.files ?? [],
                importedAt: raw.importedAt.map { Date(timeIntervalSince1970: TimeInterval($0)) },
                lineCount: LyricsSearchService.Candidate.countLines(of: raw.lyrics ?? ""),
                hasWordTiming: !(raw.yrc ?? "").isEmpty, hasTranslation: !(raw.tr ?? "").isEmpty)
        }
        return records.sorted { ($0.importedAt ?? .distantPast) > ($1.importedAt ?? .distantPast) }
    }
}

/// 监视文件夹:文件夹里有新的或改过的歌词文件,就交引擎按 `library` 方式自动导入(不弹预览、不打快照):对得上歌词库里的
/// 歌的存进导入库,条目里没有歌词的直接补上,已有的不动、等下一轮选词跟各源比;库里没有的按身份存。只在 App 里监视:文件夹
/// 可能在受保护的目录里,常驻引擎读不到。处理过的文件按「大小 + 修改时间」记在 `stateFileName`,换了文件夹就重来。
@MainActor
final class LyricsImportWatcher: ObservableObject {
    static let shared = LyricsImportWatcher()
    static let folderDefaultsKey = "np:lyricsImportWatchFolder"
    static let stateFileName = "lyrimuse-import-watch.json"
    /// 文件夹有动静之后等这么久再处理(拷一批文件进来时只处理一次)。
    static let settleDelay: TimeInterval = 3

    struct Run: Equatable {
        let at: Date
        let changed: Int
    }

    @Published private(set) var folder: URL?
    @Published private(set) var lastRun: Run?
    @Published private(set) var running = false

    private var stream: FSEventStreamRef?
    private var pendingScan: DispatchWorkItem?
    private var rescanRequested = false

    private struct State: Codable {
        var folder: String
        var files: [String: String]
    }

    private init() {}

    /// App 启动时调:清掉上次留下的暂存请求,设了监视文件夹就开始监视并补扫一次(App 没开着时放进去的文件)。
    func start() {
        LyricsBatchImport.discardStaleStaging(olderThan: 0)
        if let path = UserDefaults.standard.string(forKey: Self.folderDefaultsKey), !path.isEmpty {
            folder = URL(fileURLWithPath: path, isDirectory: true)
            startStream()
            scheduleScan(after: 10)
        }
    }

    /// 换监视的文件夹;nil = 停止监视。
    func setFolder(_ url: URL?) {
        stopStream()
        folder = url
        lastRun = nil
        UserDefaults.standard.set(url?.path, forKey: Self.folderDefaultsKey)
        guard url != nil else { return }
        startStream()
        scheduleScan(after: 0)
    }

    private func startStream() {
        guard let folder else { return }
        var context = FSEventStreamContext(
            version: 0, info: Unmanaged.passUnretained(self).toOpaque(), retain: nil, release: nil, copyDescription: nil)
        let callback: FSEventStreamCallback = { _, info, _, _, _, _ in
            guard let info else { return }
            let watcher = Unmanaged<LyricsImportWatcher>.fromOpaque(info).takeUnretainedValue()
            MainActor.assumeIsolated { watcher.scheduleScan(after: LyricsImportWatcher.settleDelay) }
        }
        guard let stream = FSEventStreamCreate(
            nil, callback, &context, [folder.path] as CFArray, FSEventStreamEventId(kFSEventStreamEventIdSinceNow), 1.0,
            FSEventStreamCreateFlags(kFSEventStreamCreateFlagNoDefer)) else {
            logger.error("import watch: could not create the event stream")
            return
        }
        FSEventStreamSetDispatchQueue(stream, .main)
        FSEventStreamStart(stream)
        self.stream = stream
    }

    private func stopStream() {
        pendingScan?.cancel()
        pendingScan = nil
        guard let stream else { return }
        FSEventStreamStop(stream)
        FSEventStreamInvalidate(stream)
        FSEventStreamRelease(stream)
        self.stream = nil
    }

    private func scheduleScan(after delay: TimeInterval) {
        pendingScan?.cancel()
        let work = DispatchWorkItem { [weak self] in
            Task { @MainActor in await self?.scan() }
        }
        pendingScan = work
        DispatchQueue.main.asyncAfter(deadline: .now() + delay, execute: work)
    }

    private var stateURL: URL { LyrimusePaths.configFile(Self.stateFileName) }

    /// 扫一遍:有新的或改过的文件就整个文件夹交引擎出清单,只导含这些文件的组。
    private func scan() async {
        guard let folder else { return }
        if running {
            rescanRequested = true
            return
        }
        running = true
        defer {
            running = false
            if rescanRequested {
                rescanRequested = false
                scheduleScan(after: Self.settleDelay)
            }
        }
        let current = await Task.detached(priority: .utility) { Self.fingerprints(of: folder) }.value
        var state = FileIO.read(stateURL).flatMap { try? JSONDecoder().decode(State.self, from: $0) }
        if state?.folder != folder.path { state = State(folder: folder.path, files: [:]) }
        let changed = Set(current.filter { state?.files[$0.key] != $0.value }.keys)
        guard !changed.isEmpty else { return }
        let groups = Set(changed.map(LyricsBatchImport.groupKey))
        guard case .success(let prepared) = await LyricsBatchImport.prepare(folder: folder, onlyGroups: groups) else { return }
        defer { LyricsBatchImport.discard(prepared.staging) }
        let items: [[String: Any]] = prepared.plan.items.compactMap { item in
            guard item.files.contains(where: changed.contains) else { return nil }
            switch item.status {
            case .matched: return item.key.map { ["id": item.id, "key": $0] }
            case .unmatched: return item.canStore == true ? ["id": item.id, "key": "", "store": true] : nil
            default: return nil
            }
        }
        var result = 0
        if !items.isEmpty {
            guard let n = await EnrichCacheStore.shared.importLyricsLibrary(staging: prepared.staging, items: items) else { return }
            result = n
        }
        state?.files = current
        if let state, let data = try? JSONEncoder().encode(state) { FileIO.write(data, to: stateURL) }
        lastRun = Run(at: Date(), changed: result)
        logger.notice("import watch: \(changed.count, privacy: .public) changed files, \(result, privacy: .public) imported")
    }

    /// 文件夹里认得的歌词文件:相对路径 → 「大小:修改时间」。
    nonisolated private static func fingerprints(of folder: URL) -> [String: String] {
        let root = folder.standardizedFileURL.path
        let extensions = Set(LyricsSearchService.importExtensions)
        guard let walker = FileManager.default.enumerator(
            at: folder, includingPropertiesForKeys: [.fileSizeKey, .contentModificationDateKey],
            options: [.skipsHiddenFiles, .skipsPackageDescendants]) else { return [:] }
        var out: [String: String] = [:]
        for case let url as URL in walker where extensions.contains(url.pathExtension.lowercased()) {
            guard let values = try? url.resourceValues(forKeys: [.fileSizeKey, .contentModificationDateKey]),
                  let size = values.fileSize else { continue }
            let full = url.standardizedFileURL.path
            let rel = full.hasPrefix(root + "/") ? String(full.dropFirst(root.count + 1)) : url.lastPathComponent
            out[rel] = "\(size):\(values.contentModificationDate?.timeIntervalSince1970 ?? 0)"
        }
        return out
    }
}

/// 歌词管理「⋯」里的「导入库…」:列出导入库里的歌词(可搜、可删、可整个清空),底下是监视文件夹。
struct LyricsImportLibrarySheet: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject private var languageSettings = AppLanguageObserver.shared
    @ObservedObject private var watcher = LyricsImportWatcher.shared
    @State private var records: [LyricsImportLibrary.Record] = []
    @State private var loaded = false
    @State private var query = ""
    @State private var confirmDeleteAll = false
    @State private var deleteFailed = false

    var body: some View {
        VStack(spacing: 0) {
            header
            Divider()
            content
            Divider()
            watchFolderBar
        }
        .frame(minWidth: 680, idealWidth: 760, minHeight: 480, idealHeight: 580)
        .task { await reload() }
        .onChange(of: watcher.lastRun) { _, _ in Task { await reload() } }
        .confirmationDialog(L10n.t("清空导入库？"), isPresented: $confirmDeleteAll, titleVisibility: .visible) {
            Button(L10n.t("全部删除"), role: .destructive) { Task { await delete(all: true) } }
            Button(L10n.t("取消"), role: .cancel) {}
        } message: {
            Text(L10n.t("已用在歌曲上的歌词保持不变，只是以后重新匹配时不再参与比较"))
        }
    }

    private var header: some View {
        HStack(alignment: .center, spacing: 12) {
            VStack(alignment: .leading, spacing: 2) {
                Text(L10n.t("导入库")).font(.headline)
                Text(String(format: L10n.plural("%@ 份导入的歌词", count: records.count), "\(records.count)"))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Spacer()
            TextField(L10n.t("搜索歌名或歌手"), text: $query)
                .textFieldStyle(.roundedBorder)
                .frame(maxWidth: 220)
            Button(L10n.t("完成")) { dismiss() }
                .keyboardShortcut(.defaultAction)
                .settingsGlassButtons(inSheet: true)
        }
        .padding(.horizontal, 18)
        .padding(.vertical, 14)
        .background(WindowDragHandle())
    }

    private var shown: [LyricsImportLibrary.Record] {
        let q = query.trimmingCharacters(in: .whitespaces).lowercased()
        guard !q.isEmpty else { return records }
        return records.filter { $0.title.lowercased().contains(q) || $0.artist.lowercased().contains(q) }
    }

    @ViewBuilder
    private var content: some View {
        if !loaded {
            ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
        } else if records.isEmpty {
            ContentUnavailableView(L10n.t("导入库是空的"), systemImage: "tray",
                                   description: Text(L10n.t("用「导入歌词文件夹…」导入，或在下方设置监视文件夹")))
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else {
            List(shown) { record in row(record) }
                .listStyle(.inset)
        }
    }

    private func row(_ r: LyricsImportLibrary.Record) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 10) {
            VStack(alignment: .leading, spacing: 2) {
                Text([r.title, r.artist].filter { !$0.isEmpty }.joined(separator: " · "))
                    .lineLimit(1)
                Text(traits(r))
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .help(r.files.joined(separator: "\n"))
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            Button {
                Task { await delete(files: [r.file]) }
            } label: {
                Image(systemName: "trash")
            }
            .buttonStyle(.borderless)
            .help(L10n.t("从导入库删除"))
            .accessibilityLabel(L10n.t("从导入库删除"))
        }
        .padding(.vertical, 2)
    }

    /// 「对应歌词库中的歌曲 · 32 行 · 译文 · 文件名 · 11月1日」。
    private func traits(_ r: LyricsImportLibrary.Record) -> String {
        var parts = [r.bySong ? L10n.t("对应歌词库中的歌曲") : L10n.t("按歌名和歌手识别")]
        if r.lineCount > 0 { parts.append(String(format: L10n.t("%@ 行"), "\(r.lineCount)")) }
        if r.hasWordTiming { parts.append(L10n.t("逐字时间轴")) }
        if r.hasTranslation { parts.append(L10n.t("译文")) }
        if let first = r.files.first { parts.append(first) }
        // 日期跟界面语言走,显式传 L10n.locale(不传会落到系统语言)。
        if let at = r.importedAt { parts.append(at.formatted(Date.FormatStyle(date: .abbreviated, time: .omitted, locale: L10n.locale))) }
        return parts.joined(separator: " · ")
    }

    private var watchFolderBar: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 10) {
                Text(L10n.t("监视文件夹"))
                    .foregroundStyle(.secondary)
                Text(watcher.folder?.path ?? L10n.t("未设置"))
                    .lineLimit(1)
                    .truncationMode(.middle)
                    .help(watcher.folder?.path ?? "")
                    .frame(maxWidth: .infinity, alignment: .leading)
                // 按钮按完整宽度排,长语言时让路径先缩短,别把按钮挤成两行。
                Button(watcher.folder == nil ? L10n.t("选择…") : L10n.t("更换…")) { chooseFolder() }
                    .settingsGlassButtons(inSheet: true)
                    .fixedSize()
                if watcher.folder != nil {
                    Button(L10n.t("停止监视")) { watcher.setFolder(nil) }
                        .settingsGlassButtons(inSheet: true)
                        .fixedSize()
                }
                Button(L10n.t("全部删除…"), role: .destructive) { confirmDeleteAll = true }
                    .settingsGlassButtons(inSheet: true)
                    .fixedSize()
                    .disabled(records.isEmpty)
            }
            Group {
                if deleteFailed {
                    Label(L10n.t("删除失败，请重试"), systemImage: "exclamationmark.triangle.fill").foregroundStyle(.orange)
                } else if watcher.running {
                    Text(L10n.t("正在导入监视文件夹中的新文件…"))
                } else if let run = watcher.lastRun {
                    Text(String(format: L10n.t("上次自动导入：%1$@，%2$@ 份"),
                                run.at.formatted(Date.FormatStyle(date: .omitted, time: .shortened, locale: L10n.locale)),
                                "\(run.changed)"))
                } else {
                    Text(L10n.t("文件夹中新增或修改的歌词文件会自动导入：对应的歌曲没有歌词时直接补上，已有歌词的不变"))
                }
            }
            .font(.caption)
            .foregroundStyle(.secondary)
            .fixedSize(horizontal: false, vertical: true)
        }
        .padding(.horizontal, 18)
        .padding(.vertical, 12)
    }

    private func chooseFolder() {
        let panel = NSOpenPanel()
        panel.canChooseFiles = false
        panel.canChooseDirectories = true
        panel.allowsMultipleSelection = false
        panel.prompt = L10n.t("监视")
        panel.message = L10n.t("选择要监视的歌词文件夹")
        guard panel.runModal() == .OK, let url = panel.url else { return }
        watcher.setFolder(url)
    }

    private func reload() async {
        records = await LyricsImportLibrary.load()
        loaded = true
    }

    private func delete(files: [String] = [], all: Bool = false) async {
        deleteFailed = !(await EnrichCacheStore.shared.dropImportedLyrics(files: files, all: all))
        await reload()
    }
}
