//go:build !no_backend_windows_amd64

package x64

import (
	"strings"

	"j5.nz/rtg/std/compiler/ir"
)

func decodeLinkStaticSpecWin64(raw string) (string, string, string, bool) {
	parts := strings.Split(raw, ",")
	if len(parts) != 3 {
		return "", "", "", false
	}
	lib := strings.TrimSpace(parts[0])
	sym := strings.TrimSpace(parts[1])
	mode := strings.TrimSpace(parts[2])
	if lib == "" || sym == "" {
		return "", "", "", false
	}
	return lib, sym, mode, true
}

func (g *CodeGen) emitGenericLinkStaticCallWin64(paramCount int, lib string, sym string) {
	if paramCount < 0 {
		panic("ICE: invalid linkstatic arg count")
	}
	extra := 0
	if paramCount > 4 {
		extra = paramCount - 4
	}
	for i := paramCount; i > 4; i-- {
		g.emitLoadLocal(i*8, REG_RAX)
		g.pushR(REG_RAX)
	}
	g.subRI(REG_RSP, 32) // Windows x64 shadow space
	if paramCount >= 1 {
		g.emitLoadLocal(1*8, REG_RCX)
	}
	if paramCount >= 2 {
		g.emitLoadLocal(2*8, REG_RDX)
	}
	if paramCount >= 3 {
		g.emitLoadLocal(3*8, REG_R8)
	}
	if paramCount >= 4 {
		g.emitLoadLocal(4*8, REG_R9)
	}
	g.emitCallIATInLib(lib, sym)
	g.addRI(REG_RSP, int32(32+extra*8))
}

func (g *CodeGen) emitLinkStaticPtrReturnWin64() {
	g.testRR(REG_RAX, REG_RAX)
	fixNonZero := g.jccRel32(CC_NE)
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.compileConstI64(1)
	fixDone := g.jmpRel32()

	g.patchRel32(fixNonZero)
	g.emitMovRegImm64(REG_RCX, 0xFFFFFFFFFFFFFFFF)
	g.cmpRR(REG_RAX, REG_RCX)
	fixNotMinusOne := g.jccRel32(CC_NE)
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.compileConstI64(1)
	fixDone2 := g.jmpRel32()

	g.patchRel32(fixNotMinusOne)
	g.opPush(REG_RAX)
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.patchRel32(fixDone2)
	g.patchRel32(fixDone)
}

func (g *CodeGen) emitLinkStaticReturnWin64(mode string) {
	switch mode {
	case "syscall":
		g.opPush(REG_RAX)
		g.compileConstI64(0)
		g.compileConstI64(0)
	case "ptr":
		g.emitLinkStaticPtrReturnWin64()
	case "rawptr":
		g.opPush(REG_RAX)
		g.compileConstI64(0)
		g.compileConstI64(0)
	case "noreturn":
		g.clearOperandCache()
	default:
		panic("ICE: unknown linkstatic mode '" + mode + "'")
	}
}

func (g *CodeGen) compileLinkStaticIntrinsicWin64(inst ir.Inst) bool {
	if g.target.GOOS != "windows" || g.irmod == nil || g.irmod.LinkStaticFuncs == nil {
		return false
	}
	raw, ok := g.irmod.LinkStaticFuncs[inst.Name]
	if !ok {
		return false
	}
	lib, sym, mode, ok := decodeLinkStaticSpecWin64(raw)
	if !ok {
		panic("ICE: invalid windows linkstatic metadata for '" + inst.Name + "'")
	}
	lib = canonicalWinImportLibrary(lib)
	if mode == "" {
		mode = "syscall"
	}
	g.emitGenericLinkStaticCallWin64(inst.Arg, lib, sym)
	g.emitLinkStaticReturnWin64(mode)
	return true
}
