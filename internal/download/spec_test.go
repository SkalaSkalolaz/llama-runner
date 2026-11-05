package download

import (
	"path/filepath"
	"strings"
	"testing"
)

const testHome = "/home/tester"

func TestParseSpec_Valid(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	tests := []struct {
		name    string
		value   string
		wantDir string
		wantURL string
	}{
		{
			name:    "tilde dir with https URL",
			value:   "~/GGUF/https://huggingface.co/org/repo/resolve/main/model.gguf",
			wantDir: "/home/tester/GGUF/",
			wantURL: "https://huggingface.co/org/repo/resolve/main/model.gguf",
		},
		{
			name:    "absolute dir with trailing slash",
			value:   "/data/models/https://huggingface.co/a/b/resolve/main/m-00001-of-00004.gguf",
			wantDir: "/data/models/",
			wantURL: "https://huggingface.co/a/b/resolve/main/m-00001-of-00004.gguf",
		},
		{
			name:    "dir without trailing slash gets one",
			value:   "~/modelshttps://example.com/model.gguf",
			wantDir: "/home/tester/models/",
			wantURL: "https://example.com/model.gguf",
		},
		{
			name:    "http scheme accepted",
			value:   "~/m/http://example.com/model.gguf",
			wantDir: "/home/tester/m/",
			wantURL: "http://example.com/model.gguf",
		},
		{
			name:    "uppercase scheme detected",
			value:   "~/m/HTTPS://example.com/Model.GGUF",
			wantDir: "/home/tester/m/",
			wantURL: "HTTPS://example.com/Model.GGUF",
		},
		{
			name:    "query parameters preserved verbatim",
			value:   "~/m/https://example.com/model.gguf?download=true&token=abc",
			wantDir: "/home/tester/m/",
			wantURL: "https://example.com/model.gguf?download=true&token=abc",
		},
		{
			name:    "root dir stays root",
			value:   "/https://example.com/model.gguf",
			wantDir: "/",
			wantURL: "https://example.com/model.gguf",
		},
		{
			name:    "tilde-only dir",
			value:   "~/https://example.com/model.gguf",
			wantDir: "/home/tester/",
			wantURL: "https://example.com/model.gguf",
		},
		{
			name:    "nested repo subdirectory in URL",
			value:   "~/GGUF/https://huggingface.co/org/repo/resolve/main/sub/dir/model.gguf",
			wantDir: "/home/tester/GGUF/",
			wantURL: "https://huggingface.co/org/repo/resolve/main/sub/dir/model.gguf",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, urlStr, err := ParseSpec(tt.value, testHome)
			if err != nil {
				t.Fatalf("ParseSpec(%q) unexpected error: %v", tt.value, err)
			}
			if dir != tt.wantDir {
				t.Errorf("dir = %q, want %q", dir, tt.wantDir)
			}
			if urlStr != tt.wantURL {
				t.Errorf("url = %q, want %q", urlStr, tt.wantURL)
			}
			if !filepath.IsAbs(dir) {
				t.Errorf("dir %q is not absolute", dir)
			}
		})
	}
}

func TestParseSpec_RelativeDirMadeAbsolute(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	dir, urlStr, err := ParseSpec("./models/https://example.com/model.gguf", testHome)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("dir %q is not absolute", dir)
	}
	if filepath.Base(dir) != "models" {
		t.Errorf("dir = %q, want a path ending in models/", dir)
	}
	if !strings.HasSuffix(dir, string(filepath.Separator)) {
		t.Errorf("dir %q must end with a separator", dir)
	}
	if urlStr != "https://example.com/model.gguf" {
		t.Errorf("url = %q", urlStr)
	}
}

func TestParseSpec_Errors(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	tests := []struct {
		name    string
		value   string
		wantSub string
	}{
		{
			name:    "no URL — trailing dir only",
			value:   "~/GGUF/",
			wantSub: "no http(s) URL found",
		},
		{
			name:    "plain file path is not a URL",
			value:   "~/GGUF/model.gguf",
			wantSub: "no http(s) URL found",
		},
		{
			name:    "URL without dir prefix",
			value:   "https://huggingface.co/org/repo/resolve/main/model.gguf",
			wantSub: "no target directory",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ParseSpec(tt.value, testHome)
			if err == nil {
				t.Fatalf("ParseSpec(%q) must fail", tt.value)
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("err = %q, want it to contain %q", err.Error(), tt.wantSub)
			}
		})
	}
}

func TestJoinDir(t *testing.T) {
	sep := string(filepath.Separator)
	if got := JoinDir("/data/models"+sep, "m.gguf"); got != "/data/models"+sep+"m.gguf" {
		t.Errorf("JoinDir with trailing sep = %q", got)
	}
	if got := JoinDir("/data/models", "m.gguf"); got != filepath.Join("/data/models", "m.gguf") {
		t.Errorf("JoinDir without trailing sep = %q", got)
	}
	if got := JoinDir("", "m.gguf"); got != "m.gguf" {
		t.Errorf("JoinDir empty dir = %q", got)
	}
}
