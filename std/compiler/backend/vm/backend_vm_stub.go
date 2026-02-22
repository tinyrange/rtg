//go:build no_backend_vm

package vm

import (
	"fmt"

	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

var ExitCode int

type EvalState struct{}

func SetArgs(args []string) {}

func Generate(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	return fmt.Errorf("vm backend disabled (built with no_backend_vm tag)")
}

func NewEvalState(target *common.Target, irmod *ir.IRModule) (*EvalState, error) {
	return nil, fmt.Errorf("vm backend disabled (built with no_backend_vm tag)")
}

func (e *EvalState) Call(funcName string, args []uint64, retCount int) ([]uint64, error) {
	return nil, fmt.Errorf("vm backend disabled (built with no_backend_vm tag)")
}

func (e *EvalState) WordSize() int {
	return 0
}

func (e *EvalState) LoadWord(addr uint64) uint64 {
	return 0
}

func (e *EvalState) LoadBytes(addr uint64, n int) ([]byte, error) {
	return nil, fmt.Errorf("vm backend disabled (built with no_backend_vm tag)")
}

func EvalCall(e *EvalState, funcName string, args []uint64, retCount int) ([]uint64, error) {
	return nil, fmt.Errorf("vm backend disabled (built with no_backend_vm tag)")
}

func EvalWordSize(e *EvalState) int {
	return 0
}

func EvalLoadWord(e *EvalState, addr uint64) uint64 {
	return 0
}

func EvalLoadBytes(e *EvalState, addr uint64, n int) ([]byte, error) {
	return nil, fmt.Errorf("vm backend disabled (built with no_backend_vm tag)")
}
