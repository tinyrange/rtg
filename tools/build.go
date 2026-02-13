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
		"gobuild":  true,
		"run":      true,
		"copy":     true,
		"mkdir":    true,
		"rm":       true,
		"sh":       true,
		"requires": true,
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

// handleSh handles the sh command
func (e *Executor) handleSh(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("sh requires a command")
	}

	// Join args back into a command string
	cmdStr := strings.Join(args, " ")

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", cmdStr)
	} else {
		cmd = exec.Command("/bin/sh", "-c", cmdStr)
	}

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
	suffix := ""
	if cb.GOOS == "windows" {
		suffix = ".exe"
	}

	if cb.IsNative() {
		return fmt.Sprintf("%s%s", name, suffix)
	} else {
		return fmt.Sprintf("%s_%s_%s%s", name, cb.GOOS, cb.GOARCH, suffix)
	}
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
