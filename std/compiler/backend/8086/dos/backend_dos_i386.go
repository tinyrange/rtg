//go:build !no_backend_dos_i386

// Package dos: minimal IR → DOS .COM backend for 8086 real mode.
// Major refactor: single file, 16-bit only, COM-only, no ELF/Mach-O/PE, no i386/amd64,
// no operand-cache optimizer, no jump relaxation, no Windows IAT.
//
// Assumptions (match your existing backend conventions):
// - Near pointers are 16-bit offsets inside the COM segment (DS=CS at load).
// - Operand stack lives in DI and grows downward in memory (DI -= 2 on push).
// - Frame uses BP-based locals. Locals are 1-word slots. Slot 0 is at [BP-2].
// - Params are passed on the operand stack; function prologue pops params into locals.
//
// Strings in rodata are stored as: [bytes...][header{data_ptr:u16, len:u16}]
// and code pushes the address of the header.
//
// Syscall intrinsic follows your shim mapping:
//
//	4   -> write(fd, buf, count) via INT 21h AH=40h
//	252 -> exit(code) via INT 21h AH=4Ch
//	192 -> mmap(...) stub returning a fixed near heap base (0x7000)
//
// NOTE: This file still depends on your existing IR and helpers (DecodeStringLiteral, etc.).
package dos

import (
	"fmt"
	"os"

	"j5.nz/rtg/std/compiler/backend/becommon"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

const (
	comLoadAddr = 0x0100
	comMaxImage = 0x10000 - comLoadAddr
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

// GenerateDOSCOM compiles an IRModule to a DOS .COM image (8086 real-mode).
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

	g.emitCOMStart(irmod)

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
		seen := map[string]bool{}
		fmt.Fprintf(os.Stderr, "error: %d unresolved calls:\n", len(unresolved))
		for _, name := range unresolved {
			if !seen[name] {
				fmt.Fprintf(os.Stderr, "  %s\n", name)
				seen[name] = true
			}
		}
		return fmt.Errorf("%d unresolved calls", len(unresolved))
	}

	com, err := g.buildCOM()
	if err != nil {
		return err
	}
	if err := os.WriteFile(outputPath, com, 0755); err != nil {
		return fmt.Errorf("write output: %v", err)
	}
	return nil
}

// ===== COM layout / patching =====

func (g *CodeGen) buildCOM() ([]byte, error) {
	textSize := len(g.code)
	rodataSize := len(g.rodata)
	dataSize := len(g.data)
	total := textSize + rodataSize + dataSize
	if total > comMaxImage {
		return nil, fmt.Errorf("COM image too large: %d bytes (max %d)", total, comMaxImage)
	}

	rodataAddr := uint16(comLoadAddr + textSize)
	dataAddr := uint16(comLoadAddr + textSize + rodataSize)

	// Patch rodata string headers: replace data_off with absolute near ptr.
	for _, headerOff := range g.stringMap {
		dataOff := getU16(g.rodata[headerOff : headerOff+2])
		putU16(g.rodata[headerOff:headerOff+2], uint16(rodataAddr)+dataOff)
	}

	// Patch code immediates referencing rodata header/data base.
	for _, fix := range g.callFixups {
		switch fix.Target {
		case "$rodata_header$":
			off := getU16(g.code[fix.CodeOffset : fix.CodeOffset+2])
			putU16(g.code[fix.CodeOffset:fix.CodeOffset+2], uint16(rodataAddr)+off)
		case "$data_addr$":
			off := getU16(g.code[fix.CodeOffset : fix.CodeOffset+2])
			putU16(g.code[fix.CodeOffset:fix.CodeOffset+2], uint16(dataAddr)+off)
		}
	}

	out := make([]byte, total)
	copy(out, g.code)
	copy(out[textSize:], g.rodata)
	copy(out[textSize+rodataSize:], g.data)
	return out, nil
}

// Minimal COM entry:
// - Set operand stack DI near top of segment.
// - Call init funcs.
// - Call main.main.
// - Exit via INT 21h AH=4Ch.
func (g *CodeGen) emitCOMStart(irmod *ir.IRModule) {
	g.emitMovImm16(REG16_DI, 0xFF00)

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

func (g *CodeGen) compileInst(inst ir.Inst) {
	switch inst.Op {
	case ir.OP_CONST_I64:
		g.compileConst(int16(inst.Val))
	case ir.OP_CONST_BOOL:
		if inst.Arg != 0 {
			g.compileConst(1)
		} else {
			g.compileConst(0)
		}
	case ir.OP_CONST_NIL:
		g.compileConst(0)
	case ir.OP_CONST_STR:
		g.compileConstStr(inst.Name)

	case ir.OP_LOCAL_GET:
		g.localGet(inst.Arg)
	case ir.OP_LOCAL_SET:
		g.localSet(inst.Arg)
	case ir.OP_LOCAL_ADD_IMM:
		g.localAddImm(inst.Arg, int16(inst.Val))
	case ir.OP_LOCAL_ADDR:
		g.localAddr(inst.Arg)

	case ir.OP_GLOBAL_GET:
		g.globalGet(inst.Arg)
	case ir.OP_GLOBAL_SET:
		g.globalSet(inst.Arg)
	case ir.OP_GLOBAL_ADDR:
		g.globalAddr(inst.Arg)

	case ir.OP_DROP:
		g.opDrop()
	case ir.OP_DUP:
		g.opLoad(REG16_AX)
		g.opPush(REG16_AX)

	case ir.OP_ADD, ir.OP_SUB, ir.OP_MUL, ir.OP_DIV, ir.OP_MOD,
		ir.OP_AND, ir.OP_OR, ir.OP_XOR, ir.OP_SHL, ir.OP_SHR:
		g.binOp(inst.Op)

	case ir.OP_NEG:
		g.opPop(REG16_AX)
		g.negR16(REG16_AX)
		g.opPush(REG16_AX)

	case ir.OP_EQ:
		g.compareToBool(CC16_E)
	case ir.OP_NEQ:
		g.compareToBool(CC16_NE)
	case ir.OP_LT:
		g.compareToBool(CC16_L)
	case ir.OP_GT:
		g.compareToBool(CC16_G)
	case ir.OP_LEQ:
		g.compareToBool(CC16_LE)
	case ir.OP_GEQ:
		g.compareToBool(CC16_GE)

	case ir.OP_NOT:
		g.opPop(REG16_AX)
		g.xorImm8_16(REG16_AX, 0x01)
		g.opPush(REG16_AX)

	case ir.OP_LABEL:
		g.labelOffsets[inst.Arg] = len(g.code)

	case ir.OP_JMP:
		off := g.jmpRel16()
		g.jumpFixups = append(g.jumpFixups, JumpFixup{
			CodeOffset: off,
			LabelID:    inst.Arg,
			Kind:       jumpFixupJmpRel16,
		})

	case ir.OP_JMP_IF:
		g.opPop(REG16_AX)
		g.testRR16(REG16_AX, REG16_AX)
		off := g.jccNearRel16(CC16_NE)
		g.jumpFixups = append(g.jumpFixups, JumpFixup{
			CodeOffset: off,
			LabelID:    inst.Arg,
			Kind:       jumpFixupJccRel16,
			CC:         CC16_NE,
		})

	case ir.OP_JMP_IF_NOT:
		g.opPop(REG16_AX)
		g.testRR16(REG16_AX, REG16_AX)
		off := g.jccNearRel16(CC16_E)
		g.jumpFixups = append(g.jumpFixups, JumpFixup{
			CodeOffset: off,
			LabelID:    inst.Arg,
			Kind:       jumpFixupJccRel16,
			CC:         CC16_E,
		})

	case ir.OP_JMP_EQ:
		g.compareJump(CC16_E, inst.Arg)
	case ir.OP_JMP_NEQ:
		g.compareJump(CC16_NE, inst.Arg)
	case ir.OP_JMP_LT:
		g.compareJump(CC16_L, inst.Arg)
	case ir.OP_JMP_GT:
		g.compareJump(CC16_G, inst.Arg)
	case ir.OP_JMP_LEQ:
		g.compareJump(CC16_LE, inst.Arg)
	case ir.OP_JMP_GEQ:
		g.compareJump(CC16_GE, inst.Arg)

	case ir.OP_CALL:
		if len(inst.Name) > 18 && inst.Name[:18] == "builtin.composite." {
			g.compositeLitCall(inst.Arg)
		} else {
			g.emitCallPlaceholder(inst.Name)
		}

	case ir.OP_CALL_INTRINSIC:
		g.callIntrinsic(inst.Name)

	case ir.OP_RETURN:
		g.leave16()
		g.ret16()

	case ir.OP_LOAD:
		g.memLoad(inst.Arg) // size in bytes: 1 or 2 (treat others as 2)
	case ir.OP_STORE:
		g.memStore(inst.Arg)
	case ir.OP_OFFSET:
		g.offset(inst.Arg)
	case ir.OP_INDEX_ADDR:
		g.indexAddr(inst.Arg)
	case ir.OP_LEN:
		g.sliceLen()
	case ir.OP_CAP:
		g.sliceCap()

	case ir.OP_CONVERT:
		g.convert(inst.Name)

	case ir.OP_IFACE_BOX:
		g.ifaceBox(inst.Arg)
	case ir.OP_IFACE_CALL:
		g.ifaceCall(inst.Name, inst.Arg)

	case ir.OP_PANIC:
		g.emitByte(0xCC) // int3

	case ir.OP_SLICE_GET, ir.OP_SLICE_MAKE, ir.OP_STRING_GET, ir.OP_STRING_MAKE:
		// handled by intrinsics/builtins in your IR

	default:
		panic("ICE: unhandled opcode in 8086 backend")
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

func (g *CodeGen) compileConstStr(raw string) {
	decoded := becommon.DecodeStringLiteral(raw)

	headerOff, ok := g.stringMap[decoded]
	if !ok {
		dataOff := len(g.rodata)
		g.rodata = append(g.rodata, []byte(decoded)...)

		headerOff = len(g.rodata)
		// header: {data_ptr:u16 (placeholder), len:u16}
		g.rodata = append(g.rodata, 0, 0, byte(len(decoded)), byte(len(decoded)>>8))

		g.stringMap[decoded] = headerOff
		putU16(g.rodata[headerOff:headerOff+2], uint16(dataOff))
	}

	// mov ax, imm16(headerOff) with a fixup to convert to absolute rodata address later.
	g.emitMovImm16(REG16_AX, uint16(headerOff))
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: len(g.code) - 2,
		Target:     "$rodata_header$",
	})
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

func (g *CodeGen) callIntrinsic(name string) {
	switch name {
	case "Syscall":
		g.compileSyscallIntrinsic()
	case "Sliceptr":
		// Param0 is in local0: load [hdr+0]
		g.loadLocal(2, REG16_BX)
		g.emitLoadRM16(REG16_AX, EA16_BX, 0)
		g.opPush(REG16_AX)
	case "Makeslice":
		g.makeSliceIntrinsic()
	case "Stringptr":
		g.loadLocal(2, REG16_BX)
		g.emitLoadRM16(REG16_AX, EA16_BX, 0)
		g.opPush(REG16_AX)
	case "Makestring":
		g.makeStringIntrinsic()
	case "Tostring":
		g.tostringIntrinsic()
	case "ReadPtr":
		g.readPtrIntrinsic()
	case "WritePtr":
		g.writePtrIntrinsic()
	case "WriteByte":
		g.writeByteIntrinsic()
	default:
		panic("ICE: unknown intrinsic in 8086 backend: " + name)
	}
}

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

func (g *CodeGen) tostringIntrinsic() {
	// This is called inside wrapper fns where params are locals.
	g.compileTostringBody()
}

func (g *CodeGen) emitTostringHelper() {
	if g.hasTostringHelper {
		return
	}
	g.hasTostringHelper = true
	g.funcOffsets[outlinedTostringHelper] = len(g.code)

	// Helper has 1 param; create 1-word frame.
	g.pushR16(REG16_BP)
	g.movRR16(REG16_BP, REG16_SP)
	g.subImm16(REG16_SP, 2)

	// Move param from operand stack into local0.
	g.opPop(REG16_AX)
	g.storeLocal(2, REG16_AX)

	g.compileTostringBody()
	g.leave16()
	g.ret16()
}

func (g *CodeGen) compileTostringBody() {
	// Param0 = value (string header ptr OR iface box ptr).
	// Heuristic: if [ptr] < 256 => iface box, else treat as string.
	g.loadLocal(2, REG16_BX)             // BX=value
	g.emitLoadRM16(REG16_CX, EA16_BX, 0) // CX = [BX]
	g.cmpImm16(REG16_CX, 256)
	stringCase := g.jccNearRel16(CC16_AE) // if CX >= 256 => string

	// iface: [box+0]=type_id, [box+2]=value
	g.emitLoadRM16(REG16_DX, EA16_BX, 2) // DX = concrete value
	// type_id in CX

	doneFixups := make([]int, 0)

	// type_id 1 = int => runtime.IntToString
	g.cmpImm16(REG16_CX, 1)
	next := g.jccNearRel16(CC16_NE)
	g.opPush(REG16_DX)
	g.emitCallPlaceholder("runtime.IntToString")
	doneFixups = append(doneFixups, g.jmpRel16())
	g.patchRel16(next)

	// type_id 2 = string => pass through (DX already string hdr*)
	g.cmpImm16(REG16_CX, 2)
	next = g.jccNearRel16(CC16_NE)
	g.opPush(REG16_DX)
	doneFixups = append(doneFixups, g.jmpRel16())
	g.patchRel16(next)

	// User-defined type dispatch: typeName.Error or typeName.String.
	var entries []becommon.DispatchEntry
	if g.irmod != nil && g.irmod.TypeIDs != nil {
		for typeName, tid := range g.irmod.TypeIDs {
			c := typeName + ".Error"
			if _, ok := g.irmod.MethodTable[c]; ok {
				entries = append(entries, becommon.DispatchEntry{TypeID: tid, FuncName: c})
				continue
			}
			c = typeName + ".String"
			if _, ok := g.irmod.MethodTable[c]; ok {
				entries = append(entries, becommon.DispatchEntry{TypeID: tid, FuncName: c})
			}
		}
	}
	for _, e := range entries {
		g.cmpImm16(REG16_CX, int16(e.TypeID))
		next = g.jccNearRel16(CC16_NE)
		g.opPush(REG16_DX)
		g.emitCallPlaceholder(e.FuncName)
		doneFixups = append(doneFixups, g.jmpRel16())
		g.patchRel16(next)
	}

	// default => empty string (nil)
	g.compileConst(0)
	doneFixups = append(doneFixups, g.jmpRel16())

	// string case: push original value
	g.patchRel16(stringCase)
	g.loadLocal(2, REG16_AX)
	g.opPush(REG16_AX)

	join := len(g.code)
	for _, f := range doneFixups {
		g.patchRel16At(f, join)
	}
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

func (g *CodeGen) compileSyscallIntrinsic() {
	// Locals: 0=sysno, 1=a0, 2=a1, 3=a2 ...
	g.loadLocal(2, REG16_AX) // sysno
	g.cmpImm16(REG16_AX, 4)
	fixWrite := g.jccNearRel16(CC16_E)
	g.cmpImm16(REG16_AX, 252)
	fixExit := g.jccNearRel16(CC16_E)
	g.cmpImm16(REG16_AX, 192)
	fixMmap := g.jccNearRel16(CC16_E)

	// default: (r1=0,r2=0,err=38)
	g.compileConst(0)
	g.compileConst(0)
	g.compileConst(38)
	done := g.jmpRel16()

	// write(fd, buf, count) via int21 ah=40h
	g.patchRel16(fixWrite)
	g.loadLocal(4, REG16_BX)              // fd
	g.loadLocal(6, REG16_DX)              // buf
	g.loadLocal(8, REG16_CX)              // count
	g.emitBytes(0xB4, 0x40)               // mov ah,40h
	g.emitBytes(0xCD, 0x21)               // int 21h
	fixWriteErr := g.jccNearRel16(CC16_C) // carry => error

	// success: push (ax,0,0)
	g.opPush(REG16_AX)
	g.compileConst(0)
	g.compileConst(0)
	writeJoin := g.jmpRel16()

	// error: AX = DOS error code => (0,0,ax)
	g.patchRel16(fixWriteErr)
	g.compileConst(0)
	g.compileConst(0)
	g.opPush(REG16_AX)
	writeDone := g.jmpRel16()

	// exit(code) via int21 ah=4Ch
	g.patchRel16(fixExit)
	g.loadLocal(4, REG16_AX) // exit code in AL
	g.emitBytes(0xB4, 0x4C)  // mov ah,4Ch
	g.emitBytes(0xCD, 0x21)  // int 21h
	g.emitByte(0xCC)         // int3 (should not return)

	// mmap stub: return (0x7000,0,0)
	g.patchRel16(fixMmap)
	g.compileConst(0x7000)
	g.compileConst(0)
	g.compileConst(0)
	mmapJoin := g.jmpRel16()

	// joins
	g.patchRel16(writeJoin)
	g.patchRel16(writeDone)
	g.patchRel16(mmapJoin)
	g.patchRel16(done)
}

// ===== Interface boxing/calls (minimal, 16-bit) =====

func (g *CodeGen) ifaceBox(typeID int) {
	// Pop concrete value into AX, save on stack (CPU stack).
	g.opPop(REG16_AX)
	g.pushR16(REG16_AX)

	// Alloc 2 words: {type_id, value}
	g.compileConst(4)
	g.emitCallPlaceholder("runtime.Alloc")
	g.opPop(REG16_BX) // box*

	// box[0]=type_id
	g.emitMovImm16(REG16_AX, uint16(typeID))
	g.emitStoreRM16(EA16_BX, 0, REG16_AX)

	// box[2]=value
	g.popR16(REG16_AX)
	g.emitStoreRM16(EA16_BX, 2, REG16_AX)

	g.opPush(REG16_BX)
}

func (g *CodeGen) ifaceCall(methodName string, argCount int) {
	// Save args from operand stack to CPU stack.
	for i := 0; i < argCount; i++ {
		g.opPop(REG16_AX)
		g.pushR16(REG16_AX)
	}

	// Pop iface pointer into BX.
	g.opPop(REG16_BX)

	// Load type_id and concrete value.
	g.emitLoadRM16(REG16_CX, EA16_BX, 0) // type_id
	g.emitLoadRM16(REG16_DX, EA16_BX, 2) // value

	// Push receiver then restore args to operand stack.
	g.opPush(REG16_DX)
	for i := argCount - 1; i >= 0; i-- {
		g.popR16(REG16_AX)
		g.opPush(REG16_AX)
	}

	// Extract bare method name after last dot.
	dot := len(methodName) - 1
	for dot >= 0 && methodName[dot] != '.' {
		dot--
	}
	bare := methodName
	if dot >= 0 && dot+1 < len(methodName) {
		bare = methodName[dot+1:]
	}

	var entries []becommon.DispatchEntry
	if g.irmod != nil && g.irmod.TypeIDs != nil {
		for typeName, tid := range g.irmod.TypeIDs {
			c := typeName + "." + bare
			if _, ok := g.irmod.MethodTable[c]; ok {
				entries = append(entries, becommon.DispatchEntry{TypeID: tid, FuncName: c})
			}
		}
	}

	if len(entries) == 0 {
		g.emitByte(0xCC)
		return
	}

	endFixups := make([]int, 0)
	for _, e := range entries {
		g.cmpImm16(REG16_CX, int16(e.TypeID))
		next := g.jccNearRel16(CC16_NE)
		g.emitCallPlaceholder(e.FuncName)
		endFixups = append(endFixups, g.jmpRel16())
		g.patchRel16(next)
	}
	g.emitByte(0xCC)

	end := len(g.code)
	for _, f := range endFixups {
		g.patchRel16At(f, end)
	}
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
