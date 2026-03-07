//go:build !no_backend_windows_i386

package windows

import (
	"fmt"
	"os"

	core "j5.nz/rtg/std/compiler/backend/i386"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// ediSaveSlot is the offset in the .data section where EDI is saved
// for callback thunks to reload the RTG operand stack pointer.
var ediSaveSlot int

// Generate compiles an IRModule to a Windows PE32 executable.
func Generate(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	g := core.NewCodeGen(target, irmod, 0x400000)

	// Reserve a slot in .data for saving EDI (the RTG operand stack pointer).
	ediSaveSlot = len(g.Data)
	g.Data = append(g.Data, 0, 0, 0, 0)

	emitStartWin386(g, irmod, common.EntryFuncName(target))
	g.CompileModuleFuncs(irmod)
	g.CollectNativeFuncSizes(irmod)
	if g.NeedTostringHelper {
		g.EmitTostringHelperI386()
	}

	// Emit callback thunks for //rtg:callback functions
	emitCallbackThunks(g, irmod)

	var unresolved []string
	for _, fix := range g.CallFixups {
		if fix.Target == "$rodata_header$" || fix.Target == "$data_addr$" {
			continue
		}
		if len(fix.Target) > 5 && fix.Target[0:5] == "$iat$" {
			continue
		}
		if len(fix.Target) > 10 && fix.Target[0:10] == "$funcaddr$" {
			refName := fix.Target[10:]
			if _, ok := g.FuncOffsets[refName]; !ok {
				unresolved = append(unresolved, fix.Target)
			}
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
func emitStartWin386(g *core.CodeGen, irmod *ir.IRModule, entryFunc string) {
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

	// Save EDI to a global so callback thunks can reload it.
	emitSaveEDI(g)

	for _, f := range irmod.Funcs {
		if ir.IsInitFunc(f.Name) {
			g.EmitCallPlaceholder(f.Name)
		}
	}

	g.EmitCallPlaceholder(entryFunc)

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

// emitSaveEDI stores EDI to the ediSaveSlot global in the .data section.
// Uses EAX as scratch (caller must not depend on EAX).
func emitSaveEDI(g *core.CodeGen) {
	// mov eax, <ediSaveSlot offset>  (patched to data VA by PE builder)
	g.EmitMovRegImm32(core.REG32_EAX, uint32(ediSaveSlot))
	g.CallFixups = append(g.CallFixups, core.CallFixup{len(g.Code) - 4, "$data_addr$", 0})
	// mov [eax], edi
	g.EmitBytes(0x89, 0x38)
}

// emitLoadEDI loads EDI from the ediSaveSlot global. Uses EAX as scratch.
func emitLoadEDI(g *core.CodeGen) {
	// mov eax, <ediSaveSlot offset>  (patched to data VA by PE builder)
	g.EmitMovRegImm32(core.REG32_EAX, uint32(ediSaveSlot))
	g.CallFixups = append(g.CallFixups, core.CallFixup{len(g.Code) - 4, "$data_addr$", 0})
	// mov edi, [eax]
	g.EmitBytes(0x8b, 0x38)
}

// emitCallbackThunks emits Win32-stdcall-compatible thunk wrappers for each
// function marked with //rtg:callback. The thunk bridges the Win32 stdcall
// calling convention (params on stack) to RTG's operand-stack convention
// (params pushed onto EDI stack, return value popped from EDI).
func emitCallbackThunks(g *core.CodeGen, irmod *ir.IRModule) {
	if irmod.CallbackFuncs == nil {
		return
	}
	for funcName := range irmod.CallbackFuncs {
		thunkName := "$callback_thunk$" + funcName
		g.FuncOffsets[thunkName] = len(g.Code)
		emitCallbackThunk(g, funcName, irmod)
	}
}

func emitCallbackThunk(g *core.CodeGen, funcName string, irmod *ir.IRModule) {
	// Look up the function's param count from IRModule
	paramCount := 0
	for _, f := range irmod.Funcs {
		if f.Name == funcName {
			paramCount = f.Params
			break
		}
	}

	// Win32 stdcall entry:
	//   [ESP]    = return address
	//   [ESP+4]  = arg0
	//   [ESP+8]  = arg1
	//   ...
	// After push ebp; mov ebp, esp:
	//   [EBP+8]  = arg0
	//   [EBP+12] = arg1
	//   ...

	// Prologue
	g.EmitBytes(0x55)       // push ebp
	g.EmitBytes(0x89, 0xe5) // mov ebp, esp

	// Save callee-saved registers that RTG code will clobber
	g.EmitBytes(0x53) // push ebx  (RTG operand cache)
	g.EmitBytes(0x56) // push esi  (RTG operand cache)
	g.EmitBytes(0x57) // push edi  (we'll overwrite with RTG opstack)

	// Load EDI (RTG operand stack pointer) from the global save slot
	emitLoadEDI(g)

	// Push params onto RTG operand stack (EDI) in left-to-right order
	// RTG expects: first param pushed first (deepest), last param on top
	for i := 0; i < paramCount; i++ {
		off := 8 + i*4                     // [ebp+8], [ebp+12], ...
		g.EmitBytes(0x8b, 0x45, byte(off)) // mov eax, [ebp+off]
		g.EmitBytes(0x8d, 0x7f, 0xfc)      // lea edi, [edi-4]
		g.EmitBytes(0x89, 0x07)            // mov [edi], eax
	}

	// Call the RTG function body
	g.EmitCallPlaceholder(funcName)

	// Pop return value from EDI operand stack into EAX
	g.EmitBytes(0x8b, 0x07)       // mov eax, [edi]
	g.EmitBytes(0x8d, 0x7f, 0x04) // lea edi, [edi+4]

	// Save EDI back to global (for nested callbacks)
	// Use ECX as scratch to preserve return value in EAX
	g.EmitBytes(0x89, 0xc1) // mov ecx, eax  (save return value)
	emitSaveEDI(g)
	g.EmitBytes(0x89, 0xc8) // mov eax, ecx  (restore return value)

	// Restore callee-saved registers
	g.EmitBytes(0x5f) // pop edi
	g.EmitBytes(0x5e) // pop esi
	g.EmitBytes(0x5b) // pop ebx
	g.EmitBytes(0x5d) // pop ebp

	// stdcall: callee cleans up parameters
	if paramCount > 0 {
		g.EmitBytes(0xc2) // ret imm16
		g.EmitBytes(byte(paramCount*4), byte((paramCount*4)>>8))
	} else {
		g.EmitBytes(0xc3) // ret
	}
}
