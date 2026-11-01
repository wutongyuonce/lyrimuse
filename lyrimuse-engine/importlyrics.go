package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// 导入歌词文件:用户在「搜索候选歌词」面板里选的(或拖进来的)歌词文件,解析成一条来源为 import 的候选,
// 面板照常预览、采用(采用走 save_edit,见 enrichedit.go)。文件由 App 读出来交过来:引擎是另一个进程,
// 读不了「桌面」「文稿」这类受系统保护的目录,用户在选择框里点了的文件只有 App 有权限读。
// 格式认后缀、认不出再看内容:TTML、QRC / KRC(加密或明文)、YRC、增强 LRC(带逐字)、LRC、纯文本;同名的
// .tr.lrc / .roma.lrc 当译文 / 读音配上。编码按 decodeLyricBytes 的规矩转。见 11 章决策 149。

// importLyricsSource:导入的那份候选、以及采用后条目的来源名。
const importLyricsSource = "import"

// lyricsImportMaxBytes:一个文件大过这个不读(不是歌词)。
const lyricsImportMaxBytes = 4 << 20

// importLyricsFile:App 交来的一个文件。Data 在 JSON 里是 base64。
type importLyricsFile struct {
	Name string `json:"name"`
	// Path:批量导入时这个文件在所选文件夹里的相对路径(含文件名),分组看它的目录;单首导入不带。
	Path string `json:"path,omitempty"`
	Data []byte `json:"data"`
}

// importLyricsResult:解析结果。认不出时 Candidate 为 nil、Error 说明原因(empty / unrecognized / too-large)。
type importLyricsResult struct {
	Candidate *scoredLyricCandidateResult `json:"candidate,omitempty"`
	// Format:正文是按哪种格式认的(ttml / qrc / krc / yrc / elrc / lrc / text)。
	Format string `json:"format,omitempty"`
	// Used / Ignored:用上的、没用上的文件名(同一首给了好几份正文时只用一份)。
	Used    []string `json:"used,omitempty"`
	Ignored []string `json:"ignored,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// importLyricsKind:一个文件按什么用。数字越小越优先当正文。
type importLyricsKind int

const (
	importKindTTML importLyricsKind = iota
	importKindQRC
	importKindKRC
	importKindYRC
	importKindLRC
	importKindText
	importKindTr
	importKindRoma
)

var importLyricsKindNames = map[importLyricsKind]string{
	importKindTTML: "ttml", importKindQRC: "qrc", importKindKRC: "krc", importKindYRC: "yrc",
	importKindLRC: "lrc", importKindText: "text",
}

var (
	importYRCLineRe = regexp.MustCompile(`(?m)^\[\d+,\d+\]`)
	importLRCLineRe = regexp.MustCompile(`(?m)^\s*\[\d+:\d+(?:[.:]\d{1,3})?\]`)
	// 正文开头别人家的身份标签:读出来当候选的歌名歌手专辑。
	importTagRe = regexp.MustCompile(`(?m)^\s*\[(ar|ti|al):\s*(.*?)\s*\]\s*$`)
	// importCreditLineRe:身份、署名这类标签行,读完身份就从正文里去掉(不显示,也别算进行数)。[offset:] 不在里面,显示层要用。
	importCreditLineRe = regexp.MustCompile(`(?m)^[ \t]*\[(?:ar|ti|al|by|au|re|ve|length|tool):[^\]\n]*\][ \t]*(?:\r?\n|$)`)
)

// importLyricsKindOf 按后缀认,认不出(或后缀是 .txt / 没有)再看内容。
func importLyricsKindOf(name string, data []byte) importLyricsKind {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".tr.lrc"):
		return importKindTr
	case strings.HasSuffix(lower, ".roma.lrc"):
		return importKindRoma
	case strings.HasSuffix(lower, ".ttml"):
		return importKindTTML
	case strings.HasSuffix(lower, ".qrc"):
		return importKindQRC
	case strings.HasSuffix(lower, ".krc"):
		return importKindKRC
	case strings.HasSuffix(lower, ".yrc"):
		return importKindYRC
	case strings.HasSuffix(lower, ".lrc"), strings.HasSuffix(lower, ".lrcx"):
		return importKindLRC
	}
	if bytes.HasPrefix(data, []byte("krc1")) {
		return importKindKRC
	}
	text, _ := openPlainLyricsText(data)
	trimmed := strings.TrimSpace(text)
	switch {
	case strings.Contains(trimmed, "<tt") && strings.Contains(trimmed, "</tt>"):
		return importKindTTML
	case strings.Contains(trimmed, "<QrcInfos") || strings.Contains(trimmed, "LyricContent="):
		return importKindQRC
	case importYRCLineRe.MatchString(trimmed):
		return importKindYRC
	case importLRCLineRe.MatchString(trimmed):
		return importKindLRC
	}
	return importKindText
}

// importBody:去掉 Lyrimuse 自己导出时写的头部(有的话,连同里面的歌名歌手专辑一起交回),别人家的标签行留着
// (显示层认得 [offset:],别的不是计时行、不显示)。
func importBody(text string) (string, parsedLyricsFile) {
	if p := parseLyricsBytes([]byte(text)); p.ok {
		return p.body, p
	}
	return strings.TrimPrefix(text, "\ufeff"), parsedLyricsFile{}
}

// importLooksBinary:文本类的文件里有 NUL,或者按 UTF-8、GB18030、Big5 都转不成文字:多半是拖错了文件(音频、图片),
// 不当歌词。QRC / KRC 本来就是加密的二进制,不走这里。
func importLooksBinary(data []byte) bool {
	if bytes.IndexByte(data, 0) >= 0 {
		return true
	}
	_, ok := decodeLyricBytesRaw(data)
	return !ok
}

// importHasTimestamps:有没有带时间戳的歌词行。不用 isTimedLRC:那是给搜索结果的(至少三行、过半带时间戳),
// 开头一串标签行的短文件会被它判成纯文本。
func importHasTimestamps(lrc string) bool {
	return importLRCLineRe.MatchString(lrc)
}

// parseImportedLyrics 把一组文件解析成一条候选。trTarget:TTML 里有几种译文时挑哪种(设置里的译文语言)。
func parseImportedLyrics(files []importLyricsFile, trTarget string) importLyricsResult {
	var res importLyricsResult
	if len(files) == 0 {
		res.Error = "empty"
		return res
	}
	type entry struct {
		file importLyricsFile
		kind importLyricsKind
	}
	var main *entry
	var tr, roma *entry
	all := make([]entry, 0, len(files))
	tooLarge := false
	for _, f := range files {
		if len(f.Data) > lyricsImportMaxBytes {
			res.Ignored = append(res.Ignored, f.Name)
			tooLarge = true
			continue
		}
		if len(bytes.TrimSpace(f.Data)) == 0 {
			res.Ignored = append(res.Ignored, f.Name)
			continue
		}
		kind := importLyricsKindOf(f.Name, f.Data)
		if kind != importKindQRC && kind != importKindKRC && importLooksBinary(f.Data) {
			res.Ignored = append(res.Ignored, f.Name)
			continue
		}
		all = append(all, entry{file: f, kind: kind})
	}
	for i := range all {
		e := &all[i]
		switch e.kind {
		case importKindTr:
			if tr == nil {
				tr = e
			}
		case importKindRoma:
			if roma == nil {
				roma = e
			}
		default:
			if main == nil || e.kind < main.kind {
				main = e
			}
		}
	}
	if main == nil {
		res.Error = "unrecognized"
		if len(all) == 0 && tooLarge {
			res.Error = "too-large"
		}
		return res
	}

	c := scoredLyricCandidateResult{Source: importLyricsSource}
	var tagText string
	var own parsedLyricsFile
	switch main.kind {
	case importKindTTML:
		plain, _ := openPlainLyricsText(main.file.Data)
		r, ok := parseAMLLTTMLFor(plain, trTarget)
		if !ok || (r.lrc == "" && r.yrc == "") {
			res.Error = "unrecognized"
			return res
		}
		c.Lyrics, c.LyricsYRC, c.LyricsTr, c.LyricsTrLang, c.LyricsRoma, c.LyricsBG = r.lrc, r.yrc, r.tr, r.trLang, r.roma, r.bg
		if head := decodeTTMLWordText(plain); head.title != "" {
			c.Title, c.Artist, c.Album = head.title, head.artist, head.album
		}
	case importKindQRC:
		plain, ok := openLyricsWordFile(lyricsWordQRC, main.file.Data)
		if !ok {
			res.Error = "unrecognized"
			return res
		}
		content, id := importBody(extractQRCLyricContent(plain))
		own = id
		c.Lyrics, c.LyricsYRC = qrcToLineLRC(content), qrcToYRC(content)
		tagText = content
	case importKindKRC:
		plain, ok := openLyricsWordFile(lyricsWordKRC, main.file.Data)
		if !ok {
			res.Error = "unrecognized"
			return res
		}
		lang, rest := splitKRCLanguageLine(strings.TrimPrefix(plain, "\ufeff"))
		body, id := importBody(rest)
		own = id
		c.Lyrics = krcToLRC(body)
		if len(krcLineStarts(body)) > 0 {
			c.LyricsYRC = krcToYRC(body)
			c.LyricsTr, c.LyricsRoma = krcLanguageTracks(lang, body)
		}
		tagText = body
	case importKindYRC:
		text, _ := openPlainLyricsText(main.file.Data)
		body, id := importBody(text)
		own = id
		c.LyricsYRC = strings.TrimSpace(body)
		c.Lyrics = yrcToLineLRC(c.LyricsYRC)
		tagText = c.LyricsYRC
	case importKindLRC:
		text, _ := openPlainLyricsText(main.file.Data)
		body, id := importBody(text)
		own = id
		if lines, yrc, split := splitWordTimedLRC(body); split {
			c.Lyrics, c.LyricsYRC = lines, yrc
			res.Format = "elrc"
		} else {
			c.Lyrics = body
		}
		tagText = body
	case importKindText:
		text, _ := openPlainLyricsText(main.file.Data)
		body, id := importBody(text)
		own = id
		c.Lyrics = strings.TrimSpace(body)
		c.PlainTextOnly = true
	}
	if res.Format == "" {
		res.Format = importLyricsKindNames[main.kind]
	}
	if strings.TrimSpace(c.Lyrics) == "" && strings.TrimSpace(c.LyricsYRC) == "" {
		res.Error = "unrecognized"
		return res
	}
	if !c.PlainTextOnly && !importHasTimestamps(c.Lyrics) && c.LyricsYRC == "" {
		// 后缀是 .lrc、里面却一个时间戳都没有:当纯文本。
		c.PlainTextOnly = true
		res.Format = "text"
	}
	res.Used = append(res.Used, main.file.Name)
	if own.ok && c.Title == "" {
		c.Title, c.Artist, c.Album = own.title, own.artist, own.album
	}
	if tagText != "" {
		for _, m := range importTagRe.FindAllStringSubmatch(tagText, -1) {
			switch m[1] {
			case "ar":
				if c.Artist == "" {
					c.Artist = m[2]
				}
			case "ti":
				if c.Title == "" {
					c.Title = m[2]
				}
			case "al":
				if c.Album == "" {
					c.Album = m[2]
				}
			}
		}
	}
	c.Lyrics = strings.TrimSpace(importCreditLineRe.ReplaceAllString(c.Lyrics, ""))
	if !c.PlainTextOnly {
		if tr != nil {
			text, _ := openPlainLyricsText(tr.file.Data)
			body, _ := importBody(text)
			c.LyricsTr, c.LyricsTrLang = strings.TrimSpace(body), ""
			res.Used = append(res.Used, tr.file.Name)
		}
		if roma != nil {
			text, _ := openPlainLyricsText(roma.file.Data)
			body, _ := importBody(text)
			c.LyricsRoma = strings.TrimSpace(body)
			res.Used = append(res.Used, roma.file.Name)
		}
	}
	for _, e := range all {
		if e.file.Name != main.file.Name && (tr == nil || e.file.Name != tr.file.Name) && (roma == nil || e.file.Name != roma.file.Name) {
			res.Ignored = append(res.Ignored, e.file.Name)
		}
	}
	c.HasWordTiming = c.LyricsYRC != ""
	res.Candidate = &c
	return res
}

// yrcToLineLRC:YRC 每一行压成整行 LRC(行始 + 词连起来)。没有计时行时回空。
func yrcToLineLRC(yrc string) string {
	var out []string
	for _, l := range parseYRCDoc(yrc) {
		if !l.timed {
			continue
		}
		var b strings.Builder
		b.WriteString(l.lead)
		for _, w := range l.words {
			b.WriteString(w.text)
		}
		out = append(out, lrcTimestamp(l.start)+strings.TrimSpace(b.String()))
	}
	return strings.Join(out, "\n")
}

// runParseLyricsFilesCLI:`lyrimuse-engine parse-lyrics-files [请求文件]`。请求是一份 {"files":[{"name","data"}]}(给了路径读文件,
// 没给读标准输入:App 交来的文件可能有几 MB,写临时文件比塞管道稳),标准输出一份 importLyricsResult。只读,不碰缓存,不拿单实例锁。
func runParseLyricsFilesCLI(args []string) {
	if configDir() != "" {
		setFeatures(loadFeatureFlags(filepath.Join(configDir(), clientName+"-features.json")))
	}
	var in io.Reader = os.Stdin
	if len(args) > 0 {
		f, err := os.Open(args[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, "parse-lyrics-files: open request:", err)
			os.Exit(2)
		}
		defer f.Close()
		in = f
	}
	raw, err := io.ReadAll(io.LimitReader(in, 64<<20))
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse-lyrics-files: read request:", err)
		os.Exit(2)
	}
	var req struct {
		Files []importLyricsFile `json:"files"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		fmt.Fprintln(os.Stderr, "parse-lyrics-files: bad request:", err)
		os.Exit(2)
	}
	out, _ := json.Marshal(parseImportedLyrics(req.Files, features().LyricsTranslationLanguage))
	fmt.Println(string(out))
}
