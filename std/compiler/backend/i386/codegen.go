package i386

import (
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// === Backend: IRModule → ELF binary ===

type IRInst = ir.Inst

// CodeGen holds state for generating machine code from IR.
type CodeGen struct {
	Target *common.Target

	Code   []byte // .text section
	Rodata []byte // .rodata section (string data + headers)
	Data   []byte // .data section (globals)

	// Function table: name → offset in code
	FuncOffsets map[string]int

	// Fixups for call instructions (need function offset resolution)
	CallFixups []CallFixup

	// Label offsets within current function
	LabelOffsets map[int]int

	// Jump fixups within current function
	JumpFixups []JumpFixup

	// String literal deduplication: string content → rodata offset of header
	StringMap map[string]int

	// Global variable info: global index → offset in .data
	GlobalOffsets []int

	// Current function being compiled
	CurFunc *ir.IRFunc

	// Number of locals (slots) in current function frame
	CurFrameSize int

	// ELF layout constants
	BaseAddr  uint64
	TextStart uint64 // offset in file where .text begins

	// Interface dispatch data from IR
	IRMod *ir.IRModule

	// Pending push optimization: tracks a push that hasn't been emitted yet
	HasPending bool
	PendingReg int
	CacheRegs  []int
	CacheFree  []int
	CacheStack []int

	// Word size for the target architecture (8 for amd64, 4 for i386)
	WordSize int

	// Outlined intrinsic helpers
	NeedTostringHelper bool
	HasTostringHelper  bool
}

const outlinedTostringHelper = "$rtg.tostring$"

// CallFixup records a location in code that needs a relative call target patched.
type CallFixup struct {
	CodeOffset int    // offset of the instruction(s) in code buffer
	Target     string // function name to resolve
	Value      uint64 // raw offset for ARM64 ADRP fixups (section-relative offset)
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

// symEntry holds symbol table entry data for ELF output.
type symEntry struct {
	nameOff int
	value   uint64
	size    uint64
}

// machoSymEntry holds symbol table entry data for Mach-O output.
type machoSymEntry struct {
	nameOff int
	value   uint64
	size    uint64
	ntype   byte
}

// === Shared byte emission ===

func (g *CodeGen) emitByte(b byte) {
	g.Code = append(g.Code, b)
}

func (g *CodeGen) emitBytes(bytes ...byte) {
	g.Code = append(g.Code, bytes...)
}

func (g *CodeGen) emitU32(v uint32) {
	g.Code = append(g.Code, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}

func (g *CodeGen) emitU16(v uint16) {
	g.Code = append(g.Code, byte(v), byte(v>>8))
}

func (g *CodeGen) emitU64(v uint64) {
	g.Code = append(g.Code, byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
		byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56))
}

func (g *CodeGen) emitRodataU64(v uint64) {
	g.Rodata = append(g.Rodata, byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
		byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56))
}

func (g *CodeGen) emitRodataU32(v uint32) {
	g.Rodata = append(g.Rodata, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}

func putU64(buf []byte, v uint64) {
	buf[0] = byte(v)
	buf[1] = byte(v >> 8)
	buf[2] = byte(v >> 16)
	buf[3] = byte(v >> 24)
	buf[4] = byte(v >> 32)
	buf[5] = byte(v >> 40)
	buf[6] = byte(v >> 48)
	buf[7] = byte(v >> 56)
}

func getU64(b []byte) uint64 {
	return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
		uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
}

func putU16(b []byte, v uint16) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
}

func putU32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}

func getU32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

// === Shared code emission helpers ===

// emitCallPlaceholder emits a `call rel32` with a placeholder that gets fixed up later.
func (g *CodeGen) emitCallPlaceholder(target string) {
	g.flush()
	g.emitBytes(0xe8) // call rel32
	g.CallFixups = append(g.CallFixups, CallFixup{
		CodeOffset: len(g.Code),
		Target:     target,
	})
	g.emitU32(0) // placeholder
}

// patchRel32At patches the rel32 at fixupOff to jump to targetOff.
func (g *CodeGen) patchRel32At(fixupOff int, targetOff int) {
	rel := int32(targetOff - (fixupOff + 4))
	g.Code[fixupOff] = byte(rel)
	g.Code[fixupOff+1] = byte(rel >> 8)
	g.Code[fixupOff+2] = byte(rel >> 16)
	g.Code[fixupOff+3] = byte(rel >> 24)
}

// patchRel32 patches the rel32 at fixupOff to jump to the current code position.
func (g *CodeGen) patchRel32(fixupOff int) {
	target := len(g.Code)
	rel := int32(target - (fixupOff + 4))
	g.Code[fixupOff] = byte(rel)
	g.Code[fixupOff+1] = byte(rel >> 8)
	g.Code[fixupOff+2] = byte(rel >> 16)
	g.Code[fixupOff+3] = byte(rel >> 24)
}

// jmpRel32 emits `jmp rel32` and returns the offset of the rel32 for fixup.
func (g *CodeGen) jmpRel32() int {
	g.flush()
	g.emitByte(0xe9)
	off := len(g.Code)
	g.emitU32(0) // placeholder
	return off
}

// jccRel32 emits `jCC rel32` (0x0f, cc) and returns the offset of the rel32.
func (g *CodeGen) jccRel32(cc byte) int {
	g.flush()
	g.emitBytes(0x0f, cc)
	off := len(g.Code)
	g.emitU32(0) // placeholder
	return off
}

// jmpRel8 emits `jmp rel8`.
func (g *CodeGen) jmpRel8(off int8) {
	g.flush()
	g.emitBytes(0xeb, byte(off))
}

// jccRel8 emits `jCC rel8`.
func (g *CodeGen) jccRel8(cc byte, off int8) {
	g.emitBytes(byte(0x70|(cc&0x0f)), byte(off))
}

func fitsRel8(rel int) bool {
	return rel >= -128 && rel <= 127
}

func (g *CodeGen) shiftOffsetsAfterDelete(cutPos int, removed int) {
	if removed <= 0 {
		return
	}
	for id, off := range g.LabelOffsets {
		if off > cutPos {
			g.LabelOffsets[id] = off - removed
		}
	}
	i := 0
	for i < len(g.JumpFixups) {
		if g.JumpFixups[i].CodeOffset > cutPos {
			g.JumpFixups[i].CodeOffset = g.JumpFixups[i].CodeOffset - removed
		}
		i++
	}
	i = 0
	for i < len(g.CallFixups) {
		if g.CallFixups[i].CodeOffset > cutPos {
			g.CallFixups[i].CodeOffset = g.CallFixups[i].CodeOffset - removed
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
		for i < len(g.JumpFixups) {
			fix := g.JumpFixups[i]
			if fix.Kind != jumpFixupJmpRel32 && fix.Kind != jumpFixupJccRel32 {
				i++
				continue
			}
			target, ok := g.LabelOffsets[fix.LabelID]
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
				g.JumpFixups[i].Kind = jumpFixupJmpRel8
				g.JumpFixups[i].CodeOffset = insPos + 1
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
			g.JumpFixups[i].Kind = jumpFixupJccRel8
			g.JumpFixups[i].CodeOffset = insPos + 1
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
	g.emitByte(0xc3)
}

// leave emits `leave`.
func (g *CodeGen) leave() {
	g.emitByte(0xc9)
}

// int3 emits `int3` (breakpoint trap).
func (g *CodeGen) int3() {
	g.emitByte(0xcc)
}

// === Word-size-aware operand stack ===
// These methods work for both amd64 (R15-based, 8-byte slots)
// and i386 (EDI-based, 4-byte slots).

func (g *CodeGen) flush() {
	if len(g.CacheRegs) > 0 {
		if len(g.CacheStack) == 0 && !g.HasPending {
			return
		}
		for _, reg := range g.CacheStack {
			g.rawPush(reg)
		}
		if g.HasPending {
			g.rawPush(g.PendingReg)
		}
		g.CacheStack = g.CacheStack[:0]
		g.CacheFree = append(g.CacheFree[:0], g.CacheRegs...)
		g.HasPending = false
		return
	}
	if !g.HasPending {
		return
	}
	g.HasPending = false
	g.rawPush(g.PendingReg)
}

func (g *CodeGen) configureOperandCache(regs ...int) {
	g.CacheRegs = append(g.CacheRegs[:0], regs...)
	g.clearOperandCache()
}

func (g *CodeGen) clearOperandCache() {
	g.HasPending = false
	g.CacheStack = g.CacheStack[:0]
	g.CacheFree = append(g.CacheFree[:0], g.CacheRegs...)
}

func (g *CodeGen) moveReg(dst, src int) {
	if dst == src {
		return
	}
	g.emitBytes(0x89, byte(0xc0|((src&7)<<3)|(dst&7)))
}

func (g *CodeGen) prepareForClobber(regs ...int) {
	if !g.HasPending {
		return
	}
	conflict := false
	for _, reg := range regs {
		if reg == g.PendingReg {
			conflict = true
			break
		}
	}
	if !conflict {
		return
	}
	if len(g.CacheRegs) > 0 {
		if len(g.CacheFree) == 0 {
			spill := g.CacheStack[0]
			g.rawPush(spill)
			g.CacheStack = g.CacheStack[1:]
			g.CacheFree = append(g.CacheFree, spill)
		}
		slot := len(g.CacheFree) - 1
		dst := g.CacheFree[slot]
		g.CacheFree = g.CacheFree[:slot]
		g.moveReg(dst, g.PendingReg)
		g.CacheStack = append(g.CacheStack, dst)
		g.HasPending = false
		return
	}
	g.flush()
}

func (g *CodeGen) rawPush(reg int) {
	g.emitBytes(0x8d, 0x7f, 0xfc)          // lea edi, [edi-4] (preserves flags)
	g.emitBytes(0x89, byte(0x07|(reg<<3))) // mov [edi], reg
}

func (g *CodeGen) rawPop(reg int) {
	g.emitBytes(0x8b, byte(0x07|(reg<<3))) // mov reg, [edi]
	g.emitBytes(0x8d, 0x7f, 0x04)          // lea edi, [edi+4] (preserves flags)
}

func (g *CodeGen) rawLoad(reg int) {
	g.emitBytes(0x8b, byte(0x07|(reg<<3)))
}

func (g *CodeGen) rawDrop() {
	g.emitBytes(0x83, 0xc7, 0x04)
}

func (g *CodeGen) opPush(reg int) {
	if len(g.CacheRegs) > 0 {
		if g.HasPending {
			if len(g.CacheFree) == 0 {
				spill := g.CacheStack[0]
				g.rawPush(spill)
				g.CacheStack = g.CacheStack[1:]
				g.CacheFree = append(g.CacheFree, spill)
			}
			slot := len(g.CacheFree) - 1
			dst := g.CacheFree[slot]
			g.CacheFree = g.CacheFree[:slot]
			g.moveReg(dst, g.PendingReg)
			g.CacheStack = append(g.CacheStack, dst)
		}
		g.HasPending = true
		g.PendingReg = reg
		return
	}
	g.flush()
	g.HasPending = true
	g.PendingReg = reg
}

func (g *CodeGen) opPop(reg int) {
	if len(g.CacheRegs) > 0 {
		if g.HasPending {
			g.HasPending = false
			g.moveReg(reg, g.PendingReg)
			return
		}
		if len(g.CacheStack) > 0 {
			last := len(g.CacheStack) - 1
			src := g.CacheStack[last]
			g.CacheStack = g.CacheStack[:last]
			g.CacheFree = append(g.CacheFree, src)
			g.moveReg(reg, src)
			return
		}
		g.rawPop(reg)
		return
	}
	if g.HasPending {
		g.HasPending = false
		g.moveReg(reg, g.PendingReg)
		return
	}
	g.rawPop(reg)
}

func (g *CodeGen) opLoad(reg int) {
	if len(g.CacheRegs) > 0 {
		if g.HasPending {
			g.moveReg(reg, g.PendingReg)
			return
		}
		if len(g.CacheStack) > 0 {
			g.moveReg(reg, g.CacheStack[len(g.CacheStack)-1])
			return
		}
		g.rawLoad(reg)
		return
	}
	if g.HasPending {
		g.moveReg(reg, g.PendingReg)
		g.flush()
		return
	}
	g.rawLoad(reg)
}

func (g *CodeGen) opStore(reg int) {
	g.flush()
	g.emitBytes(0x89, byte(0x07|(reg<<3)))
}

func (g *CodeGen) opDrop() {
	if len(g.CacheRegs) > 0 {
		if g.HasPending {
			g.HasPending = false
			return
		}
		if len(g.CacheStack) > 0 {
			last := len(g.CacheStack) - 1
			g.CacheFree = append(g.CacheFree, g.CacheStack[last])
			g.CacheStack = g.CacheStack[:last]
			return
		}
		g.rawDrop()
		return
	}
	if g.HasPending {
		g.HasPending = false
		return
	}
	g.rawDrop()
}

// emitCallIAT emits `call dword ptr [abs32]` for calling Windows IAT entries.
func (g *CodeGen) emitCallIAT(funcName string) {
	g.emitCallIATInLib(winDefaultImportLibrary, funcName)
}

func (g *CodeGen) emitCallIATInLib(libName string, funcName string) {
	g.flush()
	g.emitBytes(0xFF, 0x15) // call dword ptr [abs32]
	g.CallFixups = append(g.CallFixups, CallFixup{
		CodeOffset: len(g.Code),
		Target:     encodeIATFixupTarget(libName, funcName),
	})
	g.emitU32(0) // placeholder
}

// emitJmpIAT emits `jmp dword ptr [abs32]` for jumping to Windows IAT entries.
func (g *CodeGen) emitJmpIAT(funcName string) {
	g.emitJmpIATInLib(winDefaultImportLibrary, funcName)
}

func (g *CodeGen) emitJmpIATInLib(libName string, funcName string) {
	g.flush()
	g.emitBytes(0xFF, 0x25) // jmp dword ptr [abs32]
	g.CallFixups = append(g.CallFixups, CallFixup{
		CodeOffset: len(g.Code),
		Target:     encodeIATFixupTarget(libName, funcName),
	})
	g.emitU32(0) // placeholder
}

// alignUp aligns v up to the next multiple of align.
func alignUp(v, align int) int {
	return (v + align - 1) & ^(align - 1)
}

// sectionSpan returns the in-memory RVA span for a section.
// Even empty sections must consume one aligned slot so RVAs stay strictly increasing.
func sectionSpan(size, align int) int {
	if size <= 0 {
		return align
	}
	return alignUp(size, align)
}
