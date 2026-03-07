package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func buildTestGraph(t *testing.T) string {
	t.Helper()
	testDir, err := filepath.Abs("../../testdata/goproject")
	if err != nil {
		t.Fatal(err)
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"root_dir": testDir,
	}

	result, err := handleBuildGraph(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("buildTestGraph: %s", result.Content[0].(mcp.TextContent).Text)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &parsed); err != nil {
		t.Fatal(err)
	}
	return parsed["graph_path"].(string)
}

func TestHandleBuildGraph(t *testing.T) {
	testDir, err := filepath.Abs("../../testdata/goproject")
	if err != nil {
		t.Fatal(err)
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"root_dir": testDir,
	}

	result, err := handleBuildGraph(context.Background(), req)
	if err != nil {
		t.Fatalf("handleBuildGraph returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content[0].(mcp.TextContent).Text)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("invalid JSON in result: %v", err)
	}
	if parsed["nodes"] == nil || parsed["edges"] == nil || parsed["graph_path"] == nil {
		t.Errorf("missing fields in result: %v", parsed)
	}
}

func TestHandleBuildGraphInvalidPath(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"root_dir": "/path/with\x00null",
	}

	result, err := handleBuildGraph(context.Background(), req)
	if err != nil {
		t.Fatalf("handleBuildGraph returned error: %v", err)
	}
	if !result.IsError {
		t.Error("expected tool error for invalid path")
	}
}

func TestHandleBuildGraphWithLanguages(t *testing.T) {
	testDir, err := filepath.Abs("../../testdata/goproject")
	if err != nil {
		t.Fatal(err)
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"root_dir":  testDir,
		"languages": []any{"go"},
	}

	result, err := handleBuildGraph(context.Background(), req)
	if err != nil {
		t.Fatalf("handleBuildGraph returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content[0].(mcp.TextContent).Text)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["nodes"].(float64) == 0 {
		t.Error("expected nodes for Go language filter")
	}
}

func TestHandleIsolateContext(t *testing.T) {
	graphPath := buildTestGraph(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"prompt":     "auth middleware",
		"graph_path": graphPath,
	}

	result, err := handleIsolateContext(context.Background(), req)
	if err != nil {
		t.Fatalf("handleIsolateContext returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content[0].(mcp.TextContent).Text)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &parsed); err != nil {
		t.Fatalf("invalid JSON in result: %v", err)
	}
	if parsed["files"] == nil {
		t.Error("missing files in result")
	}
}

func TestHandleIsolateContextInvalidPath(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"prompt":     "auth",
		"graph_path": "/path/with\x00null",
	}

	result, err := handleIsolateContext(context.Background(), req)
	if err != nil {
		t.Fatalf("handleIsolateContext returned error: %v", err)
	}
	if !result.IsError {
		t.Error("expected tool error for invalid graph_path")
	}
}

func TestHandleIsolateContextFormats(t *testing.T) {
	graphPath := buildTestGraph(t)

	for _, format := range []string{"files", "contents", "llm"} {
		t.Run(format, func(t *testing.T) {
			req := mcp.CallToolRequest{}
			req.Params.Arguments = map[string]any{
				"prompt":     "auth",
				"graph_path": graphPath,
				"format":     format,
			}

			result, err := handleIsolateContext(context.Background(), req)
			if err != nil {
				t.Fatalf("handleIsolateContext(%s) returned error: %v", format, err)
			}
			if result.IsError {
				t.Fatalf("unexpected tool error for format %s", format)
			}
			text := result.Content[0].(mcp.TextContent).Text
			if text == "" {
				t.Errorf("empty result for format %s", format)
			}
		})
	}
}

func TestHandleIsolateContextWithMaxTokens(t *testing.T) {
	graphPath := buildTestGraph(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"prompt":     "auth",
		"graph_path": graphPath,
		"max_tokens": float64(50),
	}

	result, err := handleIsolateContext(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatal("unexpected error")
	}
}

func TestHandleIsolateContextMissingPrompt(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"graph_path": "/tmp/test.bin",
	}

	result, err := handleIsolateContext(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for missing prompt")
	}
}

func TestHandleBuildGraphMissingRootDir(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := handleBuildGraph(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for missing root_dir")
	}
}

func TestValidatePath(t *testing.T) {
	tests := []struct {
		path    string
		wantErr bool
	}{
		{"/tmp/test", false},
		{"/path/with\x00null", true},
		{"/path/with?query", true},
		{"/path/with#fragment", true},
		{"/path/with%encoded", true},
		{"/tmp/../tmp/test", false},
	}

	for _, tt := range tests {
		_, err := validatePath(tt.path)
		if (err != nil) != tt.wantErr {
			t.Errorf("validatePath(%q): err=%v, wantErr=%v", tt.path, err, tt.wantErr)
		}
	}
}

func TestLangToExtensions(t *testing.T) {
	exts := langToExtensions("go")
	if len(exts) == 0 {
		t.Error("langToExtensions(go): got empty")
	}
	found := false
	for _, e := range exts {
		if e == ".go" {
			found = true
		}
	}
	if !found {
		t.Error("langToExtensions(go): .go not found")
	}

	exts = langToExtensions("go,ts")
	if len(exts) < 2 {
		t.Errorf("langToExtensions(go,ts): got %v", exts)
	}

	exts = langToExtensions("nonexistent")
	if len(exts) != 0 {
		t.Errorf("langToExtensions(nonexistent): got %v", exts)
	}
}

func TestHandleIsolateContextNonexistentGraph(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"prompt":     "test",
		"graph_path": "/tmp/nonexistent-graph-12345.bin",
	}

	result, err := handleIsolateContext(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for nonexistent graph")
	}
}
