//go:build !no_backend_windows_i386

package windows

import (
	"fmt"
	"os"

	core "j5.nz/rtg/std/compiler/backend/i386"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// Generate compiles an IRModule to a Windows PE32 executable.
func Generate(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	g := core.NewCodeGen(target, irmod, 0x400000)

	emitStartWin386(g, irmod)
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
		if len(fix.Target) > 5 && fix.Target[0:5] == "$iat$" {
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

	pe := BuildPE32(g, irmod)
	if err := os.WriteFile(outputPath, pe, 0755); err != nil {
		return fmt.Errorf("write output: %v", err)
	}
	return nil
}

// emitStartWin386 generates the Windows entry point.
func emitStartWin386(g *core.CodeGen, irmod *ir.IRModule) {
	// Windows entry point: no arguments passed, we use stdcall.
	// EDI = operand stack pointer (callee-saved)
	// EBP = frame pointer (callee-saved)

	// Save callee-saved registers
	g.PushR32(core.REG32_EBX)
	g.PushR32(core.REG32_ESI)

	// Allocate 16MB operand stack via VirtualAlloc
	// VirtualAlloc(NULL, 16*1048576, MEM_COMMIT|MEM_RESERVE, PAGE_READWRITE)
	// stdcall: push args right-to-left, callee cleans stack
	pushImm32(g, 0x04)       // PAGE_READWRITE
	pushImm32(g, 0x3000)     // MEM_COMMIT | MEM_RESERVE
	pushImm32(g, 16*1048576) // dwSize = 16MB
	pushImm32(g, 0)          // lpAddress = NULL
	g.EmitCallIAT("VirtualAlloc")
	// EAX = base of allocation

	// EDI = EAX + 16MB (top of operand stack, grows down)
	g.MovRR32(core.REG32_EDI, core.REG32_EAX)
	g.AddRI32(core.REG32_EDI, int32(16*1048576))

	for _, f := range irmod.Funcs {
		if ir.IsInitFunc(f.Name) {
			g.EmitCallPlaceholder(f.Name)
		}
	}

	g.EmitCallPlaceholder("main.main")

	pushImm32(g, 0)
	g.EmitCallIAT("ExitProcess")
}

func pushImm32(g *core.CodeGen, val uint32) {
	if val < 128 {
		g.EmitBytes(0x6a, byte(val))
		return
	}
	g.EmitByte(0x68)
	g.EmitU32(val)
}

// loadFdAsHandle loads fd from local, if 0/1/2 calls GetStdHandle, else uses as-is.
// Result in EAX.
func loadFdAsHandle(g *core.CodeGen, localOffset int) {
	g.EmitLoadLocal32(localOffset, core.REG32_EAX)
	g.CmpRI32(core.REG32_EAX, 2)
	fixNotStd := g.JccRel32(0x87) // ja (unsigned above)

	g.NegR32(core.REG32_EAX)
	g.AddRI32(core.REG32_EAX, -10)
	g.PushR32(core.REG32_EAX)
	g.EmitCallIAT("GetStdHandle")
	fixDone := g.JmpRel32()

	g.PatchRel32(fixNotStd)
	g.EmitLoadLocal32(localOffset, core.REG32_EAX)
	g.PatchRel32(fixDone)
}
