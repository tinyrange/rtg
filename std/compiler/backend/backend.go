package backend

import (
	"fmt"

	"j5.nz/rtg/std/compiler/backend/c"
	"j5.nz/rtg/std/compiler/backend/irprint"
	"j5.nz/rtg/std/compiler/backend/vm"
	"j5.nz/rtg/std/compiler/backend/x64"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// Generate dispatches to the appropriate backend based on selected target.
func Generate(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	if target.Backend == "vm" {
		return vm.Generate(target, irmod, outputPath)
	}
	if target.Backend == "c" {
		return c.Generate(target, irmod, outputPath)
	}
	if target.Backend == "ir" {
		return irprint.Generate(irmod, outputPath)
	}
	switch target.GOARCH {
	// case "8086":
	// 	if target.GOOS == "dos" {
	// 		return i386.GenerateDOSCOM(irmod, outputPath)
	// 	}
	// 	return fmt.Errorf("unsupported OS for dos16: %s", target.GOOS)
	case "amd64":
		// 	if target.GOOS == "windows" {
		// 		return x64.GenerateWinPE(irmod, outputPath)
		// 	} else if target.GOOS == "linux" {
		return x64.GenerateELF(target, irmod, outputPath)
	// 	}
	// 	return fmt.Errorf("unsupported OS for amd64: %s", target.GOOS)
	// case "386":
	// 	if target.GOOS == "windows" {
	// 		return i386.GenerateWin386PE(irmod, outputPath)
	// 	} else if target.GOOS == "linux" {
	// 		return i386.GenerateI386ELF(irmod, outputPath)
	// 	}
	// 	return fmt.Errorf("unsupported OS for i386: %s", target.GOOS)
	// case "wasm32":
	// 	return wasm32.GenerateWasm32(irmod, outputPath)
	// case "arm64":
	// 	if target.GOOS == "darwin" {
	// 		return arm64.GenerateDarwin(irmod, outputPath)
	// 	} else if target.GOOS == "linux" {
	// 		return arm64.GenerateLinuxELF(irmod, outputPath)
	// 	} else if target.GOOS == "windows" {
	// 		return arm64.GenerateWinPE(irmod, outputPath)
	// 	}
	// 	return fmt.Errorf("unsupported OS for arm64: %s", target.GOOS)
	default:
		return fmt.Errorf("unsupported target architecture: %s", target.GOARCH)
	}
}
