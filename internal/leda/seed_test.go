package leda

import (
	"testing"
)

func TestTokenizePrompt(t *testing.T) {
	tests := []struct {
		prompt string
		want   []string
	}{
		{"Where is the auth middleware?", []string{"auth", "middleware"}},
		{"How does the config loading work?", []string{"config", "loading", "work"}},
		{"", nil},
		{"the is in where", nil},
	}

	for _, tt := range tests {
		got := tokenizePrompt(tt.prompt)
		if len(got) != len(tt.want) {
			t.Errorf("tokenizePrompt(%q): got %v, want %v", tt.prompt, got, tt.want)
			continue
		}
		for i := range tt.want {
			if got[i] != tt.want[i] {
				t.Errorf("tokenizePrompt(%q)[%d]: got %s, want %s", tt.prompt, i, got[i], tt.want[i])
			}
		}
	}
}

func TestSplitIdentifier(t *testing.T) {
	tests := []struct {
		name string
		want []string
	}{
		{"AuthMiddleware", []string{"auth", "middleware"}},
		{"auth_middleware", []string{"auth", "middleware"}},
		{"auth-middleware", []string{"auth", "middleware"}},
		{"auth", []string{"auth"}},
		{"HTTPServer", []string{"h", "t", "t", "p", "server"}}, // uppercase sequences split per char
	}

	for _, tt := range tests {
		got := splitIdentifier(tt.name)
		if len(got) != len(tt.want) {
			t.Errorf("splitIdentifier(%q): got %v, want %v", tt.name, got, tt.want)
			continue
		}
		for i := range tt.want {
			if got[i] != tt.want[i] {
				t.Errorf("splitIdentifier(%q)[%d]: got %s, want %s", tt.name, i, got[i], tt.want[i])
			}
		}
	}
}

func TestSeedByFilename(t *testing.T) {
	g := newGraph("/test")
	g.AddNode(NodeInfo{Path: "/test/auth.go", RelPath: "auth.go", Extension: ".go"})
	g.AddNode(NodeInfo{Path: "/test/server.go", RelPath: "server.go", Extension: ".go"})
	g.AddNode(NodeInfo{Path: "/test/config.go", RelPath: "config.go", Extension: ".go"})
	g.AddNode(NodeInfo{Path: "/test/auth/middleware.go", RelPath: "auth/middleware.go", Extension: ".go"})

	seeds := seedByFilename("Where is the auth middleware?", g)
	if len(seeds) == 0 {
		t.Fatal("seedByFilename: got no seeds")
	}

	// auth/middleware.go should be first (matches both terms).
	if seeds[0].path != "/test/auth/middleware.go" {
		t.Errorf("seedByFilename: top seed got %s, want /test/auth/middleware.go", seeds[0].path)
	}

	// auth.go should also be in results.
	found := false
	for _, s := range seeds {
		if s.path == "/test/auth.go" {
			found = true
			break
		}
	}
	if !found {
		t.Error("seedByFilename: auth.go not in seeds")
	}
}

func TestSeedBySymbol(t *testing.T) {
	g := newGraph("/test")
	g.AddNode(NodeInfo{
		Path: "/test/auth.go", RelPath: "auth.go",
		Symbols: []string{"Authenticate", "ValidateToken"},
	})
	g.AddNode(NodeInfo{
		Path: "/test/server.go", RelPath: "server.go",
		Symbols: []string{"StartServer", "HandleRequest"},
	})

	seeds := seedBySymbol("authenticate users", g)
	if len(seeds) != 1 || seeds[0].path != "/test/auth.go" {
		t.Errorf("seedBySymbol: got %v, want [{/test/auth.go}]", seeds)
	}
}

func TestSeedNoMatch(t *testing.T) {
	g := newGraph("/test")
	g.AddNode(NodeInfo{Path: "/test/foo.go", RelPath: "foo.go"})

	seeds := seedByFilename("completely unrelated query about databases", g)
	if len(seeds) != 0 {
		t.Errorf("seedByFilename: got %v, want empty (no match)", seeds)
	}
}

func TestSubstringMinLength(t *testing.T) {
	g := newGraph("/test")
	g.AddNode(NodeInfo{Path: "/test/io.go", RelPath: "io.go", Extension: ".go"})
	g.AddNode(NodeInfo{Path: "/test/auth.go", RelPath: "auth.go", Extension: ".go"})

	// "io" (2 chars) should NOT match "invocation" via reverse substring.
	seeds := seedByFilename("invocation flow", g)
	for _, s := range seeds {
		if s.path == "/test/io.go" {
			t.Error("seedByFilename: io.go should not match 'invocation' (short substring)")
		}
	}

	// "auth" (4 chars) SHOULD match "authentication" via reverse substring.
	seeds = seedByFilename("authentication system", g)
	found := false
	for _, s := range seeds {
		if s.path == "/test/auth.go" {
			found = true
			break
		}
	}
	if !found {
		t.Error("seedByFilename: auth.go should match 'authentication' (long substring)")
	}
}

func TestSeedByPath(t *testing.T) {
	g := newGraph("/test")
	g.AddNode(NodeInfo{Path: "/test/auth/middleware.go", RelPath: "auth/middleware.go"})
	g.AddNode(NodeInfo{Path: "/test/config/config.go", RelPath: "config/config.go"})
	g.AddNode(NodeInfo{Path: "/test/db/db.go", RelPath: "db/db.go"})

	seeds := seedByPath("auth middleware", g)
	if len(seeds) == 0 {
		t.Fatal("seedByPath: got no seeds")
	}
	if seeds[0].path != "/test/auth/middleware.go" {
		t.Errorf("seedByPath: top seed got %s, want /test/auth/middleware.go", seeds[0].path)
	}
}
