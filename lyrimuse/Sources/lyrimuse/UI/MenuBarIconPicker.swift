import SwiftUI

/// 「通用 → 菜单栏与 Dock」里挑菜单栏图标的那一块:12 款图标各一张卡(4 列 × 3 行),下面一张通栏的「专辑封面」卡。
///
/// 选哪张就是哪一种样子:点图标卡 = 用这款、不用封面;点封面卡 = 用封面,没有封面时退回上次选的那款
/// (`menuBarIconStyle` 不动,卡上写出来是哪一款)。所以没有单独的「图标换成专辑封面」开关。
///
/// 每张卡上半截是一小段菜单栏:垫屏幕顶上那一条真实壁纸(`DesktopWallpaperTopBand`),按真菜单栏的明暗
/// 着色(`MenuBarAppearanceStore`),图标画在中间。指针停在卡上、且「图标动画」开着时换成
/// `MenuBarLiveIconView` 演这一款的动画;平时都是静止帧,12 张一起动太吵。
///
/// 用写死的 `HStack` / `VStack` 排,不用 LazyHGrid / LazyVGrid:Lazy 容器在窗口不可见时不铺格子,
/// 表现是整块空白。选中样式跟播放器页方块格的强调色那一档一致(`CompactTileChrome` 的 `accent`)。
@MainActor
struct MenuBarIconPicker: View {
    @ObservedObject private var settings = AppSettings.shared
    @ObservedObject private var menuBarAppearance = MenuBarAppearanceStore.shared

    static let columns = 4
    private static let spacing: CGFloat = 8

    private var rows: [[MenuBarIconStyle]] {
        let all = MenuBarIconStyle.allCases
        return stride(from: 0, to: all.count, by: Self.columns).map {
            Array(all[$0 ..< min($0 + Self.columns, all.count)])
        }
    }

    var body: some View {
        VStack(spacing: Self.spacing) {
            ForEach(rows.indices, id: \.self) { r in
                HStack(spacing: Self.spacing) {
                    ForEach(rows[r]) { style in
                        styleCard(style)
                    }
                    // 最后一行不满时补空位,卡宽跟上面几行一样。
                    ForEach(0 ..< (Self.columns - rows[r].count), id: \.self) { _ in
                        Color.clear.frame(maxWidth: .infinity)
                    }
                }
            }
            artworkCard
        }
        .accessibilityElement(children: .contain)
        .accessibilityLabel(L10n.t("菜单栏图标"))
    }

    private func styleCard(_ style: MenuBarIconStyle) -> some View {
        let selected = !settings.menuBarIconShowsArtwork && settings.menuBarIconStyle == style
        return Button {
            settings.menuBarIconStyle = style
            settings.menuBarIconShowsArtwork = false
        } label: {
            MenuBarIconStyleCard(style: style, selected: selected,
                                 animatesOnHover: settings.menuBarIconAnimates,
                                 isDark: menuBarAppearance.isDark)
        }
        .buttonStyle(.plain)
        .frame(maxWidth: .infinity)
        .help(style.displayName)
        .accessibilityLabel(style.displayName)
        .accessibilityAddTraits(selected ? .isSelected : [])
    }

    private var artworkCard: some View {
        let selected = settings.menuBarIconShowsArtwork
        return Button {
            settings.menuBarIconShowsArtwork = true
        } label: {
            MenuBarArtworkCard(selected: selected, fallback: settings.menuBarIconStyle,
                               isDark: menuBarAppearance.isDark)
        }
        .buttonStyle(.plain)
        .accessibilityLabel(L10n.t("专辑封面"))
        .accessibilityAddTraits(selected ? .isSelected : [])
        // 自绘的卡不会自动挂搜索高亮(只有 SettingsRow / SettingsCardHeader 会),手挂。
        .settingsSearchHighlight(title: L10n.t("专辑封面"))
    }
}

/// 卡的底和描边:选中 = 强调色淡底 + 强调色描边;未选 = 细描边;悬停加一层淡灰。
private struct MenuBarIconCardChrome: ViewModifier {
    let selected: Bool
    let hovering: Bool

    func body(content: Content) -> some View {
        let shape = RoundedRectangle(cornerRadius: 10, style: .continuous)
        content
            .background {
                ZStack {
                    if selected {
                        shape.fill(Color.accentColor.opacity(0.09))
                        shape.strokeBorder(Color.accentColor.opacity(0.7), lineWidth: 1.5)
                    } else {
                        shape.strokeBorder(Color.primary.opacity(0.1), lineWidth: 1)
                    }
                    shape.fill(Color.primary.opacity(hovering && !selected ? 0.05 : 0))
                }
            }
            .contentShape(shape)
    }
}

/// 一小段菜单栏:屏幕右上角那一块壁纸,按真菜单栏的明暗渲染里面的图标。macOS 26 起真菜单栏全透明、
/// 直接透出壁纸;更早的系统真菜单栏自带一层半透明底,这里同样铺 `.ultraThinMaterial`(同 `MenuBarPreviewBar`)。
private struct MenuBarMiniStrip<Content: View>: View {
    let isDark: Bool
    @ViewBuilder var content: () -> Content

    static var height: CGFloat { 24 }

    var body: some View {
        ZStack {
            wallpaper
            if !Self.bareWallpaper {
                Rectangle().fill(.ultraThinMaterial)
            }
            content()
        }
        .frame(maxWidth: .infinity)
        .frame(height: Self.height)
        .clipShape(RoundedRectangle(cornerRadius: 6, style: .continuous))
        .environment(\.colorScheme, isDark ? .dark : .light)
    }

    private var wallpaper: some View {
        Group {
            if let band = DesktopWallpaperTopBand.image {
                GeometryReader { geo in
                    Image(nsImage: band)
                        .resizable()
                        .frame(width: band.size.width, height: band.size.height)
                        .frame(width: geo.size.width, height: geo.size.height, alignment: .topTrailing)
                        .clipped()
                }
            } else {
                Color(nsColor: .textColor).opacity(0.14)
            }
        }
        .allowsHitTesting(false)
        .accessibilityHidden(true)
    }

    private static var bareWallpaper: Bool {
        if #available(macOS 26.0, *) { return true }
        return false
    }
}

/// 一款图标的卡:上面一小段菜单栏,下面款名。
private struct MenuBarIconStyleCard: View {
    let style: MenuBarIconStyle
    let selected: Bool
    let animatesOnHover: Bool
    let isDark: Bool
    @State private var hovering = false

    private var live: Bool {
        hovering && animatesOnHover && !NSWorkspace.shared.accessibilityDisplayShouldReduceMotion
    }

    var body: some View {
        VStack(spacing: 5) {
            MenuBarMiniStrip(isDark: isDark) {
                let image = MenuBarIconStyle.cachedImage(for: style)
                if live {
                    MenuBarLiveIconPreview(style: style, isDark: isDark)
                        .frame(width: image.size.width, height: image.size.height)
                } else {
                    // `.template` 跟在菜单栏上一样只按 alpha 上色。
                    Image(nsImage: image)
                        .renderingMode(.template)
                        .foregroundStyle(.primary)
                }
            }
            Text(style.displayName)
                .font(.system(size: 11, weight: selected ? .semibold : .regular))
                .foregroundStyle(selected ? Color.accentColor : Color.primary)
                .lineLimit(1)
                .minimumScaleFactor(0.8)
        }
        .padding(.horizontal, 6)
        .padding(.top, 6)
        .padding(.bottom, 5)
        .frame(maxWidth: .infinity)
        .modifier(MenuBarIconCardChrome(selected: selected, hovering: hovering))
        .onHover { hovering = $0 }
    }
}

/// 「专辑封面」卡:左边名字和「没有封面时显示哪一款」,右边一小段菜单栏摆正在放的封面。
private struct MenuBarArtworkCard: View {
    let selected: Bool
    let fallback: MenuBarIconStyle
    let isDark: Bool
    @ObservedObject private var playback = PlaybackCoordinator.shared
    @State private var hovering = false

    private var artwork: CGImage? {
        guard let source = playback.highResArtworkThumbnail ?? playback.highResArtworkImage ?? playback.artworkImage
        else { return nil }
        return MenuBarIconArtwork.thumbnail(of: source)
    }

    var body: some View {
        HStack(spacing: 12) {
            VStack(alignment: .leading, spacing: 2) {
                Text(L10n.t("专辑封面"))
                    .font(.system(size: 12, weight: selected ? .semibold : .regular))
                    .foregroundStyle(selected ? Color.accentColor : Color.primary)
                Text(String(format: L10n.t("没有封面时显示「%@」"), fallback.displayName))
                    .font(.system(size: 11))
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .truncationMode(.tail)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            MenuBarMiniStrip(isDark: isDark) { tile }
                .frame(width: 104)
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 7)
        .modifier(MenuBarIconCardChrome(selected: selected, hovering: hovering))
        .onHover { hovering = $0 }
    }

    /// 封面方块,大小和圆角同菜单栏上那一枚(`MenuBarIconArtwork.side` / `cornerRadius`)。没在放歌时画一块
    /// 淡色方块占位,不画所选款式:这张卡演的是「封面」这一档。
    @ViewBuilder private var tile: some View {
        let shape = RoundedRectangle(cornerRadius: MenuBarIconArtwork.cornerRadius, style: .continuous)
        if let artwork {
            Image(decorative: artwork, scale: 1)
                .resizable()
                .interpolation(.high)
                .frame(width: MenuBarIconArtwork.side, height: MenuBarIconArtwork.side)
                .clipShape(shape)
        } else {
            shape
                .fill(Color.primary.opacity(0.35))
                .frame(width: MenuBarIconArtwork.side, height: MenuBarIconArtwork.side)
        }
    }
}

/// 卡上演动画的那一枚:直接用菜单栏上同一个 `MenuBarLiveIconView`。它按自己的 effectiveAppearance 着色,
/// 所以 appearance 要显式钉成真菜单栏的明暗,SwiftUI 的 colorScheme 传不到 NSView 上。
private struct MenuBarLiveIconPreview: NSViewRepresentable {
    let style: MenuBarIconStyle
    let isDark: Bool

    func makeNSView(context: Context) -> MenuBarLiveIconView {
        MenuBarLiveIconView()
    }

    func updateNSView(_ view: MenuBarLiveIconView, context: Context) {
        view.appearance = NSAppearance(named: isDark ? .darkAqua : .aqua)
        view.present(style: style, size: .standard)
    }

    static func dismantleNSView(_ view: MenuBarLiveIconView, coordinator: ()) {
        view.clear()
    }
}
