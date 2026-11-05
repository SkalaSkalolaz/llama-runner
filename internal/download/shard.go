package download

import (
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
)

// This module owns one responsibility: recognizing the Hugging Face
// multi-file (sharded) GGUF naming scheme and reconstructing every shard's
// file name and URL from one of them. llama.cpp takes the first shard
// (-00001-of-NNNNN) as the entry point and loads the remaining files itself,
// so downloading any single shard URL must expand into the full shard set.

// shardNameRe matches sharded GGUF names such as
//
//	DeepSeek-...-IQ1_M-00001-of-00004.gguf  →  base, 1, 4, .gguf
//	foo-1-of-4.gguf                          →  foo, 1, 4, .gguf
//
// The match is case-insensitive (.GGUF counts), and the leading group is
// greedy, so in "a-b-00001-of-00002.gguf" the base is "a-b" — the LAST
// "-<digits>-of-<digits>" run before the extension is the shard counter.
var shardNameRe = regexp.MustCompile(`(?i)^(.+)-(\d+)-of-(\d+)(\.gguf)$`)

// ShardPattern describes one sharded GGUF file name: the shared name prefix,
// the index of that file and the total shard count, plus the zero-pad widths
// captured from the name itself (HF uses 5 digits, "00001-of-00004", but
// shorter widths occur and must round-trip exactly).
type ShardPattern struct {
	Base       string // name without the -NNNNN-of-MMMMM suffix
	Cur        int    // 1-based index of the file the name belongs to
	Total      int    // number of shards in the set
	CurWidth   int    // digit width of the index field in the original name
	TotalWidth int    // digit width of the total field in the original name
	Ext        string // file extension as written (case preserved)
}

// MultiFile reports whether the pattern describes an actual multi-file
// model. A name matching the scheme with total == 1 (foo-00001-of-00001.gguf)
// is a single file and needs no shard expansion.
func (s ShardPattern) MultiFile() bool { return s.Total > 1 }

// Name returns the file name of the 1-based shard i in this set, padded with
// the widths captured from the original name.
func (s ShardPattern) Name(i int) string {
	return ShardName(s.Base, i, s.CurWidth, s.Total, s.TotalWidth, s.Ext)
}

// DetectShards parses a shard file name (only its base component is used)
// and reports whether it follows the -<cur>-of-<total>.gguf scheme. Names
// whose counters are nonsensical (index outside 1..total) are deliberately
// NOT treated as shards: treating a corrupt listing as a shard set would
// make the loader fetch a long list of 404s instead of the one real file.
func DetectShards(fileName string) (ShardPattern, bool) {
	m := shardNameRe.FindStringSubmatch(filepath.Base(fileName))
	if m == nil {
		return ShardPattern{}, false
	}
	cur, errC := strconv.Atoi(m[2])
	total, errT := strconv.Atoi(m[3])
	if errC != nil || errT != nil || cur < 1 || total < 1 || cur > total {
		return ShardPattern{}, false
	}
	return ShardPattern{
		Base:       m[1],
		Cur:        cur,
		Total:      total,
		CurWidth:   len(m[2]),
		TotalWidth: len(m[3]),
		Ext:        m[4],
	}, true
}

// ShardName builds the name of shard i (1-based) of a set: base, the index
// zero-padded to curWidth, "-of-", the total zero-padded to totalWidth, and
// the extension — the exact inverse of DetectShards. Each counter keeps its
// own width, so "foo-1-of-00004" stays short-long rather than being
// normalized to one width.
func ShardName(base string, i, curWidth, total, totalWidth int, ext string) string {
	return fmt.Sprintf("%s-%0*d-of-%0*d%s", base, curWidth, i, totalWidth, total, ext)
}

// ShardURL rewrites origURL to point at a different file name in the same
// directory of the same repository revision: only the last path segment is
// replaced, the scheme, host and any query parameters are kept. Clearing
// RawPath forces url.String to re-escape the rewritten path from its decoded
// form, which keeps names with spaces or other special characters valid.
func ShardURL(origURL, newName string) (string, error) {
	u, err := url.Parse(origURL)
	if err != nil {
		return "", err
	}
	dir, _ := path.Split(u.Path)
	u.Path = dir + newName
	u.RawPath = ""
	return u.String(), nil
}
