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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("timed out after 2s\n%s", string(out))
	}
	if err != nil {
		return fmt.Errorf("%v\n%s", err, string(out))
	}
	return nil
}

func listCases(dir string, mode string) ([]testCase, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
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
	base := strings.TrimSuffix(filepath.Base(tc.inputPath), ".c")
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
		if err := runCmdWithTimeout(exec.Command(filepath.Join(".", binPath))); err != nil {
			return "", err
		}
		return "", nil
	}

	outPath := hostBinaryPath(outStem)
	defer os.Remove(outPath)
	if err := runCmdWithTimeout(exec.Command(compilerPath, "-x", "c99", "-run", "-o", outPath, tc.inputPath)); err != nil {
		return "", err
	}
	return "", nil
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

	lexCases, err := listCases(filepath.Join("tests", "c", "lex"), "lex")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cfronttest: %v\n", err)
		os.Exit(1)
	}
	ppCases, err := listCases(filepath.Join("tests", "c", "pp"), "pp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cfronttest: %v\n", err)
		os.Exit(1)
	}
	parseCases, err := listCases(filepath.Join("tests", "c", "parse"), "parse")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cfronttest: %v\n", err)
		os.Exit(1)
	}
	runCases, err := listCases(filepath.Join("tests", "c", "run"), "run")
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

	if failed {
		os.Exit(1)
	}
}
