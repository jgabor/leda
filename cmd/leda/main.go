// Command leda builds dependency graphs and isolates context for LLM tools.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jgabor/leda/internal/extract"
	"github.com/jgabor/leda/internal/leda"
	"github.com/jgabor/leda/internal/parser"
)

var version = "0.4.2"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		usage(stderr)
		return fmt.Errorf("leda: no command specified")
	}

	switch args[0] {
	case "build":
		return cmdBuild(args[1:], stdout, stderr)
	case "query":
		return cmdQuery(args[1:], stdout, stderr)
	case "stats":
		return cmdStats(args[1:], stdout, stderr)
	case "extract":
		return cmdExtract(args[1:], stdout, stderr)
	case "serve":
		return cmdServe(args[1:], stderr)
	case "version", "-v", "--version":
		_, _ = fmt.Fprintln(stdout, version)
		return nil
	case "help", "-h", "--help":
		usage(stderr)
		return nil
	default:
		usage(stderr)
		return fmt.Errorf("leda: unknown command %q", args[0])
	}
}

func usage(w io.Writer) {
	_, _ = fmt.Fprintln(w, `Usage: leda <command> [options]

Commands:
  build    Build and serialize a dependency graph
  query    Query the graph with a natural language prompt
  stats    Print graph statistics
  extract  Extract structured codebase facts as JSON
  serve    Start MCP server
  version  Print version

Run 'leda <command> -h' for command-specific help.

Author: Jonathan Gabor (github.com/jgabor)
URL:    https://github.com/jgabor/leda`)
}

func cmdBuild(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "Root directory to scan")
	output := fs.String("output", ".leda", "Output file path")
	lang := fs.String("lang", "", "Comma-separated language filter (go,ts,py)")
	exclude := fs.String("exclude", "", "Comma-separated glob patterns to exclude")
	format := fs.String("format", "text", "Output format: text, json")
	dryRun := fs.Bool("dry-run", false, "List files that would be parsed without writing a graph")
	noGitIgnore := fs.Bool("no-gitignore", false, "Do not respect .gitignore files")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var opts []leda.Option
	if *noGitIgnore {
		opts = append(opts, leda.WithGitIgnore(false))
	}
	if *lang != "" {
		exts := langToExtensions(*lang)
		if len(exts) > 0 {
			opts = append(opts, leda.WithExtensions(exts...))
		}
	}
	if *exclude != "" {
		patterns := strings.Split(*exclude, ",")
		opts = append(opts, leda.WithExclude(patterns...))
	}

	if *format == "text" {
		_, _ = fmt.Fprintf(stderr, "leda: building graph from %s\n", *root)
	}
	g, err := leda.BuildGraph(*root, opts...)
	if err != nil {
		return fmt.Errorf("leda: %w", err)
	}

	stats := g.Stats()

	if *dryRun {
		switch *format {
		case "json":
			result := map[string]any{
				"dry_run": true,
				"nodes":   stats.NodeCount,
				"edges":   stats.EdgeCount,
				"files":   g.Nodes(),
			}
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(result)
		default:
			_, _ = fmt.Fprintf(stderr, "leda: dry run — %d nodes, %d edges (no graph written)\n", stats.NodeCount, stats.EdgeCount)
			for _, f := range g.Nodes() {
				_, _ = fmt.Fprintln(stdout, f)
			}
		}
		return nil
	}

	if err := g.SaveToFile(*output); err != nil {
		return fmt.Errorf("leda: %w", err)
	}

	switch *format {
	case "json":
		result := map[string]any{
			"graph_path": *output,
			"nodes":      stats.NodeCount,
			"edges":      stats.EdgeCount,
			"components": stats.Components,
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(result)
	default:
		_, _ = fmt.Fprintf(stderr, "leda: wrote %s (%d nodes, %d edges)\n", *output, stats.NodeCount, stats.EdgeCount)
	}
	return nil
}

func cmdQuery(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("query", flag.ContinueOnError)
	fs.SetOutput(stderr)
	graphPath := fs.String("graph", ".leda", "Path to serialized graph")
	format := fs.String("format", "files", "Output format: files, llm, json")
	maxFiles := fs.Int("max-files", 0, "Maximum number of files to return")
	maxTokens := fs.Int("max-tokens", 0, "Maximum estimated tokens")
	strategy := fs.String("strategy", "filename", "Seed strategy: filename, symbol, path")
	exclude := fs.String("exclude", "", "Comma-separated patterns to exclude from results")
	if err := fs.Parse(args); err != nil {
		return err
	}

	prompt := strings.Join(fs.Args(), " ")
	if prompt == "" {
		return fmt.Errorf("leda: query requires a prompt")
	}

	g, err := leda.LoadFromFile(*graphPath)
	if err != nil {
		return fmt.Errorf("leda: %w", err)
	}

	var opts []leda.QueryOption
	if *maxFiles > 0 {
		opts = append(opts, leda.WithMaxFiles(*maxFiles))
	}
	if *maxTokens > 0 {
		opts = append(opts, leda.WithMaxTokens(*maxTokens))
	}
	switch *strategy {
	case "symbol":
		opts = append(opts, leda.WithSeedStrategy(leda.SeedSymbol))
	case "path":
		opts = append(opts, leda.WithSeedStrategy(leda.SeedPath))
	}
	if *exclude != "" {
		patterns := strings.Split(*exclude, ",")
		opts = append(opts, leda.WithQueryExclude(patterns...))
	}

	ctx := g.IsolateContext(prompt, opts...)

	switch *format {
	case "files":
		for _, f := range ctx.Files {
			_, _ = fmt.Fprintln(stdout, f)
		}
		_, _ = fmt.Fprintf(stderr, "leda: %d files, ~%d tokens\n", len(ctx.Files), ctx.TokenCount)
	case "llm":
		out, err := ctx.FormatForLLM()
		if err != nil {
			return fmt.Errorf("leda: %w", err)
		}
		_, _ = fmt.Fprint(stdout, out)
	case "json":
		result := map[string]any{
			"files":       ctx.Files,
			"seeds":       ctx.Seeds,
			"token_count": ctx.TokenCount,
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(result)
	default:
		return fmt.Errorf("leda: unknown format %q", *format)
	}
	return nil
}

func cmdStats(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("stats", flag.ContinueOnError)
	fs.SetOutput(stderr)
	graphPath := fs.String("graph", ".leda", "Path to serialized graph")
	format := fs.String("format", "text", "Output format: text, json")
	if err := fs.Parse(args); err != nil {
		return err
	}

	g, err := leda.LoadFromFile(*graphPath)
	if err != nil {
		return fmt.Errorf("leda: %w", err)
	}

	stats := g.Stats()

	switch *format {
	case "json":
		result := map[string]any{
			"nodes":      stats.NodeCount,
			"edges":      stats.EdgeCount,
			"components": stats.Components,
		}
		if len(stats.TopFanOut) > 0 {
			fanOut := make([]map[string]any, len(stats.TopFanOut))
			for i, e := range stats.TopFanOut {
				fanOut[i] = map[string]any{"file": e.File, "count": e.Count}
			}
			result["top_fan_out"] = fanOut
		}
		if len(stats.TopFanIn) > 0 {
			fanIn := make([]map[string]any, len(stats.TopFanIn))
			for i, e := range stats.TopFanIn {
				fanIn[i] = map[string]any{"file": e.File, "count": e.Count}
			}
			result["top_fan_in"] = fanIn
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(result)
	default:
		_, _ = fmt.Fprintf(stdout, "Nodes:      %d\n", stats.NodeCount)
		_, _ = fmt.Fprintf(stdout, "Edges:      %d\n", stats.EdgeCount)
		_, _ = fmt.Fprintf(stdout, "Components: %d\n", stats.Components)

		if len(stats.TopFanOut) > 0 {
			_, _ = fmt.Fprintln(stdout, "\nTop Fan-Out (most imports):")
			for _, e := range stats.TopFanOut {
				_, _ = fmt.Fprintf(stdout, "  %3d  %s\n", e.Count, e.File)
			}
		}
		if len(stats.TopFanIn) > 0 {
			_, _ = fmt.Fprintln(stdout, "\nTop Fan-In (most imported):")
			for _, e := range stats.TopFanIn {
				_, _ = fmt.Fprintf(stdout, "  %3d  %s\n", e.Count, e.File)
			}
		}
	}
	return nil
}

func cmdExtract(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("extract", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "Project root path")
	format := fs.String("format", "", "Output format (json)")
	lang := fs.String("lang", "", "Comma-separated language filter (go,ts,py)")
	noGitIgnore := fs.Bool("no-gitignore", false, "Do not respect .gitignore files")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *format != "json" {
		return fmt.Errorf("leda: extract requires --format json")
	}

	var opts []leda.Option
	if *noGitIgnore {
		opts = append(opts, leda.WithGitIgnore(false))
	}
	reg := parser.DefaultRegistry()
	if *lang != "" {
		exts := langToExtensions(*lang)
		if len(exts) > 0 {
			opts = append(opts, leda.WithExtensions(exts...))
		}
	}

	_, _ = fmt.Fprintln(stderr, "leda: extracting codebase facts from", *root)
	g, err := leda.BuildGraph(*root, opts...)
	if err != nil {
		return fmt.Errorf("leda: %w", err)
	}

	result, err := extract.Run(*root, g, reg)
	if err != nil {
		return fmt.Errorf("leda: %w", err)
	}

	enc := json.NewEncoder(stdout)
	return enc.Encode(result)
}

func cmdServe(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(stderr, "leda: starting MCP server on stdio")
	return serveMCP()
}

// langAliases maps short names and aliases to canonical language names
// as returned by Parser.Language().
var langAliases = map[string]string{
	"go":         "go",
	"ts":         "typescript",
	"typescript": "typescript",
	"js":         "javascript",
	"javascript": "javascript",
	"py":         "python",
	"python":     "python",
	"rs":         "rust",
	"rust":       "rust",
	"java":       "java",
	"c":          "c",
	"cpp":        "cpp",
	"c++":        "cpp",
	"rb":         "ruby",
	"ruby":       "ruby",
	"php":        "php",
}

func langToExtensions(lang string) []string {
	reg := parser.DefaultRegistry()
	var exts []string
	for _, l := range strings.Split(lang, ",") {
		canonical := langAliases[strings.TrimSpace(strings.ToLower(l))]
		if p := reg.ForLanguage(canonical); p != nil {
			exts = append(exts, p.Extensions()...)
		}
	}
	return exts
}
