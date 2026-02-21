package windows

import (
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// Generate compiles an IRModule to a Windows ARM64 PE32+ executable.
func Generate(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	return GenerateWinPE(target, irmod, outputPath)
}
