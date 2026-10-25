package main

import (
	"fmt"
	"os"
	"sync"
)

// rotatingWriter — io.Writer, который пишет в файл и ротирует его по
// достижении maxSize. Архивы: <path>.1 — самый свежий, <path>.<keep> —
// самый старый. keep==0 — без архивов (трункейтим). maxSize==0 — ротация
// отключена (поведение как до M7: O_APPEND без ротации).
//
// Почему не переименование открытого *os.File в родителе (красная зона №5):
// llama-server держит fd, переименование не переключит его на новый inode.
// Поэтому cmd.Stdout/Stderr дочернего процесса указывают на этот writer
// (не *os.File), os/exec создаёт pipe, а родитель пишет в файл сам —
// дочерний процесс о ротации ничего не знает.
type rotatingWriter struct {
	path    string
	maxSize int64 // в байтах; 0 — не ротировать
	keep    int   // сколько архивов держать; 0 — без архивов
	perm    os.FileMode

	mu   sync.Mutex
	file *os.File
	size int64
}

// newRotatingWriter открывает path (O_CREATE|O_WRONLY|O_APPEND, perm) и
// устанавливает perm через Chmod (аналог M5-кода, который явно выставлял 0600).
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

// Write — io.Writer. Ротирует ПОСЛЕ записи, если размер достиг лимита.
// Текущая запись всегда попадает целиком в текущий файл (нет потери
// данных на границе).
func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, os.ErrClosed
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	if w.maxSize > 0 && w.size >= w.maxSize {
		// Ошибку ротации не возвращаем: данные уже записаны, следующий
		// Write попробует ещё раз. Так лог не теряется из-за transient
		// ошибки rename.
		_ = w.rotateLocked()
	}
	return n, err
}

// rotateLocked вызывается под w.mu. Закрывает текущий файл, сдвигает
// архивы .1..<keep>, открывает новый. Идемпотентна к отсутствующим
// архивам — Rename по несуществующему просто no-op.
func (w *rotatingWriter) rotateLocked() error {
	if err := w.file.Close(); err != nil {
		return err
	}
	w.file = nil

	if w.keep > 0 {
		// Удаляем самый старый .<keep>, чтобы после сдвига освободить имя.
		_ = os.Remove(fmt.Sprintf("%s.%d", w.path, w.keep))
		for i := w.keep - 1; i >= 1; i-- {
			_ = os.Rename(
				fmt.Sprintf("%s.%d", w.path, i),
				fmt.Sprintf("%s.%d", w.path, i+1),
			)
		}
		_ = os.Rename(w.path, w.path+".1")
	} else {
		// keep==0 — без архивов, просто трункейтим.
		_ = os.Remove(w.path)
	}

	return w.openLocked()
}

// Close закрывает файл. Идемпотентен.
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
