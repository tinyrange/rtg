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

func emitModuleWithOptions(irmod *IRModule, outputPath string, opts DriverOptions) error {
	if opts.Target.Backend != "native" {
		return emitRegisteredBackendWithOptions(opts.Target.Backend, irmod, BackendOptions{
			Target:      opts.Target,
			OutputPath:  outputPath,
			Debug:       opts.Debug,
			StripBinary: opts.StripBinary,
		})
	}
	return GenerateELF(irmod, outputPath)
}
