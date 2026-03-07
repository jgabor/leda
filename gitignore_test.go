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
