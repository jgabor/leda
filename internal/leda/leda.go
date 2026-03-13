// Package leda provides dependency-graph context isolation for LLM tools.
// Given a codebase and a natural-language prompt, leda builds a directed
// dependency graph from source files, seeds entry points from the prompt,
// and traverses the graph to return only the files the LLM actually needs.
package leda

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jgabor/leda/internal/parser"
	"github.com/jgabor/leda/internal/resolve"
)

// Option configures graph building.
type Option func(*buildConfig)

type buildConfig struct {
	parsers    *parser.Registry
	extensions map[string]bool
	exclude    []string
	maxDepth   int
	resolver   resolve.Resolver
	gitIgnore  bool
}

// WithParsers overrides the default parser registry.
func WithParsers(registry *parser.Registry) Option {
	return func(c *buildConfig) {
		c.parsers = registry
	}
}

// WithExtensions limits parsing to these file extensions.
func WithExtensions(exts ...string) Option {
	return func(c *buildConfig) {
		c.extensions = make(map[string]bool)
		for _, ext := range exts {
			if !strings.HasPrefix(ext, ".") {
				ext = "." + ext
			}
			c.extensions[ext] = true
		}
	}
}

// WithExclude adds glob patterns to exclude from graph building.
func WithExclude(patterns ...string) Option {
	return func(c *buildConfig) {
		c.exclude = append(c.exclude, patterns...)
	}
}

// WithMaxDepth limits directory recursion depth.
func WithMaxDepth(depth int) Option {
	return func(c *buildConfig) {
		c.maxDepth = depth
	}
}

// WithResolver overrides the default import resolver.
func WithResolver(r resolve.Resolver) Option {
	return func(c *buildConfig) {
		c.resolver = r
	}
}

// WithGitIgnore controls whether .gitignore files are respected during graph building.
// Enabled by default.
func WithGitIgnore(enabled bool) Option {
	return func(c *buildConfig) {
		c.gitIgnore = enabled
	}
}

const (
	defaultMaxFiles  = 20
	defaultMaxTokens = 128000
)

var genericBasenames = map[string]bool{
	"errors": true, "error": true,
	"io":     true,
	"config": true,
	"utils":  true, "util": true,
	"helpers": true, "helper": true,
	"common": true, "shared": true,
	"types": true, "constants": true,
}

// DefaultExclude contains directory names skipped by default during graph building.
var DefaultExclude = []string{
	".git", "node_modules", "vendor", ".next",
	"__pycache__", ".tox", "dist", "build",
}

// BuildGraph walks rootDir, parses all recognized source files,
// and returns a dependency graph.
func BuildGraph(rootDir string, opts ...Option) (*Graph, error) {
	rootDir, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("leda: resolving root dir: %w", err)
	}

	cfg := &buildConfig{
		parsers:   parser.DefaultRegistry(),
		maxDepth:  -1,
		gitIgnore: true,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.resolver == nil {
		cfg.resolver = resolve.DefaultResolver(rootDir, cfg.parsers.RegisteredExtensions())
	}

	g := newGraph(rootDir)

	if err := collectNodes(g, rootDir, cfg); err != nil {
		return nil, err
	}
	resolveEdges(g, rootDir, cfg)

	return g, nil
}

func collectNodes(g *Graph, rootDir string, cfg *buildConfig) error {
	var gi *gitIgnore
	if cfg.gitIgnore {
		gi = loadGitIgnores(rootDir)
	}

	return filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		relPath, _ := filepath.Rel(rootDir, path)

		if gi != nil && relPath != "." && gi.match(relPath, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		for _, pattern := range cfg.exclude {
			if matched, _ := filepath.Match(pattern, relPath); matched {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		if info.IsDir() {
			base := filepath.Base(path)
			for _, skip := range DefaultExclude {
				if base == skip {
					return filepath.SkipDir
				}
			}
			if cfg.maxDepth >= 0 {
				depth := strings.Count(relPath, string(os.PathSeparator))
				if depth >= cfg.maxDepth {
					return filepath.SkipDir
				}
			}
			return nil
		}

		ext := filepath.Ext(path)
		if cfg.extensions != nil && !cfg.extensions[ext] {
			return nil
		}

		p := cfg.parsers.ForExtension(ext)
		if p == nil {
			return nil
		}

		node := NodeInfo{
			Path:          path,
			RelPath:       relPath,
			Extension:     ext,
			Size:          info.Size(),
			TokenEstimate: int(info.Size() / 4),
		}
		if symbols, err := p.ParseSymbols(path); err == nil {
			node.Symbols = symbols
		}

		g.AddNode(node)
		return nil
	})
}

func resolveEdges(g *Graph, rootDir string, cfg *buildConfig) {
	for path := range g.nodes {
		p := cfg.parsers.ForExtension(filepath.Ext(path))
		if p == nil {
			continue
		}

		imports, err := p.ParseImports(path)
		if err != nil {
			continue
		}

		for _, imp := range imports {
			resolved, err := cfg.resolver.Resolve(imp, path, rootDir)
			if err != nil || len(resolved) == 0 {
				continue
			}
			for _, target := range resolved {
				g.AddEdge(path, target)
			}
		}
	}
}

// QueryOption configures context isolation.
type QueryOption func(*queryConfig)

type queryConfig struct {
	maxFiles        int
	maxTokens       int
	strategy        SeedStrategy
	customSeeder    func(prompt string, nodes []NodeInfo) []string
	excludePatterns []string
}

// WithMaxFiles caps the result set to n files.
func WithMaxFiles(n int) QueryOption {
	return func(c *queryConfig) {
		c.maxFiles = n
	}
}

// WithMaxTokens caps estimated tokens in the result.
func WithMaxTokens(n int) QueryOption {
	return func(c *queryConfig) {
		c.maxTokens = n
	}
}

// WithSeedStrategy sets the strategy for mapping prompt terms to graph nodes.
func WithSeedStrategy(s SeedStrategy) QueryOption {
	return func(c *queryConfig) {
		c.strategy = s
	}
}

// WithQueryExclude filters files matching the given patterns from query results.
func WithQueryExclude(patterns ...string) QueryOption {
	return func(c *queryConfig) {
		c.excludePatterns = append(c.excludePatterns, patterns...)
	}
}

// WithCustomSeeder provides a custom seed function.
func WithCustomSeeder(fn func(prompt string, nodes []NodeInfo) []string) QueryOption {
	return func(c *queryConfig) {
		c.strategy = SeedCustom
		c.customSeeder = fn
	}
}

// Context is the result of IsolateContext.
type Context struct {
	Files      []string // absolute paths of isolated files
	Seeds      []string // which nodes were entry points
	TokenCount int      // estimated token count
	TraceGraph *Graph   // subgraph showing only the traversed portion
}

// IsolateContext returns the minimal set of files relevant to the prompt.
func (g *Graph) IsolateContext(prompt string, opts ...QueryOption) *Context {
	cfg := &queryConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	seedResults := g.findSeeds(prompt, cfg)

	if len(seedResults) == 0 {
		allNodes := g.Nodes()
		return &Context{
			Files:      allNodes,
			Seeds:      nil,
			TokenCount: g.totalTokens(allNodes),
			TraceGraph: g,
		}
	}

	seeds := make([]string, len(seedResults))
	for i, r := range seedResults {
		seeds[i] = r.path
	}

	files := g.isolate(seedResults)
	if len(cfg.excludePatterns) > 0 {
		files = g.filterExcluded(files, cfg.excludePatterns)
	}
	files, totalTokens := g.applyBudget(files, cfg)

	return &Context{
		Files:      files,
		Seeds:      seeds,
		TokenCount: totalTokens,
		TraceGraph: g.subgraph(files),
	}
}

func (g *Graph) findSeeds(prompt string, cfg *queryConfig) []seedResult {
	switch cfg.strategy {
	case SeedSymbol:
		return seedBySymbol(prompt, g)
	case SeedPath:
		return seedByPath(prompt, g)
	case SeedCustom:
		if cfg.customSeeder != nil {
			paths := cfg.customSeeder(prompt, g.NodeInfos())
			results := make([]seedResult, len(paths))
			for i, p := range paths {
				results[i] = seedResult{path: p, score: 1}
			}
			return results
		}
		return nil
	default:
		return seedByFilename(prompt, g)
	}
}

func (g *Graph) isolate(seeds []seedResult) []string {
	isolated := make(map[string]bool, len(seeds))
	seedScores := make(map[string]int, len(seeds))
	seedPaths := make([]string, len(seeds))
	for i, s := range seeds {
		isolated[s.path] = true
		seedScores[s.path] = s.score
		seedPaths[i] = s.path
	}

	if len(seeds) == 1 {
		for _, d := range g.descendants(seedPaths[0]) {
			isolated[d] = true
		}
	} else {
		foundPath := false
		for i := 0; i < len(seedPaths); i++ {
			for j := i + 1; j < len(seedPaths); j++ {
				if path, err := g.shortestPath(seedPaths[i], seedPaths[j]); err == nil {
					foundPath = true
					for _, n := range path {
						isolated[n] = true
					}
					for _, d := range g.descendants(seedPaths[j]) {
						isolated[d] = true
					}
				}
				if path, err := g.shortestPath(seedPaths[j], seedPaths[i]); err == nil {
					foundPath = true
					for _, n := range path {
						isolated[n] = true
					}
					for _, d := range g.descendants(seedPaths[i]) {
						isolated[d] = true
					}
				}
			}
		}
		if !foundPath {
			for _, s := range seedPaths {
				for _, d := range g.descendants(s) {
					isolated[d] = true
				}
			}
		}
	}

	// Add 1-hop callers of seeds.
	for _, s := range seedPaths {
		for _, caller := range g.inEdges[s] {
			isolated[caller] = true
		}
	}

	// Compute depths: seeds=0, forward BFS from each seed, callers=2.
	nodeDepths := make(map[string]int)
	for _, s := range seedPaths {
		nodeDepths[s] = 0
		for node, dist := range g.reachableWithDepth(s, g.outEdges) {
			if !isolated[node] {
				continue
			}
			if existing, ok := nodeDepths[node]; !ok || dist < existing {
				nodeDepths[node] = dist
			}
		}
	}
	for _, s := range seedPaths {
		for _, caller := range g.inEdges[s] {
			if _, ok := nodeDepths[caller]; !ok {
				nodeDepths[caller] = 2
			}
		}
	}

	return g.rankNodes(nodeDepths, seedScores)
}

func (g *Graph) rankNodes(depths map[string]int, seedScores map[string]int) []string {
	totalNodes := float64(len(g.nodes))
	files := make([]string, 0, len(depths))
	for node := range depths {
		files = append(files, node)
	}

	score := func(node string) float64 {
		var seedBonus float64
		if sc, ok := seedScores[node]; ok {
			seedBonus = 10.0 + float64(sc)
		}
		s := seedBonus + 1.0/(1.0+float64(depths[node]))
		if _, isSeed := seedScores[node]; !isSeed {
			if totalNodes > 0 {
				fanIn := float64(len(g.inEdges[node]))
				if fanIn/totalNodes > 0.05 {
					s *= 1.0 - fanIn/totalNodes
				}
			}
			base := strings.TrimSuffix(filepath.Base(node), filepath.Ext(node))
			if genericBasenames[strings.ToLower(base)] {
				s *= 0.3
			}
		}
		return s
	}

	sort.Slice(files, func(i, j int) bool {
		si, sj := score(files[i]), score(files[j])
		if si != sj {
			return si > sj
		}
		return files[i] < files[j]
	})
	return files
}

func (g *Graph) applyBudget(files []string, cfg *queryConfig) ([]string, int) {
	maxFiles := cfg.maxFiles
	if maxFiles <= 0 {
		maxFiles = defaultMaxFiles
	}
	if len(files) > maxFiles {
		files = files[:maxFiles]
	}

	maxTokens := cfg.maxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	totalTokens := 0
	var capped []string
	for _, f := range files {
		if info, ok := g.nodes[f]; ok {
			if totalTokens+info.TokenEstimate > maxTokens {
				break
			}
			totalTokens += info.TokenEstimate
			capped = append(capped, f)
		}
	}
	return capped, totalTokens
}

func (g *Graph) filterExcluded(files []string, patterns []string) []string {
	var result []string
	for _, f := range files {
		info := g.nodes[f]
		if info == nil {
			continue
		}
		excluded := false
		for _, pattern := range patterns {
			if matched, _ := filepath.Match(pattern, info.RelPath); matched {
				excluded = true
				break
			}
			if matched, _ := filepath.Match(pattern, filepath.Base(f)); matched {
				excluded = true
				break
			}
		}
		if !excluded {
			result = append(result, f)
		}
	}
	return result
}

func (g *Graph) totalTokens(files []string) int {
	total := 0
	for _, f := range files {
		if info, ok := g.nodes[f]; ok {
			total += info.TokenEstimate
		}
	}
	return total
}

// Contents reads and concatenates all isolated files with filepath headers.
func (c *Context) Contents() (string, error) {
	return c.ContentsWithBudget(0)
}

// ContentsWithBudget is like Contents but stops at approximately maxTokens.
// A maxTokens of 0 means no limit.
func (c *Context) ContentsWithBudget(maxTokens int) (string, error) {
	var b strings.Builder
	tokens := 0
	for _, f := range c.Files {
		data, err := os.ReadFile(f)
		if err != nil {
			return "", fmt.Errorf("leda: reading %s: %w", f, err)
		}
		fileTokens := len(data) / 4
		if maxTokens > 0 && tokens+fileTokens > maxTokens && tokens > 0 {
			break
		}
		fmt.Fprintf(&b, "// --- %s ---\n%s\n", f, string(data))
		tokens += fileTokens
	}
	return b.String(), nil
}

// FormatForLLM returns a structured prompt-ready string with file manifest + contents.
func (c *Context) FormatForLLM() (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "# Relevant Source Files (%d files, ~%d tokens)\n\n", len(c.Files), c.TokenCount)

	if len(c.Seeds) > 0 {
		b.WriteString("## Entry Points\n")
		for _, s := range c.Seeds {
			fmt.Fprintf(&b, "- %s\n", s)
		}
		b.WriteString("\n")
	}

	b.WriteString("## File Manifest\n")
	for _, f := range c.Files {
		fmt.Fprintf(&b, "- %s\n", f)
	}
	b.WriteString("\n## Contents\n\n")

	for _, f := range c.Files {
		data, err := os.ReadFile(f)
		if err != nil {
			return "", fmt.Errorf("leda: reading %s: %w", f, err)
		}
		fmt.Fprintf(&b, "### %s\n```\n%s\n```\n\n", f, string(data))
	}

	return b.String(), nil
}

// SaveToFile is a convenience that saves the graph to a file.
func (g *Graph) SaveToFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("leda: creating %s: %w", path, err)
	}
	saveErr := g.Save(f)
	if closeErr := f.Close(); closeErr != nil && saveErr == nil {
		return fmt.Errorf("leda: closing %s: %w", path, closeErr)
	}
	return saveErr
}

// LoadFromFile is a convenience that loads a graph from a file.
func LoadFromFile(path string) (*Graph, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("leda: opening %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	return Load(f)
}

// RootDir returns the root directory the graph was built from.
func (g *Graph) RootDir() string {
	return g.rootDir
}
