// MCP-specific process handling: the lifecycle of mcp-search-server and the
// Claude Code configuration file that points at it.
package proc

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

// StopMcp shuts down mcp-search-server gracefully and is safe to call with a
// nil command or one that never started.
//
// Strategy: send SIGINT (the server's graceful-shutdown signal), then wait up
// to 3 seconds for it to exit. If it does not, escalate to SIGKILL and wait
// another 3 seconds for confirmation; a process that still refuses to die is
// only reported as a warning, never fatal. Non-ExitError failures from Wait
// (unexpected wait errors) are reported to stderr; a plain non-zero exit is
// not treated as an error worth printing.
func StopMcp(mcpCmd *exec.Cmd) {
	if mcpCmd == nil || mcpCmd.Process == nil {
		return
	}
	_ = mcpCmd.Process.Signal(syscall.SIGINT)

	done := make(chan error, 1)
	go func() {
		done <- mcpCmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			if _, ok := err.(*exec.ExitError); !ok {
				fmt.Fprint(os.Stderr, i18n.TF(i18n.MsgMcpExitErr, err))
			}
		}
	case <-time.After(3 * time.Second):
		fmt.Fprint(os.Stderr, i18n.TF(i18n.MsgMcpKill))
		_ = mcpCmd.Process.Kill()
		select {
		case err := <-done:
			if err != nil {
				if _, ok := err.(*exec.ExitError); !ok {
					fmt.Fprint(os.Stderr, i18n.TF(i18n.MsgMcpWaitErr, err))
				}
			}
		case <-time.After(3 * time.Second):
			fmt.Fprint(os.Stderr, i18n.TF(i18n.MsgMcpKillUnconfirmed))
		}
	}
}

// WriteMCPConfig writes a Claude Code MCP configuration file that registers
// the running mcp-search-server as an HTTP server named "search" pointing at
// httpURL.
//
// When path is empty, a fresh temporary file (llama-runner-mcp-*.json) is
// created and its name returned — the caller is responsible for removing it.
// When path is given, any missing parent directories are created (mode 0700)
// and the file is written (or overwritten) at that path.
//
// The file is always written with 0600 permissions. On any failure the
// temporary file is cleaned up before returning the error.
func WriteMCPConfig(path, httpURL string) (string, error) {
	temporary := path == ""
	if temporary {
		f, err := os.CreateTemp("", "llama-runner-mcp-*.json")
		if err != nil {
			return "", err
		}
		path = f.Name()
		if err := f.Close(); err != nil {
			_ = os.Remove(path)
			return "", err
		}
	} else {
		dir := filepath.Dir(path)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return "", err
			}
		}
	}

	cfg := map[string]any{
		"mcpServers": map[string]any{
			"search": map[string]any{
				"type": "http",
				"url":  httpURL,
			},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		if temporary {
			_ = os.Remove(path)
		}
		return "", err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		if temporary {
			_ = os.Remove(path)
		}
		return "", err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		if temporary {
			_ = os.Remove(path)
		}
		return "", err
	}
	return path, nil
}
