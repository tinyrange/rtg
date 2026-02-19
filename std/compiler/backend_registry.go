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
	switch backendRegistry[name] {
	case backendIDC:
		return generateCSource(irmod, outputPath)
	case backendIDIR:
		return generateIRText(irmod, outputPath)
	case backendIDVM:
		return generateVM(irmod, outputPath)
	default:
		return fmt.Errorf("unknown backend: %s", name)
	}
}
