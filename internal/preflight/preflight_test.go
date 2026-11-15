package preflight

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"runtime"
	"testing"

	"github.com/SkalaSkalolaz/llama-runner/internal/planner"
)

// fakeRunner runs real commands (sh -n is safe: it executes nothing) and
// lets the test control LookPath.
type fakeRunner struct{ lookErr error }

func (f fakeRunner) Command(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}
func (f fakeRunner) LookPath(file string) (string, error) {
	if f.lookErr != nil {
		return "", f.lookErr
	}
	return "/usr/bin/" + file, nil
}

func oneStep(verify, artifact string) planner.Plan {
	return planner.Plan{Steps: []planner.Step{
		{ID: "1", Role: "coder", Goal: "g", Alias: "auto", Verify: verify, Artifact: artifact},
	}}
}

func TestCheck_OK(t *testing.T) {
	rep := Check(context.Background(), oneStep("go build ./...", ""), t.TempDir(), fakeRunner{})
	if len(rep.Findings) != 0 || rep.Err() != nil {
		t.Errorf("findings = %+v, err = %v, want clean", rep.Findings, rep.Err())
	}
}

func TestCheck_MissingCommand(t *testing.T) {
	rep := Check(context.Background(), oneStep("nosuchtool x", ""), t.TempDir(),
		fakeRunner{lookErr: errors.New("not found")})
	if rep.Err() == nil {
		t.Fatal("expected an error")
	}
	if len(rep.Findings) != 1 || rep.Findings[0].Severity != Error {
		t.Errorf("findings = %+v, want one Error", rep.Findings)
	}
}

func TestCheck_BuiltinNotLookedUp(t *testing.T) {
	rep := Check(context.Background(), oneStep("test -f x", ""), t.TempDir(),
		fakeRunner{lookErr: errors.New("not found")})
	if len(rep.Findings) != 0 {
		t.Errorf("builtin was looked up: %+v", rep.Findings)
	}
}

func TestCheck_SyntaxError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh -n is POSIX-only")
	}
	rep := Check(context.Background(), oneStep("if [ ; then", ""), t.TempDir(), fakeRunner{})
	if rep.Err() == nil {
		t.Fatal("expected a syntax error")
	}
	if rep.Findings[0].Severity != Error {
		t.Errorf("severity = %v, want Error", rep.Findings[0].Severity)
	}
}

func TestCheck_ArtifactOutside(t *testing.T) {
	rep := Check(context.Background(), oneStep("", "../x"), t.TempDir(), fakeRunner{})
	if rep.Err() != nil {
		t.Errorf("outside artifact must be a warning, got err = %v", rep.Err())
	}
	if len(rep.Findings) != 1 || rep.Findings[0].Severity != Warning {
		t.Errorf("findings = %+v, want one Warning", rep.Findings)
	}
}

func TestCheck_DangerousIsWarning(t *testing.T) {
	rep := Check(context.Background(), oneStep("sudo rm -rf /tmp/x", ""), t.TempDir(), fakeRunner{})
	if rep.Err() != nil {
		t.Errorf("dangerous pattern must be a warning, got err = %v", rep.Err())
	}
	for _, f := range rep.Findings {
		if f.Severity != Warning {
			t.Errorf("finding %+v, want Warning", f)
		}
	}
}

func TestCheck_Deterministic(t *testing.T) {
	plan := oneStep("sudo nosuchtool x && go build ./...", "../x")
	r := fakeRunner{lookErr: errors.New("not found")}
	a := Check(context.Background(), plan, t.TempDir(), r)
	b := Check(context.Background(), plan, t.TempDir(), r)
	if !reflect.DeepEqual(a.Findings, b.Findings) {
		t.Error("findings order is not deterministic")
	}
}
