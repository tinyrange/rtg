//go:build no_backend_dos_i386

package x8086

import (
	"fmt"

	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

func GenerateDOSCOM(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	return fmt.Errorf("dos/8086 backend disabled (built with no_backend_dos_i386 tag)")
}
