package main

func compileResolvedGoModule(mod *Module, opts DriverOptions) (*IRModule, []string) {
	frontend := goFrontend{}
	return frontend.CompileResolved(mod, FrontendOptions{
		Target:    opts.Target,
		BuildTags: cloneStrings(opts.BuildTags),
		Debug:     opts.Debug,
	})
}
