// Этот файл реализует живой вывод шагов: каждый шаг получает свой
// писатель, который режет поток на строки и печатает их под общим
// мьютексом с префиксом id шага — шаги одной волны идут параллельно,
// и строки не должны перемешиваться посередине.
package orchestrator

import (
	"bytes"
	"fmt"
	"io"
	"sync"
)

// stepStreamer serializes live step output lines from concurrent waves
// into one destination.
type stepStreamer struct {
	dst io.Writer
	mu  sync.Mutex
}

func newStepStreamer(dst io.Writer) *stepStreamer { return &stepStreamer{dst: dst} }

// writer returns the per-step line writer prefixed with the step ID.
func (s *stepStreamer) writer(stepID string) *stepStreamWriter {
	return &stepStreamWriter{s: s, prefix: "[" + stepID + "] "}
}

// stepStreamWriter buffers partial lines and emits each completed line
// under the streamer mutex. Write errors are swallowed: streaming is
// observation, and a broken console must not fail the step.
type stepStreamWriter struct {
	s      *stepStreamer
	prefix string
	buf    []byte
}

func (w *stepStreamWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := w.buf[:i+1]
		w.s.mu.Lock()
		_, _ = fmt.Fprintf(w.s.dst, "%s%s", w.prefix, line)
		w.s.mu.Unlock()
		w.buf = w.buf[i+1:]
	}
	return len(p), nil
}

// flush emits the buffered partial line, if any, with a newline appended.
func (w *stepStreamWriter) flush() {
	if len(w.buf) == 0 {
		return
	}
	w.s.mu.Lock()
	_, _ = fmt.Fprintf(w.s.dst, "%s%s\n", w.prefix, w.buf)
	w.s.mu.Unlock()
	w.buf = nil
}
