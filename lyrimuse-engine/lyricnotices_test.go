package main

import (
	"strconv"
	"strings"
	"testing"
)

func TestLyricNoticeRulesCompile(t *testing.T) {
	if len(compiledLyricNotices) != len(lyricNoticeRules) {
		for _, r := range lyricNoticeRules {
			expr := r.pattern
			if r.ignoreCase {
				expr = "(?i)" + expr
			}
			if len(compileLyricNotices([]lyricNoticeRule{r})) == 0 {
				t.Errorf("规则 %s 在 RE2 下编不过: %s", r.id, expr)
			}
		}
	}
}

// 跟 App 的 selftest 跑同一批例句(shared/lyric-notices.json):drop 必须被那一条命中,keep 任何一条都不许命中;
// translation 那一面的 drop 还要过 isTranslationNotice 这个真实入口。
func TestLyricNoticeExamples(t *testing.T) {
	byID := map[string]compiledLyricNotice{}
	for _, n := range compiledLyricNotices {
		byID[n.id] = n
	}
	for _, ex := range lyricNoticeExamples {
		text := strings.TrimSpace(ex.text)
		if ex.drop {
			n, ok := byID[ex.rule]
			if !ok {
				t.Errorf("例句指向不存在的规则 %s", ex.rule)
				continue
			}
			if !n.re.MatchString(text) {
				t.Errorf("%s 应当命中 %q", ex.rule, ex.text)
			}
			if n.translation && !isTranslationNotice(ex.text) {
				t.Errorf("isTranslationNotice(%q) 应当为 true(规则 %s)", ex.text, ex.rule)
			}
			continue
		}
		for _, n := range compiledLyricNotices {
			if n.re.MatchString(text) {
				t.Errorf("%q 是真歌词,却被 %s 命中(例句挂在 %s 下)", ex.text, n.id, ex.rule)
			}
		}
	}
}

// 唱片公司那段整句版权声明,一行写完的(酷狗 / 酷我 / 网易云)和逐句拆开的(咪咕)。
const (
	noticeOneLine = "版权所有，翻版必究。本专辑涉及的任何作品，包括但不限于词曲、录音、录像、视听、文字、摄影和美术等作品，未经权利人授权不得使用。"
)

var noticeSplit = []string{"版权所有翻版必究", "本专辑涉及的任何作品", "包括但不限于词曲/录音/录像", "视听/文字/摄影和美术等作品", "未经权利人授权不得使用"}

// noticeTailLRC:dtBody 那 n 行正文,末句之后 2 秒起每半秒挂一行声明。
func noticeTailLRC(n int, shift float64, notices []string) string {
	lines := dtBody(n, shift)
	last := lines[len(lines)-1].at
	for i, s := range notices {
		lines = append(lines, dtLine{last + 2 + 0.5*float64(i), s})
	}
	return dtLRCLines(lines...)
}

func TestBodyNoticeLinesAreNotLyricBody(t *testing.T) {
	for _, s := range append([]string{noticeOneLine}, noticeSplit...) {
		if !isBodyNoticeLine(s) {
			t.Errorf("isBodyNoticeLine(%q) 应为 true", s)
		}
	}
	plain := dtLRCLines(dtBody(20, 0)...)
	for name, notices := range map[string][]string{"一行写完": {noticeOneLine}, "逐句拆开": noticeSplit} {
		lrc := noticeTailLRC(20, 0, notices)
		if got, _ := lastLRCTimestampSecs(lrc); got != 170 {
			t.Errorf("%s: 末句应是 170 秒那句正文,实际 %.2f", name, got)
		}
		tl := displayedTimeline(lrc, "")
		if len(tl) != 20 || tl[len(tl)-1].norm != normTimelineText("第 20 句歌词在这里唱") {
			t.Errorf("%s: 整行显示轴不该带声明行,%d 行,最后一行 %q", name, len(tl), tl[len(tl)-1].norm)
		}
		if lyricConsensusBody(lrc) != lyricConsensusBody(plain) {
			t.Errorf("%s: 正文共识不该带声明行", name)
		}
		if contentLineCount(lrc) != 20 {
			t.Errorf("%s: 行数应是 20,实际 %d", name, contentLineCount(lrc))
		}
	}
	// 逐字轴那一支:逐字行数够整行一半时显示轴按逐字行首取,声明行同样不算。
	var yrc strings.Builder
	for _, l := range dtBody(20, 0) {
		ms := int(l.at * 1000)
		yrc.WriteString("[" + strconv.Itoa(ms) + ",1000](" + strconv.Itoa(ms) + ",1000,0)" + l.text + "\n")
	}
	for i, s := range noticeSplit {
		ms := 172000 + 500*i
		yrc.WriteString("[" + strconv.Itoa(ms) + ",500](" + strconv.Itoa(ms) + ",500,0)" + s + "\n")
	}
	tl := displayedTimeline(noticeTailLRC(20, 0, noticeSplit), yrc.String())
	if len(tl) != 20 || tl[len(tl)-1].ms != 170000 {
		t.Errorf("逐字显示轴不该带声明行,%d 行,最后一行 %d ms", len(tl), tl[len(tl)-1].ms)
	}
}
