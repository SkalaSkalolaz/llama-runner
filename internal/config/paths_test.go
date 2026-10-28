package config

import "testing"

// ---------- ExpandHome: чистые тесты, без副作用 ----------

func TestExpandHome(t *testing.T) {
	home := "/home/test"
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"tilde", "~", home},
		{"tilde slash", "~/foo", "/home/test/foo"},
		{"tilde slash nested", "~/a/b/c", "/home/test/a/b/c"},
		{"tilde slash only", "~/", home},
		{"absolute", "/abs/path", "/abs/path"},
		{"relative", "rel/path", "rel/path"},
		{"tilde user (no expand)", "~user/x", "~user/x"},
		{"tilde no slash", "~foo", "~foo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExpandHome(tt.in, home); got != tt.want {
				t.Errorf("ExpandHome(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
