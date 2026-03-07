# leda — Linked-Edge Dependency Analyzer

## What this is

A Go CLI + MCP server for dependency-graph-based context isolation.
Given a codebase and a natural-language prompt, leda builds a directed
dependency graph from source files, seeds entry points from the prompt, and
traverses the graph to return only the files an LLM actually needs.

## Architecture

```
cmd/leda/             - CLI (build, query, stats, serve)
  mcp.go              - MCP server over stdio (via mcp-go)
internal/
  leda/               - Core library: BuildGraph, IsolateContext, Load/Save
    graph.go          - Directed graph data structure + traversal (BFS reachable,
                        shortest path, subgraph extraction)
    seed.go           - Prompt tokenization, stop-word filtering, identifier splitting,
                        seed matching (filename, symbol, path strategies)
    gitignore.go      - .gitignore pattern matching
  parser/             - Parser interface + registry (keyed by extension and language)
    treesitter.go     - Tree-sitter based parser (imports + symbols via queries)
    languages.go      - Per-language tree-sitter configs (query patterns, grammars)
  resolve/            - Import path → absolute file path resolution
    resolve.go        - Relative resolver, Go module resolver, multi-chain
                        Resolver.Resolve returns []string (multi-file resolution)
testdata/             - Multi-file Go project for integration tests
```

## Key design decisions

- **Language-agnostic core**: `BuildGraph` and `IsolateContext` contain no language-specific code.
  All language knowledge lives in `parser.Parser` and `resolve.Resolver` implementations.
- **Minimal external deps**: graph and resolver use only stdlib; parsers use tree-sitter
- **Resolver returns `[]string`**: supports both single-file (TS/Python relative) and
  multi-file (Go package directory) resolution without special-casing in the graph builder
- **Graph serialization** uses `encoding/gob` with a version number for forward compat; default output is `.leda`
- **Seed matching** tokenizes the prompt, strips stop words, splits identifiers on
  camelCase/snake_case boundaries, and scores nodes by term overlap via a shared
  `seedWith` function parameterized by a scoring function
- **IsolateContext algorithm**: 1 seed → seed + descendants; 2+ seeds → shortest
  paths between pairs + descendants of targets; no seeds → full fallback
- **Parsers are pluggable** via the `parser.Parser` interface and `parser.Registry`,
  keyed by both file extension and language name

## Conventions

- Use standard library packages whenever possible, unless stated otherwise
- Run `go vet` for static analysis, `go test` for testing
- Avoid adding comments unless critical for understanding the code; remove rather than add
- ALWAYS maintain a small, easily auditable codebase
- NEVER implement backwards compatibility unless explicitly requested
- NEVER add or commit to git unless explicitly requested
- NEVER add conversational comments; all comments must be evergreen and timeless
- NEVER add placeholder data or functionality; all data should be fetched or read from storage

## Mandatory principles that MUST be upheld

- DTC: documentation defines intent, tests enforce it, code implements it.
- SOLID: use SOLID principles to keep designs modular and maintainable; each part has a clear responsibility, depends on abstractions, and can evolve without breaking the whole system.
- DRY (Don't Repeat Yourself): avoid duplicate logic and knowledge; capture it once in a single, well-named place so changes happen in one spot and behavior stays consistent.
- YAGNI (You Aren't Gonna Need It): don't build features "just in case"; implement what's needed now, keep things simple, and add complexity only when real requirements demand it.

## Running tests

```bash
go test ./... -race
```

## Building

```bash
go build ./cmd/leda
```

## Adding a new language parser

1. Add a `langConfig` entry in `internal/parser/languages.go` with tree-sitter query patterns
2. `go get github.com/tree-sitter/tree-sitter-LANG/bindings/go@latest`
3. If the language has non-relative imports (like Go modules), implement a `resolve.Resolver`
4. If a new resolver was added, register it in `internal/resolve.DefaultResolver()`
5. Add a short alias in `cmd/leda/main.go` `langAliases` for CLI `--lang` support
6. Add test files in `testdata/`

No changes to `BuildGraph`, `IsolateContext`, or the MCP server are needed.
