package main

import "fmt"

type nativeTargetID int

const (
	nativeTargetUnknown nativeTargetID = iota
	nativeTargetDOS16
	nativeTargetLinuxAMD64
	nativeTargetWindowsAMD64
	nativeTargetLinux386
	nativeTargetWindows386
	nativeTargetDOS386
	nativeTargetWASIWASM32
	nativeTargetDarwinARM64
	nativeTargetLinuxARM64
	nativeTargetWindowsARM64
)

var nativeTargetDispatch = map[string]nativeTargetID{
	"dos/dos16":     nativeTargetDOS16,
	"linux/amd64":   nativeTargetLinuxAMD64,
	"windows/amd64": nativeTargetWindowsAMD64,
	"linux/386":     nativeTargetLinux386,
	"windows/386":   nativeTargetWindows386,
	"dos/386":       nativeTargetDOS386,
	"wasi/wasm32":   nativeTargetWASIWASM32,
	"darwin/arm64":  nativeTargetDarwinARM64,
	"linux/arm64":   nativeTargetLinuxARM64,
	"windows/arm64": nativeTargetWindowsARM64,
}

func emitNativeModule(irmod *IRModule, outputPath string, target CompilerTarget) error {
	nativeID := nativeTargetDispatch[compilerTargetString(target)]
	if nativeID == nativeTargetUnknown {
		return fmt.Errorf("unsupported native target: %s", compilerTargetString(target))
	}
	switch nativeID {
	case nativeTargetDOS16:
		return generateDOSCOM386(irmod, outputPath)
	case nativeTargetLinuxAMD64:
		return generateAmd64ELF(irmod, outputPath)
	case nativeTargetWindowsAMD64:
		return generateWinAmd64PE(irmod, outputPath)
	case nativeTargetLinux386:
		return generateI386ELF(irmod, outputPath)
	case nativeTargetWindows386:
		return generateWin386PE(irmod, outputPath)
	case nativeTargetDOS386:
		return generateDOSCOM386(irmod, outputPath)
	case nativeTargetWASIWASM32:
		return generateWasm32(irmod, outputPath)
	case nativeTargetDarwinARM64:
		return generateDarwinArm64(irmod, outputPath)
	case nativeTargetLinuxARM64:
		return generateLinuxArm64ELF(irmod, outputPath)
	case nativeTargetWindowsARM64:
		return generateWinArm64PE(irmod, outputPath)
	default:
		return fmt.Errorf("unsupported native target id: %d", int(nativeID))
	}
}
