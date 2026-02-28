//go:build !no_backend_linux_amd64

package x64

import (
	"fmt"
	"os"

	core "j5.nz/rtg/std/compiler/backend/x64"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// === Linux amd64-specific backend code ===

// emitStart generates the _start entry point.
func emitStart(g *core.CodeGen, irmod *ir.IRModule, entryFunc string) {
	// _start:
	//   mmap 1MB for operand stack → R15
	//   call program entrypoint
	//   mov rdi, 0    ; exit code
	//   mov rax, 231  ; SYS_EXIT_GROUP
	//   syscall

	// mmap(0, 1048576, PROT_READ|PROT_WRITE=3, MAP_PRIVATE|MAP_ANONYMOUS=0x22, -1, 0)
	// rax=9, rdi=0, rsi=1048576, rdx=3, r10=0x22, r8=-1(0xffffffffffffffff), r9=0
	g.XorRR(core.REG_RDI, core.REG_RDI)                   // addr = NULL
	g.EmitMovRegImm64(core.REG_RSI, 1048576)              // len = 1MB
	g.EmitByte(0xba)                                      // mov edx, 3
	g.EmitU32(3)                                          // PROT_READ|PROT_WRITE
	g.EmitBytes(0x41, 0xba)                               // mov r10d, 0x22
	g.EmitU32(0x22)                                       // MAP_PRIVATE|MAP_ANONYMOUS
	g.EmitBytes(0x49, 0xc7, 0xc0, 0xff, 0xff, 0xff, 0xff) // mov r8, -1
	g.EmitBytes(0x4d, 0x31, 0xc9)                         // xor r9, r9 (offset = 0)
	g.EmitByte(0xb8)                                      // mov eax, 9
	g.EmitU32(9)                                          // SYS_MMAP
	g.EmitBytes(0x0f, 0x05)                               // syscall

	// R15 = rax + 1048576 (stack grows down, point to top)
	// mov r15, rax
	g.EmitBytes(0x49, 0x89, 0xc7) // mov r15, rax
	// add r15, 1048576
	g.EmitMovRegImm64(core.REG_RCX, 1048576)
	g.EmitBytes(0x49, 0x01, 0xcf) // add r15, rcx

	// Call init functions in topological order
	for _, f := range irmod.Funcs {
		if ir.IsInitFunc(f.Name) {
			g.EmitCallPlaceholder(f.Name)
		}
	}

	// Call entrypoint
	g.EmitCallPlaceholder(ir.EntryFuncName(irmod))

	// exit(0)
	g.XorRR(core.REG_RDI, core.REG_RDI) // exit code 0
	g.EmitByte(0xb8)                    // mov eax, 231
	g.EmitU32(231)                      // SYS_EXIT_GROUP
	g.EmitBytes(0x0f, 0x05)             // syscall
}

func compileSyscallIntrinsic(g *core.CodeGen, paramCount int) {
	// Parameters are in locals 0-6: num, a0, a1, a2, a3, a4, a5
	// Load them into registers for syscall
	// Local 0 = [rbp-8] = syscall number → rax
	g.EmitLoadLocal(1*8, core.REG_RAX) // num → rax
	g.EmitLoadLocal(2*8, core.REG_RDI) // a0 → rdi
	g.EmitLoadLocal(3*8, core.REG_RSI) // a1 → rsi
	g.EmitLoadLocal(4*8, core.REG_RDX) // a2 → rdx
	g.EmitLoadLocal(5*8, core.REG_R10) // a3 → r10
	g.EmitLoadLocal(6*8, core.REG_R8)  // a4 → r8
	g.EmitLoadLocal(7*8, core.REG_R9)  // a5 → r9

	g.EmitBytes(0x0f, 0x05) // syscall

	// Push 3 return values: r1, r2, err
	// Linux syscall convention: on error, rax = -errno (negative)
	// We need: if rax < 0 { r1=0, err=-rax } else { r1=rax, err=0 }

	// Save rdx (r2) before we clobber it
	g.EmitBytes(0x48, 0x89, 0xd1) // mov rcx, rdx (save r2)

	// Test if rax is negative (error)
	g.EmitBytes(0x48, 0x85, 0xc0) // test rax, rax
	g.EmitBytes(0x79, 0x0c)       // jns +12 (skip error case)
	// Error case: err = -rax, r1 = 0
	g.EmitBytes(0x48, 0x89, 0xc2) // mov rdx, rax (save -errno in rdx temporarily)
	g.EmitBytes(0x48, 0xf7, 0xda) // neg rdx       (err = -rax)
	g.EmitBytes(0x48, 0x31, 0xc0) // xor rax, rax  (r1 = 0)
	g.EmitBytes(0xeb, 0x05)       // jmp +5 (skip success case)
	// Success case: err = 0
	g.EmitBytes(0x48, 0x31, 0xd2) // xor rdx, rdx  (err = 0)  -- 3 bytes
	g.EmitBytes(0xeb, 0x00)       // jmp +0 (nop, aligns flow) -- 2 bytes

	// Stack: push r1 (rax), r2 (rcx), err (rdx)
	g.OpPush(core.REG_RAX) // r1
	g.OpPush(core.REG_RCX) // r2
	g.OpPush(core.REG_RDX) // err
}

func compilePanic(g *core.CodeGen) {
	// Pop value from operand stack
	g.OpPop(core.REG_RAX)

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

// GenerateELF compiles an IRModule to an x86-64 ELF binary.
func GenerateELF(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	g := core.NewCodeGen(target, irmod, 0x400000)

	// Emit _start
	emitStart(g, irmod, common.EntryFuncName(target))

	g.EmitAllFunctions(irmod)

	unresolved := g.ResolveLinuxCallFixups()

	if err := g.CheckUnresolvedCalls(unresolved); err != nil {
		return err
	}

	// Build and write ELF
	elf := buildELF64(g, irmod)
	err := os.WriteFile(outputPath, elf, 0755)
	if err != nil {
		return fmt.Errorf("write output: %v", err)
	}

	return nil
}

func compileCallIntrinsicLinux(g *core.CodeGen, inst ir.Inst) {
	switch inst.Name {
	case "Syscall":
		compileSyscallIntrinsic(g, inst.Arg)
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
		panic("ICE: unknown intrinsic '" + inst.Name + "' in compileCallIntrinsic")
	}
}
