package main

import (
	"fmt"
	"os"
	"strings"
)

// Target and build tag globals
var targetGOOS string = "linux"
var targetGOARCH string = "amd64"
var targetPtrSize int = 8
var targetBackend string = "native" // native, c, or ir
var targetCModel int = 0            // 16/32/64 when targetBackend==c
var buildTags []string

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s [-o output] [-T os/arch|c[/16|32|64]] [-tags tag1,tag2] <file.go> [file2.go ...]\n", os.Args[0])
		os.Exit(1)
	}

	outputPath := "output"
	var entryFiles []string
	var extraTags string
	i := 1
	for i < len(os.Args) {
		if os.Args[i] == "-o" && i+1 < len(os.Args) {
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
		} else if os.Args[i] == "--" {
			i = i + 1
		} else {
			entryFiles = append(entryFiles, normalizePath(os.Args[i]))
			i = i + 1
		}
	}
	if len(entryFiles) == 0 {
		fmt.Fprintf(os.Stderr, "usage: %s [-o output] [-T os/arch|c[/16|32|64]] [-tags tag1,tag2] <file.go> [file2.go ...]\n", os.Args[0])
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

	mod := ResolveModule(baseDir, entryFiles)

	// Validate cross-package references
	valErrs := ValidateModule(mod)
	if len(valErrs) > 0 {
		fmt.Fprintf(os.Stderr, "\n%d validation errors:\n", len(valErrs))
		for _, e := range valErrs {
			fmt.Fprintf(os.Stderr, "  %s\n", e)
		}
		os.Exit(1)
	}

	// Compile to IR
	irmod, errs := CompileModule(mod)

	if len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "\n%d compile errors:\n", len(errs))
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  %s\n", e)
		}
		os.Exit(1)
	}

	eliminateDeadFunctions(irmod)

	err := GenerateELF(irmod, outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "codegen error: %v\n", err)
		os.Exit(1)
	}

	writeSizeAnalysis()
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
