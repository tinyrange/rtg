//go:build !no_backend_windows_amd64

package x64

import (
	"fmt"
	"os"

	core "j5.nz/rtg/std/compiler/backend/x64"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// r15SaveSlot is the offset in the .data section where R15 is saved
// for callback thunks to reload the RTG operand stack pointer.
var r15SaveSlot int

// Generate compiles an IRModule to a Windows PE32+ (x86-64) executable.
func Generate(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	g := core.NewCodeGen(target, irmod, 0x400000)

	// Reserve a slot in .data for saving R15 (the RTG operand stack pointer).
	// Callbacks from Windows see a different R15 because callee-saved only means
	// "restored before return", not "constant during execution".
	r15SaveSlot = len(g.Data)
	g.Data = append(g.Data, 0, 0, 0, 0, 0, 0, 0, 0)

	// Emit entry point
	emitStart(g, irmod)

	g.EmitAllFunctions(irmod)

	// Emit callback thunks for //rtg:callback functions
	emitCallbackThunks(g, irmod)

	// Resolve call fixups (skip $rodata_header$, $data_addr$, $iat$, $funcaddr$ — handled by buildPE64)
	var unresolved []string
	for _, fix := range g.CallFixups() {
		targetName := core.CallFixupTarget(fix)
		if targetName == "$rodata_header$" || targetName == "$data_addr$" {
			continue
		}
		if len(targetName) > 5 && targetName[0:5] == "$iat$" {
			continue
		}
		if len(targetName) > 10 && targetName[0:10] == "$funcaddr$" {
			continue
		}
		target, ok := g.MaybeGetFuncOffsets(targetName)
		if !ok {
			unresolved = append(unresolved, targetName)
			continue
		}
		g.PatchRel32At(core.CallFixupOffset(fix), target)
	}

	if err := g.CheckUnresolvedCalls(unresolved); err != nil {
		return err
	}

	// Build PE32+
	pe := buildPE64(g, irmod)
	err := os.WriteFile(outputPath, pe, 0755)
	if err != nil {
		return fmt.Errorf("write output: %v", err)
	}

	return nil
}

// emitStart generates the Windows x64 entry point.
func emitStart(g *core.CodeGen, irmod *ir.IRModule) {
	// Windows x64 entry point. RSP is 16-byte aligned + 8 on entry
	// (the loader calls us via `call`, pushing a return address).
	// We use R15 as the operand stack pointer (callee-saved, preserved by kernel32).

	// Allocate shadow space (32 bytes) + 8 alignment (to restore 16-byte alignment)
	g.SubRI(core.REG_RSP, 40) // 32 shadow + 8 to realign RSP to 16 bytes

	// VirtualAlloc(NULL, 16MB, MEM_COMMIT|MEM_RESERVE, PAGE_READWRITE)
	// Microsoft x64 ABI: RCX, RDX, R8, R9
	g.XorRR(core.REG_RCX, core.REG_RCX)         // lpAddress = NULL
	g.EmitMovRegImm64(core.REG_RDX, 16*1048576) // dwSize = 16MB
	g.EmitMovRegImm64(core.REG_R8, 0x3000)      // MEM_COMMIT | MEM_RESERVE
	g.EmitMovRegImm64(core.REG_R9, 0x04)        // PAGE_READWRITE
	emitCallIAT(g, "VirtualAlloc")

	// R15 = RAX + 16MB (operand stack top, grows down)
	g.MovRR(core.REG_R15, core.REG_RAX)
	g.EmitMovRegImm64(core.REG_RCX, 16*1048576)
	g.AddRR(core.REG_R15, core.REG_RCX)

	// Save R15 to a global so callback thunks can reload it.
	// (Callee-saved only guarantees restore-on-return, not constant-during-call.)
	emitSaveR15(g)

	// Call init functions
	for _, f := range irmod.Funcs {
		if ir.IsInitFunc(f.Name) {
			g.EmitCallPlaceholder(f.Name)
		}
	}

	// Call main.main
	g.EmitCallPlaceholder("main.main")

	// ExitProcess(0)
	g.XorRR(core.REG_RCX, core.REG_RCX) // uExitCode = 0
	emitCallIAT(g, "ExitProcess")
}

// === Intrinsic dispatcher for Windows x64 ===

func compileCallIntrinsicWin64(g *core.CodeGen, inst ir.Inst) {
	g.Flush()
	if compileLinkStaticIntrinsicWin64(g, inst) {
		return
	}
	switch inst.Name {
	case "Sliceptr":
		g.CompileSliceptrIntrinsic()
	case "Makeslice":
		g.CompileMakesliceIntrinsic()
	case "Stringptr":
		g.CompileStringptrIntrinsic()
	case "Makestring":
		g.CompileMakestringIntrinsic()
	case "Tostring":
		g.CompileTostringIntrinsic()
	case "ReadPtr":
		g.CompileReadPtrIntrinsic()
	case "WritePtr":
		g.CompileWritePtrIntrinsic()
	case "WriteByte":
		g.CompileWriteByteIntrinsic()
	default:
		panic("ICE: unknown intrinsic '" + inst.Name + "' in compileCallIntrinsicWin64")
	}
}

// compilePanicWin64 handles panic on Windows x64.
func compilePanicWin64(g *core.CodeGen) {
	// Pop value from operand stack
	g.OpPop(core.REG_RAX)

	// Tostring heuristic: if first qword < 256, it's an interface box
	g.EmitBytes(0x48, 0x8b, 0x08) // mov rcx, [rax]
	g.EmitBytes(0x48, 0x81, 0xf9) // cmp rcx, 256
	g.EmitU32(256)
	g.EmitBytes(0x73, 0x04) // jae +4 (skip next instruction)
	// Interface box: extract value field (the string ptr)
	g.EmitBytes(0x48, 0x8b, 0x40, 0x08) // mov rax, [rax+8]

	// RAX = string header ptr {data_ptr, len}
	// Save string info to RBX/R12 (callee-saved, safe across Win64 API calls)
	g.PushR(core.REG_RBX)
	g.PushR(core.REG_R12)
	g.LoadMem(core.REG_RBX, core.REG_RAX, 0) // RBX = data_ptr
	g.LoadMem(core.REG_R12, core.REG_RAX, 8) // R12 = len

	// GetStdHandle(STD_ERROR_HANDLE = -12)
	g.SubRI(core.REG_RSP, 32)
	g.EmitMovRegImm64(core.REG_RCX, 0xFFFFFFFFFFFFFFF4) // -12
	emitCallIAT(g, "GetStdHandle")
	// RAX = stderr handle

	// WriteFile(hFile, lpBuffer, nBytes, &nwritten, NULL)
	// Reuse stack: 32 shadow + 8 for 5th arg + 8 for nwritten = 48
	g.AddRI(core.REG_RSP, 32)
	g.SubRI(core.REG_RSP, 48)
	g.MovRR(core.REG_RCX, core.REG_RAX)                               // hFile = stderr
	g.MovRR(core.REG_RDX, core.REG_RBX)                               // lpBuffer = data_ptr
	g.MovRR(core.REG_R8, core.REG_R12)                                // nBytes = len
	g.EmitBytes(0x4c, 0x8d, 0x4c, 0x24, 0x28)                         // lea r9, [rsp+40] = &nwritten
	g.EmitBytes(0x48, 0xc7, 0x44, 0x24, 0x20, 0x00, 0x00, 0x00, 0x00) // mov qword [rsp+32], 0 (lpOverlapped)
	emitCallIAT(g, "WriteFile")
	g.AddRI(core.REG_RSP, 48)

	// Write newline: push '\n' onto stack as a 1-byte buffer
	g.EmitBytes(0x6a, 0x0a)             // push 0x0a
	g.MovRR(core.REG_RBX, core.REG_RSP) // RBX = &'\n'

	// GetStdHandle(STD_ERROR_HANDLE)
	g.SubRI(core.REG_RSP, 32)
	g.EmitMovRegImm64(core.REG_RCX, 0xFFFFFFFFFFFFFFF4)
	emitCallIAT(g, "GetStdHandle")
	g.AddRI(core.REG_RSP, 32)

	// WriteFile newline
	g.SubRI(core.REG_RSP, 48)
	g.MovRR(core.REG_RCX, core.REG_RAX)                               // hFile
	g.MovRR(core.REG_RDX, core.REG_RBX)                               // lpBuffer = &'\n'
	g.EmitMovRegImm64(core.REG_R8, 1)                                 // nBytes = 1
	g.EmitBytes(0x4c, 0x8d, 0x4c, 0x24, 0x28)                         // lea r9, [rsp+40]
	g.EmitBytes(0x48, 0xc7, 0x44, 0x24, 0x20, 0x00, 0x00, 0x00, 0x00) // mov qword [rsp+32], 0
	emitCallIAT(g, "WriteFile")
	g.AddRI(core.REG_RSP, 48)

	g.AddRI(core.REG_RSP, 8) // pop '\n' slot

	// Restore callee-saved
	g.PopR(core.REG_R12)
	g.PopR(core.REG_RBX)

	// ExitProcess(2)
	g.SubRI(core.REG_RSP, 32)
	g.EmitMovRegImm64(core.REG_RCX, 2)
	emitCallIAT(g, "ExitProcess")
	g.AddRI(core.REG_RSP, 32)
}

// emitCallIAT emits `call dword ptr [abs32]` for calling Windows IAT entries.
func emitCallIAT(g *core.CodeGen, funcName string) {
	emitCallIATInLib(g, winDefaultImportLibrary, funcName)
}

func emitCallIATInLib(g *core.CodeGen, libName string, funcName string) {
	g.Flush()
	g.EmitBytes(0xFF, 0x15) // call dword ptr [abs32]
	g.AddCallFixup(encodeIATFixupTarget(libName, funcName))
	g.EmitU32(0) // placeholder
}

// emitSaveR15 stores R15 to the r15SaveSlot global in the .data section.
// Uses RAX as scratch (caller must not depend on RAX).
func emitSaveR15(g *core.CodeGen) {
	// movabs rax, <r15SaveSlot offset>  (patched to data VA by PE builder)
	g.EmitMovRegImm64(core.REG_RAX, uint64(r15SaveSlot))
	g.AddCallFixupAt("$data_addr$", len(g.Code)-8)
	// mov [rax], r15
	g.EmitBytes(0x4c, 0x89, 0x38) // REX.WR (0x4C) MOV r/m64,r64 ModRM(00,111,000)
}

// emitLoadR15 loads R15 from the r15SaveSlot global. Uses RAX as scratch.
func emitLoadR15(g *core.CodeGen) {
	// movabs rax, <r15SaveSlot offset>  (patched to data VA by PE builder)
	g.EmitMovRegImm64(core.REG_RAX, uint64(r15SaveSlot))
	g.AddCallFixupAt("$data_addr$", len(g.Code)-8)
	// mov r15, [rax]
	g.EmitBytes(0x4c, 0x8b, 0x38) // REX.WR (0x4C) MOV r64,r/m64 ModRM(00,111,000)
}

// emitCallbackThunks emits Win64-ABI-compatible thunk wrappers for each
// function marked with //rtg:callback. The thunk bridges the Win64 calling
// convention (params in RCX, RDX, R8, R9) to RTG's operand-stack convention
// (params pushed onto R15 stack, return value popped from R15).
func emitCallbackThunks(g *core.CodeGen, irmod *ir.IRModule) {
	if irmod.CallbackFuncs == nil {
		return
	}
	for funcName := range irmod.CallbackFuncs {
		thunkName := "$callback_thunk$" + funcName
		g.SetFuncOffset(thunkName, len(g.Code))
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

	// Win64 ABI entry: RCX=arg0, RDX=arg1, R8=arg2, R9=arg3
	//
	// R15 is callee-saved in Win64 — it's restored before a call returns,
	// but NOT held constant during execution. When Windows calls us back,
	// R15 may be anything. We reload the RTG operand stack from a global.

	// Prologue: push rbp; mov rbp, rsp; sub rsp, 64
	g.EmitBytes(0x55)                   // push rbp
	g.EmitBytes(0x48, 0x89, 0xe5)       // mov rbp, rsp
	g.EmitBytes(0x48, 0x83, 0xec, 0x40) // sub rsp, 64

	// Save Win64 params to local frame
	g.EmitBytes(0x48, 0x89, 0x4d, 0xf8) // mov [rbp-8], rcx   (arg0)
	g.EmitBytes(0x48, 0x89, 0x55, 0xf0) // mov [rbp-16], rdx  (arg1)
	g.EmitBytes(0x4c, 0x89, 0x45, 0xe8) // mov [rbp-24], r8   (arg2)
	g.EmitBytes(0x4c, 0x89, 0x4d, 0xe0) // mov [rbp-32], r9   (arg3)

	// Load R15 (RTG operand stack pointer) from the global save slot.
	emitLoadR15(g)

	// Push params onto RTG operand stack (R15) in left-to-right order.
	// RTG expects: first param pushed first (deepest), last param on top.
	offsets := []byte{0xf8, 0xf0, 0xe8, 0xe0} // -8, -16, -24, -32
	for i := 0; i < paramCount && i < 4; i++ {
		g.EmitBytes(0x48, 0x8b, 0x45, offsets[i]) // mov rax, [rbp+offset]
		g.EmitBytes(0x4d, 0x8d, 0x7f, 0xf8)       // lea r15, [r15-8]
		g.EmitBytes(0x49, 0x89, 0x07)              // mov [r15], rax
	}

	// Call the RTG function body
	g.EmitCallPlaceholder(funcName)

	// Pop return value from R15 operand stack into RAX
	g.EmitBytes(0x49, 0x8b, 0x07)       // mov rax, [r15]
	g.EmitBytes(0x4d, 0x8d, 0x7f, 0x08) // lea r15, [r15+8]

	// Save R15 back to global (RTG code in the callback may have moved it)
	// Actually, push/pop is balanced so R15 is back where it was. But save
	// it anyway for safety with nested callbacks.
	emitSaveR15(g)

	// Epilogue: leave + ret
	g.EmitBytes(0xc9) // leave (mov rsp, rbp; pop rbp)
	g.EmitBytes(0xc3) // ret
}
