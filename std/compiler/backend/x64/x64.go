//go:build !no_backend_linux_amd64 || !no_backend_windows_amd64

package x64

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
	CC_B  = 0x82 // below (unsigned)
	CC_BE = 0x86 // below or equal (unsigned)
	CC_A  = 0x87 // above (unsigned)
	CC_AE = 0x83 // above or equal (unsigned) / not carry
	CC_NS = 0x89 // not sign
)

// === Register-immediate64 move ===

// EmitMovRegImm64 emits `movabs reg, imm64` (REX.W + B8+rd + imm64)
//rtg:profile
func (g *CodeGen) EmitMovRegImm64(reg int, val uint64) {
	rex := byte(0x48)
	if reg >= 8 {
		rex = 0x49
	}
	g.EmitByte(rex)
	g.EmitByte(byte(0xb8 + (reg & 7)))
	g.EmitU64(val)
}

// === Local variable access (rbp-relative) ===

// EmitLoadLocal emits `mov reg, [rbp - offset]`
//rtg:profile
func (g *CodeGen) EmitLoadLocal(offset int, reg int) {
	rex := byte(0x48)
	if reg >= 8 {
		rex = 0x4c
	}
	modrm := byte(0x45 | ((reg & 7) << 3)) // [rbp + disp8] or disp32
	negOff := -offset
	if negOff >= -128 && negOff <= 127 {
		g.EmitBytes(rex, 0x8b, modrm, byte(negOff))
	} else {
		modrm = byte(0x85 | ((reg & 7) << 3)) // [rbp + disp32]
		g.EmitBytes(rex, 0x8b, modrm)
		g.EmitU32(uint32(int32(negOff)))
	}
}

// emitStoreLocal emits `mov [rbp - offset], reg`
//rtg:profile
func (g *CodeGen) emitStoreLocal(offset int, reg int) {
	rex := byte(0x48)
	if reg >= 8 {
		rex = 0x4c
	}
	modrm := byte(0x45 | ((reg & 7) << 3))
	negOff := -offset
	if negOff >= -128 && negOff <= 127 {
		g.EmitBytes(rex, 0x89, modrm, byte(negOff))
	} else {
		modrm = byte(0x85 | ((reg & 7) << 3))
		g.EmitBytes(rex, 0x89, modrm)
		g.EmitU32(uint32(int32(negOff)))
	}
}

// emitLeaLocal emits `lea reg, [rbp - offset]`
//rtg:profile
func (g *CodeGen) emitLeaLocal(offset int, reg int) {
	rex := byte(0x48)
	if reg >= 8 {
		rex = 0x4c
	}
	modrm := byte(0x45 | ((reg & 7) << 3))
	negOff := -offset
	if negOff >= -128 && negOff <= 127 {
		g.EmitBytes(rex, 0x8d, modrm, byte(negOff))
	} else {
		modrm = byte(0x85 | ((reg & 7) << 3))
		g.EmitBytes(rex, 0x8d, modrm)
		g.EmitU32(uint32(int32(negOff)))
	}
}

// emitAddLocalImm emits `add qword [rbp - offset], imm{8,32}`.
//rtg:profile
func (g *CodeGen) emitAddLocalImm(offset int, imm int32) {
	negOff := -offset
	if imm >= -128 && imm <= 127 {
		// 48 83 /0 ib
		if negOff >= -128 && negOff <= 127 {
			g.EmitBytes(0x48, 0x83, 0x45, byte(negOff), byte(imm))
		} else {
			g.EmitBytes(0x48, 0x83, 0x85)
			g.EmitU32(uint32(int32(negOff)))
			g.EmitByte(byte(imm))
		}
		return
	}
	// 48 81 /0 id
	if negOff >= -128 && negOff <= 127 {
		g.EmitBytes(0x48, 0x81, 0x45, byte(negOff))
		g.EmitU32(uint32(imm))
	} else {
		g.EmitBytes(0x48, 0x81, 0x85)
		g.EmitU32(uint32(int32(negOff)))
		g.EmitU32(uint32(imm))
	}
}

// === x86 stack push/pop ===

// PushR emits `push reg` (handles r8-r15 with REX.B prefix)
//rtg:profile
func (g *CodeGen) PushR(reg int) {
	if reg >= 8 {
		g.EmitBytes(0x41, byte(0x50+(reg&7)))
	} else {
		g.EmitByte(byte(0x50 + reg))
	}
}

// PopR emits `pop reg` (handles r8-r15 with REX.B prefix)
//rtg:profile
func (g *CodeGen) PopR(reg int) {
	if reg >= 8 {
		g.EmitBytes(0x41, byte(0x58+(reg&7)))
	} else {
		g.EmitByte(byte(0x58 + reg))
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

// MovRR emits `mov dst, src`
//rtg:profile
func (g *CodeGen) MovRR(dst, src int) {
	g.EmitBytes(rexRR(src, dst), 0x89, modrmRR(src, dst))
}

// AddRR emits `add dst, src`
//rtg:profile
func (g *CodeGen) AddRR(dst, src int) {
	g.EmitBytes(rexRR(src, dst), 0x01, modrmRR(src, dst))
}

// subRR emits `sub dst, src`
//rtg:profile
func (g *CodeGen) subRR(dst, src int) {
	g.EmitBytes(rexRR(src, dst), 0x29, modrmRR(src, dst))
}

// andRR emits `and dst, src`
//rtg:profile
func (g *CodeGen) andRR(dst, src int) {
	g.EmitBytes(rexRR(src, dst), 0x21, modrmRR(src, dst))
}

// orRR emits `or dst, src`
//rtg:profile
func (g *CodeGen) orRR(dst, src int) {
	g.EmitBytes(rexRR(src, dst), 0x09, modrmRR(src, dst))
}

// XorRR emits `xor dst, src`
//rtg:profile
func (g *CodeGen) XorRR(dst, src int) {
	g.EmitBytes(rexRR(src, dst), 0x31, modrmRR(src, dst))
}

// CmpRR emits `cmp a, b`
//rtg:profile
func (g *CodeGen) CmpRR(a, b int) {
	g.EmitBytes(rexRR(b, a), 0x39, modrmRR(b, a))
}

// TestRR emits `test a, b`
//rtg:profile
func (g *CodeGen) TestRR(a, b int) {
	g.EmitBytes(rexRR(b, a), 0x85, modrmRR(b, a))
}

// imulRR emits `imul dst, src` (2-byte opcode 0F AF)
//rtg:profile
func (g *CodeGen) imulRR(dst, src int) {
	g.EmitBytes(rexRR(dst, src), 0x0f, 0xaf, modrmRR(dst, src))
}

// === Single-register / no-operand instructions ===

// negR emits `neg reg`
//rtg:profile
func (g *CodeGen) negR(reg int) {
	rex := byte(0x48)
	if reg >= 8 {
		rex |= 0x01
	}
	g.EmitBytes(rex, 0xf7, byte(0xd8|(reg&7)))
}

// cqo emits `cqo` (sign-extend rax into rdx:rax)
//rtg:profile
func (g *CodeGen) cqo() {
	g.EmitBytes(0x48, 0x99)
}

// idivR emits `idiv reg`
//rtg:profile
func (g *CodeGen) idivR(reg int) {
	rex := byte(0x48)
	if reg >= 8 {
		rex |= 0x01
	}
	g.EmitBytes(rex, 0xf7, byte(0xf8|(reg&7)))
}

// divR emits `div reg` (unsigned divide)
//rtg:profile
func (g *CodeGen) divR(reg int) {
	rex := byte(0x48)
	if reg >= 8 {
		rex |= 0x01
	}
	g.EmitBytes(rex, 0xf7, byte(0xf0|(reg&7)))
}

// shlCl emits `shl reg, cl`
//rtg:profile
func (g *CodeGen) shlCl(reg int) {
	rex := byte(0x48)
	if reg >= 8 {
		rex |= 0x01
	}
	g.EmitBytes(rex, 0xd3, byte(0xe0|(reg&7)))
}

// sarCl emits `sar reg, cl` (arithmetic shift right)
//rtg:profile
func (g *CodeGen) sarCl(reg int) {
	rex := byte(0x48)
	if reg >= 8 {
		rex |= 0x01
	}
	g.EmitBytes(rex, 0xd3, byte(0xf8|(reg&7)))
}

// shrCl emits `shr reg, cl` (logical shift right)
//rtg:profile
func (g *CodeGen) shrCl(reg int) {
	rex := byte(0x48)
	if reg >= 8 {
		rex |= 0x01
	}
	g.EmitBytes(rex, 0xd3, byte(0xe8|(reg&7)))
}

// shlImm emits `shl reg, imm8`
//rtg:profile
func (g *CodeGen) shlImm(reg int, n byte) {
	rex := byte(0x48)
	if reg >= 8 {
		rex |= 0x01
	}
	g.EmitBytes(rex, 0xc1, byte(0xe0|(reg&7)), n)
}

// emitSyscall emits the `syscall` instruction (0x0f, 0x05)
//rtg:profile
func (g *CodeGen) emitSyscall() {
	g.EmitBytes(0x0f, 0x05)
}

// === Register-immediate operations ===

// AddRI emits `add reg, imm` (auto-selects imm8 or imm32)
//rtg:profile
func (g *CodeGen) AddRI(reg int, val int32) {
	rex := byte(0x48)
	if reg >= 8 {
		rex |= 0x01
	}
	if val >= -128 && val <= 127 {
		g.EmitBytes(rex, 0x83, byte(0xc0|(reg&7)), byte(val))
	} else {
		if reg == REG_RAX {
			g.EmitBytes(rex, 0x05)
		} else {
			g.EmitBytes(rex, 0x81, byte(0xc0|(reg&7)))
		}
		g.EmitU32(uint32(val))
	}
}

// SubRI emits `sub reg, imm` (auto-selects imm8 or imm32)
//rtg:profile
func (g *CodeGen) SubRI(reg int, val int32) {
	rex := byte(0x48)
	if reg >= 8 {
		rex |= 0x01
	}
	if val >= -128 && val <= 127 {
		g.EmitBytes(rex, 0x83, byte(0xe8|(reg&7)), byte(val))
	} else {
		g.EmitBytes(rex, 0x81, byte(0xe8|(reg&7)))
		g.EmitU32(uint32(val))
	}
}

// cmpRI emits `cmp reg, imm` (auto-selects imm8 or imm32)
//rtg:profile
func (g *CodeGen) cmpRI(reg int, val int32) {
	rex := byte(0x48)
	if reg >= 8 {
		rex |= 0x01
	}
	if val >= -128 && val <= 127 {
		g.EmitBytes(rex, 0x83, byte(0xf8|(reg&7)), byte(val))
	} else {
		g.EmitBytes(rex, 0x81, byte(0xf8|(reg&7)))
		g.EmitU32(uint32(val))
	}
}

// xorRI8 emits `xor reg, imm8`
//rtg:profile
func (g *CodeGen) xorRI8(reg int, val byte) {
	rex := byte(0x48)
	if reg >= 8 {
		rex |= 0x01
	}
	g.EmitBytes(rex, 0x83, byte(0xf0|(reg&7)), val)
}

// imulRRI32 emits `imul dst, src, imm32`
//rtg:profile
func (g *CodeGen) imulRRI32(dst, src int, val int32) {
	g.EmitBytes(rexRR(dst, src), 0x69, modrmRR(dst, src))
	g.EmitU32(uint32(val))
}

// === Memory load/store with fixed offsets ===

// LoadMem emits `mov dst, [base+off]` (64-bit, handles 0/disp8/disp32)
//rtg:profile
func (g *CodeGen) LoadMem(dst, base, off int) {
	rex := rexRR(dst, base)
	if off == 0 && (base&7) != REG_RBP {
		g.EmitBytes(rex, 0x8b, byte((dst&7)<<3|(base&7)))
		if (base & 7) == REG_RSP {
			g.EmitByte(0x24) // SIB for RSP-based
		}
	} else if off >= -128 && off <= 127 {
		g.EmitBytes(rex, 0x8b, byte(0x40|(dst&7)<<3|(base&7)), byte(off))
		if (base & 7) == REG_RSP {
			// Need SIB byte - re-emit
			g.Code = g.Code[0 : len(g.Code)-2]
			g.EmitBytes(byte(0x44|(dst&7)<<3), 0x24, byte(off))
		}
	} else {
		g.EmitBytes(rex, 0x8b, byte(0x80|(dst&7)<<3|(base&7)))
		if (base & 7) == REG_RSP {
			g.Code = g.Code[0 : len(g.Code)-1]
			g.EmitBytes(byte(0x84|(dst&7)<<3), 0x24)
		}
		g.EmitU32(uint32(int32(off)))
	}
}

// storeMem emits `mov [base+off], src` (64-bit, handles 0/disp8/disp32)
//rtg:profile
func (g *CodeGen) storeMem(base, off, src int) {
	rex := rexRR(src, base)
	if off == 0 && (base&7) != REG_RBP {
		g.EmitBytes(rex, 0x89, byte((src&7)<<3|(base&7)))
		if (base & 7) == REG_RSP {
			g.EmitByte(0x24)
		}
	} else if off >= -128 && off <= 127 {
		g.EmitBytes(rex, 0x89, byte(0x40|(src&7)<<3|(base&7)), byte(off))
		if (base & 7) == REG_RSP {
			g.Code = g.Code[0 : len(g.Code)-2]
			g.EmitBytes(byte(0x44|(src&7)<<3), 0x24, byte(off))
		}
	} else {
		g.EmitBytes(rex, 0x89, byte(0x80|(src&7)<<3|(base&7)))
		if (base & 7) == REG_RSP {
			g.Code = g.Code[0 : len(g.Code)-1]
			g.EmitBytes(byte(0x84|(src&7)<<3), 0x24)
		}
		g.EmitU32(uint32(int32(off)))
	}
}

// loadMemByte emits `movzx dst, byte [base+off]`
//rtg:profile
func (g *CodeGen) loadMemByte(dst, base, off int) {
	rex := rexRR(dst, base)
	if off == 0 && (base&7) != REG_RBP {
		g.EmitBytes(rex, 0x0f, 0xb6, byte((dst&7)<<3|(base&7)))
	} else if off >= -128 && off <= 127 {
		g.EmitBytes(rex, 0x0f, 0xb6, byte(0x40|(dst&7)<<3|(base&7)), byte(off))
	} else {
		g.EmitBytes(rex, 0x0f, 0xb6, byte(0x80|(dst&7)<<3|(base&7)))
		g.EmitU32(uint32(int32(off)))
	}
}

// storeMemByte emits `mov byte [base+off], src_lo8`
//rtg:profile
func (g *CodeGen) storeMemByte(base, off, src int) {
	rex := byte(0x40)
	if src >= 8 {
		rex |= 0x04
	}
	if base >= 8 {
		rex |= 0x01
	}
	if off == 0 && (base&7) != REG_RBP {
		g.EmitBytes(rex, 0x88, byte((src&7)<<3|(base&7)))
	} else if off >= -128 && off <= 127 {
		g.EmitBytes(rex, 0x88, byte(0x40|(src&7)<<3|(base&7)), byte(off))
	} else {
		g.EmitBytes(rex, 0x88, byte(0x80|(src&7)<<3|(base&7)))
		g.EmitU32(uint32(int32(off)))
	}
}

// === Extend/truncate ===

// movzxB emits `movzx reg, reg_lo8`
//rtg:profile
func (g *CodeGen) movzxB(reg int) {
	rex := rexRR(reg, reg)
	g.EmitBytes(rex, 0x0f, 0xb6, modrmRR(reg, reg))
}

// movzxW emits `movzx reg, reg_lo16`
//rtg:profile
func (g *CodeGen) movzxW(reg int) {
	rex := rexRR(reg, reg)
	g.EmitBytes(rex, 0x0f, 0xb7, modrmRR(reg, reg))
}

// movsxB emits `movsx reg, reg_lo8`
//rtg:profile
func (g *CodeGen) movsxB(reg int) {
	rex := rexRR(reg, reg)
	g.EmitBytes(rex, 0x0f, 0xbe, modrmRR(reg, reg))
}

// movsxW emits `movsx reg, reg_lo16`
//rtg:profile
func (g *CodeGen) movsxW(reg int) {
	rex := rexRR(reg, reg)
	g.EmitBytes(rex, 0x0f, 0xbf, modrmRR(reg, reg))
}

// movsxD emits `movsxd reg, reg_lo32`
//rtg:profile
func (g *CodeGen) movsxD(reg int) {
	rex := rexRR(reg, reg)
	g.EmitBytes(rex, 0x63, modrmRR(reg, reg))
}

// clearHi32 emits `mov e_reg, e_reg` (zero-extends 32→64)
//rtg:profile
func (g *CodeGen) clearHi32(reg int) {
	prefix := byte(0)
	if reg >= 8 {
		prefix = 0x45 // REX.R + REX.B
	}
	if prefix != 0 {
		g.EmitByte(prefix)
	}
	g.EmitBytes(0x89, modrmRR(reg, reg))
}

// === Setcc ===

// setcc emits `setCC reg_lo8` where cc is a condition code constant
//rtg:profile
func (g *CodeGen) setcc(cc byte, reg int) {
	setccOp := byte(0x90 | (cc & 0x0f))
	rex := byte(0)
	if reg >= 8 {
		rex = 0x41
	}
	if rex != 0 {
		g.EmitBytes(rex, 0x0f, setccOp, byte(0xc0|(reg&7)))
	} else {
		g.EmitBytes(0x0f, setccOp, byte(0xc0|(reg&7)))
	}
}
