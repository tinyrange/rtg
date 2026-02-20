//go:build no_frontend && exp_ir_binary

package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// Target and build tag globals — defaults to host platform
var target.GOOS string = runtime.GOOS
var targetGOARCH string = runtime.GOARCH
var targetPtrSize int = defaultPtrSize()

func defaultPtrSize() int {
	if runtime.GOARCH == "386" || runtime.GOARCH == "wasm32" {
		return 4
	}
	return 8
}

var targetBackend string = "native"       // native, c, ir, or vm
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
	var fromIRBinaryPath string
	var extraTags string
	i := 1
	for i < len(os.Args) {
		if os.Args[i] == "-h" || os.Args[i] == "--help" {
			printHelp(os.Args[0], os.Stdout)
			os.Exit(0)
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
				target.GOOS = "c"
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
				target.GOOS = "c"
				bits := targetWordSize * 8
				targetGOARCH = fmt.Sprintf("c%d", bits)
			} else {
				if target == "dos/8086" {
					target.GOOS = "dos"
					targetGOARCH = "dos16"
					targetPtrSize = 2
					i = i + 2
					continue
				}
				slashIdx := strings.Index(target, "/")
				if slashIdx < 0 {
					fmt.Fprintf(os.Stderr, "invalid target %q: expected os/arch, dos/8086, c[/16|32|64], ir, or vm/<8|16|32|64>\n", target)
					os.Exit(1)
				}
				target.GOOS = target[0:slashIdx]
				targetGOARCH = target[slashIdx+1:]
				if targetGOARCH == "386" || targetGOARCH == "wasm32" {
					targetPtrSize = 4
				} else {
					targetPtrSize = 8
				}
			}
			i = i + 2
		} else if os.Args[i] == "-from-ir-binary" && i+1 < len(os.Args) {
			fromIRBinaryPath = os.Args[i+1]
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
		} else if os.Args[i] == "-strip" || os.Args[i] == "-s" {
			stripBinary = true
			i = i + 1
		} else {
			fmt.Fprintf(os.Stderr, "no_frontend build only supports -from-ir-binary input\n")
			os.Exit(1)
		}
	}

	if fromIRBinaryPath == "" {
		fmt.Fprintf(os.Stderr, "no_frontend build requires -from-ir-binary <path>\n")
		os.Exit(1)
	}

	// Build active tag set from target + explicit tags.
	if targetBackend == "c" {
		buildTags = append(buildTags, "c")
		buildTags = append(buildTags, fmt.Sprintf("c%d", targetCModel))
	} else if target.GOOS == "wasi" && targetGOARCH == "wasm32" {
		buildTags = append(buildTags, "wasi")
		buildTags = append(buildTags, "wasm32")
	} else {
		buildTags = append(buildTags, target.GOOS)
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
	if sizeAnalysisPath != "" {
		stripBinary = true
	}

	initEmbeddedStd()

	irmod, err := readIRBinary(fromIRBinaryPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading IR binary: %v\n", err)
		os.Exit(1)
	}
	if compilerDebug {
		fmt.Fprintf(os.Stderr, "debug: loaded IR binary (%d funcs, %d globals)\n", len(irmod.Funcs), len(irmod.Globals))
		fmt.Fprintf(os.Stderr, "debug: generating output (backend=%s, target=%s/%s)\n", targetBackend, target.GOOS, targetGOARCH)
	}
	err = GenerateELF(irmod, outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "codegen error: %v\n", err)
		os.Exit(1)
	}
	writeSizeAnalysis()
	if targetBackend == "vm" {
		os.Exit(vmExitCode)
	}
}

func printHelp(program string, out *os.File) {
	fmt.Fprintf(out, "Usage: %s [options] -from-ir-binary <module.irb>\n", program)
	fmt.Fprintf(out, "\nThis build is backend-only (built with no_frontend,exp_ir_binary).\n")
	fmt.Fprintf(out, "\nOptions:\n")
	fmt.Fprintf(out, "  -o <path>              Output path (default: output)\n")
	fmt.Fprintf(out, "  -T <target>            Target triple or backend mode\n")
	fmt.Fprintf(out, "  -from-ir-binary <p>    Load binary IR module from path and run codegen\n")
	fmt.Fprintf(out, "  -tags <a,b,c>          Extra build tags\n")
	fmt.Fprintf(out, "  -size-analysis <path>  Write per-function size analysis JSON\n")
	fmt.Fprintf(out, "  -debug                 Enable compiler debug logging\n")
	fmt.Fprintf(out, "  -strip, -s             Strip symbol/debug metadata from native binaries\n")
	fmt.Fprintf(out, "  -h, --help             Show this help\n")
	fmt.Fprintf(out, "\nDefault target: %s/%s\n", runtime.GOOS, runtime.GOARCH)
}
