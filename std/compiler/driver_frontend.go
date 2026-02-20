package main

import "os"

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
