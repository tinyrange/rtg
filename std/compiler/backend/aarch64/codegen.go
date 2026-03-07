package aarch64

import (
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// === Backend: IRModule → ELF binary ===

// CodeGen holds state for generating machine code from IR.
type CodeGen struct {
	target *common.Target

	code   []byte // .text section
	rodata []byte // .rodata section (string data + headers)
	data   []byte // .data section (globals)

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
	// Ordered list of string header offsets in .data
	stringHeaderOff []int

	// Global variable info: global index → offset in .data
	globalOffsets []int

	// Current function being compiled
	curFunc *ir.IRFunc

	// Number of locals (slots) in current function frame
	curFrameSize int
	// True while compiling a function that should share one epilogue across
	// multiple RETURN instructions.
	shareReturnEpilogue bool
	// Code offset of the shared function epilogue for multi-return functions.
	// -1 means the shared epilogue has not been emitted yet.
	returnEpilogueOffset int

	// ELF layout constants
	baseAddr  uint64
	textStart uint64 // offset in file where .text begins

	// Interface dispatch data from IR
	irmod *ir.IRModule

	// Pending push optimization: tracks a push that hasn't been emitted yet
	hasPending bool
	pendingReg int
	pendingOwn int
	cacheRegs  []int
	cacheFree  []int
	cacheStack []int
	cacheOwn   []int

	// Word size for the target architecture (8 for amd64, 4 for i386)
	wordSize int

	// ARM64-specific
	isArm64         bool
	gotEntries      map[string]int // libSystem symbol name → GOT slot index
	gotSymbols      []string       // ordered list of imported symbols
	stringRodataMap map[int]int    // string header offset in data → rodata offset of bytes

	// Outlined intrinsic helpers
	needTostringHelper bool
	hasTostringHelper  bool

	// Per-instruction machine-byte trace (enabled by -emit-ir-and-binary).
	traceEnabled      bool
	traceCurInst      int
	traceForcedInst   int
	traceCurFuncInsts []InstByteTrace
	traceByFunc       map[string][]InstByteTrace
}

type InstByteSegment struct {
	Start int
	End   int
}

type InstByteTrace struct {
	Segments []InstByteSegment
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

type compiledFuncSignature struct {
	start int
}

// NewCodeGen creates an ARM64 code generator with initialized global/data layout.
func NewCodeGen(target *common.Target, irmod *ir.IRModule, baseAddr uint64, extraGlobals int, withGOT bool) *CodeGen {
	g := &CodeGen{}
	g.target = target
	g.funcOffsets = make(map[string]int)
	g.labelOffsets = make(map[int]int)
	g.stringMap = make(map[string]int)
	g.globalOffsets = make([]int, len(irmod.Globals))
	g.baseAddr = baseAddr
	g.irmod = irmod
	g.wordSize = 8
	g.isArm64 = true
	g.pendingOwn = -1
	g.traceEnabled = target.EmitIRAndBinaryPath != ""
	g.traceCurInst = -1
	g.traceForcedInst = -1
	if g.traceEnabled {
		g.traceByFunc = make(map[string][]InstByteTrace)
	}
	if withGOT {
		g.gotEntries = make(map[string]int)
	}
	for i := range irmod.Globals {
		g.globalOffsets[i] = i * 8
	}
	g.data = make([]byte, (len(irmod.Globals)+extraGlobals)*8)
	g.initOperandCacheArm64()
	return g
}

// Target returns the selected compilation target.
func (g *CodeGen) Target() *common.Target { return g.target }

// BaseAddr returns the image base virtual address used by the output format.
func (g *CodeGen) BaseAddr() uint64 { return g.baseAddr }

// IsArm64 reports whether this code generator is configured for AArch64 output.
func (g *CodeGen) IsArm64() bool { return g.isArm64 }

// Code returns the mutable .text section buffer.
func (g *CodeGen) Code() []byte { return g.code }

// CodeLen returns the current .text section size.
func (g *CodeGen) CodeLen() int { return len(g.code) }

// Rodata returns the mutable .rodata section buffer.
func (g *CodeGen) Rodata() []byte { return g.rodata }

// Data returns the mutable .data section buffer.
func (g *CodeGen) Data() []byte { return g.data }

// SetFuncOffset records the function start offset in .text.
func (g *CodeGen) SetFuncOffset(name string, off int) {
	g.funcOffsets[name] = off
}

// LookupFuncOffset resolves a function start offset in .text.
func (g *CodeGen) LookupFuncOffset(name string) (int, bool) {
	v, ok := g.funcOffsets[name]
	return v, ok
}

func (g *CodeGen) FuncOffsets() map[string]int {
	return g.funcOffsets
}

func (g *CodeGen) FuncInstTraces() map[string][]InstByteTrace {
	return g.traceByFunc
}

func (g *CodeGen) traceOwner() int {
	if !g.traceEnabled {
		return -1
	}
	owner := g.traceCurInst
	if g.traceForcedInst >= 0 {
		owner = g.traceForcedInst
	}
	if owner < 0 || owner >= len(g.traceCurFuncInsts) {
		return -1
	}
	return owner
}

func (g *CodeGen) traceRecordCode(start int, end int) {
	if !g.traceEnabled {
		return
	}
	if end <= start {
		return
	}
	owner := g.traceOwner()
	if owner < 0 {
		return
	}
	segments := g.traceCurFuncInsts[owner].Segments
	if len(segments) > 0 {
		last := len(segments) - 1
		if segments[last].End == start {
			segments[last].End = end
			g.traceCurFuncInsts[owner].Segments = segments
			return
		}
	}
	segments = append(segments, InstByteSegment{start, end})
	g.traceCurFuncInsts[owner].Segments = segments
}

func (g *CodeGen) traceSetCurrentInst(owner int) {
	if !g.traceEnabled {
		return
	}
	g.traceCurInst = owner
	g.traceForcedInst = -1
}

func (g *CodeGen) traceClearCurrentInst() {
	if !g.traceEnabled {
		return
	}
	g.traceCurInst = -1
	g.traceForcedInst = -1
}

func (g *CodeGen) traceForceInst(owner int) int {
	if !g.traceEnabled {
		return -1
	}
	prev := g.traceForcedInst
	g.traceForcedInst = owner
	return prev
}

func (g *CodeGen) traceRestoreForcedInst(prev int) {
	if !g.traceEnabled {
		return
	}
	g.traceForcedInst = prev
}

// CompileModuleFuncs compiles all IR functions and records deterministic function offsets.
func (g *CodeGen) CompileModuleFuncs(irmod *ir.IRModule) {
	if g.traceEnabled {
		for _, f := range irmod.Funcs {
			g.funcOffsets[f.Name] = len(g.code)
			g.CompileFuncArm64(f)
		}
		return
	}
	seen := make(map[string]compiledFuncSignature)
	for _, f := range irmod.Funcs {
		start := len(g.code)
		fixStart := len(g.callFixups)
		g.funcOffsets[f.Name] = start
		g.CompileFuncArm64(f)
		sig := g.compiledFuncBodyKey(start, fixStart)
		if prior, ok := seen[sig]; ok {
			g.code = g.code[:start]
			g.callFixups = g.callFixups[:fixStart]
			g.funcOffsets[f.Name] = prior.start
			continue
		}
		seen[sig] = compiledFuncSignature{start}
	}
}

func (g *CodeGen) compiledFuncBodyKey(start int, fixStart int) string {
	body := g.code[start:]
	sig := make([]byte, 0, 4+len(body)+(len(g.callFixups)-fixStart)*32)
	sig = append(sig, 0, 0, 0, 0)
	common.PutU32(sig[len(sig)-4:], uint32(len(body)))
	sig = append(sig, body...)
	for i := fixStart; i < len(g.callFixups); i++ {
		fix := g.callFixups[i]
		rel := fix.CodeOffset - start
		sig = append(sig, 0, 0, 0, 0)
		common.PutU32(sig[len(sig)-4:], uint32(rel))
		sig = append(sig, 0, 0, 0, 0, 0, 0, 0, 0)
		common.PutU64(sig[len(sig)-8:], fix.Value)
		sig = append(sig, fix.Target...)
		sig = append(sig, 0)
	}
	return string(sig)
}

// CollectNativeFuncSizes records final native function sizes into IR metadata.
func (g *CodeGen) CollectNativeFuncSizes(irmod *ir.IRModule) {
	ir.CollectNativeFuncSizes(irmod, g.funcOffsets, len(g.code))
}

const (
	FixupSkipRodataHeader = 1 << iota
	FixupSkipDataAddr
	FixupSkipGotAddr
	FixupSkipIAT
)

// ResolveCallFixups patches direct call placeholders against known function offsets.
func (g *CodeGen) ResolveCallFixups(skipMask int) []string {
	var unresolved []string
	for _, fix := range g.callFixups {
		if fix.Target == "$rodata_header$" && (skipMask&FixupSkipRodataHeader) != 0 {
			continue
		}
		if fix.Target == "$data_addr$" && (skipMask&FixupSkipDataAddr) != 0 {
			continue
		}
		if fix.Target == "$got_addr$" && (skipMask&FixupSkipGotAddr) != 0 {
			continue
		}
		if (skipMask&FixupSkipIAT) != 0 && len(fix.Target) > 5 && fix.Target[0:5] == "$iat$" {
			continue
		}
		targetOff, ok := g.funcOffsets[fix.Target]
		if !ok {
			unresolved = append(unresolved, fix.Target)
			continue
		}
		g.PatchArm64BAt(fix.CodeOffset, targetOff)
	}
	return unresolved
}

// CallFixupCount returns number of recorded call fixups.
func (g *CodeGen) CallFixupCount() int { return len(g.callFixups) }

// CallFixupAt returns the code offset, symbolic target and raw value for fixup i.
func (g *CodeGen) CallFixupAt(i int) (int, string, uint64) {
	fix := g.callFixups[i]
	return fix.CodeOffset, fix.Target, fix.Value
}

// AddCallFixup records a call fixup entry.
func (g *CodeGen) AddCallFixup(codeOffset int, target string, value uint64) {
	g.callFixups = append(g.callFixups, CallFixup{codeOffset, target, value})
}

// StringMap returns deduplicated string-header offsets by string content.
func (g *CodeGen) StringMap() map[string]int { return g.stringMap }

// StringHeaderOffsets returns the ordered offsets of string headers in .data.
func (g *CodeGen) StringHeaderOffsets() []int { return g.stringHeaderOff }

// StringRodataMap returns mapping from string header offsets to rodata byte offsets.
func (g *CodeGen) StringRodataMap() map[int]int { return g.stringRodataMap }

// GotSymbols returns imported symbol order for GOT emission.
func (g *CodeGen) GotSymbols() []string { return g.gotSymbols }

// NeedTostringHelper reports whether the outlined tostring helper must be emitted.
func (g *CodeGen) NeedTostringHelper() bool { return g.needTostringHelper }

// === Shared byte emission ===

func (g *CodeGen) emitByte(b byte) {
	start := len(g.code)
	g.code = append(g.code, b)
	g.traceRecordCode(start, len(g.code))
}

func (g *CodeGen) emitBytes(bytes ...byte) {
	start := len(g.code)
	g.code = append(g.code, bytes...)
	g.traceRecordCode(start, len(g.code))
}

func (g *CodeGen) emitU32(v uint32) {
	start := len(g.code)
	g.code = append(g.code, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
	g.traceRecordCode(start, len(g.code))
}

func (g *CodeGen) emitU16(v uint16) {
	start := len(g.code)
	g.code = append(g.code, byte(v), byte(v>>8))
	g.traceRecordCode(start, len(g.code))
}

func (g *CodeGen) emitU64(v uint64) {
	start := len(g.code)
	g.code = append(g.code, byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
		byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56))
	g.traceRecordCode(start, len(g.code))
}

func (g *CodeGen) emitRodataU64(v uint64) {
	g.rodata = append(g.rodata, byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
		byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56))
}

func (g *CodeGen) emitRodataU32(v uint32) {
	g.rodata = append(g.rodata, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}

// === Shared code emission helpers ===

// emitCallPlaceholder emits a `call rel32` with a placeholder that gets fixed up later.
func (g *CodeGen) emitCallPlaceholder(target string) {
	g.Flush()
	if g.target.GOOS == "dos" && g.wordSize == 2 {
		g.emitBytes(0xe8) // call rel16
		g.callFixups = append(g.callFixups, CallFixup{len(g.code), target, 0})
		g.emitU16(0)
	} else {
		g.emitBytes(0xe8) // call rel32
		g.callFixups = append(g.callFixups, CallFixup{len(g.code), target, 0})
		g.emitU32(0) // placeholder
	}
}

// patchRel32At patches the rel32 at fixupOff to jump to targetOff.
func (g *CodeGen) patchRel32At(fixupOff int, targetOff int) {
	if g.target.GOOS == "dos" && g.wordSize == 2 {
		rel := int16(targetOff - (fixupOff + 2))
		g.code[fixupOff] = byte(rel)
		g.code[fixupOff+1] = byte(rel >> 8)
	} else {
		rel := int32(targetOff - (fixupOff + 4))
		g.code[fixupOff] = byte(rel)
		g.code[fixupOff+1] = byte(rel >> 8)
		g.code[fixupOff+2] = byte(rel >> 16)
		g.code[fixupOff+3] = byte(rel >> 24)
	}
}

// patchRel32 patches the rel32 at fixupOff to jump to the current code position.
func (g *CodeGen) patchRel32(fixupOff int) {
	target := len(g.code)
	if g.target.GOOS == "dos" && g.wordSize == 2 {
		rel := int16(target - (fixupOff + 2))
		g.code[fixupOff] = byte(rel)
		g.code[fixupOff+1] = byte(rel >> 8)
	} else {
		rel := int32(target - (fixupOff + 4))
		g.code[fixupOff] = byte(rel)
		g.code[fixupOff+1] = byte(rel >> 8)
		g.code[fixupOff+2] = byte(rel >> 16)
		g.code[fixupOff+3] = byte(rel >> 24)
	}
}

// jmpRel32 emits `jmp rel32` and returns the offset of the rel32 for fixup.
func (g *CodeGen) jmpRel32() int {
	g.Flush()
	if g.target.GOOS == "dos" && g.wordSize == 2 {
		g.emitByte(0xe9) // jmp rel16
		off := len(g.code)
		g.emitU16(0)
		return off
	}
	g.emitByte(0xe9)
	off := len(g.code)
	g.emitU32(0) // placeholder
	return off
}

// jccRel32 emits `jCC rel32` (0x0f, cc) and returns the offset of the rel32.
func (g *CodeGen) jccRel32(cc byte) int {
	g.Flush()
	if g.target.GOOS == "dos" && g.wordSize == 2 {
		// 8086 has only short conditional branches.
		// Lower near Jcc as:
		//   j!cc +3
		//   jmp  rel16
		inv := (cc & 0x0f) ^ 0x01
		g.emitBytes(byte(0x70|inv), 0x03, 0xe9)
		off := len(g.code)
		g.emitU16(0)
		return off
	}
	if g.target.GOOS == "dos" && g.wordSize == 4 {
		g.emitByte(0x66) // jcc rel32 in 16-bit mode
	}
	g.emitBytes(0x0f, cc)
	off := len(g.code)
	g.emitU32(0) // placeholder
	return off
}

// jmpRel8 emits `jmp rel8`.
func (g *CodeGen) jmpRel8(off int8) {
	g.Flush()
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
				g.code[insPos] = 0xeb
				g.jumpFixups[i].Kind = jumpFixupJmpRel8
				g.jumpFixups[i].CodeOffset = insPos + 1
				// Delete trailing bytes of old rel32 encoding.
				g.code = append(g.code[:insPos+2], g.code[insPos+5:]...)
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
			g.code[insPos] = byte(0x70 | (fix.CC & 0x0f))
			g.jumpFixups[i].Kind = jumpFixupJccRel8
			g.jumpFixups[i].CodeOffset = insPos + 1
			// Delete trailing bytes of old near-jcc encoding.
			g.code = append(g.code[:insPos+2], g.code[insPos+6:]...)
			g.shiftOffsetsAfterDelete(insPos+1, 4)
			changed = true
			i++
		}
	}
}

// ret emits `ret`.
func (g *CodeGen) ret() {
	if g.target.GOOS == "dos" && g.wordSize == 4 {
		g.emitByte(0x66)
	}
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

func (g *CodeGen) Flush() {
	if len(g.cacheRegs) > 0 {
		if len(g.cacheStack) == 0 && !g.hasPending {
			return
		}
		for i, reg := range g.cacheStack {
			owner := -1
			if i < len(g.cacheOwn) {
				owner = g.cacheOwn[i]
			}
			g.rawPushOwned(reg, owner)
		}
		if g.hasPending {
			g.rawPushOwned(g.pendingReg, g.pendingOwn)
		}
		g.cacheStack = g.cacheStack[:0]
		g.cacheOwn = g.cacheOwn[:0]
		g.cacheFree = append(g.cacheFree[:0], g.cacheRegs...)
		g.hasPending = false
		g.pendingOwn = -1
		return
	}
	if !g.hasPending {
		return
	}
	g.hasPending = false
	g.rawPushOwned(g.pendingReg, g.pendingOwn)
	g.pendingOwn = -1
}

func (g *CodeGen) configureOperandCache(regs ...int) {
	g.cacheRegs = append(g.cacheRegs[:0], regs...)
	g.ClearOperandCache()
}

func (g *CodeGen) ClearOperandCache() {
	g.hasPending = false
	g.pendingOwn = -1
	g.cacheStack = g.cacheStack[:0]
	g.cacheOwn = g.cacheOwn[:0]
	g.cacheFree = append(g.cacheFree[:0], g.cacheRegs...)
}

func (g *CodeGen) moveReg(dst, src int) {
	if dst == src {
		return
	}
	if g.isArm64 {
		g.EmitMovRRArm64(dst, src)
		return
	}
	if g.wordSize == 2 {
		g.emitBytes(0x89, byte(0xc0|((src&7)<<3)|(dst&7)))
		return
	}
	if g.wordSize == 4 {
		if g.target.GOOS == "dos" {
			g.emitBytes(0x66, 0x89, byte(0xc0|((src&7)<<3)|(dst&7)))
		} else {
			g.emitBytes(0x89, byte(0xc0|((src&7)<<3)|(dst&7)))
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
	g.emitBytes(rex, 0x89, byte(0xc0|((src&7)<<3)|(dst&7)))
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
			owner := -1
			if len(g.cacheOwn) > 0 {
				owner = g.cacheOwn[0]
			}
			g.rawPushOwned(spill, owner)
			g.cacheStack = g.cacheStack[1:]
			if len(g.cacheOwn) > 0 {
				g.cacheOwn = g.cacheOwn[1:]
			}
			g.cacheFree = append(g.cacheFree, spill)
		}
		slot := len(g.cacheFree) - 1
		dst := g.cacheFree[slot]
		g.cacheFree = g.cacheFree[:slot]
		prev := g.traceForceInst(g.pendingOwn)
		g.moveReg(dst, g.pendingReg)
		g.traceRestoreForcedInst(prev)
		g.cacheStack = append(g.cacheStack, dst)
		g.cacheOwn = append(g.cacheOwn, g.pendingOwn)
		g.hasPending = false
		g.pendingOwn = -1
		return
	}
	g.Flush()
}

func (g *CodeGen) rawPush(reg int) {
	g.rawPushOwned(reg, g.traceOwner())
}

func (g *CodeGen) rawPushOwned(reg int, owner int) {
	prev := g.traceForceInst(owner)
	defer g.traceRestoreForcedInst(prev)
	if g.isArm64 {
		// SUB X28, X28, #8; STR Xreg, [X28]
		g.emitSubImm(REG_X28, REG_X28, 8)
		g.EmitStr(reg, REG_X28, 0)
		return
	}
	if g.wordSize == 4 {
		if g.target.GOOS == "dos" {
			g.emitBytes(0x66, 0x67, 0x8d, 0x7f, 0xfc)          // lea edi, [edi-4]
			g.emitBytes(0x66, 0x67, 0x89, byte(0x07|(reg<<3))) // mov [edi], reg
		} else {
			g.emitBytes(0x8d, 0x7f, 0xfc)          // lea edi, [edi-4] (preserves flags)
			g.emitBytes(0x89, byte(0x07|(reg<<3))) // mov [edi], reg
		}
	} else if g.wordSize == 2 {
		g.emitBytes(0x8d, 0x7d, 0xfe)          // lea di, [di-2]
		g.emitBytes(0x89, byte(0x05|(reg<<3))) // mov [di], reg16
	} else {
		g.emitBytes(0x4d, 0x8d, 0x7f, 0xf8) // lea r15, [r15-8] (preserves flags)
		rex := byte(0x49)
		if reg >= 8 {
			rex = 0x4d
		}
		g.emitBytes(rex, 0x89, byte(0x07|((reg&7)<<3)))
	}
}

func (g *CodeGen) rawPop(reg int) {
	if g.isArm64 {
		// LDR Xreg, [X28]; ADD X28, X28, #8
		g.emitLdr(reg, REG_X28, 0)
		g.emitAddImm(REG_X28, REG_X28, 8)
		return
	}
	if g.wordSize == 4 {
		if g.target.GOOS == "dos" {
			g.emitBytes(0x66, 0x67, 0x8b, byte(0x07|(reg<<3))) // mov reg, [edi]
			g.emitBytes(0x66, 0x67, 0x8d, 0x7f, 0x04)          // lea edi, [edi+4]
		} else {
			g.emitBytes(0x8b, byte(0x07|(reg<<3))) // mov reg, [edi]
			g.emitBytes(0x8d, 0x7f, 0x04)          // lea edi, [edi+4] (preserves flags)
		}
	} else if g.wordSize == 2 {
		g.emitBytes(0x8b, byte(0x05|(reg<<3))) // mov reg16, [di]
		g.emitBytes(0x8d, 0x7d, 0x02)          // lea di, [di+2]
	} else {
		rex := byte(0x49)
		if reg >= 8 {
			rex = 0x4d
		}
		g.emitBytes(rex, 0x8b, byte(0x07|((reg&7)<<3)))
		g.emitBytes(0x4d, 0x8d, 0x7f, 0x08) // lea r15, [r15+8] (preserves flags)
	}
}

func (g *CodeGen) rawLoad(reg int) {
	if g.isArm64 {
		g.emitLdr(reg, REG_X28, 0)
		return
	}
	if g.wordSize == 4 {
		if g.target.GOOS == "dos" {
			g.emitBytes(0x66, 0x67, 0x8b, byte(0x07|(reg<<3)))
		} else {
			g.emitBytes(0x8b, byte(0x07|(reg<<3)))
		}
	} else if g.wordSize == 2 {
		g.emitBytes(0x8b, byte(0x05|(reg<<3)))
	} else {
		rex := byte(0x49)
		if reg >= 8 {
			rex = 0x4d
		}
		g.emitBytes(rex, 0x8b, byte(0x07|((reg&7)<<3)))
	}
}

func (g *CodeGen) rawDrop() {
	if g.isArm64 {
		g.emitAddImm(REG_X28, REG_X28, 8)
		return
	}
	if g.wordSize == 4 {
		if g.target.GOOS == "dos" {
			g.emitBytes(0x66, 0x67, 0x83, 0xc7, 0x04)
		} else {
			g.emitBytes(0x83, 0xc7, 0x04)
		}
	} else if g.wordSize == 2 {
		g.emitBytes(0x83, 0xc7, 0x02)
	} else {
		g.emitBytes(0x49, 0x83, 0xc7, 0x08)
	}
}

func (g *CodeGen) opPush(reg int) {
	if len(g.cacheRegs) > 0 {
		if g.hasPending {
			if len(g.cacheFree) == 0 {
				spill := g.cacheStack[0]
				owner := -1
				if len(g.cacheOwn) > 0 {
					owner = g.cacheOwn[0]
				}
				g.rawPushOwned(spill, owner)
				g.cacheStack = g.cacheStack[1:]
				if len(g.cacheOwn) > 0 {
					g.cacheOwn = g.cacheOwn[1:]
				}
				g.cacheFree = append(g.cacheFree, spill)
			}
			slot := len(g.cacheFree) - 1
			dst := g.cacheFree[slot]
			g.cacheFree = g.cacheFree[:slot]
			prev := g.traceForceInst(g.pendingOwn)
			g.moveReg(dst, g.pendingReg)
			g.traceRestoreForcedInst(prev)
			g.cacheStack = append(g.cacheStack, dst)
			g.cacheOwn = append(g.cacheOwn, g.pendingOwn)
		}
		g.hasPending = true
		g.pendingReg = reg
		g.pendingOwn = g.traceOwner()
		return
	}
	g.Flush()
	g.hasPending = true
	g.pendingReg = reg
	g.pendingOwn = g.traceOwner()
}

func (g *CodeGen) opPop(reg int) {
	if len(g.cacheRegs) > 0 {
		if g.hasPending {
			g.hasPending = false
			g.pendingOwn = -1
			g.moveReg(reg, g.pendingReg)
			return
		}
		if len(g.cacheStack) > 0 {
			last := len(g.cacheStack) - 1
			src := g.cacheStack[last]
			g.cacheStack = g.cacheStack[:last]
			if len(g.cacheOwn) > last {
				g.cacheOwn = g.cacheOwn[:last]
			}
			g.cacheFree = append(g.cacheFree, src)
			g.moveReg(reg, src)
			return
		}
		g.rawPop(reg)
		return
	}
	if g.hasPending {
		g.hasPending = false
		g.pendingOwn = -1
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

func (g *CodeGen) opStore(reg int) {
	g.Flush()
	if g.isArm64 {
		g.EmitStr(reg, REG_X28, 0)
		return
	}
	if g.wordSize == 4 {
		if g.target.GOOS == "dos" {
			g.emitBytes(0x66, 0x67, 0x89, byte(0x07|(reg<<3)))
		} else {
			g.emitBytes(0x89, byte(0x07|(reg<<3)))
		}
	} else if g.wordSize == 2 {
		g.emitBytes(0x89, byte(0x05|(reg<<3)))
	} else {
		rex := byte(0x49)
		if reg >= 8 {
			rex = 0x4d
		}
		g.emitBytes(rex, 0x89, byte(0x07|((reg&7)<<3)))
	}
}

func (g *CodeGen) opDrop() {
	if len(g.cacheRegs) > 0 {
		if g.hasPending {
			g.hasPending = false
			g.pendingOwn = -1
			return
		}
		if len(g.cacheStack) > 0 {
			last := len(g.cacheStack) - 1
			g.cacheFree = append(g.cacheFree, g.cacheStack[last])
			g.cacheStack = g.cacheStack[:last]
			if len(g.cacheOwn) > last {
				g.cacheOwn = g.cacheOwn[:last]
			}
			return
		}
		g.rawDrop()
		return
	}
	if g.hasPending {
		g.hasPending = false
		g.pendingOwn = -1
		return
	}
	g.rawDrop()
}

func (g *CodeGen) opDropN(count int) {
	if count <= 0 {
		return
	}
	if len(g.cacheRegs) > 0 {
		i := 0
		for i < count {
			g.opDrop()
			i++
		}
		return
	}
	if g.hasPending {
		g.hasPending = false
		g.pendingOwn = -1
		count--
		if count <= 0 {
			return
		}
	}
	if g.isArm64 {
		bytes := count * 8
		for bytes > 0 {
			chunk := bytes
			if chunk > 4095 {
				chunk = 4095
			}
			g.emitAddImm(REG_X28, REG_X28, uint32(chunk))
			bytes = bytes - chunk
		}
		return
	}
	i := 0
	for i < count {
		g.rawDrop()
		i++
	}
}

// === ARM64 GOT helpers ===

// gotSlot returns the GOT slot index for a libSystem symbol, allocating one if needed.
func (g *CodeGen) gotSlot(name string) int {
	if idx, ok := g.gotEntries[name]; ok {
		return idx
	}
	idx := len(g.gotSymbols)
	g.gotEntries[name] = idx
	g.gotSymbols = append(g.gotSymbols, name)
	return idx
}

// EmitCallGOT emits a GOT-indirect call: load address from GOT, branch via BLR.
// Uses X16 as scratch (IP0, caller-saved).
func (g *CodeGen) EmitCallGOT(funcName string) {
	g.Flush()
	slot := g.gotSlot(funcName)
	// ADRP+LDR loads the function pointer from the GOT entry
	g.emitAdrpLdr(REG_X16, "$got_addr$", uint64(slot*8))
	// BLR X16
	g.EmitBlr(REG_X16)
}

// EmitCallPlaceholderArm64 emits a BL with placeholder for later fixup.
func (g *CodeGen) EmitCallPlaceholderArm64(target string) {
	g.Flush()
	g.callFixups = append(g.callFixups, CallFixup{len(g.code), target, 0})
	g.EmitArm64(0x94000000) // BL #0 (placeholder)
}

// emitCallIAT emits `call dword ptr [abs32]` for calling Windows IAT entries.
func (g *CodeGen) emitCallIAT(funcName string) {
	g.Flush()
	g.emitBytes(0xFF, 0x15) // call dword ptr [abs32]
	g.callFixups = append(g.callFixups, CallFixup{len(g.code), "$iat$" + funcName, 0})
	g.emitU32(0) // placeholder
}

// emitJmpIAT emits `jmp dword ptr [abs32]` for jumping to Windows IAT entries.
func (g *CodeGen) emitJmpIAT(funcName string) {
	g.Flush()
	g.emitBytes(0xFF, 0x25) // jmp dword ptr [abs32]
	g.callFixups = append(g.callFixups, CallFixup{len(g.code), "$iat$" + funcName, 0})
	g.emitU32(0) // placeholder
}

// alignUp aligns v up to the next multiple of align.
