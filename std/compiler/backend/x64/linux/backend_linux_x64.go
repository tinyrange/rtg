//go:build !no_backend_linux_amd64

package linux

import (
	"fmt"
	"os"

	"j5.nz/rtg/std/compiler/backend/x64"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// GenerateLinuxELF compiles an IRModule to an x86-64 ELF binary.
func GenerateLinuxELF(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	g := x64.NewCodeGen(target, irmod, 0x400000)

	emitStartLinux(g, irmod)
	g.CompileModuleFuncs(irmod)
	g.CollectNativeFuncSizes(irmod)
	if g.NeedTostringHelper() {
		g.EmitTostringHelperX64()
	}

	unresolved := g.ResolveCallFixups(x64.FixupSkipRodataHeader | x64.FixupSkipDataAddr)
	if len(unresolved) > 0 {
		fmt.Fprintf(os.Stderr, "error: %d unresolved calls:\n", len(unresolved))
		seen := make(map[string]bool)
		for _, name := range unresolved {
			if !seen[name] {
				fmt.Fprintf(os.Stderr, "  %s\n", name)
				seen[name] = true
			}
		}
		return fmt.Errorf("%d unresolved calls", len(unresolved))
	}

	elf := g.BuildELF64(irmod)
	if err := os.WriteFile(outputPath, elf, 0755); err != nil {
		return fmt.Errorf("write output: %v", err)
	}
	return nil
}

func init() {
	x64.RegisterPlatformHooks("linux", platformHooks{})
}

type platformHooks struct{}

func (h platformHooks) CompileIntrinsic(g *x64.CodeGen, name string, arg int) bool {
	_ = h
	if name != "Syscall" {
		return false
	}
	compileSyscallIntrinsicLinux(g, arg)
	return true
}

func (h platformHooks) Panic(g *x64.CodeGen) {
	_ = h
	compilePanicLinux(g)
}

func (h platformHooks) AlignFrameBytes(frameBytes int) int {
	_ = h
	return frameBytes
}

func (h platformHooks) ShouldSkipCallFixup(target string, skipMask int) bool {
	_ = h
	_ = target
	_ = skipMask
	return false
}

// emitStartLinux generates the _start entry point.
func emitStartLinux(g *x64.CodeGen, irmod *ir.IRModule) {
	// _start:
	//   mmap 1MB for operand stack -> R15
	//   call main.main
	//   mov rdi, 0    ; exit code
	//   mov rax, 231  ; SYS_EXIT_GROUP
	//   syscall

	// mmap(0, 1048576, PROT_READ|PROT_WRITE=3, MAP_PRIVATE|MAP_ANONYMOUS=0x22, -1, 0)
	// rax=9, rdi=0, rsi=1048576, rdx=3, r10=0x22, r8=-1(0xffffffffffffffff), r9=0
	g.XorRR(x64.REG_RDI, x64.REG_RDI)                     // addr = NULL
	g.EmitMovRegImm64(x64.REG_RSI, 1048576)               // len = 1MB
	g.EmitByte(0xba)                                      // mov edx, 3
	g.EmitU32(3)                                          // PROT_READ|PROT_WRITE
	g.EmitBytes(0x41, 0xba)                               // mov r10d, 0x22
	g.EmitU32(0x22)                                       // MAP_PRIVATE|MAP_ANONYMOUS
	g.EmitBytes(0x49, 0xc7, 0xc0, 0xff, 0xff, 0xff, 0xff) // mov r8, -1
	g.EmitBytes(0x4d, 0x31, 0xc9)                         // xor r9, r9 (offset = 0)
	g.EmitByte(0xb8)                                      // mov eax, 9
	g.EmitU32(9)                                          // SYS_MMAP
	g.EmitBytes(0x0f, 0x05)                               // syscall
	g.EmitBytes(0x49, 0x89, 0xc7)                         // mov r15, rax
	g.EmitMovRegImm64(x64.REG_RCX, 1048576)               // add r15, 1048576
	g.EmitBytes(0x49, 0x01, 0xcf)                         // add r15, rcx

	for _, f := range irmod.Funcs {
		if ir.IsInitFunc(f.Name) {
			g.EmitCallPlaceholder(f.Name)
		}
	}

	g.EmitCallPlaceholder("main.main")

	g.XorRR(x64.REG_RDI, x64.REG_RDI) // exit code 0
	g.EmitByte(0xb8)                  // mov eax, 231
	g.EmitU32(231)                    // SYS_EXIT_GROUP
	g.EmitBytes(0x0f, 0x05)           // syscall
}

func compileSyscallIntrinsicLinux(g *x64.CodeGen, paramCount int) {
	_ = paramCount
	// Parameters are in locals 0-6: num, a0, a1, a2, a3, a4, a5
	g.EmitLoadLocal(1*8, x64.REG_RAX) // num -> rax
	g.EmitLoadLocal(2*8, x64.REG_RDI) // a0 -> rdi
	g.EmitLoadLocal(3*8, x64.REG_RSI) // a1 -> rsi
	g.EmitLoadLocal(4*8, x64.REG_RDX) // a2 -> rdx
	g.EmitLoadLocal(5*8, x64.REG_R10) // a3 -> r10
	g.EmitLoadLocal(6*8, x64.REG_R8)  // a4 -> r8
	g.EmitLoadLocal(7*8, x64.REG_R9)  // a5 -> r9

	g.EmitBytes(0x0f, 0x05) // syscall

	g.EmitBytes(0x48, 0x89, 0xd1) // mov rcx, rdx (save r2)
	g.EmitBytes(0x48, 0x85, 0xc0) // test rax, rax
	g.EmitBytes(0x79, 0x0c)       // jns +12 (skip error case)
	g.EmitBytes(0x48, 0x89, 0xc2) // mov rdx, rax
	g.EmitBytes(0x48, 0xf7, 0xda) // neg rdx
	g.EmitBytes(0x48, 0x31, 0xc0) // xor rax, rax
	g.EmitBytes(0xeb, 0x05)       // jmp +5
	g.EmitBytes(0x48, 0x31, 0xd2) // xor rdx, rdx
	g.EmitBytes(0xeb, 0x00)       // jmp +0 (nop)

	g.OpPush(x64.REG_RAX) // r1
	g.OpPush(x64.REG_RCX) // r2
	g.OpPush(x64.REG_RDX) // err
}

func compilePanicLinux(g *x64.CodeGen) {
	g.OpPop(x64.REG_RAX)

	g.EmitBytes(0x48, 0x8b, 0x08) // mov rcx, [rax]
	g.EmitBytes(0x48, 0x81, 0xf9) // cmp rcx, 256
	g.EmitU32(256)
	g.EmitBytes(0x73, 0x04)             // jae +4
	g.EmitBytes(0x48, 0x8b, 0x40, 0x08) // mov rax, [rax+8]
	g.EmitBytes(0x48, 0x8b, 0x30)       // mov rsi, [rax]
	g.EmitBytes(0x48, 0x8b, 0x50, 0x08) // mov rdx, [rax+8]
	g.EmitBytes(0xbf, 0x02, 0x00, 0x00, 0x00)
	g.EmitBytes(0xb8, 0x01, 0x00, 0x00, 0x00)
	g.EmitBytes(0x0f, 0x05)

	g.EmitBytes(0x6a, 0x0a)
	g.EmitBytes(0x48, 0x89, 0xe6)
	g.EmitBytes(0xba, 0x01, 0x00, 0x00, 0x00)
	g.EmitBytes(0xbf, 0x02, 0x00, 0x00, 0x00)
	g.EmitBytes(0xb8, 0x01, 0x00, 0x00, 0x00)
	g.EmitBytes(0x0f, 0x05)
	g.EmitBytes(0x48, 0x83, 0xc4, 0x08)

	g.EmitBytes(0xbf, 0x02, 0x00, 0x00, 0x00)
	g.EmitBytes(0xb8, 0x3c, 0x00, 0x00, 0x00)
	g.EmitBytes(0x0f, 0x05)
}
