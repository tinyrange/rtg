package armv8m

type Assembler struct {
	code []byte
}

func NewAssembler() *Assembler {
	return &Assembler{}
}

//rtg:profile
func (a *Assembler) Code() []byte {
	return a.code
}

//rtg:profile
func (a *Assembler) Pos() int {
	return len(a.code)
}

//rtg:profile
func (a *Assembler) Emit16(v uint16) {
	a.code = append(a.code, byte(v), byte(v>>8))
}

//rtg:profile
func (a *Assembler) Emit32(v uint32) {
	a.code = append(a.code, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}

//rtg:profile
func (a *Assembler) Patch16(off int, v uint16) {
	a.code[off] = byte(v)
	a.code[off+1] = byte(v >> 8)
}

//rtg:profile
func (a *Assembler) Patch32(off int, v uint32) {
	a.code[off] = byte(v)
	a.code[off+1] = byte(v >> 8)
	a.code[off+2] = byte(v >> 16)
	a.code[off+3] = byte(v >> 24)
}

//rtg:profile
func (a *Assembler) EmitNop() {
	a.Emit16(0xBF00)
}

//rtg:profile
func (a *Assembler) EmitMovsImm(reg uint8, imm uint8) {
	if reg > 7 {
		panic("armv8m: MOVS immediate supports only r0-r7")
	}
	a.Emit16(0x2000 | uint16(reg)<<8 | uint16(imm))
}

//rtg:profile
func (a *Assembler) EmitAddsImm(reg uint8, imm uint8) {
	if reg > 7 {
		panic("armv8m: ADDS immediate supports only r0-r7")
	}
	a.Emit16(0x3000 | uint16(reg)<<8 | uint16(imm))
}

//rtg:profile
func (a *Assembler) EmitSubsImm(reg uint8, imm uint8) {
	if reg > 7 {
		panic("armv8m: SUBS immediate supports only r0-r7")
	}
	a.Emit16(0x3800 | uint16(reg)<<8 | uint16(imm))
}

//rtg:profile
func (a *Assembler) EmitAddRRR(rd uint8, rn uint8, rm uint8) {
	if rd > 7 || rn > 7 || rm > 7 {
		panic("armv8m: ADD register supports only r0-r7")
	}
	a.Emit16(0x1800 | uint16(rm)<<6 | uint16(rn)<<3 | uint16(rd))
}

//rtg:profile
func (a *Assembler) EmitSubRRR(rd uint8, rn uint8, rm uint8) {
	if rd > 7 || rn > 7 || rm > 7 {
		panic("armv8m: SUB register supports only r0-r7")
	}
	a.Emit16(0x1A00 | uint16(rm)<<6 | uint16(rn)<<3 | uint16(rd))
}

//rtg:profile
func (a *Assembler) EmitMulRR(rd uint8, rm uint8) {
	if rd > 7 || rm > 7 {
		panic("armv8m: MUL supports only r0-r7")
	}
	a.Emit16(0x4340 | uint16(rm)<<3 | uint16(rd))
}

//rtg:profile
func (a *Assembler) EmitAndRR(rd uint8, rm uint8) {
	if rd > 7 || rm > 7 {
		panic("armv8m: AND supports only r0-r7")
	}
	a.Emit16(0x4000 | uint16(rm)<<3 | uint16(rd))
}

//rtg:profile
func (a *Assembler) EmitEorRR(rd uint8, rm uint8) {
	if rd > 7 || rm > 7 {
		panic("armv8m: EOR supports only r0-r7")
	}
	a.Emit16(0x4040 | uint16(rm)<<3 | uint16(rd))
}

//rtg:profile
func (a *Assembler) EmitOrrRR(rd uint8, rm uint8) {
	if rd > 7 || rm > 7 {
		panic("armv8m: ORR supports only r0-r7")
	}
	a.Emit16(0x4300 | uint16(rm)<<3 | uint16(rd))
}

//rtg:profile
func (a *Assembler) EmitLslRR(rd uint8, rm uint8) {
	if rd > 7 || rm > 7 {
		panic("armv8m: LSL supports only r0-r7")
	}
	a.Emit16(0x4080 | uint16(rm)<<3 | uint16(rd))
}

//rtg:profile
func (a *Assembler) EmitLsrRR(rd uint8, rm uint8) {
	if rd > 7 || rm > 7 {
		panic("armv8m: LSR supports only r0-r7")
	}
	a.Emit16(0x40C0 | uint16(rm)<<3 | uint16(rd))
}

//rtg:profile
func (a *Assembler) EmitNegRR(rd uint8, rm uint8) {
	if rd > 7 || rm > 7 {
		panic("armv8m: NEG supports only r0-r7")
	}
	a.Emit16(0x4240 | uint16(rm)<<3 | uint16(rd))
}

//rtg:profile
func (a *Assembler) EmitMvnRR(rd uint8, rm uint8) {
	if rd > 7 || rm > 7 {
		panic("armv8m: MVN supports only r0-r7")
	}
	a.Emit16(0x43C0 | uint16(rm)<<3 | uint16(rd))
}

//rtg:profile
func (a *Assembler) EmitCmpRR(rn uint8, rm uint8) {
	if rn > 7 || rm > 7 {
		panic("armv8m: CMP register supports only r0-r7")
	}
	a.Emit16(0x4280 | uint16(rm)<<3 | uint16(rn))
}

//rtg:profile
func (a *Assembler) EmitCmpImm(rn uint8, imm uint8) {
	if rn > 7 {
		panic("armv8m: CMP immediate supports only r0-r7")
	}
	a.Emit16(0x2800 | uint16(rn)<<8 | uint16(imm))
}

//rtg:profile
func (a *Assembler) EmitMovReg(dst uint8, src uint8) {
	// Thumb MOV(register): 010001 10 D Rm Rdn
	a.Emit16(0x4600 | uint16(dst&8)<<4 | uint16(src)<<3 | uint16(dst&7))
}

//rtg:profile
func (a *Assembler) EmitLdrImm(rt uint8, rn uint8, immWords uint8) {
	if rt > 7 || rn > 7 || immWords > 31 {
		panic("armv8m: LDR imm out of range")
	}
	a.Emit16(0x6800 | uint16(immWords)<<6 | uint16(rn)<<3 | uint16(rt))
}

//rtg:profile
func (a *Assembler) EmitStrImm(rt uint8, rn uint8, immWords uint8) {
	if rt > 7 || rn > 7 || immWords > 31 {
		panic("armv8m: STR imm out of range")
	}
	a.Emit16(0x6000 | uint16(immWords)<<6 | uint16(rn)<<3 | uint16(rt))
}

//rtg:profile
func (a *Assembler) EmitLdrbImm(rt uint8, rn uint8, imm uint8) {
	if rt > 7 || rn > 7 || imm > 31 {
		panic("armv8m: LDRB imm out of range")
	}
	a.Emit16(0x7800 | uint16(imm)<<6 | uint16(rn)<<3 | uint16(rt))
}

//rtg:profile
func (a *Assembler) EmitStrbImm(rt uint8, rn uint8, imm uint8) {
	if rt > 7 || rn > 7 || imm > 31 {
		panic("armv8m: STRB imm out of range")
	}
	a.Emit16(0x7000 | uint16(imm)<<6 | uint16(rn)<<3 | uint16(rt))
}

//rtg:profile
func (a *Assembler) EmitPush(regList uint8, withLR bool) {
	v := uint16(0xB400 | uint16(regList))
	if withLR {
		v = v | 0x0100
	}
	a.Emit16(v)
}

//rtg:profile
func (a *Assembler) EmitPop(regList uint8, withPC bool) {
	v := uint16(0xBC00 | uint16(regList))
	if withPC {
		v = v | 0x0100
	}
	a.Emit16(v)
}

//rtg:profile
func (a *Assembler) EmitBImm11(imm11 uint16) int {
	off := a.Pos()
	a.Emit16(0xE000 | (imm11 & 0x07FF))
	return off
}

//rtg:profile
func (a *Assembler) EmitBCond(cond uint8, imm8 uint8) int {
	off := a.Pos()
	a.Emit16(0xD000 | uint16(cond)<<8 | uint16(imm8))
	return off
}

//rtg:profile
func (a *Assembler) EmitBLPlaceholder() int {
	off := a.Pos()
	a.Emit16(0xF000)
	a.Emit16(0xF800)
	return off
}

//rtg:profile
func (a *Assembler) PatchBL(off int, rel int32) {
	// rel is byte offset from instruction address + 4.
	// Thumb BL immediate uses signed 25-bit (low bit always zero).
	if (rel & 1) != 0 {
		panic("armv8m: BL rel must be halfword-aligned")
	}
	imm := rel >> 1
	if imm < -(1<<24) || imm >= (1<<24) {
		panic("armv8m: BL target out of range")
	}
	u := uint32(imm) & 0x01FFFFFF
	s := (u >> 24) & 1
	i1 := (u >> 23) & 1
	i2 := (u >> 22) & 1
	imm10 := (u >> 11) & 0x03FF
	imm11 := u & 0x07FF
	j1 := (^i1 ^ s) & 1
	j2 := (^i2 ^ s) & 1
	h1 := uint16(0xF000 | (s << 10) | imm10)
	h2 := uint16(0xF800 | (j1 << 13) | (j2 << 11) | imm11)
	a.Patch16(off, h1)
	a.Patch16(off+2, h2)
}

//rtg:profile
func (a *Assembler) PatchBImm11(off int, rel int32) {
	// rel is byte offset from instruction address + 4.
	if (rel & 1) != 0 {
		panic("armv8m: B rel must be halfword-aligned")
	}
	imm := rel >> 1
	if imm < -1024 || imm > 1023 {
		panic("armv8m: B target out of range")
	}
	a.Patch16(off, 0xE000|uint16(int16(imm)&0x07FF))
}

//rtg:profile
func (a *Assembler) PatchBCond(off int, cond uint8, rel int32) {
	// rel is byte offset from instruction address + 4.
	if (rel & 1) != 0 {
		panic("armv8m: B.cond rel must be halfword-aligned")
	}
	imm := rel >> 1
	if imm < -128 || imm > 127 {
		panic("armv8m: B.cond target out of range")
	}
	a.Patch16(off, 0xD000|uint16(cond)<<8|uint16(int8(imm)))
}

//rtg:profile
func (a *Assembler) EmitBkpt(v uint8) {
	a.Emit16(0xBE00 | uint16(v))
}

//rtg:profile
func (a *Assembler) EmitLdrLiteral(rt uint8, immWords uint8) int {
	if rt > 7 {
		panic("armv8m: LDR literal supports only r0-r7")
	}
	off := a.Pos()
	a.Emit16(0x4800 | uint16(rt)<<8 | uint16(immWords))
	return off
}

//rtg:profile
func (a *Assembler) EmitBSelf() {
	a.Emit16(0xE7FE)
}
