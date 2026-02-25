//go:build !no_backend_linux_i386

package linux

import (
	core "j5.nz/rtg/std/compiler/backend/i386"
	"j5.nz/rtg/std/compiler/ir"
)

// EmitStart generates the _start entry point for i386.
func EmitStart(g *core.CodeGen, irmod *ir.IRModule) {
	// _start:
	//   mmap2(NULL, 1MB, PROT_RW, MAP_PRIV|MAP_ANON, 0, 0) via int 0x80
	//   edi = eax + 1MB (operand stack top, grows down)
	//   call init funcs
	//   call main.main
	//   exit(0)

	// i386 syscall ABI: int 0x80
	// eax=num, ebx=a0, ecx=a1, edx=a2, esi=a3, edi=a4, ebp=a5

	// Save ebp before clobbering it for the syscall
	g.PushR32(core.REG32_EBP)

	// mmap2(NULL, 1048576, 3, 0x22, 0, 0)
	g.XorRR32(core.REG32_EBX, core.REG32_EBX)  // addr = NULL
	g.EmitMovRegImm32(core.REG32_ECX, 1048576) // size = 1MB
	g.EmitMovRegImm32(core.REG32_EDX, 3)       // prot = RW
	g.EmitMovRegImm32(core.REG32_ESI, 0x22)    // flags = PRIVATE|ANONYMOUS
	g.XorRR32(core.REG32_EDI, core.REG32_EDI)  // fd = 0
	g.XorRR32(core.REG32_EBP, core.REG32_EBP)  // offset = 0
	g.EmitMovRegImm32(core.REG32_EAX, 192)     // SYS_MMAP2
	g.EmitInt80()

	// Restore ebp
	g.PopR32(core.REG32_EBP)

	// edi = eax + 1048576 (operand stack top)
	g.MovRR32(core.REG32_EDI, core.REG32_EAX)
	g.AddRI32(core.REG32_EDI, int32(1048576))

	// Call init functions
	for _, f := range irmod.Funcs {
		if ir.IsInitFunc(f.Name) {
			g.EmitCallPlaceholder(f.Name)
		}
	}

	// Call main.main
	g.EmitCallPlaceholder("main.main")

	// exit(0): mov eax, 252 (SYS_EXIT_GROUP); xor ebx, ebx; int 0x80
	g.XorRR32(core.REG32_EBX, core.REG32_EBX)
	g.EmitMovRegImm32(core.REG32_EAX, 252)
	g.EmitInt80()
}

func compileSyscallIntrinsicLinux(g *core.CodeGen, paramCount int) {
	_ = paramCount
	// i386 syscall ABI: int 0x80
	// eax=num, ebx=a0, ecx=a1, edx=a2, esi=a3, edi=a4, ebp=a5
	// We must save EDI (operand stack) and EBP (frame) before loading syscall args

	// Save edi and ebp
	g.PushR32(core.REG32_EDI)
	g.PushR32(core.REG32_EBP)

	g.EmitLoadLocal32(1*4, core.REG32_EAX) // syscall num
	g.EmitLoadLocal32(2*4, core.REG32_EBX) // a0
	g.EmitLoadLocal32(3*4, core.REG32_ECX) // a1
	g.EmitLoadLocal32(4*4, core.REG32_EDX) // a2
	g.EmitLoadLocal32(5*4, core.REG32_ESI) // a3
	g.EmitLoadLocal32(6*4, core.REG32_EDI) // a4 (clobbers operand stack)
	g.EmitLoadLocal32(7*4, core.REG32_EBP) // a5 (clobbers frame pointer)

	g.EmitInt80()

	// Restore ebp and edi
	g.PopR32(core.REG32_EBP)
	g.PopR32(core.REG32_EDI)

	// Handle return: on i386, syscall errors are in range [-4095, -1]
	g.MovRR32(core.REG32_ECX, core.REG32_EDX) // save r2

	// Check if eax is an error: cmp eax, 0xfffff001; jb success (unsigned)
	g.CmpRI32(core.REG32_EAX, int32(-4095)) // cmp eax, 0xfffff001
	g.EmitBytes(0x72, 0x08)                 // jb +8 (unsigned below = success)
	// Error case: err = -eax, r1 = 0
	g.MovRR32(core.REG32_EDX, core.REG32_EAX)
	g.NegR32(core.REG32_EDX)
	g.XorRR32(core.REG32_EAX, core.REG32_EAX)
	g.JmpRel8(0x04)
	// Success case: err = 0
	g.XorRR32(core.REG32_EDX, core.REG32_EDX)
	g.JmpRel8(0x00)

	// Push r1 (eax), r2 (ecx), err (edx)
	g.OpPush(core.REG32_EAX)
	g.OpPush(core.REG32_ECX)
	g.OpPush(core.REG32_EDX)
}

func compilePanicLinux(g *core.CodeGen) {
	// Pop value from operand stack
	g.OpPop(core.REG32_EAX)

	// Tostring heuristic: if first dword < 256, it's an interface box
	g.LoadMem32(core.REG32_ECX, core.REG32_EAX, 0)
	g.CmpRI32(core.REG32_ECX, int32(256))
	g.EmitBytes(0x73, 0x03)
	g.LoadMem32(core.REG32_EAX, core.REG32_EAX, 4)

	// eax = string header ptr {data_ptr:4, len:4}
	g.PushR32(core.REG32_EDI)
	g.PushR32(core.REG32_EBP)

	g.LoadMem32(core.REG32_ECX, core.REG32_EAX, 0) // ecx = data_ptr
	g.LoadMem32(core.REG32_EDX, core.REG32_EAX, 4) // edx = len
	g.EmitMovRegImm32(core.REG32_EBX, 2)           // fd = stderr
	g.MovRR32(core.REG32_ECX, core.REG32_ECX)      // ecx already has buf
	g.PushR32(core.REG32_ECX)
	g.EmitMovRegImm32(core.REG32_EBX, 2)
	g.PopR32(core.REG32_ECX)
	g.EmitMovRegImm32(core.REG32_EAX, 4) // SYS_WRITE = 4
	g.XorRR32(core.REG32_EBP, core.REG32_EBP)
	g.EmitInt80()

	// Write newline
	g.EmitBytes(0x6a, 0x0a)
	g.MovRR32(core.REG32_ECX, core.REG32_ESP)
	g.EmitMovRegImm32(core.REG32_EDX, 1)
	g.EmitMovRegImm32(core.REG32_EBX, 2)
	g.EmitMovRegImm32(core.REG32_EAX, 4)
	g.EmitInt80()
	g.AddRI32(core.REG32_ESP, 4)

	g.PopR32(core.REG32_EBP)
	g.PopR32(core.REG32_EDI)

	// exit(2): return a clean panic status instead of deliberate SIGSEGV.
	g.EmitMovRegImm32(core.REG32_EBX, 2)
	g.EmitMovRegImm32(core.REG32_EAX, 1)
	g.EmitInt80()
}
