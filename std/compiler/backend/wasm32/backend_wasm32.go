//go:build !no_backend_wasi_wasm32

package wasm32

import (
	"fmt"
	"os"

	"j5.nz/rtg/std/compiler/backend/becommon"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// === WASM32 Backend: IR → WASM binary ===

// WasmGen holds state for generating WASM code from IR.
type WasmGen struct {
	mod     *wasmModule
	irmod   *ir.IRModule
	w       wasmCodeWriter // current function body writer
	funcMap map[string]int // IR func name → WASM func index
	entryFn string

	// WASI import function indices
	wasiFdWrite          int
	wasiFdRead           int
	wasiFdClose          int
	wasiPathOpen         int
	wasiArgsSizesGet     int
	wasiArgsGet          int
	wasiEnvironSizesGet  int
	wasiEnvironGet       int
	wasiProcExit         int
	wasiPathCreateDir    int
	wasiPathRemoveDir    int
	wasiPathUnlinkFile   int
	wasiFdReaddir        int
	wasiFdPrestatGet     int
	wasiFdPrestatDirName int
	wasiClockTimeGet     int

	// WASM global indices
	globalSP int // shadow stack pointer

	// Memory layout
	scratchAddr int32 // WASI scratch area (iovec etc.)
	globalsAddr int32 // start of global variables in linear memory
	globalsSize int32 // total bytes for globals
	stringsAddr int32 // start of string data + headers
	stringsSize int32 // total bytes for strings
	shadowBase  int32 // initial shadow stack pointer (top of shadow region)

	// String data area: raw bytes followed by per-string 8-byte headers.
	stringData []byte
	stringMap  map[string]int32

	// Current function state
	curFunc       *ir.IRFunc
	curFrameSize  int
	numParams     int
	numWasmLocals int // WASM locals beyond params (frame slots + temps)
	tempLocal     int // index of a temp i32 local for reordering
	tempLocal64   int // index of a temp i64 local for DUP of i64 values
	tempLocalF64  int // index of a temp f64 local for DUP/reordering

	// i64 type tracking
	valTypes     []byte       // type stack: WASM_TYPE_I32/I64/F64 per stack entry
	localI64     map[int]bool // which IR locals hold i64 values
	localF64     map[int]bool // which IR locals hold f64 values
	localOffsets []int32      // per-local byte offset in shadow stack frame

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

// Generate is the entry point for the WASM backend.
func Generate(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	g := &WasmGen{
		mod:       &wasmModule{memMin: 2}, // start with 2 pages (128KB)
		irmod:     irmod,
		funcMap:   make(map[string]int),
		stringMap: make(map[string]int32),
		entryFn:   common.EntryFuncName(target),
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
			params[pi] = g.wasmParamType(f, pi)
			pi++
		}
		results := make([]byte, f.RetCount)
		ri := 0
		for ri < len(results) {
			results[ri] = g.wasmResultType(f, ri)
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
		ir.FuncSizes = append(ir.FuncSizes, ir.FuncSize{Name: f.Name, Size: len(body)})
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

	// clock_time_get(clock_id: i32, precision: i64, time_ptr: i32) -> i32
	g.wasiClockTimeGet = g.mod.addImport(wasi, "clock_time_get",
		[]byte{WASM_TYPE_I32, WASM_TYPE_I64, WASM_TYPE_I32},
		[]byte{WASM_TYPE_I32})
}

// === Memory Layout ===

func (g *WasmGen) setupMemoryLayout() {
	// 0x0000 - 0x03FF: Null guard (1024 bytes)
	// 0x0400 - 0x07FF: Scratch space (WASI structs + transient stack spills)
	// 0x0800+: Global variables
	g.scratchAddr = 0x0400
	g.globalsAddr = 0x0800

	g.globalsSize = 0
	for i := range g.irmod.Globals {
		g.globalsSize = g.globalsSize + g.globalSize(i)
	}

	// String data comes after globals
	g.stringsAddr = g.globalsAddr + g.globalsSize

	// Shadow stack pointer: WASM global (mutable i32)
	// Actual value set after we know string data size
	g.globalSP = g.mod.addGlobal(WASM_TYPE_I32, true, 0) // placeholder, updated later
}

func (g *WasmGen) globalSize(idx int) int32 {
	if idx >= 0 && idx < len(g.irmod.Globals) && g.irmod.Globals[idx].Type != nil && g.irmod.Globals[idx].Type.Kind == ir.TY_FLOAT64 {
		return 8
	}
	return 4
}

func (g *WasmGen) globalOffset(idx int) int32 {
	var off int32
	i := 0
	for i < idx && i < len(g.irmod.Globals) {
		off = off + g.globalSize(i)
		i++
	}
	return off
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

func lookupStringOffsetLinear(m map[string]int32, key string) (int32, bool) {
	for k, v := range m {
		if k == key {
			return v, true
		}
	}
	return 0, false
}

func (g *WasmGen) internString(s string) int32 {
	decoded := becommon.DecodeStringLiteral(s)
	if headerOff, ok := lookupStringOffsetLinear(g.stringMap, decoded); ok {
		return headerOff + g.stringsAddr
	}

	// Append string data bytes
	dataOff := len(g.stringData)
	i := 0
	for i < len(decoded) {
		g.stringData = append(g.stringData, decoded[i])
		i++
	}

	// Append 8-byte header {data_ptr:4, len:4}
	headerOff := len(g.stringData)
	// data_ptr: absolute address = stringsAddr + dataOff
	dataAddr := g.stringsAddr + int32(dataOff)
	g.stringData = append(g.stringData, byte(dataAddr))
	g.stringData = append(g.stringData, byte(dataAddr>>8))
	g.stringData = append(g.stringData, byte(dataAddr>>16))
	g.stringData = append(g.stringData, byte(dataAddr>>24))
	lenVal := int32(len(decoded))
	g.stringData = append(g.stringData, byte(lenVal))
	g.stringData = append(g.stringData, byte(lenVal>>8))
	g.stringData = append(g.stringData, byte(lenVal>>16))
	g.stringData = append(g.stringData, byte(lenVal>>24))
	g.stringMap[decoded] = int32(headerOff)
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

func (g *WasmGen) promoteI32ToI64(unsigned bool) {
	if g.peekType() == WASM_TYPE_I32 {
		if unsigned {
			g.w.i64ExtendI32U()
		} else {
			g.w.i64ExtendI32S()
		}
		g.valTypes[len(g.valTypes)-1] = WASM_TYPE_I64
	}
}

// ensureBothSameType promotes i32 operand to i64 if the other is i64.
// Returns the common type after promotion.
func (g *WasmGen) ensureBothSameType(unsigned bool) byte {
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
		if unsigned {
			g.w.i64ExtendI32U()
		} else {
			g.w.i64ExtendI32S()
		}
		g.w.localGet(uint32(g.tempLocal64)) // restore i64 top
		g.valTypes[len(g.valTypes)-2] = WASM_TYPE_I64
		return WASM_TYPE_I64
	}
	if top == WASM_TYPE_I32 && below == WASM_TYPE_I64 {
		// Top is i32, promote it
		if unsigned {
			g.w.i64ExtendI32U()
		} else {
			g.w.i64ExtendI32S()
		}
		g.valTypes[len(g.valTypes)-1] = WASM_TYPE_I64
		return WASM_TYPE_I64
	}
	return WASM_TYPE_I32
}

// === Function Compilation ===

func (g *WasmGen) compileFunc(f *ir.IRFunc) []byte {
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

	// Initialize local type maps from IRLocal flags.
	g.localI64 = make(map[int]bool)
	g.localF64 = make(map[int]bool)
	for i, loc := range f.Locals {
		if loc.IsFloat64 {
			g.localF64[i] = true
		} else if loc.Is64 {
			g.localI64[i] = true
		}
	}

	// Compute localOffsets: f64/i64 locals get 8 bytes, others get 4.
	g.localOffsets = make([]int32, frameSize)
	var frameBytes int32
	i := 0
	for i < frameSize {
		g.localOffsets[i] = frameBytes
		if g.localI64[i] || g.localF64[i] {
			frameBytes = frameBytes + 8
		} else {
			frameBytes = frameBytes + 4
		}
		i++
	}

	// WASM locals: params are implicit (index 0..Params-1)
	// We need additional locals for:
	//   - 2 temp i32 locals for operand reordering/STORE swap
	//   - 1 temp i64 local for DUP/i64 spills
	//   - 1 temp f64 local for DUP/f64 spills
	// With shadow stack approach: ALL frame slots are in shadow stack memory.
	// WASM params are copied to shadow stack in prologue.
	g.numWasmLocals = 2 // 2 i32 temp locals (declared as first group)
	g.tempLocal = f.Params + 0
	g.tempLocal64 = f.Params + 2 // i64 temp local (declared as second group)
	g.tempLocalF64 = f.Params + 3

	// Prologue: allocate shadow stack frame
	if frameBytes > 0 {
		g.w.globalGet(uint32(g.globalSP))
		g.w.i32Const(frameBytes)
		g.w.op(OP_WASM_I32_SUB)
		g.w.globalSet(uint32(g.globalSP))
	}

	// Copy params from WASM params to shadow stack.
	if f.Params > 0 {
		i = 0
		for i < f.Params {
			g.w.globalGet(uint32(g.globalSP))
			switch g.wasmParamType(f, i) {
			case WASM_TYPE_F64:
				g.w.localGet(uint32(i))
				g.w.f64Store(3, uint32(g.localOffsets[i]))
			case WASM_TYPE_I64:
				g.w.localGet(uint32(i))
				g.w.i64Store(3, uint32(g.localOffsets[i]))
			default:
				g.w.localGet(uint32(i))
				g.w.i32Store(2, uint32(g.localOffsets[i]))
			}
			i++
		}
	}

	// Compile instructions via stackifier
	g.stackify(f.Code)

	// Build function body with local declarations.
	localCounts := []uint32{uint32(g.numWasmLocals), 1, 1}
	localTypes := []byte{WASM_TYPE_I32, WASM_TYPE_I64, WASM_TYPE_F64}
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

	// Step 2: reserve linear memory for argv data and os.Args structures.
	// We avoid calling runtime helpers here because _start does not use the
	// regular shadow-stack call setup.
	// Layout:
	//   [allocBase]                 argv pointers (argc*4)
	//   [argvBufPtr]                argv bytes (buf_size)
	//   [sliceDataPtr]              []string backing data (argc*4, string header ptrs)
	//   [strHdrBase]                per-arg string headers (argc*8)
	//   [sliceHdrPtr]               os.Args slice header (16 bytes)
	g.w.i32Const(scratch + 64)
	g.w.i32Load(2, 0) // argc
	g.w.localSet(0)   // argc
	g.w.i32Const(scratch + 68)
	g.w.i32Load(2, 0) // buf_size
	g.w.localSet(1)   // buf_size

	// total = argc*16 + buf_size + 64
	g.w.localGet(0)
	g.w.i32Const(16)
	g.w.op(OP_WASM_I32_MUL)
	g.w.localGet(1)
	g.w.op(OP_WASM_I32_ADD)
	g.w.i32Const(64)
	g.w.op(OP_WASM_I32_ADD)
	g.w.localSet(11) // total bytes

	// pages = (total + 65535) >> 16
	g.w.localGet(11)
	g.w.i32Const(65535)
	g.w.op(OP_WASM_I32_ADD)
	g.w.i32Const(16)
	g.w.op(OP_WASM_I32_SHR_U)
	g.w.localTee(11)
	g.w.op(OP_WASM_MEMORY_GROW)
	g.w.byte(0x00) // memory index 0
	g.w.i32Const(16)
	g.w.op(OP_WASM_I32_SHL)
	g.w.localSet(5) // allocBase

	// Derive region pointers from allocBase.
	g.w.localGet(5)
	g.w.localSet(6) // argvPtr

	g.w.localGet(6)
	g.w.localGet(0)
	g.w.i32Const(4)
	g.w.op(OP_WASM_I32_MUL)
	g.w.op(OP_WASM_I32_ADD)
	g.w.localSet(7) // argvBufPtr

	g.w.localGet(7)
	g.w.localGet(1)
	g.w.op(OP_WASM_I32_ADD)
	g.w.localSet(8) // sliceDataPtr

	g.w.localGet(8)
	g.w.localGet(0)
	g.w.i32Const(4)
	g.w.op(OP_WASM_I32_MUL)
	g.w.op(OP_WASM_I32_ADD)
	g.w.localSet(9) // strHdrBase

	g.w.localGet(9)
	g.w.localGet(0)
	g.w.i32Const(8)
	g.w.op(OP_WASM_I32_MUL)
	g.w.op(OP_WASM_I32_ADD)
	g.w.localSet(10) // sliceHdrPtr

	// Step 3: args_get(argvPtr, argvBufPtr)
	g.w.localGet(6)
	g.w.localGet(7)
	g.w.call(uint32(g.wasiArgsGet))
	g.w.drop()

	// Step 4: Build os.Args as []string directly in linear memory.
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
		g.w.i32Const(0)
		g.w.localSet(2) // i

		g.w.block(WASM_TYPE_VOID)
		g.w.loop(WASM_TYPE_VOID)
		// if i >= argc, break
		g.w.localGet(2)
		g.w.localGet(0) // argc
		g.w.op(OP_WASM_I32_GE_S)
		g.w.brIf(1)

		// argPtr = argvPtr[i]
		g.w.localGet(6) // argvPtr
		g.w.localGet(2) // i
		g.w.i32Const(4)
		g.w.op(OP_WASM_I32_MUL)
		g.w.op(OP_WASM_I32_ADD)
		g.w.i32Load(2, 0)
		g.w.localSet(3) // argPtr

		// Compute strlen(argPtr)
		g.w.i32Const(0)
		g.w.localSet(4) // len

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

		// strHdrPtr = strHdrBase + i*8
		g.w.localGet(9)
		g.w.localGet(2)
		g.w.i32Const(8)
		g.w.op(OP_WASM_I32_MUL)
		g.w.op(OP_WASM_I32_ADD)
		g.w.localSet(3) // strHdrPtr

		// Write string header: {ptr,len}
		g.w.localGet(3)
		g.w.localGet(6) // argvPtr
		g.w.localGet(2)
		g.w.i32Const(4)
		g.w.op(OP_WASM_I32_MUL)
		g.w.op(OP_WASM_I32_ADD)
		g.w.i32Load(2, 0)
		g.w.i32Store(2, 0)
		g.w.localGet(3)
		g.w.localGet(4)
		g.w.i32Store(2, 4)

		// sliceData[i] = strHdrPtr
		g.w.localGet(8) // sliceDataPtr
		g.w.localGet(2)
		g.w.i32Const(4)
		g.w.op(OP_WASM_I32_MUL)
		g.w.op(OP_WASM_I32_ADD)
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

		// Build slice header: {data_ptr, len, cap, elem_size}
		g.w.localGet(10) // sliceHdrPtr
		g.w.localGet(8)  // data ptr
		g.w.i32Store(2, 0)
		g.w.localGet(10)
		g.w.localGet(0) // len
		g.w.i32Store(2, 4)
		g.w.localGet(10)
		g.w.localGet(0) // cap
		g.w.i32Store(2, 8)
		g.w.localGet(10)
		g.w.i32Const(4) // sizeof(string value on wasm32)
		g.w.i32Store(2, 12)

		// Store slice header pointer into os.Args global.
		argsAddr := g.globalsAddr + g.globalOffset(argsGlobalIdx)
		g.w.i32Const(argsAddr)
		g.w.localGet(10)
		g.w.i32Store(2, 0)
	}

	// Call init functions
	for _, f := range g.irmod.Funcs {
		if ir.IsInitFunc(f.Name) {
			idx, ok := g.funcMap[f.Name]
			if ok {
				g.w.call(uint32(idx))
			}
		}
	}

	// Call entrypoint
	entryName := ir.EntryFuncName(g.irmod)
	if idx, ok := g.funcMap[entryName]; ok {
		g.w.call(uint32(idx))
	}

	// proc_exit(entryRet0OrZero)
	entryRet := ir.EntryFuncRetCount(g.irmod)
	if entryRet > 0 {
		for i := 1; i < entryRet; i++ {
			g.w.drop()
		}
	} else {
		g.w.i32Const(0)
	}
	g.w.call(uint32(g.wasiProcExit))

	// _start needs locals for args population bookkeeping.
	localCounts := []uint32{12}
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
func detectShortCircuit(code []ir.Inst, jumpPos int, end int) (targetLabel int, endLabel int, jmpToEndPos int, ok bool) {
	targetLabel = code[jumpPos].Arg
	// Find target label position within range
	targetPos := -1
	for j := jumpPos + 1; j < end; j++ {
		if code[j].Op == ir.OP_LABEL && code[j].Arg == targetLabel {
			targetPos = j
			break
		}
	}
	if targetPos < 0 || targetPos+2 >= end {
		return 0, 0, 0, false
	}
	// Check: LABEL target, CONST, LABEL end
	if code[targetPos+1].Op != ir.OP_CONST_BOOL && code[targetPos+1].Op != ir.OP_CONST_I64 {
		return 0, 0, 0, false
	}
	if code[targetPos+2].Op != ir.OP_LABEL {
		return 0, 0, 0, false
	}
	endLabel = code[targetPos+2].Arg
	// Verify JMP to endLabel immediately precedes target label
	if targetPos > 0 && code[targetPos-1].Op == ir.OP_JMP && code[targetPos-1].Arg == endLabel {
		jmpToEndPos = targetPos - 1
		return targetLabel, endLabel, jmpToEndPos, true
	}
	return 0, 0, 0, false
}

// detectPanicUnwindCheck matches panic propagation checks emitted around calls:
//
//	JMP_IF_NOT continueLabel
//	DROP...
//	JMP panicLabel
//	LABEL continueLabel
//
// The condition is produced by a prior runtime.PanicShouldUnwind call.
func detectPanicUnwindCheck(code []ir.Inst, jumpPos int, end int) (continueLabel int, panicLabel int, dropCount int, continuePos int, ok bool) {
	if code[jumpPos].Op != ir.OP_JMP_IF_NOT {
		return 0, 0, 0, 0, false
	}
	if jumpPos <= 0 {
		return 0, 0, 0, 0, false
	}
	prev := code[jumpPos-1]
	if prev.Op != ir.OP_CALL || prev.Name != "runtime.PanicShouldUnwind" {
		return 0, 0, 0, 0, false
	}

	continueLabel = code[jumpPos].Arg
	continuePos = -1
	for j := jumpPos + 1; j < end; j++ {
		if code[j].Op == ir.OP_LABEL && code[j].Arg == continueLabel {
			continuePos = j
			break
		}
	}
	if continuePos < 0 || continuePos <= jumpPos+1 {
		return 0, 0, 0, 0, false
	}

	jmpPos := continuePos - 1
	if code[jmpPos].Op != ir.OP_JMP {
		return 0, 0, 0, 0, false
	}
	panicLabel = code[jmpPos].Arg

	dropCount = 0
	for j := jumpPos + 1; j < jmpPos; j++ {
		if code[j].Op != ir.OP_DROP {
			return 0, 0, 0, 0, false
		}
		dropCount++
	}
	return continueLabel, panicLabel, dropCount, continuePos, true
}

func (g *WasmGen) stackify(code []ir.Inst) {
	// Pass 1: analyze labels using two separate maps
	loopHeaders := make(map[int]bool)
	blockTargets := make(map[int]bool)
	panicTargets := make(map[int]bool)
	for i, inst := range code {
		switch inst.Op {
		case ir.OP_JMP, ir.OP_JMP_IF, ir.OP_JMP_IF_NOT, ir.OP_JMP_EQ, ir.OP_JMP_NEQ, ir.OP_JMP_LT, ir.OP_JMP_GT, ir.OP_JMP_LEQ, ir.OP_JMP_GEQ:
			targetLabel := inst.Arg
			// Determine if forward or backward jump
			labelPos := -1
			for j, c := range code {
				if c.Op == ir.OP_LABEL {
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
		if inst.Op == ir.OP_JMP_IF_NOT {
			_, panicLabel, _, _, ok := detectPanicUnwindCheck(code, i, len(code))
			if ok {
				panicTargets[panicLabel] = true
			}
		}
	}

	// Pass 2: emit WASM structured control flow
	g.emitStructured(code, 0, len(code), loopHeaders, blockTargets, panicTargets)
}

func (g *WasmGen) emitStructured(code []ir.Inst, start int, end int, loopHeaders map[int]bool, blockTargets map[int]bool, panicTargets map[int]bool) {
	// --- Phase 1: Pre-open blocks for forward jump targets ---
	// Collect all forward jump targets at this level, skipping loop bodies
	// and short-circuit (||/&&) patterns.
	var fwdTargets []fwdTarget
	excludedLabels := make(map[int]bool) // labels in short-circuit patterns

	scanPos := start
	for scanPos < end {
		inst := code[scanPos]

		// Skip loop bodies during pre-scan
		if inst.Op == ir.OP_LABEL {
			if loopHeaders[inst.Arg] {
				breakLabel := g.findBreakLabel(code, scanPos, end, inst.Arg)
				loopEnd := end
				if breakLabel >= 0 {
					for j := scanPos + 1; j < end; j++ {
						if code[j].Op == ir.OP_LABEL && code[j].Arg == breakLabel {
							loopEnd = j
							break
						}
					}
				}
				scanPos = loopEnd
				continue
			}
		}

		// Detect panic-check patterns and short-circuit patterns, and exclude
		// labels handled by custom lowering.
		if inst.Op == ir.OP_JMP_IF_NOT {
			continueLabel, _, _, _, panicOk := detectPanicUnwindCheck(code, scanPos, end)
			if panicOk {
				excludedLabels[continueLabel] = true
				scanPos++
				continue
			}
		}
		if inst.Op == ir.OP_JMP_IF || inst.Op == ir.OP_JMP_IF_NOT {
			tgtLabel, endLabel, _, scOk := detectShortCircuit(code, scanPos, end)
			if scOk {
				excludedLabels[tgtLabel] = true
				excludedLabels[endLabel] = true
				scanPos++
				continue
			}
		}

		// Collect forward jump targets (excluding short-circuit and loop labels)
		if inst.Op == ir.OP_JMP || inst.Op == ir.OP_JMP_IF || inst.Op == ir.OP_JMP_IF_NOT || inst.Op == ir.OP_JMP_EQ || inst.Op == ir.OP_JMP_NEQ || inst.Op == ir.OP_JMP_LT || inst.Op == ir.OP_JMP_GT || inst.Op == ir.OP_JMP_LEQ || inst.Op == ir.OP_JMP_GEQ {
			targetLabel := inst.Arg
			if excludedLabels[targetLabel] {
				scanPos++
				continue
			}
			if !loopHeaders[targetLabel] {
				labelPos := -1
				for j := scanPos + 1; j < end; j++ {
					if code[j].Op == ir.OP_LABEL && code[j].Arg == targetLabel {
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

	// Ensure panic-unwind labels are available as branch targets even when
	// checks appear in nested loop bodies.
	for panicLabel := range panicTargets {
		if excludedLabels[panicLabel] {
			continue
		}
		labelPos := -1
		for j := start; j < end; j++ {
			if code[j].Op == ir.OP_LABEL && code[j].Arg == panicLabel {
				labelPos = j
				break
			}
		}
		if labelPos <= start {
			continue
		}
		dup := false
		for _, t := range fwdTargets {
			if t.labelID == panicLabel {
				dup = true
				break
			}
		}
		if !dup {
			fwdTargets = append(fwdTargets, fwdTarget{labelID: panicLabel, labelPos: labelPos})
		}
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
		case ir.OP_LABEL:
			if loopHeaders[inst.Arg] {
				breakLabel := g.findBreakLabel(code, i, end, inst.Arg)

				g.w.block(WASM_TYPE_VOID)
				g.blockStack = append(g.blockStack, wasmCtrl{kind: WASM_CTRL_BLOCK, labelID: breakLabel})

				g.w.loop(WASM_TYPE_VOID)
				g.blockStack = append(g.blockStack, wasmCtrl{kind: WASM_CTRL_LOOP, labelID: inst.Arg})

				loopEnd := end
				if breakLabel >= 0 {
					for j := i + 1; j < end; j++ {
						if code[j].Op == ir.OP_LABEL && code[j].Arg == breakLabel {
							loopEnd = j
							break
						}
					}
				}

				g.emitStructured(code, i+1, loopEnd, loopHeaders, blockTargets, panicTargets)

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
						for peek < end && code[peek].Op == ir.OP_LABEL {
							peek++
						}
						if peek < end {
							nextOp := code[peek].Op
							if nextOp == ir.OP_DUP || nextOp == ir.OP_DROP {
								g.w.localGet(uint32(g.tempLocal))
								g.pushType(WASM_TYPE_I32)
							}
						}
					}
				}
			}
			i++

		case ir.OP_JMP:
			if !g.dead {
				depth := g.findBlockDepth(inst.Arg)
				if depth >= 0 {
					g.markLiveBreak(depth)
					g.w.br(uint32(depth))
					g.dead = true
				}
			}
			i++

		case ir.OP_JMP_IF:
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
					if code[j].Op == ir.OP_LABEL && code[j].Arg == tgtLabel {
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
				g.emitStructured(code, i+1, jmpToEndPos, loopHeaders, blockTargets, panicTargets)
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

		case ir.OP_JMP_IF_NOT:
			if g.dead {
				i++
				continue
			}
			// Panic-check pattern:
			//   jmp_if_not continue
			//   drop...
			//   jmp panic
			//   label continue
			// Lower to a conditional br_if panic with explicit spill/restore
			// of the known transient values.
			_, panicLabel, dropCount, continuePos, panicOk := detectPanicUnwindCheck(code, i, end)
			if panicOk {
				g.compilePanicUnwindCheckBranch(panicLabel, dropCount)
				i = continuePos + 1
				continue
			}
			// Check for && short-circuit pattern
			tgtLabel, _, jmpToEndPos, scOk := detectShortCircuit(code, i, end)
			if scOk {
				// && pattern: if condition true → right side, else → const 0
				targetPos := -1
				for j := i + 1; j < end; j++ {
					if code[j].Op == ir.OP_LABEL && code[j].Arg == tgtLabel {
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
				g.emitStructured(code, i+1, jmpToEndPos, loopHeaders, blockTargets, panicTargets)
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

		case ir.OP_JMP_EQ, ir.OP_JMP_NEQ, ir.OP_JMP_LT, ir.OP_JMP_GT, ir.OP_JMP_LEQ, ir.OP_JMP_GEQ:
			if g.dead {
				i++
				continue
			}
			g.compileCompareJump(inst)
			depth := g.findBlockDepth(inst.Arg)
			if depth >= 0 {
				g.markLiveBreak(depth)
				g.w.brIf(uint32(depth))
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
func (g *WasmGen) findBreakLabel(code []ir.Inst, loopStart int, end int, loopLabel int) int {
	// Find the last backward JMP to the loop header.
	// The label immediately after it is the break label.
	lastJmpToLoop := -1
	for j := loopStart + 1; j < end; j++ {
		if code[j].Op == ir.OP_JMP && code[j].Arg == loopLabel {
			lastJmpToLoop = j
		}
	}
	if lastJmpToLoop >= 0 && lastJmpToLoop+1 < end {
		next := code[lastJmpToLoop+1]
		if next.Op == ir.OP_LABEL {
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

// compilePanicUnwindCheckBranch lowers the panic propagation pattern while
// preserving exactly dropCount transient values on the non-branch path.
func (g *WasmGen) compilePanicUnwindCheckBranch(targetLabel int, dropCount int) {
	g.popType() // PanicShouldUnwind result consumed by br_if

	if dropCount <= 0 {
		depth := g.findBlockDepth(targetLabel)
		if depth >= 0 {
			g.markLiveBreak(depth)
			g.w.brIf(uint32(depth))
		}
		return
	}

	savedTypes := make([]byte, dropCount)
	base := len(g.valTypes) - dropCount
	i := 0
	for i < dropCount {
		savedTypes[i] = WASM_TYPE_I32
		idx := base + i
		if idx >= 0 && idx < len(g.valTypes) {
			savedTypes[i] = g.valTypes[idx]
		}
		i++
	}

	offsets := make([]int32, dropCount)
	spillAddr := g.scratchAddr + 128
	i = 0
	for i < dropCount {
		offsets[i] = spillAddr
		if savedTypes[i] == WASM_TYPE_I64 || savedTypes[i] == WASM_TYPE_F64 {
			spillAddr = spillAddr + 8
		} else {
			spillAddr = spillAddr + 4
		}
		i++
	}

	// Save condition, then spill transient values from top to bottom.
	g.w.localSet(uint32(g.tempLocal))
	i = dropCount - 1
	for i >= 0 {
		t := savedTypes[i]
		if len(g.valTypes) > 0 {
			t = g.popType()
		}
		if t == WASM_TYPE_F64 {
			g.w.localSet(uint32(g.tempLocalF64))
			g.w.i32Const(offsets[i])
			g.w.localGet(uint32(g.tempLocalF64))
			g.w.f64Store(3, 0)
		} else if t == WASM_TYPE_I64 {
			g.w.localSet(uint32(g.tempLocal64))
			g.w.i32Const(offsets[i])
			g.w.localGet(uint32(g.tempLocal64))
			g.w.i64Store(3, 0)
		} else {
			g.w.localSet(uint32(g.tempLocal + 1))
			g.w.i32Const(offsets[i])
			g.w.localGet(uint32(g.tempLocal + 1))
			g.w.i32Store(2, 0)
		}
		i = i - 1
	}

	g.w.localGet(uint32(g.tempLocal))
	depth := g.findBlockDepth(targetLabel)
	if depth >= 0 {
		g.markLiveBreak(depth)
		g.w.brIf(uint32(depth))
	}

	// Restore values for the non-branch path.
	i = 0
	for i < dropCount {
		g.w.i32Const(offsets[i])
		if savedTypes[i] == WASM_TYPE_F64 {
			g.w.f64Load(3, 0)
		} else if savedTypes[i] == WASM_TYPE_I64 {
			g.w.i64Load(3, 0)
		} else {
			g.w.i32Load(2, 0)
		}
		g.pushType(savedTypes[i])
		i++
	}
}

// === Instruction Compilation ===

func (g *WasmGen) compileInst(inst ir.Inst) {
	switch inst.Op {
	case ir.OP_CONST_I64:
		// Some IR paths leave Width unset for 64-bit immediates. Promote
		// out-of-i32-range values to i64 even when Width metadata is absent.
		if inst.Width == 8 || inst.Val > 2147483647 || inst.Val < -2147483648 {
			g.w.i64Const(inst.Val)
			g.pushType(WASM_TYPE_I64)
		} else {
			g.w.i32Const(int32(inst.Val))
			g.pushType(WASM_TYPE_I32)
		}
	case ir.OP_CONST_F64:
		bits, ok := parseFloatLiteralBits(inst.Name)
		if !ok {
			bits = 0
		}
		g.w.f64ConstBits(bits)
		g.pushType(WASM_TYPE_F64)
	case ir.OP_CONST_BOOL:
		if inst.Arg != 0 {
			g.w.i32Const(1)
		} else {
			g.w.i32Const(0)
		}
		g.pushType(WASM_TYPE_I32)
	case ir.OP_CONST_NIL:
		g.w.i32Const(0)
		g.pushType(WASM_TYPE_I32)
	case ir.OP_CONST_STR:
		addr := g.internString(inst.Name)
		g.w.i32Const(addr)
		g.pushType(WASM_TYPE_I32)

	case ir.OP_LOCAL_GET:
		g.compileLocalGet(inst.Arg)
	case ir.OP_LOCAL_SET:
		g.compileLocalSet(inst.Arg)
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
		g.popType()
		g.w.drop()
	case ir.OP_DUP:
		g.compileDup()

	case ir.OP_ADD:
		if inst.Name == "float64" {
			g.compileBinaryOpFloat(OP_WASM_F64_ADD)
		} else {
			g.compileBinaryOp(OP_WASM_I32_ADD, OP_WASM_I64_ADD, inst.Name == "unsigned")
		}
	case ir.OP_SUB:
		if inst.Name == "float64" {
			g.compileBinaryOpFloat(OP_WASM_F64_SUB)
		} else {
			g.compileBinaryOp(OP_WASM_I32_SUB, OP_WASM_I64_SUB, inst.Name == "unsigned")
		}
	case ir.OP_MUL:
		if inst.Name == "float64" {
			g.compileBinaryOpFloat(OP_WASM_F64_MUL)
		} else {
			g.compileBinaryOp(OP_WASM_I32_MUL, OP_WASM_I64_MUL, inst.Name == "unsigned")
		}
	case ir.OP_DIV:
		if inst.Name == "float64" {
			g.compileBinaryOpFloat(OP_WASM_F64_DIV)
		} else if inst.Name == "unsigned" {
			g.compileBinaryOp(OP_WASM_I32_DIV_U, OP_WASM_I64_DIV_U, true)
		} else {
			g.compileBinaryOp(OP_WASM_I32_DIV_S, OP_WASM_I64_DIV_S, false)
		}
	case ir.OP_MOD:
		if inst.Name == "unsigned" {
			g.compileBinaryOp(OP_WASM_I32_REM_U, OP_WASM_I64_REM_U, true)
		} else {
			g.compileBinaryOp(OP_WASM_I32_REM_S, OP_WASM_I64_REM_S, false)
		}

	case ir.OP_AND:
		g.compileBinaryOp(OP_WASM_I32_AND, OP_WASM_I64_AND, inst.Name == "unsigned")
	case ir.OP_OR:
		g.compileBinaryOp(OP_WASM_I32_OR, OP_WASM_I64_OR, inst.Name == "unsigned")
	case ir.OP_XOR:
		g.compileBinaryOp(OP_WASM_I32_XOR, OP_WASM_I64_XOR, inst.Name == "unsigned")
	case ir.OP_SHL:
		g.compileBinaryOp(OP_WASM_I32_SHL, OP_WASM_I64_SHL, inst.Name == "unsigned")
	case ir.OP_SHR:
		if inst.Name == "unsigned" {
			g.compileBinaryOp(OP_WASM_I32_SHR_U, OP_WASM_I64_SHR_U, true)
		} else {
			g.compileBinaryOp(OP_WASM_I32_SHR_S, OP_WASM_I64_SHR_S, false)
		}

	case ir.OP_EQ:
		if inst.Name == "float64" {
			g.compileCompareOpFloat(OP_WASM_F64_EQ)
		} else {
			g.compileCompareOp(OP_WASM_I32_EQ, OP_WASM_I64_EQ, inst.Name == "unsigned")
		}
	case ir.OP_NEQ:
		if inst.Name == "float64" {
			g.compileCompareOpFloat(OP_WASM_F64_NE)
		} else {
			g.compileCompareOp(OP_WASM_I32_NE, OP_WASM_I64_NE, inst.Name == "unsigned")
		}
	case ir.OP_LT:
		if inst.Name == "float64" {
			g.compileCompareOpFloat(OP_WASM_F64_LT)
		} else if inst.Name == "unsigned" {
			g.compileCompareOp(OP_WASM_I32_LT_U, OP_WASM_I64_LT_U, true)
		} else {
			g.compileCompareOp(OP_WASM_I32_LT_S, OP_WASM_I64_LT_S, false)
		}
	case ir.OP_GT:
		if inst.Name == "float64" {
			g.compileCompareOpFloat(OP_WASM_F64_GT)
		} else if inst.Name == "unsigned" {
			g.compileCompareOp(OP_WASM_I32_GT_U, OP_WASM_I64_GT_U, true)
		} else {
			g.compileCompareOp(OP_WASM_I32_GT_S, OP_WASM_I64_GT_S, false)
		}
	case ir.OP_LEQ:
		if inst.Name == "float64" {
			g.compileCompareOpFloat(OP_WASM_F64_LE)
		} else if inst.Name == "unsigned" {
			g.compileCompareOp(OP_WASM_I32_LE_U, OP_WASM_I64_LE_U, true)
		} else {
			g.compileCompareOp(OP_WASM_I32_LE_S, OP_WASM_I64_LE_S, false)
		}
	case ir.OP_GEQ:
		if inst.Name == "float64" {
			g.compileCompareOpFloat(OP_WASM_F64_GE)
		} else if inst.Name == "unsigned" {
			g.compileCompareOp(OP_WASM_I32_GE_U, OP_WASM_I64_GE_U, true)
		} else {
			g.compileCompareOp(OP_WASM_I32_GE_S, OP_WASM_I64_GE_S, false)
		}

	case ir.OP_NOT:
		t := g.popType()
		if t == WASM_TYPE_I64 {
			g.w.op(OP_WASM_I64_EQZ)
		} else {
			g.w.op(OP_WASM_I32_EQZ)
		}
		g.pushType(WASM_TYPE_I32)

	case ir.OP_NEG:
		t := g.peekType()
		if inst.Name == "float64" || t == WASM_TYPE_F64 {
			g.w.op(OP_WASM_F64_NEG)
		} else if t == WASM_TYPE_I64 {
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

	case ir.OP_CALL:
		g.compileCall(inst)
	case ir.OP_CALL_INTRINSIC:
		g.compileCallIntrinsic(inst)
	case ir.OP_RETURN:
		g.compileReturn(inst)

	case ir.OP_CONVERT:
		g.compileConvert(inst.Name)

	case ir.OP_IFACE_BOX:
		g.compileIfaceBox(inst)
	case ir.OP_IFACE_CALL:
		g.compileIfaceCall(inst)
	case ir.OP_PANIC:
		g.compilePanic()

	case ir.OP_SLICE_GET, ir.OP_SLICE_MAKE, ir.OP_STRING_GET, ir.OP_STRING_MAKE:
		// Handled by intrinsics

	case ir.OP_LABEL, ir.OP_JMP, ir.OP_JMP_IF, ir.OP_JMP_IF_NOT, ir.OP_JMP_EQ, ir.OP_JMP_NEQ, ir.OP_JMP_LT, ir.OP_JMP_GT, ir.OP_JMP_LEQ, ir.OP_JMP_GEQ:
		// Handled by stackifier

	default:
		// Unknown opcode - trap
		g.w.unreachable()
	}
}

// compileBinaryOp emits a binary operation, promoting to i64 if either operand is i64.
func (g *WasmGen) compileBinaryOp(i32op byte, i64op byte, unsigned bool) {
	t := g.ensureBothSameType(unsigned)
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

func (g *WasmGen) compileBinaryOpFloat(op byte) {
	g.popType()
	g.popType()
	g.w.op(op)
	g.pushType(WASM_TYPE_F64)
}

// compileCompareOp emits a comparison, promoting to i64 if needed. Result is always i32.
func (g *WasmGen) compileCompareOp(i32op byte, i64op byte, unsigned bool) {
	t := g.ensureBothSameType(unsigned)
	g.popType()
	g.popType()
	if t == WASM_TYPE_I64 {
		g.w.op(i64op)
	} else {
		g.w.op(i32op)
	}
	g.pushType(WASM_TYPE_I32)
}

func (g *WasmGen) compileCompareOpFloat(op byte) {
	g.popType()
	g.popType()
	g.w.op(op)
	g.pushType(WASM_TYPE_I32)
}

func (g *WasmGen) compileCompareJump(inst ir.Inst) {
	if inst.Name == "float64" {
		g.popType()
		g.popType()
		switch inst.Op {
		case ir.OP_JMP_EQ:
			g.w.op(OP_WASM_F64_EQ)
		case ir.OP_JMP_NEQ:
			g.w.op(OP_WASM_F64_NE)
		case ir.OP_JMP_LT:
			g.w.op(OP_WASM_F64_LT)
		case ir.OP_JMP_GT:
			g.w.op(OP_WASM_F64_GT)
		case ir.OP_JMP_LEQ:
			g.w.op(OP_WASM_F64_LE)
		case ir.OP_JMP_GEQ:
			g.w.op(OP_WASM_F64_GE)
		}
		return
	}
	unsigned := inst.Name == "unsigned"
	t := g.ensureBothSameType(unsigned)
	g.popType()
	g.popType()
	if t == WASM_TYPE_I64 {
		switch inst.Op {
		case ir.OP_JMP_EQ:
			g.w.op(OP_WASM_I64_EQ)
		case ir.OP_JMP_NEQ:
			g.w.op(OP_WASM_I64_NE)
		case ir.OP_JMP_LT:
			if unsigned {
				g.w.op(OP_WASM_I64_LT_U)
			} else {
				g.w.op(OP_WASM_I64_LT_S)
			}
		case ir.OP_JMP_GT:
			if unsigned {
				g.w.op(OP_WASM_I64_GT_U)
			} else {
				g.w.op(OP_WASM_I64_GT_S)
			}
		case ir.OP_JMP_LEQ:
			if unsigned {
				g.w.op(OP_WASM_I64_LE_U)
			} else {
				g.w.op(OP_WASM_I64_LE_S)
			}
		case ir.OP_JMP_GEQ:
			if unsigned {
				g.w.op(OP_WASM_I64_GE_U)
			} else {
				g.w.op(OP_WASM_I64_GE_S)
			}
		}
		return
	}
	switch inst.Op {
	case ir.OP_JMP_EQ:
		g.w.op(OP_WASM_I32_EQ)
	case ir.OP_JMP_NEQ:
		g.w.op(OP_WASM_I32_NE)
	case ir.OP_JMP_LT:
		if unsigned {
			g.w.op(OP_WASM_I32_LT_U)
		} else {
			g.w.op(OP_WASM_I32_LT_S)
		}
	case ir.OP_JMP_GT:
		if unsigned {
			g.w.op(OP_WASM_I32_GT_U)
		} else {
			g.w.op(OP_WASM_I32_GT_S)
		}
	case ir.OP_JMP_LEQ:
		if unsigned {
			g.w.op(OP_WASM_I32_LE_U)
		} else {
			g.w.op(OP_WASM_I32_LE_S)
		}
	case ir.OP_JMP_GEQ:
		if unsigned {
			g.w.op(OP_WASM_I32_GE_U)
		} else {
			g.w.op(OP_WASM_I32_GE_S)
		}
	}
}

// compileDup duplicates the top of stack, using the appropriate temp local.
func (g *WasmGen) compileDup() {
	t := g.peekType()
	if t == WASM_TYPE_I64 {
		g.w.localTee(uint32(g.tempLocal64))
		g.w.localGet(uint32(g.tempLocal64))
	} else if t == WASM_TYPE_F64 {
		g.w.localTee(uint32(g.tempLocalF64))
		g.w.localGet(uint32(g.tempLocalF64))
	} else {
		g.w.localTee(uint32(g.tempLocal))
		g.w.localGet(uint32(g.tempLocal))
	}
	g.pushType(t)
}

// === Local variable access (shadow stack) ===

func (g *WasmGen) compileLocalGet(idx int) {
	offset := g.localOffsets[idx]
	if g.localF64[idx] {
		g.w.globalGet(uint32(g.globalSP))
		g.w.f64Load(3, uint32(offset))
		g.pushType(WASM_TYPE_F64)
	} else if g.localI64[idx] {
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
	if g.localF64[idx] {
		if t != WASM_TYPE_F64 {
			if t == WASM_TYPE_I64 {
				g.w.op(OP_WASM_F64_CONVERT_I64_S)
			} else {
				g.w.op(OP_WASM_F64_CONVERT_I32_S)
			}
		}
		g.w.localSet(uint32(g.tempLocalF64))
		g.w.globalGet(uint32(g.globalSP))
		g.w.localGet(uint32(g.tempLocalF64))
		g.w.f64Store(3, uint32(offset))
	} else if g.localI64[idx] {
		// Local has an 8-byte slot — use i64.store
		if t == WASM_TYPE_I32 {
			g.w.i64ExtendI32U() // promote to i64
		}
		g.w.localSet(uint32(g.tempLocal64))
		g.w.globalGet(uint32(g.globalSP))
		g.w.localGet(uint32(g.tempLocal64))
		g.w.i64Store(3, uint32(offset))
	} else {
		// Local has a 4-byte slot — always wrap to i32
		if t == WASM_TYPE_I64 {
			g.w.i32WrapI64()
		}
		g.w.localSet(uint32(g.tempLocal))
		g.w.globalGet(uint32(g.globalSP))
		g.w.localGet(uint32(g.tempLocal))
		g.w.i32Store(2, uint32(offset))
	}
}

func (g *WasmGen) compileLocalAddImm(idx int, imm int32) {
	g.compileLocalGet(idx)
	if g.peekType() == WASM_TYPE_F64 {
		g.w.f64ConstBits(float64BitsFromI32(imm))
		g.pushType(WASM_TYPE_F64)
		g.compileBinaryOpFloat(OP_WASM_F64_ADD)
	} else if g.peekType() == WASM_TYPE_I64 {
		g.w.i64Const(int64(imm))
		g.pushType(WASM_TYPE_I64)
		g.compileBinaryOp(OP_WASM_I32_ADD, OP_WASM_I64_ADD, false)
	} else {
		g.w.i32Const(imm)
		g.pushType(WASM_TYPE_I32)
		g.compileBinaryOp(OP_WASM_I32_ADD, OP_WASM_I64_ADD, false)
	}
	g.compileLocalSet(idx)
}

func (g *WasmGen) compileLocalAddr(idx int) {
	offset := g.localOffsets[idx]
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Const(offset)
	g.w.op(OP_WASM_I32_ADD)
	g.pushType(WASM_TYPE_I32)
}

// === Global variable access (linear memory) ===

func (g *WasmGen) compileGlobalGet(inst ir.Inst) {
	if inst.Arg >= 0 && inst.Arg < len(g.irmod.Globals) && g.irmod.Globals[inst.Arg].Type != nil && g.irmod.Globals[inst.Arg].Type.Kind == ir.TY_FLOAT64 {
		g.w.i32Const(g.globalsAddr + g.globalOffset(inst.Arg))
		g.w.f64Load(3, 0)
		g.pushType(WASM_TYPE_F64)
		return
	}
	g.w.i32Const(g.globalsAddr + g.globalOffset(inst.Arg))
	g.w.i32Load(2, 0)
	g.pushType(WASM_TYPE_I32)
}

func (g *WasmGen) compileGlobalSet(inst ir.Inst) {
	t := g.popType()
	isFloat := inst.Arg >= 0 && inst.Arg < len(g.irmod.Globals) && g.irmod.Globals[inst.Arg].Type != nil && g.irmod.Globals[inst.Arg].Type.Kind == ir.TY_FLOAT64
	if isFloat {
		if t != WASM_TYPE_F64 {
			if t == WASM_TYPE_I64 {
				g.w.op(OP_WASM_F64_CONVERT_I64_S)
			} else {
				g.w.op(OP_WASM_F64_CONVERT_I32_S)
			}
		}
		g.w.localSet(uint32(g.tempLocalF64))
		g.w.i32Const(g.globalsAddr + g.globalOffset(inst.Arg))
		g.w.localGet(uint32(g.tempLocalF64))
		g.w.f64Store(3, 0)
		return
	}
	if t == WASM_TYPE_I64 {
		g.w.i32WrapI64()
	}
	g.w.localSet(uint32(g.tempLocal))
	g.w.i32Const(g.globalsAddr + g.globalOffset(inst.Arg))
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Store(2, 0)
}

func (g *WasmGen) compileGlobalAddr(inst ir.Inst) {
	g.w.i32Const(g.globalsAddr + g.globalOffset(inst.Arg))
	g.pushType(WASM_TYPE_I32)
}

// === Memory operations ===

func (g *WasmGen) compileLoad(inst ir.Inst) {
	size := inst.Arg
	offset := inst.Val
	memOffset := uint32(0)
	useMemOffset := false
	if offset >= 0 && offset <= 0xFFFFFFFF {
		memOffset = uint32(offset)
		useMemOffset = true
	}
	// Stack: [addr] → [value]
	t := g.popType()
	if t == WASM_TYPE_I64 {
		g.w.i32WrapI64()
	}
	if offset != 0 {
		if !ir.IsNonNilMemoryBase(inst.Name) {
			// Preserve IR semantics: nil-guarded LOAD checks the effective
			// address after OP_OFFSET, not the original base pointer.
			g.w.i32Const(int32(offset))
			g.w.op(OP_WASM_I32_ADD)
			memOffset = 0
			useMemOffset = false
		} else if !useMemOffset {
			g.w.i32Const(int32(offset))
			g.w.op(OP_WASM_I32_ADD)
		}
	}
	if ir.IsNonNilMemoryBase(inst.Name) {
		if size == 8 && inst.Name == "float64" {
			g.w.f64Load(3, memOffset)
			g.pushType(WASM_TYPE_F64)
		} else if size == 8 {
			g.w.i64Load(3, memOffset)
			g.pushType(WASM_TYPE_I64)
		} else if size == 1 {
			g.w.i32Load8u(0, memOffset)
			g.pushType(WASM_TYPE_I32)
		} else {
			g.w.i32Load(2, memOffset)
			g.pushType(WASM_TYPE_I32)
		}
		return
	}
	g.w.localTee(uint32(g.tempLocal))
	g.w.op(OP_WASM_I32_EQZ)
	blockType := byte(WASM_TYPE_I32)
	if size == 8 && inst.Name == "float64" {
		blockType = WASM_TYPE_F64
	} else if size == 8 {
		blockType = WASM_TYPE_I64
	}
	g.w.ifOp(blockType)
	if blockType == WASM_TYPE_F64 {
		g.w.f64ConstBits(0)
	} else if blockType == WASM_TYPE_I64 {
		g.w.i64Const(0)
	} else {
		g.w.i32Const(0)
	}
	g.w.elseOp()
	g.w.localGet(uint32(g.tempLocal))
	if size == 8 && inst.Name == "float64" {
		g.w.f64Load(3, memOffset)
	} else if size == 8 {
		g.w.i64Load(3, memOffset)
	} else if size == 1 {
		g.w.i32Load8u(0, memOffset)
	} else {
		g.w.i32Load(2, memOffset)
	}
	g.w.end()
	if size == 8 && inst.Name == "float64" {
		g.pushType(WASM_TYPE_F64)
	} else if size == 8 {
		g.pushType(WASM_TYPE_I64)
	} else {
		g.pushType(WASM_TYPE_I32)
	}
}

func (g *WasmGen) compileStore(inst ir.Inst) {
	size := inst.Arg
	offset := inst.Val
	memOffset := uint32(0)
	useMemOffset := false
	if offset >= 0 && offset <= 0xFFFFFFFF {
		memOffset = uint32(offset)
		useMemOffset = true
	}
	// IR stack: [value, addr] (addr on top)
	// WASM i32.store wants: [addr, value]
	addrType := g.popType()
	if addrType == WASM_TYPE_I64 {
		g.w.i32WrapI64()
	}
	g.w.localSet(uint32(g.tempLocal)) // temp0 = addr

	// value is below.
	vt := g.popType()
	temp2 := uint32(g.tempLocal + 1)
	if size == 8 && inst.Name == "float64" {
		if vt != WASM_TYPE_F64 {
			if vt == WASM_TYPE_I64 {
				g.w.op(OP_WASM_F64_CONVERT_I64_S)
			} else {
				g.w.op(OP_WASM_F64_CONVERT_I32_S)
			}
		}
		g.w.localSet(uint32(g.tempLocalF64))
	} else if size == 8 {
		if vt == WASM_TYPE_I32 {
			g.w.i64ExtendI32U()
		}
		g.w.localSet(uint32(g.tempLocal64))
	} else {
		if vt == WASM_TYPE_I64 {
			g.w.i32WrapI64()
		}
		g.w.localSet(temp2)
	}
	g.w.localGet(uint32(g.tempLocal))
	if offset != 0 && !useMemOffset {
		g.w.i32Const(int32(offset))
		g.w.op(OP_WASM_I32_ADD)
	}
	if size == 8 && inst.Name == "float64" {
		g.w.localGet(uint32(g.tempLocalF64))
		g.w.f64Store(3, memOffset)
	} else if size == 8 {
		g.w.localGet(uint32(g.tempLocal64))
		g.w.i64Store(3, memOffset)
	} else {
		g.w.localGet(temp2)
		if size == 1 {
			g.w.i32Store8(0, memOffset)
		} else {
			g.w.i32Store(2, memOffset)
		}
	}
}

func wasmValueSize(t byte) int32 {
	if t == WASM_TYPE_I64 || t == WASM_TYPE_F64 {
		return 8
	}
	return 4
}

func (g *WasmGen) wasmLocalType(loc ir.IRLocal) byte {
	if loc.IsFloat64 {
		return WASM_TYPE_F64
	}
	if loc.Is64 {
		return WASM_TYPE_I64
	}
	return WASM_TYPE_I32
}

func (g *WasmGen) wasmParamType(f *ir.IRFunc, idx int) byte {
	if f != nil && idx >= 0 && idx < len(f.Locals) {
		return g.wasmLocalType(f.Locals[idx])
	}
	return WASM_TYPE_I32
}

func (g *WasmGen) wasmResultType(f *ir.IRFunc, idx int) byte {
	if f != nil && idx >= 0 && idx < len(f.ResultKinds) && f.ResultKinds[idx] == ir.TY_FLOAT64 {
		return WASM_TYPE_F64
	}
	if f != nil && idx >= 0 && idx < len(f.ResultIs64) && f.ResultIs64[idx] {
		return WASM_TYPE_I64
	}
	return WASM_TYPE_I32
}

func (g *WasmGen) compileOffset(inst ir.Inst) {
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
	g.popType()                       // pop sliceHdrPtr type
	g.w.i32Load(2, 0)                 // load data_ptr from header[0]
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

func (g *WasmGen) compileLen(inst ir.Inst) {
	// Stack: [headerPtr] → [length]
	g.popType() // pop headerPtr
	if ir.IsNonNilMemoryBase(inst.Name) {
		g.w.i32Load(2, 4) // len at offset 4
		g.pushType(WASM_TYPE_I32)
		return
	}
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

func (g *WasmGen) compileCap(inst ir.Inst) {
	// Stack: [headerPtr] → [capacity]
	g.popType() // pop headerPtr
	if ir.IsNonNilMemoryBase(inst.Name) {
		g.w.i32Load(2, 8) // cap at offset 8 (2*4)
		g.pushType(WASM_TYPE_I32)
		return
	}
	g.w.localTee(uint32(g.tempLocal))
	g.w.op(OP_WASM_I32_EQZ)
	g.w.ifOp(WASM_TYPE_I32)
	g.w.i32Const(0)
	g.w.elseOp()
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Load(2, 8) // cap at offset 8 (2*4)
	g.w.end()
	g.pushType(WASM_TYPE_I32)
}

// === Function calls ===

func (g *WasmGen) compileCall(inst ir.Inst) {
	if len(inst.Name) > 18 && inst.Name[0:18] == "builtin.composite." {
		g.compileCompositeLitCall(inst)
		return
	}
	var callee *ir.IRFunc
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
	for _, f := range g.irmod.Funcs {
		if f.Name == inst.Name {
			callee = f
			break
		}
	}
	nArgs := inst.Arg
	baseIdx := len(g.valTypes) - nArgs
	if baseIdx < 0 {
		baseIdx = 0
	}
	needMarshal := false
	i := 0
	for i < nArgs && baseIdx+i < len(g.valTypes) {
		if g.valTypes[baseIdx+i] != g.wasmParamType(callee, i) {
			needMarshal = true
			break
		}
		i++
	}
	if needMarshal {
		argTypes := make([]byte, nArgs)
		offsets := make([]int32, nArgs)
		var totalBytes int32
		i = 0
		for i < nArgs {
			t := byte(WASM_TYPE_I32)
			if baseIdx+i >= 0 && baseIdx+i < len(g.valTypes) {
				t = g.valTypes[baseIdx+i]
			}
			argTypes[i] = t
			offsets[i] = totalBytes
			totalBytes = totalBytes + wasmValueSize(t)
			i++
		}

		g.w.globalGet(uint32(g.globalSP))
		g.w.i32Const(totalBytes)
		g.w.op(OP_WASM_I32_SUB)
		g.w.globalSet(uint32(g.globalSP))

		i = nArgs - 1
		for i >= 0 {
			t := g.popType()
			switch t {
			case WASM_TYPE_F64:
				g.w.localSet(uint32(g.tempLocalF64))
				g.w.globalGet(uint32(g.globalSP))
				g.w.i32Const(offsets[i])
				g.w.op(OP_WASM_I32_ADD)
				g.w.localGet(uint32(g.tempLocalF64))
				g.w.f64Store(3, 0)
			case WASM_TYPE_I64:
				g.w.localSet(uint32(g.tempLocal64))
				g.w.globalGet(uint32(g.globalSP))
				g.w.i32Const(offsets[i])
				g.w.op(OP_WASM_I32_ADD)
				g.w.localGet(uint32(g.tempLocal64))
				g.w.i64Store(3, 0)
			default:
				g.w.localSet(uint32(g.tempLocal))
				g.w.globalGet(uint32(g.globalSP))
				g.w.i32Const(offsets[i])
				g.w.op(OP_WASM_I32_ADD)
				g.w.localGet(uint32(g.tempLocal))
				g.w.i32Store(2, 0)
			}
			i = i - 1
		}

		i = 0
		for i < nArgs {
			want := g.wasmParamType(callee, i)
			got := argTypes[i]
			g.w.globalGet(uint32(g.globalSP))
			g.w.i32Const(offsets[i])
			g.w.op(OP_WASM_I32_ADD)
			if want == WASM_TYPE_F64 {
				if got == WASM_TYPE_F64 {
					g.w.f64Load(3, 0)
				} else if got == WASM_TYPE_I64 {
					g.w.i64Load(3, 0)
					g.w.op(OP_WASM_F64_CONVERT_I64_S)
				} else {
					g.w.i32Load(2, 0)
					g.w.op(OP_WASM_F64_CONVERT_I32_S)
				}
			} else if want == WASM_TYPE_I64 {
				if got == WASM_TYPE_F64 {
					g.w.f64Load(3, 0)
					g.w.op(OP_WASM_I64_TRUNC_F64_S)
				} else if got == WASM_TYPE_I64 {
					g.w.i64Load(3, 0)
				} else {
					g.w.i32Load(2, 0)
					g.w.i64ExtendI32S()
				}
			} else {
				if got == WASM_TYPE_F64 {
					g.w.f64Load(3, 0)
					g.w.op(OP_WASM_I32_TRUNC_F64_S)
				} else if got == WASM_TYPE_I64 {
					g.w.i64Load(3, 0)
					g.w.i32WrapI64()
				} else {
					g.w.i32Load(2, 0)
				}
			}
			i++
		}

		g.w.globalGet(uint32(g.globalSP))
		g.w.i32Const(totalBytes)
		g.w.op(OP_WASM_I32_ADD)
		g.w.globalSet(uint32(g.globalSP))
	} else {
		i = 0
		for i < nArgs {
			g.popType()
			i++
		}
	}
	g.w.call(uint32(idx))
	retCount := 0
	if callee != nil {
		retCount = callee.RetCount
		i = 0
		for i < retCount {
			g.pushType(g.wasmResultType(callee, i))
			i++
		}
	}
}

func (g *WasmGen) compileCompositeLitCall(inst ir.Inst) {
	fieldCount := inst.Arg
	fieldTypes := make([]byte, fieldCount)
	fieldOffsets := make([]int32, fieldCount)
	var structSize int32
	baseIdx := len(g.valTypes) - fieldCount
	if baseIdx < 0 {
		baseIdx = 0
	}
	i := 0
	for i < fieldCount {
		t := byte(WASM_TYPE_I32)
		if baseIdx+i >= 0 && baseIdx+i < len(g.valTypes) {
			t = g.valTypes[baseIdx+i]
		}
		fieldTypes[i] = t
		fieldOffsets[i] = structSize
		structSize = structSize + wasmValueSize(t)
		i++
	}

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
	g.w.i32Const(structSize)
	g.w.op(OP_WASM_I32_SUB)
	g.w.globalSet(uint32(g.globalSP))

	// Pop fields from WASM stack into shadow scratch (reverse order since stack is LIFO)
	i = fieldCount - 1
	for i >= 0 {
		t := g.popType()
		g.w.globalGet(uint32(g.globalSP))
		g.w.i32Const(fieldOffsets[i])
		g.w.op(OP_WASM_I32_ADD)
		switch t {
		case WASM_TYPE_F64:
			g.w.localSet(uint32(g.tempLocal))
			g.w.localSet(uint32(g.tempLocalF64))
			g.w.localGet(uint32(g.tempLocal))
			g.w.localGet(uint32(g.tempLocalF64))
			g.w.f64Store(3, 0)
		case WASM_TYPE_I64:
			g.w.localSet(uint32(g.tempLocal))
			g.w.localSet(uint32(g.tempLocal64))
			g.w.localGet(uint32(g.tempLocal))
			g.w.localGet(uint32(g.tempLocal64))
			g.w.i64Store(3, 0)
		default:
			g.w.localSet(uint32(g.tempLocal + 1))
			g.w.localSet(uint32(g.tempLocal))
			g.w.localGet(uint32(g.tempLocal + 1))
			g.w.localGet(uint32(g.tempLocal))
			g.w.i32Store(2, 0)
		}
		i = i - 1
	}

	// Call runtime.Alloc(structSize)
	g.w.i32Const(structSize)
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
		if fieldOffsets[i] != 0 {
			g.w.i32Const(fieldOffsets[i])
			g.w.op(OP_WASM_I32_ADD)
		}
		switch fieldTypes[i] {
		case WASM_TYPE_F64:
			g.w.globalGet(uint32(g.globalSP))
			g.w.i32Const(fieldOffsets[i])
			g.w.op(OP_WASM_I32_ADD)
			g.w.f64Load(3, 0)
			g.w.f64Store(3, 0)
		case WASM_TYPE_I64:
			g.w.globalGet(uint32(g.globalSP))
			g.w.i32Const(fieldOffsets[i])
			g.w.op(OP_WASM_I32_ADD)
			g.w.i64Load(3, 0)
			g.w.i64Store(3, 0)
		default:
			g.w.globalGet(uint32(g.globalSP))
			g.w.i32Const(fieldOffsets[i])
			g.w.op(OP_WASM_I32_ADD)
			g.w.i32Load(2, 0)
			g.w.i32Store(2, 0)
		}
		i++
	}

	// Restore shadow stack
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Const(structSize)
	g.w.op(OP_WASM_I32_ADD)
	g.w.globalSet(uint32(g.globalSP))

	// Push struct pointer
	g.w.localGet(uint32(g.tempLocal))
	g.pushType(WASM_TYPE_I32)
}

func (g *WasmGen) compileReturn(inst ir.Inst) {
	// Compute frame bytes from localOffsets
	var frameBytes int32
	if g.curFrameSize > 0 {
		lastIdx := g.curFrameSize - 1
		frameBytes = g.localOffsets[lastIdx]
		if g.localI64[lastIdx] || g.localF64[lastIdx] {
			frameBytes = frameBytes + 8
		} else {
			frameBytes = frameBytes + 4
		}
	}

	retCount := g.curFunc.RetCount
	scratch := g.scratchAddr
	if retCount > 0 {
		i := retCount - 1
		for i >= 0 {
			t := g.popType()
			addr := scratch + int32(i*8)
			switch t {
			case WASM_TYPE_F64:
				g.w.localSet(uint32(g.tempLocalF64))
				g.w.i32Const(addr)
				g.w.localGet(uint32(g.tempLocalF64))
				g.w.f64Store(3, 0)
			case WASM_TYPE_I64:
				g.w.localSet(uint32(g.tempLocal64))
				g.w.i32Const(addr)
				g.w.localGet(uint32(g.tempLocal64))
				g.w.i64Store(3, 0)
			default:
				g.w.localSet(uint32(g.tempLocal))
				g.w.i32Const(addr)
				g.w.localGet(uint32(g.tempLocal))
				g.w.i32Store(2, 0)
			}
			i = i - 1
		}
	}

	if frameBytes > 0 {
		g.w.globalGet(uint32(g.globalSP))
		g.w.i32Const(frameBytes)
		g.w.op(OP_WASM_I32_ADD)
		g.w.globalSet(uint32(g.globalSP))
	}

	if retCount > 0 {
		i := 0
		for i < retCount {
			g.w.i32Const(scratch + int32(i*8))
			switch g.wasmResultType(g.curFunc, i) {
			case WASM_TYPE_F64:
				g.w.f64Load(3, 0)
			case WASM_TYPE_I64:
				g.w.i64Load(3, 0)
			default:
				g.w.i32Load(2, 0)
			}
			i++
		}
	}

	// Use WASM return instruction (works from any block depth)
	g.w.returnOp()
	g.dead = true
}

// === Intrinsics ===

func (g *WasmGen) compileCallIntrinsic(inst ir.Inst) {
	switch inst.Name {
	case "SysWrite":
		scratch := g.scratchAddr
		r1Addr := scratch + 40
		r2Addr := scratch + 44
		errAddr := scratch + 48
		g.compileSyscallWrite(scratch, r1Addr, r2Addr, errAddr)
		g.w.i32Const(r1Addr)
		g.w.i32Load(2, 0)
		g.w.i32Const(r2Addr)
		g.w.i32Load(2, 0)
		g.w.i32Const(errAddr)
		g.w.i32Load(2, 0)
		g.pushType(WASM_TYPE_I32)
		g.pushType(WASM_TYPE_I32)
		g.pushType(WASM_TYPE_I32)
	case "SysRead":
		scratch := g.scratchAddr
		r1Addr := scratch + 40
		r2Addr := scratch + 44
		errAddr := scratch + 48
		g.compileSyscallRead(scratch, r1Addr, r2Addr, errAddr)
		g.w.i32Const(r1Addr)
		g.w.i32Load(2, 0)
		g.w.i32Const(r2Addr)
		g.w.i32Load(2, 0)
		g.w.i32Const(errAddr)
		g.w.i32Load(2, 0)
		g.pushType(WASM_TYPE_I32)
		g.pushType(WASM_TYPE_I32)
		g.pushType(WASM_TYPE_I32)
	case "SysOpen":
		scratch := g.scratchAddr
		r1Addr := scratch + 40
		r2Addr := scratch + 44
		errAddr := scratch + 48
		g.compileSyscallOpen(scratch, r1Addr, r2Addr, errAddr)
		g.w.i32Const(r1Addr)
		g.w.i32Load(2, 0)
		g.w.i32Const(r2Addr)
		g.w.i32Load(2, 0)
		g.w.i32Const(errAddr)
		g.w.i32Load(2, 0)
		g.pushType(WASM_TYPE_I32)
		g.pushType(WASM_TYPE_I32)
		g.pushType(WASM_TYPE_I32)
	case "SysClose":
		scratch := g.scratchAddr
		r1Addr := scratch + 40
		r2Addr := scratch + 44
		errAddr := scratch + 48
		g.compileSyscallClose(r1Addr, r2Addr, errAddr)
		g.w.i32Const(r1Addr)
		g.w.i32Load(2, 0)
		g.w.i32Const(r2Addr)
		g.w.i32Load(2, 0)
		g.w.i32Const(errAddr)
		g.w.i32Load(2, 0)
		g.pushType(WASM_TYPE_I32)
		g.pushType(WASM_TYPE_I32)
		g.pushType(WASM_TYPE_I32)
	case "SysMmap":
		scratch := g.scratchAddr
		r1Addr := scratch + 40
		r2Addr := scratch + 44
		errAddr := scratch + 48
		g.compileSyscallMmap(r1Addr, r2Addr, errAddr)
		g.w.i32Const(r1Addr)
		g.w.i32Load(2, 0)
		g.w.i32Const(r2Addr)
		g.w.i32Load(2, 0)
		g.w.i32Const(errAddr)
		g.w.i32Load(2, 0)
		g.pushType(WASM_TYPE_I32)
		g.pushType(WASM_TYPE_I32)
		g.pushType(WASM_TYPE_I32)
	case "SysExit":
		scratch := g.scratchAddr
		r1Addr := scratch + 40
		r2Addr := scratch + 44
		errAddr := scratch + 48
		g.compileSyscallExit(r1Addr, r2Addr, errAddr)
	case "SysMkdir":
		scratch := g.scratchAddr
		r1Addr := scratch + 40
		r2Addr := scratch + 44
		errAddr := scratch + 48
		g.compileSyscallMkdir(r1Addr, r2Addr, errAddr)
		g.w.i32Const(r1Addr)
		g.w.i32Load(2, 0)
		g.w.i32Const(r2Addr)
		g.w.i32Load(2, 0)
		g.w.i32Const(errAddr)
		g.w.i32Load(2, 0)
		g.pushType(WASM_TYPE_I32)
		g.pushType(WASM_TYPE_I32)
		g.pushType(WASM_TYPE_I32)
	case "SysRmdir":
		scratch := g.scratchAddr
		r1Addr := scratch + 40
		r2Addr := scratch + 44
		errAddr := scratch + 48
		g.compileSyscallRmdir(r1Addr, r2Addr, errAddr)
		g.w.i32Const(r1Addr)
		g.w.i32Load(2, 0)
		g.w.i32Const(r2Addr)
		g.w.i32Load(2, 0)
		g.w.i32Const(errAddr)
		g.w.i32Load(2, 0)
		g.pushType(WASM_TYPE_I32)
		g.pushType(WASM_TYPE_I32)
		g.pushType(WASM_TYPE_I32)
	case "SysUnlink":
		scratch := g.scratchAddr
		r1Addr := scratch + 40
		r2Addr := scratch + 44
		errAddr := scratch + 48
		g.compileSyscallUnlink(r1Addr, r2Addr, errAddr)
		g.w.i32Const(r1Addr)
		g.w.i32Load(2, 0)
		g.w.i32Const(r2Addr)
		g.w.i32Load(2, 0)
		g.w.i32Const(errAddr)
		g.w.i32Load(2, 0)
		g.pushType(WASM_TYPE_I32)
		g.pushType(WASM_TYPE_I32)
		g.pushType(WASM_TYPE_I32)
	case "SysGetcwd":
		scratch := g.scratchAddr
		r1Addr := scratch + 40
		r2Addr := scratch + 44
		errAddr := scratch + 48
		g.compileSyscallGetcwd(r1Addr, r2Addr, errAddr)
		g.w.i32Const(r1Addr)
		g.w.i32Load(2, 0)
		g.w.i32Const(r2Addr)
		g.w.i32Load(2, 0)
		g.w.i32Const(errAddr)
		g.w.i32Load(2, 0)
		g.pushType(WASM_TYPE_I32)
		g.pushType(WASM_TYPE_I32)
		g.pushType(WASM_TYPE_I32)
	case "SysGetdents64":
		scratch := g.scratchAddr
		r1Addr := scratch + 40
		r2Addr := scratch + 44
		errAddr := scratch + 48
		g.compileSyscallGetdents(scratch, r1Addr, r2Addr, errAddr)
		g.w.i32Const(r1Addr)
		g.w.i32Load(2, 0)
		g.w.i32Const(r2Addr)
		g.w.i32Load(2, 0)
		g.w.i32Const(errAddr)
		g.w.i32Load(2, 0)
		g.pushType(WASM_TYPE_I32)
		g.pushType(WASM_TYPE_I32)
		g.pushType(WASM_TYPE_I32)
	case "SysGetpid":
		// WASI has no getpid; return 0
		g.w.i32Const(0)
		g.pushType(WASM_TYPE_I32)
		g.w.i32Const(0)
		g.pushType(WASM_TYPE_I32)
		g.w.i32Const(0)
		g.pushType(WASM_TYPE_I32)
	case "SysNanoTime":
		// clock_time_get(CLOCK_MONOTONIC=1, precision=1ns, out_ptr)
		scratch := g.scratchAddr
		tmpErr := uint32(g.tempLocal)
		g.w.i32Const(1)
		g.w.i64Const(1)
		g.w.i32Const(scratch)
		g.w.call(uint32(g.wasiClockTimeGet))
		g.w.localSet(tmpErr)
		g.w.localGet(tmpErr)
		g.w.ifOp(WASM_TYPE_I32)
		g.w.i32Const(0)
		g.w.elseOp()
		g.w.i32Const(scratch)
		g.w.i64Load(3, 0)
		g.w.i32WrapI64()
		g.w.end()
		g.pushType(WASM_TYPE_I32)
	case "Alloc":
		g.compileAllocIntrinsic()
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

func (g *WasmGen) compileAllocIntrinsic() {
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 0)
	if idx, ok := g.funcMap["runtime.Alloc"]; ok {
		g.w.call(uint32(idx))
		g.pushType(WASM_TYPE_I32)
		return
	}
	g.w.unreachable()
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
	g.w.i32Load(2, 0)  // param 0 = ptr
	g.w.i32Store(2, 0) // hdr[0] = ptr

	g.w.localGet(uint32(g.tempLocal))
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 4)  // param 1 = len
	g.w.i32Store(2, 4) // hdr[4] = len

	g.w.localGet(uint32(g.tempLocal))
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 8)  // param 2 = cap
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
	g.w.i32Load(2, 0) // load param 0
	g.w.i32Load(2, 0) // load [header+0] = data_ptr
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
	g.w.i32Load(2, 0)                 // param 0 = value
	g.w.localSet(uint32(g.tempLocal)) // save value ptr

	// Load first dword to check if interface box
	g.w.localGet(uint32(g.tempLocal))
	g.w.i32Load(2, 0)   // first dword
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
	var entries []becommon.DispatchEntry
	findFunc := func(name string) *ir.IRFunc {
		if g.irmod == nil {
			return nil
		}
		for _, f := range g.irmod.Funcs {
			if f.Name == name {
				return f
			}
		}
		return nil
	}
	if g.irmod != nil && g.irmod.TypeIDs != nil {
		for typeName, tid := range g.irmod.TypeIDs {
			candidate := typeName + ".Error"
			if fnName, ok := becommon.LookupStringMapLinear(g.irmod.MethodTable, candidate); ok {
				fn := findFunc(fnName)
				if fn != nil && fn.Params == 1 && fn.RetCount == 1 {
					entries = append(entries, becommon.DispatchEntry{tid, fnName})
				}
				continue
			}
			candidate = typeName + ".String"
			if fnName, ok := becommon.LookupStringMapLinear(g.irmod.MethodTable, candidate); ok {
				fn := findFunc(fnName)
				if fn != nil && fn.Params == 1 && fn.RetCount == 1 {
					entries = append(entries, becommon.DispatchEntry{tid, fnName})
				}
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
		g.w.i32Const(int32(entry.TypeID))
		g.w.op(OP_WASM_I32_EQ)
		g.w.ifOp(WASM_TYPE_I32)
		if idx, ok := g.funcMap[entry.FuncName]; ok {
			g.w.localGet(uint32(g.tempLocal)) // push concrete value as arg
			g.w.call(uint32(idx))
		} else {
			// Keep if/else block type-consistent even if method body was
			// removed from the lowered module.
			g.w.i32Const(0)
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

// === Syscall → WASI helpers ===

func (g *WasmGen) compileSyscallWrite(scratch int32, r1Addr int32, r2Addr int32, errAddr int32) {
	// fd_write(fd, iovs, iovs_len, nwritten) -> errno
	// params: fd=SP+0, buf=SP+4, count=SP+8
	// Build iovec at scratch: {buf_ptr:4, buf_len:4}
	g.w.i32Const(scratch)
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 4) // buf ptr
	g.w.i32Store(2, 0)

	g.w.i32Const(scratch)
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 8) // count
	g.w.i32Store(2, 4)

	// Call fd_write(fd, scratch, 1, scratch+8)
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 0)         // fd
	g.w.i32Const(scratch)     // iovs
	g.w.i32Const(1)           // iovs_len
	g.w.i32Const(scratch + 8) // nwritten ptr
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
	// params: fd=SP+0, buf=SP+4, count=SP+8
	g.w.i32Const(scratch)
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 4) // buf
	g.w.i32Store(2, 0)

	g.w.i32Const(scratch)
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 8) // count
	g.w.i32Store(2, 4)

	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 0) // fd
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
	// params: path_ptr=SP+0 (C string), flags=SP+4, mode=SP+8
	//
	// We need to compute path length from C string (null terminated)
	// path is at param 0, need to find strlen

	// For WASI: the paths from our os package are already C strings (null-terminated).
	// We need the length without the null.
	// Also WASI uses capability-based paths relative to a preopened directory.
	// We use fd=3 (first preopened dir, typically .) and strip leading /.

	// Load path ptr
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 0)                 // path ptr
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
	// flags = SP+4. WASI oflags: 0=none, 1=creat, 2=directory, 4=excl, 8=trunc
	// Linux: O_RDONLY=0, O_WRONLY=1, O_RDWR=2, O_CREAT=64, O_TRUNC=512
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 4)                 // flags
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
	g.w.i32Const(3)     // dirfd (first preopened directory)
	g.w.i32Const(0)     // dirflags (no symlink follow)
	g.w.i32Const(scratch + 20)
	g.w.i32Load(2, 0) // path
	g.w.i32Const(scratch + 24)
	g.w.i32Load(2, 0)   // path_len
	g.w.localGet(temp2) // oflags
	// rights_base: i64 - directory opens must NOT include FD_READ(bit1)/FD_WRITE(bit6)
	// or WASI returns EISDIR. Check oflags bit 1 (OFLAGS_DIRECTORY).
	g.w.localGet(temp2)
	g.w.i32Const(2) // OFLAGS_DIRECTORY
	g.w.op(OP_WASM_I32_AND)
	g.w.ifOp(WASM_TYPE_I64)
	g.w.op(0x42)         // i64.const
	g.w.sleb(0x1fffffbd) // directory rights (no FD_READ/FD_WRITE)
	g.w.elseOp()
	g.w.op(0x42)         // i64.const
	g.w.sleb(0x1fffffff) // file rights (all)
	g.w.end()
	// rights_inheriting: i64
	g.w.op(0x42)               // i64.const
	g.w.sleb(0x1fffffff)       // rights_inheriting
	g.w.i32Const(0)            // fdflags
	g.w.i32Const(scratch + 28) // fd_out ptr
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
	// params: fd=SP+0
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 0) // fd
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
	// params: addr=SP+0, length=SP+4, prot=SP+8, flags=SP+12, fd=SP+16, offset=SP+20
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 4) // length
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
	// params: code=SP+0
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 0) // exit code
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
	// params: path=SP+0 (C string), mode=SP+4 (ignored)
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
	// params: path=SP+0
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 0) // path ptr
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
	// params: buf=SP+0, size=SP+4
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 0) // buf
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
	// Linux getdents64: fd=SP+0, buf=SP+4, buf_size=SP+8
	// We need to translate WASI dirent format to Linux getdents64 format.
	// For simplicity: call fd_readdir and translate in-place.
	//
	// Actually, this is complex. The RTG os package reads getdents64 format directly.
	// We'll use fd_readdir and translate the results.
	// WASI dirent: {d_next:8, d_ino:8, d_namlen:4, d_type:1} + name
	// Linux getdents64: {d_ino:8, d_off:8, d_reclen:2, d_type:1} + name + padding

	// params: fd=SP+0, buf=SP+4, buf_len=SP+8
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 0) // fd
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 4) // buf
	g.w.globalGet(uint32(g.globalSP))
	g.w.i32Load(2, 8) // buf_len

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

func (g *WasmGen) compileIfaceBox(inst ir.Inst) {
	typeID := inst.Arg

	// Stack: [concrete_value]
	t := g.popType()
	boxSize := int32(8)
	if t == WASM_TYPE_I64 || t == WASM_TYPE_F64 {
		boxSize = 12
	}
	switch t {
	case WASM_TYPE_F64:
		g.w.localSet(uint32(g.tempLocalF64))
	case WASM_TYPE_I64:
		g.w.localSet(uint32(g.tempLocal64))
	default:
		g.w.localSet(uint32(g.tempLocal))
	}

	// Allocate {type_id:4, value:4|8}.
	g.w.i32Const(boxSize)
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
	switch t {
	case WASM_TYPE_F64:
		g.w.localGet(uint32(g.tempLocalF64))
		g.w.f64Store(3, 4)
	case WASM_TYPE_I64:
		g.w.localGet(uint32(g.tempLocal64))
		g.w.i64Store(3, 4)
	default:
		g.w.localGet(uint32(g.tempLocal))
		g.w.i32Store(2, 4)
	}

	// Push box ptr
	g.w.localGet(temp2)
	g.pushType(WASM_TYPE_I32)
}

func (g *WasmGen) compileIfaceCall(inst ir.Inst) {
	argCount := inst.Arg
	methodName := inst.Name
	argTypes := make([]byte, argCount)
	argOffsets := make([]int32, argCount)
	baseIdx := len(g.valTypes) - argCount
	if baseIdx < 0 {
		baseIdx = 0
	}
	var argBytes int32
	i := 0
	for i < argCount {
		t := byte(WASM_TYPE_I32)
		if baseIdx+i >= 0 && baseIdx+i < len(g.valTypes) {
			t = g.valTypes[baseIdx+i]
		}
		argTypes[i] = t
		argOffsets[i] = argBytes
		argBytes = argBytes + wasmValueSize(t)
		i++
	}

	// Pop all types from valTypes: args + iface_ptr
	i = 0
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
		g.w.i32Const(argBytes)
		g.w.op(OP_WASM_I32_SUB)
		g.w.globalSet(uint32(g.globalSP))

		i := argCount - 1
		for i >= 0 {
			g.w.globalGet(uint32(g.globalSP))
			g.w.i32Const(argOffsets[i])
			g.w.op(OP_WASM_I32_ADD)
			switch argTypes[i] {
			case WASM_TYPE_F64:
				g.w.localSet(uint32(g.tempLocal))
				g.w.localSet(uint32(g.tempLocalF64))
				g.w.localGet(uint32(g.tempLocal))
				g.w.localGet(uint32(g.tempLocalF64))
				g.w.f64Store(3, 0)
			case WASM_TYPE_I64:
				g.w.localSet(uint32(g.tempLocal))
				g.w.localSet(uint32(g.tempLocal64))
				g.w.localGet(uint32(g.tempLocal))
				g.w.localGet(uint32(g.tempLocal64))
				g.w.i64Store(3, 0)
			default:
				g.w.localSet(uint32(g.tempLocal + 1))
				g.w.localSet(uint32(g.tempLocal))
				g.w.localGet(uint32(g.tempLocal + 1))
				g.w.localGet(uint32(g.tempLocal))
				g.w.i32Store(2, 0)
			}
			i = i - 1
		}
	}

	// Now iface_ptr is on stack top
	g.w.localTee(uint32(g.tempLocal))
	g.w.i32Load(2, 0) // type_id
	temp2 := uint32(g.tempLocal + 1)
	g.w.localSet(temp2) // type_id in temp2

	// Extract interface and bare method names using the last '.'
	// so fully-qualified names like "main.Stringer.String" are handled.
	dotIdx := len(methodName) - 1
	for dotIdx >= 0 {
		if methodName[dotIdx] == '.' {
			break
		}
		dotIdx--
	}
	ifaceName := ""
	bareMethod := methodName
	if dotIdx >= 0 {
		ifaceName = methodName[:dotIdx]
		if dotIdx+1 < len(methodName) {
			bareMethod = methodName[dotIdx+1:]
		}
	}

	// Expected return count from interface signature (if known)
	expectedRetCount := -1
	if ifaceName != "" && g.irmod != nil && g.irmod.IfaceMethodRets != nil {
		if ret, ok := g.irmod.IfaceMethodRets[ifaceName+"\x00"+bareMethod]; ok {
			expectedRetCount = ret
		}
	}

	// Collect dispatch entries with signature filtering
	var entries []becommon.DispatchEntry
	var entryFuncs []*ir.IRFunc
	var resultTypes []byte
	if g.irmod != nil && g.irmod.TypeIDs != nil {
		for typeName, tid := range g.irmod.TypeIDs {
			candidate := typeName + "." + bareMethod
			funcName, ok := becommon.LookupStringMapLinear(g.irmod.MethodTable, candidate)
			if !ok {
				continue
			}

			var fn *ir.IRFunc
			for _, f := range g.irmod.Funcs {
				if f.Name == funcName {
					fn = f
					break
				}
			}
			if fn == nil {
				continue
			}

			// Receiver + regular args must match exactly.
			if fn.Params != argCount+1 {
				continue
			}

			// If interface signature is known, enforce return count.
			// Otherwise infer it from the first match and keep it consistent.
			if expectedRetCount >= 0 {
				if fn.RetCount != expectedRetCount {
					continue
				}
			} else {
				expectedRetCount = fn.RetCount
			}

			if resultTypes == nil {
				resultTypes = make([]byte, fn.RetCount)
				ri := 0
				for ri < fn.RetCount {
					resultTypes[ri] = g.wasmResultType(fn, ri)
					ri++
				}
			} else if len(resultTypes) != fn.RetCount {
				continue
			} else {
				match := true
				ri := 0
				for ri < fn.RetCount {
					if resultTypes[ri] != g.wasmResultType(fn, ri) {
						match = false
						break
					}
					ri++
				}
				if !match {
					continue
				}
			}

			entries = append(entries, becommon.DispatchEntry{tid, funcName})
			entryFuncs = append(entryFuncs, fn)
		}
	}

	if len(entries) == 0 {
		g.w.unreachable()
	} else {
		retCount := 0
		if expectedRetCount > 0 {
			retCount = expectedRetCount
		}
		var retScratchBytes int32
		retOffsets := make([]int32, retCount)
		if retCount > 1 {
			ri := 0
			for ri < retCount {
				retOffsets[ri] = retScratchBytes
				if ri < len(resultTypes) {
					retScratchBytes = retScratchBytes + wasmValueSize(resultTypes[ri])
				} else {
					retScratchBytes = retScratchBytes + 4
				}
				ri++
			}
			// Reserve scratch for multi-value dispatch results.
			g.w.globalGet(uint32(g.globalSP))
			g.w.i32Const(retScratchBytes)
			g.w.op(OP_WASM_I32_SUB)
			g.w.globalSet(uint32(g.globalSP))
		}

		// Build result type for if blocks
		var blockType byte
		if retCount == 0 {
			blockType = WASM_TYPE_VOID
		} else if retCount == 1 {
			if len(resultTypes) > 0 {
				blockType = resultTypes[0]
			} else {
				blockType = WASM_TYPE_I32
			}
		} else {
			// Multi-value blocks use shadow scratch and a void block.
			blockType = WASM_TYPE_VOID
		}

		// Dispatch chain
		for ei, entry := range entries {
			fn := entryFuncs[ei]
			g.w.localGet(temp2) // type_id
			g.w.i32Const(int32(entry.TypeID))
			g.w.op(OP_WASM_I32_EQ)

			g.w.ifOp(blockType)

			recvType := g.wasmParamType(fn, 0)
			g.w.localGet(uint32(g.tempLocal))
			if recvType == WASM_TYPE_F64 {
				g.w.f64Load(3, 4)
			} else {
				g.w.i32Load(2, 4)
			}

			// Load values from scratch and call
			j := 0
			for j < argCount {
				want := g.wasmParamType(fn, j+1)
				got := argTypes[j]
				g.w.globalGet(uint32(g.globalSP))
				g.w.i32Const(retScratchBytes + argOffsets[j])
				g.w.op(OP_WASM_I32_ADD)
				if want == WASM_TYPE_F64 {
					if got == WASM_TYPE_F64 {
						g.w.f64Load(3, 0)
					} else if got == WASM_TYPE_I64 {
						g.w.i64Load(3, 0)
						g.w.op(OP_WASM_F64_CONVERT_I64_S)
					} else {
						g.w.i32Load(2, 0)
						g.w.op(OP_WASM_F64_CONVERT_I32_S)
					}
				} else if want == WASM_TYPE_I64 {
					if got == WASM_TYPE_F64 {
						g.w.f64Load(3, 0)
						g.w.op(OP_WASM_I64_TRUNC_F64_S)
					} else if got == WASM_TYPE_I64 {
						g.w.i64Load(3, 0)
					} else {
						g.w.i32Load(2, 0)
						g.w.i64ExtendI32S()
					}
				} else {
					if got == WASM_TYPE_F64 {
						g.w.f64Load(3, 0)
						g.w.op(OP_WASM_I32_TRUNC_F64_S)
					} else if got == WASM_TYPE_I64 {
						g.w.i64Load(3, 0)
						g.w.i32WrapI64()
					} else {
						g.w.i32Load(2, 0)
					}
				}
				j++
			}
			if idx, ok := g.funcMap[entry.FuncName]; ok {
				g.w.call(uint32(idx))
			}
			if retCount > 1 {
				// Store all returned values into ret scratch so void if/else
				// blocks remain type-consistent.
				ri := retCount - 1
				for ri >= 0 {
					g.w.globalGet(uint32(g.globalSP))
					g.w.i32Const(retOffsets[ri])
					g.w.op(OP_WASM_I32_ADD)
					rt := byte(WASM_TYPE_I32)
					if ri < len(resultTypes) {
						rt = resultTypes[ri]
					}
					switch rt {
					case WASM_TYPE_F64:
						g.w.localSet(uint32(g.tempLocal))
						g.w.localSet(uint32(g.tempLocalF64))
						g.w.localGet(uint32(g.tempLocal))
						g.w.localGet(uint32(g.tempLocalF64))
						g.w.f64Store(3, 0)
					case WASM_TYPE_I64:
						g.w.localSet(uint32(g.tempLocal))
						g.w.localSet(uint32(g.tempLocal64))
						g.w.localGet(uint32(g.tempLocal))
						g.w.localGet(uint32(g.tempLocal64))
						g.w.i64Store(3, 0)
					default:
						g.w.localSet(uint32(g.tempLocal + 1))
						g.w.localSet(uint32(g.tempLocal))
						g.w.localGet(uint32(g.tempLocal + 1))
						g.w.localGet(uint32(g.tempLocal))
						g.w.i32Store(2, 0)
					}
					ri = ri - 1
				}
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
		if retCount == 1 {
			if blockType == WASM_TYPE_F64 {
				g.w.f64ConstBits(0)
			} else {
				g.w.i32Const(0)
			}
		}
		g.w.unreachable()

		// Close all if/else blocks
		ei := 0
		for ei < len(entries) {
			g.w.end()
			ei++
		}

		// Rehydrate multi-return values from ret scratch.
		if retCount > 1 {
			ri := 0
			for ri < retCount {
				g.w.globalGet(uint32(g.globalSP))
				g.w.i32Const(retOffsets[ri])
				g.w.op(OP_WASM_I32_ADD)
				if ri < len(resultTypes) && resultTypes[ri] == WASM_TYPE_F64 {
					g.w.f64Load(3, 0)
				} else if ri < len(resultTypes) && resultTypes[ri] == WASM_TYPE_I64 {
					g.w.i64Load(3, 0)
				} else {
					g.w.i32Load(2, 0)
				}
				ri++
			}
		}

		// Restore shadow stack
		g.w.globalGet(uint32(g.globalSP))
		g.w.i32Const(argBytes + retScratchBytes)
		g.w.op(OP_WASM_I32_ADD)
		g.w.globalSet(uint32(g.globalSP))

		// Push result types
		ri := 0
		for ri < retCount {
			if ri < len(resultTypes) {
				g.pushType(resultTypes[ri])
			} else {
				g.pushType(WASM_TYPE_I32)
			}
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
		if t == WASM_TYPE_F64 {
			g.w.op(OP_WASM_I64_TRUNC_F64_S)
		} else if t == WASM_TYPE_I32 {
			g.w.i64ExtendI32U()
		}
		g.pushType(WASM_TYPE_I64)
	case "int64":
		t := g.popType()
		if t == WASM_TYPE_F64 {
			g.w.op(OP_WASM_I64_TRUNC_F64_S)
		} else if t == WASM_TYPE_I32 {
			g.w.i64ExtendI32S()
		}
		g.pushType(WASM_TYPE_I64)
	case "float64":
		t := g.popType()
		if t == WASM_TYPE_I64 {
			g.w.op(OP_WASM_F64_CONVERT_I64_S)
		} else if t == WASM_TYPE_I32 {
			g.w.op(OP_WASM_F64_CONVERT_I32_S)
		}
		g.pushType(WASM_TYPE_F64)
	case "byte":
		t := g.popType()
		if t == WASM_TYPE_F64 {
			g.w.op(OP_WASM_I32_TRUNC_F64_S)
		} else if t == WASM_TYPE_I64 {
			g.w.i32WrapI64()
		}
		g.w.i32Const(0xff)
		g.w.op(OP_WASM_I32_AND)
		g.pushType(WASM_TYPE_I32)
	case "uint8":
		t := g.popType()
		if t == WASM_TYPE_F64 {
			g.w.op(OP_WASM_I32_TRUNC_F64_S)
		} else if t == WASM_TYPE_I64 {
			g.w.i32WrapI64()
		}
		g.w.i32Const(0xff)
		g.w.op(OP_WASM_I32_AND)
		g.pushType(WASM_TYPE_I32)
	case "int8":
		t := g.popType()
		if t == WASM_TYPE_F64 {
			g.w.op(OP_WASM_I32_TRUNC_F64_S)
		} else if t == WASM_TYPE_I64 {
			g.w.i32WrapI64()
		}
		g.w.i32Const(24)
		g.w.op(OP_WASM_I32_SHL)
		g.w.i32Const(24)
		g.w.op(OP_WASM_I32_SHR_S)
		g.pushType(WASM_TYPE_I32)
	case "uint16":
		t := g.popType()
		if t == WASM_TYPE_F64 {
			g.w.op(OP_WASM_I32_TRUNC_F64_S)
		} else if t == WASM_TYPE_I64 {
			g.w.i32WrapI64()
		}
		g.w.i32Const(0xffff)
		g.w.op(OP_WASM_I32_AND)
		g.pushType(WASM_TYPE_I32)
	case "int16":
		t := g.popType()
		if t == WASM_TYPE_F64 {
			g.w.op(OP_WASM_I32_TRUNC_F64_S)
		} else if t == WASM_TYPE_I64 {
			g.w.i32WrapI64()
		}
		g.w.i32Const(16)
		g.w.op(OP_WASM_I32_SHL)
		g.w.i32Const(16)
		g.w.op(OP_WASM_I32_SHR_S)
		g.pushType(WASM_TYPE_I32)
	case "int", "uintptr", "uint", "int32", "uint32":
		t := g.popType()
		if t == WASM_TYPE_F64 {
			g.w.op(OP_WASM_I32_TRUNC_F64_S)
		} else if t == WASM_TYPE_I64 {
			g.w.i32WrapI64()
		}
		g.pushType(WASM_TYPE_I32)
	}
}

func mul10Checked(v uint64) (uint64, bool) {
	if v > ^uint64(0)/10 {
		return 0, false
	}
	return v * 10, true
}

func float64BitsFromI32(v int32) uint64 {
	if v == 0 {
		return 0
	}
	sign := uint64(0)
	u := uint64(v)
	if v < 0 {
		sign = uint64(1) << 63
		u = uint64(-int64(v))
	}
	exp := 0
	tmp := u
	for tmp > 1 {
		tmp = tmp >> 1
		exp++
	}
	mant := (u << uint(52-exp)) & ((uint64(1) << 52) - 1)
	return sign | (uint64(exp+1023) << 52) | mant
}

func parseFloatLiteralBits(s string) (uint64, bool) {
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
		mant, ok = mul10Checked(mant)
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
		num, ok = mul10Checked(num)
		if !ok {
			return 0, false
		}
		exp10--
	}
	for exp10 < 0 {
		var ok bool
		den, ok = mul10Checked(den)
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
	if expBits >= 0x7ff {
		return sign | (uint64(0x7ff) << 52), true
	}
	return sign | (uint64(expBits) << 52) | mantBits, true
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
