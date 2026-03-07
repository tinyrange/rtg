package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	cfront "j5.nz/rtg/std/compiler/frontend/c"
)

type testCase struct {
	mode       string // lex, pp, or parse
	inputPath  string
	expectPath string
}

type skippedError struct {
	reason string
}

func (e skippedError) Error() string {
	return e.reason
}

func hostBinaryPath(base string) string {
	if runtime.GOOS == "windows" && filepath.Ext(base) == "" {
		return base + ".exe"
	}
	return base
}

func runCmdWithTimeout(cmd *exec.Cmd) error {
	return runCmdWithTimeoutExpect(cmd, 0)
}

func runCmdWithTimeoutExpect(cmd *exec.Cmd, expectedExit int) error {
	timeout := 10 * time.Second
	if runtime.GOOS == "windows" {
		timeout = 20 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("timed out after %s\n%s", timeout, string(out))
	}
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			return fmt.Errorf("%v\n%s", err, string(out))
		}
	}
	if exitCode != expectedExit {
		return fmt.Errorf("exit code %d (expected %d)\n%s", exitCode, expectedExit, string(out))
	}
	return nil
}

func unsupportedRunExecution() (bool, string) {
	if runtime.GOOS == "windows" && runtime.GOARCH == "arm64" {
		return true, "runtime execution is not supported on windows/arm64 for this suite"
	}
	return false, ""
}

func listCases(dir string, mode string, optional bool) ([]testCase, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if optional && os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".c") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var out []testCase
	expectExt := ".tokens"
	if mode == "parse" {
		expectExt = ".ast"
	}
	for _, name := range names {
		base := strings.TrimSuffix(name, ".c")
		expectPath := ""
		if mode != "run" {
			expectPath = filepath.Join(dir, base+expectExt)
		}
		out = append(out, testCase{
			mode:       mode,
			inputPath:  filepath.Join(dir, name),
			expectPath: expectPath,
		})
	}
	return out, nil
}

func serializeTokens(tokens []cfront.Token) string {
	var lines []string
	for _, tok := range tokens {
		if tok.Kind == cfront.TokEOF {
			continue
		}
		lines = append(lines, tok.Kind.String()+" "+strconv.Quote(tok.Text))
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func runLexCase(tc testCase) (string, error) {
	src, err := os.ReadFile(tc.inputPath)
	if err != nil {
		return "", err
	}
	lx := cfront.NewLexer(tc.inputPath, string(src))
	toks, err := lx.Tokenize()
	if err != nil {
		return "", err
	}
	return serializeTokens(toks), nil
}

func runPPCase(tc testCase) (string, error) {
	pp := cfront.NewPreprocessor(cfront.Options{
		IncludePaths: []string{filepath.Join("tests", "c", "include")},
	})
	toks, err := pp.ProcessFile(tc.inputPath)
	if err != nil {
		return "", err
	}
	return serializeTokens(toks), nil
}

func runCase(tc testCase) (string, error) {
	if tc.mode == "lex" {
		return runLexCase(tc)
	}
	if tc.mode == "run" {
		return runRunCase(tc)
	}
	if tc.mode == "parse" {
		return runParseCase(tc)
	}
	return runPPCase(tc)
}

func runRunCase(tc testCase) (string, error) {
	if skip, reason := unsupportedRunExecution(); skip {
		return "", skippedError{reason: fmt.Sprintf("skipping %s: %s", tc.inputPath, reason)}
	}

	base := strings.TrimSuffix(filepath.Base(tc.inputPath), ".c")
	expectedExit := 0
	exitPath := filepath.Join(filepath.Dir(tc.inputPath), base+".exit")
	if raw, err := os.ReadFile(exitPath); err == nil {
		v, perr := strconv.Atoi(strings.TrimSpace(string(raw)))
		if perr != nil {
			return "", fmt.Errorf("invalid exit expectation in %s: %v", exitPath, perr)
		}
		expectedExit = v
	} else if !os.IsNotExist(err) {
		return "", err
	}

	compilerPath := filepath.Join(".", hostBinaryPath(filepath.Join("build", "rtg")))
	outStem := filepath.Join("build", "cfront_run_"+base)
	useCBackend := strings.HasSuffix(base, "_cbackend")
	if useCBackend {
		srcPath := outStem + ".c"
		binPath := hostBinaryPath(outStem + ".bin")
		defer os.Remove(srcPath)
		defer os.Remove(binPath)

		if err := runCmdWithTimeout(exec.Command(compilerPath, "-x", "c99", "-T", "c/64", "-o", srcPath, tc.inputPath)); err != nil {
			return "", err
		}
		cc := os.Getenv("CC")
		if cc == "" {
			cc = "cc"
		}
		if _, err := exec.LookPath(cc); err != nil {
			return "", skippedError{reason: fmt.Sprintf("skipping %s: C compiler %q not found in PATH", tc.inputPath, cc)}
		}
		if err := runCmdWithTimeout(exec.Command(cc, "-x", "c", srcPath, "-o", binPath)); err != nil {
			return "", err
		}
		if err := runCmdWithTimeoutExpect(exec.Command(filepath.Join(".", binPath)), expectedExit); err != nil {
			return "", err
		}
		return "", nil
	}

	outPath := hostBinaryPath(outStem)
	defer os.Remove(outPath)
	if err := runCmdWithTimeoutExpect(exec.Command(compilerPath, "-x", "c99", "-run", "-o", outPath, tc.inputPath), expectedExit); err != nil {
		return "", err
	}
	return "", nil
}

func runExitCodePropagationCase() error {
	if skip, reason := unsupportedRunExecution(); skip {
		return skippedError{reason: reason}
	}

	compilerPath := filepath.Join(".", hostBinaryPath(filepath.Join("build", "rtg")))
	srcPath := filepath.Join("build", "cfront_exit_code_check.c")
	outPath := hostBinaryPath(filepath.Join("build", "cfront_exit_code_check.bin"))
	defer os.Remove(srcPath)
	defer os.Remove(outPath)

	src := "int main(void) { return 7; }\n"
	if err := os.WriteFile(srcPath, []byte(src), 0644); err != nil {
		return err
	}
	return runCmdWithTimeoutExpect(exec.Command(compilerPath, "-x", "c99", "-run", "-o", outPath, srcPath), 7)
}

func runParseCase(tc testCase) (string, error) {
	pp := cfront.NewPreprocessor(cfront.Options{
		IncludePaths: []string{filepath.Join("tests", "c", "include")},
	})
	toks, err := pp.ProcessFile(tc.inputPath)
	if err != nil {
		return "", err
	}
	parser := cfront.NewParser(toks)
	tu := parser.ParseTranslationUnit()
	if len(parser.Errors()) > 0 {
		return "", fmt.Errorf("%s", strings.Join(parser.Errors(), "\n"))
	}
	return cfront.FormatNode(tu), nil
}

func main() {
	update := flag.Bool("update", false, "rewrite expected token snapshots")
	flag.Parse()

	lexCases, err := listCases(filepath.Join("tests", "c", "lex"), "lex", true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cfronttest: %v\n", err)
		os.Exit(1)
	}
	ppCases, err := listCases(filepath.Join("tests", "c", "pp"), "pp", true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cfronttest: %v\n", err)
		os.Exit(1)
	}
	parseCases, err := listCases(filepath.Join("tests", "c", "parse"), "parse", true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cfronttest: %v\n", err)
		os.Exit(1)
	}
	runCases, err := listCases(filepath.Join("tests", "c", "run"), "run", false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cfronttest: %v\n", err)
		os.Exit(1)
	}
	cases := append(lexCases, ppCases...)
	cases = append(cases, parseCases...)
	cases = append(cases, runCases...)
	if len(cases) == 0 {
		fmt.Fprintf(os.Stderr, "cfronttest: no cases found\n")
		os.Exit(1)
	}

	failed := false
	for _, tc := range cases {
		got, err := runCase(tc)
		if err != nil {
			if skip, ok := err.(skippedError); ok {
				fmt.Printf("SKIP %s %s: %s\n", tc.mode, tc.inputPath, skip.reason)
				continue
			}
			failed = true
			fmt.Fprintf(os.Stderr, "FAIL %s %s: %v\n", tc.mode, tc.inputPath, err)
			continue
		}
		if tc.mode == "run" {
			fmt.Printf("PASS %s %s\n", tc.mode, tc.inputPath)
			continue
		}
		if *update {
			if err := os.WriteFile(tc.expectPath, []byte(got), 0644); err != nil {
				failed = true
				fmt.Fprintf(os.Stderr, "FAIL write %s: %v\n", tc.expectPath, err)
				continue
			}
			fmt.Printf("UPDATE %s\n", tc.expectPath)
			continue
		}
		wantBytes, err := os.ReadFile(tc.expectPath)
		if err != nil {
			failed = true
			fmt.Fprintf(os.Stderr, "FAIL read %s: %v\n", tc.expectPath, err)
			continue
		}
		want := string(wantBytes)
		if got != want {
			failed = true
			fmt.Fprintf(os.Stderr, "FAIL %s %s\n", tc.mode, tc.inputPath)
			fmt.Fprintf(os.Stderr, "--- want (%s) ---\n%s", tc.expectPath, want)
			fmt.Fprintf(os.Stderr, "--- got ---\n%s", got)
			continue
		}
		fmt.Printf("PASS %s %s\n", tc.mode, tc.inputPath)
	}
	if err := runExitCodePropagationCase(); err != nil {
		if skip, ok := err.(skippedError); ok {
			fmt.Printf("SKIP run exit_code_propagation: %s\n", skip.reason)
		} else {
			failed = true
			fmt.Fprintf(os.Stderr, "FAIL run exit_code_propagation: %v\n", err)
		}
	} else {
		fmt.Printf("PASS run exit_code_propagation\n")
	}

	if failed {
		os.Exit(1)
	}
}
