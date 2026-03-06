package linux

import (
	"fmt"
	"os"

	"j5.nz/rtg/std/compiler/backend/riscv"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

func GenerateELF(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	base := uint64(0x10000)
	g := riscv.NewCodeGen(target, irmod, base)
	emitStart(g, irmod, common.EntryFuncName(target))
	g.CompileModuleFuncs()
	unresolved := g.ResolveFunctionFixups()
	if len(unresolved) > 0 {
		seen := make(map[string]bool)
		fmt.Fprintf(os.Stderr, "error: %d unresolved calls:\n", len(unresolved))
		for _, name := range unresolved {
			if !seen[name] {
				fmt.Fprintf(os.Stderr, "  %s\n", name)
				seen[name] = true
			}
		}
		return fmt.Errorf("%d unresolved calls", len(unresolved))
	}
	g.CollectNativeFuncSizes()
	elf := BuildELF(g, irmod)
	if err := os.WriteFile(outputPath, elf, 0755); err != nil {
		return fmt.Errorf("write output: %v", err)
	}
	_ = os.Chmod(outputPath, 0755)
	return nil
}

func emitStart(g *riscv.CodeGen, irmod *ir.IRModule, entryFunc string) {
	g.EmitImmToReg(riscv.REG_A0, 0)
	g.EmitImmToReg(riscv.REG_A1, 1048576)
	g.EmitImmToReg(riscv.REG_A2, 3)
	g.EmitImmToReg(riscv.REG_A3, 0x22)
	g.EmitImmToReg(riscv.REG_A4, -1)
	g.EmitImmToReg(riscv.REG_A5, 0)
	g.EmitImmToReg(riscv.REG_A7, 222)
	g.EmitEcall()
	g.EmitImmToReg(riscv.REG_T0, 1048576)
	g.EmitAdd(riscv.REG_OPSP, riscv.REG_A0, riscv.REG_T0)
	for _, f := range irmod.Funcs {
		if ir.IsInitFunc(f.Name) {
			g.EmitCallPlaceholder(f.Name)
		}
	}
	g.EmitCallPlaceholder(entryFunc)
	g.EmitImmToReg(riscv.REG_A0, 0)
	g.EmitImmToReg(riscv.REG_A7, 94)
	g.EmitEcall()
}
