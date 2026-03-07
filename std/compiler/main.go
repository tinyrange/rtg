//go:build !no_frontend

package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"j5.nz/rtg/std/buildtool"
	"j5.nz/rtg/std/compiler/backend"
	"j5.nz/rtg/std/compiler/backend/irprint"
	"j5.nz/rtg/std/compiler/backend/vm"
	"j5.nz/rtg/std/compiler/binary"
	"j5.nz/rtg/std/compiler/common"
	cfrontend "j5.nz/rtg/std/compiler/frontend/c"
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
	EntryFunc:             "main.main",
}

func defaultPtrSize() int {
	if runtime.GOARCH == "386" || runtime.GOARCH == "rv32" || runtime.GOARCH == "wasm32" || runtime.GOARCH == "armv8m" {
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

func isCAssemblySource(path string) bool {
	return strings.HasSuffix(path, ".S") || strings.HasSuffix(path, ".s")
}

func compileExternalAssemblyObject(tgt common.Target, inputPath string, outputPath string, includePaths []string, systemIncludePaths []string, defines []string, undefs []string) error {
	var cmdName string
	var args []string
	switch {
	case tgt.GOOS == "darwin" && tgt.GOARCH == "arm64":
		cmdName = "cc"
		args = []string{"-c", "-arch", "arm64", "-x", "assembler-with-cpp"}
	case tgt.GOOS == "linux" && tgt.GOARCH == "386":
		cmdName = "clang"
		args = []string{"-target", "i386-unknown-linux-gnu", "-c", "-x", "assembler-with-cpp"}
	default:
		return fmt.Errorf("assembly object compilation is not yet supported for %s/%s", tgt.GOOS, tgt.GOARCH)
	}
	for _, p := range includePaths {
		args = append(args, "-I", p)
	}
	for _, p := range systemIncludePaths {
		args = append(args, "-isystem", p)
	}
	for _, d := range defines {
		args = append(args, "-D", d)
	}
	for _, u := range undefs {
		args = append(args, "-U", u)
	}
	args = append(args, "-o", outputPath, inputPath)
	cmd := exec.Command(cmdName, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("assemble %s: %v", inputPath, err)
	}
	return nil
}

func readStdinSourceToTemp(ext string) error {
	if runTmpSrc == "" {
		pid := fmt.Sprintf("%d", os.Getpid())
		runTmpSrc = tempDirPath() + pathSep() + "rtg-run-" + pid + ext
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

func inferSourceLanguage(explicit string, entryFiles []string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	lang := ""
	for _, path := range entryFiles {
		if strings.HasSuffix(path, ".go") {
			if lang != "" && lang != "go" {
				return "", fmt.Errorf("mixed language inputs: both Go and C sources provided; pass -x to select one language")
			}
			lang = "go"
		} else if strings.HasSuffix(path, ".c") {
			if lang != "" && lang != "c99" {
				return "", fmt.Errorf("mixed language inputs: both Go and C sources provided; pass -x to select one language")
			}
			lang = "c99"
		}
	}
	if lang == "" {
		return "go", nil
	}
	return lang, nil
}

func cModuleNeedsRuntime(irmod *ir.IRModule) bool {
	if irmod == nil {
		return false
	}
	for _, f := range irmod.Funcs {
		for _, inst := range f.Code {
			if inst.Op == ir.OP_CALL_INTRINSIC && inst.Name == "Alloc" {
				return true
			}
		}
	}
	return false
}

func compileCRuntimeSupportIR(baseDir string, target common.Target) (*ir.IRModule, error) {
	path := tempDirPath() + pathSep() + "rtg-c-runtime-" + fmt.Sprintf("%d", os.Getpid()) + ".go"
	defer os.RemoveAll(path)
	if err := os.WriteFile(path, []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		return nil, err
	}
	mod := frontend.ResolveModule(&target, baseDir, []string{path})
	if errs := frontend.ValidateModule(mod); len(errs) > 0 {
		return nil, fmt.Errorf("runtime validation failed: %s", strings.Join(errs, "; "))
	}
	irmod, errs := frontend.CompileModule(target, mod)
	if len(errs) > 0 {
		return nil, fmt.Errorf("runtime compile failed: %s", strings.Join(errs, "; "))
	}
	return irmod, nil
}

func mergeRuntimeSupportIR(dst *ir.IRModule, runtimeIR *ir.IRModule) {
	if dst == nil || runtimeIR == nil {
		return
	}
	globalMap := make(map[int]int)
	for _, g := range runtimeIR.Globals {
		if !strings.HasPrefix(g.Name, "runtime.") {
			continue
		}
		ng := g
		ng.Index = len(dst.Globals)
		globalMap[g.Index] = ng.Index
		dst.Globals = append(dst.Globals, ng)
	}
	for _, f := range runtimeIR.Funcs {
		if !strings.HasPrefix(f.Name, "runtime.") {
			continue
		}
		for i := range f.Code {
			switch f.Code[i].Op {
			case ir.OP_GLOBAL_GET, ir.OP_GLOBAL_SET, ir.OP_GLOBAL_ADDR:
				if newIdx, ok := globalMap[f.Code[i].Arg]; ok {
					f.Code[i].Arg = newIdx
				}
			}
		}
		dst.Funcs = append(dst.Funcs, f)
	}
	dst.Types = append(dst.Types, runtimeIR.Types...)
	if len(runtimeIR.LinkStaticFuncs) > 0 {
		if dst.LinkStaticFuncs == nil {
			dst.LinkStaticFuncs = make(map[string]string)
		}
		for k, v := range runtimeIR.LinkStaticFuncs {
			dst.LinkStaticFuncs[k] = v
		}
	}
	if len(runtimeIR.ZeroCallFuncs) > 0 {
		if dst.ZeroCallFuncs == nil {
			dst.ZeroCallFuncs = make(map[string]bool)
		}
		for k, v := range runtimeIR.ZeroCallFuncs {
			dst.ZeroCallFuncs[k] = v
		}
	}
	if len(runtimeIR.TypeIDs) > 0 {
		if dst.TypeIDs == nil {
			dst.TypeIDs = make(map[string]int)
		}
		for k, v := range runtimeIR.TypeIDs {
			dst.TypeIDs[k] = v
		}
	}
	if len(runtimeIR.MethodTable) > 0 {
		if dst.MethodTable == nil {
			dst.MethodTable = make(map[string]string)
		}
		for k, v := range runtimeIR.MethodTable {
			dst.MethodTable[k] = v
		}
	}
	if len(runtimeIR.IfaceMethods) > 0 {
		if dst.IfaceMethods == nil {
			dst.IfaceMethods = make(map[string][]string)
		}
		for k, v := range runtimeIR.IfaceMethods {
			dst.IfaceMethods[k] = append([]string{}, v...)
		}
	}
	if len(runtimeIR.IfaceMethodRets) > 0 {
		if dst.IfaceMethodRets == nil {
			dst.IfaceMethodRets = make(map[string]int)
		}
		for k, v := range runtimeIR.IfaceMethodRets {
			dst.IfaceMethodRets[k] = v
		}
	}
	if len(runtimeIR.CallbackFuncs) > 0 {
		if dst.CallbackFuncs == nil {
			dst.CallbackFuncs = make(map[string]bool)
		}
		for k, v := range runtimeIR.CallbackFuncs {
			dst.CallbackFuncs[k] = v
		}
	}
}

func isRecognizedSourceLanguage(lang string) bool {
	return lang == "go" || lang == "c99"
}

func extForSourceLanguage(lang string) string {
	if lang == "c99" {
		return ".c"
	}
	return ".go"
}

func serializeCTokens(tokens []cfrontend.Token) string {
	var out strings.Builder
	for _, tok := range tokens {
		if tok.Kind == cfrontend.TokEOF {
			continue
		}
		out.WriteString(tok.Kind.String())
		out.WriteByte(' ')
		out.WriteString(quoteForDebug(tok.Text))
		out.WriteByte('\n')
	}
	return out.String()
}

func quoteForDebug(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			b.WriteString("\\\\")
		case '"':
			b.WriteString("\\\"")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		default:
			if c < 32 || c > 126 {
				hex := "0123456789abcdef"
				b.WriteString("\\x")
				b.WriteByte(hex[(c>>4)&0xf])
				b.WriteByte(hex[c&0xf])
			} else {
				b.WriteByte(c)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
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
	var outputPathExplicit bool
	var entryFiles []string
	var extraTags string
	var parseOnly bool
	var preprocessOnly bool
	var sourceLangExplicit string
	var sourceLang string
	var rawDefineArgs []string
	var cIncludePaths []string
	var cSystemIncludePaths []string
	var cUndefs []string
	var buildTagsPath string
	var emitIRPath string
	var emitIRAndBinaryPath string
	var emitIRBinaryPath string
	var fromIRBinaryPath string
	var fromIRTextPath string
	var profileReportPath string
	var extractStdlibDest string
	var buildFilePath string
	var buildList bool
	var runMode bool
	var objectMode bool
	var testMode bool
	var stdinInput bool
	var dashInputCount int
	var showVersion bool
	var fromKind string = "go"
	var targetIsIR bool
	var programArgs []string
	i := 1
	for i < len(args) {
		arg := args[i]
		if strings.HasPrefix(arg, "-I") && arg != "-I" {
			val := common.NormalizePath(arg[2:])
			if val == "" {
				fmt.Fprintf(os.Stderr, "missing path after -I\n")
				runCleanup()
				os.Exit(1)
			}
			cIncludePaths = append(cIncludePaths, val)
			i = i + 1
			continue
		}
		if strings.HasPrefix(arg, "-U") && arg != "-U" {
			name := strings.TrimSpace(arg[2:])
			if name == "" {
				fmt.Fprintf(os.Stderr, "missing macro name after -U\n")
				runCleanup()
				os.Exit(1)
			}
			cUndefs = append(cUndefs, name)
			i = i + 1
			continue
		}
		switch arg {
		case "-h", "--help":
			printHelp(args[0], os.Stdout)
			os.Exit(0)
		case "-x":
			if i+1 < len(args) {
				sourceLangExplicit = args[i+1]
				if !isRecognizedSourceLanguage(sourceLangExplicit) {
					fmt.Fprintf(os.Stderr, "invalid source language %q: expected go or c99\n", sourceLangExplicit)
					runCleanup()
					os.Exit(1)
				}
				i = i + 2
				continue
			}
		case "-E":
			preprocessOnly = true
			i = i + 1
			continue
		case "-version", "--version":
			showVersion = true
			i = i + 1
			continue
		case "-run":
			runMode = true
			i = i + 1
			continue
		case "-c":
			objectMode = true
			compileTarget.RelocatableObject = true
			i = i + 1
			continue
		case "-test":
			testMode = true
			compileTarget.TestMode = true
			i = i + 1
			continue
		case "-o":
			if i+1 < len(args) {
				outputPath = args[i+1]
				outputPathExplicit = true
				i = i + 2
				continue
			}
		case "-F", "--from":
			if i+1 < len(args) {
				fromKind = args[i+1]
				i = i + 2
				continue
			}
		case "-T", "--to":
			if i+1 < len(args) {
				target := args[i+1]
				if target == "ir" {
					targetIsIR = true
					i = i + 2
					continue
				}
				if err := applyTargetSelection(target, &compileTarget); err != nil {
					fmt.Fprintf(os.Stderr, "invalid target %q: %v\n", target, err)
					runCleanup()
					os.Exit(1)
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
		case "-emit-codegen-debug", "-emit-ir-and-binary":
			if i+1 < len(args) {
				emitIRAndBinaryPath = args[i+1]
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
				rawDefineArgs = append(rawDefineArgs, args[i+1])
				i = i + 2
				continue
			}
		case "-I":
			if i+1 < len(args) {
				val := common.NormalizePath(args[i+1])
				if val == "" {
					fmt.Fprintf(os.Stderr, "missing path after -I\n")
					runCleanup()
					os.Exit(1)
				}
				cIncludePaths = append(cIncludePaths, val)
				i = i + 2
				continue
			}
		case "-isystem":
			if i+1 < len(args) {
				val := common.NormalizePath(args[i+1])
				if val == "" {
					fmt.Fprintf(os.Stderr, "missing path after -isystem\n")
					runCleanup()
					os.Exit(1)
				}
				cSystemIncludePaths = append(cSystemIncludePaths, val)
				i = i + 2
				continue
			}
		case "-U":
			if i+1 < len(args) {
				name := strings.TrimSpace(args[i+1])
				if name == "" {
					fmt.Fprintf(os.Stderr, "missing macro name after -U\n")
					runCleanup()
					os.Exit(1)
				}
				cUndefs = append(cUndefs, name)
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
		case "-buildfile":
			if i+1 < len(args) {
				buildFilePath = common.NormalizePath(args[i+1])
				i = i + 2
				continue
			}
		case "-build-list":
			buildList = true
			i = i + 1
			continue
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
			dashInputCount = dashInputCount + 1
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
	if buildFilePath != "" {
		if err := buildtool.RunFile(buildFilePath, entryFiles, buildList); err != nil {
			fmt.Fprintf(os.Stderr, "build error: %v\n", err)
			runCleanup()
			os.Exit(1)
		}
		runCleanup()
		os.Exit(0)
	}
	if dashInputCount > 1 {
		fmt.Fprintf(os.Stderr, "at most one '-' input is allowed\n")
		runCleanup()
		os.Exit(1)
	}
	if dashInputCount == 1 {
		if fromKind == "ir" {
			if len(entryFiles) > 0 {
				fmt.Fprintf(os.Stderr, "cannot combine -F ir stdin input with IR text file path\n")
				runCleanup()
				os.Exit(1)
			}
			fromIRTextPath = "-"
		} else {
			stdinInput = true
		}
	}
	sourceLang, err = inferSourceLanguage(sourceLangExplicit, entryFiles)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rtg: %v\n", err)
		runCleanup()
		os.Exit(1)
	}
	if sourceLang == "go" {
		for _, path := range entryFiles {
			if strings.HasSuffix(path, ".c") {
				fmt.Fprintf(os.Stderr, "C source %q provided with Go frontend; pass -x c99\n", path)
				runCleanup()
				os.Exit(1)
			}
		}
	} else if sourceLang == "c99" {
		for _, path := range entryFiles {
			if strings.HasSuffix(path, ".go") {
				fmt.Fprintf(os.Stderr, "Go source %q provided with C99 frontend; pass -x go\n", path)
				runCleanup()
				os.Exit(1)
			}
		}
	}
	if sourceLang == "go" {
		for _, raw := range rawDefineArgs {
			key, value, ok := parseDefineArg(raw)
			if !ok {
				fmt.Fprintf(os.Stderr, "invalid -D value %q: expected key=value for Go mode\n", raw)
				runCleanup()
				os.Exit(1)
			}
			compileTarget.Defines[key] = value
		}
	}
	if sourceLang == "c99" && len(compileTarget.Defines) > 0 {
		// C mode does not consume target symbol defines from -D key=value yet.
		compileTarget.Defines = map[string]string{}
	}
	if stdinInput {
		if fromIRBinaryPath != "" {
			fmt.Fprintf(os.Stderr, "cannot use - with -from-ir-binary\n")
			runCleanup()
			os.Exit(1)
		}
		err := readStdinSourceToTemp(extForSourceLanguage(sourceLang))
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
			runTmpSrc = tmpDir + sep + "rtg-run-" + pid + extForSourceLanguage(sourceLang)
		}
		runTmpBin = tmpDir + sep + "rtg-run-" + pid
		if compileTarget.GOOS == "windows" {
			runTmpBin = runTmpBin + ".exe"
		}

		// Read from stdin if no entry files
		if len(entryFiles) == 0 {
			err := readStdinSourceToTemp(extForSourceLanguage(sourceLang))
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
	if preprocessOnly && runMode {
		fmt.Fprintf(os.Stderr, "-E cannot be combined with -run\n")
		runCleanup()
		os.Exit(1)
	}
	if objectMode && runMode {
		fmt.Fprintf(os.Stderr, "-c cannot be combined with -run\n")
		runCleanup()
		os.Exit(1)
	}
	if objectMode && compileTarget.Backend != "native" {
		fmt.Fprintf(os.Stderr, "-c is currently only supported for native targets\n")
		runCleanup()
		os.Exit(1)
	}
	if emitIRPath != "" && emitIRBinaryPath != "" {
		fmt.Fprintf(os.Stderr, "-emit-ir cannot be combined with -emit-ir-binary\n")
		runCleanup()
		os.Exit(1)
	}
	if preprocessOnly && (emitIRPath != "" || emitIRBinaryPath != "") {
		fmt.Fprintf(os.Stderr, "-E cannot be combined with IR emission options\n")
		runCleanup()
		os.Exit(1)
	}

	if fromIRBinaryPath != "" && len(entryFiles) > 0 {
		fmt.Fprintf(os.Stderr, "cannot combine source files with -from-ir-binary\n")
		runCleanup()
		os.Exit(1)
	}
	if fromKind == "ir" {
		if fromIRTextPath == "-" {
			if len(entryFiles) != 0 {
				fmt.Fprintf(os.Stderr, "-F ir with stdin cannot include file paths\n")
				runCleanup()
				os.Exit(1)
			}
		} else {
			if len(entryFiles) != 1 {
				fmt.Fprintf(os.Stderr, "-F ir requires exactly one IR text input path (or '-')\n")
				runCleanup()
				os.Exit(1)
			}
			fromIRTextPath = entryFiles[0]
		}
		entryFiles = nil
	}
	if fromIRBinaryPath != "" && testMode {
		fmt.Fprintf(os.Stderr, "-test is not valid with -from-ir-binary\n")
		runCleanup()
		os.Exit(1)
	}
	if preprocessOnly && fromIRBinaryPath != "" {
		fmt.Fprintf(os.Stderr, "-E is not valid with -from-ir-binary\n")
		runCleanup()
		os.Exit(1)
	}
	if extractStdlibDest == "" && profileReportPath == "" && fromIRBinaryPath == "" && fromIRTextPath == "" && len(entryFiles) == 0 {
		printHelp(os.Args[0], os.Stderr)
		os.Exit(1)
	}

	// Build active tag set from target + explicit tags
	applyBuildTags(&compileTarget, extraTags)
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
		if extractStdlibDest != "" || fromIRBinaryPath != "" || runMode || stdinInput || parseOnly || emitIRPath != "" || emitIRAndBinaryPath != "" || emitIRBinaryPath != "" || buildTagsPath != "" {
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
		if fromIRBinaryPath != "" || len(entryFiles) > 0 || runMode || testMode || stdinInput || parseOnly || emitIRPath != "" || emitIRAndBinaryPath != "" || emitIRBinaryPath != "" || buildTagsPath != "" {
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
	} else if fromIRTextPath != "" {
		if parseOnly {
			fmt.Fprintf(os.Stderr, "-parse-only is not valid with -F ir\n")
			runCleanup()
			os.Exit(1)
		}
		if buildTagsPath != "" {
			fmt.Fprintf(os.Stderr, "-list-build-tags is not valid with -F ir\n")
			runCleanup()
			os.Exit(1)
		}
		var err error
		irmod, err = binary.ReadIRText(fromIRTextPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading IR text: %v\n", err)
			runCleanup()
			os.Exit(1)
		}
		if compileTarget.CompilerDebug {
			fmt.Fprintf(os.Stderr, "debug: loaded IR text (%d funcs, %d globals)\n", len(irmod.Funcs), len(irmod.Globals))
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

		if sourceLang == "c99" {
			if buildTagsPath != "" {
				fmt.Fprintf(os.Stderr, "-list-build-tags is only supported for Go frontend\n")
				runCleanup()
				os.Exit(1)
			}
			if parseOnly && preprocessOnly {
				fmt.Fprintf(os.Stderr, "cannot combine -parse-only with -E\n")
				runCleanup()
				os.Exit(1)
			}
			if len(entryFiles) == 0 {
				fmt.Fprintf(os.Stderr, "no C input files provided\n")
				runCleanup()
				os.Exit(1)
			}
			asmCount := 0
			for _, path := range entryFiles {
				if isCAssemblySource(path) {
					asmCount++
				}
			}
			if asmCount > 0 {
				if !objectMode {
					fmt.Fprintf(os.Stderr, "assembly inputs currently require -c object mode\n")
					runCleanup()
					os.Exit(1)
				}
				if len(entryFiles) != 1 || asmCount != len(entryFiles) {
					fmt.Fprintf(os.Stderr, "assembly inputs currently support exactly one .S/.s source per invocation with no mixed C sources\n")
					runCleanup()
					os.Exit(1)
				}
				if preprocessOnly || parseOnly {
					fmt.Fprintf(os.Stderr, "assembly inputs do not support -E or -parse-only\n")
					runCleanup()
					os.Exit(1)
				}
				err := compileExternalAssemblyObject(compileTarget, entryFiles[0], outputPath, cIncludePaths, cSystemIncludePaths, rawDefineArgs, cUndefs)
				if err != nil {
					fmt.Fprintf(os.Stderr, "assembly error: %v\n", err)
					runCleanup()
					os.Exit(1)
				}
				runCleanup()
				os.Exit(0)
			}
			if parseOnly && emitIRPath != "" {
				fmt.Fprintf(os.Stderr, "-emit-ir is not valid with -parse-only\n")
				runCleanup()
				os.Exit(1)
			}
			ppOpts := cfrontend.Options{
				IncludePaths:       cIncludePaths,
				SystemIncludePaths: cSystemIncludePaths,
				Defines:            rawDefineArgs,
				Undefs:             cUndefs,
				TargetOS:           compileTarget.GOOS,
				TargetArch:         compileTarget.GOARCH,
				PtrSize:            compileTarget.PtrSize,
				Hosted:             len(cSystemIncludePaths) > 0,
			}
			var preprocessOut strings.Builder
			var parseOut strings.Builder
			parseErrors := false
			var cUnits []cfrontend.Unit
			for _, path := range entryFiles {
				if !strings.HasSuffix(path, ".c") {
					fmt.Fprintf(os.Stderr, "unsupported C99 input path %q (expected .c source file)\n", path)
					runCleanup()
					os.Exit(1)
				}
				pp := cfrontend.NewPreprocessor(ppOpts)
				tokens, err := pp.ProcessFile(path)
				if err != nil {
					fmt.Fprintf(os.Stderr, "preprocess error: %v\n", err)
					runCleanup()
					os.Exit(1)
				}
				if preprocessOnly {
					preprocessOut.WriteString(serializeCTokens(tokens))
					continue
				}
				parser := cfrontend.NewParser(tokens)
				tu := parser.ParseTranslationUnit()
				if errs := parser.Errors(); len(errs) > 0 {
					parseErrors = true
					fmt.Fprintf(os.Stderr, "\n%d parse errors in %s:\n", len(errs), path)
					for _, pe := range errs {
						fmt.Fprintf(os.Stderr, "  %s\n", pe)
					}
					continue
				}
				if parseOnly {
					if outputPathExplicit {
						if len(entryFiles) > 1 {
							parseOut.WriteString("== ")
							parseOut.WriteString(path)
							parseOut.WriteString(" ==\n")
						}
						parseOut.WriteString(cfrontend.FormatNode(tu))
					}
					continue
				}
				cUnits = append(cUnits, cfrontend.Unit{File: path, Root: tu})
			}
			if parseErrors {
				runCleanup()
				os.Exit(1)
			}
			if preprocessOnly || parseOnly {
				if outputPathExplicit {
					content := ""
					if preprocessOnly {
						content = preprocessOut.String()
					} else if parseOnly {
						content = parseOut.String()
					}
					err := os.WriteFile(outputPath, []byte(content), 0644)
					if err != nil {
						fmt.Fprintf(os.Stderr, "error writing output: %v\n", err)
						runCleanup()
						os.Exit(1)
					}
				} else if preprocessOnly {
					fmt.Fprintf(os.Stdout, "%s", preprocessOut.String())
				}
				runCleanup()
				os.Exit(0)
			}
			if compileTarget.CompilerDebug {
				fmt.Fprintf(os.Stderr, "debug: compiling C99 translation units to IR (%d files)\n", len(cUnits))
			}
			var errs []string
			irmod, errs = cfrontend.CompileUnits(compileTarget, cUnits)
			if len(errs) > 0 {
				fmt.Fprintf(os.Stderr, "\n%d compile errors:\n", len(errs))
				for _, e := range errs {
					fmt.Fprintf(os.Stderr, "  %s\n", e)
				}
				runCleanup()
				os.Exit(1)
			}
			if cModuleNeedsRuntime(irmod) {
				if compileTarget.CompilerDebug {
					fmt.Fprintf(os.Stderr, "debug: merging runtime support into C99 IR\n")
				}
				runtimeIR, err := compileCRuntimeSupportIR(baseDir, compileTarget)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error preparing C runtime support: %v\n", err)
					runCleanup()
					os.Exit(1)
				}
				mergeRuntimeSupportIR(irmod, runtimeIR)
			}
			traceExit(20)
		} else {
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

			if preprocessOnly {
				fmt.Fprintf(os.Stderr, "-E is only supported for C99 frontend; pass -x c99\n")
				runCleanup()
				os.Exit(1)
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
		}

		if compileTarget.CompilerDebug {
			fmt.Fprintf(os.Stderr, "debug: IR compiled (%d funcs, %d globals)\n", len(irmod.Funcs), len(irmod.Globals))
		}
		traceExit(30)
		if !compileTarget.RelocatableObject {
			ir.EliminateDeadFunctions(irmod)
			if compileTarget.CompilerDebug {
				fmt.Fprintf(os.Stderr, "debug: DCE done (%d funcs remaining)\n", len(irmod.Funcs))
			}
		} else if compileTarget.CompilerDebug {
			fmt.Fprintf(os.Stderr, "debug: skipping DCE for relocatable object output\n")
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
	if compileTarget.Backend == "vm" && emitIRPath == "" && !targetIsIR {
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
			fmt.Fprintf(os.Stderr, "debug: generating output (backend=%s, target=%s/%s, emit=ir-pretty)\n", compileTarget.Backend, compileTarget.GOOS, compileTarget.GOARCH)
		} else if targetIsIR {
			fmt.Fprintf(os.Stderr, "debug: generating output (backend=%s, target=%s/%s, emit=ir-text)\n", compileTarget.Backend, compileTarget.GOOS, compileTarget.GOARCH)
		} else {
			fmt.Fprintf(os.Stderr, "debug: generating output (backend=%s, target=%s/%s)\n", compileTarget.Backend, compileTarget.GOOS, compileTarget.GOARCH)
		}
	}
	if emitIRAndBinaryPath != "" {
		if !backend.SupportsCodegenDebug(&compileTarget) {
			fmt.Fprintf(os.Stderr, "-emit-codegen-debug is not supported for backend=%s target=%s/%s\n", compileTarget.Backend, compileTarget.GOOS, compileTarget.GOARCH)
			runCleanup()
			os.Exit(1)
		}
		compileTarget.EmitIRAndBinaryPath = emitIRAndBinaryPath
	}
	if emitIRPath != "" {
		err = irprint.Generate(irmod, emitIRPath)
	} else if targetIsIR {
		err = binary.WriteIRText(irmod, outputPath)
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
	if compileTarget.Backend == "vm" && emitIRPath == "" && !targetIsIR {
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

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneTargetForCompileAs(base *common.Target) *common.Target {
	out := &common.Target{}
	if base == nil {
		return out
	}
	out.Triple = base.Triple
	out.GOOS = base.GOOS
	out.GOARCH = base.GOARCH
	out.PtrSize = base.PtrSize
	out.Backend = base.Backend
	out.CModel = base.CModel
	out.WordSize = base.WordSize
	out.Defines = cloneStringMap(base.Defines)
	out.Strict = base.Strict
	out.CompilerDebug = base.CompilerDebug
	out.EmitIRAndBinaryPath = base.EmitIRAndBinaryPath
	out.StripBinary = base.StripBinary
	if len(base.StdlibIncludePaths) > 0 {
		out.StdlibIncludePaths = append([]string{}, base.StdlibIncludePaths...)
	}
	out.StdlibIncludeExplicit = base.StdlibIncludeExplicit
	out.StdlibIncludeEmbedded = base.StdlibIncludeEmbedded
	out.EntryFunc = base.EntryFunc
	out.BuildTags = nil
	out.Profile = false
	out.TestMode = false
	out.CompileAsArtifacts = nil
	return out
}

func applyBuildTags(tgt *common.Target, extraTags string) {
	tgt.BuildTags = nil
	if tgt.Backend == "c" {
		tgt.BuildTags = append(tgt.BuildTags, "c")
		tgt.BuildTags = append(tgt.BuildTags, fmt.Sprintf("c%d", tgt.CModel))
	} else if tgt.GOOS == "wasi" && tgt.GOARCH == "wasm32" {
		tgt.BuildTags = append(tgt.BuildTags, "wasi")
		tgt.BuildTags = append(tgt.BuildTags, "wasm32")
	} else {
		tgt.BuildTags = append(tgt.BuildTags, tgt.GOOS)
		tgt.BuildTags = append(tgt.BuildTags, tgt.GOARCH)
	}
	if extraTags != "" {
		parts := strings.Split(extraTags, ",")
		for _, t := range parts {
			if t != "" {
				tgt.BuildTags = append(tgt.BuildTags, t)
			}
		}
	}
	tgt.BuildTags = append(tgt.BuildTags, "rtg")
}

func applyTargetSelection(targetName string, cfg *common.Target) error {
	if targetName == "c" || strings.HasPrefix(targetName, "c/") {
		cfg.Triple = targetName
		cfg.Backend = "c"
		cfg.CModel = 64
		if strings.HasPrefix(targetName, "c/") {
			model := targetName[2:]
			if model == "16" {
				cfg.CModel = 16
			} else if model == "32" {
				cfg.CModel = 32
			} else if model == "64" {
				cfg.CModel = 64
			} else {
				return fmt.Errorf("expected c, c/16, c/32, or c/64")
			}
		}
		if cfg.CModel == 16 {
			cfg.PtrSize = 2
		} else if cfg.CModel == 32 {
			cfg.PtrSize = 4
		} else {
			cfg.PtrSize = 8
		}
		cfg.WordSize = cfg.PtrSize
		cfg.GOOS = "c"
		cfg.GOARCH = fmt.Sprintf("c%d", cfg.CModel)
		return nil
	}
	if targetName == "ir" {
		return fmt.Errorf("target %q is no longer supported; use -emit-ir <path> with a concrete -T <target>", targetName)
	}
	if strings.HasPrefix(targetName, "vm/") {
		cfg.Triple = targetName
		cfg.Backend = "vm"
		model := targetName[3:]
		if model == "8" {
			cfg.WordSize = 1
			cfg.PtrSize = 2
		} else if model == "16" {
			cfg.WordSize = 2
			cfg.PtrSize = 2
		} else if model == "32" {
			cfg.WordSize = 4
			cfg.PtrSize = 4
		} else if model == "64" {
			cfg.WordSize = 8
			cfg.PtrSize = 8
		} else {
			return fmt.Errorf("expected vm/8, vm/16, vm/32, or vm/64")
		}
		cfg.GOOS = "c"
		bits := cfg.WordSize * 8
		cfg.GOARCH = fmt.Sprintf("c%d", bits)
		return nil
	}
	_, handledByTargetPkg, err := targetcfg.Apply(targetName, cfg)
	if handledByTargetPkg {
		if err != nil {
			return err
		}
		cfg.Triple = targetName
		return nil
	}
	if targetName == "dos/8086" {
		cfg.Triple = targetName
		cfg.Backend = "native"
		cfg.GOOS = "dos"
		cfg.GOARCH = "dos16"
		cfg.PtrSize = 2
		cfg.WordSize = 2
		return nil
	}
	slashIdx := strings.Index(targetName, "/")
	if slashIdx < 0 {
		return fmt.Errorf("expected os/arch, dos/8086, c[/16|32|64], or vm/<8|16|32|64>")
	}
	cfg.Backend = "native"
	cfg.GOOS = targetName[0:slashIdx]
	cfg.GOARCH = targetName[slashIdx+1:]
	cfg.Triple = targetName
	if cfg.GOARCH == "386" || cfg.GOARCH == "rv32" || cfg.GOARCH == "wasm32" || cfg.GOARCH == "armv8m" {
		cfg.PtrSize = 4
	} else {
		cfg.PtrSize = 8
	}
	cfg.WordSize = cfg.PtrSize
	return nil
}

func compileAsOutputExt(tgt *common.Target) string {
	if tgt == nil {
		return ""
	}
	if tgt.Backend == "c" {
		return ".c"
	}
	if tgt.GOOS == "windows" {
		return ".exe"
	}
	if tgt.GOOS == "wasi" && tgt.GOARCH == "wasm32" {
		return ".wasm"
	}
	return ""
}

func sanitizeCompileAsName(name string) string {
	if name == "" {
		return "compileas"
	}
	var out []byte
	for i := 0; i < len(name); i++ {
		ch := name[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '.' || ch == '_' || ch == '-' {
			out = append(out, ch)
		} else {
			out = append(out, '_')
		}
	}
	return string(out)
}

func summarizeErrors(errs []string) string {
	if len(errs) == 0 {
		return ""
	}
	limit := 3
	if len(errs) < limit {
		limit = len(errs)
	}
	out := ""
	for i := 0; i < limit; i++ {
		if i > 0 {
			out = out + "; "
		}
		out = out + errs[i]
	}
	if len(errs) > limit {
		out = out + fmt.Sprintf("; ... (%d more)", len(errs)-limit)
	}
	return out
}

func readCompileAsArtifact(path string) ([]byte, error) {
	payload, err := os.ReadFile(path)
	if err == nil {
		return payload, nil
	}
	// Some selfhosted outputs can land with an unreadable initial mode until chmod.
	_ = os.Chmod(path, 0600)
	payload, err2 := os.ReadFile(path)
	if err2 == nil {
		return payload, nil
	}
	return nil, err
}

func buildCompileAsArtifacts(baseTarget *common.Target, baseDir string, entryFiles []string, extraTags string, specs []frontend.CompileAsSpec) (map[string]string, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	tmpDir := tempDirPath() + pathSep() + "rtg-compileas-" + fmt.Sprintf("%d", os.Getpid())
	_ = os.RemoveAll(tmpDir)
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	artifacts := make(map[string]string, len(specs))
	for i, spec := range specs {
		innerTarget := cloneTargetForCompileAs(baseTarget)
		innerTarget.EntryFunc = spec.EntryFunc
		if err := applyTargetSelection(spec.Target, innerTarget); err != nil {
			return nil, fmt.Errorf("id=%s target=%s: %v", spec.ID, spec.Target, err)
		}
		if innerTarget.Backend == "vm" {
			return nil, fmt.Errorf("id=%s target=%s: vm backend is not supported by //rtg:compileas", spec.ID, spec.Target)
		}
		if innerTarget.Backend == "c" {
			return nil, fmt.Errorf("id=%s target=%s: c backend is not supported by //rtg:compileas", spec.ID, spec.Target)
		}
		applyBuildTags(innerTarget, extraTags)

		frontend.ResetDiscoveredBuildTags()
		innerMod := frontend.ResolveModule(innerTarget, baseDir, entryFiles)
		if valErrs := frontend.ValidateModule(innerMod); len(valErrs) > 0 {
			return nil, fmt.Errorf("id=%s target=%s validation failed: %s", spec.ID, spec.Target, summarizeErrors(valErrs))
		}
		innerIR, compileErrs := frontend.CompileModule(*innerTarget, innerMod)
		if len(compileErrs) > 0 {
			return nil, fmt.Errorf("id=%s target=%s compile failed: %s", spec.ID, spec.Target, summarizeErrors(compileErrs))
		}
		ir.EliminateDeadFunctions(innerIR)

		outPath := fmt.Sprintf("%s/%03d_%s%s", tmpDir, i, sanitizeCompileAsName(spec.ID), compileAsOutputExt(innerTarget))
		if err := backend.Generate(innerTarget, innerIR, outPath); err != nil {
			return nil, fmt.Errorf("id=%s target=%s codegen failed: %v", spec.ID, spec.Target, err)
		}
		payload, err := readCompileAsArtifact(outPath)
		if err != nil {
			return nil, fmt.Errorf("id=%s target=%s read artifact: %v", spec.ID, spec.Target, err)
		}
		artifacts[spec.ArtifactVar] = string(payload)
	}
	return artifacts, nil
}

func printHelp(program string, out *os.File) {
	fmt.Fprintf(out, "Usage: %s [options] <file.go|file.c> [more files...]\n", program)
	fmt.Fprintf(out, "\nOptions:\n")
	fmt.Fprintf(out, "  -o <path>              Output path (default: output)\n")
	fmt.Fprintf(out, "  -x <go|c99>            Select source frontend (default: inferred from file extension, fallback go)\n")
	fmt.Fprintf(out, "  -E                     C99 mode: preprocess only and emit tokens (stdout unless -o is set)\n")
	fmt.Fprintf(out, "  -T <target>            Target triple or backend mode\n")
	fmt.Fprintf(out, "  -emit-ir <path>        Emit textual IR for the selected target instead of native/C/VM output\n")
	fmt.Fprintf(out, "  -c                     Emit a relocatable object for supported native targets\n")
	fmt.Fprintf(out, "  -tags <a,b,c>          Extra build tags\n")
	fmt.Fprintf(out, "  -D <v>                 Go mode: key=value global define; C99 mode: macro define NAME or NAME=VALUE\n")
	fmt.Fprintf(out, "  -U <name>              C99 mode: undefine macro\n")
	fmt.Fprintf(out, "  -I<dir>, -I <dir>      C99 mode: add quote include search path\n")
	fmt.Fprintf(out, "  -isystem <dir>         C99 mode: add system include search path\n")
	fmt.Fprintf(out, "  -target-file <path>    Load a single-file target definition before -T resolution\n")
	fmt.Fprintf(out, "  -target-root <path>    Recursively load *.go target definitions from a directory\n")
	fmt.Fprintf(out, "  -buildfile <path>      Run build targets from a build file instead of compiling sources\n")
	fmt.Fprintf(out, "  -build-list            List targets from -buildfile and exit\n")
	fmt.Fprintf(out, "  -include <path|->      Add stdlib search root; first -include disables default embedded stdlib, -include - re-enables it\n")
	fmt.Fprintf(out, "  -extract-stdlib <dest> Extract standard library files into destination directory and exit\n")
	fmt.Fprintf(out, "  -parse-only            Parse and resolve imports only (no codegen)\n")
	fmt.Fprintf(out, "  -strict                Reject RTG-only language extensions in user packages\n")
	fmt.Fprintf(out, "  -profile               Enable profiling (compiler/target methods+functions default-on; //rtg:noprofile opts out; //rtg:profile opts in elsewhere)\n")
	fmt.Fprintf(out, "  -profile-report <p>    Read profile records from path and print aggregated timing and allocation trees\n")
	if binary.IrBinaryEnabled {
		fmt.Fprintf(out, "  -emit-ir-binary <p>    Compile source and write binary IR module to path\n")
		fmt.Fprintf(out, "  -from-ir-binary <p>    Load binary IR module from path and run codegen\n")
	}
	fmt.Fprintf(out, "  -list-build-tags <p>   Write discovered build tags (one per line)\n")
	fmt.Fprintf(out, "  -run                   Compile and run the output binary\n")
	fmt.Fprintf(out, "  -test                  Build test binary with synthetic test runner main\n")
	fmt.Fprintf(out, "  -size-analysis <path>  Write per-function size analysis JSON\n")
	fmt.Fprintf(out, "  -version, --version    Print compiler stamp\n")
	fmt.Fprintf(out, "  -debug                 Enable compiler debug logging\n")
	fmt.Fprintf(out, "  -strip, -s             Strip symbol/debug metadata from native binaries\n")
	fmt.Fprintf(out, "  -h, --help             Show this help\n")
	fmt.Fprintf(out, "\nIR text stdin: pass '-' as the input with -F ir\n")
	fmt.Fprintf(out, "\nDefault target: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(out, "\nPossible -T values:\n")
	for _, target := range possibleTargets() {
		fmt.Fprintf(out, "  %s\n", target)
	}
}

type profileTreeNode struct {
	Hash     uint32
	Name     string
	Total    uint64
	Calls    uint64
	Children []*profileTreeNode
}

type profileEdgeStat struct {
	Parent uint32
	Child  uint32
	Total  uint64
	Calls  uint64
}

const (
	profileRecordSizeV1    = 12
	profileRecordSizeV2    = 16
	profileHeaderSizeV2    = 8
	profileRecordKindTime  = 1
	profileRecordKindAlloc = 2
)

func runProfileReport(profilePath string, entryFiles []string) error {
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return err
	}
	recordSize := profileRecordSizeV1
	recordStart := 0
	typedRecords := false
	if len(data) >= profileHeaderSizeV2 && data[0] == 'R' && data[1] == 'T' && data[2] == 'P' && data[3] == '2' {
		recordSize = profileRecordSizeV2
		recordStart = profileHeaderSizeV2
		typedRecords = true
	}
	if len(data) < recordStart+recordSize {
		fmt.Fprintf(os.Stdout, "Profile report for %s\n", profilePath)
		fmt.Fprintf(os.Stdout, "no records\n")
		return nil
	}
	limit := len(data) - ((len(data) - recordStart) % recordSize)
	edgeTimeTotals := make(map[uint64]uint64)
	edgeTimeCalls := make(map[uint64]uint64)
	edgeAllocTotals := make(map[uint64]uint64)
	edgeAllocCalls := make(map[uint64]uint64)
	calleeSeen := make(map[uint32]bool)
	timeCalleeSeen := make(map[uint32]bool)
	allocCalleeSeen := make(map[uint32]bool)
	totalNS := uint64(0)
	totalAllocBytes := uint64(0)
	records := 0
	timeRecords := 0
	allocSamples := 0
	ignoredKindRecords := 0
	i := recordStart
	for i+recordSize <= limit {
		methodHash := common.GetU32(data[i : i+4])
		parentHash := common.GetU32(data[i+4 : i+8])
		value := common.GetU32(data[i+8 : i+12])
		kind := uint32(profileRecordKindTime)
		if typedRecords {
			kind = common.GetU32(data[i+12 : i+16])
		}
		key := (uint64(parentHash) << 32) | uint64(methodHash)
		switch kind {
		case profileRecordKindTime:
			edgeTimeTotals[key] = edgeTimeTotals[key] + uint64(value)
			edgeTimeCalls[key] = edgeTimeCalls[key] + 1
			timeCalleeSeen[methodHash] = true
			calleeSeen[methodHash] = true
			totalNS = totalNS + uint64(value)
			timeRecords++
			records++
		case profileRecordKindAlloc:
			edgeAllocTotals[key] = edgeAllocTotals[key] + uint64(value)
			edgeAllocCalls[key] = edgeAllocCalls[key] + 1
			allocCalleeSeen[methodHash] = true
			calleeSeen[methodHash] = true
			totalAllocBytes = totalAllocBytes + uint64(value)
			allocSamples++
			records++
		default:
			ignoredKindRecords++
		}
		i = i + recordSize
	}

	nameByHash := make(map[uint32]string)
	if len(entryFiles) > 0 {
		mapped, err := collectProfileCallableNameHashes(entryFiles)
		if err != nil {
			return err
		}
		nameByHash = mapped
	}

	buildChildrenByParent := func(totals map[uint64]uint64, calls map[uint64]uint64) map[uint32][]profileEdgeStat {
		childrenByParent := make(map[uint32][]profileEdgeStat)
		for key, total := range totals {
			parentHash := uint32(key >> 32)
			methodHash := uint32(key)
			childrenByParent[parentHash] = append(childrenByParent[parentHash], profileEdgeStat{
				Parent: parentHash,
				Child:  methodHash,
				Total:  total,
				Calls:  calls[key],
			})
		}
		for parent, edges := range childrenByParent {
			sortProfileEdges(edges)
			childrenByParent[parent] = edges
		}
		return childrenByParent
	}

	childrenByParent := buildChildrenByParent(edgeTimeTotals, edgeTimeCalls)
	allocChildrenByParent := buildChildrenByParent(edgeAllocTotals, edgeAllocCalls)

	root := buildProfileTree(childrenByParent, timeCalleeSeen, nameByHash)
	allocRoot := buildProfileTree(allocChildrenByParent, allocCalleeSeen, nameByHash)

	fmt.Fprintf(os.Stdout, "Profile report for %s\n", profilePath)
	fmt.Fprintf(os.Stdout, "records=%d unique=%d total_ns=%d\n", records, len(calleeSeen), totalNS)
	profilePrintTree(root, "")
	if allocSamples > 0 {
		fmt.Fprintf(os.Stdout, "Allocation report\n")
		fmt.Fprintf(os.Stdout, "alloc_samples=%d total_alloc_bytes=%d\n", allocSamples, totalAllocBytes)
		profilePrintAllocTree(allocRoot, "")
	}
	if len(data) != limit {
		fmt.Fprintf(os.Stdout, "note: ignored %d trailing bytes (incomplete record)\n", len(data)-limit)
	}
	if ignoredKindRecords > 0 {
		fmt.Fprintf(os.Stdout, "note: ignored %d records with unknown kind\n", ignoredKindRecords)
	}
	if len(entryFiles) > 0 && len(nameByHash) == 0 {
		fmt.Fprintf(os.Stdout, "note: no functions or methods discovered in provided source inputs\n")
	}
	if timeRecords == 0 && allocSamples > 0 {
		fmt.Fprintf(os.Stdout, "note: file contains allocation samples but no timing records\n")
	}
	return nil
}

func profilePrintAllocTree(node *profileTreeNode, prefix string) {
	for i, child := range node.Children {
		last := i == len(node.Children)-1
		branch := "|- "
		nextPrefix := prefix + "|  "
		if last {
			branch = "\\- "
			nextPrefix = prefix + "   "
		}
		avg := uint64(0)
		if child.Calls > 0 {
			avg = child.Total / child.Calls
		}
		fmt.Fprintf(os.Stdout, "%s%s%s bytes=%d calls=%d avg=%dB\n", prefix, branch, child.Name, child.Total, child.Calls, avg)
		profilePrintAllocTree(child, nextPrefix)
	}
}

func buildProfileTree(childrenByParent map[uint32][]profileEdgeStat, calleeSeen map[uint32]bool, nameByHash map[uint32]string) *profileTreeNode {
	root := &profileTreeNode{Name: "<root>"}
	visited := make(map[uint32]bool)
	for _, edge := range childrenByParent[0] {
		child := buildProfileTreeNode(edge, childrenByParent, nameByHash, visited)
		root.Children = append(root.Children, child)
		root.Total = root.Total + child.Total
	}
	var parentHashes []uint32
	for parent := range childrenByParent {
		if parent == 0 || calleeSeen[parent] {
			continue
		}
		parentHashes = append(parentHashes, parent)
	}
	sortU32s(parentHashes)
	for _, parent := range parentHashes {
		node := &profileTreeNode{
			Hash: parent,
			Name: profileNameForHash(parent, nameByHash),
		}
		if visited[parent] {
			continue
		}
		visited[parent] = true
		profileTreeAppendChildren(node, parent, childrenByParent, nameByHash, visited)
		visited[parent] = false
		root.Children = append(root.Children, node)
		root.Total = root.Total + node.Total
	}
	sortProfileTree(root)
	return root
}

func buildProfileTreeNode(edge profileEdgeStat, childrenByParent map[uint32][]profileEdgeStat, nameByHash map[uint32]string, visited map[uint32]bool) *profileTreeNode {
	node := &profileTreeNode{
		Hash:  edge.Child,
		Name:  profileNameForHash(edge.Child, nameByHash),
		Total: edge.Total,
		Calls: edge.Calls,
	}
	if visited[edge.Child] {
		return node
	}
	visited[edge.Child] = true
	profileTreeAppendChildren(node, edge.Child, childrenByParent, nameByHash, visited)
	visited[edge.Child] = false
	return node
}

func profileTreeAppendChildren(node *profileTreeNode, parentHash uint32, childrenByParent map[uint32][]profileEdgeStat, nameByHash map[uint32]string, visited map[uint32]bool) {
	edges := childrenByParent[parentHash]
	for _, edge := range edges {
		child := buildProfileTreeNode(edge, childrenByParent, nameByHash, visited)
		node.Children = append(node.Children, child)
		if node.Calls == 0 {
			// Parent-only synthetic nodes don't have direct edge stats.
			node.Total = node.Total + child.Total
		}
	}
}

func sortProfileTree(node *profileTreeNode) {
	sortProfileTreeChildren(node.Children)
	for _, child := range node.Children {
		sortProfileTree(child)
	}
}

func sortProfileEdges(edges []profileEdgeStat) {
	i := 1
	for i < len(edges) {
		j := i
		for j > 0 && profileEdgeLess(edges[j], edges[j-1]) {
			edges[j], edges[j-1] = edges[j-1], edges[j]
			j = j - 1
		}
		i++
	}
}

func profileEdgeLess(left profileEdgeStat, right profileEdgeStat) bool {
	if left.Total != right.Total {
		return left.Total > right.Total
	}
	if left.Child != right.Child {
		return left.Child < right.Child
	}
	return left.Parent < right.Parent
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

func sortU32s(values []uint32) {
	i := 1
	for i < len(values) {
		j := i
		for j > 0 && values[j] < values[j-1] {
			values[j], values[j-1] = values[j-1], values[j]
			j = j - 1
		}
		i++
	}
}

func profileNameForHash(hash uint32, nameByHash map[uint32]string) string {
	if hash == 0 {
		return "<root>"
	}
	if name, ok := nameByHash[hash]; ok && name != "" {
		return name
	}
	return fmt.Sprintf("0x%08x", hash)
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
		avg := uint64(0)
		if child.Calls > 0 {
			avg = child.Total / child.Calls
		}
		if child.Calls > 0 {
			fmt.Fprintf(os.Stdout, "%s%s%s total=%dns calls=%d avg=%dns\n", prefix, branch, child.Name, child.Total, child.Calls, avg)
		} else if len(child.Children) == 0 {
			// Keep explicit zero-call leaf records stable for tooling.
			avg := uint64(0)
			fmt.Fprintf(os.Stdout, "%s%s%s total=%dns calls=%d avg=%dns\n", prefix, branch, child.Name, child.Total, child.Calls, avg)
		} else {
			fmt.Fprintf(os.Stdout, "%s%s%s total=%dns\n", prefix, branch, child.Name, child.Total)
		}
		profilePrintTree(child, nextPrefix)
	}
}

func collectProfileCallableNameHashes(entryFiles []string) (map[uint32]string, error) {
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
	for _, qname := range frontend.CollectCallableQualNames(mod) {
		out[profileHash32(qname)] = qname
	}
	return out, nil
}

func profileHash32(name string) uint32 {
	var h uint32 = (uint32(0x811c) << 16) | uint32(0x9dc5)
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
	targets = common.AppendUnique(targets, "linux/rv64")
	targets = common.AppendUnique(targets, "linux/rv32")
	targets = common.AppendUnique(targets, "darwin/amd64")
	targets = common.AppendUnique(targets, "darwin/arm64")
	targets = common.AppendUnique(targets, "windows/amd64")
	targets = common.AppendUnique(targets, "windows/386")
	targets = common.AppendUnique(targets, "windows/arm64")
	targets = common.AppendUnique(targets, "wasi/wasm32")
	targets = common.AppendUnique(targets, "dos/8086")
	targets = common.AppendUnique(targets, "ir")
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
