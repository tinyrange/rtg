package armv8m

import (
	"fmt"

	"j5.nz/rtg/std/compiler/backend/becommon"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

const (
	condEQ = 0x0
	condNE = 0x1
	condHS = 0x2
	condLO = 0x3
	condHI = 0x8
	condLS = 0x9
	condLT = 0xB
	condLE = 0xD
	condGT = 0xC
	condGE = 0xA
	// Place mutable globals in Cortex-M SRAM.
	globalDataBase = uint32(0x20000000)
)

type callFixup struct {
	at     int
	target string
}

type jumpFixup struct {
	wordOff int
	labelID int
}

type jumpLiteralFixup struct {
	wordOff int
	target  int
}

type CodeGen struct {
	target *common.Target
	irmod  *ir.IRModule
	asm    *Assembler

	funcOffsets map[string]int
	callFixups  []callFixup
	jumpLits    []jumpLiteralFixup
	allJumpWord []int

	curLabels map[int]int
	curFixups []jumpFixup
	curFunc   *ir.IRFunc
	locals    int
	funcLabels map[string]map[int]int

	rodata []byte

	strIndex map[string]int
	strs     []strEntry
	strFix   []strFixup
}

type strEntry struct {
	headerOff int
	dataOff   int
	length    int
}

type strFixup struct {
	litOff int
	index  int
}

func NewCodeGen(target *common.Target, irmod *ir.IRModule) *CodeGen {
	return &CodeGen{
		target:      target,
		irmod:       irmod,
		asm:         NewAssembler(),
		funcOffsets: make(map[string]int),
		funcLabels:  make(map[string]map[int]int),
		strIndex:    make(map[string]int),
	}
}

//rtg:profile
func (g *CodeGen) Code() []byte {
	return g.asm.Code()
}

//rtg:profile
func (g *CodeGen) Asm() *Assembler {
	return g.asm
}

//rtg:profile
func (g *CodeGen) EmitMovsImm(rd uint8, imm uint8) {
	g.asm.EmitMovsImm(rd, imm)
}

//rtg:profile
func (g *CodeGen) EmitBkpt(imm uint8) {
	g.asm.EmitBkpt(imm)
}

//rtg:profile
func (g *CodeGen) EmitBSelf() {
	g.asm.EmitBSelf()
}

//rtg:profile
func (g *CodeGen) LoadImm32(reg uint8, v uint32) {
	g.loadImm32(reg, v)
}

//rtg:profile
func (g *CodeGen) FunctionOffset(name string) (int, bool) {
	off, ok := g.funcOffsets[name]
	return off, ok
}

//rtg:profile
func (g *CodeGen) FunctionLabels(name string) map[int]int {
	src, ok := g.funcLabels[name]
	if !ok {
		return nil
	}
	out := make(map[int]int, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

//rtg:profile
func (g *CodeGen) EmitCallPlaceholder(target string) {
	at := g.asm.EmitBLPlaceholder()
	g.callFixups = append(g.callFixups, callFixup{at: at, target: target})
}

//rtg:profile
func (g *CodeGen) ResolveCalls(codeBaseAddr uint32) error {
	var unresolvedStubOff int
	var haveUnresolvedStub bool
	for _, fx := range g.callFixups {
		toOff, ok := g.funcOffsets[fx.target]
		if !ok {
			if !haveUnresolvedStub {
				unresolvedStubOff = g.asm.Pos()
				g.asm.Emit16(0x4770) // bx lr
				haveUnresolvedStub = true
			}
			toOff = unresolvedStubOff
		}
		fromAddr := int32(codeBaseAddr) + int32(fx.at)
		toAddr := int32(codeBaseAddr) + int32(toOff)
		rel := toAddr - (fromAddr + 4)
		g.asm.PatchBL(fx.at, rel)
	}
	for _, fx := range g.jumpLits {
		targetAddr := uint32(int(codeBaseAddr)+fx.target) | 1
		g.asm.Patch32(fx.wordOff, targetAddr)
	}
	for _, off := range g.allJumpWord {
		if off < 0 || off+3 >= len(g.asm.code) {
			return fmt.Errorf("jump literal out of range at %d", off)
		}
		v := uint32(g.asm.code[off]) | uint32(g.asm.code[off+1])<<8 | uint32(g.asm.code[off+2])<<16 | uint32(g.asm.code[off+3])<<24
		if v == 0 {
			return fmt.Errorf("unpatched jump literal at code offset %d", off)
		}
	}
	return nil
}

//rtg:profile
func (g *CodeGen) CompileModuleFuncs() error {
	for _, f := range g.irmod.Funcs {
		g.funcOffsets[f.Name] = g.asm.Pos()
		if err := g.compileFunc(f); err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
	}
	g.finalizeRodata()
	return nil
}

//rtg:profile
func (g *CodeGen) compileFunc(f *ir.IRFunc) error {
	if f.Native != nil {
		g.asm.code = append(g.asm.code, f.Native.Code...)
		return nil
	}
	g.curFunc = f
	g.curLabels = make(map[int]int)
	g.curFixups = nil
	g.locals = len(f.Locals)
	if f.Params > g.locals {
		g.locals = f.Params
	}

	// Prologue:
	//   push {r4,r5,r7,lr}
	//   mov r7, sp
	//   allocate locals by pushing zeroes
	//   mov r5, sp
	g.asm.EmitPush((1<<4)|(1<<5)|(1<<7), true)
	g.asm.EmitMovReg(7, 13)
	if g.locals > 0 {
		g.asm.EmitMovsImm(0, 0)
		i := 0
		for i < g.locals {
			g.asm.EmitPush(1<<0, false)
			i = i + 1
		}
	}
	g.asm.EmitMovReg(5, 13)

	// Move params from operand stack into locals.
	if f.Params > 0 {
		i := f.Params - 1
		for i >= 0 {
			g.opPop(0)
			g.storeLocal(i, 0)
			i = i - 1
		}
	}

	for _, inst := range f.Code {
		if err := g.compileInst(inst); err != nil {
			return err
		}
	}

	for _, fx := range g.curFixups {
		labelOff, ok := g.curLabels[fx.labelID]
		if !ok {
			return fmt.Errorf("unknown label %d", fx.labelID)
		}
		g.jumpLits = append(g.jumpLits, jumpLiteralFixup{
			wordOff: fx.wordOff,
			target:  labelOff,
		})
	}
	if g.curLabels != nil {
		lbl := make(map[int]int, len(g.curLabels))
		for k, v := range g.curLabels {
			lbl[k] = v
		}
		g.funcLabels[f.Name] = lbl
	}

	g.emitEpilogue()
	g.curFunc = nil
	return nil
}

//rtg:profile
func (g *CodeGen) compileInst(inst ir.Inst) error {
	switch inst.Op {
	case ir.OP_CONST_I64:
		g.loadImm32(0, uint32(inst.Val))
		g.opPush(0)
	case ir.OP_CONST_BOOL:
		if inst.Arg != 0 {
			g.asm.EmitMovsImm(0, 1)
		} else {
			g.asm.EmitMovsImm(0, 0)
		}
		g.opPush(0)
	case ir.OP_CONST_NIL:
		g.asm.EmitMovsImm(0, 0)
		g.opPush(0)
	case ir.OP_CONST_STR:
		s := becommon.DecodeStringLiteral(inst.Name)
		idx := g.internString(s)
		litOff := g.loadImm32(0, 0)
		g.strFix = append(g.strFix, strFixup{litOff: litOff, index: idx})
		g.opPush(0)
	case ir.OP_LOCAL_GET:
		g.loadLocal(inst.Arg, 0)
		g.opPush(0)
	case ir.OP_LOCAL_SET:
		g.opPop(0)
		g.storeLocal(inst.Arg, 0)
	case ir.OP_LOCAL_ADD_IMM:
		g.loadLocal(inst.Arg, 0)
		if inst.Val >= 0 && inst.Val <= 255 {
			g.asm.EmitAddsImm(0, uint8(inst.Val))
		} else if inst.Val < 0 && -inst.Val <= 255 {
			g.asm.EmitSubsImm(0, uint8(-inst.Val))
		} else {
			return fmt.Errorf("local_add_imm out of immediate range: %d", inst.Val)
		}
		g.storeLocal(inst.Arg, 0)
	case ir.OP_LOCAL_ADDR:
		g.localAddr(inst.Arg, 0)
		g.opPush(0)
	case ir.OP_GLOBAL_GET:
		addr := globalDataBase + uint32(inst.Arg*4)
		g.loadImm32(1, addr)
		g.asm.EmitLdrImm(0, 1, 0)
		g.opPush(0)
	case ir.OP_GLOBAL_SET:
		addr := globalDataBase + uint32(inst.Arg*4)
		g.opPop(0)
		g.loadImm32(1, addr)
		g.asm.EmitStrImm(0, 1, 0)
	case ir.OP_GLOBAL_ADDR:
		addr := globalDataBase + uint32(inst.Arg*4)
		g.loadImm32(0, addr)
		g.opPush(0)
	case ir.OP_DROP:
		g.opPop(0)
	case ir.OP_DUP:
		g.opPop(0)
		g.opPush(0)
		g.opPush(0)
	case ir.OP_ADD:
		g.opPop(1)
		g.opPop(0)
		g.asm.EmitAddRRR(0, 0, 1)
		g.opPush(0)
	case ir.OP_SUB:
		g.opPop(1)
		g.opPop(0)
		g.asm.EmitSubRRR(0, 0, 1)
		g.opPush(0)
	case ir.OP_MUL:
		g.opPop(1)
		g.opPop(0)
		g.asm.EmitMulRR(0, 1)
		g.opPush(0)
	case ir.OP_DIV:
		g.emitDivMod(false)
	case ir.OP_MOD:
		g.emitDivMod(true)
	case ir.OP_NEG:
		g.opPop(0)
		g.asm.EmitNegRR(0, 0)
		g.opPush(0)
	case ir.OP_AND:
		g.opPop(1)
		g.opPop(0)
		g.asm.EmitAndRR(0, 1)
		g.opPush(0)
	case ir.OP_OR:
		g.opPop(1)
		g.opPop(0)
		g.asm.EmitOrrRR(0, 1)
		g.opPush(0)
	case ir.OP_XOR:
		g.opPop(1)
		g.opPop(0)
		g.asm.EmitEorRR(0, 1)
		g.opPush(0)
	case ir.OP_SHL:
		g.opPop(1)
		g.opPop(0)
		g.asm.EmitLslRR(0, 1)
		g.opPush(0)
	case ir.OP_SHR:
		g.opPop(1)
		g.opPop(0)
		g.asm.EmitLsrRR(0, 1)
		g.opPush(0)
	case ir.OP_NOT:
		g.opPop(0)
		g.asm.EmitCmpImm(0, 0)
		setTrue := g.asm.EmitBCond(condEQ, 0)
		g.asm.EmitMovsImm(0, 0)
		endJump := g.asm.EmitBImm11(0)
		setPos := g.asm.Pos()
		g.asm.EmitMovsImm(0, 1)
		endPos := g.asm.Pos()
		g.asm.PatchBCond(setTrue, condEQ, int32(setPos-(setTrue+4)))
		g.asm.PatchBImm11(endJump, int32(endPos-(endJump+4)))
		g.opPush(0)
	case ir.OP_EQ:
		g.compareToBool(condEQ)
	case ir.OP_NEQ:
		g.compareToBool(condNE)
	case ir.OP_LT:
		if inst.Name == "unsigned" {
			g.compareToBool(condLO)
		} else {
			g.compareToBool(condLT)
		}
	case ir.OP_GT:
		if inst.Name == "unsigned" {
			g.compareToBool(condHI)
		} else {
			g.compareToBool(condGT)
		}
	case ir.OP_LEQ:
		if inst.Name == "unsigned" {
			g.compareToBool(condLS)
		} else {
			g.compareToBool(condLE)
		}
	case ir.OP_GEQ:
		if inst.Name == "unsigned" {
			g.compareToBool(condHS)
		} else {
			g.compareToBool(condGE)
		}
	case ir.OP_LABEL:
		g.curLabels[inst.Arg] = g.asm.Pos()
	case ir.OP_JMP:
		wordOff := g.emitLongJumpPlaceholder()
		g.curFixups = append(g.curFixups, jumpFixup{wordOff: wordOff, labelID: inst.Arg})
	case ir.OP_JMP_IF:
		g.opPop(0)
		g.asm.EmitCmpImm(0, 0)
		skip := g.asm.EmitBCond(condEQ, 0)
		wordOff := g.emitLongJumpPlaceholder()
		g.asm.PatchBCond(skip, condEQ, int32(g.asm.Pos()-(skip+4)))
		g.curFixups = append(g.curFixups, jumpFixup{wordOff: wordOff, labelID: inst.Arg})
	case ir.OP_JMP_IF_NOT:
		g.opPop(0)
		g.asm.EmitCmpImm(0, 0)
		skip := g.asm.EmitBCond(condNE, 0)
		wordOff := g.emitLongJumpPlaceholder()
		g.asm.PatchBCond(skip, condNE, int32(g.asm.Pos()-(skip+4)))
		g.curFixups = append(g.curFixups, jumpFixup{wordOff: wordOff, labelID: inst.Arg})
	case ir.OP_JMP_EQ:
		g.compareJump(condEQ, inst.Arg)
	case ir.OP_JMP_NEQ:
		g.compareJump(condNE, inst.Arg)
	case ir.OP_JMP_LT:
		if inst.Name == "unsigned" {
			g.compareJump(condLO, inst.Arg)
		} else {
			g.compareJump(condLT, inst.Arg)
		}
	case ir.OP_JMP_GT:
		if inst.Name == "unsigned" {
			g.compareJump(condHI, inst.Arg)
		} else {
			g.compareJump(condGT, inst.Arg)
		}
	case ir.OP_JMP_LEQ:
		if inst.Name == "unsigned" {
			g.compareJump(condLS, inst.Arg)
		} else {
			g.compareJump(condLE, inst.Arg)
		}
	case ir.OP_JMP_GEQ:
		if inst.Name == "unsigned" {
			g.compareJump(condHS, inst.Arg)
		} else {
			g.compareJump(condGE, inst.Arg)
		}
	case ir.OP_CALL:
		if len(inst.Name) > 18 && inst.Name[0:18] == "builtin.composite." {
			g.compileCompositeLitCall(inst)
			return nil
		}
		g.EmitCallPlaceholder(inst.Name)
	case ir.OP_CALL_INTRINSIC:
		if err := g.compileIntrinsic(inst); err != nil {
			return err
		}
	case ir.OP_RETURN:
		g.emitEpilogue()
	case ir.OP_LOAD:
		g.opPop(1)
		if inst.Arg == 1 {
			g.asm.EmitLdrbImm(0, 1, 0)
		} else if inst.Arg == 0 || inst.Arg >= 2 {
			g.emitLoadWordUnaligned(1, 0)
		} else {
			return fmt.Errorf("unsupported load width: %d", inst.Arg)
		}
		g.opPush(0)
	case ir.OP_STORE:
		g.opPop(0) // addr
		g.opPop(1) // value
		if inst.Arg == 1 {
			g.asm.EmitStrbImm(1, 0, 0)
		} else if inst.Arg == 0 || inst.Arg >= 2 {
			g.emitStoreWordUnaligned(0, 1)
		} else {
			return fmt.Errorf("unsupported store width: %d", inst.Arg)
		}
	case ir.OP_OFFSET:
		g.opPop(0)
		if inst.Arg >= 0 && inst.Arg <= 255 {
			g.asm.EmitAddsImm(0, uint8(inst.Arg))
		} else if inst.Arg < 0 && -inst.Arg <= 255 {
			g.asm.EmitSubsImm(0, uint8(-inst.Arg))
		} else {
			if inst.Arg >= 0 {
				g.loadImm32(1, uint32(inst.Arg))
				g.asm.EmitAddRRR(0, 0, 1)
			} else {
				g.loadImm32(1, uint32(-inst.Arg))
				g.asm.EmitSubRRR(0, 0, 1)
			}
		}
		g.opPush(0)
	case ir.OP_INDEX_ADDR:
		g.opPop(1)                // index
		g.opPop(0)                // header
		g.asm.EmitLdrImm(2, 0, 0) // data ptr
		if inst.Arg > 0 {
			g.loadImm32(3, uint32(inst.Arg))
		} else {
			g.asm.EmitLdrImm(3, 0, 3) // elem size
		}
		g.asm.EmitMulRR(1, 3)
		g.asm.EmitAddRRR(0, 2, 1)
		g.opPush(0)
	case ir.OP_LEN:
		g.opPop(0)
		g.asm.EmitCmpImm(0, 0)
		nonNil := g.asm.EmitBCond(condNE, 0)
		g.asm.EmitMovsImm(0, 0)
		endJump := g.asm.EmitBImm11(0)
		loadPos := g.asm.Pos()
		g.asm.EmitLdrImm(0, 0, 1)
		endPos := g.asm.Pos()
		g.asm.PatchBCond(nonNil, condNE, int32(loadPos-(nonNil+4)))
		g.asm.PatchBImm11(endJump, int32(endPos-(endJump+4)))
		g.opPush(0)
	case ir.OP_CAP:
		g.opPop(0)
		g.asm.EmitCmpImm(0, 0)
		nonNil := g.asm.EmitBCond(condNE, 0)
		g.asm.EmitMovsImm(0, 0)
		endJump := g.asm.EmitBImm11(0)
		loadPos := g.asm.Pos()
		g.asm.EmitLdrImm(0, 0, 2)
		endPos := g.asm.Pos()
		g.asm.PatchBCond(nonNil, condNE, int32(loadPos-(nonNil+4)))
		g.asm.PatchBImm11(endJump, int32(endPos-(endJump+4)))
		g.opPush(0)
	case ir.OP_CONVERT:
		if inst.Name == "[]byte" {
			g.opPop(0)
			g.opPush(0)
			g.EmitCallPlaceholder("runtime.StringToBytes")
		} else if inst.Name == "string" {
			g.opPop(0)
			g.opPush(0)
			g.EmitCallPlaceholder("runtime.BytesToString")
		} else if inst.Name == "byte" || inst.Name == "uint8" {
			g.opPop(0)
			g.asm.EmitMovsImm(1, 0xFF)
			g.asm.EmitAndRR(0, 1)
			g.opPush(0)
		} else if inst.Name == "uint16" {
			g.opPop(0)
			g.loadImm32(1, 0x0000FFFF)
			g.asm.EmitAndRR(0, 1)
			g.opPush(0)
		}
	case ir.OP_IFACE_BOX:
		if err := g.compileIfaceBox(inst); err != nil {
			return err
		}
	case ir.OP_IFACE_CALL:
		if err := g.compileIfaceCall(inst); err != nil {
			return err
		}
	case ir.OP_PANIC:
		g.opPop(0)
		g.asm.EmitMovsImm(0, 0x18)
		g.loadImm32(1, 0x00020026)
		g.asm.EmitBkpt(0xAB)
		g.asm.EmitBSelf()
	default:
		return fmt.Errorf("unsupported opcode: %d", inst.Op)
	}
	return nil
}

//rtg:profile
func (g *CodeGen) compileIntrinsic(inst ir.Inst) error {
	switch inst.Name {
	case "Sliceptr", "Stringptr":
		g.loadLocal(0, 0)
		g.asm.EmitCmpImm(0, 0)
		nonNil := g.asm.EmitBCond(condNE, 0)
		g.asm.EmitMovsImm(0, 0)
		endJump := g.asm.EmitBImm11(0)
		loadPos := g.asm.Pos()
		g.emitLoadWordUnaligned(0, 0)
		endPos := g.asm.Pos()
		g.asm.PatchBCond(nonNil, condNE, int32(loadPos-(nonNil+4)))
		g.asm.PatchBImm11(endJump, int32(endPos-(endJump+4)))
		g.opPush(0)
		return nil
	case "ReadPtr":
		g.loadLocal(0, 0)
		g.emitLoadWordUnaligned(0, 0)
		g.opPush(0)
		return nil
	case "WritePtr":
		g.loadLocal(0, 0)
		g.loadLocal(1, 1)
		g.emitStoreWordUnaligned(0, 1)
		return nil
	case "WriteByte":
		g.loadLocal(0, 0)
		g.loadLocal(1, 1)
		g.asm.EmitStrbImm(1, 0, 0)
		return nil
	case "Semihost":
		// r0=op, r1=arg, bkpt 0xAB
		g.loadLocal(0, 0)
		g.loadLocal(1, 1)
		g.asm.EmitBkpt(0xAB)
		// Return (r1, r2, err) convention.
		g.opPush(0)
		g.asm.EmitMovsImm(0, 0)
		g.opPush(0)
		g.opPush(0)
		return nil
	case "Makestring":
		// header := runtime.Alloc(8); header.ptr=local0; header.len=local1
		g.loadImm32(0, 8)
		g.opPush(0)
		g.EmitCallPlaceholder("runtime.Alloc")
		g.opPop(1)
		g.loadLocal(0, 0)
		g.asm.EmitStrImm(0, 1, 0)
		g.loadLocal(1, 0)
		g.asm.EmitStrImm(0, 1, 1)
		g.opPush(1)
		return nil
	case "Makeslice":
		// header := runtime.Alloc(16)
		g.loadImm32(0, 16)
		g.opPush(0)
		g.EmitCallPlaceholder("runtime.Alloc")
		g.opPop(1)
		g.loadLocal(0, 0) // ptr
		g.asm.EmitStrImm(0, 1, 0)
		g.loadLocal(1, 0) // len
		g.asm.EmitStrImm(0, 1, 1)
		g.loadLocal(2, 0) // cap
		g.asm.EmitStrImm(0, 1, 2)
		g.loadImm32(0, 1) // elem size
		g.asm.EmitStrImm(0, 1, 3)
		g.opPush(1)
		return nil
	case "Tostring":
		// Param 0 may be either:
		// - a direct string header pointer, or
		// - an interface box pointer {typeID, concreteValuePtr}.
		g.loadLocal(0, 0)
		g.emitLoadWordUnaligned(0, 1) // candidate typeID / string.data
		g.loadImm32(2, 256)
		g.asm.EmitCmpRR(1, 2)
		isIface := g.asm.EmitBCond(condLO, 0)

		// String fast-path: passthrough pointer.
		g.loadLocal(0, 0)
		g.opPush(0)
		done := g.emitLongJumpPlaceholder()

		ifacePos := g.asm.Pos()
		g.asm.PatchBCond(isIface, condLO, int32(ifacePos-(isIface+4)))

		// Interface path: r1=typeID, r2=concrete value pointer.
		g.loadLocal(0, 0)
		g.asm.EmitMovReg(3, 0)
		g.asm.EmitAddsImm(3, 4)
		g.emitLoadWordUnaligned(3, 2)
		g.emitLoadWordUnaligned(0, 1)

		var endJumps []int

		// Builtin int
		g.asm.EmitCmpImm(1, 1)
		skipInt := g.asm.EmitBCond(condNE, 0)
		g.opPush(2)
		g.EmitCallPlaceholder("runtime.IntToString")
		endJumps = append(endJumps, g.emitLongJumpPlaceholder())
		g.asm.PatchBCond(skipInt, condNE, int32(g.asm.Pos()-(skipInt+4)))

		// Builtin string
		g.asm.EmitCmpImm(1, 2)
		skipStr := g.asm.EmitBCond(condNE, 0)
		g.opPush(2)
		endJumps = append(endJumps, g.emitLongJumpPlaceholder())
		g.asm.PatchBCond(skipStr, condNE, int32(g.asm.Pos()-(skipStr+4)))

		// User-defined Error()/String() dispatch.
		entries := g.collectDispatch("Error")
		strEntries := g.collectDispatch("String")
		seen := make(map[int]bool, len(entries))
		i := 0
		for i < len(entries) {
			seen[entries[i].TypeID] = true
			i++
		}
		i = 0
		for i < len(strEntries) {
			if !seen[strEntries[i].TypeID] {
				entries = append(entries, strEntries[i])
			}
			i++
		}
		for _, entry := range entries {
			if entry.TypeID >= 0 && entry.TypeID <= 255 {
				g.asm.EmitCmpImm(1, uint8(entry.TypeID))
			} else {
				g.loadImm32(3, uint32(entry.TypeID))
				g.asm.EmitCmpRR(1, 3)
			}
			skip := g.asm.EmitBCond(condNE, 0)
			g.opPush(2)
			g.EmitCallPlaceholder(entry.FuncName)
			endJumps = append(endJumps, g.emitLongJumpPlaceholder())
			g.asm.PatchBCond(skip, condNE, int32(g.asm.Pos()-(skip+4)))
		}

		// Default: pass through concrete value pointer.
		// For interface{} values that box strings on this target, this preserves output.
		g.opPush(2)

		end := g.asm.Pos()
		g.jumpLits = append(g.jumpLits, jumpLiteralFixup{
			wordOff: done,
			target:  end,
		})
		i = 0
		for i < len(endJumps) {
			g.jumpLits = append(g.jumpLits, jumpLiteralFixup{
				wordOff: endJumps[i],
				target:  end,
			})
			i++
		}
		return nil
	default:
		return fmt.Errorf("unsupported intrinsic: %s", inst.Name)
	}
}

//rtg:profile
func (g *CodeGen) compileCompositeLitCall(inst ir.Inst) {
	fieldCount := inst.Arg
	structSize := fieldCount * 4
	if structSize == 0 {
		g.asm.EmitMovsImm(0, 0)
		g.opPush(0)
		return
	}

	// Save fields to call stack.
	i := 0
	for i < fieldCount {
		g.opPop(0)
		g.asm.EmitPush(1<<0, false)
		i = i + 1
	}

	g.loadImm32(0, uint32(structSize))
	g.opPush(0)
	g.EmitCallPlaceholder("runtime.Alloc")
	g.opPop(1) // struct ptr

	i = 0
	for i < fieldCount {
		g.asm.EmitPop(1<<0, false)
		if i <= 31 {
			g.asm.EmitStrImm(0, 1, uint8(i))
		} else {
			g.loadImm32(2, uint32(i*4))
			g.asm.EmitAddRRR(2, 2, 1)
			g.asm.EmitStrImm(0, 2, 0)
		}
		i = i + 1
	}
	g.opPush(1)
}

//rtg:profile
func (g *CodeGen) emitEpilogue() {
	g.asm.EmitMovReg(13, 7)
	g.asm.EmitPop((1<<4)|(1<<5)|(1<<7), true)
}

func (g *CodeGen) opPush(reg uint8) {
	g.asm.EmitSubsImm(6, 4)
	g.asm.EmitStrImm(reg, 6, 0)
}

//rtg:profile
func (g *CodeGen) PushOperand(reg uint8) {
	g.opPush(reg)
}

func (g *CodeGen) opPop(reg uint8) {
	g.asm.EmitLdrImm(reg, 6, 0)
	g.asm.EmitAddsImm(6, 4)
}

//rtg:profile
func (g *CodeGen) emitLoadWordUnaligned(addrReg uint8, outReg uint8) {
	// Preserve scratch registers so callers only observe outReg being clobbered.
	// If addrReg == outReg, copy the base first to avoid destroying the address
	// on the first byte load.
	baseReg := addrReg
	tmp := uint8(0xFF)
	shift := uint8(0xFF)
	baseCopy := uint8(0xFF)

	cand := uint8(0)
	for cand < 8 {
		if cand != outReg && cand != addrReg {
			if tmp == 0xFF {
				tmp = cand
			} else if shift == 0xFF {
				shift = cand
			} else if baseCopy == 0xFF {
				baseCopy = cand
			}
		}
		cand++
	}
	if tmp == 0xFF || shift == 0xFF {
		panic("armv8m: no scratch registers for unaligned load")
	}
	if addrReg == outReg {
		if baseCopy == 0xFF {
			panic("armv8m: no base scratch register for unaligned load")
		}
		baseReg = baseCopy
	}

	mask := uint8((1 << tmp) | (1 << shift))
	if addrReg == outReg {
		mask = mask | (1 << baseReg)
	}
	g.asm.EmitPush(mask, false)
	if addrReg == outReg {
		g.asm.EmitMovReg(baseReg, addrReg)
	}

	g.asm.EmitLdrbImm(outReg, baseReg, 0)
	g.asm.EmitLdrbImm(tmp, baseReg, 1)
	g.asm.EmitMovsImm(shift, 8)
	g.asm.EmitLslRR(tmp, shift)
	g.asm.EmitOrrRR(outReg, tmp)
	g.asm.EmitLdrbImm(tmp, baseReg, 2)
	g.asm.EmitMovsImm(shift, 16)
	g.asm.EmitLslRR(tmp, shift)
	g.asm.EmitOrrRR(outReg, tmp)
	g.asm.EmitLdrbImm(tmp, baseReg, 3)
	g.asm.EmitMovsImm(shift, 24)
	g.asm.EmitLslRR(tmp, shift)
	g.asm.EmitOrrRR(outReg, tmp)

	g.asm.EmitPop(mask, false)
}

//rtg:profile
func (g *CodeGen) emitStoreWordUnaligned(addrReg uint8, valReg uint8) {
	tmp := uint8(0xFF)
	shift := uint8(0xFF)
	cand := uint8(0)
	for cand < 8 {
		if cand != addrReg && cand != valReg {
			if tmp == 0xFF {
				tmp = cand
			} else {
				shift = cand
				break
			}
		}
		cand++
	}
	if tmp == 0xFF || shift == 0xFF {
		panic("armv8m: no scratch registers for unaligned store")
	}
	mask := uint8((1 << tmp) | (1 << shift))
	g.asm.EmitPush(mask, false)

	g.asm.EmitStrbImm(valReg, addrReg, 0)
	g.asm.EmitMovReg(tmp, valReg)
	g.asm.EmitMovsImm(shift, 8)
	g.asm.EmitLsrRR(tmp, shift)
	g.asm.EmitStrbImm(tmp, addrReg, 1)
	g.asm.EmitLsrRR(tmp, shift)
	g.asm.EmitStrbImm(tmp, addrReg, 2)
	g.asm.EmitLsrRR(tmp, shift)
	g.asm.EmitStrbImm(tmp, addrReg, 3)

	g.asm.EmitPop(mask, false)
}

//rtg:profile
func (g *CodeGen) emitPushStringFromValuePtr(valReg uint8) {
	// If valReg already points at a {data,len} header, pass it through.
	// Otherwise treat valReg as a C-style data pointer and build a header.
	g.asm.EmitMovReg(2, valReg)
	g.emitLoadWordUnaligned(2, 0) // r0 = [val+0] candidate data ptr
	g.asm.EmitMovReg(3, 2)
	g.asm.EmitAddsImm(3, 4)
	g.emitLoadWordUnaligned(3, 1) // r1 = [val+4] candidate len
	g.loadImm32(3, 0x00010000)
	g.asm.EmitCmpRR(0, 3)
	badHdr := g.asm.EmitBCond(condLO, 0)
	g.loadImm32(3, 1<<20)
	g.asm.EmitCmpRR(1, 3)
	badHdr2 := g.asm.EmitBCond(condHS, 0)
	g.opPush(2)
	done := g.emitLongJumpPlaceholder()
	badPos := g.asm.Pos()
	g.asm.PatchBCond(badHdr, condLO, int32(badPos-(badHdr+4)))
	g.asm.PatchBCond(badHdr2, condHS, int32(badPos-(badHdr2+4)))
	g.opPush(2)
	g.EmitCallPlaceholder("runtime.StringFromPtrZ")
	donePos := g.asm.Pos()
	g.jumpLits = append(g.jumpLits, jumpLiteralFixup{
		wordOff: done,
		target:  donePos,
	})
}

//rtg:profile
func (g *CodeGen) compareToBool(cond int) {
	g.opPop(1)
	g.opPop(0)
	g.asm.EmitCmpRR(0, 1)
	set := g.asm.EmitBCond(uint8(cond), 0)
	g.asm.EmitMovsImm(0, 0)
	endJump := g.asm.EmitBImm11(0)
	setPos := g.asm.Pos()
	g.asm.EmitMovsImm(0, 1)
	end := g.asm.Pos()
	g.asm.PatchBCond(set, uint8(cond), int32(setPos-(set+4)))
	g.asm.PatchBImm11(endJump, int32(end-(endJump+4)))
	g.opPush(0)
}

//rtg:profile
func (g *CodeGen) compareJump(cond int, labelID int) {
	g.opPop(1)
	g.opPop(0)
	g.asm.EmitCmpRR(0, 1)
	inv := cond ^ 1
	skip := g.asm.EmitBCond(uint8(inv), 0)
	wordOff := g.emitLongJumpPlaceholder()
	g.asm.PatchBCond(skip, uint8(inv), int32(g.asm.Pos()-(skip+4)))
	g.curFixups = append(g.curFixups, jumpFixup{wordOff: wordOff, labelID: labelID})
}

//rtg:profile
func (g *CodeGen) emitDivMod(wantMod bool) {
	// Stack: ... dividend, divisor
	// Uses a simple repeated-subtraction algorithm for bringup.
	g.opPop(1)              // divisor
	g.opPop(0)              // dividend
	g.asm.EmitMovsImm(2, 0) // quotient
	g.asm.EmitCmpImm(1, 0)
	divByZero := g.asm.EmitBCond(condEQ, 0)
	loop := g.asm.Pos()
	g.asm.EmitCmpRR(0, 1)
	done := g.asm.EmitBCond(condLT, 0)
	g.asm.EmitSubRRR(0, 0, 1)
	g.asm.EmitAddsImm(2, 1)
	back := int32(loop - (g.asm.Pos() + 4))
	loopJump := g.asm.EmitBImm11(0)
	g.asm.PatchBImm11(loopJump, back)
	afterDone := g.asm.Pos()
	g.asm.PatchBCond(done, condLT, int32(afterDone-(done+4)))
	if !wantMod {
		g.asm.EmitMovReg(0, 2)
	}
	end := g.asm.EmitBImm11(0)
	zeroPos := g.asm.Pos()
	g.asm.PatchBCond(divByZero, condEQ, int32(zeroPos-(divByZero+4)))
	g.asm.EmitMovsImm(0, 0)
	endPos := g.asm.Pos()
	g.asm.PatchBImm11(end, int32(endPos-(end+4)))
	g.opPush(0)
}

//rtg:profile
func (g *CodeGen) compileIfaceBox(inst ir.Inst) error {
	// box layout: [typeID, concreteValue]
	g.opPop(0)
	g.asm.EmitPush(1<<0, false) // save concrete value across runtime.Alloc call
	g.loadImm32(0, 8)
	g.opPush(0)
	g.EmitCallPlaceholder("runtime.Alloc")
	g.opPop(1)                 // box ptr
	g.asm.EmitPop(1<<0, false) // concrete value
	g.loadImm32(2, uint32(inst.Arg))
	g.asm.EmitStrImm(2, 1, 0) // type id
	g.asm.EmitStrImm(0, 1, 1) // value
	g.opPush(1)
	return nil
}

//rtg:profile
func (g *CodeGen) collectDispatch(methodName string) []becommon.DispatchEntry {
	dotIdx := len(methodName) - 1
	for dotIdx >= 0 {
		if methodName[dotIdx] == '.' {
			break
		}
		dotIdx = dotIdx - 1
	}
	bareMethod := methodName
	if dotIdx >= 0 && dotIdx+1 < len(methodName) {
		bareMethod = methodName[dotIdx+1:]
	}
	var out []becommon.DispatchEntry
	if g.irmod != nil && g.irmod.TypeIDs != nil {
		for typeName, tid := range g.irmod.TypeIDs {
			candidate := typeName + "." + bareMethod
			if _, ok := g.irmod.MethodTable[candidate]; ok {
				out = append(out, becommon.DispatchEntry{TypeID: tid, FuncName: candidate})
			}
		}
	}
	return out
}

//rtg:profile
func (g *CodeGen) compileIfaceCall(inst ir.Inst) error {
	argCount := inst.Arg
	entries := g.collectDispatch(inst.Name)

	i := 0
	for i < argCount {
		g.opPop(0)
		g.asm.EmitPush(1<<0, false)
		i = i + 1
	}

	// Pop interface pointer and unpack it.
	g.opPop(0)
	g.asm.EmitLdrImm(1, 0, 0) // type id
	g.asm.EmitLdrImm(2, 0, 1) // value
	g.opPush(2)               // receiver

	i = argCount - 1
	for i >= 0 {
		g.asm.EmitPop(1<<0, false)
		g.opPush(0)
		i = i - 1
	}

	if len(entries) == 0 {
		// Conservative fallback: nil/zero return for unresolved dispatch tables.
		g.asm.EmitMovsImm(0, 0)
		g.opPush(0)
		return nil
	}

	var endJumps []int
	i = 0
	for i < len(entries) {
		e := entries[i]
		g.loadImm32(3, uint32(e.TypeID))
		g.asm.EmitCmpRR(1, 3)
		skip := g.asm.EmitBCond(condNE, 0)
		g.EmitCallPlaceholder(e.FuncName)
		endJumps = append(endJumps, g.emitLongJumpPlaceholder())
		g.asm.PatchBCond(skip, condNE, int32(g.asm.Pos()-(skip+4)))
		i = i + 1
	}

	// Unknown type id for this interface method: return zero/nil.
	g.asm.EmitMovsImm(0, 0)
	g.opPush(0)

	end := g.asm.Pos()
	i = 0
	for i < len(endJumps) {
		g.jumpLits = append(g.jumpLits, jumpLiteralFixup{
			wordOff: endJumps[i],
			target:  end,
		})
		i = i + 1
	}
	return nil
}

//rtg:profile
func (g *CodeGen) loadImm32(reg uint8, v uint32) int {
	ldrOff := g.asm.EmitLdrLiteral(reg, 0)
	bOff := g.asm.EmitBImm11(0)
	for (g.asm.Pos() & 3) != 0 {
		g.asm.EmitNop()
	}
	off := g.asm.Pos()
	g.asm.Emit32(v)
	after := g.asm.Pos()
	g.asm.PatchBImm11(bOff, int32(after-(bOff+4)))
	base := (ldrOff + 4) &^ 3
	immWords := (off - base) / 4
	g.asm.Patch16(ldrOff, 0x4800|uint16(reg)<<8|uint16(immWords))
	return off
}

//rtg:profile
func (g *CodeGen) emitLongJumpPlaceholder() int {
	wordOff := g.loadImm32(3, 0)
	g.asm.Emit16(0x4718) // bx r3
	g.allJumpWord = append(g.allJumpWord, wordOff)
	return wordOff
}

//rtg:profile
func (g *CodeGen) localAddr(slot int, outReg uint8) {
	if slot < 0 {
		panic("negative local slot")
	}
	g.asm.EmitMovReg(outReg, 5)
	off := slot * 4
	for off > 0 {
		step := off
		if step > 255 {
			step = 255
		}
		g.asm.EmitAddsImm(outReg, uint8(step))
		off = off - step
	}
}

//rtg:profile
func (g *CodeGen) loadLocal(slot int, outReg uint8) {
	if slot < 0 {
		panic("negative local slot")
	}
	if slot <= 31 {
		g.asm.EmitLdrImm(outReg, 5, uint8(slot))
		return
	}
	g.localAddr(slot, 1)
	g.asm.EmitLdrImm(outReg, 1, 0)
}

//rtg:profile
func (g *CodeGen) storeLocal(slot int, inReg uint8) {
	if slot < 0 {
		panic("negative local slot")
	}
	if slot <= 31 {
		g.asm.EmitStrImm(inReg, 5, uint8(slot))
		return
	}
	g.localAddr(slot, 1)
	g.asm.EmitStrImm(inReg, 1, 0)
}

//rtg:profile
func (g *CodeGen) alignRodata(a int) {
	for (len(g.rodata) % a) != 0 {
		g.rodata = append(g.rodata, 0)
	}
}

//rtg:profile
func (g *CodeGen) internString(s string) int {
	if idx, ok := g.strIndex[s]; ok {
		return idx
	}
	g.alignRodata(4)
	headerOff := len(g.rodata)
	g.rodata = append(g.rodata, 0, 0, 0, 0, 0, 0, 0, 0)
	dataOff := len(g.rodata)
	i := 0
	for i < len(s) {
		g.rodata = append(g.rodata, s[i])
		i = i + 1
	}
	g.alignRodata(4)
	idx := len(g.strs)
	g.strs = append(g.strs, strEntry{
		headerOff: headerOff,
		dataOff:   dataOff,
		length:    len(s),
	})
	g.strIndex[s] = idx
	return idx
}

func putU32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}

//rtg:profile
func (g *CodeGen) finalizeRodata() {
	if len(g.rodata) == 0 {
		return
	}
	codeBase := uint32(0x10000000 + 0x100)
	for (g.asm.Pos() & 3) != 0 {
		g.asm.EmitNop()
	}
	rodataOff := g.asm.Pos()
	g.asm.code = append(g.asm.code, g.rodata...)
	i := 0
	for i < len(g.strs) {
		se := g.strs[i]
		headerAbs := codeBase + uint32(rodataOff+se.headerOff)
		dataAbs := codeBase + uint32(rodataOff+se.dataOff)
		putU32(g.asm.code[rodataOff+se.headerOff:], dataAbs)
		putU32(g.asm.code[rodataOff+se.headerOff+4:], uint32(se.length))
		_ = headerAbs
		i = i + 1
	}
	i = 0
	for i < len(g.strFix) {
		fx := g.strFix[i]
		se := g.strs[fx.index]
		headerAbs := codeBase + uint32(rodataOff+se.headerOff)
		g.asm.Patch32(fx.litOff, headerAbs)
		i = i + 1
	}
}
