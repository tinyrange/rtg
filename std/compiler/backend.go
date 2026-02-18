package main

import "fmt"

// === Backend: IRModule → ELF binary ===

// CodeGen holds state for generating machine code from IR.
type CodeGen struct {
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

	// Global variable info: global index → offset in .data
	globalOffsets []int

	// Current function being compiled
	curFunc *IRFunc

	// Number of locals (slots) in current function frame
	curFrameSize int

	// ELF layout constants
	baseAddr  uint64
	textStart uint64 // offset in file where .text begins

	// Interface dispatch data from IR
	irmod *IRModule

	// Pending push optimization: tracks a push that hasn't been emitted yet
	hasPending bool
	pendingReg int
	cacheRegs  []int
	cacheFree  []int
	cacheStack []int

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

// dispatchEntry pairs a type ID with a method function name for interface dispatch.
type dispatchEntry struct {
	typeID   int
	funcName string
}

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

// GenerateELF dispatches to the appropriate backend based on selected target.
func GenerateELF(irmod *IRModule, outputPath string) error {
	if targetBackend == "vm" {
		return generateVM(irmod, outputPath)
	}
	if targetBackend == "c" {
		return generateCSource(irmod, outputPath)
	}
	if targetBackend == "ir" {
		return generateIRText(irmod, outputPath)
	}
	switch targetGOARCH {
	case "dos16":
		if targetGOOS == "dos" {
			return generateDOSCOM386(irmod, outputPath)
		}
		return fmt.Errorf("unsupported OS for dos16: %s", targetGOOS)
	case "amd64":
		if targetGOOS == "windows" {
			return generateWinAmd64PE(irmod, outputPath)
		}
		return generateAmd64ELF(irmod, outputPath)
	case "386":
		if targetGOOS == "windows" {
			return generateWin386PE(irmod, outputPath)
		}
		if targetGOOS == "dos" {
			return generateDOSCOM386(irmod, outputPath)
		}
		return generateI386ELF(irmod, outputPath)
	case "wasm32":
		return generateWasm32(irmod, outputPath)
	case "arm64":
		if targetGOOS == "darwin" {
			return generateDarwinArm64(irmod, outputPath)
		}
		if targetGOOS == "linux" {
			return generateLinuxArm64ELF(irmod, outputPath)
		}
		if targetGOOS == "windows" {
			return generateWinArm64PE(irmod, outputPath)
		}
		return fmt.Errorf("unsupported OS for arm64: %s", targetGOOS)
	default:
		return fmt.Errorf("unsupported target architecture: %s", targetGOARCH)
	}
}

// isInitFunc checks if a function name is a package init function.
func isInitFunc(name string) bool {
	n := len(name)
	if n < 5 {
		return false
	}
	// Match both ".init" and ".init$globals"
	if n >= 13 && name[n-13:n] == ".init$globals" {
		return true
	}
	return name[n-5:n] == ".init"
}

// === Shared byte emission ===

func (g *CodeGen) emitByte(b byte) {
	g.code = append(g.code, b)
}

func (g *CodeGen) emitBytes(bytes ...byte) {
	g.code = append(g.code, bytes...)
}

func (g *CodeGen) emitU32(v uint32) {
	g.code = append(g.code, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}

func (g *CodeGen) emitU16(v uint16) {
	g.code = append(g.code, byte(v), byte(v>>8))
}

func (g *CodeGen) emitU64(v uint64) {
	g.code = append(g.code, byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
		byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56))
}

func (g *CodeGen) emitRodataU64(v uint64) {
	g.rodata = append(g.rodata, byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
		byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56))
}

func (g *CodeGen) emitRodataU32(v uint32) {
	g.rodata = append(g.rodata, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
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
	if targetGOOS == "dos" && g.wordSize == 2 {
		g.emitBytes(0xe8) // call rel16
		g.callFixups = append(g.callFixups, CallFixup{
			CodeOffset: len(g.code),
			Target:     target,
		})
		g.emitU16(0)
		return
	}
	if targetGOOS == "dos" && g.wordSize == 4 {
		g.emitByte(0x66) // call rel32 in 16-bit mode
	}
	g.emitBytes(0xe8) // call rel32
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: len(g.code),
		Target:     target,
	})
	g.emitU32(0) // placeholder
}

// patchRel32At patches the rel32 at fixupOff to jump to targetOff.
func (g *CodeGen) patchRel32At(fixupOff int, targetOff int) {
	if targetGOOS == "dos" && g.wordSize == 2 {
		rel := int16(targetOff - (fixupOff + 2))
		g.code[fixupOff] = byte(rel)
		g.code[fixupOff+1] = byte(rel >> 8)
		return
	}
	rel := int32(targetOff - (fixupOff + 4))
	g.code[fixupOff] = byte(rel)
	g.code[fixupOff+1] = byte(rel >> 8)
	g.code[fixupOff+2] = byte(rel >> 16)
	g.code[fixupOff+3] = byte(rel >> 24)
}

// patchRel32 patches the rel32 at fixupOff to jump to the current code position.
func (g *CodeGen) patchRel32(fixupOff int) {
	target := len(g.code)
	if targetGOOS == "dos" && g.wordSize == 2 {
		rel := int16(target - (fixupOff + 2))
		g.code[fixupOff] = byte(rel)
		g.code[fixupOff+1] = byte(rel >> 8)
		return
	}
	rel := int32(target - (fixupOff + 4))
	g.code[fixupOff] = byte(rel)
	g.code[fixupOff+1] = byte(rel >> 8)
	g.code[fixupOff+2] = byte(rel >> 16)
	g.code[fixupOff+3] = byte(rel >> 24)
}

// jmpRel32 emits `jmp rel32` and returns the offset of the rel32 for fixup.
func (g *CodeGen) jmpRel32() int {
	g.flush()
	if targetGOOS == "dos" && g.wordSize == 2 {
		g.emitByte(0xe9) // jmp rel16
		off := len(g.code)
		g.emitU16(0)
		return off
	}
	if targetGOOS == "dos" && g.wordSize == 4 {
		g.emitByte(0x66) // jmp rel32 in 16-bit mode
	}
	g.emitByte(0xe9)
	off := len(g.code)
	g.emitU32(0) // placeholder
	return off
}

// jccRel32 emits `jCC rel32` (0x0f, cc) and returns the offset of the rel32.
func (g *CodeGen) jccRel32(cc byte) int {
	g.flush()
	if targetGOOS == "dos" && g.wordSize == 2 {
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
	if targetGOOS == "dos" && g.wordSize == 4 {
		g.emitByte(0x66) // jcc rel32 in 16-bit mode
	}
	g.emitBytes(0x0f, cc)
	off := len(g.code)
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
	if targetGOOS == "dos" && g.wordSize == 4 {
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

func (g *CodeGen) flush() {
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
	g.clearOperandCache()
}

func (g *CodeGen) clearOperandCache() {
	g.hasPending = false
	g.cacheStack = g.cacheStack[:0]
	g.cacheFree = append(g.cacheFree[:0], g.cacheRegs...)
}

func (g *CodeGen) moveReg(dst, src int) {
	if dst == src {
		return
	}
	if g.isArm64 {
		g.emitMovRRArm64(dst, src)
		return
	}
	if g.wordSize == 2 {
		g.emitBytes(0x89, byte(0xc0|((src&7)<<3)|(dst&7)))
		return
	}
	if g.wordSize == 4 {
		if targetGOOS == "dos" {
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
	g.flush()
}

func (g *CodeGen) rawPush(reg int) {
	if g.isArm64 {
		// SUB X28, X28, #8; STR Xreg, [X28]
		g.emitSubImm(REG_X28, REG_X28, 8)
		g.emitStr(reg, REG_X28, 0)
		return
	}
	if g.wordSize == 4 {
		if targetGOOS == "dos" {
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
		if targetGOOS == "dos" {
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
		if targetGOOS == "dos" {
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
		if targetGOOS == "dos" {
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
	g.flush()
	g.hasPending = true
	g.pendingReg = reg
}

func (g *CodeGen) opPop(reg int) {
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
		g.flush()
		return
	}
	g.rawLoad(reg)
}

func (g *CodeGen) opStore(reg int) {
	g.flush()
	if g.isArm64 {
		g.emitStr(reg, REG_X28, 0)
		return
	}
	if g.wordSize == 4 {
		if targetGOOS == "dos" {
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

// emitCallGOT emits a GOT-indirect call: load address from GOT, branch via BLR.
// Uses X16 as scratch (IP0, caller-saved).
func (g *CodeGen) emitCallGOT(funcName string) {
	g.flush()
	slot := g.gotSlot(funcName)
	// ADRP+LDR loads the function pointer from the GOT entry
	g.emitAdrpLdr(REG_X16, "$got_addr$", uint64(slot*8))
	// BLR X16
	g.emitBlr(REG_X16)
}

// emitCallPlaceholderArm64 emits a BL with placeholder for later fixup.
func (g *CodeGen) emitCallPlaceholderArm64(target string) {
	g.flush()
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: len(g.code),
		Target:     target,
	})
	g.emitArm64(0x94000000) // BL #0 (placeholder)
}

// emitCallIAT emits `call dword ptr [abs32]` for calling Windows IAT entries.
func (g *CodeGen) emitCallIAT(funcName string) {
	g.flush()
	g.emitBytes(0xFF, 0x15) // call dword ptr [abs32]
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: len(g.code),
		Target:     "$iat$" + funcName,
	})
	g.emitU32(0) // placeholder
}

// emitJmpIAT emits `jmp dword ptr [abs32]` for jumping to Windows IAT entries.
func (g *CodeGen) emitJmpIAT(funcName string) {
	g.flush()
	g.emitBytes(0xFF, 0x25) // jmp dword ptr [abs32]
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: len(g.code),
		Target:     "$iat$" + funcName,
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

// === String literal helpers ===

// decodeStringLiteral processes escape sequences in a string literal.
func decodeStringLiteral(s string) string {
	var result []byte
	i := 0
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				result = append(result, '\n')
			case 't':
				result = append(result, '\t')
			case 'r':
				result = append(result, '\r')
			case '\\':
				result = append(result, '\\')
			case '"':
				result = append(result, '"')
			case '\'':
				result = append(result, '\'')
			case '0':
				result = append(result, 0)
			case 'x':
				if i+3 < len(s) {
					hi := unhex(s[i+2])
					lo := unhex(s[i+3])
					result = append(result, byte(hi<<4|lo))
					i = i + 4
					continue
				}
				result = append(result, s[i+1])
			default:
				result = append(result, s[i+1])
			}
			i = i + 2
		} else {
			result = append(result, s[i])
			i++
		}
	}
	return string(result)
}

func unhex(c byte) int {
	if c >= '0' && c <= '9' {
		return int(c - '0')
	}
	if c >= 'a' && c <= 'f' {
		return int(c - 'a' + 10)
	}
	if c >= 'A' && c <= 'F' {
		return int(c - 'A' + 10)
	}
	return 0
}
