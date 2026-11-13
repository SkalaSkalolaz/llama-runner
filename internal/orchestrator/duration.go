package orchestrator

import (
	"fmt"
	"time"
)

// formatDuration renders d for the terminal:
//
//	< 1s   → milliseconds          ("250ms", "0ms")
//	< 1min → seconds, one decimal, truncated ("3.4s"; 3450ms → "3.4s")
//	< 1h   → minutes and seconds   ("6m 32s")
//	>= 1h  → hours, minutes, seconds ("1h 2m 3s")
//
// Negative values are clamped to zero: a clock hiccup must not print "-3ms".
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		tenths := d.Milliseconds() / 100 // truncate, never round up
		return fmt.Sprintf("%d.%ds", tenths/10, tenths%10)
	case d < time.Hour:
		return fmt.Sprintf("%dm %ds", d/time.Minute, (d%time.Minute)/time.Second)
	default:
		return fmt.Sprintf("%dh %dm %ds",
			d/time.Hour, (d%time.Hour)/time.Minute, (d%time.Minute)/time.Second)
	}
}
