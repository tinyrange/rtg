package frontend

import (
	"fmt"
	"os"
	"strings"

	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/stdlib"
)

// SymKind represents the kind of a symbol.
type SymKind int

const (
	SymFunc SymKind = iota
	SymType
	SymVar
	SymConst
)

// Symbol represents a named entity in a package.
type Symbol struct {
	Name       string
	Kind       SymKind
	Node       *Node
	Pkg        *Package
	Intern     string
	Embed      string
	LinkStatic bool
}

// Package represents a parsed Go package.
type Package struct {
	Name          string
	Path          string
	Dir           string
	Files         []*Node
	Imports       []string
	ImportAliases map[string]string
	Symbols       map[string]*Symbol
	Inits         []*Node
	qualNames     map[string]string // name → "Path.name"
	qualPtrNames  map[string]string // name → "Path.*name"
}

func (pkg *Package) QualName(name string) string {
	if q, ok := pkg.qualNames[name]; ok {
		return q
	}
	q := pkg.Path + "." + name
	if pkg.qualNames == nil {
		pkg.qualNames = make(map[string]string)
	}
	pkg.qualNames[name] = q
	return q
}

func (pkg *Package) QualPtrName(name string) string {
	if q, ok := pkg.qualPtrNames[name]; ok {
		return q
	}
	q := pkg.Path + ".*" + name
	if pkg.qualPtrNames == nil {
		pkg.qualPtrNames = make(map[string]string)
	}
	pkg.qualPtrNames[name] = q
	return q
}

// Module represents the complete module with all resolved packages.
type Module struct {
	BaseDir  string
	Packages map[string]*Package
	Order    []string
	Entry    *Package
}

const modulePathPrefix = "j5.nz/rtg/"

var discoveredBuildTags []string

func importPathCandidates(importPath string) []string {
	candidates := []string{importPath}
	if strings.HasPrefix(importPath, modulePathPrefix) {
		stripped := importPath[len(modulePathPrefix):]
		candidates = common.AppendUnique(candidates, stripped)
		if strings.HasPrefix(stripped, "std/") {
			candidates = common.AppendUnique(candidates, stripped[len("std/"):])
		}
	}
	return candidates
}

func ResetDiscoveredBuildTags() {
	discoveredBuildTags = nil
}

func addDiscoveredBuildTag(tag string) {
	if tag == "" || tag == "true" || tag == "false" {
		return
	}
	for _, existing := range discoveredBuildTags {
		if existing == tag {
			return
		}
	}
	discoveredBuildTags = append(discoveredBuildTags, tag)
}

func GetDiscoveredBuildTags() []string {
	var tags []string
	for _, tag := range discoveredBuildTags {
		if isKnownOS(tag) || isKnownArch(tag) {
			continue
		}
		tags = append(tags, tag)
	}
	sortStrings(tags)
	return tags
}

// ResolveModule parses entry files and recursively resolves all imports.
func ResolveModule(target *common.Target, baseDir string, entryFiles []string) *Module {
	p := &Preprocessor{
		target: target,
	}

	mod := &Module{
		BaseDir:  baseDir,
		Packages: make(map[string]*Package),
	}

	// Parse entry package.
	// If specific .go files are given, parse only those files (like go run file.go).
	// If a directory or bare package name is given, parse all .go files in it.
	entryDir := dirOfPath(entryFiles[0])
	var mainPkg *Package
	arg := entryFiles[0]
	if isGoFile(arg) {
		// Specific .go files: parse only the named files
		mainPkg = &Package{
			Name:    "main",
			Path:    "main",
			Dir:     entryDir,
			Symbols: make(map[string]*Symbol),
		}
		for _, f := range entryFiles {
			node := parseFile(f)
			if node != nil {
				mainPkg.Files = append(mainPkg.Files, node)
			}
		}
		mainPkg.Imports = collectImports(mainPkg)
	} else if arg != "." {
		// Bare package name: try embedded std first, then directory scan
		mainPkg = p.parsePackageFromStdlibSources(baseDir, arg)
		if mainPkg == nil {
			mainPkg = p.parsePackageDir(entryDir, "main")
		}
	} else {
		// "." or directory: scan the directory for all .go files
		mainPkg = p.parsePackageDir(entryDir, "main")
	}
	if mainPkg == nil {
		fmt.Fprintf(os.Stderr, "error: no Go files found in %s\n", entryDir)
		os.Exit(1)
	}
	if target.TestMode {
		injectSyntheticTestRunner(mainPkg)
	}
	mainPkg.Path = "main"
	mod.Packages["main"] = mainPkg
	mod.Entry = mainPkg

	// Worklist loop: resolve imports recursively
	var worklist []string
	for _, imp := range mainPkg.Imports {
		worklist = append(worklist, imp)
	}
	// Runtime is required by compiler-emitted helpers (alloc/map/string/etc),
	// even for programs that do not explicitly import it.
	if mainPkg.Path != "runtime" {
		hasRuntime := false
		for _, imp := range mainPkg.Imports {
			if imp == "runtime" {
				hasRuntime = true
				break
			}
		}
		if !hasRuntime {
			mainPkg.Imports = append(mainPkg.Imports, "runtime")
			worklist = append(worklist, "runtime")
		}
	}

	for len(worklist) > 0 {
		importPath := worklist[0]
		worklist = worklist[1:len(worklist)]

		_, already := mod.Packages[importPath]
		if already {
			continue
		}

		pkg := p.parsePackageFromStdlibSources(baseDir, importPath)
		if pkg == nil {
			dirs := p.resolveImportDirs(baseDir, importPath)
			if len(dirs) == 0 {
				fmt.Fprintf(os.Stderr, "error: cannot resolve import %s\n", importPath)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "error: no Go files for import %s\n", importPath)
			os.Exit(1)
		}
		mod.Packages[importPath] = pkg

		for _, imp := range pkg.Imports {
			_, seen := mod.Packages[imp]
			if !seen {
				worklist = append(worklist, imp)
			}
		}
	}

	// Topological sort
	mod.Order = topologicalSort(mod.Packages)

	// Collect symbols for each package
	for _, path := range mod.Order {
		pkg, ok := mod.Packages[path]
		if ok {
			collectSymbols(pkg)
		}
	}

	return mod
}

func ShouldUseEmbeddedStdlib(target *common.Target) bool {
	if !stdlib.HasEmbeddedStd() {
		return false
	}
	if !target.StdlibIncludeExplicit {
		return true
	}
	return target.StdlibIncludeEmbedded
}

func appendStdlibDirCandidates(candidates []string, root string, importPath string) []string {
	if root == "" {
		return candidates
	}
	root = common.TrimTrailingSlash(common.NormalizePath(root))
	if root == "" {
		return candidates
	}
	candidates = common.AppendUnique(candidates, root+"/"+importPath)
	candidates = common.AppendUnique(candidates, root+"/std/"+importPath)
	return candidates
}

// resolveImportDirs maps an import path to possible directories on disk.
func (c *Preprocessor) resolveImportDirs(baseDir string, importPath string) []string {
	var dirs []string
	importCandidates := importPathCandidates(importPath)
	if c.target.StdlibIncludeExplicit {
		for _, include := range c.target.StdlibIncludePaths {
			for _, candidate := range importCandidates {
				dirs = appendStdlibDirCandidates(dirs, include, candidate)
			}
		}
		return dirs
	}
	for _, candidate := range importCandidates {
		dirs = appendStdlibDirCandidates(dirs, baseDir, candidate)
	}
	return dirs
}

func (c *Preprocessor) parsePackageFromStdlibSources(baseDir string, importPath string) *Package {
	importCandidates := importPathCandidates(importPath)
	if ShouldUseEmbeddedStdlib(c.target) {
		for _, candidate := range importCandidates {
			pkg := c.parsePackageFromEmbed(candidate)
			if pkg != nil {
				// Preserve canonical import path for symbol qualification while
				// keeping embed-relative package directory for //go:embed patterns.
				pkg.Path = importPath
				return pkg
			}
		}
	}
	if c.target.StdlibIncludeExplicit {
		for _, include := range c.target.StdlibIncludePaths {
			for _, candidate := range importCandidates {
				dirs := appendStdlibDirCandidates(nil, include, candidate)
				for _, dir := range dirs {
					pkg := c.parsePackageDir(dir, importPath)
					if pkg != nil {
						return pkg
					}
				}
			}
		}
		return nil
	}
	for _, candidate := range importCandidates {
		dirs := appendStdlibDirCandidates(nil, baseDir, candidate)
		for _, dir := range dirs {
			pkg := c.parsePackageDir(dir, importPath)
			if pkg != nil {
				return pkg
			}
		}
	}
	return nil
}

// stringLess compares two strings lexicographically (byte-by-byte).
// This is needed because the RTG compiler's < operator does integer comparison,
// not string content comparison.
func stringLess(a string, b string) bool {
	la := len(a)
	lb := len(b)
	n := la
	if lb < n {
		n = lb
	}
	i := 0
	for i < n {
		if a[i] < b[i] {
			return true
		}
		if a[i] > b[i] {
			return false
		}
		i = i + 1
	}
	return la < lb
}

// sortStrings sorts a string slice in-place using insertion sort.
func sortStrings(s []string) {
	i := 1
	for i < len(s) {
		j := i
		for j > 0 && stringLess(s[j], s[j-1]) {
			tmp := s[j]
			s[j] = s[j-1]
			s[j-1] = tmp
			j = j - 1
		}
		i = i + 1
	}
}

// shouldIncludeFile checks if a .go file should be included based on build tags.
// If a //go:build directive exists, it takes precedence over filename-based filtering.
// Otherwise, filename-based GOOS/GOARCH conventions are used.
func (c *Preprocessor) shouldIncludeFile(path string, name string) bool {
	// 1. Check //go:build directive in file content (takes precedence)
	src, err := os.ReadFile(path)
	if err != nil {
		return true // if can't read, include by default
	}
	content := string(src)
	collectBuildTagsFromContent(content)
	collectBuildTagsFromFilename(name)

	// Scan first few lines for //go:build
	pos := 0
	for pos < len(content) {
		// Find end of line
		eol := pos
		for eol < len(content) && content[eol] != '\n' {
			eol++
		}
		line := content[pos:eol]

		// Skip blank lines and comments at top of file
		trimmed := trimLeftSpace(line)
		if len(trimmed) == 0 {
			pos = eol + 1
			continue
		}

		// Check for //go:build
		if len(trimmed) >= 11 && trimmed[0:11] == "//go:build " {
			expr := trimmed[11:len(trimmed)]
			return c.evalBuildExpr(expr)
		}

		// Check for regular comments (skip them)
		if len(trimmed) >= 2 && trimmed[0:2] == "//" {
			pos = eol + 1
			continue
		}

		// First non-comment, non-blank line — stop looking
		break
	}

	// 2. Filename-based tag filtering (only if no //go:build directive)
	// Strip .go suffix
	base := name[0 : len(name)-3]

	// Check for _GOOS_GOARCH.go, _GOOS.go, _GOARCH.go patterns
	// Find last underscore segment(s)
	parts := splitString(base, '_')
	if len(parts) >= 3 {
		// Could be name_GOOS_GOARCH.go
		maybearch := parts[len(parts)-1]
		maybeos := parts[len(parts)-2]
		if isKnownOS(maybeos) && isKnownArch(maybearch) {
			if !c.hasTag(maybeos) || !c.hasTag(maybearch) {
				return false
			}
		} else if isKnownOS(maybearch) || isKnownArch(maybearch) {
			if !c.hasTag(maybearch) {
				return false
			}
		}
	} else if len(parts) >= 2 {
		last := parts[len(parts)-1]
		if isKnownOS(last) || isKnownArch(last) {
			if !c.hasTag(last) {
				return false
			}
		}
	}

	return true
}

// splitString splits a string by a separator byte.
func splitString(s string, sep byte) []string {
	var result []string
	start := 0
	i := 0
	for i < len(s) {
		if s[i] == sep {
			result = append(result, s[start:i])
			start = i + 1
		}
		i++
	}
	result = append(result, s[start:len(s)])
	return result
}

func collectBuildTagsFromContent(content string) {
	// Scan first few lines for //go:build expression.
	pos := 0
	for pos < len(content) {
		eol := pos
		for eol < len(content) && content[eol] != '\n' {
			eol++
		}
		line := content[pos:eol]
		trimmed := trimLeftSpace(line)
		if len(trimmed) == 0 {
			pos = eol + 1
			continue
		}
		if len(trimmed) >= 11 && trimmed[0:11] == "//go:build " {
			collectBuildTagsFromExpr(trimmed[11:len(trimmed)])
			return
		}
		if len(trimmed) >= 2 && trimmed[0:2] == "//" {
			pos = eol + 1
			continue
		}
		return
	}
}

func collectBuildTagsFromExpr(expr string) {
	i := 0
	for i < len(expr) {
		if isAlphaNum(expr[i]) || expr[i] == '_' {
			start := i
			for i < len(expr) && (isAlphaNum(expr[i]) || expr[i] == '_') {
				i++
			}
			addDiscoveredBuildTag(expr[start:i])
			continue
		}
		i++
	}
}

func collectBuildTagsFromFilename(name string) {
	if !isGoFile(name) {
		return
	}
	base := name[0 : len(name)-3]
	parts := splitString(base, '_')
	if len(parts) >= 3 {
		maybearch := parts[len(parts)-1]
		maybeos := parts[len(parts)-2]
		if isKnownOS(maybeos) && isKnownArch(maybearch) {
			addDiscoveredBuildTag(maybeos)
			addDiscoveredBuildTag(maybearch)
			return
		}
	}
	if len(parts) >= 2 {
		last := parts[len(parts)-1]
		if isKnownOS(last) || isKnownArch(last) {
			addDiscoveredBuildTag(last)
		}
	}
}

// trimLeftSpace trims leading spaces and tabs.
func trimLeftSpace(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return s[i:len(s)]
}

// isKnownOS returns true if s is a known GOOS value.
func isKnownOS(s string) bool {
	return s == "linux" || s == "darwin" || s == "windows" || s == "freebsd" || s == "wasi" || s == "dos" || s == "c"
}

// isKnownArch returns true if s is a known GOARCH value.
func isKnownArch(s string) bool {
	return s == "amd64" || s == "386" || s == "arm64" || s == "arm" || s == "wasm32" || s == "dos16" || s == "c8" || s == "c16" || s == "c32" || s == "c64"
}

type Preprocessor struct {
	target *common.Target
}

// hasTag checks if a tag is in the active build tag set.
func (c *Preprocessor) hasTag(tag string) bool {
	i := 0
	for i < len(c.target.BuildTags) {
		if c.target.BuildTags[i] == tag {
			return true
		}
		i++
	}
	return false
}

// evalBuildExpr evaluates a //go:build expression against the active tag set.
// Supports: bare tags, &&, ||, !, and parentheses.
func (c *Preprocessor) evalBuildExpr(expr string) bool {
	expr = trimLeftSpace(expr)
	result, _ := c.parseBuildOr(expr)
	return result
}

// parseBuildOr parses: term (|| term)*
func (c *Preprocessor) parseBuildOr(expr string) (bool, string) {
	left, rest := c.parseBuildAnd(expr)
	for {
		rest = trimLeftSpace(rest)
		if len(rest) >= 2 && rest[0] == '|' && rest[1] == '|' {
			var right bool
			right, rest = c.parseBuildAnd(rest[2:len(rest)])
			left = left || right
		} else {
			break
		}
	}
	return left, rest
}

// parseBuildAnd parses: unary (&& unary)*
func (c *Preprocessor) parseBuildAnd(expr string) (bool, string) {
	left, rest := c.parseBuildUnary(expr)
	for {
		rest = trimLeftSpace(rest)
		if len(rest) >= 2 && rest[0] == '&' && rest[1] == '&' {
			var right bool
			right, rest = c.parseBuildUnary(rest[2:len(rest)])
			left = left && right
		} else {
			break
		}
	}
	return left, rest
}

// parseBuildUnary parses: !unary | atom
func (c *Preprocessor) parseBuildUnary(expr string) (bool, string) {
	expr = trimLeftSpace(expr)
	if len(expr) > 0 && expr[0] == '!' {
		val, rest := c.parseBuildUnary(expr[1:len(expr)])
		return !val, rest
	}
	return c.parseBuildAtom(expr)
}

// parseBuildAtom parses: (expr) | tag
func (c *Preprocessor) parseBuildAtom(expr string) (bool, string) {
	expr = trimLeftSpace(expr)
	if len(expr) > 0 && expr[0] == '(' {
		val, rest := c.parseBuildOr(expr[1:len(expr)])
		rest = trimLeftSpace(rest)
		if len(rest) > 0 && rest[0] == ')' {
			rest = rest[1:len(rest)]
		}
		return val, rest
	}
	// Parse a bare tag identifier (alphanumeric + _)
	i := 0
	for i < len(expr) && (isAlphaNum(expr[i]) || expr[i] == '_') {
		i++
	}
	if i == 0 {
		return false, expr
	}
	tag := expr[0:i]
	return c.hasTag(tag), expr[i:len(expr)]
}

// isAlphaNum returns true if c is a letter or digit.
func isAlphaNum(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// parsePackageDir lists .go files in a directory, parses each, and merges into one Package.
func (c *Preprocessor) parsePackageDir(dir string, importPath string) *Package {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	// Collect .go file names and sort for deterministic order.
	// Go's os.ReadDir sorts by name, but RTG's ReadDir returns filesystem order.
	var goFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !isGoFile(entry.Name()) {
			continue
		}
		if !c.target.TestMode && isGoTestFile(entry.Name()) {
			continue
		}
		// Check build tags before including
		if !c.shouldIncludeFile(dir+"/"+entry.Name(), entry.Name()) {
			continue
		}
		goFiles = append(goFiles, entry.Name())
	}
	sortStrings(goFiles)

	pkg := &Package{
		Path:    importPath,
		Dir:     dir,
		Symbols: make(map[string]*Symbol),
	}

	for _, name := range goFiles {
		path := dir + "/" + name
		node := parseFile(path)
		if node != nil {
			if pkg.Name == "" {
				pkg.Name = node.Name
			}
			pkg.Files = append(pkg.Files, node)
		}
	}

	if len(pkg.Files) == 0 {
		return nil
	}

	pkg.Imports = collectImports(pkg)
	return pkg
}

// dirOfPath returns the directory portion of a file path.
func dirOfPath(path string) string {
	hasSep := false
	j := 0
	for j < len(path) {
		if path[j] == '/' || path[j] == '\\' {
			hasSep = true
			break
		}
		j = j + 1
	}
	if !isGoFile(path) && hasSep {
		trimmed := strings.TrimRight(path, "/\\")
		if trimmed == "" {
			return path
		}
		return trimmed
	}
	i := len(path) - 1
	for i >= 0 {
		if path[i] == '/' || path[i] == '\\' {
			if i == 0 {
				return "/"
			}
			return path[0:i]
		}
		i = i - 1
	}
	return "."
}

// isGoFile checks if a filename ends with ".go".
func isGoFile(name string) bool {
	if len(name) < 4 {
		return false
	}
	return name[len(name)-3:len(name)] == ".go"
}

func isGoTestFile(name string) bool {
	return len(name) > 8 && strings.HasSuffix(name, "_test.go")
}

func isUpperASCII(ch byte) bool {
	return ch >= 'A' && ch <= 'Z'
}

func isTopLevelTestFuncName(name string, prefix string) bool {
	if len(name) <= len(prefix) {
		return false
	}
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	return isUpperASCII(name[len(prefix)])
}

func collectTopLevelFuncsWithPrefix(pkg *Package, prefix string) []string {
	var names []string
	seen := make(map[string]bool)
	for _, file := range pkg.Files {
		if file == nil || file.Kind != NFile {
			continue
		}
		for _, node := range file.Nodes {
			if node == nil || node.Kind != NFunc || node.X != nil {
				continue
			}
			if !isTopLevelTestFuncName(node.Name, prefix) {
				continue
			}
			if !seen[node.Name] {
				seen[node.Name] = true
				names = append(names, node.Name)
			}
		}
	}
	sortStrings(names)
	return names
}

func astIdent(name string) *Node {
	return &Node{Kind: NIdent, Name: name}
}

func astString(value string) *Node {
	return &Node{Kind: NStringLit, Name: value}
}

func astInt(value int) *Node {
	return &Node{Kind: NIntLit, Name: fmt.Sprintf("%d", value)}
}

func astSelector(base string, field string) *Node {
	return &Node{Kind: NSelectorExpr, X: astIdent(base), Name: field}
}

func astSelect(base *Node, field string) *Node {
	return &Node{Kind: NSelectorExpr, X: base, Name: field}
}

func astCall(callee *Node, args ...*Node) *Node {
	return &Node{Kind: NCallExpr, X: callee, Nodes: args}
}

func astExprStmt(expr *Node) *Node {
	return &Node{Kind: NExprStmt, X: expr}
}

func astAssign(op string, lhs *Node, rhs *Node) *Node {
	return &Node{Kind: NAssign, Name: op, X: lhs, Y: rhs}
}

func astMultiAssign(op string, lhs []*Node, rhs *Node) *Node {
	return &Node{Kind: NAssign, Name: op, Nodes: lhs, Y: rhs}
}

func astBinary(op string, left *Node, right *Node) *Node {
	return &Node{Kind: NBinaryExpr, Name: op, X: left, Y: right}
}

func astUnary(op string, expr *Node) *Node {
	return &Node{Kind: NUnaryExpr, Name: op, X: expr}
}

func astBlock(stmts ...*Node) *Node {
	var out []*Node
	for _, stmt := range stmts {
		if stmt != nil {
			out = append(out, stmt)
		}
	}
	return &Node{Kind: NBlock, Nodes: out}
}

func astIf(cond *Node, thenBlock *Node) *Node {
	return &Node{Kind: NIf, X: cond, Body: thenBlock}
}

func injectSyntheticTestRunner(pkg *Package) {
	if pkg == nil {
		return
	}
	testNames := collectTopLevelFuncsWithPrefix(pkg, "Test")
	benchNames := collectTopLevelFuncsWithPrefix(pkg, "Benchmark")
	packageName := pkg.Name
	if packageName == "" {
		packageName = "main"
	}
	var decls []*Node
	decls = append(decls, &Node{Kind: NImport, Name: "testing"})

	for _, testName := range testNames {
		fn := &Node{
			Kind: NFunc,
			Name: "__rtg_run_" + testName,
			Nodes: []*Node{
				{Kind: NField, Name: "verbose", Type: astIdent("bool")},
			},
			Type: astIdent("bool"),
			Body: astBlock(
				astAssign(":=", astIdent("t"), astCall(astSelector("testing", "BeginTest"), astString(testName), astIdent("verbose"))),
				&Node{
					Kind: NDeferStmt,
					X: astCall(
						astSelector("testing", "FinishTest"),
						astIdent("t"),
						astString(testName),
						astIdent("verbose"),
					),
				},
				astExprStmt(astCall(astIdent(testName), astIdent("t"))),
				&Node{
					Kind: NReturn,
					X:    astUnary("!", astCall(astSelect(astIdent("t"), "Failed"))),
				},
			),
		}
		decls = append(decls, fn)
	}

	for _, benchName := range benchNames {
		fn := &Node{
			Kind: NFunc,
			Name: "__rtg_bench_" + benchName,
			Nodes: []*Node{
				{Kind: NField, Name: "verbose", Type: astIdent("bool")},
			},
			Type: astIdent("bool"),
			Body: astBlock(
				astAssign(":=", astIdent("b"), astCall(astSelector("testing", "BeginBenchmark"), astString(benchName), astIdent("verbose"))),
				&Node{
					Kind: NDeferStmt,
					X: astCall(
						astSelector("testing", "FinishBenchmark"),
						astIdent("b"),
						astString(benchName),
						astIdent("verbose"),
					),
				},
				astExprStmt(astCall(astSelect(astIdent("b"), "ResetTimer"))),
				astExprStmt(astCall(astIdent(benchName), astIdent("b"))),
				astExprStmt(astCall(astSelect(astIdent("b"), "StopTimer"))),
				astIf(
					astBinary("<=", astSelect(astIdent("b"), "N"), astInt(0)),
					astBlock(astAssign("=", astSelect(astIdent("b"), "N"), astInt(1))),
				),
				astAssign(":=", astIdent("nsPerOp"), astBinary("/", astCall(astSelect(astIdent("b"), "Elapsed")), astSelect(astIdent("b"), "N"))),
				astExprStmt(astCall(astSelector("testing", "PrintBenchmarkResult"), astString(benchName), astSelect(astIdent("b"), "N"), astIdent("nsPerOp"))),
				&Node{
					Kind: NReturn,
					X:    astUnary("!", astCall(astSelect(astIdent("b"), "Failed"))),
				},
			),
		}
		decls = append(decls, fn)
	}

	var stmts []*Node
	stmts = append(stmts, astMultiAssign(":=", []*Node{astIdent("runPattern"), astIdent("benchPattern"), astIdent("verbose")}, astCall(astSelector("testing", "ParseTestArgs"))))
	stmts = append(stmts, astAssign(":=", astIdent("failures"), astInt(0)))
	stmts = append(stmts, astAssign(":=", astIdent("testsRun"), astInt(0)))
	stmts = append(stmts, astAssign(":=", astIdent("benchesRun"), astInt(0)))

	for _, testName := range testNames {
		stmts = append(stmts, astIf(
			astCall(astSelector("testing", "Match"), astString(testName), astIdent("runPattern")),
			astBlock(
				astAssign("=", astIdent("testsRun"), astBinary("+", astIdent("testsRun"), astInt(1))),
				astIf(
					astUnary("!", astCall(astIdent("__rtg_run_"+testName), astIdent("verbose"))),
					astBlock(astAssign("=", astIdent("failures"), astBinary("+", astIdent("failures"), astInt(1)))),
				),
			),
		))
	}

	for _, benchName := range benchNames {
		stmts = append(stmts, astIf(
			astBinary("&&",
				astBinary("!=", astIdent("benchPattern"), astString("")),
				astCall(astSelector("testing", "Match"), astString(benchName), astIdent("benchPattern")),
			),
			astBlock(
				astAssign("=", astIdent("benchesRun"), astBinary("+", astIdent("benchesRun"), astInt(1))),
				astIf(
					astUnary("!", astCall(astIdent("__rtg_bench_"+benchName), astIdent("verbose"))),
					astBlock(astAssign("=", astIdent("failures"), astBinary("+", astIdent("failures"), astInt(1)))),
				),
			),
		))
	}

	stmts = append(stmts, astIf(
		astBinary("!=", astIdent("failures"), astInt(0)),
		astBlock(astExprStmt(astCall(astSelector("testing", "FailAndExit"), astIdent("failures")))),
	))
	stmts = append(stmts, astExprStmt(astCall(astSelector("testing", "PassAndExit"), astIdent("verbose"), astIdent("testsRun"), astIdent("benchesRun"))))
	decls = append(decls, &Node{Kind: NFunc, Name: "init", Body: astBlock(stmts...)})

	file := &Node{Kind: NFile, Name: packageName, Nodes: decls}

	pkg.Files = append(pkg.Files, file)
	pkg.Imports = collectImports(pkg)
}

// parseFile reads, lexes, and parses a single Go source file.
func parseFile(path string) *Node {
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading %s: %v\n", path, err)
		return nil
	}

	// fmt.Fprintf(os.Stderr, "  parsing %s (%d bytes, %d tokens)...\n", path, len(src), 0)
	lexer := NewLexer(string(src))
	tokens := lexer.Tokenize()
	// fmt.Fprintf(os.Stderr, "  tokenized %s: %d tokens\n", path, len(tokens))

	parser := NewParser(tokens)
	file := parser.ParseFile()

	if len(parser.errors) > 0 {
		fmt.Fprintf(os.Stderr, "parse errors in %s:\n", path)
		for _, e := range parser.errors {
			fmt.Fprintf(os.Stderr, "  %s\n", e)
		}
		return nil
	}

	return file
}

// ParseFile is an exported wrapper used by non-frontend packages that need
// RTG-compatible parsing of Go source files with //rtg: directives.
func ParseFile(path string) *Node {
	return parseFile(path)
}

// parseSource lexes and parses source code from a string.
func parseSource(name string, src string) *Node {
	lexer := NewLexer(src)
	tokens := lexer.Tokenize()
	parser := NewParser(tokens)
	file := parser.ParseFile()

	if len(parser.errors) > 0 {
		fmt.Fprintf(os.Stderr, "parse errors in %s:\n", name)
		for _, e := range parser.errors {
			fmt.Fprintf(os.Stderr, "  %s\n", e)
		}
		return nil
	}

	return file
}

// ParseSource is an exported wrapper used by tooling that needs AST access.
func ParseSource(name string, src string) *Node {
	return parseSource(name, src)
}

// shouldIncludeContent checks if source content should be included based on build tags.
// This is like shouldIncludeFile but takes content directly instead of reading from disk.
func (c *Preprocessor) shouldIncludeContent(content string, name string) bool {
	collectBuildTagsFromContent(content)
	collectBuildTagsFromFilename(name)

	// 1. Check //go:build directive in content (takes precedence)
	pos := 0
	for pos < len(content) {
		eol := pos
		for eol < len(content) && content[eol] != '\n' {
			eol++
		}
		line := content[pos:eol]
		trimmed := trimLeftSpace(line)
		if len(trimmed) == 0 {
			pos = eol + 1
			continue
		}
		if len(trimmed) >= 11 && trimmed[0:11] == "//go:build " {
			expr := trimmed[11:len(trimmed)]
			return c.evalBuildExpr(expr)
		}
		if len(trimmed) >= 2 && trimmed[0:2] == "//" {
			pos = eol + 1
			continue
		}
		break
	}

	// 2. Filename-based tag filtering
	base := name[0 : len(name)-3]
	parts := splitString(base, '_')
	if len(parts) >= 3 {
		maybearch := parts[len(parts)-1]
		maybeos := parts[len(parts)-2]
		if isKnownOS(maybeos) && isKnownArch(maybearch) {
			if !c.hasTag(maybeos) || !c.hasTag(maybearch) {
				return false
			}
		} else if isKnownOS(maybearch) || isKnownArch(maybearch) {
			if !c.hasTag(maybearch) {
				return false
			}
		}
	} else if len(parts) >= 2 {
		last := parts[len(parts)-1]
		if isKnownOS(last) || isKnownArch(last) {
			if !c.hasTag(last) {
				return false
			}
		}
	}
	return true
}

// collectImports walks NFile.Nodes for NImport nodes and returns deduplicated import paths.
func collectImports(pkg *Package) []string {
	seen := make(map[string]bool)
	aliases := make(map[string]string)
	var result []string
	for _, file := range pkg.Files {
		for _, node := range file.Nodes {
			if node.Kind == NImport {
				path := node.Name
				if !seen[path] {
					seen[path] = true
					result = append(result, path)
				}
				if node.X != nil && node.X.Kind == NIdent {
					alias := node.X.Name
					if alias != "" && alias != "_" && alias != "." {
						aliases[alias] = path
					}
				}
			}
		}
	}
	pkg.ImportAliases = aliases
	return result
}

// topologicalSort performs a DFS-based topological sort on the import graph.
type topoState struct {
	pkgs    map[string]*Package
	visited map[string]bool
	order   []string
}

func (ts *topoState) visit(path string) {
	if ts.visited[path] {
		return
	}
	ts.visited[path] = true
	pkg, ok := ts.pkgs[path]
	if ok {
		for _, imp := range pkg.Imports {
			ts.visit(imp)
		}
	}
	ts.order = append(ts.order, path)
}

func topologicalSort(pkgs map[string]*Package) []string {
	ts := &topoState{
		pkgs:    pkgs,
		visited: make(map[string]bool),
	}
	// Sort map keys to keep package visitation deterministic across runtimes.
	var paths []string
	for path := range pkgs {
		paths = append(paths, path)
	}
	sortStrings(paths)
	for _, path := range paths {
		ts.visit(path)
	}
	return ts.order
}

// collectSymbols walks top-level declarations and populates pkg.Symbols.
func collectSymbols(pkg *Package) {
	for _, file := range pkg.Files {
		for _, node := range file.Nodes {
			collectDeclSymbol(pkg, node)
		}
	}
}

// collectDeclSymbol registers a single top-level declaration as a symbol.
func collectDeclSymbol(pkg *Package, node *Node) {
	if node == nil {
		return
	}

	switch node.Kind {
	case NFunc:
		sym := &Symbol{Name: node.Name, Kind: SymFunc, Node: node, Pkg: pkg}
		pkg.Symbols[node.Name] = sym
		if node.Name == "init" {
			pkg.Inits = append(pkg.Inits, node)
		}
	case NTypeDecl:
		sym := &Symbol{Name: node.Name, Kind: SymType, Node: node, Pkg: pkg}
		pkg.Symbols[node.Name] = sym
	case NVarDecl:
		if len(node.Nodes) > 0 {
			for _, child := range node.Nodes {
				sym := &Symbol{Name: child.Name, Kind: SymVar, Node: child, Pkg: pkg}
				pkg.Symbols[child.Name] = sym
			}
		} else {
			sym := &Symbol{Name: node.Name, Kind: SymVar, Node: node, Pkg: pkg}
			pkg.Symbols[node.Name] = sym
		}
	case NConstDecl:
		if len(node.Nodes) > 0 {
			// Grouped const declaration
			for _, child := range node.Nodes {
				sym := &Symbol{Name: child.Name, Kind: SymConst, Node: child, Pkg: pkg}
				pkg.Symbols[child.Name] = sym
			}
		} else {
			sym := &Symbol{Name: node.Name, Kind: SymConst, Node: node, Pkg: pkg}
			pkg.Symbols[node.Name] = sym
		}
	case NDirective:
		// Unwrap the directive, register the inner decl, and mark intrinsic
		if node.X != nil {
			collectDeclSymbol(pkg, node.X)
			// Parse directive name for "internal FuncName"
			intern := parseInternalDirective(node.Name)
			if intern != "" && node.X.Name != "" {
				sym, ok := pkg.Symbols[node.X.Name]
				if ok {
					sym.Intern = intern
				}
			}
			// Check for embed directive
			if len(node.Name) > 6 && node.Name[0:6] == "embed " && node.X.Name != "" {
				sym, ok := pkg.Symbols[node.X.Name]
				if ok {
					sym.Embed = node.Name[6:len(node.Name)]
				}
			}
			if _, ok := parseLinkStaticDirective(node.Name); ok && node.X.Name != "" {
				sym, exists := pkg.Symbols[node.X.Name]
				if exists {
					sym.LinkStatic = true
				}
			}
		}
	case NBlock:
		// Grouped type declarations
		for _, child := range node.Nodes {
			collectDeclSymbol(pkg, child)
		}
	case NImport:
		// Skip imports
	}
}

// ValidateModule checks cross-package references and returns errors.
func ValidateModule(mod *Module) []string {
	var errors []string
	for _, path := range mod.Order {
		pkg, ok := mod.Packages[path]
		if !ok {
			continue
		}
		// Build import map: package name → *Package
		importMap := make(map[string]*Package)
		for _, imp := range pkg.Imports {
			ipkg, iok := mod.Packages[imp]
			if iok {
				importMap[ipkg.Name] = ipkg
			}
		}
		for alias, imp := range pkg.ImportAliases {
			ipkg, iok := mod.Packages[imp]
			if iok {
				importMap[alias] = ipkg
			}
		}
		for _, file := range pkg.Files {
			for _, node := range file.Nodes {
				validateNode(pkg, importMap, node, &errors)
			}
		}
	}
	return errors
}

func validateNode(pkg *Package, imports map[string]*Package, node *Node, errors *[]string) {
	if node == nil {
		return
	}
	stack := make([]*Node, 0, 64)
	stack = append(stack, node)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n == nil {
			continue
		}

		// Check selector expressions: pkg.Name references
		if n.Kind == NSelectorExpr && n.X != nil && n.X.Kind == NIdent {
			target, isImport := imports[n.X.Name]
			if isImport {
				_, hasSym := target.Symbols[n.Name]
				allowRuntimeMemBuiltin := target.Path == "runtime" && isRuntimeMemBuiltinName(n.Name)
				if !hasSym && !allowRuntimeMemBuiltin {
					*errors = append(*errors, fmt.Sprintf("%s: %s.%s undefined (package %s has no symbol %s)", pkg.Path, n.X.Name, n.Name, target.Path, n.Name))
				}
			}
		}

		// Check that imported package names used as bare identifiers are valid
		if n.Kind == NCallExpr && n.X != nil && n.X.Kind == NIdent {
			name := n.X.Name
			// If calling an identifier that matches an import name, that's wrong
			_, isImport := imports[name]
			if isImport {
				*errors = append(*errors, fmt.Sprintf("%s: %s used as function (is a package name)", pkg.Path, name))
			}
		}

		if n.X != nil {
			stack = append(stack, n.X)
		}
		if n.Y != nil {
			stack = append(stack, n.Y)
		}
		if n.Body != nil {
			stack = append(stack, n.Body)
		}
		if n.Type != nil {
			stack = append(stack, n.Type)
		}
		for i := len(n.Nodes) - 1; i >= 0; i-- {
			child := n.Nodes[i]
			if child != nil {
				stack = append(stack, child)
			}
		}
	}
}

// parseInternalDirective parses a directive value like "internal Syscall"
// and returns the intrinsic name ("Syscall"), or "" if not an internal directive.
func parseInternalDirective(val string) string {
	prefix := "internal "
	if len(val) <= len(prefix) {
		return ""
	}
	if val[0:len(prefix)] != prefix {
		return ""
	}
	return val[len(prefix):len(val)]
}

func isComptimeDirective(val string) bool {
	return strings.TrimSpace(val) == "comptime"
}

func isZeroCallDirective(val string) bool {
	return strings.TrimSpace(val) == "zerocall"
}

func parseProfileDirective(val string) bool {
	return strings.TrimSpace(val) == "profile"
}

func parseNoProfileDirective(val string) bool {
	return strings.TrimSpace(val) == "noprofile"
}

// CollectProfileMethodQualNames returns qualified method names marked with //rtg:profile.
func CollectProfileMethodQualNames(mod *Module) []string {
	return collectCallableQualNames(mod, true, true)
}

// CollectMethodQualNames returns qualified method names for all methods in the module.
func CollectMethodQualNames(mod *Module) []string {
	return collectCallableQualNames(mod, true, false)
}

// CollectCallableQualNames returns qualified function and method names in the module.
func CollectCallableQualNames(mod *Module) []string {
	return collectCallableQualNames(mod, false, false)
}

func collectCallableQualNames(mod *Module, methodsOnly bool, profileOnly bool) []string {
	if mod == nil {
		return nil
	}
	var out []string
	seen := make(map[string]bool)
	for _, pkgPath := range mod.Order {
		pkg, ok := mod.Packages[pkgPath]
		if !ok {
			continue
		}
		for _, file := range pkg.Files {
			for _, node := range file.Nodes {
				base, directives := unwrapDirectiveNode(node)
				if base == nil || base.Kind != NFunc {
					continue
				}
				if profileOnly {
					hasProfile := false
					for _, d := range directives {
						if parseProfileDirective(d) {
							hasProfile = true
							break
						}
					}
					if !hasProfile {
						continue
					}
				}
				var qname string
				if base.X != nil && base.X.Type != nil {
					recvType := nodeTypeName(base.X.Type)
					if recvType == "" {
						continue
					}
					qname = pkg.QualName(recvType) + "." + base.Name
				} else {
					if methodsOnly {
						continue
					}
					qname = pkg.QualName(base.Name)
				}
				if !seen[qname] {
					seen[qname] = true
					out = append(out, qname)
				}
			}
		}
	}
	return out
}

func isCallbackDirective(val string) bool {
	return strings.TrimSpace(val) == "callback"
}

func parseAssembleDirective(val string) (string, bool) {
	prefix := "assemble "
	trimmed := strings.TrimSpace(val)
	if len(trimmed) <= len(prefix) || trimmed[0:len(prefix)] != prefix {
		return "", false
	}
	arch := strings.TrimSpace(trimmed[len(prefix):len(trimmed)])
	if arch == "" {
		return "", false
	}
	return arch, true
}

func parseTargetDirective(val string) (string, bool) {
	prefix := "target "
	trimmed := strings.TrimSpace(val)
	if len(trimmed) <= len(prefix) || trimmed[0:len(prefix)] != prefix {
		return "", false
	}
	triple := strings.TrimSpace(trimmed[len(prefix):len(trimmed)])
	if triple == "" || strings.Index(triple, "/") < 0 {
		return "", false
	}
	return triple, true
}

func parseTargetABIDirective(val string) (string, bool) {
	prefix := "targetabi "
	trimmed := strings.TrimSpace(val)
	if len(trimmed) <= len(prefix) || trimmed[0:len(prefix)] != prefix {
		return "", false
	}
	triple := strings.TrimSpace(trimmed[len(prefix):len(trimmed)])
	if triple == "" || strings.Index(triple, "/") < 0 {
		return "", false
	}
	return triple, true
}

func parseAssemblerDirective(val string) (string, bool) {
	prefix := "assembler "
	trimmed := strings.TrimSpace(val)
	if len(trimmed) <= len(prefix) || trimmed[0:len(prefix)] != prefix {
		return "", false
	}
	name := strings.TrimSpace(trimmed[len(prefix):len(trimmed)])
	if name == "" {
		return "", false
	}
	return name, true
}

func parseBinFormatDirective(val string) (string, bool) {
	prefix := "binfmt "
	trimmed := strings.TrimSpace(val)
	if len(trimmed) <= len(prefix) || trimmed[0:len(prefix)] != prefix {
		return "", false
	}
	name := strings.TrimSpace(trimmed[len(prefix):len(trimmed)])
	if name == "" {
		return "", false
	}
	return name, true
}

type CompileAsDirective struct {
	ID     string
	Target string
}

func parseDirectiveKVArgs(rest string) map[string]string {
	parts := strings.Fields(strings.TrimSpace(rest))
	values := make(map[string]string)
	for _, part := range parts {
		eq := strings.Index(part, "=")
		if eq <= 0 || eq >= len(part)-1 {
			continue
		}
		key := strings.TrimSpace(part[0:eq])
		val := strings.TrimSpace(part[eq+1:])
		if key == "" || val == "" {
			continue
		}
		values[key] = val
	}
	return values
}

func parseCompileAsDirective(val string) (CompileAsDirective, bool) {
	prefix := "compileas "
	trimmed := strings.TrimSpace(val)
	if len(trimmed) <= len(prefix) || trimmed[0:len(prefix)] != prefix {
		return CompileAsDirective{}, false
	}
	rest := strings.TrimSpace(trimmed[len(prefix):len(trimmed)])
	if rest == "" {
		return CompileAsDirective{}, false
	}
	kv := parseDirectiveKVArgs(rest)
	id := kv["id"]
	target := kv["target"]
	if id == "" || target == "" {
		return CompileAsDirective{}, false
	}
	if strings.Index(target, "/") < 0 {
		return CompileAsDirective{}, false
	}
	return CompileAsDirective{ID: id, Target: target}, true
}

func parseArtifactDirective(val string) (string, bool) {
	prefix := "artifact "
	trimmed := strings.TrimSpace(val)
	if len(trimmed) <= len(prefix) || trimmed[0:len(prefix)] != prefix {
		return "", false
	}
	rest := strings.TrimSpace(trimmed[len(prefix):len(trimmed)])
	if rest == "" {
		return "", false
	}
	kv := parseDirectiveKVArgs(rest)
	id := kv["id"]
	if id == "" {
		return "", false
	}
	return id, true
}

func funcReturnCount(node *Node) int {
	if node == nil || node.Type == nil {
		return 0
	}
	if node.Type.Kind == NFuncType {
		return len(node.Type.Nodes)
	}
	return 1
}

func isByteSliceType(node *Node) bool {
	if node == nil || node.Kind != NSliceType || node.X == nil {
		return false
	}
	return node.X.Kind == NIdent && node.X.Name == "byte"
}

type CompileAsSpec struct {
	ID          string
	Target      string
	EntryFunc   string
	ArtifactVar string
}

func collectCompileAsDirectives(pkg *Package) ([]CompileAsSpec, []CompileAsSpec, []string) {
	var compileSpecs []CompileAsSpec
	var artifactSpecs []CompileAsSpec
	var errs []string

	for _, file := range pkg.Files {
		for _, node := range file.Nodes {
			base, directives := unwrapDirectiveNode(node)
			if base == nil {
				continue
			}
			for _, d := range directives {
				if spec, ok := parseCompileAsDirective(d); ok {
					if base.Kind != NFunc {
						errs = append(errs, fmt.Sprintf("%s: line %d: //rtg:compileas must annotate a function", pkg.Path, node.Pos))
						continue
					}
					if base.X != nil {
						errs = append(errs, fmt.Sprintf("%s.%s: //rtg:compileas cannot annotate methods", pkg.Path, base.Name))
						continue
					}
					if len(base.Nodes) != 0 {
						errs = append(errs, fmt.Sprintf("%s.%s: //rtg:compileas requires zero parameters", pkg.Path, base.Name))
						continue
					}
					if funcReturnCount(base) != 0 {
						errs = append(errs, fmt.Sprintf("%s.%s: //rtg:compileas requires zero return values", pkg.Path, base.Name))
						continue
					}
					compileSpecs = append(compileSpecs, CompileAsSpec{
						ID:        spec.ID,
						Target:    spec.Target,
						EntryFunc: pkg.QualName(base.Name),
					})
				}
				if id, ok := parseArtifactDirective(d); ok {
					if base.Kind != NVarDecl {
						errs = append(errs, fmt.Sprintf("%s: line %d: //rtg:artifact must annotate a variable", pkg.Path, node.Pos))
						continue
					}
					if len(base.Nodes) != 0 || base.Name == "" {
						errs = append(errs, fmt.Sprintf("%s: line %d: //rtg:artifact must annotate a single variable declaration", pkg.Path, node.Pos))
						continue
					}
					if base.X != nil {
						errs = append(errs, fmt.Sprintf("%s.%s: //rtg:artifact variable cannot have an initializer", pkg.Path, base.Name))
						continue
					}
					if !isByteSliceType(base.Type) {
						errs = append(errs, fmt.Sprintf("%s.%s: //rtg:artifact requires type []byte", pkg.Path, base.Name))
						continue
					}
					artifactSpecs = append(artifactSpecs, CompileAsSpec{
						ID:          id,
						ArtifactVar: pkg.QualName(base.Name),
					})
				}
			}
		}
	}
	return compileSpecs, artifactSpecs, errs
}

func sortCompileAsSpecs(specs []CompileAsSpec) {
	i := 1
	for i < len(specs) {
		j := i
		for j > 0 {
			if specs[j-1].ID < specs[j].ID {
				break
			}
			if specs[j-1].ID == specs[j].ID && specs[j-1].EntryFunc <= specs[j].EntryFunc {
				break
			}
			specs[j-1], specs[j] = specs[j], specs[j-1]
			j--
		}
		i++
	}
}

func CollectCompileAsSpecs(mod *Module) ([]CompileAsSpec, []string) {
	if mod == nil {
		return nil, nil
	}

	compileByID := make(map[string]CompileAsSpec)
	artifactByID := make(map[string]CompileAsSpec)
	var errs []string

	for _, pkgPath := range mod.Order {
		pkg, ok := mod.Packages[pkgPath]
		if !ok {
			continue
		}
		compileSpecs, artifactSpecs, pkgErrs := collectCompileAsDirectives(pkg)
		if len(pkgErrs) > 0 {
			errs = append(errs, pkgErrs...)
		}
		for _, spec := range compileSpecs {
			if prev, exists := compileByID[spec.ID]; exists {
				errs = append(errs, fmt.Sprintf("duplicate //rtg:compileas id=%s (%s, %s)", spec.ID, prev.EntryFunc, spec.EntryFunc))
				continue
			}
			compileByID[spec.ID] = spec
		}
		for _, spec := range artifactSpecs {
			if prev, exists := artifactByID[spec.ID]; exists {
				errs = append(errs, fmt.Sprintf("duplicate //rtg:artifact id=%s (%s, %s)", spec.ID, prev.ArtifactVar, spec.ArtifactVar))
				continue
			}
			artifactByID[spec.ID] = spec
		}
	}

	var out []CompileAsSpec
	for id, compileSpec := range compileByID {
		artifactSpec, ok := artifactByID[id]
		if !ok {
			errs = append(errs, fmt.Sprintf("//rtg:compileas id=%s is missing matching //rtg:artifact", id))
			continue
		}
		out = append(out, CompileAsSpec{
			ID:          id,
			Target:      compileSpec.Target,
			EntryFunc:   compileSpec.EntryFunc,
			ArtifactVar: artifactSpec.ArtifactVar,
		})
	}
	for id := range artifactByID {
		if _, ok := compileByID[id]; !ok {
			errs = append(errs, fmt.Sprintf("//rtg:artifact id=%s is missing matching //rtg:compileas", id))
		}
	}
	sortCompileAsSpecs(out)
	return out, errs
}

// LinkStaticDirective describes a static-link external symbol target.
type LinkStaticDirective struct {
	Library string
	Symbol  string
	Mode    string
}

// parseLinkStaticDirective parses a directive value like:
//
//	"linkstatic libSystem.dylib,_write"
//	"linkstatic libSystem.dylib,_getcwd,ptr"
//
// and returns metadata plus true on success.
func parseLinkStaticDirective(val string) (LinkStaticDirective, bool) {
	prefix := "linkstatic "
	if len(val) <= len(prefix) || val[0:len(prefix)] != prefix {
		return LinkStaticDirective{}, false
	}
	rest := strings.TrimSpace(val[len(prefix):len(val)])
	if rest == "" {
		return LinkStaticDirective{}, false
	}
	parts := strings.Split(rest, ",")
	if len(parts) < 2 || len(parts) > 3 {
		return LinkStaticDirective{}, false
	}
	lib := strings.TrimSpace(parts[0])
	sym := strings.TrimSpace(parts[1])
	mode := ""
	if len(parts) == 3 {
		mode = strings.TrimSpace(parts[2])
	}
	if lib == "" || sym == "" {
		return LinkStaticDirective{}, false
	}
	return LinkStaticDirective{Library: lib, Symbol: sym, Mode: mode}, true
}
