package main

import "context"

// QQ 音乐非会员试听:会员歌只放开头 60 秒,系统报的时长是 60 秒、位置从 0 起,跟整首的时间轴重合。这里只把这一拍的
// 时长换回整首(appPlaybackTickFor):缓存条目按整首认、不另开「~durN」,歌词按整首搜,60 秒的试听也够不着「听满
// 一半」。整首时长先查 QQ 客户端本地曲库(Mac 版、iPad 版两份,qqlocal.go),再看歌词缓存里这一首记下的。
// 见 02 章决策 205。
// qqTrialMinSecs / qqTrialMaxSecs 跟 App 侧 LocalPlaybackSource.qqTrialLength 必须一起改。
const (
	qqTrialMinSecs = 59.0
	qqTrialMaxSecs = 61.0
)

// qqTrialFull 判这一拍是不是 QQ 音乐试听,是就给出整首时长。full 只在时长落在试听范围里时才调。纯函数。
func qqTrialFull(bundle string, duration float64, full func() float64) (float64, bool) {
	if bundle != qqMusicBundleID || duration < qqTrialMinSecs || duration > qqTrialMaxSecs {
		return 0, false
	}
	f := full()
	if f-duration < trialMinGapSecs {
		return 0, false
	}
	return f, true
}

// liveQQTrialFull:liveTrialFull 的 QQ 音乐那一支,入参是 App 报的原始标签。
func liveQQTrialFull(bundle, title, artist, album string, duration float64) (float64, bool) {
	return qqTrialFull(bundle, duration, func() float64 {
		if d := qqLocalFullDuration(context.Background(), artist, title, album); d > 0 {
			return d
		}
		return cachedFullDuration(artist, title, album)
	})
}

// qqLocalFullDuration:QQ 本地曲库(两份合在一起)里这首歌的整首时长(秒),查不到或认不准返回 0。按「歌手 + 歌名」找,
// 键跟 qqLocalMatch 一样是 QQ 自己的署名原样,挑法见 pickFullDuration。
func qqLocalFullDuration(ctx context.Context, artist, title, album string) float64 {
	key := qqLocalKey(artist, title)
	if key == "" {
		return 0
	}
	qqLocalMu.Lock()
	refreshQQLocalIndexLocked(ctx)
	refreshQQIPadLocalIndexLocked(ctx)
	ents := append(append([]qqLocalEntry(nil), qqLocalIndex[key]...), qqIPadLocalIndex[key]...)
	qqLocalMu.Unlock()
	cands := make([]fullDurationCand, 0, len(ents))
	for _, e := range ents {
		cands = append(cands, fullDurationCand{album: e.album, secs: e.duration})
	}
	return pickFullDuration(cands, album)
}
