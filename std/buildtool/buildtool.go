package buildtool

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type command struct {
	LineNum  int
	Platform string
	Script   string
}

type target struct {
	Name      string
	Platforms []string
	Requires  []string
	Deps      []string
	Commands  []command
	LineNum   int
}

type buildfile struct {
	Path      string
	Dir       string
	Variables map[string]string
	Targets   map[string]*target
}

func RunCLI(args []string) int {
	listOnly := false
	var filtered []string
	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			printHelp(os.Args[0], os.Stdout)
			return 0
		case "--list":
			listOnly = true
		default:
			filtered = append(filtered, arg)
		}
	}
	if len(filtered) == 0 {
		printHelp(os.Args[0], os.Stderr)
		return 1
	}
	path := filtered[0]
	targets := filtered[1:]
	if err := RunFile(path, targets, listOnly); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func RunFile(path string, targets []string, listOnly bool) error {
	bf, err := parseFile(path)
	if err != nil {
		return err
	}
	if listOnly {
		names := make([]string, 0, len(bf.Targets))
		for name := range bf.Targets {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Println(name)
		}
		return nil
	}
	if len(targets) == 0 {
		targets = []string{"default"}
	}
	ex := executor{
		bf:       bf,
		executed: make(map[string]bool),
	}
	for _, name := range targets {
		if err := ex.runTarget(name); err != nil {
			return err
		}
	}
	return nil
}

type executor struct {
	bf       *buildfile
	executed map[string]bool
	stack    map[string]bool
}

func (e *executor) runTarget(name string) error {
	if e.executed[name] {
		return nil
	}
	if e.stack == nil {
		e.stack = make(map[string]bool)
	}
	if e.stack[name] {
		return fmt.Errorf("circular dependency detected involving %q", name)
	}
	t := e.bf.Targets[name]
	if t == nil {
		return fmt.Errorf("target %q not found", name)
	}
	e.stack[name] = true
	for _, dep := range t.Deps {
		if err := e.runTarget(dep); err != nil {
			return err
		}
	}
	delete(e.stack, name)
	if e.executed[name] {
		return nil
	}
	if !targetRunsOnPlatform(t.Platforms) {
		fmt.Printf("skipping target %q (not for current platform)\n", name)
		e.executed[name] = true
		return nil
	}
	if len(t.Requires) > 0 && !containsString(t.Requires, runtime.GOOS) {
		return fmt.Errorf("target %q requires one of %v, but running on %s", name, t.Requires, runtime.GOOS)
	}
	fmt.Printf("=== %s ===\n", name)
	for _, cmd := range t.Commands {
		if cmd.Platform != "" && cmd.Platform != runtime.GOOS {
			continue
		}
		script := expandVariables(cmd.Script, e.bf.Variables)
		if err := runShell(script, e.bf.Dir); err != nil {
			return fmt.Errorf("%s:%d: %w", e.bf.Path, cmd.LineNum, err)
		}
	}
	e.executed[name] = true
	return nil
}

func parseFile(path string) (*buildfile, error) {
	abs, err := absPath(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	bf := &buildfile{
		Path:      abs,
		Dir:       filepath.Dir(abs),
		Variables: defaultVariables(abs),
		Targets:   make(map[string]*target),
	}
	lines := strings.Split(string(data), "\n")
	var current *target
	var continued string
	continuedLine := 0
	for idx, raw := range lines {
		lineNum := idx + 1
		line := strings.TrimRight(raw, " \t\r")
		if strings.HasSuffix(line, "\\") {
			if continued == "" {
				continuedLine = lineNum
			}
			continued += strings.TrimSuffix(line, "\\") + " "
			continue
		}
		if continued != "" {
			line = continued + line
			lineNum = continuedLine
			continued = ""
			continuedLine = 0
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		indented := line[0] == ' ' || line[0] == '\t'
		if indented {
			if current == nil {
				return nil, fmt.Errorf("%s:%d: command without target", abs, lineNum)
			}
			cmd, requires, err := parseCommand(strings.TrimSpace(line), lineNum)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %w", abs, lineNum, err)
			}
			if len(requires) > 0 {
				if len(current.Commands) > 0 {
					return nil, fmt.Errorf("%s:%d: requires must appear before commands", abs, lineNum)
				}
				current.Requires = append([]string{}, requires...)
				continue
			}
			current.Commands = append(current.Commands, cmd)
			continue
		}
		current = nil
		line = stripLineComment(line)
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if eq := strings.Index(trimmed, "="); eq > 0 {
			if colon := strings.Index(trimmed, ":"); colon < 0 || eq < colon {
				name := strings.TrimSpace(trimmed[:eq])
				if isIdent(name) {
					bf.Variables[name] = strings.TrimSpace(trimmed[eq+1:])
					continue
				}
			}
		}
		colon := strings.Index(trimmed, ":")
		if colon <= 0 {
			return nil, fmt.Errorf("%s:%d: unexpected line: %s", abs, lineNum, trimmed)
		}
		namePart := strings.TrimSpace(trimmed[:colon])
		depsPart := strings.TrimSpace(trimmed[colon+1:])
		name, plats, err := parseTargetName(namePart)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", abs, lineNum, err)
		}
		t := &target{
			Name:      name,
			Platforms: plats,
			Deps:      fieldsNonEmpty(depsPart),
			LineNum:   lineNum,
		}
		bf.Targets[name] = t
		current = t
	}
	return bf, nil
}

func defaultVariables(path string) map[string]string {
	vars := map[string]string{
		"GOOS":       runtime.GOOS,
		"GOARCH":     runtime.GOARCH,
		"BUILD_FILE": path,
		"BUILD_DIR":  filepath.Dir(path),
		"EXE":        "",
		"SHLIB_EXT":  ".so",
	}
	if runtime.GOOS == "windows" {
		vars["EXE"] = ".exe"
		vars["SHLIB_EXT"] = ".dll"
	} else if runtime.GOOS == "darwin" {
		vars["SHLIB_EXT"] = ".dylib"
	}
	return vars
}

func parseTargetName(s string) (string, []string, error) {
	if i := strings.Index(s, "["); i >= 0 {
		if !strings.HasSuffix(s, "]") {
			return "", nil, fmt.Errorf("unclosed platform specifier")
		}
		name := strings.TrimSpace(s[:i])
		if !isIdent(name) {
			return "", nil, fmt.Errorf("invalid target name %q", name)
		}
		return name, fieldsNonEmpty(s[i+1 : len(s)-1]), nil
	}
	if !isIdent(s) {
		return "", nil, fmt.Errorf("invalid target name %q", s)
	}
	return s, nil, nil
}

func parseCommand(line string, lineNum int) (command, []string, error) {
	cmd := command{LineNum: lineNum}
	if strings.HasPrefix(line, "@") {
		space := firstSpace(line)
		if space < 0 {
			return cmd, nil, fmt.Errorf("platform conditional without command")
		}
		cmd.Platform = strings.TrimPrefix(line[:space], "@")
		line = strings.TrimSpace(line[space+1:])
	}
	if strings.HasPrefix(line, "requires ") {
		return cmd, fieldsNonEmpty(strings.TrimSpace(line[len("requires "):])), nil
	}
	if strings.HasPrefix(line, "sh ") {
		line = strings.TrimSpace(line[len("sh "):])
	}
	if line == "" {
		return cmd, nil, fmt.Errorf("empty command")
	}
	cmd.Script = line
	return cmd, nil, nil
}

func runShell(script string, dir string) error {
	if dir != "" && runtime.GOOS != "windows" {
		script = "cd " + shellQuote(dir) + " && " + script
	} else if dir != "" {
		script = "cd /d " + shellQuoteWindows(dir) + " && " + script
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", script)
	} else {
		cmd = exec.Command("/bin/sh", "-c", script)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	fmt.Printf("running %s\n", script)
	return cmd.Run()
}

func expandVariables(s string, vars map[string]string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '$' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		opener := s[i+1]
		if opener != '(' && opener != '{' {
			b.WriteByte(s[i])
			continue
		}
		closer := byte(')')
		if opener == '{' {
			closer = '}'
		}
		end := -1
		for j := i + 2; j < len(s); j++ {
			if s[j] == closer {
				end = j
				break
			}
		}
		if end < 0 {
			b.WriteByte(s[i])
			continue
		}
		name := s[i+2 : end]
		if val, ok := vars[name]; ok {
			b.WriteString(val)
		} else {
			b.WriteString(os.Getenv(name))
		}
		i = end
	}
	return b.String()
}

func fieldsNonEmpty(s string) []string {
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return nil
	}
	return parts
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func targetRunsOnPlatform(platforms []string) bool {
	if len(platforms) == 0 {
		return true
	}
	full := runtime.GOOS + "/" + runtime.GOARCH
	for _, p := range platforms {
		if p == runtime.GOOS || p == full {
			return true
		}
	}
	return false
}

func stripLineComment(line string) string {
	inSingle := false
	inDouble := false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return strings.TrimRight(line[:i], " \t")
			}
		}
	}
	return line
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if i == 0 {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_') {
				return false
			}
			continue
		}
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func absPath(path string) (string, error) {
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "\\") || (len(path) >= 2 && path[1] == ':') {
		return path, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, path), nil
}

func firstSpace(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' {
			return i
		}
	}
	return -1
}

func printHelp(program string, out *os.File) {
	fmt.Fprintf(out, "Usage: %s [--list] <buildfile> [target...]\n", program)
}

func shellQuote(s string) string {
	return "'" + replaceAll(s, "'", "'\"'\"'") + "'"
}

func shellQuoteWindows(s string) string {
	return `"` + replaceAll(s, `"`, `""`) + `"`
}

func replaceAll(s string, old string, new string) string {
	if old == "" {
		return s
	}
	var b strings.Builder
	i := 0
	for i < len(s) {
		if i+len(old) <= len(s) && s[i:i+len(old)] == old {
			b.WriteString(new)
			i += len(old)
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
