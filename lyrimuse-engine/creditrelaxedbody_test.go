package main

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// 严格版认不出、宽松版认得的三种署名排版;角色名、人名、公司名都是虚构占位。
var relaxedOnlyCredits = []string{"吉他 : 甲", "OP：某某音乐版权有限公司", "制作人Producer：乙", "企划制作 A&R：丙"}

// creditTailLRC:dtBody 那 n 行正文,末句之后 2 秒起每半秒挂一行署名。
func creditTailLRC(n int, shift float64, credits []string) string {
	lines := dtBody(n, shift)
	last := lines[len(lines)-1].at
	for i, s := range credits {
		lines = append(lines, dtLine{last + 2 + 0.5*float64(i), s})
	}
	return dtLRCLines(lines...)
}

func TestRelaxedCreditLinesAreNotLyricBody(t *testing.T) {
	for _, s := range relaxedOnlyCredits {
		if isCreditLine(s) || !isRelaxedCreditLine(s, nil) {
			t.Fatalf("样本 %q 应是严格版认不出、宽松版认得的署名", s)
		}
	}
	plain := dtLRCLines(dtBody(20, 0)...)
	lrc := creditTailLRC(20, 0, relaxedOnlyCredits)
	if got, _ := lastLRCTimestampSecs(lrc); got != 170 {
		t.Errorf("末句应是 170 秒那句正文,实际 %.2f", got)
	}
	tl := displayedTimeline(lrc, "")
	if len(tl) != 20 || tl[len(tl)-1].norm != normTimelineText("第 20 句歌词在这里唱") {
		t.Errorf("整行显示轴不该带署名行,%d 行,最后一行 %q", len(tl), tl[len(tl)-1].norm)
	}
	if lyricConsensusBody(lrc) != lyricConsensusBody(plain) {
		t.Error("正文共识不该带署名行")
	}
	if contentLineCount(lrc) != 20 {
		t.Errorf("行数应是 20,实际 %d", contentLineCount(lrc))
	}
	var yrc strings.Builder
	for _, l := range dtBody(20, 0) {
		ms := int(l.at * 1000)
		yrc.WriteString("[" + strconv.Itoa(ms) + ",1000](" + strconv.Itoa(ms) + ",1000,0)" + l.text + "\n")
	}
	for i, s := range relaxedOnlyCredits {
		ms := 172000 + 500*i
		yrc.WriteString("[" + strconv.Itoa(ms) + ",500](" + strconv.Itoa(ms) + ",500,0)" + s + "\n")
	}
	tl = displayedTimeline(lrc, yrc.String())
	if len(tl) != 20 || tl[len(tl)-1].ms != 170000 {
		t.Errorf("逐字显示轴不该带署名行,%d 行,最后一行 %d ms", len(tl), tl[len(tl)-1].ms)
	}
}

// 对唱标签「男：」本身就是「短汉字 + 冒号」:正文那几处要先认标签、取标签后面那段,剩下的才判署名。
func TestSpeakerLabeledLinesStayInLyricBody(t *testing.T) {
	var lines []dtLine
	var bare []dtLine
	for i, l := range dtBody(20, 0) {
		label := []string{"男", "女"}[i%2]
		lines = append(lines, dtLine{l.at, label + "：" + l.text})
		bare = append(bare, l)
	}
	lrc := dtLRCLines(append(lines, dtLine{172, "制作人Producer：乙"})...)
	plain := dtLRCLines(bare...)
	if !reflect.DeepEqual(displayedTimeline(lrc, ""), displayedTimeline(plain, "")) {
		t.Errorf("整行显示轴应只剥掉标签、保留对唱行: %d 行", len(displayedTimeline(lrc, "")))
	}
	var yrc strings.Builder
	for _, l := range lines {
		ms := int(l.at * 1000)
		yrc.WriteString("[" + strconv.Itoa(ms) + ",1000](" + strconv.Itoa(ms) + ",1000,0)" + l.text + "\n")
	}
	tl := displayedTimeline(lrc, yrc.String())
	if len(tl) != 20 || tl[0].norm != normTimelineText("第 1 句歌词在这里唱") {
		t.Errorf("逐字显示轴应只剥掉标签、保留对唱行: %d 行", len(tl))
	}
	if lyricConsensusBody(lrc) != lyricConsensusBody(plain) || contentLineCount(lrc) != 20 {
		t.Errorf("共识 / 行数应只剥掉标签: 行数 %d", contentLineCount(lrc))
	}

	// 句中插进来的单字母演唱者标签不是中英连写的职务。
	var cg []dtLine
	for i, l := range dtBody(12, 0) {
		cg = append(cg, dtLine{l.at, []string{"C", "G", "T"}[i%3] + "：" + l.text})
	}
	cg = append(cg, dtLine{120, "我知你爱我 G：讲我知你爱我"})
	lrc = dtLRCLines(cg...)
	if sp := lyricSpeakerLabels(lrc); !sp["G"] {
		t.Fatalf("样本应认出 G 是演唱者标签: %v", sp)
	}
	if contentLineCount(lrc) != 13 {
		t.Errorf("句中带单字母标签的那句是正文,行数应是 13,实际 %d", contentLineCount(lrc))
	}
}

// 同一首歌各家署名排版不同:正文一样、只是一家末尾多挂几行宽松版才认得的署名,两家分项要一样。
func TestRelaxedCreditTailDoesNotSkewScores(t *testing.T) {
	raw := map[string]lyricSourceResult{
		"qq":   {source: "qq", lyr: creditTailLRC(20, 0, []string{"作词：甲"}), matchTitle: "只有我和你的地方", matchArtist: "邓紫棋"},
		"migu": {source: "migu", lyr: creditTailLRC(20, 0.4, relaxedOnlyCredits), matchTitle: "只有我和你的地方", matchArtist: "邓紫棋"},
	}
	scored := rankLyricSourceResults("邓紫棋", "只有我和你的地方", "", 187, raw)
	if len(scored) != 2 || scored[0].Score != scored[1].Score || scored[0].Source != "qq" {
		t.Fatalf("署名排版不同不该拉开分数,应打平且 QQ(时间轴早)在前: %v", dtSortedSources(scored))
	}
	terms := func(r scoredLyricCandidateResult) string {
		var parts []string
		for _, s := range r.ScoreTerms {
			parts = append(parts, fmt.Sprintf("%s:%d", s.Kind, s.Points))
		}
		return strings.Join(parts, " ")
	}
	if terms(scored[0]) != terms(scored[1]) {
		t.Errorf("两家分项应一样: %s / %s", terms(scored[0]), terms(scored[1]))
	}
}
