//go:build no_backend_arm64

package macos

import (
	"fmt"

	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

func Generate(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	return fmt.Errorf("darwin/arm64 backend disabled (built with no_backend_arm64 tag)")
}
