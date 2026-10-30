package main

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// dtLRCBy:20 行同样的正文,第 i 行在 18+8i 秒再加 shift(i) 秒。
func dtLRCBy(shift func(i int) float64) string {
	var b strings.Builder
	for i := 0; i < 20; i++ {
		at := 18 + float64(i)*8 + shift(i)
		fmt.Fprintf(&b, "[%02d:%05.2f]第 %d 句歌词在这里唱\n", int(at)/60, at-float64(int(at)/60*60), i+1)
	}
	return b.String()
}

// dtLRC:整首平移 shift 秒,末句落在 170+shift 秒。
func dtLRC(shift float64) string {
	return dtLRCBy(func(int) float64 { return shift })
}

// dtLRCLines:按给出的(秒, 正文)逐行写 LRC。
func dtLRCLines(lines ...dtLine) string {
	var b strings.Builder
	for _, l := range lines {
		fmt.Fprintf(&b, "[%02d:%05.2f]%s\n", int(l.at)/60, l.at-float64(int(l.at)/60*60), l.text)
	}
	return b.String()
}

type dtLine struct {
	at   float64
	text string
}

// dtBody:dtLRC 那 20 行的前 n 行,整首平移 shift 秒。
func dtBody(n int, shift float64) []dtLine {
	var out []dtLine
	for i := 0; i < n; i++ {
		out = append(out, dtLine{18 + float64(i)*8 + shift, fmt.Sprintf("第 %d 句歌词在这里唱", i+1)})
	}
	return out
}

// dtResult:分项为 duration + lines + rest,Score 是分项之和。
func dtResult(source string, durationPts, linesPts int, lrc string, rest ...scoreTerm) scoredLyricCandidateResult {
	terms := []scoreTerm{{Kind: scoreTermDuration, Points: durationPts}, {Kind: scoreTermLines, Points: linesPts}}
	terms = append(terms, rest...)
	sum := 0
	for _, t := range terms {
		sum += t.Points
	}
	return scoredLyricCandidateResult{Source: source, Lyrics: lrc, Score: sum, ScoreTerms: terms}
}

var dtCommon = []scoreTerm{
	{Kind: scoreTermWordTiming, Points: 400},
	{Kind: scoreTermAlbum, Points: 150},
	{Kind: scoreTermConsensus, Points: 250},
}

func dtSortedSources(results []scoredLyricCandidateResult) []string {
	sorted := append([]scoredLyricCandidateResult(nil), results...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Score > sorted[j].Score })
	var out []string
	for _, r := range sorted {
		out = append(out, fmt.Sprintf("%s:%d", r.Source, r.Score))
	}
	return out
}

func TestApplyDurationNearTie(t *testing.T) {
	cases := []struct {
		name  string
		batch []scoredLyricCandidateResult
		want  []string
	}{
		{
			name: "三个声部挑居中,组外候选不挪位置",
			batch: []scoredLyricCandidateResult{
				dtResult("lrclib", 266, 34, dtLRC(0), dtCommon[0], scoreTerm{Kind: scoreTermTitleMatch, Points: 150}, dtCommon[2]),
				dtResult("netease", 262, 34, dtLRC(-0.2), dtCommon...),
				dtResult("qq", 264, 34, dtLRC(0), dtCommon...),
				dtResult("migu", 266, 34, dtLRC(0.3), dtCommon...),
			},
			// 到其余声部的中位差之和:网易云 0.7、QQ 0.5、咪咕 0.8,挑 QQ,换到组内最靠前(网易云)的位置。
			want: []string{"lrclib:1100", "qq:1100", "netease:1100", "migu:1100"},
		},
		{
			name: "两个声部挑时间轴早的",
			batch: []scoredLyricCandidateResult{
				dtResult("qq", 266, 34, dtLRC(0.4), dtCommon...),
				dtResult("soda", 264, 34, dtLRC(0), dtCommon...),
			},
			want: []string{"soda:1100", "qq:1100"},
		},
		{
			name: "两个声部差不到 0.1 秒按构造顺序",
			batch: []scoredLyricCandidateResult{
				dtResult("qq", 264, 33, dtLRC(0.08), dtCommon...),
				dtResult("soda", 264, 34, dtLRC(0), dtCommon...),
			},
			want: []string{"qq:1098", "soda:1098"},
		},
		{
			name: "同一份时间轴并成一个声部",
			batch: []scoredLyricCandidateResult{
				dtResult("qq", 264, 34, dtLRC(0), dtCommon...),
				dtResult("kugou", 264, 34, dtLRC(0), dtCommon...),
				dtResult("kuwo", 264, 34, dtLRC(0), dtCommon...),
				dtResult("migu", 266, 34, dtLRC(0.4), dtCommon...),
				dtResult("soda", 264, 34, dtLRC(0.01), dtCommon...),
			},
			// 腾讯系三家 + 汽水是一票,咪咕晚 0.4 秒是另一票;两票挑早的,声部里按构造顺序取 QQ。
			want: []string{"qq:1100", "kugou:1100", "kuwo:1100", "migu:1100", "soda:1100"},
		},
		{
			name: "声部只跟第一家比、不串链",
			batch: []scoredLyricCandidateResult{
				dtResult("netease", 264, 34, dtLRC(0.04), dtCommon...),
				dtResult("qq", 264, 34, dtLRC(0), dtCommon...),
				dtResult("soda", 266, 34, dtLRC(-0.04), dtCommon...),
			},
			// 网易云与 QQ 差 0.04 秒并成一个声部,汽水跟网易云差 0.08 秒另成一个;两个声部差 0.04~0.08 秒,按构造顺序。
			want: []string{"netease:1100", "qq:1100", "soda:1100"},
		},
		{
			name: "只有一个声部不动",
			batch: []scoredLyricCandidateResult{
				dtResult("qq", 264, 33, dtLRC(0), dtCommon...),
				dtResult("kugou", 264, 34, dtLRC(0), dtCommon...),
			},
			want: []string{"kugou:1098", "qq:1097"},
		},
		{
			name: "lines 差 3 分以内并组",
			batch: []scoredLyricCandidateResult{
				dtResult("qq", 264, 31, dtLRC(0), dtCommon...),
				dtResult("migu", 266, 34, dtLRC(0.4), dtCommon...),
			},
			want: []string{"qq:1100", "migu:1100"},
		},
		{
			name: "lines 差超过 3 分不并",
			batch: []scoredLyricCandidateResult{
				dtResult("qq", 264, 30, dtLRC(0), dtCommon...),
				dtResult("migu", 266, 34, dtLRC(0.4), dtCommon...),
			},
			want: []string{"migu:1100", "qq:1094"},
		},
		{
			name: "整首差超过 1 秒不并",
			batch: []scoredLyricCandidateResult{
				dtResult("qq", 264, 34, dtLRC(0), dtCommon...),
				dtResult("migu", 269, 34, dtLRC(1.2), dtCommon...),
			},
			want: []string{"migu:1103", "qq:1098"},
		},
		{
			name: "末句近、整首错开超过 1 秒不并",
			batch: []scoredLyricCandidateResult{
				dtResult("qq", 264, 34, dtLRC(0), dtCommon...),
				dtResult("migu", 265, 34, dtLRCBy(func(i int) float64 {
					if i == 19 {
						return 0.2
					}
					return 1.5
				}), dtCommon...),
			},
			want: []string{"migu:1099", "qq:1098"},
		},
		{
			name: "领头取分最高的,只跟领头比",
			batch: []scoredLyricCandidateResult{
				dtResult("qq", 260, 34, dtLRC(0), dtCommon...),
				dtResult("kugou", 263, 34, dtLRC(0.6), dtCommon...),
				dtResult("migu", 266, 34, dtLRC(1.2), dtCommon...),
			},
			// 领头咪咕;酷狗差 0.6 秒并进来、时间轴早,赢;QQ 差 1.2 秒留在外面,自成一组没有伙伴。
			want: []string{"kugou:1100", "migu:1100", "qq:1094"},
		},
		{
			name: "最后一句拆成两行,结尾是同一句,末句差几秒照样并组",
			batch: []scoredLyricCandidateResult{
				dtResult("qq", 264, 34, dtLRC(0), dtCommon...),
				dtResult("migu", 278, 35, dtLRCLines(append(dtBody(19, 0.3), dtLine{170.3, "第 20 句"}, dtLine{173.3, "歌词在这里唱"})...), dtCommon...),
			},
			want: []string{"qq:1113", "migu:1113"},
		},
		{
			name: "行数一样、每行都配上,只有最后一行时刻不同照样并组",
			batch: []scoredLyricCandidateResult{
				dtResult("qq", 264, 34, dtLRC(0), dtCommon...),
				dtResult("migu", 275, 34, dtLRCBy(func(i int) float64 {
					if i == 19 {
						return 2.5
					}
					return 0.4
				}), dtCommon...),
			},
			want: []string{"qq:1109", "migu:1109"},
		},
		{
			name: "结尾少了几行不并",
			batch: []scoredLyricCandidateResult{
				dtResult("qq", 264, 34, dtLRC(0), dtCommon...),
				dtResult("migu", 250, 31, dtLRCLines(dtBody(17, 0.3)...), dtCommon...),
			},
			want: []string{"qq:1098", "migu:1081"},
		},
		{
			name: "结尾是重复句、少了几遍不并",
			batch: []scoredLyricCandidateResult{
				dtResult("qq", 264, 34, dtLRCLines(append(dtBody(16, 0), dtLine{146, "我想要你我想要你"}, dtLine{154, "我想要你我想要你"}, dtLine{162, "我想要你我想要你"}, dtLine{170, "我想要你我想要你"})...), dtCommon...),
				dtResult("migu", 250, 31, dtLRCLines(append(dtBody(16, 0.3), dtLine{146.3, "我想要你我想要你"})...), dtCommon...),
			},
			want: []string{"qq:1098", "migu:1081"},
		},
		{
			name: "分项顺序不同仍算相同",
			batch: []scoredLyricCandidateResult{
				dtResult("qq", 264, 34, dtLRC(0), dtCommon...),
				dtResult("migu", 266, 34, dtLRC(0.4), dtCommon[2], dtCommon[1], dtCommon[0]),
			},
			want: []string{"qq:1100", "migu:1100"},
		},
		{
			name: "整首配对行不到八成落在中位差附近不并",
			batch: []scoredLyricCandidateResult{
				dtResult("qq", 264, 34, dtLRC(0), dtCommon...),
				dtResult("migu", 277, 34, dtLRCBy(func(i int) float64 {
					if i >= 12 {
						return 3
					}
					return 0.2
				}), dtCommon...),
			},
			want: []string{"migu:1111", "qq:1098"},
		},
		{
			name: "领头那一组之外剩下的另起一组",
			batch: []scoredLyricCandidateResult{
				dtResult("qq", 260, 34, dtLRC(0), dtCommon...),
				dtResult("kugou", 262, 34, dtLRC(0.4), dtCommon...),
				dtResult("migu", 268, 34, dtLRC(1.5), dtCommon...),
			},
			// 领头咪咕跟两家都差 1 秒以上;QQ 与酷狗另成一组,QQ 早 0.4 秒,抬到酷狗的分。
			want: []string{"migu:1102", "qq:1096", "kugou:1096"},
		},
		{
			name:  "只有一条候选",
			batch: []scoredLyricCandidateResult{dtResult("qq", 264, 34, dtLRC(0), dtCommon...)},
			want:  []string{"qq:1098"},
		},
		{
			name: "判废的与没有 duration 分的不参与",
			batch: []scoredLyricCandidateResult{
				dtResult("qq", 266, 34, dtLRC(0.4), dtCommon...),
				{Source: "migu", Lyrics: dtLRC(0), Score: -1, ScoreTerms: []scoreTerm{{Kind: scoreRejectNoLastTimestamp, Points: -1}}},
				dtResult("kugou", 0, 34, dtLRC(0), append([]scoreTerm{{Kind: scoreTermDurationOff, Points: -durationMismatchPenalty}}, dtCommon...)...),
				{Source: "kuwo", Lyrics: dtLRC(0), Score: 834, ScoreTerms: append([]scoreTerm{{Kind: scoreTermLines, Points: 34}}, dtCommon...)},
			},
			want: []string{"qq:1100", "kuwo:834", "kugou:334", "migu:-1"},
		},
		{
			name: "吃了时间轴扣分的不并进没吃的那一组",
			batch: []scoredLyricCandidateResult{
				dtResult("qq", 264, 34, dtLRC(0), dtCommon...),
				dtResult("migu", 266, 34, dtLRC(0.4), append([]scoreTerm{{Kind: scoreTermTimelineOffset, Points: -timelineOffsetPenalty}}, dtCommon...)...),
			},
			want: []string{"qq:1098", "migu:500"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			applyDurationNearTie(tc.batch, 187)
			if got := dtSortedSources(tc.batch); strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Fatalf("排序后 %v,要 %v", got, tc.want)
			}
			for _, r := range tc.batch {
				if r.Score < 0 {
					continue
				}
				sum := 0
				for _, term := range r.ScoreTerms {
					sum += term.Points
				}
				if max(sum, 1) != r.Score {
					t.Fatalf("%s 分数 %d 跟分项之和 %d 对不上", r.Source, r.Score, sum)
				}
			}
		})
	}
}

func TestApplyDurationNearTieUnknownDuration(t *testing.T) {
	batch := []scoredLyricCandidateResult{
		dtResult("qq", 264, 34, dtLRC(0), dtCommon...),
		dtResult("migu", 266, 34, dtLRC(0.4), dtCommon...),
	}
	applyDurationNearTie(batch, 0)
	if batch[0].Source != "qq" || batch[0].Score != 1098 || batch[1].Score != 1100 {
		t.Fatalf("曲长未知不该动,实际 %s %d / %s %d", batch[0].Source, batch[0].Score, batch[1].Source, batch[1].Score)
	}
}

// 没有 lines 分项的那份抬平时补上这一项。
func TestRaiseDurationTieTermsAddsLines(t *testing.T) {
	r := scoredLyricCandidateResult{Score: 300, ScoreTerms: []scoreTerm{{Kind: scoreTermDuration, Points: 260}, {Kind: scoreTermAlbum, Points: 40}}}
	raiseDurationTieTerms(&r, 264, 2)
	if r.Score != 306 || scoreTermPoints(r.ScoreTerms, scoreTermLines) != 2 || scoreTermPoints(r.ScoreTerms, scoreTermDuration) != 264 {
		t.Fatalf("抬平后 %d %v", r.Score, r.ScoreTerms)
	}
}

// 两条流水线都接上了:同一份正文的 QQ 与咪咕,咪咕整首晚 0.4 秒,冠军是时间轴早的 QQ。
func TestDurationNearTieBothPipelines(t *testing.T) {
	early, late := dtLRC(0), dtLRC(0.4)
	check := func(t *testing.T, scored []scoredLyricCandidateResult) {
		t.Helper()
		var qq, migu *scoredLyricCandidateResult
		for i := range scored {
			switch scored[i].Source {
			case "qq":
				qq = &scored[i]
			case "migu":
				migu = &scored[i]
			}
		}
		if qq == nil || migu == nil {
			t.Fatalf("两家都该在结果里: %+v", scored)
		}
		if qq.Score != migu.Score {
			t.Fatalf("整首只差 0.4 秒应打平,QQ %d(%v) 咪咕 %d(%v)", qq.Score, qq.ScoreTerms, migu.Score, migu.ScoreTerms)
		}
		if scored[0].Source != "qq" {
			t.Fatalf("冠军应是时间轴早的 QQ,实际 %s", scored[0].Source)
		}
	}
	t.Run("rankLyricSourceResults", func(t *testing.T) {
		raw := map[string]lyricSourceResult{
			"qq":   {source: "qq", lyr: early, matchTitle: "Goodbye Melody Rose", matchArtist: "方大同"},
			"migu": {source: "migu", lyr: late, matchTitle: "Goodbye Melody Rose", matchArtist: "方大同"},
		}
		check(t, rankLyricSourceResults("方大同", "Goodbye Melody Rose", "", 187, raw))
	})
	t.Run("mergeLyricCandidateRounds", func(t *testing.T) {
		base := []scoredLyricCandidateResult{{Source: "migu", Lyrics: late, Title: "Goodbye Melody Rose", Artist: "方大同", Score: 900}}
		extra := []scoredLyricCandidateResult{{Source: "qq", Lyrics: early, Title: "Goodbye Melody Rose", Artist: "方大同", Score: 890}}
		check(t, mergeLyricCandidateRounds("方大同", "Goodbye Melody Rose", "", 187, base, extra))
	})
}
