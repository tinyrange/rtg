//go:build !no_backend_linux_i386

package linux

import (
	"fmt"
	"os"

	core "j5.nz/rtg/std/compiler/backend/i386"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// GenerateELF compiles an IRModule to an i386 (32-bit) ELF binary.
func GenerateELF(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	g := core.NewCodeGen(target, irmod, 0x08048000)

	EmitStart(g, irmod, common.EntryFuncName(target))
	g.CompileModuleFuncs(irmod)
	g.CollectNativeFuncSizes(irmod)
	if g.NeedTostringHelper {
		g.EmitTostringHelperI386()
	}

	var unresolved []string
	for _, fix := range g.CallFixups {
		if fix.Target == "$rodata_header$" || fix.Target == "$data_addr$" {
			continue
		}
		targetOff, ok := g.FuncOffsets[fix.Target]
		if !ok {
			unresolved = append(unresolved, fix.Target)
			continue
		}
		g.PatchRel32At(fix.CodeOffset, targetOff)
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

	elf := BuildELF32(g, irmod)
	if err := os.WriteFile(outputPath, elf, 0755); err != nil {
		return fmt.Errorf("write output: %v", err)
	}
	return nil
}
