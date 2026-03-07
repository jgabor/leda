// Command leda builds dependency graphs and isolates context for LLM tools.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jgabor/leda"
	"github.com/jgabor/leda/parser"
)

var version = "0.2.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "build":
		cmdBuild(os.Args[2:])
	case "query":
		cmdQuery(os.Args[2:])
	case "stats":
		cmdStats(os.Args[2:])
	case "serve":
		cmdServe(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Println(version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "leda: unknown command %q\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage: leda <command> [options]

Commands:
  build    Build and serialize a dependency graph
  query    Query the graph with a natural language prompt
  stats    Print graph statistics
  serve    Start MCP server
  version  Print version

Author: Jonathan Gabor (github.com/jgabor)

Run 'leda <command> -h' for command-specific help.`)
}

func cmdBuild(args []string) {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	root := fs.String("root", ".", "Root directory to scan")
	output := fs.String("output", ".leda", "Output file path")
	lang := fs.String("lang", "", "Comma-separated language filter (go,ts,py)")
	exclude := fs.String("exclude", "", "Comma-separated glob patterns to exclude")
	format := fs.String("format", "text", "Output format: text, json")
	dryRun := fs.Bool("dry-run", false, "List files that would be parsed without writing a graph")
	noGitIgnore := fs.Bool("no-gitignore", false, "Do not respect .gitignore files")
	_ = fs.Parse(args)

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
		fmt.Fprintf(os.Stderr, "leda: building graph from %s\n", *root)
	}
	g, err := leda.BuildGraph(*root, opts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "leda: %v\n", err)
		os.Exit(1)
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
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(result)
		default:
			fmt.Fprintf(os.Stderr, "leda: dry run — %d nodes, %d edges (no graph written)\n", stats.NodeCount, stats.EdgeCount)
			for _, f := range g.Nodes() {
				fmt.Println(f)
			}
		}
		return
	}

	if err := g.SaveToFile(*output); err != nil {
		fmt.Fprintf(os.Stderr, "leda: %v\n", err)
		os.Exit(1)
	}

	switch *format {
	case "json":
		result := map[string]any{
			"graph_path": *output,
			"nodes":      stats.NodeCount,
			"edges":      stats.EdgeCount,
			"components": stats.Components,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(result)
	default:
		fmt.Fprintf(os.Stderr, "leda: wrote %s (%d nodes, %d edges)\n", *output, stats.NodeCount, stats.EdgeCount)
	}
}

func cmdQuery(args []string) {
	fs := flag.NewFlagSet("query", flag.ExitOnError)
	graphPath := fs.String("graph", ".leda", "Path to serialized graph")
	format := fs.String("format", "files", "Output format: files, llm, json")
	maxFiles := fs.Int("max-files", 0, "Maximum number of files to return")
	maxTokens := fs.Int("max-tokens", 0, "Maximum estimated tokens")
	strategy := fs.String("strategy", "filename", "Seed strategy: filename, symbol, path")
	_ = fs.Parse(args)

	prompt := strings.Join(fs.Args(), " ")
	if prompt == "" {
		fmt.Fprintln(os.Stderr, "leda: query requires a prompt")
		os.Exit(1)
	}

	g, err := leda.LoadFromFile(*graphPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "leda: %v\n", err)
		os.Exit(1)
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

	ctx := g.IsolateContext(prompt, opts...)

	switch *format {
	case "files":
		for _, f := range ctx.Files {
			fmt.Println(f)
		}
		fmt.Fprintf(os.Stderr, "leda: %d files, ~%d tokens\n", len(ctx.Files), ctx.TokenCount)
	case "llm":
		out, err := ctx.FormatForLLM()
		if err != nil {
			fmt.Fprintf(os.Stderr, "leda: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(out)
	case "json":
		result := map[string]any{
			"files":       ctx.Files,
			"seeds":       ctx.Seeds,
			"token_count": ctx.TokenCount,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(result)
	default:
		fmt.Fprintf(os.Stderr, "leda: unknown format %q\n", *format)
		os.Exit(1)
	}
}

func cmdStats(args []string) {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	graphPath := fs.String("graph", ".leda", "Path to serialized graph")
	format := fs.String("format", "text", "Output format: text, json")
	_ = fs.Parse(args)

	g, err := leda.LoadFromFile(*graphPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "leda: %v\n", err)
		os.Exit(1)
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
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(result)
	default:
		fmt.Printf("Nodes:      %d\n", stats.NodeCount)
		fmt.Printf("Edges:      %d\n", stats.EdgeCount)
		fmt.Printf("Components: %d\n", stats.Components)

		if len(stats.TopFanOut) > 0 {
			fmt.Println("\nTop Fan-Out (most imports):")
			for _, e := range stats.TopFanOut {
				fmt.Printf("  %3d  %s\n", e.Count, e.File)
			}
		}
		if len(stats.TopFanIn) > 0 {
			fmt.Println("\nTop Fan-In (most imported):")
			for _, e := range stats.TopFanIn {
				fmt.Printf("  %3d  %s\n", e.Count, e.File)
			}
		}
	}
}

func cmdServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	transport := fs.String("transport", "stdio", "Transport: stdio")
	_ = fs.Parse(args)

	if *transport != "stdio" {
		fmt.Fprintf(os.Stderr, "leda: unsupported transport %q (only stdio supported)\n", *transport)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "leda: starting MCP server on stdio")
	if err := serveMCP(); err != nil {
		fmt.Fprintf(os.Stderr, "leda: %v\n", err)
		os.Exit(1)
	}
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
