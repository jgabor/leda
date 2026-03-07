package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jgabor/leda"
)

// JSON-RPC types for MCP protocol.

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpToolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpToolResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

func serveMCP() error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var req jsonRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			writeResponse(jsonRPCResponse{
				JSONRPC: "2.0",
				Error:   &rpcError{Code: -32700, Message: "Parse error"},
			})
			continue
		}

		resp := handleRequest(req)
		writeResponse(resp)
	}

	return scanner.Err()
}

func writeResponse(resp jsonRPCResponse) {
	resp.JSONRPC = "2.0"
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}

func handleRequest(req jsonRPCRequest) jsonRPCResponse {
	switch req.Method {
	case "initialize":
		return jsonRPCResponse{
			ID: req.ID,
			Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]any{
					"tools": map[string]any{},
				},
				"serverInfo": map[string]any{
					"name":    "leda",
					"version": "0.2.0",
				},
			},
		}

	case "notifications/initialized":
		// No response needed for notifications.
		return jsonRPCResponse{ID: req.ID, Result: map[string]any{}}

	case "tools/list":
		return jsonRPCResponse{
			ID: req.ID,
			Result: map[string]any{
				"tools": []mcpToolInfo{
					{
						Name:        "leda_build_graph",
						Description: "Build a dependency graph from a codebase directory",
						InputSchema: json.RawMessage(`{
							"type": "object",
							"properties": {
								"root_dir": {"type": "string", "description": "Path to the codebase root"},
								"languages": {"type": "array", "items": {"type": "string", "enum": ["go", "typescript", "javascript", "python", "rust", "java", "c", "cpp", "ruby", "php"]}, "description": "Languages to parse (default: all)"}
							},
							"required": ["root_dir"]
						}`),
					},
					{
						Name:        "leda_isolate_context",
						Description: "Given a prompt, return only the source files relevant to answer it. Uses dependency graph traversal instead of vector similarity.",
						InputSchema: json.RawMessage(`{
							"type": "object",
							"properties": {
								"prompt": {"type": "string", "description": "Natural language query about the codebase"},
								"graph_path": {"type": "string", "description": "Path to serialized graph (from leda_build_graph)"},
								"max_tokens": {"type": "integer", "description": "Optional token budget cap"},
								"format": {"type": "string", "enum": ["files", "contents", "llm"], "description": "Output format"}
							},
							"required": ["prompt", "graph_path"]
						}`),
					},
				},
			},
		}

	case "tools/call":
		return handleToolCall(req)

	default:
		return jsonRPCResponse{
			ID:    req.ID,
			Error: &rpcError{Code: -32601, Message: fmt.Sprintf("Method not found: %s", req.Method)},
		}
	}
}

func handleToolCall(req jsonRPCRequest) jsonRPCResponse {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return jsonRPCResponse{
			ID:    req.ID,
			Error: &rpcError{Code: -32602, Message: "Invalid params"},
		}
	}

	switch params.Name {
	case "leda_build_graph":
		return toolBuildGraph(req.ID, params.Arguments)
	case "leda_isolate_context":
		return toolIsolateContext(req.ID, params.Arguments)
	default:
		return jsonRPCResponse{
			ID: req.ID,
			Result: mcpToolResult{
				Content: []mcpContent{{Type: "text", Text: fmt.Sprintf("Unknown tool: %s", params.Name)}},
				IsError: true,
			},
		}
	}
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

func toolBuildGraph(id any, args json.RawMessage) jsonRPCResponse {
	var input struct {
		RootDir   string   `json:"root_dir"`
		Languages []string `json:"languages"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult(id, "Invalid arguments: "+err.Error())
	}

	rootDir, err := validatePath(input.RootDir)
	if err != nil {
		return errorResult(id, "Invalid root_dir: "+err.Error())
	}
	input.RootDir = rootDir

	var opts []leda.Option
	if len(input.Languages) > 0 {
		exts := langToExtensions(strings.Join(input.Languages, ","))
		if len(exts) > 0 {
			opts = append(opts, leda.WithExtensions(exts...))
		}
	}

	g, err := leda.BuildGraph(input.RootDir, opts...)
	if err != nil {
		return errorResult(id, err.Error())
	}

	// Save to temp file.
	h := sha256.Sum256([]byte(input.RootDir))
	graphPath := filepath.Join(os.TempDir(), fmt.Sprintf("leda-%x.bin", h[:8]))
	if err := g.SaveToFile(graphPath); err != nil {
		return errorResult(id, err.Error())
	}

	stats := g.Stats()
	result := map[string]any{
		"nodes":      stats.NodeCount,
		"edges":      stats.EdgeCount,
		"graph_path": graphPath,
	}
	resultJSON, _ := json.Marshal(result)

	return jsonRPCResponse{
		ID: id,
		Result: mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: string(resultJSON)}},
		},
	}
}

func toolIsolateContext(id any, args json.RawMessage) jsonRPCResponse {
	var input struct {
		Prompt    string `json:"prompt"`
		GraphPath string `json:"graph_path"`
		MaxTokens int    `json:"max_tokens"`
		Format    string `json:"format"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult(id, "Invalid arguments: "+err.Error())
	}

	graphPath, err := validatePath(input.GraphPath)
	if err != nil {
		return errorResult(id, "Invalid graph_path: "+err.Error())
	}

	g, err := leda.LoadFromFile(graphPath)
	if err != nil {
		return errorResult(id, err.Error())
	}

	var opts []leda.QueryOption
	if input.MaxTokens > 0 {
		opts = append(opts, leda.WithMaxTokens(input.MaxTokens))
	}

	ctx := g.IsolateContext(input.Prompt, opts...)

	var text string
	switch input.Format {
	case "llm":
		text, err = ctx.FormatForLLM()
		if err != nil {
			return errorResult(id, err.Error())
		}
	case "contents":
		text, err = ctx.Contents()
		if err != nil {
			return errorResult(id, err.Error())
		}
	default:
		result := map[string]any{
			"files":       ctx.Files,
			"seeds":       ctx.Seeds,
			"token_count": ctx.TokenCount,
		}
		resultJSON, _ := json.Marshal(result)
		text = string(resultJSON)
	}

	return jsonRPCResponse{
		ID: id,
		Result: mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: text}},
		},
	}
}

func errorResult(id any, msg string) jsonRPCResponse {
	return jsonRPCResponse{
		ID: id,
		Result: mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: msg}},
			IsError: true,
		},
	}
}
