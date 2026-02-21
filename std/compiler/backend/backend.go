package backend

import (
	"fmt"

	x8086 "j5.nz/rtg/std/compiler/backend/8086"
	aarch64linux "j5.nz/rtg/std/compiler/backend/aarch64/linux"
	aarch64macos "j5.nz/rtg/std/compiler/backend/aarch64/macos"
	aarch64windows "j5.nz/rtg/std/compiler/backend/aarch64/windows"
	"j5.nz/rtg/std/compiler/backend/c"
	"j5.nz/rtg/std/compiler/backend/i386"
	"j5.nz/rtg/std/compiler/backend/irprint"
	"j5.nz/rtg/std/compiler/backend/vm"
	"j5.nz/rtg/std/compiler/backend/wasm32"
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
	case "8086", "dos16":
		if target.GOOS == "dos" {
			return x8086.GenerateDOSCOM(target, irmod, outputPath)
		}
		return fmt.Errorf("unsupported OS for dos16: %s", target.GOOS)
	case "amd64":
		if target.GOOS == "windows" {
			return x64.GenerateWinPE(target, irmod, outputPath)
		} else if target.GOOS == "linux" {
			return x64.GenerateELF(target, irmod, outputPath)
		}
		return fmt.Errorf("unsupported OS for amd64: %s", target.GOOS)
	case "386":
		if target.GOOS == "windows" {
			return i386.GenerateWinPE(target, irmod, outputPath)
		} else if target.GOOS == "linux" {
			return i386.GenerateELF(target, irmod, outputPath)
		}
		return fmt.Errorf("unsupported OS for i386: %s", target.GOOS)
	case "wasm32":
		return wasm32.Generate(target, irmod, outputPath)
	case "arm64":
		if target.GOOS == "darwin" {
			return aarch64macos.Generate(target, irmod, outputPath)
		} else if target.GOOS == "linux" {
			return aarch64linux.Generate(target, irmod, outputPath)
		} else if target.GOOS == "windows" {
			return aarch64windows.Generate(target, irmod, outputPath)
		}
		return fmt.Errorf("unsupported OS for arm64: %s", target.GOOS)
	default:
		return fmt.Errorf("unsupported target architecture: %s", target.GOARCH)
	}
}
