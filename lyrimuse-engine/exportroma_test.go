package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func romaPtr(s string) *string { return &s }

// exportRomaFixture:临时缓存 + 临时歌词文件夹 + 假 helper。helper 每次被问到的条目按正文记进 asked,规则版本按 rules。
type exportRomaFixture struct {
	dir   string
	asked []string
	rules int
	reply func(it exportRomaItem) exportRomaResult
	fail  error
}

func withExportRomaFixture(t *testing.T, entries map[string]enrichEntry) *exportRomaFixture {
	t.Helper()
	withTempLeanCache(t)
	fx := &exportRomaFixture{dir: t.TempDir(), rules: 1, reply: func(exportRomaItem) exportRomaResult { return exportRomaResult{Same: true} }}
	oldDir := lyricsDir()
	setLyricsDir(fx.dir)
	savedHelper := exportRomaHelper
	exportRomaSeenMu.Lock()
	savedSeen := exportRomaSeenBy
	exportRomaSeenBy = map[string]exportRomaSeen{}
	exportRomaSeenMu.Unlock()
	t.Cleanup(func() {
		setLyricsDir(oldDir)
		exportRomaHelper = savedHelper
		exportRomaSeenMu.Lock()
		exportRomaSeenBy = savedSeen
		exportRomaSeenMu.Unlock()
	})
	exportRomaHelper = func(items []exportRomaItem) ([]exportRomaResult, int, error) {
		if fx.fail != nil {
			return nil, 0, fx.fail
		}
		out := make([]exportRomaResult, len(items))
		for i, it := range items {
			fx.asked = append(fx.asked, it.Lyrics)
			out[i] = fx.reply(it)
		}
		return out, fx.rules, nil
	}
	enrichMu.Lock()
	for k, e := range entries {
		enrichCache[k] = e
	}
	enrichMu.Unlock()
	return fx
}

func exportRomaEntry(key string) enrichEntry {
	enrichMu.Lock()
	defer enrichMu.Unlock()
	return enrichCache[key]
}

// 存着的导出读音只在输入指纹对得上、不是手改条目时用;否则照原样。
func TestExportedRomaUsesStoredOnlyWhenFresh(t *testing.T) {
	const key = "A|T|X"
	lyrics, roma := "[00:01.00]君の名は", "[00:01.00]kimi no na wa（kimi）"
	src := exportRomaSource(key, lyrics, roma, "", "", "", "")
	e := enrichEntry{Lyrics: bt(lyrics), LyricsRoma: bt(roma), LyricsRomaExport: romaPtr("[00:01.00]kimi no na wa"), LyricsRomaExportSrc: src}
	if got := exportedRoma(key, e); got != "[00:01.00]kimi no na wa" {
		t.Errorf("指纹对得上应导出存着的那份,实得 %q", got)
	}
	stale := e
	stale.Lyrics = bt("[00:01.00]君の名は。")
	if got := exportedRoma(key, stale); got != roma {
		t.Errorf("正文变了应照原样导出,实得 %q", got)
	}
	if got := exportedRoma("A|T|Y", e); got != roma {
		t.Errorf("换了写法键(同曲同步拷过去的)应照原样导出,实得 %q", got)
	}
	manual := e
	manual.ManualLyrics = true
	if got := exportedRoma(key, manual); got != roma {
		t.Errorf("手改条目应照原样导出,实得 %q", got)
	}
	none := e
	none.LyricsRomaExport = nil
	if got := exportedRoma(key, none); got != roma {
		t.Errorf("没有导出读音应照原样导出,实得 %q", got)
	}
	cleared := e
	cleared.LyricsRomaExport = romaPtr("")
	if got := exportedRoma(key, cleared); got != "" {
		t.Errorf("屏上整首不显示读音应导出空,实得 %q", got)
	}
	if exportRomaSource(key, lyrics, roma, "", "", "", "") == exportRomaSource(key, lyrics, roma, "", "", "", songLanguageCantonese) {
		t.Error("语种变了指纹应跟着变")
	}
}

// 只问有正文、不是纯音乐、不是手改、原样有读音的条目;跟原样一致的只记指纹,不同的存下来,整首不显示读音的存空串。
func TestRefreshExportRomaStoresOnlyDifferences(t *testing.T) {
	const same, differs, cleared, manual, inst, noRoma = "A|same|X", "A|diff|X", "A|clear|X", "A|manual|X", "A|inst|X", "A|noroma|X"
	fx := withExportRomaFixture(t, map[string]enrichEntry{
		same:    {Lyrics: bt("[00:01.00]사랑해"), LyricsRoma: bt("[00:01.00]saranghae")},
		differs: {Lyrics: bt("[00:01.00]君の名は"), LyricsRoma: bt("[00:01.00]v1：kimi no na wa"), SongLanguage: songLanguageHokkien},
		cleared: {Lyrics: bt("[00:01.00]Fеmеіе"), LyricsRoma: bt("[00:01.00]Femeie")},
		manual:  {Lyrics: bt("[00:01.00]我爱你"), LyricsRoma: bt("[00:01.00]wo"), ManualLyrics: true},
		inst:    {Lyrics: bt("[00:01.00]我爱你"), LyricsRoma: bt("[00:01.00]wo"), Instrumental: true},
		noRoma:  {Lyrics: bt("[00:01.00]hello")},
	})
	var gotItem exportRomaItem
	fx.reply = func(it exportRomaItem) exportRomaResult {
		switch it.Lyrics {
		case "[00:01.00]君の名は":
			gotItem = it
			return exportRomaResult{Roma: romaPtr("[00:01.00]kimi no na wa")}
		case "[00:01.00]Fеmеіе":
			return exportRomaResult{Roma: romaPtr("")}
		}
		return exportRomaResult{Same: true}
	}
	if n := refreshExportRoma(context.Background(), nil); n != 2 {
		t.Fatalf("导出内容应变 2 条,实变 %d", n)
	}
	slices.Sort(fx.asked)
	if want := []string{"[00:01.00]Fеmеіе", "[00:01.00]君の名は", "[00:01.00]사랑해"}; !slices.Equal(fx.asked, want) {
		t.Errorf("只该问原样有读音的三条,实问 %v", fx.asked)
	}
	if gotItem.Roma != "[00:01.00]v1：kimi no na wa" || gotItem.Title != "diff" || gotItem.Artist != "A" || !gotItem.Hokkien || gotItem.Cantonese {
		t.Errorf("交给 helper 的应带原样读音、歌名歌手、语种,实得 %+v", gotItem)
	}
	if e := exportRomaEntry(same); e.LyricsRomaExport != nil || e.LyricsRomaExportSrc == 0 || e.LyricsRomaExportRules != 1 {
		t.Errorf("跟原样一致的只记指纹和规则版本,实得 %v %d %d", e.LyricsRomaExport, e.LyricsRomaExportSrc, e.LyricsRomaExportRules)
	}
	if e := exportRomaEntry(differs); e.LyricsRomaExport == nil || *e.LyricsRomaExport != "[00:01.00]kimi no na wa" {
		t.Errorf("不同的应存下来,实得 %v", e.LyricsRomaExport)
	}
	if e := exportRomaEntry(cleared); e.LyricsRomaExport == nil || *e.LyricsRomaExport != "" {
		t.Errorf("整首不显示读音的应存空串,实得 %v", e.LyricsRomaExport)
	}
	for _, k := range []string{manual, inst, noRoma} {
		if e := exportRomaEntry(k); e.LyricsRomaExportSrc != 0 {
			t.Errorf("%s 不该算导出读音", k)
		}
	}
	data, err := os.ReadFile(filepath.Join(fx.dir, sanitizeLyricsFilename(differs)+".roma.lrc"))
	if err != nil || !strings.Contains(string(data), "[00:01.00]kimi no na wa") || strings.Contains(string(data), "v1：") {
		t.Errorf(".roma.lrc 应写导出读音,实得 %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(fx.dir, sanitizeLyricsFilename(cleared)+".roma.lrc")); !os.IsNotExist(err) {
		t.Errorf("整首不显示读音的不该有 .roma.lrc: %v", err)
	}
}

// 输入和规则版本都没变就不再问;规则版本变了全部重问;正文变了只问那一条。
func TestRefreshExportRomaRecomputesOnlyWhenStale(t *testing.T) {
	const a, b = "A|a|X", "A|b|X"
	fx := withExportRomaFixture(t, map[string]enrichEntry{
		a: {Lyrics: bt("[00:01.00]사랑해"), LyricsRoma: bt("[00:01.00]saranghae")},
		b: {Lyrics: bt("[00:01.00]君の名は"), LyricsRoma: bt("[00:01.00]kimi")},
	})
	refreshExportRoma(context.Background(), nil)
	fx.asked = nil
	refreshExportRoma(context.Background(), nil)
	if len(fx.asked) != 0 {
		t.Errorf("都没变不该再问,实问 %v", fx.asked)
	}
	// 新进程(记忆清空)读到存着的指纹也对得上:不问。
	exportRomaSeenMu.Lock()
	exportRomaSeenBy = map[string]exportRomaSeen{}
	exportRomaSeenMu.Unlock()
	refreshExportRoma(context.Background(), nil)
	if len(fx.asked) != 0 {
		t.Errorf("存着的指纹和规则版本对得上不该再问,实问 %v", fx.asked)
	}
	enrichMu.Lock()
	e := enrichCache[b]
	e.Lyrics = bt("[00:01.00]君の名は。")
	enrichCache[b] = e
	enrichMu.Unlock()
	refreshExportRoma(context.Background(), nil)
	if !slices.Equal(fx.asked, []string{"[00:01.00]君の名は。"}) {
		t.Errorf("只该重问正文变了的那条,实问 %v", fx.asked)
	}
	fx.asked = nil
	fx.rules = 2
	refreshExportRoma(context.Background(), nil)
	if len(fx.asked) != 2 {
		t.Errorf("规则版本变了应全部重问,实问 %v", fx.asked)
	}
	fx.asked = nil
	refreshExportRoma(context.Background(), map[string]bool{a: true})
	if len(fx.asked) != 0 {
		t.Errorf("只看一条且没变不该问,实问 %v", fx.asked)
	}
}

// helper 算的时候条目被改了(重评分换了读音):不写回。helper 出错:什么都不写。
func TestRefreshExportRomaSkipsChangedAndFailures(t *testing.T) {
	const key = "A|k|X"
	fx := withExportRomaFixture(t, map[string]enrichEntry{key: {Lyrics: bt("[00:01.00]君の名は"), LyricsRoma: bt("[00:01.00]kimi")}})
	fx.reply = func(exportRomaItem) exportRomaResult {
		enrichMu.Lock()
		e := enrichCache[key]
		e.LyricsRoma = bt("[00:01.00]kimi no na wa")
		enrichCache[key] = e
		enrichMu.Unlock()
		return exportRomaResult{Roma: romaPtr("[00:01.00]x")}
	}
	refreshExportRoma(context.Background(), nil)
	if e := exportRomaEntry(key); e.LyricsRomaExport != nil || e.LyricsRomaExportSrc != 0 {
		t.Errorf("算的时候读音被换了不该写回,实得 %v %d", e.LyricsRomaExport, e.LyricsRomaExportSrc)
	}
	fx.reply = func(exportRomaItem) exportRomaResult { return exportRomaResult{Roma: romaPtr("[00:01.00]x")} }
	fx.fail = errExportRomaUnsupported
	refreshExportRoma(context.Background(), nil)
	if e := exportRomaEntry(key); e.LyricsRomaExportSrc != 0 {
		t.Error("helper 出错不该写回")
	}
	fx.fail = errRomanizeHelperMissing
	if n := refreshExportRoma(context.Background(), nil); n != 0 {
		t.Errorf("找不到 helper 应什么都不做,实得 %d", n)
	}
}

// 导入:文件是原样或存着的导出读音(不管指纹还对不对得上)都算没改;别的是手改,读进来、钉住照原样导出,补算不再动它。
func TestImportRomaFileRecognisesExportedFormsAndPinsEdits(t *testing.T) {
	const key = "A|k|X"
	fx := withExportRomaFixture(t, map[string]enrichEntry{key: {Lyrics: bt("[00:01.00]君の名は"), LyricsRoma: bt("[00:01.00]v1：kimi")}})
	fx.reply = func(exportRomaItem) exportRomaResult {
		return exportRomaResult{Roma: romaPtr("[00:01.00]kimi no na wa")}
	}
	refreshExportRoma(context.Background(), nil)
	path := filepath.Join(fx.dir, sanitizeLyricsFilename(key)+".roma.lrc")
	// 原样重写一遍、修改时间往后挪:文件状态记录认成动过,导入真的读它、拿内容比。
	touch := func() {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		later := time.Now().Add(time.Duration(len(fx.asked)+2) * time.Minute)
		if err := os.Chtimes(path, later, later); err != nil {
			t.Fatal(err)
		}
	}

	touch()
	importLyricsFromFiles()
	if e := exportRomaEntry(key); !e.LyricsRoma.EqualString("[00:01.00]v1：kimi") || e.LyricsRomaExport == nil {
		t.Fatalf("文件是写出去的导出读音,导入不该改缓存,实得 %q %v", e.LyricsRoma, e.LyricsRomaExport)
	}
	// 规则版本刚变、还没重算:文件里仍是上次的导出读音,照样认。
	enrichMu.Lock()
	e := enrichCache[key]
	e.LyricsRomaExportSrc++
	enrichCache[key] = e
	enrichMu.Unlock()
	touch()
	importLyricsFromFiles()
	if e := exportRomaEntry(key); !e.LyricsRoma.EqualString("[00:01.00]v1：kimi") {
		t.Fatalf("指纹过期时文件里的导出读音也不算手改,实得 %q", e.LyricsRoma)
	}

	header := lyricsFileHeader("A", "k", "X", "", false)
	if err := os.WriteFile(path, []byte(header+"[00:01.00]kimi-no-na-wa\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	importLyricsFromFiles()
	e = exportRomaEntry(key)
	if !e.LyricsRoma.EqualString("[00:01.00]kimi-no-na-wa\n") {
		t.Fatalf("手改的读音应读进来,实得 %q", e.LyricsRoma)
	}
	if e.LyricsRomaExport != nil || e.LyricsRomaExportRules != exportRomaPinned || exportedRoma(key, e) != "[00:01.00]kimi-no-na-wa\n" {
		t.Errorf("手改的读音应钉住照原样导出,实得 %v rules=%d", e.LyricsRomaExport, e.LyricsRomaExportRules)
	}
	fx.asked = nil
	refreshExportRoma(context.Background(), nil)
	if len(fx.asked) != 0 {
		t.Errorf("钉住的不该再按屏上规则重算,实问 %v", fx.asked)
	}
	if e := exportRomaEntry(key); e.LyricsRomaExport != nil {
		t.Errorf("钉住的不该再存导出读音,实得 %v", e.LyricsRomaExport)
	}
	// 正文变了钉子作废,照常重算。
	enrichMu.Lock()
	e = enrichCache[key]
	e.Lyrics = bt("[00:01.00]君の名は。")
	enrichCache[key] = e
	enrichMu.Unlock()
	refreshExportRoma(context.Background(), nil)
	if len(fx.asked) != 1 {
		t.Errorf("正文变了应重算,实问 %v", fx.asked)
	}
}

// TTML 里也带读音:文件是按原样或存着的导出读音写的,都算没改。
func TestImportedTTMLAcceptsExportedRomaForms(t *testing.T) {
	const key = "A|k|X"
	lyrics, raw, final := "[00:01.00]君の名は", "[00:01.00]v1：kimi", "[00:01.00]kimi no na wa"
	yrc := "[1000,1000](1000,1000,0)君の名は"
	e := enrichEntry{Lyrics: bt(lyrics), LyricsRoma: bt(raw), LyricsYRC: bt(yrc), LyricsRomaExport: romaPtr(final),
		LyricsRomaExportSrc: exportRomaSource(key, lyrics, raw, yrc, "", "", "")}
	in := lyricsWordInputOf(key, e, yrc)
	if in.roma != final {
		t.Fatalf("逐字文件应写导出读音,实得 %q", in.roma)
	}
	for _, roma := range []string{final, raw} {
		in.roma = roma
		w := importedWordFile{plain: encodeLyricsWordText(lyricsWordTTML, in)}
		if !importedWordFileUnchanged(w, lyricsWordTTML, key, e, yrc, false) {
			t.Errorf("按 %q 写的 TTML 应算没改", roma)
		}
	}
	in.roma = "[00:01.00]edited"
	if importedWordFileUnchanged(importedWordFile{plain: encodeLyricsWordText(lyricsWordTTML, in)}, lyricsWordTTML, key, e, yrc, false) {
		t.Error("读音被改过的 TTML 不该算没改")
	}
}

// 同曲其它写法的歌名歌手:去掉自己、按写法键排序、去重、最多 12 组,缺歌手或歌名的不要 —— 跟 Swift 的 headerAliases 同一个取法。
func TestExportRomaAliasesMatchHeaderAliases(t *testing.T) {
	keys := []string{"B|t|x", "A|t|x", "A|t|y", "|t|x", "C||x", "own|t|x"}
	got := exportRomaAliases("own|t|x", keys)
	want := [][]string{{"t", "A"}, {"t", "B"}}
	if len(got) != len(want) || got[0][1] != "A" || got[1][1] != "B" {
		t.Errorf("实得 %v,应为 %v", got, want)
	}
	var many []string
	for i := range 20 {
		many = append(many, string(rune('a'+i))+"|t|x")
	}
	if n := len(exportRomaAliases("own|t|x", many)); n != exportRomaMaxAliases {
		t.Errorf("最多 %d 组,实得 %d", exportRomaMaxAliases, n)
	}
}

// 接线:常驻进程起补算;单首导出后排一次只看这几条的;导出任务、逐字文件、导入走同一份导出读音;Swift 那边的抬头上限同一个数。
func TestExportRomaWiring(t *testing.T) {
	checks := map[string][]string{
		"main.go":             {"startExportRomaRefresh(ctx)"},
		"lyricsexport.go":     {"requestExportRoma(keys...)", "exportedRomaOf(j.key"},
		"lyricswordformat.go": {"roma: exportedRoma(key, e)", "exportRomaFileForms(e)"},
		"lyricsimport.go":     {"exportRomaFileForms(e)", "pinExportRoma(key, &e)"},
		"../lyrimuse/Sources/LyrimuseCore/Local/EnrichCacheReader.swift": {"static let maxHeaderAliases = 12"},
		"../lyrimuse/Sources/lyrics-romanize/main.swift":                 {"export_items", "LyricsExportRomanization.compute"},
	}
	for file, needles := range checks {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range needles {
			if !containsCode(string(src), n) {
				t.Errorf("%s 里应有 %q", file, n)
			}
		}
	}
	if exportRomaLive.Load() {
		t.Error("单测里不该起补算")
	}
	requestExportRoma("A|b|c")
	exportRomaPendingMu.Lock()
	n := len(exportRomaPending)
	exportRomaPendingMu.Unlock()
	if n != 0 {
		t.Error("补算没起时单首导出不该排补算")
	}
}
