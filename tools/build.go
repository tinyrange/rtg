///usr/bin/true; exec /usr/bin/env go run "$0" "$@"

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// ============================================================================
// Data Structures
// ============================================================================

// Command represents a single command in a target
type Command struct {
	Line     string   // Original line for error messages
	LineNum  int      // Line number for error messages
	Platform string   // "" (any), "darwin", "linux", "windows"
	Type     string   // "gobuild", "run", "copy", "mkdir", "rm", "sh"
	Args     []string // Parsed arguments
}

// Target represents a build target
type Target struct {
	Name         string
	Platforms    []string  // Empty = all platforms (from name[platforms]:)
	Requires     []string  // Required OS (errors if not met)
	Dependencies []string  // Target names this depends on
	Commands     []Command // Commands to execute
	LineNum      int       // Line number where target was defined
}

// Buildfile represents a parsed build configuration
type Buildfile struct {
	Variables map[string]string
	Targets   map[string]*Target
	Path      string // Path to the buildfile
}

// ============================================================================
// Parser
// ============================================================================

type parseState int

const (
	stateTopLevel parseState = iota
	stateInTarget
)

// parseBuildfile parses a Buildfile from a reader
func parseBuildfile(path string, content []byte) (*Buildfile, error) {
	bf := &Buildfile{
		Variables: make(map[string]string),
		Targets:   make(map[string]*Target),
		Path:      path,
	}

	// Add built-in variables
	bf.Variables["GOOS"] = runtime.GOOS
	bf.Variables["GOARCH"] = runtime.GOARCH
	bf.Variables["EXE"] = ""
	if runtime.GOOS == "windows" {
		bf.Variables["EXE"] = ".exe"
	}
	bf.Variables["SHLIB_EXT"] = ".so"
	if runtime.GOOS == "darwin" {
		bf.Variables["SHLIB_EXT"] = ".dylib"
	} else if runtime.GOOS == "windows" {
		bf.Variables["SHLIB_EXT"] = ".dll"
	}

	// Get version from git
	bf.Variables["VERSION"] = getVersionFromGit()

	lines := strings.Split(string(content), "\n")
	state := stateTopLevel
	var currentTarget *Target
	var continuedLine string
	continuedLineNum := 0

	for i, line := range lines {
		lineNum := i + 1

		// Handle line continuation
		if strings.HasSuffix(line, "\\") {
			if continuedLine == "" {
				continuedLineNum = lineNum
			}
			continuedLine += strings.TrimSuffix(line, "\\") + " "
			continue
		}
		if continuedLine != "" {
			line = continuedLine + line
			lineNum = continuedLineNum
			continuedLine = ""
			continuedLineNum = 0
		}

		// Strip comments (but not inside quoted strings - simple approach)
		if idx := strings.Index(line, "#"); idx >= 0 {
			// Simple check: count quotes before the #
			prefix := line[0:idx]
			if strings.Count(prefix, "\"")%2 == 0 && strings.Count(prefix, "'")%2 == 0 {
				line = prefix
			}
		}

		// Trim trailing whitespace
		line = strings.TrimRight(line, " \t\r")

		// Empty line
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Check if line starts with whitespace (command in target)
		startsWithWhitespace := len(line) > 0 && (line[0] == ' ' || line[0] == '\t')
		trimmedLine := strings.TrimSpace(line)

		if startsWithWhitespace && state == stateInTarget && currentTarget != nil {
			// Parse command
			cmd, err := parseCommand(trimmedLine, lineNum)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %w", path, lineNum, err)
			}

			// Check for requires directive (must be first command)
			if cmd.Type == "requires" {
				if len(currentTarget.Commands) > 0 {
					return nil, fmt.Errorf("%s:%d: 'requires' must be the first directive in a target", path, lineNum)
				}
				currentTarget.Requires = cmd.Args
				continue
			}

			currentTarget.Commands = append(currentTarget.Commands, cmd)
			continue
		}

		// Non-indented line - either variable or target header
		state = stateTopLevel
		currentTarget = nil

		// Variable definition: NAME = value
		if idx := strings.Index(trimmedLine, "="); idx > 0 {
			// Check it's not inside a target header (has :)
			if !strings.Contains(trimmedLine[0:idx], ":") {
				name := strings.TrimSpace(trimmedLine[0:idx])
				value := strings.TrimSpace(trimmedLine[idx+1:])
				if isValidIdentifier(name) {
					bf.Variables[name] = value
					continue
				}
			}
		}

		// Target header: name: deps or name[platforms]: deps
		if idx := strings.Index(trimmedLine, ":"); idx > 0 {
			header := trimmedLine[0:idx]
			deps := strings.TrimSpace(trimmedLine[idx+1:])

			// Parse platform specifier: name[platforms]
			var name string
			var platforms []string
			if bracketIdx := strings.Index(header, "["); bracketIdx > 0 {
				if !strings.HasSuffix(header, "]") {
					return nil, fmt.Errorf("%s:%d: unclosed platform specifier", path, lineNum)
				}
				name = strings.TrimSpace(header[0:bracketIdx])
				platformStr := header[bracketIdx+1 : len(header)-1]
				platFields := strings.Fields(platformStr)
				for _, p := range platFields {
					platforms = append(platforms, p)
				}
			} else {
				name = strings.TrimSpace(header)
			}

			if !isValidIdentifier(name) {
				return nil, fmt.Errorf("%s:%d: invalid target name %q", path, lineNum, name)
			}

			// Parse dependencies
			var dependencies []string
			if deps != "" {
				depFields := strings.Fields(deps)
				for _, dep := range depFields {
					dependencies = append(dependencies, dep)
				}
			}

			currentTarget = &Target{
				Name:         name,
				Platforms:    platforms,
				Dependencies: dependencies,
				LineNum:      lineNum,
			}
			bf.Targets[name] = currentTarget
			state = stateInTarget
			continue
		}

		return nil, fmt.Errorf("%s:%d: unexpected line: %s", path, lineNum, trimmedLine)
	}

	return bf, nil
}

// parseCommand parses a command line into a Command struct
func parseCommand(line string, lineNum int) (Command, error) {
	cmd := Command{
		Line:    line,
		LineNum: lineNum,
	}

	// Check for platform conditional: @darwin, @linux, @windows
	if strings.HasPrefix(line, "@") {
		parts := strings.SplitN(line, " ", 2)
		cmd.Platform = strings.TrimPrefix(parts[0], "@")
		if len(parts) < 2 {
			return cmd, fmt.Errorf("platform conditional without command")
		}
		line = strings.TrimSpace(parts[1])
	}

	// Parse command type and arguments
	args := tokenize(line)
	if len(args) == 0 {
		return cmd, fmt.Errorf("empty command")
	}

	cmd.Type = args[0]
	cmd.Args = args[1:]

	// Validate command type
	validTypes := map[string]bool{
		"gobuild":      true,
		"fullcompiler": true,
		"run":          true,
		"copy":         true,
		"mkdir":        true,
		"rm":           true,
		"sh":           true,
		"requires":     true,
	}

	if !validTypes[cmd.Type] {
		return cmd, fmt.Errorf("unknown command type %q (use 'sh' prefix for shell commands)", cmd.Type)
	}

	return cmd, nil
}

// tokenize splits a command line into tokens, respecting quoted strings
func tokenize(line string) []string {
	var tokens []string
	current := strings.Builder{}
	inQuote := false
	var quoteChar byte

	i := 0
	for i < len(line) {
		ch := line[i]
		if inQuote {
			if ch == quoteChar {
				inQuote = false
			} else {
				current.WriteByte(ch)
			}
		} else {
			if ch == '"' || ch == '\'' {
				inQuote = true
				quoteChar = ch
			} else if ch == ' ' || ch == '\t' {
				if current.Len() > 0 {
					tokens = append(tokens, current.String())
					current.Reset()
				}
			} else {
				current.WriteByte(ch)
			}
		}
		i++
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

// isValidIdentifier checks if a string is a valid identifier
func isValidIdentifier(s string) bool {
	if s == "" {
		return false
	}
	i := 0
	for i < len(s) {
		r := s[i]
		if i == 0 {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_') {
				return false
			}
		} else {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
				return false
			}
		}
		i++
	}
	return true
}

// expandVariables expands $(VAR) and ${VAR} in a string
func (bf *Buildfile) expandVariables(s string) string {
	result := strings.Builder{}
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '$' && (s[i+1] == '(' || s[i+1] == '{') {
			closer := byte(')')
			if s[i+1] == '{' {
				closer = '}'
			}
			end := -1
			for j := i + 2; j < len(s); j++ {
				if s[j] == closer {
					end = j
					break
				}
			}
			if end > 0 {
				name := s[i+2 : end]
				if val, ok := bf.Variables[name]; ok {
					result.WriteString(val)
				} else if val := os.Getenv(name); val != "" {
					result.WriteString(val)
				} else {
					// Keep original if not found
					result.WriteString(s[i : end+1])
				}
				i = end + 1
				continue
			}
		}
		result.WriteByte(s[i])
		i++
	}
	return result.String()
}

// ============================================================================
// Dependency Resolution & Execution
// ============================================================================

// resolveDependencies returns targets in execution order (topological sort)
func (bf *Buildfile) resolveDependencies(targetName string) ([]*Target, error) {
	visited := make(map[string]bool)
	inStack := make(map[string]bool)

	result, err := bf.depVisit(targetName, visited, inStack, nil)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (bf *Buildfile) depVisit(name string, visited map[string]bool, inStack map[string]bool, result []*Target) ([]*Target, error) {
	if inStack[name] {
		return result, fmt.Errorf("circular dependency detected involving %q", name)
	}
	if visited[name] {
		return result, nil
	}

	target, ok := bf.Targets[name]
	if !ok {
		return result, fmt.Errorf("target %q not found", name)
	}

	inStack[name] = true

	di := 0
	for di < len(target.Dependencies) {
		dep := target.Dependencies[di]
		var err error
		result, err = bf.depVisit(dep, visited, inStack, result)
		if err != nil {
			return result, err
		}
		di = di + 1
	}

	inStack[name] = false
	visited[name] = true
	result = append(result, target)
	return result, nil
}

// shouldRunOnPlatform checks if a target should run on the current platform
func (target *Target) shouldRunOnPlatform() bool {
	if len(target.Platforms) == 0 {
		return true
	}

	currentPlatform := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
	for _, p := range target.Platforms {
		if p == currentPlatform || p == runtime.GOOS {
			return true
		}
	}
	return false
}

// checkRequires checks if the current OS is in the requires list
func (target *Target) checkRequires() error {
	if len(target.Requires) == 0 {
		return nil
	}

	for _, req := range target.Requires {
		if req == runtime.GOOS {
			return nil
		}
	}

	return fmt.Errorf("target %q requires one of %v, but running on %s", target.Name, target.Requires, runtime.GOOS)
}

// Executor handles running targets
type Executor struct {
	Buildfile    *Buildfile
	DryRun       bool
	ExtraArgs    []string // Arguments passed after --
	builtOutputs map[string]buildOutput
}

// NewExecutor creates a new executor
func NewExecutor(bf *Buildfile, dryRun bool, extraArgs []string) *Executor {
	return &Executor{
		Buildfile:    bf,
		DryRun:       dryRun,
		ExtraArgs:    extraArgs,
		builtOutputs: make(map[string]buildOutput),
	}
}

// Run executes a target and its dependencies
func (e *Executor) Run(targetName string) error {
	targets, err := e.Buildfile.resolveDependencies(targetName)
	if err != nil {
		return err
	}

	for _, target := range targets {
		if err := e.executeTarget(target); err != nil {
			return err
		}
	}

	return nil
}

// executeTarget runs a single target
func (e *Executor) executeTarget(target *Target) error {
	// Check platform filter from target header
	if !target.shouldRunOnPlatform() {
		if !e.DryRun {
			fmt.Printf("skipping target %q (not for current platform)\n", target.Name)
		}
		return nil
	}

	// Check requires directive
	if err := target.checkRequires(); err != nil {
		return err
	}

	fmt.Printf("=== %s ===\n", target.Name)

	for _, cmd := range target.Commands {
		if err := e.executeCommand(cmd, target); err != nil {
			return fmt.Errorf("target %s: %w", target.Name, err)
		}
	}

	return nil
}

// executeCommand runs a single command
func (e *Executor) executeCommand(cmd Command, target *Target) error {
	// Check platform conditional
	if cmd.Platform != "" && cmd.Platform != runtime.GOOS {
		return nil
	}

	// Expand variables in arguments
	args := make([]string, len(cmd.Args))
	for i, arg := range cmd.Args {
		args[i] = e.Buildfile.expandVariables(arg)
	}

	if e.DryRun {
		if cmd.Platform != "" {
			fmt.Printf("  [@%s] %s %s\n", cmd.Platform, cmd.Type, strings.Join(args, " "))
		} else {
			fmt.Printf("  %s %s\n", cmd.Type, strings.Join(args, " "))
		}
		return nil
	}

	switch cmd.Type {
	case "gobuild":
		return e.handleGoBuild(args, target)
	case "fullcompiler":
		return e.handleFullCompiler(args)
	case "run":
		return e.handleRun(args)
	case "copy":
		return e.handleCopy(args)
	case "mkdir":
		return e.handleMkdir(args)
	case "rm":
		return e.handleRm(args)
	case "sh":
		return e.handleSh(args)
	default:
		return fmt.Errorf("unknown command type %q", cmd.Type)
	}
}

func parseTarget(target string) (string, string, error) {
	parts := strings.Split(target, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid target %q (expected os/arch)", target)
	}
	return parts[0], parts[1], nil
}

func isWSL() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if os.Getenv("WSL_INTEROP") != "" || os.Getenv("WSL_DISTRO_NAME") != "" {
		return true
	}
	paths := []string{"/proc/version", "/proc/sys/kernel/osrelease"}
	for _, p := range paths {
		if b, err := os.ReadFile(p); err == nil {
			if strings.Contains(strings.ToLower(string(b)), "microsoft") {
				return true
			}
		}
	}
	return false
}

func wslPathToWindows(path string) (string, error) {
	if !filepath.IsAbs(path) {
		if cwd, err := os.Getwd(); err == nil {
			path = filepath.Join(cwd, path)
		}
	}
	out, err := exec.Command("wslpath", "-m", path).Output()
	if err != nil {
		return "", fmt.Errorf("wslpath -m %s failed: %w", path, err)
	}
	return trimCommandOutput(out), nil
}

func quoteForCmd(arg string) string {
	if strings.ContainsAny(arg, " \t") {
		return `"` + arg + `"`
	}
	return arg
}

func trimCommandOutput(out []byte) string {
	return strings.TrimRight(string(out), "\r\n")
}

func detectRTGCompilerPath() (string, error) {
	if p := os.Getenv("RTG_COMPILER"); p != "" {
		return p, nil
	}
	candidates := []string{"./build/rtg"}
	if runtime.GOOS == "windows" {
		candidates = []string{"./build/rtg.exe", "./build/rtg"}
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("could not find RTG compiler; set RTG_COMPILER or build ./build/rtg")
}

func (e *Executor) runAndCapture(name string, args ...string) (string, error) {
	fmt.Fprintf(os.Stderr, "running %s %s\n", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("%s %s failed: %s", name, strings.Join(args, " "), trimCommandOutput(ee.Stderr))
		}
		return "", err
	}
	return trimCommandOutput(out), nil
}

func listGoFilesInDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".go") {
			out = append(out, filepath.Join(dir, name))
		}
	}
	sort.Strings(out)
	return out, nil
}

func fileExt(path string) string {
	lastSep := -1
	lastDot := -1
	i := 0
	for i < len(path) {
		if path[i] == '/' || path[i] == '\\' {
			lastSep = i
		} else if path[i] == '.' {
			lastDot = i
		}
		i++
	}
	if lastDot <= lastSep {
		return ""
	}
	return path[lastDot:len(path)]
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	i := 0
	for i < len(a) {
		aa := a[i]
		bb := b[i]
		if aa >= 'A' && aa <= 'Z' {
			aa = aa + ('a' - 'A')
		}
		if bb >= 'A' && bb <= 'Z' {
			bb = bb + ('a' - 'A')
		}
		if aa != bb {
			return false
		}
		i++
	}
	return true
}

// handleFullCompiler runs the top-level fullcompiler suite for a backend.
// Usage: fullcompiler <rtg|c|wasm> [rtg-target]
func (e *Executor) handleFullCompiler(args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return fmt.Errorf("fullcompiler requires 1 or 2 arguments: <rtg|c|wasm> [rtg-target]")
	}
	backend := args[0]
	switch backend {
	case "rtg", "c", "wasm":
	default:
		return fmt.Errorf("unsupported fullcompiler backend: %s", backend)
	}
	explicitTarget := ""
	if len(args) == 2 {
		explicitTarget = args[1]
		if backend != "rtg" {
			return fmt.Errorf("fullcompiler target override is only supported for rtg backend")
		}
	}

	tests, err := listGoFilesInDir("tests")
	if err != nil {
		return err
	}
	sort.Strings(tests)
	if len(tests) == 0 {
		return fmt.Errorf("no tests found under tests/*.go")
	}

	rtgCompiler, err := detectRTGCompilerPath()
	if err != nil {
		return err
	}

	rtgTarget := explicitTarget
	if rtgTarget == "" {
		rtgTarget = os.Getenv("RTG_TARGET")
	}
	targetOS := runtime.GOOS
	targetArch := runtime.GOARCH
	if rtgTarget != "" {
		targetOS, targetArch, err = parseTarget(rtgTarget)
		if err != nil {
			return err
		}
	}

	for _, testPath := range tests {
		base := filepath.Base(testPath)
		name := strings.TrimSuffix(base, fileExt(base))
		if backend == "wasm" && name == "iface_typeassert" {
			fmt.Printf("SKIP: %s/%s (known wasm32 type-assertion issue)\n", backend, name)
			continue
		}

		var got string
		switch backend {
		case "rtg":
			out := filepath.Join("build", "fullcompiler_"+name)
			if targetOS == "windows" {
				out += ".exe"
			}
			compileArgs := []string{}
			if rtgTarget != "" {
				compileArgs = append(compileArgs, "-T", rtgTarget)
			}
			compileArgs = append(compileArgs, testPath, "-o", out)
			if _, err := e.runAndCapture(rtgCompiler, compileArgs...); err != nil {
				return err
			}

			runBin := out
			runArgs := []string{}
			switch {
			case targetOS == runtime.GOOS:
				if targetArch != runtime.GOARCH {
					allow32On64 := runtime.GOARCH == "amd64" && targetArch == "386" &&
						(runtime.GOOS == "linux" || runtime.GOOS == "windows")
					if !allow32On64 {
						return fmt.Errorf("cannot execute %s/%s binary on %s/%s host: %s", targetOS, targetArch, runtime.GOOS, runtime.GOARCH, out)
					}
				}
			case runtime.GOOS == "linux" && targetOS == "windows" && targetArch == "386":
				runBin = "wine"
				runArgs = append(runArgs, out)
			case runtime.GOOS == "linux" && targetOS == "windows" && targetArch == "arm64":
				if !isWSL() {
					return fmt.Errorf("cannot execute %s/%s binary on %s/%s host: %s", targetOS, targetArch, runtime.GOOS, runtime.GOARCH, out)
				}
				winOut, err := wslPathToWindows(out)
				if err != nil {
					return err
				}
				runBin = "cmd.exe"
				runArgs = []string{"/c", quoteForCmd(winOut)}
			default:
				return fmt.Errorf("cannot execute %s/%s binary on %s/%s host: %s", targetOS, targetArch, runtime.GOOS, runtime.GOARCH, out)
			}
			got, err = e.runAndCapture(runBin, runArgs...)
			if err != nil {
				return err
			}
		case "c":
			csrc := filepath.Join("build", "fullcompiler_"+name+".c")
			out := filepath.Join("build", "fullcompiler_c_"+name)
			if runtime.GOOS == "windows" {
				out += ".exe"
			}
			if _, err := e.runAndCapture(rtgCompiler, "-T", "c/64", testPath, "-o", csrc); err != nil {
				return err
			}
			cc := os.Getenv("CC")
			if cc == "" {
				cc = "cc"
			}
			if _, err := e.runAndCapture(cc, csrc, "-o", out); err != nil {
				return err
			}
			got, err = e.runAndCapture(out)
			if err != nil {
				return err
			}
		case "wasm":
			wasmOut := filepath.Join("build", "fullcompiler_"+name+".wasm")
			if _, err := e.runAndCapture(rtgCompiler, "-T", "wasi/wasm32", testPath, "-o", wasmOut); err != nil {
				return err
			}
			got, err = e.runAndCapture("wasmtime", "--dir=.", wasmOut)
			if err != nil {
				return err
			}
		}

		if got != "PASS" {
			return fmt.Errorf("FAIL: %s/%s expected %q got %q", backend, name, "PASS", got)
		}
		fmt.Printf("PASS: %s/%s\n", backend, name)
	}

	return nil
}

// ============================================================================
// Command Handlers
// ============================================================================

// handleGoBuild handles the gobuild command
func (e *Executor) handleGoBuild(args []string, target *Target) error {
	if len(args) < 1 {
		return fmt.Errorf("gobuild requires a package argument")
	}

	opts := buildOptions{
		Package: args[0],
		Build:   crossBuild{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH},
	}

	// Parse flags
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "-o":
			if i+1 >= len(args) {
				return fmt.Errorf("-o requires an argument")
			}
			i++
			opts.OutputName = filepath.Base(args[i])
			// Handle output directory
			if dir := filepath.Dir(args[i]); dir != "." {
				opts.OutputDir = dir
			}
		case "-os":
			if i+1 >= len(args) {
				return fmt.Errorf("-os requires an argument")
			}
			i++
			opts.Build.GOOS = args[i]
		case "-arch":
			if i+1 >= len(args) {
				return fmt.Errorf("-arch requires an argument")
			}
			i++
			opts.Build.GOARCH = args[i]
		case "-tags":
			if i+1 >= len(args) {
				return fmt.Errorf("-tags requires an argument")
			}
			i++
			opts.Tags = strings.Split(args[i], ",")
		case "-cgo":
			opts.CgoEnabled = true
		case "-shared":
			opts.BuildShared = true
		case "-appname":
			if i+1 >= len(args) {
				return fmt.Errorf("-appname requires an argument")
			}
			i++
			opts.ApplicationName = args[i]
		default:
			return fmt.Errorf("unknown gobuild flag: %s", args[i])
		}
	}

	// Default output name from package
	if opts.OutputName == "" {
		opts.OutputName = filepath.Base(opts.Package)
	}

	out, err := goBuild(opts)
	if err != nil {
		return err
	}

	// Store the output for later use
	e.builtOutputs[target.Name] = out
	fmt.Printf("built %s\n", out.Path)

	return nil
}

// handleRun handles the run command
func (e *Executor) handleRun(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("run requires a binary argument")
	}

	binary := args[0]
	if runtime.GOOS == "windows" && !equalFoldASCII(fileExt(binary), ".exe") {
		// If caller passed a suffix-less path, prefer an existing ".exe" peer.
		if _, err := os.Stat(binary); err != nil {
			candidate := binary + ".exe"
			if _, statErr := os.Stat(candidate); statErr == nil {
				binary = candidate
			}
		}
	}
	runArgs := args[1:]

	// Append extra args from command line
	runArgs = append(runArgs, e.ExtraArgs...)

	out := buildOutput{Path: binary}

	return runBuildOutput(out, runArgs, runOptions{})
}

// handleCopy handles the copy command
func (e *Executor) handleCopy(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("copy requires exactly 2 arguments (src dst)")
	}

	return copyFile(args[1], args[0], 0644)
}

// handleMkdir handles the mkdir command
func (e *Executor) handleMkdir(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("mkdir requires at least 1 argument")
	}

	for _, dir := range args {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

// handleRm handles the rm command
func (e *Executor) handleRm(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("rm requires at least 1 argument")
	}

	for _, path := range args {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}

// shShell returns the shell and args to run a Unix-style sh command on the current OS.
// On Windows, prefers Git for Windows bash (avoids WSL stub which may have no distro).
func shShell(cmdStr string) (name string, args []string) {
	if runtime.GOOS != "windows" {
		return "/bin/sh", []string{"-c", cmdStr}
	}
	// On Windows, cmd.exe parses "./build/foo" as command "." with arg "build/foo",
	// and does not provide mv, cmp, etc. Prefer a real bash (Git for Windows).
	for _, candidate := range []string{
		os.Getenv("GIT_BASH"), // e.g. C:\Program Files\Git\bin\bash.exe
		"C:\\Program Files\\Git\\bin\\bash.exe",
		"C:\\Program Files (x86)\\Git\\bin\\bash.exe",
	} {
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate, []string{"-c", cmdStr}
		}
	}
	// Fallback: bash in PATH.
	return "bash", []string{"-c", cmdStr}
}

// normalizeWindowsGoBuildOutput appends ".exe" to "go build -o <path>" outputs on
// Windows when the output has no extension. This keeps sh-based Buildfile commands
// aligned with expected executable naming.
func normalizeWindowsGoBuildOutput(cmdStr string) string {
	if runtime.GOOS != "windows" {
		return cmdStr
	}

	parts := strings.Fields(cmdStr)
	if len(parts) < 4 {
		return cmdStr
	}

	goBuildIdx := -1
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "go" && parts[i+1] == "build" {
			goBuildIdx = i
			break
		}
	}
	if goBuildIdx == -1 {
		return cmdStr
	}

	for i := goBuildIdx + 2; i < len(parts); i++ {
		if strings.HasPrefix(parts[i], "-o=") {
			out := strings.TrimPrefix(parts[i], "-o=")
			if out != "" && fileExt(out) == "" {
				parts[i] = "-o=" + out + ".exe"
				return strings.Join(parts, " ")
			}
			return cmdStr
		}
		if parts[i] == "-o" && i+1 < len(parts) {
			out := parts[i+1]
			if out != "" && !strings.HasPrefix(out, "-") && fileExt(out) == "" {
				parts[i+1] = out + ".exe"
				return strings.Join(parts, " ")
			}
			return cmdStr
		}
	}

	return cmdStr
}

// handleSh handles the sh command
func (e *Executor) handleSh(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("sh requires a command")
	}

	cmdStr := strings.Join(args, " ")
	cmdStr = normalizeWindowsGoBuildOutput(cmdStr)
	name, shellArgs := shShell(cmdStr)
	fmt.Printf("running %s %s\n", name, strings.Join(shellArgs, " "))
	cmd := exec.Command(name, shellArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

// ============================================================================
// Build System Core (preserved from original)
// ============================================================================

type crossBuild struct {
	GOOS   string
	GOARCH string
}

func (cb crossBuild) IsNative() bool {
	return cb.GOOS == runtime.GOOS && cb.GOARCH == runtime.GOARCH
}

func (cb crossBuild) OutputName(name string) string {
	baseName := name
	if cb.GOOS == "windows" {
		// Avoid generating ".exe.exe" when caller already provides an extension.
		if ext := fileExt(baseName); equalFoldASCII(ext, ".exe") {
			baseName = strings.TrimSuffix(baseName, ext)
		}
	}

	if cb.IsNative() {
		if cb.GOOS == "windows" {
			return baseName + ".exe"
		}
		return baseName
	}
	if cb.GOOS == "windows" {
		return fmt.Sprintf("%s_%s_%s.exe", baseName, cb.GOOS, cb.GOARCH)
	}
	return fmt.Sprintf("%s_%s_%s", baseName, cb.GOOS, cb.GOARCH)
}

type buildOptions struct {
	Package         string
	ApplicationName string
	OutputName      string
	OutputDir       string
	CgoEnabled      bool
	Build           crossBuild
	Tags            []string
	BuildShared     bool
}

type buildOutput struct {
	Path string
}

func copyFile(dstPath, srcPath string, perm os.FileMode) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		src.Close()
		return fmt.Errorf("mkdir dst dir: %w", err)
	}

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		src.Close()
		return fmt.Errorf("open dst: %w", err)
	}

	if _, err := io.Copy(dst, src); err != nil {
		src.Close()
		dst.Close()
		return fmt.Errorf("copy: %w", err)
	}

	src.Close()
	dst.Close()
	return nil
}

func goBuild(opts buildOptions) (buildOutput, error) {
	outputDir := "build"
	if opts.OutputDir != "" {
		outputDir = opts.OutputDir
	}

	output := filepath.Join(outputDir, opts.Build.OutputName(opts.OutputName))

	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return buildOutput{}, fmt.Errorf("failed to create build directory: %w", err)
	}

	pkg := opts.Package

	env := os.Environ()
	env = append(env, "GOOS="+opts.Build.GOOS)
	env = append(env, "GOARCH="+opts.Build.GOARCH)
	if opts.CgoEnabled || opts.BuildShared {
		env = append(env, "CGO_ENABLED=1")
	} else {
		env = append(env, "CGO_ENABLED=0")
	}

	var args []string
	args = append(args, "go")
	args = append(args, "build")
	if opts.BuildShared {
		args = append(args, "-buildmode=c-shared")
	}
	args = append(args, "-o")
	args = append(args, output)

	if len(opts.Tags) > 0 {
		args = append(args, "-tags", strings.Join(opts.Tags, " "))
	}

	args = append(args, pkg)

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return buildOutput{}, fmt.Errorf("go build failed: %w", err)
	}

	return buildOutput{Path: output}, nil
}

type runOptions struct {
	CpuProfilePath string
	MemProfilePath string
}

func runBuildOutput(output buildOutput, args []string, opts runOptions) error {
	if runtime.GOOS == "darwin" && strings.HasSuffix(output.Path, ".app") {
		var openArgs []string
		openArgs = append(openArgs, "-n")
		openArgs = append(openArgs, output.Path)
		if len(args) > 0 {
			openArgs = append(openArgs, "--args")
			openArgs = append(openArgs, args...)
		}
		cmd := exec.Command("open", openArgs...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to run app bundle: %w", err)
		}
		return nil
	}

	if opts.CpuProfilePath != "" {
		var prefixed []string
		prefixed = append(prefixed, "-cpuprofile")
		prefixed = append(prefixed, opts.CpuProfilePath)
		args = append(prefixed, args...)
	}

	if opts.MemProfilePath != "" {
		var prefixed []string
		prefixed = append(prefixed, "-memprofile")
		prefixed = append(prefixed, opts.MemProfilePath)
		args = append(prefixed, args...)
	}

	cmd := exec.Command(output.Path, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run build output: %w", err)
	}

	return nil
}

func getVersionFromGit() string {
	if ref := os.Getenv("GITHUB_REF_NAME"); ref != "" && strings.HasPrefix(ref, "v") {
		return ref
	}

	cmd := exec.Command("git", "describe", "--tags", "--always")
	out, err := cmd.Output()
	if err == nil {
		version := strings.TrimSpace(string(out))
		if version != "" {
			return version
		}
	}

	return "dev"
}

// ============================================================================
// CLI Interface
// ============================================================================

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [options] [target] [-- args...]\n\nOptions:\n  -f <file>      Use specified Buildfile (default: tools/Buildfile)\n  --dry-run      Show what would be done without executing\n  --list         List all available targets\n  -h, --help     Show this help message\n\nArguments after -- are passed to 'run' commands.\n\nExamples:\n  %s                    Build default target\n  %s cc                 Build the cc target\n  %s bringup            Build and run bringup tests\n  %s --list             List all targets\n  %s cc -- --help       Build cc and pass --help to run commands\n", os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}

func main() {
	// Parse arguments manually to handle -- separator
	var buildfilePath string
	var dryRun bool
	var listTargets bool
	var showHelp bool
	var targetName string
	var extraArgs []string

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--" {
			extraArgs = args[i+1:]
			break
		}

		switch arg {
		case "-f":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "-f requires an argument\n")
				os.Exit(1)
			}
			i++
			buildfilePath = args[i]
		case "--dry-run":
			dryRun = true
		case "--list":
			listTargets = true
		case "-h", "--help":
			showHelp = true
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(os.Stderr, "unknown option: %s\n", arg)
				usage()
				os.Exit(1)
			}
			if targetName == "" {
				targetName = arg
			} else {
				fmt.Fprintf(os.Stderr, "unexpected argument: %s\n", arg)
				usage()
				os.Exit(1)
			}
		}
	}

	if showHelp {
		usage()
		os.Exit(0)
	}

	// Default buildfile path
	if buildfilePath == "" {
		buildfilePath = filepath.Join("tools", "Buildfile")
	}

	// Read and parse buildfile
	content, err := os.ReadFile(buildfilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read buildfile: %v\n", err)
		os.Exit(1)
	}

	bf, err := parseBuildfile(buildfilePath, content)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse buildfile: %v\n", err)
		os.Exit(1)
	}

	// List targets
	if listTargets {
		var names []string
		for name := range bf.Targets {
			names = append(names, name)
		}
		sort.Strings(names)

		fmt.Println("Available targets:")
		for _, name := range names {
			fmt.Printf("  %s\n", name)
		}
		os.Exit(0)
	}

	// Default target
	if targetName == "" {
		targetName = "default"
		if _, ok := bf.Targets["default"]; !ok {
			fmt.Fprintf(os.Stderr, "no target specified and no 'default' target found\n")
			usage()
			os.Exit(1)
		}
	}

	// Check target exists
	if _, ok := bf.Targets[targetName]; !ok {
		fmt.Fprintf(os.Stderr, "target %q not found\n", targetName)
		os.Exit(1)
	}

	// Execute
	executor := NewExecutor(bf, dryRun, extraArgs)
	if err := executor.Run(targetName); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
