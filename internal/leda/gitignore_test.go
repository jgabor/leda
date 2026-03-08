package leda

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGitIgnoreMatch(t *testing.T) {
	tests := []struct {
		patterns []string
		path     string
		isDir    bool
		want     bool
	}{
		{[]string{"*.log"}, "debug.log", false, true},
		{[]string{"*.log"}, "src/debug.log", false, true},
		{[]string{"*.log"}, "debug.go", false, false},
		{[]string{"build/"}, "build", true, true},
		{[]string{"build/"}, "build/output.js", false, true},
		{[]string{"build/"}, "build", false, false},
		{[]string{"vendor"}, "vendor", true, true},
		{[]string{"vendor"}, "vendor/lib.go", false, true},
		{[]string{"*.log", "!important.log"}, "debug.log", false, true},
		{[]string{"*.log", "!important.log"}, "important.log", false, false},
		{[]string{"**/temp"}, "a/b/temp", true, true},
		{[]string{"**/temp"}, "temp", true, true},
		{[]string{"doc/*.txt"}, "doc/notes.txt", false, true},
		{[]string{"doc/*.txt"}, "src/doc/notes.txt", false, false},
		// ** at end of pattern
		{[]string{"logs/**"}, "logs/debug.log", false, true},
		{[]string{"logs/**"}, "logs/sub/trace.log", false, true},
		// ? wildcard
		{[]string{"file?.txt"}, "file1.txt", false, true},
		{[]string{"file?.txt"}, "file12.txt", false, false},
		// Character class [abc]
		{[]string{"file[0-9].txt"}, "file3.txt", false, true},
		{[]string{"file[0-9].txt"}, "fileA.txt", false, false},
		// Escaped character
		{[]string{"file\\*.txt"}, "file*.txt", false, true},
		// Unclosed bracket
		{[]string{"file[.txt"}, "file[.txt", false, true},
		// Special regex chars in pattern
		{[]string{"file.bak"}, "file.bak", false, true},
		{[]string{"a+b.txt"}, "a+b.txt", false, true},
		// Subdirectory .gitignore (anchored pattern with subdir)
		{[]string{"sub/dir"}, "sub/dir", true, true},
		// Patterns with comments and empty lines
		{[]string{"# comment", "", "*.tmp"}, "debug.tmp", false, true},
	}

	for _, tt := range tests {
		gi := newGitIgnore()
		for _, p := range tt.patterns {
			gi.addPattern(p, "/root", "/root")
		}
		got := gi.match(tt.path, tt.isDir)
		if got != tt.want {
			t.Errorf("patterns=%v path=%q isDir=%v: got %v, want %v", tt.patterns, tt.path, tt.isDir, got, tt.want)
		}
	}
}

func TestBuildGraphRespectsGitIgnore(t *testing.T) {
	dir := t.TempDir()

	// Create a simple project with a .gitignore.
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "ignored"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignored", "junk.go"), []byte("package junk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	g, err := BuildGraph(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, n := range g.Nodes() {
		rel, _ := filepath.Rel(dir, n)
		if rel == filepath.Join("ignored", "junk.go") {
			t.Error("gitignored file should not be in graph")
		}
	}

	// With gitignore disabled, the ignored file should appear.
	g2, err := BuildGraph(dir, WithGitIgnore(false))
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, n := range g2.Nodes() {
		rel, _ := filepath.Rel(dir, n)
		if rel == filepath.Join("ignored", "junk.go") {
			found = true
		}
	}
	if !found {
		t.Error("with gitignore disabled, ignored file should be in graph")
	}
}

func TestNestedGitIgnoreWildcardDoesNotLeakGlobally(t *testing.T) {
	dir := t.TempDir()

	// Create a source file that should be found.
	if err := os.MkdirAll(filepath.Join(dir, "cmd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cmd", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create an ignored directory with a nested .gitignore that uses "*".
	nested := filepath.Join(dir, "tmp", "sub", ".pi")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, ".gitignore"), []byte("*\n!.gitignore\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Root .gitignore ignores tmp/.
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("tmp/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	g, err := BuildGraph(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(g.Nodes()) == 0 {
		t.Fatal("expected nodes in graph, got 0 — nested .gitignore wildcard leaked globally")
	}

	found := false
	for _, n := range g.Nodes() {
		rel, _ := filepath.Rel(dir, n)
		if rel == filepath.Join("cmd", "main.go") {
			found = true
		}
	}
	if !found {
		t.Error("cmd/main.go should be in graph")
	}
}

func TestNestedGitIgnorePatternsScopedToSubdir(t *testing.T) {
	dir := t.TempDir()

	// Create files at root and in a subdirectory.
	if err := os.WriteFile(filepath.Join(dir, "root.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "keep.go"), []byte("package sub\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "hide.log"), []byte("log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "root.log"), []byte("log\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// sub/.gitignore ignores *.log — should only apply under sub/.
	if err := os.WriteFile(filepath.Join(sub, ".gitignore"), []byte("*.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	gi := loadGitIgnores(dir)

	// sub/hide.log should be ignored.
	if !gi.match("sub/hide.log", false) {
		t.Error("sub/hide.log should be ignored by sub/.gitignore")
	}
	// root.log should NOT be ignored (pattern is scoped to sub/).
	if gi.match("root.log", false) {
		t.Error("root.log should not be ignored — sub/.gitignore pattern should be scoped")
	}
}
