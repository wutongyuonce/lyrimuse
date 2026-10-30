package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// 推送页的「测一下」和推送记录里的「重发」:App 写一份请求,这里推一次,把结论写进结果文件。
//   - 测一下 {"id","channel":{…}}:往那个渠道推一份这一期的日报(notifyTestMessage),按它选的那一套内容排。带的是
//     完整的渠道(编辑表单里还没保存的也能测),所以文件里有密钥:App 以 0600 写,这里一读到就消费掉。渠道停用着也照推:
//     测的就是它填得对不对。
//   - 重发 {"id","resend":"<推送记录 id>"}:把那条没推成的原样(标题、正文、事件)推到同一个渠道,渠道按 id 从
//     此刻的配置里找(改过地址、密钥的用改过的)。
//   - 预览 {"id","preview":"daily"|"weekly"}:不推,算一份真实报告写进 notifyPreviewResultPath(digestpreview.go)。
//
// 请求 notifyTestRequestPath,形制同 lyricsrematch.go:盯着目录,一变就看一次(盯不了时每
// notifyTestRequestCheckInterval 看一次)。结果 notifyTestResultPath 留最近 notifyTestResultMax 次,App 按自己发的
// id 找;只留最后一次的话,接连测两个渠道时第二个的结果会在 App 读到之前盖掉第一个。
//
// 两种都记进推送记录(测试的 event 为 test),渠道行的「上次推送」因此一推就更新。

const (
	notifyTestRequestCheckInterval = 500 * time.Millisecond
	notifyTestResultMax            = 10
)

type notifyTestRequest struct {
	ID      string        `json:"id"`
	Channel notifyChannel `json:"channel"`
	Resend  string        `json:"resend,omitempty"`
	Preview string        `json:"preview,omitempty"`
}

type notifyTestResult struct {
	ID        string `json:"id"`
	OK        bool   `json:"ok"`
	Kind      string `json:"kind"`
	Status    int    `json:"status,omitempty"`
	Code      string `json:"code,omitempty"`
	Detail    string `json:"detail,omitempty"`
	ElapsedMS int64  `json:"elapsed_ms"`
	At        int64  `json:"at"`
	// Sample:测一下推的是示例报告(取不到真实数据),App 据此提一句。
	Sample bool `json:"sample,omitempty"`
}

type notifyTestResultFile struct {
	Results []notifyTestResult `json:"results"` // 旧的在前
}

var (
	notifyTestRequestPath string
	notifyTestResultPath  string
	notifyTestResultMu    sync.Mutex
)

// setNotifyPaths 在 main() 启动时调一次:推送记录、「测一下」的请求和结果。请求和结果是上一个进程的,清掉。
func setNotifyPaths() {
	setNotifyLogPath(configFilePath(clientName + "-notify-log.json"))
	notifyTestRequestPath = configFilePath(clientName + "-notify-test-request.json")
	notifyTestResultPath = configFilePath(clientName + "-notify-test-result.json")
	notifyPreviewResultPath = configFilePath(clientName + "-notify-preview-result.json")
	_ = os.Remove(notifyTestRequestPath)
	_ = os.Remove(notifyTestResultPath)
	_ = os.Remove(notifyPreviewResultPath)
}

// 「测一下」推的内容:这一期的真实日报,同预览取数(computeDigestPreview,结果也进预览的缓存)。取不到(没连账号、今天昨天
// 都没听、取数出错)或等了 notifyTestDigestBudget 还没取完时推示例报告(digestSampleStats),标题写明是示例(title_sample);没取完的那次
// 照旧在后台取完、留给下一次。消息带简体标题当 Key:繁体、英文界面下钉钉 / 飞书关键词(多半按简体填的「听歌」)没过时,跟正式报告
// 一样按它重推一次,测试结论跟真推一致。见 15 章决策 96、99。
const (
	notifyTestDigestBudget = 20 * time.Second
)

// startNotifyTestWatcher 由 run() 单开一个 goroutine,ctx 取消时退出。每次推送另起 goroutine(Telegram 走代理
// 回退时一次能到二十来秒),这个循环一直转着收下一份请求。
func startNotifyTestWatcher(ctx context.Context) {
	if notifyTestRequestPath == "" {
		return
	}
	writes, poll := requestWakeups(ctx, filepath.Dir(notifyTestRequestPath), notifyTestRequestCheckInterval)
	defer poll.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-writes:
		case <-poll.C:
		}
		data, err := claimRequestFile(notifyTestRequestPath)
		if err != nil {
			continue
		}
		req, ok := parseNotifyTestRequest(data)
		if !ok {
			continue
		}
		if req.Preview != "" {
			go runDigestPreview(req)
			continue
		}
		go runNotifyTest(req)
	}
}

func parseNotifyTestRequest(data []byte) (notifyTestRequest, bool) {
	var req notifyTestRequest
	if err := json.Unmarshal(data, &req); err != nil || req.ID == "" {
		slog.Warn("notify test: ignoring malformed request", "err", err)
		return notifyTestRequest{}, false
	}
	req.Channel = req.Channel.trimmed()
	return req, true
}

// runNotifyTest 推一次并写结果。
func runNotifyTest(req notifyTestRequest) {
	ch, r, ok, sample := req.Channel, notifyResult{}, true, false
	var msg notifyMessage
	if req.Resend != "" {
		ch, msg, r, ok = resendTarget(req.Resend)
	} else {
		msg, sample = notifyTestMessage(time.Now(), notifyTestDigestBudget)
		msg = msg.forChannel(ch)
	}
	if ok {
		rememberNotifyChannelSecrets(ch)
		r = pushToChannel(ch, msg)
		recordNotifyDelivery(ch, msg, r)
	}
	slog.Info("notify test: done", "resend", req.Resend != "", "platform", ch.Platform, "kind", r.Kind, "status", r.Status, "code", r.Code)
	writeNotifyTestResult(notifyTestResult{ID: req.ID, OK: r.OK, Kind: r.Kind, Status: r.Status, Code: r.Code,
		Detail: r.Detail, ElapsedMS: r.Elapsed.Milliseconds(), At: time.Now().Unix(), Sample: sample})
}

// notifyTestMessage:测一下要推的那份报告(默认标题、完整正文,推前按渠道改写),sample 为 true 是示例。
func notifyTestMessage(now time.Time, budget time.Duration) (notifyMessage, bool) {
	done := make(chan digestPreviewResult, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), digestPreviewTimeout)
		defer cancel()
		done <- computeDigestPreview(ctx, liveConfig(), "daily", now)
	}()
	title, key, stats, sample := dw()["title_sample"], digestWordsFor(digestLangHans)["title_sample"], digestSampleStats(), true
	select {
	case res := <-done:
		if res.OK && res.Digest != nil {
			title, key, stats, sample = res.Title, res.Key, *res.Digest, false
		}
	case <-time.After(budget):
	}
	return notifyMessage{Event: notifyEventTest, Title: title, Body: digestBody(stats), Digest: &stats, Key: key}, sample
}

// digestSampleStats:示例日报,同 App 预览的测试数据(Core `NotificationPreview.sample(.daily)`,两边对得上靠共用样例
// preview.json 的 samples.daily),每类截到「每类列出」的条数。
func digestSampleStats() digestStats {
	a := func(name string, n int) digestTally { return digestTally{Name: name, Count: n} }
	t := func(name, sub string, n int) digestTally { return digestTally{Name: name, Sub: sub, Count: n} }
	return digestStats{
		TotalPlays: 42, TotalDurationMs: 9_960_000,
		TopArtists: topTallies([]digestTally{a("周杰伦", 12), a("方大同", 8), a("陈绮贞", 5), a("孙燕姿", 4), a("陶喆", 3),
			a("王菲", 3), a("林忆莲", 2), a("卢广仲", 2), a("Prince", 1), a("D'Angelo", 1)}),
		TopAlbums: topTallies([]digestTally{t("叶惠美", "周杰伦", 6), t("15", "方大同", 5), t("还是会寂寞", "陈绮贞", 4),
			t("未完成", "孙燕姿", 3), t("黑色柳丁", "陶喆", 3), t("只爱陌生人", "王菲", 2), t("范特西", "周杰伦", 2),
			t("野花", "林忆莲", 2), t("100 种生活", "卢广仲", 2), t("橙月", "方大同", 1)}),
		TopTracks: topTallies([]digestTally{t("晴天", "周杰伦", 4), t("春风吹", "方大同", 3), t("越洋电话", "陈绮贞", 3),
			t("遇见", "孙燕姿", 2), t("爱很简单", "陶喆", 2), t("红豆", "王菲", 2), t("以父之名", "周杰伦", 2),
			t("至少还有你", "林忆莲", 1), t("我爱你", "卢广仲", 1), t("特别的人", "方大同", 1)}),
		NewArtists: 2, NewArtistNames: []string{"Prince", "D'Angelo"},
	}
}

// resendTarget:重发那条推送记录要推的渠道和消息。记录或渠道找不到(记录被挤出去了、渠道删了)时 ok 为 false,
// r 是要写回去的结论。那条之后同一渠道已经推成过同一条(自动重推、之前点过重发)时也不推,结论记成送达。
func resendTarget(entryID string) (notifyChannel, notifyMessage, notifyResult, bool) {
	e, found := findNotifyLogEntry(entryID)
	if !found || e.OK {
		return notifyChannel{}, notifyMessage{}, notifyResult{Kind: notifyKindConfig, Detail: "delivery record not found"}, false
	}
	if notifyDeliveredSince(e.Channel, e.Event, e.dedupKey(), e.At) {
		return notifyChannel{ID: e.Channel, Platform: e.Platform}, notifyMessage{},
			notifyResult{OK: true, Kind: notifyKindOK, Detail: "already delivered"}, false
	}
	for _, ch := range configNotifyChannels(liveConfig()) {
		if ch.ID == e.Channel {
			return ch, notifyMessage{Event: e.Event, Title: e.Title, Body: e.Body, Digest: e.Digest, Key: e.Key, Custom: e.Custom, Style: e.Style},
				notifyResult{}, true
		}
	}
	return notifyChannel{}, notifyMessage{}, notifyResult{Kind: notifyKindConfig, Detail: "channel no longer exists"}, false
}

func writeNotifyTestResult(res notifyTestResult) {
	notifyTestResultMu.Lock()
	defer notifyTestResultMu.Unlock()
	var f notifyTestResultFile
	if b, err := os.ReadFile(notifyTestResultPath); err == nil {
		_ = json.Unmarshal(b, &f)
	} else {
		noteFileErr("read", notifyTestResultPath, err)
	}
	f.Results = append(f.Results, res)
	if n := len(f.Results); n > notifyTestResultMax {
		f.Results = f.Results[n-notifyTestResultMax:]
	}
	b, err := json.Marshal(f)
	if err != nil {
		return
	}
	_ = writeFileAtomic(notifyTestResultPath, b)
}
