package planner

import (
	"testing"
)

func orderedIDs(t *testing.T, p Plan) []string {
	t.Helper()
	got, err := p.Order()
	if err != nil {
		t.Fatalf("Order: %v", err)
	}
	ids := make([]string, len(got))
	for i, s := range got {
		ids[i] = s.ID
	}
	return ids
}

func TestOrder_DependenciesFirst(t *testing.T) {
	p := Plan{Steps: []Step{
		{ID: "2", Role: "coder", Goal: "g", Alias: "auto", DependsOn: []string{"1"}},
		{ID: "1", Role: "architect", Goal: "g", Alias: "auto"},
		{ID: "3", Role: "tester", Goal: "g", Alias: "auto", DependsOn: []string{"1", "2"}},
	}}
	if err := p.Validate([]string{"auto"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"1", "2", "3"}
	got := orderedIDs(t, p)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestOrder_AlreadySortedIsUnchanged(t *testing.T) {
	p := Plan{Steps: []Step{
		{ID: "a", Role: "r", Goal: "g", Alias: "auto"},
		{ID: "b", Role: "r", Goal: "g", Alias: "auto", DependsOn: []string{"a"}},
		{ID: "c", Role: "r", Goal: "g", Alias: "auto"},
	}}
	if err := p.Validate([]string{"auto"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "b", "c"}
	got := orderedIDs(t, p)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v (plan order must be preserved among ready steps)", got, want)
		}
	}
}

func TestOrder_Diamond(t *testing.T) {
	p := Plan{Steps: []Step{
		{ID: "top", Role: "r", Goal: "g", Alias: "auto", DependsOn: []string{"l", "r"}},
		{ID: "l", Role: "r", Goal: "g", Alias: "auto", DependsOn: []string{"root"}},
		{ID: "r", Role: "r", Goal: "g", Alias: "auto", DependsOn: []string{"root"}},
		{ID: "root", Role: "r", Goal: "g", Alias: "auto"},
	}}
	if err := p.Validate([]string{"auto"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"root", "l", "r", "top"}
	got := orderedIDs(t, p)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestOrder_CycleIsAnError(t *testing.T) {
	// Not Validate'd on purpose: Order must be safe on its own too.
	p := Plan{Steps: []Step{
		{ID: "x", Role: "r", Goal: "g", Alias: "auto", DependsOn: []string{"y"}},
		{ID: "y", Role: "r", Goal: "g", Alias: "auto", DependsOn: []string{"x"}},
	}}
	if _, err := p.Order(); err == nil {
		t.Fatal("a cycle must be an error")
	}
}
