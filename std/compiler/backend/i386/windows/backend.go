//go:build !no_backend_windows_i386

package windows

import (
	"j5.nz/rtg/std/compiler/backend/i386"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// Generate compiles an IRModule to a Windows i386 PE executable.
func Generate(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	return i386.GenerateWinPE(target, irmod, outputPath)
}
