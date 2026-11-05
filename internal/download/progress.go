package download

import (
	"fmt"
	"io"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

// This module owns one responsibility: rendering the human-readable
// download progress line
//
//	downloading <name>: 45% [==========>       ] 1.2 GiB / 2.7 GiB  5.2 MiB/s  ETA 04:30
//
// with at most one update per second, independent of how many Write calls
// the transfer makes.
//
// Output format outside a terminal: the throttled updates always carry "\r",
// so a redirected stdout ends up with one long line per file instead of a
// tidy log. That is deliberate — filtering is easy (keep the text after the
// last "\r"), and it preserves the full progress history for CI logs; no
// TTY detection is done, keeping the package free of terminal dependencies.

// progressInterval is the minimum time between two rendered updates.
const progressInterval = time.Second

// progressBarWidth is the size of the ASCII progress bar in brackets.
const progressBarWidth = 20

// progressWriter wraps the download body: every Write records the byte
// count, and at most once per second a progress line is written to out.
// It does not wrap or cancel the transfer itself — Finish/Done report the
// outcome to the user.
type progressWriter struct {
	out      io.Writer
	name     string
	total    int64 // expected size in bytes, -1 when unknown
	start    time.Time
	now      func() time.Time // injectable clock for tests
	written  int64
	last     time.Time // time of the last rendered update (zero = none yet)
	rendered bool
}

// newProgressWriter creates a writer reporting progress for name to out.
// out == nil disables all output. total is the expected size, or -1 when the
// server did not report one.
func newProgressWriter(out io.Writer, name string, total int64, start time.Time) *progressWriter {
	return &progressWriter{out: out, name: name, total: total, start: start, now: time.Now}
}

// Write implements io.Writer: it counts bytes (progress only — the caller
// owns the actual file write) and renders a throttled update.
func (p *progressWriter) Write(b []byte) (int, error) {
	p.written += int64(len(b))
	p.maybeUpdate()
	return len(b), nil
}

// maybeUpdate renders an update when the interval elapsed. The very first
// chunk always renders, so a slow start still shows activity immediately.
func (p *progressWriter) maybeUpdate() {
	if p.out == nil {
		return
	}
	now := p.now()
	if !p.last.IsZero() && now.Sub(p.last) < progressInterval {
		return
	}
	p.last = now
	fmt.Fprint(p.out, "\r"+p.line(now)+"          ")
	p.rendered = true
}

// Done finishes the transfer: it clears a leftover partial line and prints
// the final "✓ <name> (<size>)" line. cause != nil prints a localized error
// instead, and a failure after a resumed transfer is reported with
// MsgErrDownloadResumeFailed — the .partial file stays on disk and the next
// run retries.
func (p *progressWriter) Done(size int64, resumed bool, cause error) {
	if p.out == nil {
		return
	}
	if p.rendered {
		fmt.Fprint(p.out, "\r\033[K")
	}
	if cause != nil {
		fmt.Fprint(p.out, i18n.TF(i18n.MsgDownloadFailed, p.name, cause))
		if resumed {
			fmt.Fprint(p.out, i18n.TF(i18n.MsgErrDownloadResumeFailed))
		}
		return
	}
	fmt.Fprint(p.out, i18n.TF(i18n.MsgDownloadDone, p.name, formatBytes(size)))
}

// line renders the current progress line (without any carriage control).
func (p *progressWriter) line(now time.Time) string {
	elapsed := now.Sub(p.start)
	var speed float64
	if elapsed > 0 {
		speed = float64(p.written) / elapsed.Seconds()
	}
	pct, bar := 0, emptyBar()
	eta := "--:--"
	if p.total > 0 {
		pct = int(p.written * 100 / p.total)
		if pct > 100 {
			pct = 100
		}
		filled := pct * progressBarWidth / 100
		bar = repeat('=', filled) + repeat(' ', progressBarWidth-filled)
		if filled > 0 {
			// move the "cursor" to the leading edge of the bar
			bar = bar[:filled-1] + ">" + bar[filled:]
		}
		bar = "[" + bar + "]"
		if speed > 0 {
			remaining := float64(p.total-p.written) / speed
			eta = formatETA(time.Duration(remaining * float64(time.Second)))
		}
	} else {
		bar = "[...]"
	}
	return fmt.Sprintf("downloading %s: %d%% %s %s / %s  %s/s  ETA %s",
		p.name, pct, bar, formatBytes(p.written), formatBytes(p.total), formatBytes(int64(speed)), eta)
}

// formatBytes renders a byte count with binary units: "1.2 GiB", "512 KiB".
// Negative sizes render as "0 B".
func formatBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// formatETA renders a remaining-time duration as MM:SS, saturating at
// 99:59 so a very slow link cannot push the field wider than five columns.
func formatETA(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d > 99*time.Minute+59*time.Second {
		return "99:59"
	}
	d = d.Round(time.Second)
	m := int(d / time.Minute)
	s := int((d % time.Minute) / time.Second)
	return fmt.Sprintf("%02d:%02d", m, s)
}

func emptyBar() string { return "[--------------------]" }

// repeat returns a string of n copies of c (n < 0 is treated as 0).
func repeat(c rune, n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]rune, n)
	for i := range b {
		b[i] = c
	}
	return string(b)
}
