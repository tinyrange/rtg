package riscv

import (
	"j5.nz/rtg/std/compiler/backend/becommon"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

const debugCheckOperandStack = false

func (g *CodeGen) emitRet() {
	g.emitJalr(REG_ZERO, REG_RA, 0)
}

func (g *CodeGen) emitTrap() {
	g.emit32(0x00100073)
}

func (g *CodeGen) emitJalPlaceholder(rd int) int {
	return g.emitJal(rd, 0)
}

func (g *CodeGen) compileFunc(f *ir.IRFunc) {
	if f.Native != nil {
		panic("ICE: riscv backend does not support native func blobs")
	}
	g.curFunc = f
	g.curFrameSize = len(f.Locals)
	if f.Params > g.curFrameSize {
		g.curFrameSize = f.Params
	}
	for k := range g.labelOffsets {
		delete(g.labelOffsets, k)
	}
	g.jumpFixups = g.jumpFixups[:0]

	frameBytes := g.curFrameSize * g.wordSize
	saveBytes := 2 * g.wordSize
	if debugCheckOperandStack {
		saveBytes += g.wordSize
	}
	total := common.AlignUp(frameBytes+saveBytes, 16)
	localsArea := total - saveBytes
	g.emitAddImmConst(REG_SP, REG_SP, int64(-total), REG_T6)
	g.emitStoreAt(REG_FP, REG_SP, localsArea)
	g.emitStoreAt(REG_RA, REG_SP, localsArea+g.wordSize)
	if debugCheckOperandStack {
		g.emitStoreAt(REG_ZERO, REG_SP, localsArea+2*g.wordSize)
	}
	g.emitAddImmConst(REG_FP, REG_SP, int64(localsArea), REG_T6)

	for i := f.Params - 1; i >= 0; i-- {
		g.rawPop(REG_T0)
		g.emitStoreLocal(i, REG_T0)
	}
	if debugCheckOperandStack {
		g.emitStoreAt(REG_OPSP, REG_FP, 2*g.wordSize)
	}

	i := 0
	for i < len(f.Code) {
		inst := f.Code[i]
		if inst.Op == ir.OP_DROP {
			drops := 1
			j := i + 1
			for j < len(f.Code) && f.Code[j].Op == ir.OP_DROP {
				drops++
				j++
			}
			g.rawDropN(drops)
			i = j
			continue
		}
		g.compileInst(inst)
		i++
	}

	for _, fix := range g.jumpFixups {
		labelOff, ok := g.labelOffsets[fix.LabelID]
		if ok {
			g.patchJalAt(fix.CodeOffset, labelOff)
		}
	}

	g.curFunc = nil
}

func (g *CodeGen) emitEpilogue() {
	if debugCheckOperandStack {
		g.emitLoadAt(REG_T1, REG_FP, 2*g.wordSize)
		if g.curFunc != nil && g.curFunc.RetCount != 0 {
			g.emitSubImmConst(REG_T1, REG_T1, int64(g.curFunc.RetCount*g.wordSize), REG_T6)
		}
		g.emitBeq(REG_OPSP, REG_T1, 8)
		g.emitTrap()
	}
	g.emitLoadAt(REG_T0, REG_FP, 0)
	g.emitLoadAt(REG_RA, REG_FP, g.wordSize)
	saveBytes := 2 * g.wordSize
	if debugCheckOperandStack {
		saveBytes += g.wordSize
	}
	g.emitAddImmConst(REG_SP, REG_FP, int64(saveBytes), REG_T6)
	g.emitAdd(REG_FP, REG_T0, REG_ZERO)
	g.emitRet()
}

func (g *CodeGen) compileInst(inst ir.Inst) {
	switch inst.Op {
	case ir.OP_CONST_I64:
		if len(inst.Name) > 10 && inst.Name[0:10] == "$funcaddr$" {
			g.emitFuncAddrPlaceholder(inst.Name[10:])
		} else {
			g.emitImmToReg(REG_T0, inst.Val)
			g.rawPush(REG_T0)
		}
	case ir.OP_CONST_BOOL:
		if inst.Arg != 0 {
			g.emitAddi(REG_T0, REG_ZERO, 1)
		} else {
			g.emitAddi(REG_T0, REG_ZERO, 0)
		}
		g.rawPush(REG_T0)
	case ir.OP_CONST_NIL:
		g.rawPush(REG_ZERO)
	case ir.OP_CONST_STR:
		headerOff := g.internString(becommon.DecodeStringLiteral(inst.Name))
		g.emitAddrFixup(REG_T0, "$rodata$", uint64(headerOff))
		g.rawPush(REG_T0)

	case ir.OP_LOCAL_GET:
		g.emitLoadLocal(inst.Arg, REG_T0)
		g.rawPush(REG_T0)
	case ir.OP_LOCAL_SET:
		g.rawPop(REG_T0)
		g.emitStoreLocal(inst.Arg, REG_T0)
	case ir.OP_LOCAL_ADD_IMM:
		g.emitLoadLocal(inst.Arg, REG_T0)
		g.emitAddImmConst(REG_T0, REG_T0, inst.Val, REG_T6)
		g.emitStoreLocal(inst.Arg, REG_T0)
	case ir.OP_LOCAL_ADDR:
		g.emitLocalAddr(inst.Arg, REG_T0)
		g.rawPush(REG_T0)

	case ir.OP_GLOBAL_GET:
		g.emitLoadFixup(REG_T0, "$data$", uint64(inst.Arg*g.wordSize))
		g.rawPush(REG_T0)
	case ir.OP_GLOBAL_SET:
		g.rawPop(REG_T0)
		g.emitAddrFixup(REG_T1, "$data$", uint64(inst.Arg*g.wordSize))
		g.emitStoreWord(REG_T0, REG_T1, 0)
	case ir.OP_GLOBAL_ADDR:
		g.emitAddrFixup(REG_T0, "$data$", uint64(inst.Arg*g.wordSize))
		g.rawPush(REG_T0)

	case ir.OP_DROP:
		g.rawDrop()
	case ir.OP_DUP:
		g.rawLoad(REG_T0)
		g.rawPush(REG_T0)

	case ir.OP_ADD, ir.OP_SUB, ir.OP_MUL, ir.OP_DIV, ir.OP_MOD, ir.OP_AND, ir.OP_OR, ir.OP_XOR, ir.OP_SHL, ir.OP_SHR:
		g.compileBinOp(inst.Op)
	case ir.OP_NEG:
		g.rawPop(REG_T0)
		g.emitSub(REG_T0, REG_ZERO, REG_T0)
		g.rawPush(REG_T0)
	case ir.OP_NOT:
		g.rawPop(REG_T0)
		g.emitXori(REG_T0, REG_T0, 1)
		g.rawPush(REG_T0)

	case ir.OP_EQ, ir.OP_NEQ, ir.OP_LT, ir.OP_GT, ir.OP_LEQ, ir.OP_GEQ:
		g.compileComparePush(inst.Op)

	case ir.OP_LABEL:
		g.labelOffsets[inst.Arg] = len(g.code)
	case ir.OP_JMP:
		off := g.emitJalPlaceholder(REG_ZERO)
		g.jumpFixups = append(g.jumpFixups, JumpFixup{CodeOffset: off, LabelID: inst.Arg})
	case ir.OP_JMP_IF:
		g.rawPop(REG_T0)
		g.emitBeq(REG_T0, REG_ZERO, 8)
		off := g.emitJalPlaceholder(REG_ZERO)
		g.jumpFixups = append(g.jumpFixups, JumpFixup{CodeOffset: off, LabelID: inst.Arg})
	case ir.OP_JMP_IF_NOT:
		g.rawPop(REG_T0)
		g.emitBne(REG_T0, REG_ZERO, 8)
		off := g.emitJalPlaceholder(REG_ZERO)
		g.jumpFixups = append(g.jumpFixups, JumpFixup{CodeOffset: off, LabelID: inst.Arg})
	case ir.OP_JMP_EQ, ir.OP_JMP_NEQ, ir.OP_JMP_LT, ir.OP_JMP_GT, ir.OP_JMP_LEQ, ir.OP_JMP_GEQ:
		g.compileCompareJump(inst.Op, inst.Arg)

	case ir.OP_CALL:
		g.compileCall(inst)
	case ir.OP_CALL_INTRINSIC:
		g.compileCallIntrinsic(inst)
	case ir.OP_RETURN:
		g.emitEpilogue()

	case ir.OP_LOAD:
		g.compileLoad(inst)
	case ir.OP_STORE:
		g.compileStore(inst)
	case ir.OP_OFFSET:
		g.rawPop(REG_T0)
		if inst.Arg != 0 {
			g.emitAddImmConst(REG_T0, REG_T0, int64(inst.Arg), REG_T6)
		}
		g.rawPush(REG_T0)
	case ir.OP_INDEX_ADDR:
		g.compileIndexAddr(inst.Arg)
	case ir.OP_LEN:
		g.compileLen(inst)
	case ir.OP_CAP:
		g.compileCap(inst)
	case ir.OP_CONVERT:
		g.compileConvert(inst.Name)
	case ir.OP_IFACE_BOX:
		g.compileIfaceBox(inst)
	case ir.OP_IFACE_CALL:
		g.compileIfaceCall(inst)
	case ir.OP_PANIC:
		g.compilePanic()
	case ir.OP_SLICE_GET, ir.OP_SLICE_MAKE, ir.OP_STRING_GET, ir.OP_STRING_MAKE:
		// handled elsewhere
	default:
		panic("ICE: unhandled opcode in riscv backend")
	}
}

func (g *CodeGen) compileBinOp(op ir.Opcode) {
	g.rawPop(REG_T0)
	g.rawPop(REG_T1)
	switch op {
	case ir.OP_ADD:
		g.emitAdd(REG_T1, REG_T1, REG_T0)
	case ir.OP_SUB:
		g.emitSub(REG_T1, REG_T1, REG_T0)
	case ir.OP_MUL:
		g.emitMul(REG_T1, REG_T1, REG_T0)
	case ir.OP_DIV:
		g.emitDiv(REG_T1, REG_T1, REG_T0)
	case ir.OP_MOD:
		g.emitRem(REG_T1, REG_T1, REG_T0)
	case ir.OP_AND:
		g.emitAnd(REG_T1, REG_T1, REG_T0)
	case ir.OP_OR:
		g.emitOr(REG_T1, REG_T1, REG_T0)
	case ir.OP_XOR:
		g.emitXor(REG_T1, REG_T1, REG_T0)
	case ir.OP_SHL:
		g.emitSll(REG_T1, REG_T1, REG_T0)
	case ir.OP_SHR:
		g.emitSra(REG_T1, REG_T1, REG_T0)
	}
	g.rawPush(REG_T1)
}

func (g *CodeGen) compileComparePush(op ir.Opcode) {
	g.rawPop(REG_T0)
	g.rawPop(REG_T1)
	switch op {
	case ir.OP_EQ:
		g.emitXor(REG_T1, REG_T1, REG_T0)
		g.emitSltiu(REG_T1, REG_T1, 1)
	case ir.OP_NEQ:
		g.emitXor(REG_T1, REG_T1, REG_T0)
		g.emitSltu(REG_T1, REG_ZERO, REG_T1)
	case ir.OP_LT:
		g.emitSlt(REG_T1, REG_T1, REG_T0)
	case ir.OP_GT:
		g.emitSlt(REG_T1, REG_T0, REG_T1)
	case ir.OP_LEQ:
		g.emitSlt(REG_T1, REG_T0, REG_T1)
		g.emitXori(REG_T1, REG_T1, 1)
	case ir.OP_GEQ:
		g.emitSlt(REG_T1, REG_T1, REG_T0)
		g.emitXori(REG_T1, REG_T1, 1)
	}
	g.rawPush(REG_T1)
}

func (g *CodeGen) compileCompareJump(op ir.Opcode, label int) {
	g.rawPop(REG_T0)
	g.rawPop(REG_T1)
	switch op {
	case ir.OP_JMP_EQ:
		g.emitBne(REG_T1, REG_T0, 8)
	case ir.OP_JMP_NEQ:
		g.emitBeq(REG_T1, REG_T0, 8)
	case ir.OP_JMP_LT:
		g.emitSlt(REG_T2, REG_T1, REG_T0)
		g.emitBeq(REG_T2, REG_ZERO, 8)
	case ir.OP_JMP_GT:
		g.emitSlt(REG_T2, REG_T0, REG_T1)
		g.emitBeq(REG_T2, REG_ZERO, 8)
	case ir.OP_JMP_LEQ:
		g.emitSlt(REG_T2, REG_T0, REG_T1)
		g.emitBne(REG_T2, REG_ZERO, 8)
	case ir.OP_JMP_GEQ:
		g.emitSlt(REG_T2, REG_T1, REG_T0)
		g.emitBne(REG_T2, REG_ZERO, 8)
	}
	off := g.emitJalPlaceholder(REG_ZERO)
	g.jumpFixups = append(g.jumpFixups, JumpFixup{CodeOffset: off, LabelID: label})
}

func (g *CodeGen) compileCall(inst ir.Inst) {
	if len(inst.Name) > 18 && inst.Name[0:18] == "builtin.composite." {
		g.compileCompositeLitCall(inst)
		return
	}
	g.emitCallPlaceholder(inst.Name)
}

func (g *CodeGen) compileCompositeLitCall(inst ir.Inst) {
	fieldCount := inst.Arg
	structSize := fieldCount * g.wordSize
	if structSize == 0 {
		g.rawPush(REG_ZERO)
		return
	}
	saveBytes := common.AlignUp(fieldCount*g.wordSize, 16)
	g.emitAddImmConst(REG_SP, REG_SP, int64(-saveBytes), REG_T6)
	for i := 0; i < fieldCount; i++ {
		g.rawPop(REG_T0)
		saveOff := (fieldCount - 1 - i) * g.wordSize
		g.emitStoreAt(REG_T0, REG_SP, saveOff)
	}
	g.emitImmToReg(REG_T0, int64(structSize))
	g.rawPush(REG_T0)
	g.emitCallPlaceholder("runtime.Alloc")
	g.rawPop(REG_T1)
	for i := 0; i < fieldCount; i++ {
		g.emitLoadAt(REG_T0, REG_SP, i*g.wordSize)
		g.emitStoreAt(REG_T0, REG_T1, i*g.wordSize)
	}
	g.emitAddImmConst(REG_SP, REG_SP, int64(saveBytes), REG_T6)
	g.rawPush(REG_T1)
}

func (g *CodeGen) compileCallIntrinsic(inst ir.Inst) {
	switch inst.Name {
	case "Syscall":
		g.compileSyscallIntrinsic()
	case "Sliceptr":
		g.emitLoadLocal(0, REG_T0)
		g.emitLoadWord(REG_T0, REG_T0, 0)
		g.rawPush(REG_T0)
	case "Makeslice":
		g.emitImmToReg(REG_T0, int64(4*g.wordSize))
		g.rawPush(REG_T0)
		g.emitCallPlaceholder("runtime.Alloc")
		g.rawPop(REG_T1)
		g.emitLoadLocal(0, REG_T0)
		g.emitStoreAt(REG_T0, REG_T1, 0)
		g.emitLoadLocal(1, REG_T0)
		g.emitStoreAt(REG_T0, REG_T1, g.wordSize)
		g.emitLoadLocal(2, REG_T0)
		g.emitStoreAt(REG_T0, REG_T1, 2*g.wordSize)
		g.emitAddi(REG_T0, REG_ZERO, 1)
		g.emitStoreAt(REG_T0, REG_T1, 3*g.wordSize)
		g.rawPush(REG_T1)
	case "Stringptr":
		g.emitLoadLocal(0, REG_T0)
		g.emitLoadWord(REG_T0, REG_T0, 0)
		g.rawPush(REG_T0)
	case "Makestring":
		g.emitImmToReg(REG_T0, int64(2*g.wordSize))
		g.rawPush(REG_T0)
		g.emitCallPlaceholder("runtime.Alloc")
		g.rawPop(REG_T1)
		g.emitLoadLocal(0, REG_T0)
		g.emitStoreAt(REG_T0, REG_T1, 0)
		g.emitLoadLocal(1, REG_T0)
		g.emitStoreAt(REG_T0, REG_T1, g.wordSize)
		g.rawPush(REG_T1)
	case "Tostring":
		g.compileTostringIntrinsicBody()
	case "ReadPtr":
		g.emitLoadLocal(0, REG_T0)
		g.emitLoadWord(REG_T0, REG_T0, 0)
		g.rawPush(REG_T0)
	case "WritePtr":
		g.emitLoadLocal(0, REG_T0)
		g.emitLoadLocal(1, REG_T1)
		g.emitStoreWord(REG_T1, REG_T0, 0)
	case "WriteByte":
		g.emitLoadLocal(0, REG_T0)
		g.emitLoadLocal(1, REG_T1)
		g.emitStoreByte(REG_T1, REG_T0, 0)
	default:
		panic("ICE: unknown intrinsic in riscv backend: " + inst.Name)
	}
}

func (g *CodeGen) compileSyscallIntrinsic() {
	g.emitLoadLocal(0, REG_A7)
	g.emitLoadLocal(1, REG_A0)
	g.emitLoadLocal(2, REG_A1)
	g.emitLoadLocal(3, REG_A2)
	g.emitLoadLocal(4, REG_A3)
	g.emitLoadLocal(5, REG_A4)
	g.emitLoadLocal(6, REG_A5)
	g.emitEcall()
	g.emitAdd(REG_T2, REG_A1, REG_ZERO)
	g.emitSlt(REG_T0, REG_A0, REG_ZERO)
	g.emitBne(REG_T0, REG_ZERO, 32)
	g.rawPush(REG_A0)
	g.rawPush(REG_T2)
	g.rawPush(REG_ZERO)
	done := g.emitJalPlaceholder(REG_ZERO)
	g.rawPush(REG_ZERO)
	g.rawPush(REG_ZERO)
	g.emitSub(REG_T1, REG_ZERO, REG_A0)
	g.rawPush(REG_T1)
	g.patchJalAt(done, len(g.code))
}

func (g *CodeGen) compileTostringIntrinsicBody() {
	g.emitLoadLocal(0, REG_T0)
	g.emitLoadWord(REG_T1, REG_T0, 0)
	g.emitImmToReg(REG_T2, 256)
	g.emitSltu(REG_T3, REG_T1, REG_T2)
	g.emitBne(REG_T3, REG_ZERO, 8)
	stringCase := g.emitJalPlaceholder(REG_ZERO)
	g.emitLoadWord(REG_T2, REG_T0, int32(g.wordSize))
	g.rawPush(REG_T2)
	tmpBytes := 16
	g.emitAddImmConst(REG_SP, REG_SP, int64(-tmpBytes), REG_T6)
	g.emitStoreWord(REG_T1, REG_SP, 0)
	g.emitLoadWord(REG_T1, REG_SP, 0)
	g.emitAddImmConst(REG_SP, REG_SP, int64(tmpBytes), REG_T6)
	endFixups := make([]int, 0)
	g.emitAddi(REG_T2, REG_ZERO, 1)
	g.emitBne(REG_T1, REG_T2, 16)
	g.emitCallPlaceholder("runtime.IntToString")
	endFixups = append(endFixups, g.emitJalPlaceholder(REG_ZERO))
	g.emitAddi(REG_T2, REG_ZERO, 2)
	g.emitBne(REG_T1, REG_T2, 8)
	endFixups = append(endFixups, g.emitJalPlaceholder(REG_ZERO))
	entries := sortedDispatchEntries(g.irmod, ".Error")
	if len(entries) == 0 {
		entries = sortedDispatchEntries(g.irmod, ".String")
	}
	for _, entry := range entries {
		g.emitImmToReg(REG_T2, int64(entry.TypeID))
		g.emitBne(REG_T1, REG_T2, 16)
		g.emitCallPlaceholder(entry.FuncName)
		endFixups = append(endFixups, g.emitJalPlaceholder(REG_ZERO))
	}
	g.rawDrop()
	g.rawPush(REG_ZERO)
	finalDone := len(g.code)
	for _, off := range endFixups {
		g.patchJalAt(off, finalDone)
	}
	finalSkip := g.emitJalPlaceholder(REG_ZERO)
	strLabel := len(g.code)
	g.emitLoadLocal(0, REG_T0)
	g.rawPush(REG_T0)
	g.patchJalAt(stringCase, strLabel)
	g.patchJalAt(finalSkip, len(g.code))
}

func (g *CodeGen) compileIfaceBox(inst ir.Inst) {
	g.rawPop(REG_T0)
	g.emitAddImmConst(REG_SP, REG_SP, -16, REG_T6)
	g.emitStoreWord(REG_T0, REG_SP, 0)
	g.emitImmToReg(REG_T0, int64(2*g.wordSize))
	g.rawPush(REG_T0)
	g.emitCallPlaceholder("runtime.Alloc")
	g.rawPop(REG_T1)
	g.emitImmToReg(REG_T0, int64(inst.Arg))
	g.emitStoreAt(REG_T0, REG_T1, 0)
	g.emitLoadAt(REG_T0, REG_SP, 0)
	g.emitAddImmConst(REG_SP, REG_SP, 16, REG_T6)
	g.emitStoreAt(REG_T0, REG_T1, g.wordSize)
	g.rawPush(REG_T1)
}

func (g *CodeGen) compileIfaceCall(inst ir.Inst) {
	argCount := inst.Arg
	saveBytes := common.AlignUp(argCount*g.wordSize, 16)
	if saveBytes > 0 {
		g.emitAddImmConst(REG_SP, REG_SP, int64(-saveBytes), REG_T6)
		for i := 0; i < argCount; i++ {
			g.rawPop(REG_T0)
			g.emitStoreAt(REG_T0, REG_SP, i*g.wordSize)
		}
	}
	g.rawPop(REG_T0)
	g.emitLoadWord(REG_T1, REG_T0, 0)
	g.emitLoadWord(REG_T2, REG_T0, int32(g.wordSize))
	g.rawPush(REG_T2)
	for i := argCount - 1; i >= 0; i-- {
		g.emitLoadAt(REG_T0, REG_SP, i*g.wordSize)
		g.rawPush(REG_T0)
	}
	if saveBytes > 0 {
		g.emitAddImmConst(REG_SP, REG_SP, int64(saveBytes), REG_T6)
	}
	tmpBytes := 16
	g.emitAddImmConst(REG_SP, REG_SP, int64(-tmpBytes), REG_T6)
	g.emitStoreWord(REG_T1, REG_SP, 0)
	methodName := inst.Name
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
			cand := typeName + "." + bare
			if _, ok := g.irmod.MethodTable[cand]; ok {
				entries = append(entries, becommon.DispatchEntry{TypeID: tid, FuncName: cand})
			}
		}
	}
	for i := 1; i < len(entries); i++ {
		j := i
		for j > 0 && (entries[j].TypeID < entries[j-1].TypeID || (entries[j].TypeID == entries[j-1].TypeID && entries[j].FuncName < entries[j-1].FuncName)) {
			entries[j], entries[j-1] = entries[j-1], entries[j]
			j--
		}
	}
	g.emitLoadWord(REG_T1, REG_SP, 0)
	g.emitAddImmConst(REG_SP, REG_SP, int64(tmpBytes), REG_T6)
	endFixups := make([]int, 0)
	for _, entry := range entries {
		g.emitImmToReg(REG_T2, int64(entry.TypeID))
		g.emitBne(REG_T1, REG_T2, 16)
		g.emitCallPlaceholder(entry.FuncName)
		endFixups = append(endFixups, g.emitJalPlaceholder(REG_ZERO))
	}
	g.emitTrap()
	end := len(g.code)
	for _, off := range endFixups {
		g.patchJalAt(off, end)
	}
}

func (g *CodeGen) compileLoad(inst ir.Inst) {
	size := inst.Arg
	offset := int(inst.Val)
	g.rawPop(REG_T1)
	if offset != 0 && !ir.IsNonNilMemoryBase(inst.Name) {
		g.emitAddImmConst(REG_T1, REG_T1, int64(offset), REG_T6)
		offset = 0
	}
	if ir.IsNonNilMemoryBase(inst.Name) {
		if size == 1 {
			g.emitLoadByteAt(REG_T0, REG_T1, offset)
		} else {
			g.emitLoadAt(REG_T0, REG_T1, offset)
		}
		g.rawPush(REG_T0)
		return
	}
	g.emitBne(REG_T1, REG_ZERO, 12)
	g.emitAddi(REG_T0, REG_ZERO, 0)
	skip := g.emitJalPlaceholder(REG_ZERO)
	if size == 1 {
		g.emitLoadByteAt(REG_T0, REG_T1, offset)
	} else {
		g.emitLoadAt(REG_T0, REG_T1, offset)
	}
	g.patchJalAt(skip, len(g.code))
	g.rawPush(REG_T0)
}

func (g *CodeGen) compileStore(inst ir.Inst) {
	size := inst.Arg
	offset := int(inst.Val)
	g.rawPop(REG_T1)
	g.rawPop(REG_T0)
	if size == 1 {
		g.emitStoreByteAt(REG_T0, REG_T1, offset)
	} else {
		g.emitStoreAt(REG_T0, REG_T1, offset)
	}
}

func (g *CodeGen) compileIndexAddr(elemSize int) {
	g.rawPop(REG_T0)
	g.rawPop(REG_T1)
	g.emitLoadWord(REG_T1, REG_T1, 0)
	if elemSize == 1 {
		g.emitAdd(REG_T1, REG_T1, REG_T0)
	} else if elemSize == 2 {
		g.emitSlli(REG_T0, REG_T0, 1)
		g.emitAdd(REG_T1, REG_T1, REG_T0)
	} else if elemSize == 4 {
		g.emitSlli(REG_T0, REG_T0, 2)
		g.emitAdd(REG_T1, REG_T1, REG_T0)
	} else if elemSize == 8 {
		g.emitSlli(REG_T0, REG_T0, 3)
		g.emitAdd(REG_T1, REG_T1, REG_T0)
	} else {
		g.emitImmToReg(REG_T2, int64(elemSize))
		g.emitMul(REG_T0, REG_T0, REG_T2)
		g.emitAdd(REG_T1, REG_T1, REG_T0)
	}
	g.rawPush(REG_T1)
}

func (g *CodeGen) compileLen(inst ir.Inst) {
	g.rawPop(REG_T0)
	if ir.IsNonNilMemoryBase(inst.Name) {
		g.emitLoadAt(REG_T0, REG_T0, g.wordSize)
		g.rawPush(REG_T0)
		return
	}
	g.emitBne(REG_T0, REG_ZERO, 12)
	g.emitAddi(REG_T0, REG_ZERO, 0)
	skip := g.emitJalPlaceholder(REG_ZERO)
	g.emitLoadAt(REG_T0, REG_T0, g.wordSize)
	g.patchJalAt(skip, len(g.code))
	g.rawPush(REG_T0)
}

func (g *CodeGen) compileCap(inst ir.Inst) {
	g.rawPop(REG_T0)
	if ir.IsNonNilMemoryBase(inst.Name) {
		g.emitLoadAt(REG_T0, REG_T0, 2*g.wordSize)
		g.rawPush(REG_T0)
		return
	}
	g.emitBne(REG_T0, REG_ZERO, 12)
	g.emitAddi(REG_T0, REG_ZERO, 0)
	skip := g.emitJalPlaceholder(REG_ZERO)
	g.emitLoadAt(REG_T0, REG_T0, 2*g.wordSize)
	g.patchJalAt(skip, len(g.code))
	g.rawPush(REG_T0)
}

func (g *CodeGen) compileConvert(typeName string) {
	switch typeName {
	case "string":
		g.emitCallPlaceholder("runtime.BytesToString")
	case "[]byte":
		g.emitCallPlaceholder("runtime.StringToBytes")
	case "int", "uintptr", "uint", "int64", "uint64", "int32", "uint32":
		if g.wordSize == 8 && typeName == "int32" {
			g.rawPop(REG_T0)
			g.emitAddiw(REG_T0, REG_T0, 0)
			g.rawPush(REG_T0)
		} else if g.wordSize == 8 && typeName == "uint32" {
			g.rawPop(REG_T0)
			g.emitSlli(REG_T0, REG_T0, 32)
			g.emitSrli(REG_T0, REG_T0, 32)
			g.rawPush(REG_T0)
		}
	case "byte", "uint8":
		g.rawPop(REG_T0)
		g.emitAndi(REG_T0, REG_T0, 0xff)
		g.rawPush(REG_T0)
	case "int8":
		g.rawPop(REG_T0)
		g.emitSlli(REG_T0, REG_T0, int32(g.wordSize*8-8))
		g.emitSrai(REG_T0, REG_T0, int32(g.wordSize*8-8))
		g.rawPush(REG_T0)
	case "uint16":
		g.rawPop(REG_T0)
		g.emitSlli(REG_T0, REG_T0, int32(g.wordSize*8-16))
		g.emitSrli(REG_T0, REG_T0, int32(g.wordSize*8-16))
		g.rawPush(REG_T0)
	case "int16":
		g.rawPop(REG_T0)
		g.emitSlli(REG_T0, REG_T0, int32(g.wordSize*8-16))
		g.emitSrai(REG_T0, REG_T0, int32(g.wordSize*8-16))
		g.rawPush(REG_T0)
	}
}

func (g *CodeGen) compilePanic() {
	g.rawPop(REG_T0)
	g.emitLoadWord(REG_T1, REG_T0, 0)
	g.emitImmToReg(REG_T2, 256)
	g.emitSltu(REG_T3, REG_T1, REG_T2)
	g.emitBeq(REG_T3, REG_ZERO, 8)
	g.emitLoadWord(REG_T0, REG_T0, int32(g.wordSize))
	g.emitLoadWord(REG_A1, REG_T0, 0)
	g.emitLoadAt(REG_A2, REG_T0, g.wordSize)
	g.emitAddi(REG_A0, REG_ZERO, 2)
	g.emitAddi(REG_A7, REG_ZERO, 64)
	g.emitEcall()
	g.emitAddImmConst(REG_SP, REG_SP, -16, REG_T6)
	g.emitAddi(REG_T0, REG_ZERO, 10)
	g.emitStoreByte(REG_T0, REG_SP, 0)
	g.emitAddi(REG_A0, REG_ZERO, 2)
	g.emitAdd(REG_A1, REG_SP, REG_ZERO)
	g.emitAddi(REG_A2, REG_ZERO, 1)
	g.emitAddi(REG_A7, REG_ZERO, 64)
	g.emitEcall()
	g.emitAddImmConst(REG_SP, REG_SP, 16, REG_T6)
	g.emitAddi(REG_A0, REG_ZERO, 2)
	g.emitAddi(REG_A7, REG_ZERO, 94)
	g.emitEcall()
}
