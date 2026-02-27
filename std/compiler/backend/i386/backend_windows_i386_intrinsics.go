//go:build !no_backend_windows_i386

package i386

import (
	"strings"

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

//rtg:profile
func (g *CodeGen) emitGenericLinkStaticCallWin386(paramCount int, lib string, sym string) {
	if paramCount < 0 {
		panic("ICE: invalid linkstatic arg count")
	}
	for i := paramCount; i > 0; i-- {
		g.emitLoadLocal32(i*4, REG32_EAX)
		g.pushR32(REG32_EAX)
	}
	g.emitCallIATInLib(lib, sym)
}

//rtg:profile
func (g *CodeGen) emitLinkStaticPtrReturnWin386() {
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

//rtg:profile
func (g *CodeGen) emitLinkStaticReturnWin386(mode string) {
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

//rtg:profile
func (g *CodeGen) compileLinkStaticIntrinsicWin386(inst ir.Inst) bool {
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
	g.emitGenericLinkStaticCallWin386(inst.Arg, lib, sym)
	g.emitLinkStaticReturnWin386(mode)
	return true
}

//rtg:profile
func (g *CodeGen) pushImm32(val uint32) {
	if val < 128 {
		g.emitBytes(0x6a, byte(val))
		return
	}
	g.emitByte(0x68)
	g.emitU32(val)
}

//rtg:profile
func (g *CodeGen) compilePanic_win386() {
	g.opPop(REG32_EAX)

	// Tostring heuristic: if first dword < 256, it's an interface box
	g.loadMem32(REG32_ECX, REG32_EAX, 0)
	g.cmpRI32(REG32_ECX, int32(256))
	g.emitBytes(0x73, 0x03)
	g.loadMem32(REG32_EAX, REG32_EAX, 4)

	// EAX = string header ptr {data_ptr:4, len:4}
	g.pushR32(REG32_ESI)
	g.pushR32(REG32_EBX)
	g.loadMem32(REG32_ESI, REG32_EAX, 0)
	g.loadMem32(REG32_EBX, REG32_EAX, 4)

	g.emitMovRegImm32(REG32_EAX, 0xFFFFFFF4)
	g.pushR32(REG32_EAX)
	g.emitCallIAT("GetStdHandle")
	g.movRR32(REG32_ECX, REG32_EAX)

	g.subRI32(REG32_ESP, 4)
	g.movRR32(REG32_EDX, REG32_ESP)
	g.pushImm32(0)
	g.pushR32(REG32_EDX)
	g.pushR32(REG32_EBX)
	g.pushR32(REG32_ESI)
	g.pushR32(REG32_ECX)
	g.emitCallIAT("WriteFile")
	g.addRI32(REG32_ESP, 4)

	g.emitBytes(0x6a, 0x0a)
	g.movRR32(REG32_ESI, REG32_ESP)
	g.emitMovRegImm32(REG32_EAX, 0xFFFFFFF4)
	g.pushR32(REG32_EAX)
	g.emitCallIAT("GetStdHandle")
	g.movRR32(REG32_ECX, REG32_EAX)

	g.subRI32(REG32_ESP, 4)
	g.movRR32(REG32_EDX, REG32_ESP)
	g.pushImm32(0)
	g.pushR32(REG32_EDX)
	g.pushImm32(1)
	g.pushR32(REG32_ESI)
	g.pushR32(REG32_ECX)
	g.emitCallIAT("WriteFile")
	g.addRI32(REG32_ESP, 8)

	g.popR32(REG32_EBX)
	g.popR32(REG32_ESI)

	g.pushImm32(2)
	g.emitCallIAT("ExitProcess")
}
