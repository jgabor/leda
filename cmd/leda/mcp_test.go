package main

import (
	"encoding/json"
	"testing"
)

func TestHandleInitialize(t *testing.T) {
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
	}

	resp := handleRequest(req)

	if resp.ID != 1 {
		t.Errorf("ID: got %v, want 1", resp.ID)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("result is not a map")
	}
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("protocolVersion: got %v", result["protocolVersion"])
	}
}

func TestHandleToolsList(t *testing.T) {
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	}

	resp := handleRequest(req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("result is not a map")
	}
	tools, ok := result["tools"].([]mcpToolInfo)
	if !ok {
		t.Fatal("tools is not []mcpToolInfo")
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if tools[0].Name != "leda_build_graph" {
		t.Errorf("tool[0].Name: got %s, want leda_build_graph", tools[0].Name)
	}
	if tools[1].Name != "leda_isolate_context" {
		t.Errorf("tool[1].Name: got %s, want leda_isolate_context", tools[1].Name)
	}
}

func TestHandleUnknownMethod(t *testing.T) {
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "unknown/method",
	}

	resp := handleRequest(req)

	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("error code: got %d, want -32601", resp.Error.Code)
	}
}

func TestHandleToolCallUnknownTool(t *testing.T) {
	params, _ := json.Marshal(map[string]any{
		"name":      "nonexistent_tool",
		"arguments": map[string]any{},
	})

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "tools/call",
		Params:  params,
	}

	resp := handleRequest(req)

	result, ok := resp.Result.(mcpToolResult)
	if !ok {
		t.Fatal("result is not mcpToolResult")
	}
	if !result.IsError {
		t.Error("expected IsError=true for unknown tool")
	}
}

func TestHandleBuildGraphTool(t *testing.T) {
	args := json.RawMessage(`{"root_dir": "../../testdata/goproject"}`)
	params, _ := json.Marshal(map[string]any{
		"name":      "leda_build_graph",
		"arguments": args,
	})

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      5,
		Method:  "tools/call",
		Params:  params,
	}

	resp := handleRequest(req)

	result, ok := resp.Result.(mcpToolResult)
	if !ok {
		t.Fatal("result is not mcpToolResult")
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	if len(result.Content) == 0 {
		t.Fatal("empty content")
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &parsed); err != nil {
		t.Fatalf("invalid JSON in result: %v", err)
	}
	if parsed["nodes"] == nil || parsed["edges"] == nil || parsed["graph_path"] == nil {
		t.Errorf("missing fields in result: %v", parsed)
	}
}
