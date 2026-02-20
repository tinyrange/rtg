//go:build !no_backend_dos_i386

package x64

// === x86-16 Assembler: mnemonic-level instruction encoding ===
//
// This is a 16-bit real-mode encoder for DOS-target bringup. It is intentionally
// separate from i386.go so the dos/8086 backend can migrate incrementally from the
// current 32-bit stream to proper 16-bit machine code.

// 16-bit general-purpose registers.
const (
	REG16_AX = 0
	REG16_CX = 1
	REG16_DX = 2
	REG16_BX = 3
	REG16_SP = 4
	REG16_BP = 5
	REG16_SI = 6
	REG16_DI = 7
)

// 16-bit addressing forms (r/m field when mod != 00 for BP-only form).
const (
	EA16_BX_SI = 0
	EA16_BX_DI = 1
	EA16_BP_SI = 2
	EA16_BP_DI = 3
	EA16_SI    = 4
	EA16_DI    = 5
	EA16_BP    = 6
	EA16_BX    = 7
)

// Condition code low-nibble values used by short Jcc (0x70+cc).
const (
	CC16_O  = 0x0
	CC16_NO = 0x1
	CC16_B  = 0x2
	CC16_AE = 0x3
	CC16_E  = 0x4
	CC16_NE = 0x5
	CC16_BE = 0x6
	CC16_A  = 0x7
	CC16_S  = 0x8
	CC16_NS = 0x9
	CC16_P  = 0xA
	CC16_NP = 0xB
	CC16_L  = 0xC
	CC16_GE = 0xD
	CC16_LE = 0xE
	CC16_G  = 0xF
)

// modrmRR16 builds a register-direct ModR/M byte (mod=11).
func modrmRR16(dst, src int) byte {
	return byte(0xC0 | ((dst & 7) << 3) | (src & 7))
}

// emitMovRegImm16 emits `mov reg16, imm16` (B8+rw iw).
func (g *CodeGen) emitMovRegImm16(reg int, val uint16) {
	g.emitByte(byte(0xB8 + (reg & 7)))
	g.emitU16(val)
}

// pushR16 emits `push reg16`.
func (g *CodeGen) pushR16(reg int) {
	g.emitByte(byte(0x50 + (reg & 7)))
}

// popR16 emits `pop reg16`.
func (g *CodeGen) popR16(reg int) {
	g.emitByte(byte(0x58 + (reg & 7)))
}

// movRR16 emits `mov dst, src`.
func (g *CodeGen) movRR16(dst, src int) {
	g.emitBytes(0x89, modrmRR16(src, dst))
}

// addRR16 emits `add dst, src`.
func (g *CodeGen) addRR16(dst, src int) {
	g.emitBytes(0x01, modrmRR16(src, dst))
}

// subRR16 emits `sub dst, src`.
func (g *CodeGen) subRR16(dst, src int) {
	g.emitBytes(0x29, modrmRR16(src, dst))
}

// andRR16 emits `and dst, src`.
func (g *CodeGen) andRR16(dst, src int) {
	g.emitBytes(0x21, modrmRR16(src, dst))
}

// orRR16 emits `or dst, src`.
func (g *CodeGen) orRR16(dst, src int) {
	g.emitBytes(0x09, modrmRR16(src, dst))
}

// xorRR16 emits `xor dst, src`.
func (g *CodeGen) xorRR16(dst, src int) {
	g.emitBytes(0x31, modrmRR16(src, dst))
}

// cmpRR16 emits `cmp a, b`.
func (g *CodeGen) cmpRR16(a, b int) {
	g.emitBytes(0x39, modrmRR16(b, a))
}

// testRR16 emits `test a, b`.
func (g *CodeGen) testRR16(a, b int) {
	g.emitBytes(0x85, modrmRR16(b, a))
}

// negR16 emits `neg reg`.
func (g *CodeGen) negR16(reg int) {
	g.emitBytes(0xF7, byte(0xD8|(reg&7)))
}

// cwd16 emits `cwd` (sign-extend ax into dx:ax).
func (g *CodeGen) cwd16() {
	g.emitByte(0x99)
}

// idivR16 emits `idiv reg16`.
func (g *CodeGen) idivR16(reg int) {
	g.emitBytes(0xF7, byte(0xF8|(reg&7)))
}

// shlCl16 emits `shl reg16, cl`.
func (g *CodeGen) shlCl16(reg int) {
	g.emitBytes(0xD3, byte(0xE0|(reg&7)))
}

// sarCl16 emits `sar reg16, cl`.
func (g *CodeGen) sarCl16(reg int) {
	g.emitBytes(0xD3, byte(0xF8|(reg&7)))
}

// shlImm16 emits `shl reg16, imm8`.
func (g *CodeGen) shlImm16(reg int, n byte) {
	g.emitBytes(0xC1, byte(0xE0|(reg&7)), n)
}

// addRI16 emits `add reg16, imm` (imm8 or imm16).
func (g *CodeGen) addRI16(reg int, val int16) {
	if val >= -128 && val <= 127 {
		g.emitBytes(0x83, byte(0xC0|(reg&7)), byte(val))
		return
	}
	if reg == REG16_AX {
		g.emitByte(0x05)
	} else {
		g.emitBytes(0x81, byte(0xC0|(reg&7)))
	}
	g.emitU16(uint16(val))
}

// subRI16 emits `sub reg16, imm` (imm8 or imm16).
func (g *CodeGen) subRI16(reg int, val int16) {
	if val >= -128 && val <= 127 {
		g.emitBytes(0x83, byte(0xE8|(reg&7)), byte(val))
		return
	}
	g.emitBytes(0x81, byte(0xE8|(reg&7)))
	g.emitU16(uint16(val))
}

// cmpRI16 emits `cmp reg16, imm` (imm8 or imm16).
func (g *CodeGen) cmpRI16(reg int, val int16) {
	if val >= -128 && val <= 127 {
		g.emitBytes(0x83, byte(0xF8|(reg&7)), byte(val))
		return
	}
	if reg == REG16_AX {
		g.emitByte(0x3D)
	} else {
		g.emitBytes(0x81, byte(0xF8|(reg&7)))
	}
	g.emitU16(uint16(val))
}

// xorRI8_16 emits `xor reg16, imm8` (sign-extended).
func (g *CodeGen) xorRI8_16(reg int, val byte) {
	g.emitBytes(0x83, byte(0xF0|(reg&7)), val)
}

// jccRel8_16 emits `jcc rel8`.
func (g *CodeGen) jccRel8_16(cc byte, off int8) {
	g.emitBytes(byte(0x70|(cc&0x0F)), byte(off))
}

// jmpRel8_16 emits `jmp rel8`.
func (g *CodeGen) jmpRel8_16(off int8) {
	g.emitBytes(0xEB, byte(off))
}

// callRel16 emits `call rel16`.
func (g *CodeGen) callRel16(off int16) {
	g.emitByte(0xE8)
	g.emitU16(uint16(off))
}

// jmpRel16 emits `jmp rel16`.
func (g *CodeGen) jmpRel16(off int16) {
	g.emitByte(0xE9)
	g.emitU16(uint16(off))
}

// ret16 emits `ret`.
func (g *CodeGen) ret16() {
	g.emitByte(0xC3)
}

// emitInt21 emits `int 21h`.
func (g *CodeGen) emitInt21() {
	g.emitBytes(0xCD, 0x21)
}

// emitInt encodes `int imm8`.
func (g *CodeGen) emitInt(vec byte) {
	g.emitBytes(0xCD, vec)
}

func modrmMem16(mod byte, reg int, rm byte) byte {
	return byte((mod << 6) | byte((reg&7)<<3) | (rm & 7))
}

// emitEA16 emits displacement for a 16-bit addressing mode.
func (g *CodeGen) emitEA16(mod byte, disp int16) {
	if mod == 1 {
		g.emitByte(byte(disp))
	} else if mod == 2 || mod == 0 {
		g.emitU16(uint16(disp))
	}
}

// emitLoadRM16 emits `mov reg16, [ea+disp]`.
func (g *CodeGen) emitLoadRM16(dst int, ea int, disp int16) {
	mod := byte(0)
	if ea == EA16_BP && disp == 0 {
		mod = 1 // [bp] requires disp8=0 in 16-bit mode.
	} else if disp >= -128 && disp <= 127 {
		if disp != 0 {
			mod = 1
		}
	} else {
		mod = 2
	}
	g.emitBytes(0x8B, modrmMem16(mod, dst, byte(ea)))
	if mod == 1 {
		g.emitByte(byte(disp))
	} else if mod == 2 {
		g.emitU16(uint16(disp))
	} else if mod == 0 && ea == EA16_BP {
		g.emitByte(0)
	}
}

// emitStoreRM16 emits `mov [ea+disp], reg16`.
func (g *CodeGen) emitStoreRM16(ea int, disp int16, src int) {
	mod := byte(0)
	if ea == EA16_BP && disp == 0 {
		mod = 1
	} else if disp >= -128 && disp <= 127 {
		if disp != 0 {
			mod = 1
		}
	} else {
		mod = 2
	}
	g.emitBytes(0x89, modrmMem16(mod, src, byte(ea)))
	if mod == 1 {
		g.emitByte(byte(disp))
	} else if mod == 2 {
		g.emitU16(uint16(disp))
	} else if mod == 0 && ea == EA16_BP {
		g.emitByte(0)
	}
}

// emitLeaRM16 emits `lea reg16, [ea+disp]`.
func (g *CodeGen) emitLeaRM16(dst int, ea int, disp int16) {
	mod := byte(0)
	if ea == EA16_BP && disp == 0 {
		mod = 1
	} else if disp >= -128 && disp <= 127 {
		if disp != 0 {
			mod = 1
		}
	} else {
		mod = 2
	}
	g.emitBytes(0x8D, modrmMem16(mod, dst, byte(ea)))
	if mod == 1 {
		g.emitByte(byte(disp))
	} else if mod == 2 {
		g.emitU16(uint16(disp))
	} else if mod == 0 && ea == EA16_BP {
		g.emitByte(0)
	}
}

// emitLoadAbs16 emits `mov reg16, [disp16]` (DS default segment).
func (g *CodeGen) emitLoadAbs16(dst int, disp uint16) {
	g.emitBytes(0x8B, modrmMem16(0, dst, 6))
	g.emitU16(disp)
}

// emitStoreAbs16 emits `mov [disp16], reg16` (DS default segment).
func (g *CodeGen) emitStoreAbs16(disp uint16, src int) {
	g.emitBytes(0x89, modrmMem16(0, src, 6))
	g.emitU16(disp)
}
