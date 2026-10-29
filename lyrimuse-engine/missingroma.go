package main

import (
	"context"
	"sync"
	"time"
)

// 按规则该有预生成读音、实际却空着的条目,补算一份:常驻进程里定期扫,兜住那些改写正文或读音、却不接预生成的路径
// (导入歌词文件、去遮蔽、拆出内嵌译文、跨专辑 / 同曲共享、helper 当时失败)。判据跟写入时同一套(粤语出粤拼,其余看
// shouldGenerateHelperRoma),手改过的、标了纯音乐的不动。见 10 章决策 62。

// missingRomaDelay:启动后等这么久再扫第一遍,排在另外几道读音迁移后面。
const missingRomaDelay = 5 * time.Minute

// missingRomaInterval:之后每隔多久再扫一遍。
const missingRomaInterval = time.Hour

// romaWorthGenerating:这份正文按写入时的规则该不该有预生成读音(粤语歌出粤拼,别的看 shouldGenerateHelperRoma)。
func romaWorthGenerating(lyrics, songLanguage string) bool {
	e := enrichEntry{Lyrics: bt(lyrics), SongLanguage: songLanguage}
	return songLanguage == songLanguageCantonese || e.shouldGenerateHelperRoma()
}

// missingRomaGenerator 是补算实际调用的;测试换成假实现。只交给它 romaWorthGenerating 判过的。没出读音回空串;
// helper 出错、找不到要报出来,那一条下一遍再试。
var missingRomaGenerator = func(lyrics, songLanguage, kana string) (string, error) {
	if songLanguage == songLanguageCantonese {
		return jyutpingLRC(lyrics), nil
	}
	reply, err := runLyricsRomanize(romanizeRequest{Lyrics: lyrics, KanaDonor: kana})
	if err != nil || !reply.OK {
		return "", err
	}
	return reply.Roma, nil
}

// barrenRoma / missingRomaBarren 记本进程里判过「没什么可注音的」条目:正文、语种、借来的假名都没变就不再读正文、不再问。
// 正文按 bodyText.Equal 比,不读小文件。
type barrenRoma struct {
	lyrics       bodyText
	songLanguage string
	kana         string
}

var (
	missingRomaBarrenMu sync.Mutex
	missingRomaBarren   = map[string]barrenRoma{}
)

// startMissingRomaBackfill:常驻进程启动后调。
func startMissingRomaBackfill(ctx context.Context) {
	go func() {
		wait := missingRomaDelay
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			backfillMissingRoma(ctx)
			wait = missingRomaInterval
		}
	}()
}

// isMissingRomaCandidate:有正文、读音空着、用户没手改过、没标纯音乐的条目。值不值得注音交给 missingRomaGenerator 判。
func isMissingRomaCandidate(e enrichEntry) bool {
	return !e.Lyrics.IsZero() && e.LyricsRoma.IsZero() && !e.ManualLyrics && !e.Instrumental
}

// backfillMissingRoma 扫一遍,返回补了几条。锁里只挑候选、拷正文句柄,读正文和起 helper 都在锁外。
func backfillMissingRoma(ctx context.Context) int {
	type pending struct {
		key  string
		mark barrenRoma
	}
	var picked []pending
	enrichMu.Lock()
	missingRomaBarrenMu.Lock()
	for k, e := range enrichCache {
		if !isMissingRomaCandidate(e) {
			continue
		}
		mark := barrenRoma{lyrics: e.Lyrics, songLanguage: e.SongLanguage, kana: e.LyricsKana}
		if b, ok := missingRomaBarren[k]; ok && b.lyrics.Equal(mark.lyrics) && b.songLanguage == mark.songLanguage && b.kana == mark.kana {
			continue
		}
		picked = append(picked, pending{key: k, mark: mark})
	}
	missingRomaBarrenMu.Unlock()
	enrichMu.Unlock()

	items := make([]storedRomaItem, 0, len(picked))
	marks := make(map[string]barrenRoma, len(picked))
	for _, p := range picked {
		if ctx.Err() != nil {
			return 0
		}
		lyrics, err := p.mark.lyrics.load()
		if err != nil {
			continue
		}
		if !romaWorthGenerating(lyrics, p.mark.songLanguage) {
			noteBarrenRoma(p.key, p.mark)
			continue
		}
		items = append(items, storedRomaItem{key: p.key, lyrics: lyrics, kana: p.mark.kana})
		marks[p.key] = p.mark
	}
	if len(items) == 0 {
		return 0
	}
	filled, _ := regenerateStoredRoma(ctx, items, storedRomaRegeneration{
		label: "missing romanization backfill",
		regenerate: func(it storedRomaItem) (string, error) {
			roma, err := missingRomaGenerator(it.lyrics, marks[it.key].songLanguage, it.kana)
			if err == nil && roma == "" {
				noteBarrenRoma(it.key, marks[it.key])
			}
			return roma, err
		},
	})
	return filled
}

func noteBarrenRoma(key string, mark barrenRoma) {
	missingRomaBarrenMu.Lock()
	missingRomaBarren[key] = mark
	missingRomaBarrenMu.Unlock()
}
