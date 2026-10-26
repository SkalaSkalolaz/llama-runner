// Command llama-runner launches a local llama-server with a GGUF model and,
// by default, hands control over to Claude Code configured to talk to that
// server through ANTHROPIC_* environment variables. It can additionally run
// an mcp-search-server (web search), a local proxy that routes between the
// local llama-server and an external OpenAI-compatible provider, and offers
// llama.cpp update (-update) and diagnostics (-doctor) modes.
//
// Run "llama-runner -h" for the full flag reference; see printUsageEN and
// printUsageRU for the English and Russian help texts.
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

// version is the human-readable program version reported by -version and
// -doctor. Release builds override it at link time with
// -ldflags "-X main.version=<git describe>"; this literal is the fallback
// used for local development builds.
var version = "1.5.3"

// isRussianLocale reports whether the console locale is Russian.
//
// Only LC_ALL and LANG are consulted — the POSIX variables that actually
// select the message locale, with LC_ALL taking precedence over LANG.
// LANGUAGE is deliberately ignored: GNU gettext only falls back to it when
// LANG/LC_ALL select the "C"/POSIX locale, so it does not reliably reflect
// the console language. A value matches when it starts with the "ru"
// language tag (e.g. "ru_RU.UTF-8"); anything else — including an unset or
// empty variable — is treated as non-Russian.
func isRussianLocale() bool {
	if v := os.Getenv("LC_ALL"); strings.HasPrefix(v, "ru") {
		return true
	}
	if v := os.Getenv("LANG"); strings.HasPrefix(v, "ru") {
		return true
	}
	return false
}

// printUsageForLocale writes the command-line help to w, selecting the
// language from the console locale: Russian when isRussianLocale reports
// true, English otherwise. It is installed as flag.FlagSet.Usage, so -h and
// --help automatically print the help in the user's console language.
func printUsageForLocale(w io.Writer) {
	if isRussianLocale() {
		printUsageRU(w)
	} else {
		printUsageEN(w)
	}
}

// main is the process entry point. It delegates everything to run (which is
// fully testable with injected streams and runner) and turns the returned
// status code into the process exit code.
func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, realRunner{}))
}

// trackedProcess represents a running child process whose termination is
// awaited by exactly one background goroutine calling cmd.Wait. The result is
// published once: done is closed when Wait returns, and err holds Wait's
// error (nil for a clean exit).
//
// This indirection exists because calling exec.Cmd.Wait from several
// goroutines (for example, the readiness watcher and the shutdown path) is a
// data race on the cmd struct; everyone instead waits on the shared channel.
type trackedProcess struct {
	done chan struct{}
	err  error
}

// trackProcess starts watching cmd for termination in a background goroutine
// and returns a handle that any number of goroutines can safely Wait on.
// cmd must already have been started with Start.
func trackProcess(cmd *exec.Cmd) *trackedProcess {
	p := &trackedProcess{done: make(chan struct{})}
	go func() {
		p.err = cmd.Wait()
		close(p.done)
	}()
	return p
}

// Wait blocks until the tracked child process exits and returns the error
// from exec.Cmd.Wait (an *exec.ExitError for a non-zero exit status). It is
// safe to call from multiple goroutines and returns immediately after the
// first call once the process has exited.
func (p *trackedProcess) Wait() error {
	<-p.done
	return p.err
}

// waitForReady polls url with HTTP GET once per second until it answers
// 200 OK, the timeout expires, ctx is cancelled, or done is closed —
// whichever happens first.
//
// Parameters:
//
//	done  — if non-nil, polling stops immediately as soon as done is
//	        closed, meaning the process being waited for has died;
//	label — names the waited-for component in error messages
//	        (e.g. "llama-server", "proxy").
//
// Each probe has its own 3-second HTTP client timeout so a hanging server
// cannot stall the poll loop. Errors:
//
//   - done closed           → "<label> finished before ready";
//   - timeout / ctx done    → "<label> didn't become ready in <d>: <lastErr>",
//     where lastErr is the most recent probe failure
//     (connection error or non-200 status); when no probe
//     ever failed with a message, "timeout" is reported.
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

// waitForServer polls url until it returns 200 OK or timeout elapses.
// It has no process-death shortcut (done is nil), which suits daemons that
// are not represented by a tracked child — the proxy and the MCP server.
// Retained as a thin compatibility wrapper around waitForReady.
func waitForServer(url string, timeout time.Duration) error {
	return waitForReady(context.Background(), url, timeout, nil, "сервер")
}

// waitForServerProcess is waitForServer with an additional early-exit
// condition: if the tracked llama-server process terminates before becoming
// ready, it returns immediately instead of waiting out the full timeout.
func waitForServerProcess(url string, timeout time.Duration, process *trackedProcess) error {
	return waitForReady(context.Background(), url, timeout, process.done, "llama-server")
}

// stopMcp shuts down mcp-search-server gracefully and is safe to call with a
// nil command or one that never started.
//
// Strategy: send SIGINT (the server's graceful-shutdown signal), then wait up
// to 3 seconds for it to exit. If it does not, escalate to SIGKILL and wait
// another 3 seconds for confirmation; a process that still refuses to die is
// only reported as a warning, never fatal. Non-ExitError failures from Wait
// (unexpected wait errors) are reported to stderr; a plain non-zero exit is
// not treated as an error worth printing.
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

// writeMCPConfig writes a Claude Code MCP configuration file that registers
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

// expandHome replaces a leading "~" with the given home directory:
// "~" becomes home itself, "~/foo" becomes home/foo, and anything else
// (relative or absolute paths, "~user" forms, embedded tildes) is returned
// unchanged. The home value is injected by the caller rather than looked up
// here, which keeps the function pure and testable.
func expandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}
