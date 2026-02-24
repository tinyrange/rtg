//go:build no_backend_i386 || no_backend_linux_i386

package linux

import (
	"fmt"

	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

func Generate(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	return fmt.Errorf("linux/386 backend disabled (built with no_backend_linux_i386 tag)")
}
