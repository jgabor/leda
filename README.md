# leda

Leda is a linked-edge dependency analyzer that provides context isolation for LLM tools. Given a codebase and a natural-language prompt, it builds a directed dependency graph from source files, seeds entry points from the prompt, and traverses the graph to return only the files the LLM actually needs.

By deterministically tracing dependency paths rather than relying on vector similarity, leda avoids the irrelevant results common in typical RAG setups — delivering >70% reduction in token usage.

## Install

```bash
go install github.com/jgabor/leda/cmd/leda@latest
```

## CLI

```
leda <command> [options]

Commands:
  build    Build and serialize a dependency graph
  query    Query the graph with a natural language prompt
  stats    Print graph statistics
  serve    Start MCP server (stdio)
```

### Build a graph

```bash
leda build --root ./myproject --output .leda --lang go,ts
leda build --root ./myproject --dry-run              # preview files without writing
leda build --root ./myproject --format json           # machine-readable output
```

### Query with a prompt

```bash
leda query --graph .leda "fix the auth middleware"
leda query --graph .leda --format llm --strategy symbol "database connection"
leda query --graph .leda --format json "auth"    # structured JSON output
```

### Graph statistics

```bash
leda stats --graph .leda
leda stats --graph .leda --format json
```

### MCP server

```bash
leda serve
```

Exposes `leda_build_graph` and `leda_isolate_context` as MCP tools over stdio using [mcp-go](https://github.com/mark3labs/mcp-go). All input paths are validated and canonicalized.

All commands support `--format json` for agent-friendly output.

## Supported languages

All parsers use [tree-sitter](https://tree-sitter.github.io/tree-sitter/) for accurate AST-based import and symbol extraction.

| Language   | CLI alias | Import resolution    |
| ---------- | --------- | -------------------- |
| Go         | `go`      | Go module + relative |
| TypeScript | `ts`      | Relative             |
| JavaScript | `js`      | Relative             |
| Python     | `py`      | Relative             |
| Rust       | `rs`      | Relative             |
| Java       | `java`    | Relative             |
| C          | `c`       | Relative             |
| C++        | `cpp`     | Relative             |
| Ruby       | `rb`      | Relative             |
| PHP        | `php`     | Relative             |

New languages can be added by defining a `langConfig` in `internal/parser/languages.go` with tree-sitter query patterns.

## Project structure

```
cmd/leda/           CLI and MCP server
internal/
  leda/             Graph building, context isolation, seeding
  parser/           Tree-sitter parsers (imports + symbols)
  resolve/          Import path → file resolution
testdata/           Integration test fixtures
```

## How it works

1. **Build**: Walk the project tree, parse each file for symbols and imports, construct a directed graph where edges represent dependencies.
2. **Seed**: Tokenize the prompt, split identifiers on camelCase/snake_case boundaries, and match against filenames, symbols, or paths to find entry-point nodes.
3. **Isolate**: From seed nodes, traverse the graph (descendants for single seeds, shortest paths + descendants for multiple seeds) to collect the relevant subgraph.
4. **Budget**: Optionally cap results by file count or estimated token count.

## Acknowledgements

Leda is inspired by [graph-oriented-generation](https://github.com/dchisholm125/graph-oriented-generation).

## License

MIT

## Author

Jonathan Gabor ([@jgabor](https://github.com/jgabor))
