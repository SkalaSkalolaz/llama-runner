// Package proc owns the launcher's child-process plumbing: tracking a child
// so that exactly one goroutine calls exec.Cmd.Wait, polling an HTTP endpoint
// until the process behind it reports ready, and shutting the MCP helper down.
//
// These primitives are shared by the run loop (internal/app) and the readiness
// checks, so they live in a library package rather than in the executable.
package proc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

// Tracked represents a running child process whose termination is awaited by
// exactly one background goroutine calling cmd.Wait. The result is published
// once: Done is closed when Wait returns, and Err holds Wait's error (nil for
// a clean exit).
//
// This indirection exists because calling exec.Cmd.Wait from several
// goroutines (for example, the readiness watcher and the shutdown path) is a
// data race on the cmd struct; everyone instead waits on the shared channel.
type Tracked struct {
	// Done is closed as soon as the child process has exited. Wait blocks on
	// it, and select-style callers may wait on it directly.
	Done chan struct{}
	// Err is published before Done is closed: exec.ExitError for a non-zero
	// exit status, nil for a clean one.
	Err error
}

// Track starts watching cmd for termination in a background goroutine and
// returns a handle that any number of goroutines can safely Wait on. cmd
// must already have been started with Start.
func Track(cmd *exec.Cmd) *Tracked {
	p := &Tracked{Done: make(chan struct{})}
	go func() {
		p.Err = cmd.Wait()
		close(p.Done)
	}()
	return p
}

// Wait blocks until the tracked child process exits and returns the error from
// exec.Cmd.Wait (an exec.ExitError for a non-zero exit status). It is safe to
// call from multiple goroutines and returns immediately after the first call
// once the process has exited.
func (p *Tracked) Wait() error {
	<-p.Done
	return p.Err
}

// WaitForReady polls url with HTTP GET once per second until it answers 200
// OK, the timeout expires, ctx is cancelled, or done is closed — whichever
// happens first.
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
func WaitForReady(ctx context.Context, url string, timeout time.Duration, done <-chan struct{}, label string) error {
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
		lastErr = fmt.Errorf(i18n.T(i18n.MsgStatus), resp.StatusCode)
		return false
	}

	for {
		if done != nil {
			select {
			case <-done:
				return i18n.Errorv(i18n.MsgFinishedBeforeRdy, label)
			default:
			}
		}
		if check() {
			return nil
		}
		select {
		case <-done:
			return i18n.Errorv(i18n.MsgFinishedBeforeRdy, label)
		case <-ctx.Done():
			if done != nil {
				select {
				case <-done:
					return i18n.Errorv(i18n.MsgFinishedBeforeRdy, label)
				default:
				}
			}
			if lastErr == nil {
				lastErr = errors.New(i18n.T(i18n.MsgTimeout))
			}
			return i18n.Errorv(i18n.MsgNotReadyInTime, label, timeout, lastErr)
		case <-ticker.C:
		}
	}
}

// WaitForServer polls url until it returns 200 OK or timeout elapses. It has
// no process-death shortcut (done is nil), which suits daemons that are not
// represented by a tracked child — the proxy and the MCP server. Retained as a
// thin compatibility wrapper around WaitForReady.
func WaitForServer(url string, timeout time.Duration) error {
	return WaitForReady(context.Background(), url, timeout, nil, i18n.T(i18n.MsgLabelServer))
}

// WaitForServerProcess is WaitForServer with an additional early-exit
// condition: if the tracked llama-server process terminates before becoming
// ready, it returns immediately instead of waiting out the full timeout.
func WaitForServerProcess(url string, timeout time.Duration, process *Tracked) error {
	return WaitForReady(context.Background(), url, timeout, process.Done, "llama-server")
}
