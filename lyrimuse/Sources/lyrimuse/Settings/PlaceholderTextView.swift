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
            // 一行放不下时横着滚、光标跟着走(同系统单行输入框),不截成「…」。
            let unbounded = NSSize(width: CGFloat.greatestFiniteMagnitude, height: CGFloat.greatestFiniteMagnitude)
            tv.isHorizontallyResizable = true
            tv.maxSize = unbounded
            tv.textContainer?.widthTracksTextView = false
            tv.textContainer?.containerSize = unbounded
            tv.textContainer?.maximumNumberOfLines = 1
            tv.textContainer?.lineBreakMode = .byClipping
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
        context.coordinator.replaceAll(tv, with: text)
    }

    static let baseFont = NSFont.systemFont(ofSize: 13)

    final class Coordinator: NSObject, NSTextViewDelegate {
        var parent: PlaceholderTextView

        /// `replaceAll` 正在换文字:文字是外面给的,不回写 `parent.text`(SwiftUI 刷新途中改状态)。
        private var replacing = false

        init(_ parent: PlaceholderTextView) { self.parent = parent }

        /// 程序化换掉整段文字(点预设档、拆开默认正文、标题里的换行换成空格)。走 shouldChangeText / didChangeText,这一次替换
        /// 本身进撤销栈;直接写 `string` 不登记撤销,之后 ⌘Z 会按旧文字里的位置去改新文字。
        func replaceAll(_ tv: NSTextView, with text: String) {
            let full = NSRange(location: 0, length: (tv.string as NSString).length)
            guard tv.shouldChangeText(in: full, replacementString: text) else {
                tv.string = text
                tv.undoManager?.removeAllActions()
                highlight(tv)
                return
            }
            replacing = true
            defer { replacing = false }
            tv.replaceCharacters(in: full, with: text)
            tv.didChangeText()
        }

        func textDidChange(_ notification: Notification) {
            guard let tv = notification.object as? NSTextView else { return }
            if replacing {
                highlight(tv)
                return
            }
            if parent.singleLine, tv.string.contains("\n") {
                // 粘贴进来的多行文字:标题里换行当空格。
                replaceAll(tv, with: tv.string.replacingOccurrences(of: "\n", with: " "))
            }
            parent.text = tv.string
            highlight(tv)
        }

        func textView(_ textView: NSTextView, doCommandBy selector: Selector) -> Bool {
            // 标题只有一行:回车不换行,交给 sheet 的默认按钮也不对(会直接保存),只吞掉。
            parent.singleLine && (selector == #selector(NSResponder.insertNewline(_:))
                || selector == #selector(NSResponder.insertLineBreak(_:)))
        }

        /// 输入法正在组字时不上色:整段重设属性会抹掉组字的下划线和底色,上屏后 textDidChange 会再来一次。
        func highlight(_ tv: NSTextView) {
            guard let storage = tv.textStorage, !tv.hasMarkedText() else { return }
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
