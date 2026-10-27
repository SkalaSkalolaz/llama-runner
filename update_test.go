package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestUpdateLlamaCPP_NoUpstream(t *testing.T) {
	t.Setenv("LC_ALL", "ru_RU.UTF-8") // сообщение локализовано
	dir := t.TempDir()
	runner := fakeRunner{scripts: map[string]string{
		"git rev-parse": "exit 1", // нет upstream
	}}
	var buf bytes.Buffer
	err := updateLlamaCPP(runner, dir, t.TempDir(), "61", true, false, &buf)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(buf.String(), "нет upstream") {
		t.Errorf("missing 'нет upstream' in output: %s", buf.String())
	}
}

func TestUpdateLlamaCPP_PullFails(t *testing.T) {
	dir := t.TempDir()
	runner := fakeRunner{scripts: map[string]string{
		"git rev-parse": "exit 0",
		"git pull":      "exit 1",
	}}
	var buf bytes.Buffer
	err := updateLlamaCPP(runner, dir, t.TempDir(), "61", true, false, &buf)
	if err == nil || !strings.Contains(err.Error(), "git pull") {
		t.Errorf("err = %v, want containing 'git pull'", err)
	}
}

func TestUpdateLlamaCPP_ConfigureFails(t *testing.T) {
	dir := t.TempDir()
	runner := fakeRunner{scripts: map[string]string{
		"git rev-parse": "exit 0",
		"git pull":      "exit 0",
		"cmake -S":      "exit 1",
	}}
	var buf bytes.Buffer
	err := updateLlamaCPP(runner, dir, t.TempDir(), "61", true, false, &buf)
	if err == nil || !strings.Contains(err.Error(), "cmake configure") {
		t.Errorf("err = %v, want containing 'cmake configure'", err)
	}
}

func TestUpdateLlamaCPP_BuildFails(t *testing.T) {
	dir := t.TempDir()
	runner := fakeRunner{scripts: map[string]string{
		"git rev-parse": "exit 0",
		"git pull":      "exit 0",
		"cmake -S":      "exit 0",
		"cmake --build": "exit 1",
	}}
	var buf bytes.Buffer
	err := updateLlamaCPP(runner, dir, t.TempDir(), "61", true, false, &buf)
	if err == nil || !strings.Contains(err.Error(), "cmake build") {
		t.Errorf("err = %v, want containing 'cmake build'", err)
	}
}

func TestUpdateLlamaCPP_InstallFails(t *testing.T) {
	dir := t.TempDir()
	runner := fakeRunner{scripts: map[string]string{
		"git rev-parse":   "exit 0",
		"git pull":        "exit 0",
		"cmake -S":        "exit 0",
		"cmake --build":   "exit 0",
		"cmake --install": "exit 1",
	}}
	var buf bytes.Buffer
	err := updateLlamaCPP(runner, dir, t.TempDir(), "61", true, false, &buf)
	if err == nil || !strings.Contains(err.Error(), "cmake install") {
		t.Errorf("err = %v, want containing 'cmake install'", err)
	}
}
