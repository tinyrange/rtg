//go:build no_backend_windows_i386

package i386

import "j5.nz/rtg/std/compiler/ir"

func (g *CodeGen) compileLinkStaticIntrinsicWin386(inst ir.Inst) bool {
	_ = inst
	return false
}

func (g *CodeGen) compilePanic_win386() {
	panic("windows/386 backend disabled")
}
