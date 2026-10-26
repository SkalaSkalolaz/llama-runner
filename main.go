package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var version = "1.5.3"

// isRussianLocale returns true if LC_ALL or LANG env is set to a Russian locale.
func isRussianLocale() bool {
	if v := os.Getenv("LC_ALL"); strings.HasPrefix(v, "ru") {
		return true
	}
	if v := os.Getenv("LANG"); strings.HasPrefix(v, "ru") {
		return true
	}
	return false
}

// printUsageForLocale prints help text in the current console locale (RU or EN).
func printUsageForLocale(w io.Writer) {
	if isRussianLocale() {
		printUsageRU(w)
	} else {
		printUsageEN(w)
	}
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, realRunner{}))
}

// trackedProcess wraps exec.Cmd.Wait so each child process is waited on exactly once.
type trackedProcess struct {
	done chan struct{}
	err  error
}

func trackProcess(cmd *exec.Cmd) *trackedProcess {
	p := &trackedProcess{done: make(chan struct{})}
	go func() {
		p.err = cmd.Wait()
		close(p.done)
	}()
	return p
}

func (p *trackedProcess) Wait() error {
	<-p.done
	return p.err
}

// waitForReady polls url once per second until 200 OK or timeout expires.
//
//	done != nil — stop immediately if closed (process died);
//	label       — prefix for error message ("llama-server", "proxy").
//
// Errors:
//   - done closed → "<label> finished before ready";
//   - timeout    → "<label> didn't become ready in <d>: <lastErr>".
func waitForReady(ctx context.Context, url string, timeout time.Duration, done <-chan struct{}, label string) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := &http.Client{Timeout: 3 * time.Second}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var lastErr error
	check := func() bool {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			lastErr = err
			return false
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			return false
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return true
		}
		lastErr = fmt.Errorf("статус %d", resp.StatusCode)
		return false
	}

	for {
		if done != nil {
			select {
			case <-done:
				return fmt.Errorf("%s завершился до готовности", label)
			default:
			}
		}
		if check() {
			return nil
		}
		select {
		case <-done:
			return fmt.Errorf("%s завершился до готовности", label)
		case <-ctx.Done():
			if done != nil {
				select {
				case <-done:
					return fmt.Errorf("%s завершился до готовности", label)
				default:
				}
			}
			if lastErr == nil {
				lastErr = fmt.Errorf("таймаут")
			}
			return fmt.Errorf("%s не стал готов за %v: %v", label, timeout, lastErr)
		case <-ticker.C:
		}
	}
}

// waitForServer — wrapper for compatibility with old calls and tests.
func waitForServer(url string, timeout time.Duration) error {
	return waitForReady(context.Background(), url, timeout, nil, "сервер")
}

// waitForServerProcess — wrapper for compatibility with old tests.
func waitForServerProcess(url string, timeout time.Duration, process *trackedProcess) error {
	return waitForReady(context.Background(), url, timeout, process.done, "llama-server")
}

// stopMcp correctly stops mcp-search-server.
func stopMcp(mcpCmd *exec.Cmd) {
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
				fmt.Fprintf(os.Stderr, "mcp-search-server завершился с ошибкой: %v\n", err)
			}
		}
	case <-time.After(3 * time.Second):
		fmt.Fprintln(os.Stderr, "mcp-search-server не завершился за 3 секунды — убиваем")
		_ = mcpCmd.Process.Kill()
		select {
		case err := <-done:
			if err != nil {
				if _, ok := err.(*exec.ExitError); !ok {
					fmt.Fprintf(os.Stderr, "ошибка ожидания mcp-search-server: %v\n", err)
				}
			}
		case <-time.After(3 * time.Second):
			fmt.Fprintln(os.Stderr, "предупреждение: процесс mcp-search-server не подтвердил завершение после Kill")
		}
	}
}

func writeMCPConfig(path, httpURL string) (string, error) {
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

// expandHome replaces a leading "~" with the user's home directory.
func expandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}
