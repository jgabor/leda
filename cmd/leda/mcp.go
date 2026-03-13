package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jgabor/leda/internal/leda"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func serveMCP() error {
	s := server.NewMCPServer("leda", version,
		server.WithToolCapabilities(false),
	)

	s.AddTool(
		mcp.NewTool("leda_build_graph",
			mcp.WithDescription("Build a dependency graph from a codebase directory"),
			mcp.WithString("root_dir",
				mcp.Required(),
				mcp.Description("Path to the codebase root"),
			),
			mcp.WithArray("languages",
				mcp.Description("Languages to parse (default: all)"),
				mcp.WithStringEnumItems([]string{"go", "typescript", "javascript", "python", "rust", "java", "c", "cpp", "ruby", "php"}),
			),
		),
		handleBuildGraph,
	)

	s.AddTool(
		mcp.NewTool("leda_isolate_context",
			mcp.WithDescription("Given a prompt, return only the source files relevant to answer it. Uses dependency graph traversal instead of vector similarity."),
			mcp.WithString("prompt",
				mcp.Required(),
				mcp.Description("Natural language query about the codebase"),
			),
			mcp.WithString("graph_path",
				mcp.Required(),
				mcp.Description("Path to serialized graph (from leda_build_graph)"),
			),
			mcp.WithNumber("max_tokens",
				mcp.Description("Optional token budget cap"),
			),
			mcp.WithString("format",
				mcp.Description("Output format: files, contents, llm"),
				mcp.Enum("files", "contents", "llm"),
			),
			mcp.WithNumber("max_files",
				mcp.Description("Maximum number of files to return (default: 20)"),
			),
			mcp.WithString("exclude",
				mcp.Description("Comma-separated patterns to exclude from results"),
			),
		),
		handleIsolateContext,
	)

	return server.ServeStdio(s)
}

// validatePath ensures a path is absolute after cleaning, contains no traversal
// sequences, and contains no control characters or query-string fragments.
func validatePath(p string) (string, error) {
	for _, r := range p {
		if r < 0x20 {
			return "", fmt.Errorf("path contains control character: %q", p)
		}
	}
	if strings.ContainsAny(p, "?#") {
		return "", fmt.Errorf("path contains invalid character: %q", p)
	}
	if strings.Contains(p, "%") {
		return "", fmt.Errorf("path contains percent-encoding: %q", p)
	}

	cleaned := filepath.Clean(p)
	abs, err := filepath.Abs(cleaned)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	return abs, nil
}

func handleBuildGraph(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rootDir, err := req.RequireString("root_dir")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	rootDir, err = validatePath(rootDir)
	if err != nil {
		return mcp.NewToolResultError("Invalid root_dir: " + err.Error()), nil
	}

	var opts []leda.Option
	if langs := req.GetStringSlice("languages", nil); len(langs) > 0 {
		exts := langToExtensions(strings.Join(langs, ","))
		if len(exts) > 0 {
			opts = append(opts, leda.WithExtensions(exts...))
		}
	}

	g, err := leda.BuildGraph(rootDir, opts...)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	h := sha256.Sum256([]byte(rootDir))
	graphPath := filepath.Join(os.TempDir(), fmt.Sprintf("leda-%x.bin", h[:8]))
	if err := g.SaveToFile(graphPath); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	stats := g.Stats()
	result := map[string]any{
		"nodes":      stats.NodeCount,
		"edges":      stats.EdgeCount,
		"graph_path": graphPath,
	}
	resultJSON, _ := json.Marshal(result)

	return mcp.NewToolResultText(string(resultJSON)), nil
}

func handleIsolateContext(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt, err := req.RequireString("prompt")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	graphPathRaw, err := req.RequireString("graph_path")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	graphPath, err := validatePath(graphPathRaw)
	if err != nil {
		return mcp.NewToolResultError("Invalid graph_path: " + err.Error()), nil
	}

	g, err := leda.LoadFromFile(graphPath)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var qopts []leda.QueryOption
	if maxTokens := req.GetInt("max_tokens", 0); maxTokens > 0 {
		qopts = append(qopts, leda.WithMaxTokens(maxTokens))
	}
	if maxFiles := req.GetInt("max_files", 0); maxFiles > 0 {
		qopts = append(qopts, leda.WithMaxFiles(maxFiles))
	}
	if exclude := req.GetString("exclude", ""); exclude != "" {
		patterns := strings.Split(exclude, ",")
		qopts = append(qopts, leda.WithQueryExclude(patterns...))
	}

	isolatedCtx := g.IsolateContext(prompt, qopts...)

	format := req.GetString("format", "files")
	var text string
	switch format {
	case "llm":
		text, err = isolatedCtx.FormatForLLM()
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	case "contents":
		text, err = isolatedCtx.Contents()
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	default:
		result := map[string]any{
			"files":       isolatedCtx.Files,
			"seeds":       isolatedCtx.Seeds,
			"token_count": isolatedCtx.TokenCount,
		}
		resultJSON, _ := json.Marshal(result)
		text = string(resultJSON)
	}

	return mcp.NewToolResultText(text), nil
}
