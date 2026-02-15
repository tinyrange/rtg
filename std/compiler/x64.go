//go:build !no_backend_linux_amd64 || !no_backend_windows_amd64

package main

// === x86-64 Assembler: mnemonic-level instruction encoding ===

// Register constants
const (
	REG_RAX = 0
	REG_RCX = 1
	REG_RDX = 2
	REG_RBX = 3
	REG_RSP = 4
	REG_RBP = 5
	REG_RSI = 6
	REG_RDI = 7
	REG_R8  = 8
	REG_R9  = 9
	REG_R10 = 10
	REG_R11 = 11
	REG_R12 = 12
	REG_R13 = 13
	REG_R14 = 14
	REG_R15 = 15
)

// Condition code constants for jcc/setcc.
const (
	CC_E  = 0x84 // equal / zero
	CC_NE = 0x85 // not equal / not zero
	CC_L  = 0x8C // less (signed)
	CC_GE = 0x8D // greater or equal (signed)
	CC_LE = 0x8E // less or equal (signed)
	CC_G  = 0x8F // greater (signed)
	CC_AE = 0x83 // above or equal (unsigned) / not carry
	CC_NS = 0x89 // not sign
)

// === Register-immediate64 move ===

// emitMovRegImm64 emits `movabs reg, imm64` (REX.W + B8+rd + imm64)
func (g *CodeGen) emitMovRegImm64(reg int, val uint64) {
	rex := byte(0x48)
	if reg >= 8 {
		rex = 0x49
	}
	g.emitByte(rex)
	g.emitByte(byte(0xb8 + (reg & 7)))
	g.emitU64(val)
}

// === Local variable access (rbp-relative) ===

// emitLoadLocal emits `mov reg, [rbp - offset]`
func (g *CodeGen) emitLoadLocal(offset int, reg int) {
	rex := byte(0x48)
	if reg >= 8 {
		rex = 0x4c
	}
	modrm := byte(0x45 | ((reg & 7) << 3)) // [rbp + disp8] or disp32
	negOff := -offset
	if negOff >= -128 && negOff <= 127 {
		g.emitBytes(rex, 0x8b, modrm, byte(negOff))
	} else {
		modrm = byte(0x85 | ((reg & 7) << 3)) // [rbp + disp32]
		g.emitBytes(rex, 0x8b, modrm)
		g.emitU32(uint32(int32(negOff)))
	}
}

// emitStoreLocal emits `mov [rbp - offset], reg`
func (g *CodeGen) emitStoreLocal(offset int, reg int) {
	rex := byte(0x48)
	if reg >= 8 {
		rex = 0x4c
	}
	modrm := byte(0x45 | ((reg & 7) << 3))
	negOff := -offset
	if negOff >= -128 && negOff <= 127 {
		g.emitBytes(rex, 0x89, modrm, byte(negOff))
	} else {
		modrm = byte(0x85 | ((reg & 7) << 3))
		g.emitBytes(rex, 0x89, modrm)
		g.emitU32(uint32(int32(negOff)))
	}
}

// emitLeaLocal emits `lea reg, [rbp - offset]`
func (g *CodeGen) emitLeaLocal(offset int, reg int) {
	rex := byte(0x48)
	if reg >= 8 {
		rex = 0x4c
	}
	modrm := byte(0x45 | ((reg & 7) << 3))
	negOff := -offset
	if negOff >= -128 && negOff <= 127 {
		g.emitBytes(rex, 0x8d, modrm, byte(negOff))
	} else {
		modrm = byte(0x85 | ((reg & 7) << 3))
		g.emitBytes(rex, 0x8d, modrm)
		g.emitU32(uint32(int32(negOff)))
	}
}

// === x86 stack push/pop ===

// pushR emits `push reg` (handles r8-r15 with REX.B prefix)
func (g *CodeGen) pushR(reg int) {
	if reg >= 8 {
		g.emitBytes(0x41, byte(0x50+(reg&7)))
	} else {
		g.emitByte(byte(0x50 + reg))
	}
}

// popR emits `pop reg` (handles r8-r15 with REX.B prefix)
func (g *CodeGen) popR(reg int) {
	if reg >= 8 {
		g.emitBytes(0x41, byte(0x58+(reg&7)))
	} else {
		g.emitByte(byte(0x58 + reg))
	}
}

// === Register-register operations ===

// rexRR computes the REX prefix for a 64-bit reg-reg operation.
func rexRR(dst, src int) byte {
	rex := byte(0x48)
	if dst >= 8 {
		rex |= 0x04 // REX.R
	}
	if src >= 8 {
		rex |= 0x01 // REX.B
	}
	return rex
}

// modrmRR builds the ModR/M byte for register-direct addressing (mod=11).
func modrmRR(dst, src int) byte {
	return byte(0xc0 | ((dst & 7) << 3) | (src & 7))
}

// movRR emits `mov dst, src`
func (g *CodeGen) movRR(dst, src int) {
	g.emitBytes(rexRR(src, dst), 0x89, modrmRR(src, dst))
}

// addRR emits `add dst, src`
func (g *CodeGen) addRR(dst, src int) {
	g.emitBytes(rexRR(src, dst), 0x01, modrmRR(src, dst))
}

// subRR emits `sub dst, src`
func (g *CodeGen) subRR(dst, src int) {
	g.emitBytes(rexRR(src, dst), 0x29, modrmRR(src, dst))
}

// andRR emits `and dst, src`
func (g *CodeGen) andRR(dst, src int) {
	g.emitBytes(rexRR(src, dst), 0x21, modrmRR(src, dst))
}

// orRR emits `or dst, src`
func (g *CodeGen) orRR(dst, src int) {
	g.emitBytes(rexRR(src, dst), 0x09, modrmRR(src, dst))
}

// xorRR emits `xor dst, src`
func (g *CodeGen) xorRR(dst, src int) {
	g.emitBytes(rexRR(src, dst), 0x31, modrmRR(src, dst))
}

// cmpRR emits `cmp a, b`
func (g *CodeGen) cmpRR(a, b int) {
	g.emitBytes(rexRR(b, a), 0x39, modrmRR(b, a))
}

// testRR emits `test a, b`
func (g *CodeGen) testRR(a, b int) {
	g.emitBytes(rexRR(b, a), 0x85, modrmRR(b, a))
}

// imulRR emits `imul dst, src` (2-byte opcode 0F AF)
func (g *CodeGen) imulRR(dst, src int) {
	g.emitBytes(rexRR(dst, src), 0x0f, 0xaf, modrmRR(dst, src))
}

// === Single-register / no-operand instructions ===

// negR emits `neg reg`
func (g *CodeGen) negR(reg int) {
	rex := byte(0x48)
	if reg >= 8 {
		rex |= 0x01
	}
	g.emitBytes(rex, 0xf7, byte(0xd8|(reg&7)))
}

// cqo emits `cqo` (sign-extend rax into rdx:rax)
func (g *CodeGen) cqo() {
	g.emitBytes(0x48, 0x99)
}

// idivR emits `idiv reg`
func (g *CodeGen) idivR(reg int) {
	rex := byte(0x48)
	if reg >= 8 {
		rex |= 0x01
	}
	g.emitBytes(rex, 0xf7, byte(0xf8|(reg&7)))
}

// shlCl emits `shl reg, cl`
func (g *CodeGen) shlCl(reg int) {
	rex := byte(0x48)
	if reg >= 8 {
		rex |= 0x01
	}
	g.emitBytes(rex, 0xd3, byte(0xe0|(reg&7)))
}

// sarCl emits `sar reg, cl` (arithmetic shift right)
func (g *CodeGen) sarCl(reg int) {
	rex := byte(0x48)
	if reg >= 8 {
		rex |= 0x01
	}
	g.emitBytes(rex, 0xd3, byte(0xf8|(reg&7)))
}

// shlImm emits `shl reg, imm8`
func (g *CodeGen) shlImm(reg int, n byte) {
	rex := byte(0x48)
	if reg >= 8 {
		rex |= 0x01
	}
	g.emitBytes(rex, 0xc1, byte(0xe0|(reg&7)), n)
}

// emitSyscall emits the `syscall` instruction (0x0f, 0x05)
func (g *CodeGen) emitSyscall() {
	g.emitBytes(0x0f, 0x05)
}

// === Register-immediate operations ===

// addRI emits `add reg, imm` (auto-selects imm8 or imm32)
func (g *CodeGen) addRI(reg int, val int32) {
	rex := byte(0x48)
	if reg >= 8 {
		rex |= 0x01
	}
	if val >= -128 && val <= 127 {
		g.emitBytes(rex, 0x83, byte(0xc0|(reg&7)), byte(val))
	} else {
		if reg == REG_RAX {
			g.emitBytes(rex, 0x05)
		} else {
			g.emitBytes(rex, 0x81, byte(0xc0|(reg&7)))
		}
		g.emitU32(uint32(val))
	}
}

// subRI emits `sub reg, imm` (auto-selects imm8 or imm32)
func (g *CodeGen) subRI(reg int, val int32) {
	rex := byte(0x48)
	if reg >= 8 {
		rex |= 0x01
	}
	if val >= -128 && val <= 127 {
		g.emitBytes(rex, 0x83, byte(0xe8|(reg&7)), byte(val))
	} else {
		g.emitBytes(rex, 0x81, byte(0xe8|(reg&7)))
		g.emitU32(uint32(val))
	}
}

// cmpRI emits `cmp reg, imm` (auto-selects imm8 or imm32)
func (g *CodeGen) cmpRI(reg int, val int32) {
	rex := byte(0x48)
	if reg >= 8 {
		rex |= 0x01
	}
	if val >= -128 && val <= 127 {
		g.emitBytes(rex, 0x83, byte(0xf8|(reg&7)), byte(val))
	} else {
		g.emitBytes(rex, 0x81, byte(0xf8|(reg&7)))
		g.emitU32(uint32(val))
	}
}

// xorRI8 emits `xor reg, imm8`
func (g *CodeGen) xorRI8(reg int, val byte) {
	rex := byte(0x48)
	if reg >= 8 {
		rex |= 0x01
	}
	g.emitBytes(rex, 0x83, byte(0xf0|(reg&7)), val)
}

// imulRRI32 emits `imul dst, src, imm32`
func (g *CodeGen) imulRRI32(dst, src int, val int32) {
	g.emitBytes(rexRR(dst, src), 0x69, modrmRR(dst, src))
	g.emitU32(uint32(val))
}

// === Memory load/store with fixed offsets ===

// loadMem emits `mov dst, [base+off]` (64-bit, handles 0/disp8/disp32)
func (g *CodeGen) loadMem(dst, base, off int) {
	rex := rexRR(dst, base)
	if off == 0 && (base&7) != REG_RBP {
		g.emitBytes(rex, 0x8b, byte((dst&7)<<3|(base&7)))
		if (base & 7) == REG_RSP {
			g.emitByte(0x24) // SIB for RSP-based
		}
	} else if off >= -128 && off <= 127 {
		g.emitBytes(rex, 0x8b, byte(0x40|(dst&7)<<3|(base&7)), byte(off))
		if (base & 7) == REG_RSP {
			// Need SIB byte - re-emit
			g.code = g.code[0 : len(g.code)-2]
			g.emitBytes(byte(0x44|(dst&7)<<3), 0x24, byte(off))
		}
	} else {
		g.emitBytes(rex, 0x8b, byte(0x80|(dst&7)<<3|(base&7)))
		if (base & 7) == REG_RSP {
			g.code = g.code[0 : len(g.code)-1]
			g.emitBytes(byte(0x84|(dst&7)<<3), 0x24)
		}
		g.emitU32(uint32(int32(off)))
	}
}

// storeMem emits `mov [base+off], src` (64-bit, handles 0/disp8/disp32)
func (g *CodeGen) storeMem(base, off, src int) {
	rex := rexRR(src, base)
	if off == 0 && (base&7) != REG_RBP {
		g.emitBytes(rex, 0x89, byte((src&7)<<3|(base&7)))
		if (base & 7) == REG_RSP {
			g.emitByte(0x24)
		}
	} else if off >= -128 && off <= 127 {
		g.emitBytes(rex, 0x89, byte(0x40|(src&7)<<3|(base&7)), byte(off))
		if (base & 7) == REG_RSP {
			g.code = g.code[0 : len(g.code)-2]
			g.emitBytes(byte(0x44|(src&7)<<3), 0x24, byte(off))
		}
	} else {
		g.emitBytes(rex, 0x89, byte(0x80|(src&7)<<3|(base&7)))
		if (base & 7) == REG_RSP {
			g.code = g.code[0 : len(g.code)-1]
			g.emitBytes(byte(0x84|(src&7)<<3), 0x24)
		}
		g.emitU32(uint32(int32(off)))
	}
}

// loadMemByte emits `movzx dst, byte [base+off]`
func (g *CodeGen) loadMemByte(dst, base, off int) {
	rex := rexRR(dst, base)
	if off == 0 && (base&7) != REG_RBP {
		g.emitBytes(rex, 0x0f, 0xb6, byte((dst&7)<<3|(base&7)))
	} else if off >= -128 && off <= 127 {
		g.emitBytes(rex, 0x0f, 0xb6, byte(0x40|(dst&7)<<3|(base&7)), byte(off))
	} else {
		g.emitBytes(rex, 0x0f, 0xb6, byte(0x80|(dst&7)<<3|(base&7)))
		g.emitU32(uint32(int32(off)))
	}
}

// storeMemByte emits `mov byte [base+off], src_lo8`
func (g *CodeGen) storeMemByte(base, off, src int) {
	rex := byte(0x40)
	if src >= 8 {
		rex |= 0x04
	}
	if base >= 8 {
		rex |= 0x01
	}
	if off == 0 && (base&7) != REG_RBP {
		g.emitBytes(rex, 0x88, byte((src&7)<<3|(base&7)))
	} else if off >= -128 && off <= 127 {
		g.emitBytes(rex, 0x88, byte(0x40|(src&7)<<3|(base&7)), byte(off))
	} else {
		g.emitBytes(rex, 0x88, byte(0x80|(src&7)<<3|(base&7)))
		g.emitU32(uint32(int32(off)))
	}
}

// === Extend/truncate ===

// movzxB emits `movzx reg, reg_lo8`
func (g *CodeGen) movzxB(reg int) {
	rex := rexRR(reg, reg)
	g.emitBytes(rex, 0x0f, 0xb6, modrmRR(reg, reg))
}

// movzxW emits `movzx reg, reg_lo16`
func (g *CodeGen) movzxW(reg int) {
	rex := rexRR(reg, reg)
	g.emitBytes(rex, 0x0f, 0xb7, modrmRR(reg, reg))
}

// movsxD emits `movsxd reg, reg_lo32`
func (g *CodeGen) movsxD(reg int) {
	rex := rexRR(reg, reg)
	g.emitBytes(rex, 0x63, modrmRR(reg, reg))
}

// clearHi32 emits `mov e_reg, e_reg` (zero-extends 32→64)
func (g *CodeGen) clearHi32(reg int) {
	prefix := byte(0)
	if reg >= 8 {
		prefix = 0x45 // REX.R + REX.B
	}
	if prefix != 0 {
		g.emitByte(prefix)
	}
	g.emitBytes(0x89, modrmRR(reg, reg))
}

// === Setcc ===

// setcc emits `setCC reg_lo8` where cc is a condition code constant
func (g *CodeGen) setcc(cc byte, reg int) {
	setccOp := byte(0x90 | (cc & 0x0f))
	rex := byte(0)
	if reg >= 8 {
		rex = 0x41
	}
	if rex != 0 {
		g.emitBytes(rex, 0x0f, setccOp, byte(0xc0|(reg&7)))
	} else {
		g.emitBytes(0x0f, setccOp, byte(0xc0|(reg&7)))
	}
}
