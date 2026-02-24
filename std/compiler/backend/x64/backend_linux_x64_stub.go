//go:build no_backend_linux_amd64

package x64

import "j5.nz/rtg/std/compiler/ir"

func (g *CodeGen) emitStart(irmod *ir.IRModule) {
	panic("linux/amd64 backend disabled")
}

func (g *CodeGen) compileSyscallIntrinsic(paramCount int) {
	panic("linux/amd64 backend disabled")
}

func (g *CodeGen) compilePanic() {
	panic("linux/amd64 backend disabled")
}
