package leda

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type gitIgnore struct {
	patterns []ignorePattern
}

type ignorePattern struct {
	regex   *regexp.Regexp
	negate  bool
	dirOnly bool
}

func newGitIgnore() *gitIgnore {
	return &gitIgnore{}
}

func (gi *gitIgnore) loadFile(path, baseDir, rootDir string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		gi.addPattern(scanner.Text(), baseDir, rootDir)
	}
	return scanner.Err()
}

func (gi *gitIgnore) addPattern(line, baseDir, rootDir string) {
	line = strings.TrimRight(line, " ")
	if line == "" || strings.HasPrefix(line, "#") {
		return
	}

	negate := false
	if strings.HasPrefix(line, "!") {
		negate = true
		line = line[1:]
	}

	dirOnly := false
	if strings.HasSuffix(line, "/") {
		dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}

	// A pattern with a slash (other than trailing) is anchored to the base dir.
	anchored := strings.Contains(line, "/")

	var regexStr string
	if anchored {
		line = strings.TrimPrefix(line, "/")
		rel, _ := filepath.Rel(rootDir, baseDir)
		if rel == "." {
			regexStr = "^" + gitignorePatternToRegex(line)
		} else {
			regexStr = "^" + regexp.QuoteMeta(rel+"/") + gitignorePatternToRegex(line)
		}
	} else {
		regexStr = "(^|/)" + gitignorePatternToRegex(line)
	}
	regexStr += "(/.*)?$"

	re, err := regexp.Compile(regexStr)
	if err != nil {
		return
	}

	gi.patterns = append(gi.patterns, ignorePattern{
		regex:   re,
		negate:  negate,
		dirOnly: dirOnly,
	})
}

func (gi *gitIgnore) match(relPath string, isDir bool) bool {
	matched := false
	for _, p := range gi.patterns {
		if !p.regex.MatchString(relPath) {
			continue
		}
		if p.dirOnly && !isDir {
			// dirOnly patterns match files only if they're inside the
			// ignored directory. Strip the (/.*)?$ suffix and check
			// whether the regex still matches — if so, the path IS the
			// directory name (not a child) and should be skipped.
			parent := filepath.Dir(relPath)
			if parent == "." || !p.regex.MatchString(parent) {
				continue
			}
		}
		matched = !p.negate
	}
	return matched
}

func gitignorePatternToRegex(pattern string) string {
	var b strings.Builder
	i := 0
	for i < len(pattern) {
		ch := pattern[i]
		switch ch {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				if i+2 < len(pattern) && pattern[i+2] == '/' {
					b.WriteString("(.*/)?")
					i += 3
					continue
				}
				b.WriteString(".*")
				i += 2
				continue
			}
			b.WriteString("[^/]*")
		case '?':
			b.WriteString("[^/]")
		case '.', '+', '^', '$', '|', '(', ')', '{', '}':
			b.WriteByte('\\')
			b.WriteByte(ch)
		case '[':
			// Pass through character classes.
			end := strings.IndexByte(pattern[i:], ']')
			if end == -1 {
				b.WriteByte('\\')
				b.WriteByte(ch)
			} else {
				b.WriteString(pattern[i : i+end+1])
				i += end
			}
		case '\\':
			if i+1 < len(pattern) {
				i++
				b.WriteByte('\\')
				b.WriteByte(pattern[i])
			}
		default:
			b.WriteByte(ch)
		}
		i++
	}
	return b.String()
}

// loadGitIgnores walks rootDir and loads all .gitignore files into a matcher.
func loadGitIgnores(rootDir string) *gitIgnore {
	gi := newGitIgnore()
	_ = filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "node_modules" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Base(path) == ".gitignore" {
			_ = gi.loadFile(path, filepath.Dir(path), rootDir)
		}
		return nil
	})
	return gi
}
