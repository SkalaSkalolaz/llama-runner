package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

func TestOpenServerLog_InheritsStreamsInServerMode(t *testing.T) {
	cfg := &config.Config{ServerOnly: true, QuietServer: true, LogFile: "/unused/log"}
	sl, code := OpenServerLog(cfg, "/unused", newBuffer(), newBuffer())
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	if sl.Writer != nil || sl.Name != "" {
		t.Errorf("writer = %v, name = %q, want inherited streams", sl.Writer, sl.Name)
	}
	sl.Close()
}

func TestOpenServerLog_InheritsStreamsWithoutQuietServer(t *testing.T) {
	cfg := &config.Config{QuietServer: false, LogFile: "/unused/log"}
	sl, code := OpenServerLog(cfg, "/unused", newBuffer(), newBuffer())
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	if sl.Writer != nil {
		t.Errorf("writer set without -quiet-server")
	}
	sl.Close()
}

func TestOpenServerLog_PlainFileMode0600(t *testing.T) {
	root := t.TempDir()
	logPath := filepath.Join(root, "server.log")
	cfg := &config.Config{QuietServer: true, LogFile: logPath, LogMaxMB: 0}

	sl, code := OpenServerLog(cfg, root, newBuffer(), newBuffer())
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if sl.Writer == nil {
		t.Fatal("Writer = nil, want file sink")
	}
	if sl.Name != logPath {
		t.Errorf("Name = %q, want %q", sl.Name, logPath)
	}
	info, statErr := os.Stat(logPath)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("log mode = %o, want 600", info.Mode().Perm())
	}
	if got, readErr := os.ReadFile(logPath); readErr != nil || len(got) != 0 {
		t.Errorf("fresh log = %d bytes, want empty", len(got))
	}
	sl.Close()
}

func TestOpenServerLog_AppendsToExistingLog(t *testing.T) {
	root := t.TempDir()
	logPath := filepath.Join(root, "server.log")
	if err := os.WriteFile(logPath, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{QuietServer: true, LogFile: logPath, LogMaxMB: 0}

	sl, code := OpenServerLog(cfg, root, newBuffer(), newBuffer())
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	sl.Writer.Write([]byte("new\n"))
	sl.Close()

	got, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(got, []byte("old\nnew\n")) {
		t.Errorf("log content = %q, want appended", got)
	}
}

func TestOpenServerLog_RotatingSink(t *testing.T) {
	root := t.TempDir()
	logPath := filepath.Join(root, "rotating", "server.log")
	cfg := &config.Config{QuietServer: true, LogFile: logPath, LogMaxMB: 1, LogKeep: 2}

	sl, code := OpenServerLog(cfg, root, newBuffer(), newBuffer())
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if sl.Writer == nil {
		t.Fatal("Writer = nil, want rotating sink")
	}
	sl.Writer.Write([]byte("hello\n"))
	sl.Close()

	if _, statErr := os.Stat(logPath); statErr != nil {
		t.Errorf("rotating log not created: %v", statErr)
	}
}

func TestOpenServerLog_UncreatableDirectoryFails(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{QuietServer: true, LogFile: filepath.Join(blocker, "sub", "server.log")}

	err := newBuffer()
	sl, code := OpenServerLog(cfg, root, newBuffer(), err)
	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if err.Len() == 0 {
		t.Error("no diagnostic printed for uncreatable log directory")
	}
	sl.Close()
}

func TestOpenProxyLog_CreatesCacheFile(t *testing.T) {
	root := t.TempDir()
	out := newBuffer()

	w, f := OpenProxyLog(root, out, true)
	if f == nil || w == nil {
		t.Fatalf("sink = (%v, %v), want opened file", w, f)
	}
	defer f.Close()

	logPath := filepath.Join(root, ".cache", "llama-runner", "proxy.log")
	info, statErr := os.Stat(logPath)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("proxy log mode = %o, want 600", info.Mode().Perm())
	}
	if out.String() == "" {
		t.Error("-v should report the proxy log path")
	}
	if !strings.Contains(out.String(), logPath) {
		t.Errorf("proxy log path not reported: %q", out.String())
	}
}

func TestOpenProxyLog_FailureIsSilentlyNonFatal(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".cache"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := newBuffer()
	w, f := OpenProxyLog(root, out, false)
	if w != nil || f != nil {
		t.Errorf("sink = (%v, %v), want no sink", w, f)
	}
	if f != nil {
		f.Close()
	}
}

func TestServerLog_CloseIsIdempotentForEmptySink(t *testing.T) {
	sl := ServerLog{}
	sl.Close()
	sl.Close()
}

func newBuffer() *bytes.Buffer { return new(bytes.Buffer) }
