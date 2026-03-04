//go:build !exp_ir_binary

package binary

import (
	"fmt"

	"j5.nz/rtg/std/compiler/ir"
)

const IrBinaryEnabled = false

func WriteIRBinary(irmod *ir.IRModule, path string) error {
	return fmt.Errorf("IR binary I/O is experimental; rebuild with -tags exp_ir_binary")
}

func ReadIRBinary(path string) (*ir.IRModule, error) {
	return nil, fmt.Errorf("IR binary I/O is experimental; rebuild with -tags exp_ir_binary")
}
