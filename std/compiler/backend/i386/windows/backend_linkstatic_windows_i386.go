//go:build !no_backend_windows_i386

package windows

import (
	"strings"

	core "j5.nz/rtg/std/compiler/backend/i386"
	"j5.nz/rtg/std/compiler/ir"
)

func decodeLinkStaticSpecWin386(raw string) (string, string, string, bool) {
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

func emitGenericLinkStaticCallWin386(g *core.CodeGen, paramCount int, lib string, sym string) {
	if paramCount < 0 {
		panic("ICE: invalid linkstatic arg count")
	}
	for i := paramCount; i > 0; i-- {
		g.EmitLoadLocal32(i*4, core.REG32_EAX)
		g.PushR32(core.REG32_EAX)
	}
	g.EmitCallIATInLib(lib, sym)
}

func emitLinkStaticPtrReturnWin386(g *core.CodeGen) {
	g.TestRR32(core.REG32_EAX, core.REG32_EAX)
	fixNonZero := g.JccRel32(core.CC32_NE)
	g.CompileConstI32(0)
	g.CompileConstI32(0)
	g.CompileConstI32(1)
	fixDone := g.JmpRel32()

	g.PatchRel32(fixNonZero)
	g.CmpRI32(core.REG32_EAX, -1)
	fixNotMinusOne := g.JccRel32(core.CC32_NE)
	g.CompileConstI32(0)
	g.CompileConstI32(0)
	g.CompileConstI32(1)
	fixDone2 := g.JmpRel32()

	g.PatchRel32(fixNotMinusOne)
	g.OpPush(core.REG32_EAX)
	g.CompileConstI32(0)
	g.CompileConstI32(0)
	g.PatchRel32(fixDone2)
	g.PatchRel32(fixDone)
}

func emitLinkStaticReturnWin386(g *core.CodeGen, mode string) {
	switch mode {
	case "syscall":
		g.OpPush(core.REG32_EAX)
		g.CompileConstI32(0)
		g.CompileConstI32(0)
	case "ptr":
		emitLinkStaticPtrReturnWin386(g)
	case "rawptr":
		g.OpPush(core.REG32_EAX)
		g.CompileConstI32(0)
		g.CompileConstI32(0)
	case "noreturn":
		g.ClearOperandCache()
	default:
		panic("ICE: unknown linkstatic mode '" + mode + "'")
	}
}

func compileLinkStaticIntrinsicWin386(g *core.CodeGen, inst ir.Inst) bool {
	if g.Target.GOOS != "windows" || g.IRMod == nil || g.IRMod.LinkStaticFuncs == nil {
		return false
	}
	raw, ok := g.IRMod.LinkStaticFuncs[inst.Name]
	if !ok {
		return false
	}
	lib, sym, mode, ok := decodeLinkStaticSpecWin386(raw)
	if !ok {
		panic("ICE: invalid windows linkstatic metadata for '" + inst.Name + "'")
	}
	lib = canonicalWinImportLibrary(lib)
	if mode == "" {
		mode = "syscall"
	}
	emitGenericLinkStaticCallWin386(g, inst.Arg, lib, sym)
	emitLinkStaticReturnWin386(g, mode)
	return true
}
