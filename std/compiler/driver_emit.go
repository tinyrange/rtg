package main

func currentDriverOptions() DriverOptions {
	tags := make([]string, len(buildTags))
	copy(tags, buildTags)
	return DriverOptions{
		Target:      currentCompilerTarget(),
		BuildTags:   tags,
		Debug:       compilerDebug,
		StripBinary: stripBinary,
	}
}

func buildAndApplyDriverOptions(extraTags string, forceStrip bool) DriverOptions {
	opts := currentDriverOptions()
	opts.BuildTags = buildActiveBuildTagsForTarget(opts.Target, extraTags)
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
		err = emitRegisteredBackendWithOptions(opts.Target.Backend, irmod, BackendOptions{
			Target:      opts.Target,
			OutputPath:  outputPath,
			Debug:       opts.Debug,
			StripBinary: opts.StripBinary,
		})
	} else {
		err = GenerateELF(irmod, outputPath)
	}

	return err
}
