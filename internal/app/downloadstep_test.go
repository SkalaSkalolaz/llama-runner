package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

// downloadTestServer is a minimal file server for -download integration
// tests: serves the given paths, counts every request.
type downloadTestServer struct {
	mu     sync.Mutex
	hits   int
	files  map[string][]byte
	server *httptest.Server
}

func newDownloadTestServer(t *testing.T, files map[string][]byte) *downloadTestServer {
	ts := &downloadTestServer{files: files}
	ts.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.mu.Lock()
		ts.hits++
		data, ok := ts.files[r.URL.Path]
		ts.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write(data)
	}))
	t.Cleanup(ts.server.Close)
	return ts
}

func (ts *downloadTestServer) requestCount() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.hits
}

// -version must print the version and exit before the download pre-step
// can start any transfer.
func TestRun_DownloadWithVersion_SkipsDownload(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ts := newDownloadTestServer(t, map[string][]byte{"/model.gguf": []byte("data")})
	dir := t.TempDir() + string(filepath.Separator)

	var out, errb bytes.Buffer
	code := Run([]string{"-download", dir + ts.server.URL + "/model.gguf", "-version"}, &out, &errb, fakeRunner{}, testVersion)
	if code != 0 {
		t.Errorf("code = %d, want 0 (stderr=%q)", code, errb.String())
	}
	if strings.TrimSpace(out.String()) != testVersion {
		t.Errorf("out = %q, want the version", out.String())
	}
	if ts.requestCount() != 0 {
		t.Errorf("download must not start with -version, server hits = %d", ts.requestCount())
	}
}

func TestRun_DownloadWithUpdate_ExitsTwo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LC_ALL", "C")
	dir := t.TempDir() + string(filepath.Separator)
	var out, errb bytes.Buffer
	code := Run([]string{"-download", dir + "https://example.com/model.gguf", "-update"}, &out, &errb, fakeRunner{}, testVersion)
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "not compatible") {
		t.Errorf("stderr = %q, want the conflict message", errb.String())
	}
}

func TestRun_DownloadBadSpec_ExitsTwo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LC_ALL", "C")
	var out, errb bytes.Buffer
	code := Run([]string{"-download", "~/GGUF/"}, &out, &errb, fakeRunner{}, testVersion)
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "no http(s) URL found") {
		t.Errorf("stderr = %q", errb.String())
	}
}

// A successful download lets the launch continue; with no llama-server
// binary in the sandboxed HOME the run ends at path validation (exit 1),
// which also proves the downloaded file passed the model check.
func TestRun_DownloadThenLaunch_ModelCheckPasses(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LC_ALL", "C")
	payload := []byte("model-bytes")
	ts := newDownloadTestServer(t, map[string][]byte{"/model.gguf": payload})
	dir := t.TempDir()

	var out, errb bytes.Buffer
	code := Run([]string{"-download", dir + string(filepath.Separator) + ts.server.URL + "/model.gguf"}, &out, &errb, fakeRunner{}, testVersion)
	if code != 1 {
		t.Fatalf("code = %d, want 1 (stderr=%q)", code, errb.String())
	}
	if _, serr := os.Stat(filepath.Join(dir, "model.gguf")); serr != nil {
		t.Errorf("downloaded file missing: %v", serr)
	}
	// The failure must be the missing llama-server binary, NOT the model:
	// ResolvePaths checks the bin first.
	if !strings.Contains(errb.String(), "llama-server not found") {
		t.Errorf("stderr = %q, want the server-binary failure", errb.String())
	}
	if strings.Contains(errb.String(), "model not found") {
		t.Errorf("the downloaded model should satisfy the path check: %q", errb.String())
	}
}

func TestDownloadModel_RewiresConfig(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	payload := []byte("entry-point-payload")
	ts := newDownloadTestServer(t, map[string][]byte{"/model.gguf": payload})
	dir := t.TempDir()
	cfg := &config.Config{
		Download: dir + string(filepath.Separator) + ts.server.URL + "/model.gguf",
		Model:    "~/GGUF/old-model.gguf",
	}
	var out, errb bytes.Buffer
	if code := downloadModel(cfg, "/home/unused", &out, &errb, "7.7.7"); code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, errb.String())
	}
	if want := filepath.Join(dir, "model.gguf"); cfg.Model != want {
		t.Errorf("cfg.Model = %q, want %q", cfg.Model, want)
	}
	if cfg.Download != "" {
		t.Errorf("cfg.Download must be reset, got %q", cfg.Download)
	}
	if !cfg.ModelSet {
		t.Error("cfg.ModelSet must be true so the alias derives from the new GGUF")
	}
}

func TestDownloadModel_WarnsOverridesInVerbose(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	ts := newDownloadTestServer(t, map[string][]byte{"/model.gguf": []byte("x")})
	dir := t.TempDir()
	cfg := &config.Config{
		Download:               dir + string(filepath.Separator) + ts.server.URL + "/model.gguf",
		Model:                  "~/GGUF/chosen.gguf",
		Verbose:                true,
		DownloadOverridesModel: true,
	}
	var out, errb bytes.Buffer
	if code := downloadModel(cfg, "/home/unused", &out, &errb, "7.7.7"); code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "-model is ignored") {
		t.Errorf("warning missing in -v output: %q", out.String())
	}
}

// The HF token must travel as a bearer header — from HF_TOKEN first, from
// HUGGING_FACE_HUB_TOKEN as fallback — and must never be printed.
func TestDownloadModel_PassesTokenFromEnv(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	var gotAuth string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotAuth = r.Header.Get("Authorization")
		mu.Unlock()
		w.Header().Set("Content-Length", "3")
		_, _ = w.Write([]byte("abc"))
	}))
	defer srv.Close()

	for _, env := range []string{"HF_TOKEN", "HUGGING_FACE_HUB_TOKEN"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv("HF_TOKEN", "")
			t.Setenv("HUGGING_FACE_HUB_TOKEN", "")
			t.Setenv(env, "sekrit-token")
			dir := t.TempDir()
			cfg := &config.Config{Download: dir + string(filepath.Separator) + srv.URL + "/model.gguf"}
			var out, errb bytes.Buffer
			if code := downloadModel(cfg, "/home/unused", &out, &errb, "7.7.7"); code != 0 {
				t.Fatalf("code = %d, stderr = %q", code, errb.String())
			}
			mu.Lock()
			auth := gotAuth
			mu.Unlock()
			if auth != "Bearer sekrit-token" {
				t.Errorf("Authorization = %q, want the bearer token", auth)
			}
			if strings.Contains(out.String(), "sekrit-token") || strings.Contains(errb.String(), "sekrit-token") {
				t.Error("the token leaked into the output")
			}
		})
	}
}
