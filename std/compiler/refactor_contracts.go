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
