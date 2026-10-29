package main

import (
	"context"
	"math"
)

// 网易云非会员试听:会员歌只放一段,系统报的是这一段自己的时间轴 —— 时长 30 秒左右、位置从 0 起。网易云不给这一段在
// 整首里的起点,位置换不回原曲口径;这里只把这一拍的时长换回整首(appPlaybackTickFor):缓存条目按整首认、不另开
// 「~durN」,歌词按整首搜,30 秒的试听也够不着「听满一半」。整首时长先查网易云客户端本地曲库(neteaselocal.go 的
// dbTrack),再看歌词缓存里这一首记下的。见 02 章决策 97。
// neteaseTrialMinSecs / neteaseTrialMaxSecs 跟 App 侧 TornTrackHold.neteaseTrialLength 必须一起改。
const (
	neteaseTrialMinSecs = 29.0
	neteaseTrialMaxSecs = 31.0
	// trialMinGapSecs:整首至少比报的长这么多才算试听(网易云、QQ 音乐共用)。
	trialMinGapSecs = 15.0
)

// neteaseTrialFull 判这一拍是不是网易云试听,是就给出整首时长。full 只在时长落在试听范围里时才调。纯函数。
func neteaseTrialFull(bundle string, duration float64, full func() float64) (float64, bool) {
	if bundle != neteaseMusicBundleID || duration < neteaseTrialMinSecs || duration > neteaseTrialMaxSecs {
		return 0, false
	}
	f := full()
	if f-duration < trialMinGapSecs {
		return 0, false
	}
	return f, true
}

// liveTrialFull:appPlaybackJudge.trial 的实现,入参是 App 报的原始标签。网易云、QQ 音乐各按各的试听长度认。
func liveTrialFull(bundle, title, artist, album string, duration float64) (float64, bool) {
	if full, ok := liveNeteaseTrialFull(bundle, title, artist, album, duration); ok {
		return full, true
	}
	return liveQQTrialFull(bundle, title, artist, album, duration)
}

// liveNeteaseTrialFull:liveTrialFull 的网易云那一支。
func liveNeteaseTrialFull(bundle, title, artist, album string, duration float64) (float64, bool) {
	return neteaseTrialFull(bundle, duration, func() float64 {
		if d := neteaseLocalFullDuration(context.Background(), artist, title, album); d > 0 {
			return d
		}
		return cachedFullDuration(artist, title, album)
	})
}

// neteaseLocalFullDuration:网易云本地曲库里这首歌的整首时长(秒),查不到或认不准返回 0。按「歌手 + 歌名」找,
// 署名有几位就逐位试,挑法见 neteaseLocalPickFullDuration。
func neteaseLocalFullDuration(ctx context.Context, artist, title, album string) float64 {
	names := append([]string{artist}, artistCreditParts(artist)...)
	keys := make([]string, 0, len(names))
	for _, a := range names {
		keys = append(keys, neteaseLocalKey(a, title))
	}
	neteaseLocalMu.Lock()
	ents := neteaseLocalEntriesLocked(ctx, keys...)
	neteaseLocalMu.Unlock()
	return neteaseLocalPickFullDuration(ents, album)
}

// neteaseLocalPickFullDuration:网易云本地曲库的几条记录(时长是毫秒)交给 pickFullDuration 挑。
func neteaseLocalPickFullDuration(ents []neteaseLocalTrack, album string) float64 {
	cands := make([]fullDurationCand, 0, len(ents))
	for _, e := range ents {
		cands = append(cands, fullDurationCand{album: e.Album.Name, secs: e.Duration / 1000})
	}
	return pickFullDuration(cands, album)
}

// fullDurationCand:本地曲库里同名同歌手的一条记录,secs 为 0 = 没记时长。
type fullDurationCand struct {
	album string
	secs  float64
}

// pickFullDuration:同名的几条里有专辑对得上的取那一条;没有就要几条的时长都差不多(3% 以内)才取,否则不猜。
func pickFullDuration(cands []fullDurationCand, album string) float64 {
	if album != "" {
		for _, c := range cands {
			if c.secs > 0 && c.album != "" && normLoose(c.album) == normLoose(album) {
				return c.secs
			}
		}
	}
	lo, hi := math.Inf(1), 0.0
	for _, c := range cands {
		if c.secs > 0 {
			lo, hi = math.Min(lo, c.secs), math.Max(hi, c.secs)
		}
	}
	if hi <= 0 || (hi-lo)/hi > 0.03 {
		return 0
	}
	return hi
}

// cachedFullDuration:歌词缓存里这一首记下的时长,取条目时长与解析时用的时长里大的那个;没有这一首返回 0。
func cachedFullDuration(artist, title, album string) float64 {
	enrichMu.Lock()
	defer enrichMu.Unlock()
	key := enrichKey(artist, title, album)
	e, ok := enrichCache[key]
	if !ok {
		alt, found := canonicalEnrichKey(key)
		if !found {
			return 0
		}
		e = enrichCache[alt]
	}
	return math.Max(e.DurationSecs, e.ResolvedDurationSecs)
}
