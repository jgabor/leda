package extract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jgabor/leda/internal/leda"
	"github.com/jgabor/leda/internal/parser"
)

func TestClassifyNaming(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"snake_case", "snake_case"},
		{"my_var_name", "snake_case"},
		{"kebab-case", "kebab-case"},
		{"my-component", "kebab-case"},
		{"PascalCase", "PascalCase"},
		{"BuildGraph", "PascalCase"},
		{"camelCase", "camelCase"},
		{"myFunction", "camelCase"},
		{"UPPER_SNAKE", "UPPER_SNAKE"},
		{"MAX_RETRIES", "UPPER_SNAKE"},
		{"cmd", "short_lowercase"},
		{"leda", "short_lowercase"},
		{"parser", "short_lowercase"},
		{"a", "short_lowercase"},
		{"", "other"},
		{"_private", "other"},
		{"123abc", "other"},
		{"A", "other"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := classifyNaming(tt.input)
			if got != tt.want {
				t.Errorf("classifyNaming(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"graph_test.go", true},
		{"graph.go", false},
		{"App.test.tsx", true},
		{"App.spec.ts", true},
		{"test_utils.py", true},
		{"utils_test.py", true},
		{"main_test.rs", true},
		{"main.rs", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTestFile(tt.name); got != tt.want {
				t.Errorf("isTestFile(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestInferTestNaming(t *testing.T) {
	tests := []struct {
		names []string
		want  string
	}{
		{nil, ""},
		{[]string{"TestBuild", "TestQuery"}, "Test<Name>"},
		{[]string{"TestBuild_success", "TestBuild_error", "TestQuery"}, "Test<Name>_<scenario>"},
	}
	for _, tt := range tests {
		got := inferTestNaming(tt.names)
		if got != tt.want {
			t.Errorf("inferTestNaming(%v) = %q, want %q", tt.names, got, tt.want)
		}
	}
}

func TestAnalyzeTooling(t *testing.T) {
	dir := t.TempDir()

	mustWrite(t, filepath.Join(dir, "go.mod"), "module test\n")
	mustWrite(t, filepath.Join(dir, "Makefile"), "all:\n")
	if err := os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".github", "workflows", "ci.yml"), "name: CI\n")

	result := analyzeTooling(dir)

	if result.DependencyManifest != "go.mod" {
		t.Errorf("DependencyManifest = %q, want %q", result.DependencyManifest, "go.mod")
	}
	if !contains(result.Build, "go build") {
		t.Error("Build should contain 'go build'")
	}
	if !contains(result.Build, "Makefile") {
		t.Error("Build should contain 'Makefile'")
	}
	if !contains(result.CI, "github-actions") {
		t.Error("CI should contain 'github-actions'")
	}
}

func TestAnalyzeErrors(t *testing.T) {
	dir := t.TempDir()

	goSrc := `package main

import (
	"errors"
	"fmt"
)

func wrap(err error) error {
	return fmt.Errorf("failed: %w", err)
}

func newErr() error {
	return errors.New("oops")
}

func check(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}
`
	mustWrite(t, filepath.Join(dir, "main.go"), goSrc)

	reg := parser.DefaultRegistry()
	infos := []leda.NodeInfo{
		{Path: filepath.Join(dir, "main.go"), RelPath: "main.go", Extension: ".go"},
	}

	result := analyzeErrors(infos, reg)

	if result.Patterns["fmt_errorf_w"] != 1 {
		t.Errorf("fmt_errorf_w = %d, want 1", result.Patterns["fmt_errorf_w"])
	}
	if result.Patterns["errors_new"] != 1 {
		t.Errorf("errors_new = %d, want 1", result.Patterns["errors_new"])
	}
	if result.Patterns["errors_is"] != 1 {
		t.Errorf("errors_is = %d, want 1", result.Patterns["errors_is"])
	}
	if result.DominantStyle != "fmt_errorf_w" && result.DominantStyle != "errors_new" && result.DominantStyle != "errors_is" {
		t.Errorf("DominantStyle = %q, want one of the detected patterns", result.DominantStyle)
	}
}

func TestRunIntegration(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "goproject")
	if _, err := os.Stat(root); os.IsNotExist(err) {
		t.Skip("testdata/goproject not found")
	}

	reg := parser.DefaultRegistry()
	g, err := leda.BuildGraph(root)
	if err != nil {
		t.Fatal(err)
	}

	result, err := Run(root, g, reg)
	if err != nil {
		t.Fatal(err)
	}

	if result.Extractor != "leda" {
		t.Errorf("Extractor = %q, want %q", result.Extractor, "leda")
	}
	if result.FileCount == 0 {
		t.Error("FileCount should be > 0")
	}
	if result.Language == "" {
		t.Error("Language should not be empty")
	}
	if len(result.LanguagesDetected) == 0 {
		t.Error("LanguagesDetected should not be empty")
	}
	if result.Layering.Modules == 0 {
		t.Error("Layering.Modules should be > 0")
	}

	// Verify JSON serialization roundtrip
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("JSON marshal: %v", err)
	}
	var decoded Result
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("JSON unmarshal: %v", err)
	}
	if decoded.FileCount != result.FileCount {
		t.Errorf("roundtrip FileCount mismatch: %d vs %d", decoded.FileCount, result.FileCount)
	}
}

func TestInferPlacement(t *testing.T) {
	tests := []struct {
		name     string
		testDirs map[string]bool
		allDirs  map[string]bool
		want     string
	}{
		{"__tests__", map[string]bool{"src/__tests__": true}, map[string]bool{"src": true}, "__tests__"},
		{"tests/", map[string]bool{"tests": true}, map[string]bool{"src": true}, "tests/"},
		{"spec/", map[string]bool{"spec": true}, map[string]bool{"src": true}, "spec/"},
		{"same_package", map[string]bool{"src": true}, map[string]bool{"src": true}, "same_package"},
		{"empty", map[string]bool{}, map[string]bool{"src": true}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferPlacement(tt.testDirs, tt.allDirs)
			if got != tt.want {
				t.Errorf("inferPlacement = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAnalyzeTesting(t *testing.T) {
	dir := t.TempDir()

	goTestSrc := `package main

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestFoo_bar(t *testing.T) {
	assert.True(t, true)
}

func TestBaz_qux(t *testing.T) {}
`
	mustWrite(t, filepath.Join(dir, "main_test.go"), goTestSrc)
	mustWrite(t, filepath.Join(dir, "main.go"), "package main\n")

	reg := parser.DefaultRegistry()
	infos := []leda.NodeInfo{
		{Path: filepath.Join(dir, "main_test.go"), RelPath: "main_test.go", Extension: ".go"},
		{Path: filepath.Join(dir, "main.go"), RelPath: "main.go", Extension: ".go"},
	}

	result := analyzeTesting(dir, infos, reg)

	if result.TestFileCount != 1 {
		t.Errorf("TestFileCount = %d, want 1", result.TestFileCount)
	}
	if result.Framework == "" {
		t.Error("Framework should be detected")
	}
	if result.TestNaming == "" {
		t.Error("TestNaming should be inferred")
	}
	if result.Placement != "same_package" {
		t.Errorf("Placement = %q, want %q", result.Placement, "same_package")
	}
}

func TestAnalyzeTestingWithHelpers(t *testing.T) {
	dir := t.TempDir()

	_ = os.MkdirAll(filepath.Join(dir, "testutil"), 0o755)
	mustWrite(t, filepath.Join(dir, "testutil", "helper.go"), "package testutil\n")

	reg := parser.DefaultRegistry()
	infos := []leda.NodeInfo{
		{Path: filepath.Join(dir, "testutil", "helper.go"), RelPath: "testutil/helper.go", Extension: ".go"},
	}

	result := analyzeTesting(dir, infos, reg)
	if len(result.Helpers) == 0 {
		t.Error("Helpers should include testutil/")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
