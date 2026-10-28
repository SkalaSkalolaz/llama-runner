package app

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/proc"
)

// waitExit ждёт завершения процесса до timeout; возвращает true, если завершился.
func waitExit(cmd *exec.Cmd, p *proc.Tracked, timeout time.Duration) bool {
	select {
	case <-p.Done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func TestShutdown_StopAllKillsGroup(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 30 & wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := proc.Track(cmd)
	defer func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-p.Done
	}()

	shut := NewShutdown()
	shut.SetServer(cmd)
	shut.StopAll()

	if !waitExit(cmd, p, 3*time.Second) {
		t.Fatal("SIGINT не убил группу за 3с")
	}
}

func TestShutdown_StopAllDirectProcess(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := proc.Track(cmd)
	defer func() {
		_ = cmd.Process.Kill()
		<-p.Done
	}()

	shut := NewShutdown()
	shut.SetClaude(cmd)
	shut.StopAll()

	if !waitExit(cmd, p, 3*time.Second) {
		t.Fatal("SIGINT не убил процесс за 3с")
	}
}

func TestShutdown_EscalateSIGKILL(t *testing.T) {
	cmd := exec.Command("sh", "-c", "trap '' INT TERM; while true; do sleep 0.1; done")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := proc.Track(cmd)
	defer func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-p.Done
	}()

	shut := NewShutdown()
	shut.SetServer(cmd)
	shut.StopAll()
	time.Sleep(100 * time.Millisecond)
	shut.Escalate(syscall.SIGTERM)
	time.Sleep(100 * time.Millisecond)
	shut.Escalate(syscall.SIGKILL)

	if !waitExit(cmd, p, 3*time.Second) {
		t.Fatal("SIGKILL не убил процесс за 3с")
	}
}

func TestHandleSignals_EscalatesByTimeout(t *testing.T) {
	// sh ловит INT и TERM, умирает только от KILL: так тест реально
	// проверяет эскалацию по таймауту, а не смерть от первого SIGINT.
	cmd := exec.Command("sh", "-c", "trap '' INT TERM; while true; do sleep 0.1; done")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := proc.Track(cmd)
	defer func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-p.Done
	}()

	t.Setenv("LC_ALL", "ru_RU.UTF-8") // сообщения HandleSignals локализованы

	shut := NewShutdown()
	shut.SetServer(cmd)

	sigCh := make(chan os.Signal, 4)
	done := make(chan struct{})
	var buf bytes.Buffer
	returned := make(chan struct{})
	go func() {
		HandleSignals(sigCh, done, shut, &buf, 200*time.Millisecond)
		close(returned)
	}()

	sigCh <- syscall.SIGINT
	// Дожидаемся завершения HandleSignals ПЕРЕД чтением buf: иначе
	// buf.Len()/buf.String() гоняются с fmt.Fprintf из goroutine.
	select {
	case <-returned:
	case <-time.After(3 * time.Second):
		t.Fatal("HandleSignals не завершился за 3с")
	}
	if !waitExit(cmd, p, 3*time.Second) {
		t.Fatalf("эскалация не убила процесс за 3с; buf=%q", buf.String())
	}
	log := buf.String()
	if !strings.Contains(log, "Эскалация: SIGTERM") || !strings.Contains(log, "Эскалация: SIGKILL") {
		t.Errorf("в логе нет эскалации SIGTERM/SIGKILL: %q", log)
	}
}

func TestHandleSignals_SecondSignalEscalates(t *testing.T) {
	// sh ловит INT и TERM, умирает только от KILL.
	cmd := exec.Command("sh", "-c", "trap '' INT TERM; while true; do sleep 0.1; done")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := proc.Track(cmd)
	defer func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-p.Done
	}()

	t.Setenv("LC_ALL", "ru_RU.UTF-8")

	shut := NewShutdown()
	shut.SetServer(cmd)

	sigCh := make(chan os.Signal, 4)
	done := make(chan struct{})
	var buf bytes.Buffer
	returned := make(chan struct{})
	// timeout=1s: без второго сигнала SIGKILL — через 2×1s=2s.
	// С вторым сигналом (через 50ms) — через 1s+50ms ≈ 1.05s.
	go func() {
		HandleSignals(sigCh, done, shut, &buf, time.Second)
		close(returned)
	}()

	sigCh <- syscall.SIGINT
	time.Sleep(50 * time.Millisecond)
	sigCh <- syscall.SIGINT // ускоряет: SIGTERM сразу, SIGKILL через 1s

	// 1.5s — граница: с ускорением процесс умирает за ~1.05s,
	// без ускорения — за ~2s и тест упадёт.
	if !waitExit(cmd, p, 1500*time.Millisecond) {
		// В buf не лезем: goroutine может ещё писать.
		t.Fatal("второй сигнал не ускорил эскалацию (процесс жив >1.5s)")
	}
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("HandleSignals не завершился")
	}
	if !strings.Contains(buf.String(), "Повторный сигнал") {
		t.Errorf("в логе нет ускорения: %q", buf.String())
	}
}

func TestHandleSignals_DoneStops(t *testing.T) {
	shut := NewShutdown()
	sigCh := make(chan os.Signal, 1)
	done := make(chan struct{})
	var buf bytes.Buffer
	returned := make(chan struct{})
	go func() {
		HandleSignals(sigCh, done, shut, &buf, time.Second)
		close(returned)
	}()
	close(done)
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("HandleSignals не завершился по close(done)")
	}
}
