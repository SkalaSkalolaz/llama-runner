package orchestrator

import (
	"testing"
	"time"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0ms"},
		{time.Millisecond, "1ms"},
		{999 * time.Millisecond, "999ms"},
		{time.Second, "1.0s"},
		{3450 * time.Millisecond, "3.4s"},
		{60 * time.Second, "1m 0s"},
		{65 * time.Second, "1m 5s"},
		{time.Hour, "1h 0m 0s"},
	}
	for _, tt := range tests {
		if got := formatDuration(tt.d); got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
