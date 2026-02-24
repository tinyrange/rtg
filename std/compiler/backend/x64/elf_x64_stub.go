//go:build no_backend_linux_amd64 && no_backend_arm64

package x64

import "j5.nz/rtg/std/compiler/ir"

func (g *CodeGen) buildELF64(irmod *ir.IRModule) []byte {
	panic("linux/amd64 backend disabled")
}

func (g *CodeGen) BuildELF64(irmod *ir.IRModule) []byte {
	panic("linux/amd64 backend disabled")
}
