//go:build !no_backend_linux_i386 || !no_backend_windows_i386

package i386

import (
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

func NewCodeGen(target *common.Target, irmod *ir.IRModule, baseAddr uint64) *CodeGen {
	g := &CodeGen{
		Target:        target,
		FuncOffsets:   make(map[string]int),
		LabelOffsets:  make(map[int]int),
		StringMap:     make(map[string]int),
		GlobalOffsets: make([]int, len(irmod.Globals)),
		BaseAddr:      baseAddr,
		IRMod:         irmod,
		WordSize:      4,
	}
	slot := g.slotBytes_i386()
	for i := range irmod.Globals {
		g.GlobalOffsets[i] = i * slot
	}
	g.Data = make([]byte, len(irmod.Globals)*slot)
	return g
}

func (g *CodeGen) CompileModuleFuncs(irmod *ir.IRModule) {
	for _, f := range irmod.Funcs {
		g.FuncOffsets[f.Name] = len(g.Code)
		g.compileFunc_i386(f)
	}
}

func (g *CodeGen) CollectNativeFuncSizes(irmod *ir.IRModule) {
	ir.CollectNativeFuncSizes(irmod, g.FuncOffsets, len(g.Code))
}

func (g *CodeGen) SlotBytesI386() int                        { return g.slotBytes_i386() }
func (g *CodeGen) CompileFuncI386(f *ir.IRFunc)              { g.compileFunc_i386(f) }
func (g *CodeGen) CompileConstI32(val int64)                 { g.compileConstI32(val) }
func (g *CodeGen) EmitByte(b byte)                           { g.emitByte(b) }
func (g *CodeGen) EmitBytes(bytes ...byte)                   { g.emitBytes(bytes...) }
func (g *CodeGen) EmitU32(v uint32)                          { g.emitU32(v) }
func (g *CodeGen) EmitMovRegImm32(reg int, val uint32)       { g.emitMovRegImm32(reg, val) }
func (g *CodeGen) EmitLoadLocal32(offset int, reg int)       { g.emitLoadLocal32(offset, reg) }
func (g *CodeGen) EmitStoreLocal32(offset int, reg int)      { g.emitStoreLocal32(offset, reg) }
func (g *CodeGen) PushR32(reg int)                           { g.pushR32(reg) }
func (g *CodeGen) PopR32(reg int)                            { g.popR32(reg) }
func (g *CodeGen) MovRR32(dst, src int)                      { g.movRR32(dst, src) }
func (g *CodeGen) AddRI32(reg int, val int32)                { g.addRI32(reg, val) }
func (g *CodeGen) SubRI32(reg int, val int32)                { g.subRI32(reg, val) }
func (g *CodeGen) CmpRI32(reg int, val int32)                { g.cmpRI32(reg, val) }
func (g *CodeGen) XorRR32(dst, src int)                      { g.xorRR32(dst, src) }
func (g *CodeGen) TestRR32(a, b int)                         { g.testRR32(a, b) }
func (g *CodeGen) NegR32(reg int)                            { g.negR32(reg) }
func (g *CodeGen) LoadMem32(dst, base, off int)              { g.loadMem32(dst, base, off) }
func (g *CodeGen) StoreMem32(base, off, src int)             { g.storeMem32(base, off, src) }
func (g *CodeGen) OpPush(reg int)                            { g.opPush(reg) }
func (g *CodeGen) OpPop(reg int)                             { g.opPop(reg) }
func (g *CodeGen) EmitInt80()                                { g.emitInt80() }
func (g *CodeGen) EmitCallPlaceholder(target string)         { g.emitCallPlaceholder(target) }
func (g *CodeGen) EmitCallIAT(funcName string)               { g.emitCallIAT(funcName) }
func (g *CodeGen) EmitCallIATInLib(libName, funcName string) { g.emitCallIATInLib(libName, funcName) }
func (g *CodeGen) JccRel32(cc byte) int                      { return g.jccRel32(cc) }
func (g *CodeGen) JmpRel32() int                             { return g.jmpRel32() }
func (g *CodeGen) JmpRel8(off int8)                          { g.jmpRel8(off) }
func (g *CodeGen) PatchRel32(fixupOff int)                   { g.patchRel32(fixupOff) }
func (g *CodeGen) PatchRel32At(fixupOff int, targetOff int)  { g.patchRel32At(fixupOff, targetOff) }
func (g *CodeGen) Flush()                                    { g.flush() }
func (g *CodeGen) ClearOperandCache()                        { g.clearOperandCache() }
func (g *CodeGen) EmitTostringHelperI386()                   { g.emitTostringHelperI386() }
