package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var version = "1.5.2"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, realRunner{}))
}

// trackedProcess централизует Wait: для каждого дочернего процесса Wait вызывается ровно один раз.
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

// waitForServerProcess прекращает ожидание готовности, если процесс сервера завершился раньше.
func waitForServerProcess(url string, timeout time.Duration, process *trackedProcess) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	client := &http.Client{Timeout: 3 * time.Second}
	var lastErr error
	check := func() (bool, error) {
		resp, err := client.Get(url)
		if err != nil {
			lastErr = err
			return false, nil
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return true, nil
		}
		lastErr = fmt.Errorf("статус %d", resp.StatusCode)
		return false, nil
	}
	for {
		select {
		case <-process.done:
			return fmt.Errorf("llama-server завершился до готовности: %v", process.Wait())
		default:
		}
		ready, _ := check()
		if ready {
			return nil
		}
		select {
		case <-process.done:
			return fmt.Errorf("llama-server завершился до готовности: %v", process.Wait())
		case <-deadline.C:
			if lastErr == nil {
				lastErr = fmt.Errorf("таймаут")
			}
			return fmt.Errorf("сервер не стал готов за %v: %v", timeout, lastErr)
		case <-ticker.C:
		}
	}
}

// waitForServer периодически опрашивает /health, пока сервер не ответит 200 OK.
func waitForServer(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 3 * time.Second}
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			lastErr = fmt.Errorf("статус %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(1 * time.Second)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("таймаут")
	}
	return fmt.Errorf("сервер не стал готов за %v: %v", timeout, lastErr)
}

// stopMcp корректно останавливает mcp-search-server.
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

// expandHome заменяет ведущий "~" на домашний каталог пользователя.
func expandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}