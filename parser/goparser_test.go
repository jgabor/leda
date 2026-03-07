package parser

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestGoParserImports(t *testing.T) {
	dir := t.TempDir()
	src := `package main

import (
	"fmt"
	"os"

	"example.com/myapp/auth"
	"example.com/myapp/server"
)

func main() {
	fmt.Println("hello")
}
`
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	p := DefaultRegistry().ForExtension(".go")
	imports, err := p.ParseImports(path)
	if err != nil {
		t.Fatalf("ParseImports: %v", err)
	}

	want := []string{"fmt", "os", "example.com/myapp/auth", "example.com/myapp/server"}
	sort.Strings(imports)
	sort.Strings(want)

	if len(imports) != len(want) {
		t.Fatalf("imports: got %v, want %v", imports, want)
	}
	for i := range want {
		if imports[i] != want[i] {
			t.Errorf("imports[%d]: got %s, want %s", i, imports[i], want[i])
		}
	}
}

func TestGoParserSymbols(t *testing.T) {
	dir := t.TempDir()
	src := `package auth

// Authenticate checks credentials.
func Authenticate(user, pass string) bool {
	return true
}

func privateHelper() {}

// UserRole is a type.
type UserRole int

const (
	RoleAdmin UserRole = iota
	RoleUser
	internalConst = "secret"
)

var ExportedVar = "hello"
var unexported = "bye"
`
	path := filepath.Join(dir, "auth.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	p := DefaultRegistry().ForExtension(".go")
	symbols, err := p.ParseSymbols(path)
	if err != nil {
		t.Fatalf("ParseSymbols: %v", err)
	}

	// Should include: Authenticate, UserRole, RoleAdmin, RoleUser, ExportedVar
	// Should NOT include: privateHelper, internalConst, unexported
	want := map[string]bool{
		"Authenticate": true,
		"UserRole":     true,
		"RoleAdmin":    true,
		"RoleUser":     true,
		"ExportedVar":  true,
	}

	for _, s := range symbols {
		if !want[s] {
			t.Errorf("unexpected exported symbol: %s", s)
		}
		delete(want, s)
	}
	for s := range want {
		t.Errorf("missing exported symbol: %s", s)
	}
}
