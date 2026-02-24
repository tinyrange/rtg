//go:build !no_backend_windows_amd64

package x64

import (
	"fmt"
	"os"

	"j5.nz/rtg/std/compiler/backend/x64/core"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// Generate compiles an IRModule to a Windows PE32+ (x86-64) executable.
func Generate(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	g := core.NewCodeGen(target, irmod, 0x400000)

	// Emit entry point
	emitStart(g, irmod)

	g.EmitAllFunctions(irmod)

	// Resolve call fixups (skip $rodata_header$, $data_addr$, $iat$ — handled by buildPE64)
	var unresolved []string
	for _, fix := range g.CallFixups() {
		if fix.Target == "$rodata_header$" || fix.Target == "$data_addr$" {
			continue
		}
		if len(fix.Target) > 5 && fix.Target[0:5] == "$iat$" {
			continue
		}
		target, ok := g.MaybeGetFuncOffsets(fix.Target)
		if !ok {
			unresolved = append(unresolved, fix.Target)
			continue
		}
		g.PatchRel32At(fix.CodeOffset, target)
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
