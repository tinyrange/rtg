//go:build !no_backend_linux_i386 || !no_backend_windows_i386

package i386

import (
	"j5.nz/rtg/std/compiler/backend/becommon"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

func (g *CodeGen) funcABI_i386(name string) string {
	if g.IRMod == nil || g.IRMod.FuncABIs == nil {
		return ""
	}
	return g.IRMod.FuncABIs[name]
}

func (g *CodeGen) funcRetCount_i386(name string) int {
	if g.IRMod == nil || g.IRMod.FuncRetCounts == nil {
		return 0
	}
	return g.IRMod.FuncRetCounts[name]
}

func (g *CodeGen) isNativeCABI_i386(name string) bool {
	return g.funcABI_i386(name) == "native-c-linux-386"
}

func (g *CodeGen) usesFrameOperandStack_i386(name string) bool {
	return g.Target != nil && g.Target.RelocatableObject && g.isNativeCABI_i386(name)
}

func intrinsicRetCount_i386(name string) int {
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

func (g *CodeGen) nativeEvalStackSlots_i386(f *ir.IRFunc) int {
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
		case ir.OP_DUP:
			depth++
		case ir.OP_ADD, ir.OP_SUB, ir.OP_MUL, ir.OP_DIV, ir.OP_MOD, ir.OP_AND, ir.OP_OR, ir.OP_XOR, ir.OP_SHL, ir.OP_SHR, ir.OP_EQ, ir.OP_NEQ, ir.OP_LT, ir.OP_GT, ir.OP_LEQ, ir.OP_GEQ, ir.OP_INDEX_ADDR:
			depth--
		case ir.OP_STORE:
			depth -= 2
		case ir.OP_JMP_EQ, ir.OP_JMP_NEQ, ir.OP_JMP_LT, ir.OP_JMP_GT, ir.OP_JMP_LEQ, ir.OP_JMP_GEQ:
			depth -= 2
		case ir.OP_CALL:
			retCount := g.funcRetCount_i386(inst.Name)
			if len(inst.Name) > 18 && inst.Name[0:18] == "builtin.composite." {
				retCount = 1
			}
			depth += retCount - inst.Arg
		case ir.OP_CALL_INTRINSIC:
			depth += intrinsicRetCount_i386(inst.Name)
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

func (g *CodeGen) slotBytes_i386() int {
	return 4
}

func (g *CodeGen) ptrBytes_i386() int {
	return g.slotBytes_i386()
}

func (g *CodeGen) initOperandCache_i386() {
	g.configureOperandCache(REG32_EBX, REG32_ESI)
}

// compileFunc_i386 generates i386 code for a single IR function.
func (g *CodeGen) compileFunc_i386(f *ir.IRFunc) {
	if f.Native != nil {
		if f.Native.Arch != "386" {
			panic("ICE: i386 backend received native function for arch " + f.Native.Arch)
		}
		funcStart := g.FuncOffsets[f.Name]
		g.Code = append(g.Code, f.Native.Code...)
		for _, fx := range f.Native.Fixups {
			if fx.Kind != ir.NativeFixupCallRel32 {
				continue
			}
			g.CallFixups = append(g.CallFixups, CallFixup{funcStart + fx.Off, fx.Target, 0})
		}
		return
	}
	g.CurFunc = f
	g.initOperandCache_i386()
	slot := g.slotBytes_i386()
	g.CurFrameSize = len(f.Locals)
	if f.Params > g.CurFrameSize {
		g.CurFrameSize = f.Params
	}
	g.CurNativeSavedOpStackOffset = 0
	g.CurNativeEvalSlots = 0
	if g.usesFrameOperandStack_i386(f.Name) {
		g.CurNativeEvalSlots = g.nativeEvalStackSlots_i386(f)
		g.CurNativeSavedOpStackOffset = (g.CurFrameSize + 1) * slot
		g.CurFrameSize = g.CurFrameSize + 1 + g.CurNativeEvalSlots
	}
	g.LabelOffsets = make(map[int]int)
	g.JumpFixups = nil

	// Prologue: push ebp; mov ebp, esp; sub esp, frameBytes
	g.pushR32(REG32_EBP)
	g.movRR32(REG32_EBP, REG32_ESP)

	frameBytes := g.CurFrameSize * slot
	if frameBytes > 0 {
		g.subRI32(REG32_ESP, int32(frameBytes))
	}

	if g.usesFrameOperandStack_i386(f.Name) {
		g.emitStoreLocal32(g.CurNativeSavedOpStackOffset, REG32_EDI)
		g.emitLeaLocal32(g.CurNativeSavedOpStackOffset, REG32_EDI)
	}

	if g.isNativeCABI_i386(f.Name) {
		i := 0
		for i < f.Params {
			g.loadMem32(REG32_EAX, REG32_EBP, 8+i*slot)
			offset := (i + 1) * slot
			g.emitStoreLocal32(offset, REG32_EAX)
			i++
		}
	} else if f.Params > 0 {
		// Pop params from operand stack (EDI) into local frame slots
		i := f.Params - 1
		for i >= 0 {
			g.opPop(REG32_EAX)
			offset := (i + 1) * slot
			g.emitStoreLocal32(offset, REG32_EAX)
			i = i - 1
		}
	}

	// Compile instructions
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
			g.opDropN(drops)
			i = j
			continue
		}
		g.compileInst_i386(inst)
		i++
	}

	// Resolve jump fixups within this function
	g.relaxCurrentFuncJumps()
	for _, fix := range g.JumpFixups {
		labelOff, ok := g.LabelOffsets[fix.LabelID]
		if !ok {
			continue
		}
		switch fix.Kind {
		case jumpFixupJmpRel8, jumpFixupJccRel8:
			rel := labelOff - (fix.CodeOffset + 1)
			g.Code[fix.CodeOffset] = byte(rel)
		default:
			g.patchRel32At(fix.CodeOffset, labelOff)
		}
	}

	g.CurFunc = nil
}

// compileInst_i386 generates code for a single IR instruction (i386).
func (g *CodeGen) compileInst_i386(inst ir.Inst) {
	switch inst.Op {
	case ir.OP_CONST_I64:
		if len(inst.Name) > 10 && inst.Name[0:10] == "$funcaddr$" {
			g.compileFuncAddr_i386(inst.Name)
		} else {
			g.compileConstI32(inst.Val)
		}
	case ir.OP_CONST_BOOL:
		if inst.Arg != 0 {
			g.compileConstI32(1)
		} else {
			g.compileConstI32(0)
		}
	case ir.OP_CONST_NIL:
		g.compileConstI32(0)
	case ir.OP_CONST_STR:
		g.compileConstStr_i386(inst.Name)

	case ir.OP_LOCAL_GET:
		g.compileLocalGet_i386(inst.Arg)
	case ir.OP_LOCAL_SET:
		g.compileLocalSet_i386(inst.Arg)
	case ir.OP_LOCAL_ADD_IMM:
		g.compileLocalAddImm_i386(inst.Arg, int32(inst.Val))
	case ir.OP_LOCAL_ADDR:
		g.compileLocalAddr_i386(inst.Arg)

	case ir.OP_GLOBAL_GET:
		g.compileGlobalGet_i386(inst)
	case ir.OP_GLOBAL_SET:
		g.compileGlobalSet_i386(inst)
	case ir.OP_GLOBAL_ADDR:
		g.compileGlobalAddr_i386(inst)

	case ir.OP_DROP:
		g.opDrop()
	case ir.OP_DUP:
		g.opLoad(REG32_EAX)
		g.opPush(REG32_EAX)

	case ir.OP_ADD:
		g.compileBinOP_i386(inst.Op)
	case ir.OP_SUB:
		g.compileBinOP_i386(inst.Op)
	case ir.OP_MUL:
		g.compileBinOP_i386(inst.Op)
	case ir.OP_DIV:
		g.compileBinOP_i386(inst.Op)
	case ir.OP_MOD:
		g.compileBinOP_i386(inst.Op)
	case ir.OP_NEG:
		g.opPop(REG32_EAX)
		g.negR32(REG32_EAX)
		g.opPush(REG32_EAX)

	case ir.OP_AND:
		g.compileBinOP_i386(inst.Op)
	case ir.OP_OR:
		g.compileBinOP_i386(inst.Op)
	case ir.OP_XOR:
		g.compileBinOP_i386(inst.Op)
	case ir.OP_SHL:
		g.compileBinOP_i386(inst.Op)
	case ir.OP_SHR:
		g.compileBinOP_i386(inst.Op)

	case ir.OP_EQ:
		g.compileCompare_i386(0x94) // sete
	case ir.OP_NEQ:
		g.compileCompare_i386(0x95) // setne
	case ir.OP_LT:
		g.compileCompare_i386(0x9c) // setl
	case ir.OP_GT:
		g.compileCompare_i386(0x9f) // setg
	case ir.OP_LEQ:
		g.compileCompare_i386(0x9e) // setle
	case ir.OP_GEQ:
		g.compileCompare_i386(0x9d) // setge

	case ir.OP_NOT:
		g.opPop(REG32_EAX)
		g.xorRI8_32(REG32_EAX, 0x01)
		g.opPush(REG32_EAX)

	case ir.OP_LABEL:
		g.flush()
		g.LabelOffsets[inst.Arg] = len(g.Code)
	case ir.OP_JMP:
		g.flush()
		fixup := g.jmpRel32()
		g.JumpFixups = append(g.JumpFixups, JumpFixup{fixup, inst.Arg, jumpFixupJmpRel32, 0})
	case ir.OP_JMP_IF:
		g.opPop(REG32_EAX)
		g.testRR32(REG32_EAX, REG32_EAX)
		fixup := g.jccRel32(CC32_NE)
		g.JumpFixups = append(g.JumpFixups, JumpFixup{fixup, inst.Arg, jumpFixupJccRel32, CC32_NE})
	case ir.OP_JMP_IF_NOT:
		g.opPop(REG32_EAX)
		g.testRR32(REG32_EAX, REG32_EAX)
		fixup := g.jccRel32(CC32_E)
		g.JumpFixups = append(g.JumpFixups, JumpFixup{fixup, inst.Arg, jumpFixupJccRel32, CC32_E})
	case ir.OP_JMP_EQ:
		g.compileCompareJump_i386(CC32_E, inst.Arg)
	case ir.OP_JMP_NEQ:
		g.compileCompareJump_i386(CC32_NE, inst.Arg)
	case ir.OP_JMP_LT:
		g.compileCompareJump_i386(CC32_L, inst.Arg)
	case ir.OP_JMP_GT:
		g.compileCompareJump_i386(CC32_G, inst.Arg)
	case ir.OP_JMP_LEQ:
		g.compileCompareJump_i386(CC32_LE, inst.Arg)
	case ir.OP_JMP_GEQ:
		g.compileCompareJump_i386(CC32_GE, inst.Arg)

	case ir.OP_CALL:
		g.compileCall_i386(inst)
	case ir.OP_CALL_INTRINSIC:
		g.compileCallIntrinsic_i386(inst)
	case ir.OP_RETURN:
		g.compileReturn_i386(inst)

	case ir.OP_LOAD:
		g.compileLoad_i386(inst)
	case ir.OP_STORE:
		g.compileStore_i386(inst)
	case ir.OP_OFFSET:
		g.compileOffset_i386(inst)
	case ir.OP_INDEX_ADDR:
		g.compileIndexAddr_i386(inst.Arg)
	case ir.OP_LEN:
		g.compileLen_i386(inst)
	case ir.OP_CAP:
		g.compileCap_i386(inst)

	case ir.OP_CONVERT:
		g.compileConvert_i386(inst.Name)

	case ir.OP_IFACE_BOX:
		g.compileIfaceBox_i386(inst)
	case ir.OP_IFACE_CALL:
		g.compileIfaceCall_i386(inst)
	case ir.OP_PANIC:
		if g.Target.GOOS == "windows" {
			g.compilePanic_win386()
		} else {
			g.compilePanic_linux386()
		}

	case ir.OP_SLICE_GET, ir.OP_SLICE_MAKE, ir.OP_STRING_GET, ir.OP_STRING_MAKE:
		// Handled by intrinsics or builtins

	default:
		panic("ICE: unhandled opcode in compileInst_i386")
	}
}

// === Constant loading (i386) ===

func (g *CodeGen) compileConstI32(val int64) {
	g.prepareForClobber(REG32_EAX)
	v32 := uint32(val)
	if v32 == 0 {
		g.xorRR32(REG32_EAX, REG32_EAX)
	} else {
		g.emitMovRegImm32(REG32_EAX, v32)
	}
	g.opPush(REG32_EAX)
}

// compileFuncAddr_i386 emits a mov eax, imm32 with a fixup to be resolved
// to the virtual address of the named function (or its callback thunk).
func (g *CodeGen) compileFuncAddr_i386(marker string) {
	g.prepareForClobber(REG32_EAX)
	funcName := marker[10:] // strip "$funcaddr$"
	// If it's a callback function, reference the thunk wrapper instead
	thunkName := funcName
	if g.IRMod != nil && g.IRMod.CallbackFuncs != nil && g.IRMod.CallbackFuncs[funcName] {
		thunkName = "$callback_thunk$" + funcName
	}
	g.emitMovRegImm32(REG32_EAX, 0) // placeholder imm32
	g.CallFixups = append(g.CallFixups, CallFixup{len(g.Code) - 4, "$funcaddr$" + thunkName, 0})
	g.opPush(REG32_EAX)
}

func (g *CodeGen) compileConstStr_i386(s string) {
	g.prepareForClobber(REG32_EAX)
	decoded := becommon.DecodeStringLiteral(s)

	headerOff, ok := g.StringMap[decoded]
	if !ok {
		// Store string bytes in rodata
		dataOff := len(g.Rodata)
		g.Rodata = append(g.Rodata, []byte(decoded)...)

		// Store header {data_ptr, len} in rodata.
		headerOff = len(g.Rodata)
		g.emitRodataU32(0)                    // data_ptr placeholder (4 bytes)
		g.emitRodataU32(uint32(len(decoded))) // len (4 bytes)

		g.StringMap[decoded] = headerOff
		// Store dataOff in the placeholder temporarily
		common.PutU32(g.Rodata[headerOff:headerOff+4], uint32(dataOff))
	}

	// Push header address onto operand stack: mov eax, imm32
	g.emitMovRegImm32(REG32_EAX, uint32(headerOff))
	g.CallFixups = append(g.CallFixups, CallFixup{len(g.Code) - 4, "$rodata_header$", 0})
	g.opPush(REG32_EAX)
}

// === Local variable access (i386) ===

func (g *CodeGen) compileLocalGet_i386(idx int) {
	g.prepareForClobber(REG32_EAX)
	offset := (idx + 1) * g.slotBytes_i386()
	g.emitLoadLocal32(offset, REG32_EAX)
	g.opPush(REG32_EAX)
}

func (g *CodeGen) compileLocalSet_i386(idx int) {
	g.opPop(REG32_EAX)
	offset := (idx + 1) * g.slotBytes_i386()
	g.emitStoreLocal32(offset, REG32_EAX)
}

func (g *CodeGen) compileLocalAddImm_i386(idx int, imm int32) {
	offset := (idx + 1) * g.slotBytes_i386()
	g.emitLoadLocal32(offset, REG32_EAX)
	g.addRI32(REG32_EAX, imm)
	g.emitStoreLocal32(offset, REG32_EAX)
}

func (g *CodeGen) compileLocalAddr_i386(idx int) {
	g.prepareForClobber(REG32_EAX)
	offset := (idx + 1) * g.slotBytes_i386()
	g.emitLeaLocal32(offset, REG32_EAX)
	g.opPush(REG32_EAX)
}

// === Global variable access (i386) ===

func (g *CodeGen) compileGlobalGet_i386(inst ir.Inst) {
	g.prepareForClobber(REG32_EAX, REG32_ECX)
	g.emitMovRegImm32(REG32_ECX, uint32(inst.Arg*g.slotBytes_i386()))
	g.CallFixups = append(g.CallFixups, CallFixup{len(g.Code) - 4, "$data_addr$", 0})
	g.loadMem32(REG32_EAX, REG32_ECX, 0)
	g.opPush(REG32_EAX)
}

func (g *CodeGen) compileGlobalSet_i386(inst ir.Inst) {
	g.opPop(REG32_EAX)
	g.emitMovRegImm32(REG32_ECX, uint32(inst.Arg*g.slotBytes_i386()))
	g.CallFixups = append(g.CallFixups, CallFixup{len(g.Code) - 4, "$data_addr$", 0})
	g.storeMem32(REG32_ECX, 0, REG32_EAX)
}

func (g *CodeGen) compileGlobalAddr_i386(inst ir.Inst) {
	g.prepareForClobber(REG32_EAX)
	g.emitMovRegImm32(REG32_EAX, uint32(inst.Arg*g.slotBytes_i386()))
	g.CallFixups = append(g.CallFixups, CallFixup{len(g.Code) - 4, "$data_addr$", 0})
	g.opPush(REG32_EAX)
}

// === Binary operations (i386) ===

func (g *CodeGen) compileBinOP_i386(op ir.Opcode) {
	g.opPop(REG32_EAX)
	g.opPop(REG32_ECX)

	switch op {
	case ir.OP_ADD:
		g.addRR32(REG32_ECX, REG32_EAX)
	case ir.OP_SUB:
		g.subRR32(REG32_ECX, REG32_EAX)
	case ir.OP_MUL:
		g.imulRR32(REG32_ECX, REG32_EAX)
	case ir.OP_DIV:
		g.movRR32(REG32_EDX, REG32_EAX)
		g.movRR32(REG32_EAX, REG32_ECX)
		g.movRR32(REG32_ECX, REG32_EDX)
		g.cdq32()
		g.idivR32(REG32_ECX)
		g.movRR32(REG32_ECX, REG32_EAX)
	case ir.OP_MOD:
		g.movRR32(REG32_EDX, REG32_EAX)
		g.movRR32(REG32_EAX, REG32_ECX)
		g.movRR32(REG32_ECX, REG32_EDX)
		g.cdq32()
		g.idivR32(REG32_ECX)
		g.movRR32(REG32_ECX, REG32_EDX)
	case ir.OP_AND:
		g.andRR32(REG32_ECX, REG32_EAX)
	case ir.OP_OR:
		g.orRR32(REG32_ECX, REG32_EAX)
	case ir.OP_XOR:
		g.xorRR32(REG32_ECX, REG32_EAX)
	case ir.OP_SHL:
		g.movRR32(REG32_EDX, REG32_ECX)
		g.movRR32(REG32_ECX, REG32_EAX)
		g.shlCl32(REG32_EDX)
		g.movRR32(REG32_ECX, REG32_EDX)
	case ir.OP_SHR:
		g.movRR32(REG32_EDX, REG32_ECX)
		g.movRR32(REG32_ECX, REG32_EAX)
		g.sarCl32(REG32_EDX)
		g.movRR32(REG32_ECX, REG32_EDX)
	}

	g.opPush(REG32_ECX)
}

// === Comparison operations (i386) ===

func (g *CodeGen) compileCompare_i386(setccOpcode byte) {
	g.opPop(REG32_EAX)
	g.opPop(REG32_ECX)
	g.cmpRR32(REG32_ECX, REG32_EAX)
	g.emitBytes(0x0f, setccOpcode, 0xc1) // setCC cl
	g.emitBytes(0x0f, 0xb6, 0xc9)        // movzx ecx, cl
	g.opPush(REG32_ECX)
}

func (g *CodeGen) compileCompareJump_i386(cc byte, label int) {
	g.opPop(REG32_EAX)
	g.opPop(REG32_ECX)
	g.cmpRR32(REG32_ECX, REG32_EAX)
	fixup := g.jccRel32(cc)
	g.JumpFixups = append(g.JumpFixups, JumpFixup{fixup, label, jumpFixupJccRel32, cc})
}

// === Function calls (i386) ===

func (g *CodeGen) compileCall_i386(inst ir.Inst) {
	if len(inst.Name) > 18 && inst.Name[0:18] == "builtin.composite." {
		g.compileCompositeLitCall_i386(inst)
		return
	}
	if g.isNativeCABI_i386(inst.Name) {
		g.compileNativeCall_i386(inst)
		return
	}
	g.emitCallPlaceholder(inst.Name)
}

func (g *CodeGen) compileNativeCall_i386(inst ir.Inst) {
	g.flush()
	i := 0
	for i < inst.Arg {
		g.opPop(REG32_EAX)
		g.pushR32(REG32_EAX)
		i++
	}
	g.emitCallPlaceholder(inst.Name)
	if inst.Arg > 0 {
		g.addRI32(REG32_ESP, int32(inst.Arg*g.slotBytes_i386()))
	}
	if g.funcRetCount_i386(inst.Name) > 0 {
		g.opPush(REG32_EAX)
	}
}

func (g *CodeGen) compileCompositeLitCall_i386(inst ir.Inst) {
	fieldCount := inst.Arg
	structSize := fieldCount * g.slotBytes_i386()

	if structSize == 0 {
		g.compileConstI32(0)
		return
	}

	// Save field values from operand stack onto call stack (in reverse)
	i := 0
	for i < fieldCount {
		g.opPop(REG32_EAX)
		g.pushR32(REG32_EAX)
		i++
	}

	// Allocate struct: push size, call Alloc
	g.compileConstI32(int64(structSize))
	g.emitCallPlaceholder("runtime.Alloc")
	g.opPop(REG32_ECX)

	// Pop fields from call stack and store into struct
	i = 0
	for i < fieldCount {
		g.popR32(REG32_EAX)
		offset := i * g.slotBytes_i386()
		g.storeMem32(REG32_ECX, offset, REG32_EAX)
		i++
	}

	g.opPush(REG32_ECX)
}

func (g *CodeGen) compileReturn_i386(inst ir.Inst) {
	if g.CurFunc != nil && g.isNativeCABI_i386(g.CurFunc.Name) {
		if g.CurFunc.RetCount > 0 {
			g.opPop(REG32_EAX)
		}
		if g.usesFrameOperandStack_i386(g.CurFunc.Name) && g.CurNativeSavedOpStackOffset > 0 {
			g.emitLoadLocal32(g.CurNativeSavedOpStackOffset, REG32_EDI)
		}
		g.clearOperandCache()
	} else {
		g.flush()
	}
	g.leave()
	g.ret()
}

// === Intrinsics (i386) ===

func (g *CodeGen) compileCallIntrinsic_i386(inst ir.Inst) {
	g.flush()
	if g.compileLinkStaticIntrinsicWin386(inst) {
		return
	}
	switch inst.Name {
	case "Syscall":
		// Linux i386 syscall lowering.
		g.compileSyscallIntrinsic_linux386(inst.Arg)
	case "Alloc":
		g.compileAllocIntrinsic_i386()
	case "Sliceptr":
		g.compileSliceptrIntrinsic_i386()
	case "Makeslice":
		g.compileMakesliceIntrinsic_i386()
	case "Stringptr":
		g.compileStringptrIntrinsic_i386()
	case "Makestring":
		g.compileMakestringIntrinsic_i386()
	case "Tostring":
		g.compileTostringIntrinsic_i386()
	case "ReadPtr":
		g.compileReadPtrIntrinsic_i386()
	case "WritePtr":
		g.compileWritePtrIntrinsic_i386()
	case "WriteByte":
		g.compileWriteByteIntrinsic_i386()
	default:
		panic("ICE: unknown intrinsic '" + inst.Name + "' in compileCallIntrinsic_i386")
	}
}

func (g *CodeGen) compileAllocIntrinsic_i386() {
	g.emitLoadLocal32(1*g.slotBytes_i386(), REG32_EAX)
	g.opPush(REG32_EAX)
	g.emitCallPlaceholder("runtime.Alloc")
}

func (g *CodeGen) compileSliceptrIntrinsic_i386() {
	// Param 0 = slice header pointer. Read [header+0] = data ptr.
	g.emitLoadLocal32(1*g.slotBytes_i386(), REG32_EAX)
	g.loadMem32(REG32_EAX, REG32_EAX, 0)
	g.opPush(REG32_EAX)
}

func (g *CodeGen) compileMakesliceIntrinsic_i386() {
	// Params: ptr (local 0), len (local 1), cap (local 2)
	// Allocate 4-word header {ptr, len, cap, elem_size}
	sz := 4 * g.slotBytes_i386()
	g.compileConstI32(int64(sz))
	g.emitCallPlaceholder("runtime.Alloc")
	g.opPop(REG32_ECX)

	// Fill header
	w := g.slotBytes_i386()
	g.emitLoadLocal32(1*w, REG32_EAX) // ptr
	g.storeMem32(REG32_ECX, 0, REG32_EAX)
	g.emitLoadLocal32(2*w, REG32_EAX) // len
	g.storeMem32(REG32_ECX, 1*w, REG32_EAX)
	g.emitLoadLocal32(3*w, REG32_EAX) // cap
	g.storeMem32(REG32_ECX, 2*w, REG32_EAX)
	g.emitMovRegImm32(REG32_EAX, 1) // elem_size = 1
	g.storeMem32(REG32_ECX, 3*w, REG32_EAX)

	g.opPush(REG32_ECX)
}

func (g *CodeGen) compileStringptrIntrinsic_i386() {
	// Param 0 = string header pointer. Read [header+0] = data ptr.
	g.emitLoadLocal32(1*g.slotBytes_i386(), REG32_EAX)
	g.loadMem32(REG32_EAX, REG32_EAX, 0)
	g.opPush(REG32_EAX)
}

func (g *CodeGen) compileMakestringIntrinsic_i386() {
	// Params: ptr (local 0), len (local 1)
	// Allocate 2-word header {ptr, len}
	g.compileConstI32(int64(2 * g.slotBytes_i386()))
	g.emitCallPlaceholder("runtime.Alloc")
	g.opPop(REG32_ECX)

	w := g.slotBytes_i386()
	g.emitLoadLocal32(1*w, REG32_EAX) // ptr
	g.storeMem32(REG32_ECX, 0, REG32_EAX)
	g.emitLoadLocal32(2*w, REG32_EAX) // len
	g.storeMem32(REG32_ECX, w, REG32_EAX)

	g.opPush(REG32_ECX)
}

func (g *CodeGen) compileTostringIntrinsic_i386() {
	// ir.OP_CALL_INTRINSIC is emitted inside intrinsic wrapper functions where
	// parameters are in frame locals, not on the operand stack. Inline the
	// body directly so it reads Param 0 via emitLoadLocal32.
	g.compileTostringIntrinsicBody_i386()
}

func (g *CodeGen) emitTostringHelperI386() {
	if g.HasTostringHelper {
		return
	}
	g.HasTostringHelper = true
	g.FuncOffsets[outlinedTostringHelper] = len(g.Code)
	g.initOperandCache_i386()

	slot := g.slotBytes_i386()
	g.pushR32(REG32_EBP)
	g.movRR32(REG32_EBP, REG32_ESP)
	if slot > 0 {
		g.subRI32(REG32_ESP, int32(slot))
	}

	g.opPop(REG32_EAX)
	g.emitStoreLocal32(1*slot, REG32_EAX)

	g.compileTostringIntrinsicBody_i386()
	g.compileReturn_i386(ir.Inst{})
}

func (g *CodeGen) compileTostringIntrinsicBody_i386() {
	// Param 0 = value (could be string ptr or interface box ptr)
	// Heuristic: if [ptr+0] < 256, it's a type_id (interface box)
	g.emitLoadLocal32(1*g.slotBytes_i386(), REG32_EAX) // load value

	// Test: check if [eax] < 256
	g.loadMem32(REG32_ECX, REG32_EAX, 0)
	g.cmpRI32(REG32_ECX, 256)
	stringCaseFixup := g.jccRel32(CC32_AE)

	// Interface case: ecx = type_id, [eax+ptr] = concrete value
	g.loadMem32(REG32_EDX, REG32_EAX, g.slotBytes_i386())

	// Save type_id (ecx) on call stack
	g.pushR32(REG32_ECX)

	// Generate dispatch chain for Error/String methods
	var entries []becommon.DispatchEntry
	if g.IRMod != nil && g.IRMod.TypeIDs != nil {
		for typeName, tid := range g.IRMod.TypeIDs {
			candidate := typeName + ".Error"
			if fnName, ok := becommon.LookupStringMapLinear(g.IRMod.MethodTable, candidate); ok {
				entries = append(entries, becommon.DispatchEntry{tid, fnName})
				continue
			}
			candidate = typeName + ".String"
			if fnName, ok := becommon.LookupStringMapLinear(g.IRMod.MethodTable, candidate); ok {
				entries = append(entries, becommon.DispatchEntry{tid, fnName})
			}
		}
	}
	becommon.SortDispatchEntries(entries)

	g.popR32(REG32_ECX) // type_id

	doneFixups := make([]int, 0)

	// type_id 1 = int: call runtime.IntToString
	g.cmpRI32(REG32_ECX, 1)
	nextFixup := g.jccRel32(CC32_NE)
	g.opPush(REG32_EDX)
	g.emitCallPlaceholder("runtime.IntToString")
	doneFixups = append(doneFixups, g.jmpRel32())
	g.patchRel32(nextFixup)

	// type_id 2 = string: concrete value is already a string header pointer
	g.cmpRI32(REG32_ECX, 2)
	nextFixup = g.jccRel32(CC32_NE)
	g.opPush(REG32_EDX)
	g.flush()
	doneFixups = append(doneFixups, g.jmpRel32())
	g.patchRel32(nextFixup)

	// User-defined type dispatch
	for _, entry := range entries {
		g.cmpRI32(REG32_ECX, int32(entry.TypeID))
		nextFixup = g.jccRel32(CC32_NE)
		g.opPush(REG32_EDX)
		g.emitCallPlaceholder(entry.FuncName)
		doneFixups = append(doneFixups, g.jmpRel32())
		g.patchRel32(nextFixup)
	}

	// Default: push empty string
	g.compileConstI32(0)
	g.flush()
	doneFixups = append(doneFixups, g.jmpRel32())

	// string_case: just pass through the value
	g.patchRel32(stringCaseFixup)
	g.emitLoadLocal32(1*g.slotBytes_i386(), REG32_EAX)
	g.opPush(REG32_EAX)
	g.flush()

	doneAddr := len(g.Code)
	for _, fixup := range doneFixups {
		g.patchRel32At(fixup, doneAddr)
	}
}

func (g *CodeGen) compileReadPtrIntrinsic_i386() {
	// Param 0 = addr. Read 4 bytes at addr, push result.
	g.emitLoadLocal32(1*g.slotBytes_i386(), REG32_EAX)
	g.loadMem32(REG32_EAX, REG32_EAX, 0)
	g.opPush(REG32_EAX)
}

func (g *CodeGen) compileWritePtrIntrinsic_i386() {
	// Param 0 = addr, Param 1 = val. Write 4 bytes.
	w := g.slotBytes_i386()
	g.emitLoadLocal32(1*w, REG32_EAX) // addr
	g.emitLoadLocal32(2*w, REG32_ECX) // val
	g.storeMem32(REG32_EAX, 0, REG32_ECX)
}

func (g *CodeGen) compileWriteByteIntrinsic_i386() {
	// Param 0 = addr, Param 1 = val. Write 1 byte.
	w := g.slotBytes_i386()
	g.emitLoadLocal32(1*w, REG32_EAX) // addr
	g.emitLoadLocal32(2*w, REG32_ECX) // val
	g.storeMemByte32(REG32_EAX, 0, REG32_ECX)
}

// === Interface dispatch (i386) ===

func (g *CodeGen) compileIfaceBox_i386(inst ir.Inst) {
	typeID := inst.Arg

	g.opPop(REG32_EAX)
	g.pushR32(REG32_EAX) // save concrete value

	// Allocate 2 words: {type_id, value}
	g.compileConstI32(int64(2 * g.slotBytes_i386()))
	g.emitCallPlaceholder("runtime.Alloc")
	g.opPop(REG32_ECX) // box ptr

	// Store type_id at [box+0]
	g.emitMovRegImm32(REG32_EAX, uint32(typeID))
	g.storeMem32(REG32_ECX, 0, REG32_EAX)

	// Restore concrete value and store at [box+ptr]
	g.popR32(REG32_EAX)
	g.storeMem32(REG32_ECX, g.slotBytes_i386(), REG32_EAX)

	g.opPush(REG32_ECX)
}

func (g *CodeGen) compileIfaceCall_i386(inst ir.Inst) {
	argCount := inst.Arg
	methodName := inst.Name

	// Save regular args from operand stack to call stack
	i := 0
	for i < argCount {
		g.opPop(REG32_EAX)
		g.pushR32(REG32_EAX)
		i++
	}

	// Pop interface pointer
	g.opPop(REG32_EAX)

	// Load type_id from [eax+0], concrete value from [eax+ptr]
	g.loadMem32(REG32_ECX, REG32_EAX, 0)                  // type_id
	g.loadMem32(REG32_EDX, REG32_EAX, g.slotBytes_i386()) // concrete value

	// Push receiver once and materialize it before branch dispatch.
	g.opPush(REG32_EDX)

	// Restore regular args
	i = argCount - 1
	for i >= 0 {
		g.popR32(REG32_EAX)
		g.opPush(REG32_EAX)
		i = i - 1
	}
	// Ensure restored args are materialized for all dispatch branches.
	g.flush()

	// Save ecx (type_id)
	g.pushR32(REG32_ECX)

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
	if g.IRMod != nil && g.IRMod.TypeIDs != nil {
		for typeName, tid := range g.IRMod.TypeIDs {
			candidate := typeName + "." + bareMethod
			if fnName, ok := becommon.LookupStringMapLinear(g.IRMod.MethodTable, candidate); ok {
				entries = append(entries, becommon.DispatchEntry{tid, fnName})
			}
		}
	}
	becommon.SortDispatchEntries(entries)

	g.popR32(REG32_ECX) // type_id

	if len(entries) == 0 {
		g.int3()
	} else {
		endFixups := make([]int, 0)
		for _, entry := range entries {
			g.cmpRI32(REG32_ECX, int32(entry.TypeID))
			nextFixup := g.jccRel32(CC32_NE)
			g.emitCallPlaceholder(entry.FuncName)
			endFixups = append(endFixups, g.jmpRel32())
			g.patchRel32(nextFixup)
		}
		g.int3()
		endAddr := len(g.Code)
		for _, fixup := range endFixups {
			g.patchRel32At(fixup, endAddr)
		}
	}
}

// === Memory operations (i386) ===

func (g *CodeGen) compileLoad_i386(inst ir.Inst) {
	size := inst.Arg
	offset := int(inst.Val)
	g.opPop(REG32_ECX)
	if offset != 0 && !ir.IsNonNilMemoryBase(inst.Name) {
		// Preserve IR semantics: nil-guarded LOAD checks the effective address
		// after OP_OFFSET, not the original base pointer.
		g.addRI32(REG32_ECX, int32(offset))
		offset = 0
	}
	if ir.IsNonNilMemoryBase(inst.Name) {
		if size == 1 {
			g.loadMemByte32(REG32_EAX, REG32_ECX, offset) // movzx eax, byte [ecx+off]
		} else {
			g.loadMem32(REG32_EAX, REG32_ECX, offset) // mov eax, [ecx+off]
		}
		g.opPush(REG32_EAX)
		return
	}
	g.testRR32(REG32_ECX, REG32_ECX)
	if size == 0 {
		size = 4
	}
	if size == 1 {
		g.emitBytes(0x75, 0x04)                  // jnz +4
		g.xorRR32(REG32_EAX, REG32_EAX)          // 2 bytes
		g.emitBytes(0xeb, 0x03)                  // jmp +3
		g.loadMemByte32(REG32_EAX, REG32_ECX, 0) // movzx eax, byte [ecx]
	} else if size == 2 {
		g.emitBytes(0x75, 0x04)         // jnz +4
		g.xorRR32(REG32_EAX, REG32_EAX) // 2 bytes
		g.emitBytes(0xeb, 0x03)         // jmp +3
		g.emitBytes(0x0f, 0xb7, 0x01)   // movzx eax, word [ecx]
	} else {
		g.emitBytes(0x75, 0x04)         // jnz +4
		g.xorRR32(REG32_EAX, REG32_EAX) // 2 bytes
		g.emitBytes(0xeb, 0x02)         // jmp +2
		g.loadMem32(REG32_EAX, REG32_ECX, offset)
	}
	g.opPush(REG32_EAX)
}

func (g *CodeGen) compileStore_i386(inst ir.Inst) {
	size := inst.Arg
	offset := int(inst.Val)
	g.opPop(REG32_ECX) // addr
	g.opPop(REG32_EAX) // value
	if size == 0 {
		size = 4
	}
	if size == 1 {
		g.storeMemByte32(REG32_ECX, offset, REG32_EAX)
	} else if size == 2 {
		g.storeMem16(REG32_ECX, offset, REG32_EAX)
	} else {
		g.storeMem32(REG32_ECX, offset, REG32_EAX)
	}
}

func (g *CodeGen) compileOffset_i386(inst ir.Inst) {
	g.opPop(REG32_EAX)
	if inst.Arg != 0 {
		g.addRI32(REG32_EAX, int32(inst.Arg))
	}
	g.opPush(REG32_EAX)
}

func (g *CodeGen) compileIndexAddr_i386(elemSize int) {
	g.opPop(REG32_EAX) // index
	g.opPop(REG32_ECX) // slice header ptr

	// Load data_ptr from header: [ecx+0]
	g.loadMem32(REG32_ECX, REG32_ECX, 0)

	// Compute address: data_ptr + index * elemSize
	if elemSize == 1 {
		g.addRR32(REG32_ECX, REG32_EAX)
	} else if elemSize == 4 {
		g.shlImm32(REG32_EAX, 2)
		g.addRR32(REG32_ECX, REG32_EAX)
	} else {
		g.imulRRI32_32(REG32_EAX, REG32_EAX, int32(elemSize))
		g.addRR32(REG32_ECX, REG32_EAX)
	}

	g.opPush(REG32_ECX)
}

func (g *CodeGen) compileLen_i386(inst ir.Inst) {
	g.opPop(REG32_EAX)
	if ir.IsNonNilMemoryBase(inst.Name) {
		g.loadMem32(REG32_EAX, REG32_EAX, g.slotBytes_i386()) // len at offset ptr
		g.opPush(REG32_EAX)
		return
	}
	g.testRR32(REG32_EAX, REG32_EAX)
	fixNonNil := g.jccRel32(CC32_NE)
	g.xorRR32(REG32_EAX, REG32_EAX)
	fixDone := g.jmpRel32()
	g.patchRel32(fixNonNil)
	g.loadMem32(REG32_EAX, REG32_EAX, g.slotBytes_i386()) // len at offset ptr
	g.patchRel32(fixDone)
	g.opPush(REG32_EAX)
}

func (g *CodeGen) compileCap_i386(inst ir.Inst) {
	g.opPop(REG32_EAX)
	if ir.IsNonNilMemoryBase(inst.Name) {
		g.loadMem32(REG32_EAX, REG32_EAX, 2*g.slotBytes_i386()) // cap at offset 2*ptr
		g.opPush(REG32_EAX)
		return
	}
	g.testRR32(REG32_EAX, REG32_EAX)
	fixNonNil := g.jccRel32(CC32_NE)
	g.xorRR32(REG32_EAX, REG32_EAX)
	fixDone := g.jmpRel32()
	g.patchRel32(fixNonNil)
	g.loadMem32(REG32_EAX, REG32_EAX, 2*g.slotBytes_i386()) // cap at offset 2*ptr
	g.patchRel32(fixDone)
	g.opPush(REG32_EAX)
}

// === Type conversions (i386) ===

func (g *CodeGen) compileConvert_i386(typeName string) {
	switch typeName {
	case "string":
		g.emitCallPlaceholder("runtime.BytesToString")
	case "[]byte":
		g.emitCallPlaceholder("runtime.StringToBytes")
	case "int", "uintptr", "uint", "int32", "uint32":
		// No-op: all 4-byte integers on i386
	case "byte":
		g.opPop(REG32_EAX)
		g.movzxB32(REG32_EAX)
		g.opPush(REG32_EAX)
	case "uint8":
		g.opPop(REG32_EAX)
		g.movzxB32(REG32_EAX)
		g.opPush(REG32_EAX)
	case "int8":
		g.opPop(REG32_EAX)
		// Sign-extend low byte via shift pair (EAX = int8(EAX)).
		g.emitBytes(0xc1, 0xe0, 24) // shl eax, 24
		g.emitBytes(0xc1, 0xf8, 24) // sar eax, 24
		g.opPush(REG32_EAX)
	case "uint16":
		g.opPop(REG32_EAX)
		g.movzxW32(REG32_EAX)
		g.opPush(REG32_EAX)
	case "int16":
		g.opPop(REG32_EAX)
		g.movsxW32(REG32_EAX)
		g.opPush(REG32_EAX)
	case "int64", "uint64":
		// On i386, 64-bit types truncated to 32-bit (best effort)
	}
}
