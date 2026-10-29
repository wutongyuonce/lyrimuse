package main

import (
	"context"
	"path/filepath"
	"testing"
)

func TestQQTrialFull(t *testing.T) {
	cases := []struct {
		name         string
		bundle       string
		dur, full    float64
		want         float64
		ok, askedFor bool
	}{
		{"试听", qqMusicBundleID, 60, 324, 324, true, true},
		{"整首也很短", qqMusicBundleID, 60, 70, 0, false, true},
		{"查不到整首", qqMusicBundleID, 60, 0, 0, false, true},
		{"不是试听长度", qqMusicBundleID, 45, 324, 0, false, false},
		{"比试听长一点也不算", qqMusicBundleID, 62, 324, 0, false, false},
		{"网易云的试听长度不归这里", qqMusicBundleID, 30, 324, 0, false, false},
		{"别的播放器", neteaseMusicBundleID, 60, 324, 0, false, false},
	}
	for _, c := range cases {
		asked := false
		got, ok := qqTrialFull(c.bundle, c.dur, func() float64 { asked = true; return c.full })
		if got != c.want || ok != c.ok || asked != c.askedFor {
			t.Errorf("%s: qqTrialFull = %v %v (查整首=%v), want %v %v (查整首=%v)", c.name, got, ok, asked, c.want, c.ok, c.askedFor)
		}
	}
}

// 整首时长从两份曲库合在一起挑:专辑对得上的那条优先。
func TestQQLocalFullDuration(t *testing.T) {
	resetQQLocalIndex(t, writeTestQQDB(t, []qqLocalRow{
		{Mid: "004aupsp1RFXAN", Name: "候鸟", Singer: "低苦艾乐队", Album: "另一张", MS: 200000},
	}))
	resetQQIPadLocalIndex(t, writeTestQQDB(t, []qqLocalRow{
		{Mid: "0047tFBc4M5Dsq", Name: "候鸟", Singer: "低苦艾乐队", Album: "我们不由自主的亲吻对方", MS: 324000},
	}))
	if got := qqLocalFullDuration(context.Background(), "低苦艾乐队", "候鸟", "我们不由自主的亲吻对方"); got != 324 {
		t.Fatalf("专辑对得上的 iPad 版那条: got %v, want 324", got)
	}
	if got := qqLocalFullDuration(context.Background(), "低苦艾乐队", "候鸟", "第三张"); got != 0 {
		t.Fatalf("专辑都对不上、两条时长差得多不猜: got %v, want 0", got)
	}
	resetQQIPadLocalIndex(t, filepath.Join(t.TempDir(), "missing", "qqmusic.sqlite"))
	if got := qqLocalFullDuration(context.Background(), "低苦艾乐队", "候鸟", ""); got != 200 {
		t.Fatalf("只剩 Mac 版一条: got %v, want 200", got)
	}
	if got := qqLocalFullDuration(context.Background(), "", "候鸟", ""); got != 0 {
		t.Fatalf("没有歌手不查: got %v, want 0", got)
	}
}

// 接线:读 App 状态那一拍用的 liveTrialFull 认得 QQ 音乐的试听,整首放的不动。
func TestLiveTrialFullQQ(t *testing.T) {
	resetQQLocalIndex(t, filepath.Join(t.TempDir(), "missing", "qqmusic.sqlite"))
	resetQQIPadLocalIndex(t, writeTestQQDB(t, []qqLocalRow{
		{Mid: "0047tFBc4M5Dsq", Name: "候鸟", Singer: "低苦艾乐队", Album: "我们不由自主的亲吻对方", MS: 324000},
	}))
	if full, ok := liveTrialFull(qqMusicBundleID, "候鸟", "低苦艾乐队", "我们不由自主的亲吻对方", 60); !ok || full != 324 {
		t.Fatalf("试听: got %v %v, want 324 true", full, ok)
	}
	if full, ok := liveTrialFull(qqMusicBundleID, "候鸟", "低苦艾乐队", "我们不由自主的亲吻对方", 324); ok {
		t.Fatalf("整首放的不换: got %v %v", full, ok)
	}
}
