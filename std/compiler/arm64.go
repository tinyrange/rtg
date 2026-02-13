//go:build !no_backend_darwin_arm64

package main

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
	REG_X28 = 28 // operand stack pointer (callee-saved)
	REG_FP  = 29 // frame pointer (X29)
	REG_LR  = 30 // link register (X30)
	REG_SP  = 31 // stack pointer (context-dependent)
	REG_XZR = 31 // zero register (context-dependent)
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

// emitArm64 appends a 32-bit ARM64 instruction (little-endian).
func (g *CodeGen) emitArm64(inst uint32) {
	g.code = append(g.code, byte(inst), byte(inst>>8), byte(inst>>16), byte(inst>>24))
}

// === Immediate loading ===

// emitMovZ emits MOVZ Xd, #imm16, LSL #shift (shift=0,16,32,48)
func (g *CodeGen) emitMovZ(rd int, imm16 uint16, shift int) {
	hw := uint32(shift / 16)
	inst := uint32(0xD2800000) | (hw << 21) | (uint32(imm16) << 5) | uint32(rd&0x1f)
	g.emitArm64(inst)
}

// emitMovK emits MOVK Xd, #imm16, LSL #shift (shift=0,16,32,48)
func (g *CodeGen) emitMovK(rd int, imm16 uint16, shift int) {
	hw := uint32(shift / 16)
	inst := uint32(0xF2800000) | (hw << 21) | (uint32(imm16) << 5) | uint32(rd&0x1f)
	g.emitArm64(inst)
}

// emitMovN emits MOVN Xd, #imm16, LSL #shift (move wide with NOT)
func (g *CodeGen) emitMovN(rd int, imm16 uint16, shift int) {
	hw := uint32(shift / 16)
	inst := uint32(0x92800000) | (hw << 21) | (uint32(imm16) << 5) | uint32(rd&0x1f)
	g.emitArm64(inst)
}

// emitLoadImm64 loads a full 64-bit value into rd using MOVZ/MOVK sequence.
// Always emits exactly 4 instructions (16 bytes) so it can be patched.
func (g *CodeGen) emitLoadImm64(rd int, val uint64) {
	g.emitMovZ(rd, uint16(val&0xFFFF), 0)
	g.emitMovK(rd, uint16((val>>16)&0xFFFF), 16)
	g.emitMovK(rd, uint16((val>>32)&0xFFFF), 32)
	g.emitMovK(rd, uint16((val>>48)&0xFFFF), 48)
}

// emitLoadImm64Compact loads a 64-bit value, using fewer instructions when possible.
// NOT patchable (variable length). Use for constants that don't need fixup.
func (g *CodeGen) emitLoadImm64Compact(rd int, val uint64) {
	if val == 0 {
		// MOVZ Xd, #0
		g.emitMovZ(rd, 0, 0)
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
				g.emitMovZ(rd, chunk, shift)
				first = false
			} else {
				g.emitMovK(rd, chunk, shift)
			}
		}
	}
}

// === Arithmetic ===

// emitAddRR emits ADD Xd, Xn, Xm
func (g *CodeGen) emitAddRR(rd, rn, rm int) {
	inst := uint32(0x8B000000) | (uint32(rm&0x1f) << 16) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.emitArm64(inst)
}

// emitSubRR emits SUB Xd, Xn, Xm
func (g *CodeGen) emitSubRR(rd, rn, rm int) {
	inst := uint32(0xCB000000) | (uint32(rm&0x1f) << 16) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.emitArm64(inst)
}

// emitAddImm emits ADD Xd, Xn, #imm12
func (g *CodeGen) emitAddImm(rd, rn int, imm12 uint32) {
	inst := uint32(0x91000000) | ((imm12 & 0xFFF) << 10) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.emitArm64(inst)
}

// emitSubImm emits SUB Xd, Xn, #imm12
func (g *CodeGen) emitSubImm(rd, rn int, imm12 uint32) {
	inst := uint32(0xD1000000) | ((imm12 & 0xFFF) << 10) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.emitArm64(inst)
}

// emitMul emits MUL Xd, Xn, Xm (alias for MADD Xd, Xn, Xm, XZR)
func (g *CodeGen) emitMul(rd, rn, rm int) {
	inst := uint32(0x9B007C00) | (uint32(rm&0x1f) << 16) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.emitArm64(inst)
}

// emitSdiv emits SDIV Xd, Xn, Xm
func (g *CodeGen) emitSdiv(rd, rn, rm int) {
	inst := uint32(0x9AC00C00) | (uint32(rm&0x1f) << 16) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.emitArm64(inst)
}

// emitMsub emits MSUB Xd, Xn, Xm, Xa  (Xd = Xa - Xn*Xm)
func (g *CodeGen) emitMsub(rd, rn, rm, ra int) {
	inst := uint32(0x9B008000) | (uint32(rm&0x1f) << 16) | (uint32(ra&0x1f) << 10) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.emitArm64(inst)
}

// emitNeg emits NEG Xd, Xm (alias for SUB Xd, XZR, Xm)
func (g *CodeGen) emitNeg(rd, rm int) {
	g.emitSubRR(rd, REG_XZR, rm)
}

// === Logic ===

// emitAndRR emits AND Xd, Xn, Xm
func (g *CodeGen) emitAndRR(rd, rn, rm int) {
	inst := uint32(0x8A000000) | (uint32(rm&0x1f) << 16) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.emitArm64(inst)
}

// emitOrrRR emits ORR Xd, Xn, Xm
func (g *CodeGen) emitOrrRR(rd, rn, rm int) {
	inst := uint32(0xAA000000) | (uint32(rm&0x1f) << 16) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.emitArm64(inst)
}

// emitEorRR emits EOR Xd, Xn, Xm (exclusive or)
func (g *CodeGen) emitEorRR(rd, rn, rm int) {
	inst := uint32(0xCA000000) | (uint32(rm&0x1f) << 16) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.emitArm64(inst)
}

// emitLslRR emits LSLV Xd, Xn, Xm
func (g *CodeGen) emitLslRR(rd, rn, rm int) {
	inst := uint32(0x9AC02000) | (uint32(rm&0x1f) << 16) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.emitArm64(inst)
}

// emitAsrRR emits ASRV Xd, Xn, Xm (arithmetic shift right)
func (g *CodeGen) emitAsrRR(rd, rn, rm int) {
	inst := uint32(0x9AC02800) | (uint32(rm&0x1f) << 16) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.emitArm64(inst)
}

// emitLslImm emits LSL Xd, Xn, #shift (alias for UBFM)
func (g *CodeGen) emitLslImm(rd, rn int, shift uint32) {
	// LSL Xd, Xn, #shift is UBFM Xd, Xn, #(64-shift), #(63-shift)
	immr := (64 - shift) & 0x3F
	imms := (63 - shift) & 0x3F
	inst := uint32(0xD3400000) | (immr << 16) | (imms << 10) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.emitArm64(inst)
}

// === Compare ===

// emitCmpRR emits CMP Xn, Xm (alias for SUBS XZR, Xn, Xm)
func (g *CodeGen) emitCmpRR(rn, rm int) {
	inst := uint32(0xEB000000) | (uint32(rm&0x1f) << 16) | (uint32(rn&0x1f) << 5) | uint32(REG_XZR&0x1f)
	g.emitArm64(inst)
}

// emitCmpImm emits CMP Xn, #imm12 (alias for SUBS XZR, Xn, #imm12)
func (g *CodeGen) emitCmpImm(rn int, imm12 uint32) {
	inst := uint32(0xF1000000) | ((imm12 & 0xFFF) << 10) | (uint32(rn&0x1f) << 5) | uint32(REG_XZR&0x1f)
	g.emitArm64(inst)
}

// emitTstRR emits TST Xn, Xm (alias for ANDS XZR, Xn, Xm)
func (g *CodeGen) emitTstRR(rn, rm int) {
	inst := uint32(0xEA000000) | (uint32(rm&0x1f) << 16) | (uint32(rn&0x1f) << 5) | uint32(REG_XZR&0x1f)
	g.emitArm64(inst)
}

// emitCset emits CSET Xd, cond (alias for CSINC Xd, XZR, XZR, invert(cond))
func (g *CodeGen) emitCset(rd int, cond int) {
	// CSET is CSINC Rd, XZR, XZR, invert(cond)
	// invert = cond ^ 1
	inv := uint32(cond ^ 1)
	inst := uint32(0x9A9F07E0) | (inv << 12) | uint32(rd&0x1f)
	g.emitArm64(inst)
}

// === Memory: LDR/STR ===

// emitLdr emits LDR Xt, [Xn, #offset] (unsigned offset, scaled by 8)
func (g *CodeGen) emitLdr(rt, rn int, offset int) {
	if offset == 0 {
		// LDR Xt, [Xn]
		inst := uint32(0xF9400000) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.emitArm64(inst)
	} else if offset > 0 && offset%8 == 0 && offset/8 < 4096 {
		// LDR Xt, [Xn, #uimm] (scaled unsigned offset)
		uimm := uint32(offset / 8)
		inst := uint32(0xF9400000) | (uimm << 10) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.emitArm64(inst)
	} else if offset >= -256 && offset <= 255 {
		// LDUR Xt, [Xn, #simm9]
		simm9 := uint32(offset) & 0x1FF
		inst := uint32(0xF8400000) | (simm9 << 12) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.emitArm64(inst)
	} else {
		// Offset too large — use scratch register X16
		g.emitLoadImm64Compact(REG_X16, uint64(int64(offset)))
		g.emitAddRR(REG_X16, rn, REG_X16)
		inst := uint32(0xF9400000) | (uint32(REG_X16&0x1f) << 5) | uint32(rt&0x1f)
		g.emitArm64(inst)
	}
}

// emitStr emits STR Xt, [Xn, #offset] (unsigned offset, scaled by 8)
func (g *CodeGen) emitStr(rt, rn int, offset int) {
	if offset == 0 {
		inst := uint32(0xF9000000) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.emitArm64(inst)
	} else if offset > 0 && offset%8 == 0 && offset/8 < 4096 {
		uimm := uint32(offset / 8)
		inst := uint32(0xF9000000) | (uimm << 10) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.emitArm64(inst)
	} else if offset >= -256 && offset <= 255 {
		simm9 := uint32(offset) & 0x1FF
		inst := uint32(0xF8000000) | (simm9 << 12) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.emitArm64(inst)
	} else {
		g.emitLoadImm64Compact(REG_X16, uint64(int64(offset)))
		g.emitAddRR(REG_X16, rn, REG_X16)
		inst := uint32(0xF9000000) | (uint32(REG_X16&0x1f) << 5) | uint32(rt&0x1f)
		g.emitArm64(inst)
	}
}

// emitLdrb emits LDRB Wt, [Xn, #offset] (zero-extend byte)
func (g *CodeGen) emitLdrb(rt, rn int, offset int) {
	if offset >= 0 && offset < 4096 {
		inst := uint32(0x39400000) | (uint32(offset&0xFFF) << 10) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.emitArm64(inst)
	} else if offset >= -256 && offset <= 255 {
		simm9 := uint32(offset) & 0x1FF
		inst := uint32(0x38400000) | (simm9 << 12) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.emitArm64(inst)
	} else {
		g.emitLoadImm64Compact(REG_X16, uint64(int64(offset)))
		g.emitAddRR(REG_X16, rn, REG_X16)
		inst := uint32(0x39400000) | (uint32(REG_X16&0x1f) << 5) | uint32(rt&0x1f)
		g.emitArm64(inst)
	}
}

// emitStrb emits STRB Wt, [Xn, #offset]
func (g *CodeGen) emitStrb(rt, rn int, offset int) {
	if offset >= 0 && offset < 4096 {
		inst := uint32(0x39000000) | (uint32(offset&0xFFF) << 10) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.emitArm64(inst)
	} else if offset >= -256 && offset <= 255 {
		simm9 := uint32(offset) & 0x1FF
		inst := uint32(0x38000000) | (simm9 << 12) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f)
		g.emitArm64(inst)
	} else {
		g.emitLoadImm64Compact(REG_X16, uint64(int64(offset)))
		g.emitAddRR(REG_X16, rn, REG_X16)
		inst := uint32(0x39000000) | (uint32(REG_X16&0x1f) << 5) | uint32(rt&0x1f)
		g.emitArm64(inst)
	}
}

// emitStp emits STP Xt1, Xt2, [Xn, #offset]! (pre-index)
func (g *CodeGen) emitStp(rt1, rt2, rn int, offset int) {
	// STP (pre-index): [Xn, #imm7*8]!
	imm7 := uint32(offset/8) & 0x7F
	inst := uint32(0xA9800000) | (imm7 << 15) | (uint32(rt2&0x1f) << 10) | (uint32(rn&0x1f) << 5) | uint32(rt1&0x1f)
	g.emitArm64(inst)
}

// emitLdp emits LDP Xt1, Xt2, [Xn], #offset (post-index)
func (g *CodeGen) emitLdp(rt1, rt2, rn int, offset int) {
	// LDP (post-index): [Xn], #imm7*8
	imm7 := uint32(offset/8) & 0x7F
	inst := uint32(0xA8C00000) | (imm7 << 15) | (uint32(rt2&0x1f) << 10) | (uint32(rn&0x1f) << 5) | uint32(rt1&0x1f)
	g.emitArm64(inst)
}

// === Branch ===

// emitB emits B (unconditional branch, imm26) with placeholder.
// Returns the code offset of the instruction for later fixup.
func (g *CodeGen) emitB() int {
	off := len(g.code)
	g.emitArm64(0x14000000) // B #0 (placeholder)
	return off
}

// emitBL emits BL (branch with link) with placeholder.
// Returns the code offset of the instruction for later fixup.
func (g *CodeGen) emitBL() int {
	off := len(g.code)
	g.emitArm64(0x94000000) // BL #0 (placeholder)
	return off
}

// emitBCond emits B.cond with placeholder.
// Returns the code offset of the instruction for later fixup.
func (g *CodeGen) emitBCond(cond int) int {
	off := len(g.code)
	inst := uint32(0x54000000) | uint32(cond&0xF)
	g.emitArm64(inst) // B.cond #0 (placeholder)
	return off
}

// emitBlr emits BLR Xn (branch to register with link)
func (g *CodeGen) emitBlr(rn int) {
	inst := uint32(0xD63F0000) | (uint32(rn&0x1f) << 5)
	g.emitArm64(inst)
}

// emitRet emits RET (return via LR, X30)
func (g *CodeGen) emitRet() {
	g.emitArm64(0xD65F03C0) // RET
}

// emitBrk emits BRK #0 (breakpoint)
func (g *CodeGen) emitBrk() {
	g.emitArm64(0xD4200000)
}

// emitNop emits NOP
func (g *CodeGen) emitNop() {
	g.emitArm64(0xD503201F)
}

// === Move ===

// emitMovRRArm64 emits MOV Xd, Xm.
// For SP-involving moves, uses ADD Xd, Xn, #0 (SP is only valid in ADD/SUB, not ORR).
// For all other registers, uses ORR Xd, XZR, Xm.
func (g *CodeGen) emitMovRRArm64(rd, rm int) {
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
	g.emitArm64(inst)
}

// emitUxth emits UXTH Xd, Xn (zero-extend halfword)
func (g *CodeGen) emitUxth(rd, rn int) {
	// UXTH Wd, Wn = UBFM Wd, Wn, #0, #15
	inst := uint32(0x53003C00) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.emitArm64(inst)
}

// emitSxtw emits SXTW Xd, Wn (sign-extend 32→64)
func (g *CodeGen) emitSxtw(rd, rn int) {
	// SXTW Xd, Wn = SBFM Xd, Xn, #0, #31
	inst := uint32(0x93407C00) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.emitArm64(inst)
}

// emitUxtw emits UXTW Xd, Wn (zero-extend 32→64, alias for UBFM Xd, Xn, #0, #31)
// Actually, MOV Wd, Wn (writing Wd zeros the top 32 bits)
func (g *CodeGen) emitUxtw(rd, rn int) {
	// Use 32-bit ORR: MOV Wd, Wn = ORR Wd, WZR, Wn (zero-extends)
	inst := uint32(0x2A0003E0) | (uint32(rn&0x1f) << 16) | uint32(rd&0x1f)
	g.emitArm64(inst)
}

// emitEorImm emits EOR Xd, Xn, #1 (for boolean NOT: XOR with 1)
func (g *CodeGen) emitEorImm1(rd, rn int) {
	// EOR Xd, Xn, #1 — bitmask immediate encoding for 1:
	// N=1, immr=0, imms=0 → encodes the value 1 for 64-bit
	inst := uint32(0xD2400000) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f)
	g.emitArm64(inst)
}

// === Frame access (FP-relative) ===

// emitLoadLocalArm64 emits LDR Xt, [FP, #-offset]
func (g *CodeGen) emitLoadLocalArm64(offset int, rd int) {
	g.emitLdr(rd, REG_FP, -offset)
}

// emitStoreLocalArm64 emits STR Xt, [FP, #-offset]
func (g *CodeGen) emitStoreLocalArm64(offset int, rd int) {
	g.emitStr(rd, REG_FP, -offset)
}

// emitLeaLocalArm64 emits SUB Xd, FP, #offset (compute address of local)
func (g *CodeGen) emitLeaLocalArm64(offset int, rd int) {
	if offset > 0 && offset < 4096 {
		g.emitSubImm(rd, REG_FP, uint32(offset))
	} else {
		g.emitLoadImm64Compact(rd, uint64(int64(offset)))
		g.emitSubRR(rd, REG_FP, rd)
	}
}

// === PC-relative addressing (ADRP + ADD/LDR) ===

// emitAdrp emits ADRP Xd, #0 (placeholder). Returns the code offset for later fixup.
func (g *CodeGen) emitAdrp(rd int) int {
	off := len(g.code)
	// ADRP: 1 immlo(2) 10000 immhi(19) Rd(5) — base = 0x90000000
	inst := uint32(0x90000000) | uint32(rd&0x1f)
	g.emitArm64(inst)
	return off
}

// emitAdrpAdd emits ADRP+ADD pair for loading an address (PC-relative).
// Records a fixup with the given target and raw section-relative offset.
func (g *CodeGen) emitAdrpAdd(rd int, target string, rawOff uint64) {
	off := g.emitAdrp(rd)
	g.emitAddImm(rd, rd, 0) // placeholder pageoff
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: off,
		Target:     target,
		Value:      rawOff,
	})
}

// emitAdrpLdr emits ADRP+LDR pair for loading a 64-bit value from a PC-relative address.
// The LDR uses unsigned scaled offset (divided by 8). Records a fixup.
func (g *CodeGen) emitAdrpLdr(rd int, target string, rawOff uint64) {
	off := g.emitAdrp(rd)
	// LDR Xt, [Xn, #0] — unsigned offset scaled by 8, placeholder
	inst := uint32(0xF9400000) | (uint32(rd&0x1f) << 5) | uint32(rd&0x1f)
	g.emitArm64(inst)
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: off,
		Target:     target,
		Value:      rawOff,
	})
}

// === Fixup helpers ===

// patchArm64BAt patches a B or BL instruction at codeOffset to branch to target.
func (g *CodeGen) patchArm64BAt(codeOffset int, target int) {
	delta := (target - codeOffset) / 4 // offset in instructions
	existing := getU32(g.code[codeOffset : codeOffset+4])
	opcode := existing & 0xFC000000 // preserve opcode bits
	imm26 := uint32(delta) & 0x03FFFFFF
	putU32(g.code[codeOffset:], opcode|imm26)
}

// patchArm64BCondAt patches a B.cond instruction at codeOffset.
func (g *CodeGen) patchArm64BCondAt(codeOffset int, target int) {
	delta := (target - codeOffset) / 4
	existing := getU32(g.code[codeOffset : codeOffset+4])
	cond := existing & 0xF // preserve condition
	imm19 := (uint32(delta) & 0x7FFFF) << 5
	putU32(g.code[codeOffset:], 0x54000000|imm19|cond)
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
		existing := getU32(g.code[off : off+4])
		// Clear the imm16 field (bits 20:5) and re-encode
		cleared := existing & 0xFFE0001F
		putU32(g.code[off:], cleared|(uint32(chunks[i])<<5))
	}
}

// patchAdrpAdd patches an ADRP+ADD pair at codeOffset to address targetAddr,
// given the PC (virtual address of the ADRP instruction).
func (g *CodeGen) patchAdrpAdd(codeOffset int, pcAddr, targetAddr uint64) {
	pageDelta := int64(targetAddr>>12) - int64(pcAddr>>12)
	pageOff := targetAddr & 0xFFF

	// Patch ADRP: immhi = bits 23:5, immlo = bits 30:29
	immlo := uint32(pageDelta) & 0x3
	immhi := (uint32(pageDelta) >> 2) & 0x7FFFF
	adrp := getU32(g.code[codeOffset:])
	adrp = (adrp & 0x9F00001F) | (immlo << 29) | (immhi << 5)
	putU32(g.code[codeOffset:], adrp)

	// Patch ADD: imm12 = bits 21:10
	addOff := codeOffset + 4
	add := getU32(g.code[addOff:])
	add = (add & 0xFFC003FF) | (uint32(pageOff) << 10)
	putU32(g.code[addOff:], add)
}

// patchAdrpLdr patches an ADRP+LDR pair at codeOffset to load from targetAddr,
// given the PC (virtual address of the ADRP instruction).
// The LDR uses unsigned offset scaled by 8 (for 64-bit loads).
func (g *CodeGen) patchAdrpLdr(codeOffset int, pcAddr, targetAddr uint64) {
	pageDelta := int64(targetAddr>>12) - int64(pcAddr>>12)
	pageOff := targetAddr & 0xFFF

	// Patch ADRP
	immlo := uint32(pageDelta) & 0x3
	immhi := (uint32(pageDelta) >> 2) & 0x7FFFF
	adrp := getU32(g.code[codeOffset:])
	adrp = (adrp & 0x9F00001F) | (immlo << 29) | (immhi << 5)
	putU32(g.code[codeOffset:], adrp)

	// Patch LDR: imm12 = pageOff/8, in bits 21:10
	ldrOff := codeOffset + 4
	ldr := getU32(g.code[ldrOff:])
	scaledOff := uint32(pageOff / 8)
	ldr = (ldr & 0xFFC003FF) | (scaledOff << 10)
	putU32(g.code[ldrOff:], ldr)
}
