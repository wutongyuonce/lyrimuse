package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestChannelDigestStyle(t *testing.T) {
	legacy := digestTemplate{Body: "旧的 {plays}"}
	own := &digestTemplate{Body: "我的 {plays}"}
	cases := []struct {
		name      string
		ch        notifyChannel
		legacy    digestTemplate
		wantStyle string
		wantBody  string
	}{
		{"没选过、没有旧模板", notifyChannel{}, digestTemplate{}, digestStyleFull, ""},
		{"没选过、沿用旧模板", notifyChannel{}, legacy, digestStyleCustom, "旧的 {plays}"},
		{"选了完整就不再沿用旧模板", notifyChannel{DigestStyle: digestStyleFull}, legacy, digestStyleFull, ""},
		{"精简", notifyChannel{DigestStyle: digestStyleBrief}, legacy, digestStyleBrief, ""},
		{"概况", notifyChannel{DigestStyle: digestStyleOverview}, digestTemplate{}, digestStyleOverview, ""},
		{"自定义用渠道自己的模板", notifyChannel{DigestStyle: digestStyleCustom, DigestTemplate: own}, legacy, digestStyleCustom, "我的 {plays}"},
		{"自定义没写模板按完整", notifyChannel{DigestStyle: digestStyleCustom}, legacy, digestStyleFull, ""},
		{"自定义写的跟默认一样按完整", notifyChannel{DigestStyle: digestStyleCustom, DigestTemplate: &digestTemplate{Title: "{title}", Body: "{body}"}},
			digestTemplate{}, digestStyleFull, ""},
		{"认不出的值按完整", notifyChannel{DigestStyle: "fancy"}, legacy, digestStyleFull, ""},
	}
	for _, c := range cases {
		style, tpl := c.ch.digestStyle(c.legacy)
		if style != c.wantStyle || tpl.Body != c.wantBody {
			t.Errorf("%s: got %s %q, want %s %q", c.name, style, tpl.Body, c.wantStyle, c.wantBody)
		}
	}
}

// 同一期报告推到几个渠道,各按各选的那一套:精简、概况照旧按平台排版,自定义按渠道自己的模板、纯文本;没选过的沿用旧模板。
// 推送记录记下每个渠道实际推的那一份,判重仍认默认标题。
func TestDigestPushPerChannelStyle(t *testing.T) {
	withNotifyLogForTest(t)
	var mu sync.Mutex
	got := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		key := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")[0]
		mu.Lock()
		got[key] = string(b)
		mu.Unlock()
		switch key {
		case "tg":
			io.WriteString(w, `{"ok":true,"result":{}}`)
		case "dc":
			w.WriteHeader(http.StatusNoContent)
		default:
			io.WriteString(w, `{"code":200,"message":"success"}`)
		}
	}))
	defer srv.Close()
	useDigestFeatures(t, featureFlags{DigestTopN: 3, DigestTemplate: digestTemplate{Body: "旧模板 {plays}"}})
	a := alerterFromConfig(&config{NotificationChannels: []notifyChannel{
		{ID: "tg", Platform: platformTelegram, URL: srv.URL + "/tg/bot123:abc", ChatID: "42", Enabled: true, DigestStyle: digestStyleBrief},
		{ID: "dc", Platform: platformDiscord, URL: srv.URL + "/dc/hook", Enabled: true, DigestStyle: digestStyleOverview},
		{ID: "bk", Platform: platformBark, URL: srv.URL + "/bk/key", Enabled: true, DigestStyle: digestStyleCustom,
			DigestTemplate: &digestTemplate{Title: "{title} · {plays} 次", Body: "最爱 {top_artist}"}},
		{ID: "old", Platform: platformBark, URL: srv.URL + "/old/key", Enabled: true},
	}})
	stats := digestStats{TotalPlays: 7, NewArtists: 2, NewArtistNames: []string{"X", "Y"},
		TopArtists: []digestTally{{Name: "A&B", Count: 4}, {Name: "C", Count: 3}},
		TopAlbums:  []digestTally{{Name: "专辑一", Sub: "A&B", Count: 4}, {Name: "专辑二", Sub: "C", Count: 3}}}
	if err := digestPush(a, notifyEventDaily, "🎧 今日听歌报告（10-22）", stats); err != nil {
		t.Fatal(err)
	}
	var tg map[string]any
	_ = json.Unmarshal([]byte(got["tg"]), &tg)
	text, _ := tg["text"].(string)
	if tg["parse_mode"] != "HTML" || !strings.Contains(text, "<b>最常听</b>\n歌手：A&amp;B · 4\n专辑：专辑一 — A&amp;B · 4") ||
		strings.Contains(text, "Top ") || !strings.Contains(text, "新听了 2 位歌手：X、Y") {
		t.Errorf("Telegram 精简: 排版、只有各类第一名、名字转义, got %q", text)
	}
	if !strings.Contains(got["dc"], `"description":"共播放 7 次\n\n🆕 新听了 2 位歌手"`) || strings.Contains(got["dc"], `"fields"`) {
		t.Errorf("Discord 概况: 只有第一行和新歌手数、不分栏, got %s", got["dc"])
	}
	if !strings.Contains(got["bk"], `"title":"🎧 今日听歌报告（10-22） · 7 次"`) || !strings.Contains(got["bk"], `"body":"最爱 A\u0026B"`) {
		t.Errorf("Bark 自定义: 用渠道自己的模板, got %s", got["bk"])
	}
	if !strings.Contains(got["old"], `"body":"旧模板 7"`) {
		t.Errorf("没选过的渠道沿用旧模板, got %s", got["old"])
	}
	notifyLogMu.Lock()
	entries := readNotifyLog(notifyLogPath).Entries
	notifyLogMu.Unlock()
	byChannel := map[string]notifyLogEntry{}
	for _, e := range entries {
		byChannel[e.Channel] = e
	}
	if e := byChannel["tg"]; e.Style != digestStyleBrief || !strings.HasPrefix(e.Body, "共播放 7 次\n最常听的歌手：A&B（4）") || e.Custom {
		t.Errorf("推送记录: 精简记下 style 和那一套的纯文本, got %+v", e)
	}
	if e := byChannel["bk"]; !e.Custom || e.Key != "🎧 今日听歌报告（10-22）" {
		t.Errorf("推送记录: 自定义记 Custom、判重认默认标题, got %+v", e)
	}
}

func TestResendKeepsStyle(t *testing.T) {
	withNotifyLogForTest(t)
	ch := notifyChannel{ID: "tg", Platform: platformTelegram, URL: "123:abc", ChatID: "1", Enabled: true, DigestStyle: digestStyleBrief}
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	setLiveConfig(cfgPath, &config{NotificationChannels: []notifyChannel{ch}})
	t.Cleanup(func() { setLiveConfig("", nil) })
	stats := &digestStats{TotalPlays: 3, TopArtists: []digestTally{{Name: "A", Count: 3}}}
	recordNotifyDelivery(ch, notifyMessage{Event: notifyEventDaily, Title: "t", Body: "b", Digest: stats, Style: digestStyleBrief},
		notifyResult{Kind: notifyKindNetwork})
	e, _ := lastNotifyLogEntry(t)
	_, msg, _, ok := resendTarget(e.ID)
	if !ok || msg.Style != digestStyleBrief {
		t.Fatalf("重发带上 Style, ok=%v msg=%+v", ok, msg)
	}
	payload, _, rich, err := buildRichPayload(ch, msg)
	if err != nil || !rich || !strings.Contains(string(payload), "最常听") {
		t.Errorf("重发照那一套排版, rich=%v payload=%s", rich, payload)
	}
}
