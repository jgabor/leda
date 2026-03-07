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

	"github.com/jgabor/leda/parser"
	"github.com/jgabor/leda/resolve"
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
	maxFiles     int
	maxTokens    int
	strategy     SeedStrategy
	customSeeder func(prompt string, nodes []NodeInfo) []string
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

	seeds := g.findSeeds(prompt, cfg)

	if len(seeds) == 0 {
		allNodes := g.Nodes()
		return &Context{
			Files:      allNodes,
			Seeds:      nil,
			TokenCount: g.totalTokens(allNodes),
			TraceGraph: g,
		}
	}

	files := g.isolate(seeds)
	files, totalTokens := g.applyBudget(files, cfg)

	return &Context{
		Files:      files,
		Seeds:      seeds,
		TokenCount: totalTokens,
		TraceGraph: g.subgraph(files),
	}
}

func (g *Graph) findSeeds(prompt string, cfg *queryConfig) []string {
	switch cfg.strategy {
	case SeedSymbol:
		return seedBySymbol(prompt, g)
	case SeedPath:
		return seedByPath(prompt, g)
	case SeedCustom:
		if cfg.customSeeder != nil {
			return cfg.customSeeder(prompt, g.NodeInfos())
		}
		return nil
	default:
		return seedByFilename(prompt, g)
	}
}

func (g *Graph) isolate(seeds []string) []string {
	isolated := make(map[string]bool, len(seeds))
	for _, s := range seeds {
		isolated[s] = true
	}

	if len(seeds) == 1 {
		for _, d := range g.descendants(seeds[0]) {
			isolated[d] = true
		}
	} else {
		foundPath := false
		for i := 0; i < len(seeds); i++ {
			for j := i + 1; j < len(seeds); j++ {
				if path, err := g.shortestPath(seeds[i], seeds[j]); err == nil {
					foundPath = true
					for _, n := range path {
						isolated[n] = true
					}
					for _, d := range g.descendants(seeds[j]) {
						isolated[d] = true
					}
				}
				if path, err := g.shortestPath(seeds[j], seeds[i]); err == nil {
					foundPath = true
					for _, n := range path {
						isolated[n] = true
					}
					for _, d := range g.descendants(seeds[i]) {
						isolated[d] = true
					}
				}
			}
		}
		if !foundPath {
			for _, s := range seeds {
				for _, d := range g.descendants(s) {
					isolated[d] = true
				}
			}
		}
	}

	files := make([]string, 0, len(isolated))
	for f := range isolated {
		files = append(files, f)
	}
	sort.Strings(files)
	return files
}

func (g *Graph) applyBudget(files []string, cfg *queryConfig) ([]string, int) {
	if cfg.maxFiles > 0 && len(files) > cfg.maxFiles {
		files = files[:cfg.maxFiles]
	}

	if cfg.maxTokens > 0 {
		totalTokens := 0
		var capped []string
		for _, f := range files {
			if info, ok := g.nodes[f]; ok {
				if totalTokens+info.TokenEstimate > cfg.maxTokens {
					break
				}
				totalTokens += info.TokenEstimate
				capped = append(capped, f)
			}
		}
		return capped, totalTokens
	}

	return files, g.totalTokens(files)
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
