package proc

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------- WriteMCPConfig: JSON structure, permissions, cleanup ----------

func TestWriteMCPConfig_Temporary(t *testing.T) {
	path, err := WriteMCPConfig("", "http://127.0.0.1:8090/mcp")
	if err != nil {
		t.Fatalf("WriteMCPConfig: %v", err)
	}
	defer os.Remove(path)

	// File created inside os.TempDir()
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

	got, err := WriteMCPConfig(path, "http://localhost:8090/mcp")
	if err != nil {
		t.Fatalf("WriteMCPConfig: %v", err)
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
	// Parent directory created
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
	// Try to create a file "inside" a regular file
	_, err := WriteMCPConfig(filepath.Join(regularFile, "mcp.json"), "http://x")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- StopMcp: graceful shutdown and force kill ----------

func TestStopMcp_Nil(t *testing.T) {
	StopMcp(nil) // must not panic
}

func TestStopMcp_GracefulShutdown(t *testing.T) {
	// sleep reacts to SIGINT and exits
	cmd := exec.Command("sh", "-c", "sleep 10")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	done := make(chan struct{})
	go func() {
		StopMcp(cmd)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("StopMcp did not finish in 10s")
	}
}

func TestStopMcp_ForceKill(t *testing.T) {
	// sh ignores SIGINT here — must be killed with SIGKILL
	cmd := exec.Command("sh", "-c", "trap '' INT; while true; do sleep 0.1; done")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	done := make(chan struct{})
	go func() {
		StopMcp(cmd)
		close(done)
	}()

	select {
	case <-done:
		// ok — it either exited on its own or was killed
	case <-time.After(15 * time.Second):
		t.Fatal("StopMcp did not finish in 15s")
	}
}
