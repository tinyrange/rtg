//go:build no_backend_ir

package irprint

import (
	"fmt"

	"j5.nz/rtg/std/compiler/ir"
)

func Generate(irmod *ir.IRModule, outputPath string) error {
	return fmt.Errorf("ir backend disabled (built with no_backend_ir tag)")
}
