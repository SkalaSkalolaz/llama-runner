package app

import (
	"bytes"
	"io"
	"sync"
	"time"
)

// timePrefixWriter wraps an io.Writer and prefixes every output line with
// the local wall-clock time ("15:04:05" plus one space). Bytes are buffered
// until a newline, so the prefix appears once per LINE, not per Write;
// Flush emits a trailing partial line with its own prefix so nothing is
// lost. The mutex serializes concurrent wave writes. Only the orchestrator
// phase is wrapped — planning, the plan listing and -plan-only JSON stay
// prefix-free.
type timePrefixWriter struct {
	dst io.Writer
	now func() time.Time // injectable clock for tests
	mu  sync.Mutex
	buf []byte
}

func newTimePrefixWriter(dst io.Writer) *timePrefixWriter {
	return &timePrefixWriter{dst: dst, now: time.Now}
}

// Write implements io.Writer. Complete lines are emitted immediately;
// a partial tail is buffered until the next Write or Flush.
func (w *timePrefixWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		if err := w.emit(w.buf[:i+1]); err != nil {
			return 0, err
		}
		w.buf = w.buf[i+1:]
	}
	return len(p), nil
}

// Flush emits the buffered partial line (if any) with a time prefix.
func (w *timePrefixWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) == 0 {
		return nil
	}
	line := w.buf
	w.buf = nil
	return w.emit(line)
}

// emit writes prefix+line as ONE Write, so the pair cannot be torn apart
// by a concurrent writer sharing the destination. Caller holds w.mu.
func (w *timePrefixWriter) emit(line []byte) error {
	out := append([]byte(w.now().Format("15:04:05")+" "), line...)
	_, err := w.dst.Write(out)
	return err
}
