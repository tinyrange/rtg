//go:build !no_backend_arm64

package aarch64

// compileSyscallIntrinsicArm64 implements the Syscall intrinsic for Linux ARM64.
// Parameters in locals: num(0), a0(1), a1(2), a2(3), a3(4), a4(5), a5(6)
func (g *CodeGen) compileSyscallIntrinsicArm64(paramCount int) {
	g.emitLoadLocalArm64(1*8, REG_X8)
	g.emitLoadLocalArm64(2*8, REG_X0)
	g.emitLoadLocalArm64(3*8, REG_X1)
	g.emitLoadLocalArm64(4*8, REG_X2)
	g.emitLoadLocalArm64(5*8, REG_X3)
	g.emitLoadLocalArm64(6*8, REG_X4)
	g.emitLoadLocalArm64(7*8, REG_X5)

	g.EmitSvc()
	g.Flush()

	g.EmitMovRRArm64(REG_X2, REG_X0)
	g.emitCmpImm(REG_X2, 0)
	errFixup := g.emitBCond(COND_LT)

	g.rawPush(REG_X2)
	g.EmitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	doneFixup := g.emitB()

	g.patchArm64BCondAt(errFixup, len(g.code))
	g.EmitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.emitNeg(REG_X0, REG_X2)
	g.rawPush(REG_X0)

	g.PatchArm64BAt(doneFixup, len(g.code))
	g.ClearOperandCache()
}

// compilePanicArm64Linux handles panic on Linux ARM64 using direct syscalls.
func (g *CodeGen) compilePanicArm64Linux() {
	g.opPop(REG_X0)
	g.emitLdr(REG_X1, REG_X0, 0)
	g.emitCmpImm(REG_X1, 256)
	stringFixup := g.emitBCond(COND_CS)
	g.emitLdr(REG_X0, REG_X0, 8)
	g.patchArm64BCondAt(stringFixup, len(g.code))

	g.emitSubImm(REG_SP, REG_SP, 16)
	g.EmitStr(REG_X0, REG_SP, 0)
	g.emitLdr(REG_X1, REG_X0, 0)
	g.emitLdr(REG_X2, REG_X0, 8)

	g.EmitLoadImm64Compact(REG_X0, 2)
	g.EmitLoadImm64Compact(REG_X8, 64)
	g.EmitSvc()

	g.EmitLoadImm64Compact(REG_X0, 0x0A)
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStrb(REG_X0, REG_SP, 0)
	g.EmitLoadImm64Compact(REG_X0, 2)
	g.EmitMovRRArm64(REG_X1, REG_SP)
	g.EmitLoadImm64Compact(REG_X2, 1)
	g.EmitLoadImm64Compact(REG_X8, 64)
	g.EmitSvc()
	g.emitAddImm(REG_SP, REG_SP, 16)

	g.emitAddImm(REG_SP, REG_SP, 16)
	g.EmitLoadImm64Compact(REG_X0, 2)
	g.EmitLoadImm64Compact(REG_X8, 94)
	g.EmitSvc()
}

// emitSyscallReturnArm64 handles the standard libSystem return convention.
func (g *CodeGen) emitSyscallReturnArm64() {
	g.Flush()
	g.EmitMovRRArm64(REG_X2, REG_X0)
	g.emitCmpImm(REG_X2, 0)
	errFixup := g.emitBCond(COND_LT)

	g.rawPush(REG_X2)
	g.EmitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	doneFixup := g.emitB()

	g.patchArm64BCondAt(errFixup, len(g.code))
	g.EmitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.emitNeg(REG_X0, REG_X2)
	g.rawPush(REG_X0)

	g.PatchArm64BAt(doneFixup, len(g.code))
	g.ClearOperandCache()
}

// emitSyscallReturnPtrArm64 handles pointer-returning calls (NULL or MAP_FAILED = error).
func (g *CodeGen) emitSyscallReturnPtrArm64() {
	g.Flush()
	g.EmitMovRRArm64(REG_X2, REG_X0)
	g.emitCmpImm(REG_X2, 0)
	errFixup := g.emitBCond(COND_LE)

	g.rawPush(REG_X2)
	g.EmitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	doneFixup := g.emitB()

	g.patchArm64BCondAt(errFixup, len(g.code))
	g.EmitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.EmitLoadImm64Compact(REG_X0, 1)
	g.rawPush(REG_X0)

	g.PatchArm64BAt(doneFixup, len(g.code))
	g.ClearOperandCache()
}

// compilePanicArm64 handles panic on macOS ARM64.
func (g *CodeGen) compilePanicArm64() {
	g.opPop(REG_X0)
	g.emitLdr(REG_X1, REG_X0, 0)
	g.emitCmpImm(REG_X1, 256)
	stringFixup := g.emitBCond(COND_CS)
	g.emitLdr(REG_X0, REG_X0, 8)
	g.patchArm64BCondAt(stringFixup, len(g.code))

	g.emitSubImm(REG_SP, REG_SP, 16)
	g.EmitStr(REG_X0, REG_SP, 0)
	g.emitLdr(REG_X1, REG_X0, 0)
	g.emitLdr(REG_X2, REG_X0, 8)

	g.EmitLoadImm64Compact(REG_X0, 2)
	g.EmitCallGOT("_write")

	g.EmitLoadImm64Compact(REG_X0, 0x0A)
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStrb(REG_X0, REG_SP, 0)
	g.EmitLoadImm64Compact(REG_X0, 2)
	g.EmitMovRRArm64(REG_X1, REG_SP)
	g.EmitLoadImm64Compact(REG_X2, 1)
	g.EmitCallGOT("_write")
	g.emitAddImm(REG_SP, REG_SP, 16)

	g.emitAddImm(REG_SP, REG_SP, 16)
	g.EmitLoadImm64Compact(REG_X0, 2)
	g.EmitCallGOT("_exit")
}
