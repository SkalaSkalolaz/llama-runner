package proc

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------- Track: Wait ровно один раз, корректный exit code ----------

func TestTrackProcess_ImmediateExit(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	p := Track(cmd)

	select {
	case <-p.Done:
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
	p := Track(cmd)
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
	p := Track(cmd)

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

// ---------- WaitForServer: httptest.Server, different scenarios ----------

func TestWaitForServer_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := WaitForServer(srv.URL, 2*time.Second); err != nil {
		t.Errorf("WaitForServer: %v", err)
	}
}

func TestWaitForServer_Timeout(t *testing.T) {
	t.Setenv("LC_ALL", "ru_RU.UTF-8") // localized error text
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	err := WaitForServer(srv.URL, 1500*time.Millisecond)
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

	if err := WaitForServer(srv.URL, 5*time.Second); err != nil {
		t.Errorf("WaitForServer: %v", err)
	}
}

func TestWaitForServer_BadURL(t *testing.T) {
	// Deliberately non-working port
	err := WaitForServer("http://127.0.0.1:1", 500*time.Millisecond)
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---------- WaitForServerProcess: reacts to process death ----------

func TestWaitForServerProcess_ServerReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cmd := exec.Command("sh", "-c", "sleep 10")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	p := Track(cmd)
	defer func() {
		_ = cmd.Process.Kill()
		<-p.Done // wait until the goroutine finishes Wait
	}()

	if err := WaitForServerProcess(srv.URL, 2*time.Second, p); err != nil {
		t.Errorf("WaitForServerProcess: %v", err)
	}
}

func TestWaitForServerProcess_ProcessDiesEarly(t *testing.T) {
	t.Setenv("LC_ALL", "ru_RU.UTF-8")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	cmd := exec.Command("sh", "-c", "exit 1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	p := Track(cmd)

	err := WaitForServerProcess(srv.URL, 5*time.Second, p)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "заверши") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWaitForServerProcess_Timeout(t *testing.T) {
	t.Setenv("LC_ALL", "ru_RU.UTF-8")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	cmd := exec.Command("sh", "-c", "sleep 10")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	p := Track(cmd)
	defer func() {
		_ = cmd.Process.Kill()
		<-p.Done
	}()

	err := WaitForServerProcess(srv.URL, 1500*time.Millisecond, p)
	if err == nil {
		t.Fatal("expected timeout")
	}
	if !strings.Contains(err.Error(), "не стал готов") {
		t.Errorf("unexpected error: %v", err)
	}
}
