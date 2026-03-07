//go:build !no_backend_linux_i386

package linux

import (
	core "j5.nz/rtg/std/compiler/backend/i386"
	"j5.nz/rtg/std/compiler/ir"
)

// EmitStart generates the _start entry point for i386.
func EmitStart(g *core.CodeGen, irmod *ir.IRModule, entryFunc string) {
	// _start:
	//   mmap2(NULL, 1MB, PROT_RW, MAP_PRIV|MAP_ANON, 0, 0) via int 0x80
	//   edi = eax + 1MB (operand stack top, grows down)
	//   call init funcs
	//   call program entrypoint
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

	// Call entrypoint
	g.EmitCallPlaceholder(ir.EntryFuncName(irmod))

	// exit(entryRet0OrZero): mov eax, 252 (SYS_EXIT_GROUP); int 0x80
	entryRet := ir.EntryFuncRetCount(irmod)
	if entryRet > 0 {
		g.OpPop(core.REG32_EBX)
		for i := 1; i < entryRet; i++ {
			g.OpPop(core.REG32_EAX)
		}
	} else {
		g.XorRR32(core.REG32_EBX, core.REG32_EBX)
	}
	g.EmitMovRegImm32(core.REG32_EAX, 252)
	g.EmitInt80()
}
