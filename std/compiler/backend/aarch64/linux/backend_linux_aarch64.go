//go:build !no_backend_arm64

package linux

import (
	"fmt"
	"os"

	"j5.nz/rtg/std/compiler/backend/aarch64"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// Generate compiles an IRModule to a Linux ARM64 ELF binary.
func Generate(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	return GenerateLinuxELF(target, irmod, outputPath)
}

// GenerateLinuxELF compiles an IRModule to a Linux ARM64 ELF binary.
func GenerateLinuxELF(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	g := aarch64.NewCodeGen(target, irmod, 0x400000, 0, false)

	// Emit _start entry point
	emitStartArm64Linux(g, irmod, common.EntryFuncName(target))

	// Compile all functions
	g.CompileModuleFuncs(irmod)

	g.CollectNativeFuncSizes(irmod)
	if g.NeedTostringHelper() {
		g.EmitTostringHelperArm64()
	}

	// Resolve call fixups (skip special targets handled by BuildELF64)
	unresolved := g.ResolveCallFixups(aarch64.FixupSkipRodataHeader | aarch64.FixupSkipDataAddr)
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
	if target.EmitIRAndBinaryPath != "" {
		if err := aarch64.WriteIRAndBinaryDebug(target.EmitIRAndBinaryPath, irmod, g); err != nil {
			return fmt.Errorf("write debug ir+binary: %v", err)
		}
	}
	elf := BuildELF64(g, irmod)
	err := os.WriteFile(outputPath, elf, 0755)
	if err != nil {
		return fmt.Errorf("write output: %v", err)
	}

	return nil
}

// emitStartArm64Linux generates the _start entry point for Linux ARM64.
// The kernel enters _start with SP pointing to argc on the stack.
// Linux ARM64 does not need argc/argv/envp — the os package reads from /proc.
func emitStartArm64Linux(g *aarch64.CodeGen, irmod *ir.IRModule, entryFunc string) {
	// Allocate operand stack: mmap(NULL, 1MB, PROT_READ|PROT_WRITE, MAP_PRIVATE|MAP_ANON, -1, 0)
	// Linux ARM64: SYS_mmap = 222, MAP_ANONYMOUS = 0x20, MAP_PRIVATE = 0x02 → flags = 0x22
	g.EmitMovZ(aarch64.REG_X0, 0, 0)
	g.EmitLoadImm64Compact(aarch64.REG_X1, 1048576)
	g.EmitLoadImm64Compact(aarch64.REG_X2, 3)
	g.EmitLoadImm64Compact(aarch64.REG_X3, 0x22)
	g.EmitLoadImm64Compact(aarch64.REG_X4, 0xFFFFFFFFFFFFFFFF)
	g.EmitMovZ(aarch64.REG_X5, 0, 0)
	g.EmitLoadImm64Compact(aarch64.REG_X8, 222)
	g.EmitSvc()

	// X28 = mmap result + 1MB (top of operand stack, grows down)
	g.EmitLoadImm64Compact(aarch64.REG_X1, 1048576)
	g.EmitAddRR(aarch64.REG_X28, aarch64.REG_X0, aarch64.REG_X1)

	// Call init functions in topological order
	for _, f := range irmod.Funcs {
		if ir.IsInitFunc(f.Name) {
			g.EmitCallPlaceholderArm64(f.Name)
		}
	}

	// Call entrypoint
	g.EmitCallPlaceholderArm64(ir.EntryFuncName(irmod))

	// exit_group(entryRet0OrZero): X8=94, X0=status
	entryRet := ir.EntryFuncRetCount(irmod)
	if entryRet > 0 {
		g.OpPop(aarch64.REG_X0)
		for i := 1; i < entryRet; i++ {
			g.OpPop(aarch64.REG_X1)
		}
	} else {
		g.EmitMovZ(aarch64.REG_X0, 0, 0)
	}
	g.EmitLoadImm64Compact(aarch64.REG_X8, 94)
	g.EmitSvc()
}
