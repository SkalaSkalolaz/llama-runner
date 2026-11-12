package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
	"github.com/SkalaSkalolaz/llama-runner/internal/orchestrator"
	"github.com/SkalaSkalolaz/llama-runner/internal/runner"
)

// runInteractive is the `-p auto` (without -task, without -plan-only) mode:
// a small REPL where the user types one task per line, each task is
// decomposed by the planner, executed by the orchestrator, and the result
// is printed. "exit"/"quit" leaves; EOF on stdin also leaves.
//
// Each iteration gets its own cancelable context and its own signal
// handler, so Ctrl-C during a step cancels that step and returns to the
// prompt instead of tearing the session down. Ctrl-C at the prompt (or a
// SIGTERM anywhere) leaves cleanly; the deferred cleanup in Run() stops
// the server and the MCP child.
//
// The orchestrator owns the model — every step goes through the same
// Swapper the headless path uses — so Claude Code and the routing proxy
// are deliberately not started in this mode.
func runInteractive(ctx context.Context, cfg *config.Config, home, baseURL string,
	sw orchestrator.Swapper, cmdRunner runner.CommandRunner,
	stdin io.Reader, stdout, stderr io.Writer) int {

	scanner := bufio.NewScanner(stdin)
	// A single task line may carry a paragraph; the default 64 KiB
	// scanner limit is too tight for that. 1 MiB is generous without
	// being unbounded.
	scanner.Buffer(make([]byte, 4096), 1024*1024)

	// Own signal registration for the prompt loop: Ctrl-C at the prompt
	// must return from this function instead of blocking forever on
	// scanner.Scan(). The outer HandleSignals was detached in Run()
	// before this function was called (see the `signal.Stop(sigCh)`
	// there), so we are the only receiver.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	for {
		fmt.Fprint(stdout, i18n.TF(i18n.MsgInteractivePrompt))

		// Race the blocking stdin read against a signal, so Ctrl-C at
		// the prompt exits immediately. The read goroutine is left
		// behind on the signal path — it dies with the process, and
		// this function is already on its way out when that happens.
		lineCh := make(chan string, 1)
		eofCh := make(chan struct{})
		go func() {
			if scanner.Scan() {
				lineCh <- strings.TrimSpace(scanner.Text())
			} else {
				close(eofCh)
			}
		}()

		var task string
		select {
		case <-sigCh:
			fmt.Fprint(stdout, i18n.TF(i18n.MsgInteractiveBye))
			return 0
		case <-eofCh:
			fmt.Fprint(stdout, i18n.TF(i18n.MsgInteractiveBye))
			return 0
		case task = <-lineCh:
		}

		if task == "" {
			continue
		}
		if task == "exit" || task == "quit" {
			fmt.Fprint(stdout, i18n.TF(i18n.MsgInteractiveBye))
			return 0
		}

		// One task: plan + execute. A per-iteration signal channel lets
		// Ctrl-C cancel only the running step, not the whole session.
		execCtx, cancel := context.WithCancel(ctx)
		execSig := make(chan os.Signal, 1)
		signal.Notify(execSig, os.Interrupt, syscall.SIGTERM)
		go func() {
			select {
			case <-execSig:
				cancel()
			case <-execCtx.Done():
			}
		}()

		// runExecute reads cfg.Task; a per-iteration copy keeps the
		// outer config pristine across iterations.
		taskCfg := *cfg
		taskCfg.Task = task
		code := runExecute(execCtx, &taskCfg, home, baseURL, sw, cmdRunner, stdout, stderr)
		signal.Stop(execSig)
		cancel()

		if code != 0 {
			fmt.Fprint(stderr, i18n.TF(i18n.MsgInteractiveTaskFailed, code))
		}
	}
}
