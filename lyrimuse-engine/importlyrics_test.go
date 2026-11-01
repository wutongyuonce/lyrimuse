package main

import (
	"os"
	"strings"
	"testing"
)

func importOne(t *testing.T, files ...importLyricsFile) importLyricsResult {
	t.Helper()
	return parseImportedLyrics(files, "zh")
}

func file(name, data string) importLyricsFile {
	return importLyricsFile{Name: name, Data: []byte(data)}
}

// 别人家的 LRC:身份标签当歌名歌手,正文原样(标签行留着,显示层认 [offset:]),配同名的 .tr.lrc / .roma.lrc。
func TestImportLyricsPlainLRCWithCompanions(t *testing.T) {
	lrc := "\ufeff[ti:晴天]\n[ar:周杰伦]\n[al:叶惠美]\n[offset:200]\n[00:01.00]故事的小黄花\n[00:03.50]从出生那年就飘着"
	r := importOne(t, file("周杰伦 - 晴天.lrc", lrc),
		file("周杰伦 - 晴天.tr.lrc", "[00:01.00]The little yellow flower\n[00:03.50]Has been floating"),
		file("周杰伦 - 晴天.roma.lrc", "[00:01.00]gù shì de xiǎo huáng huā"))
	c := r.Candidate
	if c == nil || r.Error != "" {
		t.Fatalf("应解析出候选: %+v", r)
	}
	if c.Source != lyricsImportSource || c.Title != "晴天" || c.Artist != "周杰伦" || c.Album != "叶惠美" {
		t.Errorf("来源和身份不对: %+v", c)
	}
	if c.Lyrics != "[offset:200]\n[00:01.00]故事的小黄花\n[00:03.50]从出生那年就飘着" {
		t.Errorf("正文应去 BOM、去身份标签行,留 [offset:]: %q", c.Lyrics)
	}
	if !strings.Contains(c.LyricsTr, "yellow flower") || !strings.Contains(c.LyricsRoma, "huáng") {
		t.Errorf("译文 / 读音没配上: tr=%q roma=%q", c.LyricsTr, c.LyricsRoma)
	}
	if c.HasWordTiming || c.PlainTextOnly || r.Format != "lrc" || len(r.Used) != 3 {
		t.Errorf("格式 / 用到的文件不对: %+v used=%v", r.Format, r.Used)
	}
}

// 增强 LRC(带 <mm:ss.xx> 词时间):拆成整行 + 逐字。
func TestImportLyricsEnhancedLRC(t *testing.T) {
	r := importOne(t, file("a.lrc", "[00:01.00]<00:01.00>Hello <00:01.50>world<00:02.00>\n[00:03.00]<00:03.00>Bye<00:03.80>"))
	c := r.Candidate
	if c == nil || !c.HasWordTiming || r.Format != "elrc" || !strings.Contains(c.Lyrics, "[00:01.00]Hello world") || c.LyricsYRC == "" {
		t.Errorf("增强 LRC 应拆成整行 + 逐字: %+v", r)
	}
}

// YRC:整行从逐字压出来。
func TestImportLyricsYRC(t *testing.T) {
	r := importOne(t, file("song.yrc", "[1000,2000](1000,500,0)Hel(1500,500,0)lo\n[3000,1000](3000,1000,0)Bye"))
	c := r.Candidate
	if c == nil || r.Format != "yrc" || !c.HasWordTiming || c.Lyrics != "[00:01.00]Hello\n[00:03.00]Bye" {
		t.Errorf("YRC 应压出整行: %+v / %+v", r, c)
	}
}

// TTML、QRC、KRC 用引擎自己导出的写法造样本:导进来跟导出前的内容一致。
func TestImportLyricsWordFormatsRoundTrip(t *testing.T) {
	in := lyricsWordFileInput{artist: "A", title: "T", album: "X", source: "qq",
		lrc:          "[00:01.00]你好\n[00:03.00]再见\n[00:05.00]明天\n[00:07.00]见",
		tr:           "[00:01.00]Hello\n[00:03.00]Bye\n[00:05.00]Tomorrow\n[00:07.00]See you",
		roma:         "[00:01.00]nǐ hǎo\n[00:03.00]zài jiàn\n[00:05.00]míng tiān\n[00:07.00]jiàn",
		durationSecs: 10}
	in.yrc = "[1000,2000](1000,1000,0)你(2000,1000,0)好\n[3000,2000](3000,1000,0)再(4000,1000,0)见\n" +
		"[5000,2000](5000,1000,0)明(6000,1000,0)天\n[7000,1000](7000,1000,0)见"
	in.nativeYRC = in.yrc
	for _, f := range []lyricsWordFormat{lyricsWordTTML, lyricsWordQRC, lyricsWordKRC} {
		data := sealLyricsWordFile(f, encodeLyricsWordText(f, in))
		r := importOne(t, importLyricsFile{Name: "A - T." + string(f), Data: data})
		c := r.Candidate
		if c == nil || r.Format != string(f) {
			t.Fatalf("%s: 应认出: %+v", f, r)
		}
		if !c.HasWordTiming || !strings.Contains(c.Lyrics, "你好") || !strings.Contains(c.Lyrics, "再见") || !strings.Contains(c.LyricsYRC, "(2000,1000,0)好") {
			t.Errorf("%s: 正文 / 逐字不对: lrc=%q yrc=%q", f, c.Lyrics, c.LyricsYRC)
		}
		// TTML 带译文和读音;KRC 只带译文(酷狗的读音轨是逐词的,导出不写);QRC 都不带。
		if f != lyricsWordQRC && !strings.Contains(c.LyricsTr, "Hello") {
			t.Errorf("%s: 自带的译文没拆出来: tr=%q", f, c.LyricsTr)
		}
		if f == lyricsWordTTML && !strings.Contains(c.LyricsRoma, "nǐ hǎo") {
			t.Errorf("%s: 自带的读音没拆出来: roma=%q", f, c.LyricsRoma)
		}
		if c.Title != "T" || c.Artist != "A" {
			t.Errorf("%s: 身份没读出来: %+v", f, c)
		}
	}
}

// 纯文本(.txt、或者 .lrc 里一个时间戳都没有):当静态文本。
func TestImportLyricsPlainText(t *testing.T) {
	for _, f := range []importLyricsFile{file("words.txt", "第一句\n第二句"), file("words.lrc", "第一句\n第二句")} {
		r := importOne(t, f)
		if r.Candidate == nil || !r.Candidate.PlainTextOnly || r.Format != "text" || r.Candidate.Lyrics != "第一句\n第二句" {
			t.Errorf("%s 应当纯文本: %+v", f.Name, r)
		}
	}
	// 纯文本不配译文 / 读音(没有时间戳对不上)。
	r := importOne(t, file("w.txt", "一句"), file("w.tr.lrc", "[00:01.00]x"))
	if r.Candidate == nil || r.Candidate.LyricsTr != "" {
		t.Errorf("纯文本不该配译文: %+v", r.Candidate)
	}
}

// Big5 / GBK 的文件按 decodeLyricBytes 转码;Lyrimuse 自己导出的文件头去掉。
func TestImportLyricsEncodingAndOwnHeader(t *testing.T) {
	r := importOne(t, importLyricsFile{Name: "big5.lrc", Data: []byte(big5Lyrics)})
	if r.Candidate == nil || !strings.Contains(r.Candidate.Lyrics, "我愛你") {
		t.Errorf("Big5 应转码: %+v", r)
	}
	own := lyricsFileHeader("A", "T", "X", "qq", true) + "[00:01.00]正文"
	r = importOne(t, file("A - T - X.lrc", own))
	if r.Candidate == nil || r.Candidate.Lyrics != "[00:01.00]正文" {
		t.Errorf("自己导出的文件头应去掉: %q", r.Candidate.Lyrics)
	}
}

// 同时给了好几份正文:带逐字的优先,别的记进 Ignored;空的、太大的不用;一份都认不出报原因。
func TestImportLyricsPicksBestAndReportsProblems(t *testing.T) {
	r := importOne(t, file("a.lrc", "[00:01.00]一句"), file("a.yrc", "[1000,1000](1000,1000,0)一句"), file("empty.lrc", "  \n"))
	if r.Candidate == nil || r.Format != "yrc" || len(r.Ignored) != 2 {
		t.Errorf("应挑逐字那份,另两份记进 Ignored: %+v", r)
	}
	if r := importOne(t); r.Error != "empty" {
		t.Errorf("没文件应报 empty: %+v", r)
	}
	if r := importOne(t, file("x.lrc", "   ")); r.Error != "unrecognized" {
		t.Errorf("只有空文件应报 unrecognized: %+v", r)
	}
	big := importLyricsFile{Name: "big.lrc", Data: make([]byte, lyricsImportMaxBytes+1)}
	if r := importOne(t, big); r.Error != "too-large" {
		t.Errorf("太大应报 too-large: %+v", r)
	}
	// 拖错了文件(音频、图片):不当纯文本导进来。
	mp3 := importLyricsFile{Name: "song.mp3", Data: []byte("ID3\x04\x00\x00\x00\x00\x00\x23TIT2\x00\x00")}
	if r := importOne(t, mp3); r.Error != "unrecognized" || r.Candidate != nil {
		t.Errorf("二进制文件应报 unrecognized: %+v", r)
	}
	if r := importOne(t, file("a.lrc", "[00:01.00]一句"), mp3); r.Candidate == nil || len(r.Ignored) != 1 || r.Ignored[0] != "song.mp3" {
		t.Errorf("跟歌词一起拖进来的二进制文件应记进 Ignored: %+v", r)
	}
	if r := importOne(t, file("only.tr.lrc", "[00:01.00]x")); r.Error != "unrecognized" {
		t.Errorf("只有译文没有正文应报 unrecognized: %+v", r)
	}
}

// 后缀不认识时看内容。
func TestImportLyricsKindBySniffing(t *testing.T) {
	cases := map[string]importLyricsKind{
		"[00:01.00]x":                    importKindLRC,
		"[1000,500](1000,500,0)x":        importKindYRC,
		`<?xml version="1.0"?><tt></tt>`: importKindTTML,
		"just words":                     importKindText,
	}
	for data, want := range cases {
		if got := importLyricsKindOf("pasted", []byte(data)); got != want {
			t.Errorf("%q 认成 %v,应为 %v", data, got, want)
		}
	}
	if got := importLyricsKindOf("x.bin", []byte("krc1\x00\x01")); got != importKindKRC {
		t.Errorf("krc1 开头应认成 KRC,实得 %v", got)
	}
}

// 接线:命令行子命令登记了、只读(不碰单实例锁、不存盘)。
func TestImportLyricsCLIWiring(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !containsCode(string(src), `os.Args[1] == "parse-lyrics-files"`) || !containsCode(string(src), "runParseLyricsFilesCLI(os.Args[2:])") {
		t.Error("main.go 应登记 parse-lyrics-files")
	}
	body, err := os.ReadFile("importlyrics.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"saveEnrichCache", "acquireSingleInstance", "enrichMu.Lock"} {
		if strings.Contains(string(body), banned) {
			t.Errorf("解析导入文件应只读,不该出现 %s", banned)
		}
	}
}
