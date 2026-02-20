package main

import (
	"fmt"
	"os"
)

func currentDriverOptions() DriverOptions {
	return DriverOptions{
		Target:      currentCompilerTarget(),
		BuildTags:   cloneStrings(buildTags),
		Debug:       compilerDebug,
		StripBinary: stripBinary,
		SizeReport:  sizeAnalysisPath,
	}
}

func buildAndApplyDriverOptionsFrom(base DriverOptions, extraTags string) DriverOptions {
	opts := base
	opts.BuildTags = cloneStrings(buildActiveBuildTagsForTarget(opts.Target, extraTags))
	if opts.SizeReport != "" {
		opts.StripBinary = true
	}
	applyDriverOptions(opts)
	return opts
}

func emitModuleWithOptions(irmod *IRModule, outputPath string, opts DriverOptions) error {
	prev := currentDriverOptions()
	applyDriverOptions(opts)
	defer applyDriverOptions(prev)
	return GenerateELF(irmod, outputPath, opts)
}

func validateFromIRBinaryFlags(parseOnly bool, buildTagsPath string) error {
	if parseOnly {
		return fmt.Errorf("-parse-only is not valid with -from-ir-binary")
	}
	if buildTagsPath != "" {
		return fmt.Errorf("-list-build-tags is not valid with -from-ir-binary")
	}
	return nil
}

func loadIRBinaryModule(path string, opts DriverOptions) (*IRModule, error) {
	irmod, err := readIRBinary(path)
	if err != nil {
		return nil, fmt.Errorf("error reading IR binary: %v", err)
	}
	if opts.Debug {
		fmt.Fprintf(os.Stderr, "debug: loaded IR binary (%d funcs, %d globals)\n", len(irmod.Funcs), len(irmod.Globals))
	}
	return irmod, nil
}

func emitAndFinalizeWithOptions(irmod *IRModule, outputPath string, opts DriverOptions) (int, error) {
	if opts.Debug {
		fmt.Fprintf(os.Stderr, "debug: generating output (backend=%s, target=%s)\n", opts.Target.Backend, compilerTargetString(opts.Target))
	}
	err := emitModuleWithOptions(irmod, outputPath, opts)
	if err != nil {
		return 1, fmt.Errorf("codegen error: %v", err)
	}
	if opts.Debug {
		fmt.Fprintf(os.Stderr, "debug: output generated successfully\n")
	}
	writeSizeAnalysis()
	if opts.Target.Backend == "vm" {
		return vmExitCode, nil
	}
	return 0, nil
}
