package planner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// planServer answers chat/completions with the given replies in order and
// records every request body it saw.
func planServer(t *testing.T, replies ...string) (*httptest.Server, func() [][]byte) {
	t.Helper()
	var mu sync.Mutex
	var bodies [][]byte
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, b)
		reply := replies[len(replies)-1]
		if n < len(replies) {
			reply = replies[n]
		}
		n++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{
				"role": "assistant", "content": reply}}},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, func() [][]byte {
		mu.Lock()
		defer mu.Unlock()
		return append([][]byte(nil), bodies...)
	}
}

const goodPlan = `{"summary":"s","steps":[{"id":"1","role":"coder","goal":"g","alias":"auto"}]}`

func optsFor(url string) Options {
	return Options{
		BaseURL: url, Alias: "auto",
		Models: []Model{
			{Alias: "coder", Description: "code", Context: 131072, Parallel: 1},
			{Alias: "auto", Description: "planner", Context: 32768, Parallel: 1},
		},
	}
}

func TestBuild_FirstAnswerValid(t *testing.T) {
	srv, bodies := planServer(t, goodPlan)
	res, err := New(optsFor(srv.URL)).Build(context.Background(), "do it")
	if err != nil {
		t.Fatal(err)
	}
	if res.Attempts != 1 || res.Repaired || res.Fallback {
		t.Errorf("result = %+v, want one clean attempt", res)
	}
	if len(res.Plan.Steps) != 1 || res.Plan.Steps[0].Alias != "auto" {
		t.Errorf("plan = %+v", res.Plan)
	}
	var req chatRequest
	if err := json.Unmarshal(bodies()[0], &req); err != nil {
		t.Fatal(err)
	}
	if req.Model != "auto" || req.Temperature != 0 || req.Stream {
		t.Errorf("request = %+v, want model=auto, temp=0, stream=false", req)
	}
	if len(req.Messages) != 2 || req.Messages[1].Content != "do it" {
		t.Errorf("messages = %+v", req.Messages)
	}
	sys := req.Messages[0].Content
	if !strings.Contains(sys, "coder") || !strings.Contains(sys, "131072") {
		t.Errorf("catalog missing from the system prompt: %s", sys)
	}
	// Sorted catalog: auto precedes coder.
	if strings.Index(sys, "- auto") > strings.Index(sys, "- coder") {
		t.Error("catalog is not sorted by alias")
	}
}

func TestBuild_RepairRound(t *testing.T) {
	srv, bodies := planServer(t, `{"steps":[{"id":"1","role":"coder","goal":"g","alias":"ghost"}]}`, goodPlan)
	res, err := New(optsFor(srv.URL)).Build(context.Background(), "do it")
	if err != nil {
		t.Fatal(err)
	}
	if res.Attempts != 2 || !res.Repaired || res.Fallback {
		t.Errorf("result = %+v, want a repaired second attempt", res)
	}
	if len(bodies()) != 2 {
		t.Fatalf("calls = %d, want 2", len(bodies()))
	}
	var req chatRequest
	_ = json.Unmarshal(bodies()[1], &req)
	if len(req.Messages) != 4 {
		t.Fatalf("repair call must carry the bad answer and the error: %+v", req.Messages)
	}
	if !strings.Contains(req.Messages[3].Content, "not in the catalog") {
		t.Errorf("repair prompt lost the validation error: %q", req.Messages[3].Content)
	}
}

func TestBuild_FallbackAfterTwoBadAnswers(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANG", "C")
	srv, _ := planServer(t, "sorry, no JSON here")
	res, err := New(optsFor(srv.URL)).Build(context.Background(), "do it")
	if err != nil {
		t.Fatalf("a bad answer is a warning, not an error: %v", err)
	}
	if !res.Fallback || res.LastErr == nil {
		t.Fatalf("result = %+v, want a labelled fallback", res)
	}
	if len(res.Plan.Steps) != 1 || res.Plan.Steps[0].Goal != "do it" || res.Plan.Steps[0].Alias != "auto" {
		t.Errorf("fallback plan = %+v", res.Plan)
	}
}

func TestBuild_HTTPErrorIsFatal(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANG", "C")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	if _, err := New(optsFor(srv.URL)).Build(context.Background(), "x"); err == nil {
		t.Fatal("an upstream HTTP failure must be returned as an error")
	}
}

func TestBuild_EmptyChoicesIsFatal(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANG", "C")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()
	if _, err := New(optsFor(srv.URL)).Build(context.Background(), "x"); err == nil {
		t.Fatal("an empty reply must be an error, not a silent fallback")
	}
}

func TestBuild_ContextCancelled(t *testing.T) {
	srv, _ := planServer(t, goodPlan)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New(optsFor(srv.URL)).Build(ctx, "x"); err == nil {
		t.Fatal("a cancelled context must abort the call")
	}
}
