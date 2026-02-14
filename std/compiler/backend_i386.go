//go:build !no_backend_linux_i386 || !no_backend_windows_i386

package main

import (
	"fmt"
	"os"
)

// generateI386ELF compiles an IRModule to an i386 (32-bit) ELF binary.
func generateI386ELF(irmod *IRModule, outputPath string) error {
	g := &CodeGen{
		funcOffsets:   make(map[string]int),
		labelOffsets:  make(map[int]int),
		stringMap:     make(map[string]int),
		globalOffsets: make([]int, len(irmod.Globals)),
		baseAddr:      0x08048000,
		irmod:         irmod,
		wordSize:      4,
	}

	// Allocate .data space for globals (4 bytes each)
	for i := range irmod.Globals {
		g.globalOffsets[i] = i * 4
	}
	g.data = make([]byte, len(irmod.Globals)*4)

	// Emit _start
	g.emitStart_i386(irmod)

	// First pass: compile all functions to get their offsets
	for _, f := range irmod.Funcs {
		g.funcOffsets[f.Name] = len(g.code)
		g.compileFunc_i386(f)
	}

	collectNativeFuncSizes(irmod, g.funcOffsets, len(g.code))

	// Resolve call fixups
	var unresolved []string
	for _, fix := range g.callFixups {
		if fix.Target == "$rodata_header$" || fix.Target == "$data_addr$" {
			continue
		}
		target, ok := g.funcOffsets[fix.Target]
		if !ok {
			unresolved = append(unresolved, fix.Target)
			continue
		}
		g.patchRel32At(fix.CodeOffset, target)
	}
	if len(unresolved) > 0 {
		fmt.Fprintf(os.Stderr, "error: %d unresolved calls:\n", len(unresolved))
		seen := make(map[string]bool)
		for _, name := range unresolved {
			if !seen[name] {
				fmt.Fprintf(os.Stderr, "  %s\n", name)
				seen[name] = true
			}
		}
		return fmt.Errorf("%d unresolved calls", len(unresolved))
	}

	// Build and write ELF
	elf := g.buildELF32(irmod)
	err := os.WriteFile(outputPath, elf, 0755)
	if err != nil {
		return fmt.Errorf("write output: %v", err)
	}

	return nil
}

// compileFunc_i386 generates i386 code for a single IR function.
func (g *CodeGen) compileFunc_i386(f *IRFunc) {
	g.curFunc = f
	g.hasPending = false
	g.curFrameSize = len(f.Locals)
	if f.Params > g.curFrameSize {
		g.curFrameSize = f.Params
	}
	g.labelOffsets = make(map[int]int)
	g.jumpFixups = nil

	// Prologue: push ebp; mov ebp, esp; sub esp, N*4
	g.pushR32(REG32_EBP)
	g.movRR32(REG32_EBP, REG32_ESP)

	frameBytes := g.curFrameSize * 4
	if frameBytes > 0 {
		g.subRI32(REG32_ESP, int32(frameBytes))
	}

	// Pop params from operand stack (EDI) into local frame slots
	if f.Params > 0 {
		i := f.Params - 1
		for i >= 0 {
			g.opPop(REG32_EAX)
			offset := (i + 1) * 4
			g.emitStoreLocal32(offset, REG32_EAX)
			i = i - 1
		}
	}

	// Compile instructions
	for _, inst := range f.Code {
		g.compileInst_i386(inst)
	}

	// Resolve jump fixups within this function
	for _, fix := range g.jumpFixups {
		labelOff, ok := g.labelOffsets[fix.LabelID]
		if !ok {
			continue
		}
		g.patchRel32At(fix.CodeOffset, labelOff)
	}

	g.curFunc = nil
}

// compileInst_i386 generates code for a single IR instruction (i386).
func (g *CodeGen) compileInst_i386(inst Inst) {
	switch inst.Op {
	case OP_CONST_I64:
		g.compileConstI32(inst.Val)
	case OP_CONST_BOOL:
		if inst.Arg != 0 {
			g.compileConstI32(1)
		} else {
			g.compileConstI32(0)
		}
	case OP_CONST_NIL:
		g.compileConstI32(0)
	case OP_CONST_STR:
		g.compileConstStr_i386(inst.Name)

	case OP_LOCAL_GET:
		g.compileLocalGet_i386(inst.Arg)
	case OP_LOCAL_SET:
		g.compileLocalSet_i386(inst.Arg)
	case OP_LOCAL_ADDR:
		g.compileLocalAddr_i386(inst.Arg)

	case OP_GLOBAL_GET:
		g.compileGlobalGet_i386(inst)
	case OP_GLOBAL_SET:
		g.compileGlobalSet_i386(inst)
	case OP_GLOBAL_ADDR:
		g.compileGlobalAddr_i386(inst)

	case OP_DROP:
		g.opDrop()
	case OP_DUP:
		g.opLoad(REG32_EAX)
		g.opPush(REG32_EAX)

	case OP_ADD:
		g.compileBinOp_i386(inst.Op)
	case OP_SUB:
		g.compileBinOp_i386(inst.Op)
	case OP_MUL:
		g.compileBinOp_i386(inst.Op)
	case OP_DIV:
		g.compileBinOp_i386(inst.Op)
	case OP_MOD:
		g.compileBinOp_i386(inst.Op)
	case OP_NEG:
		g.opPop(REG32_EAX)
		g.negR32(REG32_EAX)
		g.opPush(REG32_EAX)

	case OP_AND:
		g.compileBinOp_i386(inst.Op)
	case OP_OR:
		g.compileBinOp_i386(inst.Op)
	case OP_XOR:
		g.compileBinOp_i386(inst.Op)
	case OP_SHL:
		g.compileBinOp_i386(inst.Op)
	case OP_SHR:
		g.compileBinOp_i386(inst.Op)

	case OP_EQ:
		g.compileCompare_i386(0x94) // sete
	case OP_NEQ:
		g.compileCompare_i386(0x95) // setne
	case OP_LT:
		g.compileCompare_i386(0x9c) // setl
	case OP_GT:
		g.compileCompare_i386(0x9f) // setg
	case OP_LEQ:
		g.compileCompare_i386(0x9e) // setle
	case OP_GEQ:
		g.compileCompare_i386(0x9d) // setge

	case OP_NOT:
		g.opPop(REG32_EAX)
		g.xorRI8_32(REG32_EAX, 0x01)
		g.opPush(REG32_EAX)

	case OP_LABEL:
		g.flush()
		g.labelOffsets[inst.Arg] = len(g.code)
	case OP_JMP:
		g.flush()
		fixup := g.jmpRel32()
		g.jumpFixups = append(g.jumpFixups, JumpFixup{
			CodeOffset: fixup,
			LabelID:    inst.Arg,
		})
	case OP_JMP_IF:
		g.opPop(REG32_EAX)
		g.testRR32(REG32_EAX, REG32_EAX)
		fixup := g.jccRel32(CC32_NE)
		g.jumpFixups = append(g.jumpFixups, JumpFixup{
			CodeOffset: fixup,
			LabelID:    inst.Arg,
		})
	case OP_JMP_IF_NOT:
		g.opPop(REG32_EAX)
		g.testRR32(REG32_EAX, REG32_EAX)
		fixup := g.jccRel32(CC32_E)
		g.jumpFixups = append(g.jumpFixups, JumpFixup{
			CodeOffset: fixup,
			LabelID:    inst.Arg,
		})

	case OP_CALL:
		g.compileCall_i386(inst)
	case OP_CALL_INTRINSIC:
		g.compileCallIntrinsic_i386(inst)
	case OP_RETURN:
		g.compileReturn_i386(inst)

	case OP_LOAD:
		g.compileLoad_i386(inst.Arg)
	case OP_STORE:
		g.compileStore_i386(inst.Arg)
	case OP_OFFSET:
		g.compileOffset_i386(inst)
	case OP_INDEX_ADDR:
		g.compileIndexAddr_i386(inst.Arg)
	case OP_LEN:
		g.compileLen_i386()

	case OP_CONVERT:
		g.compileConvert_i386(inst.Name)

	case OP_IFACE_BOX:
		g.compileIfaceBox_i386(inst)
	case OP_IFACE_CALL:
		g.compileIfaceCall_i386(inst)
	case OP_PANIC:
		if targetGOOS == "windows" {
			g.compilePanic_win386()
		} else {
			g.compilePanic_linux386()
		}

	case OP_SLICE_GET, OP_SLICE_MAKE, OP_STRING_GET, OP_STRING_MAKE:
		// Handled by intrinsics or builtins

	default:
		panic("ICE: unhandled opcode in compileInst_i386")
	}
}

// === Constant loading (i386) ===

func (g *CodeGen) compileConstI32(val int64) {
	g.flush()
	v32 := uint32(val)
	if v32 == 0 {
		g.xorRR32(REG32_EAX, REG32_EAX)
	} else {
		g.emitMovRegImm32(REG32_EAX, v32)
	}
	g.opPush(REG32_EAX)
}

func (g *CodeGen) compileConstStr_i386(s string) {
	g.flush()
	decoded := decodeStringLiteral(s)

	headerOff, ok := g.stringMap[decoded]
	if !ok {
		// Store string bytes in rodata
		dataOff := len(g.rodata)
		g.rodata = append(g.rodata, []byte(decoded)...)

		// Store 8-byte header {data_ptr:4, len:4} in rodata
		headerOff = len(g.rodata)
		g.emitRodataU32(0)                    // data_ptr placeholder (4 bytes)
		g.emitRodataU32(uint32(len(decoded))) // len (4 bytes)

		g.stringMap[decoded] = headerOff
		// Store dataOff in the placeholder temporarily
		putU32(g.rodata[headerOff:headerOff+4], uint32(dataOff))
	}

	// Push header address onto operand stack: mov eax, imm32
	g.emitMovRegImm32(REG32_EAX, uint32(headerOff))
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: len(g.code) - 4,
		Target:     "$rodata_header$",
	})
	g.opPush(REG32_EAX)
}

// === Local variable access (i386) ===

func (g *CodeGen) compileLocalGet_i386(idx int) {
	g.flush()
	offset := (idx + 1) * 4
	g.emitLoadLocal32(offset, REG32_EAX)
	g.opPush(REG32_EAX)
}

func (g *CodeGen) compileLocalSet_i386(idx int) {
	g.opPop(REG32_EAX)
	offset := (idx + 1) * 4
	g.emitStoreLocal32(offset, REG32_EAX)
}

func (g *CodeGen) compileLocalAddr_i386(idx int) {
	g.flush()
	offset := (idx + 1) * 4
	g.emitLeaLocal32(offset, REG32_EAX)
	g.opPush(REG32_EAX)
}

// === Global variable access (i386) ===

func (g *CodeGen) compileGlobalGet_i386(inst Inst) {
	g.flush()
	g.emitMovRegImm32(REG32_ECX, uint32(inst.Arg*4))
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: len(g.code) - 4,
		Target:     "$data_addr$",
	})
	g.loadMem32(REG32_EAX, REG32_ECX, 0)
	g.opPush(REG32_EAX)
}

func (g *CodeGen) compileGlobalSet_i386(inst Inst) {
	g.opPop(REG32_EAX)
	g.emitMovRegImm32(REG32_ECX, uint32(inst.Arg*4))
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: len(g.code) - 4,
		Target:     "$data_addr$",
	})
	g.storeMem32(REG32_ECX, 0, REG32_EAX)
}

func (g *CodeGen) compileGlobalAddr_i386(inst Inst) {
	g.flush()
	g.emitMovRegImm32(REG32_EAX, uint32(inst.Arg*4))
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: len(g.code) - 4,
		Target:     "$data_addr$",
	})
	g.opPush(REG32_EAX)
}

// === Binary operations (i386) ===

func (g *CodeGen) compileBinOp_i386(op Opcode) {
	g.opPop(REG32_EAX)
	g.opPop(REG32_ECX)

	switch op {
	case OP_ADD:
		g.addRR32(REG32_ECX, REG32_EAX)
	case OP_SUB:
		g.subRR32(REG32_ECX, REG32_EAX)
	case OP_MUL:
		g.imulRR32(REG32_ECX, REG32_EAX)
	case OP_DIV:
		g.movRR32(REG32_EDX, REG32_EAX)
		g.movRR32(REG32_EAX, REG32_ECX)
		g.movRR32(REG32_ECX, REG32_EDX)
		g.cdq32()
		g.idivR32(REG32_ECX)
		g.movRR32(REG32_ECX, REG32_EAX)
	case OP_MOD:
		g.movRR32(REG32_EDX, REG32_EAX)
		g.movRR32(REG32_EAX, REG32_ECX)
		g.movRR32(REG32_ECX, REG32_EDX)
		g.cdq32()
		g.idivR32(REG32_ECX)
		g.movRR32(REG32_ECX, REG32_EDX)
	case OP_AND:
		g.andRR32(REG32_ECX, REG32_EAX)
	case OP_OR:
		g.orRR32(REG32_ECX, REG32_EAX)
	case OP_XOR:
		g.xorRR32(REG32_ECX, REG32_EAX)
	case OP_SHL:
		g.movRR32(REG32_EDX, REG32_ECX)
		g.movRR32(REG32_ECX, REG32_EAX)
		g.shlCl32(REG32_EDX)
		g.movRR32(REG32_ECX, REG32_EDX)
	case OP_SHR:
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
	g.emitBytes(0x0f, 0xb6, 0xc9)         // movzx ecx, cl
	g.opPush(REG32_ECX)
}

// === Function calls (i386) ===

func (g *CodeGen) compileCall_i386(inst Inst) {
	if len(inst.Name) > 18 && inst.Name[0:18] == "builtin.composite." {
		g.compileCompositeLitCall_i386(inst)
		return
	}
	g.emitCallPlaceholder(inst.Name)
}

func (g *CodeGen) compileCompositeLitCall_i386(inst Inst) {
	fieldCount := inst.Arg
	structSize := fieldCount * 4

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
		offset := i * 4
		if offset == 0 {
			g.storeMem32(REG32_ECX, 0, REG32_EAX)
		} else if offset <= 127 {
			g.storeMem32(REG32_ECX, offset, REG32_EAX)
		} else {
			g.emitBytes(0x89, 0x81) // mov [ecx+off32], eax
			g.emitU32(uint32(offset))
		}
		i++
	}

	g.opPush(REG32_ECX)
}

func (g *CodeGen) compileReturn_i386(inst Inst) {
	g.flush()
	g.movRR32(REG32_ESP, REG32_EBP)
	g.popR32(REG32_EBP)
	g.ret()
}

// === Intrinsics (i386) ===

func (g *CodeGen) compileCallIntrinsic_i386(inst Inst) {
	g.flush()
	switch inst.Name {
	case "Syscall":
		// Linux i386 only
		g.compileSyscallIntrinsic_linux386(inst.Arg)
	case "SysRead":
		g.compileSyscallRead_win386()
	case "SysWrite":
		g.compileSyscallWrite_win386()
	case "SysOpen":
		g.compileSyscallOpen_win386()
	case "SysClose":
		g.compileSyscallClose_win386()
	case "SysExit":
		g.compileSyscallExit_win386()
	case "SysMmap":
		g.compileSyscallMmap_win386()
	case "SysMkdir":
		g.compileSyscallMkdir_win386()
	case "SysRmdir":
		g.compileSyscallRmdir_win386()
	case "SysUnlink":
		g.compileSyscallUnlink_win386()
	case "SysGetcwd":
		g.compileSyscallGetcwd_win386()
	case "SysGetdents64":
		g.compileSyscallGetdents_win386()
	case "SysStat":
		g.compileSyscallStat_win386()
	case "SysGetCommandLine":
		g.compileSyscallGetCommandLine_win386()
	case "SysGetEnvStrings":
		g.compileSyscallGetEnvStrings_win386()
	case "SysFindFirstFile":
		g.compileSyscallFindFirstFile_win386()
	case "SysFindNextFile":
		g.compileSyscallFindNextFile_win386()
	case "SysFindClose":
		g.compileSyscallFindClose_win386()
	case "SysCreateProcess":
		g.compileSyscallCreateProcess_win386()
	case "SysWaitProcess":
		g.compileSyscallWaitProcess_win386()
	case "SysCreatePipe":
		g.compileSyscallCreatePipe_win386()
	case "SysSetStdHandle":
		g.compileSyscallSetStdHandle_win386()
	case "SysGetpid":
		g.compileSyscallGetpid_win386()
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

func (g *CodeGen) compileSliceptrIntrinsic_i386() {
	// Param 0 = slice header pointer. Read [header+0] = data ptr.
	g.emitLoadLocal32(1*4, REG32_EAX)
	g.loadMem32(REG32_EAX, REG32_EAX, 0)
	g.opPush(REG32_EAX)
}

func (g *CodeGen) compileMakesliceIntrinsic_i386() {
	// Params: ptr (local 0), len (local 1), cap (local 2)
	// Allocate 16 bytes for header {ptr:4, len:4, cap:4, elem_size:4}
	g.compileConstI32(16)
	g.emitCallPlaceholder("runtime.Alloc")
	g.opPop(REG32_ECX)

	// Fill header
	g.emitLoadLocal32(1*4, REG32_EAX) // ptr
	g.storeMem32(REG32_ECX, 0, REG32_EAX)
	g.emitLoadLocal32(2*4, REG32_EAX) // len
	g.storeMem32(REG32_ECX, 4, REG32_EAX)
	g.emitLoadLocal32(3*4, REG32_EAX) // cap
	g.storeMem32(REG32_ECX, 8, REG32_EAX)
	g.emitMovRegImm32(REG32_EAX, 1)   // elem_size = 1
	g.storeMem32(REG32_ECX, 12, REG32_EAX)

	g.opPush(REG32_ECX)
}

func (g *CodeGen) compileStringptrIntrinsic_i386() {
	// Param 0 = string header pointer. Read [header+0] = data ptr.
	g.emitLoadLocal32(1*4, REG32_EAX)
	g.loadMem32(REG32_EAX, REG32_EAX, 0)
	g.opPush(REG32_EAX)
}

func (g *CodeGen) compileMakestringIntrinsic_i386() {
	// Params: ptr (local 0), len (local 1)
	// Allocate 8-byte header {ptr:4, len:4}
	g.compileConstI32(8)
	g.emitCallPlaceholder("runtime.Alloc")
	g.opPop(REG32_ECX)

	g.emitLoadLocal32(1*4, REG32_EAX) // ptr
	g.storeMem32(REG32_ECX, 0, REG32_EAX)
	g.emitLoadLocal32(2*4, REG32_EAX) // len
	g.storeMem32(REG32_ECX, 4, REG32_EAX)

	g.opPush(REG32_ECX)
}

func (g *CodeGen) compileTostringIntrinsic_i386() {
	// Param 0 = value (could be string ptr or interface box ptr)
	// Heuristic: if [ptr+0] < 256, it's a type_id (interface box)
	g.emitLoadLocal32(1*4, REG32_EAX) // load value

	// Test: check if [eax] < 256
	g.loadMem32(REG32_ECX, REG32_EAX, 0)
	g.emitBytes(0x81, 0xf9) // cmp ecx, 256
	g.emitU32(256)
	stringCaseFixup := g.jccRel32(CC32_AE)

	// Interface case: ecx = type_id, [eax+4] = concrete value
	g.loadMem32(REG32_EDX, REG32_EAX, 4)
	g.opPush(REG32_EDX)

	// Save type_id (ecx) on call stack
	g.pushR32(REG32_ECX)

	// Generate dispatch chain for Error/String methods
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

	g.popR32(REG32_ECX) // type_id

	endFixups := make([]int, 0)

	// type_id 1 = int: call runtime.IntToString
	g.cmpRI32(REG32_ECX, 1)
	nextFixup := g.jccRel32(CC32_NE)
	g.emitCallPlaceholder("runtime.IntToString")
	endFixups = append(endFixups, g.jmpRel32())
	g.patchRel32(nextFixup)

	// type_id 2 = string: value is already a string ptr
	g.cmpRI32(REG32_ECX, 2)
	nextFixup = g.jccRel32(CC32_NE)
	endFixups = append(endFixups, g.jmpRel32())
	g.patchRel32(nextFixup)

	// User-defined type dispatch
	for _, entry := range entries {
		g.cmpRI32(REG32_ECX, int32(entry.typeID))
		nextFixup = g.jccRel32(CC32_NE)
		g.emitCallPlaceholder(entry.funcName)
		endFixups = append(endFixups, g.jmpRel32())
		g.patchRel32(nextFixup)
	}

	// Default: push empty string
	g.opDrop()
	g.compileConstI32(0)
	g.flush()

	endAddr := len(g.code)
	for _, fixup := range endFixups {
		g.patchRel32At(fixup, endAddr)
	}

	finalEndFixup := g.jmpRel32()

	// string_case: just pass through the value
	g.patchRel32(stringCaseFixup)
	g.emitLoadLocal32(1*4, REG32_EAX)
	g.opPush(REG32_EAX)
	g.flush()

	g.patchRel32(finalEndFixup)
}

func (g *CodeGen) compileReadPtrIntrinsic_i386() {
	// Param 0 = addr. Read 4 bytes at addr, push result.
	g.emitLoadLocal32(1*4, REG32_EAX)
	g.loadMem32(REG32_EAX, REG32_EAX, 0)
	g.opPush(REG32_EAX)
}

func (g *CodeGen) compileWritePtrIntrinsic_i386() {
	// Param 0 = addr, Param 1 = val. Write 4 bytes.
	g.emitLoadLocal32(1*4, REG32_EAX) // addr
	g.emitLoadLocal32(2*4, REG32_ECX) // val
	g.storeMem32(REG32_EAX, 0, REG32_ECX)
}

func (g *CodeGen) compileWriteByteIntrinsic_i386() {
	// Param 0 = addr, Param 1 = val. Write 1 byte.
	g.emitLoadLocal32(1*4, REG32_EAX) // addr
	g.emitLoadLocal32(2*4, REG32_ECX) // val
	g.emitBytes(0x88, 0x08)           // mov [eax], cl
}

// === Interface dispatch (i386) ===

func (g *CodeGen) compileIfaceBox_i386(inst Inst) {
	typeID := inst.Arg

	g.opPop(REG32_EAX)
	g.pushR32(REG32_EAX) // save concrete value

	// Allocate 8 bytes: {type_id:4, value:4}
	g.compileConstI32(8)
	g.emitCallPlaceholder("runtime.Alloc")
	g.opPop(REG32_ECX) // box ptr

	// Store type_id at [box+0]
	g.emitMovRegImm32(REG32_EAX, uint32(typeID))
	g.storeMem32(REG32_ECX, 0, REG32_EAX)

	// Restore concrete value and store at [box+4]
	g.popR32(REG32_EAX)
	g.storeMem32(REG32_ECX, 4, REG32_EAX)

	g.opPush(REG32_ECX)
}

func (g *CodeGen) compileIfaceCall_i386(inst Inst) {
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

	// Load type_id from [eax+0], concrete value from [eax+4]
	g.loadMem32(REG32_ECX, REG32_EAX, 0) // type_id
	g.loadMem32(REG32_EDX, REG32_EAX, 4) // concrete value

	// Push concrete value as receiver
	g.opPush(REG32_EDX)

	// Restore regular args
	i = argCount - 1
	for i >= 0 {
		g.flush()
		g.popR32(REG32_EAX)
		g.opPush(REG32_EAX)
		i = i - 1
	}

	// Save ecx (type_id)
	g.pushR32(REG32_ECX)

	// Extract bare method name
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

	g.popR32(REG32_ECX) // type_id

	if len(entries) == 0 {
		g.int3()
	} else {
		endFixups := make([]int, 0)
		for _, entry := range entries {
			g.cmpRI32(REG32_ECX, int32(entry.typeID))
			nextFixup := g.jccRel32(CC32_NE)
			g.emitCallPlaceholder(entry.funcName)
			endFixups = append(endFixups, g.jmpRel32())
			g.patchRel32(nextFixup)
		}
		g.int3()
		endAddr := len(g.code)
		for _, fixup := range endFixups {
			g.patchRel32At(fixup, endAddr)
		}
	}
}

// === Memory operations (i386) ===

func (g *CodeGen) compileLoad_i386(size int) {
	g.opPop(REG32_ECX)
	g.testRR32(REG32_ECX, REG32_ECX)
	if size == 1 {
		g.emitBytes(0x75, 0x04)                       // jnz +4
		g.xorRR32(REG32_EAX, REG32_EAX)               // 2 bytes
		g.jmpRel8(0x03)                                // jmp +3
		g.loadMemByte32(REG32_EAX, REG32_ECX, 0)      // movzx eax, byte [ecx]
	} else {
		g.emitBytes(0x75, 0x04)                  // jnz +4
		g.xorRR32(REG32_EAX, REG32_EAX)          // 2 bytes
		g.jmpRel8(0x02)                           // jmp +2
		g.loadMem32(REG32_EAX, REG32_ECX, 0)     // mov eax, [ecx]
	}
	g.opPush(REG32_EAX)
}

func (g *CodeGen) compileStore_i386(size int) {
	g.opPop(REG32_ECX) // addr
	g.opPop(REG32_EAX) // value
	if size == 1 {
		g.emitBytes(0x88, 0x01) // mov [ecx], al
	} else {
		g.storeMem32(REG32_ECX, 0, REG32_EAX)
	}
}

func (g *CodeGen) compileOffset_i386(inst Inst) {
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

func (g *CodeGen) compileLen_i386() {
	g.opPop(REG32_EAX)
	g.testRR32(REG32_EAX, REG32_EAX)
	g.emitBytes(0x75, 0x04)                    // jnz +4
	g.xorRR32(REG32_EAX, REG32_EAX)            // 2 bytes
	g.jmpRel8(0x03)                             // jmp +3
	g.loadMem32(REG32_EAX, REG32_EAX, 4)       // len at offset 4 (not 8)
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
	case "uint16":
		g.opPop(REG32_EAX)
		g.movzxW32(REG32_EAX)
		g.opPush(REG32_EAX)
	case "int64", "uint64":
		// On i386, 64-bit types truncated to 32-bit (best effort)
	}
}
