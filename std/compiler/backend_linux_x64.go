//go:build !no_backend_linux_amd64

package main

// === Linux amd64-specific backend code ===

// emitStart generates the _start entry point.
func (g *CodeGen) emitStart(irmod *IRModule) {
	// _start:
	//   mmap 1MB for operand stack → R15
	//   call main.main
	//   mov rdi, 0    ; exit code
	//   mov rax, 231  ; SYS_EXIT_GROUP
	//   syscall

	// mmap(0, 1048576, PROT_READ|PROT_WRITE=3, MAP_PRIVATE|MAP_ANONYMOUS=0x22, -1, 0)
	// rax=9, rdi=0, rsi=1048576, rdx=3, r10=0x22, r8=-1(0xffffffffffffffff), r9=0
	g.xorRR(REG_RDI, REG_RDI)         // addr = NULL
	g.emitMovRegImm64(REG_RSI, 1048576) // len = 1MB
	g.emitByte(0xba)                   // mov edx, 3
	g.emitU32(3)                       // PROT_READ|PROT_WRITE
	g.emitBytes(0x41, 0xba)           // mov r10d, 0x22
	g.emitU32(0x22)                    // MAP_PRIVATE|MAP_ANONYMOUS
	g.emitBytes(0x49, 0xc7, 0xc0, 0xff, 0xff, 0xff, 0xff) // mov r8, -1
	g.emitBytes(0x4d, 0x31, 0xc9)     // xor r9, r9 (offset = 0)
	g.emitByte(0xb8)                   // mov eax, 9
	g.emitU32(9)                       // SYS_MMAP
	g.emitBytes(0x0f, 0x05)                       // syscall

	// R15 = rax + 1048576 (stack grows down, point to top)
	// mov r15, rax
	g.emitBytes(0x49, 0x89, 0xc7) // mov r15, rax
	// add r15, 1048576
	g.emitMovRegImm64(REG_RCX, 1048576)
	g.emitBytes(0x49, 0x01, 0xcf) // add r15, rcx

	// Call init functions in topological order
	for _, f := range irmod.Funcs {
		if isInitFunc(f.Name) {
			g.emitCallPlaceholder(f.Name)
		}
	}

	// Call main.main
	g.emitCallPlaceholder("main.main")

	// exit(0)
	g.xorRR(REG_RDI, REG_RDI) // exit code 0
	g.emitByte(0xb8)           // mov eax, 231
	g.emitU32(231)             // SYS_EXIT_GROUP
	g.emitBytes(0x0f, 0x05)         // syscall
}

func (g *CodeGen) compileSyscallIntrinsic(paramCount int) {
	// Parameters are in locals 0-6: num, a0, a1, a2, a3, a4, a5
	// Load them into registers for syscall
	// Local 0 = [rbp-8] = syscall number → rax
	g.emitLoadLocal(1*8, REG_RAX) // num → rax
	g.emitLoadLocal(2*8, REG_RDI) // a0 → rdi
	g.emitLoadLocal(3*8, REG_RSI) // a1 → rsi
	g.emitLoadLocal(4*8, REG_RDX) // a2 → rdx
	g.emitLoadLocal(5*8, REG_R10) // a3 → r10
	g.emitLoadLocal(6*8, REG_R8)  // a4 → r8
	g.emitLoadLocal(7*8, REG_R9)  // a5 → r9

	g.emitBytes(0x0f, 0x05) // syscall

	// Push 3 return values: r1, r2, err
	// Linux syscall convention: on error, rax = -errno (negative)
	// We need: if rax < 0 { r1=0, err=-rax } else { r1=rax, err=0 }

	// Save rdx (r2) before we clobber it
	g.emitBytes(0x48, 0x89, 0xd1) // mov rcx, rdx (save r2)

	// Test if rax is negative (error)
	g.emitBytes(0x48, 0x85, 0xc0) // test rax, rax
	g.emitBytes(0x79, 0x0c)       // jns +12 (skip error case)
	// Error case: err = -rax, r1 = 0
	g.emitBytes(0x48, 0x89, 0xc2) // mov rdx, rax (save -errno in rdx temporarily)
	g.emitBytes(0x48, 0xf7, 0xda) // neg rdx       (err = -rax)
	g.emitBytes(0x48, 0x31, 0xc0) // xor rax, rax  (r1 = 0)
	g.emitBytes(0xeb, 0x05)       // jmp +5 (skip success case)
	// Success case: err = 0
	g.emitBytes(0x48, 0x31, 0xd2) // xor rdx, rdx  (err = 0)  -- 3 bytes
	g.emitBytes(0xeb, 0x00)       // jmp +0 (nop, aligns flow) -- 2 bytes

	// Stack: push r1 (rax), r2 (rcx), err (rdx)
	g.opPush(REG_RAX) // r1
	g.opPush(REG_RCX) // r2
	g.opPush(REG_RDX) // err
}

func (g *CodeGen) compilePanic() {
	// Pop value from operand stack
	g.opPop(REG_RAX)

	// Tostring heuristic: if first qword < 256, it's an interface box
	g.emitBytes(0x48, 0x8b, 0x08) // mov rcx, [rax]
	g.emitBytes(0x48, 0x81, 0xf9) // cmp rcx, 256
	g.emitU32(256)
	g.emitBytes(0x73, 0x04) // jae +4 (skip next instruction)
	// Interface box: extract value field (the string ptr)
	g.emitBytes(0x48, 0x8b, 0x40, 0x08) // mov rax, [rax+8]
	// .is_string:
	// rax = string header ptr {data_ptr, len}
	g.emitBytes(0x48, 0x8b, 0x30)             // mov rsi, [rax]     ; data_ptr
	g.emitBytes(0x48, 0x8b, 0x50, 0x08)       // mov rdx, [rax+8]   ; len
	g.emitBytes(0xbf, 0x02, 0x00, 0x00, 0x00) // mov edi, 2   ; fd=stderr
	g.emitBytes(0xb8, 0x01, 0x00, 0x00, 0x00) // mov eax, 1   ; SYS_write
	g.emitBytes(0x0f, 0x05)                   // syscall

	// Write newline: push '\n' on stack, write 1 byte from rsp
	g.emitBytes(0x6a, 0x0a)                   // push 0x0a ('\n')
	g.emitBytes(0x48, 0x89, 0xe6)             // mov rsi, rsp       ; buf = &'\n'
	g.emitBytes(0xba, 0x01, 0x00, 0x00, 0x00) // mov edx, 1   ; len=1
	g.emitBytes(0xbf, 0x02, 0x00, 0x00, 0x00) // mov edi, 2   ; fd=stderr
	g.emitBytes(0xb8, 0x01, 0x00, 0x00, 0x00) // mov eax, 1   ; SYS_write
	g.emitBytes(0x0f, 0x05)                   // syscall
	g.emitBytes(0x48, 0x83, 0xc4, 0x08)       // add rsp, 8         ; pop the '\n'

	// Crash: null dereference → SIGSEGV
	g.emitBytes(0x48, 0x31, 0xc0) // xor rax, rax
	g.emitBytes(0x48, 0x8b, 0x00) // mov rax, [rax]
}
