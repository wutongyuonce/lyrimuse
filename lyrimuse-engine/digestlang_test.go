package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// fixedTitle:不管哪种语言都是这个标题(测试里直接给标题字面量时用)。
func fixedTitle(s string) func(digestWords) string { return func(digestWords) string { return s } }

// 用语表跟共用样例 shared/testdata/notify/words.json 一字不差(App 的 Core `ReportWords` 读同一份对照)。
func TestDigestWordsMatchShared(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "shared", "testdata", "notify", "words.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want map[digestLang]digestWords
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(digestWordTable, want) {
		t.Errorf("digestWordTable 跟 words.json 不一致")
	}
	// 每种语言都有简体那份的全部句子。
	for lang, w := range digestWordTable {
		for key := range digestWordTable[digestLangHans] {
			if _, ok := w[key]; !ok {
				t.Errorf("%s 缺 %s", lang, key)
			}
		}
	}
}

func TestParseDigestLang(t *testing.T) {
	for in, want := range map[string]digestLang{"": digestLangHans, "zh-hans": digestLangHans, " ZH-HANT ": digestLangHant, "en": digestLangEn, "ja": digestLangHans} {
		if got := parseDigestLang(in); got != want {
			t.Errorf("parseDigestLang(%q) = %q, want %q", in, got, want)
		}
	}
	path := filepath.Join(t.TempDir(), "lyrimuse-features.json")
	if err := os.WriteFile(path, []byte(`{"report_language":"en"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadFeatureFlags(path).ReportLanguage; got != digestLangEn {
		t.Fatalf("features.json 里 App 写的 report_language 没用上: %q", got)
	}
}

// 英文、繁体的报告:标题、第一行、正文、排版标签都换成那种语言,歌名歌手名原样。
func TestDigestReportInOtherLanguages(t *testing.T) {
	d := richSample()
	useDigestFeatures(t, featureFlags{DigestTopN: 2, ReportLanguage: digestLangEn})
	now := time.Date(2026, 10, 20, 22, 5, 0, 0, time.Local)
	w := dw()
	d.PrevLabel = weeklyDigestPeriod(0, 7*24*3600).prevLabel // 上一期的叫法在取数时按当时的语言算好
	for got, want := range map[string]string{
		dailyDigestTitle(w, now, midnightOf(now)):                              "🎧 Today's Listening Report (Oct 20)",
		dailyDigestTitle(w, now, midnightOf(now).AddDate(0, 0, -1)):            "🎧 Yesterday's Listening Report (Oct 19)",
		calendarDigestTitle(w, calendarDigestMonthly, now):                     "📅 October 2026 Listening Digest",
		calendarDigestTitle(w, calendarDigestYearly, now):                      "🏆 2026 Listening Digest",
		calendarDigestPeriod(calendarDigestMonthly, now, now, false).prevLabel: "in September",
		digestSummaryLine(*d):                         "601 plays (vs. 883 the week before, ↓32%) · about 42 h 3 min",
		digestSummaryLine(digestStats{TotalPlays: 1}): "1 play",
		digestNewArtistsLine(*d):                      "44 new artists: Patti LaBelle, The Crane, 99 God and more",
	} {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
	body := digestBody(*d)
	for _, want := range []string{"\n\nTop artists:\n1. 方大同 (129)\n", "\n\nTop albums:\n1. Michael Jackson - Xscape (Deluxe) (30)\n"} {
		if !strings.Contains(body, want) {
			t.Errorf("正文缺 %q:\n%s", want, body)
		}
	}
	if brief := digestBodyStyled(*d, digestStyleBrief); !strings.Contains(brief, "\nTop artist: 方大同\n") || !strings.Contains(brief, "New artists (44): Patti LaBelle, The Crane, 99 God") {
		t.Errorf("精简按英文预设:\n%s", brief)
	}
	m := decodeJSON(t, buildFor(t, notifyChannel{Platform: platformTelegram, URL: "TOKEN", ChatID: "42"}, richMessage()).body)
	if text, _ := m["text"].(string); !strings.Contains(text, "<b>Top artists</b>") ||
		m["reply_markup"].(map[string]any)["inline_keyboard"].([]any)[0].([]any)[0].(map[string]any)["text"] != "See the full chart on Last.fm" {
		t.Errorf("Telegram 排版用英文标签: %v", m)
	}

	useDigestFeatures(t, featureFlags{DigestTopN: 2, ReportLanguage: digestLangHant})
	w = dw()
	if got := weeklyDigestTitle(w, digestSourceLastfm, time.Date(2026, 10, 11, 0, 0, 0, 0, time.Local).Unix(), time.Date(2026, 10, 18, 0, 0, 0, 0, time.Local).Unix()); got != "🎵 上週聽歌小結（10-11~10-18）" {
		t.Errorf("繁体周报标题: %q", got)
	}
	d.PrevLabel = weeklyDigestPeriod(0, 7*24*3600).prevLabel
	if got := digestSummaryLine(*d); got != "共播放 601 次（前一週 883，↓32%） · 約 42 小時 3 分" {
		t.Errorf("繁体第一行: %q", got)
	}
	if got := richFooter(d); got != "Lyrimuse · 資料來自 Last.fm" {
		t.Errorf("繁体来源: %q", got)
	}
}

// 切成英文以后推的报告:标题是英文,推送记录里认「同一期」的键仍是简体标题,同一期再推一次认得出已经推过。
func TestDigestPushKeepsHansKey(t *testing.T) {
	withNotifyLogForTest(t)
	var mu sync.Mutex
	var titles []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		mu.Lock()
		titles = append(titles, m["title"].(string))
		mu.Unlock()
	}))
	defer srv.Close()
	useDigestFeatures(t, featureFlags{DigestTopN: 2, ReportLanguage: digestLangEn})
	a := alerterFromConfig(&config{NotificationChannels: []notifyChannel{{ID: "wh", Platform: platformWebhook, URL: srv.URL, Enabled: true}}})
	now := time.Date(2026, 10, 20, 22, 5, 0, 0, time.Local)
	title := func(w digestWords) string { return dailyDigestTitle(w, now, midnightOf(now)) }
	if err := digestPush(a, notifyEventDaily, title, *richSample()); err != nil {
		t.Fatal(err)
	}
	e, ok := lastNotifyLogEntry(t)
	if len(titles) != 1 || titles[0] != "🎧 Today's Listening Report (Oct 20)" || !ok || e.Key != "🎧 今日听歌报告（10-20）" {
		t.Fatalf("titles=%v entry=%+v", titles, e)
	}
	if err := digestPush(a, notifyEventDaily, title, *richSample()); err != nil || len(titles) != 1 {
		t.Errorf("同一期再推一次应认出已经推过: err=%v titles=%v", err, titles)
	}
}

// reportLangCase 是共用样例 shared/testdata/notify/report-lang.json 的一条:preview.json 里的日报 / 周报样例按某种语言、
// 某一套内容拼出来的标题、第一行、纯文本正文、排版用的各段和新歌手那句。App 的 Core `NotificationPreview` 照同一份对拍。
type reportLangCase struct {
	Lang     string `json:"lang"`
	Kind     string `json:"kind"`
	Style    string `json:"style"`
	TopN     int    `json:"top_n"`
	Title    string `json:"title"`
	Summary  string `json:"summary"`
	Body     string `json:"body"`
	Sections []struct {
		Heading string   `json:"heading"`
		Title   string   `json:"title"`
		Lines   []string `json:"lines"`
	} `json:"sections"`
	NewLine string `json:"new_line"`
}

func reportLangCaseFor(t *testing.T, s notifyPreviewSample, lang digestLang, kind, style string, topN int) reportLangCase {
	t.Helper()
	useDigestFeatures(t, featureFlags{DigestTopN: topN, ReportLanguage: lang})
	w := dw()
	st := s.statsFor(kind, topN)
	if st.PrevPlays > 0 {
		st.PrevLabel = w["prev_week"]
	}
	now := time.Date(s.Now.Year, time.Month(s.Now.Month), s.Now.Day, s.Now.Hour, s.Now.Minute, 0, 0, time.Local)
	c := reportLangCase{Lang: string(lang), Kind: kind, Style: style, TopN: topN, Summary: digestSummaryLine(st), Body: digestBodyStyled(st, style)}
	if kind == "daily" {
		_, from, _ := dailyDigestTarget(now, defaultDigestPushMinutes)
		c.Title = dailyDigestTitle(w, now, from)
	} else {
		monday := time.Date(now.Year(), now.Month(), now.Day()-int((now.Weekday()+6)%7), 0, 0, 0, 0, time.Local)
		c.Title = weeklyDigestTitle(w, digestSourceListenBrainz, monday.AddDate(0, 0, -7).Unix(), monday.Unix())
	}
	d := &richDigest{digestStats: &st, style: style}
	for _, sec := range d.sections(func(s string) string { return s }) {
		c.Sections = append(c.Sections, struct {
			Heading string   `json:"heading"`
			Title   string   `json:"title"`
			Lines   []string `json:"lines"`
		}{sec.heading, sec.title, sec.lines})
	}
	c.NewLine = d.newLine()
	return c
}

// 三种语言 × 日报周报 × 三套内容拼出来的跟共用样例一字不差。LYRIMUSE_WRITE_REPORT_LANG=1 时按现在的实现重写样例
// (改了用语表以后用,写完逐条看一遍再提交)。
func TestDigestReportLangSample(t *testing.T) {
	s := loadNotifyPreviewSample(t)
	path := filepath.Join("..", "shared", "testdata", "notify", "report-lang.json")
	var got []reportLangCase
	for _, lang := range []digestLang{digestLangHans, digestLangHant, digestLangEn} {
		for _, kind := range []string{"daily", "weekly"} {
			for _, style := range []string{digestStyleFull, digestStyleBrief, digestStyleOverview} {
				got = append(got, reportLangCaseFor(t, s, lang, kind, style, 3))
			}
		}
	}
	if os.Getenv("LYRIMUSE_WRITE_REPORT_LANG") == "1" {
		b, _ := json.MarshalIndent(map[string]any{"cases": got}, "", "  ")
		if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var want struct {
		Cases []reportLangCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want.Cases) {
		for i := range got {
			if i < len(want.Cases) && !reflect.DeepEqual(got[i], want.Cases[i]) {
				t.Errorf("%s %s %s:\n got  %+v\n want %+v", got[i].Lang, got[i].Kind, got[i].Style, got[i], want.Cases[i])
			}
		}
		if len(got) != len(want.Cases) {
			t.Errorf("cases = %d, want %d", len(got), len(want.Cases))
		}
	}
}
