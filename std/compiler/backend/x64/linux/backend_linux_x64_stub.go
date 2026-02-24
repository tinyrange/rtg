//go:build no_backend_linux_amd64

package x64

import (
	"fmt"

	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

func GenerateELF(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	return fmt.Errorf("linux/amd64 backend disabled (built with no_backend_linux_amd64 tag)")
}
