package leda

import (
	"container/list"
	"encoding/gob"
	"fmt"
	"io"
	"sort"
)

// graphVersion is incremented when the serialization format changes.
const graphVersion = 1

// Edge represents a directed dependency from one file to another.
type Edge struct {
	From string
	To   string
}

// NodeInfo holds metadata about a file node in the graph.
type NodeInfo struct {
	Path          string
	RelPath       string
	Extension     string
	Symbols       []string
	Size          int64
	TokenEstimate int
}

// GraphStats summarizes a graph.
type GraphStats struct {
	NodeCount  int
	EdgeCount  int
	Components int
	TopFanOut  []FanOutEntry
	TopFanIn   []FanOutEntry
}

// FanOutEntry pairs a file with its edge count.
type FanOutEntry struct {
	File  string
	Count int
}

// Graph is a directed dependency graph of source files.
type Graph struct {
	rootDir  string
	nodes    map[string]*NodeInfo
	outEdges map[string][]string
	inEdges  map[string][]string
}

func newGraph(rootDir string) *Graph {
	return &Graph{
		rootDir:  rootDir,
		nodes:    make(map[string]*NodeInfo),
		outEdges: make(map[string][]string),
		inEdges:  make(map[string][]string),
	}
}

// AddNode adds a file node to the graph.
func (g *Graph) AddNode(info NodeInfo) {
	g.nodes[info.Path] = &info
	if _, ok := g.outEdges[info.Path]; !ok {
		g.outEdges[info.Path] = nil
	}
	if _, ok := g.inEdges[info.Path]; !ok {
		g.inEdges[info.Path] = nil
	}
}

// AddEdge adds a directed edge from → to. Both nodes must exist.
func (g *Graph) AddEdge(from, to string) {
	if _, ok := g.nodes[from]; !ok {
		return
	}
	if _, ok := g.nodes[to]; !ok {
		return
	}
	// Avoid duplicate edges.
	for _, e := range g.outEdges[from] {
		if e == to {
			return
		}
	}
	g.outEdges[from] = append(g.outEdges[from], to)
	g.inEdges[to] = append(g.inEdges[to], from)
}

// Nodes returns all file paths in the graph, sorted.
func (g *Graph) Nodes() []string {
	paths := make([]string, 0, len(g.nodes))
	for p := range g.nodes {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// NodeInfos returns metadata for all nodes.
func (g *Graph) NodeInfos() []NodeInfo {
	infos := make([]NodeInfo, 0, len(g.nodes))
	for _, info := range g.nodes {
		infos = append(infos, *info)
	}
	return infos
}

// Edges returns all directed edges.
func (g *Graph) Edges() []Edge {
	var edges []Edge
	for from, tos := range g.outEdges {
		for _, to := range tos {
			edges = append(edges, Edge{From: from, To: to})
		}
	}
	return edges
}

// Stats returns summary statistics about the graph.
func (g *Graph) Stats() GraphStats {
	stats := GraphStats{
		NodeCount: len(g.nodes),
	}
	for _, tos := range g.outEdges {
		stats.EdgeCount += len(tos)
	}
	stats.Components = g.countComponents()
	stats.TopFanOut = g.topN(g.outEdges, 10)
	stats.TopFanIn = g.topN(g.inEdges, 10)
	return stats
}

func (g *Graph) topN(edges map[string][]string, n int) []FanOutEntry {
	entries := make([]FanOutEntry, 0, len(edges))
	for file, targets := range edges {
		if len(targets) > 0 {
			entries = append(entries, FanOutEntry{File: file, Count: len(targets)})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Count > entries[j].Count
	})
	if len(entries) > n {
		entries = entries[:n]
	}
	return entries
}

func (g *Graph) countComponents() int {
	visited := make(map[string]bool, len(g.nodes))
	count := 0
	for node := range g.nodes {
		if visited[node] {
			continue
		}
		count++
		// BFS treating edges as undirected.
		queue := list.New()
		queue.PushBack(node)
		visited[node] = true
		for queue.Len() > 0 {
			cur := queue.Remove(queue.Front()).(string)
			for _, next := range g.outEdges[cur] {
				if !visited[next] {
					visited[next] = true
					queue.PushBack(next)
				}
			}
			for _, next := range g.inEdges[cur] {
				if !visited[next] {
					visited[next] = true
					queue.PushBack(next)
				}
			}
		}
	}
	return count
}

// reachable returns all nodes reachable from start following the given edge map (BFS).
func (g *Graph) reachable(start string, edges map[string][]string) []string {
	visited := map[string]bool{start: true}
	queue := list.New()
	queue.PushBack(start)
	var result []string
	for queue.Len() > 0 {
		cur := queue.Remove(queue.Front()).(string)
		for _, next := range edges[cur] {
			if !visited[next] {
				visited[next] = true
				queue.PushBack(next)
				result = append(result, next)
			}
		}
	}
	return result
}

func (g *Graph) descendants(start string) []string {
	return g.reachable(start, g.outEdges)
}

func (g *Graph) ancestors(start string) []string {
	return g.reachable(start, g.inEdges)
}

// shortestPath finds the shortest directed path from src to dst using BFS.
// Returns the path as a slice of nodes including src and dst, or error if no path.
func (g *Graph) shortestPath(src, dst string) ([]string, error) {
	if src == dst {
		return []string{src}, nil
	}
	visited := map[string]bool{src: true}
	parent := map[string]string{}
	queue := list.New()
	queue.PushBack(src)
	for queue.Len() > 0 {
		cur := queue.Remove(queue.Front()).(string)
		for _, next := range g.outEdges[cur] {
			if visited[next] {
				continue
			}
			visited[next] = true
			parent[next] = cur
			if next == dst {
				return reconstructPath(parent, src, dst), nil
			}
			queue.PushBack(next)
		}
	}
	return nil, fmt.Errorf("no path from %s to %s", src, dst)
}

func reconstructPath(parent map[string]string, src, dst string) []string {
	var path []string
	for cur := dst; cur != src; cur = parent[cur] {
		path = append(path, cur)
	}
	path = append(path, src)
	// Reverse.
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

// subgraph extracts a new graph containing only the specified nodes and edges between them.
func (g *Graph) subgraph(nodes []string) *Graph {
	nodeSet := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		nodeSet[n] = true
	}
	sub := newGraph(g.rootDir)
	for _, n := range nodes {
		if info, ok := g.nodes[n]; ok {
			sub.AddNode(*info)
		}
	}
	for _, n := range nodes {
		for _, to := range g.outEdges[n] {
			if nodeSet[to] {
				sub.AddEdge(n, to)
			}
		}
	}
	return sub
}

type graphData struct {
	Version  int
	RootDir  string
	Nodes    map[string]*NodeInfo
	OutEdges map[string][]string
}

// Save serializes the graph to a writer in gob format.
func (g *Graph) Save(w io.Writer) error {
	enc := gob.NewEncoder(w)
	data := graphData{
		Version:  graphVersion,
		RootDir:  g.rootDir,
		Nodes:    g.nodes,
		OutEdges: g.outEdges,
	}
	if err := enc.Encode(data); err != nil {
		return fmt.Errorf("leda: saving graph: %w", err)
	}
	return nil
}

// Load deserializes a graph from a reader.
func Load(r io.Reader) (*Graph, error) {
	dec := gob.NewDecoder(r)
	var data graphData
	if err := dec.Decode(&data); err != nil {
		return nil, fmt.Errorf("leda: loading graph: %w", err)
	}
	if data.Version != graphVersion {
		return nil, fmt.Errorf("leda: unsupported graph version %d (expected %d)", data.Version, graphVersion)
	}
	g := newGraph(data.RootDir)
	g.nodes = data.Nodes
	g.outEdges = data.OutEdges
	// Rebuild inEdges.
	for from, tos := range g.outEdges {
		if _, ok := g.inEdges[from]; !ok {
			g.inEdges[from] = nil
		}
		for _, to := range tos {
			g.inEdges[to] = append(g.inEdges[to], from)
		}
	}
	// Ensure all nodes are in edge maps.
	for path := range g.nodes {
		if _, ok := g.outEdges[path]; !ok {
			g.outEdges[path] = nil
		}
		if _, ok := g.inEdges[path]; !ok {
			g.inEdges[path] = nil
		}
	}
	return g, nil
}
