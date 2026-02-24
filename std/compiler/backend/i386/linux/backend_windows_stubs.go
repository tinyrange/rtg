//go:build !no_backend_i386 && !no_backend_linux_i386

package linux

import "j5.nz/rtg/std/compiler/ir"

func (g *CodeGen) compileLinkStaticIntrinsicWin386(inst ir.Inst) bool {
	return false
}

func (g *CodeGen) compileSyscallGetdents_win386() {
	panic("ICE: windows/386 intrinsic in linux/386 backend")
}

func (g *CodeGen) compilePanic_win386() {
	panic("ICE: windows/386 panic path in linux/386 backend")
}
