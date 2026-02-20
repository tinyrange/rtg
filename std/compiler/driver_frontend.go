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

type irAcquisitionResult struct {
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

func acquireIRModule(entryFiles []string, fromIRBinaryPath string, buildTagsPath string, parseOnly bool, emitIRBinaryPath string, opts DriverOptions) (irAcquisitionResult, error) {
	if fromIRBinaryPath != "" {
		err := validateFromIRBinaryFlags(parseOnly, buildTagsPath)
		if err != nil {
			return irAcquisitionResult{}, err
		}
		irmod, err := loadIRBinaryModule(fromIRBinaryPath, opts)
		if err != nil {
			return irAcquisitionResult{}, err
		}
		return irAcquisitionResult{IRModule: irmod}, nil
	}

	res, err := compileFromSourceInputs(entryFiles, buildTagsPath, parseOnly, emitIRBinaryPath, opts)
	if err != nil {
		return irAcquisitionResult{}, err
	}
	return irAcquisitionResult{
		IRModule:      res.IRModule,
		FrontendErrs:  res.FrontendErrs,
		ShouldExitNow: res.ShouldExitNow,
	}, nil
}

func handleExtractStdlibMode(extractStdlibDest string, fromIRBinaryPath string, entryFiles []string, runMode bool, stdinInput bool, parseOnly bool, emitIRBinaryPath string, buildTagsPath string) (bool, error) {
	if extractStdlibDest == "" {
		return false, nil
	}
	if fromIRBinaryPath != "" || len(entryFiles) > 0 || runMode || stdinInput || parseOnly || emitIRBinaryPath != "" || buildTagsPath != "" {
		return false, fmt.Errorf("-extract-stdlib cannot be combined with compilation inputs/options")
	}
	err := extractEmbeddedStdlib(extractStdlibDest)
	if err != nil {
		return false, fmt.Errorf("error extracting stdlib: %v", err)
	}
	return true, nil
}

func validateMainInputs(extractStdlibDest string, fromIRBinaryPath string, entryFiles []string) (bool, error) {
	if fromIRBinaryPath != "" && len(entryFiles) > 0 {
		return false, fmt.Errorf("cannot combine source files with -from-ir-binary")
	}
	if extractStdlibDest == "" && fromIRBinaryPath == "" && len(entryFiles) == 0 {
		return true, nil
	}
	return false, nil
}
