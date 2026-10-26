package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------- expandHome: чистые тесты, без побочных эффектов ----------

func TestExpandHome(t *testing.T) {
	home := "/home/test"
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"tilde", "~", home},
		{"tilde slash", "~/foo", "/home/test/foo"},
		{"tilde slash nested", "~/a/b/c", "/home/test/a/b/c"},
		{"tilde slash only", "~/", home},
		{"absolute", "/abs/path", "/abs/path"},
		{"relative", "rel/path", "rel/path"},
		{"tilde user (no expand)", "~user/x", "~user/x"},
		{"tilde no slash", "~foo", "~foo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := expandHome(tt.in, home); got != tt.want {
				t.Errorf("expandHome(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// ---------- writeMCPConfig: структура JSON, права, cleanup ----------

func TestWriteMCPConfig_Temporary(t *testing.T) {
	path, err := writeMCPConfig("", "http://127.0.0.1:8090/mcp")
	if err != nil {
		t.Fatalf("writeMCPConfig: %v", err)
	}
	defer os.Remove(path)

	// Файл создан в os.TempDir()
	if !strings.HasPrefix(path, os.TempDir()) {
		t.Errorf("temp file %q is not under %q", path, os.TempDir())
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("permissions = %o, want 0600", perm)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var cfg struct {
		MCPServers map[string]struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	search, ok := cfg.MCPServers["search"]
	if !ok {
		t.Fatal("mcpServers.search missing")
	}
	if search.Type != "http" {
		t.Errorf("type = %q, want http", search.Type)
	}
	if search.URL != "http://127.0.0.1:8090/mcp" {
		t.Errorf("url = %q", search.URL)
	}
}

func TestWriteMCPConfig_ExplicitPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "mcp.json")

	got, err := writeMCPConfig(path, "http://localhost:8090/mcp")
	if err != nil {
		t.Fatalf("writeMCPConfig: %v", err)
	}
	if got != path {
		t.Errorf("returned path = %q, want %q", got, path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("permissions = %o, want 0600", perm)
	}
	// Родительский каталог создан
	if _, err := os.Stat(filepath.Join(dir, "nested")); err != nil {
		t.Errorf("nested dir not created: %v", err)
	}
}

func TestWriteMCPConfig_InvalidParent(t *testing.T) {
	dir := t.TempDir()
	regularFile := filepath.Join(dir, "afile")
	if err := os.WriteFile(regularFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Пытаемся создать файл "внутри" обычного файла
	_, err := writeMCPConfig(filepath.Join(regularFile, "mcp.json"), "http://x")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- trackProcess: Wait ровно один раз, корректный exit code ----------

func TestTrackProcess_ImmediateExit(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	p := trackProcess(cmd)

	select {
	case <-p.done:
	case <-time.After(2 * time.Second):
		t.Fatal("process did not complete within 2s")
	}
	if err := p.Wait(); err != nil {
		t.Errorf("Wait() = %v, want nil", err)
	}
}

func TestTrackProcess_NonZeroExit(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 42")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	p := trackProcess(cmd)
	err := p.Wait()
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("err is not *exec.ExitError: %T", err)
	}
	if exitErr.ExitCode() != 42 {
		t.Errorf("exit code = %d, want 42", exitErr.ExitCode())
	}
}

func TestTrackProcess_ConcurrentWait(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 0.1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	p := trackProcess(cmd)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = p.Wait()
		}()
	}
	wg.Wait()
}

// ---------- waitForServer: httptest.Server, разные сценарии ----------

func TestWaitForServer_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := waitForServer(srv.URL, 2*time.Second); err != nil {
		t.Errorf("waitForServer: %v", err)
	}
}

func TestWaitForServer_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	err := waitForServer(srv.URL, 1500*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "не стал готов") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWaitForServer_BecomesReady(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) >= 2 {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if err := waitForServer(srv.URL, 5*time.Second); err != nil {
		t.Errorf("waitForServer: %v", err)
	}
}

func TestWaitForServer_BadURL(t *testing.T) {
	// Заведомо нерабочий порт
	err := waitForServer("http://127.0.0.1:1", 500*time.Millisecond)
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---------- waitForServerProcess: реагирует на смерть процесса ----------

func TestWaitForServerProcess_ServerReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cmd := exec.Command("sh", "-c", "sleep 10")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	p := trackProcess(cmd)
	defer func() {
		_ = cmd.Process.Kill()
		<-p.done // дождаться, пока goroutine сделает Wait
	}()

	if err := waitForServerProcess(srv.URL, 2*time.Second, p); err != nil {
		t.Errorf("waitForServerProcess: %v", err)
	}
}

func TestWaitForServerProcess_ProcessDiesEarly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	cmd := exec.Command("sh", "-c", "exit 1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	p := trackProcess(cmd)

	err := waitForServerProcess(srv.URL, 5*time.Second, p)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "завершился до готовности") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWaitForServerProcess_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	cmd := exec.Command("sh", "-c", "sleep 10")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	p := trackProcess(cmd)
	defer func() {
		_ = cmd.Process.Kill()
		<-p.done
	}()

	err := waitForServerProcess(srv.URL, 1500*time.Millisecond, p)
	if err == nil {
		t.Fatal("expected timeout")
	}
	if !strings.Contains(err.Error(), "не стал готов") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------- stopMcp: graceful и force-kill ----------

func TestStopMcp_Nil(t *testing.T) {
	stopMcp(nil) // не должно паниковать
}

func TestStopMcp_GracefulShutdown(t *testing.T) {
	// sleep реагирует на SIGINT и завершается
	cmd := exec.Command("sh", "-c", "sleep 10")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	done := make(chan struct{})
	go func() {
		stopMcp(cmd)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("stopMcp did not finish in 10s")
	}
}

func TestIsRussianLocale(t *testing.T) {
	origLang := os.Getenv("LANG")
	origLcAll := os.Getenv("LC_ALL")
	origLangEnv := os.Getenv("LANGUAGE")
	defer func() {
		os.Setenv("LANG", origLang)
		os.Setenv("LC_ALL", origLcAll)
		os.Setenv("LANGUAGE", origLangEnv)
	}()

	tests := []struct {
		name     string
		lcAll    string
		lang     string
		language string
		want     bool
	}{
		{"empty all", "", "", "", false},
		{"LANG ru", "ru_RU.UTF-8", "", "", true},
		{"LC_ALL ru", "ru_RU.UTF-8", "en_US.UTF-8", "ru_RU:ru", true},
		{"LANG=en, LANGUAGE=ru (ignored)", "en_US.UTF-8", "", "ru_RU:ru", false},
		{"LANGUAGE=ru only (not locale)", "", "", "ru_RU:ru", false},
		{"LC_ALL=en, LANG=ru", "en_US.UTF-8", "ru_RU.UTF-8", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("LC_ALL", tt.lcAll)
			os.Setenv("LANG", tt.lang)
			os.Setenv("LANGUAGE", tt.language)
			if got := isRussianLocale(); got != tt.want {
				t.Errorf("isRussianLocale() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStopMcp_ForceKill(t *testing.T) {
	// sh игнорирует SIGINT — должен быть убит через SIGKILL
	cmd := exec.Command("sh", "-c", "trap '' INT; while true; do sleep 0.1; done")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	done := make(chan struct{})
	go func() {
		stopMcp(cmd)
		close(done)
	}()

	select {
	case <-done:
		// ok — либо завершился сам, либо был убит
	case <-time.After(15 * time.Second):
		t.Fatal("stopMcp did not finish in 15s")
	}
}
