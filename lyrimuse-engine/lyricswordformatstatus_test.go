package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// 换格式一趟:状态文件最后写「做完了」,总数和完成数都是有逐字的歌数(只有逐行的不算),格式名同 features.json 的写法。
func TestSwitchLyricsWordFormatsWritesStatus(t *testing.T) {
	lrcOnly := enrichEntry{LyricsSource: "qq", Lyrics: bt("[00:01.00]只有逐行\n[00:04.00]第二句"), DurationSecs: 180}
	withExportFixture(t, map[string]enrichEntry{"A|Song|": lyricsWordFilesSample(), "B|Plain|": lrcOnly})
	resetFeaturesForTest(t)
	savedReady := lyricsDirSwitchReady.Load()
	t.Cleanup(func() { lyricsDirSwitchReady.Store(savedReady) })
	lyricsDirSwitchReady.Store(true)
	savedPath := lyricsWordFormatStatusPath
	t.Cleanup(func() { lyricsWordFormatStatusPath = savedPath })
	lyricsWordFormatStatusPath = filepath.Join(t.TempDir(), "status.json")
	withLyricsWordFormats(t, lyricsWordYRC)
	exportLyricsFiles()

	next := []lyricsWordFormat{lyricsWordQRC, lyricsWordTTML}
	setFeatures(featureFlags{LyricsWordFormats: next})
	switchLyricsWordFormats(next)
	data, err := os.ReadFile(lyricsWordFormatStatusPath)
	if err != nil {
		t.Fatalf("换格式要写状态文件: %v", err)
	}
	var s lyricsWordFormatStatus
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	if s.Running || s.FinishedAt == 0 || s.StartedAt == 0 {
		t.Errorf("最后一份是做完了: %+v", s)
	}
	if s.Total != 1 || s.Done != 1 {
		t.Errorf("只数有逐字的歌: %+v", s)
	}
	if !slices.Equal(s.Formats, []string{"qrc", "ttml"}) {
		t.Errorf("格式名同设置里的写法: %v", s.Formats)
	}
	if lyricsWordSwitchProgress.Load() != nil {
		t.Error("做完要撤掉计数")
	}
}

// 计数只在全量导出里记:编好、写出各记半首;只导出几首的那种(提交一首歌词)不记。
func TestLyricsWordProgressCountsFullExportOnly(t *testing.T) {
	key := "A|Song|"
	withExportFixture(t, map[string]enrichEntry{key: lyricsWordFilesSample()})
	withLyricsWordFormats(t, lyricsWordKRC)
	p := &lyricsWordProgress{}
	lyricsWordSwitchProgress.Store(p)
	t.Cleanup(func() { lyricsWordSwitchProgress.Store(nil) })
	exportLyricsFilesFor(key)
	if p.total.Load() != 0 || p.done() != 0 {
		t.Errorf("只导出几首的不记: total=%d done=%d", p.total.Load(), p.done())
	}
	exportLyricsFiles()
	if p.total.Load() != 1 || p.encoded.Load() != 1 || p.written.Load() != 1 || p.done() != 1 {
		t.Errorf("全量导出编好、写出各记一次: total=%d encoded=%d written=%d", p.total.Load(), p.encoded.Load(), p.written.Load())
	}
}
