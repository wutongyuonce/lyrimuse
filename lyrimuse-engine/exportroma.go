package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"log/slog"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// 导出用的最终读音:.roma.lrc 写屏上实际显示的那份,而不是 LyricsRoma 原样。屏上那几道修正(源读音覆盖不到八成整份不用、
// 残留原文、体系用错、按内容重新对行、去段落标签、形近字母行不标)在 App 显示时做,Go 里没有那套逻辑;交给 lyrics-romanize
// 按同一份 Swift 代码(LyricsExportRomanization)算,跟原样逐行一致的只记个指纹,不同的才存进 LyricsRomaExport。
// LyricsRoma 照旧存源给的原样,屏上的修正规则以后再改也能按原样重判。见 10 章决策 66。
//
// 什么时候算:常驻进程启动后 exportRomaDelay 扫一遍、之后每 exportRomaInterval 一遍,另外有条目要导出时(exportLyricsFilesFor)
// 排一次只看这几条的。导出、导入只认指纹对得上的那份,对不上就照原样导出,不会把旧正文的读音写出去。

const (
	// exportRomaDelay:启动后等这么久再扫第一遍,排在 missingRomaDelay 那道补算后面。
	exportRomaDelay = 8 * time.Minute
	// exportRomaInterval:之后每隔多久再扫一遍。
	exportRomaInterval = time.Hour
	// exportRomaBatch:一次交给 helper 几首。
	exportRomaBatch = 40
	// exportRomaDebounce:有条目要导出后等这么久再算这几条,同一首连着导好几次(换词、机翻回填)只算一次。
	exportRomaDebounce = 3 * time.Second
	// exportRomaPinned:LyricsRomaExportRules 取这个值 = 读音是在歌词文件夹里手改的,照原样导出。
	exportRomaPinned = -1
	// exportRomaMaxAliases:跟 Swift 的 EnrichCacheReader.maxHeaderAliases 同一个数。
	exportRomaMaxAliases = 12
)

// exportRomaSource:算导出读音用到的那组输入的指纹,恒不为 0。同曲其它写法的歌名歌手(抬头)不算进来:只影响认歌词开头那几行
// 抬头,变了也是下一遍按规则版本或正文变化重算时带上。
func exportRomaSource(key, lyrics, roma, yrc, bg, kana, songLanguage string) uint32 {
	c := crc32.ChecksumIEEE([]byte("export-roma"))
	for _, s := range []string{key, lyrics, roma, yrc, bg, kana, songLanguage} {
		c = crc32.Update(c, crc32.IEEETable, []byte(s))
		c = crc32.Update(c, crc32.IEEETable, []byte{0})
	}
	if c == 0 {
		c = 1
	}
	return c
}

// exportedRomaOf:这一条导出成 .roma.lrc 的内容。存着的导出读音只在指纹对得上、不是手改条目时用,否则照原样。
func exportedRomaOf(key, lyrics, roma, yrc, bg, kana, songLanguage string, manual bool, export *string, src uint32) string {
	if export == nil || src == 0 || manual {
		return roma
	}
	if src != exportRomaSource(key, lyrics, roma, yrc, bg, kana, songLanguage) {
		return roma
	}
	return *export
}

// exportedRoma:同 exportedRomaOf,按条目取(正文从小文件读)。
func exportedRoma(key string, e enrichEntry) string {
	return exportedRomaOf(key, e.Lyrics.String(), e.LyricsRoma.String(), e.LyricsYRC.String(), e.LyricsBG.String(),
		e.LyricsKana, e.SongLanguage, e.ManualLyrics, e.LyricsRomaExport, e.LyricsRomaExportSrc)
}

// exportRomaFileForms:歌词文件夹里这一条的读音文件「是这边写出去的那份」可能长什么样:原样,以及存着的导出读音(不管指纹还对不对
// 得上 —— 规则版本刚变、还没重算时,文件里仍是上次写出的那份,不能当成用户改过)。
func exportRomaFileForms(e enrichEntry) []string {
	forms := []string{e.LyricsRoma.String()}
	if e.LyricsRomaExport != nil && !slices.Contains(forms, *e.LyricsRomaExport) {
		forms = append(forms, *e.LyricsRomaExport)
	}
	return forms
}

// pinExportRoma:读音刚从歌词文件夹里的手改读进来,照原样导出、不按屏上规则重算(重算会把用户写的改写掉)。输入一变就作废。
func pinExportRoma(key string, e *enrichEntry) {
	e.LyricsRomaExport = nil
	e.LyricsRomaExportSrc = exportRomaSource(key, e.Lyrics.String(), e.LyricsRoma.String(), e.LyricsYRC.String(),
		e.LyricsBG.String(), e.LyricsKana, e.SongLanguage)
	e.LyricsRomaExportRules = exportRomaPinned
}

// exportRomaItem / exportRomaResult:lyrics-romanize 批量导出读音的入参和回包(协议见 lyrics-romanize/main.swift)。
type exportRomaItem struct {
	Lyrics    string     `json:"lyrics"`
	Roma      string     `json:"roma"`
	YRC       string     `json:"yrc,omitempty"`
	BG        string     `json:"bg,omitempty"`
	KanaDonor string     `json:"kana_donor,omitempty"`
	Title     string     `json:"title,omitempty"`
	Artist    string     `json:"artist,omitempty"`
	Aliases   [][]string `json:"aliases,omitempty"`
	Cantonese bool       `json:"cantonese,omitempty"`
	Hokkien   bool       `json:"hokkien,omitempty"`
}

type exportRomaResult struct {
	Same bool    `json:"same"`
	Roma *string `json:"roma"`
}

type exportRomaReply struct {
	OK      bool               `json:"ok"`
	Results []exportRomaResult `json:"export_results"`
	Rules   int                `json:"export_rules"`
}

// errExportRomaUnsupported:helper 是不认批量导出读音的旧版本。
var errExportRomaUnsupported = errors.New("lyrics-romanize did not answer export_items")

// exportRomaHelper 是补算实际调用的;测试换成假实现。items 为空时只问规则版本。
var exportRomaHelper = func(items []exportRomaItem) ([]exportRomaResult, int, error) {
	bin, err := lyricsRomanizeBin()
	if err != nil {
		return nil, 0, err
	}
	if _, err := os.Stat(bin); err != nil {
		return nil, 0, errRomanizeHelperMissing
	}
	if items == nil {
		items = []exportRomaItem{}
	}
	payload, err := json.Marshal(map[string]any{"export_items": items})
	if err != nil {
		return nil, 0, fmt.Errorf("marshal export roma request: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), romanizeHelperTimeout+time.Duration(len(items))*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin)
	cmd.Stdin = bytes.NewReader(payload)
	out, err := cmd.Output()
	if err != nil {
		return nil, 0, fmt.Errorf("run lyrics-romanize: %w", err)
	}
	var reply exportRomaReply
	if err := json.Unmarshal(out, &reply); err != nil {
		return nil, 0, fmt.Errorf("parse lyrics-romanize output: %w", err)
	}
	if !reply.OK || reply.Rules == 0 || len(reply.Results) != len(items) {
		return nil, 0, errExportRomaUnsupported
	}
	return reply.Results, reply.Rules, nil
}

// exportRomaSeen:本进程里核过的条目,正文、读音这几样都没变、规则版本也一样就不再读正文算指纹。按 bodyText.Equal 比,不读小文件。
type exportRomaSeen struct {
	lyrics, roma, yrc, bg bodyText
	kana, songLanguage    string
	src                   uint32
	rules                 int
}

var (
	exportRomaMu     sync.Mutex // 一次只跑一遍
	exportRomaSeenMu sync.Mutex
	exportRomaSeenBy = map[string]exportRomaSeen{}
	// exportRomaLive:常驻进程起了补算(startExportRomaRefresh)才为真;一次性子命令、单测里导出不排补算。
	exportRomaLive      atomic.Bool
	exportRomaCtx       atomic.Pointer[context.Context]
	exportRomaPendingMu sync.Mutex
	exportRomaPending   = map[string]bool{}
	exportRomaTimer     *time.Timer
)

func (s exportRomaSeen) matches(e enrichEntry, rules int) bool {
	return s.rules == rules && s.src == e.LyricsRomaExportSrc && s.kana == e.LyricsKana && s.songLanguage == e.SongLanguage &&
		s.lyrics.Equal(e.Lyrics) && s.roma.Equal(e.LyricsRoma) && s.yrc.Equal(e.LyricsYRC) && s.bg.Equal(e.LyricsBG)
}

func noteExportRomaSeen(key string, s exportRomaSeen) {
	exportRomaSeenMu.Lock()
	exportRomaSeenBy[key] = s
	exportRomaSeenMu.Unlock()
}

// isExportRomaCandidate:有正文、不是纯音乐、不是手改条目,而且原样有读音或者存着导出读音(原样没读音、该有的那种由
// missingroma.go 先补原样)。
func isExportRomaCandidate(e enrichEntry) bool {
	return !e.Lyrics.IsZero() && !e.Instrumental && !e.ManualLyrics && (!e.LyricsRoma.IsZero() || e.LyricsRomaExport != nil)
}

// startExportRomaRefresh:常驻进程启动后调。
func startExportRomaRefresh(ctx context.Context) {
	exportRomaCtx.Store(&ctx)
	exportRomaLive.Store(true)
	go func() {
		wait := exportRomaDelay
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			refreshExportRoma(ctx, nil)
			wait = exportRomaInterval
		}
	}()
}

// requestExportRoma:这几条要导出了,过 exportRomaDebounce 再按屏上规则核一遍它们的导出读音。补算没起时什么都不做。
func requestExportRoma(keys ...string) {
	if !exportRomaLive.Load() || len(keys) == 0 {
		return
	}
	exportRomaPendingMu.Lock()
	defer exportRomaPendingMu.Unlock()
	for _, k := range keys {
		exportRomaPending[k] = true
	}
	if exportRomaTimer != nil {
		exportRomaTimer.Reset(exportRomaDebounce)
		return
	}
	exportRomaTimer = time.AfterFunc(exportRomaDebounce, func() {
		exportRomaPendingMu.Lock()
		only := exportRomaPending
		exportRomaPending = map[string]bool{}
		exportRomaPendingMu.Unlock()
		ctx := context.Background()
		if p := exportRomaCtx.Load(); p != nil {
			ctx = *p
		}
		refreshExportRoma(ctx, only)
	})
}

// refreshExportRoma 核一遍(only 为 nil 时全部,否则只看这几条),返回导出内容变了的条数。锁里只挑候选、拷正文句柄,
// 读正文、起 helper 都在锁外;写回时正文、读音、借来的假名、语种都还是当初那份才写。
func refreshExportRoma(ctx context.Context, only map[string]bool) int {
	exportRomaMu.Lock()
	defer exportRomaMu.Unlock()
	_, rules, err := exportRomaHelper(nil)
	if err != nil {
		if !errors.Is(err, errRomanizeHelperMissing) {
			warnf("export romanization: helper unavailable: %v", err)
		}
		return 0
	}
	type pending struct {
		key    string
		seen   exportRomaSeen
		manual bool
		export *string
		song   string
	}
	var picked []pending
	songKeys := map[string][]string{}
	enrichMu.Lock()
	exportRomaSeenMu.Lock()
	for k, e := range enrichCache {
		if e.Song != "" {
			songKeys[e.Song] = append(songKeys[e.Song], k)
		}
		if only != nil && !only[k] {
			continue
		}
		if !isExportRomaCandidate(e) {
			continue
		}
		if s, ok := exportRomaSeenBy[k]; ok && s.matches(e, rules) {
			continue
		}
		picked = append(picked, pending{key: k, manual: e.ManualLyrics, export: e.LyricsRomaExport, song: e.Song,
			seen: exportRomaSeen{lyrics: e.Lyrics, roma: e.LyricsRoma, yrc: e.LyricsYRC, bg: e.LyricsBG,
				kana: e.LyricsKana, songLanguage: e.SongLanguage, src: e.LyricsRomaExportSrc,
				rules: e.LyricsRomaExportRules}})
	}
	exportRomaSeenMu.Unlock()
	enrichMu.Unlock()
	if len(picked) == 0 {
		return 0
	}

	type job struct {
		pending
		src                   uint32
		lyrics, roma, yrc, bg string
		item                  exportRomaItem
	}
	var jobs []job
	for _, p := range picked {
		if ctx.Err() != nil {
			return 0
		}
		lyrics, e1 := p.seen.lyrics.load()
		roma, e2 := p.seen.roma.load()
		yrc, e3 := p.seen.yrc.load()
		bg, e4 := p.seen.bg.load()
		if err := errors.Join(e1, e2, e3, e4); err != nil {
			continue
		}
		src := exportRomaSource(p.key, lyrics, roma, yrc, bg, p.seen.kana, p.seen.songLanguage)
		if src == p.seen.src && (p.seen.rules == rules || p.seen.rules == exportRomaPinned) {
			p.seen.rules = rules
			noteExportRomaSeen(p.key, p.seen)
			continue
		}
		artist, title, _ := splitEnrichKey(p.key)
		jobs = append(jobs, job{pending: p, src: src, lyrics: lyrics, roma: roma, yrc: yrc, bg: bg,
			item: exportRomaItem{Lyrics: lyrics, Roma: roma, YRC: yrc, BG: bg, KanaDonor: p.seen.kana,
				Title: title, Artist: artist, Aliases: exportRomaAliases(p.key, songKeys[p.song]),
				Cantonese: p.seen.songLanguage == songLanguageCantonese,
				Hokkien:   p.seen.songLanguage == songLanguageHokkien}})
	}

	type answer struct {
		job
		export *string
	}
	var answers []answer
	failed := 0
	for start := 0; start < len(jobs); start += exportRomaBatch {
		if ctx.Err() != nil {
			break
		}
		batch := jobs[start:min(start+exportRomaBatch, len(jobs))]
		items := make([]exportRomaItem, len(batch))
		for i, j := range batch {
			items[i] = j.item
		}
		results, got, err := exportRomaHelper(items)
		if err != nil || got != rules {
			failed += len(batch)
			if failed == len(batch) {
				warnf("export romanization: helper failed candidates=%d: %v", len(batch), err)
			}
			break
		}
		for i, r := range results {
			var export *string
			if !r.Same && r.Roma != nil {
				v := *r.Roma
				export = &v
			}
			answers = append(answers, answer{job: batch[i], export: export})
		}
	}

	var changed []string
	stored := 0
	enrichMu.Lock()
	for _, a := range answers {
		e, ok := enrichCache[a.key]
		if !ok || e.ManualLyrics || !e.Lyrics.Equal(a.seen.lyrics) || !e.LyricsRoma.Equal(a.seen.roma) ||
			!e.LyricsYRC.Equal(a.seen.yrc) || !e.LyricsBG.Equal(a.seen.bg) ||
			e.LyricsKana != a.seen.kana || e.SongLanguage != a.seen.songLanguage {
			continue
		}
		before := exportedRomaOf(a.key, a.lyrics, a.roma, a.yrc, a.bg, e.LyricsKana, e.SongLanguage, false,
			e.LyricsRomaExport, e.LyricsRomaExportSrc)
		e.LyricsRomaExport, e.LyricsRomaExportSrc, e.LyricsRomaExportRules = a.export, a.src, rules
		enrichCache[a.key] = e
		stored++
		seen := a.seen
		seen.src, seen.rules = a.src, rules
		noteExportRomaSeen(a.key, seen)
		after := exportedRomaOf(a.key, a.lyrics, a.roma, a.yrc, a.bg, e.LyricsKana, e.SongLanguage, false,
			e.LyricsRomaExport, e.LyricsRomaExportSrc)
		if after != before {
			changed = append(changed, a.key)
		}
	}
	if stored > 0 {
		enrichDirty = true
	}
	enrichMu.Unlock()
	if stored > 0 {
		if err := saveEnrichCacheChecked(); err != nil {
			warnf("export romanization: save failed stored=%d: %v", stored, err)
		}
	}
	if len(changed) > 0 {
		exportLyricsFilesFor(changed...)
	}
	if only == nil || len(jobs) > 0 {
		slog.Info("export romanization: done", "candidates", len(picked), "computed", len(answers),
			"export_changed", len(changed), "failed", failed, "rules", rules)
	}
	return len(changed)
}

// exportRomaAliases:同曲其它写法的歌名、歌手(写法键前两段),排序、去重、最多 exportRomaMaxAliases 组 ——
// 跟 Swift 的 EnrichCacheReader.headerAliases 同一个取法,屏上认歌词开头抬头时用的就是它。
func exportRomaAliases(ownKey string, keys []string) [][]string {
	others := make([]string, 0, len(keys))
	for _, k := range keys {
		if k != ownKey {
			others = append(others, k)
		}
	}
	slices.Sort(others)
	var out [][]string
	for _, k := range others {
		parts := strings.SplitN(k, "|", 3)
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" {
			continue
		}
		pair := []string{parts[1], parts[0]}
		if slices.ContainsFunc(out, func(p []string) bool { return p[0] == pair[0] && p[1] == pair[1] }) {
			continue
		}
		out = append(out, pair)
		if len(out) == exportRomaMaxAliases {
			break
		}
	}
	return out
}
