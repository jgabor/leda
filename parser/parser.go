// Package parser provides interfaces and implementations for extracting
// import dependencies and exported symbols from source files.
package parser

// Parser extracts import/dependency edges from a source file.
type Parser interface {
	// Language returns the language name (e.g., "go", "typescript", "python").
	Language() string

	// Extensions returns file extensions this parser handles (e.g., ".go", ".ts").
	Extensions() []string

	// ParseImports returns the raw import paths found in the file.
	ParseImports(filePath string) ([]string, error)

	// ParseSymbols returns exported symbol names from the file.
	// May return nil if the parser doesn't support symbol extraction.
	ParseSymbols(filePath string) ([]string, error)
}

// Registry holds parsers keyed by extension and language.
type Registry struct {
	byExt  map[string]Parser
	byLang map[string]Parser
}

// NewRegistry creates a registry from the given parsers.
func NewRegistry(parsers ...Parser) *Registry {
	r := &Registry{
		byExt:  make(map[string]Parser),
		byLang: make(map[string]Parser),
	}
	for _, p := range parsers {
		for _, ext := range p.Extensions() {
			r.byExt[ext] = p
		}
		r.byLang[p.Language()] = p
	}
	return r
}

// ForExtension returns the parser registered for the given extension, or nil.
func (r *Registry) ForExtension(ext string) Parser {
	return r.byExt[ext]
}

// ForLanguage returns the parser registered for the given language name, or nil.
func (r *Registry) ForLanguage(lang string) Parser {
	return r.byLang[lang]
}

// RegisteredExtensions returns all file extensions the registry handles.
func (r *Registry) RegisteredExtensions() []string {
	exts := make([]string, 0, len(r.byExt))
	for ext := range r.byExt {
		exts = append(exts, ext)
	}
	return exts
}

// Languages returns all registered language names.
func (r *Registry) Languages() []string {
	langs := make([]string, 0, len(r.byLang))
	for lang := range r.byLang {
		langs = append(langs, lang)
	}
	return langs
}

// DefaultRegistry returns a registry with tree-sitter parsers for all supported languages.
func DefaultRegistry() *Registry {
	parsers := make([]Parser, len(defaultLanguages))
	for i, cfg := range defaultLanguages {
		parsers[i] = newTreeSitterParser(cfg)
	}
	return NewRegistry(parsers...)
}
