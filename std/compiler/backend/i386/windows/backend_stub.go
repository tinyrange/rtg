//go:build no_backend_i386 || no_backend_windows_i386

package windows

import (
	"fmt"

	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

func Generate(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	return fmt.Errorf("windows/386 backend disabled (built with no_backend_windows_i386 tag)")
}
