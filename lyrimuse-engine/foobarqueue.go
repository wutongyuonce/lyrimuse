package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// foobar2000 的播放列表:App 读它配置目录里落盘的 .fplite(含着当前文件的那一份),按认当前这首那一套换好歌名歌手,
// 连同菜单「Order」里的档一起交来(App 侧 FoobarPlaylist,形状同 Pine 那份 pineQueueReply)。当前这首按完整路径在里面找,
// 找不到退回同专辑预取。单曲循环没有要预取的;随机:菜单读得到就按它,读不到才按换歌行为推断;列表循环放到最后接回开头。
// 见 02 章决策 207。

// foobarPlaylistsDir:它落盘播放列表的目录。单测换成临时目录。
var foobarPlaylistsDir = func() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "foobar2000-v2", "playlists-v2.0")
}

// foobarQueueStamp:几份 .fplite 里最晚的修改时间,连同份数。换了列表它要开播后 4~5 秒才落盘,开播那一刻 App 读到的是上一份、
// 认不出当前这首;落盘后标记一变就重读(refreshUpcomingOnQueueChange)。目录读不到为空串(不按文件变化重读)。
func foobarQueueStamp() string {
	dir := foobarPlaylistsDir()
	entries, err := os.ReadDir(dir)
	if dir == "" || err != nil {
		return ""
	}
	var latest int64
	count := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".fplite") {
			continue
		}
		count++
		if t := fileModTime(filepath.Join(dir, e.Name())).UnixNano(); t > latest {
			latest = t
		}
	}
	return fmt.Sprintf("%d/%d", latest, count)
}

// foobarQueueAsk 让 App 读一次列表。单测替换它。
var foobarQueueAsk = func() (string, bool) {
	return askApp(appQueryRequest{Kind: appQueryFoobarQueue}, appQueryScriptTimeout)
}

var (
	foobarPlayOrderMu   sync.Mutex
	foobarPlayOrderLast queueOrder
)

func foobarUpcoming(artist, title string, n int) ([]upcomingTrack, bool) {
	out, ok := foobarQueueAsk()
	if !ok {
		return nil, false
	}
	var q pineQueueReply
	if json.Unmarshal([]byte(out), &q) != nil || len(q.Tracks) == 0 {
		return nil, false
	}
	for i := range q.Tracks {
		q.Tracks[i].path = q.Tracks[i].File
	}
	pos := pineCurrentIndex(q.Tracks, localFileCurrentPath(artist, title), artist, title)
	if pos < 0 {
		return nil, false
	}
	if q.Repeating == "one" {
		return nil, true
	}
	foobarPlayOrderMu.Lock()
	shuffled, _ := foobarPlayOrderLast.observeWrapping(pineQueueID(q.Tracks), pos, len(q.Tracks))
	foobarPlayOrderMu.Unlock()
	switch q.Repeating {
	case "shuffle":
		shuffled = true
	case "off", "all":
		shuffled = false
	}
	return pineListUpcoming(q.Tracks, pos, n, shuffled, q.Repeating == "all")
}
