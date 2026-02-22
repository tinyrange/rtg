//go:build !no_backend_vm

package vm

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"j5.nz/rtg/std/compiler/backend/becommon"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// === VM Backend: IR interpreter ===

var ExitCode int

// VMConfig holds word/pointer size configuration.
type VMConfig struct {
	WordSize  int
	PtrSize   int
	WordMask  uint64
	SignBit   uint64
	ShiftMask uint64
}

func newVMConfig(wordSize int) VMConfig {
	ptrSize := wordSize
	if ptrSize < 2 {
		ptrSize = 2
	}
	bits := wordSize * 8
	var wordMask uint64
	if bits >= 64 {
		wordMask = 0xFFFFFFFFFFFFFFFF
	} else {
		wordMask = (1 << uint(bits)) - 1
	}
	return VMConfig{
		WordSize:  wordSize,
		PtrSize:   ptrSize,
		WordMask:  wordMask,
		SignBit:   1 << uint(bits-1),
		ShiftMask: uint64(bits - 1),
	}
}

// VM is the abstract machine state.
type VM struct {
	config       VMConfig
	targetGOOS   string
	targetGOARCH string

	// Operand stack
	stack []uint64
	sp    int

	// Flat byte-addressable memory
	memory  []byte
	memNext int

	// Globals base address in VM memory
	globalsAddr uint64
	numGlobals  int

	// Function lookup
	funcs map[string]*ir.IRFunc

	// String literal interning
	stringAddrs map[string]uint64

	// Interface dispatch
	dispatch       []vmDispatchEntry
	methodIDs      map[string]int
	errorMethodID  int
	stringMethodID int

	// Special functions
	intToStringFunc   *ir.IRFunc
	bytesToStringFunc *ir.IRFunc
	stringToBytesFunc *ir.IRFunc

	// Host I/O: fd table uses parallel slices
	fdFiles   []*os.File
	fdUsed    []bool
	fdIsPopen []bool
	nextFD    int

	// Pre-allocated argv strings
	argvAddrs []uint64

	// Directory handles: parallel slices
	dirEntries [][]os.DirEntry
	dirPos     []int
	dirUsed    []bool
	nextDirID  int

	// Frame stack: dedicated region for function call frames
	frameStackBase int
	frameStackTop  int
	frameStackSize int

	// Slab allocator for fixed-size objects
	slabPageSize int

	slabSmallSize int
	slabSmallBump int
	slabSmallEnd  int
	slabSmallFree uint64

	slabLargeSize int
	slabLargeBump int
	slabLargeEnd  int
	slabLargeFree uint64

	// Reusable scratch area for intrinsic argument marshalling.
	intrinsicScratchAddr  uint64
	intrinsicScratchWords int

	// Compile-time assembler state for //rtg:assemble builders.
	asmAmd64Code   []byte
	asmAmd64Fixups []vmAsmFixup
	asmAmd64Armed  bool

	// Execution state
	exited bool

	// Debug: step counting
	stepCount int64
	stepLimit int64
	callStack []string
	stackHWM  int

	// Memory tracking (RTG_VM_MEM=1 summary, RTG_VM_ALLOC=1 per-alloc log)
	trackMem    bool
	logAllocs   bool
	heapAllocs  int64
	allocCount  int64
	frameHWM    int64
	callCount   int64
	tagBytes    map[string]int64
	tagCount    map[string]int64
	callerBytes map[string]int64
	callerCount map[string]int64
}

type vmDispatchEntry struct {
	typeID   int
	methodID int
	funcName string
}

type vmAsmFixup struct {
	Off    int
	Target string
}

// generateVM interprets the IR module directly.
func Generate(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	cfg := newVMConfig(target.WordSize)
	// Null guard region: small for small address spaces
	guard := 0x10000
	if cfg.PtrSize <= 2 {
		guard = 0x100
	}
	vm := &VM{
		config:       cfg,
		targetGOOS:   target.GOOS,
		targetGOARCH: target.GOARCH,
		stack:        make([]uint64, 0, 4096),
		memory:       make([]byte, 256*1024),
		memNext:      guard,
		funcs:        make(map[string]*ir.IRFunc),
		stringAddrs:  make(map[string]uint64),
		methodIDs:    make(map[string]int),
		fdFiles:      make([]*os.File, 256),
		fdUsed:       make([]bool, 256),
		fdIsPopen:    make([]bool, 256),
		nextFD:       3,
		dirEntries:   make([][]os.DirEntry, 64),
		dirPos:       make([]int, 64),
		dirUsed:      make([]bool, 64),
		nextDirID:    1,
	}

	// RTG_VM_STEPS=N: halt after N instructions and dump call stack
	stepEnv := os.Getenv("RTG_VM_STEPS")
	if stepEnv != "" {
		n := int64(0)
		si := 0
		for si < len(stepEnv) {
			ch := stepEnv[si]
			if ch >= '0' && ch <= '9' {
				n = n*10 + int64(ch-'0')
			}
			si = si + 1
		}
		vm.stepLimit = n
	}

	// Set up stdio
	vm.fdFiles[0] = os.Stdin
	vm.fdUsed[0] = true
	vm.fdFiles[1] = os.Stdout
	vm.fdUsed[1] = true
	vm.fdFiles[2] = os.Stderr
	vm.fdUsed[2] = true

	// RTG_VM_MEM=1: print memory summary at exit
	// RTG_VM_ALLOC=1: also log each heap allocation
	if os.Getenv("RTG_VM_MEM") != "" {
		vm.trackMem = true
		vm.tagBytes = make(map[string]int64)
		vm.tagCount = make(map[string]int64)
		vm.callerBytes = make(map[string]int64)
		vm.callerCount = make(map[string]int64)
	}
	if os.Getenv("RTG_VM_ALLOC") != "" {
		vm.trackMem = true
		vm.logAllocs = true
	}

	// Register all functions
	for _, f := range irmod.Funcs {
		vm.funcs[f.Name] = f
	}

	// Allocate globals in VM memory
	vm.numGlobals = len(irmod.Globals)
	if vm.numGlobals > 0 {
		vm.globalsAddr = vm.alloc(uint64(vm.numGlobals)*uint64(vm.config.WordSize), "globals")
	}

	// Intern string literals
	for _, f := range irmod.Funcs {
		for _, inst := range f.Code {
			if inst.Op == ir.OP_CONST_STR {
				s := becommon.DecodeStringLiteral(inst.Name)
				if _, ok := vm.stringAddrs[s]; !ok {
					vm.internString(s)
				}
			}
		}
	}

	// Build interface dispatch table
	vm.buildDispatchTable(irmod)

	// Find special functions
	if f, ok := vm.funcs["runtime.IntToString"]; ok {
		vm.intToStringFunc = f
	}
	if f, ok := vm.funcs["runtime.BytesToString"]; ok {
		vm.bytesToStringFunc = f
	}
	if f, ok := vm.funcs["runtime.StringToBytes"]; ok {
		vm.stringToBytesFunc = f
	}

	// Pre-allocate argv strings
	vm.argvAddrs = make([]uint64, len(vmArgs))
	i := 0
	for i < len(vmArgs) {
		vm.argvAddrs[i] = vm.writeCString(vmArgs[i])
		i = i + 1
	}

	// Allocate frame stack (dedicated region for function call frames)
	vm.frameStackSize = 64 * 1024
	if cfg.PtrSize <= 2 {
		vm.frameStackSize = 16 * 1024
	}
	vm.frameStackBase = int(vm.alloc(uint64(vm.frameStackSize), "frame-stack"))
	vm.frameStackTop = vm.frameStackBase + vm.frameStackSize

	// Initialize slab allocator for fixed-size objects
	vm.slabPageSize = 65536
	if cfg.PtrSize <= 2 {
		vm.slabPageSize = 4096
	}
	vm.slabSmallSize = 2 * vm.config.WordSize
	vm.slabLargeSize = 4 * vm.config.WordSize

	// Run init functions
	for _, f := range irmod.Funcs {
		if ir.IsInitFunc(f.Name) {
			vm.execFunc(f)
			if vm.exited {
				return nil
			}
		}
	}

	// Run main
	mainFunc, ok := vm.funcs["main.main"]
	if !ok {
		return fmt.Errorf("main.main not found")
	}
	vm.execFunc(mainFunc)

	// Always print VM summary on exit
	fmt.Fprintf(os.Stderr, "vm: %s steps, %s calls, %s memory, %s frame hwm, %s stack hwm\n",
		vmFormatCount(vm.stepCount), vmFormatCount(vm.callCount),
		vmFormatBytes(int64(vm.memNext)), vmFormatBytes(vm.frameHWM),
		vmFormatCount(int64(vm.stackHWM)))

	if vm.trackMem {
		fmt.Fprintf(os.Stderr, "  heap=%dKB allocs=%d\n",
			vm.heapAllocs/1024, vm.allocCount)
		fmt.Fprintf(os.Stderr, "  by tag:\n")
		for tag, bytes := range vm.tagBytes {
			fmt.Fprintf(os.Stderr, "    %-16s %8dKB  %d allocs\n", tag, bytes/1024, vm.tagCount[tag])
		}
		// Per-caller breakdown sorted by bytes descending (insertion sort)
		var callerNames []string
		var callerBytesArr []int64
		var callerCountArr []int64
		for name, bytes := range vm.callerBytes {
			// Insert in sorted order (descending by bytes)
			pos := 0
			for pos < len(callerBytesArr) && callerBytesArr[pos] > bytes {
				pos = pos + 1
			}
			callerNames = append(callerNames, "")
			callerBytesArr = append(callerBytesArr, 0)
			callerCountArr = append(callerCountArr, 0)
			j := len(callerNames) - 1
			for j > pos {
				callerNames[j] = callerNames[j-1]
				callerBytesArr[j] = callerBytesArr[j-1]
				callerCountArr[j] = callerCountArr[j-1]
				j = j - 1
			}
			callerNames[pos] = name
			callerBytesArr[pos] = bytes
			callerCountArr[pos] = vm.callerCount[name]
		}
		fmt.Fprintf(os.Stderr, "  by caller:\n")
		ci := 0
		for ci < len(callerNames) {
			fmt.Fprintf(os.Stderr, "    %-40s %8dKB  %d allocs\n", callerNames[ci], callerBytesArr[ci]/1024, callerCountArr[ci])
			ci = ci + 1
		}
	}

	return nil
}

// === Memory operations ===

func (vm *VM) ensureMemory(needed int) {
	if needed <= len(vm.memory) {
		return
	}
	newSize := len(vm.memory) * 2
	if newSize < needed {
		newSize = needed + 4*1024*1024
	}
	grown := make([]byte, newSize)
	j := 0
	for j < len(vm.memory) {
		grown[j] = vm.memory[j]
		j = j + 1
	}
	vm.memory = grown
}

// trackCaller attributes an allocation to the nearest non-runtime caller.
func (vm *VM) trackCaller(size int64) {
	depth := len(vm.callStack)
	if depth == 0 {
		return
	}
	// Skip runtime functions to find the real caller
	ci := depth - 1
	for ci > 0 && strings.HasPrefix(vm.callStack[ci], "runtime.") {
		ci = ci - 1
	}
	caller := vm.callStack[ci]
	vm.callerBytes[caller] = vm.callerBytes[caller] + size
	vm.callerCount[caller] = vm.callerCount[caller] + 1
}

func (vm *VM) alloc(size uint64, tag string) uint64 {
	if size == 0 {
		size = 1
	}
	align := vm.config.WordSize
	vm.memNext = (vm.memNext + align - 1) &^ (align - 1)
	addr := uint64(vm.memNext)
	vm.memNext = vm.memNext + int(size)
	vm.ensureMemory(vm.memNext)
	if vm.trackMem {
		// Slab pages are internal infrastructure — individual slots are
		// tracked by the slab allocator functions instead.
		if tag != "slab-small" && tag != "slab-large" {
			vm.heapAllocs = vm.heapAllocs + int64(size)
			vm.allocCount = vm.allocCount + 1
			vm.tagBytes[tag] = vm.tagBytes[tag] + int64(size)
			vm.tagCount[tag] = vm.tagCount[tag] + 1
			vm.trackCaller(int64(size))
		}
		if vm.logAllocs {
			fmt.Fprintf(os.Stderr, "vm alloc: %6d bytes at 0x%s  [%s] total=%dKB\n",
				size, hexAddr(addr), tag, vm.memNext/1024)
		}
	}
	return addr
}

func (vm *VM) slabAllocSmall(tag string) uint64 {
	sz := int64(vm.slabSmallSize)
	if vm.slabSmallFree != 0 {
		addr := vm.slabSmallFree
		vm.slabSmallFree = vm.loadWord(addr)
		vm.storeWord(addr, 0)
		vm.storeWord(addr+uint64(vm.config.WordSize), 0)
		if vm.trackMem {
			vm.heapAllocs = vm.heapAllocs + sz
			vm.allocCount = vm.allocCount + 1
			vm.tagBytes[tag] = vm.tagBytes[tag] + sz
			vm.tagCount[tag] = vm.tagCount[tag] + 1
			vm.trackCaller(sz)
		}
		return addr
	}
	if vm.slabSmallBump+vm.slabSmallSize <= vm.slabSmallEnd {
		addr := uint64(vm.slabSmallBump)
		vm.slabSmallBump = vm.slabSmallBump + vm.slabSmallSize
		if vm.trackMem {
			vm.heapAllocs = vm.heapAllocs + sz
			vm.allocCount = vm.allocCount + 1
			vm.tagBytes[tag] = vm.tagBytes[tag] + sz
			vm.tagCount[tag] = vm.tagCount[tag] + 1
			vm.trackCaller(sz)
		}
		return addr
	}
	page := vm.alloc(uint64(vm.slabPageSize), "slab-small")
	vm.slabSmallBump = int(page) + vm.slabSmallSize
	vm.slabSmallEnd = int(page) + vm.slabPageSize
	if vm.trackMem {
		vm.heapAllocs = vm.heapAllocs + sz
		vm.allocCount = vm.allocCount + 1
		vm.tagBytes[tag] = vm.tagBytes[tag] + sz
		vm.tagCount[tag] = vm.tagCount[tag] + 1
		vm.trackCaller(sz)
	}
	return page
}

func (vm *VM) slabAllocLarge(tag string) uint64 {
	sz := int64(vm.slabLargeSize)
	if vm.slabLargeFree != 0 {
		addr := vm.slabLargeFree
		vm.slabLargeFree = vm.loadWord(addr)
		ws := uint64(vm.config.WordSize)
		vm.storeWord(addr, 0)
		vm.storeWord(addr+ws, 0)
		vm.storeWord(addr+2*ws, 0)
		vm.storeWord(addr+3*ws, 0)
		if vm.trackMem {
			vm.heapAllocs = vm.heapAllocs + sz
			vm.allocCount = vm.allocCount + 1
			vm.tagBytes[tag] = vm.tagBytes[tag] + sz
			vm.tagCount[tag] = vm.tagCount[tag] + 1
			vm.trackCaller(sz)
		}
		return addr
	}
	if vm.slabLargeBump+vm.slabLargeSize <= vm.slabLargeEnd {
		addr := uint64(vm.slabLargeBump)
		vm.slabLargeBump = vm.slabLargeBump + vm.slabLargeSize
		if vm.trackMem {
			vm.heapAllocs = vm.heapAllocs + sz
			vm.allocCount = vm.allocCount + 1
			vm.tagBytes[tag] = vm.tagBytes[tag] + sz
			vm.tagCount[tag] = vm.tagCount[tag] + 1
			vm.trackCaller(sz)
		}
		return addr
	}
	page := vm.alloc(uint64(vm.slabPageSize), "slab-large")
	vm.slabLargeBump = int(page) + vm.slabLargeSize
	vm.slabLargeEnd = int(page) + vm.slabPageSize
	if vm.trackMem {
		vm.heapAllocs = vm.heapAllocs + sz
		vm.allocCount = vm.allocCount + 1
		vm.tagBytes[tag] = vm.tagBytes[tag] + sz
		vm.tagCount[tag] = vm.tagCount[tag] + 1
		vm.trackCaller(sz)
	}
	return page
}

func (vm *VM) slabFreeSmall(addr uint64) {
	vm.storeWord(addr, vm.slabSmallFree)
	vm.slabSmallFree = addr
}

func (vm *VM) slabFreeLarge(addr uint64) {
	vm.storeWord(addr, vm.slabLargeFree)
	vm.slabLargeFree = addr
}

func hexAddr(v uint64) string {
	if v == 0 {
		return "0"
	}
	digits := "0123456789abcdef"
	var buf []byte
	for v > 0 {
		buf = append(buf, digits[v&0xf])
		v = v >> 4
	}
	// reverse
	i := 0
	j := len(buf) - 1
	for i < j {
		tmp := buf[i]
		buf[i] = buf[j]
		buf[j] = tmp
		i = i + 1
		j = j - 1
	}
	return string(buf)
}

func vmFormatCount(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%d.%dK", n/1000, (n%1000)/100)
	}
	if n < 1000000000 {
		return fmt.Sprintf("%d.%dM", n/1000000, (n%1000000)/100000)
	}
	return fmt.Sprintf("%d.%dB", n/1000000000, (n%1000000000)/100000000)
}

func vmFormatBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%dB", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%dKB", bytes/1024)
	}
	mb := bytes / (1024 * 1024)
	frac := (bytes % (1024 * 1024)) * 10 / (1024 * 1024)
	if mb < 100 {
		return fmt.Sprintf("%d.%dMB", mb, frac)
	}
	return fmt.Sprintf("%dMB", mb)
}

func (vm *VM) loadN(addr uint64, n int) uint64 {
	if addr == 0 {
		return 0
	}
	a := int(addr)
	if a+n > len(vm.memory) {
		return 0
	}
	var val uint64
	i := 0
	for i < n {
		val = val | (uint64(vm.memory[a+i]) << uint(i*8))
		i = i + 1
	}
	return val
}

func (vm *VM) storeN(addr uint64, val uint64, n int) {
	a := int(addr)
	vm.ensureMemory(a + n)
	i := 0
	for i < n {
		vm.memory[a+i] = byte(val >> uint(i*8))
		i = i + 1
	}
}

func (vm *VM) loadWord(addr uint64) uint64 {
	return vm.loadN(addr, vm.config.WordSize)
}

func (vm *VM) storeWord(addr uint64, val uint64) {
	vm.storeN(addr, val&vm.config.WordMask, vm.config.WordSize)
}

func (vm *VM) vmAsmEmit(b ...byte) {
	vm.asmAmd64Code = append(vm.asmAmd64Code, b...)
}

func (vm *VM) vmAsmEmitU32(v uint32) {
	vm.vmAsmEmit(byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}

func (vm *VM) vmAsmRex(w bool, r int, b int) byte {
	rex := byte(0x40)
	if w {
		rex |= 0x08
	}
	if (r & 8) != 0 {
		rex |= 0x04
	}
	if (b & 8) != 0 {
		rex |= 0x01
	}
	return rex
}

func (vm *VM) vmAsmEmitMovRegImm64(dst int, imm int64) {
	vm.vmAsmEmit(vm.vmAsmRex(true, 0, dst), byte(0xb8+(dst&7)))
	u := uint64(imm)
	i := 0
	for i < 8 {
		vm.vmAsmEmit(byte(u >> uint(i*8)))
		i = i + 1
	}
}

func (vm *VM) vmAsmEmitLoadParam(dst int, paramIdx int) {
	off := (paramIdx + 1) * 8
	rex := vm.vmAsmRex(true, dst, 5)
	if off <= 127 {
		vm.vmAsmEmit(rex, 0x8b, byte(0x45|((dst&7)<<3)), byte(int64(-off)))
		return
	}
	vm.vmAsmEmit(rex, 0x8b, byte(0x85|((dst&7)<<3)))
	d := uint32(uint32(^uint32(off)) + 1)
	vm.vmAsmEmit(byte(d), byte(d>>8), byte(d>>16), byte(d>>24))
}

func (vm *VM) vmAsmEmitStoreLocal(src int, slot int) {
	off := (slot + 1) * 8
	rex := vm.vmAsmRex(true, src, 5)
	if off <= 127 {
		vm.vmAsmEmit(rex, 0x89, byte(0x45|((src&7)<<3)), byte(int64(-off)))
		return
	}
	vm.vmAsmEmit(rex, 0x89, byte(0x85|((src&7)<<3)))
	d := uint32(uint32(^uint32(off)) + 1)
	vm.vmAsmEmit(byte(d), byte(d>>8), byte(d>>16), byte(d>>24))
}

func (vm *VM) vmAsmPushReg(reg int) {
	vm.vmAsmEmit(0x4d, 0x8d, 0x7f, 0xf8)
	rex := byte(0x49)
	if reg >= 8 {
		rex = 0x4d
	}
	vm.vmAsmEmit(rex, 0x89, byte(0x07|((reg&7)<<3)))
}

func (vm *VM) vmAsmPopReg(reg int) {
	rex := byte(0x49)
	if reg >= 8 {
		rex = 0x4d
	}
	vm.vmAsmEmit(rex, 0x8b, byte(0x07|((reg&7)<<3)))
	vm.vmAsmEmit(0x4d, 0x8d, 0x7f, 0x08)
}

func (vm *VM) vmAsmBegin(params int) {
	vm.asmAmd64Code = vm.asmAmd64Code[:0]
	vm.asmAmd64Fixups = vm.asmAmd64Fixups[:0]
	vm.asmAmd64Armed = true
	vm.vmAsmEmit(0x55)
	vm.vmAsmEmit(0x48, 0x89, 0xe5)
	frameBytes := params * 8
	if frameBytes > 0 {
		if frameBytes <= 127 {
			vm.vmAsmEmit(0x48, 0x83, 0xec, byte(frameBytes))
		} else {
			v := uint32(frameBytes)
			vm.vmAsmEmit(0x48, 0x81, 0xec, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
		}
	}
	i := params - 1
	for i >= 0 {
		vm.vmAsmPopReg(0)
		vm.vmAsmEmitStoreLocal(0, i)
		i = i - 1
	}
}

func (vm *VM) vmAsmI386EmitMovRegImm32(dst int, imm int32) {
	vm.vmAsmEmit(byte(0xb8 + (dst & 7)))
	vm.vmAsmEmitU32(uint32(imm))
}

func (vm *VM) vmAsmI386EmitLoadParam(dst int, paramIdx int) {
	off := (paramIdx + 1) * 4
	if off <= 127 {
		vm.vmAsmEmit(0x8b, byte(0x45|((dst&7)<<3)), byte(int64(-off)))
		return
	}
	vm.vmAsmEmit(0x8b, byte(0x85|((dst&7)<<3)))
	vm.vmAsmEmitU32(uint32(-off))
}

func (vm *VM) vmAsmI386EmitStoreLocal(src int, slot int) {
	off := (slot + 1) * 4
	if off <= 127 {
		vm.vmAsmEmit(0x89, byte(0x45|((src&7)<<3)), byte(int64(-off)))
		return
	}
	vm.vmAsmEmit(0x89, byte(0x85|((src&7)<<3)))
	vm.vmAsmEmitU32(uint32(-off))
}

func (vm *VM) vmAsmI386PushReg(reg int) {
	vm.vmAsmEmit(0x8d, 0x7f, 0xfc)
	vm.vmAsmEmit(0x89, byte(0x07|((reg&7)<<3)))
}

func (vm *VM) vmAsmI386PopReg(reg int) {
	vm.vmAsmEmit(0x8b, byte(0x07|((reg&7)<<3)))
	vm.vmAsmEmit(0x8d, 0x7f, 0x04)
}

func (vm *VM) vmAsmI386Begin(params int) {
	vm.asmAmd64Code = vm.asmAmd64Code[:0]
	vm.asmAmd64Fixups = vm.asmAmd64Fixups[:0]
	vm.asmAmd64Armed = true
	vm.vmAsmEmit(0x55)
	vm.vmAsmEmit(0x89, 0xe5)
	frameBytes := params * 4
	if frameBytes > 0 {
		if frameBytes <= 127 {
			vm.vmAsmEmit(0x83, 0xec, byte(frameBytes))
		} else {
			vm.vmAsmEmit(0x81, 0xec)
			vm.vmAsmEmitU32(uint32(frameBytes))
		}
	}
	i := params - 1
	for i >= 0 {
		vm.vmAsmI386PopReg(0)
		vm.vmAsmI386EmitStoreLocal(0, i)
		i = i - 1
	}
}

func (vm *VM) vmAsmArm64Emit(inst uint32) {
	vm.vmAsmEmitU32(inst)
}

func (vm *VM) vmAsmArm64AddImm(rd int, rn int, imm12 uint32) {
	vm.vmAsmArm64Emit(uint32(0x91000000) | ((imm12 & 0xFFF) << 10) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f))
}

func (vm *VM) vmAsmArm64SubImm(rd int, rn int, imm12 uint32) {
	vm.vmAsmArm64Emit(uint32(0xD1000000) | ((imm12 & 0xFFF) << 10) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f))
}

func (vm *VM) vmAsmArm64SubRR(rd int, rn int, rm int) {
	vm.vmAsmArm64Emit(uint32(0xCB000000) | (uint32(rm&0x1f) << 16) | (uint32(rn&0x1f) << 5) | uint32(rd&0x1f))
}

func (vm *VM) vmAsmArm64MovReg(rd int, rm int) {
	vm.vmAsmArm64AddImm(rd, rm, 0)
}

func (vm *VM) vmAsmArm64MovZ(rd int, imm16 uint16, shift int) {
	hw := uint32((shift / 16) & 3)
	vm.vmAsmArm64Emit(uint32(0xD2800000) | (hw << 21) | (uint32(imm16) << 5) | uint32(rd&0x1f))
}

func (vm *VM) vmAsmArm64MovK(rd int, imm16 uint16, shift int) {
	hw := uint32((shift / 16) & 3)
	vm.vmAsmArm64Emit(uint32(0xF2800000) | (hw << 21) | (uint32(imm16) << 5) | uint32(rd&0x1f))
}

func (vm *VM) vmAsmArm64LoadImm64(dst int, imm int64) {
	u := uint64(imm)
	vm.vmAsmArm64MovZ(dst, uint16(u&0xffff), 0)
	vm.vmAsmArm64MovK(dst, uint16((u>>16)&0xffff), 16)
	vm.vmAsmArm64MovK(dst, uint16((u>>32)&0xffff), 32)
	vm.vmAsmArm64MovK(dst, uint16((u>>48)&0xffff), 48)
}

func (vm *VM) vmAsmArm64Ldr(rt int, rn int, offset int) {
	if offset == 0 {
		vm.vmAsmArm64Emit(uint32(0xF9400000) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f))
		return
	}
	if offset > 0 && offset%8 == 0 && offset/8 < 4096 {
		uimm := uint32(offset / 8)
		vm.vmAsmArm64Emit(uint32(0xF9400000) | (uimm << 10) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f))
		return
	}
	simm9 := uint32(offset) & 0x1FF
	vm.vmAsmArm64Emit(uint32(0xF8400000) | (simm9 << 12) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f))
}

func (vm *VM) vmAsmArm64Str(rt int, rn int, offset int) {
	if offset == 0 {
		vm.vmAsmArm64Emit(uint32(0xF9000000) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f))
		return
	}
	if offset > 0 && offset%8 == 0 && offset/8 < 4096 {
		uimm := uint32(offset / 8)
		vm.vmAsmArm64Emit(uint32(0xF9000000) | (uimm << 10) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f))
		return
	}
	simm9 := uint32(offset) & 0x1FF
	vm.vmAsmArm64Emit(uint32(0xF8000000) | (simm9 << 12) | (uint32(rn&0x1f) << 5) | uint32(rt&0x1f))
}

func (vm *VM) vmAsmArm64PushReg(reg int) {
	vm.vmAsmArm64SubImm(28, 28, 8)
	vm.vmAsmArm64Str(reg, 28, 0)
}

func (vm *VM) vmAsmArm64PopReg(reg int) {
	vm.vmAsmArm64Ldr(reg, 28, 0)
	vm.vmAsmArm64AddImm(28, 28, 8)
}

func (vm *VM) vmAsmArm64EmitLoadParam(dst int, paramIdx int) {
	vm.vmAsmArm64Ldr(dst, 29, -(paramIdx+1)*8)
}

func (vm *VM) vmAsmArm64EmitStoreLocal(src int, slot int) {
	vm.vmAsmArm64Str(src, 29, -(slot+1)*8)
}

func (vm *VM) vmAsmArm64Begin(params int) {
	vm.asmAmd64Code = vm.asmAmd64Code[:0]
	vm.asmAmd64Fixups = vm.asmAmd64Fixups[:0]
	vm.asmAmd64Armed = true
	vm.vmAsmArm64Emit(0xA9BF7BFD) // stp x29, x30, [sp, #-16]!
	vm.vmAsmArm64MovReg(29, 31)   // mov x29, sp
	frameBytes := params * 8
	if frameBytes%16 != 0 {
		frameBytes = frameBytes + (16 - frameBytes%16)
	}
	if frameBytes > 0 {
		if frameBytes < 4096 {
			vm.vmAsmArm64SubImm(31, 31, uint32(frameBytes))
		} else {
			vm.vmAsmArm64LoadImm64(16, int64(frameBytes))
			vm.vmAsmArm64SubRR(31, 31, 16)
		}
	}
	i := params - 1
	for i >= 0 {
		vm.vmAsmArm64PopReg(0)
		vm.vmAsmArm64EmitStoreLocal(0, i)
		i = i - 1
	}
}

func (vm *VM) vmAsmByteSlice(data []byte, tag string) uint64 {
	ws := uint64(vm.config.WordSize)
	dataAddr := vm.alloc(uint64(len(data)), tag+"-data")
	vm.copyToVM(int(dataAddr), data, len(data))
	hdr := vm.alloc(4*ws, tag+"-hdr")
	vm.storeWord(hdr, dataAddr)
	vm.storeWord(hdr+ws, uint64(len(data)))
	vm.storeWord(hdr+2*ws, uint64(len(data)))
	vm.storeWord(hdr+3*ws, 1)
	return hdr
}

func (vm *VM) vmAsmFixupBytes() []byte {
	var out []byte
	for _, fx := range vm.asmAmd64Fixups {
		off := uint32(fx.Off)
		out = append(out, byte(off), byte(off>>8), byte(off>>16), byte(off>>24))
		n := uint32(len(fx.Target))
		out = append(out, byte(n), byte(n>>8), byte(n>>16), byte(n>>24))
		out = append(out, []byte(fx.Target)...)
	}
	return out
}

func (vm *VM) signExtend(val uint64) int64 {
	if val&vm.config.SignBit != 0 {
		return int64(val | ^vm.config.WordMask)
	}
	return int64(val)
}

// widthMask returns the bitmask for a given byte width.
func widthMask(w int) uint64 {
	if w == 1 {
		return 0xFF
	}
	if w == 2 {
		return 0xFFFF
	}
	if w == 4 {
		return 0xFFFFFFFF
	}
	return 0xFFFFFFFFFFFFFFFF
}

// signExtendWidth sign-extends a value from the given byte width to int64.
func signExtendWidth(val uint64, w int) int64 {
	if w == 1 {
		if val&0x80 != 0 {
			return int64(val | 0xFFFFFFFFFFFFFF00)
		}
		return int64(val & 0xFF)
	}
	if w == 2 {
		if val&0x8000 != 0 {
			return int64(val | 0xFFFFFFFFFFFF0000)
		}
		return int64(val & 0xFFFF)
	}
	if w == 4 {
		return int64(int32(val))
	}
	return int64(val)
}

// effectiveWidth returns the instruction's width, or the VM word size if 0.
func (vm *VM) effectiveWidth(w int) int {
	if w == 0 {
		return vm.config.WordSize
	}
	return w
}

// maskResult masks a value to the effective width and returns the masked result.
func (vm *VM) maskResult(val uint64, instWidth int) uint64 {
	w := vm.effectiveWidth(instWidth)
	return val & widthMask(w)
}

// signExtendW sign-extends a value using the effective width.
func (vm *VM) signExtendW(val uint64, instWidth int) int64 {
	w := vm.effectiveWidth(instWidth)
	if w >= 8 {
		return int64(val)
	}
	return signExtendWidth(val, w)
}

// === Stack operations ===

func (vm *VM) push(val uint64) {
	if vm.sp >= len(vm.stack) {
		vm.stack = append(vm.stack, val)
	} else {
		vm.stack[vm.sp] = val
	}
	vm.sp = vm.sp + 1
	if vm.sp > vm.stackHWM {
		vm.stackHWM = vm.sp
	}
}

func (vm *VM) pop() uint64 {
	vm.sp = vm.sp - 1
	return vm.stack[vm.sp]
}

// === String helpers ===

func (vm *VM) internString(s string) uint64 {
	ws := uint64(vm.config.WordSize)
	dataAddr := vm.alloc(uint64(len(s)), "str-data")
	i := 0
	for i < len(s) {
		vm.memory[int(dataAddr)+i] = s[i]
		i = i + 1
	}
	hdrAddr := vm.alloc(2*ws, "str-hdr")
	vm.storeWord(hdrAddr, dataAddr)
	vm.storeWord(hdrAddr+ws, uint64(len(s)))
	vm.stringAddrs[s] = hdrAddr
	return hdrAddr
}

func (vm *VM) readCString(addr uint64) string {
	var buf []byte
	a := int(addr)
	for a < len(vm.memory) && vm.memory[a] != 0 {
		buf = append(buf, vm.memory[a])
		a = a + 1
	}
	return string(buf)
}

func (vm *VM) readString(addr uint64) string {
	if addr == 0 {
		return ""
	}
	ws := uint64(vm.config.WordSize)
	dataAddr := vm.loadWord(addr)
	n := int(vm.loadWord(addr + ws))
	if dataAddr == 0 || n <= 0 {
		return ""
	}
	start := int(dataAddr)
	if start < 0 || start+n > len(vm.memory) {
		return ""
	}
	buf := make([]byte, n)
	i := 0
	for i < n {
		buf[i] = vm.memory[start+i]
		i = i + 1
	}
	return string(buf)
}

func (vm *VM) writeCString(s string) uint64 {
	addr := vm.alloc(uint64(len(s)+1), "cstr")
	a := int(addr)
	i := 0
	for i < len(s) {
		vm.memory[a+i] = s[i]
		i = i + 1
	}
	vm.memory[a+len(s)] = 0
	return addr
}

func (vm *VM) copyToVM(dst int, src []byte, n int) {
	vm.ensureMemory(dst + n)
	i := 0
	for i < n {
		vm.memory[dst+i] = src[i]
		i = i + 1
	}
}

func (vm *VM) copyFromVM(dst []byte, src int, n int) {
	i := 0
	for i < n {
		dst[i] = vm.memory[src+i]
		i = i + 1
	}
}

func (vm *VM) copyStringToVM(dst int, s string, n int) {
	vm.ensureMemory(dst + n)
	i := 0
	for i < n {
		vm.memory[dst+i] = s[i]
		i = i + 1
	}
}

// === Dispatch table ===

func (vm *VM) buildDispatchTable(irmod *ir.IRModule) {
	var methods []string
	for _, f := range irmod.Funcs {
		for _, inst := range f.Code {
			if inst.Op == ir.OP_IFACE_CALL {
				name := vmBareMethod(inst.Name)
				if _, ok := vm.methodIDs[name]; !ok {
					vm.methodIDs[name] = len(methods)
					methods = append(methods, name)
				}
			}
		}
	}

	if _, ok := vm.methodIDs["Error"]; !ok {
		vm.methodIDs["Error"] = len(methods)
		methods = append(methods, "Error")
	}
	vm.errorMethodID = vm.methodIDs["Error"]

	if _, ok := vm.methodIDs["String"]; !ok {
		vm.methodIDs["String"] = len(methods)
		methods = append(methods, "String")
	}
	vm.stringMethodID = vm.methodIDs["String"]

	for mkey, funcName := range irmod.MethodTable {
		if _, ok := vm.funcs[funcName]; !ok {
			continue
		}
		dotIdx := -1
		i := len(mkey) - 1
		for i >= 0 {
			if mkey[i] == '.' {
				dotIdx = i
				break
			}
			i = i - 1
		}
		if dotIdx < 0 {
			continue
		}
		typeName := mkey[0:dotIdx]
		methodName := mkey[dotIdx+1:]

		tid, ok := irmod.TypeIDs[typeName]
		if !ok {
			continue
		}
		mid, ok := vm.methodIDs[methodName]
		if !ok {
			mid = len(methods)
			vm.methodIDs[methodName] = mid
			methods = append(methods, methodName)
		}
		vm.dispatch = append(vm.dispatch, vmDispatchEntry{
			typeID:   tid,
			methodID: mid,
			funcName: funcName,
		})
	}
}

func (vm *VM) findDispatch(typeID int, methodID int) string {
	i := 0
	for i < len(vm.dispatch) {
		if vm.dispatch[i].typeID == typeID && vm.dispatch[i].methodID == methodID {
			return vm.dispatch[i].funcName
		}
		i = i + 1
	}
	return ""
}

func vmBareMethod(name string) string {
	i := len(name) - 1
	for i >= 0 {
		if name[i] == '.' {
			return name[i+1:]
		}
		i = i - 1
	}
	return name
}

// === Tostring ===

func (vm *VM) tostring(v uint64) uint64 {
	if v == 0 {
		return 0
	}
	ws := uint64(vm.config.WordSize)
	if v < 4096 {
		if vm.intToStringFunc == nil {
			return 0
		}
		vm.push(v)
		vm.execFunc(vm.intToStringFunc)
		return vm.pop()
	}
	first := vm.loadWord(v)
	if first >= 256 {
		return v
	}
	concrete := vm.loadWord(v + ws)
	if first == 1 {
		if vm.intToStringFunc == nil {
			return 0
		}
		vm.push(concrete)
		vm.execFunc(vm.intToStringFunc)
		return vm.pop()
	}
	if first == 2 {
		return concrete
	}
	fn := vm.findDispatch(int(first), vm.errorMethodID)
	if fn != "" {
		vm.push(concrete)
		vm.execFunc(vm.funcs[fn])
		return vm.pop()
	}
	fn = vm.findDispatch(int(first), vm.stringMethodID)
	if fn != "" {
		vm.push(concrete)
		vm.execFunc(vm.funcs[fn])
		return vm.pop()
	}
	return 0
}

// === Syscall return helper ===

func (vm *VM) vmSysReturn(rv int64) {
	if rv < 0 {
		vm.push(0)
		vm.push(0)
		vm.push(uint64(-rv) & vm.config.WordMask)
	} else {
		vm.push(uint64(rv) & vm.config.WordMask)
		vm.push(0)
		vm.push(0)
	}
}

func (vm *VM) execIntrinsicArgs(name string, ws uint64, args ...uint64) {
	if len(args) == 0 {
		vm.execIntrinsic(name, 0, ws)
		return
	}
	if vm.intrinsicScratchWords < len(args) || vm.intrinsicScratchAddr == 0 {
		words := len(args)
		if words < 8 {
			words = 8
		}
		vm.intrinsicScratchAddr = vm.alloc(uint64(words)*ws, "intrinsic-args-scratch")
		vm.intrinsicScratchWords = words
	}
	addr := vm.intrinsicScratchAddr
	i := 0
	for i < len(args) {
		vm.storeWord(addr+uint64(i)*ws, args[i])
		i = i + 1
	}
	vm.execIntrinsic(name, addr, ws)
}

// === Local variable helper ===

func (vm *VM) localGet(localsAddr uint64, ws uint64, idx int) uint64 {
	return vm.loadWord(localsAddr + uint64(idx)*ws)
}

// === Execution ===

func (vm *VM) execFunc(f *ir.IRFunc) {
	if vm.exited {
		return
	}

	// Track call stack depth for step limit debugging
	csDepth := len(vm.callStack)

	ws := uint64(vm.config.WordSize)

	// Slot pitch: max(ws, maxLocalWidth) so int64 locals don't overflow into adjacent slots.
	slotPitch := ws
	i := 0
	for i < len(f.Locals) {
		lw := uint64(f.Locals[i].Width)
		if lw > slotPitch {
			slotPitch = lw
		}
		i = i + 1
	}

	frameSize := len(f.Locals)
	if f.Params > frameSize {
		frameSize = f.Params
	}
	if frameSize <= 0 {
		frameSize = 1
	}

	// Allocate frame from dedicated frame stack (grows downward)
	frameBytes := int(uint64(frameSize) * slotPitch)
	savedFrameTop := vm.frameStackTop
	vm.frameStackTop = vm.frameStackTop - frameBytes
	vm.frameStackTop = vm.frameStackTop &^ (int(ws) - 1) // align down
	localsAddr := uint64(vm.frameStackTop)

	if vm.frameStackTop < vm.frameStackBase {
		fmt.Fprintf(os.Stderr, "vm: frame stack overflow in %s (depth=%d)\n", f.Name, len(vm.callStack))
		os.Exit(2)
	}

	vm.callCount = vm.callCount + 1
	used := int64(vm.frameStackSize) - int64(vm.frameStackTop-vm.frameStackBase)
	if used > vm.frameHWM {
		vm.frameHWM = used
	}

	// Zero locals
	i = 0
	for i < frameSize {
		vm.storeN(localsAddr+uint64(i)*slotPitch, 0, int(slotPitch))
		i = i + 1
	}

	// Pop params from stack into locals (params are word-sized on the stack)
	i = f.Params - 1
	for i >= 0 {
		vm.storeN(localsAddr+uint64(i)*slotPitch, vm.pop(), int(slotPitch))
		i = i - 1
	}

	// Pre-resolve labels
	labels := make(map[int]int)
	i = 0
	for i < len(f.Code) {
		if f.Code[i].Op == ir.OP_LABEL {
			labels[f.Code[i].Arg] = i
		}
		i = i + 1
	}

	// Execute
	ip := 0
	code := f.Code
	codeLen := len(code)

	vm.callStack = append(vm.callStack, f.Name)

	for ip < codeLen && !vm.exited {
		inst := code[ip]
		ip = ip + 1

		vm.stepCount = vm.stepCount + 1
		if vm.stepLimit > 0 && vm.stepCount >= vm.stepLimit {
			fmt.Fprintf(os.Stderr, "vm: step limit %d reached\nCall stack (%d frames):\n", vm.stepLimit, len(vm.callStack))
			si := len(vm.callStack) - 1
			for si >= 0 {
				fmt.Fprintf(os.Stderr, "  %s\n", vm.callStack[si])
				si = si - 1
			}
			fmt.Fprintf(os.Stderr, "Current instruction: op=%d (ip=%d in %s)\n", inst.Op, ip-1, f.Name)
			os.Exit(99)
		}

		switch inst.Op {
		case ir.OP_LABEL:
			// no-op

		case ir.OP_CONST_I64:
			// Push full 64-bit constant; consuming ops (arithmetic, LOCAL_SET) handle masking.
			vm.push(uint64(inst.Val))

		case ir.OP_CONST_STR:
			s := becommon.DecodeStringLiteral(inst.Name)
			addr, ok := vm.stringAddrs[s]
			if !ok {
				addr = vm.internString(s)
			}
			vm.push(addr)

		case ir.OP_CONST_BOOL:
			if inst.Arg != 0 {
				vm.push(1)
			} else {
				vm.push(0)
			}

		case ir.OP_CONST_NIL:
			vm.push(0)

		case ir.OP_LOCAL_GET:
			w := vm.effectiveWidth(inst.Width)
			vm.push(vm.loadN(localsAddr+uint64(inst.Arg)*slotPitch, w))

		case ir.OP_LOCAL_SET:
			w := vm.effectiveWidth(inst.Width)
			vm.storeN(localsAddr+uint64(inst.Arg)*slotPitch, vm.pop(), w)

		case ir.OP_LOCAL_ADD_IMM:
			w := vm.effectiveWidth(inst.Width)
			addr := localsAddr + uint64(inst.Arg)*slotPitch
			v := vm.loadN(addr, w)
			vm.storeN(addr, v+uint64(inst.Val), w)

		case ir.OP_LOCAL_ADDR:
			vm.push(localsAddr + uint64(inst.Arg)*slotPitch)

		case ir.OP_GLOBAL_GET:
			vm.push(vm.loadWord(vm.globalsAddr + uint64(inst.Arg)*ws))

		case ir.OP_GLOBAL_SET:
			vm.storeWord(vm.globalsAddr+uint64(inst.Arg)*ws, vm.pop())

		case ir.OP_GLOBAL_ADDR:
			vm.push(vm.globalsAddr + uint64(inst.Arg)*ws)

		case ir.OP_DROP:
			vm.pop()

		case ir.OP_DUP:
			v := vm.pop()
			vm.push(v)
			vm.push(v)

		case ir.OP_ADD:
			a := vm.pop()
			c := vm.pop()
			vm.push(vm.maskResult(uint64(int64(c)+int64(a)), inst.Width))

		case ir.OP_SUB:
			a := vm.pop()
			c := vm.pop()
			vm.push(vm.maskResult(uint64(int64(c)-int64(a)), inst.Width))

		case ir.OP_MUL:
			a := vm.pop()
			c := vm.pop()
			vm.push(vm.maskResult(uint64(vm.signExtendW(c, inst.Width)*vm.signExtendW(a, inst.Width)), inst.Width))

		case ir.OP_DIV:
			a := vm.pop()
			c := vm.pop()
			if a == 0 {
				vm.push(0)
			} else {
				vm.push(vm.maskResult(uint64(vm.signExtendW(c, inst.Width)/vm.signExtendW(a, inst.Width)), inst.Width))
			}

		case ir.OP_MOD:
			a := vm.pop()
			c := vm.pop()
			if a == 0 {
				vm.push(0)
			} else {
				vm.push(vm.maskResult(uint64(vm.signExtendW(c, inst.Width)%vm.signExtendW(a, inst.Width)), inst.Width))
			}

		case ir.OP_NEG:
			a := vm.pop()
			vm.push(vm.maskResult(uint64(-vm.signExtendW(a, inst.Width)), inst.Width))

		case ir.OP_AND:
			a := vm.pop()
			c := vm.pop()
			vm.push(c & a)

		case ir.OP_OR:
			a := vm.pop()
			c := vm.pop()
			vm.push(c | a)

		case ir.OP_XOR:
			a := vm.pop()
			c := vm.pop()
			vm.push((c ^ a) & widthMask(vm.effectiveWidth(inst.Width)))

		case ir.OP_SHL:
			a := vm.pop()
			c := vm.pop()
			w := vm.effectiveWidth(inst.Width)
			shiftMask := uint64(w*8 - 1)
			vm.push((c << (a & shiftMask)) & widthMask(w))

		case ir.OP_SHR:
			a := vm.pop()
			c := vm.pop()
			w := vm.effectiveWidth(inst.Width)
			shiftMask := uint64(w*8 - 1)
			vm.push((c & widthMask(w)) >> (a & shiftMask))

		case ir.OP_EQ:
			a := vm.pop()
			c := vm.pop()
			if vm.signExtendW(c, inst.Width) == vm.signExtendW(a, inst.Width) {
				vm.push(1)
			} else {
				vm.push(0)
			}

		case ir.OP_NEQ:
			a := vm.pop()
			c := vm.pop()
			if vm.signExtendW(c, inst.Width) != vm.signExtendW(a, inst.Width) {
				vm.push(1)
			} else {
				vm.push(0)
			}

		case ir.OP_LT:
			a := vm.pop()
			c := vm.pop()
			if vm.signExtendW(c, inst.Width) < vm.signExtendW(a, inst.Width) {
				vm.push(1)
			} else {
				vm.push(0)
			}

		case ir.OP_GT:
			a := vm.pop()
			c := vm.pop()
			if vm.signExtendW(c, inst.Width) > vm.signExtendW(a, inst.Width) {
				vm.push(1)
			} else {
				vm.push(0)
			}

		case ir.OP_LEQ:
			a := vm.pop()
			c := vm.pop()
			if vm.signExtendW(c, inst.Width) <= vm.signExtendW(a, inst.Width) {
				vm.push(1)
			} else {
				vm.push(0)
			}

		case ir.OP_GEQ:
			a := vm.pop()
			c := vm.pop()
			if vm.signExtendW(c, inst.Width) >= vm.signExtendW(a, inst.Width) {
				vm.push(1)
			} else {
				vm.push(0)
			}

		case ir.OP_NOT:
			a := vm.pop()
			if a == 0 {
				vm.push(1)
			} else {
				vm.push(0)
			}

		case ir.OP_LOAD:
			addr := vm.pop()
			n := inst.Arg
			if n == 0 {
				n = vm.config.WordSize
			}
			vm.push(vm.loadN(addr, n))

		case ir.OP_STORE:
			addr := vm.pop()
			val := vm.pop()
			n := inst.Arg
			if n == 0 {
				n = vm.config.WordSize
			}
			vm.storeN(addr, val, n)

		case ir.OP_OFFSET:
			a := vm.pop()
			vm.push(a + uint64(inst.Arg))

		case ir.OP_INDEX_ADDR:
			idx := vm.pop()
			base := vm.pop()
			var dataPtr uint64
			if base != 0 {
				dataPtr = vm.loadWord(base)
			}
			vm.push(dataPtr + idx*uint64(inst.Arg))

		case ir.OP_LEN:
			a := vm.pop()
			if a == 0 {
				vm.push(0)
			} else {
				vm.push(vm.loadWord(a + ws))
			}
		case ir.OP_CAP:
			a := vm.pop()
			if a == 0 {
				vm.push(0)
			} else {
				vm.push(vm.loadWord(a + 2*ws))
			}

		case ir.OP_JMP:
			ip = labels[inst.Arg]

		case ir.OP_JMP_IF:
			a := vm.pop()
			if a != 0 {
				ip = labels[inst.Arg]
			}

		case ir.OP_JMP_IF_NOT:
			a := vm.pop()
			if a == 0 {
				ip = labels[inst.Arg]
			}

		case ir.OP_JMP_EQ:
			a := vm.pop()
			c := vm.pop()
			if vm.signExtendW(c, inst.Width) == vm.signExtendW(a, inst.Width) {
				ip = labels[inst.Arg]
			}

		case ir.OP_JMP_NEQ:
			a := vm.pop()
			c := vm.pop()
			if vm.signExtendW(c, inst.Width) != vm.signExtendW(a, inst.Width) {
				ip = labels[inst.Arg]
			}

		case ir.OP_JMP_LT:
			a := vm.pop()
			c := vm.pop()
			if vm.signExtendW(c, inst.Width) < vm.signExtendW(a, inst.Width) {
				ip = labels[inst.Arg]
			}

		case ir.OP_JMP_GT:
			a := vm.pop()
			c := vm.pop()
			if vm.signExtendW(c, inst.Width) > vm.signExtendW(a, inst.Width) {
				ip = labels[inst.Arg]
			}

		case ir.OP_JMP_LEQ:
			a := vm.pop()
			c := vm.pop()
			if vm.signExtendW(c, inst.Width) <= vm.signExtendW(a, inst.Width) {
				ip = labels[inst.Arg]
			}

		case ir.OP_JMP_GEQ:
			a := vm.pop()
			c := vm.pop()
			if vm.signExtendW(c, inst.Width) >= vm.signExtendW(a, inst.Width) {
				ip = labels[inst.Arg]
			}

		case ir.OP_CALL:
			if strings.HasPrefix(inst.Name, "builtin.composite.") {
				vm.builtinComposite(inst.Arg)
			} else {
				target, ok := vm.funcs[inst.Name]
				if !ok {
					fmt.Fprintf(os.Stderr, "vm: unresolved call target: %s\n", inst.Name)
					vm.exited = true
					ExitCode = 2
					return
				}
				vm.execFunc(target)
			}

		case ir.OP_CALL_INTRINSIC:
			vm.execIntrinsic(inst.Name, localsAddr, slotPitch)

		case ir.OP_RETURN:
			vm.frameStackTop = savedFrameTop
			vm.callStack = vm.callStack[0:csDepth]
			return

		case ir.OP_CONVERT:
			switch inst.Name {
			case "string":
				if vm.bytesToStringFunc != nil {
					vm.execFunc(vm.bytesToStringFunc)
				}
			case "[]byte":
				if vm.stringToBytesFunc != nil {
					vm.execFunc(vm.stringToBytesFunc)
				}
			case "byte":
				a := vm.pop()
				vm.push(a & 0xFF)
			case "uint8":
				a := vm.pop()
				vm.push(a & 0xFF)
			case "int8":
				a := vm.pop()
				v := int8(uint8(a))
				vm.push(uint64(int64(v)) & vm.config.WordMask)
			case "uint16":
				a := vm.pop()
				vm.push(a & 0xFFFF)
			case "int16":
				a := vm.pop()
				v := int16(uint16(a))
				vm.push(uint64(int64(v)) & vm.config.WordMask)
			case "int32":
				a := vm.pop()
				v := int32(uint32(a))
				vm.push(uint64(int64(v)) & vm.config.WordMask)
			case "uint32":
				a := vm.pop()
				vm.push(uint64(uint32(a)))
			default:
				// no-op conversion
			}

		case ir.OP_IFACE_BOX:
			val := vm.pop()
			box := vm.slabAllocSmall("iface-box")
			vm.storeWord(box, uint64(inst.Arg))
			vm.storeWord(box+ws, val)
			vm.push(box)

		case ir.OP_IFACE_CALL:
			methodName := vmBareMethod(inst.Name)
			mid := vm.methodIDs[methodName]
			argc := inst.Arg

			args := make([]uint64, argc)
			j := 0
			for j < argc {
				args[j] = vm.pop()
				j = j + 1
			}
			receiver := vm.pop()
			var typeID uint64
			var value uint64
			if receiver != 0 {
				typeID = vm.loadWord(receiver)
				value = vm.loadWord(receiver + ws)
			}
			vm.push(value)
			j = argc - 1
			for j >= 0 {
				vm.push(args[j])
				j = j - 1
			}
			funcName := vm.findDispatch(int(typeID), mid)
			if funcName == "" {
				fmt.Fprintf(os.Stderr, "vm: interface dispatch failed: typeID=%d method=%s\n", typeID, methodName)
				vm.exited = true
				ExitCode = 2
				return
			}
			vm.execFunc(vm.funcs[funcName])

		case ir.OP_PANIC:
			a := vm.pop()
			if a != 0 {
				c := vm.loadWord(a)
				if c < 256 {
					a = vm.loadWord(a + ws)
				}
				dataPtr := vm.loadWord(a)
				dataLen := vm.loadWord(a + ws)
				if dataPtr != 0 && dataLen != 0 {
					n := int(dataLen)
					p := int(dataPtr)
					if p+n <= len(vm.memory) {
						os.Stderr.Write(vm.memory[p : p+n])
					}
				}
			}
			os.Stderr.Write([]byte("\n"))
			vm.exited = true
			ExitCode = 2
			return

		case ir.OP_SLICE_GET, ir.OP_SLICE_MAKE, ir.OP_STRING_GET, ir.OP_STRING_MAKE:
			fmt.Fprintf(os.Stderr, "vm: unsupported opcode %d\n", inst.Op)
			vm.exited = true
			ExitCode = 2
			return

		default:
			fmt.Fprintf(os.Stderr, "vm: unhandled opcode %d\n", inst.Op)
			vm.exited = true
			ExitCode = 2
			return
		}

		if vm.exited {
			vm.frameStackTop = savedFrameTop
			vm.callStack = vm.callStack[0:csDepth]
			return
		}
	}
	vm.frameStackTop = savedFrameTop
	vm.callStack = vm.callStack[0:csDepth]
}

// === Builtin composite literal ===

func (vm *VM) builtinComposite(fieldCount int) {
	ws := uint64(vm.config.WordSize)
	if fieldCount <= 0 {
		vm.push(0)
		return
	}
	tmp := make([]uint64, fieldCount)
	i := 0
	for i < fieldCount {
		tmp[i] = vm.pop()
		i = i + 1
	}
	p := vm.alloc(uint64(fieldCount)*ws, "composite")
	i = 0
	for i < fieldCount {
		vm.storeWord(p+uint64(i)*ws, tmp[fieldCount-1-i])
		i = i + 1
	}
	vm.push(p)
}

// === Intrinsics ===

func (vm *VM) execIntrinsic(name string, localsAddr uint64, ws uint64) {
	switch name {
	case "SysRead":
		fd := int(vm.localGet(localsAddr, ws, 0))
		bufAddr := vm.localGet(localsAddr, ws, 1)
		count := vm.localGet(localsAddr, ws, 2)
		if fd < 0 || fd >= 256 || !vm.fdUsed[fd] {
			vm.vmSysReturn(-1)
			return
		}
		n := int(count)
		buf := make([]byte, n)
		f := vm.fdFiles[fd]
		nr, _ := f.Read(buf)
		if nr > 0 {
			vm.copyToVM(int(bufAddr), buf, nr)
		}
		// Return bytes read count; 0 means EOF (Unix convention)
		vm.vmSysReturn(int64(nr))

	case "SysWrite":
		fd := int(vm.localGet(localsAddr, ws, 0))
		bufAddr := vm.localGet(localsAddr, ws, 1)
		count := vm.localGet(localsAddr, ws, 2)
		if fd < 0 || fd >= 256 || !vm.fdUsed[fd] {
			vm.vmSysReturn(-1)
			return
		}
		n := int(count)
		a := int(bufAddr)
		if a+n > len(vm.memory) {
			n = len(vm.memory) - a
		}
		if n <= 0 {
			vm.vmSysReturn(0)
			return
		}
		f := vm.fdFiles[fd]
		nw, err := f.Write(vm.memory[a : a+n])
		if err != nil {
			vm.vmSysReturn(-1)
		} else {
			vm.vmSysReturn(int64(nw))
		}

	case "SysOpen":
		pathAddr := vm.localGet(localsAddr, ws, 0)
		flags := vm.localGet(localsAddr, ws, 1)
		path := vm.readCString(pathAddr)
		fl := int(flags)
		var flag int
		if fl&1 != 0 {
			flag = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		} else if fl&2 != 0 {
			flag = os.O_RDWR
		} else {
			flag = os.O_RDONLY
		}
		f, err := os.OpenFile(path, flag, 0644)
		if err != nil {
			vm.vmSysReturn(-2)
			return
		}
		fd := vm.nextFD
		vm.nextFD = vm.nextFD + 1
		if fd >= 256 {
			f.Close()
			vm.vmSysReturn(-1)
			return
		}
		vm.fdFiles[fd] = f
		vm.fdUsed[fd] = true
		vm.vmSysReturn(int64(fd))

	case "SysClose":
		fd := int(vm.localGet(localsAddr, ws, 0))
		if fd < 3 || fd >= 256 || !vm.fdUsed[fd] {
			vm.vmSysReturn(-1)
			return
		}
		f := vm.fdFiles[fd]
		f.Close()
		vm.fdFiles[fd] = nil
		vm.fdUsed[fd] = false
		vm.vmSysReturn(0)

	case "SysStat":
		pathAddr := vm.localGet(localsAddr, ws, 0)
		path := vm.readCString(pathAddr)
		// Check if file/directory exists by trying to open it
		f, err := os.Open(path)
		if err == nil {
			f.Close()
			vm.vmSysReturn(0)
		} else {
			vm.vmSysReturn(-2)
		}

	case "SysMkdir":
		pathAddr := vm.localGet(localsAddr, ws, 0)
		path := vm.readCString(pathAddr)
		err := os.MkdirAll(path, 0755)
		if err != nil {
			vm.vmSysReturn(-17)
		} else {
			vm.vmSysReturn(0)
		}

	case "SysRmdir":
		pathAddr := vm.localGet(localsAddr, ws, 0)
		path := vm.readCString(pathAddr)
		err := os.RemoveAll(path)
		if err != nil {
			vm.vmSysReturn(-1)
		} else {
			vm.vmSysReturn(0)
		}

	case "SysUnlink":
		pathAddr := vm.localGet(localsAddr, ws, 0)
		path := vm.readCString(pathAddr)
		err := os.RemoveAll(path)
		if err != nil {
			vm.vmSysReturn(-1)
		} else {
			vm.vmSysReturn(0)
		}

	case "SysGetcwd":
		bufAddr := vm.localGet(localsAddr, ws, 0)
		bufSize := vm.localGet(localsAddr, ws, 1)
		cwd, err := os.Getwd()
		if err != nil {
			vm.vmSysReturn(-1)
			return
		}
		n := len(cwd)
		if n >= int(bufSize) {
			n = int(bufSize) - 1
		}
		vm.copyStringToVM(int(bufAddr), cwd, n)
		vm.memory[int(bufAddr)+n] = 0
		vm.vmSysReturn(int64(n))

	case "SysExit":
		ExitCode = int(vm.signExtend(vm.localGet(localsAddr, ws, 0)))
		vm.exited = true

	case "SysMmap":
		size := vm.localGet(localsAddr, ws, 1)
		if size == 0 {
			size = 1
		}
		addr := vm.alloc(size, "mmap")
		vm.vmSysReturn(int64(addr))

	case "SysChmod":
		pathAddr := vm.localGet(localsAddr, ws, 0)
		mode := vm.localGet(localsAddr, ws, 1)
		path := vm.readCString(pathAddr)
		err := os.Chmod(path, os.FileMode(mode))
		if err != nil {
			vm.vmSysReturn(-1)
		} else {
			vm.vmSysReturn(0)
		}

	case "SysGetargc":
		vm.push(uint64(len(vmArgs)) & vm.config.WordMask)
		vm.push(0)
		vm.push(0)

	case "SysGetargv":
		idx := int(vm.localGet(localsAddr, ws, 0))
		if idx >= len(vm.argvAddrs) {
			vm.push(0)
		} else {
			vm.push(vm.argvAddrs[idx])
		}
		vm.push(0)
		vm.push(0)

	case "SysGetenv":
		keyAddr := vm.localGet(localsAddr, ws, 0)
		key := vm.readCString(keyAddr)
		val := os.Getenv(key)
		if val == "" {
			// Could be empty or not set; os.Getenv returns "" for both
			vm.vmSysReturn(0)
		} else {
			addr := vm.writeCString(val)
			vm.vmSysReturn(int64(addr))
		}

	case "SysGetCommandLine":
		cmd := ""
		i := 0
		for i < len(vmArgs) {
			if i > 0 {
				cmd = cmd + " "
			}
			cmd = cmd + vmArgs[i]
			i = i + 1
		}
		vm.vmSysReturn(int64(vm.writeCString(cmd)))

	case "SysGetEnvStrings":
		// Return an empty environment block (double-NUL terminated).
		addr := vm.alloc(2, "env-strings")
		vm.storeN(addr, 0, 1)
		vm.storeN(addr+1, 0, 1)
		vm.vmSysReturn(int64(addr))

	case "ComptimeReadFile":
		pathArg := vm.localGet(localsAddr, ws, 0)
		path := vm.readString(pathArg)
		data, err := os.ReadFile(path)
		if err != nil {
			alt := vm.readCString(pathArg)
			if alt != "" && alt != path {
				if data2, err2 := os.ReadFile(alt); err2 == nil {
					data = data2
					err = nil
				}
			}
		}
		if err != nil {
			vm.push(0)
			vm.push(0)
			return
		}
		s := string(data)
		strAddr, ok := vm.stringAddrs[s]
		if !ok {
			strAddr = vm.internString(s)
		}
		vm.push(strAddr)
		vm.push(1)

	case "SysOpendir":
		pathAddr := vm.localGet(localsAddr, ws, 0)
		path := vm.readCString(pathAddr)
		entries, err := os.ReadDir(path)
		if err != nil {
			vm.vmSysReturn(-1)
			return
		}
		id := vm.nextDirID
		vm.nextDirID = vm.nextDirID + 1
		if id >= 64 {
			vm.vmSysReturn(-1)
			return
		}
		vm.dirEntries[id] = entries
		vm.dirPos[id] = 0
		vm.dirUsed[id] = true
		vm.vmSysReturn(int64(id))

	case "SysReaddir":
		handle := int(vm.localGet(localsAddr, ws, 0))
		nameBuf := vm.localGet(localsAddr, ws, 1)
		nameBufSize := int(vm.localGet(localsAddr, ws, 2))
		if handle < 0 || handle >= 64 || !vm.dirUsed[handle] {
			vm.vmSysReturn(0)
			return
		}
		if vm.dirPos[handle] >= len(vm.dirEntries[handle]) {
			vm.vmSysReturn(0)
			return
		}
		entry := vm.dirEntries[handle][vm.dirPos[handle]]
		vm.dirPos[handle] = vm.dirPos[handle] + 1
		ename := entry.Name()
		n := len(ename)
		if n > nameBufSize {
			n = nameBufSize
		}
		vm.copyStringToVM(int(nameBuf), ename, n)
		vm.vmSysReturn(int64(n))

	case "SysReaddirWithType":
		handle := int(vm.localGet(localsAddr, ws, 0))
		nameBuf := vm.localGet(localsAddr, ws, 1)
		nameBufSize := int(vm.localGet(localsAddr, ws, 2))
		isDirBuf := vm.localGet(localsAddr, ws, 3)
		if handle < 0 || handle >= 64 || !vm.dirUsed[handle] {
			vm.vmSysReturn(0)
			return
		}
		if vm.dirPos[handle] >= len(vm.dirEntries[handle]) {
			vm.vmSysReturn(0)
			return
		}
		entry := vm.dirEntries[handle][vm.dirPos[handle]]
		vm.dirPos[handle] = vm.dirPos[handle] + 1
		ename := entry.Name()
		n := len(ename)
		if n > nameBufSize {
			n = nameBufSize
		}
		vm.copyStringToVM(int(nameBuf), ename, n)
		if isDirBuf != 0 {
			if entry.IsDir() {
				vm.storeN(isDirBuf, 1, 1)
			} else {
				vm.storeN(isDirBuf, 0, 1)
			}
		}
		vm.vmSysReturn(int64(n))

	case "SysClosedir":
		handle := int(vm.localGet(localsAddr, ws, 0))
		if handle >= 0 && handle < 64 {
			vm.dirEntries[handle] = nil
			vm.dirUsed[handle] = false
		}
		vm.vmSysReturn(0)

	case "SysSystem":
		cmdAddr := vm.localGet(localsAddr, ws, 0)
		cmdStr := vm.readCString(cmdAddr)
		cmd := exec.Command("sh", "-c", cmdStr)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		err := cmd.Run()
		if err != nil {
			vm.vmSysReturn(-1)
		} else {
			vm.vmSysReturn(0)
		}

	case "SysPopen":
		cmdAddr := vm.localGet(localsAddr, ws, 0)
		cmdStr := vm.readCString(cmdAddr)
		// Run command, capture output to temp file, return fd to that file
		cmd := exec.Command("sh", "-c", cmdStr)
		cmd.Stderr = os.Stderr
		output, _ := cmd.Output()
		// Write output to a temp file
		tmpPath := fmt.Sprintf("/tmp/rtg-vm-popen-%d", vm.nextFD)
		err := os.WriteFile(tmpPath, output, 0600)
		if err != nil {
			vm.vmSysReturn(-1)
			return
		}
		f, err := os.Open(tmpPath)
		if err != nil {
			vm.vmSysReturn(-1)
			return
		}
		os.RemoveAll(tmpPath) // unlink immediately; fd keeps it open on Unix
		fd := vm.nextFD
		vm.nextFD = vm.nextFD + 1
		if fd >= 256 {
			f.Close()
			vm.vmSysReturn(-1)
			return
		}
		vm.fdFiles[fd] = f
		vm.fdUsed[fd] = true
		vm.fdIsPopen[fd] = true
		vm.vmSysReturn(int64(fd))

	case "SysPclose":
		fd := int(vm.localGet(localsAddr, ws, 0))
		if fd < 3 || fd >= 256 || !vm.fdUsed[fd] {
			vm.vmSysReturn(-1)
			return
		}
		f := vm.fdFiles[fd]
		f.Close()
		vm.fdFiles[fd] = nil
		vm.fdUsed[fd] = false
		vm.fdIsPopen[fd] = false
		vm.vmSysReturn(0)

	case "SysGetpid":
		vm.push(uint64(os.Getpid()) & vm.config.WordMask)
		vm.push(0)
		vm.push(0)

	case "SysGetdents64":
		// Directory entry layout differs by target ABI; not emulated here.
		vm.vmSysReturn(-38)

	case "Syscall":
		num := vm.localGet(localsAddr, ws, 0)
		a0 := vm.localGet(localsAddr, ws, 1)
		a1 := vm.localGet(localsAddr, ws, 2)
		a2 := vm.localGet(localsAddr, ws, 3)
		a3 := vm.localGet(localsAddr, ws, 4)
		a4 := vm.localGet(localsAddr, ws, 5)
		a5 := vm.localGet(localsAddr, ws, 6)
		vm.execSyscallIntrinsic(num, ws, a0, a1, a2, a3, a4, a5)

	case "winVirtualAlloc":
		_ = vm.localGet(localsAddr, ws, 0) // addr (ignored)
		size := vm.localGet(localsAddr, ws, 1)
		_ = vm.localGet(localsAddr, ws, 2) // allocType (ignored)
		_ = vm.localGet(localsAddr, ws, 3) // protect (ignored)
		if size == 0 {
			vm.push(0)
			vm.push(0)
			vm.push(1)
			return
		}
		p := vm.alloc(size, "win-virtualalloc")
		vm.push(p)
		vm.push(0)
		vm.push(0)

	case "AsmAmd64Begin":
		params := int(vm.localGet(localsAddr, ws, 1))
		vm.vmAsmBegin(params)

	case "AsmAmd64Load":
		dst := int(vm.localGet(localsAddr, ws, 0))
		src := int(vm.signExtend(vm.localGet(localsAddr, ws, 1)))
		if src < 0 {
			paramIdx := -1 - src
			vm.vmAsmEmitLoadParam(dst, paramIdx)
			return
		}
		vm.vmAsmEmitMovRegImm64(dst, int64(src))

	case "AsmAmd64Emit":
		n := int(vm.localGet(localsAddr, ws, 0))
		if n < 0 {
			panic("AsmAmd64Emit: negative length")
		}
		if n > 8 {
			panic("AsmAmd64Emit: length exceeds 8")
		}
		i := 0
		for i < n {
			vm.vmAsmEmit(byte(vm.localGet(localsAddr, ws, 1+i)))
			i = i + 1
		}

	case "AsmAmd64Push":
		src := int(vm.localGet(localsAddr, ws, 0))
		vm.vmAsmPushReg(src)

	case "AsmAmd64Pop":
		dst := int(vm.localGet(localsAddr, ws, 0))
		vm.vmAsmPopReg(dst)

	case "AsmAmd64Call0":
		dst := int(vm.localGet(localsAddr, ws, 0))
		target := vm.readString(vm.localGet(localsAddr, ws, 1))
		off := len(vm.asmAmd64Code) + 1
		vm.vmAsmEmit(0xe8, 0x00, 0x00, 0x00, 0x00)
		vm.asmAmd64Fixups = append(vm.asmAmd64Fixups, vmAsmFixup{Off: off, Target: target})
		vm.vmAsmPopReg(dst)

	case "AsmAmd64Call1":
		dst := int(vm.localGet(localsAddr, ws, 0))
		target := vm.readString(vm.localGet(localsAddr, ws, 1))
		a0 := int(vm.localGet(localsAddr, ws, 2))
		vm.vmAsmPushReg(a0)
		off := len(vm.asmAmd64Code) + 1
		vm.vmAsmEmit(0xe8, 0x00, 0x00, 0x00, 0x00)
		vm.asmAmd64Fixups = append(vm.asmAmd64Fixups, vmAsmFixup{Off: off, Target: target})
		vm.vmAsmPopReg(dst)

	case "AsmAmd64Call2":
		dst := int(vm.localGet(localsAddr, ws, 0))
		target := vm.readString(vm.localGet(localsAddr, ws, 1))
		a0 := int(vm.localGet(localsAddr, ws, 2))
		a1 := int(vm.localGet(localsAddr, ws, 3))
		vm.vmAsmPushReg(a0)
		vm.vmAsmPushReg(a1)
		off := len(vm.asmAmd64Code) + 1
		vm.vmAsmEmit(0xe8, 0x00, 0x00, 0x00, 0x00)
		vm.asmAmd64Fixups = append(vm.asmAmd64Fixups, vmAsmFixup{Off: off, Target: target})
		vm.vmAsmPopReg(dst)

	case "AsmAmd64Call3":
		dst := int(vm.localGet(localsAddr, ws, 0))
		target := vm.readString(vm.localGet(localsAddr, ws, 1))
		a0 := int(vm.localGet(localsAddr, ws, 2))
		a1 := int(vm.localGet(localsAddr, ws, 3))
		a2 := int(vm.localGet(localsAddr, ws, 4))
		vm.vmAsmPushReg(a0)
		vm.vmAsmPushReg(a1)
		vm.vmAsmPushReg(a2)
		off := len(vm.asmAmd64Code) + 1
		vm.vmAsmEmit(0xe8, 0x00, 0x00, 0x00, 0x00)
		vm.asmAmd64Fixups = append(vm.asmAmd64Fixups, vmAsmFixup{Off: off, Target: target})
		vm.vmAsmPopReg(dst)

	case "AsmAmd64Call4":
		dst := int(vm.localGet(localsAddr, ws, 0))
		target := vm.readString(vm.localGet(localsAddr, ws, 1))
		a0 := int(vm.localGet(localsAddr, ws, 2))
		a1 := int(vm.localGet(localsAddr, ws, 3))
		a2 := int(vm.localGet(localsAddr, ws, 4))
		a3 := int(vm.localGet(localsAddr, ws, 5))
		vm.vmAsmPushReg(a0)
		vm.vmAsmPushReg(a1)
		vm.vmAsmPushReg(a2)
		vm.vmAsmPushReg(a3)
		off := len(vm.asmAmd64Code) + 1
		vm.vmAsmEmit(0xe8, 0x00, 0x00, 0x00, 0x00)
		vm.asmAmd64Fixups = append(vm.asmAmd64Fixups, vmAsmFixup{Off: off, Target: target})
		vm.vmAsmPopReg(dst)

	case "AsmAmd64Ret":
		vm.vmAsmEmit(0xc9, 0xc3)
		vm.asmAmd64Armed = false

	case "AsmAmd64TakeCode":
		vm.push(vm.vmAsmByteSlice(vm.asmAmd64Code, "asm-code"))

	case "AsmAmd64TakeFixups":
		vm.push(vm.vmAsmByteSlice(vm.vmAsmFixupBytes(), "asm-fixups"))

	case "AsmI386Begin":
		params := int(vm.localGet(localsAddr, ws, 1))
		vm.vmAsmI386Begin(params)

	case "AsmI386Load":
		dst := int(vm.localGet(localsAddr, ws, 0))
		src := int(vm.signExtend(vm.localGet(localsAddr, ws, 1)))
		if src < 0 {
			paramIdx := -1 - src
			vm.vmAsmI386EmitLoadParam(dst, paramIdx)
			return
		}
		vm.vmAsmI386EmitMovRegImm32(dst, int32(src))

	case "AsmI386Emit":
		n := int(vm.localGet(localsAddr, ws, 0))
		if n < 0 {
			panic("AsmI386Emit: negative length")
		}
		if n > 8 {
			panic("AsmI386Emit: length exceeds 8")
		}
		i := 0
		for i < n {
			vm.vmAsmEmit(byte(vm.localGet(localsAddr, ws, 1+i)))
			i = i + 1
		}

	case "AsmI386Push":
		src := int(vm.localGet(localsAddr, ws, 0))
		vm.vmAsmI386PushReg(src)

	case "AsmI386Pop":
		dst := int(vm.localGet(localsAddr, ws, 0))
		vm.vmAsmI386PopReg(dst)

	case "AsmI386Call0":
		dst := int(vm.localGet(localsAddr, ws, 0))
		target := vm.readString(vm.localGet(localsAddr, ws, 1))
		off := len(vm.asmAmd64Code) + 1
		vm.vmAsmEmit(0xe8, 0x00, 0x00, 0x00, 0x00)
		vm.asmAmd64Fixups = append(vm.asmAmd64Fixups, vmAsmFixup{Off: off, Target: target})
		vm.vmAsmI386PopReg(dst)

	case "AsmI386Call1":
		dst := int(vm.localGet(localsAddr, ws, 0))
		target := vm.readString(vm.localGet(localsAddr, ws, 1))
		a0 := int(vm.localGet(localsAddr, ws, 2))
		vm.vmAsmI386PushReg(a0)
		off := len(vm.asmAmd64Code) + 1
		vm.vmAsmEmit(0xe8, 0x00, 0x00, 0x00, 0x00)
		vm.asmAmd64Fixups = append(vm.asmAmd64Fixups, vmAsmFixup{Off: off, Target: target})
		vm.vmAsmI386PopReg(dst)

	case "AsmI386Call2":
		dst := int(vm.localGet(localsAddr, ws, 0))
		target := vm.readString(vm.localGet(localsAddr, ws, 1))
		a0 := int(vm.localGet(localsAddr, ws, 2))
		a1 := int(vm.localGet(localsAddr, ws, 3))
		vm.vmAsmI386PushReg(a0)
		vm.vmAsmI386PushReg(a1)
		off := len(vm.asmAmd64Code) + 1
		vm.vmAsmEmit(0xe8, 0x00, 0x00, 0x00, 0x00)
		vm.asmAmd64Fixups = append(vm.asmAmd64Fixups, vmAsmFixup{Off: off, Target: target})
		vm.vmAsmI386PopReg(dst)

	case "AsmI386Call3":
		dst := int(vm.localGet(localsAddr, ws, 0))
		target := vm.readString(vm.localGet(localsAddr, ws, 1))
		a0 := int(vm.localGet(localsAddr, ws, 2))
		a1 := int(vm.localGet(localsAddr, ws, 3))
		a2 := int(vm.localGet(localsAddr, ws, 4))
		vm.vmAsmI386PushReg(a0)
		vm.vmAsmI386PushReg(a1)
		vm.vmAsmI386PushReg(a2)
		off := len(vm.asmAmd64Code) + 1
		vm.vmAsmEmit(0xe8, 0x00, 0x00, 0x00, 0x00)
		vm.asmAmd64Fixups = append(vm.asmAmd64Fixups, vmAsmFixup{Off: off, Target: target})
		vm.vmAsmI386PopReg(dst)

	case "AsmI386Call4":
		dst := int(vm.localGet(localsAddr, ws, 0))
		target := vm.readString(vm.localGet(localsAddr, ws, 1))
		a0 := int(vm.localGet(localsAddr, ws, 2))
		a1 := int(vm.localGet(localsAddr, ws, 3))
		a2 := int(vm.localGet(localsAddr, ws, 4))
		a3 := int(vm.localGet(localsAddr, ws, 5))
		vm.vmAsmI386PushReg(a0)
		vm.vmAsmI386PushReg(a1)
		vm.vmAsmI386PushReg(a2)
		vm.vmAsmI386PushReg(a3)
		off := len(vm.asmAmd64Code) + 1
		vm.vmAsmEmit(0xe8, 0x00, 0x00, 0x00, 0x00)
		vm.asmAmd64Fixups = append(vm.asmAmd64Fixups, vmAsmFixup{Off: off, Target: target})
		vm.vmAsmI386PopReg(dst)

	case "AsmI386Ret":
		vm.vmAsmEmit(0xc9, 0xc3)
		vm.asmAmd64Armed = false

	case "AsmI386TakeCode":
		vm.push(vm.vmAsmByteSlice(vm.asmAmd64Code, "asm-code"))

	case "AsmI386TakeFixups":
		vm.push(vm.vmAsmByteSlice(vm.vmAsmFixupBytes(), "asm-fixups"))

	case "AsmArm64Begin":
		params := int(vm.localGet(localsAddr, ws, 1))
		vm.vmAsmArm64Begin(params)

	case "AsmArm64Load":
		dst := int(vm.localGet(localsAddr, ws, 0))
		src := int(vm.signExtend(vm.localGet(localsAddr, ws, 1)))
		if src < 0 {
			paramIdx := -1 - src
			vm.vmAsmArm64EmitLoadParam(dst, paramIdx)
			return
		}
		vm.vmAsmArm64LoadImm64(dst, int64(src))

	case "AsmArm64Emit32":
		vm.vmAsmArm64Emit(uint32(vm.localGet(localsAddr, ws, 0)))

	case "AsmArm64Push":
		src := int(vm.localGet(localsAddr, ws, 0))
		vm.vmAsmArm64PushReg(src)

	case "AsmArm64Pop":
		dst := int(vm.localGet(localsAddr, ws, 0))
		vm.vmAsmArm64PopReg(dst)

	case "AsmArm64Call0":
		dst := int(vm.localGet(localsAddr, ws, 0))
		target := vm.readString(vm.localGet(localsAddr, ws, 1))
		off := len(vm.asmAmd64Code)
		vm.vmAsmArm64Emit(0x94000000)
		vm.asmAmd64Fixups = append(vm.asmAmd64Fixups, vmAsmFixup{Off: off, Target: target})
		vm.vmAsmArm64PopReg(dst)

	case "AsmArm64Call1":
		dst := int(vm.localGet(localsAddr, ws, 0))
		target := vm.readString(vm.localGet(localsAddr, ws, 1))
		a0 := int(vm.localGet(localsAddr, ws, 2))
		vm.vmAsmArm64PushReg(a0)
		off := len(vm.asmAmd64Code)
		vm.vmAsmArm64Emit(0x94000000)
		vm.asmAmd64Fixups = append(vm.asmAmd64Fixups, vmAsmFixup{Off: off, Target: target})
		vm.vmAsmArm64PopReg(dst)

	case "AsmArm64Call2":
		dst := int(vm.localGet(localsAddr, ws, 0))
		target := vm.readString(vm.localGet(localsAddr, ws, 1))
		a0 := int(vm.localGet(localsAddr, ws, 2))
		a1 := int(vm.localGet(localsAddr, ws, 3))
		vm.vmAsmArm64PushReg(a0)
		vm.vmAsmArm64PushReg(a1)
		off := len(vm.asmAmd64Code)
		vm.vmAsmArm64Emit(0x94000000)
		vm.asmAmd64Fixups = append(vm.asmAmd64Fixups, vmAsmFixup{Off: off, Target: target})
		vm.vmAsmArm64PopReg(dst)

	case "AsmArm64Call3":
		dst := int(vm.localGet(localsAddr, ws, 0))
		target := vm.readString(vm.localGet(localsAddr, ws, 1))
		a0 := int(vm.localGet(localsAddr, ws, 2))
		a1 := int(vm.localGet(localsAddr, ws, 3))
		a2 := int(vm.localGet(localsAddr, ws, 4))
		vm.vmAsmArm64PushReg(a0)
		vm.vmAsmArm64PushReg(a1)
		vm.vmAsmArm64PushReg(a2)
		off := len(vm.asmAmd64Code)
		vm.vmAsmArm64Emit(0x94000000)
		vm.asmAmd64Fixups = append(vm.asmAmd64Fixups, vmAsmFixup{Off: off, Target: target})
		vm.vmAsmArm64PopReg(dst)

	case "AsmArm64Call4":
		dst := int(vm.localGet(localsAddr, ws, 0))
		target := vm.readString(vm.localGet(localsAddr, ws, 1))
		a0 := int(vm.localGet(localsAddr, ws, 2))
		a1 := int(vm.localGet(localsAddr, ws, 3))
		a2 := int(vm.localGet(localsAddr, ws, 4))
		a3 := int(vm.localGet(localsAddr, ws, 5))
		vm.vmAsmArm64PushReg(a0)
		vm.vmAsmArm64PushReg(a1)
		vm.vmAsmArm64PushReg(a2)
		vm.vmAsmArm64PushReg(a3)
		off := len(vm.asmAmd64Code)
		vm.vmAsmArm64Emit(0x94000000)
		vm.asmAmd64Fixups = append(vm.asmAmd64Fixups, vmAsmFixup{Off: off, Target: target})
		vm.vmAsmArm64PopReg(dst)

	case "AsmArm64Ret":
		vm.vmAsmArm64MovReg(31, 29)
		vm.vmAsmArm64Emit(0xA8C17BFD) // ldp x29, x30, [sp], #16
		vm.vmAsmArm64Emit(0xD65F03C0) // ret
		vm.asmAmd64Armed = false

	case "AsmArm64TakeCode":
		vm.push(vm.vmAsmByteSlice(vm.asmAmd64Code, "asm-code"))

	case "AsmArm64TakeFixups":
		vm.push(vm.vmAsmByteSlice(vm.vmAsmFixupBytes(), "asm-fixups"))

	// Memory intrinsics
	case "Sliceptr":
		a := vm.localGet(localsAddr, ws, 0)
		if a == 0 {
			vm.push(0)
		} else {
			vm.push(vm.loadWord(a))
		}

	case "Makeslice":
		h := vm.slabAllocLarge("makeslice")
		vm.storeWord(h, vm.localGet(localsAddr, ws, 0))
		vm.storeWord(h+ws, vm.localGet(localsAddr, ws, 1))
		vm.storeWord(h+2*ws, vm.localGet(localsAddr, ws, 2))
		vm.storeWord(h+3*ws, 1)
		vm.push(h)

	case "Stringptr":
		a := vm.localGet(localsAddr, ws, 0)
		if a == 0 {
			vm.push(0)
		} else {
			vm.push(vm.loadWord(a))
		}

	case "Makestring":
		h := vm.slabAllocSmall("makestring")
		vm.storeWord(h, vm.localGet(localsAddr, ws, 0))
		vm.storeWord(h+ws, vm.localGet(localsAddr, ws, 1))
		vm.push(h)

	case "Tostring":
		vm.push(vm.tostring(vm.localGet(localsAddr, ws, 0)))

	case "ReadPtr":
		vm.push(vm.loadWord(vm.localGet(localsAddr, ws, 0)))

	case "WritePtr":
		vm.storeWord(vm.localGet(localsAddr, ws, 0), vm.localGet(localsAddr, ws, 1))

	case "WriteByte":
		vm.storeN(vm.localGet(localsAddr, ws, 0), vm.localGet(localsAddr, ws, 1), 1)

	default:
		dot := -1
		i := 0
		for i < len(name) {
			if name[i] == '.' {
				dot = i
			}
			i = i + 1
		}
		if dot >= 0 && dot+1 < len(name) {
			vm.execIntrinsic(name[dot+1:len(name)], localsAddr, ws)
			return
		}
		if len(name) >= 4 && name[0] == 's' && name[1] == 'y' && name[2] == 's' {
			mapped := "S" + name[1:len(name)]
			vm.execIntrinsic(mapped, localsAddr, ws)
			return
		}
		fmt.Fprintf(os.Stderr, "vm: unknown intrinsic %q\n", name)
		vm.exited = true
		ExitCode = 2
	}
}

// vmArgs holds the arguments passed to the VM program.
var vmArgs []string

func SetArgs(args []string) {
	vmArgs = append(vmArgs[:0], args...)
}

// Suppress unused import warnings.
var _ = strings.HasPrefix
var _ = fmt.Sprintf
