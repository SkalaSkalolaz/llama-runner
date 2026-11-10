package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/config"
)

func TestProfileSwapper_IncDec_Wait(t *testing.T) {
	s := &profileSwapper{}
	s.IncInflight()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := s.WaitInflight(ctx); err == nil {
		t.Fatal("expected timeout")
	}
	s.DecInflight()
	if err := s.WaitInflight(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestProfileSwapper_SwapTo_SameAlias_Noop(t *testing.T) {
	s := &profileSwapper{
		currentAlias: "a",
		profiles:     map[string]*config.Config{"a": {Alias: "a", Host: "127.0.0.1", Port: "8080"}},
	}
	url, err := s.SwapTo(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(url, ":") {
		t.Errorf("url = %q", url)
	}
}

func TestProfileSwapper_SwapTo_Unknown(t *testing.T) {
	s := &profileSwapper{profiles: map[string]*config.Config{}}
	if _, err := s.SwapTo(context.Background(), "nope"); err == nil {
		t.Fatal("expected error")
	}
}

func TestProfileSwapper_Stop_PreventsSwap(t *testing.T) {
	s := &profileSwapper{profiles: map[string]*config.Config{"b": {Alias: "b"}}}
	s.Stop()
	if _, err := s.SwapTo(context.Background(), "b"); err == nil {
		t.Fatal("expected error after Stop")
	}
}
