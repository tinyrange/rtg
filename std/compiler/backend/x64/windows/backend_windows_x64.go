//go:build !no_backend_windows_amd64

package windows

import (
	"fmt"
	"os"

	"j5.nz/rtg/std/compiler/backend/x64"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// GenerateWinPE compiles an IRModule to a Windows amd64 PE32+ executable.
func GenerateWinPE(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	g := x64.NewCodeGen(target, irmod, 0x400000)

	g.EmitStartWin64(irmod)
	g.CompileModuleFuncs(irmod)
	g.CollectNativeFuncSizes(irmod)
	if g.NeedTostringHelper() {
		g.EmitTostringHelperX64()
	}

	unresolved := g.ResolveCallFixups(x64.FixupSkipRodataHeader | x64.FixupSkipDataAddr | x64.FixupSkipIAT)
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

	pe := g.BuildPE64(irmod)
	if err := os.WriteFile(outputPath, pe, 0755); err != nil {
		return fmt.Errorf("write output: %v", err)
	}
	return nil
}
