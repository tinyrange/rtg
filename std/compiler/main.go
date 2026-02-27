//go:build !no_frontend

package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"j5.nz/rtg/std/compiler/backend"
	"j5.nz/rtg/std/compiler/backend/irprint"
	"j5.nz/rtg/std/compiler/backend/vm"
	"j5.nz/rtg/std/compiler/binary"
	"j5.nz/rtg/std/compiler/common"
	frontend "j5.nz/rtg/std/compiler/frontend/go"
	"j5.nz/rtg/std/compiler/ir"
	"j5.nz/rtg/std/compiler/stdlib"
	targetcfg "j5.nz/rtg/std/target"
)

// Target and build tag globals — defaults to host platform
var compileTarget = common.Target{
	Triple:                runtime.GOOS + "/" + runtime.GOARCH,
	GOOS:                  runtime.GOOS,
	GOARCH:                runtime.GOARCH,
	PtrSize:               defaultPtrSize(),
	Backend:               "native",         // native, c, or vm
	CModel:                0,                // 16/32/64 when targetBackend==c
	WordSize:              defaultPtrSize(), // word size in bytes
	BuildTags:             []string{},
	Defines:               map[string]string{},
	CompilerDebug:         false,
	StripBinary:           false,
	StdlibIncludePaths:    []string{},
	StdlibIncludeExplicit: false,
	StdlibIncludeEmbedded: false,
}

func defaultPtrSize() int {
	if runtime.GOARCH == "386" || runtime.GOARCH == "wasm32" || runtime.GOARCH == "armv8m" {
		return 4
	}
	return 8
}

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

func traceExit(code int) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "arm64" {
		return
	}
	want := os.Getenv("RTG_TRACE_EXIT")
	if want == "" {
		return
	}
	switch want {
	case "10":
		if code == 10 {
			os.Exit(code)
		}
	case "20":
		if code == 20 {
			os.Exit(code)
		}
	case "30":
		if code == 30 {
			os.Exit(code)
		}
	case "40":
		if code == 40 {
			os.Exit(code)
		}
	case "50":
		if code == 50 {
			os.Exit(code)
		}
	}
}

func main() {
	if err := loadBuiltinTargetDefinitions(); err != nil {
		fmt.Fprintf(os.Stderr, "rtg: failed to load built-in target definitions: %v\n", err)
		os.Exit(1)
	}

	if len(os.Args) < 2 {
		printHelp(os.Args[0], os.Stderr)
		os.Exit(1)
	}
	args := os.Args

	targetFiles, err := collectTargetFileArgs(args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "rtg: %v\n", err)
		os.Exit(1)
	}
	targetRoots, err := collectTargetRootArgs(args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "rtg: %v\n", err)
		os.Exit(1)
	}
	if len(targetRoots) > 0 {
		if err := targetcfg.LoadTargetRoots(targetRoots); err != nil {
			fmt.Fprintf(os.Stderr, "rtg: failed to load target root: %v\n", err)
			os.Exit(1)
		}
	}
	if len(targetFiles) > 0 {
		if err := targetcfg.LoadTargetFiles(targetFiles); err != nil {
			fmt.Fprintf(os.Stderr, "rtg: failed to load target file: %v\n", err)
			os.Exit(1)
		}
	}

	outputPath := "output"
	var entryFiles []string
	var extraTags string
	var parseOnly bool
	var buildTagsPath string
	var emitIRPath string
	var emitIRBinaryPath string
	var fromIRBinaryPath string
	var profileReportPath string
	var extractStdlibDest string
	var runMode bool
	var stdinInput bool
	var showVersion bool
	var programArgs []string
	i := 1
	for i < len(args) {
		arg := args[i]
		switch arg {
		case "-h", "--help":
			printHelp(args[0], os.Stdout)
			os.Exit(0)
		case "-version", "--version":
			showVersion = true
			i = i + 1
			continue
		case "-run":
			runMode = true
			i = i + 1
			continue
		case "-o":
			if i+1 < len(args) {
				outputPath = args[i+1]
				i = i + 2
				continue
			}
		case "-T":
			if i+1 < len(args) {
				target := args[i+1]
				if target == "c" || strings.HasPrefix(target, "c/") {
					compileTarget.Triple = target
					compileTarget.Backend = "c"
					compileTarget.CModel = 64
					if strings.HasPrefix(target, "c/") {
						model := target[2:]
						if model == "16" {
							compileTarget.CModel = 16
						} else if model == "32" {
							compileTarget.CModel = 32
						} else if model == "64" {
							compileTarget.CModel = 64
						} else {
							fmt.Fprintf(os.Stderr, "invalid target %q: expected c, c/16, c/32, or c/64\n", target)
							os.Exit(1)
						}
					}
					if compileTarget.CModel == 16 {
						compileTarget.PtrSize = 2
					} else if compileTarget.CModel == 32 {
						compileTarget.PtrSize = 4
					} else {
						compileTarget.PtrSize = 8
					}
					compileTarget.WordSize = compileTarget.PtrSize
					compileTarget.GOOS = "c"
					compileTarget.GOARCH = fmt.Sprintf("c%d", compileTarget.CModel)
				} else if target == "ir" {
					fmt.Fprintf(os.Stderr, "target %q is no longer supported; use -emit-ir <path> with a concrete -T <target>\n", target)
					os.Exit(1)
				} else if strings.HasPrefix(target, "vm/") {
					compileTarget.Triple = target
					compileTarget.Backend = "vm"
					model := target[3:]
					if model == "8" {
						compileTarget.WordSize = 1
						compileTarget.PtrSize = 2
					} else if model == "16" {
						compileTarget.WordSize = 2
						compileTarget.PtrSize = 2
					} else if model == "32" {
						compileTarget.WordSize = 4
						compileTarget.PtrSize = 4
					} else if model == "64" {
						compileTarget.WordSize = 8
						compileTarget.PtrSize = 8
					} else {
						fmt.Fprintf(os.Stderr, "invalid target %q: expected vm/8, vm/16, vm/32, or vm/64\n", target)
						os.Exit(1)
					}
					compileTarget.GOOS = "c"
					bits := compileTarget.WordSize * 8
					compileTarget.GOARCH = fmt.Sprintf("c%d", bits)
				} else {
					_, handledByTargetPkg, err := targetcfg.Apply(target, &compileTarget)
					if handledByTargetPkg {
						if err != nil {
							fmt.Fprintf(os.Stderr, "invalid target %q: %v\n", target, err)
							os.Exit(1)
						}
						compileTarget.Triple = target
						i = i + 2
						continue
					}
					if target == "dos/8086" {
						compileTarget.Triple = target
						compileTarget.GOOS = "dos"
						compileTarget.GOARCH = "dos16"
						compileTarget.PtrSize = 2
						compileTarget.WordSize = 2
						i = i + 2
						continue
					}
					slashIdx := strings.Index(target, "/")
					if slashIdx < 0 {
						fmt.Fprintf(os.Stderr, "invalid target %q: expected os/arch, dos/8086, c[/16|32|64], or vm/<8|16|32|64>\n", target)
						os.Exit(1)
					}
					compileTarget.GOOS = target[0:slashIdx]
					compileTarget.GOARCH = target[slashIdx+1:]
					compileTarget.Triple = target
					if compileTarget.GOARCH == "386" || compileTarget.GOARCH == "wasm32" || compileTarget.GOARCH == "armv8m" {
						compileTarget.PtrSize = 4
					} else {
						compileTarget.PtrSize = 8
					}
					compileTarget.WordSize = compileTarget.PtrSize
				}
				i = i + 2
				continue
			}
		case "-size-analysis":
			if i+1 < len(args) {
				ir.SizeAnalysisPath = args[i+1]
				i = i + 2
				continue
			}
		case "-parse-only":
			parseOnly = true
			i = i + 1
			continue
		case "-strict":
			compileTarget.Strict = true
			i = i + 1
			continue
		case "-profile":
			compileTarget.Profile = true
			i = i + 1
			continue
		case "-profile-report":
			if i+1 < len(args) {
				profileReportPath = args[i+1]
				i = i + 2
				continue
			}
		case "-emit-ir":
			if i+1 < len(args) {
				emitIRPath = args[i+1]
				i = i + 2
				continue
			}
		case "-emit-ir-binary", "-from-ir-binary":
			if i+1 < len(args) {
				if !binary.IrBinaryEnabled {
					fmt.Fprintf(os.Stderr, "IR binary I/O is experimental; rebuild with -tags exp_ir_binary\n")
					runCleanup()
					os.Exit(1)
				}
				if arg == "-emit-ir-binary" {
					emitIRBinaryPath = args[i+1]
				} else {
					fromIRBinaryPath = args[i+1]
				}
				i = i + 2
				continue
			}
		case "-list-build-tags":
			if i+1 < len(args) {
				buildTagsPath = args[i+1]
				i = i + 2
				continue
			}
		case "-tags":
			if i+1 < len(args) {
				extraTags = args[i+1]
				i = i + 2
				continue
			}
		case "-target-file", "-target-root":
			if i+1 < len(args) {
				i = i + 2
				continue
			}
		case "-D":
			if i+1 < len(args) {
				key, value, ok := parseDefineArg(args[i+1])
				if !ok {
					fmt.Fprintf(os.Stderr, "invalid -D value %q: expected key=value\n", args[i+1])
					runCleanup()
					os.Exit(1)
				}
				compileTarget.Defines[key] = value
				i = i + 2
				continue
			}
		case "-include":
			if i+1 < len(args) {
				val := common.NormalizePath(args[i+1])
				if !compileTarget.StdlibIncludeExplicit {
					compileTarget.StdlibIncludeExplicit = true
					compileTarget.StdlibIncludeEmbedded = false
				}
				if val == "-" {
					compileTarget.StdlibIncludeEmbedded = true
				} else if val != "" {
					compileTarget.StdlibIncludePaths = common.AppendUnique(
						compileTarget.StdlibIncludePaths, common.TrimTrailingSlash(val),
					)
				}
				i = i + 2
				continue
			}
		case "-extract-stdlib":
			if i+1 < len(args) {
				extractStdlibDest = common.NormalizePath(args[i+1])
				i = i + 2
				continue
			}
		case "-debug":
			compileTarget.CompilerDebug = true
			i = i + 1
			continue
		case "-strip", "-s":
			compileTarget.StripBinary = true
			i = i + 1
			continue
		case "--":
			i = i + 1
			for i < len(args) {
				programArgs = append(programArgs, args[i])
				i = i + 1
			}
			continue
		case "-":
			stdinInput = true
			i = i + 1
			continue
		}
		entryFiles = append(entryFiles, common.NormalizePath(arg))
		i = i + 1
	}
	if showVersion {
		fmt.Fprintf(os.Stdout, "%s\n", compilerStamp())
		os.Exit(0)
	}
	if stdinInput {
		if fromIRBinaryPath != "" {
			fmt.Fprintf(os.Stderr, "cannot use - with -from-ir-binary\n")
			runCleanup()
			os.Exit(1)
		}
		err := readStdinSourceToTemp()
		if err != nil {
			fmt.Fprintf(os.Stderr, "rtg: failed to read stdin source: %v\n", err)
			runCleanup()
			os.Exit(1)
		}
		entryFiles = append(entryFiles, runTmpSrc)
	}
	if runMode {
		tmpDir := tempDirPath()
		sep := pathSep()
		pid := fmt.Sprintf("%d", os.Getpid())
		if runTmpSrc == "" {
			runTmpSrc = tmpDir + sep + "rtg-run-" + pid + ".go"
		}
		runTmpBin = tmpDir + sep + "rtg-run-" + pid
		if compileTarget.GOOS == "windows" {
			runTmpBin = runTmpBin + ".exe"
		}

		// Read from stdin if no entry files
		if len(entryFiles) == 0 {
			err := readStdinSourceToTemp()
			if err != nil {
				fmt.Fprintf(os.Stderr, "rtg -run: failed to read stdin source: %v\n", err)
				runCleanup()
				os.Exit(1)
			}
			entryFiles = append(entryFiles, runTmpSrc)
		}

		// Override output to temp binary
		outputPath = runTmpBin
	}
	if emitIRPath != "" && runMode {
		fmt.Fprintf(os.Stderr, "-emit-ir cannot be combined with -run\n")
		runCleanup()
		os.Exit(1)
	}
	if emitIRPath != "" && emitIRBinaryPath != "" {
		fmt.Fprintf(os.Stderr, "-emit-ir cannot be combined with -emit-ir-binary\n")
		runCleanup()
		os.Exit(1)
	}

	if fromIRBinaryPath != "" && len(entryFiles) > 0 {
		fmt.Fprintf(os.Stderr, "cannot combine source files with -from-ir-binary\n")
		runCleanup()
		os.Exit(1)
	}
	if extractStdlibDest == "" && profileReportPath == "" && fromIRBinaryPath == "" && len(entryFiles) == 0 {
		printHelp(os.Args[0], os.Stderr)
		os.Exit(1)
	}

	// Build active tag set from target + explicit tags
	if compileTarget.Backend == "c" {
		compileTarget.BuildTags = append(compileTarget.BuildTags, "c")
		compileTarget.BuildTags = append(compileTarget.BuildTags, fmt.Sprintf("c%d", compileTarget.CModel))
	} else if compileTarget.GOOS == "wasi" && compileTarget.GOARCH == "wasm32" {
		compileTarget.BuildTags = append(compileTarget.BuildTags, "wasi")
		compileTarget.BuildTags = append(compileTarget.BuildTags, "wasm32")
	} else {
		compileTarget.BuildTags = append(compileTarget.BuildTags, compileTarget.GOOS)
		compileTarget.BuildTags = append(compileTarget.BuildTags, compileTarget.GOARCH)
	}
	if extraTags != "" {
		parts := strings.Split(extraTags, ",")
		for _, t := range parts {
			if t != "" {
				compileTarget.BuildTags = append(compileTarget.BuildTags, t)
			}
		}
	}
	compileTarget.BuildTags = append(compileTarget.BuildTags, "rtg")
	if ir.SizeAnalysisPath != "" {
		compileTarget.StripBinary = true
	}
	if compilerBuildGitHash != "" {
		if _, ok := compileTarget.Defines["main.compilerBuildGitHash"]; !ok {
			compileTarget.Defines["main.compilerBuildGitHash"] = compilerBuildGitHash
		}
	}
	traceExit(10)

	if profileReportPath != "" {
		if extractStdlibDest != "" || fromIRBinaryPath != "" || runMode || stdinInput || parseOnly || emitIRPath != "" || emitIRBinaryPath != "" || buildTagsPath != "" {
			fmt.Fprintf(os.Stderr, "-profile-report cannot be combined with compilation/runtime options\n")
			runCleanup()
			os.Exit(1)
		}
		err := runProfileReport(profileReportPath, entryFiles)
		if err != nil {
			fmt.Fprintf(os.Stderr, "profile report error: %v\n", err)
			runCleanup()
			os.Exit(1)
		}
		runCleanup()
		os.Exit(0)
	}

	if extractStdlibDest != "" {
		if fromIRBinaryPath != "" || len(entryFiles) > 0 || runMode || stdinInput || parseOnly || emitIRPath != "" || emitIRBinaryPath != "" || buildTagsPath != "" {
			fmt.Fprintf(os.Stderr, "-extract-stdlib cannot be combined with compilation inputs/options\n")
			runCleanup()
			os.Exit(1)
		}
		err := extractEmbeddedStdlib(extractStdlibDest)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error extracting stdlib: %v\n", err)
			runCleanup()
			os.Exit(1)
		}
		runCleanup()
		os.Exit(0)
	}

	var irmod *ir.IRModule
	if fromIRBinaryPath != "" {
		if parseOnly {
			fmt.Fprintf(os.Stderr, "-parse-only is not valid with -from-ir-binary\n")
			runCleanup()
			os.Exit(1)
		}
		if buildTagsPath != "" {
			fmt.Fprintf(os.Stderr, "-list-build-tags is not valid with -from-ir-binary\n")
			runCleanup()
			os.Exit(1)
		}
		var err error
		irmod, err = binary.ReadIRBinary(fromIRBinaryPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading IR binary: %v\n", err)
			runCleanup()
			os.Exit(1)
		}
		if compileTarget.CompilerDebug {
			fmt.Fprintf(os.Stderr, "debug: loaded IR binary (%d funcs, %d globals)\n", len(irmod.Funcs), len(irmod.Globals))
		}
	} else {
		// Determine base directory for the std library.
		// When embedded std is available, skip the disk search entirely.
		var baseDir string
		if stdlib.HasEmbeddedStd() {
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
				parent := common.DirName(search)
				if parent == search || parent == "" {
					break
				}
				search = parent
			}
		}

		if compileTarget.CompilerDebug {
			fmt.Fprintf(os.Stderr, "debug: resolving module (%d entry files)\n", len(entryFiles))
		}
		frontend.ResetDiscoveredBuildTags()
		mod := frontend.ResolveModule(&compileTarget, baseDir, entryFiles)
		if compileTarget.CompilerDebug {
			fmt.Fprintf(os.Stderr, "debug: resolved %d packages\n", len(mod.Packages))
		}
		traceExit(20)

		if buildTagsPath != "" {
			tags := frontend.GetDiscoveredBuildTags()
			var out string
			for _, t := range tags {
				out = out + t + "\n"
			}
			err := os.WriteFile(buildTagsPath, []byte(out), 0644)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error writing build tag list: %v\n", err)
				runCleanup()
				os.Exit(1)
			}
		}

		if parseOnly {
			if emitIRPath != "" {
				fmt.Fprintf(os.Stderr, "-emit-ir is not valid with -parse-only\n")
				runCleanup()
				os.Exit(1)
			}
			runCleanup()
			os.Exit(0)
		}

		// Validate cross-package references
		valErrs := frontend.ValidateModule(mod)
		if len(valErrs) > 0 {
			fmt.Fprintf(os.Stderr, "\n%d validation errors:\n", len(valErrs))
			for _, e := range valErrs {
				fmt.Fprintf(os.Stderr, "  %s\n", e)
			}
			runCleanup()
			os.Exit(1)
		}

		// Compile to IR
		if compileTarget.CompilerDebug {
			fmt.Fprintf(os.Stderr, "debug: compiling to IR\n")
		}
		var errs []string
		irmod, errs = frontend.CompileModule(compileTarget, mod)

		if len(errs) > 0 {
			fmt.Fprintf(os.Stderr, "\n%d compile errors:\n", len(errs))
			for _, e := range errs {
				fmt.Fprintf(os.Stderr, "  %s\n", e)
			}
			runCleanup()
			os.Exit(1)
		}

		if compileTarget.CompilerDebug {
			fmt.Fprintf(os.Stderr, "debug: IR compiled (%d funcs, %d globals)\n", len(irmod.Funcs), len(irmod.Globals))
		}
		traceExit(30)
		ir.EliminateDeadFunctions(irmod)
		if compileTarget.CompilerDebug {
			fmt.Fprintf(os.Stderr, "debug: DCE done (%d funcs remaining)\n", len(irmod.Funcs))
		}
		traceExit(40)
		if emitIRBinaryPath != "" {
			err := binary.WriteIRBinary(irmod, emitIRBinaryPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error writing IR binary: %v\n", err)
				runCleanup()
				os.Exit(1)
			}
			runCleanup()
			os.Exit(0)
		}
	}

	var vmArgs []string

	// Set VM program arguments if using VM backend
	if compileTarget.Backend == "vm" && emitIRPath == "" {
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
		vm.SetArgs(vmArgs)
	}

	if compileTarget.CompilerDebug {
		if emitIRPath != "" {
			fmt.Fprintf(os.Stderr, "debug: generating output (backend=%s, target=%s/%s, emit=ir)\n", compileTarget.Backend, compileTarget.GOOS, compileTarget.GOARCH)
		} else {
			fmt.Fprintf(os.Stderr, "debug: generating output (backend=%s, target=%s/%s)\n", compileTarget.Backend, compileTarget.GOOS, compileTarget.GOARCH)
		}
	}
	if emitIRPath != "" {
		err = irprint.Generate(irmod, emitIRPath)
	} else {
		err = backend.Generate(&compileTarget, irmod, outputPath)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "codegen error: %v\n", err)
		runCleanup()
		os.Exit(1)
	}

	if compileTarget.CompilerDebug {
		fmt.Fprintf(os.Stderr, "debug: output generated successfully\n")
	}
	traceExit(50)

	ir.WriteSizeAnalysis(compileTarget)

	// VM backend executes directly — no binary to run
	if compileTarget.Backend == "vm" && emitIRPath == "" {
		runCleanup()
		os.Exit(vm.ExitCode)
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

	runCleanup()
}

func printHelp(program string, out *os.File) {
	fmt.Fprintf(out, "Usage: %s [options] <file.go> [file2.go ...]\n", program)
	fmt.Fprintf(out, "\nOptions:\n")
	fmt.Fprintf(out, "  -o <path>              Output path (default: output)\n")
	fmt.Fprintf(out, "  -T <target>            Target triple or backend mode\n")
	fmt.Fprintf(out, "  -emit-ir <path>        Emit textual IR for the selected target instead of native/C/VM output\n")
	fmt.Fprintf(out, "  -tags <a,b,c>          Extra build tags\n")
	fmt.Fprintf(out, "  -D <key=value>         Set a string value for a global variable symbol\n")
	fmt.Fprintf(out, "  -target-file <path>    Load a single-file target definition before -T resolution\n")
	fmt.Fprintf(out, "  -target-root <path>    Recursively load *.go target definitions from a directory\n")
	fmt.Fprintf(out, "  -include <path|->      Add stdlib search root; first -include disables default embedded stdlib, -include - re-enables it\n")
	fmt.Fprintf(out, "  -extract-stdlib <dest> Extract standard library files into destination directory and exit\n")
	fmt.Fprintf(out, "  -parse-only            Parse and resolve imports only (no codegen)\n")
	fmt.Fprintf(out, "  -strict                Reject RTG-only language extensions in user packages\n")
	fmt.Fprintf(out, "  -profile               Enable //rtg:profile method instrumentation\n")
	fmt.Fprintf(out, "  -profile-report <p>    Read profile records from path and print aggregated method tree\n")
	if binary.IrBinaryEnabled {
		fmt.Fprintf(out, "  -emit-ir-binary <p>    Compile source and write binary IR module to path\n")
		fmt.Fprintf(out, "  -from-ir-binary <p>    Load binary IR module from path and run codegen\n")
	}
	fmt.Fprintf(out, "  -list-build-tags <p>   Write discovered build tags (one per line)\n")
	fmt.Fprintf(out, "  -run                   Compile and run the output binary\n")
	fmt.Fprintf(out, "  -size-analysis <path>  Write per-function size analysis JSON\n")
	fmt.Fprintf(out, "  -version, --version    Print compiler stamp\n")
	fmt.Fprintf(out, "  -debug                 Enable compiler debug logging\n")
	fmt.Fprintf(out, "  -strip, -s             Strip symbol/debug metadata from native binaries\n")
	fmt.Fprintf(out, "  -h, --help             Show this help\n")
	fmt.Fprintf(out, "\nDefault target: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(out, "\nPossible -T values:\n")
	for _, target := range possibleTargets() {
		fmt.Fprintf(out, "  %s\n", target)
	}
}

type profileStat struct {
	Hash  uint32
	Name  string
	Total uint64
	Calls uint64
}

type profileTreeNode struct {
	Name     string
	Total    uint64
	Calls    uint64
	Children []*profileTreeNode
}

func runProfileReport(profilePath string, entryFiles []string) error {
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return err
	}
	if len(data) < 8 {
		fmt.Fprintf(os.Stdout, "Profile report for %s\n", profilePath)
		fmt.Fprintf(os.Stdout, "no records\n")
		return nil
	}
	limit := len(data) - (len(data) % 8)
	totals := make(map[uint32]uint64)
	calls := make(map[uint32]uint64)
	i := 0
	for i+8 <= limit {
		hash := common.GetU32(data[i : i+4])
		duration := common.GetU32(data[i+4 : i+8])
		totals[hash] = totals[hash] + uint64(duration)
		calls[hash] = calls[hash] + 1
		i = i + 8
	}

	nameByHash := make(map[uint32]string)
	if len(entryFiles) > 0 {
		mapped, err := collectProfileMethodNameHashes(entryFiles)
		if err != nil {
			return err
		}
		nameByHash = mapped
	}

	var stats []profileStat
	for hash, total := range totals {
		name := nameByHash[hash]
		if name == "" {
			name = fmt.Sprintf("0x%08x", hash)
		}
		stats = append(stats, profileStat{
			Hash:  hash,
			Name:  name,
			Total: total,
			Calls: calls[hash],
		})
	}
	sortProfileStats(stats)

	root := &profileTreeNode{Name: "<root>"}
	for _, st := range stats {
		profileTreeInsert(root, st.Name, st.Total, st.Calls)
	}
	sortProfileTree(root)

	fmt.Fprintf(os.Stdout, "Profile report for %s\n", profilePath)
	fmt.Fprintf(os.Stdout, "records=%d unique=%d total_ns=%d\n", limit/8, len(stats), root.Total)
	profilePrintTree(root, "")
	if len(data) != limit {
		fmt.Fprintf(os.Stdout, "note: ignored %d trailing bytes (incomplete record)\n", len(data)-limit)
	}
	if len(entryFiles) > 0 && len(nameByHash) == 0 {
		fmt.Fprintf(os.Stdout, "note: no //rtg:profile methods discovered in provided source inputs\n")
	}
	return nil
}

func profileTreeInsert(root *profileTreeNode, name string, total uint64, count uint64) {
	root.Total = root.Total + total
	parts := strings.Split(name, ".")
	cur := root
	for _, part := range parts {
		if part == "" {
			continue
		}
		next := profileTreeFindOrAdd(cur, part)
		next.Total = next.Total + total
		cur = next
	}
	cur.Calls = cur.Calls + count
}

func profileTreeFindOrAdd(node *profileTreeNode, name string) *profileTreeNode {
	for _, child := range node.Children {
		if child.Name == name {
			return child
		}
	}
	child := &profileTreeNode{Name: name}
	node.Children = append(node.Children, child)
	return child
}

func sortProfileTree(node *profileTreeNode) {
	sortProfileTreeChildren(node.Children)
	for _, child := range node.Children {
		sortProfileTree(child)
	}
}

func sortProfileStats(stats []profileStat) {
	i := 1
	for i < len(stats) {
		j := i
		for j > 0 && profileStatLess(stats[j], stats[j-1]) {
			stats[j], stats[j-1] = stats[j-1], stats[j]
			j = j - 1
		}
		i++
	}
}

func profileStatLess(left profileStat, right profileStat) bool {
	if left.Total != right.Total {
		return left.Total > right.Total
	}
	return left.Name < right.Name
}

func sortProfileTreeChildren(children []*profileTreeNode) {
	i := 1
	for i < len(children) {
		j := i
		for j > 0 && profileTreeNodeLess(children[j], children[j-1]) {
			children[j], children[j-1] = children[j-1], children[j]
			j = j - 1
		}
		i++
	}
}

func profileTreeNodeLess(left *profileTreeNode, right *profileTreeNode) bool {
	if left.Total != right.Total {
		return left.Total > right.Total
	}
	return left.Name < right.Name
}

func profilePrintTree(node *profileTreeNode, prefix string) {
	for i, child := range node.Children {
		last := i == len(node.Children)-1
		branch := "|- "
		nextPrefix := prefix + "|  "
		if last {
			branch = "\\- "
			nextPrefix = prefix + "   "
		}
		if len(child.Children) == 0 {
			avg := uint64(0)
			if child.Calls > 0 {
				avg = child.Total / child.Calls
			}
			fmt.Fprintf(os.Stdout, "%s%s%s total=%dns calls=%d avg=%dns\n", prefix, branch, child.Name, child.Total, child.Calls, avg)
		} else {
			fmt.Fprintf(os.Stdout, "%s%s%s total=%dns\n", prefix, branch, child.Name, child.Total)
			profilePrintTree(child, nextPrefix)
		}
	}
}

func collectProfileMethodNameHashes(entryFiles []string) (map[uint32]string, error) {
	var baseDir string
	var err error
	if stdlib.HasEmbeddedStd() {
		baseDir = "."
	} else {
		baseDir, err = detectStdlibBaseDir()
		if err != nil {
			return nil, err
		}
	}
	frontend.ResetDiscoveredBuildTags()
	mod := frontend.ResolveModule(&compileTarget, baseDir, entryFiles)
	out := make(map[uint32]string)
	for _, qname := range frontend.CollectProfileMethodQualNames(mod) {
		out[profileHash32(qname)] = qname
	}
	return out, nil
}

func profileHash32(name string) uint32 {
	var h uint32 = 2166136261
	i := 0
	for i < len(name) {
		h = h ^ uint32(name[i])
		h = h * 16777619
		i++
	}
	return h
}

func parseDefineArg(raw string) (string, string, bool) {
	eq := strings.Index(raw, "=")
	if eq <= 0 {
		return "", "", false
	}
	key := raw[0:eq]
	value := raw[eq+1:]
	if key == "" {
		return "", "", false
	}
	return key, value, true
}

func collectTargetFileArgs(args []string) ([]string, error) {
	var files []string
	i := 0
	for i < len(args) {
		if args[i] == "--" {
			break
		}
		if args[i] == "-target-file" {
			if i+1 >= len(args) {
				return nil, fmt.Errorf("missing value after -target-file")
			}
			files = common.AppendUnique(files, common.NormalizePath(args[i+1]))
			i = i + 2
			continue
		}
		i = i + 1
	}
	return files, nil
}

func collectTargetRootArgs(args []string) ([]string, error) {
	var roots []string
	i := 0
	for i < len(args) {
		if args[i] == "--" {
			break
		}
		if args[i] == "-target-root" {
			if i+1 >= len(args) {
				return nil, fmt.Errorf("missing value after -target-root")
			}
			roots = common.AppendUnique(roots, common.NormalizePath(args[i+1]))
			i = i + 2
			continue
		}
		i = i + 1
	}
	return roots, nil
}

func loadBuiltinTargetDefinitions() error {
	baseDir, err := detectStdlibBaseDir()
	if err != nil {
		return err
	}
	root := baseDir + "/std/target"
	files := walkTargetDefinitionFiles(root, nil)
	if len(files) == 0 {
		return nil
	}
	return targetcfg.LoadTargetFiles(files)
}

func walkTargetDefinitionFiles(root string, out []string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	for _, entry := range entries {
		name := entry.Name()
		path := root + "/" + name
		if entry.IsDir() {
			out = walkTargetDefinitionFiles(path, out)
			continue
		}
		if name == "target.go" {
			out = append(out, path)
		}
	}
	return out
}

func possibleTargets() []string {
	var targets []string
	targets = common.AppendUnique(targets, runtime.GOOS+"/"+runtime.GOARCH)
	targets = common.AppendUnique(targets, "linux/amd64")
	targets = common.AppendUnique(targets, "linux/386")
	targets = common.AppendUnique(targets, "linux/arm64")
	targets = common.AppendUnique(targets, "darwin/amd64")
	targets = common.AppendUnique(targets, "darwin/arm64")
	targets = common.AppendUnique(targets, "windows/amd64")
	targets = common.AppendUnique(targets, "windows/386")
	targets = common.AppendUnique(targets, "windows/arm64")
	targets = common.AppendUnique(targets, "wasi/wasm32")
	targets = common.AppendUnique(targets, "dos/8086")
	targets = common.AppendUnique(targets, "c")
	targets = common.AppendUnique(targets, "c/16")
	targets = common.AppendUnique(targets, "c/32")
	targets = common.AppendUnique(targets, "c/64")
	targets = common.AppendUnique(targets, "vm/8")
	targets = common.AppendUnique(targets, "vm/16")
	targets = common.AppendUnique(targets, "vm/32")
	targets = common.AppendUnique(targets, "vm/64")
	for _, target := range targetcfg.RegisteredTriples() {
		targets = common.AppendUnique(targets, target)
	}
	return targets
}

func detectStdlibBaseDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	baseDir := cwd
	search := cwd
	for {
		if common.PathExists(search + "/std/runtime/runtime.go") {
			return search, nil
		}
		parent := common.DirName(search)
		if parent == search || parent == "" {
			break
		}
		search = parent
	}
	return baseDir, nil
}

func appendStdlibRootCandidates(roots []string, include string) []string {
	include = common.TrimTrailingSlash(common.NormalizePath(include))
	if include == "" || include == "-" {
		return roots
	}
	added := false
	if common.PathExists(include + "/runtime/runtime.go") {
		roots = common.AppendUnique(roots, include)
		added = true
	}
	if common.PathExists(include + "/std/runtime/runtime.go") {
		roots = common.AppendUnique(roots, include+"/std")
		added = true
	}
	if !added {
		roots = common.AppendUnique(roots, include)
	}
	return roots
}

func resolveStdlibDiskRoots() ([]string, error) {
	var roots []string
	if compileTarget.StdlibIncludeExplicit {
		for _, include := range compileTarget.StdlibIncludePaths {
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
	dest = common.TrimTrailingSlash(dest)
	if dest == "" {
		return fmt.Errorf("destination path cannot be empty")
	}
	err := os.MkdirAll(dest, 0755)
	if err != nil {
		return err
	}
	extracted := false
	if frontend.ShouldUseEmbeddedStdlib(&compileTarget) {
		names, data := stdlib.WalkEmbedFromFS(".")
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
		names, data := common.WalkDirectory(root, root)
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
	dest = common.TrimTrailingSlash(dest)
	sortNameDataPairs(names, data)
	i := 0
	for i < len(names) {
		rel := common.NormalizePath(names[i])
		if strings.HasPrefix(rel, "./") {
			rel = rel[2:len(rel)]
		}
		if !isSafeRelativePath(rel) {
			return fmt.Errorf("unsafe embedded path %q", names[i])
		}
		outPath := dest + "/" + rel
		parent := common.DirName(outPath)
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
