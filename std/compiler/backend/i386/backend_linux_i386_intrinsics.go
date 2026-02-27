//go:build !no_backend_linux_i386

package i386

//rtg:profile
func (g *CodeGen) compileSyscallIntrinsic_linux386(paramCount int) {
	_ = paramCount
	// i386 syscall ABI: int 0x80
	// eax=num, ebx=a0, ecx=a1, edx=a2, esi=a3, edi=a4, ebp=a5
	// We must save EDI (operand stack) and EBP (frame) before loading syscall args

	// Save edi and ebp
	g.pushR32(REG32_EDI)
	g.pushR32(REG32_EBP)

	g.emitLoadLocal32(1*4, REG32_EAX) // syscall num
	g.emitLoadLocal32(2*4, REG32_EBX) // a0
	g.emitLoadLocal32(3*4, REG32_ECX) // a1
	g.emitLoadLocal32(4*4, REG32_EDX) // a2
	g.emitLoadLocal32(5*4, REG32_ESI) // a3
	g.emitLoadLocal32(6*4, REG32_EDI) // a4 (clobbers operand stack)
	g.emitLoadLocal32(7*4, REG32_EBP) // a5 (clobbers frame ptr)

	g.emitInt80()

	// Restore ebp and edi
	g.popR32(REG32_EBP)
	g.popR32(REG32_EDI)

	// Handle return: on i386, syscall errors are in range [-4095, -1]
	g.movRR32(REG32_ECX, REG32_EDX) // save r2

	// Check if eax is an error: cmp eax, 0xfffff001; jb success (unsigned)
	g.cmpRI32(REG32_EAX, int32(-4095))
	g.emitBytes(0x72, 0x08) // jb +8
	// Error case: err = -eax, r1 = 0
	g.movRR32(REG32_EDX, REG32_EAX)
	g.negR32(REG32_EDX)
	g.xorRR32(REG32_EAX, REG32_EAX)
	g.jmpRel8(0x04)
	// Success case: err = 0
	g.xorRR32(REG32_EDX, REG32_EDX)
	g.jmpRel8(0x00)

	g.opPush(REG32_EAX)
	g.opPush(REG32_ECX)
	g.opPush(REG32_EDX)
}

//rtg:profile
func (g *CodeGen) compilePanic_linux386() {
	g.opPop(REG32_EAX)

	// Tostring heuristic: if first dword < 256, it's an interface box
	g.loadMem32(REG32_ECX, REG32_EAX, 0)
	g.cmpRI32(REG32_ECX, int32(256))
	g.emitBytes(0x73, 0x03)
	g.loadMem32(REG32_EAX, REG32_EAX, 4)

	// eax = string header ptr {data_ptr:4, len:4}
	g.pushR32(REG32_EDI)
	g.pushR32(REG32_EBP)

	g.loadMem32(REG32_ECX, REG32_EAX, 0)
	g.loadMem32(REG32_EDX, REG32_EAX, 4)
	g.emitMovRegImm32(REG32_EBX, 2)
	g.movRR32(REG32_ECX, REG32_ECX)
	g.pushR32(REG32_ECX)
	g.emitMovRegImm32(REG32_EBX, 2)
	g.popR32(REG32_ECX)
	g.emitMovRegImm32(REG32_EAX, 4)
	g.xorRR32(REG32_EBP, REG32_EBP)
	g.emitInt80()

	g.emitBytes(0x6a, 0x0a)
	g.movRR32(REG32_ECX, REG32_ESP)
	g.emitMovRegImm32(REG32_EDX, 1)
	g.emitMovRegImm32(REG32_EBX, 2)
	g.emitMovRegImm32(REG32_EAX, 4)
	g.emitInt80()
	g.addRI32(REG32_ESP, 4)

	g.popR32(REG32_EBP)
	g.popR32(REG32_EDI)

	// exit(2): return a clean panic status instead of deliberate SIGSEGV.
	g.emitMovRegImm32(REG32_EBX, 2)
	g.emitMovRegImm32(REG32_EAX, 1)
	g.emitInt80()
}
