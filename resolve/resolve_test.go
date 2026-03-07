package resolve

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestRelativeResolver(t *testing.T) {
	dir := t.TempDir()

	mustWriteFile(t, filepath.Join(dir, "foo.ts"), []byte(""))
	mustMkdirAll(t, filepath.Join(dir, "bar"))
	mustWriteFile(t, filepath.Join(dir, "bar", "index.ts"), []byte(""))
	mustWriteFile(t, filepath.Join(dir, "baz.go"), []byte(""))

	r := NewRelativeResolver([]string{".ts", ".go"})

	tests := []struct {
		importPath string
		fromFile   string
		want       []string
	}{
		{"./foo", filepath.Join(dir, "main.ts"), []string{filepath.Join(dir, "foo.ts")}},
		{"./bar", filepath.Join(dir, "main.ts"), []string{filepath.Join(dir, "bar", "index.ts")}},
		{"./baz", filepath.Join(dir, "main.go"), []string{filepath.Join(dir, "baz.go")}},
		{"../foo", filepath.Join(dir, "sub", "main.ts"), []string{filepath.Join(dir, "foo.ts")}},
		{"react", filepath.Join(dir, "main.ts"), nil},
	}

	for _, tt := range tests {
		got, err := r.Resolve(tt.importPath, tt.fromFile, dir)
		if err != nil {
			t.Errorf("Resolve(%q, %q): %v", tt.importPath, tt.fromFile, err)
			continue
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("Resolve(%q): got %v, want %v", tt.importPath, got, tt.want)
		}
	}
}

func TestGoModResolver(t *testing.T) {
	dir := t.TempDir()

	mustWriteFile(t, filepath.Join(dir, "go.mod"), []byte("module example.com/myapp\n\ngo 1.21\n"))
	mustMkdirAll(t, filepath.Join(dir, "auth"))
	mustWriteFile(t, filepath.Join(dir, "auth", "auth.go"), []byte("package auth"))
	mustWriteFile(t, filepath.Join(dir, "auth", "auth_test.go"), []byte("package auth"))

	r, err := NewGoModResolver(dir)
	if err != nil {
		t.Fatalf("NewGoModResolver: %v", err)
	}

	got, err := r.Resolve("example.com/myapp/auth", filepath.Join(dir, "main.go"), dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := []string{filepath.Join(dir, "auth", "auth.go")}
	if !slices.Equal(got, want) {
		t.Errorf("Resolve(internal): got %v, want %v", got, want)
	}

	got, err = r.Resolve("fmt", filepath.Join(dir, "main.go"), dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != nil {
		t.Errorf("Resolve(external): got %v, want nil", got)
	}
}
