import Foundation
import LyrimuseCore

// 导出用的最终读音(LyricsExportRomanization,10 章决策 66),由 runRomanizationTests 开头调。
@MainActor
func runExportRomanizationChecks() {
    typealias X = LyricsExportRomanization
    let korean = "[00:01.00]사랑해\n[00:03.00]안녕\n[00:05.00]고마워\n[00:07.00]미안해\n[00:09.00]보고 싶어"

    // 原样就是屏上那份(按现算的结果造):跟原样一致,不另存。
    guard case let .final(computed) = X.compute(.init(lyrics: korean, roma: "")) else {
        expectEqual(true, false, "LyricsExportRomanization: 原样没读音时应交出屏上现算的那份")
        return
    }
    expectEqual(computed.split(separator: "\n").count, 5, "LyricsExportRomanization: 五行韩文都有读音")
    expectEqual(X.compute(.init(lyrics: korean, roma: computed)), .sameAsStored,
                "LyricsExportRomanization: 原样跟屏上逐行一致,照原样导出")

    // 源读音只覆盖一行(不到八成):屏上整份不用、全部现算;导出跟屏上一样是五行。
    let partial = X.compute(.init(lyrics: korean, roma: "[00:01.00]saranghae"))
    expectEqual(partial, .final(computed), "LyricsExportRomanization: 源读音覆盖不到八成时导出屏上现算的那份")

    // 导出的那份再当原样算一遍还是它自己:导出、导入来回不变。
    if case let .final(roma) = partial {
        expectEqual(X.compute(.init(lyrics: korean, roma: roma)), .sameAsStored, "LyricsExportRomanization: 导出的读音再算一遍跟原样一致")
    }

    // 夹形近字母的拉丁行屏上不标读音:存着的那份不导出(决策 63)。
    expectEqual(X.compute(.init(lyrics: "[00:01.00]Ѕunt vаgаbоntul реrfес", roma: "[00:01.00]Sunt vagabontul perfec")), .final(""),
                "LyricsExportRomanization: 屏上整首不显示读音时导出空")
    expectEqual(X.compute(.init(lyrics: "[00:01.00]hello world", roma: "")), .sameAsStored,
                "LyricsExportRomanization: 拉丁字母歌没读音,原样也没有,照原样")
    expectEqual(X.compute(.init(lyrics: "", roma: "[00:01.00]x")), .final(""), "LyricsExportRomanization: 没正文时不导出读音")

    // 僧伽罗文:系统 ICU 没有它的音译,也要认成该有读音;读音用自带的表出,屏上显示、导出照原样。
    expectEqual(Romanizer.needsRomanization("හිටි හැටියේම"), true, "Romanizer.needsRomanization: 僧伽罗文要读音")
    let sinhala = "[00:01.00]හිටි හැටියේම ඇද හැලුණු"
    if case let .final(roma) = X.compute(.init(lyrics: sinhala, roma: "")) {
        expectEqual(roma.hasPrefix("[00:01.00]hiṭi"), true, "LyricsExportRomanization: 僧伽罗文屏上现算出读音,实得 \(roma)")
        expectEqual(X.compute(.init(lyrics: sinhala, roma: roma)), .sameAsStored, "LyricsExportRomanization: 僧伽罗文原样读音照原样导出")
    } else {
        expectEqual(true, false, "LyricsExportRomanization: 僧伽罗文应有读音")
    }

    // 沿用正文那一行的时间标签原文(三位毫秒)。
    if case let .final(roma) = X.compute(.init(lyrics: "[00:01.234]사랑해", roma: "")) {
        expectEqual(roma.hasPrefix("[00:01.234]"), true, "LyricsExportRomanization: 读音行沿用正文的时间标签,实得 \(roma)")
    } else {
        expectEqual(true, false, "LyricsExportRomanization: 一行韩文应有读音")
    }
    expectEqual(X.tag(61_230), "[01:01.23]", "LyricsExportRomanization.tag: 整十毫秒写两位")
    expectEqual(X.tag(1_234), "[00:01.234]", "LyricsExportRomanization.tag: 其余写三位")
    expectEqual(X.allScripts.contains([.japanese, .korean, .chinese, .cantonese, .thai, .malayalam]), true,
                "LyricsExportRomanization.allScripts: 读音语言全开")
}
