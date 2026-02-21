//go:build !no_backend_arm64

package linux

import (
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// Generate compiles an IRModule to a Linux ARM64 ELF binary.
func Generate(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	return GenerateLinuxELF(target, irmod, outputPath)
}
