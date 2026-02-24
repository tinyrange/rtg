package core

import (
	"fmt"
	"os"

	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// === Backend: IRModule → ELF binary ===

// CodeGen holds state for generating machine code from IR.
type CodeGen struct {
	target *common.Target

	Code   []byte // .text section
	Rodata []byte // .rodata section (string data + headers)
	Data   []byte // .data section (globals)

	// Function table: name → offset in code
	funcOffsets map[string]int

	// Fixups for call instructions (need function offset resolution)
	callFixups []CallFixup

	// Label offsets within current function
	labelOffsets map[int]int

	// Jump fixups within current function
	jumpFixups []JumpFixup

	// String literal deduplication: string content → rodata offset of header
	stringMap map[string]int

	// Global variable info: global index → offset in .data
	globalOffsets []int

	// Current function being compiled
	curFunc *ir.IRFunc

	// Number of locals (slots) in current function frame
	curFrameSize int

	// ELF layout constants
	BaseAddr  uint64
	textStart uint64 // offset in file where .text begins

	// Interface dispatch data from IR
	irmod *ir.IRModule

	// Pending push optimization: tracks a push that hasn't been emitted yet
	hasPending bool
	pendingReg int
	cacheRegs  []int
	cacheFree  []int
	cacheStack []int

	// Word size for the target architecture (8 for amd64, 4 for i386)
	wordSize int

	// Outlined intrinsic helpers
	NeedTostringHelper bool
	hasTostringHelper  bool
}

func NewCodeGen(target *common.Target, irmod *ir.IRModule, baseAddr uint64) *CodeGen {
	g := &CodeGen{
		target:        target,
		funcOffsets:   make(map[string]int),
		labelOffsets:  make(map[int]int),
		stringMap:     make(map[string]int),
		globalOffsets: make([]int, len(irmod.Globals)),
		BaseAddr:      0x400000,
		irmod:         irmod,
		wordSize:      8,
	}

	// Allocate .data space for globals (8 bytes each)
	for i := range irmod.Globals {
		g.globalOffsets[i] = i * 8
	}
	g.Data = make([]byte, len(irmod.Globals)*8)

	return g
}

const outlinedTostringHelper = "$rtg.tostring$"

// CallFixup records a location in code that needs a relative call target patched.
type CallFixup struct {
	CodeOffset int    // offset of the instruction(s) in code buffer
	Target     string // function name to resolve
	Value      uint64 // raw offset for ARM64 ADRP fixups (section-relative offset)
}

func CallFixupTarget(f CallFixup) string {
	return f.Target
}

func CallFixupOffset(f CallFixup) int {
	return f.CodeOffset
}

// JumpFixup records a location that needs a relative jump target patched.
type JumpFixup struct {
	CodeOffset int // offset of the 4-byte rel32 in code buffer
	LabelID    int // label to resolve
	Kind       int // jumpFixup* kind
	CC         byte
}

const (
	jumpFixupJmpRel32 = iota
	jumpFixupJccRel32
	jumpFixupJmpRel8
	jumpFixupJccRel8
)

// dispatchEntry pairs a type ID with a method function name for interface dispatch.
type DispatchEntry struct {
	typeID   int
	funcName string
}

// SymEntry holds symbol table entry data for ELF output.
type SymEntry struct {
	NameOff int
	Value   uint64
	Size    uint64
}

// === Accessors ===

func (g *CodeGen) StringMap() map[string]int {
	return g.stringMap
}

func (g *CodeGen) Target() *common.Target {
	return g.target
}

func (g *CodeGen) IRModule() *ir.IRModule {
	return g.irmod
}

func (g *CodeGen) GetFuncOffset(name string) int {
	return g.funcOffsets[name]
}

func (g *CodeGen) MaybeGetFuncOffsets(name string) (int, bool) {
	offset, ok := g.funcOffsets[name]
	return offset, ok
}

func (g *CodeGen) CallFixups() []CallFixup {
	return g.callFixups
}

func (g *CodeGen) AddCallFixup(target string) {
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: len(g.Code),
		Target:     target,
	})
}

func (g *CodeGen) ResolveLinuxCallFixups() []string {
	var unresolved []string
	for _, fix := range g.callFixups {
		if fix.Target == "$rodata_header$" || fix.Target == "$data_addr$" {
			continue
		}
		target, ok := g.MaybeGetFuncOffsets(fix.Target)
		if !ok {
			unresolved = append(unresolved, fix.Target)
			continue
		}
		g.PatchRel32At(fix.CodeOffset, target)
	}
	return unresolved
}

func (g *CodeGen) PatchLinuxDataAndRodataFixups(rodataVAddr uint64, dataVAddr uint64) {
	for _, fix := range g.callFixups {
		if fix.Target == "$rodata_header$" {
			headerOff := common.GetU64(g.Code[fix.CodeOffset : fix.CodeOffset+8])
			common.PutU64(g.Code[fix.CodeOffset:fix.CodeOffset+8], rodataVAddr+headerOff)
		} else if fix.Target == "$data_addr$" {
			dataOff := common.GetU64(g.Code[fix.CodeOffset : fix.CodeOffset+8])
			common.PutU64(g.Code[fix.CodeOffset:fix.CodeOffset+8], dataVAddr+dataOff)
		}
	}
}

func (g *CodeGen) PatchLinuxStringHeaders(rodataVAddr uint64) {
	for _, headerOff := range g.stringMap {
		dataOff := common.GetU64(g.Rodata[headerOff : headerOff+8])
		common.PutU64(g.Rodata[headerOff:headerOff+8], rodataVAddr+dataOff)
	}
}

// === Public Methods ===

func (g *CodeGen) EmitAllFunctions(irmod *ir.IRModule) {
	// Compile all functions
	for _, f := range irmod.Funcs {
		g.funcOffsets[f.Name] = len(g.Code)
		g.CompileFunc(f)
	}

	ir.CollectNativeFuncSizes(irmod, g.funcOffsets, len(g.Code))
	if g.NeedTostringHelper {
		g.EmitTostringHelperX64()
	}
}

func (g *CodeGen) CheckUnresolvedCalls(unresolved []string) error {
	if len(unresolved) > 0 {
		fmt.Fprintf(os.Stderr, "error: %d unresolved calls:\n", len(unresolved))
		seen := make(map[string]bool)
		for _, name := range unresolved {
			if !seen[name] {
				fmt.Fprintf(os.Stderr, "  %s\n", name)
				seen[name] = true
			}
		}
		return fmt.Errorf("%d unresolved calls", len(unresolved))
	}

	return nil
}

// === Shared byte emission ===

func (g *CodeGen) EmitByte(b byte) {
	g.Code = append(g.Code, b)
}

func (g *CodeGen) EmitBytes(bytes ...byte) {
	g.Code = append(g.Code, bytes...)
}

func (g *CodeGen) EmitU32(v uint32) {
	g.Code = append(g.Code, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}

func (g *CodeGen) EmitU16(v uint16) {
	g.Code = append(g.Code, byte(v), byte(v>>8))
}

func (g *CodeGen) EmitU64(v uint64) {
	g.Code = append(g.Code, byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
		byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56))
}

func (g *CodeGen) EmitRodataU64(v uint64) {
	g.Rodata = append(g.Rodata, byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
		byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56))
}

// === Shared code emission helpers ===

// EmitCallPlaceholder emits a `call rel32` with a placeholder that gets fixed up later.
func (g *CodeGen) EmitCallPlaceholder(target string) {
	g.Flush()
	g.EmitBytes(0xe8) // call rel32
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: len(g.Code),
		Target:     target,
	})
	g.EmitU32(0) // placeholder
}

// PatchRel32At patches the rel32 at fixupOff to jump to targetOff.
func (g *CodeGen) PatchRel32At(fixupOff int, targetOff int) {
	rel := int32(targetOff - (fixupOff + 4))
	g.Code[fixupOff] = byte(rel)
	g.Code[fixupOff+1] = byte(rel >> 8)
	g.Code[fixupOff+2] = byte(rel >> 16)
	g.Code[fixupOff+3] = byte(rel >> 24)
}

// PatchRel32 patches the rel32 at fixupOff to jump to the current code position.
func (g *CodeGen) PatchRel32(fixupOff int) {
	target := len(g.Code)
	rel := int32(target - (fixupOff + 4))
	g.Code[fixupOff] = byte(rel)
	g.Code[fixupOff+1] = byte(rel >> 8)
	g.Code[fixupOff+2] = byte(rel >> 16)
	g.Code[fixupOff+3] = byte(rel >> 24)
}

// JmpRel32 emits `jmp rel32` and returns the offset of the rel32 for fixup.
func (g *CodeGen) JmpRel32() int {
	g.Flush()
	g.EmitByte(0xe9)
	off := len(g.Code)
	g.EmitU32(0) // placeholder
	return off
}

// JccRel32 emits `jCC rel32` (0x0f, cc) and returns the offset of the rel32.
func (g *CodeGen) JccRel32(cc byte) int {
	g.Flush()
	g.EmitBytes(0x0f, cc)
	off := len(g.Code)
	g.EmitU32(0) // placeholder
	return off
}

func fitsRel8(rel int) bool {
	return rel >= -128 && rel <= 127
}

func (g *CodeGen) shiftOffsetsAfterDelete(cutPos int, removed int) {
	if removed <= 0 {
		return
	}
	for id, off := range g.labelOffsets {
		if off > cutPos {
			g.labelOffsets[id] = off - removed
		}
	}
	i := 0
	for i < len(g.jumpFixups) {
		if g.jumpFixups[i].CodeOffset > cutPos {
			g.jumpFixups[i].CodeOffset = g.jumpFixups[i].CodeOffset - removed
		}
		i++
	}
	i = 0
	for i < len(g.callFixups) {
		if g.callFixups[i].CodeOffset > cutPos {
			g.callFixups[i].CodeOffset = g.callFixups[i].CodeOffset - removed
		}
		i++
	}
}

// relaxCurrentFuncJumps shortens rel32 jumps/jccs to rel8 when possible for x86 backends.
// Must be called before resolving rel32 fixups.
func (g *CodeGen) relaxCurrentFuncJumps() {
	changed := true
	for changed {
		changed = false
		i := 0
		for i < len(g.jumpFixups) {
			fix := g.jumpFixups[i]
			if fix.Kind != jumpFixupJmpRel32 && fix.Kind != jumpFixupJccRel32 {
				i++
				continue
			}
			target, ok := g.labelOffsets[fix.LabelID]
			if !ok {
				i++
				continue
			}
			if fix.Kind == jumpFixupJmpRel32 {
				insPos := fix.CodeOffset - 1
				rel := target - (insPos + 2)
				if !fitsRel8(rel) {
					i++
					continue
				}
				g.Code[insPos] = 0xeb
				g.jumpFixups[i].Kind = jumpFixupJmpRel8
				g.jumpFixups[i].CodeOffset = insPos + 1
				// Delete trailing bytes of old rel32 encoding.
				g.Code = append(g.Code[:insPos+2], g.Code[insPos+5:]...)
				g.shiftOffsetsAfterDelete(insPos+1, 3)
				changed = true
				i++
				continue
			}
			insPos := fix.CodeOffset - 2
			rel := target - (insPos + 2)
			if !fitsRel8(rel) {
				i++
				continue
			}
			g.Code[insPos] = byte(0x70 | (fix.CC & 0x0f))
			g.jumpFixups[i].Kind = jumpFixupJccRel8
			g.jumpFixups[i].CodeOffset = insPos + 1
			// Delete trailing bytes of old near-jcc encoding.
			g.Code = append(g.Code[:insPos+2], g.Code[insPos+6:]...)
			g.shiftOffsetsAfterDelete(insPos+1, 4)
			changed = true
			i++
		}
	}
}

// ret emits `ret`.
func (g *CodeGen) ret() {
	if g.target.GOOS == "dos" && g.wordSize == 4 {
		g.EmitByte(0x66)
	}
	g.EmitByte(0xc3)
}

// leave emits `leave`.
func (g *CodeGen) leave() {
	g.EmitByte(0xc9)
}

// int3 emits `int3` (breakpoint trap).
func (g *CodeGen) int3() {
	g.EmitByte(0xcc)
}

// === Word-size-aware operand stack ===
// These methods work for both amd64 (R15-based, 8-byte slots)
// and i386 (EDI-based, 4-byte slots).

func (g *CodeGen) Flush() {
	if len(g.cacheRegs) > 0 {
		if len(g.cacheStack) == 0 && !g.hasPending {
			return
		}
		for _, reg := range g.cacheStack {
			g.rawPush(reg)
		}
		if g.hasPending {
			g.rawPush(g.pendingReg)
		}
		g.cacheStack = g.cacheStack[:0]
		g.cacheFree = append(g.cacheFree[:0], g.cacheRegs...)
		g.hasPending = false
		return
	}
	if !g.hasPending {
		return
	}
	g.hasPending = false
	g.rawPush(g.pendingReg)
}

func (g *CodeGen) configureOperandCache(regs ...int) {
	g.cacheRegs = append(g.cacheRegs[:0], regs...)
	g.ClearOperandCache()
}

func (g *CodeGen) ClearOperandCache() {
	g.hasPending = false
	g.cacheStack = g.cacheStack[:0]
	g.cacheFree = append(g.cacheFree[:0], g.cacheRegs...)
}

func (g *CodeGen) moveReg(dst, src int) {
	if dst == src {
		return
	}
	if g.wordSize == 2 {
		g.EmitBytes(0x89, byte(0xc0|((src&7)<<3)|(dst&7)))
		return
	}
	if g.wordSize == 4 {
		if g.target.GOOS == "dos" {
			g.EmitBytes(0x66, 0x89, byte(0xc0|((src&7)<<3)|(dst&7)))
		} else {
			g.EmitBytes(0x89, byte(0xc0|((src&7)<<3)|(dst&7)))
		}
		return
	}
	rex := byte(0x48)
	if src >= 8 {
		rex |= 0x04
	}
	if dst >= 8 {
		rex |= 0x01
	}
	g.EmitBytes(rex, 0x89, byte(0xc0|((src&7)<<3)|(dst&7)))
}

func (g *CodeGen) prepareForClobber(regs ...int) {
	if !g.hasPending {
		return
	}
	conflict := false
	for _, reg := range regs {
		if reg == g.pendingReg {
			conflict = true
			break
		}
	}
	if !conflict {
		return
	}
	if len(g.cacheRegs) > 0 {
		if len(g.cacheFree) == 0 {
			spill := g.cacheStack[0]
			g.rawPush(spill)
			g.cacheStack = g.cacheStack[1:]
			g.cacheFree = append(g.cacheFree, spill)
		}
		slot := len(g.cacheFree) - 1
		dst := g.cacheFree[slot]
		g.cacheFree = g.cacheFree[:slot]
		g.moveReg(dst, g.pendingReg)
		g.cacheStack = append(g.cacheStack, dst)
		g.hasPending = false
		return
	}
	g.Flush()
}

func (g *CodeGen) rawPush(reg int) {
	g.EmitBytes(0x4d, 0x8d, 0x7f, 0xf8) // lea r15, [r15-8] (preserves flags)
	rex := byte(0x49)
	if reg >= 8 {
		rex = 0x4d
	}
	g.EmitBytes(rex, 0x89, byte(0x07|((reg&7)<<3)))
}

func (g *CodeGen) rawPop(reg int) {
	rex := byte(0x49)
	if reg >= 8 {
		rex = 0x4d
	}
	g.EmitBytes(rex, 0x8b, byte(0x07|((reg&7)<<3)))
	g.EmitBytes(0x4d, 0x8d, 0x7f, 0x08) // lea r15, [r15+8] (preserves flags)
}

func (g *CodeGen) rawLoad(reg int) {
	rex := byte(0x49)
	if reg >= 8 {
		rex = 0x4d
	}
	g.EmitBytes(rex, 0x8b, byte(0x07|((reg&7)<<3)))
}

func (g *CodeGen) rawDrop() {
	g.EmitBytes(0x49, 0x83, 0xc7, 0x08)
}

func (g *CodeGen) OpPush(reg int) {
	if len(g.cacheRegs) > 0 {
		if g.hasPending {
			if len(g.cacheFree) == 0 {
				spill := g.cacheStack[0]
				g.rawPush(spill)
				g.cacheStack = g.cacheStack[1:]
				g.cacheFree = append(g.cacheFree, spill)
			}
			slot := len(g.cacheFree) - 1
			dst := g.cacheFree[slot]
			g.cacheFree = g.cacheFree[:slot]
			g.moveReg(dst, g.pendingReg)
			g.cacheStack = append(g.cacheStack, dst)
		}
		g.hasPending = true
		g.pendingReg = reg
		return
	}
	g.Flush()
	g.hasPending = true
	g.pendingReg = reg
}

func (g *CodeGen) OpPop(reg int) {
	if len(g.cacheRegs) > 0 {
		if g.hasPending {
			g.hasPending = false
			g.moveReg(reg, g.pendingReg)
			return
		}
		if len(g.cacheStack) > 0 {
			last := len(g.cacheStack) - 1
			src := g.cacheStack[last]
			g.cacheStack = g.cacheStack[:last]
			g.cacheFree = append(g.cacheFree, src)
			g.moveReg(reg, src)
			return
		}
		g.rawPop(reg)
		return
	}
	if g.hasPending {
		g.hasPending = false
		g.moveReg(reg, g.pendingReg)
		return
	}
	g.rawPop(reg)
}

func (g *CodeGen) opLoad(reg int) {
	if len(g.cacheRegs) > 0 {
		if g.hasPending {
			g.moveReg(reg, g.pendingReg)
			return
		}
		if len(g.cacheStack) > 0 {
			g.moveReg(reg, g.cacheStack[len(g.cacheStack)-1])
			return
		}
		g.rawLoad(reg)
		return
	}
	if g.hasPending {
		g.moveReg(reg, g.pendingReg)
		g.Flush()
		return
	}
	g.rawLoad(reg)
}

func (g *CodeGen) opDrop() {
	if len(g.cacheRegs) > 0 {
		if g.hasPending {
			g.hasPending = false
			return
		}
		if len(g.cacheStack) > 0 {
			last := len(g.cacheStack) - 1
			g.cacheFree = append(g.cacheFree, g.cacheStack[last])
			g.cacheStack = g.cacheStack[:last]
			return
		}
		g.rawDrop()
		return
	}
	if g.hasPending {
		g.hasPending = false
		return
	}
	g.rawDrop()
}

// AlignUp aligns v up to the next multiple of align.
func AlignUp(v, align int) int {
	return (v + align - 1) & ^(align - 1)
}

// sectionSpan returns the in-memory RVA span for a section.
// Even empty sections must consume one aligned slot so RVAs stay strictly increasing.
func SectionSpan(size, align int) int {
	if size <= 0 {
		return align
	}
	return AlignUp(size, align)
}
