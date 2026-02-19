//go:build no_frontend && exp_ir_binary

package main

import (
	"fmt"
	"os"
	"runtime"
)

// Target and build tag globals — defaults to host platform
var targetGOOS string = runtime.GOOS
var targetGOARCH string = runtime.GOARCH
var targetPtrSize int = defaultPtrSize()

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
			errMsg := applyTargetFlag(os.Args[i+1])
			if errMsg != "" {
				fmt.Fprintf(os.Stderr, "%s\n", errMsg)
				os.Exit(1)
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
	rebuildActiveBuildTags(extraTags)
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
		fmt.Fprintf(os.Stderr, "debug: generating output (backend=%s, target=%s/%s)\n", targetBackend, targetGOOS, targetGOARCH)
	}
	err = emitModuleWithOptions(irmod, outputPath, currentDriverOptions())
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
