//go:build !no_backend_dos_i386 && !no_size_analysis

package dos

import (
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

func writeSizeAnalysisTarget(target common.Target) {
	ir.WriteSizeAnalysis(target)
}
