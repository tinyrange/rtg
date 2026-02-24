//go:build !no_backend_i386 && !no_backend_linux_i386

package linux

import (
	"fmt"
	"os"

	"j5.nz/rtg/std/compiler/backend/i386"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// Generate compiles an IRModule to a Linux i386 ELF executable.
func Generate(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	g := i386.NewCodeGen(target, irmod, 0x08048000, 4)
	g.InitGlobals(4, len(irmod.Globals))

	EmitStart(g, irmod)
	g.CompileModuleFuncsI386(irmod)
	g.CollectNativeFuncSizes(irmod)
	if g.NeedTostringHelper() {
		g.EmitTostringHelperI386Shared()
	}

	unresolved := g.ResolveCallFixupsI386(false)
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

	elf := g.BuildELF32Binary(irmod)
	if err := os.WriteFile(outputPath, elf, 0755); err != nil {
		return fmt.Errorf("write output: %v", err)
	}
	return nil
}
