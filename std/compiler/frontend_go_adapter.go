package main

type goFrontend struct{}

func (f goFrontend) Name() string {
	return "go"
}

func (f goFrontend) Compile(baseDir string, inputs []string, opts FrontendOptions) (*IRModule, []string) {
	prev := currentDriverOptions()
	applyDriverOptions(DriverOptions{
		Target:      opts.Target,
		BuildTags:   cloneStrings(opts.BuildTags),
		Debug:       opts.Debug,
		StripBinary: prev.StripBinary,
	})
	defer applyDriverOptions(prev)

	mod := ResolveModule(baseDir, inputs)
	if mod == nil {
		return nil, []string{"failed to resolve module"}
	}
	if valErrs := ValidateModule(mod); len(valErrs) > 0 {
		return nil, valErrs
	}
	irmod, errs := CompileModule(mod)
	if len(errs) > 0 {
		return nil, errs
	}
	return irmod, nil
}
