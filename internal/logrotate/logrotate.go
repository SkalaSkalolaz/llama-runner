package logrotate

import (
	"fmt"
	"os"
	"sync"
)

// RotatingWriter is a goroutine-safe io.Writer that appends to a file and
// rotates it once its size reaches maxSize.
//
// Rotation keeps numbered archives: <path>.1 is the most recent rotated file
// and <path>.<keep> the oldest; when keep > 0 the oldest archive is discarded
// to make room. keep == 0 disables archiving — the file is simply removed
// (truncated) at the limit. maxSize == 0 disables rotation entirely, giving
// the pre-M7 plain O_APPEND behaviour.
//
// The child process (llama-server) is deliberately not told about rotation.
// Handing it the *os.File directly would pin the opened inode: renaming the
// file afterwards would not move the child's file descriptor to the new file
// (red zone #5). Instead cmd.Stdout/Stderr are set to this writer (not an
// *os.File), os/exec wires an OS pipe, and the parent process copies pipe
// output into the current file — so each rotation only changes where the
// parent writes, transparently to the child.
type RotatingWriter struct {
	path    string
	maxSize int64 // rotation threshold in bytes; 0 — do not rotate
	keep    int   // number of archives to keep; 0 — no archives
	perm    os.FileMode

	mu   sync.Mutex
	file *os.File
	size int64
}

// NewRotatingWriter creates a RotatingWriter for path, opening it for
// appending (O_CREATE|O_WRONLY|O_APPEND with perm) and additionally Chmod-ing
// it, because O_CREATE's mode argument is masked by the process umask — the
// explicit Chmod guarantees the requested permissions (production uses 0600).
// The size counter is seeded from the existing file, so rotation accounts for
// previously written content.
func NewRotatingWriter(path string, maxSize int64, keep int, perm os.FileMode) (*RotatingWriter, error) {
	w := &RotatingWriter{path: path, maxSize: maxSize, keep: keep, perm: perm}
	if err := w.openLocked(); err != nil {
		return nil, err
	}
	return w, nil
}

// openLocked (re)opens the log file and refreshes the size counter from its
// current length. Callers must hold w.mu. On any failure after a successful
// open the descriptor is closed before returning, so no handle leaks.
func (w *RotatingWriter) openLocked() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, w.perm)
	if err != nil {
		return err
	}
	if err := f.Chmod(w.perm); err != nil {
		_ = f.Close()
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	w.file = f
	w.size = info.Size()
	return nil
}

// Write implements io.Writer. The bytes are appended first and rotation is
// considered afterwards, so every write lands wholly in one file — maxSize is
// exceeded slightly at most, but no record is ever split across the rotation
// boundary. Writing to a closed (or just-rotated-away) writer returns
// os.ErrClosed.
func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, os.ErrClosed
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	if w.maxSize > 0 && w.size >= w.maxSize {
		_ = w.rotateLocked()
	}
	return n, err
}

// rotateLocked performs one rotation and must be called with w.mu held: it
// closes the current file, then either shifts the archive chain
// (delete <path>.<keep>, rename .N → .N+1 down to .1 → its current name, and
// finally <path> → <path>.1) or, with keep == 0, simply removes the file.
// Missing intermediate archives are tolerated — a failing rename is ignored —
// so the shift is safe even when fewer than keep archives exist. A freshly
// opened file finishes the rotation.
func (w *RotatingWriter) rotateLocked() error {
	if err := w.file.Close(); err != nil {
		return err
	}
	w.file = nil

	if w.keep > 0 {
		// Remove the oldest .<keep>, then shift to free the name.
		_ = os.Remove(fmt.Sprintf("%s.%d", w.path, w.keep))
		for i := w.keep - 1; i >= 1; i-- {
			_ = os.Rename(
				fmt.Sprintf("%s.%d", w.path, i),
				fmt.Sprintf("%s.%d", w.path, i+1),
			)
		}
		_ = os.Rename(w.path, w.path+".1")
	} else {
		// keep==0 — no archives, just truncate.
		_ = os.Remove(w.path)
	}

	return w.openLocked()
}

// Close closes the underlying file and releases it from the writer. It is
// idempotent — subsequent calls succeed without doing anything — and any
// later Write returns os.ErrClosed.
func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}
