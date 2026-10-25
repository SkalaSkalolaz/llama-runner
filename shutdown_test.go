package main

import (
	"bytes"
	"os/exec"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// waitExit ждёт завершения процесса до timeout; возвращает true, если завершился.
func waitExit(cmd *exec.Cmd, p *trackedProcess, timeout time.Duration) bool {
	select {
	case <-p.done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func TestShutdown_StopAllKillsGroup(t *testing.T) {
	// Процесс с потомком: sh запускает sleep в фоне. Групповой SIGINT
	// должен убить и sh, и sleep.
	cmd := exec.Command("sh", "-c", "sleep 30 & wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := trackProcess(cmd)
	defer func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-p.done
	}()

	shut := newShutdown()
	shut.SetServer(cmd)
	shut.StopAll()

	if !waitExit(cmd, p, 3*time.Second) {
		t.Fatal("SIGINT не убил группу за 3с")
	}
}

func TestShutdown_StopAllDirectProcess(t *testing.T) {
	// Без Setpgid — сигнал идёт прямому процессу.
	cmd := exec.Command("sh", "-c", "sleep 30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := trackProcess(cmd)
	defer func() {
		_ = cmd.Process.Kill()
		<-p.done
	}()

	shut := newShutdown()
	shut.SetClaude(cmd) // claude обрабатывается как «прямой процесс»
	shut.StopAll()

	if !waitExit(cmd, p, 3*time.Second) {
		t.Fatal("SIGINT не убил процесс за 3с")
	}
}

func TestShutdown_EscalateSIGKILL(t *testing.T) {
	// Процесс игнорирует SIGINT и SIGTERM — только SIGKILL.
	cmd := exec.Command("sh", "-c", "trap '' INT TERM; while true; do sleep 0.1; done")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := trackProcess(cmd)
	defer func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-p.done
	}()

	shut := newShutdown()
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
	// Первый сигнал → SIGINT, через timeout → SIGTERM.
	cmd := exec.Command("sh", "-c", "trap '' TERM; while true; do sleep 0.1; done")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := trackProcess(cmd)
	defer func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-p.done
	}()

	shut := newShutdown()
	shut.SetServer(cmd)

	sigCh := make(chan os.Signal, 4)
	done := make(chan struct{})
	var buf bytes.Buffer
	go handleSignals(sigCh, done, shut, &buf, 200*time.Millisecond)

	sigCh <- syscall.SIGINT
	// Через ~200 мс handleSignals отправит SIGTERM, через ещё 200 — SIGKILL.
	// Процесс игнорирует TERM, значит нужен KILL, а он — на 3-й итерации (~400 мс).
	if !waitExit(cmd, p, 3*time.Second) {
		t.Fatal("эскалация не убила процесс за 3с; buf=%q", buf.String())
	}
	if buf.Len() == 0 {
		t.Error("handleSignals ничего не написал в stdout")
	}
}

func TestHandleSignals_SecondSignalEscalates(t *testing.T) {
	// Второй сигнал ускоряет эскалацию: SIGINT, потом сразу SIGTERM (без timeout).
	cmd := exec.Command("sh", "-c", "trap '' TERM; while true; do sleep 0.1; done")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := trackProcess(cmd)
	defer func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-p.done
	}()

	shut := newShutdown()
	shut.SetServer(cmd)

	sigCh := make(chan os.Signal, 4)
	done := make(chan struct{})
	var buf bytes.Buffer
	go handleSignals(sigCh, done, shut, &buf, 5*time.Second)

	sigCh <- syscall.SIGINT
	time.Sleep(50 * time.Millisecond)
	sigCh <- syscall.SIGINT // второй — должен ускорить
	// Ещё один SIGKILL прилетит через 5с timeout — но процесс должен умереть
	// от SIGTERM+повторного сигнала быстрее. Ждём 2с.
	if !waitExit(cmd, p, 2*time.Second) {
		t.Fatal("второй сигнал не ускорил эскалацию")
	}
}

func TestHandleSignals_DoneStops(t *testing.T) {
	shut := newShutdown()
	sigCh := make(chan os.Signal, 1)
	done := make(chan struct{})
	var buf bytes.Buffer
	returned := make(chan struct{})
	go func() {
		handleSignals(sigCh, done, shut, &buf, time.Second)
		close(returned)
	}()
	close(done)
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("handleSignals не завершился по close(done)")
	}
}

var _ = atomic.AddInt32 // избежать unused-import при изменениях
