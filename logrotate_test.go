package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingWriter_NoRotationUnderLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	w, err := newRotatingWriter(path, 1024, 3, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	payload := []byte("hello\n")
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, payload) {
		t.Errorf("data = %q, want %q", data, payload)
	}
	// Никаких .1 нет.
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Errorf(".1 exists, want absent: %v", err)
	}
}

func TestRotatingWriter_RotatesAtLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	// maxSize=10: запись 10 байт уже триггерит ротацию.
	w, err := newRotatingWriter(path, 10, 3, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	if _, err := w.Write([]byte("0123456789")); err != nil {
		t.Fatal(err)
	}

	// После ротации: log.1 содержит "0123456789", log — пустой (открыт заново).
	old, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatalf("read .1: %v", err)
	}
	if string(old) != "0123456789" {
		t.Errorf(".1 = %q, want %q", old, "0123456789")
	}
	cur, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if len(cur) != 0 {
		t.Errorf("current log not empty: %q", cur)
	}
}

func TestRotatingWriter_KeepsOnlyN(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	// maxSize=1: ротация на каждой записи. keep=2 → только .1 и .2.
	w, err := newRotatingWriter(path, 1, 2, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	for i := 0; i < 5; i++ {
		if _, err := w.Write([]byte{'A' + byte(i)}); err != nil {
			t.Fatal(err)
		}
	}

	// Должны быть .1 и .2; .3 не должно быть.
	for _, name := range []string{".1", ".2"} {
		if _, err := os.Stat(path + name); err != nil {
			t.Errorf("%s missing: %v", name, err)
		}
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Errorf(".3 exists, want absent")
	}

	// .1 — самый свежий архив (последняя ротированная запись 'E'),
	// .2 — старше (запись 'D'). Классическая семантика logrotate.
	if data, _ := os.ReadFile(path + ".1"); string(data) != "E" {
		t.Errorf(".1 = %q, want %q", data, "E")
	}
	if data, _ := os.ReadFile(path + ".2"); string(data) != "D" {
		t.Errorf(".2 = %q, want %q", data, "D")
	}
}

func TestRotatingWriter_PreservesPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	w, err := newRotatingWriter(path, 5, 2, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	if _, err := w.Write([]byte("123456")); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"", ".1"} {
		info, err := os.Stat(path + name)
		if err != nil {
			t.Fatalf("stat %q: %v", name, err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("%q perm = %o, want 0600", name, perm)
		}
	}
}

func TestRotatingWriter_NoKeepTruncates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	w, err := newRotatingWriter(path, 5, 0, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	if _, err := w.Write([]byte("123456")); err != nil {
		t.Fatal(err)
	}
	// keep=0 → .1 нет, старый файл удалён, новый открыт.
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Errorf(".1 exists, want absent (keep=0)")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("log not empty after truncation: %q", data)
	}
}

func TestRotatingWriter_DisabledByZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	w, err := newRotatingWriter(path, 0, 3, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	// Пишем много больше «лимита»: maxSize=0 — ротации нет.
	payload := strings.Repeat("x", 1024)
	if _, err := w.Write([]byte(payload)); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != payload {
		t.Errorf("data length = %d, want %d", len(data), len(payload))
	}
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Errorf(".1 exists, want absent (maxSize=0)")
	}
}

func TestRotatingWriter_CloseIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	w, err := newRotatingWriter(path, 0, 0, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("first close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("second close: %v", err)
	}
	// Write после Close — ошибка, не паника.
	if _, err := w.Write([]byte("x")); err == nil {
		t.Error("Write после Close: err == nil")
	}
}

func TestRotatingWriter_AppendsToExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	if err := os.WriteFile(path, []byte("pre-existing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := newRotatingWriter(path, 0, 0, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	if _, err := w.Write([]byte("appended\n")); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "pre-existing\nappended\n" {
		t.Errorf("data = %q", data)
	}
}
