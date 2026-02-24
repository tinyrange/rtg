//go:build no_backend_i386

package i386

import (
	"fmt"

	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

func GenerateELF(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	return fmt.Errorf("i386 backend disabled (built with no_backend_i386 tag)")
}

func (g *CodeGen) buildELF32(irmod *ir.IRModule) []byte {
	panic("i386 backend disabled")
}
