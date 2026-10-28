import AppKit
import LyrimuseCore
import SwiftUI

/// 推送内容编辑器(`DigestTemplateEditor`)的两个输入框。用 NSTextView 不用 SwiftUI 的 TextField / TextEditor:
/// 点占位符要插到光标处,SwiftUI 的两个都拿不到光标位置。认得的 `{占位符}` 用强调色画,认不出的画红色波浪线
/// (`DigestTemplate.tokens`)。`singleLine` 时回车不换行(标题只有一行)。
struct PlaceholderTextView: NSViewRepresentable {
    @Binding var text: String
    var singleLine = false
    let controller: PlaceholderInsertionController
    let field: PlaceholderInsertionController.Field
    /// 打开时由它拿焦点、光标放到末尾。
    var initialFocus = false

    func makeCoordinator() -> Coordinator { Coordinator(self) }

    func makeNSView(context: Context) -> NSScrollView {
        let scroll = NSScrollView()
        scroll.drawsBackground = false
        scroll.borderType = .noBorder
        scroll.hasVerticalScroller = !singleLine
        scroll.autohidesScrollers = true
        let tv = FocusReportingTextView()
        tv.onFocus = { [weak controller] in controller?.focused(field) }
        tv.delegate = context.coordinator
        tv.isRichText = false
        tv.allowsUndo = true
        tv.isAutomaticQuoteSubstitutionEnabled = false
        tv.isAutomaticDashSubstitutionEnabled = false
        tv.isAutomaticTextReplacementEnabled = false
        tv.isAutomaticSpellingCorrectionEnabled = false
        tv.drawsBackground = false
        tv.font = Self.baseFont
        tv.textContainerInset = NSSize(width: 4, height: singleLine ? 4 : 6)
        tv.isVerticallyResizable = true
        tv.isHorizontallyResizable = false
        tv.autoresizingMask = [.width]
        tv.textContainer?.widthTracksTextView = true
        if singleLine {
            tv.textContainer?.maximumNumberOfLines = 1
            tv.textContainer?.lineBreakMode = .byTruncatingTail
        }
        tv.string = text
        context.coordinator.highlight(tv)
        scroll.documentView = tv
        controller.register(tv, for: field)
        if initialFocus {
            DispatchQueue.main.async {
                tv.window?.makeFirstResponder(tv)
                tv.setSelectedRange(NSRange(location: (tv.string as NSString).length, length: 0))
            }
        }
        return scroll
    }

    func updateNSView(_ scroll: NSScrollView, context: Context) {
        context.coordinator.parent = self
        guard let tv = scroll.documentView as? NSTextView, tv.string != text else { return }
        tv.string = text
        context.coordinator.highlight(tv)
    }

    static let baseFont = NSFont.systemFont(ofSize: 13)

    final class Coordinator: NSObject, NSTextViewDelegate {
        var parent: PlaceholderTextView

        init(_ parent: PlaceholderTextView) { self.parent = parent }

        func textDidChange(_ notification: Notification) {
            guard let tv = notification.object as? NSTextView else { return }
            if parent.singleLine, tv.string.contains("\n") {
                // 粘贴进来的多行文字:标题里换行当空格。
                tv.string = tv.string.replacingOccurrences(of: "\n", with: " ")
            }
            parent.text = tv.string
            highlight(tv)
        }

        func textView(_ textView: NSTextView, doCommandBy selector: Selector) -> Bool {
            // 标题只有一行:回车不换行,交给 sheet 的默认按钮也不对(会直接保存),只吞掉。
            parent.singleLine && (selector == #selector(NSResponder.insertNewline(_:))
                || selector == #selector(NSResponder.insertLineBreak(_:)))
        }

        func highlight(_ tv: NSTextView) {
            guard let storage = tv.textStorage else { return }
            let full = NSRange(location: 0, length: storage.length)
            storage.beginEditing()
            storage.setAttributes([.font: PlaceholderTextView.baseFont, .foregroundColor: NSColor.labelColor], range: full)
            for token in DigestTemplate.tokens(in: tv.string) where NSMaxRange(token.range) <= storage.length {
                if token.known {
                    storage.addAttributes([.foregroundColor: NSColor.controlAccentColor,
                                           .backgroundColor: NSColor.controlAccentColor.withAlphaComponent(0.12)], range: token.range)
                } else {
                    storage.addAttributes([.foregroundColor: NSColor.systemRed,
                                           .underlineStyle: NSUnderlineStyle.single.rawValue | NSUnderlineStyle.patternDot.rawValue,
                                           .underlineColor: NSColor.systemRed], range: token.range)
                }
            }
            storage.endEditing()
            tv.typingAttributes = [.font: PlaceholderTextView.baseFont, .foregroundColor: NSColor.labelColor]
        }
    }
}

/// 拿焦点时告诉控制器(点占位符插到最近在编辑的那一栏)。
final class FocusReportingTextView: NSTextView {
    var onFocus: (() -> Void)?

    override func becomeFirstResponder() -> Bool {
        let ok = super.becomeFirstResponder()
        if ok { onFocus?() }
        return ok
    }
}

/// 记着两个输入框和最近在编辑的那一栏,点占位符时插到那一栏的光标处。
@MainActor
final class PlaceholderInsertionController: ObservableObject {
    enum Field { case title, body }

    @Published private(set) var lastField: Field = .body
    private var views: [Field: NSTextView] = [:]

    func register(_ tv: NSTextView, for field: Field) { views[field] = tv }
    func focused(_ field: Field) { lastField = field }

    /// 多行的占位符只进正文。插进去的文字见 `DigestTemplate.insertionText`。
    func insert(_ name: String) {
        let multiline = DigestTemplate.multilinePlaceholders.contains(name)
        let field: Field = multiline ? .body : lastField
        guard let tv = views[field] else { return }
        tv.window?.makeFirstResponder(tv)
        let ns = tv.string as NSString
        var range = tv.selectedRange()
        if range.location > ns.length { range = NSRange(location: ns.length, length: 0) }
        let before = range.location > 0 ? ns.substring(with: NSRange(location: range.location - 1, length: 1)).last : nil
        let afterIndex = NSMaxRange(range)
        let after = afterIndex < ns.length ? ns.substring(with: NSRange(location: afterIndex, length: 1)).first : nil
        tv.insertText(DigestTemplate.insertionText(for: name, before: before, after: after), replacementRange: range)
        tv.scrollRangeToVisible(tv.selectedRange())
    }
}
