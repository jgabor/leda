package leda

import (
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// SeedStrategy controls how prompt terms map to graph nodes.
type SeedStrategy int

const (
	// SeedFilename matches prompt words against file basenames.
	SeedFilename SeedStrategy = iota
	// SeedSymbol matches against exported symbols.
	SeedSymbol
	// SeedPath matches against full relative paths.
	SeedPath
	// SeedCustom uses a user-provided seed function.
	SeedCustom
)

// seedResult pairs a node path with its match score.
type seedResult struct {
	path  string
	score int
}

// stopWords that are filtered from the prompt before matching.
var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true,
	"was": true, "were": true, "be": true, "been": true, "being": true,
	"have": true, "has": true, "had": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "could": true, "should": true,
	"may": true, "might": true, "shall": true, "can": true,
	"in": true, "on": true, "at": true, "to": true, "for": true,
	"of": true, "with": true, "by": true, "from": true, "about": true,
	"into": true, "through": true, "during": true, "before": true, "after": true,
	"above": true, "below": true, "between": true, "under": true,
	"and": true, "but": true, "or": true, "nor": true, "not": true,
	"so": true, "yet": true, "both": true, "either": true, "neither": true,
	"it": true, "its": true, "this": true, "that": true, "these": true,
	"those": true, "i": true, "me": true, "my": true, "we": true,
	"you": true, "your": true, "he": true, "she": true, "they": true,
	"what": true, "which": true, "who": true, "whom": true, "whose": true,
	"where": true, "when": true, "why": true, "how": true,
	"all": true, "each": true, "every": true, "any": true, "some": true,
	"no": true, "if": true, "then": true, "than": true, "as": true,
	"up": true, "out": true, "just": true, "also": true, "very": true,
	"there": true, "here": true, "only": true,
	// Code-specific stop words.
	"file": true, "files": true, "code": true, "function": true,
	"method": true, "class": true, "find": true, "show": true,
	"look": true, "get": true, "set": true,
}

// tokenizePrompt splits a prompt into lowercase words, removes stop words and punctuation.
func tokenizePrompt(prompt string) []string {
	words := strings.Fields(strings.ToLower(prompt))
	var result []string
	for _, w := range words {
		// Strip punctuation from edges.
		w = strings.TrimFunc(w, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-'
		})
		if w == "" || stopWords[w] {
			continue
		}
		result = append(result, w)
	}
	return result
}

// splitIdentifier splits a camelCase or snake_case identifier into lowercase parts.
func splitIdentifier(name string) []string {
	// First split on underscores, hyphens, dots.
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '-' || r == '.'
	})
	var result []string
	for _, part := range parts {
		result = append(result, splitCamelCase(part)...)
	}
	return result
}

// splitCamelCase splits a camelCase string into lowercase words.
func splitCamelCase(s string) []string {
	var words []string
	runes := []rune(s)
	start := 0
	for i := 1; i < len(runes); i++ {
		if unicode.IsUpper(runes[i]) && (i > start) {
			words = append(words, strings.ToLower(string(runes[start:i])))
			start = i
		}
	}
	if start < len(runes) {
		words = append(words, strings.ToLower(string(runes[start:])))
	}
	return words
}

// scoreFunc scores a node against prompt terms. Higher score = better match.
type scoreFunc func(terms []string, path string, info *NodeInfo) int

// seedWith runs the common seed pattern: tokenize prompt, score all nodes, return sorted.
func seedWith(prompt string, g *Graph, scorer scoreFunc) []string {
	terms := tokenizePrompt(prompt)
	if len(terms) == 0 {
		return nil
	}

	var results []seedResult
	for path, info := range g.nodes {
		if score := scorer(terms, path, info); score > 0 {
			results = append(results, seedResult{path: path, score: score})
		}
	}

	return sortedSeeds(results)
}

func seedByFilename(prompt string, g *Graph) []string {
	return seedWith(prompt, g, func(terms []string, path string, info *NodeInfo) int {
		score := 0
		base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		baseParts := splitIdentifier(base)
		dirParts := splitIdentifier(filepath.Dir(info.RelPath))

		for _, term := range terms {
			for _, bp := range baseParts {
				if bp == term || strings.Contains(bp, term) || strings.Contains(term, bp) {
					score++
					break
				}
			}
			for _, dp := range dirParts {
				if dp == term || strings.Contains(dp, term) {
					score++
					break
				}
			}
		}
		return score
	})
}

func seedBySymbol(prompt string, g *Graph) []string {
	return seedWith(prompt, g, func(terms []string, _ string, info *NodeInfo) int {
		score := 0
		for _, sym := range info.Symbols {
			symParts := splitIdentifier(sym)
			for _, term := range terms {
				for _, sp := range symParts {
					if sp == term || strings.Contains(sp, term) || strings.Contains(term, sp) {
						score++
						break
					}
				}
			}
		}
		return score
	})
}

func seedByPath(prompt string, g *Graph) []string {
	return seedWith(prompt, g, func(terms []string, _ string, info *NodeInfo) int {
		score := 0
		relParts := splitIdentifier(info.RelPath)
		for _, term := range terms {
			for _, rp := range relParts {
				if rp == term || strings.Contains(rp, term) {
					score++
					break
				}
			}
		}
		return score
	})
}

func sortedSeeds(results []seedResult) []string {
	if len(results) == 0 {
		return nil
	}
	// Sort by score descending, then path for stability.
	sortSeedResults(results)
	seeds := make([]string, len(results))
	for i, r := range results {
		seeds[i] = r.path
	}
	return seeds
}

func sortSeedResults(results []seedResult) {
	sort.Slice(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		return results[i].path < results[j].path
	})
}
