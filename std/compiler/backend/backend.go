package backend

import (
	"fmt"

	x8086 "j5.nz/rtg/std/compiler/backend/8086"
	aarch64ccmetal "j5.nz/rtg/std/compiler/backend/aarch64/ccmetal"
	aarch64linux "j5.nz/rtg/std/compiler/backend/aarch64/linux"
	aarch64macos "j5.nz/rtg/std/compiler/backend/aarch64/macos"
	aarch64windows "j5.nz/rtg/std/compiler/backend/aarch64/windows"
	armv8melf "j5.nz/rtg/std/compiler/backend/armv8m/elf"
	"j5.nz/rtg/std/compiler/backend/c"
	i386linux "j5.nz/rtg/std/compiler/backend/i386/linux"
	i386windows "j5.nz/rtg/std/compiler/backend/i386/windows"
	"j5.nz/rtg/std/compiler/backend/irprint"
	"j5.nz/rtg/std/compiler/backend/vm"
	"j5.nz/rtg/std/compiler/backend/wasm32"
	x64linux "j5.nz/rtg/std/compiler/backend/x64/linux"
	x64windows "j5.nz/rtg/std/compiler/backend/x64/windows"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
	targetcfg "j5.nz/rtg/std/target"
)

// Generate dispatches to the appropriate backend based on selected target.
func Generate(tgt *common.Target, irmod *ir.IRModule, outputPath string) error {
	triple := tgt.Triple
	if triple == "" {
		triple = tgt.GOOS + "/" + tgt.GOARCH
	}
	if tgt.Backend == "vm" {
		return vm.Generate(tgt, irmod, outputPath)
	}
	if tgt.Backend == "c" {
		return c.Generate(tgt, irmod, outputPath)
	}
	if tgt.Backend == "ir" {
		return irprint.Generate(irmod, outputPath)
	}
	if handled, err := targetcfg.Generate(triple, tgt, irmod, outputPath); handled {
		return err
	}
	if spec, ok := targetcfg.Lookup(triple); ok {
		if handled, err := generateRegisteredProfile(spec, tgt, irmod, outputPath); handled {
			return err
		}
	}
	switch tgt.GOARCH {
	case "8086", "dos16":
		if tgt.GOOS == "dos" {
			return x8086.GenerateDOSCOM(tgt, irmod, outputPath)
		}
		return fmt.Errorf("unsupported OS for dos16: %s", tgt.GOOS)
	case "amd64":
		if tgt.GOOS == "windows" {
			return x64windows.Generate(tgt, irmod, outputPath)
		} else if tgt.GOOS == "linux" {
			return x64linux.GenerateELF(tgt, irmod, outputPath)
		}
		return fmt.Errorf("unsupported OS for amd64: %s", tgt.GOOS)
	case "386":
		if tgt.GOOS == "windows" {
			return i386windows.Generate(tgt, irmod, outputPath)
		} else if tgt.GOOS == "linux" {
			return i386linux.GenerateELF(tgt, irmod, outputPath)
		}
		return fmt.Errorf("unsupported OS for i386: %s", tgt.GOOS)
	case "wasm32":
		return wasm32.Generate(tgt, irmod, outputPath)
	case "arm64":
		if tgt.GOOS == "linux" {
			return aarch64linux.Generate(tgt, irmod, outputPath)
		} else if tgt.GOOS == "ccmetal" {
			return aarch64ccmetal.Generate(tgt, irmod, outputPath)
		} else if tgt.GOOS == "darwin" {
			return aarch64macos.Generate(tgt, irmod, outputPath)
		} else if tgt.GOOS == "windows" {
			return aarch64windows.Generate(tgt, irmod, outputPath)
		}
		return fmt.Errorf("unsupported OS for arm64: %s", tgt.GOOS)
	case "armv8m":
		if tgt.Triple == "elf/armv8m" || tgt.GOOS == "elf" || tgt.GOOS == "semihost" || tgt.GOOS == "bare" {
			return armv8melf.Generate(tgt, irmod, outputPath)
		}
		return fmt.Errorf("unsupported OS for armv8m: %s", tgt.GOOS)
	default:
		return fmt.Errorf("unsupported target architecture: %s", tgt.GOARCH)
	}
}
