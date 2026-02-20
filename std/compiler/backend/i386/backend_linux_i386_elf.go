//go:build !no_backend_linux_i386

package i386

import (
	"fmt"
	"os"

	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// GenerateELF compiles an IRModule to an i386 (32-bit) ELF binary.
func GenerateELF(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	g := &CodeGen{
		target:        target,
		funcOffsets:   make(map[string]int),
		labelOffsets:  make(map[int]int),
		stringMap:     make(map[string]int),
		globalOffsets: make([]int, len(irmod.Globals)),
		baseAddr:      0x08048000,
		irmod:         irmod,
		wordSize:      4,
	}

	slot := g.slotBytes_i386()
	for i := range irmod.Globals {
		g.globalOffsets[i] = i * slot
	}
	g.data = make([]byte, len(irmod.Globals)*slot)

	g.emitStart_i386(irmod)

	for _, f := range irmod.Funcs {
		g.funcOffsets[f.Name] = len(g.code)
		g.compileFunc_i386(f)
	}

	ir.CollectNativeFuncSizes(irmod, g.funcOffsets, len(g.code))
	if g.needTostringHelper {
		g.emitTostringHelperI386()
	}

	var unresolved []string
	for _, fix := range g.callFixups {
		if fix.Target == "$rodata_header$" || fix.Target == "$data_addr$" {
			continue
		}
		target, ok := g.funcOffsets[fix.Target]
		if !ok {
			unresolved = append(unresolved, fix.Target)
			continue
		}
		g.patchRel32At(fix.CodeOffset, target)
	}
	if len(unresolved) > 0 {
		fmt.Fprintf(os.Stderr, "error: %d unresolved calls:\n", len(unresolved))
		seen := make(map[string]bool)
		for _, name := range unresolved {
			if !seen[name] {
				fmt.Fprintf(os.Stderr, "  %s\n", name)
				seen[name] = true
			}
		}
		return fmt.Errorf("%d unresolved calls", len(unresolved))
	}

	elf := g.buildELF32(irmod)
	if err := os.WriteFile(outputPath, elf, 0755); err != nil {
		return fmt.Errorf("write output: %v", err)
	}
	return nil
}
