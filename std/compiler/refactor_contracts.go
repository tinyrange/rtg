package main

// CompilerTarget captures the selected OS/arch/backend profile.
// This is a migration shim for moving global target settings into explicit
// driver/frontend/backend options.
type CompilerTarget struct {
	GOOS     string
	GOARCH   string
	PtrSize  int
	Backend  string
	CModel   int
	WordSize int
}

// DriverOptions captures top-level compile options that should eventually be
// threaded through the driver pipeline rather than read via globals.
type DriverOptions struct {
	Target      CompilerTarget
	BuildTags   []string
	Debug       bool
	StripBinary bool
}

// FrontendOptions are the frontend-facing options.
type FrontendOptions struct {
	Target    CompilerTarget
	BuildTags []string
	Debug     bool
}

// BackendOptions are the backend-facing options.
type BackendOptions struct {
	Target      CompilerTarget
	OutputPath  string
	Debug       bool
	StripBinary bool
}

// Frontend compiles source/module inputs into IR.
type Frontend interface {
	Name() string
	Compile(baseDir string, inputs []string, opts FrontendOptions) (*IRModule, []string)
}

// Backend emits a final artifact from IR.
type Backend interface {
	Name() string
	Emit(mod *IRModule, opts BackendOptions) error
}

func cloneStrings(values []string) []string {
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func backendOptionsFromDriver(opts DriverOptions, outputPath string) BackendOptions {
	return BackendOptions{
		Target:      opts.Target,
		OutputPath:  outputPath,
		Debug:       opts.Debug,
		StripBinary: opts.StripBinary,
	}
}

func compilerTargetString(target CompilerTarget) string {
	return target.GOOS + "/" + target.GOARCH
}

func currentCompilerTarget() CompilerTarget {
	return CompilerTarget{
		GOOS:     targetGOOS,
		GOARCH:   targetGOARCH,
		PtrSize:  targetPtrSize,
		Backend:  targetBackend,
		CModel:   targetCModel,
		WordSize: targetWordSize,
	}
}

func setCompilerTarget(target CompilerTarget) {
	targetGOOS = target.GOOS
	targetGOARCH = target.GOARCH
	targetPtrSize = target.PtrSize
	targetBackend = target.Backend
	targetCModel = target.CModel
	targetWordSize = target.WordSize
}

func applyDriverOptions(opts DriverOptions) {
	setCompilerTarget(opts.Target)
	buildTags = cloneStrings(opts.BuildTags)
	compilerDebug = opts.Debug
	stripBinary = opts.StripBinary
}
