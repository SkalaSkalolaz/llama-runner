package download

import (
	"testing"
)

func TestDetectShards(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		wantOK   bool
		base     string
		cur      int
		total    int
		curW     int
		totalW   int
		ext      string
	}{
		{
			name: "typical HF five-digit shard", fileName: "DeepSeek-V4.1-IQ1_M-00001-of-00004.gguf",
			wantOK: true, base: "DeepSeek-V4.1-IQ1_M", cur: 1, total: 4, curW: 5, totalW: 5, ext: ".gguf",
		},
		{
			name: "quant tag in base", fileName: "model-Q4_K_M-00002-of-00003.gguf",
			wantOK: true, base: "model-Q4_K_M", cur: 2, total: 3, curW: 5, totalW: 5, ext: ".gguf",
		},
		{
			name: "short widths", fileName: "foo-1-of-4.gguf",
			wantOK: true, base: "foo", cur: 1, total: 4, curW: 1, totalW: 1, ext: ".gguf",
		},
		{
			name: "single file matching the scheme", fileName: "foo-00001-of-00001.gguf",
			wantOK: true, base: "foo", cur: 1, total: 1, curW: 5, totalW: 5, ext: ".gguf",
		},
		{
			name: "cur equals total", fileName: "model-00002-of-00002.gguf",
			wantOK: true, base: "model", cur: 2, total: 2, curW: 5, totalW: 5, ext: ".gguf",
		},
		{
			name: "uppercase extension", fileName: "model-00001-of-00004.GGUF",
			wantOK: true, base: "model", cur: 1, total: 4, curW: 5, totalW: 5, ext: ".GGUF",
		},
		{
			name: "dashes in base — last counter run wins", fileName: "a-b-00001-of-00002.gguf",
			wantOK: true, base: "a-b", cur: 1, total: 2, curW: 5, totalW: 5, ext: ".gguf",
		},
		{name: "plain file", fileName: "single.gguf", wantOK: false},
		{name: "no extension", fileName: "model-00001-of-00004", wantOK: false},
		{name: "index beyond total", fileName: "model-00005-of-00004.gguf", wantOK: false},
		{name: "zero index", fileName: "model-00000-of-00004.gguf", wantOK: false},
		{name: "zero total", fileName: "model-00001-of-00000.gguf", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sp, ok := DetectShards(tt.fileName)
			if ok != tt.wantOK {
				t.Fatalf("DetectShards(%q) ok = %v, want %v", tt.fileName, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if sp.Base != tt.base || sp.Cur != tt.cur || sp.Total != tt.total ||
				sp.CurWidth != tt.curW || sp.TotalWidth != tt.totalW || sp.Ext != tt.ext {
				t.Errorf("DetectShards(%q) = %+v, want base=%q cur=%d total=%d curW=%d totalW=%d ext=%q",
					tt.fileName, sp, tt.base, tt.cur, tt.total, tt.curW, tt.totalW, tt.ext)
			}
		})
	}
}

func TestDetectShards_UsesBaseComponent(t *testing.T) {
	sp, ok := DetectShards("/some/dir/model-00002-of-00003.gguf")
	if !ok || sp.Base != "model" || sp.Cur != 2 {
		t.Errorf("path input not handled: %+v ok=%v", sp, ok)
	}
}

func TestShardPattern_MultiFile(t *testing.T) {
	if sp, _ := DetectShards("foo-00001-of-00001.gguf"); sp.MultiFile() {
		t.Error("1-of-1 must not be treated as multi-file")
	}
	if sp, _ := DetectShards("foo-00001-of-00002.gguf"); !sp.MultiFile() {
		t.Error("1-of-2 must be treated as multi-file")
	}
}

func TestShardName(t *testing.T) {
	tests := []struct {
		name          string
		base          string
		i, curW       int
		total, totalW int
		ext           string
		want          string
	}{
		{name: "width 1 round trip", base: "foo", i: 1, curW: 1, total: 4, totalW: 1, ext: ".gguf", want: "foo-1-of-4.gguf"},
		{name: "width 5 round trip", base: "foo", i: 1, curW: 5, total: 4, totalW: 5, ext: ".gguf", want: "foo-00001-of-00004.gguf"},
		{name: "each counter keeps its own width", base: "foo", i: 2, curW: 1, total: 4, totalW: 5, ext: ".gguf", want: "foo-2-of-00004.gguf"},
		{name: "uppercase ext preserved", base: "Model", i: 3, curW: 5, total: 7, totalW: 5, ext: ".GGUF", want: "Model-00003-of-00007.GGUF"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShardName(tt.base, tt.i, tt.curW, tt.total, tt.totalW, tt.ext); got != tt.want {
				t.Errorf("ShardName = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShardPattern_NameRoundTrips(t *testing.T) {
	sp, ok := DetectShards("DeepSeek-IQ1_M-00003-of-00004.gguf")
	if !ok {
		t.Fatal("pattern not detected")
	}
	if got := sp.Name(3); got != "DeepSeek-IQ1_M-00003-of-00004.gguf" {
		t.Errorf("Name(3) = %q, want the original name back", got)
	}
	if got := sp.Name(1); got != "DeepSeek-IQ1_M-00001-of-00004.gguf" {
		t.Errorf("Name(1) = %q", got)
	}
}

func TestShardURL(t *testing.T) {
	tests := []struct {
		name    string
		origURL string
		newName string
		want    string
	}{
		{
			name:    "replace last segment",
			origURL: "https://huggingface.co/org/repo/resolve/main/model-00001-of-00004.gguf",
			newName: "model-00002-of-00004.gguf",
			want:    "https://huggingface.co/org/repo/resolve/main/model-00002-of-00004.gguf",
		},
		{
			name:    "subdirectory preserved",
			origURL: "https://huggingface.co/org/repo/resolve/main/sub/model-00001-of-00002.gguf",
			newName: "sub/model-nope.gguf", // only the name replaces; dir part of newName is not special
			want:    "https://huggingface.co/org/repo/resolve/main/sub/sub/model-nope.gguf",
		},
		{
			name:    "query preserved",
			origURL: "https://example.com/a/b/model.gguf?download=true",
			newName: "model-2.gguf",
			want:    "https://example.com/a/b/model-2.gguf?download=true",
		},
		{
			name:    "name with space re-escaped",
			origURL: "https://example.com/a/b/model%20one.gguf",
			newName: "model two.gguf",
			want:    "https://example.com/a/b/model%20two.gguf",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ShardURL(tt.origURL, tt.newName)
			if err != nil {
				t.Fatalf("ShardURL error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ShardURL = %q, want %q", got, tt.want)
			}
		})
	}
}
