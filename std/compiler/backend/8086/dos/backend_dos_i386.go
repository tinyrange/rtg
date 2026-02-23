//go:build !no_backend_dos_i386

// Package dos: minimal IR → DOS MZ .EXE backend for 8086 real mode.
// Major refactor: single file, 16-bit only, DOS EXE small-model, no ELF/Mach-O/PE, no i386/amd64,
// no operand-cache optimizer, no jump relaxation, no Windows IAT.
//
// Assumptions (match your existing backend conventions):
// - Near pointers are 16-bit offsets inside the data segment (DS).
// - Operand stack lives in DI and grows downward in memory (DI -= 2 on push).
// - Frame uses BP-based locals. Locals are 1-word slots. Slot 0 is at [BP-2].
// - Params are passed on the operand stack; function prologue pops params into locals.
//
// Strings in rodata are stored as: [bytes...][header{data_ptr:u16, len:u16}]
// and code pushes the address of the header.
//
// Syscall intrinsic follows your shim mapping:
//
//	3   -> read(fd, buf, count) via INT 21h AH=3Fh
//	4   -> write(fd, buf, count) via INT 21h AH=40h
//	5   -> open/create via INT 21h AH=3Dh/3Ch
//	6   -> close(fd) via INT 21h AH=3Eh
//	252 -> exit(code) via INT 21h AH=4Ch
//	192 -> mmap(...) stub returning a fixed near heap base (0x7000)
//
// NOTE: This file still depends on your existing IR and helpers (DecodeStringLiteral, etc.).
package dos

import (
	"os"

	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

const (
	segLimitU32   = uint32(0x10000)
	mzHeaderParas = 4
	mzHeaderBytes = mzHeaderParas * 16
)

const outlinedTostringHelper = "$rtg.tostring$"

// ===== CodeGen state (16-bit only) =====

type CodeGen struct {
	target *common.Target
	irmod  *ir.IRModule

	code   []byte // .text
	rodata []byte // string bytes + headers
	data   []byte // globals (word array)

	funcOffsets   map[string]int
	callFixups    []CallFixup
	labelOffsets  map[int]int
	jumpFixups    []JumpFixup
	stringMap     map[string]int // decoded string -> header offset in rodata
	globalOffsets []int
	dataSegFixups []int

	curFrameWords int // locals/params frame size in words (not bytes)

	needTostringHelper bool
	hasTostringHelper  bool
}

type CallFixup struct {
	CodeOffset int    // offset of immediate/rel field inside g.code
	Target     string // function name or special "$rodata_header$" / "$data_addr$"
}

type JumpFixup struct {
	CodeOffset int // offset of rel16 field inside g.code
	LabelID    int
	Kind       int
	CC         byte // low-nibble condition for Jcc (0..15)
}

const (
	jumpFixupJmpRel16 = iota
	jumpFixupJccRel16 // lowered Jcc: j!cc +3; jmp rel16 (fixup points to rel16)
)

// ===== 8086 registers / CC =====

const (
	REG16_AX = 0
	REG16_CX = 1
	REG16_DX = 2
	REG16_BX = 3
	REG16_SP = 4
	REG16_BP = 5
	REG16_SI = 6
	REG16_DI = 7
)

// Condition code low-nibble values used by short Jcc (0x70+cc).
const (
	CC16_E  = 0x4
	CC16_NE = 0x5
	CC16_L  = 0xC
	CC16_GE = 0xD
	CC16_LE = 0xE
	CC16_G  = 0xF
	CC16_AE = 0x3 // JAE / JNC
	CC16_C  = 0x2 // JC
)

// 16-bit addressing r/m forms (ModR/M rm field).
const (
	EA16_BX_SI = 0
	EA16_BX_DI = 1
	EA16_BP_SI = 2
	EA16_BP_DI = 3
	EA16_SI    = 4
	EA16_DI    = 5
	EA16_BP    = 6 // note: mod=00,rm=110 means [disp16], so [BP] needs mod!=00
	EA16_BX    = 7
)

// ===== Public entry =====

// GenerateDOSCOM compiles an IRModule to a DOS MZ .EXE image (8086 real-mode).
func GenerateDOSCOM(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	g := &CodeGen{
		target:        target,
		irmod:         irmod,
		funcOffsets:   make(map[string]int),
		labelOffsets:  make(map[int]int),
		stringMap:     make(map[string]int),
		globalOffsets: make([]int, len(irmod.Globals)),
	}

	// Globals are a packed word array.
	for i := range irmod.Globals {
		g.globalOffsets[i] = i * 2
	}
	g.data = make([]byte, len(irmod.Globals)*2)

	g.emitEXEStart(irmod)

	for _, f := range irmod.Funcs {
		g.funcOffsets[f.Name] = len(g.code)
		g.compileFunc(f)
	}
	ir.CollectNativeFuncSizes(irmod, g.funcOffsets, len(g.code))
	if g.needTostringHelper {
		g.emitTostringHelper()
	}

	// Resolve normal call rel16 fixups (functions only).
	var unresolved []string
	for _, fix := range g.callFixups {
		if fix.Target == "$rodata_header$" || fix.Target == "$data_addr$" {
			continue
		}
		off, ok := g.funcOffsets[fix.Target]
		if !ok {
			unresolved = append(unresolved, fix.Target)
			continue
		}
		g.patchRel16At(fix.CodeOffset, off)
	}
	if len(unresolved) > 0 {
		reportUnresolvedCalls(unresolved)
		return errUnresolvedCalls(len(unresolved))
	}

	exe, err := g.buildEXE()
	if err != nil {
		writeSizeAnalysisTarget(*target)
		return err
	}
	if err := os.WriteFile(outputPath, exe, 0755); err != nil {
		return errWriteOutput(err)
	}
	return nil
}

// ===== EXE layout / patching =====

func (g *CodeGen) buildEXE() ([]byte, error) {
	textSize := uint32(len(g.code))
	rodataSize := uint32(len(g.rodata))
	dataSize := uint32(len(g.data))

	if exeSegmentTooLarge(textSize, rodataSize+dataSize) {
		total := int(textSize + rodataSize + dataSize)
		return nil, errCOMTooLarge(total, int(segLimitU32-1), int(textSize), int(rodataSize), int(dataSize))
	}

	dataBlobSize := rodataSize + dataSize
	dataBlob := make([]byte, int(dataBlobSize))
	copy(dataBlob, g.rodata)
	copy(dataBlob[int(rodataSize):], g.data)

	// In EXE small-model, rodata/data live in DS with offset-based pointers.
	for _, headerOff := range g.stringMap {
		dataOff := getU16(dataBlob[headerOff : headerOff+2])
		putU16(dataBlob[headerOff:headerOff+2], dataOff)
	}

	// Module image is code followed by paragraph-aligned data segment payload.
	codeParas := (textSize + 15) >> 4
	dataStart := codeParas << 4
	moduleSize := dataStart + dataBlobSize
	module := make([]byte, int(moduleSize))
	copy(module, g.code)
	copy(module[int(dataStart):], dataBlob)

	dataSegRel := uint16(codeParas)
	for _, off := range g.dataSegFixups {
		putU16(module[off:off+2], dataSegRel)
	}

	// Patch code immediates referencing rodata header/data base.
	for _, fix := range g.callFixups {
		switch fix.Target {
		case "$rodata_header$":
			off := getU16(module[fix.CodeOffset : fix.CodeOffset+2])
			putU16(module[fix.CodeOffset:fix.CodeOffset+2], off)
		case "$data_addr$":
			off := getU16(module[fix.CodeOffset : fix.CodeOffset+2])
			putU16(module[fix.CodeOffset:fix.CodeOffset+2], uint16(rodataSize)+off)
		}
	}

	// Build minimal MZ EXE header (no relocations).
	fileSize := mzHeaderBytes + moduleSize
	pages := (fileSize + 511) / 512
	last := fileSize % 512
	if last == 0 {
		last = 512
	}
	header := make([]byte, mzHeaderBytes)
	putU16(header[0:2], 0x5A4D)                // MZ
	putU16(header[2:4], uint16(last))          // e_cblp
	putU16(header[4:6], uint16(pages))         // e_cp
	putU16(header[6:8], 0)                     // e_crlc
	putU16(header[8:10], mzHeaderParas)        // e_cparhdr
	putU16(header[10:12], 0x0010)              // e_minalloc
	putU16(header[12:14], 0xFFFF)              // e_maxalloc
	putU16(header[14:16], dataSegRel)          // e_ss
	putU16(header[16:18], 0xFFFE)              // e_sp
	putU16(header[18:20], 0)                   // e_csum
	putU16(header[20:22], 0x0000)              // e_ip
	putU16(header[22:24], 0x0000)              // e_cs
	putU16(header[24:26], 0x0040)              // e_lfarlc
	putU16(header[26:28], 0)                   // e_ovno

	out := make([]byte, fileSize)
	copy(out, header)
	copy(out[mzHeaderBytes:], module)
	return out, nil
}

// Minimal EXE entry:
// - Initialize DS/ES = CS + data segment relative paragraph.
// - Set operand stack DI near top of segment.
// - Call init funcs.
// - Call main.main.
// - Exit via INT 21h AH=4Ch.
func (g *CodeGen) emitEXEStart(irmod *ir.IRModule) {
	g.emitBytes(0x8C, 0xC8) // mov ax, cs
	g.emitBytes(0x05)       // add ax, imm16
	g.dataSegFixups = append(g.dataSegFixups, len(g.code))
	g.emitU16(0)
	g.emitBytes(0x8E, 0xD8) // mov ds, ax
	g.emitBytes(0x8E, 0xC0) // mov es, ax

	// Keep operand stack well below CPU call stack (SP starts near FFFE).
	g.emitMovImm16(REG16_DI, 0xC000)

	for _, f := range irmod.Funcs {
		if ir.IsInitFunc(f.Name) {
			g.emitCallPlaceholder(f.Name)
		}
	}
	g.emitCallPlaceholder("main.main")

	g.emitMovImm16(REG16_AX, 0x4C00) // AH=4Ch, AL=0
	g.emitBytes(0xCD, 0x21)          // int 21h
}

// ===== Instruction selection =====

func (g *CodeGen) compileFunc(f *ir.IRFunc) {
	// Frame words = max(locals, params).
	g.curFrameWords = len(f.Locals)
	if f.Params > g.curFrameWords {
		g.curFrameWords = f.Params
	}

	g.labelOffsets = make(map[int]int)
	g.jumpFixups = g.jumpFixups[:0]

	// Prologue: push bp; mov bp, sp; sub sp, frameBytes
	g.pushR16(REG16_BP)
	g.movRR16(REG16_BP, REG16_SP)

	frameBytes := g.curFrameWords * 2
	if frameBytes > 0 {
		g.subImm16(REG16_SP, int16(frameBytes))
	}

	// Pop params from operand stack (DI) into locals (reverse order).
	for i := f.Params - 1; i >= 0; i-- {
		g.opPop(REG16_AX)
		off := (i + 1) * 2
		g.storeLocal(off, REG16_AX)
	}

	for _, inst := range f.Code {
		g.compileInst(inst)
	}

	// Resolve jumps in this function.
	for _, fix := range g.jumpFixups {
		labelOff, ok := g.labelOffsets[fix.LabelID]
		if !ok {
			continue
		}
		switch fix.Kind {
		case jumpFixupJmpRel16, jumpFixupJccRel16:
			g.patchRel16At(fix.CodeOffset, labelOff)
		}
	}
}

// ===== Constants / locals / globals =====

func (g *CodeGen) compileConst(v int16) {
	if v == 0 {
		g.xorRR16(REG16_AX, REG16_AX)
	} else {
		g.emitMovImm16(REG16_AX, uint16(v))
	}
	g.opPush(REG16_AX)
}

func (g *CodeGen) localGet(idx int) {
	off := (idx + 1) * 2
	g.loadLocal(off, REG16_AX)
	g.opPush(REG16_AX)
}

func (g *CodeGen) localSet(idx int) {
	off := (idx + 1) * 2
	g.opPop(REG16_AX)
	g.storeLocal(off, REG16_AX)
}

func (g *CodeGen) localAddImm(idx int, imm int16) {
	off := (idx + 1) * 2
	g.loadLocal(off, REG16_AX)
	if imm != 0 {
		g.addImm16(REG16_AX, imm)
	}
	g.storeLocal(off, REG16_AX)
}

func (g *CodeGen) localAddr(idx int) {
	off := (idx + 1) * 2
	g.leaLocal(off, REG16_AX)
	g.opPush(REG16_AX)
}

func (g *CodeGen) globalGet(gidx int) {
	// bx = data_base + gidx*2 (data base patched later)
	g.emitMovImm16(REG16_BX, uint16(gidx*2))
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: len(g.code) - 2,
		Target:     "$data_addr$",
	})
	g.emitLoadRM16(REG16_AX, EA16_BX, 0)
	g.opPush(REG16_AX)
}

func (g *CodeGen) globalSet(gidx int) {
	g.opPop(REG16_AX)
	g.emitMovImm16(REG16_BX, uint16(gidx*2))
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: len(g.code) - 2,
		Target:     "$data_addr$",
	})
	g.emitStoreRM16(EA16_BX, 0, REG16_AX)
}

func (g *CodeGen) globalAddr(gidx int) {
	g.emitMovImm16(REG16_AX, uint16(gidx*2))
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: len(g.code) - 2,
		Target:     "$data_addr$",
	})
	g.opPush(REG16_AX)
}

// ===== Arithmetic / compare / branches =====

func (g *CodeGen) binOp(op ir.Opcode) {
	g.opPop(REG16_AX) // rhs
	g.opPop(REG16_CX) // lhs

	switch op {
	case ir.OP_ADD:
		g.addRR16(REG16_CX, REG16_AX)
	case ir.OP_SUB:
		g.subRR16(REG16_CX, REG16_AX)
	case ir.OP_AND:
		g.andRR16(REG16_CX, REG16_AX)
	case ir.OP_OR:
		g.orRR16(REG16_CX, REG16_AX)
	case ir.OP_XOR:
		g.xorRR16(REG16_CX, REG16_AX)

	case ir.OP_SHL:
		// lhs in CX, shift count in AX -> use CL.
		g.movRR16(REG16_DX, REG16_CX) // save lhs in DX
		g.movRR16(REG16_CX, REG16_AX) // count in CX
		g.movRR16(REG16_AX, REG16_DX) // value in AX
		g.shlCl16(REG16_AX)
		g.movRR16(REG16_CX, REG16_AX)

	case ir.OP_SHR:
		// arithmetic shift right
		g.movRR16(REG16_DX, REG16_CX)
		g.movRR16(REG16_CX, REG16_AX)
		g.movRR16(REG16_AX, REG16_DX)
		g.sarCl16(REG16_AX)
		g.movRR16(REG16_CX, REG16_AX)

	case ir.OP_MUL:
		// Signed multiply: AX = lhs; IMUL rhs; keep low word in AX
		g.movRR16(REG16_DX, REG16_AX) // rhs
		g.movRR16(REG16_AX, REG16_CX) // lhs
		g.imulR16(REG16_DX)
		g.movRR16(REG16_CX, REG16_AX)

	case ir.OP_DIV:
		// Signed divide lhs / rhs.
		// AX = lhs, DX = sign-extend, IDIV rhs -> AX=quot.
		// Keep rhs out of DX because CWD overwrites DX.
		g.movRR16(REG16_BX, REG16_AX) // rhs
		g.movRR16(REG16_AX, REG16_CX) // lhs
		g.cwd16()
		g.idivR16(REG16_BX)
		g.movRR16(REG16_CX, REG16_AX)

	case ir.OP_MOD:
		// Signed mod lhs % rhs -> DX = rem
		// Keep rhs out of DX because CWD overwrites DX.
		g.movRR16(REG16_BX, REG16_AX) // rhs
		g.movRR16(REG16_AX, REG16_CX) // lhs
		g.cwd16()
		g.idivR16(REG16_BX)
		g.movRR16(REG16_CX, REG16_DX)
	}

	g.opPush(REG16_CX)
}

func (g *CodeGen) compareToBool(cc byte) {
	g.opPop(REG16_AX)
	g.opPop(REG16_CX)
	g.cmpRR16(REG16_CX, REG16_AX)

	// Produce 0/1 in CX with branches (8086 has no SETcc).
	// Do not clobber flags before branching.
	fixTrue := g.jccNearRel16(cc)
	g.emitMovImm16(REG16_CX, 0)
	fixDone := g.jmpRel16()
	g.patchRel16(fixTrue)
	g.emitMovImm16(REG16_CX, 1)
	g.patchRel16(fixDone)
	g.opPush(REG16_CX)
}

func (g *CodeGen) compareJump(cc byte, label int) {
	g.opPop(REG16_AX)
	g.opPop(REG16_CX)
	g.cmpRR16(REG16_CX, REG16_AX)

	off := g.jccNearRel16(cc)
	g.jumpFixups = append(g.jumpFixups, JumpFixup{
		CodeOffset: off,
		LabelID:    label,
		Kind:       jumpFixupJccRel16,
		CC:         cc,
	})
}

// ===== Calls / return =====

func (g *CodeGen) emitCallPlaceholder(target string) {
	g.emitByte(0xE8) // call rel16
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: len(g.code),
		Target:     target,
	})
	g.emitU16(0)
}

func (g *CodeGen) patchRel16At(fixupOff int, targetOff int) {
	rel := int16(targetOff - (fixupOff + 2))
	putU16(g.code[fixupOff:fixupOff+2], uint16(rel))
}

func (g *CodeGen) patchRel16(fixupOff int) {
	g.patchRel16At(fixupOff, len(g.code))
}

func (g *CodeGen) jmpRel16() int {
	g.emitByte(0xE9)
	off := len(g.code)
	g.emitU16(0)
	return off
}

// jccNearRel16 lowers a conditional branch to a near target on 8086:
//
//	j!cc +3
//	jmp rel16
//
// returns offset of rel16 field in the jmp instruction.
func (g *CodeGen) jccNearRel16(cc byte) int {
	inv := (cc & 0x0F) ^ 0x01
	g.emitBytes(byte(0x70|inv), 0x03, 0xE9)
	off := len(g.code)
	g.emitU16(0)
	return off
}

// ===== Memory / pointer ops =====

func (g *CodeGen) memLoad(size int) {
	g.opPop(REG16_BX) // addr
	g.testRR16(REG16_BX, REG16_BX)

	if size == 1 {
		// if nil -> push 0
		g.emitBytes(0x75, 0x04)       // jnz +4
		g.xorRR16(REG16_AX, REG16_AX) // ax=0
		g.emitBytes(0xEB, 0x04)       // jmp +4
		// mov al, [bx]
		g.emitBytes(0x30, 0xE4) // xor ah, ah
		g.emitBytes(0x8A, 0x07) // mov al, [bx]
	} else {
		// if nil -> push 0
		g.emitBytes(0x75, 0x04)       // jnz +4
		g.xorRR16(REG16_AX, REG16_AX) // ax=0
		g.emitBytes(0xEB, 0x02)       // jmp +2
		g.emitLoadRM16(REG16_AX, EA16_BX, 0)
	}
	g.opPush(REG16_AX)
}

func (g *CodeGen) memStore(size int) {
	g.opPop(REG16_BX) // addr
	g.opPop(REG16_AX) // value
	if size == 1 {
		// mov [bx], al
		g.emitBytes(0x88, 0x07)
	} else {
		g.emitStoreRM16(EA16_BX, 0, REG16_AX)
	}
}

func (g *CodeGen) offset(delta int) {
	g.opPop(REG16_AX)
	if delta != 0 {
		g.addImm16(REG16_AX, int16(delta))
	}
	g.opPush(REG16_AX)
}

func (g *CodeGen) indexAddr(elemSize int) {
	// stack: sliceHeaderPtr, index -> address
	g.opPop(REG16_AX) // index
	g.opPop(REG16_BX) // slice header ptr
	// header layout used by your intrinsics: [0]=data_ptr, [2]=len, [4]=cap, [6]=elem_size
	g.emitLoadRM16(REG16_BX, EA16_BX, 0) // BX = data_ptr

	if elemSize == 1 {
		g.addRR16(REG16_BX, REG16_AX)
	} else if elemSize == 2 {
		g.shlImm16(REG16_AX, 1)
		g.addRR16(REG16_BX, REG16_AX)
	} else {
		// Generic: BX += AX * elemSize (slow but correct on 8086).
		// Multiply via repeated add (elemSize is compile-time constant).
		g.pushR16(REG16_CX)
		g.movRR16(REG16_CX, REG16_AX) // CX = index
		g.xorRR16(REG16_AX, REG16_AX) // AX = 0 (accumulator)
		for i := 0; i < elemSize; i++ {
			g.addRR16(REG16_AX, REG16_CX)
		}
		g.addRR16(REG16_BX, REG16_AX)
		g.popR16(REG16_CX)
	}

	g.opPush(REG16_BX)
}

func (g *CodeGen) sliceLen() {
	// if nil => 0 else [hdr+2]
	g.opPop(REG16_BX)
	g.testRR16(REG16_BX, REG16_BX)
	fixNonNil := g.jccNearRel16(CC16_NE)
	g.xorRR16(REG16_AX, REG16_AX)
	fixDone := g.jmpRel16()
	g.patchRel16(fixNonNil)
	g.emitLoadRM16(REG16_AX, EA16_BX, 2)
	g.patchRel16(fixDone)
	g.opPush(REG16_AX)
}

func (g *CodeGen) sliceCap() {
	// if nil => 0 else [hdr+4]
	g.opPop(REG16_BX)
	g.testRR16(REG16_BX, REG16_BX)
	fixNonNil := g.jccNearRel16(CC16_NE)
	g.xorRR16(REG16_AX, REG16_AX)
	fixDone := g.jmpRel16()
	g.patchRel16(fixNonNil)
	g.emitLoadRM16(REG16_AX, EA16_BX, 4)
	g.patchRel16(fixDone)
	g.opPush(REG16_AX)
}

// ===== Conversions =====

func (g *CodeGen) convert(typeName string) {
	switch typeName {
	case "string":
		g.emitCallPlaceholder("runtime.BytesToString")
	case "[]byte":
		g.emitCallPlaceholder("runtime.StringToBytes")
	case "byte":
		g.opPop(REG16_AX)
		// and ax, 00FFh
		g.emitBytes(0x25, 0xFF, 0x00)
		g.opPush(REG16_AX)
	case "uint16", "int16", "int", "uintptr", "uint", "int32", "uint32", "int64", "uint64":
		// 16-bit target: represent as one 16-bit word (best effort / truncation).
	default:
		// unknown conversions are treated as no-op
	}
}

// ===== Intrinsics =====

func (g *CodeGen) makeSliceIntrinsic() {
	// Allocate 4-word header {ptr,len,cap,elem_size}
	g.compileConst(8)
	g.emitCallPlaceholder("runtime.Alloc")
	g.opPop(REG16_BX) // hdr*

	// locals: 0=ptr,1=len,2=cap
	g.loadLocal(2, REG16_AX) // ptr
	g.emitStoreRM16(EA16_BX, 0, REG16_AX)
	g.loadLocal(4, REG16_AX) // len
	g.emitStoreRM16(EA16_BX, 2, REG16_AX)
	g.loadLocal(6, REG16_AX) // cap
	g.emitStoreRM16(EA16_BX, 4, REG16_AX)

	g.emitMovImm16(REG16_AX, 1) // elem_size=1
	g.emitStoreRM16(EA16_BX, 6, REG16_AX)

	g.opPush(REG16_BX)
}

func (g *CodeGen) makeStringIntrinsic() {
	// Allocate 2-word header {ptr,len}
	g.compileConst(4)
	g.emitCallPlaceholder("runtime.Alloc")
	g.opPop(REG16_BX)

	g.loadLocal(2, REG16_AX) // ptr
	g.emitStoreRM16(EA16_BX, 0, REG16_AX)
	g.loadLocal(4, REG16_AX) // len
	g.emitStoreRM16(EA16_BX, 2, REG16_AX)

	g.opPush(REG16_BX)
}

func (g *CodeGen) readPtrIntrinsic() {
	// Param0 = addr (local0). Read word at addr.
	g.loadLocal(2, REG16_BX)
	g.emitLoadRM16(REG16_AX, EA16_BX, 0)
	g.opPush(REG16_AX)
}

func (g *CodeGen) writePtrIntrinsic() {
	// Param0=addr, Param1=val
	g.loadLocal(2, REG16_BX)
	g.loadLocal(4, REG16_AX)
	g.emitStoreRM16(EA16_BX, 0, REG16_AX)
}

func (g *CodeGen) writeByteIntrinsic() {
	// Param0=addr, Param1=val (low byte)
	g.loadLocal(2, REG16_BX)
	g.loadLocal(4, REG16_AX)
	g.emitBytes(0x88, 0x07) // mov [bx], al
}

func (g *CodeGen) compositeLitCall(fieldCount int) {
	structSize := fieldCount * 2
	if structSize == 0 {
		g.compileConst(0)
		return
	}

	// Save fields to CPU stack.
	for i := 0; i < fieldCount; i++ {
		g.opPop(REG16_AX)
		g.pushR16(REG16_AX)
	}

	// Allocate.
	g.compileConst(int16(structSize))
	g.emitCallPlaceholder("runtime.Alloc")
	g.opPop(REG16_BX) // struct*

	// Store fields.
	for i := 0; i < fieldCount; i++ {
		g.popR16(REG16_AX)
		g.emitStoreRM16(EA16_BX, int16(i*2), REG16_AX)
	}

	g.opPush(REG16_BX)
}

// ===== Operand stack in DI (downward, 2-byte slots) =====

func (g *CodeGen) opPush(reg int) {
	// lea di, [di-2]
	g.emitBytes(0x8D, 0x7D, 0xFE)
	// mov [di], reg
	g.emitBytes(0x89, byte(0x05|((reg&7)<<3)))
}

func (g *CodeGen) opPop(reg int) {
	// mov reg, [di]
	g.emitBytes(0x8B, byte(0x05|((reg&7)<<3)))
	// lea di, [di+2]
	g.emitBytes(0x8D, 0x7D, 0x02)
}

func (g *CodeGen) opLoad(reg int) {
	// mov reg, [di]
	g.emitBytes(0x8B, byte(0x05|((reg&7)<<3)))
}

func (g *CodeGen) opDrop() {
	// add di, 2
	g.emitBytes(0x83, 0xC7, 0x02)
}

// ===== Locals via BP =====

// Local slot i is stored at [BP-2*(i+1)].
// We pass "offset bytes" as (idx+1)*2 to match your existing layout.

func (g *CodeGen) loadLocal(offset int, reg int) {
	disp := int16(-offset)
	g.emitLoadRM16(reg, EA16_BP, disp)
}

func (g *CodeGen) storeLocal(offset int, reg int) {
	disp := int16(-offset)
	g.emitStoreRM16(EA16_BP, disp, reg)
}

func (g *CodeGen) leaLocal(offset int, reg int) {
	disp := int16(-offset)
	g.emitLeaRM16(reg, EA16_BP, disp)
}

// ===== Tiny 8086 encoder =====

func (g *CodeGen) emitByte(b byte)      { g.code = append(g.code, b) }
func (g *CodeGen) emitBytes(bs ...byte) { g.code = append(g.code, bs...) }
func (g *CodeGen) emitU16(v uint16)     { g.code = append(g.code, byte(v), byte(v>>8)) }
func putU16(b []byte, v uint16)         { b[0], b[1] = byte(v), byte(v>>8) }
func getU16(b []byte) uint16            { return uint16(b[0]) | uint16(b[1])<<8 }
func modrmRR16(dst, src int) byte       { return byte(0xC0 | ((dst & 7) << 3) | (src & 7)) }
func modrmMem16(mod byte, reg int, rm byte) byte {
	return byte((mod << 6) | byte((reg&7)<<3) | (rm & 7))
}

func (g *CodeGen) emitMovImm16(reg int, v uint16) {
	g.emitByte(byte(0xB8 + (reg & 7)))
	g.emitU16(v)
}

func (g *CodeGen) pushR16(reg int) { g.emitByte(byte(0x50 + (reg & 7))) }
func (g *CodeGen) popR16(reg int)  { g.emitByte(byte(0x58 + (reg & 7))) }

func (g *CodeGen) movRR16(dst, src int) { g.emitBytes(0x89, modrmRR16(src, dst)) }
func (g *CodeGen) addRR16(dst, src int) { g.emitBytes(0x01, modrmRR16(src, dst)) }
func (g *CodeGen) subRR16(dst, src int) { g.emitBytes(0x29, modrmRR16(src, dst)) }
func (g *CodeGen) andRR16(dst, src int) { g.emitBytes(0x21, modrmRR16(src, dst)) }
func (g *CodeGen) orRR16(dst, src int)  { g.emitBytes(0x09, modrmRR16(src, dst)) }
func (g *CodeGen) xorRR16(dst, src int) { g.emitBytes(0x31, modrmRR16(src, dst)) }
func (g *CodeGen) cmpRR16(a, b int)     { g.emitBytes(0x39, modrmRR16(b, a)) }
func (g *CodeGen) testRR16(a, b int)    { g.emitBytes(0x85, modrmRR16(b, a)) }

func (g *CodeGen) negR16(reg int)  { g.emitBytes(0xF7, byte(0xD8|(reg&7))) }
func (g *CodeGen) cwd16()          { g.emitByte(0x99) }
func (g *CodeGen) idivR16(reg int) { g.emitBytes(0xF7, byte(0xF8|(reg&7))) }

// one-operand IMUL r/m16: F7 /5
func (g *CodeGen) imulR16(reg int) { g.emitBytes(0xF7, byte(0xE8|(reg&7))) }

func (g *CodeGen) shlCl16(reg int) { g.emitBytes(0xD3, byte(0xE0|(reg&7))) }
func (g *CodeGen) sarCl16(reg int) { g.emitBytes(0xD3, byte(0xF8|(reg&7))) }
func (g *CodeGen) shlImm16(reg int, n byte) {
	// 8086 has no C1 /4 imm8 form; use repeated 1-bit shifts (D1 /4).
	for i := byte(0); i < n; i++ {
		g.emitBytes(0xD1, byte(0xE0|(reg&7)))
	}
}

func (g *CodeGen) addImm16(reg int, v int16) {
	if v >= -128 && v <= 127 {
		g.emitBytes(0x83, byte(0xC0|(reg&7)), byte(v))
		return
	}
	if reg == REG16_AX {
		g.emitByte(0x05)
	} else {
		g.emitBytes(0x81, byte(0xC0|(reg&7)))
	}
	g.emitU16(uint16(v))
}

func (g *CodeGen) subImm16(reg int, v int16) {
	if v >= -128 && v <= 127 {
		g.emitBytes(0x83, byte(0xE8|(reg&7)), byte(v))
		return
	}
	g.emitBytes(0x81, byte(0xE8|(reg&7)))
	g.emitU16(uint16(v))
}

func (g *CodeGen) cmpImm16(reg int, v int16) {
	if v >= -128 && v <= 127 {
		g.emitBytes(0x83, byte(0xF8|(reg&7)), byte(v))
		return
	}
	if reg == REG16_AX {
		g.emitByte(0x3D)
	} else {
		g.emitBytes(0x81, byte(0xF8|(reg&7)))
	}
	g.emitU16(uint16(v))
}

func (g *CodeGen) xorImm8_16(reg int, v byte) {
	g.emitBytes(0x83, byte(0xF0|(reg&7)), v)
}

func (g *CodeGen) leave16() { g.emitByte(0xC9) }
func (g *CodeGen) ret16()   { g.emitByte(0xC3) }

// emitLoadRM16: mov reg16, [ea+disp]
func (g *CodeGen) emitLoadRM16(dst int, ea int, disp int16) {
	mod := byte(0)
	if ea == EA16_BP && disp == 0 {
		mod = 1
	} else if disp >= -128 && disp <= 127 {
		if disp != 0 {
			mod = 1
		}
	} else {
		mod = 2
	}
	g.emitBytes(0x8B, modrmMem16(mod, dst, byte(ea)))
	if mod == 1 {
		g.emitByte(byte(disp))
	} else if mod == 2 {
		g.emitU16(uint16(disp))
	} else if mod == 0 && ea == EA16_BP {
		g.emitByte(0)
	}
}

// emitStoreRM16: mov [ea+disp], reg16
func (g *CodeGen) emitStoreRM16(ea int, disp int16, src int) {
	mod := byte(0)
	if ea == EA16_BP && disp == 0 {
		mod = 1
	} else if disp >= -128 && disp <= 127 {
		if disp != 0 {
			mod = 1
		}
	} else {
		mod = 2
	}
	g.emitBytes(0x89, modrmMem16(mod, src, byte(ea)))
	if mod == 1 {
		g.emitByte(byte(disp))
	} else if mod == 2 {
		g.emitU16(uint16(disp))
	} else if mod == 0 && ea == EA16_BP {
		g.emitByte(0)
	}
}

// emitLeaRM16: lea reg16, [ea+disp]
func (g *CodeGen) emitLeaRM16(dst int, ea int, disp int16) {
	mod := byte(0)
	if ea == EA16_BP && disp == 0 {
		mod = 1
	} else if disp >= -128 && disp <= 127 {
		if disp != 0 {
			mod = 1
		}
	} else {
		mod = 2
	}
	g.emitBytes(0x8D, modrmMem16(mod, dst, byte(ea)))
	if mod == 1 {
		g.emitByte(byte(disp))
	} else if mod == 2 {
		g.emitU16(uint16(disp))
	} else if mod == 0 && ea == EA16_BP {
		g.emitByte(0)
	}
}
