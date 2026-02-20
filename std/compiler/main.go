//go:build !no_frontend

package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

var stdlibIncludePaths []string
var stdlibIncludeExplicit bool
var stdlibIncludeEmbedded bool

// Temp file paths for -run mode; cleaned up on exit.
var runTmpSrc string
var runTmpBin string

func runCleanup() {
	if runTmpBin != "" {
		os.RemoveAll(runTmpBin)
	}
	if runTmpSrc != "" {
		os.RemoveAll(runTmpSrc)
	}
}

func tempDirPath() string {
	tmpDir := os.Getenv("TMPDIR") // macOS, some Linux
	if tmpDir == "" {
		tmpDir = os.Getenv("TEMP") // Windows
	}
	if tmpDir == "" {
		tmpDir = os.Getenv("TMP") // Windows fallback
	}
	if tmpDir == "" {
		tmpDir = "/tmp" // Linux/Unix fallback
	}
	return tmpDir
}

func pathSep() string {
	if runtime.GOOS == "windows" {
		return "\\"
	}
	return "/"
}

func readStdinSourceToTemp() error {
	if runTmpSrc == "" {
		pid := fmt.Sprintf("%d", os.Getpid())
		runTmpSrc = tempDirPath() + pathSep() + "rtg-run-" + pid + ".go"
	}
	var src []byte
	buf := make([]byte, 4096)
	for {
		n, _ := os.Stdin.Read(buf)
		if n > 0 {
			src = append(src, buf[0:n]...)
		}
		if n == 0 {
			break
		}
	}
	if len(src) == 0 {
		return fmt.Errorf("no input on stdin")
	}
	return os.WriteFile(runTmpSrc, src, 0644)
}

func main() {
	if len(os.Args) < 2 {
		printHelp(os.Args[0], os.Stderr)
		os.Exit(1)
	}

	outputPath := "output"
	var entryFiles []string
	var extraTags string
	var parseOnly bool
	var buildTagsPath string
	var emitIRBinaryPath string
	var fromIRBinaryPath string
	var extractStdlibDest string
	var runMode bool
	var stdinInput bool
	var programArgs []string
	parsedOpts := currentDriverOptions()
	i := 1
	for i < len(os.Args) {
		if os.Args[i] == "-h" || os.Args[i] == "--help" {
			printHelp(os.Args[0], os.Stdout)
			os.Exit(0)
		} else if os.Args[i] == "-run" {
			runMode = true
			i = i + 1
		} else if os.Args[i] == "-o" && i+1 < len(os.Args) {
			outputPath = os.Args[i+1]
			i = i + 2
		} else if os.Args[i] == "-T" && i+1 < len(os.Args) {
			var errMsg string
			parsedOpts.Target, errMsg = parseTargetFlag(parsedOpts.Target, os.Args[i+1])
			if errMsg != "" {
				fmt.Fprintf(os.Stderr, "%s\n", errMsg)
				os.Exit(1)
			}
			i = i + 2
		} else if os.Args[i] == "-size-analysis" && i+1 < len(os.Args) {
			sizeAnalysisPath = os.Args[i+1]
			i = i + 2
		} else if os.Args[i] == "-parse-only" {
			parseOnly = true
			i = i + 1
		} else if (os.Args[i] == "-emit-ir-binary" || os.Args[i] == "-from-ir-binary") && i+1 < len(os.Args) {
			if !irBinaryEnabled {
				fmt.Fprintf(os.Stderr, "IR binary I/O is experimental; rebuild with -tags exp_ir_binary\n")
				runCleanup()
				os.Exit(1)
			}
			if os.Args[i] == "-emit-ir-binary" {
				emitIRBinaryPath = os.Args[i+1]
			} else {
				fromIRBinaryPath = os.Args[i+1]
			}
			i = i + 2
		} else if os.Args[i] == "-list-build-tags" && i+1 < len(os.Args) {
			buildTagsPath = os.Args[i+1]
			i = i + 2
		} else if os.Args[i] == "-tags" && i+1 < len(os.Args) {
			extraTags = os.Args[i+1]
			i = i + 2
		} else if os.Args[i] == "-include" && i+1 < len(os.Args) {
			val := normalizePath(os.Args[i+1])
			if !stdlibIncludeExplicit {
				stdlibIncludeExplicit = true
				stdlibIncludeEmbedded = false
			}
			if val == "-" {
				stdlibIncludeEmbedded = true
			} else if val != "" {
				stdlibIncludePaths = appendUnique(stdlibIncludePaths, trimTrailingSlash(val))
			}
			i = i + 2
		} else if os.Args[i] == "-extract-stdlib" && i+1 < len(os.Args) {
			extractStdlibDest = normalizePath(os.Args[i+1])
			i = i + 2
		} else if os.Args[i] == "-debug" {
			parsedOpts.Debug = true
			i = i + 1
		} else if os.Args[i] == "-strip" || os.Args[i] == "-s" {
			parsedOpts.StripBinary = true
			i = i + 1
		} else if os.Args[i] == "--" {
			i = i + 1
			for i < len(os.Args) {
				programArgs = append(programArgs, os.Args[i])
				i = i + 1
			}
		} else if os.Args[i] == "-" {
			stdinInput = true
			i = i + 1
		} else {
			entryFiles = append(entryFiles, normalizePath(os.Args[i]))
			i = i + 1
		}
	}
	entryFiles, outputPath, err := prepareRuntimeInputs(entryFiles, fromIRBinaryPath, stdinInput, runMode, outputPath, parsedOpts.Target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		runCleanup()
		os.Exit(1)
	}

	showHelp, err := validateMainInputs(extractStdlibDest, fromIRBinaryPath, entryFiles)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		runCleanup()
		os.Exit(1)
	}
	if showHelp {
		printHelp(os.Args[0], os.Stderr)
		os.Exit(1)
	}

	// Build and apply driver options explicitly.
	opts := buildAndApplyDriverOptionsFrom(parsedOpts, extraTags, sizeAnalysisPath != "")

	// Initialize embedded std if available
	initEmbeddedStd()

	didExtractStdlib, err := handleExtractStdlibMode(extractStdlibDest, fromIRBinaryPath, entryFiles, runMode, stdinInput, parseOnly, emitIRBinaryPath, buildTagsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		runCleanup()
		os.Exit(1)
	}
	if didExtractStdlib {
		runCleanup()
		os.Exit(0)
	}

	irmod, frontendErrMsg, shouldExitNow, err := resolveIRModuleForMain(entryFiles, fromIRBinaryPath, buildTagsPath, parseOnly, emitIRBinaryPath, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		runCleanup()
		os.Exit(1)
	}
	if frontendErrMsg != "" {
		fmt.Fprintf(os.Stderr, "%s", frontendErrMsg)
		runCleanup()
		os.Exit(1)
	}
	if shouldExitNow {
		runCleanup()
		os.Exit(0)
	}

	// Set VM program arguments if using VM backend
	if opts.Target.Backend == "vm" {
		configureVMProgramArgs(entryFiles, programArgs)
	}

	exitCode, err := emitAndFinalizeWithOptions(irmod, outputPath, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		runCleanup()
		os.Exit(1)
	}

	// VM backend executes directly — no binary to run.
	if exitCode != 0 {
		runCleanup()
		os.Exit(exitCode)
	}

	if runMode {
		err = runCompiledBinary(outputPath)

		runCleanup()

		if err != nil {
			errStr := err.Error()
			code, msg := classifyRunModeError(errStr)
			if msg != "" {
				fmt.Fprintf(os.Stderr, "%s\n", msg)
			}
			os.Exit(code)
		}
		os.Exit(0)
	}

	runCleanup()
}

func printHelp(program string, out *os.File) {
	fmt.Fprintf(out, "Usage: %s [options] <file.go> [file2.go ...]\n", program)
	fmt.Fprintf(out, "\nOptions:\n")
	fmt.Fprintf(out, "  -o <path>              Output path (default: output)\n")
	fmt.Fprintf(out, "  -T <target>            Target triple or backend mode\n")
	fmt.Fprintf(out, "  -tags <a,b,c>          Extra build tags\n")
	fmt.Fprintf(out, "  -include <path|->      Add stdlib search root; first -include disables default embedded stdlib, -include - re-enables it\n")
	fmt.Fprintf(out, "  -extract-stdlib <dest> Extract standard library files into destination directory and exit\n")
	fmt.Fprintf(out, "  -parse-only            Parse and resolve imports only (no codegen)\n")
	if irBinaryEnabled {
		fmt.Fprintf(out, "  -emit-ir-binary <p>    Compile source and write binary IR module to path\n")
		fmt.Fprintf(out, "  -from-ir-binary <p>    Load binary IR module from path and run codegen\n")
	}
	fmt.Fprintf(out, "  -list-build-tags <p>   Write discovered build tags (one per line)\n")
	fmt.Fprintf(out, "  -run                   Compile and run the output binary\n")
	fmt.Fprintf(out, "  -size-analysis <path>  Write per-function size analysis JSON\n")
	fmt.Fprintf(out, "  -debug                 Enable compiler debug logging\n")
	fmt.Fprintf(out, "  -strip, -s             Strip symbol/debug metadata from native binaries\n")
	fmt.Fprintf(out, "  -h, --help             Show this help\n")
	fmt.Fprintf(out, "\nDefault target: %s\n", compilerTargetString(hostCompilerTarget()))
	fmt.Fprintf(out, "\nPossible -T values:\n")
	for _, target := range possibleTargets() {
		fmt.Fprintf(out, "  %s\n", target)
	}
}

func possibleTargets() []string {
	var targets []string
	targets = appendUnique(targets, compilerTargetString(hostCompilerTarget()))
	targets = appendUnique(targets, "linux/amd64")
	targets = appendUnique(targets, "linux/386")
	targets = appendUnique(targets, "linux/arm64")
	targets = appendUnique(targets, "darwin/amd64")
	targets = appendUnique(targets, "darwin/arm64")
	targets = appendUnique(targets, "windows/amd64")
	targets = appendUnique(targets, "windows/386")
	targets = appendUnique(targets, "windows/arm64")
	targets = appendUnique(targets, "wasi/wasm32")
	targets = appendUnique(targets, "dos/8086")
	targets = appendUnique(targets, "c")
	targets = appendUnique(targets, "c/16")
	targets = appendUnique(targets, "c/32")
	targets = appendUnique(targets, "c/64")
	targets = appendUnique(targets, "ir")
	targets = appendUnique(targets, "vm/8")
	targets = appendUnique(targets, "vm/16")
	targets = appendUnique(targets, "vm/32")
	targets = appendUnique(targets, "vm/64")
	return targets
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

// normalizePath replaces backslashes with forward slashes for Windows compatibility.
func normalizePath(path string) string {
	buf := make([]byte, len(path))
	i := 0
	for i < len(path) {
		if path[i] == '\\' {
			buf[i] = '/'
		} else {
			buf[i] = path[i]
		}
		i = i + 1
	}
	return string(buf)
}

// dirName returns the directory portion of a path.
func dirName(path string) string {
	i := len(path) - 1
	for i >= 0 {
		if path[i] == '/' {
			if i == 0 {
				return "/"
			}
			return path[0:i]
		}
		i = i - 1
	}
	return "."
}

func trimTrailingSlash(path string) string {
	for len(path) > 1 && path[len(path)-1] == '/' {
		path = path[0 : len(path)-1]
	}
	return path
}

func pathExists(path string) bool {
	_, err := os.ReadFile(path)
	return err == nil
}

func detectStdlibBaseDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	baseDir := cwd
	search := cwd
	for {
		if pathExists(search + "/std/runtime/runtime.go") {
			return search, nil
		}
		parent := dirName(search)
		if parent == search || parent == "" {
			break
		}
		search = parent
	}
	return baseDir, nil
}

func appendStdlibRootCandidates(roots []string, include string) []string {
	include = trimTrailingSlash(normalizePath(include))
	if include == "" || include == "-" {
		return roots
	}
	added := false
	if pathExists(include + "/runtime/runtime.go") {
		roots = appendUnique(roots, include)
		added = true
	}
	if pathExists(include + "/std/runtime/runtime.go") {
		roots = appendUnique(roots, include+"/std")
		added = true
	}
	if !added {
		roots = appendUnique(roots, include)
	}
	return roots
}

func resolveStdlibDiskRoots() ([]string, error) {
	var roots []string
	if stdlibIncludeExplicit {
		for _, include := range stdlibIncludePaths {
			roots = appendStdlibRootCandidates(roots, include)
		}
		return roots, nil
	}
	baseDir, err := detectStdlibBaseDir()
	if err != nil {
		return nil, err
	}
	roots = append(roots, baseDir+"/std")
	return roots, nil
}

func isSafeRelativePath(path string) bool {
	if path == "" || path == "." {
		return false
	}
	if path[0] == '/' {
		return false
	}
	if len(path) >= 2 && path[1] == ':' {
		return false
	}
	if path == ".." || strings.HasPrefix(path, "../") || strings.Contains(path, "/../") || strings.HasSuffix(path, "/..") {
		return false
	}
	return true
}

func sortNameDataPairs(names []string, data []string) {
	i := 1
	for i < len(names) {
		j := i
		for j > 0 && names[j] < names[j-1] {
			tmpN := names[j]
			tmpD := data[j]
			names[j] = names[j-1]
			data[j] = data[j-1]
			names[j-1] = tmpN
			data[j-1] = tmpD
			j = j - 1
		}
		i = i + 1
	}
}

func extractEmbeddedStdlib(dest string) error {
	dest = trimTrailingSlash(dest)
	if dest == "" {
		return fmt.Errorf("destination path cannot be empty")
	}
	err := os.MkdirAll(dest, 0755)
	if err != nil {
		return err
	}
	extracted := false
	if shouldUseEmbeddedStdlib() {
		names, data := walkEmbedFromFS(".")
		if len(names) > 0 {
			err = writeExtractedStdlibFiles(dest, names, data)
			if err != nil {
				return err
			}
			extracted = true
		}
	}
	roots, err := resolveStdlibDiskRoots()
	if err != nil {
		return err
	}
	for _, root := range roots {
		names, data := walkEmbedDir(root, root)
		if len(names) == 0 {
			continue
		}
		err = writeExtractedStdlibFiles(dest, names, data)
		if err != nil {
			return err
		}
		extracted = true
	}
	if !extracted {
		return fmt.Errorf("no standard library files found in configured sources")
	}
	return nil
}

func writeExtractedStdlibFiles(dest string, names []string, data []string) error {
	dest = trimTrailingSlash(dest)
	sortNameDataPairs(names, data)
	i := 0
	for i < len(names) {
		rel := normalizePath(names[i])
		if strings.HasPrefix(rel, "./") {
			rel = rel[2:len(rel)]
		}
		if !isSafeRelativePath(rel) {
			return fmt.Errorf("unsafe embedded path %q", names[i])
		}
		outPath := dest + "/" + rel
		parent := dirName(outPath)
		if parent != "" && parent != "." {
			err := os.MkdirAll(parent, 0755)
			if err != nil {
				return err
			}
		}
		err := os.WriteFile(outPath, []byte(data[i]), 0644)
		if err != nil {
			return err
		}
		i = i + 1
	}
	return nil
}
