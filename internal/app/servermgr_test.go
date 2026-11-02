package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

// aliveRunner подменяет llama-server долгоживущим процессом sleep: нам важно
// не содержимое сервера, а жизненный цикл (запустился, зарегистрирован,
// дожидаемся health).
type aliveRunner struct{}

func (aliveRunner) Command(name string, args ...string) *exec.Cmd {
	return exec.Command("sleep", "30")
}
func (aliveRunner) LookPath(file string) (string, error) { return file, nil }

// dyingRunner возвращает процесс, мгновенно завершающийся с кодом 1.
type dyingRunner struct{}

func (dyingRunner) Command(name string, args ...string) *exec.Cmd {
	return exec.Command("sh", "-c", "exit 1")
}
func (dyingRunner) LookPath(file string) (string, error) { return file, nil }

// tempLaunchCfg — конфигурация, проходящую проверки SpecForConfig: реальные
// (пустые) файлы бинарника и модели во временном home.
func tempLaunchCfg(t *testing.T) (*config.Config, string) {
	t.Helper()
	home := t.TempDir()
	bin := filepath.Join(home, "llama-server")
	model := filepath.Join(home, "model.gguf")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(model, []byte("gguf"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &config.Config{
		ServerBin:    bin,
		Model:        model,
		Host:         "127.0.0.1",
		Port:         "8080",
		CtxSize:      "8192",
		BatchSize:    "512",
		UbatchSize:   "128",
		Tools:        "all",
		Parallel:     "1",
		ReadyTimeout: 5 * time.Second,
	}, home
}

func TestSpecForConfig_ResolvesBinArgsAndHealth(t *testing.T) {
	cfg, home := tempLaunchCfg(t)
	spec, code := SpecForConfig(cfg, home, io.Discard, io.Discard)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if spec.Bin != filepath.Join(home, "llama-server") {
		t.Errorf("Bin = %s", spec.Bin)
	}
	if len(spec.Args) == 0 || spec.Args[0] != "-m" {
		t.Fatalf("Args = %v, want starting with -m", spec.Args)
	}
	if spec.HealthURL != "http://127.0.0.1:8080/health" {
		t.Errorf("HealthURL = %s", spec.HealthURL)
	}
	if spec.ReadyTimeout != 5*time.Second {
		t.Errorf("ReadyTimeout = %v", spec.ReadyTimeout)
	}
	if spec.WorkDir == "" {
		t.Error("WorkDir пуст, хотя должен падать на cwd запускающего")
	}
}

func TestSpecForConfig_MissingModelIsError(t *testing.T) {
	cfg, home := tempLaunchCfg(t)
	cfg.Model = filepath.Join(home, "absent.gguf")
	_, code := SpecForConfig(cfg, home, io.Discard, io.Discard)
	if code == 0 {
		t.Fatal("отсутствующая модель должна делать спек незапускаемым")
	}
}

func TestServerBaseURL_WildcardsThroughLoopback(t *testing.T) {
	for _, host := range []string{"", "0.0.0.0", "::"} {
		cfg := &config.Config{Host: host, Port: "8080"}
		if got := ServerBaseURL(cfg); got != "http://127.0.0.1:8080" {
			t.Errorf("host %q: got %s, want loopback", host, got)
		}
	}
	cfg := &config.Config{Host: "192.168.0.5", Port: "9090"}
	if got := ServerBaseURL(cfg); got != "http://192.168.0.5:9090" {
		t.Errorf("got %s", got)
	}
}

func TestServerManager_StartTracksAndRegisters(t *testing.T) {
	cfg, home := tempLaunchCfg(t)
	spec, _ := SpecForConfig(cfg, home, io.Discard, io.Discard)

	shut := NewShutdown()
	mgr := NewServerManager(aliveRunner{}, shut, io.Discard, io.Discard)
	if err := mgr.Start(spec); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(killCurrent(mgr))

	if mgr.Current() == nil {
		t.Fatal("Current() = nil после успешного Start")
	}
	// Координатор выключений обязан видеть именно текущий процесс — иначе
	// Ctrl-C промахнётся по серверу.
	shut.mu.Lock()
	registered := shut.server
	shut.mu.Unlock()
	if registered != mgr.CurrentCmd() {
		t.Error("shutdown зарегистрировал другой процесс")
	}
}

func TestServerManager_StartReplacesCurrent(t *testing.T) {
	cfg, home := tempLaunchCfg(t)
	spec, _ := SpecForConfig(cfg, home, io.Discard, io.Discard)

	shut := NewShutdown()
	mgr := NewServerManager(aliveRunner{}, shut, io.Discard, io.Discard)
	if err := mgr.Start(spec); err != nil {
		t.Fatal(err)
	}
	first := mgr.CurrentCmd()
	if err := mgr.Start(spec); err != nil {
		t.Fatal(err)
	}
	second := mgr.CurrentCmd()
	t.Cleanup(func() {
		for _, c := range []*exec.Cmd{first, second} {
			if c != nil && c.Process != nil {
				_ = c.Process.Kill()
				_, _ = c.Process.Wait()
			}
		}
	})

	if first == second {
		t.Fatal("второй Start не заменил текущий процесс")
	}
	shut.mu.Lock()
	registered := shut.server
	shut.mu.Unlock()
	if registered != second {
		t.Error("координатор должен указывать на замену, а не на прежний процесс")
	}
}

func TestServerManager_WaitReadySucceeds(t *testing.T) {
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer hs.Close()

	cfg, home := tempLaunchCfg(t)
	spec, _ := SpecForConfig(cfg, home, io.Discard, io.Discard)
	spec.HealthURL = hs.URL + "/health"

	mgr := NewServerManager(aliveRunner{}, NewShutdown(), io.Discard, io.Discard)
	if err := mgr.Start(spec); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(killCurrent(mgr))

	if err := mgr.WaitReady(spec); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}
}

func TestServerManager_WaitReadyReturnsWhenProcessDies(t *testing.T) {
	cfg, home := tempLaunchCfg(t)
	spec, _ := SpecForConfig(cfg, home, io.Discard, io.Discard)
	spec.HealthURL = "http://127.0.0.1:1/health" // никогда не ответит
	spec.ReadyTimeout = 30 * time.Second         // но ждать не придется

	mgr := NewServerManager(dyingRunner{}, NewShutdown(), io.Discard, io.Discard)
	if err := mgr.Start(spec); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if err := mgr.WaitReady(spec); err == nil {
		t.Fatal("ожидалась ошибка умершего процесса")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("WaitReady не вернулся сразу после смерти процесса (ждал %v)", time.Since(start))
	}
}

func TestServerManager_WaitReadyBeforeStart(t *testing.T) {
	mgr := NewServerManager(aliveRunner{}, NewShutdown(), io.Discard, io.Discard)
	if err := mgr.WaitReady(ServerSpec{}); err == nil {
		t.Fatal("до Start WaitReady обязан ошибаться")
	}
}

// killCurrent освобождает sleep-процесс, запущенный менеджером.
func killCurrent(mgr *ServerManager) func() {
	return func() {
		if c := mgr.CurrentCmd(); c != nil && c.Process != nil {
			_ = c.Process.Kill()
			_, _ = c.Process.Wait()
		}
	}
}
