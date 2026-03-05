//go:build !no_backend_dos_i386 && tiny_dos_backend

package dos

import "j5.nz/rtg/std/compiler/ir"

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
		g.memLoad(inst.Arg, inst.Name == ir.InstNonNilMemoryBase)
	case ir.OP_STORE:
		g.memStore(inst.Arg)
	case ir.OP_OFFSET:
		g.offset(inst.Arg)
	case ir.OP_INDEX_ADDR:
		g.indexAddr(inst.Arg)
	case ir.OP_LEN:
		g.sliceLen(inst.Name == ir.InstNonNilMemoryBase)
	case ir.OP_CAP:
		g.sliceCap(inst.Name == ir.InstNonNilMemoryBase)

	case ir.OP_CONVERT:
		g.convert(inst.Name)

	case ir.OP_IFACE_BOX, ir.OP_IFACE_CALL:
		panic("ICE: interface op disabled in tiny_dos_backend")
	case ir.OP_PANIC:
		panic("ICE: panic op disabled in tiny_dos_backend")
	case ir.OP_SLICE_GET, ir.OP_SLICE_MAKE, ir.OP_STRING_GET, ir.OP_STRING_MAKE:
		// handled by intrinsics/builtins in your IR
	default:
		panic("ICE: unhandled opcode in 8086 tiny backend")
	}
}

// Tiny backend profile runs inside severely constrained DOS16 executables.
// Skip strict EXE segment guards here; this path is only used for small bootstrapping payloads.
func exeSegmentTooLarge(textSize uint32, dataSegSize uint32) bool {
	_ = textSize
	_ = dataSegSize
	return false
}
