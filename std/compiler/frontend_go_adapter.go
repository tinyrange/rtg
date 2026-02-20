package main

type goFrontend struct{}

func (f goFrontend) Name() string {
	return "go"
}

func (f goFrontend) Compile(baseDir string, inputs []string, opts FrontendOptions) (*IRModule, []string) {
	mod := ResolveModule(baseDir, inputs)
	if mod == nil {
		return nil, []string{"failed to resolve module"}
	}
	return f.CompileResolved(mod, opts)
}

func (f goFrontend) CompileResolved(mod *Module, opts FrontendOptions) (*IRModule, []string) {
	prev := currentDriverOptions()
	applyDriverOptions(DriverOptions{
		Target:      opts.Target,
		BuildTags:   cloneStrings(opts.BuildTags),
		Debug:       opts.Debug,
		StripBinary: prev.StripBinary,
	})
	defer applyDriverOptions(prev)

	if valErrs := ValidateModule(mod); len(valErrs) > 0 {
		return nil, valErrs
	}
	irmod, errs := CompileModule(mod)
	if len(errs) > 0 {
		return nil, errs
	}
	if irmod == nil {
		return nil, []string{"frontend produced nil IR module"}
	}
	return irmod, nil
}
