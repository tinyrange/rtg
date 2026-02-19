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

func emitRegisteredBackend(name string, irmod *IRModule, outputPath string) error {
	return emitRegisteredBackendWithOptions(name, irmod, BackendOptions{
		Target:      currentCompilerTarget(),
		OutputPath:  outputPath,
		Debug:       compilerDebug,
		StripBinary: stripBinary,
	})
}

func emitRegisteredBackendWithOptions(name string, irmod *IRModule, opts BackendOptions) error {
	switch backendRegistry[name] {
	case backendIDC:
		return generateCSource(irmod, opts.OutputPath)
	case backendIDIR:
		return generateIRText(irmod, opts.OutputPath)
	case backendIDVM:
		return generateVM(irmod, opts.OutputPath)
	default:
		return fmt.Errorf("unknown backend: %s", name)
	}
}
