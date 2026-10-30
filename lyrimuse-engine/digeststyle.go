package main

import "strconv"

// 听歌报告推到每个渠道时用哪一套内容(渠道的 digest_style):完整、精简、概况三套预设,各平台照旧按自己的格式排版
// (notifyrich.go);自定义按这个渠道自己的模板出标题正文、按纯文本推(digesttemplate.go)。三套预设各自也写成一份模板
// (digestPresetTemplates),App 编辑器里照它显示、从它改起;精简、概况的纯文本正文就是渲染这份模板。
// 渠道没选过(digest_style 为空)时沿用 features.json 里四档共用的那份模板:自定义过就按它,没有就是完整。
// App 侧同一套规则在 Core `DigestStyle` / `NotificationPreview`,两边对得上靠共用样例 shared/testdata/notify/preview.json。
// 见 15 章决策 90、91。

const (
	digestStyleFull     = "full"
	digestStyleBrief    = "brief"
	digestStyleOverview = "overview"
	digestStyleCustom   = "custom"
)

// digestPresetTemplate:三套预设写成模板的样子(标题都是默认标题),正文按报告语言取(digestWordTable 的 preset_*),
// 同 Core `DigestTemplate.preset`,两边对得上靠共用样例 shared/testdata/notify/presets.json。完整这份渲染出来跟默认正文
// 一样(只有一条的榜单照列、新歌手那句不带「等」两处除外),真推完整时仍用 digestBody。认不得的 style 返回空模板。
func digestPresetTemplate(w digestWords, style string) digestTemplate {
	switch style {
	case digestStyleFull, digestStyleBrief, digestStyleOverview:
		return digestTemplate{Body: w["preset_"+style]}
	}
	return digestTemplate{}
}

// digestStyle:这个渠道这一期按哪一套推;自定义时另返回要用的模板。legacy 是 features.json 里四档共用的那份。
// 选了自定义却没写模板(或写的跟默认一样)按完整推;认不出的值按完整推。
func (ch notifyChannel) digestStyle(legacy digestTemplate) (string, digestTemplate) {
	switch ch.DigestStyle {
	case digestStyleBrief, digestStyleOverview:
		return ch.DigestStyle, digestTemplate{}
	case digestStyleCustom:
		if t := resolveDigestTemplate(ch.DigestTemplate); t.custom() {
			return digestStyleCustom, t
		}
	case "":
		if legacy.custom() {
			return digestStyleCustom, legacy
		}
	}
	return digestStyleFull, digestTemplate{}
}

// forChannel:听歌报告(和「测一下」推的那份)按这个渠道选的那一套改写标题正文。进来的是默认标题和完整正文;不带报告的原样返回。
func (m notifyMessage) forChannel(ch notifyChannel) notifyMessage {
	if m.Digest == nil {
		return m
	}
	style, tpl := ch.digestStyle(features().DigestTemplate)
	switch style {
	case digestStyleCustom:
		m.Title, m.Body = renderDigestTemplate(tpl, m.Title, m.Body, *m.Digest, digestTopN())
		m.Custom = true
	case digestStyleBrief, digestStyleOverview:
		m.Body = digestBodyStyled(*m.Digest, style)
		m.Style = style
	}
	return m
}

// digestBodyStyled:纯文本正文。精简、概况渲染各自的预设模板(这两套不空行,Bark 锁屏只显示前几行);其余按完整(digestBody)。
func digestBodyStyled(s digestStats, style string) string {
	body := digestBody(s)
	switch style {
	case digestStyleBrief, digestStyleOverview:
		_, body = renderDigestTemplate(digestPresetTemplate(dw(), style), "", body, s, digestTopN())
	}
	return body
}

// digestTopOnes:歌手、专辑、歌曲各自的第一名,一类一行;某类没有就不写。sub 拼专辑、歌曲的那一段(带歌手),esc 是平台转义。
func digestTopOnes(s digestStats, line func(label, name string, count int) string, sub func(digestTally) string, esc func(string) string) []string {
	w := dw()
	var out []string
	if len(s.TopArtists) > 0 {
		out = append(out, line(w["label_artist"], esc(s.TopArtists[0].Name), s.TopArtists[0].Count))
	}
	for _, c := range []struct {
		label string
		list  []digestTally
	}{{w["label_album"], s.TopAlbums}, {w["label_track"], s.TopTracks}} {
		if len(c.list) == 0 {
			continue
		}
		t := c.list[0]
		t.Name, t.Sub = esc(t.Name), esc(t.Sub)
		out = append(out, line(c.label, sub(t), t.Count))
	}
	return out
}

// digestNewArtistsCount:「新听了 5 位歌手」,不列名字;一位都没有返回空。
func digestNewArtistsCount(s digestStats) string {
	if s.NewArtists <= 0 {
		return ""
	}
	return dw().count("new_artists", s.NewArtists)
}

// richDigest 是按平台排版时用的那份数,带着这个渠道选的那一套。
type richDigest struct {
	*digestStats
	style string
}

// sections:完整列三类榜单;精简只有一段「最常听」,各类第一名一行(「歌手：方大同 · 61」「专辑：JTW 西游记 — 方大同 · 29」);
// 概况不列。
func (d *richDigest) sections(esc func(string) string) []richSection {
	switch d.style {
	case digestStyleBrief:
		w := dw()
		lines := digestTopOnes(*d.digestStats, func(label, name string, count int) string {
			return w.f("brief_line", "label", label, "name", name, "n", strconv.Itoa(count))
		}, func(t digestTally) string { return t.Name + " — " + t.Sub }, esc)
		if len(lines) == 0 {
			return nil
		}
		return []richSection{{heading: w["most_played"], title: w["most_played"], lines: lines}}
	case digestStyleOverview:
		return nil
	}
	return richSections(d.digestStats, esc)
}

// newLine:新听的歌手那一句。概况只写几位。
func (d *richDigest) newLine() string {
	if d.style == digestStyleOverview {
		return digestNewArtistsCount(*d.digestStats)
	}
	return digestNewArtistsLine(*d.digestStats)
}
