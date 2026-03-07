package extract

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jgabor/leda/internal/leda"
	"github.com/jgabor/leda/internal/parser"
)

type Result struct {
	Language          string         `json:"language"`
	LanguagesDetected map[string]int `json:"languages_detected"`
	Extractor         string         `json:"extractor"`
	FileCount         int            `json:"file_count"`
	TokenEstimate     int            `json:"token_estimate"`
	Naming            NamingResult   `json:"naming"`
	Layering          LayeringResult `json:"layering"`
	Errors            ErrorsResult   `json:"errors"`
	Testing           TestingResult  `json:"testing"`
	Tooling           ToolingResult  `json:"tooling"`
}

type NamingResult struct {
	Files           map[string]int `json:"files"`
	Directories     map[string]int `json:"directories"`
	ExportedSymbols map[string]int `json:"exported_symbols"`
	Packages        map[string]int `json:"packages,omitempty"`
}

type LayeringResult struct {
	Modules int                 `json:"modules"`
	Edges   int                 `json:"edges"`
	Graph   map[string][]string `json:"graph"`
	FanIn   []FanEntry          `json:"fan_in"`
	FanOut  []FanEntry          `json:"fan_out"`
}

type FanEntry struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

type ErrorsResult struct {
	Patterns      map[string]int `json:"patterns"`
	DominantStyle string         `json:"dominant_style"`
}

type TestingResult struct {
	Framework     string   `json:"framework"`
	TestFileCount int      `json:"test_file_count"`
	TestNaming    string   `json:"test_naming"`
	Placement     string   `json:"placement"`
	Helpers       []string `json:"helpers"`
	MockLibraries []string `json:"mock_libraries"`
	ConfigFiles   []string `json:"config_files"`
}

type ToolingResult struct {
	Build              []string `json:"build"`
	Lint               []string `json:"lint"`
	Format             []string `json:"format"`
	CI                 []string `json:"ci"`
	DependencyManifest string   `json:"dependency_manifest"`
}

func Run(rootDir string, g *leda.Graph, reg *parser.Registry) (*Result, error) {
	infos := g.NodeInfos()
	stats := g.Stats()

	langCounts := make(map[string]int)
	totalTokens := 0
	for _, info := range infos {
		lang := extToLanguage(info.Extension, reg)
		if lang != "" {
			langCounts[lang]++
		}
		totalTokens += info.TokenEstimate
	}

	primary := ""
	maxCount := 0
	for lang, count := range langCounts {
		if count > maxCount {
			maxCount = count
			primary = lang
		}
	}

	return &Result{
		Language:          primary,
		LanguagesDetected: langCounts,
		Extractor:         "leda",
		FileCount:         stats.NodeCount,
		TokenEstimate:     totalTokens,
		Naming:            analyzeNaming(infos, reg),
		Layering:          analyzeLayering(g),
		Errors:            analyzeErrors(infos, reg),
		Testing:           analyzeTesting(rootDir, infos, reg),
		Tooling:           analyzeTooling(rootDir),
	}, nil
}

func extToLanguage(ext string, reg *parser.Registry) string {
	p := reg.ForExtension(ext)
	if p == nil {
		return ""
	}
	return p.Language()
}

// Naming classification

var (
	reUpperSnake = regexp.MustCompile(`^[A-Z][A-Z0-9]*(_[A-Z0-9]+)+$`)
	reKebab      = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)+$`)
	reSnake      = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)+$`)
	rePascal     = regexp.MustCompile(`^[A-Z][a-zA-Z0-9]*$`)
	reCamel      = regexp.MustCompile(`^[a-z][a-zA-Z0-9]*$`)
	reShortLower = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
	reHasLower   = regexp.MustCompile(`[a-z]`)
	reHasUpper   = regexp.MustCompile(`[A-Z]`)
)

func classifyNaming(name string) string {
	if name == "" {
		return "other"
	}
	switch {
	case reUpperSnake.MatchString(name):
		return "UPPER_SNAKE"
	case reKebab.MatchString(name):
		return "kebab-case"
	case reSnake.MatchString(name):
		return "snake_case"
	case rePascal.MatchString(name) && reHasLower.MatchString(name):
		return "PascalCase"
	case reCamel.MatchString(name) && reHasUpper.MatchString(name):
		return "camelCase"
	case reShortLower.MatchString(name):
		return "short_lowercase"
	default:
		return "other"
	}
}

func analyzeNaming(infos []leda.NodeInfo, reg *parser.Registry) NamingResult {
	files := make(map[string]int)
	dirs := make(map[string]int)
	symbols := make(map[string]int)
	seenDirs := make(map[string]bool)

	for _, info := range infos {
		stem := strings.TrimSuffix(filepath.Base(info.RelPath), filepath.Ext(info.RelPath))
		files[classifyNaming(stem)]++

		dir := filepath.Dir(info.RelPath)
		for dir != "." && dir != "" {
			if !seenDirs[dir] {
				seenDirs[dir] = true
				dirs[classifyNaming(filepath.Base(dir))]++
			}
			dir = filepath.Dir(dir)
		}

		for _, sym := range info.Symbols {
			symbols[classifyNaming(sym)]++
		}
	}

	result := NamingResult{
		Files:           files,
		Directories:     dirs,
		ExportedSymbols: symbols,
	}

	pkgs := extractGoPackages(infos, reg)
	if len(pkgs) > 0 {
		result.Packages = pkgs
	}

	return result
}

var rePkgDecl = regexp.MustCompile(`^package\s+(\w+)`)

func extractGoPackages(infos []leda.NodeInfo, reg *parser.Registry) map[string]int {
	counts := make(map[string]int)
	seen := make(map[string]bool)

	for _, info := range infos {
		if extToLanguage(info.Extension, reg) != "go" {
			continue
		}
		func() {
			f, err := os.Open(info.Path)
			if err != nil {
				return
			}
			defer func() { _ = f.Close() }()
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := scanner.Text()
				if m := rePkgDecl.FindStringSubmatch(line); m != nil {
					pkg := m[1]
					if !seen[pkg] {
						seen[pkg] = true
						counts[classifyNaming(pkg)]++
					}
					return
				}
				if strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "//") {
					return
				}
			}
		}()
	}

	if len(counts) == 0 {
		return nil
	}
	return counts
}

// Layering analysis

func analyzeLayering(g *leda.Graph) LayeringResult {
	infos := g.NodeInfos()
	absToRel := make(map[string]string, len(infos))
	modulesSet := make(map[string]bool)

	for _, info := range infos {
		absToRel[info.Path] = info.RelPath
		modulesSet[filepath.Dir(info.RelPath)] = true
	}

	edges := g.Edges()

	graphMap := make(map[string][]string)
	fanInCount := make(map[string]int)
	fanOutCount := make(map[string]int)

	for _, e := range edges {
		fromRel := absToRel[e.From]
		toRel := absToRel[e.To]
		if fromRel == "" || toRel == "" {
			continue
		}
		toDir := filepath.Dir(toRel)
		graphMap[fromRel] = append(graphMap[fromRel], toDir)
		fanInCount[toDir]++
		fanOutCount[fromRel]++
	}

	for file, targets := range graphMap {
		graphMap[file] = dedup(targets)
	}

	return LayeringResult{
		Modules: len(modulesSet),
		Edges:   len(edges),
		Graph:   graphMap,
		FanIn:   topFanEntries(fanInCount, 10),
		FanOut:  topFanEntries(fanOutCount, 10),
	}
}

func dedup(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func topFanEntries(counts map[string]int, n int) []FanEntry {
	entries := make([]FanEntry, 0, len(counts))
	for path, count := range counts {
		entries = append(entries, FanEntry{Path: path, Count: count})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Count != entries[j].Count {
			return entries[i].Count > entries[j].Count
		}
		return entries[i].Path < entries[j].Path
	})
	if len(entries) > n {
		entries = entries[:n]
	}
	return entries
}

// Error pattern analysis

type errorPattern struct {
	key       string
	languages []string
	re        *regexp.Regexp
}

var errorPatterns = []errorPattern{
	{"fmt_errorf_w", []string{"go"}, regexp.MustCompile(`fmt\.Errorf\([^)]*%w`)},
	{"errors_new", []string{"go"}, regexp.MustCompile(`errors\.New\(`)},
	{"errors_is", []string{"go"}, regexp.MustCompile(`errors\.Is\(`)},
	{"errors_as", []string{"go"}, regexp.MustCompile(`errors\.As\(`)},
	{"throw_new_error", []string{"javascript", "typescript"}, regexp.MustCompile(`throw\s+new\s+\w*Error\(`)},
	{"try_catch", []string{"javascript", "typescript", "java"}, regexp.MustCompile(`try\s*\{`)},
	{"raise_exception", []string{"python"}, regexp.MustCompile(`raise\s+\w*Error\(`)},
	{"result_type", []string{"rust"}, regexp.MustCompile(`Result<`)},
	{"custom_error_class", []string{"javascript", "typescript", "python", "java"}, regexp.MustCompile(`class\s+\w+\s+extends\s+(Error|Exception)`)},
}

func analyzeErrors(infos []leda.NodeInfo, reg *parser.Registry) ErrorsResult {
	counts := make(map[string]int)
	for _, ep := range errorPatterns {
		counts[ep.key] = 0
	}

	byLang := make(map[string][]errorPattern)
	for _, ep := range errorPatterns {
		for _, l := range ep.languages {
			byLang[l] = append(byLang[l], ep)
		}
	}

	for _, info := range infos {
		lang := extToLanguage(info.Extension, reg)
		patterns := byLang[lang]
		if len(patterns) == 0 {
			continue
		}

		data, err := os.ReadFile(info.Path)
		if err != nil {
			continue
		}

		for _, ep := range patterns {
			counts[ep.key] += len(ep.re.FindAll(data, -1))
		}
	}

	dominant := ""
	maxCount := 0
	for key, count := range counts {
		if count > maxCount {
			maxCount = count
			dominant = key
		}
	}

	return ErrorsResult{
		Patterns:      counts,
		DominantStyle: dominant,
	}
}

// Testing analysis

var testFilePatterns = []struct {
	suffix string
	prefix string
}{
	{"_test.go", ""},
	{".test.ts", ""},
	{".test.tsx", ""},
	{".test.js", ""},
	{".test.jsx", ""},
	{".spec.ts", ""},
	{".spec.tsx", ""},
	{".spec.js", ""},
	{".spec.jsx", ""},
	{"_test.py", ""},
	{"_test.rs", ""},
	{"", "test_"},
}

func isTestFile(name string) bool {
	base := filepath.Base(name)
	for _, p := range testFilePatterns {
		if p.suffix != "" && strings.HasSuffix(base, p.suffix) {
			return true
		}
		if p.prefix != "" && strings.HasPrefix(base, p.prefix) {
			return true
		}
	}
	return false
}

var reGoTestFunc = regexp.MustCompile(`^func\s+(Test\w+)`)

var frameworkImports = []struct {
	pattern   string
	framework string
}{
	{"github.com/stretchr/testify", "testify"},
	{"testing", "stdlib"},
	{"jest", "jest"},
	{"@jest", "jest"},
	{"vitest", "vitest"},
	{"pytest", "pytest"},
	{"unittest", "unittest"},
}

var mockImports = []struct {
	pattern string
	name    string
}{
	{"go.uber.org/mock", "gomock"},
	{"github.com/golang/mock", "gomock"},
	{"testify/mock", "testify/mock"},
	{"jest.mock", "jest.mock"},
	{"unittest.mock", "unittest.mock"},
}

func analyzeTesting(rootDir string, infos []leda.NodeInfo, reg *parser.Registry) TestingResult {
	result := TestingResult{
		Helpers:       []string{},
		MockLibraries: []string{},
		ConfigFiles:   []string{},
	}

	var testFiles []leda.NodeInfo
	testDirs := make(map[string]bool)
	allDirs := make(map[string]bool)

	for _, info := range infos {
		dir := filepath.Dir(info.RelPath)
		allDirs[dir] = true
		if isTestFile(info.RelPath) {
			testFiles = append(testFiles, info)
			testDirs[dir] = true
		}
	}

	result.TestFileCount = len(testFiles)

	// Framework detection
	frameworkCounts := make(map[string]int)
	mockSet := make(map[string]bool)
	var testFuncNames []string

	for _, tf := range testFiles {
		p := reg.ForExtension(tf.Extension)
		if p != nil {
			imports, err := p.ParseImports(tf.Path)
			if err == nil {
				for _, imp := range imports {
					for _, fi := range frameworkImports {
						if strings.Contains(imp, fi.pattern) {
							frameworkCounts[fi.framework]++
						}
					}
					for _, mi := range mockImports {
						if strings.Contains(imp, mi.pattern) {
							mockSet[mi.name] = true
						}
					}
				}
			}
		}

		if strings.HasSuffix(tf.Path, "_test.go") {
			data, err := os.ReadFile(tf.Path)
			if err == nil {
				for _, line := range strings.Split(string(data), "\n") {
					if m := reGoTestFunc.FindStringSubmatch(line); m != nil {
						testFuncNames = append(testFuncNames, m[1])
					}
				}
			}
		}
	}

	bestFramework := ""
	bestCount := 0
	for fw, count := range frameworkCounts {
		if count > bestCount {
			bestCount = count
			bestFramework = fw
		}
	}
	result.Framework = bestFramework

	for name := range mockSet {
		result.MockLibraries = append(result.MockLibraries, name)
	}
	sort.Strings(result.MockLibraries)

	// Test naming pattern
	result.TestNaming = inferTestNaming(testFuncNames)

	// Placement
	result.Placement = inferPlacement(testDirs, allDirs)

	// Helpers — search directories already in the graph (gitignore-filtered)
	helperNames := map[string]bool{
		"testutil": true, "testdata": true, "fixtures": true,
		"testhelper": true, "testhelpers": true,
	}
	helperSet := make(map[string]bool)
	for dir := range allDirs {
		for d := dir; d != "." && d != ""; d = filepath.Dir(d) {
			base := filepath.Base(d)
			if helperNames[base] && !helperSet[d] {
				helperSet[d] = true
				result.Helpers = append(result.Helpers, d+"/")
			}
		}
	}
	sort.Strings(result.Helpers)

	// Config files
	testConfigs := []string{
		"jest.config.js", "jest.config.ts", "jest.config.json",
		"vitest.config.ts", "vitest.config.js",
		"pytest.ini",
		".mocharc.yml", ".mocharc.json",
	}
	for _, cfg := range testConfigs {
		if _, err := os.Stat(filepath.Join(rootDir, cfg)); err == nil {
			result.ConfigFiles = append(result.ConfigFiles, cfg)
		}
	}

	return result
}

func inferTestNaming(names []string) string {
	if len(names) == 0 {
		return ""
	}

	underscoreCount := 0
	for _, name := range names {
		if strings.Contains(name, "_") {
			underscoreCount++
		}
	}

	if underscoreCount > len(names)/2 {
		return "Test<Name>_<scenario>"
	}
	if strings.HasPrefix(names[0], "Test") {
		return "Test<Name>"
	}
	return ""
}

func inferPlacement(testDirs, allDirs map[string]bool) string {
	for dir := range testDirs {
		base := filepath.Base(dir)
		switch base {
		case "__tests__":
			return "__tests__"
		case "tests":
			return "tests/"
		case "spec":
			return "spec/"
		}
	}

	samePackage := 0
	for dir := range testDirs {
		if allDirs[dir] {
			samePackage++
		}
	}
	if samePackage > 0 {
		return "same_package"
	}

	return ""
}

// Tooling analysis

type toolCheck struct {
	category string
	name     string
	files    []string
	globs    []string
}

var toolChecks = []toolCheck{
	{"build", "go build", []string{"go.mod"}, nil},
	{"build", "Makefile", []string{"Makefile"}, nil},
	{"build", "npm", []string{"package.json"}, nil},
	{"build", "cargo", []string{"Cargo.toml"}, nil},
	{"lint", "golangci-lint", []string{".golangci.yml", ".golangci.yaml", ".golangci.toml"}, nil},
	{"lint", "eslint", []string{".eslintrc", ".eslintrc.js", ".eslintrc.json", ".eslintrc.yml", "eslint.config.js"}, nil},
	{"lint", "clippy", []string{"clippy.toml", ".clippy.toml"}, nil},
	{"format", "gofmt", []string{"go.mod"}, nil},
	{"format", "prettier", []string{".prettierrc", ".prettierrc.js", ".prettierrc.json", "prettier.config.js"}, nil},
	{"format", "rustfmt", []string{"rustfmt.toml", ".rustfmt.toml"}, nil},
	{"ci", "github-actions", nil, []string{".github/workflows/*.yml", ".github/workflows/*.yaml"}},
	{"ci", "gitlab-ci", []string{".gitlab-ci.yml"}, nil},
	{"ci", "circleci", []string{".circleci/config.yml"}, nil},
}

var manifestFiles = []string{
	"go.mod", "package.json", "Cargo.toml", "pyproject.toml", "requirements.txt",
	"Gemfile", "composer.json",
}

func analyzeTooling(rootDir string) ToolingResult {
	result := ToolingResult{
		Build:  []string{},
		Lint:   []string{},
		Format: []string{},
		CI:     []string{},
	}

	categories := map[string]*[]string{
		"build":  &result.Build,
		"lint":   &result.Lint,
		"format": &result.Format,
		"ci":     &result.CI,
	}

	for _, tc := range toolChecks {
		found := false
		for _, f := range tc.files {
			if _, err := os.Stat(filepath.Join(rootDir, f)); err == nil {
				found = true
				break
			}
		}
		if !found {
			for _, g := range tc.globs {
				matches, _ := filepath.Glob(filepath.Join(rootDir, g))
				if len(matches) > 0 {
					found = true
					break
				}
			}
		}
		if found {
			list := categories[tc.category]
			*list = append(*list, tc.name)
		}
	}

	for _, mf := range manifestFiles {
		if _, err := os.Stat(filepath.Join(rootDir, mf)); err == nil {
			result.DependencyManifest = mf
			break
		}
	}

	return result
}
