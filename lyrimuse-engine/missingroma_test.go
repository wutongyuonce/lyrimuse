package main

import (
	"context"
	"errors"
	"os"
	"testing"
)

func withMissingRomaFixture(t *testing.T, entries map[string]enrichEntry, gen func(lyrics, songLanguage, kana string) (string, error)) {
	t.Helper()
	withTempLeanCache(t)
	savedGen := missingRomaGenerator
	missingRomaBarrenMu.Lock()
	savedBarren := missingRomaBarren
	missingRomaBarren = map[string]barrenRoma{}
	missingRomaBarrenMu.Unlock()
	t.Cleanup(func() {
		missingRomaGenerator = savedGen
		missingRomaBarrenMu.Lock()
		missingRomaBarren = savedBarren
		missingRomaBarrenMu.Unlock()
	})
	enrichMu.Lock()
	for k, e := range entries {
		enrichCache[k] = e
	}
	enrichMu.Unlock()
	missingRomaGenerator = gen
}

// 该有读音的空条目补上(韩文、粤语);纯英文不问;已有读音、手改过、标了纯音乐的不动。
func TestBackfillMissingRomaFillsOnlyEligible(t *testing.T) {
	const korean, cantonese, english, hasRoma, manual, instrumental = "A|ko|X", "A|yue|X", "A|en|X", "A|roma|X", "A|manual|X", "A|inst|X"
	asked := map[string]string{}
	withMissingRomaFixture(t, map[string]enrichEntry{
		korean:       {Lyrics: bt("[00:01.00]사랑해요"), LyricsKana: "[kana:1あ]"},
		cantonese:    {Lyrics: bt("[00:01.00]我愛你"), SongLanguage: songLanguageCantonese},
		english:      {Lyrics: bt("[00:01.00]hello world")},
		hasRoma:      {Lyrics: bt("[00:01.00]君の名は"), LyricsRoma: bt("[00:01.00]kimi no na wa")},
		manual:       {Lyrics: bt("[00:01.00]我爱你"), ManualLyrics: true},
		instrumental: {Lyrics: bt("[00:01.00]我爱你"), Instrumental: true},
	}, func(lyrics, lang, kana string) (string, error) {
		asked[lyrics] = lang + "|" + kana
		return "[00:01.00]roma", nil
	})
	if n := backfillMissingRoma(context.Background()); n != 2 {
		t.Fatalf("应补 2 条(韩文、粤语),实补 %d", n)
	}
	if len(asked) != 2 || asked["[00:01.00]사랑해요"] != "|[kana:1あ]" || asked["[00:01.00]我愛你"] != songLanguageCantonese+"|" {
		t.Errorf("只该问韩文、粤语两条,带上语种和借来的假名,实问 %v", asked)
	}
	for _, k := range []string{korean, cantonese} {
		if got := enrichCache[k].LyricsRoma; !got.EqualString("[00:01.00]roma") {
			t.Errorf("%s 应补上读音,实得 %q", k, got)
		}
	}
	if got := enrichCache[hasRoma].LyricsRoma; !got.EqualString("[00:01.00]kimi no na wa") {
		t.Errorf("已有读音不动,实得 %q", got)
	}
	for _, k := range []string{english, manual, instrumental} {
		if !enrichCache[k].LyricsRoma.IsZero() {
			t.Errorf("%s 不该补读音", k)
		}
	}
}

// 判过没出读音的,正文没变就不再问;正文变了再问。helper 出错的不记,下一遍照问。
func TestBackfillMissingRomaRemembersBarrenNotErrors(t *testing.T) {
	const barren, failing = "A|barren|X", "A|failing|X"
	asked := map[string]int{}
	withMissingRomaFixture(t, map[string]enrichEntry{
		barren:  {Lyrics: bt("[00:01.00]我爱你")},
		failing: {Lyrics: bt("[00:01.00]君の名は")},
	}, func(lyrics, _, _ string) (string, error) {
		asked[lyrics]++
		if lyrics == "[00:01.00]君の名は" {
			return "", errors.New("helper timed out")
		}
		return "", nil
	})
	backfillMissingRoma(context.Background())
	backfillMissingRoma(context.Background())
	if asked["[00:01.00]我爱你"] != 1 {
		t.Errorf("没出读音的第二遍不该再问,实问 %d 次", asked["[00:01.00]我爱你"])
	}
	if asked["[00:01.00]君の名は"] != 2 {
		t.Errorf("helper 出错的下一遍要再问,实问 %d 次", asked["[00:01.00]君の名は"])
	}
	enrichMu.Lock()
	e := enrichCache[barren]
	e.Lyrics = bt("[00:01.00]我爱你们")
	enrichCache[barren] = e
	enrichMu.Unlock()
	backfillMissingRoma(context.Background())
	if asked["[00:01.00]我爱你们"] != 1 {
		t.Errorf("正文变了要再问,实问 %d 次", asked["[00:01.00]我爱你们"])
	}
}

// 起 helper 期间正文被换掉的,算出来的读音不写回。
func TestBackfillMissingRomaSkipsChangedLyrics(t *testing.T) {
	const key = "A|changed|X"
	withMissingRomaFixture(t, map[string]enrichEntry{key: {Lyrics: bt("[00:01.00]我爱你")}}, func(string, string, string) (string, error) {
		enrichMu.Lock()
		e := enrichCache[key]
		e.Lyrics = bt("[00:01.00]你好")
		enrichCache[key] = e
		enrichMu.Unlock()
		return "[00:01.00]wǒ ài nǐ", nil
	})
	if n := backfillMissingRoma(context.Background()); n != 0 || !enrichCache[key].LyricsRoma.IsZero() {
		t.Errorf("正文中途换了不该写回,实补 %d、读音 %q", n, enrichCache[key].LyricsRoma)
	}
}

func TestMissingRomaBackfillWiring(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !containsCode(string(src), "startMissingRomaBackfill(ctx)") {
		t.Error("main.go 没接 startMissingRomaBackfill")
	}
}
