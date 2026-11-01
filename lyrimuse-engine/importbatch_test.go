package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func lrcBody(lines int, step float64) string {
	var b strings.Builder
	for i := 0; i < lines; i++ {
		b.WriteString(lrcTimestamp(int(float64(i)*step*1000)) + "第" + string(rune('一'+i%5)) + "句\n")
	}
	return strings.TrimSpace(b.String())
}

// lrcBodyEn:英文的 LRC,选词测试用(歌手是英文名时中文正文会被语言闸拒掉)。
func lrcBodyEn(lines int, step float64) string {
	words := []string{"walking down the road", "under city lights", "holding on tonight", "never letting go", "singing out loud"}
	var b strings.Builder
	for i := 0; i < lines; i++ {
		b.WriteString(lrcTimestamp(int(float64(i)*step*1000)) + words[i%len(words)] + "\n")
	}
	return strings.TrimSpace(b.String())
}

func pathFile(rel, data string) importLyricsFile {
	return importLyricsFile{Name: filepath.Base(rel), Path: rel, Data: []byte(data)}
}

// 同名的正文 / 译文 / 读音一组;不同子目录分开;组号按目录、文件名排,两次分组一样。
func TestGroupImportFiles(t *testing.T) {
	files := []importLyricsFile{
		pathFile("b/周杰伦 - 晴天.lrc", "x"), pathFile("a/周杰伦 - 晴天.tr.lrc", "x"),
		pathFile("a/周杰伦 - 晴天.LRC", "x"), pathFile("a/周杰伦 - 晴天.roma.lrc", "x"), pathFile("a/其他.ttml", "x"),
	}
	g := groupImportFiles(files)
	if len(g) != 3 || g[0].Stem != "其他" || len(g[1].Files) != 3 || g[2].Dir != "b" {
		t.Fatalf("分组不对: %+v", g)
	}
	again := groupImportFiles([]importLyricsFile{files[4], files[2], files[0], files[3], files[1]})
	for i := range g {
		if g[i].ID != again[i].ID || g[i].Stem != again[i].Stem || len(g[i].Files) != len(again[i].Files) {
			t.Fatalf("文件顺序变了组号也变: %+v vs %+v", g, again)
		}
	}
}

// 文件名认身份:「歌手 - 歌名」两种顺序都猜、音轨号去掉、Lyrimuse 导出的「歌手 - 歌名 - 专辑~哈希」、文件里写的排最前。
func TestImportIdentityGuesses(t *testing.T) {
	g := importIdentityGuesses("01 - 周杰伦 - 晴天", nil)
	if len(g) != 2 || g[0] != (importIdentity{artist: "周杰伦", title: "晴天"}) || g[1] != (importIdentity{artist: "晴天", title: "周杰伦"}) {
		t.Errorf("音轨号 + 两段: %+v", g)
	}
	g = importIdentityGuesses("03. 晴天", nil)
	if len(g) != 1 || g[0].title != "晴天" {
		t.Errorf("只有歌名: %+v", g)
	}
	g = importIdentityGuesses("周杰伦 - 晴天 - 叶惠美~1a2b3c", nil)
	if g[0] != (importIdentity{artist: "周杰伦", title: "晴天", album: "叶惠美"}) {
		t.Errorf("自家导出的命名: %+v", g)
	}
	c := &scoredLyricCandidateResult{Title: "T", Artist: "A"}
	if g = importIdentityGuesses("whatever", c); g[0] != (importIdentity{artist: "A", title: "T"}) {
		t.Errorf("文件里写的排最前: %+v", g)
	}
	if g = importIdentityGuesses("1979", nil); len(g) != 1 || g[0].title != "1979" {
		t.Errorf("数字歌名不当音轨号: %+v", g)
	}
}

func importTestCache() map[string]enrichEntry {
	return map[string]enrichEntry{
		enrichKey("周杰伦", "晴天", "叶惠美"):           {DurationSecs: 269, LyricsSource: "qq", Lyrics: bt("[00:01.00]x")},
		enrichKey("A", "Song", "Album One"):     {DurationSecs: 60},
		enrichKey("A", "Song", "Album Two"):     {DurationSecs: 300},
		enrichKey("Other", "Song", "Elsewhere"): {DurationSecs: 60},
		enrichKey("B", "Twin", "X"):             {DurationSecs: 200},
		enrichKey("B", "Twin", "Y"):             {DurationSecs: 205},
		enrichKey("C", "Locked", "Z"):           {DurationSecs: 100, ManualLyrics: true, Lyrics: bt("[00:01.00]mine")},
	}
}

// 预览清单:认出、时长分开同名的、分不开的等用户挑、繁简写法对得上、库里没有、认不出、同一首两组留好的那组。
func TestPlanLyricsImport(t *testing.T) {
	files := []importLyricsFile{
		pathFile("周杰倫 - 晴天.lrc", lrcBody(4, 60)),                                             // 繁体歌手名,末句 180 秒
		pathFile("dup/周杰伦 - 晴天.yrc", "[1000,1000](1000,1000,0)一\n[3000,1000](3000,1000,0)二"), // 同一首,带逐字
		pathFile("A - Song.lrc", lrcBody(5, 14)),                                             // 末句 56 秒 → 60 秒那版
		pathFile("B - Twin.lrc", lrcBody(5, 50)),                                             // 200 / 205 都吻合
		pathFile("Nobody - Nothing.lrc", lrcBody(3, 10)),
		pathFile("broken.lrc", "   "),
	}
	plan := planLyricsImport(files, "zh", importTestCache())
	by := map[string]importPlanItem{}
	for _, it := range plan.Items {
		by[it.Files[0]] = it
	}
	if it := by["dup/周杰伦 - 晴天.yrc"]; it.Status != importStatusMatched || it.Key != enrichKey("周杰伦", "晴天", "叶惠美") || !it.HasWordTiming {
		t.Errorf("带逐字那组应认出: %+v", it)
	}
	if it := by["周杰倫 - 晴天.lrc"]; it.Status != importStatusDuplicate || it.DuplicateOf != by["dup/周杰伦 - 晴天.yrc"].ID {
		t.Errorf("繁体写法应对上同一首、让给带逐字那组: %+v", it)
	}
	if it := by["A - Song.lrc"]; it.Status != importStatusMatched || it.Key != enrichKey("A", "Song", "Album One") || len(it.Options) != 2 {
		t.Errorf("时长应分开同名的两版: %+v", it)
	}
	if it := by["B - Twin.lrc"]; it.Status != importStatusAmbiguous || it.Key != "" || len(it.Options) != 2 || it.Options[0].Key != enrichKey("B", "Twin", "X") {
		t.Errorf("时长分不开应等用户挑、近的排前: %+v", it)
	}
	if it := by["Nobody - Nothing.lrc"]; it.Status != importStatusUnmatched || it.Lines != 3 {
		t.Errorf("库里没有: %+v", it)
	}
	if it := by["broken.lrc"]; it.Status != importStatusUnrecognized {
		t.Errorf("认不出: %+v", it)
	}
}

func setUpImportBatch(t *testing.T, files []importLyricsFile) string {
	t.Helper()
	t.Setenv("LYRIMUSE_CONFIG_DIR", t.TempDir())
	if err := os.MkdirAll(importStagingDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	staging := filepath.Join(importStagingDir(), "s.json")
	data, _ := json.Marshal(importStagingRequest{Files: files})
	if err := os.WriteFile(staging, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return staging
}

// 三种导入方式:锁定的写成手改、不进导入库;优先的不锁、进导入库;只补的跳过已有歌词的。手改过的不 Force 不动。
func TestImportLyricsBatchModes(t *testing.T) {
	files := []importLyricsFile{
		pathFile("周杰伦 - 晴天.lrc", lrcBody(4, 60)),
		pathFile("A - Song.lrc", lrcBody(5, 14)),
		pathFile("C - Locked.lrc", lrcBody(3, 30)),
	}
	keyQ, keyA, keyC := enrichKey("周杰伦", "晴天", "叶惠美"), enrichKey("A", "Song", "Album One"), enrichKey("C", "Locked", "Z")
	ids := map[string]int{}
	for _, g := range groupImportFiles(files) {
		ids[g.Stem] = g.ID
	}
	items := []importBatchItem{{ID: ids["周杰伦 - 晴天"], Key: keyQ}, {ID: ids["A - Song"], Key: keyA}, {ID: ids["C - Locked"], Key: keyC}}

	t.Run("manual", func(t *testing.T) {
		staging := setUpImportBatch(t, files)
		setUpEnrichEditTest(t, importTestCache())
		res := applyEnrichEdit(enrichEditRequest{Op: "import_lyrics_batch", Staging: staging, Mode: importModeManual, ImportItems: items})
		if !res.OK || res.Changed != 2 {
			t.Fatalf("应写两首(手改那首跳过): %+v", res)
		}
		e, _ := cacheEntry(t, keyQ)
		if e.LyricsSource != importLyricsSource || !e.ManualLyrics || !strings.Contains(e.Lyrics.String(), "[03:00.00]") {
			t.Errorf("锁定方式应写成手改: %+v", e)
		}
		if _, ok := importedLyricsRecordFor(keyQ); ok {
			t.Error("锁定方式不进导入库")
		}
		if c, _ := cacheEntry(t, keyC); c.Lyrics.String() != "[00:01.00]mine" {
			t.Errorf("手改过的不该动: %q", c.Lyrics.String())
		}
	})
	t.Run("prefer", func(t *testing.T) {
		staging := setUpImportBatch(t, files)
		setUpEnrichEditTest(t, importTestCache())
		forced := append(items[:2:2], importBatchItem{ID: items[2].ID, Key: keyC, Force: true})
		res := applyEnrichEdit(enrichEditRequest{Op: "import_lyrics_batch", Staging: staging, Mode: importModePrefer, ImportItems: forced})
		if !res.OK || res.Changed != 3 {
			t.Fatalf("Force 的手改那首也写: %+v", res)
		}
		e, _ := cacheEntry(t, keyQ)
		if e.LyricsSource != importLyricsSource || e.ManualLyrics {
			t.Errorf("优先方式不锁: %+v", e)
		}
		rec, ok := importedLyricsRecordFor(keyQ)
		if !ok || rec.Lyrics != e.Lyrics.String() {
			t.Errorf("优先方式应进导入库: %+v", rec)
		}
		// 在这首上采用了别的歌词:导入库里那份作废。
		applyEnrichEdit(enrichEditRequest{Op: "save_edit", Key: keyQ, Lyrics: "[00:01.00]别的", Source: "qq"})
		if _, ok := importedLyricsRecordFor(keyQ); ok {
			t.Error("采用别的歌词后导入库里那份应作废")
		}
	})
	t.Run("fill", func(t *testing.T) {
		staging := setUpImportBatch(t, files)
		setUpEnrichEditTest(t, importTestCache())
		res := applyEnrichEdit(enrichEditRequest{Op: "import_lyrics_batch", Staging: staging, Mode: importModeFill, ImportItems: items})
		if !res.OK || res.Changed != 1 {
			t.Fatalf("只补没有歌词的那首: %+v", res)
		}
		if e, _ := cacheEntry(t, keyQ); e.LyricsSource != "qq" {
			t.Errorf("已有歌词的不该动: %+v", e)
		}
		if e, _ := cacheEntry(t, keyA); e.LyricsSource != importLyricsSource {
			t.Errorf("没歌词的应补上: %+v", e)
		}
	})
	t.Run("staging outside", func(t *testing.T) {
		setUpImportBatch(t, files)
		setUpEnrichEditTest(t, importTestCache())
		outside := filepath.Join(t.TempDir(), "s.json")
		os.WriteFile(outside, []byte(`{"files":[]}`), 0o644)
		if res := applyEnrichEdit(enrichEditRequest{Op: "import_lyrics_batch", Staging: outside, Mode: importModeFill}); res.OK {
			t.Error("暂存目录外的文件不该读")
		}
	})
}

// 选词:导入库里的那份进候选池,带导入加分;跟别家普通 LRC 一样好时赢,别家带逐字时输。
func TestImportedLyricsCandidateInRanking(t *testing.T) {
	t.Setenv("LYRIMUSE_CONFIG_DIR", t.TempDir())
	lyr := lrcBodyEn(20, 10)
	if err := saveImportedLyrics(importedLyricsRecord{Key: enrichKey("A", "Song", "Album"), Lyrics: lyr}); err != nil {
		t.Fatal(err)
	}
	imp, ok := importedLyricsFor("A", "Song", "Album", 0)
	if !ok {
		t.Fatal("应找到导入库里那份")
	}
	if _, ok := importedLyricsFor("a", "song", "ALBUM", 0); !ok {
		t.Error("宽松 key 应对得上")
	}
	plain := lyricSourceResult{source: "lrclib", lyr: lyr, matchTitle: "Song", matchArtist: "A", matchAlbum: "Album"}
	scored := rankLyricSourceResults("A", "Song", "Album", 200, map[string]lyricSourceResult{importLyricsSource: imp, "lrclib": plain})
	if len(scored) < 2 || scored[0].Source != importLyricsSource || scoreTermPoints(scored[0].ScoreTerms, scoreTermImported) != importedLyricsPoints {
		t.Fatalf("同等质量时导入的应排第一、带导入加分: %+v", scored)
	}
	var yrc strings.Builder
	for i := 0; i < 20; i++ {
		start := i * 10000
		yrc.WriteString("[" + itoa(start) + ",2000](" + itoa(start) + ",2000,0)" + strings.TrimSpace(strings.SplitN(strings.Split(lyr, "\n")[i], "]", 2)[1]) + "\n")
	}
	timed := lyricSourceResult{source: "qq", lyr: lyr, yrc: yrc.String(), matchTitle: "Song", matchArtist: "A", matchAlbum: "Album"}
	scored = rankLyricSourceResults("A", "Song", "Album", 200, map[string]lyricSourceResult{importLyricsSource: imp, "qq": timed})
	if len(scored) < 2 || scored[0].Source != "qq" {
		t.Errorf("别家带逐字时应换成那一份: %+v", scored)
	}
	// 顺序优先:导入的排在各源前面。
	netease := scoredLyricCandidateResult{Source: "netease", Score: 900}
	imported := scoredLyricCandidateResult{Source: importLyricsSource, Score: 100}
	if p := priorityPick([]scoredLyricCandidateResult{netease, imported}, func(scoredLyricCandidateResult) bool { return true }); p == nil || p.Source != importLyricsSource {
		t.Errorf("顺序优先里导入的应排在各源前面: %+v", p)
	}
	dropImportedLyrics(enrichKey("A", "Song", "Album"))
	if _, ok := importedLyricsFor("A", "Song", "Album", 0); ok {
		t.Error("作废后不该再找到")
	}
}

// 接线:预览子命令登记了、只读(不存盘、不拿单实例锁)。
func TestPlanLyricsImportCLIWiring(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !containsCode(string(src), `os.Args[1] == "plan-lyrics-import"`) || !containsCode(string(src), "runPlanLyricsImportCLI(os.Args[2:])") {
		t.Error("main.go 应登记 plan-lyrics-import")
	}
	all, err := os.ReadFile("importbatch.go")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(all), "\nfunc runPlanLyricsImportCLI(")
	if start < 0 {
		t.Fatal("找不到 runPlanLyricsImportCLI")
	}
	body := string(all)[start+1:]
	if end := strings.Index(body, "\nfunc "); end >= 0 {
		body = body[:end]
	}
	for _, banned := range []string{"saveEnrichCache", "acquireSingleInstance", "enrichMu.Lock", "loadEnrichCache("} {
		if strings.Contains(body, banned) {
			t.Errorf("预览子命令应只读,不该出现 %s", banned)
		}
	}
}

// 库里没有对应歌曲的文件按身份存进导入库:认得出歌手歌名才能存;放到这首时按歌名 + 歌手认(文件名两种顺序都认),
// 歌手不对、时长对不上不认;用户在这首上采用了别的歌词后作废。
func TestImportStoreUnmatched(t *testing.T) {
	files := []importLyricsFile{
		pathFile("Nobody Here - No Such Song.lrc", lrcBodyEn(4, 6)),
		pathFile("OnlyTitle.lrc", lrcBodyEn(4, 6)),
	}
	staging := setUpImportBatch(t, files)
	setUpEnrichEditTest(t, importTestCache())
	plan := planLyricsImport(files, "en", importTestCache())
	ids := map[string]importPlanItem{}
	for _, it := range plan.Items {
		ids[it.Files[0]] = it
	}
	if !ids["Nobody Here - No Such Song.lrc"].CanStore || ids["OnlyTitle.lrc"].CanStore {
		t.Fatalf("有歌手歌名的才能存: %+v", plan.Items)
	}
	items := []importBatchItem{{ID: ids["Nobody Here - No Such Song.lrc"].ID, Store: true}, {ID: ids["OnlyTitle.lrc"].ID, Store: true}}
	res := applyEnrichEdit(enrichEditRequest{Op: "import_lyrics_batch", Staging: staging, Mode: importModePrefer, ImportItems: items})
	if !res.OK || res.Changed != 1 {
		t.Fatalf("应存一份(只有歌名的不存): %+v", res)
	}
	if _, ok := importedLyricsFor("Nobody Here", "No Such Song", "Some Album", 0); !ok {
		t.Error("按歌名 + 歌手应认出")
	}
	if _, ok := importedLyricsFor("No Such Song", "Nobody Here", "", 20); !ok {
		t.Error("文件名两种顺序都应认,时长吻合也认")
	}
	if _, ok := importedLyricsFor("Somebody Else", "No Such Song", "", 0); ok {
		t.Error("歌手不对不该认")
	}
	if _, ok := importedLyricsFor("Nobody Here", "No Such Song", "", 300); ok {
		t.Error("时长对不上(同名的另一个版本)不该认")
	}
	applyEnrichEdit(enrichEditRequest{Op: "save_edit", Key: enrichKey("Nobody Here", "No Such Song", "Some Album"), Lyrics: "[00:01.00]other", Source: "qq"})
	if _, ok := importedLyricsFor("Nobody Here", "No Such Song", "", 0); ok {
		t.Error("采用别的歌词后按身份存的那份应作废")
	}
}

// 监视文件夹那种:对得上、没歌词的写进条目;已有歌词的只进导入库,条目不动;库里没有的按身份存。
func TestImportLyricsBatchLibraryMode(t *testing.T) {
	files := []importLyricsFile{
		pathFile("周杰伦 - 晴天.lrc", lrcBody(4, 60)),
		pathFile("A - Song.lrc", lrcBody(5, 14)),
		pathFile("Nobody Here - No Such Song.lrc", lrcBodyEn(4, 6)),
	}
	staging := setUpImportBatch(t, files)
	setUpEnrichEditTest(t, importTestCache())
	ids := map[string]int{}
	for _, g := range groupImportFiles(files) {
		ids[g.Stem] = g.ID
	}
	keyQ, keyA := enrichKey("周杰伦", "晴天", "叶惠美"), enrichKey("A", "Song", "Album One")
	items := []importBatchItem{{ID: ids["周杰伦 - 晴天"], Key: keyQ}, {ID: ids["A - Song"], Key: keyA},
		{ID: ids["Nobody Here - No Such Song"], Store: true}}
	res := applyEnrichEdit(enrichEditRequest{Op: "import_lyrics_batch", Staging: staging, Mode: importModeLibrary, ImportItems: items})
	if !res.OK || res.Changed != 3 {
		t.Fatalf("应写一首、存两份: %+v", res)
	}
	if e, _ := cacheEntry(t, keyQ); e.LyricsSource != "qq" {
		t.Errorf("已有歌词的条目不该动: %+v", e)
	}
	if _, ok := importedLyricsRecordFor(keyQ); !ok {
		t.Error("已有歌词的应进导入库")
	}
	if e, _ := cacheEntry(t, keyA); e.LyricsSource != importLyricsSource || e.ManualLyrics {
		t.Errorf("没歌词的应写进条目、不锁: %+v", e)
	}
	if _, ok := importedLyricsFor("Nobody Here", "No Such Song", "", 0); !ok {
		t.Error("库里没有的应按身份存")
	}
}

// 导入库管理:按文件名删、整个清空;不认导入库以外的名字。
func TestDropImportedLyricsFiles(t *testing.T) {
	t.Setenv("LYRIMUSE_CONFIG_DIR", t.TempDir())
	setUpEnrichEditTest(t, importTestCache())
	keyed := importedLyricsRecord{Key: enrichKey("A", "Song", "Album One"), Lyrics: lrcBodyEn(3, 5)}
	byID := importedLyricsRecord{Lyrics: lrcBodyEn(3, 5), Identities: []importedIdentity{{Artist: "X", Title: "Y"}}}
	if err := saveImportedLyrics(keyed, byID); err != nil {
		t.Fatal(err)
	}
	res := applyEnrichEdit(enrichEditRequest{Op: "drop_imported_lyrics", ImportNames: []string{importedLyricsFileName(keyed.Key), "../escape.json", "index.json"}})
	if !res.OK || res.Changed != 1 {
		t.Fatalf("应只删那一份: %+v", res)
	}
	if _, ok := importedLyricsRecordFor(keyed.Key); ok {
		t.Error("删掉的不该再找到")
	}
	if _, ok := importedLyricsFor("X", "Y", "", 0); !ok {
		t.Error("没删的还在")
	}
	if res := applyEnrichEdit(enrichEditRequest{Op: "drop_imported_lyrics", Value: true}); !res.OK || res.Changed != 1 {
		t.Fatalf("整个清空: %+v", res)
	}
	if _, ok := importedLyricsFor("X", "Y", "", 0); ok {
		t.Error("清空后不该再找到")
	}
}
