package main

func currentDriverOptions() DriverOptions {
	return DriverOptions{
		Target:      currentCompilerTarget(),
		BuildTags:   cloneStrings(buildTags),
		Debug:       compilerDebug,
		StripBinary: stripBinary,
	}
}

func buildAndApplyDriverOptionsFrom(base DriverOptions, extraTags string, forceStrip bool) DriverOptions {
	opts := base
	opts.BuildTags = cloneStrings(buildActiveBuildTagsForTarget(opts.Target, extraTags))
	if forceStrip {
		opts.StripBinary = true
	}
	applyDriverOptions(opts)
	return opts
}

func emitModuleWithOptions(irmod *IRModule, outputPath string, opts DriverOptions) error {
	prev := currentDriverOptions()
	applyDriverOptions(opts)
	defer applyDriverOptions(prev)

	var err error
	if opts.Target.Backend != "native" {
		err = emitRegisteredBackendWithOptions(opts.Target.Backend, irmod, backendOptionsFromDriver(opts, outputPath))
	} else {
		err = GenerateELF(irmod, outputPath)
	}

	return err
}
