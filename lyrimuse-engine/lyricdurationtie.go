package main

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// 末句只差 1 秒以内时 duration 分项不决胜负(applyDurationNearTie)。判据与全库回放见 09 章决策 349。
//
// duration 分项按末句离曲长的距离连续计分,内容相同的几家其余分项完全一样时,胜负就落在末句那零点几秒上,
// 等于让时间轴整体最晚的那份赢。这一步把这种差距抹平,同分后的先后交给稳定排序 + 候选构造顺序(lyricCandidateTieOrder)。
//
// 分组规则:
//   - 只看分项除 duration 外逐项相同(同种同分,不计顺序)、都拿到 duration 正分、未被判废的候选;
//   - 同一组分项里,取 duration 分最高的那份作领头(并列取构造顺序靠前的),末句跟领头相差不超过
//     durationTieWindowSecs 的并进它这一组,duration 分抬到领头的分;剩下的再按同样做法另起一组。
//     只跟领头比、不按两两相邻串起来:A、B、C 依次差 0.6 秒时 C 跟 A 差 1.2 秒,不进 A 那一组。
//   - 只往上抬到领头的分、领头自己不动:原来压过领头的组外候选照样领先。冠军换人只有两种:换成同组里构造顺序靠前的,
//     或者原冠军跟某组领头本来同分、那一组里构造顺序在原冠军前面的一份抬平后上位。
//
// 时间轴整体错开的几项批级扣分(timelineOffset / timelineIntrusion / sourceDurationNearMiss)排在它前面,
// 吃了扣分的候选分项就不同、不会并进没吃的那一组。两条流水线都要调,排在 applyWordTimingTitleOverride 与排序之前。
const durationTieWindowSecs = 1.0

// durationTieSignature:除 duration 外的分项,按种类和分值排好序拼成一串,相同即「其余分项完全一样」。
func durationTieSignature(terms []scoreTerm) string {
	parts := make([]string, 0, len(terms))
	for _, t := range terms {
		if t.Kind == scoreTermDuration {
			continue
		}
		parts = append(parts, t.Kind+"="+strconv.Itoa(t.Points))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func applyDurationNearTie(results []scoredLyricCandidateResult, durationSecs float64) {
	if durationSecs <= 0 {
		return
	}
	type member struct {
		idx  int
		last float64
		pts  int
	}
	groups := map[string][]member{}
	var order []string
	for i, r := range results {
		if r.Score < 0 || r.Lyrics == "" {
			continue
		}
		pts := scoreTermPoints(r.ScoreTerms, scoreTermDuration)
		if pts <= 0 {
			continue
		}
		last, ok := lastLRCTimestampSecs(r.Lyrics)
		if !ok {
			continue
		}
		sig := durationTieSignature(r.ScoreTerms)
		if _, seen := groups[sig]; !seen {
			order = append(order, sig)
		}
		groups[sig] = append(groups[sig], member{i, last, pts})
	}
	for _, sig := range order {
		rest := groups[sig]
		for len(rest) > 1 {
			lead := 0
			for k := range rest {
				if rest[k].pts > rest[lead].pts {
					lead = k
				}
			}
			var left []member
			for k, m := range rest {
				if k == lead {
					continue
				}
				if math.Abs(m.last-rest[lead].last) > durationTieWindowSecs {
					left = append(left, m)
					continue
				}
				raiseDurationTerm(&results[m.idx], rest[lead].pts)
			}
			rest = left
		}
	}
}

// raiseDurationTerm 把 duration 分项改成 pts,总分按分项重算(夹底同 scoreLyricCandidateDetailed)。
func raiseDurationTerm(r *scoredLyricCandidateResult, pts int) {
	terms := append([]scoreTerm(nil), r.ScoreTerms...)
	sum := 0
	for k := range terms {
		if terms[k].Kind == scoreTermDuration {
			terms[k].Points = pts
		}
		sum += terms[k].Points
	}
	r.ScoreTerms = terms
	r.Score = max(sum, 1)
}
