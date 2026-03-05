//go:build no_frontend && exp_ir_binary

package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"j5.nz/rtg/std/compiler/backend"
	"j5.nz/rtg/std/compiler/binary"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// Target and build tag globals — defaults to host platform
var targetTriple string = runtime.GOOS + "/" + runtime.GOARCH
var targetGOOS string = runtime.GOOS
var targetGOARCH string = runtime.GOARCH
var targetPtrSize int = defaultPtrSize()

func defaultPtrSize() int {
	if runtime.GOARCH == "386" || runtime.GOARCH == "wasm32" || runtime.GOARCH == "armv8m" {
		return 4
	}
	return 8
}

var targetBackend string = "native"       // native, c, or vm
var targetCModel int = 0                  // 16/32/64 when targetBackend==c
var targetWordSize int = defaultPtrSize() // word size in bytes
var buildTags []string
var compilerDebug bool
var stripBinary bool

func main() {
	if len(os.Args) < 2 {
		printHelp(os.Args[0], os.Stderr)
		os.Exit(1)
	}

	outputPath := "output"
	var fromKind string = "irb"
	var fromIRBinaryPath string
	var fromIRTextPath string
	var emitIRAndBinaryPath string
	var entryFiles []string
	var stdinInput bool
	var dashInputCount int
	var extraTags string
	var strictMode bool
	var profileMode bool
	i := 1
	for i < len(os.Args) {
		if os.Args[i] == "-h" || os.Args[i] == "--help" {
			printHelp(os.Args[0], os.Stdout)
			os.Exit(0)
		} else if os.Args[i] == "-o" && i+1 < len(os.Args) {
			outputPath = os.Args[i+1]
			i = i + 2
		} else if (os.Args[i] == "-F" || os.Args[i] == "--from") && i+1 < len(os.Args) {
			fromKind = os.Args[i+1]
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
				targetTriple = target
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
				targetGOOS = "c"
				bits := targetWordSize * 8
				targetGOARCH = fmt.Sprintf("c%d", bits)
				targetTriple = target
			} else {
				if target == "dos/8086" {
					targetTriple = target
					targetGOOS = "dos"
					targetGOARCH = "dos16"
					targetPtrSize = 2
					i = i + 2
					continue
				}
				slashIdx := strings.Index(target, "/")
				if slashIdx < 0 {
					fmt.Fprintf(os.Stderr, "invalid target %q: expected os/arch, dos/8086, c[/16|32|64], or vm/<8|16|32|64>\n", target)
					os.Exit(1)
				}
				targetTriple = target
				targetGOOS = target[0:slashIdx]
				targetGOARCH = target[slashIdx+1:]
				if targetGOARCH == "386" || targetGOARCH == "wasm32" || targetGOARCH == "armv8m" {
					targetPtrSize = 4
				} else {
					targetPtrSize = 8
				}
			}
			i = i + 2
		} else if os.Args[i] == "-from-ir-binary" && i+1 < len(os.Args) {
			fromIRBinaryPath = os.Args[i+1]
			fromKind = "irb"
			i = i + 2
		} else if (os.Args[i] == "-emit-codegen-debug" || os.Args[i] == "-emit-ir-and-binary") && i+1 < len(os.Args) {
			emitIRAndBinaryPath = os.Args[i+1]
			i = i + 2
		} else if os.Args[i] == "-size-analysis" && i+1 < len(os.Args) {
			ir.SizeAnalysisPath = os.Args[i+1]
			i = i + 2
		} else if os.Args[i] == "-tags" && i+1 < len(os.Args) {
			extraTags = os.Args[i+1]
			i = i + 2
		} else if os.Args[i] == "-strict" {
			strictMode = true
			i = i + 1
		} else if os.Args[i] == "-profile" {
			profileMode = true
			i = i + 1
		} else if os.Args[i] == "-debug" {
			compilerDebug = true
			i = i + 1
		} else if os.Args[i] == "-strip" || os.Args[i] == "-s" {
			stripBinary = true
			i = i + 1
		} else if os.Args[i] == "-" {
			dashInputCount = dashInputCount + 1
			i = i + 1
		} else {
			entryFiles = append(entryFiles, common.NormalizePath(os.Args[i]))
			i = i + 1
		}
	}

	if fromKind != "ir" && fromKind != "irb" {
		fmt.Fprintf(os.Stderr, "invalid -F value %q: expected ir or irb\n", fromKind)
		os.Exit(1)
	}
	if dashInputCount > 1 {
		fmt.Fprintf(os.Stderr, "at most one '-' input is allowed\n")
		os.Exit(1)
	}
	if dashInputCount == 1 {
		if fromKind == "ir" {
			if len(entryFiles) > 0 {
				fmt.Fprintf(os.Stderr, "cannot combine -F ir stdin input with IR text file path\n")
				os.Exit(1)
			}
			fromIRTextPath = "-"
		} else {
			stdinInput = true
		}
	}
	if stdinInput && len(entryFiles) > 0 {
		fmt.Fprintf(os.Stderr, "cannot combine '-' stdin input with file path arguments\n")
		os.Exit(1)
	}
	if fromKind == "irb" {
		if stdinInput {
			fmt.Fprintf(os.Stderr, "cannot use '-' with IR binary input; provide a .irb path\n")
			os.Exit(1)
		}
		if fromIRBinaryPath != "" {
			if len(entryFiles) > 0 {
				fmt.Fprintf(os.Stderr, "cannot combine -from-ir-binary with positional input path\n")
				os.Exit(1)
			}
		} else {
			if len(entryFiles) != 1 {
				fmt.Fprintf(os.Stderr, "no_frontend build requires one IR binary input path (-from-ir-binary <path> or positional)\n")
				os.Exit(1)
			}
			fromIRBinaryPath = entryFiles[0]
		}
	} else {
		if fromIRBinaryPath != "" {
			fmt.Fprintf(os.Stderr, "cannot combine -F ir with -from-ir-binary\n")
			os.Exit(1)
		}
		if fromIRTextPath == "-" {
			if len(entryFiles) != 0 {
				fmt.Fprintf(os.Stderr, "-F ir with stdin cannot include file paths\n")
				os.Exit(1)
			}
		} else {
			if len(entryFiles) != 1 {
				fmt.Fprintf(os.Stderr, "-F ir requires exactly one IR text input path (or '-')\n")
				os.Exit(1)
			}
			fromIRTextPath = entryFiles[0]
		}
	}

	// Build active tag set from target + explicit tags.
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
	if ir.SizeAnalysisPath != "" {
		stripBinary = true
	}

	var irmod *ir.IRModule
	if fromKind == "ir" {
		var err error
		irmod, err = binary.ReadIRText(fromIRTextPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading IR text: %v\n", err)
			os.Exit(1)
		}
	} else {
		var err error
		irmod, err = binary.ReadIRBinary(fromIRBinaryPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading IR binary: %v\n", err)
			os.Exit(1)
		}
	}
	if compilerDebug {
		if fromKind == "ir" {
			fmt.Fprintf(os.Stderr, "debug: loaded IR text (%d funcs, %d globals)\n", len(irmod.Funcs), len(irmod.Globals))
		} else {
			fmt.Fprintf(os.Stderr, "debug: loaded IR binary (%d funcs, %d globals)\n", len(irmod.Funcs), len(irmod.Globals))
		}
		fmt.Fprintf(os.Stderr, "debug: generating output (backend=%s, target=%s/%s)\n", targetBackend, targetGOOS, targetGOARCH)
	}
	compileTarget := common.Target{
		Triple:              targetTriple,
		GOOS:                targetGOOS,
		GOARCH:              targetGOARCH,
		PtrSize:             targetPtrSize,
		Backend:             targetBackend,
		CModel:              targetCModel,
		WordSize:            targetWordSize,
		BuildTags:           buildTags,
		Defines:             map[string]string{},
		Strict:              strictMode,
		Profile:             profileMode,
		CompilerDebug:       compilerDebug,
		EmitIRAndBinaryPath: emitIRAndBinaryPath,
		StripBinary:         stripBinary,
	}
	if emitIRAndBinaryPath != "" && !backend.SupportsCodegenDebug(&compileTarget) {
		fmt.Fprintf(os.Stderr, "-emit-codegen-debug is not supported for backend=%s target=%s/%s\n", compileTarget.Backend, compileTarget.GOOS, compileTarget.GOARCH)
		os.Exit(1)
	}
	if err := backend.Generate(&compileTarget, irmod, outputPath); err != nil {
		fmt.Fprintf(os.Stderr, "codegen error: %v\n", err)
		os.Exit(1)
	}
	ir.WriteSizeAnalysis(compileTarget)
}

func printHelp(program string, out *os.File) {
	fmt.Fprintf(out, "Usage: %s [options] <module.irt|module.irb>\n", program)
	fmt.Fprintf(out, "\nThis build is backend-only (built with no_frontend,exp_ir_binary).\n")
	fmt.Fprintf(out, "\nOptions:\n")
	fmt.Fprintf(out, "  -F, --from <kind>      Input kind: ir or irb (default: irb)\n")
	fmt.Fprintf(out, "  -o <path>              Output path (default: output)\n")
	fmt.Fprintf(out, "  -T <target>            Target triple or backend mode\n")
	fmt.Fprintf(out, "  -from-ir-binary <p>    Load binary IR module from path and run codegen\n")
	fmt.Fprintf(out, "                         (equivalent to -F irb <path>)\n")
	fmt.Fprintf(out, "  -emit-codegen-debug <p> Emit separate backend debug text (per-IR-instruction machine bytes where supported)\n")
	fmt.Fprintf(out, "  -tags <a,b,c>          Extra build tags\n")
	fmt.Fprintf(out, "  -strict                Preserve strict-mode metadata in target config\n")
	fmt.Fprintf(out, "  -profile               Preserve profile metadata in target config\n")
	fmt.Fprintf(out, "  -size-analysis <path>  Write per-function size analysis JSON\n")
	fmt.Fprintf(out, "  -debug                 Enable compiler debug logging\n")
	fmt.Fprintf(out, "  -strip, -s             Strip symbol/debug metadata from native binaries\n")
	fmt.Fprintf(out, "  -h, --help             Show this help\n")
	fmt.Fprintf(out, "\nIR text stdin: pass '-' as the input with -F ir\n")
	fmt.Fprintf(out, "\nDefault target: %s/%s\n", runtime.GOOS, runtime.GOARCH)
}
