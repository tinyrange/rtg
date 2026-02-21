//go:build !no_backend_arm64

package macos

import (
	"fmt"
	"os"

	"j5.nz/rtg/std/compiler/backend/aarch64"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// GenerateDarwin compiles an IRModule to a macOS ARM64 Mach-O binary.
func GenerateDarwin(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	g := aarch64.NewCodeGen(target, irmod, 0x100000000, 3, true)

	// Emit entry point
	emitStartArm64(g, irmod)

	// Compile all functions
	g.CompileModuleFuncs(irmod)

	g.CollectNativeFuncSizes(irmod)
	if g.NeedTostringHelper() {
		g.EmitTostringHelperArm64()
	}

	// Resolve call fixups (skip special targets handled by BuildMachO64)
	unresolved := g.ResolveCallFixups(aarch64.FixupSkipRodataHeader | aarch64.FixupSkipDataAddr | aarch64.FixupSkipGotAddr)
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
	for i := 0; i < len(outputPath); i++ {
		if outputPath[i] == '/' {
			lastSlash = i
		}
	}
	if lastSlash >= 0 {
		binName = outputPath[lastSlash+1:]
	}

	// Build Mach-O binary
	macho := BuildMachO64(g, irmod, binName)
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
func emitStartArm64(g *aarch64.CodeGen, irmod *ir.IRModule) {
	// Save LR (we're called as a function by dyld)
	g.EmitStp(aarch64.REG_FP, aarch64.REG_LR, aarch64.REG_SP, -16)
	g.EmitMovRRArm64(aarch64.REG_FP, aarch64.REG_SP)

	// Save argc, argv, envp to globals (at end of data section)
	argcGlobalOff := len(irmod.Globals) * 8
	argvGlobalOff := (len(irmod.Globals) + 1) * 8
	envpGlobalOff := (len(irmod.Globals) + 2) * 8

	g.EmitAdrpAdd(aarch64.REG_X3, "$data_addr$", uint64(argcGlobalOff))
	g.EmitStr(aarch64.REG_X0, aarch64.REG_X3, 0)
	g.EmitAdrpAdd(aarch64.REG_X3, "$data_addr$", uint64(argvGlobalOff))
	g.EmitStr(aarch64.REG_X1, aarch64.REG_X3, 0)
	g.EmitAdrpAdd(aarch64.REG_X3, "$data_addr$", uint64(envpGlobalOff))
	g.EmitStr(aarch64.REG_X2, aarch64.REG_X3, 0)

	// Allocate operand stack via mmap
	g.EmitMovZ(aarch64.REG_X0, 0, 0)
	g.EmitLoadImm64Compact(aarch64.REG_X1, 1048576)
	g.EmitLoadImm64Compact(aarch64.REG_X2, 3)
	g.EmitLoadImm64Compact(aarch64.REG_X3, 0x1002)
	g.EmitLoadImm64Compact(aarch64.REG_X4, 0xFFFFFFFFFFFFFFFF)
	g.EmitMovZ(aarch64.REG_X5, 0, 0)
	g.EmitCallGOT("_mmap")

	g.EmitLoadImm64Compact(aarch64.REG_X1, 1048576)
	g.EmitAddRR(aarch64.REG_X28, aarch64.REG_X0, aarch64.REG_X1)

	for _, f := range irmod.Funcs {
		if ir.IsInitFunc(f.Name) {
			g.EmitCallPlaceholderArm64(f.Name)
		}
	}
	g.EmitCallPlaceholderArm64("main.main")

	g.EmitMovZ(aarch64.REG_X0, 0, 0)
	g.EmitCallGOT("_exit")

	g.EmitMovRRArm64(aarch64.REG_SP, aarch64.REG_FP)
	g.EmitLdp(aarch64.REG_FP, aarch64.REG_LR, aarch64.REG_SP, 16)
	g.EmitRet()
}
