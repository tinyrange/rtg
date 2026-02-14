package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Target and build tag globals — defaults to host platform
var targetGOOS string = runtime.GOOS
var targetGOARCH string = runtime.GOARCH
var targetPtrSize int = defaultPtrSize()

func defaultPtrSize() int {
	if runtime.GOARCH == "386" || runtime.GOARCH == "wasm32" {
		return 4
	}
	return 8
}
var targetBackend string = "native" // native, c, ir, or vm
var targetCModel int = 0            // 16/32/64 when targetBackend==c
var targetWordSize int = defaultPtrSize() // word size in bytes
var buildTags []string
var compilerDebug bool

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

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s [-o output] [-T os/arch|c[/16|32|64]] [-tags tag1,tag2] [-run] <file.go> [file2.go ...]\n", os.Args[0])
		os.Exit(1)
	}

	outputPath := "output"
	var entryFiles []string
	var extraTags string
	var runMode bool
	var programArgs []string
	i := 1
	for i < len(os.Args) {
		if os.Args[i] == "-run" {
			runMode = true
			i = i + 1
		} else if os.Args[i] == "-o" && i+1 < len(os.Args) {
			outputPath = os.Args[i+1]
			i = i + 2
		} else if os.Args[i] == "-T" && i+1 < len(os.Args) {
			target := os.Args[i+1]
			if target == "c" || strings.HasPrefix(target, "c/") {
				targetBackend = "c"
				targetCModel = 64
				if strings.HasPrefix(target, "c/") {
					model := target[2:]
					if model == "16" {
						targetCModel = 16
					} else if model == "32" {
						targetCModel = 32
					} else if model == "64" {
						targetCModel = 64
					} else {
						fmt.Fprintf(os.Stderr, "invalid target %q: expected c, c/16, c/32, or c/64\n", target)
						os.Exit(1)
					}
				}
				if targetCModel == 16 {
					targetPtrSize = 2
				} else if targetCModel == 32 {
					targetPtrSize = 4
				} else {
					targetPtrSize = 8
				}
				targetGOOS = "c"
				targetGOARCH = fmt.Sprintf("c%d", targetCModel)
			} else if target == "ir" {
				targetBackend = "ir"
			} else if strings.HasPrefix(target, "vm/") {
				targetBackend = "vm"
				model := target[3:]
				if model == "8" {
					targetWordSize = 1
					targetPtrSize = 2
				} else if model == "16" {
					targetWordSize = 2
					targetPtrSize = 2
				} else if model == "32" {
					targetWordSize = 4
					targetPtrSize = 4
				} else if model == "64" {
					targetWordSize = 8
					targetPtrSize = 8
				} else {
					fmt.Fprintf(os.Stderr, "invalid target %q: expected vm/8, vm/16, vm/32, or vm/64\n", target)
					os.Exit(1)
				}
				// Reuse C backend's runtime/os files via matching build tags
				targetGOOS = "c"
				bits := targetWordSize * 8
				targetGOARCH = fmt.Sprintf("c%d", bits)
			} else {
				slashIdx := strings.Index(target, "/")
				if slashIdx < 0 {
					fmt.Fprintf(os.Stderr, "invalid target %q: expected os/arch or c[/16|32|64]\n", target)
					os.Exit(1)
				}
				targetGOOS = target[0:slashIdx]
				targetGOARCH = target[slashIdx+1:]
				if targetGOARCH == "386" || targetGOARCH == "wasm32" {
					targetPtrSize = 4
				} else {
					targetPtrSize = 8
				}
			}
			i = i + 2
		} else if os.Args[i] == "-size-analysis" && i+1 < len(os.Args) {
			sizeAnalysisPath = os.Args[i+1]
			i = i + 2
		} else if os.Args[i] == "-tags" && i+1 < len(os.Args) {
			extraTags = os.Args[i+1]
			i = i + 2
		} else if os.Args[i] == "-debug" {
			compilerDebug = true
			i = i + 1
		} else if os.Args[i] == "--" {
			i = i + 1
			for i < len(os.Args) {
				programArgs = append(programArgs, os.Args[i])
				i = i + 1
			}
		} else {
			entryFiles = append(entryFiles, normalizePath(os.Args[i]))
			i = i + 1
		}
	}
	if runMode {
		// Determine temp directory (portable across OSes)
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

		sep := "/"
		if runtime.GOOS == "windows" {
			sep = "\\"
		}
		pid := fmt.Sprintf("%d", os.Getpid())
		runTmpSrc = tmpDir + sep + "rtg-run-" + pid + ".go"
		runTmpBin = tmpDir + sep + "rtg-run-" + pid
		if targetGOOS == "windows" {
			runTmpBin = runTmpBin + ".exe"
		}

		// Read from stdin if no entry files
		if len(entryFiles) == 0 {
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
				fmt.Fprintf(os.Stderr, "rtg -run: no input on stdin and no files specified\n")
				os.Exit(1)
			}
			err := os.WriteFile(runTmpSrc, src, 0644)
			if err != nil {
				fmt.Fprintf(os.Stderr, "rtg -run: failed to write temp source: %v\n", err)
				runCleanup()
				os.Exit(1)
			}
			entryFiles = append(entryFiles, runTmpSrc)
		}

		// Override output to temp binary
		outputPath = runTmpBin
	}

	if len(entryFiles) == 0 {
		fmt.Fprintf(os.Stderr, "usage: %s [-o output] [-T os/arch|c[/16|32|64]] [-tags tag1,tag2] [-run] <file.go> [file2.go ...]\n", os.Args[0])
		os.Exit(1)
	}

	// Build active tag set from target + explicit tags
	if targetBackend == "c" {
		buildTags = append(buildTags, "c")
		buildTags = append(buildTags, fmt.Sprintf("c%d", targetCModel))
	} else if targetGOOS == "wasi" && targetGOARCH == "wasm32" {
		buildTags = append(buildTags, "wasi")
		buildTags = append(buildTags, "wasm32")
	} else {
		buildTags = append(buildTags, targetGOOS)
		buildTags = append(buildTags, targetGOARCH)
	}
	if extraTags != "" {
		parts := strings.Split(extraTags, ",")
		for _, t := range parts {
			if t != "" {
				buildTags = append(buildTags, t)
			}
		}
	}
	buildTags = append(buildTags, "rtg")

	// Initialize embedded std if available
	initEmbeddedStd()

	// Determine base directory for the std library.
	// When embedded std is available, skip the disk search entirely.
	var baseDir string
	if hasEmbeddedStd() {
		baseDir = "."
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error getting working directory: %v\n", err)
			runCleanup()
			os.Exit(1)
		}
		// Walk up from cwd until we find a directory containing std/runtime/runtime.go
		baseDir = cwd
		search := cwd
		for {
			_, err := os.ReadFile(search + "/std/runtime/runtime.go")
			if err == nil {
				baseDir = search
				break
			}
			parent := dirName(search)
			if parent == search || parent == "" {
				break
			}
			search = parent
		}
	}

	if compilerDebug {
		fmt.Fprintf(os.Stderr, "debug: resolving module (%d entry files)\n", len(entryFiles))
	}
	mod := ResolveModule(baseDir, entryFiles)
	if compilerDebug {
		fmt.Fprintf(os.Stderr, "debug: resolved %d packages\n", len(mod.Packages))
	}

	// Validate cross-package references
	valErrs := ValidateModule(mod)
	if len(valErrs) > 0 {
		fmt.Fprintf(os.Stderr, "\n%d validation errors:\n", len(valErrs))
		for _, e := range valErrs {
			fmt.Fprintf(os.Stderr, "  %s\n", e)
		}
		runCleanup()
		os.Exit(1)
	}

	// Compile to IR
	if compilerDebug {
		fmt.Fprintf(os.Stderr, "debug: compiling to IR\n")
	}
	irmod, errs := CompileModule(mod)

	if len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "\n%d compile errors:\n", len(errs))
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  %s\n", e)
		}
		runCleanup()
		os.Exit(1)
	}

	if compilerDebug {
		fmt.Fprintf(os.Stderr, "debug: IR compiled (%d funcs, %d globals)\n", len(irmod.Funcs), len(irmod.Globals))
	}
	eliminateDeadFunctions(irmod)
	if compilerDebug {
		fmt.Fprintf(os.Stderr, "debug: DCE done (%d funcs remaining)\n", len(irmod.Funcs))
	}

	// Set VM program arguments if using VM backend
	if targetBackend == "vm" {
		// argv[0] is the program name, followed by actual args
		vmArgs = append(vmArgs, "rtg")
		if len(programArgs) > 0 {
			vmArgs = append(vmArgs, programArgs...)
		} else {
			i := 0
			for i < len(entryFiles) {
				vmArgs = append(vmArgs, entryFiles[i])
				i = i + 1
			}
		}
	}

	if compilerDebug {
		fmt.Fprintf(os.Stderr, "debug: generating output (backend=%s, target=%s/%s)\n", targetBackend, targetGOOS, targetGOARCH)
	}
	err := GenerateELF(irmod, outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "codegen error: %v\n", err)
		runCleanup()
		os.Exit(1)
	}

	if compilerDebug {
		fmt.Fprintf(os.Stderr, "debug: output generated successfully\n")
	}

	writeSizeAnalysis()

	// VM backend executes directly — no binary to run
	if targetBackend == "vm" {
		runCleanup()
		os.Exit(vmExitCode)
	}

	if runMode {
		cmd := exec.Command(outputPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()

		runCleanup()

		if err != nil {
			// Parse exit code from "exit status N"
			errStr := err.Error()
			if strings.HasPrefix(errStr, "exit status ") {
				codeStr := errStr[12:]
				code := 0
				j := 0
				for j < len(codeStr) {
					if codeStr[j] >= '0' && codeStr[j] <= '9' {
						code = code*10 + int(codeStr[j]-'0')
					}
					j++
				}
				os.Exit(code)
			}
			fmt.Fprintf(os.Stderr, "rtg -run: %s\n", err.Error())
			os.Exit(1)
		}
		os.Exit(0)
	}
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
