package app

import (
	"io"
	"os"
	"strings"
)

// isTerminalFile reports whether f is connected to a terminal, detected by
// the character-device mode bit on its stat info. A nil file or a stat
// failure counts as "not a terminal", which is the safe answer for
// redirect decisions.
func isTerminalFile(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// pathContainsSegment reports whether seg appears as one of the
// colon-separated entries of an environment path variable such as
// LD_LIBRARY_PATH. Used to avoid prepending a directory that is already
// present.
func pathContainsSegment(path, seg string) bool {
	for _, p := range strings.Split(path, ":") {
		if p == seg {
			return true
		}
	}
	return false
}

// openTTY opens /dev/tty read-write and returns the handle, or nil when it
// cannot be opened — typical reasons are Windows, a process without a
// controlling terminal, or a container image with /dev/tty trimmed away.
// The caller treats nil as "no terminal available" and degrades instead of
// failing.
func openTTY() *os.File {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil
	}
	return tty
}

// shouldRedirectClaudeToTTY decides whether Claude Code's stdio must be
// reconnected to /dev/tty. Claude Code is an interactive TUI: if its stdout
// or stderr is a pipe or file it falls back to non-interactive --print mode
// and dies without a prompt. That happens whenever the parent's streams were
// redirected (e.g. `llama-runner -v 2>&1 | tee log`).
//
// The rule: redirect only when both streams are real *os.File handles and at
// least one of them is not a terminal. When either stream is something else
// (bytes.Buffer and friends, as used in tests) the answer is false, so tests
// never grab a real terminal.
func shouldRedirectClaudeToTTY(stdout, stderr io.Writer) bool {
	sf, ok1 := stdout.(*os.File)
	ef, ok2 := stderr.(*os.File)
	if !ok1 || !ok2 {
		return false
	}
	return !isTerminalFile(sf) || !isTerminalFile(ef)
}
