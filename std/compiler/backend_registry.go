package main

import "fmt"

type backendID int

const (
	backendIDUnknown backendID = iota
	backendIDNative
	backendIDC
	backendIDIR
	backendIDVM
)

var backendRegistry = map[string]backendID{
	"native": backendIDNative,
	"c":      backendIDC,
	"ir":     backendIDIR,
	"vm":     backendIDVM,
}

func newBackendForTarget(name string) (backendID, error) {
	id := backendRegistry[name]
	if id == backendIDUnknown {
		return backendIDUnknown, fmt.Errorf("unknown backend: %s", name)
	}
	return id, nil
}

func emitBackendWithOptions(name string, irmod *IRModule, opts BackendOptions) error {
	backendID, err := newBackendForTarget(name)
	if err != nil {
		return err
	}
	switch backendID {
	case backendIDNative:
		return emitNativeModule(irmod, opts.OutputPath, opts.Target)
	case backendIDC:
		return generateCSource(irmod, opts.OutputPath)
	case backendIDIR:
		return generateIRText(irmod, opts.OutputPath)
	case backendIDVM:
		return generateVM(irmod, opts.OutputPath)
	default:
		return fmt.Errorf("unknown backend id: %d", int(backendID))
	}
}
