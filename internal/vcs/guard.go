// Package vcs provides a lightweight Git guard for the auto-executor:
// it creates a branch, checkpoints per wave, and rolls back on failure.
package vcs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
	"github.com/SkalaSkalolaz/llama-runner/internal/proc"
	"github.com/SkalaSkalolaz/llama-runner/internal/runner"
)

// cappedBuffer is an io.Writer that keeps at most 64 KiB and silently
// discards the rest, so a chatty git command cannot exhaust memory.
type cappedBuffer struct{ buf bytes.Buffer }

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if room := 64*1024 - c.buf.Len(); room > 0 {
		if len(p) > room {
			_, _ = c.buf.Write(p[:room])
		} else {
			_, _ = c.buf.Write(p)
		}
	}
	return len(p), nil
}

func (c *cappedBuffer) String() string { return c.buf.String() }

// Guard manages a Git branch for the auto-executor: it creates a branch
// at the start, checkpoints per wave, and rolls back on failure.
type Guard struct {
	dir    string
	r      runner.CommandRunner
	out    io.Writer
	branch string
	base   string
	last   string
}

// New creates a new Guard for dir. It verifies the directory is a Git
// repository, creates a branch, and returns the Guard ready for use.
// If dir is not a repository or the working tree is dirty (and allowDirty
// is false), it returns an error.
func New(ctx context.Context, dir string, r runner.CommandRunner, allowDirty bool, out io.Writer) (*Guard, error) {
	// 1) rev-parse --is-inside-work-tree
	if out, err := gitCmd(ctx, r, dir, "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(out) != "true" {
		return nil, i18n.Errorv(i18n.MsgVcsNotRepo, dir)
	}

	// 2) status --porcelain
	if !allowDirty {
		if s, err := gitCmd(ctx, r, dir, "status", "--porcelain"); err == nil && strings.TrimSpace(s) != "" {
			return nil, i18n.Errorv(i18n.MsgVcsDirty, dir)
		}
	}

	// 3) rev-parse HEAD → base
	base, err := gitCmd(ctx, r, dir, "rev-parse", "HEAD")
	if err != nil {
		return nil, i18n.Errorv(i18n.MsgVcsNoCommits, dir)
	}

	// 4) create branch
	branch := "llama-runner/run-" + time.Now().Format("20060102-150405")
	if _, err := gitCmd(ctx, r, dir, "checkout", "-b", branch); err != nil {
		return nil, err
	}

	return &Guard{
		dir:    dir,
		r:      r,
		out:    out,
		branch: branch,
		base:   strings.TrimSpace(base),
		last:   strings.TrimSpace(base),
	}, nil
}

// Branch returns the name of the branch this Guard created.
func (g *Guard) Branch() string { return g.branch }

// Base returns the base commit hash the Guard started from.
func (g *Guard) Base() string { return g.base }

// Checkpoint creates a commit on the current branch with the given label.
// If there are no changes to commit, it returns the current last hash
// without creating a new commit (no-op).
func (g *Guard) Checkpoint(ctx context.Context, label string) (string, error) {
	// 1) add -A
	if _, err := gitCmd(ctx, g.r, g.dir, "add", "-A"); err != nil {
		return "", err
	}

	// 2) status --porcelain → empty: no-op
	if s, err := gitCmd(ctx, g.r, g.dir, "status", "--porcelain"); err == nil && strings.TrimSpace(s) == "" {
		return g.last, nil
	}

	// 3) commit
	_, err := gitCmd(ctx, g.r, g.dir,
		"-c", "user.name=llama-runner",
		"-c", "user.email=llama-runner@localhost",
		"-c", "commit.gpgsign=false",
		"commit", "--no-verify", "-m", "llama-runner: "+label)
	if err != nil {
		return "", err
	}

	// 4) rev-parse HEAD → g.last
	last, err := gitCmd(ctx, g.r, g.dir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	g.last = strings.TrimSpace(last)
	return g.last, nil
}

// Rollback resets the branch hard to g.last (or g.base if last is empty),
// then prints any untracked files that remain.
func (g *Guard) Rollback(ctx context.Context) error {
	target := g.last
	if target == "" {
		target = g.base
	}
	if _, err := gitCmd(ctx, g.r, g.dir, "reset", "--hard", target); err != nil {
		return err
	}

	// Check for untracked files
	if s, err := gitCmd(ctx, g.r, g.dir, "status", "--porcelain"); err == nil && strings.TrimSpace(s) != "" {
		fmt.Fprint(g.out, i18n.TF(i18n.MsgVcsUntrackedLeft, s))
	}
	return nil
}

// Summary returns a git diff --stat from base to HEAD.
func (g *Guard) Summary(ctx context.Context) (string, error) {
	return gitCmd(ctx, g.r, g.dir, "diff", "--stat", g.base, "HEAD")
}

// gitCmd runs a git command via the runner and returns combined stdout+stderr.
// If ctx is nil, a background context with a 60-second timeout is used.
func gitCmd(ctx context.Context, r runner.CommandRunner, dir string, args ...string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cmd := r.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	var out cappedBuffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Start(); err != nil {
		return "", i18n.Errorf(i18n.MsgVcsCmd, err, strings.Join(args, " "))
	}

	tr := proc.Track(cmd)
	select {
	case <-tr.Done:
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-tr.Done
		return "", i18n.Errorf(i18n.MsgVcsCmd, ctx.Err(), strings.Join(args, " "))
	}

	if err := tr.Wait(); err != nil {
		return out.String(), i18n.Errorf(i18n.MsgVcsCmd, err, strings.Join(args, " "))
	}
	return out.String(), nil
}
