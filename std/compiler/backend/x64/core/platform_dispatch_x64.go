//go:build !no_backend_linux_amd64 || !no_backend_windows_amd64

package core

import (
	"strings"

	"j5.nz/rtg/std/compiler/ir"
)

func compileCallIntrinsicTarget(g *CodeGen, inst ir.Inst) {
	if g.target.GOOS == "windows" {
		g.Flush()
		if compileLinkStaticIntrinsicWin64Core(g, inst) {
			return
		}
	}

	switch inst.Name {
	case "Syscall":
		if g.target.GOOS != "linux" {
			panic("ICE: Syscall intrinsic unsupported for GOOS=" + g.target.GOOS)
		}
		_ = inst.Arg
		// Parameters are in locals 0-6: num, a0, a1, a2, a3, a4, a5
		// Load them into registers for syscall
		// Local 0 = [rbp-8] = syscall number -> rax
		g.EmitLoadLocal(1*8, REG_RAX) // num -> rax
		g.EmitLoadLocal(2*8, REG_RDI) // a0 -> rdi
		g.EmitLoadLocal(3*8, REG_RSI) // a1 -> rsi
		g.EmitLoadLocal(4*8, REG_RDX) // a2 -> rdx
		g.EmitLoadLocal(5*8, REG_R10) // a3 -> r10
		g.EmitLoadLocal(6*8, REG_R8)  // a4 -> r8
		g.EmitLoadLocal(7*8, REG_R9)  // a5 -> r9

		g.EmitBytes(0x0f, 0x05) // syscall

		// Push 3 return values: r1, r2, err
		// Linux syscall convention: on error, rax = -errno (negative)
		// We need: if rax < 0 { r1=0, err=-rax } else { r1=rax, err=0 }
		g.EmitBytes(0x48, 0x89, 0xd1) // mov rcx, rdx (save r2)
		g.EmitBytes(0x48, 0x85, 0xc0) // test rax, rax
		g.EmitBytes(0x79, 0x0c)       // jns +12 (skip error case)
		g.EmitBytes(0x48, 0x89, 0xc2) // mov rdx, rax
		g.EmitBytes(0x48, 0xf7, 0xda) // neg rdx
		g.EmitBytes(0x48, 0x31, 0xc0) // xor rax, rax
		g.EmitBytes(0xeb, 0x05)       // jmp +5
		g.EmitBytes(0x48, 0x31, 0xd2) // xor rdx, rdx
		g.EmitBytes(0xeb, 0x00)       // jmp +0

		g.OpPush(REG_RAX) // r1
		g.OpPush(REG_RCX) // r2
		g.OpPush(REG_RDX) // err
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
		panic("ICE: unknown intrinsic '" + inst.Name + "' on GOOS=" + g.target.GOOS)
	}
}

func compilePanicTarget(g *CodeGen) {
	if g.target.GOOS == "windows" {
		compilePanicWin64Core(g)
		return
	}
	if g.target.GOOS != "linux" {
		panic("ICE: panic lowering unavailable for GOOS=" + g.target.GOOS)
	}

	// Pop value from operand stack
	g.OpPop(REG_RAX)

	// Tostring heuristic: if first qword < 256, it's an interface box
	g.EmitBytes(0x48, 0x8b, 0x08) // mov rcx, [rax]
	g.EmitBytes(0x48, 0x81, 0xf9) // cmp rcx, 256
	g.EmitU32(256)
	g.EmitBytes(0x73, 0x04) // jae +4 (skip next instruction)
	// Interface box: extract value field (the string ptr)
	g.EmitBytes(0x48, 0x8b, 0x40, 0x08) // mov rax, [rax+8]
	// .is_string:
	// rax = string header ptr {data_ptr, len}
	g.EmitBytes(0x48, 0x8b, 0x30)             // mov rsi, [rax]     ; data_ptr
	g.EmitBytes(0x48, 0x8b, 0x50, 0x08)       // mov rdx, [rax+8]   ; len
	g.EmitBytes(0xbf, 0x02, 0x00, 0x00, 0x00) // mov edi, 2   ; fd=stderr
	g.EmitBytes(0xb8, 0x01, 0x00, 0x00, 0x00) // mov eax, 1   ; SYS_write
	g.EmitBytes(0x0f, 0x05)                   // syscall

	// Write newline: push '\n' on stack, write 1 byte from rsp
	g.EmitBytes(0x6a, 0x0a)                   // push 0x0a ('\n')
	g.EmitBytes(0x48, 0x89, 0xe6)             // mov rsi, rsp       ; buf = &'\n'
	g.EmitBytes(0xba, 0x01, 0x00, 0x00, 0x00) // mov edx, 1   ; len=1
	g.EmitBytes(0xbf, 0x02, 0x00, 0x00, 0x00) // mov edi, 2   ; fd=stderr
	g.EmitBytes(0xb8, 0x01, 0x00, 0x00, 0x00) // mov eax, 1   ; SYS_write
	g.EmitBytes(0x0f, 0x05)                   // syscall
	g.EmitBytes(0x48, 0x83, 0xc4, 0x08)       // add rsp, 8         ; pop the '\n'

	// Exit(2): align with Go panic process status instead of deliberate SIGSEGV.
	g.EmitBytes(0xbf, 0x02, 0x00, 0x00, 0x00) // mov edi, 2
	g.EmitBytes(0xb8, 0x3c, 0x00, 0x00, 0x00) // mov eax, 60  ; SYS_exit
	g.EmitBytes(0x0f, 0x05)                   // syscall
}

func compilePanicWin64Core(g *CodeGen) {
	g.OpPop(REG_RAX)

	g.EmitBytes(0x48, 0x8b, 0x08)
	g.EmitBytes(0x48, 0x81, 0xf9)
	g.EmitU32(256)
	g.EmitBytes(0x73, 0x04)
	g.EmitBytes(0x48, 0x8b, 0x40, 0x08)

	g.PushR(REG_RBX)
	g.PushR(REG_R12)
	g.LoadMem(REG_RBX, REG_RAX, 0)
	g.LoadMem(REG_R12, REG_RAX, 8)

	g.SubRI(REG_RSP, 32)
	g.EmitMovRegImm64(REG_RCX, 0xFFFFFFFFFFFFFFF4)
	emitCallIATCore(g, "GetStdHandle")

	g.AddRI(REG_RSP, 32)
	g.SubRI(REG_RSP, 48)
	g.MovRR(REG_RCX, REG_RAX)
	g.MovRR(REG_RDX, REG_RBX)
	g.MovRR(REG_R8, REG_R12)
	g.EmitBytes(0x4c, 0x8d, 0x4c, 0x24, 0x28)
	g.EmitBytes(0x48, 0xc7, 0x44, 0x24, 0x20, 0x00, 0x00, 0x00, 0x00)
	emitCallIATCore(g, "WriteFile")
	g.AddRI(REG_RSP, 48)

	g.EmitBytes(0x6a, 0x0a)
	g.MovRR(REG_RBX, REG_RSP)

	g.SubRI(REG_RSP, 32)
	g.EmitMovRegImm64(REG_RCX, 0xFFFFFFFFFFFFFFF4)
	emitCallIATCore(g, "GetStdHandle")
	g.AddRI(REG_RSP, 32)

	g.SubRI(REG_RSP, 48)
	g.MovRR(REG_RCX, REG_RAX)
	g.MovRR(REG_RDX, REG_RBX)
	g.EmitMovRegImm64(REG_R8, 1)
	g.EmitBytes(0x4c, 0x8d, 0x4c, 0x24, 0x28)
	g.EmitBytes(0x48, 0xc7, 0x44, 0x24, 0x20, 0x00, 0x00, 0x00, 0x00)
	emitCallIATCore(g, "WriteFile")
	g.AddRI(REG_RSP, 48)

	g.AddRI(REG_RSP, 8)
	g.PopR(REG_R12)
	g.PopR(REG_RBX)

	g.SubRI(REG_RSP, 32)
	g.EmitMovRegImm64(REG_RCX, 2)
	emitCallIATCore(g, "ExitProcess")
	g.AddRI(REG_RSP, 32)
}

func compileLinkStaticIntrinsicWin64Core(g *CodeGen, inst ir.Inst) bool {
	if g.target.GOOS != "windows" || g.irmod == nil || g.irmod.LinkStaticFuncs == nil {
		return false
	}
	raw, ok := g.irmod.LinkStaticFuncs[inst.Name]
	if !ok {
		return false
	}
	lib, sym, mode, ok := decodeLinkStaticSpecWin64Core(raw)
	if !ok {
		panic("ICE: invalid windows linkstatic metadata for '" + inst.Name + "'")
	}
	lib = canonicalWinImportLibraryCore(lib)
	if mode == "" {
		mode = "syscall"
	}
	emitGenericLinkStaticCallWin64Core(g, inst.Arg, lib, sym)
	emitLinkStaticReturnWin64Core(g, mode)
	return true
}

func decodeLinkStaticSpecWin64Core(raw string) (string, string, string, bool) {
	parts := strings.Split(raw, ",")
	if len(parts) != 3 {
		return "", "", "", false
	}
	lib := strings.TrimSpace(parts[0])
	sym := strings.TrimSpace(parts[1])
	mode := strings.TrimSpace(parts[2])
	if lib == "" || sym == "" {
		return "", "", "", false
	}
	return lib, sym, mode, true
}

func canonicalWinImportLibraryCore(lib string) string {
	lib = strings.TrimSpace(lib)
	if lib == "" {
		return "kernel32.dll"
	}
	return lib
}

func emitCallIATCore(g *CodeGen, funcName string) {
	emitCallIATInLibCore(g, "kernel32.dll", funcName)
}

func emitCallIATInLibCore(g *CodeGen, libName string, funcName string) {
	g.Flush()
	g.EmitBytes(0xFF, 0x15)
	g.AddCallFixup("$iat$" + canonicalWinImportLibraryCore(libName) + "|" + funcName)
	g.EmitU32(0)
}

func emitGenericLinkStaticCallWin64Core(g *CodeGen, paramCount int, lib string, sym string) {
	if paramCount < 0 {
		panic("ICE: invalid linkstatic arg count")
	}
	extra := 0
	if paramCount > 4 {
		extra = paramCount - 4
	}
	alignPad := 0
	if extra&1 != 0 {
		g.SubRI(REG_RSP, 8)
		alignPad = 8
	}
	for i := paramCount; i > 4; i-- {
		g.EmitLoadLocal(i*8, REG_RAX)
		g.PushR(REG_RAX)
	}
	g.SubRI(REG_RSP, 32)
	if paramCount >= 1 {
		g.EmitLoadLocal(1*8, REG_RCX)
	}
	if paramCount >= 2 {
		g.EmitLoadLocal(2*8, REG_RDX)
	}
	if paramCount >= 3 {
		g.EmitLoadLocal(3*8, REG_R8)
	}
	if paramCount >= 4 {
		g.EmitLoadLocal(4*8, REG_R9)
	}
	emitCallIATInLibCore(g, lib, sym)
	g.AddRI(REG_RSP, int32(32+extra*8+alignPad))
}

func emitLinkStaticPtrReturnWin64Core(g *CodeGen) {
	g.TestRR(REG_RAX, REG_RAX)
	fixNonZero := g.JccRel32(CC_NE)
	g.CompileConstI64(0)
	g.CompileConstI64(0)
	g.CompileConstI64(1)
	fixDone := g.JmpRel32()

	g.PatchRel32(fixNonZero)
	g.EmitMovRegImm64(REG_RCX, 0xFFFFFFFFFFFFFFFF)
	g.CmpRR(REG_RAX, REG_RCX)
	fixNotMinusOne := g.JccRel32(CC_NE)
	g.CompileConstI64(0)
	g.CompileConstI64(0)
	g.CompileConstI64(1)
	fixDone2 := g.JmpRel32()

	g.PatchRel32(fixNotMinusOne)
	g.OpPush(REG_RAX)
	g.CompileConstI64(0)
	g.CompileConstI64(0)
	g.PatchRel32(fixDone2)
	g.PatchRel32(fixDone)
}

func emitLinkStaticReturnWin64Core(g *CodeGen, mode string) {
	switch mode {
	case "syscall":
		g.OpPush(REG_RAX)
		g.CompileConstI64(0)
		g.CompileConstI64(0)
	case "ptr":
		emitLinkStaticPtrReturnWin64Core(g)
	case "rawptr":
		g.OpPush(REG_RAX)
		g.CompileConstI64(0)
		g.CompileConstI64(0)
	case "noreturn":
		g.ClearOperandCache()
	default:
		panic("ICE: unknown linkstatic mode '" + mode + "'")
	}
}
