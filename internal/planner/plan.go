// Package planner turns a user task into a validated execution plan using
// the local "auto" model as the planner. Three responsibilities, one file
// each: the plan schema and its validation (plan.go), the prompt built from
// the worker catalog (prompt.go), and the conversation with llama-server
// that produces the plan (planner.go).
//
// The package deliberately knows nothing about config.Config, cli or the
// proxy: the caller describes the catalog as []Model and gets a Plan back.
// That keeps it testable against an httptest server and reusable by both
// the headless (-plan-only) flow and, later, the interactive one.
package planner

import (
	"encoding/json"
	"strings"

	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
)

// MaxSteps caps how large a plan may be. A local planner that runs away and
// emits hundreds of micro-steps would make the executor swap models for
// hours; the cap turns that into a validation error the repair round can fix.
const MaxSteps = 32

// Step is one unit of work assigned to one model of the catalog.
type Step struct {
	// ID identifies the step inside the plan; DependsOn refers to it.
	ID string `json:"id"`
	// Role is the adaptive role of the agent (architect, coder, tester,
	// reviewer, researcher, …). Free-form on purpose: roles are chosen per
	// task, not from a fixed enum.
	Role string `json:"role"`
	// Goal is what the step must achieve, in one or two sentences.
	Goal string `json:"goal"`
	// Alias names the catalog model that executes the step.
	Alias string `json:"alias"`
	// DependsOn lists step IDs that must finish first. Steps without
	// dependencies may run in parallel (within the model's slot limit).
	DependsOn []string `json:"depends_on,omitempty"`
	// Artifact is the verifiable result of the step (file, report, test).
	Artifact string `json:"artifact,omitempty"`
	// Verify says how to check the artifact exists and is correct.
	Verify string `json:"verify,omitempty"`
}

// Plan is the planner model's answer.
type Plan struct {
	Summary string `json:"summary,omitempty"`
	Steps   []Step `json:"steps"`
}

// Parse extracts a plan from a raw model reply. Local models wrap JSON in
// markdown fences, prepend prose or a thinking trace, and sometimes answer
// with a bare array of steps; all of those are accepted. Unknown JSON fields
// are ignored, matching the project's converter convention.
func Parse(reply string) (Plan, error) {
	body := extractJSON(reply)
	if body == "" {
		return Plan{}, i18n.Errorv(i18n.MsgErrPlanNoJSON)
	}
	var p Plan
	if strings.HasPrefix(body, "[") {
		var steps []Step
		if err := json.Unmarshal([]byte(body), &steps); err != nil {
			return Plan{}, i18n.Errorf(i18n.MsgErrPlanJSON, err)
		}
		p.Steps = steps
		return p, nil
	}
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		return Plan{}, i18n.Errorf(i18n.MsgErrPlanJSON, err)
	}
	return p, nil
}

// Validate checks the plan against the worker catalog and normalises it in
// place: fields are trimmed, aliases are canonicalised to the catalog
// spelling (the model may answer "AUTO" for "auto"), dependency lists are
// trimmed and de-duplicated.
//
// Rules: 1..MaxSteps steps; every step has a unique non-empty id and a
// non-empty role, goal and alias; every alias exists in the catalog; every
// dependency names another step of this plan (forward references are fine),
// never the step itself; the dependency graph is acyclic.
func (p *Plan) Validate(catalog []string) error {
	if len(p.Steps) == 0 {
		return i18n.Errorv(i18n.MsgErrPlanEmpty)
	}
	if len(p.Steps) > MaxSteps {
		return i18n.Errorv(i18n.MsgErrPlanTooMany, len(p.Steps), MaxSteps)
	}
	canonical := make(map[string]string, len(catalog))
	for _, a := range catalog {
		canonical[strings.ToLower(strings.TrimSpace(a))] = a
	}
	p.Summary = strings.TrimSpace(p.Summary)

	ids := make(map[string]bool, len(p.Steps))
	for i := range p.Steps {
		s := &p.Steps[i]
		s.ID = strings.TrimSpace(s.ID)
		s.Role = strings.TrimSpace(s.Role)
		s.Goal = strings.TrimSpace(s.Goal)
		s.Alias = strings.TrimSpace(s.Alias)
		s.Artifact = strings.TrimSpace(s.Artifact)
		s.Verify = strings.TrimSpace(s.Verify)
		if s.ID == "" {
			return i18n.Errorv(i18n.MsgErrPlanStepID, i+1)
		}
		if ids[s.ID] {
			return i18n.Errorv(i18n.MsgErrPlanDupID, s.ID)
		}
		ids[s.ID] = true
		for _, f := range []struct{ name, value string }{
			{"role", s.Role}, {"goal", s.Goal}, {"alias", s.Alias},
		} {
			if f.value == "" {
				return i18n.Errorv(i18n.MsgErrPlanStepField, s.ID, f.name)
			}
		}
		want, ok := canonical[strings.ToLower(s.Alias)]
		if !ok {
			return i18n.Errorv(i18n.MsgErrPlanAlias, s.ID, s.Alias)
		}
		s.Alias = want
	}

	// Dependencies are resolved in a second pass so a step may depend on a
	// later one in the list.
	for i := range p.Steps {
		s := &p.Steps[i]
		seen := make(map[string]bool, len(s.DependsOn))
		deps := make([]string, 0, len(s.DependsOn))
		for _, d := range s.DependsOn {
			d = strings.TrimSpace(d)
			if d == "" || seen[d] {
				continue
			}
			if d == s.ID {
				return i18n.Errorv(i18n.MsgErrPlanSelfDep, s.ID)
			}
			if !ids[d] {
				return i18n.Errorv(i18n.MsgErrPlanDep, s.ID, d)
			}
			seen[d] = true
			deps = append(deps, d)
		}
		s.DependsOn = deps
	}
	return p.checkAcyclic()
}


// checkAcyclic delegates to Order, which reports a cycle when it cannot
// emit every step.
func (p *Plan) checkAcyclic() error {
	_, err := p.Order()
	return err
}

// Order returns the steps in a deterministic topological order: every
// dependency precedes the steps that need it. Among the steps that are
// ready, the one earliest in the plan wins, so a plan the planner already
// wrote in execution order comes back unchanged. Validate must have passed
// first; with it, a cycle or a dangling dependency is impossible here.
func (p Plan) Order() ([]Step, error) {
	done := make(map[string]bool, len(p.Steps))
	out := make([]Step, 0, len(p.Steps))
	for len(out) < len(p.Steps) {
		progress := false
		for _, s := range p.Steps {
			if done[s.ID] {
				continue
			}
			ready := true
			for _, d := range s.DependsOn {
				if !done[d] {
					ready = false
					break
				}
			}
			if ready {
				done[s.ID] = true
				out = append(out, s)
				progress = true
				break
			}
		}
		if !progress {
			return nil, i18n.Errorv(i18n.MsgErrPlanCycle)
		}
	}
	return out, nil
}

// extractJSON pulls the outermost JSON object or array out of a reply:
// markdown fences are stripped first, then everything before the first
// opener and after the last matching closer is dropped (prose, thinking
// traces, trailing commentary).
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		}
		if j := strings.LastIndex(s, "```"); j >= 0 {
			s = s[:j]
		}
		s = strings.TrimSpace(s)
	}
	start := strings.IndexAny(s, "{[")
	if start < 0 {
		return ""
	}
	closer := byte('}')
	if s[start] == '[' {
		closer = ']'
	}
	end := strings.LastIndexByte(s, closer)
	if end <= start {
		return ""
	}
	return s[start : end+1]
}
