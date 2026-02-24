//go:build !no_backend_i386 && !no_backend_windows_i386

package windows

import (
	i386 "j5.nz/rtg/std/compiler/backend/i386"
	"j5.nz/rtg/std/compiler/ir"
)

type windowsHooks struct{}

func (h windowsHooks) EmitStart(g *i386.CodeGen, irmod *ir.IRModule) {
	wrap(g).emitStart_win386(irmod)
}

func (h windowsHooks) CompileSyscallIntrinsic(g *i386.CodeGen, paramCount int) {
	panic("ICE: Syscall intrinsic is not implemented for windows/386 hooks")
}

func (h windowsHooks) CompileSysGetdents64(g *i386.CodeGen) {
	wrap(g).compileSyscallGetdents_win386()
}

func (h windowsHooks) CompileLinkStaticIntrinsic(g *i386.CodeGen, inst ir.Inst) bool {
	return wrap(g).compileLinkStaticIntrinsicWin386(inst)
}

func (h windowsHooks) CompilePanic(g *i386.CodeGen) {
	wrap(g).compilePanic_win386()
}

func init() {
	i386.RegisterOSHooks("windows", windowsHooks{})
}
