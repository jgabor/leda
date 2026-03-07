package leda

import (
	"bytes"
	"sort"
	"testing"
)

func makeTestGraph() *Graph {
	// A → B → D
	// A → C → D
	//          D → E
	g := newGraph("/test")
	for _, name := range []string{"A", "B", "C", "D", "E", "F"} {
		g.AddNode(NodeInfo{Path: name, RelPath: name, Extension: ".go"})
	}
	g.AddEdge("A", "B")
	g.AddEdge("A", "C")
	g.AddEdge("B", "D")
	g.AddEdge("C", "D")
	g.AddEdge("D", "E")
	// F is isolated.
	return g
}

func TestDescendants(t *testing.T) {
	g := makeTestGraph()

	desc := g.descendants("A")
	sort.Strings(desc)
	want := []string{"B", "C", "D", "E"}
	if len(desc) != len(want) {
		t.Fatalf("descendants(A): got %v, want %v", desc, want)
	}
	for i := range want {
		if desc[i] != want[i] {
			t.Errorf("descendants(A)[%d]: got %s, want %s", i, desc[i], want[i])
		}
	}

	// F has no descendants.
	desc = g.descendants("F")
	if len(desc) != 0 {
		t.Errorf("descendants(F): got %v, want empty", desc)
	}
}

func TestAncestors(t *testing.T) {
	g := makeTestGraph()

	anc := g.ancestors("D")
	sort.Strings(anc)
	want := []string{"A", "B", "C"}
	if len(anc) != len(want) {
		t.Fatalf("ancestors(D): got %v, want %v", anc, want)
	}
	for i := range want {
		if anc[i] != want[i] {
			t.Errorf("ancestors(D)[%d]: got %s, want %s", i, anc[i], want[i])
		}
	}
}

func TestShortestPath(t *testing.T) {
	g := makeTestGraph()

	path, err := g.shortestPath("A", "E")
	if err != nil {
		t.Fatalf("shortestPath(A, E): %v", err)
	}
	// Should be A → B → D → E or A → C → D → E (length 4).
	if len(path) != 4 {
		t.Fatalf("shortestPath(A, E): got length %d, want 4; path=%v", len(path), path)
	}
	if path[0] != "A" || path[len(path)-1] != "E" {
		t.Errorf("shortestPath(A, E): got %v, want A...E", path)
	}

	// No path from E to A.
	_, err = g.shortestPath("E", "A")
	if err == nil {
		t.Error("shortestPath(E, A): expected error, got nil")
	}

	// Same node.
	path, err = g.shortestPath("A", "A")
	if err != nil {
		t.Fatalf("shortestPath(A, A): %v", err)
	}
	if len(path) != 1 || path[0] != "A" {
		t.Errorf("shortestPath(A, A): got %v, want [A]", path)
	}
}

func TestSubgraph(t *testing.T) {
	g := makeTestGraph()
	sub := g.subgraph([]string{"A", "B", "D"})

	nodes := sub.Nodes()
	if len(nodes) != 3 {
		t.Fatalf("subgraph nodes: got %d, want 3", len(nodes))
	}

	// Should have edges A→B, B→D but not A→C or D→E.
	edges := sub.Edges()
	if len(edges) != 2 {
		t.Fatalf("subgraph edges: got %d, want 2; edges=%v", len(edges), edges)
	}
}

func TestStats(t *testing.T) {
	g := makeTestGraph()
	stats := g.Stats()

	if stats.NodeCount != 6 {
		t.Errorf("NodeCount: got %d, want 6", stats.NodeCount)
	}
	if stats.EdgeCount != 5 {
		t.Errorf("EdgeCount: got %d, want 5", stats.EdgeCount)
	}
	if stats.Components != 2 {
		t.Errorf("Components: got %d, want 2 (F is isolated)", stats.Components)
	}
}

func TestDuplicateEdge(t *testing.T) {
	g := newGraph("/test")
	g.AddNode(NodeInfo{Path: "A", RelPath: "A"})
	g.AddNode(NodeInfo{Path: "B", RelPath: "B"})
	g.AddEdge("A", "B")
	g.AddEdge("A", "B") // duplicate

	edges := g.Edges()
	if len(edges) != 1 {
		t.Errorf("duplicate edge: got %d edges, want 1", len(edges))
	}
}

func TestSaveLoad(t *testing.T) {
	g := makeTestGraph()

	var buf bytes.Buffer
	if err := g.Save(&buf); err != nil {
		t.Fatalf("Save: %v", err)
	}

	g2, err := Load(&buf)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(g2.Nodes()) != len(g.Nodes()) {
		t.Errorf("nodes: got %d, want %d", len(g2.Nodes()), len(g.Nodes()))
	}
	if len(g2.Edges()) != len(g.Edges()) {
		t.Errorf("edges: got %d, want %d", len(g2.Edges()), len(g.Edges()))
	}
	if g2.rootDir != g.rootDir {
		t.Errorf("rootDir: got %s, want %s", g2.rootDir, g.rootDir)
	}

	// Verify inEdges were rebuilt.
	anc := g2.ancestors("D")
	sort.Strings(anc)
	if len(anc) != 3 {
		t.Errorf("ancestors(D) after load: got %v, want 3 entries", anc)
	}
}
