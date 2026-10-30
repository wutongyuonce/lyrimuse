package main

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// dtLRC:20 行同样的正文,整首平移 shift 秒,末句落在 170+shift 秒。
func dtLRC(shift float64) string {
	var b strings.Builder
	for i := 0; i < 20; i++ {
		at := 18 + float64(i)*8 + shift
		fmt.Fprintf(&b, "[%02d:%05.2f]第 %d 句歌词在这里唱\n", int(at)/60, at-float64(int(at)/60*60), i+1)
	}
	return b.String()
}

// dtResult:分项为 duration + 共同的 rest,Score 是分项之和。
func dtResult(source string, durationPts int, shift float64, rest ...scoreTerm) scoredLyricCandidateResult {
	terms := []scoreTerm{{Kind: scoreTermDuration, Points: durationPts}}
	terms = append(terms, rest...)
	sum := 0
	for _, t := range terms {
		sum += t.Points
	}
	return scoredLyricCandidateResult{Source: source, Lyrics: dtLRC(shift), Score: sum, ScoreTerms: terms}
}

var dtCommon = []scoreTerm{
	{Kind: scoreTermWordTiming, Points: 400},
	{Kind: scoreTermLines, Points: 34},
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
		name    string
		batch   []scoredLyricCandidateResult
		want    []string
		comment string
	}{
		{
			name: "亚秒打平按构造顺序",
			batch: []scoredLyricCandidateResult{
				dtResult("qq", 264, 0, dtCommon...),
				dtResult("migu", 266, 0.4, dtCommon...),
			},
			want: []string{"qq:1100", "migu:1100"},
		},
		{
			name: "末句差超过 1 秒照旧连续计分",
			batch: []scoredLyricCandidateResult{
				dtResult("qq", 264, 0, dtCommon...),
				dtResult("migu", 269, 1.2, dtCommon...),
			},
			want: []string{"migu:1103", "qq:1098"},
		},
		{
			name: "其余分项不同不动",
			batch: []scoredLyricCandidateResult{
				dtResult("qq", 264, 0, dtCommon...),
				dtResult("migu", 266, 0.4, dtCommon[0], scoreTerm{Kind: scoreTermLines, Points: 35}, dtCommon[2], dtCommon[3]),
			},
			want: []string{"migu:1101", "qq:1098"},
		},
		{
			name: "分项顺序不同仍算相同",
			batch: []scoredLyricCandidateResult{
				dtResult("qq", 264, 0, dtCommon...),
				dtResult("migu", 266, 0.4, dtCommon[3], dtCommon[2], dtCommon[1], dtCommon[0]),
			},
			want: []string{"qq:1100", "migu:1100"},
		},
		{
			name: "链式只跟领头比",
			batch: []scoredLyricCandidateResult{
				dtResult("qq", 260, 0, dtCommon...),
				dtResult("kugou", 263, 0.6, dtCommon...),
				dtResult("migu", 266, 1.2, dtCommon...),
			},
			// 领头 migu;kugou 差 0.6 秒并进来,qq 差 1.2 秒留在外面,自成一组没有伙伴。
			want: []string{"kugou:1100", "migu:1100", "qq:1094"},
		},
		{
			name: "链式里剩下的另起一组",
			batch: []scoredLyricCandidateResult{
				dtResult("netease", 255, -1.0, dtCommon...),
				dtResult("qq", 258, -0.4, dtCommon...),
				dtResult("migu", 266, 1.2, dtCommon...),
			},
			// migu 跟两家都差 1.6 秒以上;netease 与 qq 差 0.6 秒,按 qq 的分打平。
			want: []string{"migu:1100", "netease:1092", "qq:1092"},
		},
		{
			name:  "只有一条候选",
			batch: []scoredLyricCandidateResult{dtResult("qq", 264, 0, dtCommon...)},
			want:  []string{"qq:1098"},
		},
		{
			name: "判废的与没有 duration 分的不参与",
			batch: []scoredLyricCandidateResult{
				dtResult("qq", 264, 0, dtCommon...),
				{Source: "migu", Lyrics: dtLRC(0.4), Score: -1, ScoreTerms: []scoreTerm{{Kind: scoreRejectNoLastTimestamp, Points: -1}}},
				dtResult("kugou", 0, 0.2, append([]scoreTerm{{Kind: scoreTermDurationOff, Points: -durationMismatchPenalty}}, dtCommon...)...),
			},
			want: []string{"qq:1098", "kugou:334", "migu:-1"},
		},
		{
			name: "吃了时间轴扣分的不并进没吃的那一组",
			batch: []scoredLyricCandidateResult{
				dtResult("qq", 264, 0, dtCommon...),
				dtResult("migu", 266, 0.4, append([]scoreTerm{{Kind: scoreTermTimelineOffset, Points: -timelineOffsetPenalty}}, dtCommon...)...),
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
		dtResult("qq", 264, 0, dtCommon...),
		dtResult("migu", 266, 0.4, dtCommon...),
	}
	applyDurationNearTie(batch, 0)
	if batch[0].Score != 1098 || batch[1].Score != 1100 {
		t.Fatalf("曲长未知不该动分,实际 %d / %d", batch[0].Score, batch[1].Score)
	}
}

// 两条流水线都接上了:同一份正文的 QQ 与咪咕,咪咕整首晚 0.4 秒,冠军按构造顺序是 QQ。
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
			t.Fatalf("末句只差 0.4 秒应打平,QQ %d(%v) 咪咕 %d(%v)", qq.Score, qq.ScoreTerms, migu.Score, migu.ScoreTerms)
		}
		if scored[0].Source != "qq" {
			t.Fatalf("打平后冠军应按构造顺序是 QQ,实际 %s", scored[0].Source)
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
