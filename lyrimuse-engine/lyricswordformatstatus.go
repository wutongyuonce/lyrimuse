package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"sync/atomic"
	"time"
)

// 换逐字格式的进度:引擎 → App 的状态文件 lyrimuse-lyrics-word-format-status.json,设置页「逐字歌词格式」那一行按它显示
// 「正在改存为 X… 几首 / 共几首」和「已改存」。只是通知通道:写失败只记日志,换格式照常做完。
//
// 进度按有逐字的歌数:编好算半首、写出算整首(编码并行、写出按顺序,两段都占时间)。总数在导出开始时数出来,
// 之前(导入那一步)是 0,App 显示不确定进度。
type lyricsWordFormatStatus struct {
	Running    bool     `json:"running"`
	Formats    []string `json:"formats"`
	Done       int      `json:"done"`
	Total      int      `json:"total"`
	StartedAt  int64    `json:"startedAt"`
	UpdatedAt  int64    `json:"updatedAt"`
	FinishedAt int64    `json:"finishedAt,omitempty"`
}

type lyricsWordProgress struct {
	total, encoded, written atomic.Int64
}

func (p *lyricsWordProgress) done() int {
	return int((p.encoded.Load() + p.written.Load()) / 2)
}

var (
	lyricsWordFormatStatusPath string
	// lyricsWordSwitchProgress 非 nil 时全量导出在给换格式数进度(只有 switchLyricsWordFormats 那一趟设它)。
	lyricsWordSwitchProgress atomic.Pointer[lyricsWordProgress]
)

// lyricsWordFormatStatusInterval:换格式期间多久写一次进度。
var lyricsWordFormatStatusInterval = 250 * time.Millisecond

// setLyricsWordFormatStatusPath 在 main() 启动时调一次,顺带清掉上一个进程留下的状态(跑到一半被杀的那份 running 会让界面一直转)。
func setLyricsWordFormatStatusPath() {
	lyricsWordFormatStatusPath = configFilePath(clientName + "-lyrics-word-format-status.json")
	_ = os.Remove(lyricsWordFormatStatusPath)
}

func writeLyricsWordFormatStatus(s lyricsWordFormatStatus) {
	if lyricsWordFormatStatusPath == "" {
		return
	}
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	if err := writeFileAtomic(lyricsWordFormatStatusPath, data); err != nil {
		slog.Error("lyrics word format status: write failed", "err", err)
	}
}

// trackLyricsWordFormatSwitch 开始报一趟换格式的进度,返回收尾函数(写「做完了」)。期间每 lyricsWordFormatStatusInterval 写一次。
func trackLyricsWordFormatSwitch(next []lyricsWordFormat) (finish func()) {
	names := make([]string, len(next))
	for i, f := range next {
		names[i] = string(f)
	}
	p := &lyricsWordProgress{}
	started := time.Now().Unix()
	status := func(running bool) lyricsWordFormatStatus {
		now := time.Now().Unix()
		s := lyricsWordFormatStatus{Running: running, Formats: names, Done: p.done(), Total: int(p.total.Load()),
			StartedAt: started, UpdatedAt: now}
		if !running {
			s.Done, s.FinishedAt = s.Total, now
		}
		return s
	}
	writeLyricsWordFormatStatus(status(true))
	lyricsWordSwitchProgress.Store(p)
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		tick := time.NewTicker(lyricsWordFormatStatusInterval)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				writeLyricsWordFormatStatus(status(true))
			}
		}
	}()
	return func() {
		lyricsWordSwitchProgress.Store(nil)
		close(stop)
		<-stopped
		writeLyricsWordFormatStatus(status(false))
	}
}
