// Package repomap builds a small, deterministic description of the project
// in dir: stack markers, language mix, a shallow directory tree and suggested
// verify commands. Pure stdlib; never touches the network, git or any model.
// Its output is embedded into the planner's system prompt (planner.Options.Project).
package repomap

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	maxDepth       = 2    // directory tree depth
	walkDepth      = 4    // language-count depth
	maxTreeEntries = 200  // tree lines cap
	maxFiles       = 5000 // visited files cap
	promptCap      = 4096 // Prompt() byte cap, ellipsis included
)

// marker describes one root-level stack marker file.
type marker struct{ file, stack, verify string }

// markers is the fixed detection table; every derived slice is sorted,
// so the table order does not affect the output.
var markers = []marker{
	{"go.mod", "Go", "go build ./... && go test ./..."},
	{"package.json", "Node", "npm test"},
	{"Cargo.toml", "Rust", "cargo build && cargo test"},
	{"pyproject.toml", "Python", "python -m pytest -q"},
	{"requirements.txt", "Python", "python -m pytest -q"},
	{"pom.xml", "Maven", "mvn -q test"},
	{"build.gradle", "Gradle", "./gradlew test"},
	{"Makefile", "Make", "make"},
	{"CMakeLists.txt", "CMake", "cmake --build build"},
}

// skipDirs are never descended into.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true,
	"build": true, "target": true, ".venv": true, "__pycache__": true,
	".idea": true, ".cache": true,
}

// Info is the scan result. All slices are sorted so Prompt is byte-stable
// across runs (prompt caching depends on it).
type Info struct {
	Dir       string
	Stacks    []string // "Go", "Node", ...
	Markers   []string // found marker files
	Languages []string // ".go=142", count desc then name asc
	Tree      []string // directories, depth <= maxDepth, trailing slash
	Verify    []string // suggested verify commands
}

// Scan inspects dir without following symlinks. Unreadable entries are
// skipped, never fatal. A missing or non-directory dir is an error.
func Scan(dir string) (Info, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return Info{}, err
	}
	st, err := os.Stat(root)
	if err != nil {
		return Info{}, err
	}
	if !st.IsDir() {
		return Info{}, fmt.Errorf("%s is not a directory", root)
	}
	info := Info{Dir: root}

	stackSet, verifySet := map[string]bool{}, map[string]bool{}
	for _, m := range markers {
		if fi, serr := os.Stat(filepath.Join(root, m.file)); serr == nil && fi.Mode().IsRegular() {
			info.Markers = append(info.Markers, m.file)
			stackSet[m.stack] = true
			verifySet[m.verify] = true
		}
	}
	sort.Strings(info.Markers)
	info.Stacks = sortedKeys(stackSet)
	info.Verify = sortedKeys(verifySet)

	langs := map[string]int{}
	files := 0
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return nil
		}
		depth := 0
		if rel != "." {
			depth = strings.Count(filepath.ToSlash(rel), "/") + 1
		}
		if d.IsDir() {
			if rel == "." {
				return nil
			}
			if skipDirs[d.Name()] || d.Type()&fs.ModeSymlink != 0 {
				return filepath.SkipDir
			}
			if depth <= maxDepth && len(info.Tree) < maxTreeEntries {
				info.Tree = append(info.Tree, filepath.ToSlash(rel)+"/")
			}
			if depth >= walkDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		files++
		if files > maxFiles {
			return filepath.SkipAll
		}
		if ext := strings.ToLower(filepath.Ext(d.Name())); ext != "" {
			langs[ext]++
		}
		return nil
	})
	if err != nil {
		return Info{}, err
	}
	sort.Strings(info.Tree)
	info.Languages = formatLangs(langs)
	return info, nil
}

// Prompt renders the deterministic English block for the planner's system
// prompt. Empty when neither markers nor files were found. Capped at
// promptCap bytes including the ellipsis.
func (i Info) Prompt() string {
	if len(i.Markers) == 0 && len(i.Languages) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "dir: %s\n", i.Dir)
	if len(i.Stacks) > 0 {
		fmt.Fprintf(&b, "stack: %s (markers: %s)\n",
			strings.Join(i.Stacks, ", "), strings.Join(i.Markers, ", "))
	}
	if len(i.Languages) > 0 {
		fmt.Fprintf(&b, "languages: %s\n", strings.Join(i.Languages, ", "))
	}
	if len(i.Tree) > 0 {
		b.WriteString("tree:\n")
		for _, d := range i.Tree {
			fmt.Fprintf(&b, "  %s\n", d)
		}
	}
	if len(i.Verify) > 0 {
		fmt.Fprintf(&b, "suggested verify: %s\n", strings.Join(i.Verify, " | "))
	}
	s := b.String()
	if len(s) > promptCap {
		cut := promptCap - len("…")
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		s = s[:cut] + "…"
	}
	return s
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func formatLangs(counts map[string]int) []string {
	type kv struct {
		ext string
		n   int
	}
	list := make([]kv, 0, len(counts))
	for e, n := range counts {
		list = append(list, kv{e, n})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].n != list[j].n {
			return list[i].n > list[j].n
		}
		return list[i].ext < list[j].ext
	})
	out := make([]string, 0, len(list))
	for _, e := range list {
		out = append(out, fmt.Sprintf("%s=%d", e.ext, e.n))
	}
	return out
}
