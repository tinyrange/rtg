//go:build !no_frontend

package main

import "fmt"

type mainInvocation struct {
	OutputPath       string
	EntryFiles       []string
	ExtraTags        string
	ParseOnly        bool
	BuildTagsPath    string
	EmitIRBinaryPath string
	FromIRBinaryPath string
	ExtractStdlibDst string
	RunMode          bool
	StdinInput       bool
	ProgramArgs      []string
	ParsedOpts       DriverOptions
}

type parseMainArgsResult struct {
	Invocation   mainInvocation
	ShowHelp     bool
	HelpToStdout bool
}

func parseMainArgs(args []string, base DriverOptions) (parseMainArgsResult, error) {
	result := parseMainArgsResult{
		Invocation: mainInvocation{
			OutputPath: "output",
			ParsedOpts: base,
		},
	}

	if len(args) < 2 {
		result.ShowHelp = true
		result.HelpToStdout = false
		return result, nil
	}

	i := 1
	for i < len(args) {
		if args[i] == "-h" || args[i] == "--help" {
			result.ShowHelp = true
			result.HelpToStdout = true
			return result, nil
		} else if args[i] == "-run" {
			result.Invocation.RunMode = true
			i = i + 1
		} else if args[i] == "-o" && i+1 < len(args) {
			result.Invocation.OutputPath = args[i+1]
			i = i + 2
		} else if args[i] == "-T" && i+1 < len(args) {
			var errMsg string
			result.Invocation.ParsedOpts.Target, errMsg = parseTargetFlag(result.Invocation.ParsedOpts.Target, args[i+1])
			if errMsg != "" {
				return parseMainArgsResult{}, fmt.Errorf("%s", errMsg)
			}
			i = i + 2
		} else if args[i] == "-size-analysis" && i+1 < len(args) {
			sizeAnalysisPath = args[i+1]
			i = i + 2
		} else if args[i] == "-parse-only" {
			result.Invocation.ParseOnly = true
			i = i + 1
		} else if (args[i] == "-emit-ir-binary" || args[i] == "-from-ir-binary") && i+1 < len(args) {
			if !irBinaryEnabled {
				return parseMainArgsResult{}, fmt.Errorf("IR binary I/O is experimental; rebuild with -tags exp_ir_binary")
			}
			if args[i] == "-emit-ir-binary" {
				result.Invocation.EmitIRBinaryPath = args[i+1]
			} else {
				result.Invocation.FromIRBinaryPath = args[i+1]
			}
			i = i + 2
		} else if args[i] == "-list-build-tags" && i+1 < len(args) {
			result.Invocation.BuildTagsPath = args[i+1]
			i = i + 2
		} else if args[i] == "-tags" && i+1 < len(args) {
			result.Invocation.ExtraTags = args[i+1]
			i = i + 2
		} else if args[i] == "-include" && i+1 < len(args) {
			val := normalizePath(args[i+1])
			if !stdlibIncludeExplicit {
				stdlibIncludeExplicit = true
				stdlibIncludeEmbedded = false
			}
			if val == "-" {
				stdlibIncludeEmbedded = true
			} else if val != "" {
				stdlibIncludePaths = appendUnique(stdlibIncludePaths, trimTrailingSlash(val))
			}
			i = i + 2
		} else if args[i] == "-extract-stdlib" && i+1 < len(args) {
			result.Invocation.ExtractStdlibDst = normalizePath(args[i+1])
			i = i + 2
		} else if args[i] == "-debug" {
			result.Invocation.ParsedOpts.Debug = true
			i = i + 1
		} else if args[i] == "-strip" || args[i] == "-s" {
			result.Invocation.ParsedOpts.StripBinary = true
			i = i + 1
		} else if args[i] == "--" {
			i = i + 1
			for i < len(args) {
				result.Invocation.ProgramArgs = append(result.Invocation.ProgramArgs, args[i])
				i = i + 1
			}
		} else if args[i] == "-" {
			result.Invocation.StdinInput = true
			i = i + 1
		} else {
			result.Invocation.EntryFiles = append(result.Invocation.EntryFiles, normalizePath(args[i]))
			i = i + 1
		}
	}

	return result, nil
}
