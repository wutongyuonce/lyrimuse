import AppKit
import Combine
import LyrimuseCore
import SwiftUI

/// 灵动岛空闲面板上那首「上次播放」:停播前最后那首(`IdleLastTrack`)、它在缓存里的封面、从它歌词里挑出的一句。
///
/// 真窗口、各屏副本和编辑台预览共用这一份:面板高度跟着「有没有上次那首 / 挑不挑得出句子」分三档
/// (`NotchExpandedMetrics.idlePanelHeight(showsLastTrack:showsQuote:)`),几处读同一个值,卡片高度才跟画出来的内容对得上。
@MainActor
final class NotchIdleLastTrack: ObservableObject {
    static let shared = NotchIdleLastTrack()

    /// 从没放过歌是 nil。
    @Published private(set) var track: IdleLastTrack?
    @Published private(set) var cover: URL?
    /// 此刻显示的那一句(乐句各行按空格拼成一行)。挑不出句子(纯音乐、没歌词、缓存里没有)是 nil。
    @Published private(set) var quote: String?

    private var quotes: [String] = []
    private var loadTask: Task<Void, Never>?
    private var subscriptions = Set<AnyCancellable>()

    /// 面板该留哪一档高度。挂着「发现新播放器」时面板换成那条提议,按两行字那一档。
    static func panelHeight(track: IdleLastTrack?, quote: String?, promptShowing: Bool) -> CGFloat {
        let showsTrack = track != nil && !promptShowing
        return NotchExpandedMetrics.idlePanelHeight(showsLastTrack: showsTrack, showsQuote: showsTrack && quote != nil)
    }

    private init() {
        refresh()
        // 三个键在播放时随换歌写入,曲目一没就是停播前最后那首。存 sink 参数值,别回头读 PlaybackCoordinator(@Published 在 willSet 发布)。
        let playback = PlaybackCoordinator.shared
        Publishers.CombineLatest3(playback.$title, playback.$artist, playback.$isCurrentTrackAdBreak)
            .sink { [weak self] title, artist, isAd in
                if !NotchLyricsWindowController.trackPresent(title: title, artist: artist, isAdBreak: isAd) {
                    self?.refresh()
                }
            }
            .store(in: &subscriptions)
        // 取数那一刻缓存可能还没解码完(App 刚启动)、引擎还没写上封面或歌词:缓存换了内容就补封面,没挑出句子的再挑一次。
        LocalPlaybackSource.shared.$enrichContentVersion
            .removeDuplicates()
            .dropFirst()
            .sink { [weak self] _ in self?.refillFromCache() }
            .store(in: &subscriptions)
    }

    /// 按三个键重读。同一首不重挑句子。
    func refresh() {
        let current = IdleLastTrack.current
        guard !current.title.isEmpty else {
            loadTask?.cancel()
            quotes = []
            if track != nil { track = nil }
            if cover != nil { cover = nil }
            if quote != nil { quote = nil }
            return
        }
        guard current != track else { return }
        track = current
        cover = current.coverURL()
        quotes = []
        quote = nil
        loadQuotes(for: current)
    }

    /// 悬停展开时换一句。高度只看有没有句子,换句不动高度。
    func rerollQuote() {
        guard quotes.count > 1 else { return }
        quote = quotes.filter { $0 != quote }.randomElement()
    }

    private func refillFromCache() {
        guard let track else { return }
        if cover == nil { cover = track.coverURL() }
        if quotes.isEmpty { loadQuotes(for: track) }
    }

    private func loadQuotes(for track: IdleLastTrack) {
        loadTask?.cancel()
        loadTask = Task { [weak self] in
            let picked = await track.loadQuotes().map { $0.joined(separator: " ") }
            guard let self, !Task.isCancelled, self.track == track else { return }
            self.quotes = picked
            if self.quote == nil { self.quote = picked.randomElement() }
        }
    }
}

/// 空闲面板左半边:有上次那首时是封面 + 歌名 +「上次播放 · 歌手」+(挑得出时)一句歌词,没有时是「未在播放」+ 提示。
/// 字号、透明度、行距照有曲目时的头部(歌名 12 半粗 / 歌手 10;歌词那句同歌手那一档)。
struct NotchIdleLastTrackInfo: View {
    @ObservedObject private var model = NotchIdleLastTrack.shared
    let tint: Color

    var body: some View {
        HStack(spacing: 8) {
            if let track = model.track {
                cover(track)
                VStack(alignment: .leading, spacing: NotchExpandedMetrics.trackInfoLineSpacing) {
                    Text(TrackNameDisplay.cleaned(track.title))
                        .font(.system(size: 12, weight: .semibold))
                        .foregroundStyle(tint.opacity(0.9))
                    Text(track.artist.isEmpty ? L10n.t("上次收听") : String(format: L10n.t("上次播放 · %@"), track.artist))
                        .font(.system(size: 10, weight: .medium))
                        .foregroundStyle(tint.opacity(0.6))
                    if let quote = model.quote {
                        Text("\u{201C}\(quote)\u{201D}")
                            .font(.system(size: 10))
                            .foregroundStyle(tint.opacity(0.75))
                            .frame(height: NotchExpandedMetrics.trackInfoArtistLineHeight)
                    }
                }
            } else {
                VStack(alignment: .leading, spacing: NotchExpandedMetrics.trackInfoLineSpacing) {
                    Text(L10n.t("未在播放"))
                        .font(.system(size: 12, weight: .semibold))
                        .foregroundStyle(tint.opacity(0.9))
                    // 提示句不点名播放器:这句是泛指,右边那颗键的悬浮提示才带具体名字(那是一个具体动作的目标)。
                    Text(L10n.t("在播放器中播放任意歌曲，歌词将自动显示"))
                        .font(.system(size: 10, weight: .medium))
                        .foregroundStyle(tint.opacity(0.6))
                }
            }
        }
        .lineLimit(1)
        .truncationMode(.tail)
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    /// 封面跟头部那枚一样大。缓存里没有封面时退回按歌手名恒定取色的色块 + 首字,口径同歌词窗口停播页。
    private func cover(_ track: IdleLastTrack) -> some View {
        let side = NotchExpandedMetrics.trackInfoArtworkSide
        let placeholder = ZStack {
            LastfmStatsSection.stableColor(for: track.artist.isEmpty ? track.title : track.artist)
                .opacity(0.65)
            Text(String(TrackNameDisplay.cleaned(track.title).prefix(1)))
                .font(.system(size: side * 0.45, weight: .light))
                .foregroundStyle(.white.opacity(0.85))
        }
        return Group {
            if let url = model.cover {
                CachedImage(url: url) { placeholder }
            } else {
                placeholder
            }
        }
        .frame(width: side, height: side)
        .clipShape(RoundedRectangle(cornerRadius: NotchMetrics.artworkCornerRadius, style: .continuous))
        .accessibilityHidden(true)
    }
}
