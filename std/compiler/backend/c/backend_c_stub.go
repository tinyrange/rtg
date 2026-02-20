//go:build no_backend_c

package c

import (
	"fmt"

	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

func Generate(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	return fmt.Errorf("c backend disabled (built with no_backend_c tag)")
}

func generateCSource(irmod *IRModule, outputPath string) error {
	return fmt.Errorf("c backend disabled (built with no_backend_c tag)")
}
