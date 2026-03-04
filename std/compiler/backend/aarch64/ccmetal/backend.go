//go:build !no_backend_arm64

package ccmetal

import (
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// Generate compiles an IRModule to a flat ARM64 ccmetal image.
func Generate(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	return GenerateFlatBinary(target, irmod, outputPath)
}
