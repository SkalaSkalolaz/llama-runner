package planner

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	const obj = `{"steps":[{"id":"1","role":"coder","goal":"g","alias":"auto"}]}`
	tests := []struct {
		name    string
		reply   string
		want    int
		wantErr bool
	}{
		{"plain", obj, 1, false},
		{"fenced", "```json\n" + obj + "\n```", 1, false},
		{"prose around", "Here is the plan:\n" + obj + "\nHope it helps.", 1, false},
		{"thinking trace", "<think>hmm</think>\n" + obj, 1, false},
		{"bare array", `[{"id":"1","role":"coder","goal":"g","alias":"auto"}]`, 1, false},
		{"unknown fields", `{"plan_version":2,"steps":[{"id":"1","role":"coder","goal":"g","alias":"auto","x":1}]}`, 1, false},
		{"no json", "I cannot do that.", 0, true},
		{"broken json", `{"steps":[`, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Parse(tt.reply)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if err == nil && len(p.Steps) != tt.want {
				t.Errorf("steps = %d, want %d", len(p.Steps), tt.want)
			}
		})
	}
}

func validPlan() Plan {
	return Plan{Steps: []Step{
		{ID: "1", Role: "architect", Goal: "design", Alias: "auto"},
		{ID: "2", Role: "coder", Goal: "implement", Alias: "coder", DependsOn: []string{"1"}},
	}}
}

func TestValidate_OKAndNormalises(t *testing.T) {
	p := Plan{Summary: "  s  ", Steps: []Step{
		{ID: " 1 ", Role: " coder ", Goal: " g ", Alias: "AUTO",
			DependsOn: []string{"", "2", "2"}, Artifact: " a ", Verify: " v "},
		{ID: "2", Role: "tester", Goal: "t", Alias: "coder"},
	}}
	if err := p.Validate([]string{"auto", "coder"}); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	s := p.Steps[0]
	if s.ID != "1" || s.Role != "coder" || s.Goal != "g" || s.Alias != "auto" {
		t.Errorf("not trimmed/canonicalised: %+v", s)
	}
	if len(s.DependsOn) != 1 || s.DependsOn[0] != "2" {
		t.Errorf("deps = %v, want [2] (empty dropped, duplicate collapsed)", s.DependsOn)
	}
	if s.Artifact != "a" || s.Verify != "v" || p.Summary != "s" {
		t.Errorf("artifact/verify/summary not trimmed: %+v", s)
	}
}

func TestValidate_Errors(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Plan)
		wantSub string
	}{
		{"empty", func(p *Plan) { p.Steps = nil }, "no steps"},
		{"no id", func(p *Plan) { p.Steps[0].ID = " " }, "empty id"},
		{"dup id", func(p *Plan) { p.Steps[1].ID = "1" }, "duplicate step id"},
		{"no role", func(p *Plan) { p.Steps[0].Role = "" }, "role"},
		{"no goal", func(p *Plan) { p.Steps[0].Goal = "" }, "goal"},
		{"no alias", func(p *Plan) { p.Steps[0].Alias = "" }, "alias"},
		{"unknown alias", func(p *Plan) { p.Steps[0].Alias = "ghost" }, "not in the catalog"},
		{"unknown dep", func(p *Plan) { p.Steps[1].DependsOn = []string{"9"} }, "not in the plan"},
		{"self dep", func(p *Plan) { p.Steps[0].DependsOn = []string{"1"} }, "depends on itself"},
		{"cycle", func(p *Plan) { p.Steps[0].DependsOn = []string{"2"} }, "cycle"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LC_ALL", "C")
			t.Setenv("LANG", "C")
			p := validPlan()
			tt.mutate(&p)
			err := p.Validate([]string{"auto", "coder"})
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("err = %v, want it to contain %q", err, tt.wantSub)
			}
		})
	}
}

func TestValidate_TooManySteps(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANG", "C")
	p := Plan{}
	for i := 0; i <= MaxSteps; i++ {
		p.Steps = append(p.Steps, Step{
			ID: string(rune('a'+i%26)) + strconvI(i), Role: "r", Goal: "g", Alias: "auto",
		})
	}
	if err := p.Validate([]string{"auto"}); err == nil {
		t.Fatal("expected the step-count cap to fire")
	}
}

// strconvI keeps the test free of an extra import alias.
func strconvI(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
