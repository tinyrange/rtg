//go:build baremetal && armv8m

package backend

import (
	"fmt"

	armv8melf "j5.nz/rtg/std/compiler/backend/armv8m/elf"
	"j5.nz/rtg/std/compiler/backend/c"
	"j5.nz/rtg/std/compiler/backend/irprint"
	"j5.nz/rtg/std/compiler/backend/vm"
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
	if tgt.GOARCH == "armv8m" && (tgt.Triple == "elf/armv8m" || tgt.GOOS == "elf" || tgt.GOOS == "baremetal") {
		return armv8melf.Generate(tgt, irmod, outputPath)
	}
	return fmt.Errorf("unsupported target on baremetal/armv8m runtime compiler: %s/%s", tgt.GOOS, tgt.GOARCH)
}
