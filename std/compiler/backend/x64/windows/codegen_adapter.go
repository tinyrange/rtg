//go:build !no_backend_windows_amd64

package windows

import (
	"strings"

	"j5.nz/rtg/std/compiler/backend/x64"
	"j5.nz/rtg/std/compiler/ir"
)

type codegen struct {
	cg *x64.CodeGen
}

const (
	REG_RAX = x64.REG_RAX
	REG_RCX = x64.REG_RCX
	REG_RDX = x64.REG_RDX
	REG_RSI = x64.REG_RSI
	REG_RSP = x64.REG_RSP
	REG_R8  = x64.REG_R8
	REG_R9  = x64.REG_R9
	REG_R12 = x64.REG_R12
	REG_R15 = x64.REG_R15
	REG_RBX = x64.REG_RBX

	CC_E  = x64.CC_E
	CC_NE = x64.CC_NE
	CC_GE = x64.CC_GE
)

func wrap(g *x64.CodeGen) *codegen {
	return &codegen{cg: g}
}

func init() {
	x64.RegisterPlatformHooks("windows", platformHooks{})
}

type platformHooks struct{}

func (h platformHooks) CompileIntrinsic(g *x64.CodeGen, name string, arg int) bool {
	_ = h
	return wrap(g).compileCallIntrinsicWin64(name, arg)
}

func (h platformHooks) Panic(g *x64.CodeGen) {
	_ = h
	wrap(g).compilePanicWin64()
}

func (h platformHooks) AlignFrameBytes(frameBytes int) int {
	_ = h
	return alignUpWin64(frameBytes, 16)
}

func (h platformHooks) ShouldSkipCallFixup(target string, skipMask int) bool {
	_ = h
	if (skipMask & x64.FixupSkipIAT) == 0 {
		return false
	}
	_, _, ok := x64.DecodeIATFixupTarget(target)
	return ok
}

func (g *codegen) emitByte(b byte)                   { g.cg.EmitByte(b) }
func (g *codegen) emitBytes(bytes ...byte)           { g.cg.EmitBytes(bytes...) }
func (g *codegen) emitU32(v uint32)                  { g.cg.EmitU32(v) }
func (g *codegen) emitMovRegImm64(reg int, v uint64) { g.cg.EmitMovRegImm64(reg, v) }
func (g *codegen) emitLoadLocal(offset int, reg int) { g.cg.EmitLoadLocal(offset, reg) }
func (g *codegen) emitCallPlaceholder(target string) { g.cg.EmitCallPlaceholder(target) }
func (g *codegen) emitCallIAT(funcName string)       { g.cg.EmitCallIAT(funcName) }
func (g *codegen) emitCallIATInLib(libName string, funcName string) {
	g.cg.EmitCallIATInLib(libName, funcName)
}

func (g *codegen) flush()                            { g.cg.Flush() }
func (g *codegen) jccRel32(cc byte) int              { return g.cg.JccRel32(cc) }
func (g *codegen) jmpRel32() int                     { return g.cg.JmpRel32() }
func (g *codegen) patchRel32(fixupOff int)           { g.cg.PatchRel32(fixupOff) }
func (g *codegen) patchRel32At(fixupOff, target int) { g.cg.PatchRel32At(fixupOff, target) }

func (g *codegen) addRI(reg int, imm int32) { g.cg.AddRI(reg, imm) }
func (g *codegen) subRI(reg int, imm int32) { g.cg.SubRI(reg, imm) }
func (g *codegen) addRR(dst, src int)       { g.cg.AddRR(dst, src) }
func (g *codegen) cmpRI(reg int, imm int32) { g.cg.CmpRI(reg, imm) }
func (g *codegen) cmpRR(a, b int)           { g.cg.CmpRR(a, b) }
func (g *codegen) testRR(a, b int)          { g.cg.TestRR(a, b) }
func (g *codegen) xorRR(a, b int)           { g.cg.XorRR(a, b) }
func (g *codegen) movRR(dst, src int)       { g.cg.MovRR(dst, src) }
func (g *codegen) negR(reg int)             { g.cg.NegR(reg) }

func (g *codegen) pushR(reg int) { g.cg.PushR(reg) }
func (g *codegen) popR(reg int)  { g.cg.PopR(reg) }

func (g *codegen) loadMem(dst, base, offset int) { g.cg.LoadMem(dst, base, offset) }
func (g *codegen) loadMemByte(dst, base, offset int) {
	g.cg.LoadMemByte(dst, base, offset)
}
func (g *codegen) storeMem(base, offset, src int) { g.cg.StoreMem(base, offset, src) }
func (g *codegen) storeMemByte(base, offset, src int) {
	g.cg.StoreMemByte(base, offset, src)
}

func (g *codegen) opPop(reg int)  { g.cg.OpPop(reg) }
func (g *codegen) opPush(reg int) { g.cg.OpPush(reg) }

func (g *codegen) compileConstI64(v int64) { g.cg.CompileConstI64(v) }
func (g *codegen) clearOperandCache()      { g.cg.ClearOperandCache() }
func (g *codegen) buildPE64(irmod *ir.IRModule) []byte {
	return g.cg.BuildPE64(irmod)
}
func (g *codegen) CodeLen() int { return g.cg.CodeLen() }

const winDefaultImportLibrary = "kernel32.dll"

func canonicalWinImportLibrary(lib string) string {
	lib = strings.TrimSpace(lib)
	if lib == "" {
		return winDefaultImportLibrary
	}
	return lib
}

func alignUpWin64(v int, n int) int {
	if n <= 1 {
		return v
	}
	r := v % n
	if r == 0 {
		return v
	}
	return v + (n - r)
}
