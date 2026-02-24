//go:build !no_backend_i386 && !no_backend_windows_i386

package windows

import (
	"strings"

	"j5.nz/rtg/std/compiler/ir"
)

func canonicalWinImportLibrary(lib string) string {
	lib = strings.TrimSpace(lib)
	if lib == "" {
		return "kernel32.dll"
	}
	return lib
}

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

func (g *winGen) emitGenericLinkStaticCallWin386(paramCount int, lib string, sym string) {
	if paramCount < 0 {
		panic("ICE: invalid linkstatic arg count")
	}
	for i := paramCount; i > 0; i-- {
		g.emitLoadLocal32(i*4, REG32_EAX)
		g.pushR32(REG32_EAX)
	}
	g.emitCallIATInLib(lib, sym)
}

func (g *winGen) emitLinkStaticPtrReturnWin386() {
	g.testRR32(REG32_EAX, REG32_EAX)
	fixNonZero := g.jccRel32(CC32_NE)
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.compileConstI32(1)
	fixDone := g.jmpRel32()

	g.patchRel32(fixNonZero)
	g.cmpRI32(REG32_EAX, -1)
	fixNotMinusOne := g.jccRel32(CC32_NE)
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.compileConstI32(1)
	fixDone2 := g.jmpRel32()

	g.patchRel32(fixNotMinusOne)
	g.opPush(REG32_EAX)
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.patchRel32(fixDone2)
	g.patchRel32(fixDone)
}

func (g *winGen) emitLinkStaticReturnWin386(mode string) {
	switch mode {
	case "syscall":
		g.opPush(REG32_EAX)
		g.compileConstI32(0)
		g.compileConstI32(0)
	case "ptr":
		g.emitLinkStaticPtrReturnWin386()
	case "rawptr":
		g.opPush(REG32_EAX)
		g.compileConstI32(0)
		g.compileConstI32(0)
	case "noreturn":
		g.clearOperandCache()
	default:
		panic("ICE: unknown linkstatic mode '" + mode + "'")
	}
}

func (g *winGen) compileLinkStaticIntrinsicWin386(inst ir.Inst) bool {
	if g.cg.TargetGOOS() != "windows" {
		return false
	}
	raw, ok := g.cg.LookupLinkStaticSpec(inst.Name)
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
	g.emitGenericLinkStaticCallWin386(inst.Arg, lib, sym)
	g.emitLinkStaticReturnWin386(mode)
	return true
}
