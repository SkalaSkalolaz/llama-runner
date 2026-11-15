package planner

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestBuild_ProjectBlockInPrompt(t *testing.T) {
	srv, bodies := planServer(t, goodPlan)
	opts := optsFor(srv.URL)
	opts.Project = "dir: /tmp/x\nstack: Go (markers: go.mod)\n"
	if _, err := New(opts).Build(context.Background(), "do it"); err != nil {
		t.Fatal(err)
	}
	var req chatRequest
	if err := json.Unmarshal(bodies()[0], &req); err != nil {
		t.Fatal(err)
	}
	sys := req.Messages[0].Content
	if !strings.Contains(sys, "Project context") || !strings.Contains(sys, "stack: Go") {
		t.Errorf("project block missing: %s", sys)
	}
}

func TestBuild_NoProjectBlockWhenEmpty(t *testing.T) {
	srv, bodies := planServer(t, goodPlan)
	if _, err := New(optsFor(srv.URL)).Build(context.Background(), "do it"); err != nil {
		t.Fatal(err)
	}
	var req chatRequest
	_ = json.Unmarshal(bodies()[0], &req)
	if strings.Contains(req.Messages[0].Content, "Project context") {
		t.Error("project block leaked with an empty Project")
	}
}

func TestBuild_ExtraCheckTriggersRepair(t *testing.T) {
	srv, bodies := planServer(t, goodPlan, goodPlan)
	opts := optsFor(srv.URL)
	calls := 0
	opts.ExtraCheck = func(Plan) error {
		calls++
		if calls == 1 {
			return errors.New("verify command not found: nosuchtool")
		}
		return nil
	}
	res, err := New(opts).Build(context.Background(), "do it")
	if err != nil {
		t.Fatal(err)
	}
	if res.Attempts != 2 || !res.Repaired || res.Fallback {
		t.Errorf("result = %+v, want a repaired second attempt", res)
	}
	var req chatRequest
	_ = json.Unmarshal(bodies()[1], &req)
	if len(req.Messages) != 4 || !strings.Contains(req.Messages[3].Content, "nosuchtool") {
		t.Errorf("repair prompt lost the ExtraCheck error: %+v", req.Messages)
	}
}

func TestBuild_ExtraCheckNilKeepsBehaviour(t *testing.T) {
	srv, _ := planServer(t, goodPlan)
	res, err := New(optsFor(srv.URL)).Build(context.Background(), "do it")
	if err != nil {
		t.Fatal(err)
	}
	if res.Attempts != 1 || res.Repaired {
		t.Errorf("result = %+v, want one clean attempt", res)
	}
}
