package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withFoobarQueue 把问 App 换成固定的列表(repeating 为菜单的档),清掉顺序 / 随机的证据。
func withFoobarQueue(t *testing.T, repeating string, tracks []pineQueueTrack) {
	t.Helper()
	b, err := json.Marshal(pineQueueReply{Repeating: repeating, Tracks: tracks})
	if err != nil {
		t.Fatal(err)
	}
	saved := foobarQueueAsk
	foobarQueueAsk = func() (string, bool) { return string(b), true }
	foobarPlayOrderMu.Lock()
	foobarPlayOrderLast = queueOrder{}
	foobarPlayOrderMu.Unlock()
	t.Cleanup(func() { foobarQueueAsk = saved })
}

// 按列表往后取;关循环到最后为止、列表循环接回开头、单曲循环不取;列表里没有当前这首(这一轮换过列表)退回同专辑预取。
func TestFoobarUpcomingFollowsSavedPlaylist(t *testing.T) {
	withFoobarQueue(t, "off", pineTracks("A", "B", "C", "D"))
	if res, ok := foobarUpcoming("歌手B", "B", 5); !ok || pineGot(res) != "C,D" {
		t.Errorf("往后取: ok=%v %q", ok, pineGot(res))
	}
	withFoobarQueue(t, "off", pineTracks("A", "B", "C", "D"))
	if _, ok := foobarUpcoming("歌手D", "D", 5); ok {
		t.Error("关循环时最后一首后面没有")
	}
	withFoobarQueue(t, "all", pineTracks("A", "B", "C", "D"))
	if res, ok := foobarUpcoming("歌手D", "D", 2); !ok || pineGot(res) != "A,B" {
		t.Errorf("列表循环接回开头: ok=%v %q", ok, pineGot(res))
	}
	withFoobarQueue(t, "one", pineTracks("A", "B", "C"))
	if res, ok := foobarUpcoming("歌手B", "B", 5); !ok || len(res) != 0 {
		t.Errorf("单曲循环没有要预取的: ok=%v %q", ok, pineGot(res))
	}
	withFoobarQueue(t, "off", pineTracks("A", "B"))
	if _, ok := foobarUpcoming("谁", "这一轮新加的歌", 5); ok {
		t.Error("列表里没有当前这首要退回")
	}
	saved := foobarQueueAsk
	foobarQueueAsk = func() (string, bool) { return "", false }
	if _, ok := foobarUpcoming("歌手B", "B", 5); ok {
		t.Error("App 读不到列表要退回")
	}
	foobarQueueAsk = saved
}

// 菜单读得到随机就按它(第一首就交整份候选);读到关 / 列表循环就按列表,手动跳着点也不当随机;读不到才按换歌行为推断。
func TestFoobarUpcomingShuffleFromMenu(t *testing.T) {
	withFoobarQueue(t, "shuffle", pineTracks("A", "B", "C", "D"))
	if res, ok := foobarUpcoming("歌手B", "B", 1); !ok || pineGot(res) != "C,D,A" {
		t.Errorf("菜单开着随机: ok=%v %q", ok, pineGot(res))
	}
	withFoobarQueue(t, "off", pineTracks("A", "B", "C", "D"))
	foobarUpcoming("歌手A", "A", 1)
	if res, ok := foobarUpcoming("歌手C", "C", 1); !ok || pineGot(res) != "D" {
		t.Errorf("菜单是关:跳着点也按列表: ok=%v %q", ok, pineGot(res))
	}
	withFoobarQueue(t, "", pineTracks("A", "B", "C", "D"))
	foobarUpcoming("歌手A", "A", 1)
	if res, ok := foobarUpcoming("歌手C", "C", 1); !ok || pineGot(res) != "D,A,B" {
		t.Errorf("读不出菜单时按换歌行为推断随机: ok=%v %q", ok, pineGot(res))
	}
}

// 列表由 App 代读:请求种类 foobar_queue;分发接上。
func TestFoobarQueueContract(t *testing.T) {
	reqPath, repPath := useAppQueryChannel(t, true)
	seen := fakePlayerQueryApp(t, reqPath, repPath, func(r appQueryRequest) *appQueryReply {
		return &appQueryReply{Schema: appQuerySchema, ID: r.ID, OK: true, Output: `{"tracks":[]}`}
	})
	if _, ok := foobarQueueAsk(); !ok {
		t.Fatal("应拿到 App 的应答")
	}
	select {
	case req := <-seen:
		if req.Kind != appQueryFoobarQueue {
			t.Errorf("请求种类: %q", req.Kind)
		}
	case <-time.After(time.Second):
		t.Fatal("没看到请求")
	}
	src, err := os.ReadFile("../lyrimuse/Sources/LyrimuseCore/Local/FoobarPlaylist.swift")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{`return "shuffle"`, `return "all"`, `return "one"`, `return "off"`} {
		if !strings.Contains(string(src), v) {
			t.Errorf("FoobarPlaylist.repeating 里找不到 %s(两边一起改)", v)
		}
	}
	b, err := os.ReadFile("upcoming.go")
	if err != nil {
		t.Fatal(err)
	}
	if !containsCode(string(b), "case foobar2000BundleID:\n") || !containsCode(string(b), "return foobarUpcoming(artist, title, n)\n") {
		t.Error("upcomingFromQueue 里 foobar2000 没接上读列表")
	}
}

// 换了列表要过几秒才落盘:几份 .fplite 里最晚的修改时间一变,标记就变,同一首歌期间据此重读;目录读不到时不按文件变化重读。
func TestFoobarQueueStampFollowsPlaylistWrites(t *testing.T) {
	dir := t.TempDir()
	saved := foobarPlaylistsDir
	foobarPlaylistsDir = func() string { return dir }
	t.Cleanup(func() { foobarPlaylistsDir = saved })
	p := filepath.Join(dir, "playlist-A.fplite")
	if err := os.WriteFile(p, []byte("\ufefffile:///a/1.mp3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "playlist-A-props.sqlite"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Minute)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	before := upcomingQueueStamp(foobar2000BundleID)
	if before == "" {
		t.Fatal("目录读得到时应有标记")
	}
	now := time.Now()
	if err := os.Chtimes(p, now, now); err != nil {
		t.Fatal(err)
	}
	if upcomingQueueStamp(foobar2000BundleID) == before {
		t.Error("列表落盘后标记要变")
	}
	foobarPlaylistsDir = func() string { return filepath.Join(dir, "没有这个目录") }
	if foobarQueueStamp() != "" {
		t.Error("目录读不到时标记为空")
	}
}
