package riscv

import (
	"j5.nz/rtg/std/compiler/backend/becommon"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

const (
	REG_ZERO = 0
	REG_RA   = 1
	REG_SP   = 2
	REG_T0   = 5
	REG_T1   = 6
	REG_T2   = 7
	REG_FP   = 8
	REG_S1   = 9
	REG_A0   = 10
	REG_A1   = 11
	REG_A2   = 12
	REG_A3   = 13
	REG_A4   = 14
	REG_A5   = 15
	REG_A6   = 16
	REG_A7   = 17
	REG_T3   = 28
	REG_T4   = 29
	REG_T5   = 30
	REG_T6   = 31
	REG_OPSP = 27
)

type FixupKind int

const (
	FixupCall FixupKind = iota
	FixupFuncAddr
	FixupAddr
	FixupLoad
)

type Fixup struct {
	Kind       FixupKind
	CodeOffset int
	Target     string
	Value      uint64
}

type JumpFixup struct {
	CodeOffset int
	LabelID    int
}

type CodeGen struct {
	target *common.Target
	irmod  *ir.IRModule

	code   []byte
	rodata []byte
	data   []byte

	funcOffsets   map[string]int
	fixups        []Fixup
	labelOffsets  map[int]int
	jumpFixups    []JumpFixup
	stringMap     map[string]int
	stringDataMap map[int]int
	intLitMap     map[int64]int
	globalOffsets []int

	curFunc      *ir.IRFunc
	curFrameSize int
	wordSize     int
	baseAddr     uint64
}

func NewCodeGen(target *common.Target, irmod *ir.IRModule, baseAddr uint64) *CodeGen {
	g := &CodeGen{
		target:        target,
		irmod:         irmod,
		funcOffsets:   make(map[string]int),
		labelOffsets:  make(map[int]int),
		stringMap:     make(map[string]int),
		stringDataMap: make(map[int]int),
		intLitMap:     make(map[int64]int),
		globalOffsets: make([]int, len(irmod.Globals)),
		wordSize:      target.WordSize,
		baseAddr:      baseAddr,
	}
	for i := range irmod.Globals {
		g.globalOffsets[i] = i * g.wordSize
	}
	g.data = make([]byte, len(irmod.Globals)*g.wordSize)
	return g
}

func (g *CodeGen) Target() *common.Target { return g.target }
func (g *CodeGen) BaseAddr() uint64        { return g.baseAddr }
func (g *CodeGen) Code() []byte            { return g.code }
func (g *CodeGen) Rodata() []byte          { return g.rodata }
func (g *CodeGen) Data() []byte            { return g.data }
func (g *CodeGen) FuncOffsets() map[string]int {
	return g.funcOffsets
}

func (g *CodeGen) CollectNativeFuncSizes() {
	ir.CollectNativeFuncSizes(g.irmod, g.funcOffsets, len(g.code))
}

func (g *CodeGen) alignRodata(align int) int {
	off := common.AlignUp(len(g.rodata), align)
	for len(g.rodata) < off {
		g.rodata = append(g.rodata, 0)
	}
	return off
}

func (g *CodeGen) emit32(v uint32) {
	g.code = append(g.code, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}

func (g *CodeGen) patch32(off int, v uint32) {
	g.code[off] = byte(v)
	g.code[off+1] = byte(v >> 8)
	g.code[off+2] = byte(v >> 16)
	g.code[off+3] = byte(v >> 24)
}

func encR(f7 uint32, rs2 int, rs1 int, f3 uint32, rd int, opcode uint32) uint32 {
	return (f7 << 25) | (uint32(rs2&31) << 20) | (uint32(rs1&31) << 15) | (f3 << 12) | (uint32(rd&31) << 7) | opcode
}

func encI(imm int32, rs1 int, f3 uint32, rd int, opcode uint32) uint32 {
	return (uint32(imm)&0xfff)<<20 | (uint32(rs1&31) << 15) | (f3 << 12) | (uint32(rd&31) << 7) | opcode
}

func encS(imm int32, rs2 int, rs1 int, f3 uint32, opcode uint32) uint32 {
	u := uint32(imm) & 0xfff
	return ((u >> 5) << 25) | (uint32(rs2&31) << 20) | (uint32(rs1&31) << 15) | (f3 << 12) | ((u & 0x1f) << 7) | opcode
}

func encB(imm int32, rs2 int, rs1 int, f3 uint32, opcode uint32) uint32 {
	u := uint32(imm) & 0x1fff
	return (((u >> 12) & 1) << 31) | (((u >> 5) & 0x3f) << 25) | (uint32(rs2&31) << 20) | (uint32(rs1&31) << 15) | (f3 << 12) | (((u >> 1) & 0xf) << 8) | (((u >> 11) & 1) << 7) | opcode
}

func encU(imm20 int32, rd int, opcode uint32) uint32 {
	return (uint32(imm20) & 0xfffff << 12) | (uint32(rd&31) << 7) | opcode
}

func encJ(imm int32, rd int, opcode uint32) uint32 {
	u := uint32(imm) & 0x1fffff
	return (((u >> 20) & 1) << 31) | (((u >> 1) & 0x3ff) << 21) | (((u >> 11) & 1) << 20) | (((u >> 12) & 0xff) << 12) | (uint32(rd&31) << 7) | opcode
}

func (g *CodeGen) emitAddi(rd, rs1 int, imm int32)  { g.emit32(encI(imm, rs1, 0, rd, 0x13)) }
func (g *CodeGen) emitXori(rd, rs1 int, imm int32)  { g.emit32(encI(imm, rs1, 4, rd, 0x13)) }
func (g *CodeGen) emitSltiu(rd, rs1 int, imm int32) { g.emit32(encI(imm, rs1, 3, rd, 0x13)) }
func (g *CodeGen) emitAndi(rd, rs1 int, imm int32)  { g.emit32(encI(imm, rs1, 7, rd, 0x13)) }
func (g *CodeGen) emitSlli(rd, rs1 int, shamt int32) {
	g.emit32(encI(shamt, rs1, 1, rd, 0x13))
}
func (g *CodeGen) emitSrli(rd, rs1 int, shamt int32) {
	g.emit32(encI(shamt, rs1, 5, rd, 0x13))
}
func (g *CodeGen) emitSrai(rd, rs1 int, shamt int32) {
	g.emit32(encI(shamt|0x400, rs1, 5, rd, 0x13))
}
func (g *CodeGen) emitAddiw(rd, rs1 int, imm int32) {
	g.emit32(encI(imm, rs1, 0, rd, 0x1b))
}

func (g *CodeGen) emitAdd(rd, rs1, rs2 int) { g.emit32(encR(0x00, rs2, rs1, 0, rd, 0x33)) }
func (g *CodeGen) emitSub(rd, rs1, rs2 int) { g.emit32(encR(0x20, rs2, rs1, 0, rd, 0x33)) }
func (g *CodeGen) emitMul(rd, rs1, rs2 int) { g.emit32(encR(0x01, rs2, rs1, 0, rd, 0x33)) }
func (g *CodeGen) emitDiv(rd, rs1, rs2 int) { g.emit32(encR(0x01, rs2, rs1, 4, rd, 0x33)) }
func (g *CodeGen) emitRem(rd, rs1, rs2 int) { g.emit32(encR(0x01, rs2, rs1, 6, rd, 0x33)) }
func (g *CodeGen) emitAnd(rd, rs1, rs2 int) { g.emit32(encR(0x00, rs2, rs1, 7, rd, 0x33)) }
func (g *CodeGen) emitOr(rd, rs1, rs2 int)  { g.emit32(encR(0x00, rs2, rs1, 6, rd, 0x33)) }
func (g *CodeGen) emitXor(rd, rs1, rs2 int) { g.emit32(encR(0x00, rs2, rs1, 4, rd, 0x33)) }
func (g *CodeGen) emitSll(rd, rs1, rs2 int) { g.emit32(encR(0x00, rs2, rs1, 1, rd, 0x33)) }
func (g *CodeGen) emitSra(rd, rs1, rs2 int) { g.emit32(encR(0x20, rs2, rs1, 5, rd, 0x33)) }
func (g *CodeGen) emitSlt(rd, rs1, rs2 int) { g.emit32(encR(0x00, rs2, rs1, 2, rd, 0x33)) }
func (g *CodeGen) emitSltu(rd, rs1, rs2 int) { g.emit32(encR(0x00, rs2, rs1, 3, rd, 0x33)) }

func (g *CodeGen) emitLoadWord(rd, rs1 int, imm int32) {
	if g.wordSize == 8 {
		g.emit32(encI(imm, rs1, 3, rd, 0x03))
	} else {
		g.emit32(encI(imm, rs1, 2, rd, 0x03))
	}
}

func (g *CodeGen) emitStoreWord(rs2, rs1 int, imm int32) {
	if g.wordSize == 8 {
		g.emit32(encS(imm, rs2, rs1, 3, 0x23))
	} else {
		g.emit32(encS(imm, rs2, rs1, 2, 0x23))
	}
}

func (g *CodeGen) emitLoadByteU(rd, rs1 int, imm int32) { g.emit32(encI(imm, rs1, 4, rd, 0x03)) }
func (g *CodeGen) emitStoreByte(rs2, rs1 int, imm int32) { g.emit32(encS(imm, rs2, rs1, 0, 0x23)) }
func (g *CodeGen) emitAuipc(rd int, imm20 int32)         { g.emit32((uint32(imm20)&0xfffff)<<12 | (uint32(rd&31) << 7) | 0x17) }
func (g *CodeGen) emitLui(rd int, imm20 int32)           { g.emit32((uint32(imm20)&0xfffff)<<12 | (uint32(rd&31) << 7) | 0x37) }
func (g *CodeGen) emitJal(rd int, imm int32) int {
	off := len(g.code)
	g.emit32(encJ(imm, rd, 0x6f))
	return off
}
func (g *CodeGen) emitJalr(rd, rs1 int, imm int32) { g.emit32(encI(imm, rs1, 0, rd, 0x67)) }
func (g *CodeGen) emitBeq(rs1, rs2 int, imm int32) { g.emit32(encB(imm, rs2, rs1, 0, 0x63)) }
func (g *CodeGen) emitBne(rs1, rs2 int, imm int32) { g.emit32(encB(imm, rs2, rs1, 1, 0x63)) }
func (g *CodeGen) emitEcall()                      { g.emit32(0x00000073) }

func (g *CodeGen) emitAddrFixup(rd int, target string, value uint64) {
	off := len(g.code)
	g.emitAuipc(rd, 0)
	g.emitAddi(rd, rd, 0)
	g.fixups = append(g.fixups, Fixup{Kind: FixupAddr, CodeOffset: off, Target: target, Value: value})
}

func (g *CodeGen) emitLoadFixup(rd int, target string, value uint64) {
	off := len(g.code)
	g.emitAuipc(rd, 0)
	g.emitLoadWord(rd, rd, 0)
	g.fixups = append(g.fixups, Fixup{Kind: FixupLoad, CodeOffset: off, Target: target, Value: value})
}

func (g *CodeGen) emitCallPlaceholder(name string) {
	off := len(g.code)
	g.emitAuipc(REG_T0, 0)
	g.emitJalr(REG_RA, REG_T0, 0)
	g.fixups = append(g.fixups, Fixup{Kind: FixupCall, CodeOffset: off, Target: name})
}

func (g *CodeGen) emitFuncAddrPlaceholder(name string) {
	off := len(g.code)
	g.emitAuipc(REG_T0, 0)
	g.emitAddi(REG_T0, REG_T0, 0)
	g.fixups = append(g.fixups, Fixup{Kind: FixupFuncAddr, CodeOffset: off, Target: name})
	g.rawPush(REG_T0)
}

func (g *CodeGen) rawPush(reg int) {
	g.emitAddi(REG_OPSP, REG_OPSP, int32(-g.wordSize))
	g.emitStoreWord(reg, REG_OPSP, 0)
}

func (g *CodeGen) rawPop(reg int) {
	g.emitLoadWord(reg, REG_OPSP, 0)
	g.emitAddi(REG_OPSP, REG_OPSP, int32(g.wordSize))
}

func (g *CodeGen) rawLoad(reg int) {
	g.emitLoadWord(reg, REG_OPSP, 0)
}

func (g *CodeGen) rawStore(reg int) {
	g.emitStoreWord(reg, REG_OPSP, 0)
}

func (g *CodeGen) rawDrop() {
	g.emitAddi(REG_OPSP, REG_OPSP, int32(g.wordSize))
}

func (g *CodeGen) rawDropN(n int) {
	if n <= 0 {
		return
	}
	g.emitAddImmConst(REG_OPSP, REG_OPSP, int64(n*g.wordSize), REG_T6)
}

func (g *CodeGen) emitImmToReg(rd int, val int64) {
	if val >= -2048 && val <= 2047 {
		g.emitAddi(rd, REG_ZERO, int32(val))
		return
	}
	off, ok := g.intLitMap[val]
	if !ok {
		off = g.alignRodata(g.wordSize)
		if g.wordSize == 8 {
			g.rodata = append(g.rodata,
				byte(val), byte(val>>8), byte(val>>16), byte(val>>24),
				byte(val>>32), byte(val>>40), byte(val>>48), byte(val>>56),
			)
		} else {
			v := uint32(val)
			g.rodata = append(g.rodata, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
		}
		g.intLitMap[val] = off
	}
	g.emitLoadFixup(rd, "$rodata$", uint64(off))
}

func (g *CodeGen) emitAddImmConst(rd, rs int, val int64, scratch int) {
	if val >= -2048 && val <= 2047 {
		g.emitAddi(rd, rs, int32(val))
		return
	}
	g.emitImmToReg(scratch, val)
	g.emitAdd(rd, rs, scratch)
}

func (g *CodeGen) emitSubImmConst(rd, rs int, val int64, scratch int) {
	g.emitAddImmConst(rd, rs, -val, scratch)
}

func (g *CodeGen) emitLoadAt(rd, base int, off int) {
	if off >= -2048 && off <= 2047 {
		g.emitLoadWord(rd, base, int32(off))
		return
	}
	g.emitImmToReg(REG_T6, int64(off))
	g.emitAdd(REG_T6, base, REG_T6)
	g.emitLoadWord(rd, REG_T6, 0)
}

func (g *CodeGen) emitStoreAt(rs, base int, off int) {
	if off >= -2048 && off <= 2047 {
		g.emitStoreWord(rs, base, int32(off))
		return
	}
	g.emitImmToReg(REG_T6, int64(off))
	g.emitAdd(REG_T6, base, REG_T6)
	g.emitStoreWord(rs, REG_T6, 0)
}

func (g *CodeGen) emitLoadByteAt(rd, base int, off int) {
	if off >= -2048 && off <= 2047 {
		g.emitLoadByteU(rd, base, int32(off))
		return
	}
	g.emitImmToReg(REG_T6, int64(off))
	g.emitAdd(REG_T6, base, REG_T6)
	g.emitLoadByteU(rd, REG_T6, 0)
}

func (g *CodeGen) emitStoreByteAt(rs, base int, off int) {
	if off >= -2048 && off <= 2047 {
		g.emitStoreByte(rs, base, int32(off))
		return
	}
	g.emitImmToReg(REG_T6, int64(off))
	g.emitAdd(REG_T6, base, REG_T6)
	g.emitStoreByte(rs, REG_T6, 0)
}

func (g *CodeGen) localOffset(idx int) int {
	return -((idx + 1) * g.wordSize)
}

func (g *CodeGen) emitLoadLocal(idx int, reg int) {
	g.emitLoadAt(reg, REG_FP, g.localOffset(idx))
}

func (g *CodeGen) emitStoreLocal(idx int, reg int) {
	g.emitStoreAt(reg, REG_FP, g.localOffset(idx))
}

func (g *CodeGen) emitLocalAddr(idx int, reg int) {
	off := g.localOffset(idx)
	g.emitAddImmConst(reg, REG_FP, int64(off), REG_T6)
}

func (g *CodeGen) internString(decoded string) int {
	headerOff, ok := g.stringMap[decoded]
	if ok {
		return headerOff
	}
	dataOff := g.alignRodata(1)
	g.rodata = append(g.rodata, []byte(decoded)...)
	headerOff = g.alignRodata(g.wordSize)
	if g.wordSize == 8 {
		g.rodata = append(g.rodata, make([]byte, 16)...)
		common.PutU64(g.rodata[headerOff+8:headerOff+16], uint64(len(decoded)))
	} else {
		g.rodata = append(g.rodata, make([]byte, 8)...)
		common.PutU32(g.rodata[headerOff+4:headerOff+8], uint32(len(decoded)))
	}
	g.stringMap[decoded] = headerOff
	g.stringDataMap[headerOff] = dataOff
	return headerOff
}

func (g *CodeGen) ResolveFunctionFixups() []string {
	var unresolved []string
	for _, fx := range g.fixups {
		if fx.Kind != FixupCall && fx.Kind != FixupFuncAddr {
			continue
		}
		target := fx.Target
		if len(target) > 10 && target[0:10] == "$funcaddr$" {
			target = target[10:]
		}
		targetOff, ok := g.funcOffsets[target]
		if !ok {
			unresolved = append(unresolved, fx.Target)
			continue
		}
		g.patchPairPCRel(fx.CodeOffset, int64(targetOff))
	}
	return unresolved
}

func (g *CodeGen) patchSectionFixups(textVAddr, rodataVAddr, dataVAddr uint64) {
	for _, headerOff := range g.stringMap {
		dataOff := g.stringDataMap[headerOff]
		if g.wordSize == 8 {
			common.PutU64(g.rodata[headerOff:headerOff+8], rodataVAddr+uint64(dataOff))
		} else {
			common.PutU32(g.rodata[headerOff:headerOff+4], uint32(rodataVAddr)+uint32(dataOff))
		}
	}
	for _, fx := range g.fixups {
		if fx.Kind != FixupAddr && fx.Kind != FixupLoad {
			continue
		}
		var targetAddr uint64
		if fx.Target == "$rodata$" {
			targetAddr = rodataVAddr + fx.Value
		} else if fx.Target == "$data$" {
			targetAddr = dataVAddr + fx.Value
		} else {
			continue
		}
		g.patchPairAbs(textVAddr, fx.CodeOffset, targetAddr)
	}
}

func (g *CodeGen) patchPairPCRel(off int, targetOff int64) {
	pc := int64(off)
	delta := targetOff - pc
	hi := int32((delta + 0x800) >> 12)
	lo := int32(delta - int64(hi<<12))
	insn0 := common.GetU32(g.code[off : off+4])
	insn1 := common.GetU32(g.code[off+4 : off+8])
	insn0 = (insn0 & 0xfff) | (uint32(hi&0xfffff) << 12)
	insn1 = (insn1 & 0x000fffff) | ((uint32(lo) & 0xfff) << 20)
	g.patch32(off, insn0)
	g.patch32(off+4, insn1)
}

func (g *CodeGen) patchPairAbs(textVAddr uint64, off int, targetAddr uint64) {
	textAddr := textVAddr + uint64(off)
	delta := int64(targetAddr) - int64(textAddr)
	hi := int32((delta + 0x800) >> 12)
	lo := int32(delta - int64(hi<<12))
	insn0 := common.GetU32(g.code[off : off+4])
	insn1 := common.GetU32(g.code[off+4 : off+8])
	insn0 = (insn0 & 0xfff) | (uint32(hi&0xfffff) << 12)
	insn1 = (insn1 & 0x000fffff) | ((uint32(lo) & 0xfff) << 20)
	g.patch32(off, insn0)
	g.patch32(off+4, insn1)
}

func (g *CodeGen) patchJalAt(off int, targetOff int) {
	rel := int32(targetOff - off)
	insn := common.GetU32(g.code[off : off+4])
	insn = (insn & 0xfff) | (encJ(rel, int((insn>>7)&31), 0x6f) &^ 0xfff)
	g.patch32(off, insn)
}

func (g *CodeGen) CompileModuleFuncs() {
	for _, f := range g.irmod.Funcs {
		g.funcOffsets[f.Name] = len(g.code)
		g.compileFunc(f)
	}
}

func (g *CodeGen) EmitImmToReg(rd int, val int64)        { g.emitImmToReg(rd, val) }
func (g *CodeGen) EmitEcall()                            { g.emitEcall() }
func (g *CodeGen) EmitAdd(rd, rs1, rs2 int)              { g.emitAdd(rd, rs1, rs2) }
func (g *CodeGen) EmitCallPlaceholder(name string)       { g.emitCallPlaceholder(name) }
func (g *CodeGen) PatchSectionFixups(textV, rodataV, dataV uint64) {
	g.patchSectionFixups(textV, rodataV, dataV)
}

func sortedDispatchEntries(irmod *ir.IRModule, methodSuffix string) []becommon.DispatchEntry {
	var entries []becommon.DispatchEntry
	if irmod == nil || irmod.TypeIDs == nil {
		return entries
	}
	for typeName, tid := range irmod.TypeIDs {
		cand := typeName + methodSuffix
		if _, ok := irmod.MethodTable[cand]; ok {
			entries = append(entries, becommon.DispatchEntry{TypeID: tid, FuncName: cand})
		}
	}
	for i := 1; i < len(entries); i++ {
		j := i
		for j > 0 && (entries[j].TypeID < entries[j-1].TypeID || (entries[j].TypeID == entries[j-1].TypeID && entries[j].FuncName < entries[j-1].FuncName)) {
			entries[j], entries[j-1] = entries[j-1], entries[j]
			j--
		}
	}
	return entries
}
