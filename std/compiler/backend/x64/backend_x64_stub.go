//go:build no_backend_linux_amd64 && no_backend_windows_amd64

package x64

import "j5.nz/rtg/std/compiler/ir"

func (g *CodeGen) CompileFunc(f *ir.IRFunc) {
	panic("amd64 backend disabled")
}

func (g *CodeGen) EmitTostringHelperX64() {
	panic("amd64 backend disabled")
}
