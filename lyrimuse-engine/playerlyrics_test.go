package main

import (
	"fmt"
	"strings"
	"testing"
)

// testPlayerLRC 拼一份 12 行、每行文字不同的 LRC,第 i 行在 startMs + i*5000(+shiftMs)。
func testPlayerLRC(shiftMs int) string {
	var b strings.Builder
	for i := 0; i < 12; i++ {
		ms := 20000 + i*5000 + shiftMs
		fmt.Fprintf(&b, "[%02d:%02d.%02d]line number %d of the song\n", ms/60000, ms/1000%60, ms%1000/10, i)
	}
	return b.String()
}

func TestNeedsPlayerLyricsCheck(t *testing.T) {
	soda := playerBundleIDs[playerSoda]
	base := enrichEntry{Lyrics: bt(testPlayerLRC(0)), LyricsSource: "applemusic"}
	checked := base
	checked.PlayerLyrics = map[string]playerLyricsAlt{soda: {MainFP: mainLyricsFingerprint(base)}}
	stale := base
	stale.PlayerLyrics = map[string]playerLyricsAlt{soda: {MainFP: "000000000000"}}
	sameSource := base
	sameSource.LyricsSource = "soda"
	instrumental := base
	instrumental.Instrumental = true
	handEdited := base
	handEdited.ManualLyrics, handEdited.ManualPickSHA = true, "edited"
	picked := base
	picked.ManualLyrics, picked.ManualPickSHA = true, manualPickFingerprint(base.Lyrics.String())

	cases := []struct {
		name   string
		e      enrichEntry
		bundle string
		want   bool
	}{
		{"主歌词不是这个播放器自家源:要比", base, soda, true},
		{"比过、主歌词没变:不再比", checked, soda, false},
		{"比过、主歌词换了:重比", stale, soda, true},
		{"主歌词就是它自家源:不比", sameSource, soda, false},
		{"纯音乐不比", instrumental, soda, false},
		{"正文手改过不比", handEdited, soda, false},
		{"原样采纳的候选照比", picked, soda, true},
		{"认不出的播放器不比", base, "com.example.unknown", false},
		{"没有歌词不比", enrichEntry{LyricsSource: "applemusic"}, soda, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := needsPlayerLyricsCheck(c.e, c.bundle); got != c.want {
				t.Fatalf("needsPlayerLyricsCheck = %v, 期望 %v", got, c.want)
			}
		})
	}

	withFeaturesFile(t, `{"lyrics_sources":["applemusic","qq"]}`)
	if needsPlayerLyricsCheck(base, soda) {
		t.Fatal("这个播放器的自家源在设置里关着:不比")
	}
}

func TestPlayerLyricsAltFrom(t *testing.T) {
	main := testPlayerLRC(0)
	cand := func(shiftMs int, local bool, score int) scoredLyricCandidateResult {
		return scoredLyricCandidateResult{Source: "soda", Lyrics: testPlayerLRC(shiftMs), LyricsTr: "[00:20.00]译文", LyricsTrLang: "zh",
			Score: score, IdentityFromLocalClient: local}
	}
	fp := lyricSpeakersFingerprint(main, "")

	alt, ok := playerLyricsAltFrom(main, "", cand(2200, true, 1300), 42)
	if !ok || alt.Lyrics == "" || alt.Source != "soda" || alt.ShiftMs != 2200 || alt.MainFP != fp || alt.Tr == "" || alt.TrLang != "zh" || alt.At != 42 {
		t.Fatalf("整体晚 2.2 秒:该记下这一份和偏移,得到 ok=%v %+v", ok, alt)
	}
	if alt, ok = playerLyricsAltFrom(main, "", cand(-1500, true, 1300), 42); !ok || alt.Lyrics == "" || alt.ShiftMs != -1500 {
		t.Fatalf("整体早 1.5 秒:同样记下,得到 ok=%v %+v", ok, alt)
	}
	if alt, ok = playerLyricsAltFrom(main, "", cand(300, true, 1300), 42); !ok || alt.Lyrics != "" || alt.MainFP != fp || alt.ShiftMs != 0 {
		t.Fatalf("差 0.3 秒算对齐:只记比对过,得到 ok=%v %+v", ok, alt)
	}
	if _, ok = playerLyricsAltFrom(main, "", cand(2200, false, 1300), 42); ok {
		t.Fatal("不是按本机认出的这一首取的(搜出来的):不算数")
	}
	if _, ok = playerLyricsAltFrom(main, "", cand(2200, true, -1), 42); ok {
		t.Fatal("被判了负分:不算数")
	}
}
