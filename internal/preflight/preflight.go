// Package preflight checks a validated plan BEFORE any step executes:
// verify-command syntax (sh -n — nothing is run), availability of the named
// commands on PATH, artifact paths staying inside the working directory and
// dangerous command patterns. Errors feed the planner's repair round
// (planner.Options.ExtraCheck); warnings are only printed by the caller.
package preflight

import (
	"bytes"
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/SkalaSkalolaz/llama-runner/internal/i18n"
	"github.com/SkalaSkalolaz/llama-runner/internal/planner"
	"github.com/SkalaSkalolaz/llama-runner/internal/proc"
	"github.com/SkalaSkalolaz/llama-runner/internal/runner"
)

// Severity classifies a finding: Warning is printed, Error fails the plan.
type Severity int

const (
	Warning Severity = iota
	Error
)

// Finding is one problem found in one step.
type Finding struct {
	StepID   string
	Severity Severity
	Message  string
}

// Report is the ordered list of findings (plan order, then check order).
type Report struct{ Findings []Finding }

// Err returns nil when there are no Error-severity findings; otherwise a
// localized error listing them, suitable for the planner's repair round.
func (r Report) Err() error {
	var errs []string
	for _, f := range r.Findings {
		if f.Severity == Error {
			errs = append(errs, f.Message)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return i18n.Errorv(i18n.MsgPreflightFailed, strings.Join(errs, "; "))
}

// dangerousPatterns are matched case-insensitively as substrings of verify.
var dangerousPatterns = []string{
	"rm -rf", "sudo", "git push", "git reset --hard",
	"mkfs", "dd if=", "shutdown", "reboot",
}

// shellBuiltins are never looked up on PATH.
var shellBuiltins = map[string]bool{
	"test": true, "[": true, "]": true, "echo": true, "printf": true,
	"cd": true, "true": true, "false": true, "read": true, "set": true,
	"export": true, "local": true, "eval": true, "exec": true,
	"shift": true, "trap": true, "umask": true, "unset": true,
	"wait": true, "return": true, "break": true, "continue": true,
	"if": true, "then": true, "else": true, "fi": true, "for": true,
	"while": true, "do": true, "done": true, "case": true, "esac": true,
	"!": true, "{": true, "}": true, "(": true, ")": true, ":": true,
	".": true, "source": true,
}

// Check inspects every step of the plan. The order of findings is
// deterministic: plan order, then syntax → commands → artifact → dangerous.
func Check(ctx context.Context, plan planner.Plan, dir string, r runner.CommandRunner) Report {
	var rep Report
	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}
	for _, step := range plan.Steps {
		verify := strings.TrimSpace(step.Verify)
		if verify != "" && runtime.GOOS != "windows" {
			if f := checkSyntax(ctx, step, absDir, r); f != nil {
				rep.Findings = append(rep.Findings, *f)
			}
		}
		if verify != "" {
			seen := map[string]bool{}
			for _, seg := range splitSegments(verify) {
				name := commandName(seg)
				if name == "" || seen[name] {
					continue
				}
				seen[name] = true
				if _, lerr := r.LookPath(name); lerr != nil {
					rep.Findings = append(rep.Findings, Finding{
						StepID: step.ID, Severity: Error,
						Message: msg(i18n.MsgPreflightNoCmd, verify, name),
					})
				}
			}
		}
		if a := step.Artifact; a != "" && !strings.ContainsAny(a, " \t\n") {
			abs := a
			if !filepath.IsAbs(abs) {
				abs = filepath.Join(absDir, abs)
			}
			rel, rerr := filepath.Rel(absDir, abs)
			if rerr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				rep.Findings = append(rep.Findings, Finding{
					StepID: step.ID, Severity: Warning,
					Message: msg(i18n.MsgPreflightOutside, a),
				})
			}
		}
		lower := strings.ToLower(verify)
		for _, p := range dangerousPatterns {
			if strings.Contains(lower, p) {
				rep.Findings = append(rep.Findings, Finding{
					StepID: step.ID, Severity: Warning,
					Message: msg(i18n.MsgPreflightDangerous, verify, p),
				})
			}
		}
	}
	return rep
}

// checkSyntax runs sh -n -c verify: the -n flag only parses, nothing is
// executed. Returns nil when the syntax is valid.
func checkSyntax(ctx context.Context, step planner.Step, dir string, r runner.CommandRunner) *Finding {
	vctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := r.Command("sh", "-n", "-c", step.Verify)
	cmd.Dir = dir
	var out cappedBuffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		return &Finding{StepID: step.ID, Severity: Error,
			Message: msg(i18n.MsgPreflightSyntax, step.Verify, err.Error())}
	}
	tr := proc.Track(cmd)
	select {
	case <-tr.Done:
	case <-vctx.Done():
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-tr.Done
		return &Finding{StepID: step.ID, Severity: Error,
			Message: msg(i18n.MsgPreflightSyntax, step.Verify, vctx.Err().Error())}
	}
	if err := tr.Wait(); err != nil {
		detail := tail(out.String())
		if detail == "" {
			detail = err.Error()
		}
		return &Finding{StepID: step.ID, Severity: Error,
			Message: msg(i18n.MsgPreflightSyntax, step.Verify, detail)}
	}
	return nil
}

// splitSegments splits a verify string on &&, ||, ;, | and newlines.
func splitSegments(verify string) []string {
	r := strings.NewReplacer("&&", "\n", "||", "\n", ";", "\n", "|", "\n")
	return strings.Split(r.Replace(verify), "\n")
}

// commandName extracts the executable name from one segment, or "" when the
// first word is not a plain command name (variable, assignment, path, number,
// quoted word) or is a shell builtin.
func commandName(seg string) string {
	fields := strings.Fields(seg)
	if len(fields) == 0 {
		return ""
	}
	w := fields[0]
	if strings.ContainsAny(w, "$=/.'\"(){}[]`") {
		return ""
	}
	if strings.ContainsAny(w, "0123456789") {
		return ""
	}
	if shellBuiltins[w] {
		return ""
	}
	return w
}

// msg renders a localized template WITHOUT the trailing newline TF adds:
// findings are printed through TF(MsgPreflightFinding, ...), which adds one.
func msg(key string, args ...any) string {
	return strings.TrimSuffix(i18n.TF(key, args...), "\n")
}

// tail returns the last non-empty line of s, trimmed.
func tail(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// cappedBuffer keeps at most 64 KiB, silently discarding the rest.
type cappedBuffer struct{ buf bytes.Buffer }

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if room := 64*1024 - c.buf.Len(); room > 0 {
		if len(p) > room {
			_, _ = c.buf.Write(p[:room])
		} else {
			_, _ = c.buf.Write(p)
		}
	}
	return len(p), nil
}

func (c *cappedBuffer) String() string { return c.buf.String() }
