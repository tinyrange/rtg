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

func (g *CodeGen) compileKnownLinkStaticIntrinsicWin386(instName string, lib string, sym string) bool {
	prevActive := g.linkStaticImportActive
	prevLib := g.linkStaticImportLib
	prevSym := g.linkStaticImportSymbol
	g.linkStaticImportActive = true
	g.linkStaticImportLib = lib
	g.linkStaticImportSymbol = sym

	known := true
	switch instName {
	case "SysRead":
		g.compileSyscallRead_win386()
	case "SysWrite":
		g.compileSyscallWrite_win386()
	case "SysOpen":
		g.compileSyscallOpen_win386()
	case "SysClose":
		g.compileSyscallClose_win386()
	case "SysStat":
		g.compileSyscallStat_win386()
	case "SysExit":
		g.compileSyscallExit_win386()
	case "SysMmap":
		g.compileSyscallMmap_win386()
	case "SysMkdir":
		g.compileSyscallMkdir_win386()
	case "SysRmdir":
		g.compileSyscallRmdir_win386()
	case "SysUnlink":
		g.compileSyscallUnlink_win386()
	case "SysGetcwd":
		g.compileSyscallGetcwd_win386()
	case "SysGetCommandLine":
		g.compileSyscallGetCommandLine_win386()
	case "SysGetEnvStrings":
		g.compileSyscallGetEnvStrings_win386()
	case "SysFindFirstFile":
		g.compileSyscallFindFirstFile_win386()
	case "SysFindNextFile":
		g.compileSyscallFindNextFile_win386()
	case "SysFindClose":
		g.compileSyscallFindClose_win386()
	case "SysCreateProcess":
		g.compileSyscallCreateProcess_win386()
	case "SysWaitProcess":
		g.compileSyscallWaitProcess_win386()
	case "SysCreatePipe":
		g.compileSyscallCreatePipe_win386()
	case "SysSetStdHandle":
		g.compileSyscallSetStdHandle_win386()
	case "SysGetpid":
		g.compileSyscallGetpid_win386()
	default:
		known = false
	}
	g.linkStaticImportActive = prevActive
	g.linkStaticImportLib = prevLib
	g.linkStaticImportSymbol = prevSym
	return known
}

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

func (g *CodeGen) compileLinkStaticIntrinsicWin386(inst ir.Inst) bool {
	if g.target.GOOS != "windows" || g.irmod == nil || g.irmod.LinkStaticFuncs == nil {
		return false
	}
	raw, ok := g.irmod.LinkStaticFuncs[inst.Name]
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

	// Preserve existing Windows runtime syscall semantics where wrappers adapt
	// arguments/returns. All other linkstatic intrinsics use generic lowering.
	if g.compileKnownLinkStaticIntrinsicWin386(inst.Name, lib, sym) {
		return true
	}

	g.emitGenericLinkStaticCallWin386(inst.Arg, lib, sym)
	g.emitLinkStaticReturnWin386(mode)
	return true
}
