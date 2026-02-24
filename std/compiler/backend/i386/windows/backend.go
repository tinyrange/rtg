//go:build !no_backend_i386 && !no_backend_windows_i386

package windows

import (
	"fmt"
	"os"

	"j5.nz/rtg/std/compiler/backend/i386"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// Generate compiles an IRModule to a Windows i386 PE executable.
func Generate(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	g := i386.NewCodeGen(target, irmod, 0x400000, 4)
	g.InitGlobals(4, len(irmod.Globals))

	wrap(g).emitStart_win386(irmod)
	g.CompileModuleFuncsI386(irmod)
	g.CollectNativeFuncSizes(irmod)
	if g.NeedTostringHelper() {
		g.EmitTostringHelperI386Shared()
	}

	unresolved := g.ResolveCallFixupsI386(true)
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

	pe := g.BuildPE32Binary(irmod)
	if err := os.WriteFile(outputPath, pe, 0755); err != nil {
		return fmt.Errorf("write output: %v", err)
	}
	return nil
}
