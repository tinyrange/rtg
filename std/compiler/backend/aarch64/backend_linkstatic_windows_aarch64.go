//go:build !no_backend_arm64

package aarch64

import "j5.nz/rtg/std/compiler/ir"

//rtg:profile
func (g *CodeGen) emitGenericLinkStaticCallWinArm64(paramCount int, lib string, sym string) {
	if paramCount < 0 {
		panic("ICE: invalid linkstatic arg count")
	}
	extra := 0
	if paramCount > 8 {
		extra = paramCount - 8
	}
	g.loadLinkStaticArgsArm64(paramCount)

	// Keep operand stack register stable across external call.
	frame := 16 + extra*8 // saved X28 + stack args beyond X7
	if frame%16 != 0 {
		frame = frame + 8
	}
	g.emitSubImm(REG_SP, REG_SP, uint32(frame))
	g.EmitStr(REG_X28, REG_SP, 0)

	stackOff := 16
	i := 9
	for i <= paramCount {
		g.emitLoadLocalArm64(i*8, REG_X16)
		g.EmitStr(REG_X16, REG_SP, stackOff)
		stackOff = stackOff + 8
		i = i + 1
	}

	g.emitCallIATArm64InLib(lib, sym)
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, uint32(frame))
}

//rtg:profile
func (g *CodeGen) emitLinkStaticPtrReturnWinArm64() {
	g.Flush()
	g.emitTstRR(REG_X0, REG_X0)
	fixNonZero := g.emitBCond(COND_NE)

	// NULL => error
	g.EmitMovZ(REG_X1, 0, 0)
	g.rawPush(REG_X1)
	g.rawPush(REG_X1)
	g.EmitLoadImm64Compact(REG_X1, 1)
	g.rawPush(REG_X1)
	fixDone := g.emitB()

	g.patchArm64BCondAt(fixNonZero, len(g.code))
	g.EmitLoadImm64Compact(REG_X1, 0xFFFFFFFFFFFFFFFF)
	g.emitCmpRR(REG_X0, REG_X1)
	fixNotMinusOne := g.emitBCond(COND_NE)

	// -1 => error
	g.EmitMovZ(REG_X1, 0, 0)
	g.rawPush(REG_X1)
	g.rawPush(REG_X1)
	g.EmitLoadImm64Compact(REG_X1, 1)
	g.rawPush(REG_X1)
	fixDone2 := g.emitB()

	g.patchArm64BCondAt(fixNotMinusOne, len(g.code))
	// success
	g.rawPush(REG_X0)
	g.EmitMovZ(REG_X1, 0, 0)
	g.rawPush(REG_X1)
	g.rawPush(REG_X1)

	g.PatchArm64BAt(fixDone2, len(g.code))
	g.PatchArm64BAt(fixDone, len(g.code))
	g.ClearOperandCache()
}

//rtg:profile
func (g *CodeGen) emitLinkStaticReturnWinArm64(mode string) {
	switch mode {
	case "syscall":
		g.rawPush(REG_X0)
		g.EmitMovZ(REG_X1, 0, 0)
		g.rawPush(REG_X1)
		g.rawPush(REG_X1)
		g.ClearOperandCache()
	case "ptr":
		g.emitLinkStaticPtrReturnWinArm64()
	case "rawptr":
		g.rawPush(REG_X0)
		g.EmitMovZ(REG_X1, 0, 0)
		g.rawPush(REG_X1)
		g.rawPush(REG_X1)
		g.ClearOperandCache()
	case "noreturn":
		g.ClearOperandCache()
	default:
		panic("ICE: unknown linkstatic mode '" + mode + "'")
	}
}

//rtg:profile
func (g *CodeGen) compileLinkStaticIntrinsicArm64Windows(inst ir.Inst) bool {
	if g.target.GOOS != "windows" || g.irmod == nil || g.irmod.LinkStaticFuncs == nil {
		return false
	}
	raw, ok := g.irmod.LinkStaticFuncs[inst.Name]
	if !ok {
		return false
	}
	lib, sym, mode, ok := decodeLinkStaticSpec(raw)
	if !ok {
		panic("ICE: invalid windows linkstatic metadata for '" + inst.Name + "'")
	}
	lib = canonicalWinImportLibraryArm64(lib)
	if mode == "" {
		mode = "syscall"
	}

	g.emitGenericLinkStaticCallWinArm64(inst.Arg, lib, sym)
	g.emitLinkStaticReturnWinArm64(mode)
	return true
}
