//go:build !exp_ir_binary

package main

import "fmt"

func writeIRBinary(irmod *IRModule, path string) error {
	return fmt.Errorf("IR binary I/O is experimental; rebuild with -tags exp_ir_binary")
}

func readIRBinary(path string) (*IRModule, error) {
	return nil, fmt.Errorf("IR binary I/O is experimental; rebuild with -tags exp_ir_binary")
}
