//go:build no_backend_arm64

package aarch64

import "j5.nz/rtg/std/compiler/ir"

func (g *CodeGen) CompileFuncArm64(f *ir.IRFunc) {
	panic("arm64 backend disabled")
}

func (g *CodeGen) PatchArm64BAt(branchInstOffset int, targetAddr uint64) {
	panic("arm64 backend disabled")
}
