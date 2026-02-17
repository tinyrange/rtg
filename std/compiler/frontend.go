package main

import (
	"fmt"
	"os"
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
	Name   string
	Kind   SymKind
	Node   *Node
	Pkg    *Package
	Intern string
	Embed  string
}

// Package represents a parsed Go package.
type Package struct {
	Name         string
	Path         string
	Dir          string
	Files        []*Node
	Imports      []string
	Symbols      map[string]*Symbol
	Inits        []*Node
	qualNames    map[string]string // name → "Path.name"
	qualPtrNames map[string]string // name → "Path.*name"
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

// ResolveModule parses entry files and recursively resolves all imports.
func ResolveModule(baseDir string, entryFiles []string) *Module {
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
		mainPkg = parsePackageFromEmbed(arg)
		if mainPkg == nil {
			mainPkg = parsePackageDir(entryDir, "main")
		}
	} else {
		// "." or directory: scan the directory for all .go files
		mainPkg = parsePackageDir(entryDir, "main")
	}
	if mainPkg == nil {
		fmt.Fprintf(os.Stderr, "error: no Go files found in %s\n", entryDir)
		os.Exit(1)
	}
	mainPkg.Path = "main"
	mod.Packages["main"] = mainPkg
	mod.Entry = mainPkg

	// Worklist loop: resolve imports recursively
	var worklist []string
	for _, imp := range mainPkg.Imports {
		worklist = append(worklist, imp)
	}

	for len(worklist) > 0 {
		importPath := worklist[0]
		worklist = worklist[1:len(worklist)]

		_, already := mod.Packages[importPath]
		if already {
			continue
		}

		// Try embedded std first, then fall back to disk
		pkg := parsePackageFromEmbed(importPath)
		if pkg == nil {
			dir := resolveImportDir(baseDir, importPath)
			if dir == "" {
				fmt.Fprintf(os.Stderr, "warning: cannot resolve import %s\n", importPath)
				continue
			}
			pkg = parsePackageDir(dir, importPath)
			if pkg == nil {
				fmt.Fprintf(os.Stderr, "warning: no Go files for import %s in %s\n", importPath, dir)
				continue
			}
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

// resolveImportDir maps an import path to a directory on disk.
func resolveImportDir(baseDir string, importPath string) string {
	return baseDir + "/std/" + importPath
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
func shouldIncludeFile(path string, name string) bool {
	// 1. Check //go:build directive in file content (takes precedence)
	src, err := os.ReadFile(path)
	if err != nil {
		return true // if can't read, include by default
	}
	content := string(src)

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
			return evalBuildExpr(expr)
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
			if !hasTag(maybeos) || !hasTag(maybearch) {
				return false
			}
		} else if isKnownOS(maybearch) || isKnownArch(maybearch) {
			if !hasTag(maybearch) {
				return false
			}
		}
	} else if len(parts) >= 2 {
		last := parts[len(parts)-1]
		if isKnownOS(last) || isKnownArch(last) {
			if !hasTag(last) {
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
	return s == "linux" || s == "darwin" || s == "windows" || s == "freebsd" || s == "wasi"
}

// isKnownArch returns true if s is a known GOARCH value.
func isKnownArch(s string) bool {
	return s == "amd64" || s == "386" || s == "arm64" || s == "arm" || s == "wasm32"
}

// hasTag checks if a tag is in the active build tag set.
func hasTag(tag string) bool {
	i := 0
	for i < len(buildTags) {
		if buildTags[i] == tag {
			return true
		}
		i++
	}
	return false
}

// evalBuildExpr evaluates a //go:build expression against the active tag set.
// Supports: bare tags, &&, ||, !, and parentheses.
func evalBuildExpr(expr string) bool {
	expr = trimLeftSpace(expr)
	result, _ := parseBuildOr(expr)
	return result
}

// parseBuildOr parses: term (|| term)*
func parseBuildOr(expr string) (bool, string) {
	left, rest := parseBuildAnd(expr)
	for {
		rest = trimLeftSpace(rest)
		if len(rest) >= 2 && rest[0] == '|' && rest[1] == '|' {
			var right bool
			right, rest = parseBuildAnd(rest[2:len(rest)])
			left = left || right
		} else {
			break
		}
	}
	return left, rest
}

// parseBuildAnd parses: unary (&& unary)*
func parseBuildAnd(expr string) (bool, string) {
	left, rest := parseBuildUnary(expr)
	for {
		rest = trimLeftSpace(rest)
		if len(rest) >= 2 && rest[0] == '&' && rest[1] == '&' {
			var right bool
			right, rest = parseBuildUnary(rest[2:len(rest)])
			left = left && right
		} else {
			break
		}
	}
	return left, rest
}

// parseBuildUnary parses: !unary | atom
func parseBuildUnary(expr string) (bool, string) {
	expr = trimLeftSpace(expr)
	if len(expr) > 0 && expr[0] == '!' {
		val, rest := parseBuildUnary(expr[1:len(expr)])
		return !val, rest
	}
	return parseBuildAtom(expr)
}

// parseBuildAtom parses: (expr) | tag
func parseBuildAtom(expr string) (bool, string) {
	expr = trimLeftSpace(expr)
	if len(expr) > 0 && expr[0] == '(' {
		val, rest := parseBuildOr(expr[1:len(expr)])
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
	return hasTag(tag), expr[i:len(expr)]
}

// isAlphaNum returns true if c is a letter or digit.
func isAlphaNum(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// parsePackageDir lists .go files in a directory, parses each, and merges into one Package.
func parsePackageDir(dir string, importPath string) *Package {
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
		// Check build tags before including
		if !shouldIncludeFile(dir+"/"+entry.Name(), entry.Name()) {
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

// shouldIncludeContent checks if source content should be included based on build tags.
// This is like shouldIncludeFile but takes content directly instead of reading from disk.
func shouldIncludeContent(content string, name string) bool {
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
			return evalBuildExpr(expr)
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
			if !hasTag(maybeos) || !hasTag(maybearch) {
				return false
			}
		} else if isKnownOS(maybearch) || isKnownArch(maybearch) {
			if !hasTag(maybearch) {
				return false
			}
		}
	} else if len(parts) >= 2 {
		last := parts[len(parts)-1]
		if isKnownOS(last) || isKnownArch(last) {
			if !hasTag(last) {
				return false
			}
		}
	}
	return true
}

// collectImports walks NFile.Nodes for NImport nodes and returns deduplicated import paths.
func collectImports(pkg *Package) []string {
	seen := make(map[string]bool)
	var result []string
	for _, file := range pkg.Files {
		for _, node := range file.Nodes {
			if node.Kind == NImport {
				path := node.Name
				if !seen[path] {
					seen[path] = true
					result = append(result, path)
				}
			}
		}
	}
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
	for path := range pkgs {
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
		sym := &Symbol{Name: node.Name, Kind: SymVar, Node: node, Pkg: pkg}
		pkg.Symbols[node.Name] = sym
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

	// Check selector expressions: pkg.Name references
	if node.Kind == NSelectorExpr && node.X != nil && node.X.Kind == NIdent {
		target, isImport := imports[node.X.Name]
		if isImport {
			_, hasSym := target.Symbols[node.Name]
			if !hasSym {
				*errors = append(*errors, fmt.Sprintf("%s: %s.%s undefined (package %s has no symbol %s)", pkg.Path, node.X.Name, node.Name, target.Path, node.Name))
			}
		}
	}

	// Check that imported package names used as bare identifiers are valid
	if node.Kind == NCallExpr && node.X != nil {
		if node.X.Kind == NIdent {
			name := node.X.Name
			// If calling an identifier that matches an import name, that's wrong
			_, isImport := imports[name]
			if isImport {
				*errors = append(*errors, fmt.Sprintf("%s: %s used as function (is a package name)", pkg.Path, name))
			}
		}
	}

	// Recurse
	validateNode(pkg, imports, node.X, errors)
	validateNode(pkg, imports, node.Y, errors)
	validateNode(pkg, imports, node.Body, errors)
	validateNode(pkg, imports, node.Type, errors)
	for _, child := range node.Nodes {
		validateNode(pkg, imports, child, errors)
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
