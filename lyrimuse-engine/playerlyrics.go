package main

import (
	"context"
	"fmt"
	"log"
	"maps"
	"time"
)

// 按播放器各存一份歌词。
//
// 同一首歌在不同平台常是不同母版(前奏长短、剪辑不同),时间轴整体差一两秒,曲长却近到共用一条缓存(差在 12% 以内)。
// 主歌词只对得上其中一份音频;在别的播放器里放时,它自家平台按本机认出的这一首取的歌词才对得上它放的那份。
// 这一首在播放器 B 里放、主歌词不是 B 自家源时,只问 B 自家那个源一次:本机认得出这一首(IdentityFromLocalClient)才算数,
// 跟主歌词按正文配对比时间轴,整体错开 timelineNearMs 以上就记进 PlayerLyrics[B],App 在 B 里放时改用它;对齐或判不了只记
// 比对过。每一项都绑着比对时主歌词的指纹(MainFP),主歌词换了就作废,下次在 B 里放时重比。正文手改过、标了纯音乐的不比。
// 见 09 章决策 351。

// playerLyricsAlt:PlayerLyrics 的一项。Lyrics 为空 = 比对过、不用换。
type playerLyricsAlt struct {
	MainFP string `json:"main_fp"`
	Source string `json:"source,omitempty"`
	Lyrics string `json:"lyrics,omitempty"`
	YRC    string `json:"yrc,omitempty"`
	Tr     string `json:"tr,omitempty"`
	TrLang string `json:"tr_lang,omitempty"`
	BG     string `json:"bg,omitempty"`
	// ShiftMs:这份相对主歌词的中位偏移(毫秒),正数 = 这份整体晚。
	ShiftMs int   `json:"shift_ms,omitempty"`
	At      int64 `json:"at"`
}

// playerLyricsCheckTimeout:一次只问一个源,给足一次搜索 + 取词的时间。
const playerLyricsCheckTimeout = 30 * time.Second

// playerLyricsTried:这一进程里比过的「key + 播放器 + 主歌词指纹」。调用方持 enrichMu。
var playerLyricsTried = map[string]bool{}

// mainLyricsFingerprint:主歌词的指纹,算法同演唱者标注(lyricSpeakersFingerprint),App 侧 LyricSpeakerTags.fingerprint 对得上。
func mainLyricsFingerprint(e enrichEntry) string {
	return lyricSpeakersFingerprint(e.Lyrics.String(), e.LyricsYRC.String())
}

// needsPlayerLyricsCheck:在 bundleID 这个播放器里放这首时,要不要问一次它自家源。纯函数(读设置的开关),单测覆盖。
func needsPlayerLyricsCheck(e enrichEntry, bundleID string) bool {
	native := playerNativeLyricSource(playerForBundleID(bundleID))
	if native == "" || !lyricSourceEnabled(native) || e.Lyrics.IsZero() || e.Instrumental || e.lyricsHandEdited() ||
		e.LyricsSource == native {
		return false
	}
	alt, ok := e.PlayerLyrics[bundleID]
	return !ok || alt.MainFP != mainLyricsFingerprint(e)
}

// playerLyricsCheckOnce:这一进程里第一次为这条主歌词问这个播放器时返回 true。调用方持 enrichMu。
func playerLyricsCheckOnce(key, bundleID string, e enrichEntry) bool {
	k := key + "\x00" + bundleID + "\x00" + mainLyricsFingerprint(e)
	if playerLyricsTried[k] {
		return false
	}
	playerLyricsTried[k] = true
	return true
}

// playerLyricsAltFrom:按主歌词和播放器自家源这一轮的那份定 PlayerLyrics 的一项。ok=false:这一份不算数(被判了负分、没给词、
// 不是按本机认出的这一首取的),什么都不记。纯函数,单测覆盖。
func playerLyricsAltFrom(mainLRC, mainYRC string, c scoredLyricCandidateResult, now int64) (playerLyricsAlt, bool) {
	if c.Score < 0 || !c.IdentityFromLocalClient || c.Lyrics == "" {
		return playerLyricsAlt{}, false
	}
	alt := playerLyricsAlt{MainFP: lyricSpeakersFingerprint(mainLRC, mainYRC), At: now}
	rel, shift := classifyTimelinesShift(displayedTimeline(c.Lyrics, c.LyricsYRC), displayedTimeline(mainLRC, mainYRC), timelineNearMs)
	if rel != timelineShiftedLater && rel != timelineShiftedEarlier {
		return alt, true
	}
	alt.Source, alt.Lyrics, alt.YRC, alt.Tr, alt.TrLang, alt.BG = c.Source, c.Lyrics, c.LyricsYRC, c.LyricsTr, c.LyricsTrLang, c.LyricsBG
	alt.ShiftMs = shift
	return alt, true
}

// checkPlayerLyrics 问一次 bundleID 的自家源、比时间轴、记进 PlayerLyrics,见文件头注。调用方已置 enrichInflight[key]。
func checkPlayerLyrics(ctx context.Context, key, artist, title, album string, durationSecs float64, bundleID string) {
	defer func() {
		enrichMu.Lock()
		delete(enrichInflight, key)
		enrichMu.Unlock()
	}()
	enrichMu.Lock()
	e, ok := enrichCache[key]
	enrichMu.Unlock()
	if !ok || !needsPlayerLyricsCheck(e, bundleID) {
		return
	}
	native := playerNativeLyricSource(playerForBundleID(bundleID))
	ctx, cancel := withCancelAfter(withLyricSourceOnly(withBackgroundOutbound(ctx), []string{native}), playerLyricsCheckTimeout)
	defer cancel()
	_, scored := scoredLyricCandidates(ctx, artist, title, album, durationSecs)
	var alt playerLyricsAlt
	found := false
	var seen []string
	for _, c := range scored {
		if c.Source != native {
			continue
		}
		seen = append(seen, fmt.Sprintf("score=%d local=%v dur=%.2f lyrics=%v", c.Score, c.IdentityFromLocalClient,
			c.SourceReportedDurationSecs, c.Lyrics != ""))
		if alt, found = playerLyricsAltFrom(e.Lyrics.String(), e.LyricsYRC.String(), c, time.Now().Unix()); found {
			break
		}
	}
	if !found {
		log.Printf("player lyrics: %s  %s did not give lyrics for the track %s has open, nothing recorded (candidates: %v)",
			key, native, bundleID, seen)
		return
	}
	enrichMu.Lock()
	cur, ok := enrichCache[key]
	// 问的这段时间里主歌词换了(重新选源、用户采纳或编辑):这次比的不再是它。
	if !ok || mainLyricsFingerprint(cur) != alt.MainFP {
		enrichMu.Unlock()
		return
	}
	cur.PlayerLyrics = maps.Clone(cur.PlayerLyrics)
	if cur.PlayerLyrics == nil {
		cur.PlayerLyrics = map[string]playerLyricsAlt{}
	}
	cur.PlayerLyrics[bundleID] = alt
	enrichCache[key] = cur
	enrichDirty = true
	// 必须先解锁:requestEnrichSaveFor 可能当场保存,保存要取 enrichMu。
	enrichMu.Unlock()
	if alt.Lyrics != "" {
		log.Printf("player lyrics: %s  %s's own %s lyrics run %+dms against %s, used while it plays", key, bundleID, native, alt.ShiftMs, cur.LyricsSource)
	} else {
		log.Printf("player lyrics: %s  %s's own %s lyrics line up with %s, nothing to swap", key, bundleID, native, cur.LyricsSource)
	}
	requestEnrichSaveFor(key)
	if enrichNotify != nil {
		select {
		case enrichNotify <- struct{}{}:
		default:
		}
	}
}
