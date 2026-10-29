import LyrimuseCore
import SwiftUI

// 推送渠道「报告内容」的模板编辑器:标题和正文四档报告共用(Core `DigestTemplate`,引擎 digesttemplate.go)。顶上一排
// 完整 / 精简 / 概况 / 自定义,点预设就把它的模板填进来,改了任何一处就算自定义(`DigestStyle.matching`)。点一个占位符
// 插到正在编辑的那一栏的光标处(PlaceholderTextView),右边按这一期的真实数据(取不到时用测试数据)实时预览。
// 见 15 章决策 84、90、91。

extension DigestStyle {
    var label: String {
        switch self {
        case .full: return L10n.t("完整")
        case .brief: return L10n.t("精简")
        case .overview: return L10n.t("概况")
        case .custom: return L10n.t("自定义")
        }
    }

    var note: String {
        switch self {
        case .full: return L10n.t("概况、三份榜单和新听的歌手")
        case .brief: return L10n.t("概况和歌手、专辑、歌曲各自的第一名")
        case .overview: return L10n.t("只有播放次数、时长和新听了几位歌手")
        case .custom: return L10n.t("按你写的模板，以纯文本推送")
        }
    }
}

/// 只读显示一份模板:标题一行、正文照写,认得的占位符同编辑器的画法(强调色加淡底)。高度固定、放不下的在框里滚,
/// 换档时弹窗不跳。
struct DigestTemplateExpression: View {
    let template: DigestTemplate

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 4) {
                Text(Self.highlighted(template.editableTitle)).fontWeight(.semibold)
                Text(Self.highlighted(template.editableBody))
            }
            .font(.system(size: 11.5))
            .textSelection(.enabled)
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(8)
        }
        .frame(height: 118)
        .background(RoundedRectangle(cornerRadius: 8, style: .continuous).fill(Color.primary.opacity(0.04)))
        .overlay(RoundedRectangle(cornerRadius: 8, style: .continuous).strokeBorder(Color.primary.opacity(0.08)))
    }

    static func highlighted(_ text: String) -> AttributedString {
        var out = AttributedString(text)
        for token in DigestTemplate.tokens(in: text) where token.known {
            guard let r = Range(token.range, in: text), let ar = Range(r, in: out) else { continue }
            out[ar].foregroundColor = .accentColor
            out[ar].backgroundColor = Color.accentColor.opacity(0.12)
        }
        return out
    }
}

/// 编辑标题和正文:左边两个输入框和一排占位符,右边按这一期的数据实时预览。保存时跟默认一样的那一段存成空
/// (`DigestTemplate.resolved`)。
struct DigestTemplateEditor: View {
    let original: DigestTemplate
    let onSave: (DigestTemplate) -> Void
    let onCancel: () -> Void

    @ObservedObject private var features = FeatureSettingsStore.shared
    @StateObject private var insertion = PlaceholderInsertionController()
    @State private var title: String
    @State private var bodyText: String
    @State private var kind: NotificationPreview.Kind = .daily
    @State private var loaded: [NotificationPreview.Kind: NotificationPreviewLoader.State] = [:]
    @State private var loading: Set<NotificationPreview.Kind> = []
    /// 从自定义换到某套预设之前写的内容,点回「自定义」时还原。
    @State private var keptCustom: DigestTemplate?
    private let now = Date()

    init(original: DigestTemplate, onSave: @escaping (DigestTemplate) -> Void, onCancel: @escaping () -> Void) {
        self.original = original
        self.onSave = onSave
        self.onCancel = onCancel
        _title = State(initialValue: original.editableTitle)
        _bodyText = State(initialValue: original.editableBody)
    }

    private var draft: DigestTemplate { DigestTemplate.resolved(title: title, body: bodyText) }

    private var style: DigestStyle { DigestStyle.matching(draft) }

    /// 点预设:填进它的模板;点自定义:还原换走之前写的那份(没有时这一档点不了)。
    private func apply(_ next: DigestStyle) {
        guard next != style else { return }
        if style == .custom { keptCustom = draft }
        guard let filled = DigestTemplate.preset(next) ?? keptCustom else { return }
        title = filled.editableTitle
        bodyText = filled.editableBody
    }

    private static let editorHeight: CGFloat = 230

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            VStack(alignment: .leading, spacing: 2) {
                Text(L10n.t("推送内容")).font(.system(size: 15, weight: .bold))
                Text(L10n.t("四档报告共用。点下面的占位符插到光标处；一行里的占位符都没有数据时，这一行不发送"))
                    .font(.system(size: 11.5))
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            HStack(alignment: .top, spacing: 18) {
                editorColumn.frame(width: 460)
                previewColumn.frame(width: 340)
            }
            Divider()
            HStack {
                Spacer()
                Button(L10n.t("取消")) { onCancel() }
                    .settingsGlassButtons(inSheet: true)
                    .keyboardShortcut(.cancelAction)
                Button(L10n.t("保存")) { onSave(draft) }
                    .keyboardShortcut(.defaultAction)
            }
        }
        .padding(20)
        .fixedSize()
        .task(id: kind) { await load(kind) }
    }

    // MARK: 编辑

    private var editorColumn: some View {
        VStack(alignment: .leading, spacing: 10) {
            // sheet 里画不出激活态的玻璃,几选一用胶囊分段(11 章决策 57)。
            SettingsPillSegmentedControl(selection: Binding(get: { style }, set: apply), options: DigestStyle.allCases,
                                         label: \.label, minWidth: 0,
                                         isDisabled: { $0 == .custom && style != .custom && keptCustom == nil })
            fieldLabel(L10n.t("标题"))
            PlaceholderTextView(text: $title, singleLine: true, controller: insertion, field: .title)
                .frame(height: 26)
                .modifier(TemplateFieldChrome(focused: insertion.lastField == .title))
            HStack(alignment: .firstTextBaseline) {
                fieldLabel(L10n.t("正文"))
                Spacer()
                if bodyText.contains("{body}") {
                    Button(L10n.t("拆开默认正文")) {
                        bodyText = bodyText.replacingOccurrences(of: "{body}", with: DigestTemplate.expandedDefaultBody)
                    }
                    .buttonStyle(.link)
                    .font(.system(size: 11.5))
                    .help(L10n.t("把「默认正文」换成概况、三份榜单、新歌手几段，可以逐段删改、调整顺序"))
                }
            }
            PlaceholderTextView(text: $bodyText, controller: insertion, field: .body, initialFocus: true)
                .frame(height: Self.editorHeight)
                .modifier(TemplateFieldChrome(focused: insertion.lastField == .body))
            placeholderChips
            unknownWarning
        }
    }

    private func fieldLabel(_ text: String) -> some View {
        Text(text).font(.system(size: 12, weight: .semibold)).foregroundStyle(.secondary)
    }

    /// 占位符一排:标签是说法,悬停写它插进去的样子和是什么。正文、三份榜单是多行的,总是插进正文;标题正在编辑时它们灰掉。
    private var placeholderChips: some View {
        SettingsFlowRow(spacing: 5) {
            ForEach(DigestTemplate.placeholders, id: \.self) { name in
                let multiline = DigestTemplate.multilinePlaceholders.contains(name)
                Button { insertion.insert(name) } label: {
                    Text(Self.label(name))
                        .font(.system(size: 11))
                        .padding(.horizontal, 8)
                        .padding(.vertical, 3)
                        .background(Capsule().fill(Color.accentColor.opacity(0.10)))
                        .foregroundStyle(Color.accentColor)
                        .contentShape(Capsule())
                }
                .buttonStyle(SettingsGlassHoverButtonStyle())
                .disabled(multiline && insertion.lastField == .title)
                .hoverNote("{\(name)}  " + Self.note(name))
            }
        }
    }

    /// 认不出的占位符会原样发出去:列出来,免得拼错了没发现。这一行的位置一直留着,出现、消失时弹窗不跳。
    private var unknownWarning: some View {
        let unknown = DigestTemplate.unknownPlaceholders(in: title + "\n" + bodyText)
        return Label(String(format: L10n.t("认不出的占位符会原样发送：%@"), unknown.map { "{\($0)}" }.joined(separator: "、")),
                     systemImage: "exclamationmark.triangle.fill")
            .font(.system(size: 11))
            .foregroundStyle(.orange)
            .lineLimit(1)
            .truncationMode(.tail)
            .opacity(unknown.isEmpty ? 0 : 1)
            .accessibilityHidden(unknown.isEmpty)
    }

    // MARK: 预览

    private var real: (title: String, stats: NotificationPreview.Stats)? {
        if case .real(let title, let stats)? = loaded[kind] { return (title, stats) }
        return nil
    }

    private var previewColumn: some View {
        let stats = real?.stats ?? NotificationPreview.sample(kind)
        let defaultTitle = real?.title ?? NotificationPreview.title(kind, now: now)
        let defaultBody = NotificationPreview.body(stats, topN: features.digestTopN)
        let out = draft.render(defaultTitle: defaultTitle, defaultBody: defaultBody, stats: stats, topN: features.digestTopN)
        return VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 8) {
                fieldLabel(L10n.t("预览"))
                Spacer()
                SettingsPillSegmentedControl(selection: $kind, options: NotificationPreview.Kind.allCases, label: {
                    $0 == .daily ? L10n.t("每日报告") : L10n.t("每周小结")
                }, minWidth: 0)
                .fixedSize()
            }
            ScrollView {
                VStack(alignment: .leading, spacing: 6) {
                    Text(out.title).font(.system(size: 13, weight: .semibold))
                    Text(out.body).font(.system(size: 12.5))
                }
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(10)
            }
            // 跟左边档位一排、标题、正文两栏加中间的标签对齐。
            .frame(height: Self.editorHeight + 26 + 26 + 38)
            .settingsCardBackground(cornerRadius: 10, inSheet: true)
            VStack(alignment: .leading, spacing: 3) {
                Group {
                    if real != nil {
                        Text(String(format: L10n.t("真实数据 · %@"), stats.sourceName))
                    } else if loading.contains(kind) {
                        HStack(spacing: 5) {
                            ProgressView().controlSize(.mini)
                            Text(L10n.t("正在取真实数据…"))
                        }
                    } else {
                        Text(L10n.t("测试数据"))
                    }
                }
                Text(style == .custom ? L10n.t("自定义内容按纯文本推送，不套用各平台的卡片排版")
                                    : L10n.t("默认内容推送时按各平台的格式排版，这里只显示文字"))
                    .fixedSize(horizontal: false, vertical: true)
            }
            .font(.system(size: 11))
            .foregroundStyle(.secondary)
        }
    }

    private func load(_ kind: NotificationPreview.Kind) async {
        if let hit = NotificationPreviewLoader.cached(kind) {
            loaded[kind] = hit
            return
        }
        loading.insert(kind)
        let state = await NotificationPreviewLoader.load(kind)
        loading.remove(kind)
        if let state { loaded[kind] = state }
    }

    // MARK: 占位符的说法

    static func label(_ name: String) -> String {
        switch name {
        case "title": return L10n.t("默认标题")
        case "body": return L10n.t("默认正文")
        case "summary": return L10n.t("概况")
        case "plays": return L10n.t("播放次数")
        case "prev_plays": return L10n.t("上期次数")
        case "prev_label": return L10n.t("上期名称")
        case "change": return L10n.t("涨跌")
        case "duration": return L10n.t("时长")
        case "top_artists": return L10n.t("歌手榜")
        case "top_albums": return L10n.t("专辑榜")
        case "top_tracks": return L10n.t("歌曲榜")
        case "top_artist": return L10n.t("最常听的歌手")
        case "top_album": return L10n.t("最常听的专辑")
        case "top_track": return L10n.t("最常听的歌曲")
        case "new_artists": return L10n.t("新歌手数")
        case "new_artist_names": return L10n.t("新歌手名单")
        case "source": return L10n.t("数据来源")
        case "from": return L10n.t("开始日期")
        case "to": return L10n.t("结束日期")
        case "link": return L10n.t("榜单链接")
        default: return name
        }
    }

    static func note(_ name: String) -> String {
        switch name {
        case "title": return L10n.t("不自定义时的标题，带这一期的日期")
        case "body": return L10n.t("不自定义时的整份正文")
        case "summary": return L10n.t("默认正文的第一行：次数、跟上一期比、时长")
        case "plays": return L10n.t("这一期的播放次数")
        case "prev_plays": return L10n.t("上一期的播放次数，日报没有")
        case "prev_label": return L10n.t("上一期怎么称呼，如「前一周」")
        case "change": return L10n.t("跟上一期比的涨跌，如「↑12%」")
        case "duration": return L10n.t("听了多久，如「17 小时 52 分」")
        case "top_artists": return L10n.t("歌手榜，按「每类列出」的条数，一行一位")
        case "top_albums": return L10n.t("专辑榜，按「每类列出」的条数，一行一张")
        case "top_tracks": return L10n.t("歌曲榜，按「每类列出」的条数，一行一首")
        case "top_artist": return L10n.t("这一期听得最多的歌手")
        case "top_album": return L10n.t("这一期听得最多的专辑")
        case "top_track": return L10n.t("这一期听得最多的歌曲")
        case "new_artists": return L10n.t("这一期新听的歌手有几位")
        case "new_artist_names": return L10n.t("新听的歌手里听得最多的几位")
        case "source": return L10n.t("Last.fm 或 ListenBrainz")
        case "from": return L10n.t("这一期的第一天")
        case "to": return L10n.t("这一期的最后一天")
        case "link": return L10n.t("在 Last.fm 或 ListenBrainz 上看完整榜单的地址")
        default: return ""
        }
    }
}

/// 输入框的底和边:跟系统文本框一样的白底细边,正在编辑的那一栏描强调色。
private struct TemplateFieldChrome: ViewModifier {
    let focused: Bool

    func body(content: Content) -> some View {
        content
            .background(RoundedRectangle(cornerRadius: 6, style: .continuous).fill(Color(nsColor: .textBackgroundColor)))
            .overlay(RoundedRectangle(cornerRadius: 6, style: .continuous)
                .strokeBorder(focused ? Color.accentColor.opacity(0.7) : Color.primary.opacity(0.15), lineWidth: focused ? 1.5 : 1))
            .clipShape(RoundedRectangle(cornerRadius: 6, style: .continuous))
    }
}
