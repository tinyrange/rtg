//go:build !no_backend_linux_i386 || !no_backend_windows_i386

package main

// === i386 Assembler: mnemonic-level instruction encoding ===

// Register constants (i386 uses only 8 registers)
const (
	REG32_EAX = 0
	REG32_ECX = 1
	REG32_EDX = 2
	REG32_EBX = 3
	REG32_ESP = 4
	REG32_EBP = 5
	REG32_ESI = 6
	REG32_EDI = 7
)

// Condition code constants for jcc/setcc (same as amd64).
const (
	CC32_E  = 0x84 // equal / zero
	CC32_NE = 0x85 // not equal / not zero
	CC32_L  = 0x8C // less (signed)
	CC32_GE = 0x8D // greater or equal (signed)
	CC32_LE = 0x8E // less or equal (signed)
	CC32_G  = 0x8F // greater (signed)
	CC32_AE = 0x83 // above or equal (unsigned) / not carry
	CC32_NS = 0x89 // not sign
)

// === Register-immediate32 move ===

// emitMovRegImm32 emits `mov reg, imm32` (B8+rd imm32, 5 bytes)
func (g *CodeGen) emitMovRegImm32(reg int, val uint32) {
	g.emitByte(byte(0xb8 + reg))
	g.emitU32(val)
}

// === Local variable access (ebp-relative, 32-bit) ===

// emitLoadLocal32 emits `mov reg, [ebp - offset]`
func (g *CodeGen) emitLoadLocal32(offset int, reg int) {
	negOff := -offset
	if negOff >= -128 && negOff <= 127 {
		g.emitBytes(0x8b, byte(0x45|((reg&7)<<3)), byte(negOff))
	} else {
		g.emitBytes(0x8b, byte(0x85|((reg&7)<<3)))
		g.emitU32(uint32(int32(negOff)))
	}
}

// emitStoreLocal32 emits `mov [ebp - offset], reg`
func (g *CodeGen) emitStoreLocal32(offset int, reg int) {
	negOff := -offset
	if negOff >= -128 && negOff <= 127 {
		g.emitBytes(0x89, byte(0x45|((reg&7)<<3)), byte(negOff))
	} else {
		g.emitBytes(0x89, byte(0x85|((reg&7)<<3)))
		g.emitU32(uint32(int32(negOff)))
	}
}

// emitLeaLocal32 emits `lea reg, [ebp - offset]`
func (g *CodeGen) emitLeaLocal32(offset int, reg int) {
	negOff := -offset
	if negOff >= -128 && negOff <= 127 {
		g.emitBytes(0x8d, byte(0x45|((reg&7)<<3)), byte(negOff))
	} else {
		g.emitBytes(0x8d, byte(0x85|((reg&7)<<3)))
		g.emitU32(uint32(int32(negOff)))
	}
}

// === x86 stack push/pop (32-bit) ===

// pushR32 emits `push reg`
func (g *CodeGen) pushR32(reg int) {
	g.emitByte(byte(0x50 + reg))
}

// popR32 emits `pop reg`
func (g *CodeGen) popR32(reg int) {
	g.emitByte(byte(0x58 + reg))
}

// === Register-register operations (32-bit, no REX) ===

// modrmRR32 builds the ModR/M byte for register-direct addressing (mod=11).
func modrmRR32(dst, src int) byte {
	return byte(0xc0 | ((dst & 7) << 3) | (src & 7))
}

// movRR32 emits `mov dst, src`
func (g *CodeGen) movRR32(dst, src int) {
	g.emitBytes(0x89, modrmRR32(src, dst))
}

// addRR32 emits `add dst, src`
func (g *CodeGen) addRR32(dst, src int) {
	g.emitBytes(0x01, modrmRR32(src, dst))
}

// subRR32 emits `sub dst, src`
func (g *CodeGen) subRR32(dst, src int) {
	g.emitBytes(0x29, modrmRR32(src, dst))
}

// andRR32 emits `and dst, src`
func (g *CodeGen) andRR32(dst, src int) {
	g.emitBytes(0x21, modrmRR32(src, dst))
}

// orRR32 emits `or dst, src`
func (g *CodeGen) orRR32(dst, src int) {
	g.emitBytes(0x09, modrmRR32(src, dst))
}

// xorRR32 emits `xor dst, src`
func (g *CodeGen) xorRR32(dst, src int) {
	g.emitBytes(0x31, modrmRR32(src, dst))
}

// cmpRR32 emits `cmp a, b`
func (g *CodeGen) cmpRR32(a, b int) {
	g.emitBytes(0x39, modrmRR32(b, a))
}

// testRR32 emits `test a, b`
func (g *CodeGen) testRR32(a, b int) {
	g.emitBytes(0x85, modrmRR32(b, a))
}

// imulRR32 emits `imul dst, src` (2-byte opcode 0F AF)
func (g *CodeGen) imulRR32(dst, src int) {
	g.emitBytes(0x0f, 0xaf, modrmRR32(dst, src))
}

// === Single-register / no-operand instructions ===

// negR32 emits `neg reg`
func (g *CodeGen) negR32(reg int) {
	g.emitBytes(0xf7, byte(0xd8|(reg&7)))
}

// cdq32 emits `cdq` (sign-extend eax into edx:eax)
func (g *CodeGen) cdq32() {
	g.emitByte(0x99)
}

// idivR32 emits `idiv reg`
func (g *CodeGen) idivR32(reg int) {
	g.emitBytes(0xf7, byte(0xf8|(reg&7)))
}

// shlCl32 emits `shl reg, cl`
func (g *CodeGen) shlCl32(reg int) {
	g.emitBytes(0xd3, byte(0xe0|(reg&7)))
}

// sarCl32 emits `sar reg, cl` (arithmetic shift right)
func (g *CodeGen) sarCl32(reg int) {
	g.emitBytes(0xd3, byte(0xf8|(reg&7)))
}

// shlImm32 emits `shl reg, imm8`
func (g *CodeGen) shlImm32(reg int, n byte) {
	g.emitBytes(0xc1, byte(0xe0|(reg&7)), n)
}

// emitInt80 emits the `int 0x80` instruction for i386 Linux syscalls
func (g *CodeGen) emitInt80() {
	g.emitBytes(0xcd, 0x80)
}

// === Register-immediate operations (32-bit) ===

// addRI32 emits `add reg, imm` (auto-selects imm8 or imm32)
func (g *CodeGen) addRI32(reg int, val int32) {
	if val >= -128 && val <= 127 {
		g.emitBytes(0x83, byte(0xc0|(reg&7)), byte(val))
	} else {
		if reg == REG32_EAX {
			g.emitByte(0x05)
		} else {
			g.emitBytes(0x81, byte(0xc0|(reg&7)))
		}
		g.emitU32(uint32(val))
	}
}

// subRI32 emits `sub reg, imm` (auto-selects imm8 or imm32)
func (g *CodeGen) subRI32(reg int, val int32) {
	if val >= -128 && val <= 127 {
		g.emitBytes(0x83, byte(0xe8|(reg&7)), byte(val))
	} else {
		g.emitBytes(0x81, byte(0xe8|(reg&7)))
		g.emitU32(uint32(val))
	}
}

// cmpRI32 emits `cmp reg, imm` (auto-selects imm8 or imm32)
func (g *CodeGen) cmpRI32(reg int, val int32) {
	if val >= -128 && val <= 127 {
		g.emitBytes(0x83, byte(0xf8|(reg&7)), byte(val))
	} else {
		g.emitBytes(0x81, byte(0xf8|(reg&7)))
		g.emitU32(uint32(val))
	}
}

// xorRI8_32 emits `xor reg, imm8`
func (g *CodeGen) xorRI8_32(reg int, val byte) {
	g.emitBytes(0x83, byte(0xf0|(reg&7)), val)
}

// imulRRI32_32 emits `imul dst, src, imm32`
func (g *CodeGen) imulRRI32_32(dst, src int, val int32) {
	g.emitBytes(0x69, modrmRR32(dst, src))
	g.emitU32(uint32(val))
}

// === Memory load/store with fixed offsets (32-bit) ===

// loadMem32 emits `mov dst, [base+off]` (32-bit)
func (g *CodeGen) loadMem32(dst, base, off int) {
	if off == 0 && (base&7) != REG32_EBP {
		g.emitBytes(0x8b, byte((dst&7)<<3|(base&7)))
		if (base & 7) == REG32_ESP {
			g.emitByte(0x24) // SIB for ESP-based
		}
	} else if off >= -128 && off <= 127 {
		g.emitBytes(0x8b, byte(0x40|(dst&7)<<3|(base&7)), byte(off))
		if (base & 7) == REG32_ESP {
			g.code = g.code[0 : len(g.code)-2]
			g.emitBytes(byte(0x44|(dst&7)<<3), 0x24, byte(off))
		}
	} else {
		g.emitBytes(0x8b, byte(0x80|(dst&7)<<3|(base&7)))
		if (base & 7) == REG32_ESP {
			g.code = g.code[0 : len(g.code)-1]
			g.emitBytes(byte(0x84|(dst&7)<<3), 0x24)
		}
		g.emitU32(uint32(int32(off)))
	}
}

// storeMem32 emits `mov [base+off], src` (32-bit)
func (g *CodeGen) storeMem32(base, off, src int) {
	if off == 0 && (base&7) != REG32_EBP {
		g.emitBytes(0x89, byte((src&7)<<3|(base&7)))
		if (base & 7) == REG32_ESP {
			g.emitByte(0x24)
		}
	} else if off >= -128 && off <= 127 {
		g.emitBytes(0x89, byte(0x40|(src&7)<<3|(base&7)), byte(off))
		if (base & 7) == REG32_ESP {
			g.code = g.code[0 : len(g.code)-2]
			g.emitBytes(byte(0x44|(src&7)<<3), 0x24, byte(off))
		}
	} else {
		g.emitBytes(0x89, byte(0x80|(src&7)<<3|(base&7)))
		if (base & 7) == REG32_ESP {
			g.code = g.code[0 : len(g.code)-1]
			g.emitBytes(byte(0x84|(src&7)<<3), 0x24)
		}
		g.emitU32(uint32(int32(off)))
	}
}

// loadMemByte32 emits `movzx dst, byte [base+off]`
func (g *CodeGen) loadMemByte32(dst, base, off int) {
	if off == 0 && (base&7) != REG32_EBP {
		g.emitBytes(0x0f, 0xb6, byte((dst&7)<<3|(base&7)))
	} else if off >= -128 && off <= 127 {
		g.emitBytes(0x0f, 0xb6, byte(0x40|(dst&7)<<3|(base&7)), byte(off))
	} else {
		g.emitBytes(0x0f, 0xb6, byte(0x80|(dst&7)<<3|(base&7)))
		g.emitU32(uint32(int32(off)))
	}
}

// storeMemByte32 emits `mov byte [base+off], src_lo8`
func (g *CodeGen) storeMemByte32(base, off, src int) {
	if off == 0 && (base&7) != REG32_EBP {
		g.emitBytes(0x88, byte((src&7)<<3|(base&7)))
	} else if off >= -128 && off <= 127 {
		g.emitBytes(0x88, byte(0x40|(src&7)<<3|(base&7)), byte(off))
	} else {
		g.emitBytes(0x88, byte(0x80|(src&7)<<3|(base&7)))
		g.emitU32(uint32(int32(off)))
	}
}

// === Extend/truncate (32-bit) ===

// movzxB32 emits `movzx reg, reg_lo8`
func (g *CodeGen) movzxB32(reg int) {
	g.emitBytes(0x0f, 0xb6, modrmRR32(reg, reg))
}

// movzxW32 emits `movzx reg, reg_lo16`
func (g *CodeGen) movzxW32(reg int) {
	g.emitBytes(0x0f, 0xb7, modrmRR32(reg, reg))
}

// === Setcc (32-bit) ===

// setcc32 emits `setCC reg_lo8` where cc is a condition code constant
func (g *CodeGen) setcc32(cc byte, reg int) {
	setccOp := byte(0x90 | (cc & 0x0f))
	g.emitBytes(0x0f, setccOp, byte(0xc0|(reg&7)))
}
