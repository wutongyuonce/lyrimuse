package main

import (
	"encoding/json"
	"sync"
)

// foobar2000 的播放列表:App 读它配置目录里落盘的 .fplite(含着当前文件的那一份),按认当前这首那一套换好歌名歌手,
// 连同菜单「Order」里的档一起交来(App 侧 FoobarPlaylist,形状同 Pine 那份 pineQueueReply)。当前这首按完整路径在里面找,
// 找不到退回同专辑预取。单曲循环没有要预取的;随机:菜单读得到就按它,读不到才按换歌行为推断;列表循环放到最后接回开头。
// 见 02 章决策 207。

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
