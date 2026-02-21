//go:build !no_backend_dos_i386

package x8086

import (
	"j5.nz/rtg/std/compiler/backend/8086/dos"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

func GenerateDOSCOM(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	return dos.GenerateDOSCOM(target, irmod, outputPath)
}
