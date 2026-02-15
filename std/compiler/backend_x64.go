//go:build !no_backend_linux_amd64 || !no_backend_windows_amd64

package main

import (
	"fmt"
	"os"
)

// generateAmd64ELF compiles an IRModule to an x86-64 ELF binary.
func generateAmd64ELF(irmod *IRModule, outputPath string) error {
	g := &CodeGen{
		funcOffsets:   make(map[string]int),
		labelOffsets:  make(map[int]int),
		stringMap:     make(map[string]int),
		globalOffsets: make([]int, len(irmod.Globals)),
		baseAddr:      0x400000,
		irmod:         irmod,
		wordSize:      8,
	}

	// Allocate .data space for globals (8 bytes each)
	for i := range irmod.Globals {
		g.globalOffsets[i] = i * 8
	}
	g.data = make([]byte, len(irmod.Globals)*8)

	// Emit _start
	g.emitStart(irmod)

	// First pass: compile all functions to get their offsets
	for _, f := range irmod.Funcs {
		g.funcOffsets[f.Name] = len(g.code)
		g.compileFunc(f)
	}

	collectNativeFuncSizes(irmod, g.funcOffsets, len(g.code))

	// Resolve call fixups (skip special targets that are resolved in buildELF64)
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
	elf := g.buildELF64(irmod)
	err := os.WriteFile(outputPath, elf, 0755)
	if err != nil {
		return fmt.Errorf("write output: %v", err)
	}

	return nil
}

// compileFunc generates x86-64 code for a single IR function.
func (g *CodeGen) compileFunc(f *IRFunc) {
	g.curFunc = f
	g.hasPending = false
	g.curFrameSize = len(f.Locals)
	// Intrinsic functions may have Params > 0 but empty Locals.
	// Ensure the frame is large enough to hold all param slots.
	if f.Params > g.curFrameSize {
		g.curFrameSize = f.Params
	}
	g.labelOffsets = make(map[int]int)
	g.jumpFixups = nil

	// Prologue: push rbp; mov rbp, rsp; sub rsp, N*8
	g.pushR(REG_RBP)
	g.movRR(REG_RBP, REG_RSP)

	frameBytes := g.curFrameSize * 8
	if targetGOOS == "windows" {
		frameBytes = alignUp(frameBytes, 16)
	}
	if frameBytes > 0 {
		g.subRI(REG_RSP, int32(frameBytes))
	}

	// Pop params from operand stack (R15) into local frame slots
	// Params are pushed left-to-right by caller, so param 0 is deepest.
	// We pop in reverse order: last param first.
	if f.Params > 0 {
		i := f.Params - 1
		for i >= 0 {
			g.opPop(REG_RAX)
			offset := (i + 1) * 8
			g.emitStoreLocal(offset, REG_RAX)
			i = i - 1
		}
	}

	// Compile instructions
	for _, inst := range f.Code {
		g.compileInst(inst)
	}

	// Resolve jump fixups within this function
	funcStart := g.funcOffsets[f.Name]
	for _, fix := range g.jumpFixups {
		labelOff, ok := g.labelOffsets[fix.LabelID]
		if !ok {
			continue
		}
		g.patchRel32At(fix.CodeOffset, labelOff)
	}

	_ = funcStart
	g.curFunc = nil
}

// compileInst generates code for a single IR instruction.
func (g *CodeGen) compileInst(inst Inst) {
	switch inst.Op {
	case OP_CONST_I64:
		g.compileConstI64(inst.Val)
	case OP_CONST_BOOL:
		if inst.Arg != 0 {
			g.compileConstI64(1)
		} else {
			g.compileConstI64(0)
		}
	case OP_CONST_NIL:
		g.compileConstI64(0)
	case OP_CONST_STR:
		g.compileConstStr(inst.Name)

	case OP_LOCAL_GET:
		g.compileLocalGet(inst.Arg)
	case OP_LOCAL_SET:
		g.compileLocalSet(inst.Arg)
	case OP_LOCAL_ADDR:
		g.compileLocalAddr(inst.Arg)

	case OP_GLOBAL_GET:
		g.compileGlobalGet(inst)
	case OP_GLOBAL_SET:
		g.compileGlobalSet(inst)
	case OP_GLOBAL_ADDR:
		g.compileGlobalAddr(inst)

	case OP_DROP:
		g.opDrop()
	case OP_DUP:
		g.opLoad(REG_RAX)
		g.opPush(REG_RAX)

	case OP_ADD:
		g.compileBinOp(inst.Op)
	case OP_SUB:
		g.compileBinOp(inst.Op)
	case OP_MUL:
		g.compileBinOp(inst.Op)
	case OP_DIV:
		g.compileBinOp(inst.Op)
	case OP_MOD:
		g.compileBinOp(inst.Op)
	case OP_NEG:
		g.opPop(REG_RAX)
		g.negR(REG_RAX)
		g.opPush(REG_RAX)

	case OP_AND:
		g.compileBinOp(inst.Op)
	case OP_OR:
		g.compileBinOp(inst.Op)
	case OP_XOR:
		g.compileBinOp(inst.Op)
	case OP_SHL:
		g.compileBinOp(inst.Op)
	case OP_SHR:
		g.compileBinOp(inst.Op)

	case OP_EQ:
		g.compileCompare(0x94) // sete
	case OP_NEQ:
		g.compileCompare(0x95) // setne
	case OP_LT:
		g.compileCompare(0x9c) // setl
	case OP_GT:
		g.compileCompare(0x9f) // setg
	case OP_LEQ:
		g.compileCompare(0x9e) // setle
	case OP_GEQ:
		g.compileCompare(0x9d) // setge

	case OP_NOT:
		g.opPop(REG_RAX)
		g.xorRI8(REG_RAX, 0x01)
		g.opPush(REG_RAX)

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
		// pop value, test, jnz
		g.opPop(REG_RAX)
		g.testRR(REG_RAX, REG_RAX)
		fixup := g.jccRel32(CC_NE)
		g.jumpFixups = append(g.jumpFixups, JumpFixup{
			CodeOffset: fixup,
			LabelID:    inst.Arg,
		})
	case OP_JMP_IF_NOT:
		// pop value, test, jz
		g.opPop(REG_RAX)
		g.testRR(REG_RAX, REG_RAX)
		fixup := g.jccRel32(CC_E)
		g.jumpFixups = append(g.jumpFixups, JumpFixup{
			CodeOffset: fixup,
			LabelID:    inst.Arg,
		})

	case OP_CALL:
		g.compileCall(inst)
	case OP_CALL_INTRINSIC:
		g.compileCallIntrinsic(inst)
	case OP_RETURN:
		g.compileReturn(inst)

	case OP_LOAD:
		g.compileLoad(inst.Arg)
	case OP_STORE:
		g.compileStore(inst.Arg)
	case OP_OFFSET:
		g.compileOffset(inst)
	case OP_INDEX_ADDR:
		g.compileIndexAddr(inst.Arg)
	case OP_LEN:
		g.compileLen()

	case OP_CONVERT:
		g.compileConvert(inst.Name)

	case OP_IFACE_BOX:
		g.compileIfaceBox(inst)
	case OP_IFACE_CALL:
		g.compileIfaceCall(inst)
	case OP_PANIC:
		if targetGOOS == "windows" {
			g.compilePanicWin64()
		} else {
			g.compilePanic()
		}

	case OP_SLICE_GET, OP_SLICE_MAKE, OP_STRING_GET, OP_STRING_MAKE:
		// These are handled by intrinsics or builtins

	default:
		panic("ICE: unhandled opcode in compileInst")
	}
}

// === Constant loading ===

func (g *CodeGen) compileConstI64(val int64) {
	g.flush()
	if val == 0 {
		g.xorRR(REG_RAX, REG_RAX) // 3 bytes instead of 10
	} else if val > 0 && val <= 0x7fffffff {
		// mov eax, imm32 (zero-extends to rax)
		g.emitByte(0xb8) // mov eax, imm32
		g.emitU32(uint32(val))
	} else if val < 0 && val >= -0x80000000 {
		// mov rax, sign-extended imm32
		g.emitBytes(0x48, 0xc7, 0xc0) // mov rax, imm32 (sign-extended)
		g.emitU32(uint32(val))
	} else {
		g.emitMovRegImm64(REG_RAX, uint64(val))
	}
	g.opPush(REG_RAX)
}

func (g *CodeGen) compileConstStr(s string) {
	g.flush()
	decoded := decodeStringLiteral(s)

	headerOff, ok := g.stringMap[decoded]
	if !ok {
		// Store string bytes in rodata
		dataOff := len(g.rodata)
		g.rodata = append(g.rodata, []byte(decoded)...)

		// Store 16-byte header {data_ptr, len} in rodata
		// data_ptr will need fixup when we know rodata's virtual address
		headerOff = len(g.rodata)
		// placeholder for data_ptr (8 bytes) — will be fixed up
		g.emitRodataU64(0)                    // data_ptr placeholder
		g.emitRodataU64(uint64(len(decoded))) // len

		// Record for fixup: header needs data_ptr = rodataVAddr + dataOff
		g.stringMap[decoded] = headerOff
		// We store dataOff in the placeholder temporarily
		putU64(g.rodata[headerOff:headerOff+8], uint64(dataOff))
	}

	// Push header address onto operand stack
	g.emitMovRegImm64(REG_RAX, uint64(headerOff))
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: len(g.code) - 8,
		Target:     "$rodata_header$",
	})
	g.opPush(REG_RAX)
}

// === Local variable access ===

func (g *CodeGen) compileLocalGet(idx int) {
	g.flush()
	offset := (idx + 1) * 8
	g.emitLoadLocal(offset, REG_RAX)
	g.opPush(REG_RAX)
}

func (g *CodeGen) compileLocalSet(idx int) {
	g.opPop(REG_RAX)
	offset := (idx + 1) * 8
	g.emitStoreLocal(offset, REG_RAX)
}

func (g *CodeGen) compileLocalAddr(idx int) {
	g.flush()
	offset := (idx + 1) * 8
	g.emitLeaLocal(offset, REG_RAX)
	g.opPush(REG_RAX)
}

// === Global variable access ===

func (g *CodeGen) compileGlobalGet(inst Inst) {
	g.flush()
	g.emitMovRegImm64(REG_RCX, uint64(inst.Arg*8)) // offset placeholder
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: len(g.code) - 8,
		Target:     "$data_addr$",
	})
	g.loadMem(REG_RAX, REG_RCX, 0)
	g.opPush(REG_RAX)
}

func (g *CodeGen) compileGlobalSet(inst Inst) {
	g.opPop(REG_RAX)
	g.emitMovRegImm64(REG_RCX, uint64(inst.Arg*8)) // offset placeholder
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: len(g.code) - 8,
		Target:     "$data_addr$",
	})
	g.storeMem(REG_RCX, 0, REG_RAX)
}

func (g *CodeGen) compileGlobalAddr(inst Inst) {
	g.flush()
	g.emitMovRegImm64(REG_RAX, uint64(inst.Arg*8)) // offset placeholder
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: len(g.code) - 8,
		Target:     "$data_addr$",
	})
	g.opPush(REG_RAX)
}

// === Binary operations ===

func (g *CodeGen) compileBinOp(op Opcode) {
	// pop two values: rax = second (top), rcx = first (below), push result
	g.opPop(REG_RAX)
	g.opPop(REG_RCX)

	switch op {
	case OP_ADD:
		g.addRR(REG_RCX, REG_RAX)
	case OP_SUB:
		g.subRR(REG_RCX, REG_RAX)
	case OP_MUL:
		g.imulRR(REG_RCX, REG_RAX)
	case OP_DIV:
		g.movRR(REG_RDX, REG_RAX)
		g.movRR(REG_RAX, REG_RCX)
		g.movRR(REG_RCX, REG_RDX)
		g.cqo()
		g.idivR(REG_RCX)
		g.movRR(REG_RCX, REG_RAX)
	case OP_MOD:
		g.movRR(REG_RDX, REG_RAX)
		g.movRR(REG_RAX, REG_RCX)
		g.movRR(REG_RCX, REG_RDX)
		g.cqo()
		g.idivR(REG_RCX)
		g.movRR(REG_RCX, REG_RDX)
	case OP_AND:
		g.andRR(REG_RCX, REG_RAX)
	case OP_OR:
		g.orRR(REG_RCX, REG_RAX)
	case OP_XOR:
		g.xorRR(REG_RCX, REG_RAX)
	case OP_SHL:
		g.movRR(REG_RDX, REG_RCX)
		g.movRR(REG_RCX, REG_RAX)
		g.shlCl(REG_RDX)
		g.movRR(REG_RCX, REG_RDX)
	case OP_SHR:
		g.movRR(REG_RDX, REG_RCX)
		g.movRR(REG_RCX, REG_RAX)
		g.sarCl(REG_RDX)
		g.movRR(REG_RCX, REG_RDX)
	}

	g.opPush(REG_RCX)
}

// === Comparison operations ===

func (g *CodeGen) compileCompare(setccOpcode byte) {
	g.opPop(REG_RAX)
	g.opPop(REG_RCX)
	g.cmpRR(REG_RCX, REG_RAX)
	g.emitBytes(0x0f, setccOpcode, 0xc1) // setCC cl
	g.emitBytes(0x48, 0x0f, 0xb6, 0xc9)  // movzx rcx, cl
	g.opPush(REG_RCX)
}

// === Function calls ===

func (g *CodeGen) compileCall(inst Inst) {
	// Handle composite literals inline
	if len(inst.Name) > 18 && inst.Name[0:18] == "builtin.composite." {
		g.compileCompositeLitCall(inst)
		return
	}

	// Arguments are already on the operand stack.
	// Just emit the call — callee will pop args.
	g.emitCallPlaceholder(inst.Name)

	// If the call target returns values, they are already pushed by callee.
	// The IR handles push/pop balance.
}

// compileCompositeLitCall handles struct/slice composite literal creation.
// Fields are on the operand stack (pushed in order). We allocate memory
// and store each field at consecutive 8-byte slots.
func (g *CodeGen) compileCompositeLitCall(inst Inst) {
	fieldCount := inst.Arg
	structSize := fieldCount * 8

	if structSize == 0 {
		// Empty struct — push nil
		g.compileConstI64(0)
		return
	}

	// Save field values from operand stack onto call stack (in reverse)
	i := 0
	for i < fieldCount {
		g.opPop(REG_RAX)
		g.pushR(REG_RAX)
		i++
	}

	// Allocate struct: push size, call Alloc
	g.compileConstI64(int64(structSize))
	g.emitCallPlaceholder("runtime.Alloc")
	// Result (struct ptr) on operand stack
	g.opPop(REG_RCX)

	// Pop fields from call stack and store into struct in declaration order.
	// The save loop popped the operand stack top-first (last field first),
	// so on the x86 stack field0 is on top. Store field0 at offset 0, etc.
	i = 0
	for i < fieldCount {
		g.popR(REG_RAX)
		offset := i * 8
		if offset == 0 {
			g.storeMem(REG_RCX, 0, REG_RAX)
		} else if offset <= 127 {
			g.storeMem(REG_RCX, offset, REG_RAX)
		} else {
			g.emitBytes(0x48, 0x89, 0x81) // mov [rcx+off32], rax
			g.emitU32(uint32(offset))
		}
		i++
	}

	// Push struct pointer as result
	g.opPush(REG_RCX)
}

func (g *CodeGen) compileReturn(inst Inst) {
	g.flush()
	g.movRR(REG_RSP, REG_RBP)
	g.popR(REG_RBP)
	g.ret()
}

// === Intrinsics ===

func (g *CodeGen) compileCallIntrinsic(inst Inst) {
	g.flush()
	if targetGOOS == "windows" {
		g.compileCallIntrinsicWin64(inst)
		return
	}
	switch inst.Name {
	case "Syscall":
		g.compileSyscallIntrinsic(inst.Arg)
	case "Sliceptr":
		g.compileSliceptrIntrinsic()
	case "Makeslice":
		g.compileMakesliceIntrinsic()
	case "Stringptr":
		g.compileStringptrIntrinsic()
	case "Makestring":
		g.compileMakestringIntrinsic()
	case "Tostring":
		g.compileTostringIntrinsic()
	case "ReadPtr":
		g.compileReadPtrIntrinsic()
	case "WritePtr":
		g.compileWritePtrIntrinsic()
	case "WriteByte":
		g.compileWriteByteIntrinsic()
	default:
		panic("ICE: unknown intrinsic '" + inst.Name + "' in compileCallIntrinsic")
	}
}

func (g *CodeGen) compileSliceptrIntrinsic() {
	// Param 0 = slice header pointer. Read [header+0] = data ptr.
	g.emitLoadLocal(1*8, REG_RAX)
	g.loadMem(REG_RAX, REG_RAX, 0)
	g.opPush(REG_RAX)
}

func (g *CodeGen) compileMakesliceIntrinsic() {
	// Params: ptr (local 0), len (local 1), cap (local 2)
	// Allocate 32 bytes for header, fill {ptr, len, cap, elem_size=1}, push header addr
	// Makeslice always creates byte slices, so elem_size=1.

	g.compileConstI64(32)
	g.emitCallPlaceholder("runtime.Alloc")
	g.opPop(REG_RCX)

	// Fill header: [rcx+0] = ptr, [rcx+8] = len, [rcx+16] = cap, [rcx+24] = 1
	g.emitLoadLocal(1*8, REG_RAX)
	g.storeMem(REG_RCX, 0, REG_RAX)
	g.emitLoadLocal(2*8, REG_RAX)
	g.storeMem(REG_RCX, 8, REG_RAX)
	g.emitLoadLocal(3*8, REG_RAX)
	g.storeMem(REG_RCX, 16, REG_RAX)
	g.emitByte(0xb8) // mov eax, 1
	g.emitU32(1)
	g.storeMem(REG_RCX, 24, REG_RAX)

	// Push header pointer
	g.opPush(REG_RCX)
}

func (g *CodeGen) compileStringptrIntrinsic() {
	// Param 0 = string header pointer. Read [header+0] = data ptr.
	g.emitLoadLocal(1*8, REG_RAX)
	g.loadMem(REG_RAX, REG_RAX, 0)
	g.opPush(REG_RAX)
}

func (g *CodeGen) compileMakestringIntrinsic() {
	// Params: ptr (local 0), len (local 1)
	// Allocate 16-byte header, fill {ptr, len}, push header addr
	g.compileConstI64(16)
	g.emitCallPlaceholder("runtime.Alloc")
	g.opPop(REG_RCX)

	g.emitLoadLocal(1*8, REG_RAX)
	g.storeMem(REG_RCX, 0, REG_RAX)
	g.emitLoadLocal(2*8, REG_RAX)
	g.storeMem(REG_RCX, 8, REG_RAX)

	g.opPush(REG_RCX)
}

func (g *CodeGen) compileTostringIntrinsic() {
	// Param 0 = value (could be string ptr or interface box ptr)
	// Heuristic: if [ptr+0] < 256, it's a type_id (interface box); otherwise it's a string data pointer
	g.emitLoadLocal(1*8, REG_RAX) // load value

	// Test: is rax a valid pointer? Check if [rax] < 256
	g.loadMem(REG_RCX, REG_RAX, 0)
	g.emitBytes(0x48, 0x81, 0xf9) // cmp rcx, 256
	g.emitU32(256)
	stringCaseFixup := g.jccRel32(CC_AE)

	// Interface case: rcx = type_id, [rax+8] = concrete value
	// Push concrete value as receiver, then call Error/String method via dispatch
	g.loadMem(REG_RDX, REG_RAX, 8)
	// Push concrete value onto operand stack
	g.opPush(REG_RDX)

	// Save type_id (rcx) for dispatch
	g.pushR(REG_RCX)

	// Generate dispatch chain for "Error" method
	var entries []dispatchEntry
	if g.irmod != nil && g.irmod.TypeIDs != nil {
		for typeName, tid := range g.irmod.TypeIDs {
			// Check for Error method first, then String
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

	g.popR(REG_RCX) // type_id

	// Always generate dispatch chain (with built-in int/string + user types)
	endFixups := make([]int, 0)

	// type_id 1 = int: call runtime.IntToString
	g.cmpRI(REG_RCX, 1)
	nextFixup := g.jccRel32(CC_NE)
	g.emitCallPlaceholder("runtime.IntToString")
	endFixups = append(endFixups, g.jmpRel32())
	g.patchRel32(nextFixup)

	// type_id 2 = string: value is already a string ptr, pass through
	g.cmpRI(REG_RCX, 2)
	nextFixup = g.jccRel32(CC_NE)
	// Concrete value is already on the operand stack, nothing to do
	endFixups = append(endFixups, g.jmpRel32())
	g.patchRel32(nextFixup)

	// User-defined type dispatch (Error/String methods)
	for _, entry := range entries {
		if entry.typeID <= 127 {
			g.cmpRI(REG_RCX, int32(entry.typeID))
		} else {
			g.emitBytes(0x48, 0x81, 0xf9)
			g.emitU32(uint32(entry.typeID))
		}
		nextFixup = g.jccRel32(CC_NE)

		g.emitCallPlaceholder(entry.funcName)

		endFixups = append(endFixups, g.jmpRel32())

		g.patchRel32(nextFixup)
	}
	// Default: push empty string
	g.opDrop() // drop receiver
	g.compileConstI64(0)
	g.flush() // materialize the pending push before setting endAddr

	endAddr := len(g.code)
	for _, fixup := range endFixups {
		g.patchRel32At(fixup, endAddr)
	}

	// Jump past the string case (jmp to final end)
	finalEndFixup := g.jmpRel32()

	// string_case: just pass through the value (already a string ptr)
	g.patchRel32(stringCaseFixup)

	g.emitLoadLocal(1*8, REG_RAX)
	g.opPush(REG_RAX)
	g.flush() // materialize result before convergence with dispatch paths

	// final_end:
	g.patchRel32(finalEndFixup)
}

func (g *CodeGen) compileReadPtrIntrinsic() {
	// Param 0 = addr. Read 8 bytes at addr, push result.
	g.emitLoadLocal(1*8, REG_RAX)
	g.loadMem(REG_RAX, REG_RAX, 0)
	g.opPush(REG_RAX)
}

func (g *CodeGen) compileWritePtrIntrinsic() {
	// Param 0 = addr, Param 1 = val. Write 8 bytes.
	g.emitLoadLocal(1*8, REG_RAX) // addr
	g.emitLoadLocal(2*8, REG_RCX) // val
	g.storeMem(REG_RAX, 0, REG_RCX)
}

func (g *CodeGen) compileWriteByteIntrinsic() {
	// Param 0 = addr, Param 1 = val. Write 1 byte.
	g.emitLoadLocal(1*8, REG_RAX) // addr
	g.emitLoadLocal(2*8, REG_RCX) // val
	g.emitBytes(0x88, 0x08)       // mov [rax], cl
}

// === Interface dispatch ===

func (g *CodeGen) compileIfaceBox(inst Inst) {
	// Stack: ... concreteValue
	// Pop concrete value, allocate 16 bytes, store {type_id, value}, push box pointer
	typeID := inst.Arg

	// Pop concrete value into rax
	g.opPop(REG_RAX)
	g.pushR(REG_RAX) // save concrete value on x86 stack

	// Allocate 16 bytes: push 16, call runtime.Alloc
	g.compileConstI64(16)
	g.emitCallPlaceholder("runtime.Alloc")
	// Result (box ptr) is on operand stack
	g.opPop(REG_RCX) // box ptr

	// Store type_id at [box+0]
	g.emitByte(0xb8) // mov eax, imm32
	g.emitU32(uint32(typeID))
	g.storeMem(REG_RCX, 0, REG_RAX)

	// Restore concrete value and store at [box+8]
	g.popR(REG_RAX)
	g.storeMem(REG_RCX, 8, REG_RAX)

	// Push box pointer as result
	g.opPush(REG_RCX)
}

func (g *CodeGen) compileIfaceCall(inst Inst) {
	// Stack: ... ifacePtr arg0 arg1 ...
	// inst.Arg = number of regular args (excluding receiver)
	// inst.Name = "ifaceType.Method" e.g. "error.Error"
	argCount := inst.Arg
	methodName := inst.Name

	// Save regular args from operand stack to call stack (in reverse)
	i := 0
	for i < argCount {
		g.opPop(REG_RAX)
		g.pushR(REG_RAX)
		i++
	}

	// Pop interface pointer into rax
	g.opPop(REG_RAX)

	// Load type_id from [rax+0] into rcx, concrete value from [rax+8] into rdx
	g.loadMem(REG_RCX, REG_RAX, 0)
	g.loadMem(REG_RDX, REG_RAX, 8)

	// Push concrete value as receiver onto operand stack
	g.opPush(REG_RDX)

	// Restore regular args from call stack (in correct order)
	i = argCount - 1
	for i >= 0 {
		g.flush()
		g.popR(REG_RAX)
		g.opPush(REG_RAX)
		i = i - 1
	}

	// Save rcx (type_id) on call stack since the call may clobber it
	g.pushR(REG_RCX)

	// Extract the method name part from "iface.Method"
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

	// Generate if/else chain over type IDs → concrete method calls
	// Collect all type IDs that implement this interface method
	var entries []dispatchEntry

	if g.irmod != nil && g.irmod.TypeIDs != nil {
		for typeName, tid := range g.irmod.TypeIDs {
			// Check if typeName.Method exists in methodTable
			candidate := typeName + "." + bareMethod
			if _, ok := g.irmod.MethodTable[candidate]; ok {
				entries = append(entries, dispatchEntry{tid, candidate})
			}
		}
	}

	// Restore type_id from call stack
	g.popR(REG_RCX)

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
				g.emitBytes(0x48, 0x81, 0xf9) // cmp rcx, imm32
				g.emitU32(uint32(entry.typeID))
			}
			// jne next (rel32)
			nextFixup := g.jccRel32(CC_NE)

			// Call the concrete method (args already on operand stack)
			g.emitCallPlaceholder(entry.funcName)

			// jmp end
			endFixups = append(endFixups, g.jmpRel32())

			// next:
			g.patchRel32(nextFixup)
		}
		// Default: trap
		g.int3()

		// Patch all end jumps
		endAddr := len(g.code)
		for _, fixup := range endFixups {
			g.patchRel32At(fixup, endAddr)
		}
	}
}

// === Memory operations ===

func (g *CodeGen) compileLoad(size int) {
	g.opPop(REG_RCX)
	g.testRR(REG_RCX, REG_RCX)
	if size == 1 {
		g.emitBytes(0x75, 0x05)             // jnz +5 (skip zero case)
		g.xorRR(REG_RAX, REG_RAX)
		g.jmpRel8(0x04)                     // jmp +4 (skip load)
		g.loadMemByte(REG_RAX, REG_RCX, 0) // movzx rax, byte [rcx]
	} else {
		g.emitBytes(0x75, 0x05)        // jnz +5 (skip zero case)
		g.xorRR(REG_RAX, REG_RAX)
		g.jmpRel8(0x03)               // jmp +3 (skip load)
		g.loadMem(REG_RAX, REG_RCX, 0) // mov rax, [rcx]
	}
	g.opPush(REG_RAX)
}

func (g *CodeGen) compileStore(size int) {
	// stack: ... value addr  → pop addr into rcx, pop value into rax, store
	g.opPop(REG_RCX) // addr
	g.opPop(REG_RAX) // value
	if size == 1 {
		g.emitBytes(0x88, 0x01) // mov [rcx], al
	} else {
		g.storeMem(REG_RCX, 0, REG_RAX)
	}
}

func (g *CodeGen) compileOffset(inst Inst) {
	g.opPop(REG_RAX)
	if inst.Arg != 0 {
		g.addRI(REG_RAX, int32(inst.Arg))
	}
	g.opPush(REG_RAX)
}

func (g *CodeGen) compileIndexAddr(elemSize int) {
	// pop index, pop slice-header-ptr, compute data_ptr + index * elemSize, push
	g.opPop(REG_RAX)  // index
	g.opPop(REG_RCX)  // slice header ptr

	// Load data_ptr from header: [rcx+0]
	g.loadMem(REG_RCX, REG_RCX, 0)

	// Compute address: data_ptr + index * elemSize
	if elemSize == 1 {
		g.addRR(REG_RCX, REG_RAX)
	} else if elemSize == 8 {
		g.shlImm(REG_RAX, 3)
		g.addRR(REG_RCX, REG_RAX)
	} else {
		g.imulRRI32(REG_RAX, REG_RAX, int32(elemSize))
		g.addRR(REG_RCX, REG_RAX)
	}

	g.opPush(REG_RCX)
}

func (g *CodeGen) compileLen() {
	g.opPop(REG_RAX)
	g.testRR(REG_RAX, REG_RAX)
	g.emitBytes(0x75, 0x05)    // jnz +5 (skip zero case)
	g.xorRR(REG_RAX, REG_RAX) // 3 bytes
	g.jmpRel8(0x04)            // jmp +4 (skip load) 2 bytes
	g.loadMem(REG_RAX, REG_RAX, 8)
	g.opPush(REG_RAX)
}

// === Type conversions ===

func (g *CodeGen) compileConvert(typeName string) {
	// Most conversions are no-ops (all values are 8 bytes)
	// string([]byte) and []byte(string) need runtime calls
	switch typeName {
	case "string":
		// []byte→string: call runtime.BytesToString
		g.emitCallPlaceholder("runtime.BytesToString")
	case "[]byte":
		// string→[]byte: call runtime.StringToBytes
		g.emitCallPlaceholder("runtime.StringToBytes")
	case "int", "uintptr", "uint", "int64", "uint64":
		// No-op: all 8-byte integers
	case "byte":
		g.opPop(REG_RAX)
		g.movzxB(REG_RAX)
		g.opPush(REG_RAX)
	case "uint16":
		g.opPop(REG_RAX)
		g.movzxW(REG_RAX)
		g.opPush(REG_RAX)
	case "int32":
		g.opPop(REG_RAX)
		g.movsxD(REG_RAX)
		g.opPush(REG_RAX)
	case "uint32":
		g.opPop(REG_RAX)
		g.clearHi32(REG_RAX)
		g.opPush(REG_RAX)
	}
}
