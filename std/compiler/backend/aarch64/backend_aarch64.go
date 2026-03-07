//go:build !no_backend_arm64

package aarch64

import (
	"j5.nz/rtg/std/compiler/backend/becommon"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// === ARM64 Code Generation ===
// Mirrors backend_x64.go but emits ARM64 instructions.
// Uses X0-X3 as working registers, X28 as operand stack pointer,
// X29 (FP) as frame pointer, X30 (LR) as link register.

func (g *CodeGen) initOperandCacheArm64() {
	g.configureOperandCache(REG_X26, REG_X27)
}

func (g *CodeGen) funcABIArm64(name string) string {
	if g.irmod == nil || g.irmod.FuncABIs == nil {
		return ""
	}
	return g.irmod.FuncABIs[name]
}

func (g *CodeGen) funcRetCountArm64(name string) int {
	if g.irmod == nil || g.irmod.FuncRetCounts == nil {
		return 0
	}
	return g.irmod.FuncRetCounts[name]
}

func (g *CodeGen) isNativeCABIArm64(name string) bool {
	return g.funcABIArm64(name) == "native-c-darwin-arm64"
}

func (g *CodeGen) usesFrameOperandStackArm64(name string) bool {
	return g.target != nil && g.target.RelocatableObject && g.isNativeCABIArm64(name)
}

func arm64IntrinsicRetCount(name string) int {
	switch name {
	case "SysGetargc", "SysGetargv", "SysGetenvp":
		return 3
	case "SysArgcValue", "SysArgvBaseValue", "Alloc", "Sliceptr", "Makeslice", "Stringptr", "Makestring", "Tostring", "ReadPtr":
		return 1
	case "WritePtr", "WriteByte":
		return 0
	default:
		return 0
	}
}

func (g *CodeGen) nativeEvalStackSlotsArm64(f *ir.IRFunc) int {
	if f == nil {
		return 0
	}
	depth := 0
	maxDepth := 0
	for _, inst := range f.Code {
		switch inst.Op {
		case ir.OP_CONST_I64, ir.OP_CONST_STR, ir.OP_CONST_BOOL, ir.OP_CONST_NIL:
			depth++
		case ir.OP_LOCAL_GET, ir.OP_GLOBAL_GET, ir.OP_LOCAL_ADDR, ir.OP_GLOBAL_ADDR:
			depth++
		case ir.OP_LOCAL_SET, ir.OP_GLOBAL_SET, ir.OP_DROP, ir.OP_JMP_IF, ir.OP_JMP_IF_NOT, ir.OP_PANIC:
			depth--
		case ir.OP_LOCAL_ADD_IMM, ir.OP_NEG, ir.OP_NOT, ir.OP_LOAD, ir.OP_OFFSET, ir.OP_LABEL, ir.OP_JMP, ir.OP_LEN, ir.OP_CAP, ir.OP_CONVERT, ir.OP_IFACE_BOX:
			// Net-zero stack effect.
		case ir.OP_DUP:
			depth++
		case ir.OP_ADD, ir.OP_SUB, ir.OP_MUL, ir.OP_DIV, ir.OP_MOD, ir.OP_AND, ir.OP_OR, ir.OP_XOR, ir.OP_SHL, ir.OP_SHR, ir.OP_EQ, ir.OP_NEQ, ir.OP_LT, ir.OP_GT, ir.OP_LEQ, ir.OP_GEQ, ir.OP_INDEX_ADDR:
			depth--
		case ir.OP_STORE:
			depth -= 2
		case ir.OP_JMP_EQ, ir.OP_JMP_NEQ, ir.OP_JMP_LT, ir.OP_JMP_GT, ir.OP_JMP_LEQ, ir.OP_JMP_GEQ:
			depth -= 2
		case ir.OP_CALL:
			retCount := g.funcRetCountArm64(inst.Name)
			if len(inst.Name) > 18 && inst.Name[0:18] == "builtin.composite." {
				retCount = 1
			}
			depth += retCount - inst.Arg
		case ir.OP_CALL_INTRINSIC:
			depth += arm64IntrinsicRetCount(inst.Name)
		case ir.OP_RETURN:
			depth -= inst.Arg
		case ir.OP_IFACE_CALL:
			depth -= inst.Arg
			depth++
		}
		if depth > maxDepth {
			maxDepth = depth
		}
		if depth < 0 {
			depth = 0
		}
	}
	return maxDepth
}

// CompileFuncArm64 generates ARM64 code for a single IR function.
func (g *CodeGen) CompileFuncArm64(f *ir.IRFunc) {
	if f.Native != nil {
		if f.Native.Arch != "arm64" {
			panic("ICE: arm64 backend received native function for arch " + f.Native.Arch)
		}
		funcStart := len(g.code)
		g.code = append(g.code, f.Native.Code...)
		for _, fx := range f.Native.Fixups {
			if fx.Kind != ir.NativeFixupCallRel32 {
				continue
			}
			g.callFixups = append(g.callFixups, CallFixup{funcStart + fx.Off, fx.Target, 0})
		}
		return
	}
	g.curFunc = f
	g.ClearOperandCache()
	g.curFrameSize = len(f.Locals)
	if f.Params > g.curFrameSize {
		g.curFrameSize = f.Params
	}
	g.curNativeSavedOpStackOffset = 0
	g.curNativeEvalSlots = 0
	if g.usesFrameOperandStackArm64(f.Name) {
		g.curNativeEvalSlots = g.nativeEvalStackSlotsArm64(f)
		g.curNativeSavedOpStackOffset = (g.curFrameSize + 1) * 8
		g.curFrameSize = g.curFrameSize + 1 + g.curNativeEvalSlots
	}
	g.labelOffsets = make(map[int]int)
	g.jumpFixups = nil

	// Prologue: STP X29, X30, [SP, #-16]!; MOV X29, SP; SUB SP, SP, #frameBytes
	g.EmitStp(REG_FP, REG_LR, REG_SP, -16)
	g.EmitMovRRArm64(REG_FP, REG_SP)

	frameBytes := g.curFrameSize * 8
	// Align to 16 bytes (ARM64 SP must be 16-byte aligned)
	if frameBytes%16 != 0 {
		frameBytes = frameBytes + (16 - frameBytes%16)
	}
	if frameBytes > 0 {
		if frameBytes < 4096 {
			g.emitSubImm(REG_SP, REG_SP, uint32(frameBytes))
		} else {
			g.EmitLoadImm64Compact(REG_X16, uint64(frameBytes))
			g.emitSubRR(REG_SP, REG_SP, REG_X16)
		}
	}

	if g.usesFrameOperandStackArm64(f.Name) {
		g.EmitStr(REG_X28, REG_FP, -g.curNativeSavedOpStackOffset)
		if g.curNativeSavedOpStackOffset < 4096 {
			g.emitSubImm(REG_X28, REG_FP, uint32(g.curNativeSavedOpStackOffset))
		} else {
			g.EmitLoadImm64Compact(REG_X16, uint64(g.curNativeSavedOpStackOffset))
			g.emitSubRR(REG_X28, REG_FP, REG_X16)
		}
	}

	if g.isNativeCABIArm64(f.Name) {
		i := 0
		for i < f.Params {
			offset := (i + 1) * 8
			if i < 8 {
				g.emitStoreLocalArm64(offset, REG_X0+i)
			} else {
				g.emitLdr(REG_X16, REG_FP, 16+(i-8)*8)
				g.emitStoreLocalArm64(offset, REG_X16)
			}
			i++
		}
	} else {
		// Pop params from operand stack (X28) into local frame slots
		if f.Params > 0 {
			i := f.Params - 1
			for i >= 0 {
				g.opPop(REG_X0)
				offset := (i + 1) * 8
				g.emitStoreLocalArm64(offset, REG_X0)
				i = i - 1
			}
		}
	}

	// Compile instructions
	if g.traceEnabled {
		g.traceCurFuncInsts = make([]InstByteTrace, len(f.Code))
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
			if g.traceEnabled {
				g.traceSetCurrentInst(i)
			}
			g.opDropN(drops)
			i = j
			continue
		}
		if g.traceEnabled {
			g.traceSetCurrentInst(i)
		}
		g.compileInstArm64(inst)
		i++
	}
	if g.traceEnabled {
		g.traceClearCurrentInst()
		g.traceByFunc[f.Name] = g.traceCurFuncInsts
		g.traceCurFuncInsts = nil
	}

	// Resolve jump fixups within this function
	for _, fix := range g.jumpFixups {
		labelOff, ok := g.labelOffsets[fix.LabelID]
		if !ok {
			continue
		}
		// Determine if this is a B.cond or B instruction
		existing := common.GetU32(g.code[fix.CodeOffset : fix.CodeOffset+4])
		if existing&0xFF000010 == 0x54000000 {
			// B.cond
			g.patchArm64BCondAt(fix.CodeOffset, labelOff)
		} else {
			// B
			g.PatchArm64BAt(fix.CodeOffset, labelOff)
		}
	}

	g.curFunc = nil
}

// compileInstArm64 generates ARM64 code for a single IR instruction.
func (g *CodeGen) compileInstArm64(inst ir.Inst) {
	switch inst.Op {
	case ir.OP_CONST_I64:
		g.compileConstI64Arm64(inst.Val)
	case ir.OP_CONST_F32:
		g.compileConstF32Arm64(inst.Name)
	case ir.OP_CONST_F64:
		g.compileConstF64Arm64(inst.Name)
	case ir.OP_CONST_BOOL:
		if inst.Arg != 0 {
			g.compileConstI64Arm64(1)
		} else {
			g.compileConstI64Arm64(0)
		}
	case ir.OP_CONST_NIL:
		g.compileConstI64Arm64(0)
	case ir.OP_CONST_STR:
		g.compileConstStrArm64(inst.Name)

	case ir.OP_LOCAL_GET:
		g.compileLocalGetArm64(inst.Arg)
	case ir.OP_LOCAL_SET:
		g.compileLocalSetArm64(inst.Arg)
	case ir.OP_LOCAL_ADD_IMM:
		g.compileLocalAddImmArm64(inst.Arg, int32(inst.Val))
	case ir.OP_LOCAL_ADDR:
		g.compileLocalAddrArm64(inst.Arg)

	case ir.OP_GLOBAL_GET:
		g.compileGlobalGetArm64(inst)
	case ir.OP_GLOBAL_SET:
		g.compileGlobalSetArm64(inst)
	case ir.OP_GLOBAL_ADDR:
		g.compileGlobalAddrArm64(inst)

	case ir.OP_DROP:
		g.opDrop()
	case ir.OP_DUP:
		g.opLoad(REG_X0)
		g.opPush(REG_X0)

	case ir.OP_ADD, ir.OP_SUB, ir.OP_MUL, ir.OP_DIV, ir.OP_MOD:
		if isFloatTypeNameArm64(inst.Name) {
			g.compileFloatBinOpArm64(inst.Op, inst.Name)
		} else {
			g.compileBinOpArm64(inst.Op)
		}
	case ir.OP_NEG:
		if isFloatTypeNameArm64(inst.Name) {
			g.compileFloatNegArm64(inst.Name)
		} else {
			g.opPop(REG_X0)
			g.emitNeg(REG_X0, REG_X0)
			g.opPush(REG_X0)
		}

	case ir.OP_AND, ir.OP_OR, ir.OP_XOR, ir.OP_SHL, ir.OP_SHR:
		g.compileBinOpArm64(inst.Op)

	case ir.OP_EQ:
		if isFloatTypeNameArm64(inst.Name) {
			g.compileFloatCompareArm64(inst.Op, inst.Name)
		} else {
			g.compileCompareArm64(COND_EQ)
		}
	case ir.OP_NEQ:
		if isFloatTypeNameArm64(inst.Name) {
			g.compileFloatCompareArm64(inst.Op, inst.Name)
		} else {
			g.compileCompareArm64(COND_NE)
		}
	case ir.OP_LT:
		if isFloatTypeNameArm64(inst.Name) {
			g.compileFloatCompareArm64(inst.Op, inst.Name)
		} else {
			g.compileCompareArm64(COND_LT)
		}
	case ir.OP_GT:
		if isFloatTypeNameArm64(inst.Name) {
			g.compileFloatCompareArm64(inst.Op, inst.Name)
		} else {
			g.compileCompareArm64(COND_GT)
		}
	case ir.OP_LEQ:
		if isFloatTypeNameArm64(inst.Name) {
			g.compileFloatCompareArm64(inst.Op, inst.Name)
		} else {
			g.compileCompareArm64(COND_LE)
		}
	case ir.OP_GEQ:
		if isFloatTypeNameArm64(inst.Name) {
			g.compileFloatCompareArm64(inst.Op, inst.Name)
		} else {
			g.compileCompareArm64(COND_GE)
		}

	case ir.OP_NOT:
		g.opPop(REG_X0)
		g.emitEorImm1(REG_X0, REG_X0) // XOR with 1
		g.opPush(REG_X0)

	case ir.OP_LABEL:
		g.Flush()
		g.labelOffsets[inst.Arg] = len(g.code)
	case ir.OP_JMP:
		g.Flush()
		fixup := g.emitB()
		g.jumpFixups = append(g.jumpFixups, JumpFixup{fixup, inst.Arg, 0, 0})
	case ir.OP_JMP_IF:
		g.opPop(REG_X0)
		g.emitCmpImm(REG_X0, 0)
		g.Flush()
		fixup := g.emitBCond(COND_NE)
		g.jumpFixups = append(g.jumpFixups, JumpFixup{fixup, inst.Arg, 0, 0})
	case ir.OP_JMP_IF_NOT:
		g.opPop(REG_X0)
		g.emitCmpImm(REG_X0, 0)
		g.Flush()
		fixup := g.emitBCond(COND_EQ)
		g.jumpFixups = append(g.jumpFixups, JumpFixup{fixup, inst.Arg, 0, 0})
	case ir.OP_JMP_EQ:
		if isFloatTypeNameArm64(inst.Name) {
			g.compileFloatCompareJumpArm64(inst.Op, inst.Arg, inst.Name)
		} else {
			g.compileCompareJumpArm64(COND_EQ, inst.Arg)
		}
	case ir.OP_JMP_NEQ:
		if isFloatTypeNameArm64(inst.Name) {
			g.compileFloatCompareJumpArm64(inst.Op, inst.Arg, inst.Name)
		} else {
			g.compileCompareJumpArm64(COND_NE, inst.Arg)
		}
	case ir.OP_JMP_LT:
		if isFloatTypeNameArm64(inst.Name) {
			g.compileFloatCompareJumpArm64(inst.Op, inst.Arg, inst.Name)
		} else {
			g.compileCompareJumpArm64(COND_LT, inst.Arg)
		}
	case ir.OP_JMP_GT:
		if isFloatTypeNameArm64(inst.Name) {
			g.compileFloatCompareJumpArm64(inst.Op, inst.Arg, inst.Name)
		} else {
			g.compileCompareJumpArm64(COND_GT, inst.Arg)
		}
	case ir.OP_JMP_LEQ:
		if isFloatTypeNameArm64(inst.Name) {
			g.compileFloatCompareJumpArm64(inst.Op, inst.Arg, inst.Name)
		} else {
			g.compileCompareJumpArm64(COND_LE, inst.Arg)
		}
	case ir.OP_JMP_GEQ:
		if isFloatTypeNameArm64(inst.Name) {
			g.compileFloatCompareJumpArm64(inst.Op, inst.Arg, inst.Name)
		} else {
			g.compileCompareJumpArm64(COND_GE, inst.Arg)
		}

	case ir.OP_CALL:
		g.compileCallArm64(inst)
	case ir.OP_CALL_INTRINSIC:
		g.compileCallIntrinsicArm64(inst)
	case ir.OP_RETURN:
		g.compileReturnArm64(inst)

	case ir.OP_LOAD:
		g.compileLoadArm64(inst)
	case ir.OP_STORE:
		g.compileStoreArm64(inst)
	case ir.OP_OFFSET:
		g.compileOffsetArm64(inst)
	case ir.OP_INDEX_ADDR:
		g.compileIndexAddrArm64(inst.Arg)
	case ir.OP_LEN:
		g.compileLenArm64(inst)
	case ir.OP_CAP:
		g.compileCapArm64(inst)

	case ir.OP_CONVERT:
		g.compileConvertArm64(inst)

	case ir.OP_IFACE_BOX:
		g.compileIfaceBoxArm64(inst)
	case ir.OP_IFACE_CALL:
		g.compileIfaceCallArm64(inst)
	case ir.OP_PANIC:
		if g.target.GOOS == "linux" {
			g.compilePanicArm64Linux()
		} else if g.target.GOOS == "windows" {
			g.compilePanicArm64Windows()
		} else {
			g.compilePanicArm64()
		}

	case ir.OP_SLICE_GET, ir.OP_SLICE_MAKE, ir.OP_STRING_GET, ir.OP_STRING_MAKE:
		// Handled by intrinsics

	default:
		panic("ICE: unhandled opcode in compileInstArm64")
	}
}

// === Constant loading ===

func (g *CodeGen) compileConstI64Arm64(val int64) {
	g.prepareForClobber(REG_X0)
	g.EmitLoadImm64Compact(REG_X0, uint64(val))
	g.opPush(REG_X0)
}

func (g *CodeGen) compileConstF32Arm64(lit string) {
	g.prepareForClobber(REG_X0)
	bits, ok := parseFloatLiteralBitsArm64(lit)
	if !ok {
		bits = 0
	}
	g.EmitLoadImm64Compact(REG_X0, uint64(float32BitsFromFloat64Bits(bits)))
	g.opPush(REG_X0)
}

func (g *CodeGen) compileConstF64Arm64(lit string) {
	g.prepareForClobber(REG_X0)
	bits, ok := parseFloatLiteralBitsArm64(lit)
	if !ok {
		bits = 0
	}
	g.EmitLoadImm64Compact(REG_X0, bits)
	g.opPush(REG_X0)
}

func (g *CodeGen) compileConstStrArm64(s string) {
	g.prepareForClobber(REG_X0, REG_X1)
	decoded := becommon.DecodeStringLiteral(s)

	headerOff, ok := g.stringMap[decoded]
	var rodataOff int
	if !ok {
		// String bytes go into rodata (read-only __TEXT,__const)
		rodataOff = len(g.rodata)
		i := 0
		for i < len(decoded) {
			g.rodata = append(g.rodata, decoded[i])
			i++
		}

		// String header goes into data (writable __DATA)
		headerOff = len(g.data)
		// data_ptr: leave as 0 (will be computed at runtime via ADRP+ADD)
		g.data = append(g.data, 0, 0, 0, 0, 0, 0, 0, 0)
		// length
		g.data = append(g.data, 0, 0, 0, 0, 0, 0, 0, 0)
		common.PutU64(g.data[headerOff+8:headerOff+16], uint64(len(decoded)))

		g.stringMap[decoded] = headerOff
		g.stringHeaderOff = append(g.stringHeaderOff, headerOff)
		if g.stringRodataMap == nil {
			g.stringRodataMap = make(map[int]int)
		}
		g.stringRodataMap[headerOff] = rodataOff
	} else {
		rodataOff = g.stringRodataMap[headerOff]
	}

	// Compute string data address at runtime (PC-relative ADRP+ADD, works with ASLR)
	// and store it into the header's data_ptr field in __DATA (writable)
	g.EmitAdrpAdd(REG_X1, "$rodata_header$", uint64(rodataOff)) // X1 = actual string data addr
	g.EmitAdrpAdd(REG_X0, "$data_addr$", uint64(headerOff))     // X0 = header addr in __DATA
	g.EmitStr(REG_X1, REG_X0, 0)                                // [header+0] = data addr

	// Push header address onto operand stack
	g.opPush(REG_X0)
}

// === Local variable access ===

func (g *CodeGen) compileLocalGetArm64(idx int) {
	g.prepareForClobber(REG_X0)
	offset := (idx + 1) * 8
	g.emitLoadLocalArm64(offset, REG_X0)
	g.opPush(REG_X0)
}

func (g *CodeGen) compileLocalSetArm64(idx int) {
	g.opPop(REG_X0)
	offset := (idx + 1) * 8
	g.emitStoreLocalArm64(offset, REG_X0)
}

func (g *CodeGen) compileLocalAddImmArm64(idx int, imm int32) {
	offset := (idx + 1) * 8
	g.emitLoadLocalArm64(offset, REG_X0)
	if imm >= 0 && imm < 4096 {
		g.emitAddImm(REG_X0, REG_X0, uint32(imm))
	} else if imm < 0 && imm > -4096 {
		g.emitSubImm(REG_X0, REG_X0, uint32(-imm))
	} else {
		g.EmitLoadImm64Compact(REG_X1, uint64(int64(imm)))
		g.EmitAddRR(REG_X0, REG_X0, REG_X1)
	}
	g.emitStoreLocalArm64(offset, REG_X0)
}

func (g *CodeGen) compileLocalAddrArm64(idx int) {
	g.prepareForClobber(REG_X0)
	offset := (idx + 1) * 8
	g.emitLeaLocalArm64(offset, REG_X0)
	g.opPush(REG_X0)
}

// === Global variable access ===

func (g *CodeGen) compileGlobalGetArm64(inst ir.Inst) {
	g.prepareForClobber(REG_X0)
	g.emitAdrpLdr(REG_X0, "$data_addr$", uint64(inst.Arg*8))
	g.opPush(REG_X0)
}

func (g *CodeGen) compileGlobalSetArm64(inst ir.Inst) {
	g.opPop(REG_X0)
	g.EmitAdrpAdd(REG_X1, "$data_addr$", uint64(inst.Arg*8))
	g.EmitStr(REG_X0, REG_X1, 0)
}

func (g *CodeGen) compileGlobalAddrArm64(inst ir.Inst) {
	g.prepareForClobber(REG_X0)
	g.EmitAdrpAdd(REG_X0, "$data_addr$", uint64(inst.Arg*8))
	g.opPush(REG_X0)
}

// === Binary operations ===

func (g *CodeGen) compileBinOpArm64(op ir.Opcode) {
	g.opPop(REG_X0) // second (top)
	g.opPop(REG_X1) // first (below)

	switch op {
	case ir.OP_ADD:
		g.EmitAddRR(REG_X1, REG_X1, REG_X0)
	case ir.OP_SUB:
		g.emitSubRR(REG_X1, REG_X1, REG_X0)
	case ir.OP_MUL:
		g.emitMul(REG_X1, REG_X1, REG_X0)
	case ir.OP_DIV:
		g.emitSdiv(REG_X1, REG_X1, REG_X0)
	case ir.OP_MOD:
		// mod = a - (a/b)*b → SDIV + MSUB
		g.emitSdiv(REG_X2, REG_X1, REG_X0)         // X2 = X1 / X0
		g.emitMsub(REG_X1, REG_X2, REG_X0, REG_X1) // X1 = X1 - X2*X0
	case ir.OP_AND:
		g.emitAndRR(REG_X1, REG_X1, REG_X0)
	case ir.OP_OR:
		g.emitOrrRR(REG_X1, REG_X1, REG_X0)
	case ir.OP_XOR:
		g.emitEorRR(REG_X1, REG_X1, REG_X0)
	case ir.OP_SHL:
		g.emitLslRR(REG_X1, REG_X1, REG_X0)
	case ir.OP_SHR:
		g.emitAsrRR(REG_X1, REG_X1, REG_X0)
	}

	g.opPush(REG_X1)
}

func isFloatTypeNameArm64(name string) bool {
	return name == "float32" || name == "float64"
}

func floatCompareCondArm64(op ir.Opcode) int {
	switch op {
	case ir.OP_EQ, ir.OP_JMP_EQ:
		return COND_EQ
	case ir.OP_NEQ, ir.OP_JMP_NEQ:
		return COND_NE
	case ir.OP_LT, ir.OP_JMP_LT:
		// Ordered LT after FCMP must stay false for NaN; COND_LT (N!=V) would be true for NaN.
		return COND_MI
	case ir.OP_GT, ir.OP_JMP_GT:
		return COND_GT
	case ir.OP_LEQ, ir.OP_JMP_LEQ:
		// Ordered LE after FCMP must stay false for NaN; COND_LE (Z==1 || N!=V) would be true for NaN.
		return COND_LS
	case ir.OP_GEQ, ir.OP_JMP_GEQ:
		return COND_GE
	default:
		panic("ICE: unsupported float compare opcode")
	}
}

func (g *CodeGen) compileFloatBinOpArm64(op ir.Opcode, floatKind string) {
	g.opPop(REG_X0)
	g.opPop(REG_X1)

	if floatKind == "float32" {
		g.emitFmovSFromW(REG_S0, REG_X1)
		g.emitFmovSFromW(REG_S1, REG_X0)
		switch op {
		case ir.OP_ADD:
			g.emitFaddS(REG_S0, REG_S0, REG_S1)
		case ir.OP_SUB:
			g.emitFsubS(REG_S0, REG_S0, REG_S1)
		case ir.OP_MUL:
			g.emitFmulS(REG_S0, REG_S0, REG_S1)
		case ir.OP_DIV:
			g.emitFdivS(REG_S0, REG_S0, REG_S1)
		default:
			panic("ICE: unsupported float32 binop")
		}
		g.emitFmovWFromS(REG_X1, REG_S0)
		g.opPush(REG_X1)
		return
	}

	g.emitFmovDFromX(REG_D0, REG_X1)
	g.emitFmovDFromX(REG_D1, REG_X0)
	switch op {
	case ir.OP_ADD:
		g.emitFaddD(REG_D0, REG_D0, REG_D1)
	case ir.OP_SUB:
		g.emitFsubD(REG_D0, REG_D0, REG_D1)
	case ir.OP_MUL:
		g.emitFmulD(REG_D0, REG_D0, REG_D1)
	case ir.OP_DIV:
		g.emitFdivD(REG_D0, REG_D0, REG_D1)
	default:
		panic("ICE: unsupported float64 binop")
	}
	g.emitFmovXFromD(REG_X1, REG_D0)
	g.opPush(REG_X1)
}

func (g *CodeGen) compileFloatNegArm64(floatKind string) {
	g.opPop(REG_X0)
	if floatKind == "float32" {
		g.emitFmovSFromW(REG_S0, REG_X0)
		g.emitFnegS(REG_S0, REG_S0)
		g.emitFmovWFromS(REG_X0, REG_S0)
		g.opPush(REG_X0)
		return
	}
	g.emitFmovDFromX(REG_D0, REG_X0)
	g.emitFnegD(REG_D0, REG_D0)
	g.emitFmovXFromD(REG_X0, REG_D0)
	g.opPush(REG_X0)
}

// === Comparison operations ===

func (g *CodeGen) compileCompareArm64(cond int) {
	g.opPop(REG_X0) // second
	g.opPop(REG_X1) // first
	g.emitCmpRR(REG_X1, REG_X0)
	g.emitCset(REG_X1, cond)
	g.opPush(REG_X1)
}

func (g *CodeGen) compileCompareJumpArm64(cond int, label int) {
	g.opPop(REG_X0)
	g.opPop(REG_X1)
	g.emitCmpRR(REG_X1, REG_X0)
	g.Flush()
	fixup := g.emitBCond(cond)
	g.jumpFixups = append(g.jumpFixups, JumpFixup{fixup, label, 0, 0})
}

func (g *CodeGen) compileFloatCompareArm64(op ir.Opcode, floatKind string) {
	g.opPop(REG_X0)
	g.opPop(REG_X1)
	if floatKind == "float32" {
		g.emitFmovSFromW(REG_S0, REG_X1)
		g.emitFmovSFromW(REG_S1, REG_X0)
		g.emitFcmpS(REG_S0, REG_S1)
	} else {
		g.emitFmovDFromX(REG_D0, REG_X1)
		g.emitFmovDFromX(REG_D1, REG_X0)
		g.emitFcmpD(REG_D0, REG_D1)
	}
	g.emitCset(REG_X1, floatCompareCondArm64(op))
	g.opPush(REG_X1)
}

func (g *CodeGen) compileFloatCompareJumpArm64(op ir.Opcode, label int, floatKind string) {
	g.opPop(REG_X0)
	g.opPop(REG_X1)
	if floatKind == "float32" {
		g.emitFmovSFromW(REG_S0, REG_X1)
		g.emitFmovSFromW(REG_S1, REG_X0)
		g.emitFcmpS(REG_S0, REG_S1)
	} else {
		g.emitFmovDFromX(REG_D0, REG_X1)
		g.emitFmovDFromX(REG_D1, REG_X0)
		g.emitFcmpD(REG_D0, REG_D1)
	}
	g.Flush()
	fixup := g.emitBCond(floatCompareCondArm64(op))
	g.jumpFixups = append(g.jumpFixups, JumpFixup{fixup, label, 0, 0})
}

// === Function calls ===

func (g *CodeGen) compileCallArm64(inst ir.Inst) {
	if len(inst.Name) > 18 && inst.Name[0:18] == "builtin.composite." {
		g.compileCompositeLitCallArm64(inst)
		return
	}
	if g.isNativeCABIArm64(inst.Name) {
		g.compileNativeCallArm64(inst)
		return
	}
	g.EmitCallPlaceholderArm64(inst.Name)
}

func (g *CodeGen) compileNativeCallArm64(inst ir.Inst) {
	g.Flush()
	stackArgs := 0
	if inst.Arg > 8 {
		stackArgs = inst.Arg - 8
	}
	frame := stackArgs * 8
	if frame%16 != 0 {
		frame += 8
	}
	if frame > 0 {
		g.emitSubImm(REG_SP, REG_SP, uint32(frame))
	}
	i := inst.Arg - 1
	for i >= 8 {
		g.opPop(REG_X16)
		g.EmitStr(REG_X16, REG_SP, (i-8)*8)
		i--
	}
	for i >= 0 {
		g.opPop(REG_X0 + i)
		i--
	}
	g.EmitCallPlaceholderArm64(inst.Name)
	if frame > 0 {
		g.emitAddImm(REG_SP, REG_SP, uint32(frame))
	}
	if g.funcRetCountArm64(inst.Name) > 0 {
		g.opPush(REG_X0)
	}
}

func (g *CodeGen) compileCompositeLitCallArm64(inst ir.Inst) {
	fieldCount := inst.Arg
	structSize := fieldCount * 8

	if structSize == 0 {
		g.compileConstI64Arm64(0)
		return
	}

	saveBytes := structSize
	if saveBytes%16 != 0 {
		saveBytes += 16 - (saveBytes % 16)
	}
	if saveBytes < 4096 {
		g.emitSubImm(REG_SP, REG_SP, uint32(saveBytes))
	} else {
		g.EmitLoadImm64Compact(REG_X16, uint64(saveBytes))
		g.emitSubRR(REG_SP, REG_SP, REG_X16)
	}

	// Save field values from operand stack to hardware stack.
	// Fields are popped in reverse order, so store reversed to preserve field0..N layout.
	i := 0
	for i < fieldCount {
		g.opPop(REG_X0)
		saveOff := (fieldCount - 1 - i) * 8
		g.EmitStr(REG_X0, REG_SP, saveOff)
		i++
	}

	// Allocate struct: push size, call Alloc
	g.compileConstI64Arm64(int64(structSize))
	g.EmitCallPlaceholderArm64("runtime.Alloc")
	g.opPop(REG_X1) // struct ptr

	// Pop fields from hardware stack and store into struct
	i = 0
	for i < fieldCount {
		g.emitLdr(REG_X0, REG_SP, i*8)
		offset := i * 8
		g.EmitStr(REG_X0, REG_X1, offset)
		i++
	}
	if saveBytes < 4096 {
		g.emitAddImm(REG_SP, REG_SP, uint32(saveBytes))
	} else {
		g.EmitLoadImm64Compact(REG_X16, uint64(saveBytes))
		g.EmitAddRR(REG_SP, REG_SP, REG_X16)
	}

	g.opPush(REG_X1)
}

func (g *CodeGen) compileReturnArm64(inst ir.Inst) {
	if g.curFunc != nil && g.isNativeCABIArm64(g.curFunc.Name) {
		if g.curFunc.RetCount > 0 {
			g.opPop(REG_X0)
		}
		if g.usesFrameOperandStackArm64(g.curFunc.Name) && g.curNativeSavedOpStackOffset > 0 {
			g.emitLdr(REG_X28, REG_FP, -g.curNativeSavedOpStackOffset)
		}
		g.ClearOperandCache()
	} else {
		g.Flush()
	}
	// Epilogue: MOV SP, FP; LDP FP, LR, [SP], #16; RET
	g.EmitMovRRArm64(REG_SP, REG_FP)
	g.EmitLdp(REG_FP, REG_LR, REG_SP, 16)
	g.EmitRet()
}

// === Intrinsics ===

func (g *CodeGen) compileCallIntrinsicArm64(inst ir.Inst) {
	g.Flush()
	if g.target.GOOS == "windows" {
		g.compileCallIntrinsicArm64Windows(inst)
		return
	}
	if g.compileLinkStaticIntrinsicArm64(inst) {
		return
	}
	switch inst.Name {
	case "SysGetargc":
		argcOff := len(g.irmod.Globals) * 8
		g.emitAdrpLdr(REG_X0, "$data_addr$", uint64(argcOff))
		g.rawPush(REG_X0) // r1=argc
		g.EmitMovZ(REG_X0, 0, 0)
		g.rawPush(REG_X0) // r2=0
		g.rawPush(REG_X0) // err=0
		g.ClearOperandCache()
	case "SysArgcValue":
		argcOff := len(g.irmod.Globals) * 8
		g.emitAdrpLdr(REG_X0, "$data_addr$", uint64(argcOff))
		g.rawPush(REG_X0)
		g.ClearOperandCache()
	case "SysGetargv":
		argvOff := (len(g.irmod.Globals) + 1) * 8
		g.emitAdrpLdr(REG_X0, "$data_addr$", uint64(argvOff))
		g.rawPush(REG_X0) // r1=argv
		g.EmitMovZ(REG_X0, 0, 0)
		g.rawPush(REG_X0) // r2=0
		g.rawPush(REG_X0) // err=0
		g.ClearOperandCache()
	case "SysArgvBaseValue":
		argvOff := (len(g.irmod.Globals) + 1) * 8
		g.emitAdrpLdr(REG_X0, "$data_addr$", uint64(argvOff))
		g.rawPush(REG_X0)
		g.ClearOperandCache()
	case "SysGetenvp":
		envpOff := (len(g.irmod.Globals) + 2) * 8
		g.emitAdrpLdr(REG_X0, "$data_addr$", uint64(envpOff))
		g.rawPush(REG_X0) // r1=envp
		g.EmitMovZ(REG_X0, 0, 0)
		g.rawPush(REG_X0) // r2=0
		g.rawPush(REG_X0) // err=0
		g.ClearOperandCache()
	case "Syscall":
		g.compileSyscallIntrinsicArm64(inst.Arg)
	case "Alloc":
		g.compileAllocIntrinsicArm64()
	case "Sliceptr":
		g.compileSliceptrIntrinsicArm64()
	case "Makeslice":
		g.compileMakesliceIntrinsicArm64()
	case "Stringptr":
		g.compileStringptrIntrinsicArm64()
	case "Makestring":
		g.compileMakestringIntrinsicArm64()
	case "Tostring":
		g.compileTostringIntrinsicArm64()
	case "ReadPtr":
		g.compileReadPtrIntrinsicArm64()
	case "WritePtr":
		g.compileWritePtrIntrinsicArm64()
	case "WriteByte":
		g.compileWriteByteIntrinsicArm64()
	default:
		panic("ICE: unknown intrinsic '" + inst.Name + "' in compileCallIntrinsicArm64")
	}
}

func (g *CodeGen) compileAllocIntrinsicArm64() {
	g.emitLoadLocalArm64(1*8, REG_X0)
	g.opPush(REG_X0)
	g.EmitCallPlaceholderArm64("runtime.Alloc")
}

func (g *CodeGen) compileSliceptrIntrinsicArm64() {
	g.emitLoadLocalArm64(1*8, REG_X0) // slice header ptr
	g.emitLdr(REG_X0, REG_X0, 0)      // [header+0] = data ptr
	g.opPush(REG_X0)
}

func (g *CodeGen) compileMakesliceIntrinsicArm64() {
	// Params: ptr (local 0), len (local 1), cap (local 2)
	g.compileConstI64Arm64(32)
	g.EmitCallPlaceholderArm64("runtime.Alloc")
	g.opPop(REG_X1) // header ptr

	g.emitLoadLocalArm64(1*8, REG_X0) // ptr
	g.EmitStr(REG_X0, REG_X1, 0)
	g.emitLoadLocalArm64(2*8, REG_X0) // len
	g.EmitStr(REG_X0, REG_X1, 8)
	g.emitLoadLocalArm64(3*8, REG_X0) // cap
	g.EmitStr(REG_X0, REG_X1, 16)
	g.EmitLoadImm64Compact(REG_X0, 1) // elem_size = 1
	g.EmitStr(REG_X0, REG_X1, 24)

	g.opPush(REG_X1)
}

func (g *CodeGen) compileStringptrIntrinsicArm64() {
	g.emitLoadLocalArm64(1*8, REG_X0)
	g.emitLdr(REG_X0, REG_X0, 0)
	g.opPush(REG_X0)
}

func (g *CodeGen) compileMakestringIntrinsicArm64() {
	// Params: ptr (local 0), len (local 1)
	g.compileConstI64Arm64(16)
	g.EmitCallPlaceholderArm64("runtime.Alloc")
	g.opPop(REG_X1) // header ptr

	g.emitLoadLocalArm64(1*8, REG_X0) // ptr
	g.EmitStr(REG_X0, REG_X1, 0)
	g.emitLoadLocalArm64(2*8, REG_X0) // len
	g.EmitStr(REG_X0, REG_X1, 8)

	g.opPush(REG_X1)
}

func (g *CodeGen) compileTostringIntrinsicArm64() {
	// ir.OP_CALL_INTRINSIC executes inside intrinsic wrapper functions where
	// params are read from frame locals. Inline directly to avoid helper
	// call/prologue interactions in native arm64 codegen.
	g.compileTostringIntrinsicBodyArm64()
}

func (g *CodeGen) EmitTostringHelperArm64() {
	if g.hasTostringHelper {
		return
	}
	g.hasTostringHelper = true
	g.funcOffsets[outlinedTostringHelper] = len(g.code)
	g.ClearOperandCache()

	g.EmitStp(REG_FP, REG_LR, REG_SP, -16)
	g.EmitMovRRArm64(REG_FP, REG_SP)

	frameBytes := 16
	g.emitSubImm(REG_SP, REG_SP, uint32(frameBytes))

	g.opPop(REG_X0)
	g.emitStoreLocalArm64(1*8, REG_X0)

	g.compileTostringIntrinsicBodyArm64()
	g.compileReturnArm64(ir.Inst{})
}

func (g *CodeGen) compileTostringIntrinsicBodyArm64() {
	g.emitLoadLocalArm64(1*8, REG_X0) // load value

	// Test: is [rax] < 256 → interface box
	g.emitLdr(REG_X1, REG_X0, 0)
	g.emitCmpImm(REG_X1, 256)
	stringCaseFixup := g.emitBCond(COND_CS) // branch if unsigned >= 256

	// Interface case: X1 = type_id, [X0+8] = concrete value
	g.emitLdr(REG_X2, REG_X0, 8) // concrete value
	g.opPush(REG_X2)
	g.Flush() // Must Flush before dispatch chain — otherwise the pending push
	// only materializes inside the type_id=1 branch (via emitCallPlaceholder's Flush)
	// and other branches (type_id=2 string passthrough) never see it.

	// Save type_id on hardware stack
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.EmitStr(REG_X1, REG_SP, 0)

	// Dispatch chain for Error/String
	var entries []becommon.DispatchEntry
	if g.irmod != nil && g.irmod.TypeIDs != nil {
		for typeName, tid := range g.irmod.TypeIDs {
			candidate := typeName + ".Error"
			if fnName, ok := becommon.LookupStringMapLinear(g.irmod.MethodTable, candidate); ok {
				entries = append(entries, becommon.DispatchEntry{tid, fnName})
				continue
			}
			candidate = typeName + ".String"
			if fnName, ok := becommon.LookupStringMapLinear(g.irmod.MethodTable, candidate); ok {
				entries = append(entries, becommon.DispatchEntry{tid, fnName})
			}
		}
	}
	// Keep dispatch deterministic for self-hosting stability.
	for i := 1; i < len(entries); i++ {
		j := i
		for j > 0 && (entries[j].TypeID < entries[j-1].TypeID ||
			(entries[j].TypeID == entries[j-1].TypeID && entries[j].FuncName < entries[j-1].FuncName)) {
			entries[j], entries[j-1] = entries[j-1], entries[j]
			j--
		}
	}

	// Restore type_id
	g.emitLdr(REG_X1, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)

	endFixups := make([]int, 0)

	// type_id 1 = int
	g.emitCmpImm(REG_X1, 1)
	nextFixup := g.emitBCond(COND_NE)
	g.EmitCallPlaceholderArm64("runtime.IntToString")
	endFixups = append(endFixups, g.emitB())
	g.patchArm64BCondAt(nextFixup, len(g.code))

	// type_id 2 = string
	g.emitCmpImm(REG_X1, 2)
	nextFixup = g.emitBCond(COND_NE)
	endFixups = append(endFixups, g.emitB())
	g.patchArm64BCondAt(nextFixup, len(g.code))

	// User types
	for _, entry := range entries {
		g.emitCmpImm(REG_X1, uint32(entry.TypeID))
		nextFixup = g.emitBCond(COND_NE)
		g.EmitCallPlaceholderArm64(entry.FuncName)
		endFixups = append(endFixups, g.emitB())
		g.patchArm64BCondAt(nextFixup, len(g.code))
	}

	// Default: drop receiver, push 0
	g.opDrop()
	g.compileConstI64Arm64(0)
	g.Flush()

	endAddr := len(g.code)
	for _, fixup := range endFixups {
		g.PatchArm64BAt(fixup, endAddr)
	}

	finalEndFixup := g.emitB()

	// string_case: pass through
	g.patchArm64BCondAt(stringCaseFixup, len(g.code))
	g.emitLoadLocalArm64(1*8, REG_X0)
	g.opPush(REG_X0)
	g.Flush()

	g.PatchArm64BAt(finalEndFixup, len(g.code))
}

func (g *CodeGen) compileReadPtrIntrinsicArm64() {
	g.emitLoadLocalArm64(1*8, REG_X0) // addr
	g.emitLdr(REG_X0, REG_X0, 0)      // read 8 bytes
	g.opPush(REG_X0)
}

func (g *CodeGen) compileWritePtrIntrinsicArm64() {
	g.emitLoadLocalArm64(1*8, REG_X0) // addr
	g.emitLoadLocalArm64(2*8, REG_X1) // val
	g.EmitStr(REG_X1, REG_X0, 0)
}

func (g *CodeGen) compileWriteByteIntrinsicArm64() {
	g.emitLoadLocalArm64(1*8, REG_X0) // addr
	g.emitLoadLocalArm64(2*8, REG_X1) // val
	g.emitStrb(REG_X1, REG_X0, 0)
}

// === Interface dispatch ===

func (g *CodeGen) compileIfaceBoxArm64(inst ir.Inst) {
	typeID := inst.Arg

	g.opPop(REG_X0) // concrete value
	// Save on hardware stack
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.EmitStr(REG_X0, REG_SP, 0)

	// Allocate 16 bytes
	g.compileConstI64Arm64(16)
	g.EmitCallPlaceholderArm64("runtime.Alloc")
	g.opPop(REG_X1) // box ptr

	// Store type_id
	g.EmitLoadImm64Compact(REG_X0, uint64(typeID))
	g.EmitStr(REG_X0, REG_X1, 0)

	// Restore concrete value and store
	g.emitLdr(REG_X0, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)
	g.EmitStr(REG_X0, REG_X1, 8)

	g.opPush(REG_X1)
}

func (g *CodeGen) compileIfaceCallArm64(inst ir.Inst) {
	argCount := inst.Arg
	methodName := inst.Name

	// Materialize pending operand-stack state before reshuffling call arguments.
	g.Flush()

	// Save regular args to hardware stack
	i := 0
	for i < argCount {
		g.rawPop(REG_X0)
		g.emitSubImm(REG_SP, REG_SP, 16)
		g.EmitStr(REG_X0, REG_SP, 0)
		i++
	}

	// Pop interface pointer
	g.rawPop(REG_X0)

	// Load type_id and concrete value
	g.emitLdr(REG_X1, REG_X0, 0) // type_id
	g.emitLdr(REG_X2, REG_X0, 8) // concrete value

	// Push receiver once and materialize it before branch dispatch.
	g.rawPush(REG_X2)

	// Restore regular args
	i = argCount - 1
	for i >= 0 {
		g.emitLdr(REG_X0, REG_SP, 0)
		g.emitAddImm(REG_SP, REG_SP, 16)
		g.rawPush(REG_X0)
		i = i - 1
	}
	// Ensure restored args are materialized for all dispatch branches.
	g.Flush()

	// Save type_id on hardware stack
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.EmitStr(REG_X1, REG_SP, 0)

	// Extract method name from the last '.' so fully-qualified interface names
	// like "main.Stringer.String" resolve to "String".
	dotIdx := len(methodName) - 1
	for dotIdx >= 0 {
		if methodName[dotIdx] == '.' {
			break
		}
		dotIdx--
	}
	bareMethod := methodName
	if dotIdx >= 0 && dotIdx+1 < len(methodName) {
		bareMethod = methodName[dotIdx+1:]
	}

	// Collect dispatch entries
	var entries []becommon.DispatchEntry
	if g.irmod != nil && g.irmod.TypeIDs != nil {
		for typeName, tid := range g.irmod.TypeIDs {
			candidate := typeName + "." + bareMethod
			if fnName, ok := becommon.LookupStringMapLinear(g.irmod.MethodTable, candidate); ok {
				entries = append(entries, becommon.DispatchEntry{tid, fnName})
			}
		}
	}
	// Keep dispatch deterministic for self-hosting stability.
	for i := 1; i < len(entries); i++ {
		j := i
		for j > 0 && (entries[j].TypeID < entries[j-1].TypeID ||
			(entries[j].TypeID == entries[j-1].TypeID && entries[j].FuncName < entries[j-1].FuncName)) {
			entries[j], entries[j-1] = entries[j-1], entries[j]
			j--
		}
	}

	// Restore type_id
	g.emitLdr(REG_X1, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)

	if len(entries) == 0 {
		g.emitBrk()
	} else {
		endFixups := make([]int, 0)
		for _, entry := range entries {
			g.emitCmpImm(REG_X1, uint32(entry.TypeID))
			nextFixup := g.emitBCond(COND_NE)
			g.EmitCallPlaceholderArm64(entry.FuncName)
			endFixups = append(endFixups, g.emitB())
			g.patchArm64BCondAt(nextFixup, len(g.code))
		}
		g.emitBrk() // default: trap

		endAddr := len(g.code)
		for _, fixup := range endFixups {
			g.PatchArm64BAt(fixup, endAddr)
		}
	}
}

// === Memory operations ===

func (g *CodeGen) compileLoadArm64(inst ir.Inst) {
	size := inst.Arg
	offset := int(inst.Val)
	g.opPop(REG_X1) // addr
	if offset != 0 && !ir.IsNonNilMemoryBase(inst.Name) {
		// Preserve IR semantics: nil-guarded LOAD checks the effective address
		// after OP_OFFSET, not the original base pointer.
		if offset > 0 && offset < 4096 {
			g.emitAddImm(REG_X1, REG_X1, uint32(offset))
		} else if offset < 0 && -offset < 4096 {
			g.emitSubImm(REG_X1, REG_X1, uint32(-offset))
		} else {
			g.EmitLoadImm64Compact(REG_X0, uint64(int64(offset)))
			g.EmitAddRR(REG_X1, REG_X1, REG_X0)
		}
		offset = 0
	}
	if ir.IsNonNilMemoryBase(inst.Name) {
		if size == 1 {
			g.emitLdrb(REG_X0, REG_X1, offset)
		} else if size == 4 {
			g.emitLdrw(REG_X0, REG_X1, offset)
		} else {
			g.emitLdr(REG_X0, REG_X1, offset)
		}
		g.opPush(REG_X0)
		return
	}
	g.emitCmpImm(REG_X1, 0)
	loadFixup := g.emitBCond(COND_NE) // branch to load if non-nil
	// nil case: X0 = 0
	g.EmitMovZ(REG_X0, 0, 0)
	doneFixup := g.emitB()
	// load case:
	g.patchArm64BCondAt(loadFixup, len(g.code))
	if size == 0 {
		size = 8
	}
	if size == 1 {
		g.emitLdrb(REG_X0, REG_X1, offset)
	} else if size == 2 {
		g.emitLdrh(REG_X0, REG_X1, offset)
	} else if size == 4 {
		g.emitLdrw(REG_X0, REG_X1, offset)
	} else {
		g.emitLdr(REG_X0, REG_X1, offset)
	}
	g.PatchArm64BAt(doneFixup, len(g.code))
	g.opPush(REG_X0)
}

func (g *CodeGen) compileStoreArm64(inst ir.Inst) {
	size := inst.Arg
	offset := int(inst.Val)
	g.opPop(REG_X1) // addr
	g.opPop(REG_X0) // value
	if size == 0 {
		size = 8
	}
	if size == 1 {
		g.emitStrb(REG_X0, REG_X1, offset)
	} else if size == 2 {
		g.emitStrh(REG_X0, REG_X1, offset)
	} else if size == 4 {
		g.emitStrw(REG_X0, REG_X1, offset)
	} else {
		g.EmitStr(REG_X0, REG_X1, offset)
	}
}

func (g *CodeGen) compileOffsetArm64(inst ir.Inst) {
	g.opPop(REG_X0)
	if inst.Arg != 0 {
		if inst.Arg > 0 && inst.Arg < 4096 {
			g.emitAddImm(REG_X0, REG_X0, uint32(inst.Arg))
		} else {
			g.EmitLoadImm64Compact(REG_X1, uint64(int64(inst.Arg)))
			g.EmitAddRR(REG_X0, REG_X0, REG_X1)
		}
	}
	g.opPush(REG_X0)
}

func (g *CodeGen) compileIndexAddrArm64(elemSize int) {
	g.opPop(REG_X0) // index
	g.opPop(REG_X1) // slice header ptr

	// Load data_ptr from header
	g.emitLdr(REG_X1, REG_X1, 0)

	// Compute address: data_ptr + index * elemSize
	if elemSize == 1 {
		g.EmitAddRR(REG_X1, REG_X1, REG_X0)
	} else if elemSize == 8 {
		g.emitLslImm(REG_X0, REG_X0, 3)
		g.EmitAddRR(REG_X1, REG_X1, REG_X0)
	} else {
		g.EmitLoadImm64Compact(REG_X2, uint64(elemSize))
		g.emitMul(REG_X0, REG_X0, REG_X2)
		g.EmitAddRR(REG_X1, REG_X1, REG_X0)
	}

	g.opPush(REG_X1)
}

func (g *CodeGen) compileLenArm64(inst ir.Inst) {
	g.opPop(REG_X0)
	if ir.IsNonNilMemoryBase(inst.Name) {
		g.emitLdr(REG_X0, REG_X0, 8) // [header+8] = len
		g.opPush(REG_X0)
		return
	}
	g.emitCmpImm(REG_X0, 0)
	nonNilFixup := g.emitBCond(COND_NE)
	// nil: len = 0
	g.EmitMovZ(REG_X0, 0, 0)
	doneFixup := g.emitB()
	g.patchArm64BCondAt(nonNilFixup, len(g.code))
	g.emitLdr(REG_X0, REG_X0, 8) // [header+8] = len
	g.PatchArm64BAt(doneFixup, len(g.code))
	g.opPush(REG_X0)
}

func (g *CodeGen) compileCapArm64(inst ir.Inst) {
	g.opPop(REG_X0)
	if ir.IsNonNilMemoryBase(inst.Name) {
		g.emitLdr(REG_X0, REG_X0, 16) // [header+16] = cap
		g.opPush(REG_X0)
		return
	}
	g.emitCmpImm(REG_X0, 0)
	nonNilFixup := g.emitBCond(COND_NE)
	// nil: cap = 0
	g.EmitMovZ(REG_X0, 0, 0)
	doneFixup := g.emitB()
	g.patchArm64BCondAt(nonNilFixup, len(g.code))
	g.emitLdr(REG_X0, REG_X0, 16) // [header+16] = cap
	g.PatchArm64BAt(doneFixup, len(g.code))
	g.opPush(REG_X0)
}

// === Type conversions ===

func normalizeIntWidthArm64(g *CodeGen, reg int, width int, signed bool) {
	switch width {
	case 1:
		if signed {
			g.emitSxtb(reg, reg)
		} else {
			g.emitUxtb(reg, reg)
		}
	case 2:
		if signed {
			g.emitSxth(reg, reg)
		} else {
			g.emitUxth(reg, reg)
		}
	case 4:
		if signed {
			g.emitSxtw(reg, reg)
		} else {
			g.emitUxtw(reg, reg)
		}
	}
}

func lowerFloatSrcToDArm64(g *CodeGen, srcKind int64) {
	switch srcKind {
	case ir.CONVERT_SRC_FLOAT32:
		g.emitFmovSFromW(REG_S0, REG_X0)
		g.emitFcvtSToD(REG_D0, REG_S0)
	case ir.CONVERT_SRC_FLOAT64:
		g.emitFmovDFromX(REG_D0, REG_X0)
	default:
		panic("ICE: expected float conversion source")
	}
}

func (g *CodeGen) compileConvertArm64(inst ir.Inst) {
	typeName := inst.Name
	switch typeName {
	case "string":
		g.EmitCallPlaceholderArm64("runtime.BytesToString")
	case "[]byte":
		g.EmitCallPlaceholderArm64("runtime.StringToBytes")
	case "int", "uintptr", "uint", "int64", "uint64":
		if inst.Val == ir.CONVERT_SRC_FLOAT32 || inst.Val == ir.CONVERT_SRC_FLOAT64 {
			g.opPop(REG_X0)
			lowerFloatSrcToDArm64(g, inst.Val)
			if typeName == "uint" || typeName == "uintptr" || typeName == "uint64" {
				g.emitFcvtzuXFromD(REG_X0, REG_D0)
			} else {
				g.emitFcvtzsXFromD(REG_X0, REG_D0)
			}
			g.opPush(REG_X0)
		}
	case "byte":
		g.opPop(REG_X0)
		if inst.Val == ir.CONVERT_SRC_FLOAT32 || inst.Val == ir.CONVERT_SRC_FLOAT64 {
			lowerFloatSrcToDArm64(g, inst.Val)
			g.emitFcvtzuXFromD(REG_X0, REG_D0)
		}
		g.emitUxtb(REG_X0, REG_X0)
		g.opPush(REG_X0)
	case "uint8":
		g.opPop(REG_X0)
		if inst.Val == ir.CONVERT_SRC_FLOAT32 || inst.Val == ir.CONVERT_SRC_FLOAT64 {
			lowerFloatSrcToDArm64(g, inst.Val)
			g.emitFcvtzuXFromD(REG_X0, REG_D0)
		}
		g.emitUxtb(REG_X0, REG_X0)
		g.opPush(REG_X0)
	case "int8":
		g.opPop(REG_X0)
		if inst.Val == ir.CONVERT_SRC_FLOAT32 || inst.Val == ir.CONVERT_SRC_FLOAT64 {
			lowerFloatSrcToDArm64(g, inst.Val)
			g.emitFcvtzsXFromD(REG_X0, REG_D0)
		}
		g.emitSxtb(REG_X0, REG_X0)
		g.opPush(REG_X0)
	case "uint16":
		g.opPop(REG_X0)
		if inst.Val == ir.CONVERT_SRC_FLOAT32 || inst.Val == ir.CONVERT_SRC_FLOAT64 {
			lowerFloatSrcToDArm64(g, inst.Val)
			g.emitFcvtzuXFromD(REG_X0, REG_D0)
		}
		g.emitUxth(REG_X0, REG_X0)
		g.opPush(REG_X0)
	case "int16":
		g.opPop(REG_X0)
		if inst.Val == ir.CONVERT_SRC_FLOAT32 || inst.Val == ir.CONVERT_SRC_FLOAT64 {
			lowerFloatSrcToDArm64(g, inst.Val)
			g.emitFcvtzsXFromD(REG_X0, REG_D0)
		}
		g.emitSxth(REG_X0, REG_X0)
		g.opPush(REG_X0)
	case "int32":
		g.opPop(REG_X0)
		if inst.Val == ir.CONVERT_SRC_FLOAT32 || inst.Val == ir.CONVERT_SRC_FLOAT64 {
			lowerFloatSrcToDArm64(g, inst.Val)
			g.emitFcvtzsXFromD(REG_X0, REG_D0)
		}
		g.emitSxtw(REG_X0, REG_X0)
		g.opPush(REG_X0)
	case "uint32":
		g.opPop(REG_X0)
		if inst.Val == ir.CONVERT_SRC_FLOAT32 || inst.Val == ir.CONVERT_SRC_FLOAT64 {
			lowerFloatSrcToDArm64(g, inst.Val)
			g.emitFcvtzuXFromD(REG_X0, REG_D0)
		}
		g.emitUxtw(REG_X0, REG_X0)
		g.opPush(REG_X0)
	case "float32":
		g.opPop(REG_X0)
		switch inst.Val {
		case ir.CONVERT_SRC_FLOAT32:
			g.opPush(REG_X0)
		case ir.CONVERT_SRC_FLOAT64:
			g.emitFmovDFromX(REG_D0, REG_X0)
			g.emitFcvtDToS(REG_S0, REG_D0)
			g.emitFmovWFromS(REG_X0, REG_S0)
			g.opPush(REG_X0)
		default:
			signed := inst.Val != ir.CONVERT_SRC_UINT
			width := inst.Width
			if width <= 0 {
				width = 8
			}
			normalizeIntWidthArm64(g, REG_X0, width, signed)
			if signed {
				g.emitScvtfDFromX(REG_D0, REG_X0)
			} else {
				g.emitUcvtfDFromX(REG_D0, REG_X0)
			}
			g.emitFcvtDToS(REG_S0, REG_D0)
			g.emitFmovWFromS(REG_X0, REG_S0)
			g.opPush(REG_X0)
		}
	case "float64":
		g.opPop(REG_X0)
		switch inst.Val {
		case ir.CONVERT_SRC_FLOAT64:
			g.opPush(REG_X0)
		case ir.CONVERT_SRC_FLOAT32:
			g.emitFmovSFromW(REG_S0, REG_X0)
			g.emitFcvtSToD(REG_D0, REG_S0)
			g.emitFmovXFromD(REG_X0, REG_D0)
			g.opPush(REG_X0)
		default:
			signed := inst.Val != ir.CONVERT_SRC_UINT
			width := inst.Width
			if width <= 0 {
				width = 8
			}
			normalizeIntWidthArm64(g, REG_X0, width, signed)
			if signed {
				g.emitScvtfDFromX(REG_D0, REG_X0)
			} else {
				g.emitUcvtfDFromX(REG_D0, REG_X0)
			}
			g.emitFmovXFromD(REG_X0, REG_D0)
			g.opPush(REG_X0)
		}
	}
}

func roundShiftNearestEvenArm64(v uint64, shift uint) uint64 {
	if shift == 0 {
		return v
	}
	if shift >= 64 {
		return 0
	}
	half := uint64(1) << (shift - 1)
	mask := (uint64(1) << shift) - 1
	out := v >> shift
	rem := v & mask
	if rem > half || (rem == half && (out&1) != 0) {
		out++
	}
	return out
}

func float32BitsFromFloat64Bits(bits uint64) uint32 {
	sign := uint32(bits >> 63)
	exp := int((bits >> 52) & 0x7FF)
	frac := bits & ((uint64(1) << 52) - 1)
	if exp == 0x7FF {
		if frac == 0 {
			return (sign << 31) | (0xFF << 23)
		}
		nan := uint32(frac >> 29)
		if nan == 0 {
			nan = 1
		}
		return (sign << 31) | (0xFF << 23) | nan
	}
	if exp == 0 && frac == 0 {
		return sign << 31
	}

	mant := frac
	unbiased := exp - 1023
	if exp != 0 {
		mant = mant | (uint64(1) << 52)
	} else {
		unbiased = -1022
	}

	if unbiased > 127 {
		return (sign << 31) | (0xFF << 23)
	}
	if unbiased < -149 {
		return sign << 31
	}

	if unbiased >= -126 {
		mant24 := roundShiftNearestEvenArm64(mant, 29)
		if mant24 >= (uint64(1) << 24) {
			mant24 = mant24 >> 1
			unbiased++
		}
		if unbiased > 127 {
			return (sign << 31) | (0xFF << 23)
		}
		return (sign << 31) | (uint32(unbiased+127) << 23) | uint32(mant24&0x7FFFFF)
	}

	shift := uint(-unbiased - 97)
	mant23 := roundShiftNearestEvenArm64(mant, shift)
	if mant23 >= (uint64(1) << 23) {
		return (sign << 31) | (1 << 23)
	}
	return (sign << 31) | uint32(mant23)
}

func mul10CheckedArm64(v uint64) (uint64, bool) {
	if v > ^uint64(0)/10 {
		return 0, false
	}
	return v * 10, true
}

func hexDigitValueArm64(ch byte) (uint64, bool) {
	switch {
	case ch >= '0' && ch <= '9':
		return uint64(ch - '0'), true
	case ch >= 'a' && ch <= 'f':
		return uint64(ch-'a') + 10, true
	case ch >= 'A' && ch <= 'F':
		return uint64(ch-'A') + 10, true
	default:
		return 0, false
	}
}

func highestBitIndexArm64(v uint64) int {
	i := -1
	for v != 0 {
		v = v >> 1
		i++
	}
	return i
}

func parseHexFloatLiteralBitsArm64(sign uint64, s string, i int) (uint64, bool) {
	if i+2 > len(s) || s[i] != '0' || (s[i+1] != 'x' && s[i+1] != 'X') {
		return 0, false
	}
	i += 2
	mant := uint64(0)
	exp2 := 0
	sawDigit := false
	sawDot := false
	for i < len(s) {
		ch := s[i]
		if ch == '_' {
			i++
			continue
		}
		if ch == '.' {
			if sawDot {
				return 0, false
			}
			sawDot = true
			i++
			continue
		}
		if ch == 'p' || ch == 'P' {
			break
		}
		digit, ok := hexDigitValueArm64(ch)
		if !ok {
			return 0, false
		}
		if mant > (^uint64(0) >> 4) {
			return 0, false
		}
		mant = (mant << 4) | digit
		if sawDot {
			exp2 -= 4
		}
		sawDigit = true
		i++
	}
	if !sawDigit || i >= len(s) || (s[i] != 'p' && s[i] != 'P') {
		return 0, false
	}
	i++
	if i >= len(s) {
		return 0, false
	}
	expNeg := false
	if s[i] == '+' || s[i] == '-' {
		expNeg = s[i] == '-'
		i++
	}
	if i >= len(s) {
		return 0, false
	}
	exp := 0
	haveExpDigit := false
	for i < len(s) {
		ch := s[i]
		if ch == '_' {
			i++
			continue
		}
		if ch < '0' || ch > '9' {
			return 0, false
		}
		exp = exp*10 + int(ch-'0')
		haveExpDigit = true
		i++
	}
	if !haveExpDigit {
		return 0, false
	}
	if expNeg {
		exp2 -= exp
	} else {
		exp2 += exp
	}
	if mant == 0 {
		return sign, true
	}

	msb := highestBitIndexArm64(mant)
	unbiased := exp2 + msb
	if unbiased > 1023 {
		return sign | (uint64(0x7FF) << 52), true
	}
	if unbiased < -1075 {
		return sign, true
	}
	if unbiased >= -1022 {
		shift := msb - 52
		mant53 := mant
		if shift > 0 {
			mant53 = mant >> uint(shift)
			mask := (uint64(1) << uint(shift)) - 1
			rem := mant & mask
			half := uint64(1) << uint(shift-1)
			if rem > half || (rem == half && (mant53&1) != 0) {
				mant53++
				if mant53 == (uint64(1) << 53) {
					mant53 = mant53 >> 1
					unbiased++
					if unbiased > 1023 {
						return sign | (uint64(0x7FF) << 52), true
					}
				}
			}
		} else if shift < 0 {
			mant53 = mant << uint(-shift)
		}
		return sign | (uint64(unbiased+1023) << 52) | (mant53 & ((uint64(1) << 52) - 1)), true
	}

	subShift := -1074 - exp2
	if subShift < 0 {
		return sign, true
	}
	if subShift >= 64 {
		return sign, true
	}
	mantBits := mant >> uint(subShift)
	if subShift > 0 {
		mask := (uint64(1) << uint(subShift)) - 1
		rem := mant & mask
		half := uint64(1) << uint(subShift-1)
		if rem > half || (rem == half && (mantBits&1) != 0) {
			mantBits++
		}
	}
	if mantBits >= (uint64(1) << 52) {
		return sign | (uint64(1) << 52), true
	}
	return sign | mantBits, true
}

func parseFloatLiteralBitsArm64(s string) (uint64, bool) {
	if len(s) == 0 {
		return 0, false
	}
	i := 0
	sign := uint64(0)
	if s[i] == '+' || s[i] == '-' {
		if s[i] == '-' {
			sign = uint64(1) << 63
		}
		i++
		if i >= len(s) {
			return 0, false
		}
	}
	if i+2 <= len(s) && s[i] == '0' && (s[i+1] == 'x' || s[i+1] == 'X') {
		return parseHexFloatLiteralBitsArm64(sign, s, i)
	}

	mant := uint64(0)
	exp10 := 0
	sawDigit := false
	sawDot := false
	for i < len(s) {
		ch := s[i]
		if ch == '_' {
			i++
			continue
		}
		if ch == '.' {
			if sawDot {
				return 0, false
			}
			sawDot = true
			i++
			continue
		}
		if ch == 'e' || ch == 'E' {
			break
		}
		if ch < '0' || ch > '9' {
			return 0, false
		}
		var ok bool
		mant, ok = mul10CheckedArm64(mant)
		if !ok {
			return 0, false
		}
		mant = mant + uint64(ch-'0')
		if sawDot {
			exp10--
		}
		sawDigit = true
		i++
	}
	if !sawDigit {
		return 0, false
	}

	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i >= len(s) {
			return 0, false
		}
		expNeg := false
		if s[i] == '+' || s[i] == '-' {
			expNeg = s[i] == '-'
			i++
		}
		if i >= len(s) {
			return 0, false
		}
		exp := 0
		haveExpDigit := false
		for i < len(s) {
			ch := s[i]
			if ch == '_' {
				i++
				continue
			}
			if ch < '0' || ch > '9' {
				return 0, false
			}
			exp = exp*10 + int(ch-'0')
			haveExpDigit = true
			i++
		}
		if !haveExpDigit {
			return 0, false
		}
		if expNeg {
			exp10 = exp10 - exp
		} else {
			exp10 = exp10 + exp
		}
	}
	if i != len(s) {
		return 0, false
	}
	if mant == 0 {
		return sign, true
	}

	num := mant
	den := uint64(1)
	for exp10 > 0 {
		var ok bool
		num, ok = mul10CheckedArm64(num)
		if !ok {
			return 0, false
		}
		exp10--
	}
	for exp10 < 0 {
		var ok bool
		den, ok = mul10CheckedArm64(den)
		if !ok {
			return 0, false
		}
		exp10++
	}

	exp2 := 0
	for num < den {
		if num > (^uint64(0) >> 1) {
			return 0, false
		}
		num = num << 1
		exp2--
	}
	for {
		if den <= (^uint64(0) >> 1) {
			if num >= (den << 1) {
				den = den << 1
				exp2++
				continue
			}
		} else if (num >> 1) >= den {
			num = (num + 1) >> 1
			exp2++
			continue
		}
		break
	}

	frac := num - den
	mantBits := uint64(0)
	bit := uint64(1) << 51
	for bit != 0 {
		if frac > (^uint64(0) >> 1) {
			den = (den + 1) >> 1
		} else {
			frac = frac << 1
		}
		if frac >= den {
			mantBits = mantBits | bit
			frac = frac - den
		}
		bit = bit >> 1
	}

	round := frac
	if round > (^uint64(0) >> 1) {
		den = (den + 1) >> 1
	} else {
		round = round << 1
	}
	if round > den || (round == den && (mantBits&1) != 0) {
		mantBits++
		if mantBits == (uint64(1) << 52) {
			mantBits = 0
			exp2++
		}
	}

	expBits := exp2 + 1023
	if expBits <= 0 {
		return sign, true
	}
	if expBits >= 0x7FF {
		return sign | (uint64(0x7FF) << 52), true
	}
	return sign | (uint64(expBits) << 52) | mantBits, true
}
