package main

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// 内容相同、整首时间轴只差 1 秒以内的几家,按时间轴挑冠军(applyDurationNearTie)。判据与回放见 09 章决策 349、352。
//
// duration 分项按末句离曲长的距离连续计分,lines 分项按行数计分;内容相同的几家其余分项完全一样时,胜负就落在
// 末句那零点几秒、各家分行多一两行上,跟哪份时间轴准无关。这一步把同一组的这两项抹平,再按整首时间轴挑一份让它赢。
//
// 分组:
//   - 候选:未被判废、拿到 duration 正分、正文能提出末句和显示轴(displayedTimeline)。
//   - 分项除 duration、lines 外逐项相同(同种同分,不计顺序)的放一桶。桶里取当前分数最高的作领头(并列取构造顺序
//     靠前的),lines 分跟领头差不超过 durationTieLinesSpread、整首显示轴跟领头对齐在 durationTieWindowMs 以内
//     (durationTieOffsetMs),并且结尾对得上的并进它这一组;只跟领头比、不串链,剩下的再按同样做法另起一组。
//   - 结尾对得上:末句(lastLRCTimestampSecs,跟打分同一口径)相差不超过 durationTieWindowMs,或者两份 LRC 唱到
//     同一句结束(durationTieSameEnding)。整首对齐、结尾却不是同一句、末句又差出几秒的,是结尾少了或多了几行,
//     duration 分在这里量的是正文全不全,不能抹平。
//
// 挑目标:中位差不超过 durationTieSameVoiceMs 的是同一份时间轴(同源转出来的副本),并成一个声部、只算一票
// (见 durationTieVoices)。
//   - 三个声部以上:挑到其余各声部中位差绝对值之和最小的那个(居中)。
//   - 两个声部:中位差不小于 durationTieEarlierMs 时挑时间轴早的那个,否则按构造顺序。
//   - 一个声部:不动。时间轴是同一份,谁赢显示时刻都一样,行数这些差别照旧由分项决定。
//   - 并列、或选中的声部里有几家时,按构造顺序取第一个。
//
// 让目标赢:组内各家的 duration、lines 分都抬到组内最大值(只往上抬),总分按分项重算,于是同组同分;再把目标在
// results 里换到组内最靠前那一家的位置,交给后面的稳定排序。不给目标额外加分。
//
// 时间轴整体错开的几项批级扣分(timelineOffset / timelineIntrusion / sourceDurationNearMiss)排在它前面,
// 吃了扣分的候选分项就不同、不会并进没吃的那一组。两条流水线都要调,排在 applyWordTimingTitleOverride 与排序之前。
const (
	durationTieWindowMs    = 1000
	durationTieLinesSpread = 3
	durationTieSameVoiceMs = 50
	durationTieEarlierMs   = 100
)

// durationTieSignature:除 duration、lines 外的分项,按种类和分值排好序拼成一串,相同即「其余分项完全一样」。
func durationTieSignature(terms []scoreTerm) string {
	parts := make([]string, 0, len(terms))
	for _, t := range terms {
		if t.Kind == scoreTermDuration || t.Kind == scoreTermLines {
			continue
		}
		parts = append(parts, t.Kind+"="+strconv.Itoa(t.Points))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// durationTieOffsetMs:a 整首显示轴相对 b 的中位差(毫秒,正数是 a 晚)。配对不够(见 timelinePairOffsets)、
// 中位差超过 durationTieWindowMs,或落在中位差 ±durationTieWindowMs 内的配对行不到 timelineMinShare 时 ok=false。
func durationTieOffsetMs(a, b []timelineLine) (med int, ok bool) {
	d, med, ok := timelinePairOffsets(a, b)
	if !ok || med > durationTieWindowMs || -med > durationTieWindowMs {
		return 0, false
	}
	near := 0
	for _, x := range d {
		if x-med <= durationTieWindowMs && med-x <= durationTieWindowMs {
			near++
		}
	}
	return med, float64(near) >= timelineMinShare*float64(len(d))
}

// durationTieSameEnding:两份 LRC 唱到同一句结束。按正文 LCS 配对(timelineLinesMatch)后,最后配上的那一对往后
// 各自的正文接起来相同(各家把最后一句拆成两行、或并成一行都认),并且满足其一:
//   - 两份每一行都配上了:正文一行不差,末句差只是这几行的时刻不同;
//   - 这一对的时差跟全部配对行的中位差相差不超过 durationTieWindowMs。
//
// 结尾少了几行的,最后配上的那一对后面正文不同;结尾是重复的副歌时,少了的那几句会被配到对方末行上,这时两份
// 行数对不上、这一对的时差又比中位差差出好几秒。配对不足 timelineOffsetMinMatched 行的不算。
func durationTieSameEnding(a, b []timelineLine) bool {
	var d []int
	li, lj := -1, -1
	for i, j := range timelineLCSAlignFunc(len(a), len(b), func(i, j int) bool { return timelineLinesMatch(a[i].norm, b[j].norm) }) {
		if j >= 0 {
			d = append(d, a[i].ms-b[j].ms)
			li, lj = i, j
		}
	}
	if len(d) < timelineOffsetMinMatched {
		return false
	}
	tail := func(ls []timelineLine, from int) string {
		var sb strings.Builder
		for _, l := range ls[from:] {
			sb.WriteString(l.norm)
		}
		return sb.String()
	}
	if tail(a, li) != tail(b, lj) {
		return false
	}
	if len(d) == len(a) && len(d) == len(b) {
		return true
	}
	x := a[li].ms - b[lj].ms
	sort.Ints(d)
	x -= d[len(d)/2]
	return x <= durationTieWindowMs && -x <= durationTieWindowMs
}

func applyDurationNearTie(results []scoredLyricCandidateResult, durationSecs float64) {
	if durationSecs <= 0 {
		return
	}
	lines := make([][]timelineLine, len(results))
	for _, g := range durationTieGroups(results, lines) {
		voices, pairMs := durationTieVoices(lines, g)
		if len(voices) < 2 {
			continue
		}
		target := durationTieTarget(voices, pairMs)
		maxDuration, maxLines := 0, 0
		for _, i := range g.members {
			maxDuration = max(maxDuration, scoreTermPoints(results[i].ScoreTerms, scoreTermDuration))
			maxLines = max(maxLines, scoreTermPoints(results[i].ScoreTerms, scoreTermLines))
		}
		for _, i := range g.members {
			raiseDurationTieTerms(&results[i], maxDuration, maxLines)
		}
		first := g.members[0]
		results[first], results[target] = results[target], results[first]
	}
}

// durationTieGroup:一组候选在 results 里的下标(升序)和各家相对领头的中位差(毫秒,正数是比领头晚)。
type durationTieGroup struct {
	members []int
	offset  map[int]int
}

// durationTieGroups 按文件头的规则分组,只返回两家以上的组;顺带把参与分组的候选显示轴填进 lines。
func durationTieGroups(results []scoredLyricCandidateResult, lines [][]timelineLine) []durationTieGroup {
	buckets := map[string][]int{}
	var order []string
	last := make([]float64, len(results))
	lrcLines := make([][]timelineLine, len(results))
	for i, r := range results {
		if r.Score < 0 || r.Lyrics == "" || scoreTermPoints(r.ScoreTerms, scoreTermDuration) <= 0 {
			continue
		}
		var ok bool
		if last[i], ok = lastLRCTimestampSecs(r.Lyrics); !ok {
			continue
		}
		if lines[i] = displayedTimeline(r.Lyrics, r.LyricsYRC); len(lines[i]) == 0 {
			continue
		}
		lrcLines[i] = timelineLines(r.Lyrics, 0, lyricSpeakerLabels(r.Lyrics))
		sig := durationTieSignature(r.ScoreTerms)
		if _, seen := buckets[sig]; !seen {
			order = append(order, sig)
		}
		buckets[sig] = append(buckets[sig], i)
	}
	var groups []durationTieGroup
	for _, sig := range order {
		rest := buckets[sig]
		for len(rest) > 1 {
			lead := rest[0]
			for _, i := range rest {
				if results[i].Score > results[lead].Score {
					lead = i
				}
			}
			g := durationTieGroup{members: []int{lead}, offset: map[int]int{lead: 0}}
			var left []int
			for _, i := range rest {
				if i == lead {
					continue
				}
				spread := scoreTermPoints(results[i].ScoreTerms, scoreTermLines) - scoreTermPoints(results[lead].ScoreTerms, scoreTermLines)
				med, ok := durationTieOffsetMs(lines[i], lines[lead])
				if !ok || spread > durationTieLinesSpread || -spread > durationTieLinesSpread ||
					math.Abs(last[i]-last[lead])*1000 > durationTieWindowMs && !durationTieSameEnding(lrcLines[i], lrcLines[lead]) {
					left = append(left, i)
					continue
				}
				g.members = append(g.members, i)
				g.offset[i] = med
			}
			rest = left
			if len(g.members) > 1 {
				sort.Ints(g.members)
				groups = append(groups, g)
			}
		}
	}
	return groups
}

// durationTieVoices 按构造顺序把组内各家分进声部:跟某个声部的第一家中位差不超过 durationTieSameVoiceMs 的并进
// 最先满足的那个声部,否则自成一个声部。只跟声部第一家比、不串链。返回各声部(声部内、声部之间都按构造顺序)和
// 两家之间的中位差(a 相对 b,正数是 a 晚):每对只配一次,两个方向正负严格相反;配不出中位差时用两家相对领头
// 之差代替。
func durationTieVoices(lines [][]timelineLine, g durationTieGroup) (voices [][]int, pairMs func(a, b int) int) {
	memo := map[[2]int]int{}
	pairMs = func(a, b int) int {
		if a == b {
			return 0
		}
		if a > b {
			return -pairMs(b, a)
		}
		if d, ok := memo[[2]int{a, b}]; ok {
			return d
		}
		d, ok := durationTieOffsetMs(lines[a], lines[b])
		if !ok {
			d = g.offset[a] - g.offset[b]
		}
		memo[[2]int{a, b}] = d
		return d
	}
	for _, i := range g.members {
		joined := false
		for k, v := range voices {
			if d := pairMs(i, v[0]); d <= durationTieSameVoiceMs && -d <= durationTieSameVoiceMs {
				voices[k] = append(v, i)
				joined = true
				break
			}
		}
		if !joined {
			voices = append(voices, []int{i})
		}
	}
	return voices, pairMs
}

// durationTieTarget 按文件头的规则挑声部(至少两个),返回该声部按构造顺序的第一家。声部之间的中位差按各自第一家算。
func durationTieTarget(voices [][]int, pairMs func(a, b int) int) int {
	switch {
	case len(voices) == 2:
		if pairMs(voices[0][0], voices[1][0]) >= durationTieEarlierMs {
			return voices[1][0]
		}
	case len(voices) >= 3:
		best, target := -1, voices[0][0]
		for _, v := range voices {
			sum := 0
			for _, w := range voices {
				if d := pairMs(v[0], w[0]); d >= 0 {
					sum += d
				} else {
					sum -= d
				}
			}
			if best < 0 || sum < best {
				best, target = sum, v[0]
			}
		}
		return target
	}
	return voices[0][0]
}

// raiseDurationTieTerms 把 duration、lines 分项抬到给定分值(只往上抬;原本没有 lines 项的补上),总分按分项重算
// (夹底同 scoreLyricCandidateDetailed)。
func raiseDurationTieTerms(r *scoredLyricCandidateResult, duration, lineCount int) {
	terms := append([]scoreTerm(nil), r.ScoreTerms...)
	hasLines := false
	for k := range terms {
		switch terms[k].Kind {
		case scoreTermDuration:
			terms[k].Points = max(terms[k].Points, duration)
		case scoreTermLines:
			terms[k].Points = max(terms[k].Points, lineCount)
			hasLines = true
		}
	}
	if !hasLines && lineCount > 0 {
		terms = append(terms, scoreTerm{Kind: scoreTermLines, Points: lineCount})
	}
	sum := 0
	for _, t := range terms {
		sum += t.Points
	}
	r.ScoreTerms = terms
	r.Score = max(sum, 1)
}
