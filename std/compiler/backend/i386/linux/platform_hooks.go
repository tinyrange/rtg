//go:build !no_backend_i386 && !no_backend_linux_i386

package linux

import (
	i386 "j5.nz/rtg/std/compiler/backend/i386"
	"j5.nz/rtg/std/compiler/ir"
)

type linuxHooks struct{}

func (h linuxHooks) EmitStart(g *i386.CodeGen, irmod *ir.IRModule) {
	EmitStart(g, irmod)
}

func (h linuxHooks) CompileSyscallIntrinsic(g *i386.CodeGen, paramCount int) {
	CompileSyscallIntrinsic(g, paramCount)
}

func (h linuxHooks) CompileSysGetdents64(g *i386.CodeGen) {
	CompileSysGetdents64(g)
}

func (h linuxHooks) CompileLinkStaticIntrinsic(g *i386.CodeGen, inst ir.Inst) bool {
	return false
}

func (h linuxHooks) CompilePanic(g *i386.CodeGen) {
	CompilePanic(g)
}

// CompileSysGetdents64 is not used on Linux i386 (reserved for Windows shim).
func CompileSysGetdents64(g *i386.CodeGen) {
	panic("ICE: SysGetdents64 not implemented for linux/386 hooks")
}

// EmitStart generates the Linux i386 _start entry point.
func EmitStart(g *i386.CodeGen, irmod *ir.IRModule) {
	// Save ebp before clobbering it for the syscall
	g.PushR32(i386.REG32_EBP)

	// mmap2(NULL, 1048576, 3, 0x22, 0, 0)
	g.XorRR32(i386.REG32_EBX, i386.REG32_EBX)
	g.EmitMovRegImm32(i386.REG32_ECX, 1048576)
	g.EmitMovRegImm32(i386.REG32_EDX, 3)
	g.EmitMovRegImm32(i386.REG32_ESI, 0x22)
	g.XorRR32(i386.REG32_EDI, i386.REG32_EDI)
	g.XorRR32(i386.REG32_EBP, i386.REG32_EBP)
	g.EmitMovRegImm32(i386.REG32_EAX, 192)
	g.EmitInt80()

	// Restore ebp
	g.PopR32(i386.REG32_EBP)

	// edi = eax + 1048576 (operand stack top)
	g.MovRR32(i386.REG32_EDI, i386.REG32_EAX)
	g.AddRI32(i386.REG32_EDI, int32(1048576))

	// Call init functions
	for _, f := range irmod.Funcs {
		if ir.IsInitFunc(f.Name) {
			g.EmitCallPlaceholder(f.Name)
		}
	}

	g.EmitCallPlaceholder("main.main")

	// exit(0): mov eax, 252 (SYS_EXIT_GROUP); xor ebx, ebx; int 0x80
	g.XorRR32(i386.REG32_EBX, i386.REG32_EBX)
	g.EmitMovRegImm32(i386.REG32_EAX, 252)
	g.EmitInt80()
}

func CompileSyscallIntrinsic(g *i386.CodeGen, paramCount int) {
	g.PushR32(i386.REG32_EDI)
	g.PushR32(i386.REG32_EBP)

	g.EmitLoadLocal32(1*4, i386.REG32_EAX)
	g.EmitLoadLocal32(2*4, i386.REG32_EBX)
	g.EmitLoadLocal32(3*4, i386.REG32_ECX)
	g.EmitLoadLocal32(4*4, i386.REG32_EDX)
	g.EmitLoadLocal32(5*4, i386.REG32_ESI)
	g.EmitLoadLocal32(6*4, i386.REG32_EDI)
	g.EmitLoadLocal32(7*4, i386.REG32_EBP)

	g.EmitInt80()

	g.PopR32(i386.REG32_EBP)
	g.PopR32(i386.REG32_EDI)

	g.MovRR32(i386.REG32_ECX, i386.REG32_EDX)
	g.CmpRI32(i386.REG32_EAX, int32(-4095))
	g.EmitBytes(0x72, 0x08)
	g.MovRR32(i386.REG32_EDX, i386.REG32_EAX)
	g.NegR32(i386.REG32_EDX)
	g.XorRR32(i386.REG32_EAX, i386.REG32_EAX)
	g.JmpRel8(0x04)
	g.XorRR32(i386.REG32_EDX, i386.REG32_EDX)
	g.JmpRel8(0x00)

	g.OpPush(i386.REG32_EAX)
	g.OpPush(i386.REG32_ECX)
	g.OpPush(i386.REG32_EDX)
}

func CompilePanic(g *i386.CodeGen) {
	g.OpPop(i386.REG32_EAX)

	g.LoadMem32(i386.REG32_ECX, i386.REG32_EAX, 0)
	g.CmpRI32(i386.REG32_ECX, int32(256))
	g.EmitBytes(0x73, 0x03)
	g.LoadMem32(i386.REG32_EAX, i386.REG32_EAX, 4)

	g.PushR32(i386.REG32_EDI)
	g.PushR32(i386.REG32_EBP)

	g.LoadMem32(i386.REG32_ECX, i386.REG32_EAX, 0)
	g.LoadMem32(i386.REG32_EDX, i386.REG32_EAX, 4)
	g.EmitMovRegImm32(i386.REG32_EBX, 2)
	g.MovRR32(i386.REG32_ECX, i386.REG32_ECX)
	g.PushR32(i386.REG32_ECX)
	g.EmitMovRegImm32(i386.REG32_EBX, 2)
	g.PopR32(i386.REG32_ECX)
	g.EmitMovRegImm32(i386.REG32_EAX, 4)
	g.XorRR32(i386.REG32_EBP, i386.REG32_EBP)
	g.EmitInt80()

	g.EmitBytes(0x6a, 0x0a)
	g.MovRR32(i386.REG32_ECX, i386.REG32_ESP)
	g.EmitMovRegImm32(i386.REG32_EDX, 1)
	g.EmitMovRegImm32(i386.REG32_EBX, 2)
	g.EmitMovRegImm32(i386.REG32_EAX, 4)
	g.EmitInt80()
	g.AddRI32(i386.REG32_ESP, 4)

	g.PopR32(i386.REG32_EBP)
	g.PopR32(i386.REG32_EDI)

	g.EmitMovRegImm32(i386.REG32_EBX, 2)
	g.EmitMovRegImm32(i386.REG32_EAX, 1)
	g.EmitInt80()
}

func init() {
	i386.RegisterOSHooks("linux", linuxHooks{})
}
