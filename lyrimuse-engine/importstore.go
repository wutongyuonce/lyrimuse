package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// 导入库:按「优先使用」「只补没有歌词的歌」导入的歌词按缓存 key 另存一份(importedLyricsRecord)。选词时它作为来源
// importLyricsSource 的候选放进候选池(importedLyricsFor),跟各源一起打分,另加 importedLyricsPoints:同等质量的普通
// LRC 比不过它,带逐字这类明显更好的还能换掉它。按「锁定为手动修改」导入的不进这里(条目锁着,选词不碰它)。
// 用户在这首上采用了别的歌词(save_edit / save_plain_text 的来源不是 import)时作废(dropImportedLyricsLocked)。
// 见 11 章决策 150。
//
// 落盘:<configDir>/<clientName>-imported-lyrics/ 下一个 index.json(key → 文件名)加每首一个 <sha1>.json。

// importedLyricsPoints:导入的歌词在打分里另加的分。比逐字时间轴(wordTimingPoints)少:别的源给的带逐字、其余差不多时
// 换成那一份;比同源加权(250)多:跟别家普通 LRC 打平时留着导入的。
const importedLyricsPoints = 300

type importedLyricsRecord struct {
	Key        string   `json:"key"`
	Lyrics     string   `json:"lyrics"`
	Tr         string   `json:"tr,omitempty"`
	TrLang     string   `json:"tr_lang,omitempty"`
	Roma       string   `json:"roma,omitempty"`
	YRC        string   `json:"yrc,omitempty"`
	BG         string   `json:"bg,omitempty"`
	Title      string   `json:"title,omitempty"`
	Artist     string   `json:"artist,omitempty"`
	Album      string   `json:"album,omitempty"`
	Format     string   `json:"format,omitempty"`
	Files      []string `json:"files,omitempty"`
	ImportedAt int64    `json:"imported_at"`
}

type importedLyricsIndexFile struct {
	Keys map[string]string `json:"keys"`
}

// importedLyricsIndex:内存里的 index.json,按文件修改时间重读(常驻进程自己写的时候直接改内存;子命令每次起来读一遍)。
var importedLyricsIndex struct {
	sync.Mutex
	loadedFor string
	mtime     time.Time
	keys      map[string]string
	loose     map[string]string // 宽松 key(去掉时长变体位)→ 原 key
}

func importedLyricsDir() string {
	dir := configDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, clientName+"-imported-lyrics")
}

func importedLyricsFileName(key string) string {
	sum := sha1.Sum([]byte(key))
	return hex.EncodeToString(sum[:]) + ".json"
}

// importedLooseKey:宽松比对用的 key,去掉歌名里的时长变体位(「~durN」)后按 loosenEnrichKey 折平。
func importedLooseKey(key string) string {
	if strings.Count(key, "|") >= 2 {
		artist, title, album := splitEnrichKey(key)
		title = strings.TrimSuffix(title, enrichKeyDurationVariantRe.FindString(title))
		key = artist + "|" + title + "|" + album
	}
	return loosenEnrichKey(key)
}

// loadImportedLyricsIndexLocked:index.json 改过(或换了配置目录)才重读。调用方持 importedLyricsIndex 的锁。
func loadImportedLyricsIndexLocked() {
	dir := importedLyricsDir()
	path := filepath.Join(dir, "index.json")
	info, err := os.Stat(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			noteFileErr("stat", path, err)
		}
		importedLyricsIndex.loadedFor, importedLyricsIndex.mtime = dir, time.Time{}
		importedLyricsIndex.keys, importedLyricsIndex.loose = nil, nil
		return
	}
	if importedLyricsIndex.loadedFor == dir && info.ModTime().Equal(importedLyricsIndex.mtime) && importedLyricsIndex.keys != nil {
		return
	}
	var idx importedLyricsIndexFile
	data, err := os.ReadFile(path)
	if err != nil {
		noteFileErr("read", path, err)
		return
	}
	if err := json.Unmarshal(data, &idx); err != nil {
		noteFileErr("decode", path, err)
		return
	}
	setImportedLyricsIndexLocked(dir, info.ModTime(), idx.Keys)
}

func setImportedLyricsIndexLocked(dir string, mtime time.Time, keys map[string]string) {
	if keys == nil {
		keys = map[string]string{}
	}
	loose := make(map[string]string, len(keys))
	for k := range keys {
		loose[importedLooseKey(k)] = k
	}
	importedLyricsIndex.loadedFor, importedLyricsIndex.mtime = dir, mtime
	importedLyricsIndex.keys, importedLyricsIndex.loose = keys, loose
}

func writeImportedLyricsIndexLocked(dir string) {
	path := filepath.Join(dir, "index.json")
	data, err := json.Marshal(importedLyricsIndexFile{Keys: importedLyricsIndex.keys})
	if err != nil {
		return
	}
	if writeFileAtomic(path, data) != nil {
		return
	}
	if info, err := os.Stat(path); err == nil {
		importedLyricsIndex.mtime = info.ModTime()
	}
}

// saveImportedLyrics 记下(或换掉)这几首的导入歌词。正文一首一个文件,index.json 整批只写一次。
func saveImportedLyrics(recs ...importedLyricsRecord) error {
	dir := importedLyricsDir()
	if dir == "" {
		return errors.New("imported lyrics: no config dir")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	saved := map[string]string{}
	var firstErr error
	for _, rec := range recs {
		if rec.Key == "" {
			continue
		}
		data, err := json.Marshal(rec)
		if err != nil {
			continue
		}
		name := importedLyricsFileName(rec.Key)
		if err := writeFileAtomic(filepath.Join(dir, name), data); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		saved[rec.Key] = name
	}
	if len(saved) == 0 {
		return firstErr
	}
	importedLyricsIndex.Lock()
	defer importedLyricsIndex.Unlock()
	loadImportedLyricsIndexLocked()
	keys := importedLyricsIndex.keys
	if keys == nil {
		keys = map[string]string{}
	}
	for k, name := range saved {
		keys[k] = name
	}
	setImportedLyricsIndexLocked(dir, importedLyricsIndex.mtime, keys)
	writeImportedLyricsIndexLocked(dir)
	return firstErr
}

// dropImportedLyrics 作废这几首的导入歌词;没有的不管。
func dropImportedLyrics(keys ...string) {
	dir := importedLyricsDir()
	if dir == "" || len(keys) == 0 {
		return
	}
	importedLyricsIndex.Lock()
	defer importedLyricsIndex.Unlock()
	loadImportedLyricsIndexLocked()
	changed := false
	for _, k := range keys {
		name, ok := importedLyricsIndex.keys[k]
		if !ok {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			noteFileErr("remove", filepath.Join(dir, name), err)
		}
		delete(importedLyricsIndex.keys, k)
		changed = true
	}
	if changed {
		setImportedLyricsIndexLocked(dir, importedLyricsIndex.mtime, importedLyricsIndex.keys)
		writeImportedLyricsIndexLocked(dir)
	}
}

// importedLyricsRecordFor:这首有没有导入的歌词。先按原 key 找,找不到按宽松 key(繁简、大小写、空格、多歌手分隔符、
// 时长变体位)找。
func importedLyricsRecordFor(key string) (importedLyricsRecord, bool) {
	dir := importedLyricsDir()
	if dir == "" || key == "" {
		return importedLyricsRecord{}, false
	}
	importedLyricsIndex.Lock()
	loadImportedLyricsIndexLocked()
	name, ok := importedLyricsIndex.keys[key]
	if !ok {
		if orig, hit := importedLyricsIndex.loose[importedLooseKey(key)]; hit {
			name, ok = importedLyricsIndex.keys[orig]
		}
	}
	importedLyricsIndex.Unlock()
	if !ok {
		return importedLyricsRecord{}, false
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			noteFileErr("read", filepath.Join(dir, name), err)
		}
		return importedLyricsRecord{}, false
	}
	var rec importedLyricsRecord
	if err := json.Unmarshal(data, &rec); err != nil || strings.TrimSpace(rec.Lyrics) == "" {
		return importedLyricsRecord{}, false
	}
	return rec, true
}

// importedLyricsFor:选词的候选池用。身份按这首本地的歌名歌手专辑,不算「播放器给定」(不吃同源加权)。
func importedLyricsFor(artist, title, album string) (lyricSourceResult, bool) {
	rec, ok := importedLyricsRecordFor(enrichKey(artist, title, album))
	if !ok {
		return lyricSourceResult{}, false
	}
	return lyricSourceResult{
		source: importLyricsSource, lyr: rec.Lyrics, yrc: rec.YRC, tr: rec.Tr, roma: rec.Roma, bg: rec.BG, trLang: rec.TrLang,
		matchTitle: title, matchArtist: artist, matchAlbum: album,
	}, true
}

// importedTrLang:导入的译文标了语言就用它,没标按设置里的译文语言(用户导入的译文多半就是想看的那种)。
func importedTrLang(lang string) string {
	if lang != "" {
		return lang
	}
	return features().LyricsTranslationLanguage
}
