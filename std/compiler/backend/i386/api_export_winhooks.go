//go:build !no_backend_i386 && !no_backend_windows_i386

package i386

import "j5.nz/rtg/std/compiler/ir"

func (g *CodeGen) BuildPE32Binary(irmod *ir.IRModule) []byte {
	return g.buildPE32(irmod)
}

func (g *CodeGen) EmitCallIAT(funcName string) { g.emitCallIAT(funcName) }

func (g *CodeGen) EmitCallIATInLib(libName, funcName string) {
	g.emitCallIATInLib(libName, funcName)
}
