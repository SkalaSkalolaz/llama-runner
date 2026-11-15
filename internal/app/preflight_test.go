package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// planSeqServer answers with replies in order and records request bodies.
func planSeqServer(t *testing.T, replies ...string) (*httptest.Server, func() [][]byte) {
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
			"choices": []any{map[string]any{"message": map[string]any{"content": reply}}},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, func() [][]byte {
		mu.Lock()
		defer mu.Unlock()
		return append([][]byte(nil), bodies...)
	}
}

func systemPromptOf(t *testing.T, body []byte) string {
	t.Helper()
	var req struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatal(err)
	}
	if len(req.Messages) == 0 {
		t.Fatal("no messages in the request")
	}
	return req.Messages[0].Content
}

// missingToolRunner fails LookPath only for "nosuchtool"; everything else
// resolves, so the repaired plan passes the second preflight round.
type missingToolRunner struct{ scriptCmdRunner }

func (r missingToolRunner) LookPath(file string) (string, error) {
	if file == "nosuchtool" {
		return "", exec.ErrNotFound
	}
	return "/usr/bin/" + file, nil
}

func TestBuildPlan_RepoMapReachesPrompt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := planOnlyCfg()
	cfg.RepoMap = true
	cfg.ClaudeDir = dir
	srv, bodies := planSeqServer(t, `{"steps":[{"id":"1","role":"coder","goal":"g","alias":"coder"}]}`)
	var out, errb bytes.Buffer
	if _, ok := buildPlan(context.Background(), cfg, t.TempDir(), srv.URL,
		scriptCmdRunner{script: "true"}, &out, &errb); !ok {
		t.Fatalf("buildPlan failed: %s", errb.String())
	}
	sys := systemPromptOf(t, bodies()[0])
	if !strings.Contains(sys, "Project context") || !strings.Contains(sys, "Go") {
		t.Errorf("project block missing from system prompt: %s", sys)
	}
}

func TestBuildPlan_PreflightRepairsPlan(t *testing.T) {
	cfg := planOnlyCfg()
	cfg.Preflight = true
	cfg.ClaudeDir = t.TempDir()
	bad := `{"steps":[{"id":"1","role":"coder","goal":"g","alias":"coder","verify":"nosuchtool x"}]}`
	good := `{"steps":[{"id":"1","role":"coder","goal":"g","alias":"coder","verify":"go build ./..."}]}`
	srv, _ := planSeqServer(t, bad, good)
	var out, errb bytes.Buffer
	plan, ok := buildPlan(context.Background(), cfg, t.TempDir(), srv.URL,
		missingToolRunner{scriptCmdRunner{script: "true"}}, &out, &errb)
	if !ok {
		t.Fatalf("buildPlan failed: %s", errb.String())
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Verify != "go build ./..." {
		t.Errorf("plan was not repaired: %+v", plan.Steps)
	}
	if !strings.Contains(errb.String(), "[preflight]") {
		t.Errorf("preflight finding missing from stderr: %s", errb.String())
	}
}

func TestBuildPlan_FlagsOffKeepsPrompt(t *testing.T) {
	srv, bodies := planSeqServer(t, `{"steps":[{"id":"1","role":"coder","goal":"g","alias":"coder"}]}`)
	var out, errb bytes.Buffer
	if _, ok := buildPlan(context.Background(), planOnlyCfg(), t.TempDir(), srv.URL,
		scriptCmdRunner{script: "true"}, &out, &errb); !ok {
		t.Fatalf("buildPlan failed: %s", errb.String())
	}
	if sys := systemPromptOf(t, bodies()[0]); strings.Contains(sys, "Project context") {
		t.Errorf("project block leaked with flags off: %s", sys)
	}
}
