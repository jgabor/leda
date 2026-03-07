package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func mustAbs(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func TestRunVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"version"}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), ".") {
		t.Errorf("version: %q", stdout.String())
	}
}

func TestRunHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"help"}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Errorf("help: %q", stderr.String())
	}
}

func TestRunNoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(nil, &stdout, &stderr)
	if err == nil {
		t.Error("no args: expected error")
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"bogus"}, &stdout, &stderr)
	if err == nil {
		t.Error("unknown: expected error")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("unknown: %v", err)
	}
}

func TestRunBuild(t *testing.T) {
	testDir := mustAbs(t, "../../testdata/goproject")
	outFile := filepath.Join(t.TempDir(), "test.leda")

	var stdout, stderr bytes.Buffer
	err := run([]string{"build", "-root", testDir, "-output", outFile}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
}

func TestRunBuildJSON(t *testing.T) {
	testDir := mustAbs(t, "../../testdata/goproject")
	outFile := filepath.Join(t.TempDir(), "test.leda")

	var stdout, stderr bytes.Buffer
	err := run([]string{"build", "-root", testDir, "-output", outFile, "-format", "json"}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "graph_path") {
		t.Errorf("build json: %q", stdout.String())
	}
}

func TestRunBuildDryRun(t *testing.T) {
	testDir := mustAbs(t, "../../testdata/goproject")

	var stdout, stderr bytes.Buffer
	err := run([]string{"build", "-root", testDir, "-dry-run"}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), ".go") {
		t.Errorf("dry-run: %q", stdout.String())
	}
}

func TestRunBuildDryRunJSON(t *testing.T) {
	testDir := mustAbs(t, "../../testdata/goproject")

	var stdout, stderr bytes.Buffer
	err := run([]string{"build", "-root", testDir, "-dry-run", "-format", "json"}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "dry_run") {
		t.Errorf("dry-run json: %q", stdout.String())
	}
}

func TestRunQuery(t *testing.T) {
	testDir := mustAbs(t, "../../testdata/goproject")
	graphFile := filepath.Join(t.TempDir(), "test.leda")

	var stdout, stderr bytes.Buffer
	err := run([]string{"build", "-root", testDir, "-output", graphFile}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	err = run([]string{"query", "-graph", graphFile, "auth", "middleware"}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "auth") {
		t.Errorf("query: %q", stdout.String())
	}
}

func TestRunQueryFormats(t *testing.T) {
	testDir := mustAbs(t, "../../testdata/goproject")
	graphFile := filepath.Join(t.TempDir(), "test.leda")

	var stdout, stderr bytes.Buffer
	_ = run([]string{"build", "-root", testDir, "-output", graphFile}, &stdout, &stderr)

	for _, format := range []string{"files", "json", "llm"} {
		t.Run(format, func(t *testing.T) {
			stdout.Reset()
			stderr.Reset()
			err := run([]string{"query", "-graph", graphFile, "-format", format, "auth"}, &stdout, &stderr)
			if err != nil {
				t.Fatalf("query %s: %v", format, err)
			}
			if stdout.Len() == 0 {
				t.Errorf("query %s: empty output", format)
			}
		})
	}
}

func TestRunQueryUnknownFormat(t *testing.T) {
	testDir := mustAbs(t, "../../testdata/goproject")
	graphFile := filepath.Join(t.TempDir(), "test.leda")

	var stdout, stderr bytes.Buffer
	_ = run([]string{"build", "-root", testDir, "-output", graphFile}, &stdout, &stderr)

	stdout.Reset()
	stderr.Reset()
	err := run([]string{"query", "-graph", graphFile, "-format", "bogus", "auth"}, &stdout, &stderr)
	if err == nil {
		t.Error("query bogus format: expected error")
	}
}

func TestRunQueryWithOptions(t *testing.T) {
	testDir := mustAbs(t, "../../testdata/goproject")
	graphFile := filepath.Join(t.TempDir(), "test.leda")

	var stdout, stderr bytes.Buffer
	_ = run([]string{"build", "-root", testDir, "-output", graphFile}, &stdout, &stderr)

	stdout.Reset()
	stderr.Reset()
	err := run([]string{"query", "-graph", graphFile, "-max-files", "2", "-max-tokens", "100", "-strategy", "symbol", "auth"}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	err = run([]string{"query", "-graph", graphFile, "-strategy", "path", "auth"}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunQueryNoPrompt(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"query"}, &stdout, &stderr)
	if err == nil {
		t.Error("query no prompt: expected error")
	}
}

func TestRunStats(t *testing.T) {
	testDir := mustAbs(t, "../../testdata/goproject")
	graphFile := filepath.Join(t.TempDir(), "test.leda")

	var stdout, stderr bytes.Buffer
	_ = run([]string{"build", "-root", testDir, "-output", graphFile}, &stdout, &stderr)

	stdout.Reset()
	stderr.Reset()
	err := run([]string{"stats", "-graph", graphFile}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "Nodes:") {
		t.Errorf("stats: %q", stdout.String())
	}
}

func TestRunStatsJSON(t *testing.T) {
	testDir := mustAbs(t, "../../testdata/goproject")
	graphFile := filepath.Join(t.TempDir(), "test.leda")

	var stdout, stderr bytes.Buffer
	_ = run([]string{"build", "-root", testDir, "-output", graphFile}, &stdout, &stderr)

	stdout.Reset()
	stderr.Reset()
	err := run([]string{"stats", "-graph", graphFile, "-format", "json"}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "nodes") {
		t.Errorf("stats json: %q", stdout.String())
	}
}

func TestRunExtract(t *testing.T) {
	testDir := mustAbs(t, "../../testdata/goproject")

	var stdout, stderr bytes.Buffer
	err := run([]string{"extract", "-root", testDir, "-format", "json"}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "language") {
		t.Errorf("extract: %q", stdout.String())
	}
}

func TestRunExtractNoFormat(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"extract"}, &stdout, &stderr)
	if err == nil {
		t.Error("extract without json: expected error")
	}
}
