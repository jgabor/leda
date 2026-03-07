package parser

import (
	"fmt"
	"os"
	"unsafe"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// langConfig defines tree-sitter queries for a single language.
type langConfig struct {
	name       string
	extensions []string
	langPtr    unsafe.Pointer // from tree-sitter grammar's Language() function
	imports    string         // tree-sitter query pattern capturing imports as @path
	symbols    string         // tree-sitter query pattern capturing symbols as @name
}

// TreeSitterParser extracts imports and symbols using tree-sitter grammars.
type TreeSitterParser struct {
	cfg  langConfig
	lang *tree_sitter.Language
}

func newTreeSitterParser(cfg langConfig) *TreeSitterParser {
	return &TreeSitterParser{
		cfg:  cfg,
		lang: tree_sitter.NewLanguage(cfg.langPtr),
	}
}

func (p *TreeSitterParser) Language() string     { return p.cfg.name }
func (p *TreeSitterParser) Extensions() []string { return p.cfg.extensions }

func (p *TreeSitterParser) ParseImports(filePath string) ([]string, error) {
	if p.cfg.imports == "" {
		return nil, nil
	}
	return p.query(filePath, p.cfg.imports, "path")
}

func (p *TreeSitterParser) ParseSymbols(filePath string) ([]string, error) {
	if p.cfg.symbols == "" {
		return nil, nil
	}
	return p.query(filePath, p.cfg.symbols, "name")
}

func (p *TreeSitterParser) query(filePath, pattern, captureName string) ([]string, error) {
	src, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("leda: reading %s: %w", filePath, err)
	}

	parser := tree_sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(p.lang); err != nil {
		return nil, fmt.Errorf("leda: setting language %s: %w", p.cfg.name, err)
	}

	tree := parser.Parse(src, nil)
	defer tree.Close()

	q, queryErr := tree_sitter.NewQuery(p.lang, pattern)
	if q == nil {
		return nil, fmt.Errorf("leda: compiling query for %s: %v", p.cfg.name, queryErr)
	}
	defer q.Close()

	captureIdx, ok := q.CaptureIndexForName(captureName)
	if !ok {
		return nil, nil
	}

	qc := tree_sitter.NewQueryCursor()
	defer qc.Close()

	matches := qc.Matches(q, tree.RootNode(), src)

	seen := map[string]bool{}
	var results []string
	for {
		match := matches.Next()
		if match == nil {
			break
		}
		nodes := match.NodesForCaptureIndex(uint(captureIdx))
		for _, node := range nodes {
			val := node.Utf8Text(src)
			if val != "" && !seen[val] {
				seen[val] = true
				results = append(results, val)
			}
		}
	}
	return results, nil
}
