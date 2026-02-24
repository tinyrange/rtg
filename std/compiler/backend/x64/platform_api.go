package x64

// Exported platform-facing wrappers. These allow OS-specific backends to live
// in subpackages without re-implementing core codegen internals.

func (g *CodeGen) EmitByte(b byte)                    { g.emitByte(b) }
func (g *CodeGen) EmitBytes(bytes ...byte)            { g.emitBytes(bytes...) }
func (g *CodeGen) EmitU32(v uint32)                   { g.emitU32(v) }
func (g *CodeGen) EmitMovRegImm64(reg int, v uint64)  { g.emitMovRegImm64(reg, v) }
func (g *CodeGen) EmitLoadLocal(offset int, reg int)  { g.emitLoadLocal(offset, reg) }
func (g *CodeGen) EmitStoreLocal(offset int, reg int) { g.emitStoreLocal(offset, reg) }

func (g *CodeGen) EmitCallPlaceholder(target string) { g.emitCallPlaceholder(target) }
func (g *CodeGen) EmitCallIAT(funcName string)       { g.emitCallIAT(funcName) }
func (g *CodeGen) EmitCallIATInLib(libName, funcName string) {
	g.emitCallIATInLib(libName, funcName)
}

func (g *CodeGen) Flush()                               { g.flush() }
func (g *CodeGen) JccRel32(cc byte) int                 { return g.jccRel32(cc) }
func (g *CodeGen) JmpRel32() int                        { return g.jmpRel32() }
func (g *CodeGen) PatchRel32(fixupOff int)              { g.patchRel32(fixupOff) }
func (g *CodeGen) PatchRel32At(fixupOff, targetOff int) { g.patchRel32At(fixupOff, targetOff) }

func (g *CodeGen) AddRI(reg int, imm int32) { g.addRI(reg, imm) }
func (g *CodeGen) SubRI(reg int, imm int32) { g.subRI(reg, imm) }
func (g *CodeGen) AddRR(dst, src int)       { g.addRR(dst, src) }
func (g *CodeGen) CmpRI(reg int, imm int32) { g.cmpRI(reg, imm) }
func (g *CodeGen) CmpRR(a, b int)           { g.cmpRR(a, b) }
func (g *CodeGen) TestRR(a, b int)          { g.testRR(a, b) }
func (g *CodeGen) XorRR(a, b int)           { g.xorRR(a, b) }
func (g *CodeGen) MovRR(dst, src int)       { g.movRR(dst, src) }
func (g *CodeGen) NegR(reg int)             { g.negR(reg) }

func (g *CodeGen) PushR(reg int) { g.pushR(reg) }
func (g *CodeGen) PopR(reg int)  { g.popR(reg) }

func (g *CodeGen) LoadMem(dst, base, offset int) { g.loadMem(dst, base, offset) }
func (g *CodeGen) LoadMemByte(dst, base, offset int) {
	g.loadMemByte(dst, base, offset)
}
func (g *CodeGen) StoreMem(base, offset, src int) { g.storeMem(base, offset, src) }
func (g *CodeGen) StoreMemByte(base, offset, src int) {
	g.storeMemByte(base, offset, src)
}

func (g *CodeGen) OpPop(reg int)  { g.opPop(reg) }
func (g *CodeGen) OpPush(reg int) { g.opPush(reg) }

func (g *CodeGen) CompileConstI64(v int64) { g.compileConstI64(v) }

func (g *CodeGen) ClearOperandCache() { g.clearOperandCache() }

func DecodeIATFixupTarget(target string) (string, string, bool) {
	return decodeIATFixupTarget(target)
}
