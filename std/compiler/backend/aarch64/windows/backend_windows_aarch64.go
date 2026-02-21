//go:build !no_backend_arm64

package windows

import (
	"fmt"
	"os"

	"j5.nz/rtg/std/compiler/backend/aarch64"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// winArm64Imports lists all kernel32.dll functions needed by the Windows ARM64 backend.
var winArm64Imports = []string{
	"VirtualAlloc",
	"ExitProcess",
	"GetStdHandle",
	"WriteFile",
	"ReadFile",
	"CreateFileA",
	"CloseHandle",
	"GetCommandLineA",
	"GetEnvironmentStringsA",
	"FreeEnvironmentStringsA",
	"GetCurrentDirectoryA",
	"CreateDirectoryA",
	"RemoveDirectoryA",
	"DeleteFileA",
	"FindFirstFileA",
	"FindNextFileA",
	"FindClose",
	"GetFileAttributesExA",
	"CreateProcessA",
	"WaitForSingleObject",
	"GetExitCodeProcess",
	"CreatePipe",
	"SetStdHandle",
	"SetHandleInformation",
	"GetLastError",
	"GetCurrentProcessId",
}

// GenerateWinPE compiles an IRModule to a Windows ARM64 PE32+ executable.
func GenerateWinPE(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	g := aarch64.NewCodeGen(target, irmod, 0x400000, 0, false)

	// Emit entry point
	emitStartArm64Windows(g, irmod)

	// Compile all functions
	g.CompileModuleFuncs(irmod)

	g.CollectNativeFuncSizes(irmod)
	if g.NeedTostringHelper() {
		g.EmitTostringHelperArm64()
	}

	// Resolve call fixups (skip special targets handled by BuildPE64)
	unresolved := g.ResolveCallFixups(aarch64.FixupSkipRodataHeader | aarch64.FixupSkipDataAddr | aarch64.FixupSkipIAT)
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

	// Build PE32+
	pe := BuildPE64(g, irmod, winArm64Imports)
	err := os.WriteFile(outputPath, pe, 0755)
	if err != nil {
		return fmt.Errorf("write output: %v", err)
	}

	return nil
}

// emitStartArm64Windows generates the Windows ARM64 entry point.
func emitStartArm64Windows(g *aarch64.CodeGen, irmod *ir.IRModule) {
	// Save LR (entry is called by Windows loader)
	g.EmitStp(aarch64.REG_FP, aarch64.REG_LR, aarch64.REG_SP, -16)
	g.EmitMovRRArm64(aarch64.REG_FP, aarch64.REG_SP)

	// Allocate 16MB operand stack via VirtualAlloc
	g.EmitMovZ(aarch64.REG_X0, 0, 0)
	g.EmitLoadImm64Compact(aarch64.REG_X1, 16*1048576)
	g.EmitLoadImm64Compact(aarch64.REG_X2, 0x3000)
	g.EmitLoadImm64Compact(aarch64.REG_X3, 0x04)
	emitCallIATArm64(g, "VirtualAlloc")

	g.EmitLoadImm64Compact(aarch64.REG_X1, 16*1048576)
	g.EmitAddRR(aarch64.REG_X28, aarch64.REG_X0, aarch64.REG_X1)

	for _, f := range irmod.Funcs {
		if ir.IsInitFunc(f.Name) {
			g.EmitCallPlaceholderArm64(f.Name)
		}
	}
	g.EmitCallPlaceholderArm64("main.main")

	g.EmitMovZ(aarch64.REG_X0, 0, 0)
	emitCallIATArm64(g, "ExitProcess")

	g.EmitMovRRArm64(aarch64.REG_SP, aarch64.REG_FP)
	g.EmitLdp(aarch64.REG_FP, aarch64.REG_LR, aarch64.REG_SP, 16)
	g.EmitRet()
}

// emitCallIATArm64 emits ADRP+LDR X16 (placeholder) then BLR X16 for calling a Windows IAT entry.
func emitCallIATArm64(g *aarch64.CodeGen, funcName string) {
	g.Flush()
	off := g.EmitAdrp(aarch64.REG_X16)
	inst := uint32(0xF9400000) | (uint32(aarch64.REG_X16&0x1f) << 5) | uint32(aarch64.REG_X16&0x1f)
	g.EmitArm64(inst)
	g.AddCallFixup(off, "$iat$"+funcName, 0)
	g.EmitBlr(aarch64.REG_X16)
}
