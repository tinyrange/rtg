//go:build !no_backend_linux_amd64 || !no_backend_windows_amd64

package x64

import (
	"j5.nz/rtg/std/compiler/backend/becommon"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

func sortDispatchEntries(entries []DispatchEntry) {
	i := 1
	for i < len(entries) {
		key := entries[i]
		j := i - 1
		for j >= 0 {
			if entries[j].typeID < key.typeID {
				break
			}
			if entries[j].typeID == key.typeID && entries[j].funcName <= key.funcName {
				break
			}
			entries[j+1] = entries[j]
			j = j - 1
		}
		entries[j+1] = key
		i = i + 1
	}
}

// CompileFunc generates x86-64 code for a single IR function.
func (g *CodeGen) CompileFunc(f *ir.IRFunc) {
	if f.Native != nil {
		if f.Native.Arch != "amd64" {
			panic("ICE: x64 backend received native function for arch " + f.Native.Arch)
		}
		funcStart := g.funcOffsets[f.Name]
		g.Code = append(g.Code, f.Native.Code...)
		for _, fx := range f.Native.Fixups {
			if fx.Kind != ir.NativeFixupCallRel32 {
				continue
			}
			g.callFixups = append(g.callFixups, CallFixup{funcStart + fx.Off, fx.Target, 0})
		}
		return
	}
	g.curFunc = f
	g.configureOperandCache(REG_R12, REG_R13, REG_R14)
	g.curFrameSize = len(f.Locals)
	// Intrinsic functions may have Params > 0 but empty Locals.
	// Ensure the frame is large enough to hold all param slots.
	if f.Params > g.curFrameSize {
		g.curFrameSize = f.Params
	}
	g.labelOffsets = make(map[int]int)
	g.jumpFixups = nil

	// Prologue: push rbp; mov rbp, rsp; sub rsp, N*8
	g.PushR(REG_RBP)
	g.MovRR(REG_RBP, REG_RSP)

	frameBytes := g.curFrameSize * 8
	if g.target.GOOS == "windows" {
		frameBytes = common.AlignUp(frameBytes, 16)
	}
	if frameBytes > 0 {
		g.SubRI(REG_RSP, int32(frameBytes))
	}

	// Pop params from operand stack (R15) into local frame slots
	// Params are pushed left-to-right by caller, so param 0 is deepest.
	// We pop in reverse order: last param first.
	if f.Params > 0 {
		i := f.Params - 1
		for i >= 0 {
			g.OpPop(REG_RAX)
			offset := (i + 1) * 8
			g.emitStoreLocal(offset, REG_RAX)
			i = i - 1
		}
	}

	// Compile ir.Instructions
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
		g.compileInst(inst)
		i++
	}

	// Resolve jump fixups within this function
	funcStart := g.funcOffsets[f.Name]
	if g.target.GOOS != "dos" {
		g.relaxCurrentFuncJumps(funcStart)
	}
	for _, fix := range g.jumpFixups {
		labelOff, ok := g.labelOffsets[fix.LabelID]
		if !ok {
			continue
		}
		switch fix.Kind {
		case jumpFixupJmpRel8, jumpFixupJccRel8:
			rel := labelOff - (fix.CodeOffset + 1)
			g.Code[fix.CodeOffset] = byte(rel)
		default:
			g.PatchRel32At(fix.CodeOffset, labelOff)
		}
	}

	_ = funcStart
	g.curFunc = nil
}

// compileInst generates code for a single IR ir.Instruction.
func (g *CodeGen) compileInst(inst ir.Inst) {
	switch inst.Op {
	case ir.OP_CONST_I64:
		if len(inst.Name) > 10 && inst.Name[0:10] == "$funcaddr$" {
			g.compileFuncAddr(inst.Name)
		} else {
			g.CompileConstI64(inst.Val)
		}
	case ir.OP_CONST_BOOL:
		if inst.Arg != 0 {
			g.CompileConstI64(1)
		} else {
			g.CompileConstI64(0)
		}
	case ir.OP_CONST_NIL:
		g.CompileConstI64(0)
	case ir.OP_CONST_STR:
		g.CompileConstStr(inst.Name)

	case ir.OP_LOCAL_GET:
		g.compileLocalGet(inst.Arg)
	case ir.OP_LOCAL_SET:
		g.compileLocalSet(inst.Arg, inst.Width)
	case ir.OP_LOCAL_ADD_IMM:
		g.compileLocalAddImm(inst.Arg, int32(inst.Val))
	case ir.OP_LOCAL_ADDR:
		g.compileLocalAddr(inst.Arg)

	case ir.OP_GLOBAL_GET:
		g.compileGlobalGet(inst)
	case ir.OP_GLOBAL_SET:
		g.compileGlobalSet(inst)
	case ir.OP_GLOBAL_ADDR:
		g.compileGlobalAddr(inst)

	case ir.OP_DROP:
		g.opDrop()
	case ir.OP_DUP:
		g.opLoad(REG_RAX)
		g.OpPush(REG_RAX)

	case ir.OP_ADD:
		g.compileBinOp(inst)
	case ir.OP_SUB:
		g.compileBinOp(inst)
	case ir.OP_MUL:
		g.compileBinOp(inst)
	case ir.OP_DIV:
		g.compileBinOp(inst)
	case ir.OP_MOD:
		g.compileBinOp(inst)
	case ir.OP_NEG:
		g.OpPop(REG_RAX)
		g.negR(REG_RAX)
		g.OpPush(REG_RAX)

	case ir.OP_AND:
		g.compileBinOp(inst)
	case ir.OP_OR:
		g.compileBinOp(inst)
	case ir.OP_XOR:
		g.compileBinOp(inst)
	case ir.OP_SHL:
		g.compileBinOp(inst)
	case ir.OP_SHR:
		g.compileBinOp(inst)

	case ir.OP_EQ:
		g.compileCompare(inst, 0x94, 0x94) // sete
	case ir.OP_NEQ:
		g.compileCompare(inst, 0x95, 0x95) // setne
	case ir.OP_LT:
		g.compileCompare(inst, 0x9c, 0x92) // setl / setb
	case ir.OP_GT:
		g.compileCompare(inst, 0x9f, 0x97) // setg / seta
	case ir.OP_LEQ:
		g.compileCompare(inst, 0x9e, 0x96) // setle / setbe
	case ir.OP_GEQ:
		g.compileCompare(inst, 0x9d, 0x93) // setge / setae

	case ir.OP_NOT:
		g.OpPop(REG_RAX)
		g.xorRI8(REG_RAX, 0x01)
		g.OpPush(REG_RAX)

	case ir.OP_LABEL:
		g.Flush()
		g.labelOffsets[inst.Arg] = len(g.Code)
	case ir.OP_JMP:
		g.Flush()
		fixup := g.JmpRel32()
		g.jumpFixups = append(g.jumpFixups, JumpFixup{fixup, inst.Arg, jumpFixupJmpRel32, 0})
	case ir.OP_JMP_IF:
		// pop value, test, jnz
		g.OpPop(REG_RAX)
		g.TestRR(REG_RAX, REG_RAX)
		fixup := g.JccRel32(CC_NE)
		g.jumpFixups = append(g.jumpFixups, JumpFixup{fixup, inst.Arg, jumpFixupJccRel32, CC_NE})
	case ir.OP_JMP_IF_NOT:
		// pop value, test, jz
		g.OpPop(REG_RAX)
		g.TestRR(REG_RAX, REG_RAX)
		fixup := g.JccRel32(CC_E)
		g.jumpFixups = append(g.jumpFixups, JumpFixup{fixup, inst.Arg, jumpFixupJccRel32, CC_E})
	case ir.OP_JMP_EQ:
		g.compileCompareJump(inst, CC_E, inst.Arg)
	case ir.OP_JMP_NEQ:
		g.compileCompareJump(inst, CC_NE, inst.Arg)
	case ir.OP_JMP_LT:
		cc := byte(CC_L)
		if inst.Name == "unsigned" {
			cc = CC_B
		}
		g.compileCompareJump(inst, cc, inst.Arg)
	case ir.OP_JMP_GT:
		cc := byte(CC_G)
		if inst.Name == "unsigned" {
			cc = CC_A
		}
		g.compileCompareJump(inst, cc, inst.Arg)
	case ir.OP_JMP_LEQ:
		cc := byte(CC_LE)
		if inst.Name == "unsigned" {
			cc = CC_BE
		}
		g.compileCompareJump(inst, cc, inst.Arg)
	case ir.OP_JMP_GEQ:
		cc := byte(CC_GE)
		if inst.Name == "unsigned" {
			cc = CC_AE
		}
		g.compileCompareJump(inst, cc, inst.Arg)

	case ir.OP_CALL:
		g.compileCall(inst)
	case ir.OP_RETURN:
		g.compileReturn(inst)

	case ir.OP_LOAD:
		g.compileLoad(inst)
	case ir.OP_STORE:
		g.compileStore(inst)
	case ir.OP_OFFSET:
		g.compileOffset(inst)
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
		compilePanicTarget(g)
	case ir.OP_CALL_INTRINSIC:
		g.Flush()
		compileCallIntrinsicTarget(g, inst)
	case ir.OP_SLICE_GET, ir.OP_SLICE_MAKE, ir.OP_STRING_GET, ir.OP_STRING_MAKE:
		// These are handled by intrinsics or builtins

	default:
		panic("ICE: unhandled ir.Opcode in compileInst")
	}
}

// compileFuncAddr emits a movabs rax, imm64 with a fixup to be resolved
// to the virtual address of the named function (or its callback thunk).
func (g *CodeGen) compileFuncAddr(marker string) {
	g.prepareForClobber(REG_RAX)
	funcName := marker[10:] // strip "$funcaddr$"
	// If it's a callback function, reference the thunk wrapper instead
	thunkName := funcName
	if g.irmod != nil && g.irmod.CallbackFuncs != nil && g.irmod.CallbackFuncs[funcName] {
		thunkName = "$callback_thunk$" + funcName
	}
	g.EmitMovRegImm64(REG_RAX, 0) // placeholder imm64
	g.callFixups = append(g.callFixups, CallFixup{len(g.Code) - 8, "$funcaddr$" + thunkName, 0})
	g.OpPush(REG_RAX)
}

// === Constant loading ===

func (g *CodeGen) CompileConstI64(val int64) {
	g.prepareForClobber(REG_RAX)
	if val == 0 {
		g.XorRR(REG_RAX, REG_RAX) // 3 bytes ir.Instead of 10
	} else if val > 0 && val <= 0x7fffffff {
		// mov eax, imm32 (zero-extends to rax)
		g.EmitByte(0xb8) // mov eax, imm32
		g.EmitU32(uint32(val))
	} else if val < 0 && val >= -0x80000000 {
		// mov rax, sign-extended imm32
		g.EmitBytes(0x48, 0xc7, 0xc0) // mov rax, imm32 (sign-extended)
		g.EmitU32(uint32(val))
	} else {
		g.EmitMovRegImm64(REG_RAX, uint64(val))
	}
	g.OpPush(REG_RAX)
}

func (g *CodeGen) CompileConstStr(s string) {
	g.prepareForClobber(REG_RAX)
	decoded, ok := g.decodedStrMap[s]
	if !ok {
		decoded = becommon.DecodeStringLiteral(s)
		g.decodedStrMap[s] = decoded
	}

	headerOff, ok := g.stringMap[decoded]
	if !ok {
		// Store string bytes in rodata
		dataOff := len(g.Rodata)
		g.Rodata = append(g.Rodata, decoded...)

		// Store 16-byte header {data_ptr, len} in rodata
		// data_ptr will need fixup when we know rodata's virtual address
		headerOff = len(g.Rodata)
		// placeholder for data_ptr (8 bytes) — will be fixed up
		g.EmitRodataU64(0)                    // data_ptr placeholder
		g.EmitRodataU64(uint64(len(decoded))) // len

		// Record for fixup: header needs data_ptr = rodataVAddr + dataOff
		g.stringMap[decoded] = headerOff
		// We store dataOff in the placeholder temporarily
		common.PutU64(g.Rodata[headerOff:headerOff+8], uint64(dataOff))
	}

	// Push header address onto operand stack
	g.EmitMovRegImm64(REG_RAX, uint64(headerOff))
	g.callFixups = append(g.callFixups, CallFixup{len(g.Code) - 8, "$rodata_header$", 0})
	g.OpPush(REG_RAX)
}

// === Local variable access ===

func (g *CodeGen) compileLocalGet(idx int) {
	g.prepareForClobber(REG_RAX)
	offset := (idx + 1) * 8
	g.EmitLoadLocal(offset, REG_RAX)
	g.OpPush(REG_RAX)
}

func (g *CodeGen) compileLocalSet(idx int, width int) {
	g.OpPop(REG_RAX)
	switch width {
	case 1:
		g.movzxB(REG_RAX)
	case 2:
		g.movzxW(REG_RAX)
	case 4:
		g.clearHi32(REG_RAX)
	}
	offset := (idx + 1) * 8
	g.emitStoreLocal(offset, REG_RAX)
}

func (g *CodeGen) compileLocalAddImm(idx int, imm int32) {
	offset := (idx + 1) * 8
	g.emitAddLocalImm(offset, imm)
}

func (g *CodeGen) compileLocalAddr(idx int) {
	g.prepareForClobber(REG_RAX)
	offset := (idx + 1) * 8
	g.emitLeaLocal(offset, REG_RAX)
	g.OpPush(REG_RAX)
}

// === Global variable access ===

func (g *CodeGen) compileGlobalGet(inst ir.Inst) {
	g.prepareForClobber(REG_RAX, REG_RCX)
	g.EmitMovRegImm64(REG_RCX, uint64(inst.Arg*8)) // offset placeholder
	g.callFixups = append(g.callFixups, CallFixup{len(g.Code) - 8, "$data_addr$", 0})
	g.LoadMem(REG_RAX, REG_RCX, 0)
	g.OpPush(REG_RAX)
}

func (g *CodeGen) compileGlobalSet(inst ir.Inst) {
	g.OpPop(REG_RAX)
	g.EmitMovRegImm64(REG_RCX, uint64(inst.Arg*8)) // offset placeholder
	g.callFixups = append(g.callFixups, CallFixup{len(g.Code) - 8, "$data_addr$", 0})
	g.storeMem(REG_RCX, 0, REG_RAX)
}

func (g *CodeGen) compileGlobalAddr(inst ir.Inst) {
	g.prepareForClobber(REG_RAX)
	g.EmitMovRegImm64(REG_RAX, uint64(inst.Arg*8)) // offset placeholder
	g.callFixups = append(g.callFixups, CallFixup{len(g.Code) - 8, "$data_addr$", 0})
	g.OpPush(REG_RAX)
}

// === Binary operations ===

func (g *CodeGen) compileBinOp(inst ir.Inst) {
	// pop two values: rax = second (top), rcx = first (below), push result
	g.OpPop(REG_RAX)
	g.OpPop(REG_RCX)

	switch inst.Op {
	case ir.OP_ADD:
		g.AddRR(REG_RCX, REG_RAX)
	case ir.OP_SUB:
		g.subRR(REG_RCX, REG_RAX)
	case ir.OP_MUL:
		g.imulRR(REG_RCX, REG_RAX)
	case ir.OP_DIV:
		g.MovRR(REG_RDX, REG_RAX)
		g.MovRR(REG_RAX, REG_RCX)
		g.MovRR(REG_RCX, REG_RDX)
		if inst.Name == "unsigned" {
			g.XorRR(REG_RDX, REG_RDX)
			g.divR(REG_RCX)
		} else {
			g.cqo()
			g.idivR(REG_RCX)
		}
		g.MovRR(REG_RCX, REG_RAX)
	case ir.OP_MOD:
		g.MovRR(REG_RDX, REG_RAX)
		g.MovRR(REG_RAX, REG_RCX)
		g.MovRR(REG_RCX, REG_RDX)
		if inst.Name == "unsigned" {
			g.XorRR(REG_RDX, REG_RDX)
			g.divR(REG_RCX)
		} else {
			g.cqo()
			g.idivR(REG_RCX)
		}
		g.MovRR(REG_RCX, REG_RDX)
	case ir.OP_AND:
		g.andRR(REG_RCX, REG_RAX)
	case ir.OP_OR:
		g.orRR(REG_RCX, REG_RAX)
	case ir.OP_XOR:
		g.XorRR(REG_RCX, REG_RAX)
	case ir.OP_SHL:
		g.MovRR(REG_RDX, REG_RCX)
		g.MovRR(REG_RCX, REG_RAX)
		g.shlCl(REG_RDX)
		g.MovRR(REG_RCX, REG_RDX)
	case ir.OP_SHR:
		g.MovRR(REG_RDX, REG_RCX)
		g.MovRR(REG_RCX, REG_RAX)
		if inst.Name == "unsigned" {
			g.shrCl(REG_RDX)
		} else {
			g.sarCl(REG_RDX)
		}
		g.MovRR(REG_RCX, REG_RDX)
	}

	g.OpPush(REG_RCX)
}

// === Comparison operations ===

func (g *CodeGen) normalizeCompareRegWidth(reg int, width int, signed bool) {
	switch width {
	case 1:
		if signed {
			g.movsxB(reg)
		} else {
			g.movzxB(reg)
		}
	case 2:
		if signed {
			g.movsxW(reg)
		} else {
			g.movzxW(reg)
		}
	case 4:
		if signed {
			g.movsxD(reg)
		} else {
			g.clearHi32(reg)
		}
	}
}

func (g *CodeGen) compileCompare(inst ir.Inst, signedSetccOpcode byte, unsignedSetccOpcode byte) {
	g.OpPop(REG_RAX)
	g.OpPop(REG_RCX)
	if inst.Width > 0 && inst.Width < 8 {
		signed := inst.Name != "unsigned" && (inst.Op == ir.OP_LT || inst.Op == ir.OP_GT || inst.Op == ir.OP_LEQ || inst.Op == ir.OP_GEQ)
		g.normalizeCompareRegWidth(REG_RCX, inst.Width, signed)
		g.normalizeCompareRegWidth(REG_RAX, inst.Width, signed)
	}
	g.CmpRR(REG_RCX, REG_RAX)
	setccOpcode := signedSetccOpcode
	if inst.Name == "unsigned" {
		setccOpcode = unsignedSetccOpcode
	}
	g.EmitBytes(0x0f, setccOpcode, 0xc1) // setCC cl
	g.EmitBytes(0x48, 0x0f, 0xb6, 0xc9)  // movzx rcx, cl
	g.OpPush(REG_RCX)
}

func (g *CodeGen) compileCompareJump(inst ir.Inst, cc byte, label int) {
	g.OpPop(REG_RAX)
	g.OpPop(REG_RCX)
	if inst.Width > 0 && inst.Width < 8 {
		signed := inst.Name != "unsigned" && (inst.Op == ir.OP_JMP_LT || inst.Op == ir.OP_JMP_GT || inst.Op == ir.OP_JMP_LEQ || inst.Op == ir.OP_JMP_GEQ)
		g.normalizeCompareRegWidth(REG_RCX, inst.Width, signed)
		g.normalizeCompareRegWidth(REG_RAX, inst.Width, signed)
	}
	g.CmpRR(REG_RCX, REG_RAX)
	fixup := g.JccRel32(cc)
	g.jumpFixups = append(g.jumpFixups, JumpFixup{fixup, label, jumpFixupJccRel32, cc})
}

// === Function calls ===

func (g *CodeGen) compileCall(inst ir.Inst) {
	// Handle composite literals inline
	if len(inst.Name) > 18 && inst.Name[0:18] == "builtin.composite." {
		g.compileCompositeLitCall(inst)
		return
	}

	// Arguments are already on the operand stack.
	// Just emit the call — callee will pop args.
	g.EmitCallPlaceholder(inst.Name)

	// If the call target returns values, they are already pushed by callee.
	// The IR handles push/pop balance.
}

// compileCompositeLitCall handles struct/slice composite literal creation.
// Fields are on the operand stack (pushed in order). We allocate memory
// and store each field at consecutive 8-byte slots.
func (g *CodeGen) compileCompositeLitCall(inst ir.Inst) {
	fieldCount := inst.Arg
	structSize := fieldCount * 8

	if structSize == 0 {
		// Empty struct — push nil
		g.CompileConstI64(0)
		return
	}

	// Save field values from operand stack onto call stack (in reverse)
	i := 0
	for i < fieldCount {
		g.OpPop(REG_RAX)
		g.PushR(REG_RAX)
		i++
	}

	// Allocate struct: push size, call Alloc
	g.CompileConstI64(int64(structSize))
	g.EmitCallPlaceholder("runtime.Alloc")
	// Result (struct ptr) on operand stack
	g.OpPop(REG_RCX)

	// Pop fields from call stack and store into struct in declaration order.
	// The save loop popped the operand stack top-first (last field first),
	// so on the x86 stack field0 is on top. Store field0 at offset 0, etc.
	i = 0
	for i < fieldCount {
		g.PopR(REG_RAX)
		offset := i * 8
		if offset == 0 {
			g.storeMem(REG_RCX, 0, REG_RAX)
		} else if offset <= 127 {
			g.storeMem(REG_RCX, offset, REG_RAX)
		} else {
			g.EmitBytes(0x48, 0x89, 0x81) // mov [rcx+off32], rax
			g.EmitU32(uint32(offset))
		}
		i++
	}

	// Push struct pointer as result
	g.OpPush(REG_RCX)
}

func (g *CodeGen) compileReturn(inst ir.Inst) {
	g.Flush()
	g.leave()
	g.ret()
}

// === Intrinsics ===
func (g *CodeGen) CompileSliceptrIntrinsic() {
	// Param 0 = slice header pointer. Read [header+0] = data ptr.
	g.EmitLoadLocal(1*8, REG_RAX)
	g.LoadMem(REG_RAX, REG_RAX, 0)
	g.OpPush(REG_RAX)
}

func (g *CodeGen) CompileMakesliceIntrinsic() {
	// Params: ptr (local 0), len (local 1), cap (local 2)
	// Allocate 32 bytes for header, fill {ptr, len, cap, elem_size=1}, push header addr
	// Makeslice always creates byte slices, so elem_size=1.

	g.CompileConstI64(32)
	g.EmitCallPlaceholder("runtime.Alloc")
	g.OpPop(REG_RCX)

	// Fill header: [rcx+0] = ptr, [rcx+8] = len, [rcx+16] = cap, [rcx+24] = 1
	g.EmitLoadLocal(1*8, REG_RAX)
	g.storeMem(REG_RCX, 0, REG_RAX)
	g.EmitLoadLocal(2*8, REG_RAX)
	g.storeMem(REG_RCX, 8, REG_RAX)
	g.EmitLoadLocal(3*8, REG_RAX)
	g.storeMem(REG_RCX, 16, REG_RAX)
	g.EmitByte(0xb8) // mov eax, 1
	g.EmitU32(1)
	g.storeMem(REG_RCX, 24, REG_RAX)

	// Push header pointer
	g.OpPush(REG_RCX)
}

func (g *CodeGen) CompileStringptrIntrinsic() {
	// Param 0 = string header pointer. Read [header+0] = data ptr.
	g.EmitLoadLocal(1*8, REG_RAX)
	g.LoadMem(REG_RAX, REG_RAX, 0)
	g.OpPush(REG_RAX)
}

func (g *CodeGen) CompileMakestringIntrinsic() {
	// Params: ptr (local 0), len (local 1)
	// Allocate 16-byte header, fill {ptr, len}, push header addr
	g.CompileConstI64(16)
	g.EmitCallPlaceholder("runtime.Alloc")
	g.OpPop(REG_RCX)

	g.EmitLoadLocal(1*8, REG_RAX)
	g.storeMem(REG_RCX, 0, REG_RAX)
	g.EmitLoadLocal(2*8, REG_RAX)
	g.storeMem(REG_RCX, 8, REG_RAX)

	g.OpPush(REG_RCX)
}

func (g *CodeGen) CompileTostringIntrinsic() {
	// ir.OP_CALL_INTRINSIC is emitted inside intrinsic wrapper functions where
	// parameters are in frame locals, not on the operand stack. Inline the
	// body directly so it reads Param 0 via emitLoadLocal.
	g.compileTostringIntrinsicBodyX64()
}

func (g *CodeGen) EmitTostringHelperX64() {
	if g.hasTostringHelper {
		return
	}
	g.hasTostringHelper = true
	g.funcOffsets[outlinedTostringHelper] = len(g.Code)
	g.configureOperandCache(REG_R12, REG_R13, REG_R14)

	g.PushR(REG_RBP)
	g.MovRR(REG_RBP, REG_RSP)

	frameBytes := 8
	if g.target.GOOS == "windows" {
		frameBytes = common.AlignUp(frameBytes, 16)
	}
	if frameBytes > 0 {
		g.SubRI(REG_RSP, int32(frameBytes))
	}

	g.OpPop(REG_RAX)
	g.emitStoreLocal(1*8, REG_RAX)

	g.compileTostringIntrinsicBodyX64()
	g.compileReturn(ir.Inst{})
}

func (g *CodeGen) compileTostringIntrinsicBodyX64() {
	// Param 0 = value (could be string ptr or interface box ptr)
	// Heuristic:
	// - canonical interface box: [ptr+0]=type_id (<256), [ptr+8]=value
	// - string header pointer: [ptr+0]=data_ptr (>=256)
	// - tolerate swapped boxes: [ptr+0]=value, [ptr+8]=type_id (<256)
	g.EmitLoadLocal(1*8, REG_RAX) // load value

	// Load first qword and check canonical interface layout.
	g.LoadMem(REG_RCX, REG_RAX, 0)
	g.EmitBytes(0x48, 0x81, 0xf9) // cmp rcx, 256
	g.EmitU32(256)
	ifaceCaseFixup := g.JccRel32(0x82) // jb

	// [ptr+0] >= 256: either string pointer or swapped interface box.
	g.LoadMem(REG_RDX, REG_RAX, 8)
	g.EmitBytes(0x48, 0x81, 0xfa) // cmp rdx, 256
	g.EmitU32(256)
	stringCaseFixup := g.JccRel32(CC_AE)

	// Swapped interface layout: rcx=value, rdx=type_id.
	g.OpPush(REG_RCX)
	g.MovRR(REG_RCX, REG_RDX)
	dispatchFixup := g.JmpRel32()

	// Canonical interface layout: rcx=type_id, [rax+8]=value.
	g.PatchRel32(ifaceCaseFixup)
	g.LoadMem(REG_RDX, REG_RAX, 8)
	g.OpPush(REG_RDX)

	// Generate dispatch chain for "Error" method
	var entries []DispatchEntry
	if g.irmod != nil && g.irmod.TypeIDs != nil {
		for typeName, tid := range g.irmod.TypeIDs {
			// Check for Error method first, then String
			candidate := typeName + ".Error"
			if fnName, ok := becommon.LookupStringMapLinear(g.irmod.MethodTable, candidate); ok {
				entries = append(entries, DispatchEntry{tid, fnName})
				continue
			}
			candidate = typeName + ".String"
			if fnName, ok := becommon.LookupStringMapLinear(g.irmod.MethodTable, candidate); ok {
				entries = append(entries, DispatchEntry{tid, fnName})
			}
		}
	}
	sortDispatchEntries(entries)

	g.PatchRel32(dispatchFixup)

	// Always generate dispatch chain (with built-in int/string + user types)
	endFixups := make([]int, 0)

	// type_id 1 = int: call runtime.IntToString
	g.cmpRI(REG_RCX, 1)
	nextFixup := g.JccRel32(CC_NE)
	g.EmitCallPlaceholder("runtime.IntToString")
	endFixups = append(endFixups, g.JmpRel32())
	g.PatchRel32(nextFixup)

	// type_id 2 = string: value is already a string ptr, pass through
	g.cmpRI(REG_RCX, 2)
	nextFixup = g.JccRel32(CC_NE)
	// Concrete value is already on the operand stack, nothing to do
	endFixups = append(endFixups, g.JmpRel32())
	g.PatchRel32(nextFixup)

	// User-defined type dispatch (Error/String methods)
	for _, entry := range entries {
		if entry.typeID <= 127 {
			g.cmpRI(REG_RCX, int32(entry.typeID))
		} else {
			g.EmitBytes(0x48, 0x81, 0xf9)
			g.EmitU32(uint32(entry.typeID))
		}
		nextFixup = g.JccRel32(CC_NE)

		g.EmitCallPlaceholder(entry.funcName)

		endFixups = append(endFixups, g.JmpRel32())

		g.PatchRel32(nextFixup)
	}
	// Default: push empty string
	g.opDrop() // drop receiver
	g.CompileConstI64(0)
	g.Flush() // materialize the pending push before setting endAddr

	endAddr := len(g.Code)
	for _, fixup := range endFixups {
		g.PatchRel32At(fixup, endAddr)
	}

	// Jump past the string case (jmp to final end)
	finalEndFixup := g.JmpRel32()

	// string_case: just pass through the value (already a string ptr)
	g.PatchRel32(stringCaseFixup)

	g.EmitLoadLocal(1*8, REG_RAX)
	g.OpPush(REG_RAX)
	g.Flush() // materialize result before convergence with dispatch paths

	// final_end:
	g.PatchRel32(finalEndFixup)
}

func (g *CodeGen) CompileReadPtrIntrinsic() {
	// Param 0 = addr. Read 8 bytes at addr, push result.
	g.EmitLoadLocal(1*8, REG_RAX)
	g.LoadMem(REG_RAX, REG_RAX, 0)
	g.OpPush(REG_RAX)
}

func (g *CodeGen) CompileWritePtrIntrinsic() {
	// Param 0 = addr, Param 1 = val. Write 8 bytes.
	g.EmitLoadLocal(1*8, REG_RAX) // addr
	g.EmitLoadLocal(2*8, REG_RCX) // val
	g.storeMem(REG_RAX, 0, REG_RCX)
}

func (g *CodeGen) CompileWriteByteIntrinsic() {
	// Param 0 = addr, Param 1 = val. Write 1 byte.
	g.EmitLoadLocal(1*8, REG_RAX) // addr
	g.EmitLoadLocal(2*8, REG_RCX) // val
	g.EmitBytes(0x88, 0x08)       // mov [rax], cl
}

// === Interface dispatch ===

func (g *CodeGen) compileIfaceBox(inst ir.Inst) {
	// Stack: ... concreteValue
	// Pop concrete value, allocate 16 bytes, store {type_id, value}, push box pointer
	typeID := inst.Arg

	// Pop concrete value into rax
	g.OpPop(REG_RAX)
	g.PushR(REG_RAX) // save concrete value on x86 stack

	// Allocate 16 bytes: push 16, call runtime.Alloc
	g.CompileConstI64(16)
	g.EmitCallPlaceholder("runtime.Alloc")
	// Result (box ptr) is on operand stack
	g.OpPop(REG_RCX) // box ptr

	// Store type_id at [box+0]
	g.EmitByte(0xb8) // mov eax, imm32
	g.EmitU32(uint32(typeID))
	// Emit stores directly to avoid depending on generic store helper
	// argument lowering in self-hosted compilers.
	g.EmitBytes(rexRR(REG_RAX, REG_RCX), 0x89, 0x01) // mov [rcx], rax

	// Restore concrete value and store at [box+8]
	g.PopR(REG_RAX)
	g.EmitBytes(rexRR(REG_RAX, REG_RCX), 0x89, 0x41, 0x08) // mov [rcx+8], rax

	// Push box pointer as result
	g.OpPush(REG_RCX)
}

func (g *CodeGen) compileIfaceCall(inst ir.Inst) {
	// Stack: ... ifacePtr arg0 arg1 ...
	// inst.Arg = number of regular args (excluding receiver)
	// inst.Name = "ifaceType.Method" e.g. "error.Error"
	argCount := inst.Arg
	methodName := inst.Name

	// Save regular args from operand stack to call stack (in reverse)
	i := 0
	for i < argCount {
		g.OpPop(REG_RAX)
		g.PushR(REG_RAX)
		i++
	}

	// Pop interface pointer into rax
	g.OpPop(REG_RAX)

	// Load type_id from [rax+0] into rcx, concrete value from [rax+8] into rdx
	g.LoadMem(REG_RCX, REG_RAX, 0)
	g.LoadMem(REG_RDX, REG_RAX, 8)

	// Push receiver once and materialize it before branch dispatch.
	g.OpPush(REG_RDX)

	// Restore regular args from call stack (in correct order)
	i = argCount - 1
	for i >= 0 {
		g.PopR(REG_RAX)
		g.OpPush(REG_RAX)
		i = i - 1
	}
	// Ensure restored args are materialized for all dispatch branches.
	g.Flush()

	// Save rcx (type_id) on call stack since the call may clobber it
	g.PushR(REG_RCX)

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

	// Generate if/else chain over type IDs → concrete method calls
	// Collect all type IDs that implement this interface method
	var entries []DispatchEntry

	if g.irmod != nil && g.irmod.TypeIDs != nil {
		for typeName, tid := range g.irmod.TypeIDs {
			// Check if typeName.Method exists in methodTable
			candidate := typeName + "." + bareMethod
			if fnName, ok := becommon.LookupStringMapLinear(g.irmod.MethodTable, candidate); ok {
				entries = append(entries, DispatchEntry{tid, fnName})
			}
		}
	}
	sortDispatchEntries(entries)

	// Restore type_id from call stack
	g.PopR(REG_RCX)

	if len(entries) == 0 {
		// No known implementors — trap
		g.int3()
	} else {
		// For each entry: cmp rcx, typeID; jne next; call method; jmp end; next:
		endFixups := make([]int, 0)
		for _, entry := range entries {
			// cmp rcx, typeID
			if entry.typeID <= 127 {
				g.cmpRI(REG_RCX, int32(entry.typeID))
			} else {
				g.EmitBytes(0x48, 0x81, 0xf9) // cmp rcx, imm32
				g.EmitU32(uint32(entry.typeID))
			}
			// jne next (rel32)
			nextFixup := g.JccRel32(CC_NE)

			// Call the concrete method (receiver/args already on operand stack).
			g.EmitCallPlaceholder(entry.funcName)

			// jmp end
			endFixups = append(endFixups, g.JmpRel32())

			// next:
			g.PatchRel32(nextFixup)
		}
		// Default: trap
		g.int3()

		// Patch all end jumps
		endAddr := len(g.Code)
		for _, fixup := range endFixups {
			g.PatchRel32At(fixup, endAddr)
		}
	}
}

// === Memory operations ===

func (g *CodeGen) compileLoad(inst ir.Inst) {
	size := inst.Arg
	offset := int(inst.Val)
	g.OpPop(REG_RCX)
	if offset != 0 && !ir.IsNonNilMemoryBase(inst.Name) {
		// Preserve IR semantics: nil-guarded LOAD checks the effective address
		// after OP_OFFSET, not the original base pointer.
		g.AddRI(REG_RCX, int32(offset))
		offset = 0
	}
	if ir.IsNonNilMemoryBase(inst.Name) {
		if size == 1 {
			g.loadMemByte(REG_RAX, REG_RCX, offset) // movzx rax, byte [rcx+off]
		} else {
			g.LoadMem(REG_RAX, REG_RCX, offset) // mov rax, [rcx+off]
		}
		g.OpPush(REG_RAX)
		return
	}
	g.TestRR(REG_RCX, REG_RCX)
	if size == 1 {
		g.EmitBytes(0x75, 0x05) // jnz +5 (skip zero case)
		g.XorRR(REG_RAX, REG_RAX)
		g.EmitBytes(0xeb, 0x04) // jmp +4 (skip load)
		g.loadMemByte(REG_RAX, REG_RCX, offset)
	} else {
		g.EmitBytes(0x75, 0x05) // jnz +5 (skip zero case)
		g.XorRR(REG_RAX, REG_RAX)
		g.EmitBytes(0xeb, 0x03) // jmp +3 (skip load)
		g.LoadMem(REG_RAX, REG_RCX, offset)
	}
	g.OpPush(REG_RAX)
}

func (g *CodeGen) compileStore(inst ir.Inst) {
	size := inst.Arg
	offset := int(inst.Val)
	// stack: ... value addr  → pop addr into rcx, pop value into rax, store
	g.OpPop(REG_RCX) // addr
	g.OpPop(REG_RAX) // value
	if size == 1 {
		g.storeMemByte(REG_RCX, offset, REG_RAX)
	} else {
		g.storeMem(REG_RCX, offset, REG_RAX)
	}
}

func (g *CodeGen) compileOffset(inst ir.Inst) {
	g.OpPop(REG_RAX)
	if inst.Arg != 0 {
		g.AddRI(REG_RAX, int32(inst.Arg))
	}
	g.OpPush(REG_RAX)
}

func (g *CodeGen) compileIndexAddr(elemSize int) {
	// pop index, pop slice-header-ptr, compute data_ptr + index * elemSize, push
	g.OpPop(REG_RAX) // index
	g.OpPop(REG_RCX) // slice header ptr

	// Load data_ptr from header: [rcx+0]
	g.LoadMem(REG_RCX, REG_RCX, 0)

	// Compute address: data_ptr + index * elemSize
	if elemSize == 1 {
		g.AddRR(REG_RCX, REG_RAX)
	} else if elemSize == 8 {
		g.shlImm(REG_RAX, 3)
		g.AddRR(REG_RCX, REG_RAX)
	} else {
		g.imulRRI32(REG_RAX, REG_RAX, int32(elemSize))
		g.AddRR(REG_RCX, REG_RAX)
	}

	g.OpPush(REG_RCX)
}

func (g *CodeGen) compileLen(inst ir.Inst) {
	g.OpPop(REG_RAX)
	if ir.IsNonNilMemoryBase(inst.Name) {
		g.LoadMem(REG_RAX, REG_RAX, 8)
		g.OpPush(REG_RAX)
		return
	}
	g.TestRR(REG_RAX, REG_RAX)
	g.EmitBytes(0x75, 0x05)   // jnz +5 (skip zero case)
	g.XorRR(REG_RAX, REG_RAX) // 3 bytes
	g.EmitBytes(0xeb, 0x04)   // jmp +4 (skip load) 2 bytes
	g.LoadMem(REG_RAX, REG_RAX, 8)
	g.OpPush(REG_RAX)
}

func (g *CodeGen) compileCap(inst ir.Inst) {
	g.OpPop(REG_RAX)
	if ir.IsNonNilMemoryBase(inst.Name) {
		g.LoadMem(REG_RAX, REG_RAX, 16) // cap at offset 16 (2*8)
		g.OpPush(REG_RAX)
		return
	}
	g.TestRR(REG_RAX, REG_RAX)
	g.EmitBytes(0x75, 0x05)         // jnz +5 (skip zero case)
	g.XorRR(REG_RAX, REG_RAX)       // 3 bytes
	g.EmitBytes(0xeb, 0x04)         // jmp +4 (skip load) 2 bytes
	g.LoadMem(REG_RAX, REG_RAX, 16) // cap at offset 16 (2*8)
	g.OpPush(REG_RAX)
}

// === Type conversions ===

func (g *CodeGen) compileConvert(typeName string) {
	// Most conversions are no-ops (all values are 8 bytes)
	// string([]byte) and []byte(string) need runtime calls
	switch typeName {
	case "string":
		// []byte→string: call runtime.BytesToString
		g.EmitCallPlaceholder("runtime.BytesToString")
	case "[]byte":
		// string→[]byte: call runtime.StringToBytes
		g.EmitCallPlaceholder("runtime.StringToBytes")
	case "int", "uintptr", "uint", "int64", "uint64":
		// No-op: all 8-byte integers
	case "byte":
		g.OpPop(REG_RAX)
		g.movzxB(REG_RAX)
		g.OpPush(REG_RAX)
	case "uint8":
		g.OpPop(REG_RAX)
		g.movzxB(REG_RAX)
		g.OpPush(REG_RAX)
	case "int8":
		g.OpPop(REG_RAX)
		g.movsxB(REG_RAX)
		g.OpPush(REG_RAX)
	case "uint16":
		g.OpPop(REG_RAX)
		g.movzxW(REG_RAX)
		g.OpPush(REG_RAX)
	case "int16":
		g.OpPop(REG_RAX)
		g.movsxW(REG_RAX)
		g.OpPush(REG_RAX)
	case "int32":
		g.OpPop(REG_RAX)
		g.movsxD(REG_RAX)
		g.OpPush(REG_RAX)
	case "uint32":
		g.OpPop(REG_RAX)
		g.clearHi32(REG_RAX)
		g.OpPush(REG_RAX)
	}
}
