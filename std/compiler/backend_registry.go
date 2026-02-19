package main

import "fmt"

type backendID int

const (
	backendIDUnknown backendID = iota
	backendIDC
	backendIDIR
	backendIDVM
)

var backendRegistry = map[string]backendID{
	"c":  backendIDC,
	"ir": backendIDIR,
	"vm": backendIDVM,
}

func newBackendForTarget(name string) (backendID, error) {
	id := backendRegistry[name]
	if id == backendIDUnknown {
		return backendIDUnknown, fmt.Errorf("unknown backend: %s", name)
	}
	return id, nil
}

type cBackendAdapter struct{}

func (b cBackendAdapter) Name() string {
	return "c"
}

func (b cBackendAdapter) Emit(mod *IRModule, opts BackendOptions) error {
	return generateCSource(mod, opts.OutputPath)
}

type irBackendAdapter struct{}

func (b irBackendAdapter) Name() string {
	return "ir"
}

func (b irBackendAdapter) Emit(mod *IRModule, opts BackendOptions) error {
	return generateIRText(mod, opts.OutputPath)
}

type vmBackendAdapter struct{}

func (b vmBackendAdapter) Name() string {
	return "vm"
}

func (b vmBackendAdapter) Emit(mod *IRModule, opts BackendOptions) error {
	return generateVM(mod, opts.OutputPath)
}

func emitRegisteredBackend(name string, irmod *IRModule, outputPath string) error {
	return emitRegisteredBackendWithOptions(name, irmod, BackendOptions{
		Target:      currentCompilerTarget(),
		OutputPath:  outputPath,
		Debug:       compilerDebug,
		StripBinary: stripBinary,
	})
}

func emitRegisteredBackendWithOptions(name string, irmod *IRModule, opts BackendOptions) error {
	backend, err := newBackendForTarget(name)
	if err != nil {
		return err
	}
	switch backend {
	case backendIDC:
		var b cBackendAdapter
		return b.Emit(irmod, opts)
	case backendIDIR:
		var b irBackendAdapter
		return b.Emit(irmod, opts)
	case backendIDVM:
		var b vmBackendAdapter
		return b.Emit(irmod, opts)
	default:
		return fmt.Errorf("unknown backend id: %d", int(backend))
	}
}
