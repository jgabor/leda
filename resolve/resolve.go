// Package resolve turns raw import strings into absolute file paths.
package resolve

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Resolver turns raw import strings into absolute file paths.
type Resolver interface {
	// Resolve attempts to resolve importPath to absolute file path(s).
	// fromFile is the absolute path of the importing file.
	// rootDir is the project root.
	// Returns nil if the import cannot be resolved (e.g., external package).
	Resolve(importPath string, fromFile string, rootDir string) ([]string, error)
}

// Multi chains resolvers, returning the first successful resolution.
func Multi(resolvers ...Resolver) Resolver {
	return &multiResolver{resolvers: resolvers}
}

type multiResolver struct {
	resolvers []Resolver
}

func (m *multiResolver) Resolve(importPath, fromFile, rootDir string) ([]string, error) {
	for _, r := range m.resolvers {
		result, err := r.Resolve(importPath, fromFile, rootDir)
		if err != nil {
			return nil, err
		}
		if len(result) > 0 {
			return result, nil
		}
	}
	return nil, nil
}

// RelativeResolver resolves relative imports (./foo, ../bar) by trying
// various file extensions.
type RelativeResolver struct {
	Extensions []string
}

// NewRelativeResolver creates a resolver for relative imports.
func NewRelativeResolver(extensions []string) *RelativeResolver {
	return &RelativeResolver{Extensions: extensions}
}

func (r *RelativeResolver) Resolve(importPath, fromFile, rootDir string) ([]string, error) {
	if !strings.HasPrefix(importPath, ".") {
		return nil, nil
	}

	dir := filepath.Dir(fromFile)
	base := filepath.Join(dir, importPath)

	// Try exact path first.
	if info, err := os.Stat(base); err == nil && !info.IsDir() {
		return []string{filepath.Clean(base)}, nil
	}

	// Try with extensions.
	for _, ext := range r.Extensions {
		candidate := base + ext
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return []string{filepath.Clean(candidate)}, nil
		}
	}

	// Try as directory with index file.
	for _, ext := range r.Extensions {
		candidate := filepath.Join(base, "index"+ext)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return []string{filepath.Clean(candidate)}, nil
		}
	}

	return nil, nil
}

// GoModResolver resolves Go module-internal imports using go.mod.
// It expands package imports to the individual .go files in the directory.
type GoModResolver struct {
	modulePath string
	rootDir    string
}

// NewGoModResolver creates a resolver that uses go.mod for module path resolution.
func NewGoModResolver(rootDir string) (*GoModResolver, error) {
	modPath := filepath.Join(rootDir, "go.mod")
	data, err := os.ReadFile(modPath)
	if err != nil {
		return nil, fmt.Errorf("leda: reading go.mod: %w", err)
	}

	moduleName := parseModuleName(string(data))
	if moduleName == "" {
		return nil, fmt.Errorf("leda: could not parse module name from go.mod")
	}

	return &GoModResolver{
		modulePath: moduleName,
		rootDir:    rootDir,
	}, nil
}

func parseModuleName(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

func (r *GoModResolver) Resolve(importPath, fromFile, rootDir string) ([]string, error) {
	if !strings.HasPrefix(importPath, r.modulePath) {
		return nil, nil
	}

	relPath := strings.TrimPrefix(importPath, r.modulePath)
	relPath = strings.TrimPrefix(relPath, "/")

	if relPath == "" {
		relPath = "."
	}

	pkgDir := filepath.Join(r.rootDir, relPath)

	info, err := os.Stat(pkgDir)
	if err != nil || !info.IsDir() {
		return nil, nil
	}

	// Expand to all .go files in the package directory.
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return nil, nil
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ".go" && !strings.HasSuffix(entry.Name(), "_test.go") {
			files = append(files, filepath.Join(pkgDir, entry.Name()))
		}
	}
	return files, nil
}

// DefaultResolver returns a resolver chain suitable for most projects.
// extensions should be the set of file extensions the project's parsers handle.
func DefaultResolver(rootDir string, extensions []string) Resolver {
	resolvers := []Resolver{
		NewRelativeResolver(extensions),
	}

	if goMod, err := NewGoModResolver(rootDir); err == nil {
		resolvers = append(resolvers, goMod)
	}

	return Multi(resolvers...)
}
