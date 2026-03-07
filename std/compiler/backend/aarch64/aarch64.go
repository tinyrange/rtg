//go:build !no_backend_arm64

package aarch64

import "j5.nz/rtg/std/compiler/common"

// === ARM64 Assembler: instruction encoding for AArch64 ===
// ARM64 uses fixed-width 32-bit instructions, little-endian.

// Register constants (X0-X30, SP/XZR=31)
const (
	REG_X0  = 0
	REG_X1  = 1
	REG_X2  = 2
	REG_X3  = 3
	REG_X4  = 4
	REG_X5  = 5
	REG_X6  = 6
	REG_X7  = 7
	REG_X8  = 8
	REG_X9  = 9
	REG_X10 = 10
	REG_X11 = 11
	REG_X12 = 12
	REG_X13 = 13
	REG_X14 = 14
	REG_X15 = 15
	REG_X16 = 16 // IP0 (intra-procedure scratch)
	REG_X17 = 17 // IP1
	REG_X26 = 26 // operand cache spill register (callee-saved)
	REG_X27 = 27 // operand cache spill register (callee-saved)
	REG_X28 = 28 // operand stack pointer (callee-saved)
	REG_FP  = 29 // frame pointer (X29)
	REG_LR  = 30 // link register (X30)
	REG_SP  = 31 // stack pointer (context-dependent)
	REG_XZR = 31 // zero register (context-dependent)
)

// Floating-point register constants (S0-S31 / D0-D31 share the same indices).
const (
	REG_S0  = 0
	REG_S1  = 1
	REG_S2  = 2
	REG_S3  = 3
	REG_S4  = 4
	REG_S5  = 5
	REG_S6  = 6
	REG_S7  = 7
	REG_S8  = 8
	REG_S9  = 9
	REG_S10 = 10
	REG_S11 = 11
	REG_S12 = 12
	REG_S13 = 13
	REG_S14 = 14
	REG_S15 = 15
	REG_S16 = 16
	REG_S17 = 17
	REG_S18 = 18
	REG_S19 = 19
	REG_S20 = 20
	REG_S21 = 21
	REG_S22 = 22
	REG_S23 = 23
	REG_S24 = 24
	REG_S25 = 25
	REG_S26 = 26
	REG_S27 = 27
	REG_S28 = 28
	REG_S29 = 29
	REG_S30 = 30
	REG_S31 = 31

	REG_D0  = 0
	REG_D1  = 1
	REG_D2  = 2
	REG_D3  = 3
	REG_D4  = 4
	REG_D5  = 5
	REG_D6  = 6
	REG_D7  = 7
	REG_D8  = 8
	REG_D9  = 9
	REG_D10 = 10
	REG_D11 = 11
	REG_D12 = 12
	REG_D13 = 13
	REG_D14 = 14
	REG_D15 = 15
	REG_D16 = 16
	REG_D17 = 17
	REG_D18 = 18
	REG_D19 = 19
	REG_D20 = 20
	REG_D21 = 21
	REG_D22 = 22
	REG_D23 = 23
	REG_D24 = 24
	REG_D25 = 25
	REG_D26 = 26
	REG_D27 = 27
	REG_D28 = 28
	REG_D29 = 29
	REG_D30 = 30
	REG_D31 = 31
)

// Condition codes for B.cond / CSET
const (
	COND_EQ = 0x0 // equal
	COND_NE = 0x1 // not equal
	COND_CS = 0x2 // carry set / unsigned >=
	COND_CC = 0x3 // carry clear / unsigned <
	COND_MI = 0x4 // minus / negative
	COND_PL = 0x5 // plus / positive or zero
	COND_VS = 0x6 // overflow
	COND_VC = 0x7 // no overflow
	COND_HI = 0x8 // unsigned >
	COND_LS = 0x9 // unsigned <=
	COND_GE = 0xA // signed >=
	COND_LT = 0xB // signed <
	COND_GT = 0xC // signed >
	COND_LE = 0xD // signed <=
)

// EmitArm64 appends a 32-bit ARM64 instruction (little-endian).
func (g *CodeGen) EmitArm64(inst uint32) {
	g.emitU32(inst)
}

// === Immediate loading ===

// EmitMovZ emits MOVZ Xd, #imm16, LSL #shift (shift=0,16,32,48)
func (g *CodeGen) EmitMovZ(rd int, imm16 uint16, shift int) {
	hw := uint32(shift / 16)
	inst := uint32(0xD2800000) | (hw << 21) | (uint32(imm16) << 5) | uint32(rd&0x1f)
	g.EmitArm64(inst)
}

// emitMovK emits MOVK Xd, #imm16, LSL #shift (shift=0,16,32,48)
func (g *CodeGen) emitMovK(rd int, imm16 uint16, shift int) {
	hw := uint32(shift / 16)
	inst := uint32(0xF2800000) | (hw << 21) | (uint32(imm16) << 5) | uint32(rd&0x1f)
	g.EmitArm64(inst)
}

// emitMovN emits MOVN Xd, #imm16, LSL #shift (move wide with NOT)
func (g *CodeGen) emitMovN(rd int, imm16 uint16, shift int) {
	hw := uint32(shift / 16)
	inst := uint32(0x92800000) | (hw << 21) | (uint32(imm16) << 5) | uint32(rd&0x1f)
	g.EmitArm64(inst)
}

// emitLoadImm64 loads a full 64-bit value into rd using MOVZ/MOVK sequence.
// Always emits exactly 4 instructions (16 bytes) so it can be patched.
func (g *CodeGen) emitLoadImm64(rd int, val uint64) {
	g.EmitMovZ(rd, uint16(val&0xFFFF), 0)
	g.emitMovK(rd, uint16((val>>16)&0xFFFF), 16)
	g.emitMovK(rd, uint16((val>>32)&0xFFFF), 32)
	g.emitMovK(rd, uint16((val>>48)&0xFFFF), 48)
}

// EmitLoadImm64Compact loads a 64-bit value, using fewer instructions when possible.
// NOT patchable (variable length). Use for constants that don't need fixup.
func (g *CodeGen) EmitLoadImm64Compact(rd int, val uint64) {
	if val == 0 {
		// MOVZ Xd, #0
		g.EmitMovZ(rd, 0, 0)
		return
	}

	// Check if value fits in MOVN (all ones except one 16-bit chunk)
	inv := ^val
	if inv&0xFFFF == inv {
		g.emitMovN(rd, uint16(inv), 0)
		return
	}

	// Use MOVZ for first non-zero chunk, MOVK for rest
	first := true
	for shift := 0; shift < 64; shift += 16 {
		chunk := uint16((val >> uint(shift)) & 0xFFFF)
		if chunk != 0 || shift == 0 {
			if first {
				g.EmitMovZ(rd, chunk, shift)
				first = false
			} else {
				g.emitMovK(rd, chunk, shift)
			}
		}
	}
}

// === Arithmetic ===

// EmitAddRR emits ADD Xd, Xn, Xm
func (g *CodeGen) EmitAddRR(rd, rn, rm int) {
	inst := uint32(0x8B000000) | (uint32(rm&0x1f) << 16) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.EmitArm64(inst)
}

// emitSubRR emits SUB Xd, Xn, Xm
func (g *CodeGen) emitSubRR(rd, rn, rm int) {
	inst := uint32(0xCB000000) | (uint32(rm&0x1f) << 16) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.EmitArm64(inst)
}

// emitAddImm emits ADD Xd, Xn, #imm12
func (g *CodeGen) emitAddImm(rd, rn int, imm12 uint32) {
	inst := uint32(0x91000000) | ((imm12 & 0xFFF) << 10) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.EmitArm64(inst)
}

// emitSubImm emits SUB Xd, Xn, #imm12
func (g *CodeGen) emitSubImm(rd, rn int, imm12 uint32) {
	inst := uint32(0xD1000000) | ((imm12 & 0xFFF) << 10) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.EmitArm64(inst)
}

// emitMul emits MUL Xd, Xn, Xm (alias for MADD Xd, Xn, Xm, XZR)
func (g *CodeGen) emitMul(rd, rn, rm int) {
	inst := uint32(0x9B007C00) | (uint32(rm&0x1f) << 16) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.EmitArm64(inst)
}

// emitSdiv emits SDIV Xd, Xn, Xm
func (g *CodeGen) emitSdiv(rd, rn, rm int) {
	inst := uint32(0x9AC00C00) | (uint32(rm&0x1f) << 16) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.EmitArm64(inst)
}

// emitMsub emits MSUB Xd, Xn, Xm, Xa  (Xd = Xa - Xn*Xm)
func (g *CodeGen) emitMsub(rd, rn, rm, ra int) {
	inst := uint32(0x9B008000) | (uint32(rm&0x1f) << 16) | (uint32(ra&0x1f) << 10) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.EmitArm64(inst)
}

// emitNeg emits NEG Xd, Xm (alias for SUB Xd, XZR, Xm)
func (g *CodeGen) emitNeg(rd, rm int) {
	g.emitSubRR(rd, REG_XZR, rm)
}

// === Logic ===

// emitAndRR emits AND Xd, Xn, Xm
func (g *CodeGen) emitAndRR(rd, rn, rm int) {
	inst := uint32(0x8A000000) | (uint32(rm&0x1f) << 16) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.EmitArm64(inst)
}

// emitOrrRR emits ORR Xd, Xn, Xm
func (g *CodeGen) emitOrrRR(rd, rn, rm int) {
	inst := uint32(0xAA000000) | (uint32(rm&0x1f) << 16) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.EmitArm64(inst)
}

// emitEorRR emits EOR Xd, Xn, Xm (exclusive or)
func (g *CodeGen) emitEorRR(rd, rn, rm int) {
	inst := uint32(0xCA000000) | (uint32(rm&0x1f) << 16) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.EmitArm64(inst)
}

// emitLslRR emits LSLV Xd, Xn, Xm
func (g *CodeGen) emitLslRR(rd, rn, rm int) {
	inst := uint32(0x9AC02000) | (uint32(rm&0x1f) << 16) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.EmitArm64(inst)
}

// emitAsrRR emits ASRV Xd, Xn, Xm (arithmetic shift right)
func (g *CodeGen) emitAsrRR(rd, rn, rm int) {
	inst := uint32(0x9AC02800) | (uint32(rm&0x1f) << 16) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.EmitArm64(inst)
}

// emitLslImm emits LSL Xd, Xn, #shift (alias for UBFM)
func (g *CodeGen) emitLslImm(rd, rn int, shift uint32) {
	// LSL Xd, Xn, #shift is UBFM Xd, Xn, #(64-shift), #(63-shift)
	immr := (64 - shift) & 0x3F
	imms := (63 - shift) & 0x3F
	inst := uint32(0xD3400000) | (immr << 16) | (imms << 10) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.EmitArm64(inst)
}

// === Compare ===

// emitCmpRR emits CMP Xn, Xm (alias for SUBS XZR, Xn, Xm)
func (g *CodeGen) emitCmpRR(rn, rm int) {
	inst := uint32(0xEB000000) | (uint32(rm&0x1f) << 16) | (uint32(rn&0x1f) << 5) | uint32(REG_XZR&0x1f)
	g.EmitArm64(inst)
}

// emitCmpImm emits CMP Xn, #imm12 (alias for SUBS XZR, Xn, #imm12)
func (g *CodeGen) emitCmpImm(rn int, imm12 uint32) {
	inst := uint32(0xF1000000) | ((imm12 & 0xFFF) << 10) | (uint32(rn&0x1f) << 5) | uint32(REG_XZR&0x1f)
	g.EmitArm64(inst)
}

// emitTstRR emits TST Xn, Xm (alias for ANDS XZR, Xn, Xm)
func (g *CodeGen) emitTstRR(rn, rm int) {
	inst := uint32(0xEA000000) | (uint32(rm&0x1f) << 16) | (uint32(rn&0x1f) << 5) | uint32(REG_XZR&0x1f)
	g.EmitArm64(inst)
}

// emitCset emits CSET Xd, cond (alias for CSINC Xd, XZR, XZR, invert(cond))
func (g *CodeGen) emitCset(rd int, cond int) {
	// CSET is CSINC Rd, XZR, XZR, invert(cond)
	// invert = cond ^ 1
	inv := uint32(cond ^ 1)
	inst := uint32(0x9A9F07E0) | (inv << 12) | uint32(rd&0x1f)
	g.EmitArm64(inst)
}

// === Memory: LDR/STR ===

// emitLdr emits LDR Xt, [Xn, #offset] (unsigned offset, scaled by 8)
func (g *CodeGen) emitLdr(rt, rn int, offset int) {
	if offset == 0 {
		// LDR Xt, [Xn]
		inst := uint32(0xF9400000) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	} else if offset > 0 && offset%8 == 0 && offset/8 < 4096 {
		// LDR Xt, [Xn, #uimm] (scaled unsigned offset)
		uimm := uint32(offset / 8)
		inst := uint32(0xF9400000) | (uimm << 10) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	} else if offset >= -256 && offset <= 255 {
		// LDUR Xt, [Xn, #simm9]
		simm9 := uint32(offset) & 0x1FF
		inst := uint32(0xF8400000) | (simm9 << 12) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	} else {
		// Offset too large — use scratch register X16
		g.EmitLoadImm64Compact(REG_X16, uint64(int64(offset)))
		g.EmitAddRR(REG_X16, rn, REG_X16)
		inst := uint32(0xF9400000) | (uint32(REG_X16&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	}
}

// EmitStr emits STR Xt, [Xn, #offset] (unsigned offset, scaled by 8)
func (g *CodeGen) EmitStr(rt, rn int, offset int) {
	if offset == 0 {
		inst := uint32(0xF9000000) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	} else if offset > 0 && offset%8 == 0 && offset/8 < 4096 {
		uimm := uint32(offset / 8)
		inst := uint32(0xF9000000) | (uimm << 10) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	} else if offset >= -256 && offset <= 255 {
		simm9 := uint32(offset) & 0x1FF
		inst := uint32(0xF8000000) | (simm9 << 12) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	} else {
		g.EmitLoadImm64Compact(REG_X16, uint64(int64(offset)))
		g.EmitAddRR(REG_X16, rn, REG_X16)
		inst := uint32(0xF9000000) | (uint32(REG_X16&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	}
}

// emitLdrb emits LDRB Wt, [Xn, #offset] (zero-extend byte)
func (g *CodeGen) emitLdrb(rt, rn int, offset int) {
	if offset >= 0 && offset < 4096 {
		inst := uint32(0x39400000) | (uint32(offset&0xFFF) << 10) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	} else if offset >= -256 && offset <= 255 {
		simm9 := uint32(offset) & 0x1FF
		inst := uint32(0x38400000) | (simm9 << 12) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	} else {
		g.EmitLoadImm64Compact(REG_X16, uint64(int64(offset)))
		g.EmitAddRR(REG_X16, rn, REG_X16)
		inst := uint32(0x39400000) | (uint32(REG_X16&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	}
}

// emitLdrh emits LDRH Wt, [Xn, #offset] (zero-extend halfword)
func (g *CodeGen) emitLdrh(rt, rn int, offset int) {
	if offset >= 0 && offset%2 == 0 && offset/2 < 4096 {
		inst := uint32(0x79400000) | (uint32((offset/2)&0xFFF) << 10) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	} else if offset >= -256 && offset <= 255 {
		simm9 := uint32(offset) & 0x1FF
		inst := uint32(0x78400000) | (simm9 << 12) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	} else {
		g.EmitLoadImm64Compact(REG_X16, uint64(int64(offset)))
		g.EmitAddRR(REG_X16, rn, REG_X16)
		inst := uint32(0x79400000) | (uint32(REG_X16&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	}
}

// emitLdrw emits LDR Wt, [Xn, #offset] (zero-extend word)
func (g *CodeGen) emitLdrw(rt, rn int, offset int) {
	if offset >= 0 && offset%4 == 0 && offset/4 < 4096 {
		inst := uint32(0xB9400000) | (uint32((offset/4)&0xFFF) << 10) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	} else if offset >= -256 && offset <= 255 {
		simm9 := uint32(offset) & 0x1FF
		inst := uint32(0xB8400000) | (simm9 << 12) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	} else {
		g.EmitLoadImm64Compact(REG_X16, uint64(int64(offset)))
		g.EmitAddRR(REG_X16, rn, REG_X16)
		inst := uint32(0xB9400000) | (uint32(REG_X16&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	}
}

// emitStrb emits STRB Wt, [Xn, #offset]
func (g *CodeGen) emitStrb(rt, rn int, offset int) {
	if offset >= 0 && offset < 4096 {
		inst := uint32(0x39000000) | (uint32(offset&0xFFF) << 10) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	} else if offset >= -256 && offset <= 255 {
		simm9 := uint32(offset) & 0x1FF
		inst := uint32(0x38000000) | (simm9 << 12) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	} else {
		g.EmitLoadImm64Compact(REG_X16, uint64(int64(offset)))
		g.EmitAddRR(REG_X16, rn, REG_X16)
		inst := uint32(0x39000000) | (uint32(REG_X16&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	}
}

// emitStrh emits STRH Wt, [Xn, #offset]
func (g *CodeGen) emitStrh(rt, rn int, offset int) {
	if offset >= 0 && offset%2 == 0 && offset/2 < 4096 {
		inst := uint32(0x79000000) | (uint32((offset/2)&0xFFF) << 10) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	} else if offset >= -256 && offset <= 255 {
		simm9 := uint32(offset) & 0x1FF
		inst := uint32(0x78000000) | (simm9 << 12) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	} else {
		g.EmitLoadImm64Compact(REG_X16, uint64(int64(offset)))
		g.EmitAddRR(REG_X16, rn, REG_X16)
		inst := uint32(0x79000000) | (uint32(REG_X16&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	}
}

// emitStrw emits STR Wt, [Xn, #offset]
func (g *CodeGen) emitStrw(rt, rn int, offset int) {
	if offset >= 0 && offset%4 == 0 && offset/4 < 4096 {
		inst := uint32(0xB9000000) | (uint32((offset/4)&0xFFF) << 10) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	} else if offset >= -256 && offset <= 255 {
		simm9 := uint32(offset) & 0x1FF
		inst := uint32(0xB8000000) | (simm9 << 12) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	} else {
		g.EmitLoadImm64Compact(REG_X16, uint64(int64(offset)))
		g.EmitAddRR(REG_X16, rn, REG_X16)
		inst := uint32(0xB9000000) | (uint32(REG_X16&0x1f) << 5) | uint32(rt&0x1f)
		g.EmitArm64(inst)
	}
}

// EmitStp emits STP Xt1, Xt2, [Xn, #offset]! (pre-index)
func (g *CodeGen) EmitStp(rt1, rt2, rn int, offset int) {
	// STP (pre-index): [Xn, #imm7*8]!
	imm7 := uint32(offset/8) & 0x7F
	inst := uint32(0xA9800000) | (imm7 << 15) | (uint32(rt2&0x1f) << 10) | (uint32(rn&0x1f) << 5) | uint32(rt1&0x1f)
	g.EmitArm64(inst)
}

// EmitLdp emits LDP Xt1, Xt2, [Xn], #offset (post-index)
func (g *CodeGen) EmitLdp(rt1, rt2, rn int, offset int) {
	// LDP (post-index): [Xn], #imm7*8
	imm7 := uint32(offset/8) & 0x7F
	inst := uint32(0xA8C00000) | (imm7 << 15) | (uint32(rt2&0x1f) << 10) | (uint32(rn&0x1f) << 5) | uint32(rt1&0x1f)
	g.EmitArm64(inst)
}

// === Branch ===

// emitB emits B (unconditional branch, imm26) with placeholder.
// Returns the code offset of the instruction for later fixup.
func (g *CodeGen) emitB() int {
	off := len(g.code)
	g.EmitArm64(0x14000000) // B #0 (placeholder)
	return off
}

// emitBL emits BL (branch with link) with placeholder.
// Returns the code offset of the instruction for later fixup.
func (g *CodeGen) emitBL() int {
	off := len(g.code)
	g.EmitArm64(0x94000000) // BL #0 (placeholder)
	return off
}

// emitBCond emits B.cond with placeholder.
// Returns the code offset of the instruction for later fixup.
func (g *CodeGen) emitBCond(cond int) int {
	off := len(g.code)
	inst := uint32(0x54000000) | uint32(cond&0xF)
	g.EmitArm64(inst) // B.cond #0 (placeholder)
	return off
}

// EmitBlr emits BLR Xn (branch to register with link)
func (g *CodeGen) EmitBlr(rn int) {
	inst := uint32(0xD63F0000) | (uint32(rn&0x1f) << 5)
	g.EmitArm64(inst)
}

// EmitRet emits RET (return via LR, X30)
func (g *CodeGen) EmitRet() {
	g.EmitArm64(0xD65F03C0) // RET
}

// emitBrk emits BRK #0 (breakpoint)
func (g *CodeGen) emitBrk() {
	g.EmitArm64(0xD4200000)
}

// emitNop emits NOP
func (g *CodeGen) emitNop() {
	g.EmitArm64(0xD503201F)
}

// EmitSvc emits SVC #0 (supervisor call for Linux syscalls)
func (g *CodeGen) EmitSvc() {
	g.EmitArm64(0xD4000001)
}

// === Move ===

// EmitMovRRArm64 emits MOV Xd, Xm.
// For SP-involving moves, uses ADD Xd, Xn, #0 (SP is only valid in ADD/SUB, not ORR).
// For all other registers, uses ORR Xd, XZR, Xm.
func (g *CodeGen) EmitMovRRArm64(rd, rm int) {
	if rd == REG_SP || rm == REG_SP {
		// ADD Xd, Xn, #0 — handles SP correctly
		g.emitAddImm(rd, rm, 0)
		return
	}
	g.emitOrrRR(rd, REG_XZR, rm)
}

// === Extensions ===

// emitUxtb emits UXTB Xd, Xn (zero-extend byte, alias for UBFM Xd, Xn, #0, #7)
func (g *CodeGen) emitUxtb(rd, rn int) {
	// Use 32-bit form: UXTB Wd, Wn = AND Wd, Wn, #0xFF = UBFM Wd, Wn, #0, #7
	inst := uint32(0x53001C00) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.EmitArm64(inst)
}

// emitSxtb emits SXTB Xd, Xn (sign-extend byte)
func (g *CodeGen) emitSxtb(rd, rn int) {
	inst := uint32(0x93401C00) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.EmitArm64(inst)
}

// emitUxth emits UXTH Xd, Xn (zero-extend halfword)
func (g *CodeGen) emitUxth(rd, rn int) {
	// UXTH Wd, Wn = UBFM Wd, Wn, #0, #15
	inst := uint32(0x53003C00) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.EmitArm64(inst)
}

// emitSxth emits SXTH Xd, Xn (sign-extend halfword)
func (g *CodeGen) emitSxth(rd, rn int) {
	// SXTH Xd, Wn = SBFM Xd, Xn, #0, #15
	inst := uint32(0x93403C00) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.EmitArm64(inst)
}

// emitSxtw emits SXTW Xd, Wn (sign-extend 32→64)
func (g *CodeGen) emitSxtw(rd, rn int) {
	// SXTW Xd, Wn = SBFM Xd, Xn, #0, #31
	inst := uint32(0x93407C00) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.EmitArm64(inst)
}

// emitUxtw emits UXTW Xd, Wn (zero-extend 32→64, alias for UBFM Xd, Xn, #0, #31)
// Actually, MOV Wd, Wn (writing Wd zeros the top 32 bits)
func (g *CodeGen) emitUxtw(rd, rn int) {
	// Use 32-bit ORR: MOV Wd, Wn = ORR Wd, WZR, Wn (zero-extends)
	inst := uint32(0x2A0003E0) | (uint32(rn&0x1f) << 16) | uint32(rd&0x1f)
	g.EmitArm64(inst)
}

// === Floating-point register moves ===

func (g *CodeGen) emitFmovDFromX(fd, xn int) {
	inst := uint32(0x9E670000) | (uint32(xn&0x1f) << 5) | uint32(fd&0x1f)
	g.EmitArm64(inst)
}

func (g *CodeGen) emitFmovXFromD(xd, fn int) {
	inst := uint32(0x9E660000) | (uint32(fn&0x1f) << 5) | uint32(xd&0x1f)
	g.EmitArm64(inst)
}

func (g *CodeGen) emitFmovSFromW(fs, wn int) {
	inst := uint32(0x1E270000) | (uint32(wn&0x1f) << 5) | uint32(fs&0x1f)
	g.EmitArm64(inst)
}

func (g *CodeGen) emitFmovWFromS(wd, fn int) {
	inst := uint32(0x1E260000) | (uint32(fn&0x1f) << 5) | uint32(wd&0x1f)
	g.EmitArm64(inst)
}

// === Floating-point arithmetic ===

func (g *CodeGen) emitFaddD(fd, fn, fm int) {
	inst := uint32(0x1E602800) | (uint32(fm&0x1f) << 16) | (uint32(fn&0x1f) << 5) | uint32(fd&0x1f)
	g.EmitArm64(inst)
}

func (g *CodeGen) emitFaddS(fd, fn, fm int) {
	inst := uint32(0x1E202800) | (uint32(fm&0x1f) << 16) | (uint32(fn&0x1f) << 5) | uint32(fd&0x1f)
	g.EmitArm64(inst)
}

func (g *CodeGen) emitFsubD(fd, fn, fm int) {
	inst := uint32(0x1E603800) | (uint32(fm&0x1f) << 16) | (uint32(fn&0x1f) << 5) | uint32(fd&0x1f)
	g.EmitArm64(inst)
}

func (g *CodeGen) emitFsubS(fd, fn, fm int) {
	inst := uint32(0x1E203800) | (uint32(fm&0x1f) << 16) | (uint32(fn&0x1f) << 5) | uint32(fd&0x1f)
	g.EmitArm64(inst)
}

func (g *CodeGen) emitFmulD(fd, fn, fm int) {
	inst := uint32(0x1E600800) | (uint32(fm&0x1f) << 16) | (uint32(fn&0x1f) << 5) | uint32(fd&0x1f)
	g.EmitArm64(inst)
}

func (g *CodeGen) emitFmulS(fd, fn, fm int) {
	inst := uint32(0x1E200800) | (uint32(fm&0x1f) << 16) | (uint32(fn&0x1f) << 5) | uint32(fd&0x1f)
	g.EmitArm64(inst)
}

func (g *CodeGen) emitFdivD(fd, fn, fm int) {
	inst := uint32(0x1E601800) | (uint32(fm&0x1f) << 16) | (uint32(fn&0x1f) << 5) | uint32(fd&0x1f)
	g.EmitArm64(inst)
}

func (g *CodeGen) emitFdivS(fd, fn, fm int) {
	inst := uint32(0x1E201800) | (uint32(fm&0x1f) << 16) | (uint32(fn&0x1f) << 5) | uint32(fd&0x1f)
	g.EmitArm64(inst)
}

func (g *CodeGen) emitFnegD(fd, fn int) {
	inst := uint32(0x1E614000) | (uint32(fn&0x1f) << 5) | uint32(fd&0x1f)
	g.EmitArm64(inst)
}

func (g *CodeGen) emitFnegS(fd, fn int) {
	inst := uint32(0x1E214000) | (uint32(fn&0x1f) << 5) | uint32(fd&0x1f)
	g.EmitArm64(inst)
}

func (g *CodeGen) emitFcmpD(fn, fm int) {
	inst := uint32(0x1E602000) | (uint32(fm&0x1f) << 16) | (uint32(fn&0x1f) << 5)
	g.EmitArm64(inst)
}

func (g *CodeGen) emitFcmpS(fn, fm int) {
	inst := uint32(0x1E202000) | (uint32(fm&0x1f) << 16) | (uint32(fn&0x1f) << 5)
	g.EmitArm64(inst)
}

// === Floating-point conversions ===

func (g *CodeGen) emitScvtfDFromX(fd, xn int) {
	inst := uint32(0x9E620000) | (uint32(xn&0x1f) << 5) | uint32(fd&0x1f)
	g.EmitArm64(inst)
}

func (g *CodeGen) emitUcvtfDFromX(fd, xn int) {
	inst := uint32(0x9E630000) | (uint32(xn&0x1f) << 5) | uint32(fd&0x1f)
	g.EmitArm64(inst)
}

func (g *CodeGen) emitScvtfSFromW(fd, wn int) {
	inst := uint32(0x1E220000) | (uint32(wn&0x1f) << 5) | uint32(fd&0x1f)
	g.EmitArm64(inst)
}

func (g *CodeGen) emitUcvtfSFromW(fd, wn int) {
	inst := uint32(0x1E230000) | (uint32(wn&0x1f) << 5) | uint32(fd&0x1f)
	g.EmitArm64(inst)
}

func (g *CodeGen) emitFcvtzsXFromD(xd, fn int) {
	inst := uint32(0x9E780000) | (uint32(fn&0x1f) << 5) | uint32(xd&0x1f)
	g.EmitArm64(inst)
}

func (g *CodeGen) emitFcvtzuXFromD(xd, fn int) {
	inst := uint32(0x9E790000) | (uint32(fn&0x1f) << 5) | uint32(xd&0x1f)
	g.EmitArm64(inst)
}

func (g *CodeGen) emitFcvtzsWFromS(wd, fn int) {
	inst := uint32(0x1E380000) | (uint32(fn&0x1f) << 5) | uint32(wd&0x1f)
	g.EmitArm64(inst)
}

func (g *CodeGen) emitFcvtzuWFromS(wd, fn int) {
	inst := uint32(0x1E390000) | (uint32(fn&0x1f) << 5) | uint32(wd&0x1f)
	g.EmitArm64(inst)
}

func (g *CodeGen) emitFcvtSToD(fd, fn int) {
	inst := uint32(0x1E22C000) | (uint32(fn&0x1f) << 5) | uint32(fd&0x1f)
	g.EmitArm64(inst)
}

func (g *CodeGen) emitFcvtDToS(fd, fn int) {
	inst := uint32(0x1E624000) | (uint32(fn&0x1f) << 5) | uint32(fd&0x1f)
	g.EmitArm64(inst)
}

// emitEorImm emits EOR Xd, Xn, #1 (for boolean NOT: XOR with 1)
func (g *CodeGen) emitEorImm1(rd, rn int) {
	// EOR Xd, Xn, #1 — bitmask immediate encoding for 1:
	// N=1, immr=0, imms=0 → encodes the value 1 for 64-bit
	inst := uint32(0xD2400000) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.EmitArm64(inst)
}

// === Frame access (FP-relative) ===

// emitLoadLocalArm64 emits LDR Xt, [FP, #-offset]
func (g *CodeGen) emitLoadLocalArm64(offset int, rd int) {
	g.emitLdr(rd, REG_FP, -offset)
}

// emitStoreLocalArm64 emits STR Xt, [FP, #-offset]
func (g *CodeGen) emitStoreLocalArm64(offset int, rd int) {
	g.EmitStr(rd, REG_FP, -offset)
}

// emitLeaLocalArm64 emits SUB Xd, FP, #offset (compute address of local)
func (g *CodeGen) emitLeaLocalArm64(offset int, rd int) {
	if offset > 0 && offset < 4096 {
		g.emitSubImm(rd, REG_FP, uint32(offset))
	} else {
		g.EmitLoadImm64Compact(rd, uint64(int64(offset)))
		g.emitSubRR(rd, REG_FP, rd)
	}
}

// === PC-relative addressing (ADRP + ADD/LDR) ===

// EmitAdrp emits ADRP Xd, #0 (placeholder). Returns the code offset for later fixup.
func (g *CodeGen) EmitAdrp(rd int) int {
	off := len(g.code)
	// ADRP: 1 immlo(2) 10000 immhi(19) Rd(5) — base = 0x90000000
	inst := uint32(0x90000000) | uint32(rd&0x1f)
	g.EmitArm64(inst)
	return off
}

// EmitAdrpAdd emits ADRP+ADD pair for loading an address (PC-relative).
// Records a fixup with the given target and raw section-relative offset.
func (g *CodeGen) EmitAdrpAdd(rd int, target string, rawOff uint64) {
	off := g.EmitAdrp(rd)
	g.emitAddImm(rd, rd, 0) // placeholder pageoff
	g.callFixups = append(g.callFixups, CallFixup{off, target, rawOff})
}

// emitAdrpLdr emits ADRP+LDR pair for loading a 64-bit value from a PC-relative address.
// The LDR uses unsigned scaled offset (divided by 8). Records a fixup.
func (g *CodeGen) emitAdrpLdr(rd int, target string, rawOff uint64) {
	off := g.EmitAdrp(rd)
	// LDR Xt, [Xn, #0] — unsigned offset scaled by 8, placeholder
	inst := uint32(0xF9400000) | (uint32(rd&0x1f) << 5) | uint32(rd&0x1f)
	g.EmitArm64(inst)
	g.callFixups = append(g.callFixups, CallFixup{off, target, rawOff})
}

// === Fixup helpers ===

// PatchArm64BAt patches a B or BL instruction at codeOffset to branch to target.
func (g *CodeGen) PatchArm64BAt(codeOffset int, target int) {
	delta := (target - codeOffset) / 4 // offset in instructions
	existing := common.GetU32(g.code[codeOffset : codeOffset+4])
	opcode := existing & 0xFC000000 // preserve opcode bits
	imm26 := uint32(delta) & 0x03FFFFFF
	common.PutU32(g.code[codeOffset:], opcode|imm26)
}

// patchArm64BCondAt patches a B.cond instruction at codeOffset.
func (g *CodeGen) patchArm64BCondAt(codeOffset int, target int) {
	delta := (target - codeOffset) / 4
	existing := common.GetU32(g.code[codeOffset : codeOffset+4])
	cond := existing & 0xF // preserve condition
	imm19 := (uint32(delta) & 0x7FFFF) << 5
	common.PutU32(g.code[codeOffset:], 0x54000000|imm19|cond)
}

// patchArm64Imm64At patches a MOVZ/MOVK 4-instruction sequence at codeOffset
// with the given 64-bit value.
func (g *CodeGen) patchArm64Imm64At(codeOffset int, val uint64) {
	chunks := make([]uint16, 4)
	chunks[0] = uint16(val & 0xFFFF)
	chunks[1] = uint16((val >> 16) & 0xFFFF)
	chunks[2] = uint16((val >> 32) & 0xFFFF)
	chunks[3] = uint16((val >> 48) & 0xFFFF)
	for i := 0; i < 4; i++ {
		off := codeOffset + i*4
		existing := common.GetU32(g.code[off : off+4])
		// Clear the imm16 field (bits 20:5) and re-encode
		cleared := existing & 0xFFE0001F
		common.PutU32(g.code[off:], cleared|(uint32(chunks[i])<<5))
	}
}

// PatchAdrpAddOrLdr dispatches to PatchAdrpAdd or PatchAdrpLdr based on the second instruction.
func (g *CodeGen) PatchAdrpAddOrLdr(codeOffset int, pcAddr, targetAddr uint64) {
	secondInst := common.GetU32(g.code[codeOffset+4:])
	if secondInst&0xFFC00000 == 0xF9400000 {
		// LDR (unsigned offset, 64-bit): top bits = 1111 1001 01xx xxxx
		g.PatchAdrpLdr(codeOffset, pcAddr, targetAddr)
	} else {
		// ADD (immediate): top bits = 1001 0001 00xx xxxx
		g.PatchAdrpAdd(codeOffset, pcAddr, targetAddr)
	}
}

// PatchAdrpAdd patches an ADRP+ADD pair at codeOffset to address targetAddr,
// given the PC (virtual address of the ADRP instruction).
func (g *CodeGen) PatchAdrpAdd(codeOffset int, pcAddr, targetAddr uint64) {
	pageDelta := int64(targetAddr>>12) - int64(pcAddr>>12)
	pageOff := targetAddr & 0xFFF

	// Patch ADRP: immhi = bits 23:5, immlo = bits 30:29
	immlo := uint32(pageDelta) & 0x3
	immhi := (uint32(pageDelta) >> 2) & 0x7FFFF
	adrp := common.GetU32(g.code[codeOffset:])
	adrp = (adrp & 0x9F00001F) | (immlo << 29) | (immhi << 5)
	common.PutU32(g.code[codeOffset:], adrp)

	// Patch ADD: imm12 = bits 21:10
	addOff := codeOffset + 4
	add := common.GetU32(g.code[addOff:])
	add = (add & 0xFFC003FF) | (uint32(pageOff) << 10)
	common.PutU32(g.code[addOff:], add)
}

// PatchAdrpLdr patches an ADRP+LDR pair at codeOffset to load from targetAddr,
// given the PC (virtual address of the ADRP instruction).
// The LDR uses unsigned offset scaled by 8 (for 64-bit loads).
func (g *CodeGen) PatchAdrpLdr(codeOffset int, pcAddr, targetAddr uint64) {
	pageDelta := int64(targetAddr>>12) - int64(pcAddr>>12)
	pageOff := targetAddr & 0xFFF

	// Patch ADRP
	immlo := uint32(pageDelta) & 0x3
	immhi := (uint32(pageDelta) >> 2) & 0x7FFFF
	adrp := common.GetU32(g.code[codeOffset:])
	adrp = (adrp & 0x9F00001F) | (immlo << 29) | (immhi << 5)
	common.PutU32(g.code[codeOffset:], adrp)

	// Patch LDR: imm12 = pageOff/8, in bits 21:10
	ldrOff := codeOffset + 4
	ldr := common.GetU32(g.code[ldrOff:])
	scaledOff := uint32(pageOff / 8)
	ldr = (ldr & 0xFFC003FF) | (scaledOff << 10)
	common.PutU32(g.code[ldrOff:], ldr)
}
