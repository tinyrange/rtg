package main

import (
	"fmt"
	"os"
)

func compileResolvedGoModule(mod *Module, opts DriverOptions) (*IRModule, []string) {
	frontend := goFrontend{}
	return frontend.CompileResolved(mod, FrontendOptions{
		Target:    opts.Target,
		BuildTags: cloneStrings(opts.BuildTags),
		Debug:     opts.Debug,
	})
}

func resolveCompilerBaseDir() (string, error) {
	if hasEmbeddedStd() {
		return ".", nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	// Walk up from cwd until we find a directory containing std/runtime/runtime.go.
	baseDir := cwd
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
	return baseDir, nil
}

func writeDiscoveredBuildTags(path string) error {
	tags := getDiscoveredBuildTags()
	var out string
	for _, t := range tags {
		out = out + t + "\n"
	}
	return os.WriteFile(path, []byte(out), 0644)
}

type sourceCompileResult struct {
	IRModule      *IRModule
	FrontendErrs  []string
	ShouldExitNow bool
}

func compileFromSourceInputs(entryFiles []string, buildTagsPath string, parseOnly bool, emitIRBinaryPath string, opts DriverOptions) (sourceCompileResult, error) {
	baseDir, err := resolveCompilerBaseDir()
	if err != nil {
		return sourceCompileResult{}, fmt.Errorf("error getting working directory: %v", err)
	}

	if opts.Debug {
		fmt.Fprintf(os.Stderr, "debug: resolving module (%d entry files)\n", len(entryFiles))
	}
	resetDiscoveredBuildTags()
	mod := ResolveModule(baseDir, entryFiles)
	if opts.Debug {
		fmt.Fprintf(os.Stderr, "debug: resolved %d packages\n", len(mod.Packages))
	}

	if buildTagsPath != "" {
		err := writeDiscoveredBuildTags(buildTagsPath)
		if err != nil {
			return sourceCompileResult{}, fmt.Errorf("error writing build tag list: %v", err)
		}
	}

	if parseOnly {
		return sourceCompileResult{ShouldExitNow: true}, nil
	}

	// Compile to IR via Go frontend adapter (concrete dispatch).
	if opts.Debug {
		fmt.Fprintf(os.Stderr, "debug: compiling to IR\n")
	}
	irmod, errs := compileResolvedGoModule(mod, opts)
	if len(errs) > 0 {
		return sourceCompileResult{FrontendErrs: errs}, nil
	}

	if opts.Debug {
		fmt.Fprintf(os.Stderr, "debug: IR compiled (%d funcs, %d globals)\n", len(irmod.Funcs), len(irmod.Globals))
	}
	eliminateDeadFunctions(irmod)
	if opts.Debug {
		fmt.Fprintf(os.Stderr, "debug: DCE done (%d funcs remaining)\n", len(irmod.Funcs))
	}
	if emitIRBinaryPath != "" {
		err := writeIRBinary(irmod, emitIRBinaryPath)
		if err != nil {
			return sourceCompileResult{}, fmt.Errorf("error writing IR binary: %v", err)
		}
		return sourceCompileResult{ShouldExitNow: true}, nil
	}
	return sourceCompileResult{IRModule: irmod}, nil
}
