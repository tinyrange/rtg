//go:build !no_backend_darwin_arm64

package main

import (
	"fmt"
	"os"
)

// generateDarwinArm64 compiles an IRModule to a macOS ARM64 Mach-O binary.
func generateDarwinArm64(irmod *IRModule, outputPath string) error {
	g := &CodeGen{
		funcOffsets:   make(map[string]int),
		labelOffsets:  make(map[int]int),
		stringMap:     make(map[string]int),
		globalOffsets: make([]int, len(irmod.Globals)),
		baseAddr:      0x100000000, // standard macOS arm64 VM base
		irmod:         irmod,
		wordSize:      8,
		isArm64:       true,
		gotEntries:    make(map[string]int),
	}

	// Allocate .data space for globals (8 bytes each)
	// Reserve 3 extra globals for argc, argv, envp
	totalGlobals := len(irmod.Globals) + 3
	for i := range irmod.Globals {
		g.globalOffsets[i] = i * 8
	}
	g.data = make([]byte, totalGlobals*8)

	// Emit entry point
	g.emitStartArm64(irmod)

	// Compile all functions
	for _, f := range irmod.Funcs {
		g.funcOffsets[f.Name] = len(g.code)
		g.compileFuncArm64(f)
	}

	collectNativeFuncSizes(irmod, g.funcOffsets, len(g.code))

	// Resolve call fixups (skip special targets handled by buildMachO64)
	var unresolved []string
	for _, fix := range g.callFixups {
		if fix.Target == "$rodata_header$" || fix.Target == "$data_addr$" || fix.Target == "$got_addr$" {
			continue
		}
		target, ok := g.funcOffsets[fix.Target]
		if !ok {
			unresolved = append(unresolved, fix.Target)
			continue
		}
		// ARM64 BL fixup: 26-bit signed offset in 4-byte units
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

	// Extract basename for code signature identifier
	binName := outputPath
	lastSlash := -1
	si := 0
	for si < len(outputPath) {
		if outputPath[si] == '/' {
			lastSlash = si
		}
		si++
	}
	if lastSlash >= 0 {
		binName = outputPath[lastSlash+1:]
	}

	// Build Mach-O binary
	macho := g.buildMachO64(irmod, binName)
	err := os.WriteFile(outputPath, macho, 0755)
	if err != nil {
		return fmt.Errorf("write output: %v", err)
	}

	// Fix permissions (workaround for _open mode issue in arm64 backend)
	os.Chmod(outputPath, 0755)

	return nil
}

// emitStartArm64 generates the entry point for macOS ARM64.
// LC_MAIN receives: X0=argc, X1=argv, X2=envp (as a C function call)
func (g *CodeGen) emitStartArm64(irmod *IRModule) {
	// Save LR (we're called as a function by dyld)
	g.emitStp(REG_FP, REG_LR, REG_SP, -16)
	g.emitMovRRArm64(REG_FP, REG_SP)

	// Save argc, argv, envp to globals (at end of data section)
	argcGlobalOff := len(irmod.Globals) * 8
	argvGlobalOff := (len(irmod.Globals) + 1) * 8
	envpGlobalOff := (len(irmod.Globals) + 2) * 8

	// Save X0 (argc) to global
	g.emitAdrpAdd(REG_X3, "$data_addr$", uint64(argcGlobalOff))
	g.emitStr(REG_X0, REG_X3, 0)

	// Save X1 (argv) to global
	g.emitAdrpAdd(REG_X3, "$data_addr$", uint64(argvGlobalOff))
	g.emitStr(REG_X1, REG_X3, 0)

	// Save X2 (envp) to global
	g.emitAdrpAdd(REG_X3, "$data_addr$", uint64(envpGlobalOff))
	g.emitStr(REG_X2, REG_X3, 0)

	// Allocate operand stack: mmap(NULL, 1MB, PROT_READ|PROT_WRITE, MAP_PRIVATE|MAP_ANON, -1, 0)
	// macOS: MAP_ANONYMOUS = 0x1000, MAP_PRIVATE = 0x02 → flags = 0x1002
	g.emitMovZ(REG_X0, 0, 0)                                   // addr = NULL
	g.emitLoadImm64Compact(REG_X1, 1048576)                     // len = 1MB
	g.emitLoadImm64Compact(REG_X2, 3)                           // PROT_READ|PROT_WRITE
	g.emitLoadImm64Compact(REG_X3, 0x1002)                      // MAP_PRIVATE|MAP_ANON
	g.emitLoadImm64Compact(REG_X4, 0xFFFFFFFFFFFFFFFF)          // fd = -1
	g.emitMovZ(REG_X5, 0, 0)                                   // offset = 0
	g.emitCallGOT("_mmap")

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

	// exit(0)
	g.emitMovZ(REG_X0, 0, 0)
	g.emitCallGOT("_exit")

	// Epilogue (won't reach here, but keep it clean)
	g.emitMovRRArm64(REG_SP, REG_FP)
	g.emitLdp(REG_FP, REG_LR, REG_SP, 16)
	g.emitRet()
}

// emitSyscallReturnArm64 handles the standard libSystem return convention.
// Returns -1 on error. We check X0 < 0: if negative, err = -X0, r1 = 0.
// Uses rawPush to avoid pending-push optimization issues across branches.
func (g *CodeGen) emitSyscallReturnArm64() {
	g.flush() // ensure clean state before branching

	// Save return value in X2 (preserved across our code below)
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
	g.hasPending = false // clean state after merge
}

// emitSyscallReturnPtrArm64 handles pointer-returning calls (NULL or MAP_FAILED = error).
// Uses rawPush to avoid pending-push optimization issues across branches.
func (g *CodeGen) emitSyscallReturnPtrArm64() {
	g.flush() // ensure clean state before branching

	// Save return value in X2
	g.emitMovRRArm64(REG_X2, REG_X0)

	// Check for NULL (0) or MAP_FAILED (-1, i.e. negative)
	g.emitCmpImm(REG_X2, 0)
	errFixup := g.emitBCond(COND_LE) // branch if <= 0 (NULL or negative)

	// Success: r1=X2, r2=0, err=0
	g.rawPush(REG_X2)
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0) // r2=0
	g.rawPush(REG_X0) // err=0
	doneFixup := g.emitB()

	// Error: r1=0, r2=0, err=1
	g.patchArm64BCondAt(errFixup, len(g.code))
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0) // r1=0
	g.rawPush(REG_X0) // r2=0
	g.emitLoadImm64Compact(REG_X0, 1)
	g.rawPush(REG_X0) // err=1

	g.patchArm64BAt(doneFixup, len(g.code))
	g.hasPending = false // clean state after merge
}

// compilePanicArm64 handles panic on macOS ARM64.
func (g *CodeGen) compilePanicArm64() {
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
	// Save on hardware stack (callee-saved)
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X0, REG_SP, 0)

	// Load data_ptr and len
	g.emitLdr(REG_X1, REG_X0, 0) // data_ptr
	g.emitLdr(REG_X2, REG_X0, 8) // len

	// _write(2, data_ptr, len)
	g.emitLoadImm64Compact(REG_X0, 2) // fd = stderr
	g.emitCallGOT("_write")

	// Write newline: allocate 1 byte on stack
	g.emitLoadImm64Compact(REG_X0, 0x0A) // '\n'
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStrb(REG_X0, REG_SP, 0)
	g.emitLoadImm64Compact(REG_X0, 2)    // fd = stderr
	g.emitMovRRArm64(REG_X1, REG_SP)     // buf = SP
	g.emitLoadImm64Compact(REG_X2, 1)    // len = 1
	g.emitCallGOT("_write")
	g.emitAddImm(REG_SP, REG_SP, 16)

	// Restore stack
	g.emitAddImm(REG_SP, REG_SP, 16)

	// _exit(2)
	g.emitLoadImm64Compact(REG_X0, 2)
	g.emitCallGOT("_exit")
}
