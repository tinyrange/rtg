//go:build !no_backend_vm

package vm

import (
	"fmt"
	"os"

	"j5.nz/rtg/std/compiler/backend/becommon"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// EvalState holds a VM instance configured for compile-time function calls.
type EvalState struct {
	vm *VM
}

// NewEvalState builds a VM, runs package init functions, and returns an evaluator.
func NewEvalState(target *common.Target, irmod *ir.IRModule) (*EvalState, error) {
	cfg := newVMConfig(target.WordSize)
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

	vm.fdFiles[0] = os.Stdin
	vm.fdUsed[0] = true
	vm.fdFiles[1] = os.Stdout
	vm.fdUsed[1] = true
	vm.fdFiles[2] = os.Stderr
	vm.fdUsed[2] = true

	for _, f := range irmod.Funcs {
		vm.funcs[f.Name] = f
	}

	vm.numGlobals = len(irmod.Globals)
	if vm.numGlobals > 0 {
		vm.globalsAddr = vm.alloc(uint64(vm.numGlobals)*uint64(vm.config.WordSize), "globals")
	}

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

	vm.buildDispatchTable(irmod)

	if f, ok := vm.funcs["runtime.IntToString"]; ok {
		vm.intToStringFunc = f
	}
	if f, ok := vm.funcs["runtime.BytesToString"]; ok {
		vm.bytesToStringFunc = f
	}
	if f, ok := vm.funcs["runtime.StringToBytes"]; ok {
		vm.stringToBytesFunc = f
	}

	vm.argvAddrs = make([]uint64, len(vmArgs))
	for i := 0; i < len(vmArgs); i++ {
		vm.argvAddrs[i] = vm.writeCString(vmArgs[i])
	}

	vm.frameStackSize = 64 * 1024
	vm.frameStackBase = int(vm.alloc(uint64(vm.frameStackSize), "frame-stack"))
	vm.frameStackTop = vm.frameStackBase + vm.frameStackSize
	vm.slabPageSize = 65536
	vm.slabSmallSize = 2 * vm.config.WordSize
	vm.slabLargeSize = 4 * vm.config.WordSize

	for _, f := range irmod.Funcs {
		if ir.IsInitFunc(f.Name) {
			vm.execFunc(f)
			if vm.exited {
				return nil, fmt.Errorf("vm exited while running init function %s", f.Name)
			}
		}
	}

	return &EvalState{vm: vm}, nil
}

func (e *EvalState) WordSize() int {
	return e.vm.config.WordSize
}

func EvalWordSize(e *EvalState) int {
	if e == nil {
		return 0
	}
	return e.WordSize()
}

func (e *EvalState) LoadWord(addr uint64) uint64 {
	return e.vm.loadWord(addr)
}

func EvalLoadWord(e *EvalState, addr uint64) uint64 {
	if e == nil {
		return 0
	}
	return e.LoadWord(addr)
}

func (e *EvalState) LoadBytes(addr uint64, n int) ([]byte, error) {
	if n < 0 {
		return nil, fmt.Errorf("negative byte count: %d", n)
	}
	a := int(addr)
	if a < 0 || a+n > len(e.vm.memory) {
		return nil, fmt.Errorf("out-of-bounds memory read at 0x%s (%d bytes)", hexAddr(addr), n)
	}
	buf := make([]byte, n)
	i := 0
	for i < n {
		buf[i] = e.vm.memory[a+i]
		i = i + 1
	}
	return buf, nil
}

func EvalLoadBytes(e *EvalState, addr uint64, n int) ([]byte, error) {
	if e == nil {
		return nil, fmt.Errorf("nil eval state")
	}
	return e.LoadBytes(addr, n)
}

// Call executes funcName with args and returns its return values.
func (e *EvalState) Call(funcName string, args []uint64, retCount int) ([]uint64, error) {
	f, ok := e.vm.funcs[funcName]
	if !ok {
		return nil, fmt.Errorf("vm function not found: %s", funcName)
	}
	startSP := e.vm.sp
	for _, arg := range args {
		e.vm.push(arg)
	}
	e.vm.execFunc(f)
	if e.vm.exited {
		return nil, fmt.Errorf("vm exited while executing %s", funcName)
	}
	if retCount < 0 {
		retCount = 0
	}
	if e.vm.sp < retCount {
		return nil, fmt.Errorf("stack underflow collecting return values from %s", funcName)
	}
	rets := make([]uint64, retCount)
	for i := retCount - 1; i >= 0; i-- {
		rets[i] = e.vm.pop()
	}
	if e.vm.sp != startSP {
		for e.vm.sp > startSP {
			e.vm.pop()
		}
		return nil, fmt.Errorf("stack imbalance after %s", funcName)
	}
	return rets, nil
}

func EvalCall(e *EvalState, funcName string, args []uint64, retCount int) ([]uint64, error) {
	if e == nil {
		return nil, fmt.Errorf("nil eval state")
	}
	return e.Call(funcName, args, retCount)
}
