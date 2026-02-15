//go:build !no_backend_arm64

package main

import (
	"fmt"
	"os"
)

// generateLinuxArm64ELF compiles an IRModule to a Linux ARM64 ELF binary.
func generateLinuxArm64ELF(irmod *IRModule, outputPath string) error {
	g := &CodeGen{
		funcOffsets:   make(map[string]int),
		labelOffsets:  make(map[int]int),
		stringMap:     make(map[string]int),
		globalOffsets: make([]int, len(irmod.Globals)),
		baseAddr:      0x400000,
		irmod:         irmod,
		wordSize:      8,
		isArm64:       true,
	}

	// Allocate .data space for globals (8 bytes each)
	for i := range irmod.Globals {
		g.globalOffsets[i] = i * 8
	}
	g.data = make([]byte, len(irmod.Globals)*8)

	// Emit _start entry point
	g.emitStartArm64Linux(irmod)

	// Compile all functions
	for _, f := range irmod.Funcs {
		g.funcOffsets[f.Name] = len(g.code)
		g.compileFuncArm64(f)
	}

	collectNativeFuncSizes(irmod, g.funcOffsets, len(g.code))

	// Resolve call fixups (skip special targets handled by buildELF64)
	var unresolved []string
	for _, fix := range g.callFixups {
		if fix.Target == "$rodata_header$" || fix.Target == "$data_addr$" {
			continue
		}
		target, ok := g.funcOffsets[fix.Target]
		if !ok {
			unresolved = append(unresolved, fix.Target)
			continue
		}
		g.patchArm64BAt(fix.CodeOffset, target)
	}
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

	// Build and write ELF
	elf := g.buildELF64(irmod)
	err := os.WriteFile(outputPath, elf, 0755)
	if err != nil {
		return fmt.Errorf("write output: %v", err)
	}

	return nil
}

// emitStartArm64Linux generates the _start entry point for Linux ARM64.
// The kernel enters _start with SP pointing to argc on the stack.
// Linux ARM64 does not need argc/argv/envp — the os package reads from /proc.
func (g *CodeGen) emitStartArm64Linux(irmod *IRModule) {
	// Allocate operand stack: mmap(NULL, 1MB, PROT_READ|PROT_WRITE, MAP_PRIVATE|MAP_ANON, -1, 0)
	// Linux ARM64: SYS_mmap = 222, MAP_ANONYMOUS = 0x20, MAP_PRIVATE = 0x02 → flags = 0x22
	g.emitMovZ(REG_X0, 0, 0)                          // addr = NULL
	g.emitLoadImm64Compact(REG_X1, 1048576)            // len = 1MB
	g.emitLoadImm64Compact(REG_X2, 3)                  // PROT_READ|PROT_WRITE
	g.emitLoadImm64Compact(REG_X3, 0x22)               // MAP_PRIVATE|MAP_ANONYMOUS
	g.emitLoadImm64Compact(REG_X4, 0xFFFFFFFFFFFFFFFF) // fd = -1
	g.emitMovZ(REG_X5, 0, 0)                          // offset = 0
	g.emitLoadImm64Compact(REG_X8, 222)                // SYS_mmap
	g.emitSvc()

	// X28 = mmap result + 1MB (top of operand stack, grows down)
	g.emitLoadImm64Compact(REG_X1, 1048576)
	g.emitAddRR(REG_X28, REG_X0, REG_X1)

	// Call init functions in topological order
	for _, f := range irmod.Funcs {
		if isInitFunc(f.Name) {
			g.emitCallPlaceholderArm64(f.Name)
		}
	}

	// Call main.main
	g.emitCallPlaceholderArm64("main.main")

	// exit_group(0): X8=94, X0=0
	g.emitMovZ(REG_X0, 0, 0)
	g.emitLoadImm64Compact(REG_X8, 94)
	g.emitSvc()
}

// compileSyscallIntrinsicArm64 implements the Syscall intrinsic for Linux ARM64.
// Parameters in locals: num(0), a0(1), a1(2), a2(3), a3(4), a4(5), a5(6)
func (g *CodeGen) compileSyscallIntrinsicArm64(paramCount int) {
	// Load syscall number → X8 (ARM64 Linux convention)
	g.emitLoadLocalArm64(1*8, REG_X8)
	// Load args → X0-X5
	g.emitLoadLocalArm64(2*8, REG_X0)
	g.emitLoadLocalArm64(3*8, REG_X1)
	g.emitLoadLocalArm64(4*8, REG_X2)
	g.emitLoadLocalArm64(5*8, REG_X3)
	g.emitLoadLocalArm64(6*8, REG_X4)
	g.emitLoadLocalArm64(7*8, REG_X5)

	g.emitSvc()

	// Handle return: same pattern as emitSyscallReturnArm64
	g.flush()

	g.emitMovRRArm64(REG_X2, REG_X0)

	g.emitCmpImm(REG_X2, 0)
	errFixup := g.emitBCond(COND_LT) // branch if negative (error)

	// Success: r1=X2, r2=0, err=0
	g.rawPush(REG_X2)
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0) // r2=0
	g.rawPush(REG_X0) // err=0
	doneFixup := g.emitB()

	// Error: r1=0, r2=0, err=-X2
	g.patchArm64BCondAt(errFixup, len(g.code))
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0) // r1=0
	g.rawPush(REG_X0) // r2=0
	g.emitNeg(REG_X0, REG_X2)
	g.rawPush(REG_X0) // err=-X2

	g.patchArm64BAt(doneFixup, len(g.code))
	g.hasPending = false
}

// compilePanicArm64Linux handles panic on Linux ARM64 using direct syscalls.
func (g *CodeGen) compilePanicArm64Linux() {
	// Pop value from operand stack
	g.opPop(REG_X0)

	// Tostring heuristic: if [X0] < 256, it's an interface box
	g.emitLdr(REG_X1, REG_X0, 0)
	g.emitCmpImm(REG_X1, 256)
	stringFixup := g.emitBCond(COND_CS) // branch if unsigned >= 256

	// Interface box: extract value (string ptr at [X0+8])
	g.emitLdr(REG_X0, REG_X0, 8)

	g.patchArm64BCondAt(stringFixup, len(g.code))

	// X0 = string header ptr {data_ptr, len}
	// Save on hardware stack
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X0, REG_SP, 0)

	// Load data_ptr and len
	g.emitLdr(REG_X1, REG_X0, 0) // data_ptr
	g.emitLdr(REG_X2, REG_X0, 8) // len

	// write(2, data_ptr, len): X8=64, X0=2, X1=buf, X2=count
	g.emitLoadImm64Compact(REG_X0, 2) // fd = stderr
	g.emitLoadImm64Compact(REG_X8, 64) // SYS_write
	g.emitSvc()

	// Write newline
	g.emitLoadImm64Compact(REG_X0, 0x0A) // '\n'
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStrb(REG_X0, REG_SP, 0)
	g.emitLoadImm64Compact(REG_X0, 2)    // fd = stderr
	g.emitMovRRArm64(REG_X1, REG_SP)     // buf = SP
	g.emitLoadImm64Compact(REG_X2, 1)    // len = 1
	g.emitLoadImm64Compact(REG_X8, 64)   // SYS_write
	g.emitSvc()
	g.emitAddImm(REG_SP, REG_SP, 16)

	// Restore stack
	g.emitAddImm(REG_SP, REG_SP, 16)

	// exit_group(2): X8=94, X0=2
	g.emitLoadImm64Compact(REG_X0, 2)
	g.emitLoadImm64Compact(REG_X8, 94)
	g.emitSvc()
}
