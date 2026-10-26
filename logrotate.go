package main

import (
	"fmt"
	"os"
	"sync"
)

// rotatingWriter is an io.Writer that writes to a file and rotates it when
// maxSize is reached. Archives: <path>.1 — newest, <path>.<keep> — oldest.
// keep==0 — no archives (truncate). maxSize==0 — rotation disabled
// (pre-M7 behavior: O_APPEND without rotation).
//
// Why not rename the open *os.File in the parent (red zone #5):
// llama-server holds the fd, rename won't switch it to a new inode.
// So cmd.Stdout/Stderr of the child process point to this writer
// (not *os.File), os/exec creates a pipe, and the parent writes to the file itself —
// the child process knows nothing about rotation.
type rotatingWriter struct {
	path    string
	maxSize int64 // в байтах; 0 — не ротировать
	keep    int   // сколько архивов держать; 0 — без архивов
	perm    os.FileMode

	mu   sync.Mutex
	file *os.File
	size int64
}

// newRotatingWriter opens path (O_CREATE|O_WRONLY|O_APPEND, perm) and
// sets perm via Chmod (M5-code analogue that explicitly set 0600).
func newRotatingWriter(path string, maxSize int64, keep int, perm os.FileMode) (*rotatingWriter, error) {
	w := &rotatingWriter{path: path, maxSize: maxSize, keep: keep, perm: perm}
	if err := w.openLocked(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rotatingWriter) openLocked() error {
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

// Write implements io.Writer. Rotates AFTER write if size reached the limit.
// The current write always fits entirely in the current file (no data loss
// at the boundary).
func (w *rotatingWriter) Write(p []byte) (int, error) {
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

// rotateLocked called under w.mu. Closes the current file, shifts
// archives .1..<keep>, opens a new one. Idempotent to missing
// archives — Rename of a non-existent file is a no-op.
func (w *rotatingWriter) rotateLocked() error {
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

// Close closes the file. Idempotent.
func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}
