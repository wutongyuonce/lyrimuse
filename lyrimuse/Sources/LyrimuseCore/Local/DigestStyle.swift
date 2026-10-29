import Foundation

/// 听歌报告推到一个渠道时用哪一套内容(渠道的 `digest_style`,引擎 digeststyle.go):完整、精简、概况三套预设,
/// 各平台照旧按自己的格式排版;自定义按这个渠道自己的模板(`digest_template`)出标题正文、按纯文本推。
/// 渠道没选过(`digest_style` 为空)时沿用 features.json 里四档共用的那份模板:自定义过就按它,没有就是完整。
/// 正文怎么拼见 `NotificationPreview.body` / `richSections` / `newArtistsLine`。见 15 章决策 90。
public enum DigestStyle: String, CaseIterable, Sendable {
    case full, brief, overview, custom
}

extension NotificationChannel {
    /// 这个渠道按哪一套推、自定义时用哪份模板(同引擎 notifyChannel.digestStyle)。`legacy` 是 features.json 里那份。
    /// 选了自定义却没写模板按完整推;认不出的值按完整推。
    public func resolvedDigest(legacy: DigestTemplate) -> (style: DigestStyle, template: DigestTemplate) {
        switch DigestStyle(rawValue: digestStyle) {
        case .brief?, .overview?:
            return (DigestStyle(rawValue: digestStyle) ?? .full, DigestTemplate())
        case .custom?:
            if digestTemplate.isCustom { return (.custom, digestTemplate) }
        case .full?:
            break
        case nil:
            if digestStyle.isEmpty, legacy.isCustom { return (.custom, legacy) }
        }
        return (.full, DigestTemplate())
    }

    /// 编辑弹窗里用的那份:没选过的按沿用的结果写成明确的一套(保存后就不再跟着 features.json 那份变)。
    public func pinningDigestStyle(legacy: DigestTemplate) -> NotificationChannel {
        guard digestStyle.isEmpty else { return self }
        var c = self
        let r = resolvedDigest(legacy: legacy)
        c.digestStyle = r.style.rawValue
        if r.style == .custom { c.digestTemplate = r.template }
        return c
    }
}
