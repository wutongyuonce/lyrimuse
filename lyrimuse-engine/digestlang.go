package main

import (
	"strconv"
	"strings"
	"time"
)

// 听歌报告用哪种语言写:App 把自己的界面语言写进 features.json 的 report_language(zh-hans / zh-hant / en),报告里的
// 固定用语(标题、第一行、小节名、三套预设模板、各平台排版的标签)按它从 digestWordTable 取;歌名、歌手名原样。
// 缺这个键、认不得的值按简体。每一句里的 {名字} 由调用处换成值(digestWords.f),Core `ReportWords` 用同一张表、
// 同一种换法,两边对得上靠共用样例 shared/testdata/notify/words.json。
// 判「这个渠道收到过这一期没有」的键照旧用简体标题(digestHansTitle,再经 digestTitleKey 去掉今日 / 昨日),切换语言不会把同一期当成新的一期。见 15 章决策 98。

type digestLang string

const (
	digestLangHans digestLang = "zh-hans"
	digestLangHant digestLang = "zh-hant"
	digestLangEn   digestLang = "en"
)

func parseDigestLang(s string) digestLang {
	switch l := digestLang(strings.ToLower(strings.TrimSpace(s))); l {
	case digestLangHant, digestLangEn:
		return l
	}
	return digestLangHans
}

// digestWords 是一种语言的那张表。
type digestWords map[string]string

func digestWordsFor(lang digestLang) digestWords {
	if w, ok := digestWordTable[lang]; ok {
		return w
	}
	return digestWordTable[digestLangHans]
}

// dw:当前报告语言的那张表。
func dw() digestWords { return digestWordsFor(features().ReportLanguage) }

// f 取 key 那一句,把 {名字} 换成 kv 里的值(名字、值交替给)。
func (w digestWords) f(key string, kv ...string) string {
	s := w[key]
	for i := 0; i+1 < len(kv); i += 2 {
		s = strings.ReplaceAll(s, "{"+kv[i]+"}", kv[i+1])
	}
	return s
}

// count:带数量 {n} 的那一句;数量是 1 且表里有 key_one 时用它(英文单复数)。
func (w digestWords) count(key string, n int, kv ...string) string {
	if _, ok := w[key+"_one"]; ok && n == 1 {
		key += "_one"
	}
	return w.f(key, append([]string{"n", strconv.Itoa(n)}, kv...)...)
}

// date:标题里的日期(本地)。
func (w digestWords) date(t time.Time) string {
	return w.f("date", "mm", t.Format("01"), "dd", t.Format("02"), "mon", t.Format("Jan"), "d", strconv.Itoa(t.Day()))
}

// month:带年、月的那一句,月份既有数字 {month} 也有英文名 {month_name}。
func (w digestWords) month(key string, year int, month time.Month) string {
	return w.f(key, "year", strconv.Itoa(year), "month", strconv.Itoa(int(month)), "month_name", month.String())
}

// digestHansTitle:简体的标题,推送记录里认「同一期」用(notifyMessage.Key)。title 是按某种语言算标题的函数。
func digestHansTitle(title func(digestWords) string) string {
	return title(digestWordsFor(digestLangHans))
}

// digestWordTable:三种语言的用语。简体那份就是引擎一直推出去的原文,改它等于改所有简体用户收到的报告。
var digestWordTable = map[digestLang]digestWords{
	digestLangHans: {
		"date":            "{mm}-{dd}",
		"title_today":     "🎧 今日听歌报告（{date}）",
		"title_yesterday": "🎧 昨日听歌报告（{date}）",
		"title_day":       "🎧 听歌报告（{date}）",
		"title_week":      "🎵 上周听歌小结（{from}~{to}）",
		"title_month":     "📅 {year} 年 {month} 月听歌小结",
		"title_year":      "🏆 {year} 年度听歌小结",
		"title_sample":    "🎧 听歌报告示例",
		"plays":           "共播放 {n} 次",
		"plays_truncated": "共播放 {n} 次以上（记录太多，只统计了最近这些）",
		"compare":         "（{label} {prev}，{change}）",
		"flat":            "持平",
		"duration_hm":     " · 约 {h} 小时 {m} 分",
		"duration_m":      " · 约 {m} 分",
		"value_hm":        "{h} 小时 {m} 分",
		"value_m":         "{m} 分",
		"prev_week":       "前一周",
		"prev_month":      "{month} 月",
		"prev_month_year": "{year} 年 {month} 月",
		"prev_year":       "{year} 年",
		"new_artists":     "新听了 {n} 位歌手",
		"names_lead":      "：",
		"name_sep":        "、",
		"more":            " 等",
		"colon":           "：",
		"head_artists":    "Top 歌手",
		"head_albums":     "Top 专辑",
		"head_tracks":     "Top 歌曲",
		"col_artists":     "歌手",
		"col_albums":      "专辑",
		"col_tracks":      "歌曲",
		"item":            "{i}. {name}（{n}）",
		"pair":            "{artist} - {name}",
		"most_played":     "最常听",
		"brief_line":      "{label}：{name} · {n}",
		"label_artist":    "歌手",
		"label_album":     "专辑",
		"label_track":     "歌曲",
		"link":            "在 {source} 上看完整榜单",
		"footer":          "Lyrimuse · 数据来自 {source}",
		"cover":           "封面",
		"keyword":         "听歌",
		"preset_full": "{summary}\n\nTop 歌手：\n{top_artists}\n\nTop 专辑：\n{top_albums}\n\nTop 歌曲：\n{top_tracks}\n\n" +
			"新听了 {new_artists} 位歌手：{new_artist_names}",
		"preset_brief": "{summary}\n最常听的歌手：{top_artist}\n最常听的专辑：{top_album}\n最常听的歌曲：{top_track}\n" +
			"新听了 {new_artists} 位歌手：{new_artist_names}",
		"preset_overview": "{summary}\n新听了 {new_artists} 位歌手",
	},
	digestLangHant: {
		"date":            "{mm}-{dd}",
		"title_today":     "🎧 今日聽歌報告（{date}）",
		"title_yesterday": "🎧 昨日聽歌報告（{date}）",
		"title_day":       "🎧 聽歌報告（{date}）",
		"title_week":      "🎵 上週聽歌小結（{from}~{to}）",
		"title_month":     "📅 {year} 年 {month} 月聽歌小結",
		"title_year":      "🏆 {year} 年度聽歌小結",
		"title_sample":    "🎧 聽歌報告範例",
		"plays":           "共播放 {n} 次",
		"plays_truncated": "共播放 {n} 次以上（記錄太多，只統計了最近這些）",
		"compare":         "（{label} {prev}，{change}）",
		"flat":            "持平",
		"duration_hm":     " · 約 {h} 小時 {m} 分",
		"duration_m":      " · 約 {m} 分",
		"value_hm":        "{h} 小時 {m} 分",
		"value_m":         "{m} 分",
		"prev_week":       "前一週",
		"prev_month":      "{month} 月",
		"prev_month_year": "{year} 年 {month} 月",
		"prev_year":       "{year} 年",
		"new_artists":     "新聽了 {n} 位歌手",
		"names_lead":      "：",
		"name_sep":        "、",
		"more":            " 等",
		"colon":           "：",
		"head_artists":    "Top 歌手",
		"head_albums":     "Top 專輯",
		"head_tracks":     "Top 歌曲",
		"col_artists":     "歌手",
		"col_albums":      "專輯",
		"col_tracks":      "歌曲",
		"item":            "{i}. {name}（{n}）",
		"pair":            "{artist} - {name}",
		"most_played":     "最常聽",
		"brief_line":      "{label}：{name} · {n}",
		"label_artist":    "歌手",
		"label_album":     "專輯",
		"label_track":     "歌曲",
		"link":            "在 {source} 上看完整排行",
		"footer":          "Lyrimuse · 資料來自 {source}",
		"cover":           "封面",
		"keyword":         "聽歌",
		"preset_full": "{summary}\n\nTop 歌手：\n{top_artists}\n\nTop 專輯：\n{top_albums}\n\nTop 歌曲：\n{top_tracks}\n\n" +
			"新聽了 {new_artists} 位歌手：{new_artist_names}",
		"preset_brief": "{summary}\n最常聽的歌手：{top_artist}\n最常聽的專輯：{top_album}\n最常聽的歌曲：{top_track}\n" +
			"新聽了 {new_artists} 位歌手：{new_artist_names}",
		"preset_overview": "{summary}\n新聽了 {new_artists} 位歌手",
	},
	digestLangEn: {
		"date":            "{mon} {d}",
		"title_today":     "🎧 Today's Listening Report ({date})",
		"title_yesterday": "🎧 Yesterday's Listening Report ({date})",
		"title_day":       "🎧 Listening Report ({date})",
		"title_week":      "🎵 Last Week's Listening Digest ({from} – {to})",
		"title_month":     "📅 {month_name} {year} Listening Digest",
		"title_year":      "🏆 {year} Listening Digest",
		"title_sample":    "🎧 Sample Listening Report",
		"plays":           "{n} plays",
		"plays_one":       "{n} play",
		"plays_truncated": "{n}+ plays (too many listens, only the most recent were counted)",
		"compare":         " (vs. {prev} {label}, {change})",
		"flat":            "no change",
		"duration_hm":     " · about {h} h {m} min",
		"duration_m":      " · about {m} min",
		"value_hm":        "{h} h {m} min",
		"value_m":         "{m} min",
		"prev_week":       "the week before",
		"prev_month":      "in {month_name}",
		"prev_month_year": "in {month_name} {year}",
		"prev_year":       "in {year}",
		"new_artists":     "{n} new artists",
		"new_artists_one": "{n} new artist",
		"names_lead":      ": ",
		"name_sep":        ", ",
		"more":            " and more",
		"colon":           ":",
		"head_artists":    "Top artists",
		"head_albums":     "Top albums",
		"head_tracks":     "Top tracks",
		"col_artists":     "Artists",
		"col_albums":      "Albums",
		"col_tracks":      "Tracks",
		"item":            "{i}. {name} ({n})",
		"pair":            "{artist} - {name}",
		"most_played":     "Most played",
		"brief_line":      "{label}: {name} · {n}",
		"label_artist":    "Artist",
		"label_album":     "Album",
		"label_track":     "Track",
		"link":            "See the full chart on {source}",
		"footer":          "Lyrimuse · Data from {source}",
		"cover":           "Cover",
		"keyword":         "Listening",
		"preset_full": "{summary}\n\nTop artists:\n{top_artists}\n\nTop albums:\n{top_albums}\n\nTop tracks:\n{top_tracks}\n\n" +
			"New artists ({new_artists}): {new_artist_names}",
		"preset_brief":    "{summary}\nTop artist: {top_artist}\nTop album: {top_album}\nTop track: {top_track}\nNew artists ({new_artists}): {new_artist_names}",
		"preset_overview": "{summary}\nNew artists: {new_artists}",
	},
}
