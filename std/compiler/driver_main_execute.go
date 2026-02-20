//go:build !no_frontend

package main

func executeMainInvocation(invocation mainInvocation) (int, string, error) {
	entryFiles, outputPath, err := prepareRuntimeInputs(invocation.EntryFiles, invocation.FromIRBinaryPath, invocation.StdinInput, invocation.RunMode, invocation.OutputPath, invocation.ParsedOpts.Target)
	if err != nil {
		return 1, "", err
	}

	showHelp, err := validateMainInputs(invocation.ExtractStdlibDst, invocation.FromIRBinaryPath, entryFiles)
	if err != nil {
		return 1, "", err
	}
	if showHelp {
		return 2, "", nil
	}

	// Build and apply driver options explicitly.
	opts := buildAndApplyDriverOptionsFrom(invocation.ParsedOpts, invocation.ExtraTags)

	// Initialize embedded std if available.
	initEmbeddedStd()

	didExtractStdlib, err := handleExtractStdlibMode(invocation.ExtractStdlibDst, invocation.FromIRBinaryPath, entryFiles, invocation.RunMode, invocation.StdinInput, invocation.ParseOnly, invocation.EmitIRBinaryPath, invocation.BuildTagsPath)
	if err != nil {
		return 1, "", err
	}
	if didExtractStdlib {
		return 0, "", nil
	}

	irmod, frontendErrMsg, shouldExitNow, err := resolveIRModuleForMain(entryFiles, invocation.FromIRBinaryPath, invocation.BuildTagsPath, invocation.ParseOnly, invocation.EmitIRBinaryPath, opts)
	if err != nil {
		return 1, "", err
	}
	if frontendErrMsg != "" {
		return 1, frontendErrMsg, nil
	}
	if shouldExitNow {
		return 0, "", nil
	}

	// Set VM program arguments if using VM backend.
	if opts.Target.Backend == "vm" {
		configureVMProgramArgs(entryFiles, invocation.ProgramArgs)
	}

	exitCode, err := emitAndFinalizeWithOptions(irmod, outputPath, opts)
	if err != nil {
		return 1, "", err
	}

	// VM backend executes directly — no binary to run.
	if exitCode != 0 {
		return exitCode, "", nil
	}

	if invocation.RunMode {
		err = runCompiledBinary(outputPath)
		if err != nil {
			code, msg := classifyRunModeError(err.Error())
			return code, msg, nil
		}
	}

	return 0, "", nil
}
