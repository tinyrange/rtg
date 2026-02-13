//go:build !no_backend_wasi_wasm32

package main

import (
	"fmt"
	"os"
)

// === WASM32 Backend: IR → WASM binary ===

// WasmGen holds state for generating WASM code from IR.
type WasmGen struct {
	mod    *wasmModule
	irmod  *IRModule
	w      wasmCodeWriter // current function body writer
	funcMap map[string]int // IR func name → WASM func index

	// WASI import function indices
	wasiFdWrite            int
	wasiFdRead             int
	wasiFdClose            int
	wasiPathOpen           int
	wasiArgsSizesGet       int
	wasiArgsGet            int
	wasiEnvironSizesGet    int
	wasiEnvironGet         int
	wasiProcExit           int
	wasiPathCreateDir      int
	wasiPathRemoveDir      int
	wasiPathUnlinkFile     int
	wasiFdReaddir          int
	wasiFdPrestatGet       int
	wasiFdPrestatDirName   int

	// WASM global indices
	globalSP int // shadow stack pointer

	// Memory layout
	scratchAddr   int32 // WASI scratch area (iovec etc.)
	globalsAddr   int32 // start of global variables in linear memory
	globalsSize   int32 // total bytes for globals
	stringsAddr   int32 // start of string data + headers
	stringsSize   int32 // total bytes for strings
	shadowBase    int32 // initial shadow stack pointer (top of shadow region)

	// String dedup: decoded content → offset of header in string data area
	stringMap     map[string]int
	stringData    []byte // raw string data + headers

	// Current function state
	curFunc      *IRFunc
	curFrameSize int
	numParams    int
	numWasmLocals int // WASM locals beyond params (frame slots + temps)
	tempLocal    int  // index of a temp i32 local for reordering
	tempLocal64  int  // index of a temp i64 local for DUP of i64 values

	// i64 type tracking
	valTypes     []byte        // type stack: WASM_TYPE_I32 or WASM_TYPE_I64 per stack entry
	localI64     map[int]bool  // which IR locals hold i64 values
	localOffsets []int32       // per-local byte offset in shadow stack frame

	// Stackifier state
	blockStack []wasmCtrl
	dead       bool // true when current code position is unreachable
}

// wasmCtrl tracks a WASM control flow block for br depth computation.
type wasmCtrl struct {
	kind         int  // 0=block, 1=loop, 2=if
	labelID      int  // IR label this block corresponds to (-1 if none)
	hasLiveBreak bool // true if any live br/br_if targets this block
}

// fwdTarget represents a forward jump target for block pre-opening.
type fwdTarget struct {
	labelID  int
	labelPos int
}

const (
	WASM_CTRL_BLOCK = 0
	WASM_CTRL_LOOP  = 1
	WASM_CTRL_IF    = 2
)

// generateWasm32 is the entry point for the WASM backend.
func generateWasm32(irmod *IRModule, outputPath string) error {
	g := &WasmGen{
		mod:       &wasmModule{memMin: 2}, // start with 2 pages (128KB)
		irmod:     irmod,
		funcMap:   make(map[string]int),
		stringMap: make(map[string]int),
	}

	// Setup WASI imports
	g.setupWASIImports()

	// Setup memory layout
	g.setupMemoryLayout()

	// Register all functions (first pass: get indices)
	for _, f := range irmod.Funcs {
		params := make([]byte, f.Params)
		pi := 0
		for pi < len(params) {
			params[pi] = WASM_TYPE_I32
			pi++
		}
		results := make([]byte, f.RetCount)
		ri := 0
		for ri < len(results) {
			results[ri] = WASM_TYPE_I32
			ri++
		}
		idx := g.mod.addFunc(params, results)
		g.funcMap[f.Name] = idx
	}

	// Add _start function
	startIdx := g.mod.addFunc(nil, nil)
	g.funcMap["_start"] = startIdx

	// Compile all functions
	for _, f := range irmod.Funcs {
		body := g.compileFunc(f)
		g.mod.codes = append(g.mod.codes, body)
		funcSizes = append(funcSizes, FuncSize{Name: f.Name, Size: len(body)})
	}

	// Compile _start
	startBody := g.compileStart()
	g.mod.codes = append(g.mod.codes, startBody)

	// Export _start and memory
	g.mod.addExport("_start", WASM_EXT_FUNC, uint32(startIdx))
	g.mod.addExport("memory", WASM_EXT_MEMORY, 0)

	// Setup data segments
	g.setupDataSegments()

	// Compute minimum memory pages needed
	totalStatic := g.shadowBase + 65536 // shadow stack + initial heap space
	pages := uint32((totalStatic + 65535) / 65536)
	if pages < 4 {
		pages = 4
	}
	g.mod.memMin = pages

	// Encode and write
	binary := g.mod.encode()
	err := os.WriteFile(outputPath, binary, 0755)
	if err != nil {
		return fmt.Errorf("write output: %v", err)
	}
	return nil
}

// === WASI Imports ===

func (g *WasmGen) setupWASIImports() {
	wasi := "wasi_snapshot_preview1"

	// fd_write(fd: i32, iovs: i32, iovs_len: i32, nwritten: i32) -> i32
	g.wasiFdWrite = g.mod.addImport(wasi, "fd_write",
		[]byte{WASM_TYPE_I32, WASM_TYPE_I32, WASM_TYPE_I32, WASM_TYPE_I32},
		[]byte{WASM_TYPE_I32})

	// fd_read(fd: i32, iovs: i32, iovs_len: i32, nread: i32) -> i32
	g.wasiFdRead = g.mod.addImport(wasi, "fd_read",
		[]byte{WASM_TYPE_I32, WASM_TYPE_I32, WASM_TYPE_I32, WASM_TYPE_I32},
		[]byte{WASM_TYPE_I32})

	// fd_close(fd: i32) -> i32
	g.wasiFdClose = g.mod.addImport(wasi, "fd_close",
		[]byte{WASM_TYPE_I32},
		[]byte{WASM_TYPE_I32})

	// path_open(dirfd: i32, dirflags: i32, path: i32, path_len: i32, oflags: i32, rights_base: i64, rights_inheriting: i64, fdflags: i32, fd_out: i32) -> i32
	g.wasiPathOpen = g.mod.addImport(wasi, "path_open",
		[]byte{WASM_TYPE_I32, WASM_TYPE_I32, WASM_TYPE_I32, WASM_TYPE_I32, WASM_TYPE_I32, WASM_TYPE_I64, WASM_TYPE_I64, WASM_TYPE_I32, WASM_TYPE_I32},
		[]byte{WASM_TYPE_I32})

	// args_sizes_get(argc: i32, argv_buf_size: i32) -> i32
	g.wasiArgsSizesGet = g.mod.addImport(wasi, "args_sizes_get",
		[]byte{WASM_TYPE_I32, WASM_TYPE_I32},
		[]byte{WASM_TYPE_I32})

	// args_get(argv: i32, argv_buf: i32) -> i32
	g.wasiArgsGet = g.mod.addImport(wasi, "args_get",
		[]byte{WASM_TYPE_I32, WASM_TYPE_I32},
		[]byte{WASM_TYPE_I32})

	// environ_sizes_get(count: i32, buf_size: i32) -> i32
	g.wasiEnvironSizesGet = g.mod.addImport(wasi, "environ_sizes_get",
		[]byte{WASM_TYPE_I32, WASM_TYPE_I32},
		[]byte{WASM_TYPE_I32})

	// environ_get(environ: i32, environ_buf: i32) -> i32
	g.wasiEnvironGet = g.mod.addImport(wasi, "environ_get",
		[]byte{WASM_TYPE_I32, WASM_TYPE_I32},
		[]byte{WASM_TYPE_I32})

	// proc_exit(code: i32) -> void
	g.wasiProcExit = g.mod.addImport(wasi, "proc_exit",
		[]byte{WASM_TYPE_I32}, nil)

	// path_create_directory(fd: i32, path: i32, path_len: i32) -> i32
	g.wasiPathCreateDir = g.mod.addImport(wasi, "path_create_directory",
		[]byte{WASM_TYPE_I32, WASM_TYPE_I32, WASM_TYPE_I32},
		[]byte{WASM_TYPE_I32})

	// path_remove_directory(fd: i32, path: i32, path_len: i32) -> i32
	g.wasiPathRemoveDir = g.mod.addImport(wasi, "path_remove_directory",
		[]byte{WASM_TYPE_I32, WASM_TYPE_I32, WASM_TYPE_I32},
		[]byte{WASM_TYPE_I32})

	// path_unlink_file(fd: i32, path: i32, path_len: i32) -> i32
	g.wasiPathUnlinkFile = g.mod.addImport(wasi, "path_unlink_file",
		[]byte{WASM_TYPE_I32, WASM_TYPE_I32, WASM_TYPE_I32},
		[]byte{WASM_TYPE_I32})

	// fd_readdir(fd: i32, buf: i32, buf_len: i32, cookie: i64, bufused: i32) -> i32
	g.wasiFdReaddir = g.mod.addImport(wasi, "fd_readdir",
		[]byte{WASM_TYPE_I32, WASM_TYPE_I32, WASM_TYPE_I32, WASM_TYPE_I64, WASM_TYPE_I32},
		[]byte{WASM_TYPE_I32})

	// fd_prestat_get(fd: i32, buf: i32) -> i32
	g.wasiFdPrestatGet = g.mod.addImport(wasi, "fd_prestat_get",
		[]byte{WASM_TYPE_I32, WASM_TYPE_I32},
		[]byte{WASM_TYPE_I32})

	// fd_prestat_dir_name(fd: i32, path: i32, path_len: i32) -> i32
	g.wasiFdPrestatDirName = g.mod.addImport(wasi, "fd_prestat_dir_name",
		[]byte{WASM_TYPE_I32, WASM_TYPE_I32, WASM_TYPE_I32},
		[]byte{WASM_TYPE_I32})
}

// === Memory Layout ===

func (g *WasmGen) setupMemoryLayout() {
	// 0x0000 - 0x03FF: Null guard (1024 bytes)
	// 0x0400 - 0x04FF: WASI scratch (256 bytes for iovec etc.)
	// 0x0500+: Global variables
	g.scratchAddr = 0x0400
	g.globalsAddr = 0x0500

	numGlobals := len(g.irmod.Globals)
	g.globalsSize = int32(numGlobals * 4)

	// String data comes after globals
	g.stringsAddr = g.globalsAddr + g.globalsSize

	// Shadow stack pointer: WASM global (mutable i32)
	// Actual value set after we know string data size
	g.globalSP = g.mod.addGlobal(WASM_TYPE_I32, true, 0) // placeholder, updated later
}

func (g *WasmGen) setupDataSegments() {
	// String data segment
	if len(g.stringData) > 0 {
		g.mod.addData(g.stringsAddr, g.stringData)
	}

	g.stringsSize = int32(len(g.stringData))

	// Compute shadow stack base (starts after strings, aligned to 16)
	afterStrings := g.stringsAddr + g.stringsSize
	aligned := (afterStrings + 15) & ^int32(15)
	// Shadow stack grows downward from a high address
	// Leave 256KB for shadow stack, heap starts after
	g.shadowBase = aligned + 262144

	// Update the SP global's initial value
	if len(g.mod.globals) > 0 {
		g.mod.globals[g.globalSP] = wasmGlobal{
			valType: WASM_TYPE_I32,
			mutable: true,
			initVal: g.shadowBase,
		}
	}
}

// === String Constants ===

func (g *WasmGen) internString(s string) int32 {
	decoded := decodeStringLiteral(s)

	if headerOff, ok := g.stringMap[decoded]; ok {
		return int32(headerOff) + g.stringsAddr
	}

	// Append string data bytes
	dataOff := len(g.stringData)
	g.stringData = append(g.stringData, []byte(decoded)...)

	// Append 8-byte header {data_ptr:4, len:4}
	headerOff := len(g.stringData)
	// data_ptr: absolute address = stringsAddr + dataOff
	dataAddr := g.stringsAddr + int32(dataOff)
	g.stringData = append(g.stringData,
		byte(dataAddr), byte(dataAddr>>8), byte(dataAddr>>16), byte(dataAddr>>24))
	lenVal := int32(len(decoded))
	g.stringData = append(g.stringData,
		byte(lenVal), byte(lenVal>>8), byte(lenVal>>16), byte(lenVal>>24))

	g.stringMap[decoded] = headerOff
	return int32(headerOff) + g.stringsAddr
}

// === i64 type stack helpers ===

func (g *WasmGen) pushType(t byte) {
	g.valTypes = append(g.valTypes, t)
}

func (g *WasmGen) popType() byte {
	if len(g.valTypes) == 0 {
		return WASM_TYPE_I32
	}
	t := g.valTypes[len(g.valTypes)-1]
	g.valTypes = g.valTypes[0 : len(g.valTypes)-1]
	return t
}

func (g *WasmGen) peekType() byte {
	if len(g.valTypes) == 0 {
		return WASM_TYPE_I32
	}
	return g.valTypes[len(g.valTypes)-1]
}

func (g *WasmGen) promoteI32ToI64() {
	if g.peekType() == WASM_TYPE_I32 {
		g.w.i64ExtendI32U()
		g.valTypes[len(g.valTypes)-1] = WASM_TYPE_I64
	}
}

// ensureBothSameType promotes i32 operand to i64 if the other is i64.
// Returns the common type after promotion.
func (g *WasmGen) ensureBothSameType() byte {
	if len(g.valTypes) < 2 {
		return WASM_TYPE_I32
	}
	top := g.valTypes[len(g.valTypes)-1]
	below := g.valTypes[len(g.valTypes)-2]
	if top == below {
		return top
	}
	if top == WASM_TYPE_I64 && below == WASM_TYPE_I32 {
		// Need to promote the below value. Save top to temp, promote below, restore top.
		g.w.localSet(uint32(g.tempLocal64)) // save i64 top
		g.w.i64ExtendI32U()                  // promote i32 below to i64
		g.w.localGet(uint32(g.tempLocal64)) // restore i64 top
		g.valTypes[len(g.valTypes)-2] = WASM_TYPE_I64
		return WASM_TYPE_I64
	}
	if top == WASM_TYPE_I32 && below == WASM_TYPE_I64 {
		// Top is i32, promote it
		g.w.i64ExtendI32U()
		g.valTypes[len(g.valTypes)-1] = WASM_TYPE_I64
		return WASM_TYPE_I64
	}
	return WASM_TYPE_I32
}

// === Function Compilation ===

func (g *WasmGen) compileFunc(f *IRFunc) []byte {
	g.curFunc = f
	g.w = wasmCodeWriter{}
	g.blockStack = nil
	g.dead = false
	g.valTypes = nil

	frameSize := len(f.Locals)
	if f.Params > frameSize {
		frameSize = f.Params
	}
	g.curFrameSize = frameSize
	g.numParams = f.Params

	// Initialize localI64 from IRLocal.Is64 flags
	g.localI64 = make(map[int]bool)
	for i, loc := range f.Locals {
		if loc.Is64 {
			g.localI64[i] = true
		}
	}

	// Compute localOffsets: i64 locals get 8 bytes, others get 4
	g.localOffsets = make([]int32, frameSize)
	var frameBytes int32
	i := 0
	for i < frameSize {
		g.localOffsets[i] = frameBytes
		if g.localI64[i] {
			frameBytes = frameBytes + 8
		} else {
			frameBytes = frameBytes + 4
		}
		i++
	}

	// WASM locals: params are implicit (index 0..Params-1)
	// We need additional locals for:
	//   - 2 temp i32 locals for DUP/operand reordering/STORE swap
	//   - 1 temp i64 local for DUP of i64 values and type promotion
	// With shadow stack approach: ALL frame slots are in shadow stack memory.
	// WASM params are copied to shadow stack in prologue.
	g.numWasmLocals = 2 // 2 i32 temp locals (declared as first group)
	g.tempLocal = f.Params + 0
	g.tempLocal64 = f.Params + 2 // i64 temp local (declared as second group)

	// Prologue: allocate shadow stack frame
	if frameBytes > 0 {
		g.w.globalGet(uint32(g.globalSP))
		g.w.i32Const(frameBytes)
		g.w.op(OP_WASM_I32_SUB)
		g.w.globalSet(uint32(g.globalSP))
	}

	// Copy params from WASM params to shadow stack
	// For i64 params, extend i32 WASM param to i64 and use i64.store
	if f.Params > 0 {
		i = 0
		for i < f.Params {
			g.w.globalGet(uint32(g.globalSP))
			g.w.localGet(uint32(i))
			if g.localI64[i] {
				g.w.i64ExtendI32U() // extend i32 param to i64
				g.w.i64Store(3, uint32(g.localOffsets[i]))
			} else {
				g.w.i32Store(2, uint32(g.localOffsets[i]))
			}
			i++
		}
	}

	// Compile instructions via stackifier
	g.stackify(f.Code)

	// Build function body with local declarations
	// 2 i32 temp locals + 1 i64 temp local
	localCounts := []uint32{uint32(g.numWasmLocals), 1}
	localTypes := []byte{WASM_TYPE_I32, WASM_TYPE_I64}
	return encodeFuncBody(localCounts, localTypes, g.w.buf)
}

// === _start Entry Point ===

func (g *WasmGen) compileStart() []byte {
	g.w = wasmCodeWriter{}

	scratch := g.scratchAddr

	// Populate os.Args via WASI args_get
	// Step 1: args_sizes_get to learn argc and total buf size
	g.w.i32Const(scratch + 64) // argc ptr
	g.w.i32Const(scratch + 68) // argv_buf_size ptr
	g.w.call(uint32(g.wasiArgsSizesGet))
	g.w.drop() // ignore errno

	// Step 2: allocate argv array and argv_buf
	// argc at [scratch+64], buf_size at [scratch+68]
	// Allocate argv: argc * 4 bytes
	g.w.i32Const(scratch + 64)
	g.w.i32Load(2, 0) // argc
	g.w.i32Const(4)
	g.w.op(OP_WASM_I32_MUL)
	if idx, ok := g.funcMap["runtime.Alloc"]; ok {
		g.w.call(uint32(idx))
	}
	g.w.localSet(0) // local 0 = argv ptr

	// Allocate argv_buf: buf_size bytes
	g.w.i32Const(scratch + 68)
	g.w.i32Load(2, 0) // buf_size
	if idx, ok := g.funcMap["runtime.Alloc"]; ok {
		g.w.call(uint32(idx))
	}
	g.w.localSet(1) // local 1 = argv_buf ptr

	// Step 3: args_get(argv, argv_buf)
	g.w.localGet(0)
	g.w.localGet(1)
	g.w.call(uint32(g.wasiArgsGet))
	g.w.drop()

	// Step 4: Build os.Args slice from argv
	// For each arg, compute strlen and call runtime.Makestring
	// Then append to os.Args
	// Find os.Args global index
	argsGlobalIdx := -1
	for i, gl := range g.irmod.Globals {
		if gl.Name == "os.Args" {
			argsGlobalIdx = i
			break
		}
	}

	if argsGlobalIdx >= 0 {
		// Iterate over argc args
		// local 2 = loop counter i
		g.w.i32Const(0)
		g.w.localSet(2) // i = 0

		g.w.block(WASM_TYPE_VOID)
		g.w.loop(WASM_TYPE_VOID)
		// if i >= argc, break
		g.w.localGet(2)
		g.w.i32Const(scratch + 64)
		g.w.i32Load(2, 0) // argc
		g.w.op(OP_WASM_I32_GE_S)
		g.w.brIf(1)

		// argPtr = argv[i] (pointer to null-terminated string in argv_buf)
		g.w.localGet(0) // argv
		g.w.localGet(2) // i
		g.w.i32Const(4)
		g.w.op(OP_WASM_I32_MUL)
		g.w.op(OP_WASM_I32_ADD)
		g.w.i32Load(2, 0) // argv[i] = ptr to C string
		g.w.localSet(3) // local 3 = argPtr

		// Compute strlen
		g.w.i32Const(0)
		g.w.localSet(4) // local 4 = len = 0

		g.w.block(WASM_TYPE_VOID)
		g.w.loop(WASM_TYPE_VOID)
		g.w.localGet(3)
		g.w.localGet(4)
		g.w.op(OP_WASM_I32_ADD)
		g.w.i32Load8u(0, 0)
		g.w.op(OP_WASM_I32_EQZ)
		g.w.brIf(1)
		g.w.localGet(4)
		g.w.i32Const(1)
		g.w.op(OP_WASM_I32_ADD)
		g.w.localSet(4)
		g.w.br(0)
		g.w.end() // loop
		g.w.end() // block

		// Call runtime.Makestring(argPtr, len) → string header ptr
		g.w.localGet(3)
		g.w.localGet(4)
		if idx, ok := g.funcMap["runtime.Makestring"]; ok {
			g.w.call(uint32(idx))
		}
		// result is string header ptr

		// Append to os.Args: call runtime.SliceAppend(os.Args_global, elem_value)
		// os.Args is a global (slice header ptr) at globalsAddr + argsGlobalIdx*4
		argsAddr := g.globalsAddr + int32(argsGlobalIdx*4)
		g.w.localSet(3) // save string header

		// Load current os.Args slice header
		g.w.i32Const(argsAddr)
		g.w.i32Load(2, 0) // current slice header ptr

		// Push element to append
		g.w.localGet(3) // string header ptr

		// Push element size (4 bytes on wasm32 for string header pointer)
		g.w.i32Const(4)

		// Call runtime.SliceAppend(sliceHdr, elem, elemSize) → new slice header
		if idx, ok := g.funcMap["runtime.SliceAppend"]; ok {
			g.w.call(uint32(idx))
		}

		// Store back to os.Args global
		g.w.localSet(3) // new slice hdr
		g.w.i32Const(argsAddr)
		g.w.localGet(3)
		g.w.i32Store(2, 0)

		// i++
		g.w.localGet(2)
		g.w.i32Const(1)
		g.w.op(OP_WASM_I32_ADD)
		g.w.localSet(2)
		g.w.br(0)
		g.w.end() // loop
		g.w.end() // block
	}

	// Call init functions
	for _, f := range g.irmod.Funcs {
		if isInitFunc(f.Name) {
			idx, ok := g.funcMap[f.Name]
			if ok {
				g.w.call(uint32(idx))
			}
		}
	}

	// Call main.main
	if idx, ok := g.funcMap["main.main"]; ok {
		g.w.call(uint32(idx))
	}

	// proc_exit(0)
	g.w.i32Const(0)
	g.w.call(uint32(g.wasiProcExit))

	// _start needs locals for the args population
	localCounts := []uint32{5}
	localTypes := []byte{WASM_TYPE_I32}
	return encodeFuncBody(localCounts, localTypes, g.w.buf)
}

// === Stackifier: IR labels/jumps → WASM structured control flow ===

// Label analysis uses two maps instead of a struct to avoid
// field offset resolution issues during self-hosting on WASM.

// detectShortCircuit checks if a JMP_IF or JMP_IF_NOT at jumpPos is part of
// a ||/&& short-circuit pattern. Returns the target label, end label,
// position of JMP-to-end, and whether it's a short-circuit.
// Pattern: JMP_IF/JMP_IF_NOT targetLabel, ..., JMP endLabel, LABEL targetLabel, CONST, LABEL endLabel
func detectShortCircuit(code []Inst, jumpPos int, end int) (targetLabel int, endLabel int, jmpToEndPos int, ok bool) {
	targetLabel = code[jumpPos].Arg
	// Find target label position within range
	targetPos := -1
	for j := jumpPos + 1; j < end; j++ {
		if code[j].Op == OP_LABEL && code[j].Arg == targetLabel {
			targetPos = j
			break
		}
	}
	if targetPos < 0 || targetPos+2 >= end {
		return 0, 0, 0, false
	}
	// Check: LABEL target, CONST, LABEL end
	if code[targetPos+1].Op != OP_CONST_BOOL && code[targetPos+1].Op != OP_CONST_I64 {
		return 0, 0, 0, false
	}
	if code[targetPos+2].Op != OP_LABEL {
		return 0, 0, 0, false
	}
	endLabel = code[targetPos+2].Arg
	// Verify JMP to endLabel immediately precedes target label
	if targetPos > 0 && code[targetPos-1].Op == OP_JMP && code[targetPos-1].Arg == endLabel {
		jmpToEndPos = targetPos - 1
		return targetLabel, endLabel, jmpToEndPos, true
	}
	return 0, 0, 0, false
}

func (g *WasmGen) stackify(code []Inst) {
	// Pass 1: analyze labels using two separate maps
	loopHeaders := make(map[int]bool)
	blockTargets := make(map[int]bool)
	for i, inst := range code {
		switch inst.Op {
		case OP_JMP, OP_JMP_IF, OP_JMP_IF_NOT:
			targetLabel := inst.Arg
			// Determine if forward or backward jump
			labelPos := -1
			for j, c := range code {
				if c.Op == OP_LABEL {
					if c.Arg == targetLabel {
						labelPos = j
						break
					}
				}
			}
			if labelPos >= 0 {
				if labelPos <= i {
					loopHeaders[targetLabel] = true
				} else {
					blockTargets[targetLabel] = true
				}
			} else {
				blockTargets[targetLabel] = true
			}
		}
	}

	// Pass 2: emit WASM structured control flow
	g.emitStructured(code, 0, len(code), loopHeaders, blockTargets)
}

func (g *WasmGen) emitStructured(code []Inst, start int, end int, loopHeaders map[int]bool, blockTargets map[int]bool) {
	// --- Phase 1: Pre-open blocks for forward jump targets ---
	// Collect all forward jump targets at this level, skipping loop bodies
	// and short-circuit (||/&&) patterns.
	var fwdTargets []fwdTarget
	excludedLabels := make(map[int]bool) // labels in short-circuit patterns

	scanPos := start
	for scanPos < end {
		inst := code[scanPos]

		// Skip loop bodies during pre-scan
		if inst.Op == OP_LABEL {
			if loopHeaders[inst.Arg] {
				breakLabel := g.findBreakLabel(code, scanPos, end, inst.Arg)
				loopEnd := end
				if breakLabel >= 0 {
					for j := scanPos + 1; j < end; j++ {
						if code[j].Op == OP_LABEL && code[j].Arg == breakLabel {
							loopEnd = j
							break
						}
					}
				}
				scanPos = loopEnd
				continue
			}
		}

		// Detect short-circuit patterns and exclude their labels
		if inst.Op == OP_JMP_IF || inst.Op == OP_JMP_IF_NOT {
			tgtLabel, endLabel, _, scOk := detectShortCircuit(code, scanPos, end)
			if scOk {
				excludedLabels[tgtLabel] = true
				excludedLabels[endLabel] = true
				scanPos++
				continue
			}
		}

		// Collect forward jump targets (excluding short-circuit and loop labels)
		if inst.Op == OP_JMP || inst.Op == OP_JMP_IF || inst.Op == OP_JMP_IF_NOT {
			targetLabel := inst.Arg
			if excludedLabels[targetLabel] {
				scanPos++
				continue
			}
			if !loopHeaders[targetLabel] {
				labelPos := -1
				for j := scanPos + 1; j < end; j++ {
					if code[j].Op == OP_LABEL && code[j].Arg == targetLabel {
						labelPos = j
						break
					}
				}
				if labelPos > scanPos {
					dup := false
					for _, t := range fwdTargets {
						if t.labelID == targetLabel {
							dup = true
							break
						}
					}
					if !dup {
						fwdTargets = append(fwdTargets, fwdTarget{labelID: targetLabel, labelPos: labelPos})
					}
				}
			}
		}
		scanPos++
	}

	// Sort by label position DESCENDING (furthest first = outermost block)
	for si := 1; si < len(fwdTargets); si++ {
		sj := si
		for sj > 0 && fwdTargets[sj].labelPos > fwdTargets[sj-1].labelPos {
			tmp := fwdTargets[sj]
			fwdTargets[sj] = fwdTargets[sj-1]
			fwdTargets[sj-1] = tmp
			sj = sj - 1
		}
	}

	// Open blocks for all forward targets (furthest first = outermost)
	for _, t := range fwdTargets {
		g.w.block(WASM_TYPE_VOID)
		g.blockStack = append(g.blockStack, wasmCtrl{kind: WASM_CTRL_BLOCK, labelID: t.labelID})
	}

	// --- Phase 2: Process instructions ---
	i := start
	for i < end {
		inst := code[i]

		switch inst.Op {
		case OP_LABEL:
			if loopHeaders[inst.Arg] {
				breakLabel := g.findBreakLabel(code, i, end, inst.Arg)

				g.w.block(WASM_TYPE_VOID)
				g.blockStack = append(g.blockStack, wasmCtrl{kind: WASM_CTRL_BLOCK, labelID: breakLabel})

				g.w.loop(WASM_TYPE_VOID)
				g.blockStack = append(g.blockStack, wasmCtrl{kind: WASM_CTRL_LOOP, labelID: inst.Arg})

				loopEnd := end
				if breakLabel >= 0 {
					for j := i + 1; j < end; j++ {
						if code[j].Op == OP_LABEL && code[j].Arg == breakLabel {
							loopEnd = j
							break
						}
					}
				}

				g.emitStructured(code, i+1, loopEnd, loopHeaders, blockTargets)

				g.w.end() // end loop
				g.blockStack = g.blockStack[0 : len(g.blockStack)-1]
				// For loop end: br @loop goes back to start, not forward.
				// Fall-through to loop end only happens if dead is false.
				// (g.dead state propagates unchanged)

				g.w.end() // end block (break target)
				blockCtrl := g.blockStack[len(g.blockStack)-1]
				g.blockStack = g.blockStack[0 : len(g.blockStack)-1]
				if g.dead && !blockCtrl.hasLiveBreak {
					g.w.unreachable()
				} else {
					g.dead = false
				}

				i = loopEnd
				continue
			}
			if blockTargets[inst.Arg] && !excludedLabels[inst.Arg] {
				// Close the pre-opened block for this label
				depth := g.findBlockDepth(inst.Arg)
				if depth >= 0 {
					ctrl := g.blockStack[len(g.blockStack)-1]
					g.w.end()
					g.blockStack = g.blockStack[0 : len(g.blockStack)-1]
					// If the block had no live breaks and code was dead,
					// the code after this block is also unreachable.
					if g.dead && !ctrl.hasLiveBreak {
						g.w.unreachable()
					} else {
						g.dead = false
						// Re-push values lost due to void block close.
						// In switch dispatch, the discriminant is on the IR operand
						// stack but lost when a void block closes via br. If the next
						// non-label instruction is DUP or DROP, the IR expects a value
						// on the stack. The last DUP saved it to tempLocal, so re-push.
						// Skip over intervening LABELs that don't close blocks.
						peek := i + 1
						for peek < end && code[peek].Op == OP_LABEL {
							peek++
						}
						if peek < end {
							nextOp := code[peek].Op
							if nextOp == OP_DUP || nextOp == OP_DROP {
								g.w.localGet(uint32(g.tempLocal))
								g.pushType(WASM_TYPE_I32)
							}
						}
					}
				}
			}
			i++

		case OP_JMP:
			if !g.dead {
				depth := g.findBlockDepth(inst.Arg)
				if depth >= 0 {
					g.markLiveBreak(depth)
					g.w.br(uint32(depth))
					g.dead = true
				}
			}
			i++

		case OP_JMP_IF:
			if g.dead {
				i++
				continue
			}
			// Check for ||  short-circuit pattern
			tgtLabel, _, jmpToEndPos, scOk := detectShortCircuit(code, i, end)
			if scOk {
				// || pattern: if condition true → const 1, else → right side
				targetPos := -1
				for j := i + 1; j < end; j++ {
					if code[j].Op == OP_LABEL && code[j].Arg == tgtLabel {
						targetPos = j
						break
					}
				}
				g.popType() // condition consumed by ifOp
				savedTypes := make([]byte, len(g.valTypes))
				copy(savedTypes, g.valTypes)
				g.w.ifOp(WASM_TYPE_I32)
				g.blockStack = append(g.blockStack, wasmCtrl{kind: WASM_CTRL_IF, labelID: -1})
				// then: short-circuit value
				g.w.i32Const(int32(code[targetPos+1].Arg))
				// else: right side
				g.w.elseOp()
				g.valTypes = savedTypes
				g.emitStructured(code, i+1, jmpToEndPos, loopHeaders, blockTargets)
				// end if
				g.w.end()
				g.blockStack = g.blockStack[0 : len(g.blockStack)-1]
				g.pushType(WASM_TYPE_I32) // if/else produces i32
				// Skip past LABEL endLabel
				i = targetPos + 3
				continue
			}
			g.popType() // condition consumed by br_if
			depth := g.findBlockDepth(inst.Arg)
			if depth >= 0 {
				g.markLiveBreak(depth)
				g.w.brIf(uint32(depth))
				// br_if is conditional, doesn't make code dead
			}
			i++

		case OP_JMP_IF_NOT:
			if g.dead {
				i++
				continue
			}
			// Check for && short-circuit pattern
			tgtLabel, _, jmpToEndPos, scOk := detectShortCircuit(code, i, end)
			if scOk {
				// && pattern: if condition true → right side, else → const 0
				targetPos := -1
				for j := i + 1; j < end; j++ {
					if code[j].Op == OP_LABEL && code[j].Arg == tgtLabel {
						targetPos = j
						break
					}
				}
				g.popType() // condition consumed by ifOp
				savedTypes := make([]byte, len(g.valTypes))
				copy(savedTypes, g.valTypes)
				g.w.ifOp(WASM_TYPE_I32)
				g.blockStack = append(g.blockStack, wasmCtrl{kind: WASM_CTRL_IF, labelID: -1})
				// then: right side
				g.emitStructured(code, i+1, jmpToEndPos, loopHeaders, blockTargets)
				// else: short-circuit value
				g.w.elseOp()
				g.valTypes = savedTypes
				g.w.i32Const(int32(code[targetPos+1].Arg))
				// end if
				g.w.end()
				g.blockStack = g.blockStack[0 : len(g.blockStack)-1]
				g.pushType(WASM_TYPE_I32) // if/else produces i32
				// Skip past LABEL endLabel
				i = targetPos + 3
				continue
			}
			g.popType() // condition consumed by eqz + br_if
			depth := g.findBlockDepth(inst.Arg)
			if depth >= 0 {
				g.markLiveBreak(depth)
				g.w.op(OP_WASM_I32_EQZ)
				g.w.brIf(uint32(depth))
				// br_if is conditional, doesn't make code dead
			}
			i++

		default:
			if !g.dead {
				g.compileInst(inst)
			}
			i++
		}
	}
}

// findBreakLabel finds the break label for a loop by locating the backward
// JMP to the loop header and returning the label that immediately follows it.
func (g *WasmGen) findBreakLabel(code []Inst, loopStart int, end int, loopLabel int) int {
	// Find the last backward JMP to the loop header.
	// The label immediately after it is the break label.
	lastJmpToLoop := -1
	for j := loopStart + 1; j < end; j++ {
		if code[j].Op == OP_JMP && code[j].Arg == loopLabel {
			lastJmpToLoop = j
		}
	}
	if lastJmpToLoop >= 0 && lastJmpToLoop+1 < end {
		next := code[lastJmpToLoop+1]
		if next.Op == OP_LABEL {
			return next.Arg
		}
	}
	return -1
}

// findBlockDepth finds the br depth for jumping to a label.
func (g *WasmGen) findBlockDepth(labelID int) int {
	// Search block stack from top (innermost) to bottom (outermost)
	i := len(g.blockStack) - 1
	for i >= 0 {
		ctrl := g.blockStack[i]
		if ctrl.labelID == labelID {
			depth := len(g.blockStack) - 1 - i
			return depth
		}
		i = i - 1
	}
	return -1
}

// markLiveBreak sets hasLiveBreak on the block at the given br depth.
func (g *WasmGen) markLiveBreak(depth int) {
	idx := len(g.blockStack) - 1 - depth
	if idx >= 0 && idx < len(g.blockStack) {
		g.blockStack[idx].hasLiveBreak = true
	}
}

// === Instruction Compilation ===

func (g *WasmGen) compileInst(inst Inst) {
	switch inst.Op {
	case OP_CONST_I64:
		g.w.i32Const(int32(inst.Val))
		g.pushType(WASM_TYPE_I32)
	case OP_CONST_BOOL:
		if inst.Arg != 0 {
			g.w.i32Const(1)
		} else {
			g.w.i32Const(0)
		}
		g.pushType(WASM_TYPE_I32)
	case OP_CONST_NIL:
		g.w.i32Const(0)
		g.pushType(WASM_TYPE_I32)
	case OP_CONST_STR:
		addr := g.internString(inst.Name)
		g.w.i32Const(addr)
		g.pushType(WASM_TYPE_I32)

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
		g.popType()
		g.w.drop()
	case OP_DUP:
		g.compileDup()

	case OP_ADD:
		g.compileBinaryOp(OP_WASM_I32_ADD, OP_WASM_I64_ADD)
	case OP_SUB:
		g.compileBinaryOp(OP_WASM_I32_SUB, OP_WASM_I64_SUB)
	case OP_MUL:
		g.compileBinaryOp(OP_WASM_I32_MUL, OP_WASM_I64_MUL)
	case OP_DIV:
		g.compileBinaryOp(OP_WASM_I32_DIV_S, OP_WASM_I64_DIV_S)
	case OP_MOD:
		g.compileBinaryOp(OP_WASM_I32_REM_S, OP_WASM_I64_REM_S)

	case OP_AND:
		g.compileBinaryOp(OP_WASM_I32_AND, OP_WASM_I64_AND)
	case OP_OR:
		g.compileBinaryOp(OP_WASM_I32_OR, OP_WASM_I64_OR)
	case OP_XOR:
		g.compileBinaryOp(OP_WASM_I32_XOR, OP_WASM_I64_XOR)
	case OP_SHL:
		g.compileBinaryOp(OP_WASM_I32_SHL, OP_WASM_I64_SHL)
	case OP_SHR:
		g.compileBinaryOp(OP_WASM_I32_SHR_S, OP_WASM_I64_SHR_U)

	case OP_EQ:
		g.compileCompareOp(OP_WASM_I32_EQ, OP_WASM_I64_EQ)
	case OP_NEQ:
		g.compileCompareOp(OP_WASM_I32_NE, OP_WASM_I64_NE)
	case OP_LT:
		g.compileCompareOp(OP_WASM_I32_LT_S, OP_WASM_I64_LT_S)
	case OP_GT:
		g.compileCompareOp(OP_WASM_I32_GT_S, OP_WASM_I64_GT_S)
	case OP_LEQ:
		g.compileCompareOp(OP_WASM_I32_LE_S, OP_WASM_I64_LE_S)
	case OP_GEQ:
		g.compileCompareOp(OP_WASM_I32_GE_S, OP_WASM_I64_GE_S)

	case OP_NOT:
		t := g.popType()
		if t == WASM_TYPE_I64 {
			g.w.op(OP_WASM_I64_EQZ)
		} else {
			g.w.op(OP_WASM_I32_EQZ)
		}
		g.pushType(WASM_TYPE_I32)

	case OP_NEG:
		t := g.peekType()
		if t == WASM_TYPE_I64 {
			g.w.localSet(uint32(g.tempLocal64))
			g.w.i64Const(0)
			g.w.localGet(uint32(g.tempLocal64))
			g.w.op(OP_WASM_I64_SUB)
			// type stays i64
		} else {
			g.w.localSet(uint32(g.tempLocal))
			g.w.i32Const(0)
			g.w.localGet(uint32(g.tempLocal))
			g.w.op(OP_WASM_I32_SUB)
			// type stays i32
		}

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

	case OP_CALL:
		g.compileCall(inst)
	case OP_CALL_INTRINSIC:
		g.compileCallIntrinsic(inst)
	case OP_RETURN:
		g.compileReturn(inst)

	case OP_CONVERT:
		g.compileConvert(inst.Name)

	case OP_IFACE_BOX:
		g.compileIfaceBox(inst)
	case OP_IFACE_CALL:
		g.compileIfaceCall(inst)
	case OP_PANIC:
		g.compilePanic()

	case OP_SLICE_GET, OP_SLICE_MAKE, OP_STRING_GET, OP_STRING_MAKE:
		// Handled by intrinsics

	case OP_LABEL, OP_JMP, OP_JMP_IF, OP_JMP_IF_NOT:
		// Handled by stackifier

	default:
		// Unknown opcode - trap
		g.w.unreachable()
	}
}

// compileBinaryOp emits a binary operation, promoting to i64 if either operand is i64.
func (g *WasmGen) compileBinaryOp(i32op byte, i64op byte) {
	t := g.ensureBothSameType()
	g.popType()
	g.popType()
	if t == WASM_TYPE_I64 {
		g.w.op(i64op)
		g.pushType(WASM_TYPE_I64)
	} else {
		g.w.op(i32op)
		g.pushType(WASM_TYPE_I32)
	}
}

// compileCompareOp emits a comparison, promoting to i64 if needed. Result is always i32.
func (g *WasmGen) compileCompareOp(i32op byte, i64op byte) {
	t := g.ensureBothSameType()
	g.popType()
	g.popType()
	if t == WASM_TYPE_I64 {
		g.w.op(i64op)
	} else {
		g.w.op(i32op)
	}
	g.pushType(WASM_TYPE_I32)
}

// compileDup duplicates the top of stack, using the appropriate temp local.
func (g *WasmGen) compileDup() {
	t := g.peekType()
	if t == WASM_TYPE_I64 {
		g.w.localTee(uint32(g.tempLocal64))
		g.w.localGet(uint32(g.tempLocal64))
	} else {
		g.w.localTee(uint32(g.tempLocal))
		g.w.localGet(uint32(g.tempLocal))
	}
	g.pushType(t)
}

// === Local variable access (shadow stack) ===

func (g *WasmGen) compileLocalGet(idx int) {
	offset := g.localOffsets[idx]
	if g.localI64[idx] {
		g.w.globalGet(uint32(g.globalSP))
		g.w.i64Load(3, uint32(offset))
		g.pushType(WASM_TYPE_I64)
	} else {
		g.w.globalGet(uint32(g.globalSP))
		g.w.i32Load(2, uint32(offset))
		g.pushType(WASM_TYPE_I32)
	}
}

func (g *WasmGen) compileLocalSet(idx int) {
	offset := g.localOffsets[idx]
	t := g.popType()
	if t == WASM_TYPE_I64 || g.localI64[idx] {
		// If value is i64 or local expects i64
		if t == WASM_TYPE_I32 {
			g.w.i64ExtendI32U() // promote to i64
		}
		g.w.localSet(uint32(g.tempLocal64))
		g.w.globalGet(uint32(g.globalSP))
		g.w.localGet(uint32(g.tempLocal64))
		g.w.i64Store(3, uint32(offset))
		g.localI64[idx] = true
	} else {
		g.w.localSet(uint32(g.tempLocal))
		g.w.globalGet(uint32(g.globalSP))
		g.w.localGet(uint32(g.tempLocal))
		g.w.i32Store(2, uint32(offset))
	}
}

func (g *WasmGen) compileLocalAddr(idx int) {
	offset := g.localOffsets[idx]
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Const(offset)
	g.w.op(OP_WASM_I32_ADD)
	g.pushType(WASM_TYPE_I32)
}

// === Global variable access (linear memory) ===

func (g *WasmGen) compileGlobalGet(inst Inst) {
	g.w.i32Const(g.globalsAddr + int32(inst.Arg*4))
	g.w.i32Load(2, 0)
	g.pushType(WASM_TYPE_I32)
}

func (g *WasmGen) compileGlobalSet(inst Inst) {
	t := g.popType()
	if t == WASM_TYPE_I64 {
		g.w.i32WrapI64() // globals are always i32
	}
	g.w.localSet(uint32(g.tempLocal))
	g.w.i32Const(g.globalsAddr + int32(inst.Arg*4))
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Store(2, 0)
}

func (g *WasmGen) compileGlobalAddr(inst Inst) {
	g.w.i32Const(g.globalsAddr + int32(inst.Arg*4))
	g.pushType(WASM_TYPE_I32)
}

// === Memory operations ===

func (g *WasmGen) compileLoad(size int) {
	// Stack: [addr] → [value]
	// addr is always i32, result is always i32
	t := g.popType()
	if t == WASM_TYPE_I64 {
		g.w.i32WrapI64()
	}
	g.w.localTee(uint32(g.tempLocal))
	g.w.op(OP_WASM_I32_EQZ)
	g.w.ifOp(WASM_TYPE_I32)
	g.w.i32Const(0)
	g.w.elseOp()
	g.w.localGet(uint32(g.tempLocal))
	if size == 1 {
		g.w.i32Load8u(0, 0)
	} else {
		g.w.i32Load(2, 0)
	}
	g.w.end()
	g.pushType(WASM_TYPE_I32)
}

func (g *WasmGen) compileStore(size int) {
	// IR stack: [value, addr] (addr on top)
	// WASM i32.store wants: [addr, value]
	addrType := g.popType()
	if addrType == WASM_TYPE_I64 {
		g.w.i32WrapI64()
	}
	g.w.localSet(uint32(g.tempLocal)) // temp0 = addr

	// value is below, could be i64 — wrap to i32 for memory store
	vt := g.popType()
	if vt == WASM_TYPE_I64 {
		g.w.i32WrapI64()
	}
	temp2 := uint32(g.tempLocal + 1)
	g.w.localSet(temp2)
	g.w.localGet(uint32(g.tempLocal))
	g.w.localGet(temp2)
	if size == 1 {
		g.w.i32Store8(0, 0)
	} else {
		g.w.i32Store(2, 0)
	}
}

func (g *WasmGen) compileOffset(inst Inst) {
	// Stack: [ptr] → [ptr + offset]
	// ptr is i32 (address)
	if inst.Arg != 0 {
		// If top is i64, wrap it first (addresses are always i32)
		if g.peekType() == WASM_TYPE_I64 {
			g.w.i32WrapI64()
			g.valTypes[len(g.valTypes)-1] = WASM_TYPE_I32
		}
		g.w.i32Const(int32(inst.Arg))
		g.w.op(OP_WASM_I32_ADD)
	}
	// type stays i32 (or unchanged if 0 offset)
}

func (g *WasmGen) compileIndexAddr(elemSize int) {
	// Stack: [sliceHdrPtr, index] → [dataPtr + index*elemSize]
	// IR: index is on top, sliceHdrPtr below
	idxType := g.popType()
	if idxType == WASM_TYPE_I64 {
		g.w.i32WrapI64() // index should be i32
	}
	g.w.localSet(uint32(g.tempLocal)) // temp = index
	g.popType() // pop sliceHdrPtr type
	g.w.i32Load(2, 0) // load data_ptr from header[0]
	g.w.localGet(uint32(g.tempLocal)) // push index
	if elemSize == 1 {
		g.w.op(OP_WASM_I32_ADD)
	} else {
		g.w.i32Const(int32(elemSize))
		g.w.op(OP_WASM_I32_MUL)
		g.w.op(OP_WASM_I32_ADD)
	}
	g.pushType(WASM_TYPE_I32)
}

func (g *WasmGen) compileLen() {
	// Stack: [headerPtr] → [length]
	g.popType() // pop headerPtr
	g.w.localTee(uint32(g.tempLocal))
	g.w.op(OP_WASM_I32_EQZ)
	g.w.ifOp(WASM_TYPE_I32)
	g.w.i32Const(0)
	g.w.elseOp()
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Load(2, 4) // len at offset 4
	g.w.end()
	g.pushType(WASM_TYPE_I32)
}

// === Function calls ===

func (g *WasmGen) compileCall(inst Inst) {
	if len(inst.Name) > 18 && inst.Name[0:18] == "builtin.composite." {
		g.compileCompositeLitCall(inst)
		return
	}
	idx, ok := g.funcMap[inst.Name]
	if !ok {
		// Unresolved call - trap
		i := 0
		for i < inst.Arg {
			g.popType()
			g.w.drop()
			i++
		}
		g.w.unreachable()
		return
	}
	// Wrap any i64 args to i32 before call (all signatures are i32)
	// We need to check the top N args and wrap as needed
	// Pop arg types, wrap i64s, then call
	nArgs := inst.Arg
	if nArgs > 0 && len(g.valTypes) >= nArgs {
		// Check each arg from bottom to top
		baseIdx := len(g.valTypes) - nArgs
		i := baseIdx
		for i < len(g.valTypes) {
			if g.valTypes[i] == WASM_TYPE_I64 {
				// We need to insert i32.wrap_i64 at the right position
				// Since args are on stack from bottom to top, we can only
				// easily wrap the topmost. For simplicity, save top args to
				// shadow stack, wrap, and restore. But that's complex.
				// Simpler: just wrap as we pop. Let's handle it below.
				break
			}
			i++
		}
		// If any arg is i64, save all args, wrap i64s, reload
		hasI64 := false
		i = baseIdx
		for i < len(g.valTypes) {
			if g.valTypes[i] == WASM_TYPE_I64 {
				hasI64 = true
				break
			}
			i++
		}
		if hasI64 {
			// Save args to shadow stack scratch, wrapping i64 to i32
			g.w.globalGet(uint32(g.globalSP))
			g.w.i32Const(int32(nArgs * 4))
			g.w.op(OP_WASM_I32_SUB)
			g.w.globalSet(uint32(g.globalSP))

			i = nArgs - 1
			for i >= 0 {
				t := g.valTypes[baseIdx+i]
				if t == WASM_TYPE_I64 {
					g.w.i32WrapI64()
				}
				g.w.localSet(uint32(g.tempLocal))
				g.w.globalGet(uint32(g.globalSP))
				g.w.localGet(uint32(g.tempLocal))
				g.w.i32Store(2, uint32(i*4))
				i = i - 1
			}
			// Reload all as i32
			i = 0
			for i < nArgs {
				g.w.globalGet(uint32(g.globalSP))
				g.w.i32Load(2, uint32(i*4))
				i++
			}
			// Restore SP
			g.w.globalGet(uint32(g.globalSP))
			g.w.i32Const(int32(nArgs * 4))
			g.w.op(OP_WASM_I32_ADD)
			g.w.globalSet(uint32(g.globalSP))
		}
	}

	// Pop arg types
	i := 0
	for i < nArgs {
		g.popType()
		i++
	}
	g.w.call(uint32(idx))
	// Push result types (all functions return i32)
	retCount := 0
	for _, f := range g.irmod.Funcs {
		if f.Name == inst.Name {
			retCount = f.RetCount
			break
		}
	}
	i = 0
	for i < retCount {
		g.pushType(WASM_TYPE_I32)
		i++
	}
}

func (g *WasmGen) compileCompositeLitCall(inst Inst) {
	fieldCount := inst.Arg
	structSize := fieldCount * 4

	if structSize == 0 {
		// Pop all field types from valTypes
		i := 0
		for i < fieldCount {
			g.popType()
			i++
		}
		g.w.i32Const(0)
		g.pushType(WASM_TYPE_I32)
		return
	}

	// Save fields to shadow stack scratch area
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Const(int32(fieldCount * 4))
	g.w.op(OP_WASM_I32_SUB)
	g.w.globalSet(uint32(g.globalSP))

	// Pop fields from WASM stack into shadow scratch (reverse order since stack is LIFO)
	i := fieldCount - 1
	for i >= 0 {
		t := g.popType()
		if t == WASM_TYPE_I64 {
			g.w.i32WrapI64() // fields are stored as i32
		}
		g.w.localSet(uint32(g.tempLocal)) // value
		g.w.globalGet(uint32(g.globalSP))
		g.w.localGet(uint32(g.tempLocal))
		g.w.i32Store(2, uint32(i*4))
		i = i - 1
	}

	// Call runtime.Alloc(structSize)
	g.w.i32Const(int32(structSize))
	if idx, ok := g.funcMap["runtime.Alloc"]; ok {
		g.w.call(uint32(idx))
	} else {
		g.w.unreachable()
		return
	}
	g.w.localSet(uint32(g.tempLocal)) // ptr

	// Copy fields from shadow scratch to allocated struct
	i = 0
	for i < fieldCount {
		g.w.localGet(uint32(g.tempLocal))
		g.w.globalGet(uint32(g.globalSP))
		g.w.i32Load(2, uint32(i*4))
		g.w.i32Store(2, uint32(i*4))
		i++
	}

	// Restore shadow stack
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Const(int32(fieldCount * 4))
	g.w.op(OP_WASM_I32_ADD)
	g.w.globalSet(uint32(g.globalSP))

	// Push struct pointer
	g.w.localGet(uint32(g.tempLocal))
	g.pushType(WASM_TYPE_I32)
}

func (g *WasmGen) compileReturn(inst Inst) {
	// Compute frame bytes from localOffsets
	var frameBytes int32
	if g.curFrameSize > 0 {
		lastIdx := g.curFrameSize - 1
		frameBytes = g.localOffsets[lastIdx]
		if g.localI64[lastIdx] {
			frameBytes = frameBytes + 8
		} else {
			frameBytes = frameBytes + 4
		}
	}

	if frameBytes > 0 {
		retCount := g.curFunc.RetCount
		if retCount > 0 {
			// Wrap any i64 return values to i32 before saving
			scratch := g.scratchAddr
			i := retCount - 1
			for i >= 0 {
				t := g.popType()
				if t == WASM_TYPE_I64 {
					g.w.i32WrapI64()
				}
				g.w.i32Const(scratch + int32(i*4))
				g.w.localSet(uint32(g.tempLocal))
				temp2 := uint32(g.tempLocal + 1)
				g.w.localSet(temp2)
				g.w.localGet(uint32(g.tempLocal))
				g.w.localGet(temp2)
				g.w.i32Store(2, 0)
				i = i - 1
			}

			// Restore SP
			g.w.globalGet(uint32(g.globalSP))
			g.w.i32Const(frameBytes)
			g.w.op(OP_WASM_I32_ADD)
			g.w.globalSet(uint32(g.globalSP))

			// Reload return values as i32
			i = 0
			for i < retCount {
				g.w.i32Const(scratch + int32(i*4))
				g.w.i32Load(2, 0)
				i++
			}
		} else {
			g.w.globalGet(uint32(g.globalSP))
			g.w.i32Const(frameBytes)
			g.w.op(OP_WASM_I32_ADD)
			g.w.globalSet(uint32(g.globalSP))
		}
	}

	// Use WASM return instruction (works from any block depth)
	g.w.returnOp()
	g.dead = true
}

// === Intrinsics ===

func (g *WasmGen) compileCallIntrinsic(inst Inst) {
	switch inst.Name {
	case "Syscall":
		g.compileSyscallIntrinsic()
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
		g.w.unreachable()
	}
}

func (g *WasmGen) compileSliceptrIntrinsic() {
	// Param 0 (frame slot 0) = slice header ptr. Read data_ptr at [header+0].
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 0) // load param 0
	g.w.i32Load(2, 0) // load [header+0] = data_ptr
	g.pushType(WASM_TYPE_I32)
}

func (g *WasmGen) compileMakesliceIntrinsic() {
	// Params: ptr (0), len (1), cap (2)
	// Allocate 16-byte header {ptr:4, len:4, cap:4, elem_size:4}
	g.w.i32Const(16)
	if idx, ok := g.funcMap["runtime.Alloc"]; ok {
		g.w.call(uint32(idx))
	}
	g.w.localSet(uint32(g.tempLocal)) // hdr ptr

	// Fill header from params
	g.w.localGet(uint32(g.tempLocal))
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 0) // param 0 = ptr
	g.w.i32Store(2, 0) // hdr[0] = ptr

	g.w.localGet(uint32(g.tempLocal))
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 4) // param 1 = len
	g.w.i32Store(2, 4) // hdr[4] = len

	g.w.localGet(uint32(g.tempLocal))
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 8) // param 2 = cap
	g.w.i32Store(2, 8) // hdr[8] = cap

	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Const(1)
	g.w.i32Store(2, 12) // hdr[12] = elem_size = 1

	g.w.localGet(uint32(g.tempLocal)) // push header ptr
	g.pushType(WASM_TYPE_I32)
}

func (g *WasmGen) compileStringptrIntrinsic() {
	// Param 0 = string header ptr. Read data_ptr at [header+0].
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 0)  // load param 0
	g.w.i32Load(2, 0)  // load [header+0] = data_ptr
	g.pushType(WASM_TYPE_I32)
}

func (g *WasmGen) compileMakestringIntrinsic() {
	// Params: ptr (0), len (1)
	// Allocate 8-byte header {ptr:4, len:4}
	g.w.i32Const(8)
	if idx, ok := g.funcMap["runtime.Alloc"]; ok {
		g.w.call(uint32(idx))
	}
	g.w.localSet(uint32(g.tempLocal))

	g.w.localGet(uint32(g.tempLocal))
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 0) // param 0 = ptr
	g.w.i32Store(2, 0)

	g.w.localGet(uint32(g.tempLocal))
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 4) // param 1 = len
	g.w.i32Store(2, 4)

	g.w.localGet(uint32(g.tempLocal))
	g.pushType(WASM_TYPE_I32)
}

func (g *WasmGen) compileTostringIntrinsic() {
	// Param 0 = value (could be string ptr or interface box ptr)
	// Heuristic: if [ptr+0] < 256, it's a type_id (interface box)
	// Result stored in tempLocal, then pushed at the end.
	temp2 := uint32(g.tempLocal + 1)

	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 0) // param 0 = value
	g.w.localSet(uint32(g.tempLocal)) // save value ptr

	// Load first dword to check if interface box
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Load(2, 0) // first dword
	g.w.localSet(temp2) // save first_dword

	g.w.localGet(temp2)
	g.w.i32Const(256)
	g.w.op(OP_WASM_I32_GE_S) // first_dword >= 256 => string case
	g.w.ifOp(WASM_TYPE_I32)

	// String case: just return the value as-is
	g.w.localGet(uint32(g.tempLocal))

	g.w.elseOp()

	// Interface case: temp2 has type_id
	// Concrete value at [value+4]
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Load(2, 4)
	g.w.localSet(uint32(g.tempLocal)) // tempLocal = concrete value

	// type_id 2 = string: concrete value is already a string ptr
	g.w.localGet(temp2)
	g.w.i32Const(2)
	g.w.op(OP_WASM_I32_EQ)
	g.w.ifOp(WASM_TYPE_I32)
	g.w.localGet(uint32(g.tempLocal))
	g.w.elseOp()

	// type_id 1 = int: call runtime.IntToString
	g.w.localGet(temp2)
	g.w.i32Const(1)
	g.w.op(OP_WASM_I32_EQ)
	g.w.ifOp(WASM_TYPE_I32)
	g.w.localGet(uint32(g.tempLocal))
	if idx, ok := g.funcMap["runtime.IntToString"]; ok {
		g.w.call(uint32(idx))
	}
	g.w.elseOp()

	// User-defined type dispatch
	g.compileTostringDispatch(temp2)

	g.w.end() // int check
	g.w.end() // string check

	g.w.end() // interface/string if
	g.pushType(WASM_TYPE_I32)
}

func (g *WasmGen) compileTostringDispatch(typeIDLocal uint32) {
	// Generate if/else chain for Error/String methods
	// concrete value is in g.tempLocal
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

	if len(entries) == 0 {
		// Default: push 0 (nil string)
		g.w.i32Const(0)
		return
	}

	// For each entry, check type_id
	for _, entry := range entries {
		g.w.localGet(typeIDLocal)
		g.w.i32Const(int32(entry.typeID))
		g.w.op(OP_WASM_I32_EQ)
		g.w.ifOp(WASM_TYPE_I32)
		g.w.localGet(uint32(g.tempLocal)) // push concrete value as arg
		if idx, ok := g.funcMap[entry.funcName]; ok {
			g.w.call(uint32(idx))
		}
		g.w.elseOp()
	}

	// Default case: nil string
	g.w.i32Const(0)

	// Close all the nested if/else
	ei := 0
	for ei < len(entries) {
		g.w.end()
		ei++
	}
}

func (g *WasmGen) compileReadPtrIntrinsic() {
	// Param 0 = addr. Read 4 bytes.
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 0) // param 0 = addr
	g.w.i32Load(2, 0)
	g.pushType(WASM_TYPE_I32)
}

func (g *WasmGen) compileWritePtrIntrinsic() {
	// Param 0 = addr, Param 1 = val. Write 4 bytes.
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 0) // addr
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 4) // val
	g.w.i32Store(2, 0)
	// No return value
}

func (g *WasmGen) compileWriteByteIntrinsic() {
	// Param 0 = addr, Param 1 = val. Write 1 byte.
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 0) // addr
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 4) // val
	g.w.i32Store8(0, 0)
	// No return value
}

// === Syscall → WASI dispatch ===

func (g *WasmGen) compileSyscallIntrinsic() {
	// Params in shadow stack frame:
	// 0: syscall number, 1: a0, 2: a1, 3: a2, 4: a3, 5: a4, 6: a5
	//
	// Results are stored to scratch memory, then loaded at the end.
	// This avoids WASM type mismatch issues in if/else branches.
	// scratch+40: r1, scratch+44: r2, scratch+48: err

	scratch := g.scratchAddr
	r1Addr := scratch + 40
	r2Addr := scratch + 44
	errAddr := scratch + 48

	// Load syscall number
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 0) // syscall num
	g.w.localSet(uint32(g.tempLocal))

	// Dispatch on syscall number using if/else chain
	// Each branch stores r1, r2, err to scratch memory.

	// SYS_WRITE (1)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Const(1)
	g.w.op(OP_WASM_I32_EQ)
	g.w.ifOp(WASM_TYPE_VOID)
	g.compileSyscallWrite(scratch, r1Addr, r2Addr, errAddr)
	g.w.elseOp()

	// SYS_READ (0)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Const(0)
	g.w.op(OP_WASM_I32_EQ)
	g.w.ifOp(WASM_TYPE_VOID)
	g.compileSyscallRead(scratch, r1Addr, r2Addr, errAddr)
	g.w.elseOp()

	// SYS_OPEN (2)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Const(2)
	g.w.op(OP_WASM_I32_EQ)
	g.w.ifOp(WASM_TYPE_VOID)
	g.compileSyscallOpen(scratch, r1Addr, r2Addr, errAddr)
	g.w.elseOp()

	// SYS_CLOSE (3)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Const(3)
	g.w.op(OP_WASM_I32_EQ)
	g.w.ifOp(WASM_TYPE_VOID)
	g.compileSyscallClose(r1Addr, r2Addr, errAddr)
	g.w.elseOp()

	// SYS_MMAP (9)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Const(9)
	g.w.op(OP_WASM_I32_EQ)
	g.w.ifOp(WASM_TYPE_VOID)
	g.compileSyscallMmap(r1Addr, r2Addr, errAddr)
	g.w.elseOp()

	// SYS_EXIT_GROUP (231)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Const(231)
	g.w.op(OP_WASM_I32_EQ)
	g.w.ifOp(WASM_TYPE_VOID)
	g.compileSyscallExit(r1Addr, r2Addr, errAddr)
	g.w.elseOp()

	// SYS_MKDIR (83)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Const(83)
	g.w.op(OP_WASM_I32_EQ)
	g.w.ifOp(WASM_TYPE_VOID)
	g.compileSyscallMkdir(r1Addr, r2Addr, errAddr)
	g.w.elseOp()

	// SYS_RMDIR (84)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Const(84)
	g.w.op(OP_WASM_I32_EQ)
	g.w.ifOp(WASM_TYPE_VOID)
	g.compileSyscallRmdir(r1Addr, r2Addr, errAddr)
	g.w.elseOp()

	// SYS_UNLINK (87)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Const(87)
	g.w.op(OP_WASM_I32_EQ)
	g.w.ifOp(WASM_TYPE_VOID)
	g.compileSyscallUnlink(r1Addr, r2Addr, errAddr)
	g.w.elseOp()

	// SYS_GETCWD (79)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Const(79)
	g.w.op(OP_WASM_I32_EQ)
	g.w.ifOp(WASM_TYPE_VOID)
	g.compileSyscallGetcwd(r1Addr, r2Addr, errAddr)
	g.w.elseOp()

	// SYS_GETDENTS64 (217)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Const(217)
	g.w.op(OP_WASM_I32_EQ)
	g.w.ifOp(WASM_TYPE_VOID)
	g.compileSyscallGetdents(scratch, r1Addr, r2Addr, errAddr)
	g.w.elseOp()

	// Unsupported syscalls
	g.compileSyscallUnsupported(r1Addr, r2Addr, errAddr)

	// Close all if/else chains
	g.w.end() // getdents
	g.w.end() // getcwd
	g.w.end() // unlink
	g.w.end() // rmdir
	g.w.end() // mkdir
	g.w.end() // exit
	g.w.end() // mmap
	g.w.end() // close
	g.w.end() // open
	g.w.end() // read
	g.w.end() // write

	// Load results from scratch onto WASM stack
	g.w.i32Const(r1Addr)
	g.w.i32Load(2, 0)
	g.w.i32Const(r2Addr)
	g.w.i32Load(2, 0)
	g.w.i32Const(errAddr)
	g.w.i32Load(2, 0)
	g.pushType(WASM_TYPE_I32)
	g.pushType(WASM_TYPE_I32)
	g.pushType(WASM_TYPE_I32)
}

func (g *WasmGen) compileSyscallWrite(scratch int32, r1Addr int32, r2Addr int32, errAddr int32) {
	// fd_write(fd, iovs, iovs_len, nwritten) -> errno
	// a0=fd, a1=buf, a2=count
	// Build iovec at scratch: {buf_ptr:4, buf_len:4}
	g.w.i32Const(scratch)
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 8) // a1 = buf ptr
	g.w.i32Store(2, 0)

	g.w.i32Const(scratch)
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 12) // a2 = count
	g.w.i32Store(2, 4)

	// Call fd_write(fd, scratch, 1, scratch+8)
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 4) // a0 = fd
	g.w.i32Const(scratch)      // iovs
	g.w.i32Const(1)            // iovs_len
	g.w.i32Const(scratch + 8)  // nwritten ptr
	g.w.call(uint32(g.wasiFdWrite))

	// Store errno
	g.w.localSet(uint32(g.tempLocal))

	// r1 = nwritten
	g.w.i32Const(r1Addr)
	g.w.i32Const(scratch + 8)
	g.w.i32Load(2, 0)
	g.w.i32Store(2, 0)
	// r2 = 0
	g.w.i32Const(r2Addr)
	g.w.i32Const(0)
	g.w.i32Store(2, 0)
	// err
	g.w.i32Const(errAddr)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Store(2, 0)
}

func (g *WasmGen) compileSyscallRead(scratch int32, r1Addr int32, r2Addr int32, errAddr int32) {
	// fd_read(fd, iovs, iovs_len, nread) -> errno
	g.w.i32Const(scratch)
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 8) // a1 = buf
	g.w.i32Store(2, 0)

	g.w.i32Const(scratch)
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 12) // a2 = count
	g.w.i32Store(2, 4)

	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 4) // a0 = fd
	g.w.i32Const(scratch)
	g.w.i32Const(1)
	g.w.i32Const(scratch + 8)
	g.w.call(uint32(g.wasiFdRead))

	g.w.localSet(uint32(g.tempLocal))
	g.w.i32Const(r1Addr)
	g.w.i32Const(scratch + 8)
	g.w.i32Load(2, 0)
	g.w.i32Store(2, 0)
	g.w.i32Const(r2Addr)
	g.w.i32Const(0)
	g.w.i32Store(2, 0)
	g.w.i32Const(errAddr)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Store(2, 0)
}

func (g *WasmGen) compileSyscallOpen(scratch int32, r1Addr int32, r2Addr int32, errAddr int32) {
	// path_open(dirfd, dirflags, path, path_len, oflags, rights_base, rights_base_hi, rights_inheriting, rights_inheriting_hi, fdflags, fd_out)
	// a0=path_ptr (C string), a1=flags, a2=mode
	//
	// We need to compute path length from C string (null terminated)
	// path is at a0, need to find strlen

	// For WASI: the paths from our os package are already C strings (null-terminated).
	// We need the length without the null.
	// Also WASI uses capability-based paths relative to a preopened directory.
	// We use fd=3 (first preopened dir, typically .) and strip leading /.

	// Load path ptr
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 4) // a0 = path ptr
	g.w.localSet(uint32(g.tempLocal)) // path_ptr

	// Compute path length (find null terminator)
	// Use scratch+16 as length counter
	g.w.i32Const(scratch + 16)
	g.w.i32Const(0)
	g.w.i32Store(2, 0) // len = 0

	g.w.block(WASM_TYPE_VOID)
	g.w.loop(WASM_TYPE_VOID)
	// if path[len] == 0, break
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Const(scratch + 16)
	g.w.i32Load(2, 0) // len
	g.w.op(OP_WASM_I32_ADD)
	g.w.i32Load8u(0, 0)
	g.w.op(OP_WASM_I32_EQZ)
	g.w.brIf(1) // break
	// len++
	g.w.i32Const(scratch + 16)
	g.w.i32Const(scratch + 16)
	g.w.i32Load(2, 0)
	g.w.i32Const(1)
	g.w.op(OP_WASM_I32_ADD)
	g.w.i32Store(2, 0)
	g.w.br(0) // continue
	g.w.end() // loop
	g.w.end() // block

	// Check if path starts with '/' and skip it for WASI
	// scratch+20 = adjusted path, scratch+24 = adjusted len
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Load8u(0, 0)
	g.w.i32Const(47) // '/'
	g.w.op(OP_WASM_I32_EQ)
	g.w.ifOp(WASM_TYPE_VOID)
	// Skip leading /
	g.w.i32Const(scratch + 20)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Const(1)
	g.w.op(OP_WASM_I32_ADD)
	g.w.i32Store(2, 0) // adjusted path

	g.w.i32Const(scratch + 24)
	g.w.i32Const(scratch + 16)
	g.w.i32Load(2, 0)
	g.w.i32Const(1)
	g.w.op(OP_WASM_I32_SUB)
	g.w.i32Store(2, 0) // adjusted len
	g.w.elseOp()
	g.w.i32Const(scratch + 20)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Store(2, 0)
	g.w.i32Const(scratch + 24)
	g.w.i32Const(scratch + 16)
	g.w.i32Load(2, 0)
	g.w.i32Store(2, 0)
	g.w.end()

	// Convert Linux open flags to WASI oflags
	// a1 = Linux flags. WASI oflags: 0=none, 1=creat, 2=directory, 4=excl, 8=trunc
	// Linux: O_RDONLY=0, O_WRONLY=1, O_RDWR=2, O_CREAT=64, O_TRUNC=512
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 8) // a1 = Linux flags
	g.w.localSet(uint32(g.tempLocal)) // linux_flags

	temp2 := uint32(g.tempLocal + 1)

	// Compute WASI oflags using bitwise operations (no branches)
	g.w.i32Const(0) // oflags = 0

	// If O_CREAT (64), OR in 1
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Const(64)
	g.w.op(OP_WASM_I32_AND)
	g.w.i32Const(6) // shift right by 6: 64->1
	g.w.op(OP_WASM_I32_SHR_U)
	g.w.op(OP_WASM_I32_OR)

	// If O_TRUNC (512), OR in 8
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Const(512)
	g.w.op(OP_WASM_I32_AND)
	g.w.i32Const(6) // shift right by 6: 512->8
	g.w.op(OP_WASM_I32_SHR_U)
	g.w.op(OP_WASM_I32_OR)

	// If O_DIRECTORY (65536), OR in 2
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Const(65536)
	g.w.op(OP_WASM_I32_AND)
	g.w.i32Const(15) // shift right by 15: 65536->2
	g.w.op(OP_WASM_I32_SHR_U)
	g.w.op(OP_WASM_I32_OR)

	g.w.localSet(temp2) // oflags
	g.w.i32Const(3)                     // dirfd (first preopened directory)
	g.w.i32Const(0)                     // dirflags (no symlink follow)
	g.w.i32Const(scratch + 20)
	g.w.i32Load(2, 0)                   // path
	g.w.i32Const(scratch + 24)
	g.w.i32Load(2, 0)                   // path_len
	g.w.localGet(temp2)                 // oflags
	// rights_base: i64 - directory opens must NOT include FD_READ(bit1)/FD_WRITE(bit6)
	// or WASI returns EISDIR. Check oflags bit 1 (OFLAGS_DIRECTORY).
	g.w.localGet(temp2)
	g.w.i32Const(2) // OFLAGS_DIRECTORY
	g.w.op(OP_WASM_I32_AND)
	g.w.ifOp(WASM_TYPE_I64)
	g.w.op(0x42) // i64.const
	g.w.sleb(0x1fffffbd) // directory rights (no FD_READ/FD_WRITE)
	g.w.elseOp()
	g.w.op(0x42) // i64.const
	g.w.sleb(0x1fffffff) // file rights (all)
	g.w.end()
	// rights_inheriting: i64
	g.w.op(0x42)                        // i64.const
	g.w.sleb(0x1fffffff)                // rights_inheriting
	g.w.i32Const(0)                     // fdflags
	g.w.i32Const(scratch + 28)          // fd_out ptr
	g.w.call(uint32(g.wasiPathOpen))

	// errno on stack
	g.w.localSet(uint32(g.tempLocal))

	g.w.i32Const(r1Addr)
	g.w.i32Const(scratch + 28)
	g.w.i32Load(2, 0)
	g.w.i32Store(2, 0)
	g.w.i32Const(r2Addr)
	g.w.i32Const(0)
	g.w.i32Store(2, 0)
	g.w.i32Const(errAddr)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Store(2, 0)
}

func (g *WasmGen) compileSyscallClose(r1Addr int32, r2Addr int32, errAddr int32) {
	// fd_close(fd) -> errno
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 4) // a0 = fd
	g.w.call(uint32(g.wasiFdClose))

	g.w.localSet(uint32(g.tempLocal))
	g.w.i32Const(r1Addr)
	g.w.i32Const(0)
	g.w.i32Store(2, 0)
	g.w.i32Const(r2Addr)
	g.w.i32Const(0)
	g.w.i32Store(2, 0)
	g.w.i32Const(errAddr)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Store(2, 0)
}

func (g *WasmGen) compileSyscallMmap(r1Addr int32, r2Addr int32, errAddr int32) {
	// memory.grow for allocation
	// a1 = size in bytes
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 8) // a1 = size
	// Round up to pages (65536 bytes)
	g.w.i32Const(65535)
	g.w.op(OP_WASM_I32_ADD)
	g.w.i32Const(16) // shift right by 16 = divide by 65536
	g.w.op(OP_WASM_I32_SHR_U)

	g.w.op(OP_WASM_MEMORY_GROW)
	g.w.byte(0x00) // memory index 0

	// Returns previous page count or -1 on failure
	g.w.localTee(uint32(g.tempLocal))
	g.w.i32Const(-1)
	g.w.op(OP_WASM_I32_EQ)
	g.w.ifOp(WASM_TYPE_VOID)
	// Failure
	g.w.i32Const(r1Addr)
	g.w.i32Const(-1)
	g.w.i32Store(2, 0)
	g.w.i32Const(r2Addr)
	g.w.i32Const(0)
	g.w.i32Store(2, 0)
	g.w.i32Const(errAddr)
	g.w.i32Const(12)
	g.w.i32Store(2, 0)
	g.w.elseOp()
	// Success: base address = prev_pages * 65536
	g.w.i32Const(r1Addr)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Const(16)
	g.w.op(OP_WASM_I32_SHL)
	g.w.i32Store(2, 0)
	g.w.i32Const(r2Addr)
	g.w.i32Const(0)
	g.w.i32Store(2, 0)
	g.w.i32Const(errAddr)
	g.w.i32Const(0)
	g.w.i32Store(2, 0)
	g.w.end()
}

func (g *WasmGen) compileSyscallExit(r1Addr int32, r2Addr int32, errAddr int32) {
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 4) // a0 = exit code
	g.w.call(uint32(g.wasiProcExit))

	// proc_exit doesn't return, but store dummy results
	g.w.i32Const(r1Addr)
	g.w.i32Const(0)
	g.w.i32Store(2, 0)
	g.w.i32Const(r2Addr)
	g.w.i32Const(0)
	g.w.i32Store(2, 0)
	g.w.i32Const(errAddr)
	g.w.i32Const(0)
	g.w.i32Store(2, 0)
}

func (g *WasmGen) compileSyscallMkdir(r1Addr int32, r2Addr int32, errAddr int32) {
	// path_create_directory(dirfd, path, path_len) -> errno
	// a0 = path ptr (C string), a1 = mode (ignored)
	g.compileSyscallPathOp(g.wasiPathCreateDir, r1Addr, r2Addr, errAddr)
}

func (g *WasmGen) compileSyscallRmdir(r1Addr int32, r2Addr int32, errAddr int32) {
	g.compileSyscallPathOp(g.wasiPathRemoveDir, r1Addr, r2Addr, errAddr)
}

func (g *WasmGen) compileSyscallUnlink(r1Addr int32, r2Addr int32, errAddr int32) {
	g.compileSyscallPathOp(g.wasiPathUnlinkFile, r1Addr, r2Addr, errAddr)
}

// compileSyscallPathOp handles mkdir/rmdir/unlink which all take (dirfd, path, path_len).
func (g *WasmGen) compileSyscallPathOp(wasiFunc int, r1Addr int32, r2Addr int32, errAddr int32) {
	scratch := g.scratchAddr

	// Load path ptr
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 4) // a0 = path ptr
	g.w.localSet(uint32(g.tempLocal))

	// Compute strlen
	g.w.i32Const(scratch + 16)
	g.w.i32Const(0)
	g.w.i32Store(2, 0)

	g.w.block(WASM_TYPE_VOID)
	g.w.loop(WASM_TYPE_VOID)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Const(scratch + 16)
	g.w.i32Load(2, 0)
	g.w.op(OP_WASM_I32_ADD)
	g.w.i32Load8u(0, 0)
	g.w.op(OP_WASM_I32_EQZ)
	g.w.brIf(1)
	g.w.i32Const(scratch + 16)
	g.w.i32Const(scratch + 16)
	g.w.i32Load(2, 0)
	g.w.i32Const(1)
	g.w.op(OP_WASM_I32_ADD)
	g.w.i32Store(2, 0)
	g.w.br(0)
	g.w.end()
	g.w.end()

	// Strip leading /
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Load8u(0, 0)
	g.w.i32Const(47)
	g.w.op(OP_WASM_I32_EQ)
	g.w.ifOp(WASM_TYPE_VOID)
	g.w.i32Const(scratch + 20)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Const(1)
	g.w.op(OP_WASM_I32_ADD)
	g.w.i32Store(2, 0)
	g.w.i32Const(scratch + 24)
	g.w.i32Const(scratch + 16)
	g.w.i32Load(2, 0)
	g.w.i32Const(1)
	g.w.op(OP_WASM_I32_SUB)
	g.w.i32Store(2, 0)
	g.w.elseOp()
	g.w.i32Const(scratch + 20)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Store(2, 0)
	g.w.i32Const(scratch + 24)
	g.w.i32Const(scratch + 16)
	g.w.i32Load(2, 0)
	g.w.i32Store(2, 0)
	g.w.end()

	g.w.i32Const(3) // dirfd
	g.w.i32Const(scratch + 20)
	g.w.i32Load(2, 0) // path
	g.w.i32Const(scratch + 24)
	g.w.i32Load(2, 0) // path_len
	g.w.call(uint32(wasiFunc))

	g.w.localSet(uint32(g.tempLocal))
	g.w.i32Const(r1Addr)
	g.w.i32Const(0)
	g.w.i32Store(2, 0)
	g.w.i32Const(r2Addr)
	g.w.i32Const(0)
	g.w.i32Store(2, 0)
	g.w.i32Const(errAddr)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Store(2, 0)
}

func (g *WasmGen) compileSyscallGetcwd(r1Addr int32, r2Addr int32, errAddr int32) {
	// WASI has no getcwd. Write "." to the buffer.
	// a0 = buf ptr, a1 = buf size
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 4) // a0 = buf
	g.w.localSet(uint32(g.tempLocal))

	// Write ".\0" to buffer
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Const(46) // '.'
	g.w.i32Store8(0, 0)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Const(0) // null terminator
	g.w.i32Store8(0, 1)

	// r1 = 2 (length including null), r2 = 0, err = 0
	g.w.i32Const(r1Addr)
	g.w.i32Const(2)
	g.w.i32Store(2, 0)
	g.w.i32Const(r2Addr)
	g.w.i32Const(0)
	g.w.i32Store(2, 0)
	g.w.i32Const(errAddr)
	g.w.i32Const(0)
	g.w.i32Store(2, 0)
}

func (g *WasmGen) compileSyscallGetdents(scratch int32, r1Addr int32, r2Addr int32, errAddr int32) {
	// fd_readdir(fd, buf, buf_len, cookie, bufused) -> errno
	// Linux getdents64: a0=fd, a1=buf, a2=buf_size
	// We need to translate WASI dirent format to Linux getdents64 format.
	// For simplicity: call fd_readdir and translate in-place.
	//
	// Actually, this is complex. The RTG os package reads getdents64 format directly.
	// We'll use fd_readdir and translate the results.
	// WASI dirent: {d_next:8, d_ino:8, d_namlen:4, d_type:1} + name
	// Linux getdents64: {d_ino:8, d_off:8, d_reclen:2, d_type:1} + name + padding

	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 4) // a0 = fd
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 8) // a1 = buf
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 12) // a2 = buf_len

	// Use scratch for cookie (start at 0) and bufused
	// cookie: i64.const 0
	g.w.op(0x42) // i64.const
	g.w.sleb(0)  // cookie = 0

	g.w.i32Const(scratch + 32) // bufused ptr
	g.w.call(uint32(g.wasiFdReaddir))

	g.w.localSet(uint32(g.tempLocal)) // errno

	// r1 = bufused
	g.w.i32Const(r1Addr)
	g.w.i32Const(scratch + 32)
	g.w.i32Load(2, 0)
	g.w.i32Store(2, 0)
	g.w.i32Const(r2Addr)
	g.w.i32Const(0)
	g.w.i32Store(2, 0)
	g.w.i32Const(errAddr)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Store(2, 0)
}

func (g *WasmGen) compileSyscallUnsupported(r1Addr int32, r2Addr int32, errAddr int32) {
	g.w.i32Const(r1Addr)
	g.w.i32Const(0)
	g.w.i32Store(2, 0)
	g.w.i32Const(r2Addr)
	g.w.i32Const(0)
	g.w.i32Store(2, 0)
	g.w.i32Const(errAddr)
	g.w.i32Const(38) // ENOSYS
	g.w.i32Store(2, 0)
}

// === Interface dispatch ===

func (g *WasmGen) compileIfaceBox(inst Inst) {
	typeID := inst.Arg

	// Stack: [concrete_value]
	t := g.popType()
	if t == WASM_TYPE_I64 {
		g.w.i32WrapI64()
	}
	g.w.localSet(uint32(g.tempLocal)) // save concrete value

	// Allocate 8 bytes: {type_id:4, value:4}
	g.w.i32Const(8)
	if idx, ok := g.funcMap["runtime.Alloc"]; ok {
		g.w.call(uint32(idx))
	}

	temp2 := uint32(g.tempLocal + 1)
	g.w.localTee(temp2) // save box ptr

	// Store type_id
	g.w.i32Const(int32(typeID))
	g.w.i32Store(2, 0)

	// Store value
	g.w.localGet(temp2)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Store(2, 4)

	// Push box ptr
	g.w.localGet(temp2)
	g.pushType(WASM_TYPE_I32)
}

func (g *WasmGen) compileIfaceCall(inst Inst) {
	argCount := inst.Arg
	methodName := inst.Name

	// Pop all types from valTypes: args + iface_ptr
	i := 0
	for i < argCount+1 {
		g.popType()
		i++
	}

	// Stack: [iface_ptr, arg0, arg1, ...argN] (iface_ptr is deepest)
	// We need to save args, pop interface ptr, extract type_id and concrete value,
	// then push concrete value as receiver, restore args, dispatch.

	// Save args to shadow stack scratch
	if argCount > 0 {
		g.w.globalGet(uint32(g.globalSP))
		g.w.i32Const(int32(argCount * 4))
		g.w.op(OP_WASM_I32_SUB)
		g.w.globalSet(uint32(g.globalSP))

		i := argCount - 1
		for i >= 0 {
			// Save to scratch: [$sp + i*4]
			temp2 := uint32(g.tempLocal + 1)
			g.w.localSet(temp2)
			g.w.globalGet(uint32(g.globalSP))
			g.w.localGet(temp2)
			g.w.i32Store(2, uint32(i*4))
			i = i - 1
		}
	}

	// Now iface_ptr is on stack top
	g.w.localTee(uint32(g.tempLocal))
	g.w.i32Load(2, 0)  // type_id
	temp2 := uint32(g.tempLocal + 1)
	g.w.localSet(temp2) // type_id in temp2

	// Push concrete value as receiver
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Load(2, 4) // concrete value

	// Restore args from shadow scratch
	if argCount > 0 {
		i := 0
		for i < argCount {
			g.w.globalGet(uint32(g.globalSP))
			g.w.i32Load(2, uint32(i*4))
			i++
		}
		// Restore SP
		g.w.globalGet(uint32(g.globalSP))
		g.w.i32Const(int32(argCount * 4))
		g.w.op(OP_WASM_I32_ADD)
		g.w.globalSet(uint32(g.globalSP))
	}

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

	if len(entries) == 0 {
		g.w.unreachable()
	} else {
		// Stack has: [concrete_value, arg0, ...argN]
		// We need to save them all, dispatch, and have them available for each branch.
		// Actually, the values are already on the WASM stack. Each call will consume them.
		// But only one branch executes. With if/else, only one path runs.
		// However, WASM requires type-consistent stack at block boundaries.
		//
		// Strategy: save all values to shadow memory, then in each dispatch branch,
		// load them and call.
		totalVals := 1 + argCount // receiver + args
		g.w.globalGet(uint32(g.globalSP))
		g.w.i32Const(int32(totalVals * 4))
		g.w.op(OP_WASM_I32_SUB)
		g.w.globalSet(uint32(g.globalSP))

		i := totalVals - 1
		for i >= 0 {
			scratch := uint32(g.tempLocal)
			g.w.localSet(scratch)
			g.w.globalGet(uint32(g.globalSP))
			g.w.localGet(scratch)
			g.w.i32Store(2, uint32(i*4))
			i = i - 1
		}

		// Determine result count from IR's funcRets or from method table
		retCount := 0
		if len(entries) > 0 {
			// Look up the first entry's return count
			funcName := entries[0].funcName
			for _, f := range g.irmod.Funcs {
				if f.Name == funcName {
					retCount = f.RetCount
					break
				}
			}
		}

		// Build result type for if blocks
		var blockType byte
		if retCount == 0 {
			blockType = WASM_TYPE_VOID
		} else if retCount == 1 {
			blockType = WASM_TYPE_I32
		} else {
			// Multi-value blocks need a type index. For now, use void and handle via shadow stack.
			blockType = WASM_TYPE_VOID
		}

		// Dispatch chain
		for ei, entry := range entries {
			g.w.localGet(temp2) // type_id
			g.w.i32Const(int32(entry.typeID))
			g.w.op(OP_WASM_I32_EQ)

			if ei < len(entries)-1 {
				g.w.ifOp(blockType)
			} else {
				g.w.ifOp(blockType)
			}

			// Load values from scratch and call
			j := 0
			for j < totalVals {
				g.w.globalGet(uint32(g.globalSP))
				g.w.i32Load(2, uint32(j*4))
				j++
			}
			if idx, ok := g.funcMap[entry.funcName]; ok {
				g.w.call(uint32(idx))
			}

			if ei < len(entries)-1 {
				g.w.elseOp()
			}
		}

		// Default case (no match): trap
		if len(entries) > 0 {
			g.w.elseOp()
		}
		// Push dummy results for type consistency
		if retCount == 1 && blockType == WASM_TYPE_I32 {
			g.w.i32Const(0)
		}
		g.w.unreachable()

		// Close all if/else blocks
		ei := 0
		for ei < len(entries) {
			g.w.end()
			ei++
		}

		// Restore shadow stack
		g.w.globalGet(uint32(g.globalSP))
		g.w.i32Const(int32(totalVals * 4))
		g.w.op(OP_WASM_I32_ADD)
		g.w.globalSet(uint32(g.globalSP))

		// Push result types
		ri := 0
		for ri < retCount {
			g.pushType(WASM_TYPE_I32)
			ri++
		}
	}
}

// === Type conversions ===

func (g *WasmGen) compileConvert(typeName string) {
	switch typeName {
	case "string":
		g.popType()
		if idx, ok := g.funcMap["runtime.BytesToString"]; ok {
			g.w.call(uint32(idx))
		}
		g.pushType(WASM_TYPE_I32)
	case "[]byte":
		g.popType()
		if idx, ok := g.funcMap["runtime.StringToBytes"]; ok {
			g.w.call(uint32(idx))
		}
		g.pushType(WASM_TYPE_I32)
	case "uint64":
		t := g.popType()
		if t == WASM_TYPE_I32 {
			g.w.i64ExtendI32U()
		}
		g.pushType(WASM_TYPE_I64)
	case "int64":
		t := g.popType()
		if t == WASM_TYPE_I32 {
			g.w.i64ExtendI32S()
		}
		g.pushType(WASM_TYPE_I64)
	case "byte":
		t := g.popType()
		if t == WASM_TYPE_I64 {
			g.w.i32WrapI64()
		}
		g.w.i32Const(0xff)
		g.w.op(OP_WASM_I32_AND)
		g.pushType(WASM_TYPE_I32)
	case "uint16":
		t := g.popType()
		if t == WASM_TYPE_I64 {
			g.w.i32WrapI64()
		}
		g.w.i32Const(0xffff)
		g.w.op(OP_WASM_I32_AND)
		g.pushType(WASM_TYPE_I32)
	case "int", "uintptr", "uint", "int32", "uint32":
		t := g.popType()
		if t == WASM_TYPE_I64 {
			g.w.i32WrapI64()
		}
		g.pushType(WASM_TYPE_I32)
	}
}

// === Panic ===

func (g *WasmGen) compilePanic() {
	scratch := g.scratchAddr

	// Stack: [value]
	t := g.popType()
	if t == WASM_TYPE_I64 {
		g.w.i32WrapI64()
	}
	g.w.localTee(uint32(g.tempLocal))

	// Tostring heuristic: if [ptr+0] < 256, it's an interface box
	g.w.i32Load(2, 0)
	temp2 := uint32(g.tempLocal + 1)
	g.w.localTee(temp2)
	g.w.i32Const(256)
	g.w.op(OP_WASM_I32_LT_S)
	g.w.ifOp(WASM_TYPE_VOID)
	// Interface box: value at [ptr+4]
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Load(2, 4)
	g.w.localSet(uint32(g.tempLocal))
	g.w.end()

	// g.tempLocal = string header ptr
	// Write string to stderr via fd_write
	// Build iovec: {data_ptr, data_len}
	g.w.i32Const(scratch)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Load(2, 0) // data_ptr
	g.w.i32Store(2, 0)

	g.w.i32Const(scratch)
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Load(2, 4) // data_len
	g.w.i32Store(2, 4)

	// fd_write(2, scratch, 1, scratch+8)
	g.w.i32Const(2)           // fd = stderr
	g.w.i32Const(scratch)     // iovs
	g.w.i32Const(1)           // iovs_len
	g.w.i32Const(scratch + 8) // nwritten
	g.w.call(uint32(g.wasiFdWrite))
	g.w.drop() // ignore result

	// Write newline
	g.w.i32Const(scratch + 12)
	g.w.i32Const(10) // '\n'
	g.w.i32Store8(0, 0)

	g.w.i32Const(scratch)
	g.w.i32Const(scratch + 12)
	g.w.i32Store(2, 0) // iovec buf = scratch+12
	g.w.i32Const(scratch)
	g.w.i32Const(1)
	g.w.i32Store(2, 4) // iovec len = 1

	g.w.i32Const(2)
	g.w.i32Const(scratch)
	g.w.i32Const(1)
	g.w.i32Const(scratch + 8)
	g.w.call(uint32(g.wasiFdWrite))
	g.w.drop()

	// Trap
	g.w.unreachable()
	g.dead = true
}
