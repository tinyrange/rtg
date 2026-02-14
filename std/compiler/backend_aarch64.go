//go:build !no_backend_darwin_arm64

package main

// === ARM64 Code Generation ===
// Mirrors backend_x64.go but emits ARM64 instructions.
// Uses X0-X3 as working registers, X28 as operand stack pointer,
// X29 (FP) as frame pointer, X30 (LR) as link register.

// compileFuncArm64 generates ARM64 code for a single IR function.
func (g *CodeGen) compileFuncArm64(f *IRFunc) {
	g.curFunc = f
	g.hasPending = false
	g.curFrameSize = len(f.Locals)
	if f.Params > g.curFrameSize {
		g.curFrameSize = f.Params
	}
	g.labelOffsets = make(map[int]int)
	g.jumpFixups = nil

	// Prologue: STP X29, X30, [SP, #-16]!; MOV X29, SP; SUB SP, SP, #frameBytes
	g.emitStp(REG_FP, REG_LR, REG_SP, -16)
	g.emitMovRRArm64(REG_FP, REG_SP)

	frameBytes := g.curFrameSize * 8
	// Align to 16 bytes (ARM64 SP must be 16-byte aligned)
	if frameBytes%16 != 0 {
		frameBytes = frameBytes + (16 - frameBytes%16)
	}
	if frameBytes > 0 {
		if frameBytes < 4096 {
			g.emitSubImm(REG_SP, REG_SP, uint32(frameBytes))
		} else {
			g.emitLoadImm64Compact(REG_X16, uint64(frameBytes))
			g.emitSubRR(REG_SP, REG_SP, REG_X16)
		}
	}

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

	// Compile instructions
	for _, inst := range f.Code {
		g.compileInstArm64(inst)
	}

	// Resolve jump fixups within this function
	for _, fix := range g.jumpFixups {
		labelOff, ok := g.labelOffsets[fix.LabelID]
		if !ok {
			continue
		}
		// Determine if this is a B.cond or B instruction
		existing := getU32(g.code[fix.CodeOffset : fix.CodeOffset+4])
		if existing&0xFF000010 == 0x54000000 {
			// B.cond
			g.patchArm64BCondAt(fix.CodeOffset, labelOff)
		} else {
			// B
			g.patchArm64BAt(fix.CodeOffset, labelOff)
		}
	}

	g.curFunc = nil
}

// compileInstArm64 generates ARM64 code for a single IR instruction.
func (g *CodeGen) compileInstArm64(inst Inst) {
	switch inst.Op {
	case OP_CONST_I64:
		g.compileConstI64Arm64(inst.Val)
	case OP_CONST_BOOL:
		if inst.Arg != 0 {
			g.compileConstI64Arm64(1)
		} else {
			g.compileConstI64Arm64(0)
		}
	case OP_CONST_NIL:
		g.compileConstI64Arm64(0)
	case OP_CONST_STR:
		g.compileConstStrArm64(inst.Name)

	case OP_LOCAL_GET:
		g.compileLocalGetArm64(inst.Arg)
	case OP_LOCAL_SET:
		g.compileLocalSetArm64(inst.Arg)
	case OP_LOCAL_ADDR:
		g.compileLocalAddrArm64(inst.Arg)

	case OP_GLOBAL_GET:
		g.compileGlobalGetArm64(inst)
	case OP_GLOBAL_SET:
		g.compileGlobalSetArm64(inst)
	case OP_GLOBAL_ADDR:
		g.compileGlobalAddrArm64(inst)

	case OP_DROP:
		g.opDrop()
	case OP_DUP:
		g.opLoad(REG_X0)
		g.opPush(REG_X0)

	case OP_ADD, OP_SUB, OP_MUL, OP_DIV, OP_MOD:
		g.compileBinOpArm64(inst.Op)
	case OP_NEG:
		g.opPop(REG_X0)
		g.emitNeg(REG_X0, REG_X0)
		g.opPush(REG_X0)

	case OP_AND, OP_OR, OP_XOR, OP_SHL, OP_SHR:
		g.compileBinOpArm64(inst.Op)

	case OP_EQ:
		g.compileCompareArm64(COND_EQ)
	case OP_NEQ:
		g.compileCompareArm64(COND_NE)
	case OP_LT:
		g.compileCompareArm64(COND_LT)
	case OP_GT:
		g.compileCompareArm64(COND_GT)
	case OP_LEQ:
		g.compileCompareArm64(COND_LE)
	case OP_GEQ:
		g.compileCompareArm64(COND_GE)

	case OP_NOT:
		g.opPop(REG_X0)
		g.emitEorImm1(REG_X0, REG_X0) // XOR with 1
		g.opPush(REG_X0)

	case OP_LABEL:
		g.flush()
		g.labelOffsets[inst.Arg] = len(g.code)
	case OP_JMP:
		g.flush()
		fixup := g.emitB()
		g.jumpFixups = append(g.jumpFixups, JumpFixup{
			CodeOffset: fixup,
			LabelID:    inst.Arg,
		})
	case OP_JMP_IF:
		g.opPop(REG_X0)
		g.emitCmpImm(REG_X0, 0)
		fixup := g.emitBCond(COND_NE)
		g.jumpFixups = append(g.jumpFixups, JumpFixup{
			CodeOffset: fixup,
			LabelID:    inst.Arg,
		})
	case OP_JMP_IF_NOT:
		g.opPop(REG_X0)
		g.emitCmpImm(REG_X0, 0)
		fixup := g.emitBCond(COND_EQ)
		g.jumpFixups = append(g.jumpFixups, JumpFixup{
			CodeOffset: fixup,
			LabelID:    inst.Arg,
		})

	case OP_CALL:
		g.compileCallArm64(inst)
	case OP_CALL_INTRINSIC:
		g.compileCallIntrinsicArm64(inst)
	case OP_RETURN:
		g.compileReturnArm64(inst)

	case OP_LOAD:
		g.compileLoadArm64(inst.Arg)
	case OP_STORE:
		g.compileStoreArm64(inst.Arg)
	case OP_OFFSET:
		g.compileOffsetArm64(inst)
	case OP_INDEX_ADDR:
		g.compileIndexAddrArm64(inst.Arg)
	case OP_LEN:
		g.compileLenArm64()

	case OP_CONVERT:
		g.compileConvertArm64(inst.Name)

	case OP_IFACE_BOX:
		g.compileIfaceBoxArm64(inst)
	case OP_IFACE_CALL:
		g.compileIfaceCallArm64(inst)
	case OP_PANIC:
		g.compilePanicArm64()

	case OP_SLICE_GET, OP_SLICE_MAKE, OP_STRING_GET, OP_STRING_MAKE:
		// Handled by intrinsics

	default:
		panic("ICE: unhandled opcode in compileInstArm64")
	}
}

// === Constant loading ===

func (g *CodeGen) compileConstI64Arm64(val int64) {
	g.flush()
	g.emitLoadImm64Compact(REG_X0, uint64(val))
	g.opPush(REG_X0)
}

func (g *CodeGen) compileConstStrArm64(s string) {
	g.flush()
	decoded := decodeStringLiteral(s)

	headerOff, ok := g.stringMap[decoded]
	var rodataOff int
	if !ok {
		// String bytes go into rodata (read-only __TEXT,__const)
		rodataOff = len(g.rodata)
		g.rodata = append(g.rodata, []byte(decoded)...)

		// String header goes into data (writable __DATA)
		headerOff = len(g.data)
		// data_ptr: leave as 0 (will be computed at runtime via ADRP+ADD)
		g.data = append(g.data, 0, 0, 0, 0, 0, 0, 0, 0)
		// length
		lenBytes := make([]byte, 8)
		putU64(lenBytes, uint64(len(decoded)))
		g.data = append(g.data, lenBytes...)

		g.stringMap[decoded] = headerOff
		if g.stringRodataMap == nil {
			g.stringRodataMap = make(map[int]int)
		}
		g.stringRodataMap[headerOff] = rodataOff
	} else {
		rodataOff = g.stringRodataMap[headerOff]
	}

	// Compute string data address at runtime (PC-relative ADRP+ADD, works with ASLR)
	// and store it into the header's data_ptr field in __DATA (writable)
	g.emitAdrpAdd(REG_X1, "$rodata_header$", uint64(rodataOff)) // X1 = actual string data addr
	g.emitAdrpAdd(REG_X0, "$data_addr$", uint64(headerOff))     // X0 = header addr in __DATA
	g.emitStr(REG_X1, REG_X0, 0)                                 // [header+0] = data addr

	// Push header address onto operand stack
	g.opPush(REG_X0)
}

// === Local variable access ===

func (g *CodeGen) compileLocalGetArm64(idx int) {
	g.flush()
	offset := (idx + 1) * 8
	g.emitLoadLocalArm64(offset, REG_X0)
	g.opPush(REG_X0)
}

func (g *CodeGen) compileLocalSetArm64(idx int) {
	g.opPop(REG_X0)
	offset := (idx + 1) * 8
	g.emitStoreLocalArm64(offset, REG_X0)
}

func (g *CodeGen) compileLocalAddrArm64(idx int) {
	g.flush()
	offset := (idx + 1) * 8
	g.emitLeaLocalArm64(offset, REG_X0)
	g.opPush(REG_X0)
}

// === Global variable access ===

func (g *CodeGen) compileGlobalGetArm64(inst Inst) {
	g.flush()
	g.emitAdrpLdr(REG_X0, "$data_addr$", uint64(inst.Arg*8))
	g.opPush(REG_X0)
}

func (g *CodeGen) compileGlobalSetArm64(inst Inst) {
	g.opPop(REG_X0)
	g.emitAdrpAdd(REG_X1, "$data_addr$", uint64(inst.Arg*8))
	g.emitStr(REG_X0, REG_X1, 0)
}

func (g *CodeGen) compileGlobalAddrArm64(inst Inst) {
	g.flush()
	g.emitAdrpAdd(REG_X0, "$data_addr$", uint64(inst.Arg*8))
	g.opPush(REG_X0)
}

// === Binary operations ===

func (g *CodeGen) compileBinOpArm64(op Opcode) {
	g.opPop(REG_X0) // second (top)
	g.opPop(REG_X1) // first (below)

	switch op {
	case OP_ADD:
		g.emitAddRR(REG_X1, REG_X1, REG_X0)
	case OP_SUB:
		g.emitSubRR(REG_X1, REG_X1, REG_X0)
	case OP_MUL:
		g.emitMul(REG_X1, REG_X1, REG_X0)
	case OP_DIV:
		g.emitSdiv(REG_X1, REG_X1, REG_X0)
	case OP_MOD:
		// mod = a - (a/b)*b → SDIV + MSUB
		g.emitSdiv(REG_X2, REG_X1, REG_X0) // X2 = X1 / X0
		g.emitMsub(REG_X1, REG_X2, REG_X0, REG_X1) // X1 = X1 - X2*X0
	case OP_AND:
		g.emitAndRR(REG_X1, REG_X1, REG_X0)
	case OP_OR:
		g.emitOrrRR(REG_X1, REG_X1, REG_X0)
	case OP_XOR:
		g.emitEorRR(REG_X1, REG_X1, REG_X0)
	case OP_SHL:
		g.emitLslRR(REG_X1, REG_X1, REG_X0)
	case OP_SHR:
		g.emitAsrRR(REG_X1, REG_X1, REG_X0)
	}

	g.opPush(REG_X1)
}

// === Comparison operations ===

func (g *CodeGen) compileCompareArm64(cond int) {
	g.opPop(REG_X0) // second
	g.opPop(REG_X1) // first
	g.emitCmpRR(REG_X1, REG_X0)
	g.emitCset(REG_X1, cond)
	g.opPush(REG_X1)
}

// === Function calls ===

func (g *CodeGen) compileCallArm64(inst Inst) {
	if len(inst.Name) > 18 && inst.Name[0:18] == "builtin.composite." {
		g.compileCompositeLitCallArm64(inst)
		return
	}
	g.emitCallPlaceholderArm64(inst.Name)
}

func (g *CodeGen) compileCompositeLitCallArm64(inst Inst) {
	fieldCount := inst.Arg
	structSize := fieldCount * 8

	if structSize == 0 {
		g.compileConstI64Arm64(0)
		return
	}

	// Save field values from operand stack to hardware stack
	i := 0
	for i < fieldCount {
		g.opPop(REG_X0)
		// STP-style push: SUB SP, SP, #16; STR X0, [SP]
		g.emitSubImm(REG_SP, REG_SP, 16)
		g.emitStr(REG_X0, REG_SP, 0)
		i++
	}

	// Allocate struct: push size, call Alloc
	g.compileConstI64Arm64(int64(structSize))
	g.emitCallPlaceholderArm64("runtime.Alloc")
	g.opPop(REG_X1) // struct ptr

	// Pop fields from hardware stack and store into struct
	i = 0
	for i < fieldCount {
		g.emitLdr(REG_X0, REG_SP, 0)
		g.emitAddImm(REG_SP, REG_SP, 16)
		offset := i * 8
		g.emitStr(REG_X0, REG_X1, offset)
		i++
	}

	g.opPush(REG_X1)
}

func (g *CodeGen) compileReturnArm64(inst Inst) {
	g.flush()
	// Epilogue: MOV SP, FP; LDP FP, LR, [SP], #16; RET
	g.emitMovRRArm64(REG_SP, REG_FP)
	g.emitLdp(REG_FP, REG_LR, REG_SP, 16)
	g.emitRet()
}

// === Intrinsics ===

func (g *CodeGen) compileCallIntrinsicArm64(inst Inst) {
	g.flush()
	switch inst.Name {
	case "SysRead":
		g.emitLoadLocalArm64(1*8, REG_X0) // fd
		g.emitLoadLocalArm64(2*8, REG_X1) // buf
		g.emitLoadLocalArm64(3*8, REG_X2) // count
		g.emitCallGOT("_read")
		g.emitSyscallReturnArm64()
	case "SysWrite":
		g.emitLoadLocalArm64(1*8, REG_X0) // fd
		g.emitLoadLocalArm64(2*8, REG_X1) // buf
		g.emitLoadLocalArm64(3*8, REG_X2) // count
		g.emitCallGOT("_write")
		g.emitSyscallReturnArm64()
	case "SysOpen":
		g.emitLoadLocalArm64(1*8, REG_X0) // path
		g.emitLoadLocalArm64(2*8, REG_X1) // flags
		g.emitLoadLocalArm64(3*8, REG_X2) // mode
		g.emitCallGOT("_open")
		g.emitSyscallReturnArm64()
	case "SysClose":
		g.emitLoadLocalArm64(1*8, REG_X0) // fd
		g.emitCallGOT("_close")
		g.emitSyscallReturnArm64()
	case "SysStat":
		g.emitLoadLocalArm64(1*8, REG_X0) // path
		g.emitLoadLocalArm64(2*8, REG_X1) // buf
		g.emitCallGOT("_stat")
		g.emitSyscallReturnArm64()
	case "SysMkdir":
		g.emitLoadLocalArm64(1*8, REG_X0) // path
		g.emitLoadLocalArm64(2*8, REG_X1) // mode
		g.emitCallGOT("_mkdir")
		g.emitSyscallReturnArm64()
	case "SysRmdir":
		g.emitLoadLocalArm64(1*8, REG_X0) // path
		g.emitCallGOT("_rmdir")
		g.emitSyscallReturnArm64()
	case "SysUnlink":
		g.emitLoadLocalArm64(1*8, REG_X0) // path
		g.emitCallGOT("_unlink")
		g.emitSyscallReturnArm64()
	case "SysGetcwd":
		g.emitLoadLocalArm64(1*8, REG_X0) // buf
		g.emitLoadLocalArm64(2*8, REG_X1) // size
		g.emitCallGOT("_getcwd")
		g.emitSyscallReturnPtrArm64()
	case "SysExit":
		g.emitLoadLocalArm64(1*8, REG_X0) // code
		g.emitCallGOT("_exit")
	case "SysMmap":
		g.emitLoadLocalArm64(1*8, REG_X0) // addr
		g.emitLoadLocalArm64(2*8, REG_X1) // len
		g.emitLoadLocalArm64(3*8, REG_X2) // prot
		g.emitLoadLocalArm64(4*8, REG_X3) // flags
		g.emitLoadLocalArm64(5*8, REG_X4) // fd
		g.emitLoadLocalArm64(6*8, REG_X5) // offset
		g.emitCallGOT("_mmap")
		g.emitSyscallReturnPtrArm64()
	case "SysOpendir":
		g.emitLoadLocalArm64(1*8, REG_X0) // path
		g.emitCallGOT("_opendir")
		g.emitSyscallReturnPtrArm64()
	case "SysReaddir":
		g.emitLoadLocalArm64(1*8, REG_X0) // dirp
		g.emitCallGOT("_readdir")
		g.rawPush(REG_X0) // r1 = dirent* or 0
		g.emitMovZ(REG_X0, 0, 0)
		g.rawPush(REG_X0) // r2=0
		g.rawPush(REG_X0) // err=0
		g.hasPending = false
	case "SysClosedir":
		g.emitLoadLocalArm64(1*8, REG_X0) // dirp
		g.emitCallGOT("_closedir")
		g.emitSyscallReturnArm64()
	case "SysDup2":
		g.emitLoadLocalArm64(1*8, REG_X0) // oldfd
		g.emitLoadLocalArm64(2*8, REG_X1) // newfd
		g.emitCallGOT("_dup2")
		g.emitSyscallReturnArm64()
	case "SysFork":
		g.emitCallGOT("_fork")
		g.emitSyscallReturnArm64()
	case "SysExecve":
		g.emitLoadLocalArm64(1*8, REG_X0) // path
		g.emitLoadLocalArm64(2*8, REG_X1) // argv
		g.emitLoadLocalArm64(3*8, REG_X2) // envp
		g.emitCallGOT("_execve")
		g.emitSyscallReturnArm64()
	case "SysWait4":
		g.emitLoadLocalArm64(1*8, REG_X0) // pid
		g.emitLoadLocalArm64(2*8, REG_X1) // status
		g.emitLoadLocalArm64(3*8, REG_X2) // options
		g.emitLoadLocalArm64(4*8, REG_X3) // rusage
		g.emitCallGOT("_wait4")
		g.emitSyscallReturnArm64()
	case "SysPipe":
		g.emitLoadLocalArm64(1*8, REG_X0) // fds
		g.emitCallGOT("_pipe")
		g.emitSyscallReturnArm64()
	case "SysChmod":
		g.emitLoadLocalArm64(1*8, REG_X0) // path
		g.emitLoadLocalArm64(2*8, REG_X1) // mode
		g.emitCallGOT("_chmod")
		g.emitSyscallReturnArm64()
	case "SysGetargc":
		argcOff := len(g.irmod.Globals) * 8
		g.emitAdrpLdr(REG_X0, "$data_addr$", uint64(argcOff))
		g.rawPush(REG_X0) // r1=argc
		g.emitMovZ(REG_X0, 0, 0)
		g.rawPush(REG_X0) // r2=0
		g.rawPush(REG_X0) // err=0
		g.hasPending = false
	case "SysGetargv":
		argvOff := (len(g.irmod.Globals) + 1) * 8
		g.emitAdrpLdr(REG_X0, "$data_addr$", uint64(argvOff))
		g.rawPush(REG_X0) // r1=argv
		g.emitMovZ(REG_X0, 0, 0)
		g.rawPush(REG_X0) // r2=0
		g.rawPush(REG_X0) // err=0
		g.hasPending = false
	case "SysGetenvp":
		envpOff := (len(g.irmod.Globals) + 2) * 8
		g.emitAdrpLdr(REG_X0, "$data_addr$", uint64(envpOff))
		g.rawPush(REG_X0) // r1=envp
		g.emitMovZ(REG_X0, 0, 0)
		g.rawPush(REG_X0) // r2=0
		g.rawPush(REG_X0) // err=0
		g.hasPending = false
	case "SysGetpid":
		g.emitCallGOT("_getpid")
		g.emitSyscallReturnArm64()
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

func (g *CodeGen) compileSliceptrIntrinsicArm64() {
	g.emitLoadLocalArm64(1*8, REG_X0) // slice header ptr
	g.emitLdr(REG_X0, REG_X0, 0)       // [header+0] = data ptr
	g.opPush(REG_X0)
}

func (g *CodeGen) compileMakesliceIntrinsicArm64() {
	// Params: ptr (local 0), len (local 1), cap (local 2)
	g.compileConstI64Arm64(32)
	g.emitCallPlaceholderArm64("runtime.Alloc")
	g.opPop(REG_X1) // header ptr

	g.emitLoadLocalArm64(1*8, REG_X0) // ptr
	g.emitStr(REG_X0, REG_X1, 0)
	g.emitLoadLocalArm64(2*8, REG_X0) // len
	g.emitStr(REG_X0, REG_X1, 8)
	g.emitLoadLocalArm64(3*8, REG_X0) // cap
	g.emitStr(REG_X0, REG_X1, 16)
	g.emitLoadImm64Compact(REG_X0, 1) // elem_size = 1
	g.emitStr(REG_X0, REG_X1, 24)

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
	g.emitCallPlaceholderArm64("runtime.Alloc")
	g.opPop(REG_X1) // header ptr

	g.emitLoadLocalArm64(1*8, REG_X0) // ptr
	g.emitStr(REG_X0, REG_X1, 0)
	g.emitLoadLocalArm64(2*8, REG_X0) // len
	g.emitStr(REG_X0, REG_X1, 8)

	g.opPush(REG_X1)
}

func (g *CodeGen) compileTostringIntrinsicArm64() {
	g.emitLoadLocalArm64(1*8, REG_X0) // load value

	// Test: is [rax] < 256 → interface box
	g.emitLdr(REG_X1, REG_X0, 0)
	g.emitCmpImm(REG_X1, 256)
	stringCaseFixup := g.emitBCond(COND_CS) // branch if unsigned >= 256

	// Interface case: X1 = type_id, [X0+8] = concrete value
	g.emitLdr(REG_X2, REG_X0, 8) // concrete value
	g.opPush(REG_X2)
	g.flush() // Must flush before dispatch chain — otherwise the pending push
	// only materializes inside the type_id=1 branch (via emitCallPlaceholder's flush)
	// and other branches (type_id=2 string passthrough) never see it.

	// Save type_id on hardware stack
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X1, REG_SP, 0)

	// Dispatch chain for Error/String
	var entries []dispatchEntry
	if g.irmod != nil && g.irmod.TypeIDs != nil {
		for typeName, tid := range g.irmod.TypeIDs {
			candidate := typeName + ".Error"
			if _, ok := g.irmod.MethodTable[candidate]; ok {
				entries = append(entries, dispatchEntry{tid, candidate})
				continue
			}
			candidate = typeName + ".String"
			if _, ok := g.irmod.MethodTable[candidate]; ok {
				entries = append(entries, dispatchEntry{tid, candidate})
			}
		}
	}

	// Restore type_id
	g.emitLdr(REG_X1, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)

	endFixups := make([]int, 0)

	// type_id 1 = int
	g.emitCmpImm(REG_X1, 1)
	nextFixup := g.emitBCond(COND_NE)
	g.emitCallPlaceholderArm64("runtime.IntToString")
	endFixups = append(endFixups, g.emitB())
	g.patchArm64BCondAt(nextFixup, len(g.code))

	// type_id 2 = string
	g.emitCmpImm(REG_X1, 2)
	nextFixup = g.emitBCond(COND_NE)
	endFixups = append(endFixups, g.emitB())
	g.patchArm64BCondAt(nextFixup, len(g.code))

	// User types
	for _, entry := range entries {
		g.emitCmpImm(REG_X1, uint32(entry.typeID))
		nextFixup = g.emitBCond(COND_NE)
		g.emitCallPlaceholderArm64(entry.funcName)
		endFixups = append(endFixups, g.emitB())
		g.patchArm64BCondAt(nextFixup, len(g.code))
	}

	// Default: drop receiver, push 0
	g.opDrop()
	g.compileConstI64Arm64(0)
	g.flush()

	endAddr := len(g.code)
	for _, fixup := range endFixups {
		g.patchArm64BAt(fixup, endAddr)
	}

	finalEndFixup := g.emitB()

	// string_case: pass through
	g.patchArm64BCondAt(stringCaseFixup, len(g.code))
	g.emitLoadLocalArm64(1*8, REG_X0)
	g.opPush(REG_X0)
	g.flush()

	g.patchArm64BAt(finalEndFixup, len(g.code))
}

func (g *CodeGen) compileReadPtrIntrinsicArm64() {
	g.emitLoadLocalArm64(1*8, REG_X0) // addr
	g.emitLdr(REG_X0, REG_X0, 0)       // read 8 bytes
	g.opPush(REG_X0)
}

func (g *CodeGen) compileWritePtrIntrinsicArm64() {
	g.emitLoadLocalArm64(1*8, REG_X0) // addr
	g.emitLoadLocalArm64(2*8, REG_X1) // val
	g.emitStr(REG_X1, REG_X0, 0)
}

func (g *CodeGen) compileWriteByteIntrinsicArm64() {
	g.emitLoadLocalArm64(1*8, REG_X0) // addr
	g.emitLoadLocalArm64(2*8, REG_X1) // val
	g.emitStrb(REG_X1, REG_X0, 0)
}

// === Interface dispatch ===

func (g *CodeGen) compileIfaceBoxArm64(inst Inst) {
	typeID := inst.Arg

	g.opPop(REG_X0) // concrete value
	// Save on hardware stack
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X0, REG_SP, 0)

	// Allocate 16 bytes
	g.compileConstI64Arm64(16)
	g.emitCallPlaceholderArm64("runtime.Alloc")
	g.opPop(REG_X1) // box ptr

	// Store type_id
	g.emitLoadImm64Compact(REG_X0, uint64(typeID))
	g.emitStr(REG_X0, REG_X1, 0)

	// Restore concrete value and store
	g.emitLdr(REG_X0, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X0, REG_X1, 8)

	g.opPush(REG_X1)
}

func (g *CodeGen) compileIfaceCallArm64(inst Inst) {
	argCount := inst.Arg
	methodName := inst.Name

	// Save regular args to hardware stack
	i := 0
	for i < argCount {
		g.opPop(REG_X0)
		g.emitSubImm(REG_SP, REG_SP, 16)
		g.emitStr(REG_X0, REG_SP, 0)
		i++
	}

	// Pop interface pointer
	g.opPop(REG_X0)

	// Load type_id and concrete value
	g.emitLdr(REG_X1, REG_X0, 0) // type_id
	g.emitLdr(REG_X2, REG_X0, 8) // concrete value

	// Push concrete value as receiver
	g.opPush(REG_X2)

	// Restore regular args
	i = argCount - 1
	for i >= 0 {
		g.flush()
		g.emitLdr(REG_X0, REG_SP, 0)
		g.emitAddImm(REG_SP, REG_SP, 16)
		g.opPush(REG_X0)
		i = i - 1
	}

	// Save type_id on hardware stack
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X1, REG_SP, 0)

	// Extract method name
	dotIdx := 0
	for dotIdx < len(methodName) {
		if methodName[dotIdx] == '.' {
			break
		}
		dotIdx++
	}
	bareMethod := methodName
	if dotIdx < len(methodName) {
		bareMethod = methodName[dotIdx+1:]
	}

	// Collect dispatch entries
	var entries []dispatchEntry
	if g.irmod != nil && g.irmod.TypeIDs != nil {
		for typeName, tid := range g.irmod.TypeIDs {
			candidate := typeName + "." + bareMethod
			if _, ok := g.irmod.MethodTable[candidate]; ok {
				entries = append(entries, dispatchEntry{tid, candidate})
			}
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
			g.emitCmpImm(REG_X1, uint32(entry.typeID))
			nextFixup := g.emitBCond(COND_NE)
			g.emitCallPlaceholderArm64(entry.funcName)
			endFixups = append(endFixups, g.emitB())
			g.patchArm64BCondAt(nextFixup, len(g.code))
		}
		g.emitBrk() // default: trap

		endAddr := len(g.code)
		for _, fixup := range endFixups {
			g.patchArm64BAt(fixup, endAddr)
		}
	}
}

// === Memory operations ===

func (g *CodeGen) compileLoadArm64(size int) {
	g.opPop(REG_X1) // addr
	g.emitCmpImm(REG_X1, 0)
	loadFixup := g.emitBCond(COND_NE) // branch to load if non-nil
	// nil case: X0 = 0
	g.emitMovZ(REG_X0, 0, 0)
	doneFixup := g.emitB()
	// load case:
	g.patchArm64BCondAt(loadFixup, len(g.code))
	if size == 1 {
		g.emitLdrb(REG_X0, REG_X1, 0)
	} else {
		g.emitLdr(REG_X0, REG_X1, 0)
	}
	g.patchArm64BAt(doneFixup, len(g.code))
	g.opPush(REG_X0)
}

func (g *CodeGen) compileStoreArm64(size int) {
	g.opPop(REG_X1) // addr
	g.opPop(REG_X0) // value
	if size == 1 {
		g.emitStrb(REG_X0, REG_X1, 0)
	} else {
		g.emitStr(REG_X0, REG_X1, 0)
	}
}

func (g *CodeGen) compileOffsetArm64(inst Inst) {
	g.opPop(REG_X0)
	if inst.Arg != 0 {
		if inst.Arg > 0 && inst.Arg < 4096 {
			g.emitAddImm(REG_X0, REG_X0, uint32(inst.Arg))
		} else {
			g.emitLoadImm64Compact(REG_X1, uint64(int64(inst.Arg)))
			g.emitAddRR(REG_X0, REG_X0, REG_X1)
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
		g.emitAddRR(REG_X1, REG_X1, REG_X0)
	} else if elemSize == 8 {
		g.emitLslImm(REG_X0, REG_X0, 3)
		g.emitAddRR(REG_X1, REG_X1, REG_X0)
	} else {
		g.emitLoadImm64Compact(REG_X2, uint64(elemSize))
		g.emitMul(REG_X0, REG_X0, REG_X2)
		g.emitAddRR(REG_X1, REG_X1, REG_X0)
	}

	g.opPush(REG_X1)
}

func (g *CodeGen) compileLenArm64() {
	g.opPop(REG_X0)
	g.emitCmpImm(REG_X0, 0)
	nonNilFixup := g.emitBCond(COND_NE)
	// nil: len = 0
	g.emitMovZ(REG_X0, 0, 0)
	doneFixup := g.emitB()
	g.patchArm64BCondAt(nonNilFixup, len(g.code))
	g.emitLdr(REG_X0, REG_X0, 8) // [header+8] = len
	g.patchArm64BAt(doneFixup, len(g.code))
	g.opPush(REG_X0)
}

// === Type conversions ===

func (g *CodeGen) compileConvertArm64(typeName string) {
	switch typeName {
	case "string":
		g.emitCallPlaceholderArm64("runtime.BytesToString")
	case "[]byte":
		g.emitCallPlaceholderArm64("runtime.StringToBytes")
	case "int", "uintptr", "uint", "int64", "uint64":
		// No-op
	case "byte":
		g.opPop(REG_X0)
		g.emitUxtb(REG_X0, REG_X0)
		g.opPush(REG_X0)
	case "uint16":
		g.opPop(REG_X0)
		g.emitUxth(REG_X0, REG_X0)
		g.opPush(REG_X0)
	case "int32":
		g.opPop(REG_X0)
		g.emitSxtw(REG_X0, REG_X0)
		g.opPush(REG_X0)
	case "uint32":
		g.opPop(REG_X0)
		g.emitUxtw(REG_X0, REG_X0)
		g.opPush(REG_X0)
	}
}
