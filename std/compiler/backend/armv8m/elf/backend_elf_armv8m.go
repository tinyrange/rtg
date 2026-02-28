package elf

import (
	"fmt"
	"os"

	"j5.nz/rtg/std/compiler/backend/armv8m"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

const (
	defaultInitialStackPointer = uint32(0x283FF000)
)

// Generate emits a minimal Cortex-M33 ELF image for backend bringup.
// The backend supports a growing subset of IR opcodes and currently
// prioritizes compiler/bootstrap bringup paths.
func Generate(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	const codeBaseAddr = uint32(0x10000000 + 0x100)
	g := armv8m.NewCodeGen(target, irmod)

	// Startup:
	//   initialize software operand stack pointer (r6)
	//   call init funcs
	//   call program entrypoint
	//   exit via runtime.SysExit(0)
	g.EmitMovsImm(0, 0)
	g.LoadImm32(6, 0x283F0000)
	for _, f := range irmod.Funcs {
		if ir.IsInitFunc(f.Name) {
			g.EmitCallPlaceholder(f.Name)
		}
	}
	g.EmitCallPlaceholder(ir.EntryFuncName(irmod))
	g.EmitMovsImm(0, 0x18)
	g.LoadImm32(1, 0x00020026)
	g.EmitBkpt(0xAB)
	g.EmitBSelf()

	if err := g.CompileModuleFuncs(); err != nil {
		return err
	}
	if err := g.ResolveCalls(codeBaseAddr); err != nil {
		return err
	}

	code := g.Code()
	symbols := []Symbol{
		{
			Name: "_start",
			Addr: codeBaseAddr,
			Size: 0,
			Info: 0x12, // STB_GLOBAL|STT_FUNC
		},
	}

	type fnSym struct {
		name string
		off  int
	}
	var funcs []fnSym
	seen := map[string]bool{}
	for _, f := range irmod.Funcs {
		if seen[f.Name] {
			continue
		}
		seen[f.Name] = true
		off, ok := g.FunctionOffset(f.Name)
		if !ok {
			continue
		}
		funcs = append(funcs, fnSym{name: f.Name, off: off})
	}
	i := 1
	for i < len(funcs) {
		j := i
		for j > 0 && funcs[j-1].off > funcs[j].off {
			funcs[j-1], funcs[j] = funcs[j], funcs[j-1]
			j = j - 1
		}
		i = i + 1
	}
	for i, fn := range funcs {
		if fn.off < 0 || fn.off >= len(code) {
			continue
		}
		labels := g.FunctionLabels(fn.name)
		var labelIDs []int
		for id := range labels {
			labelIDs = append(labelIDs, id)
		}
		j := 1
		for j < len(labelIDs) {
			k := j
			for k > 0 && labelIDs[k-1] > labelIDs[k] {
				labelIDs[k-1], labelIDs[k] = labelIDs[k], labelIDs[k-1]
				k = k - 1
			}
			j = j + 1
		}
		for _, id := range labelIDs {
			off := labels[id]
			if off < 0 || off >= len(code) {
				continue
			}
			symbols = append(symbols, Symbol{
				Name:  ".L" + fn.name + "." + fmt.Sprintf("%d", id),
				Addr:  codeBaseAddr + uint32(off),
				Size:  0,
				Info:  0x00, // STT_NOTYPE
				Local: true,
			})
		}
		size := len(code) - fn.off
		if i+1 < len(funcs) && funcs[i+1].off > fn.off {
			size = funcs[i+1].off - fn.off
		}
		symbols = append(symbols, Symbol{
			Name: fn.name,
			Addr: codeBaseAddr + uint32(fn.off),
			Size: uint32(size),
			Info: 0x12, // STB_GLOBAL|STT_FUNC
		})
	}

	image := BuildELF32BringupWithSymbols(code, defaultInitialStackPointer, symbols)
	if err := os.WriteFile(outputPath, image, 0755); err != nil {
		return fmt.Errorf("write output: %v", err)
	}
	return nil
}
