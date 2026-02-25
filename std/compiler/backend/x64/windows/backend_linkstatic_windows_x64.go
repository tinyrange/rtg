//go:build !no_backend_windows_amd64

package x64

import (
	"strings"

	core "j5.nz/rtg/std/compiler/backend/x64"
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

func emitGenericLinkStaticCallWin64(g *core.CodeGen, paramCount int, lib string, sym string) {
	if paramCount < 0 {
		panic("ICE: invalid linkstatic arg count")
	}
	extra := 0
	if paramCount > 4 {
		extra = paramCount - 4
	}
	alignPad := 0
	if extra&1 != 0 {
		// Keep RSP 16-byte aligned at external call boundaries on Win64
		// while preserving the required [rsp+32] first stack argument slot.
		g.SubRI(core.REG_RSP, 8)
		alignPad = 8
	}
	for i := paramCount; i > 4; i-- {
		g.EmitLoadLocal(i*8, core.REG_RAX)
		g.PushR(core.REG_RAX)
	}
	g.SubRI(core.REG_RSP, 32) // Windows x64 shadow space
	if paramCount >= 1 {
		g.EmitLoadLocal(1*8, core.REG_RCX)
	}
	if paramCount >= 2 {
		g.EmitLoadLocal(2*8, core.REG_RDX)
	}
	if paramCount >= 3 {
		g.EmitLoadLocal(3*8, core.REG_R8)
	}
	if paramCount >= 4 {
		g.EmitLoadLocal(4*8, core.REG_R9)
	}
	emitCallIATInLib(g, lib, sym)
	g.AddRI(core.REG_RSP, int32(32+extra*8+alignPad))
}

func emitLinkStaticPtrReturnWin64(g *core.CodeGen) {
	g.TestRR(core.REG_RAX, core.REG_RAX)
	fixNonZero := g.JccRel32(core.CC_NE)
	g.CompileConstI64(0)
	g.CompileConstI64(0)
	g.CompileConstI64(1)
	fixDone := g.JmpRel32()

	g.PatchRel32(fixNonZero)
	g.EmitMovRegImm64(core.REG_RCX, 0xFFFFFFFFFFFFFFFF)
	g.CmpRR(core.REG_RAX, core.REG_RCX)
	fixNotMinusOne := g.JccRel32(core.CC_NE)
	g.CompileConstI64(0)
	g.CompileConstI64(0)
	g.CompileConstI64(1)
	fixDone2 := g.JmpRel32()

	g.PatchRel32(fixNotMinusOne)
	g.OpPush(core.REG_RAX)
	g.CompileConstI64(0)
	g.CompileConstI64(0)
	g.PatchRel32(fixDone2)
	g.PatchRel32(fixDone)
}

func emitLinkStaticReturnWin64(g *core.CodeGen, mode string) {
	switch mode {
	case "syscall":
		g.OpPush(core.REG_RAX)
		g.CompileConstI64(0)
		g.CompileConstI64(0)
	case "ptr":
		emitLinkStaticPtrReturnWin64(g)
	case "rawptr":
		g.OpPush(core.REG_RAX)
		g.CompileConstI64(0)
		g.CompileConstI64(0)
	case "noreturn":
		g.ClearOperandCache()
	default:
		panic("ICE: unknown linkstatic mode '" + mode + "'")
	}
}

func compileLinkStaticIntrinsicWin64(g *core.CodeGen, inst ir.Inst) bool {
	if g.Target().GOOS != "windows" || g.IRModule() == nil || g.IRModule().LinkStaticFuncs == nil {
		return false
	}
	raw, ok := g.IRModule().LinkStaticFuncs[inst.Name]
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
	emitGenericLinkStaticCallWin64(g, inst.Arg, lib, sym)
	emitLinkStaticReturnWin64(g, mode)
	return true
}
