//go:build !no_backend_arm64

package aarch64

import (
	targetcfg "j5.nz/rtg/std/target"
)

const ccmetalDefaultHypercallMMIO uint64 = 0x80000000

func parseUnsignedDefine(raw string) (uint64, bool) {
	if raw == "" {
		return 0, false
	}
	base := uint64(10)
	i := 0
	if len(raw) > 2 && raw[0] == '0' {
		if raw[1] == 'x' || raw[1] == 'X' {
			base = 16
			i = 2
		} else if raw[1] == 'b' || raw[1] == 'B' {
			base = 2
			i = 2
		} else if raw[1] == 'o' || raw[1] == 'O' {
			base = 8
			i = 2
		}
	}
	var v uint64
	digits := 0
	for i < len(raw) {
		ch := raw[i]
		i = i + 1
		if ch == '_' {
			continue
		}
		d := int64(-1)
		if ch >= '0' && ch <= '9' {
			d = int64(ch - '0')
		} else if ch >= 'a' && ch <= 'f' {
			d = int64(ch-'a') + 10
		} else if ch >= 'A' && ch <= 'F' {
			d = int64(ch-'A') + 10
		}
		if d < 0 || uint64(d) >= base {
			return 0, false
		}
		v = v*base + uint64(d)
		digits = digits + 1
	}
	if digits == 0 {
		return 0, false
	}
	return v, true
}

func (g *CodeGen) ccmetalHypercallMMIO() uint64 {
	mmio := ccmetalDefaultHypercallMMIO
	if g.target == nil {
		return mmio
	}
	if abi, ok := targetcfg.LookupABI(g.target.Triple); ok {
		mmio = targetcfg.ABIUint64(abi, "hypercall_mmio", mmio)
	}
	if g.target.Defines != nil {
		if raw, ok := g.target.Defines["ccmetal.hypercall_mmio"]; ok {
			if parsed, ok := parseUnsignedDefine(raw); ok {
				mmio = parsed
			}
		}
		if raw, ok := g.target.Defines["hypercall_mmio"]; ok {
			if parsed, ok := parseUnsignedDefine(raw); ok {
				mmio = parsed
			}
		}
	}
	return mmio
}

func (g *CodeGen) emitKernelTrapArm64() {
	if g.target != nil && g.target.GOOS == "ccmetal" {
		g.EmitLoadImm64Compact(REG_X16, g.ccmetalHypercallMMIO())
		// Doorbell write triggers the VMM hypercall.
		g.EmitStr(REG_X8, REG_X16, 0)
		return
	}
	g.EmitSvc()
}

// compileSyscallIntrinsicArm64 implements the Syscall intrinsic for Linux-style
// ARM64 syscall ABIs (linux and ccmetal).
// Parameters in locals: num(0), a0(1), a1(2), a2(3), a3(4), a4(5), a5(6)
func (g *CodeGen) compileSyscallIntrinsicArm64(paramCount int) {
	g.emitLoadLocalArm64(1*8, REG_X8)
	g.emitLoadLocalArm64(2*8, REG_X0)
	g.emitLoadLocalArm64(3*8, REG_X1)
	g.emitLoadLocalArm64(4*8, REG_X2)
	g.emitLoadLocalArm64(5*8, REG_X3)
	g.emitLoadLocalArm64(6*8, REG_X4)
	g.emitLoadLocalArm64(7*8, REG_X5)

	g.emitKernelTrapArm64()
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
	g.emitKernelTrapArm64()

	g.EmitLoadImm64Compact(REG_X0, 0x0A)
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStrb(REG_X0, REG_SP, 0)
	g.EmitLoadImm64Compact(REG_X0, 2)
	g.EmitMovRRArm64(REG_X1, REG_SP)
	g.EmitLoadImm64Compact(REG_X2, 1)
	g.EmitLoadImm64Compact(REG_X8, 64)
	g.emitKernelTrapArm64()
	g.emitAddImm(REG_SP, REG_SP, 16)

	g.emitAddImm(REG_SP, REG_SP, 16)
	g.EmitLoadImm64Compact(REG_X0, 2)
	g.EmitLoadImm64Compact(REG_X8, 94)
	g.emitKernelTrapArm64()
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
