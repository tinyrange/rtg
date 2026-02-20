//go:build !no_backend_linux_i386

package main

// === Linux i386-specific backend code ===

// emitStart_i386 generates the _start entry point for i386.
func (g *CodeGen) emitStart_i386(irmod *IRModule) {
	// _start:
	//   mmap2(NULL, 1MB, PROT_RW, MAP_PRIV|MAP_ANON, 0, 0) via int 0x80
	//   edi = eax + 1MB (operand stack top, grows down)
	//   call init funcs
	//   call main.main
	//   exit(0)

	// i386 syscall ABI: int 0x80
	// eax=num, ebx=a0, ecx=a1, edx=a2, esi=a3, edi=a4, ebp=a5

	// Save ebp before clobbering it for the syscall
	g.pushR32(REG32_EBP)

	// mmap2(NULL, 1048576, 3, 0x22, 0, 0)
	g.xorRR32(REG32_EBX, REG32_EBX)        // addr = NULL
	g.emitMovRegImm32(REG32_ECX, 1048576)   // size = 1MB
	g.emitMovRegImm32(REG32_EDX, 3)         // prot = RW
	g.emitMovRegImm32(REG32_ESI, 0x22)      // flags = PRIVATE|ANONYMOUS
	g.xorRR32(REG32_EDI, REG32_EDI)         // fd = 0
	g.xorRR32(REG32_EBP, REG32_EBP)         // offset = 0
	g.emitMovRegImm32(REG32_EAX, 192)       // SYS_MMAP2
	g.emitInt80()

	// Restore ebp
	g.popR32(REG32_EBP)

	// edi = eax + 1048576 (operand stack top)
	g.movRR32(REG32_EDI, REG32_EAX)
	g.addRI32(REG32_EDI, int32(1048576))

	// Call init functions
	for _, f := range irmod.Funcs {
		if isInitFunc(f.Name) {
			g.emitCallPlaceholder(f.Name)
		}
	}

	// Call main.main
	g.emitCallPlaceholder("main.main")

	// exit(0): mov eax, 252 (SYS_EXIT_GROUP); xor ebx, ebx; int 0x80
	g.xorRR32(REG32_EBX, REG32_EBX)
	g.emitMovRegImm32(REG32_EAX, 252)
	g.emitInt80()
}

func (g *CodeGen) compileSyscallIntrinsic_linux386(paramCount int) {
	// i386 syscall ABI: int 0x80
	// eax=num, ebx=a0, ecx=a1, edx=a2, esi=a3, edi=a4, ebp=a5
	// We must save EDI (operand stack) and EBP (frame) before loading syscall args

	// Save edi and ebp
	g.pushR32(REG32_EDI)
	g.pushR32(REG32_EBP)

	// Load args from frame (ebp still valid at this point since we just pushed it)
	// After push edi, push ebp: esp points to saved ebp
	// Original ebp is at [esp], so we use [esp] to get the old ebp
	// Actually, ebp hasn't been changed yet - the push just saves it on stack
	// ebp still points to the current frame

	g.emitLoadLocal32(1*4, REG32_EAX) // syscall num
	g.emitLoadLocal32(2*4, REG32_EBX) // a0
	g.emitLoadLocal32(3*4, REG32_ECX) // a1
	g.emitLoadLocal32(4*4, REG32_EDX) // a2
	g.emitLoadLocal32(5*4, REG32_ESI) // a3
	// a4 goes into edi, a5 goes into ebp - load these LAST since they clobber
	// our operand stack ptr and frame ptr
	g.emitLoadLocal32(6*4, REG32_EDI) // a4 (clobbers operand stack!)
	// For a5 (ebp), we need special handling since ebp is our frame pointer
	// Load from [ebp-28] before clobbering ebp
	g.emitLoadLocal32(7*4, REG32_EBP) // a5 (clobbers frame ptr!)

	g.emitInt80()

	// Restore ebp and edi
	g.popR32(REG32_EBP)
	g.popR32(REG32_EDI)

	// Handle return: on i386, syscall errors are in range [-4095, -1]
	// (unsigned 0xfffff001..0xffffffff). Addresses >= 0x80000000 are valid.
	// Save edx (r2) before we clobber it
	g.movRR32(REG32_ECX, REG32_EDX) // save r2

	// Check if eax is an error: cmp eax, 0xfffff001; jb success (unsigned)
	g.cmpRI32(REG32_EAX, int32(-4095))       // cmp eax, 0xfffff001
	g.emitBytes(0x72, 0x08)                   // jb +8 (unsigned below = success)
	// Error case: err = -eax, r1 = 0
	g.movRR32(REG32_EDX, REG32_EAX)          // mov edx, eax    (2 bytes)
	g.negR32(REG32_EDX)                       // neg edx         (2 bytes)
	g.xorRR32(REG32_EAX, REG32_EAX)          // xor eax, eax    (2 bytes)
	g.jmpRel8(0x04)                           // jmp +4          (2 bytes)
	// Success case: err = 0
	g.xorRR32(REG32_EDX, REG32_EDX)          // xor edx, edx    (2 bytes)
	g.jmpRel8(0x00)                           // jmp +0 (nop)    (2 bytes)

	// Push r1 (eax), r2 (ecx), err (edx)
	g.opPush(REG32_EAX) // r1
	g.opPush(REG32_ECX) // r2
	g.opPush(REG32_EDX) // err
}

func (g *CodeGen) compilePanic_linux386() {
	// Pop value from operand stack
	g.opPop(REG32_EAX)

	// Tostring heuristic: if first dword < 256, it's an interface box
	g.loadMem32(REG32_ECX, REG32_EAX, 0)     // mov ecx, [eax]
	g.cmpRI32(REG32_ECX, int32(256))           // cmp ecx, 256
	g.emitBytes(0x73, 0x03)                   // jae +3 (skip next instruction)
	// Interface box: extract value field (the string ptr) at [eax+4]
	g.loadMem32(REG32_EAX, REG32_EAX, 4)     // mov eax, [eax+4] (3 bytes)

	// eax = string header ptr {data_ptr:4, len:4}
	// Save edi and ebp before syscall
	g.pushR32(REG32_EDI)
	g.pushR32(REG32_EBP)

	g.loadMem32(REG32_ECX, REG32_EAX, 0)     // ecx = data_ptr
	g.loadMem32(REG32_EDX, REG32_EAX, 4)     // edx = len
	g.emitMovRegImm32(REG32_EBX, 2)          // fd = stderr
	g.movRR32(REG32_ECX, REG32_ECX)          // ecx already has buf
	// Swap: ebx=fd, ecx=buf, edx=count for SYS_WRITE
	// Actually: eax=SYS_WRITE, ebx=fd, ecx=buf, edx=count
	// ecx has data_ptr, edx has len - but we need ebx=fd
	// Save data_ptr, set up regs
	g.pushR32(REG32_ECX)                     // save data_ptr
	g.emitMovRegImm32(REG32_EBX, 2)          // fd = 2 (stderr)
	g.popR32(REG32_ECX)                       // restore data_ptr into ecx (buf)
	g.emitMovRegImm32(REG32_EAX, 4)          // SYS_WRITE = 4
	g.xorRR32(REG32_EBP, REG32_EBP)
	g.emitInt80()

	// Write newline
	g.emitBytes(0x6a, 0x0a)                  // push 0x0a ('\n')
	g.movRR32(REG32_ECX, REG32_ESP)          // buf = esp
	g.emitMovRegImm32(REG32_EDX, 1)          // len = 1
	g.emitMovRegImm32(REG32_EBX, 2)          // fd = stderr
	g.emitMovRegImm32(REG32_EAX, 4)          // SYS_WRITE
	g.emitInt80()
	g.addRI32(REG32_ESP, 4)                   // pop the '\n'

	// Restore edi and ebp
	g.popR32(REG32_EBP)
	g.popR32(REG32_EDI)

	// Crash: null dereference -> SIGSEGV
	g.xorRR32(REG32_EAX, REG32_EAX)
	g.loadMem32(REG32_EAX, REG32_EAX, 0)    // mov eax, [eax] -> segfault
}
