//go:build !no_backend_dos_i386 && no_size_analysis

package dos

import "j5.nz/rtg/std/compiler/common"

func writeSizeAnalysisTarget(target common.Target) {
	_ = target
}
