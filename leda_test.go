package leda

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildGraphIntegration(t *testing.T) {
	testDir, err := filepath.Abs("testdata/goproject")
	if err != nil {
		t.Fatal(err)
	}

	g, err := BuildGraph(testDir)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	stats := g.Stats()
	t.Logf("Graph: %d nodes, %d edges, %d components", stats.NodeCount, stats.EdgeCount, stats.Components)

	// Should have all .go files.
	nodes := g.Nodes()
	if len(nodes) < 5 {
		t.Errorf("expected at least 5 nodes, got %d: %v", len(nodes), nodes)
	}

	// Verify edges exist (main.go imports auth and server).
	edges := g.Edges()
	if len(edges) == 0 {
		t.Error("expected edges, got none")
	}

	// Log for debugging.
	for _, e := range edges {
		t.Logf("  %s → %s", filepath.Base(e.From), filepath.Base(e.To))
	}
}

func TestIsolateContextAuth(t *testing.T) {
	testDir, err := filepath.Abs("testdata/goproject")
	if err != nil {
		t.Fatal(err)
	}

	g, err := BuildGraph(testDir)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	ctx := g.IsolateContext("Where is the auth middleware?")

	if len(ctx.Seeds) == 0 {
		t.Fatal("IsolateContext: no seeds found")
	}

	t.Logf("Seeds: %v", ctx.Seeds)
	t.Logf("Files (%d): %v", len(ctx.Files), ctx.Files)
	t.Logf("Tokens: %d", ctx.TokenCount)

	// Auth-related files should be in the result.
	hasAuth := false
	for _, f := range ctx.Files {
		if filepath.Base(f) == "auth.go" && filepath.Base(filepath.Dir(f)) == "auth" {
			hasAuth = true
			break
		}
	}
	if !hasAuth {
		t.Error("IsolateContext: auth/auth.go not in result")
	}

	// Config and db should NOT be in the result (unrelated).
	allNodes := g.Nodes()
	if len(ctx.Files) >= len(allNodes) {
		t.Errorf("IsolateContext: returned all %d files, expected fewer", len(ctx.Files))
	}
}

func TestIsolateContextNoMatch(t *testing.T) {
	testDir, err := filepath.Abs("testdata/goproject")
	if err != nil {
		t.Fatal(err)
	}

	g, err := BuildGraph(testDir)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	ctx := g.IsolateContext("something completely unrelated like quantum physics")

	// Should fallback to all nodes.
	allNodes := g.Nodes()
	if len(ctx.Files) != len(allNodes) {
		t.Errorf("fallback: got %d files, want %d (all)", len(ctx.Files), len(allNodes))
	}
}

func TestSerializationRoundTrip(t *testing.T) {
	testDir, err := filepath.Abs("testdata/goproject")
	if err != nil {
		t.Fatal(err)
	}

	g1, err := BuildGraph(testDir)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	var buf bytes.Buffer
	if err := g1.Save(&buf); err != nil {
		t.Fatalf("Save: %v", err)
	}

	g2, err := Load(&buf)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(g2.Nodes()) != len(g1.Nodes()) {
		t.Errorf("nodes after roundtrip: got %d, want %d", len(g2.Nodes()), len(g1.Nodes()))
	}
	if len(g2.Edges()) != len(g1.Edges()) {
		t.Errorf("edges after roundtrip: got %d, want %d", len(g2.Edges()), len(g1.Edges()))
	}
}

func TestSaveLoadFile(t *testing.T) {
	testDir, err := filepath.Abs("testdata/goproject")
	if err != nil {
		t.Fatal(err)
	}

	g1, err := BuildGraph(testDir)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	tmpFile := filepath.Join(t.TempDir(), "test.gob")
	if err := g1.SaveToFile(tmpFile); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}

	info, _ := os.Stat(tmpFile)
	t.Logf("Serialized graph size: %d bytes", info.Size())

	g2, err := LoadFromFile(tmpFile)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}

	if g2.RootDir() != g1.RootDir() {
		t.Errorf("rootDir: got %s, want %s", g2.RootDir(), g1.RootDir())
	}
}

func TestIsolateContextWithMaxFiles(t *testing.T) {
	testDir, err := filepath.Abs("testdata/goproject")
	if err != nil {
		t.Fatal(err)
	}

	g, err := BuildGraph(testDir)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	ctx := g.IsolateContext("auth", WithMaxFiles(2))
	if len(ctx.Files) > 2 {
		t.Errorf("WithMaxFiles(2): got %d files", len(ctx.Files))
	}
}

func TestIsolateContextWithSymbolStrategy(t *testing.T) {
	testDir, err := filepath.Abs("testdata/goproject")
	if err != nil {
		t.Fatal(err)
	}

	g, err := BuildGraph(testDir)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	ctx := g.IsolateContext("Authenticate", WithSeedStrategy(SeedSymbol))

	if len(ctx.Seeds) == 0 {
		t.Fatal("IsolateContext(SeedSymbol): no seeds")
	}

	t.Logf("Symbol seeds: %v", ctx.Seeds)
	t.Logf("Files: %v", ctx.Files)
}

func TestContentsWithBudget(t *testing.T) {
	testDir, err := filepath.Abs("testdata/goproject")
	if err != nil {
		t.Fatal(err)
	}

	g, err := BuildGraph(testDir)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	ctx := g.IsolateContext("auth")

	full, err := ctx.Contents()
	if err != nil {
		t.Fatalf("Contents: %v", err)
	}
	if len(full) == 0 {
		t.Fatal("Contents returned empty string")
	}

	// Budget of 10 tokens should return less content than full.
	budgeted, err := ctx.ContentsWithBudget(10)
	if err != nil {
		t.Fatalf("ContentsWithBudget: %v", err)
	}
	if len(budgeted) >= len(full) && len(ctx.Files) > 1 {
		t.Errorf("ContentsWithBudget(10) returned %d bytes, expected less than full %d bytes", len(budgeted), len(full))
	}

	// Budget of 0 should return same as Contents.
	unlimited, err := ctx.ContentsWithBudget(0)
	if err != nil {
		t.Fatalf("ContentsWithBudget(0): %v", err)
	}
	if unlimited != full {
		t.Error("ContentsWithBudget(0) differs from Contents()")
	}
}

func TestFormatForLLM(t *testing.T) {
	testDir, err := filepath.Abs("testdata/goproject")
	if err != nil {
		t.Fatal(err)
	}

	g, err := BuildGraph(testDir)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	ctx := g.IsolateContext("auth")

	out, err := ctx.FormatForLLM()
	if err != nil {
		t.Fatalf("FormatForLLM: %v", err)
	}

	if !strings.Contains(out, "# Relevant Source Files") {
		t.Error("FormatForLLM: missing header")
	}
	if !strings.Contains(out, "## Entry Points") {
		t.Error("FormatForLLM: missing entry points section")
	}
	if !strings.Contains(out, "## File Manifest") {
		t.Error("FormatForLLM: missing file manifest section")
	}
	if !strings.Contains(out, "## Contents") {
		t.Error("FormatForLLM: missing contents section")
	}
	if !strings.Contains(out, "```") {
		t.Error("FormatForLLM: missing code fences")
	}
}

func TestWithMaxTokens(t *testing.T) {
	testDir, err := filepath.Abs("testdata/goproject")
	if err != nil {
		t.Fatal(err)
	}

	g, err := BuildGraph(testDir)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	full := g.IsolateContext("auth")
	capped := g.IsolateContext("auth", WithMaxTokens(50))

	if capped.TokenCount > 50 {
		t.Errorf("WithMaxTokens(50): token count %d exceeds budget", capped.TokenCount)
	}
	if len(capped.Files) >= len(full.Files) && full.TokenCount > 50 {
		t.Errorf("WithMaxTokens(50): returned %d files, expected fewer than %d", len(capped.Files), len(full.Files))
	}
}

func BenchmarkBuildGraph(b *testing.B) {
	testDir, _ := filepath.Abs("testdata/goproject")
	for b.Loop() {
		_, _ = BuildGraph(testDir)
	}
}

func BenchmarkIsolateContext(b *testing.B) {
	testDir, _ := filepath.Abs("testdata/goproject")
	g, _ := BuildGraph(testDir)
	b.ResetTimer()
	for b.Loop() {
		g.IsolateContext("Where is the auth middleware?")
	}
}
