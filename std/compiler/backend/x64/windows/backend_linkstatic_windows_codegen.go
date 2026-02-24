//go:build !no_backend_windows_amd64

package windows

import "strings"

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

func (g *codegen) emitGenericLinkStaticCallWin64(paramCount int, lib string, sym string) {
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
		g.subRI(REG_RSP, 8)
		alignPad = 8
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
	g.addRI(REG_RSP, int32(32+extra*8+alignPad))
}

func (g *codegen) emitLinkStaticPtrReturnWin64() {
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

func (g *codegen) emitLinkStaticReturnWin64(mode string) {
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

func (g *codegen) compileLinkStaticIntrinsicWin64(name string, arg int) bool {
	mod := g.cg.IRModule()
	if g.cg.Target().GOOS != "windows" || mod == nil || mod.LinkStaticFuncs == nil {
		return false
	}
	raw, ok := mod.LinkStaticFuncs[name]
	if !ok {
		return false
	}
	lib, sym, mode, ok := decodeLinkStaticSpecWin64(raw)
	if !ok {
		panic("ICE: invalid windows linkstatic metadata for '" + name + "'")
	}
	lib = canonicalWinImportLibrary(lib)
	if mode == "" {
		mode = "syscall"
	}
	g.emitGenericLinkStaticCallWin64(arg, lib, sym)
	g.emitLinkStaticReturnWin64(mode)
	return true
}
