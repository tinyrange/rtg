//go:build !no_backend_i386

package i386

import (
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// NewCodeGen creates a shared i386 code generator configured for a target/output format.
func NewCodeGen(target *common.Target, irmod *ir.IRModule, baseAddr uint64, wordSize int) *CodeGen {
	return &CodeGen{
		target:        target,
		funcOffsets:   make(map[string]int),
		labelOffsets:  make(map[int]int),
		stringMap:     make(map[string]int),
		globalOffsets: make([]int, len(irmod.Globals)),
		baseAddr:      baseAddr,
		irmod:         irmod,
		wordSize:      wordSize,
	}
}

// InitGlobals allocates global slots in .data using the provided slot size.
func (g *CodeGen) InitGlobals(slotBytes int, globalCount int) {
	for i := 0; i < globalCount; i++ {
		g.globalOffsets[i] = i * slotBytes
	}
	g.data = make([]byte, globalCount*slotBytes)
}

// CompileModuleFuncsI386 compiles all IR functions into machine code.
func (g *CodeGen) CompileModuleFuncsI386(irmod *ir.IRModule) {
	for _, f := range irmod.Funcs {
		g.funcOffsets[f.Name] = len(g.code)
		g.compileFunc_i386(f)
	}
}

// CollectNativeFuncSizes records native function sizes for diagnostics/debug info.
func (g *CodeGen) CollectNativeFuncSizes(irmod *ir.IRModule) {
	ir.CollectNativeFuncSizes(irmod, g.funcOffsets, len(g.code))
}

func (g *CodeGen) NeedTostringHelper() bool {
	return g.needTostringHelper
}

func (g *CodeGen) EmitTostringHelperI386Shared() {
	g.emitTostringHelperI386()
}

// ResolveCallFixupsI386 patches call fixups and returns unresolved symbol names.
// If skipIAT is true, $iat$ fixups are ignored.
func (g *CodeGen) ResolveCallFixupsI386(skipIAT bool) []string {
	var unresolved []string
	for _, fix := range g.callFixups {
		if fix.Target == "$rodata_header$" || fix.Target == "$data_addr$" {
			continue
		}
		if skipIAT && len(fix.Target) > 5 && fix.Target[0:5] == "$iat$" {
			continue
		}
		target, ok := g.funcOffsets[fix.Target]
		if !ok {
			unresolved = append(unresolved, fix.Target)
			continue
		}
		g.patchRel32At(fix.CodeOffset, target)
	}
	return unresolved
}

func (g *CodeGen) BuildELF32Binary(irmod *ir.IRModule) []byte {
	return g.buildELF32(irmod)
}

func (g *CodeGen) CodeLen() int {
	return len(g.code)
}

func (g *CodeGen) TargetGOOS() string {
	return g.target.GOOS
}

func (g *CodeGen) LookupLinkStaticSpec(name string) (string, bool) {
	if g.irmod == nil || g.irmod.LinkStaticFuncs == nil {
		return "", false
	}
	raw, ok := g.irmod.LinkStaticFuncs[name]
	return raw, ok
}

// Exported assembler/codegen operation wrappers for OS subpackages.
func (g *CodeGen) EmitByte(b byte)                      { g.emitByte(b) }
func (g *CodeGen) EmitBytes(bytes ...byte)              { g.emitBytes(bytes...) }
func (g *CodeGen) EmitU32(v uint32)                     { g.emitU32(v) }
func (g *CodeGen) EmitCallPlaceholder(target string)    { g.emitCallPlaceholder(target) }
func (g *CodeGen) EmitLoadLocal32(offset int, reg int)  { g.emitLoadLocal32(offset, reg) }
func (g *CodeGen) EmitMovRegImm32(reg int, val uint32)  { g.emitMovRegImm32(reg, val) }
func (g *CodeGen) PushR32(reg int)                      { g.pushR32(reg) }
func (g *CodeGen) PopR32(reg int)                       { g.popR32(reg) }
func (g *CodeGen) MovRR32(dst, src int)                 { g.movRR32(dst, src) }
func (g *CodeGen) AddRI32(reg int, val int32)           { g.addRI32(reg, val) }
func (g *CodeGen) SubRI32(reg int, val int32)           { g.subRI32(reg, val) }
func (g *CodeGen) CmpRI32(reg int, val int32)           { g.cmpRI32(reg, val) }
func (g *CodeGen) CmpRR32(a, b int)                     { g.cmpRR32(a, b) }
func (g *CodeGen) XorRR32(dst, src int)                 { g.xorRR32(dst, src) }
func (g *CodeGen) TestRR32(a, b int)                    { g.testRR32(a, b) }
func (g *CodeGen) NegR32(reg int)                       { g.negR32(reg) }
func (g *CodeGen) JmpRel8(off int8)                     { g.jmpRel8(off) }
func (g *CodeGen) JmpRel32() int                        { return g.jmpRel32() }
func (g *CodeGen) JccRel32(cc byte) int                 { return g.jccRel32(cc) }
func (g *CodeGen) PatchRel32(fixupOff int)              { g.patchRel32(fixupOff) }
func (g *CodeGen) PatchRel32At(fixupOff, targetOff int) { g.patchRel32At(fixupOff, targetOff) }
func (g *CodeGen) LoadMem32(dst, base, off int)         { g.loadMem32(dst, base, off) }
func (g *CodeGen) StoreMem32(base, off, src int)        { g.storeMem32(base, off, src) }
func (g *CodeGen) LoadMemByte32(dst, base, off int)     { g.loadMemByte32(dst, base, off) }
func (g *CodeGen) StoreMemByte32(base, off, src int)    { g.storeMemByte32(base, off, src) }
func (g *CodeGen) OpPop(reg int)                        { g.opPop(reg) }
func (g *CodeGen) OpPush(reg int)                       { g.opPush(reg) }
func (g *CodeGen) Flush()                               { g.flush() }
func (g *CodeGen) ClearOperandCache()                   { g.clearOperandCache() }
func (g *CodeGen) CompileConstI32(val int64)            { g.compileConstI32(val) }
func (g *CodeGen) EmitInt80()                           { g.emitInt80() }
