//go:build !no_backend_linux_i386

package main

import (
	"fmt"
	"os"
)

// generateI386ELF compiles an IRModule to an i386 (32-bit) ELF binary.
func generateI386ELF(irmod *IRModule, outputPath string) error {
	g := newNativeCodeGen(irmod, 4, 0x08048000, false)

	slot := g.slotBytes_i386()
	initNativeGlobalsData(g, len(irmod.Globals), slot)

	g.emitStart_i386(irmod)

	compileNativeModuleFuncs(g, irmod, nativeCompileModeI386)
	if err := resolveNativeCallFixups(g, false, false); err != nil {
		return err
	}

	elf := g.buildELF32(irmod)
	if err := os.WriteFile(outputPath, elf, 0755); err != nil {
		return fmt.Errorf("write output: %v", err)
	}
	return nil
}
