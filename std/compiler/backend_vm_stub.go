//go:build no_backend_vm

package main

import "fmt"

var vmExitCode int

func generateVM(irmod *IRModule, outputPath string) error {
	return fmt.Errorf("vm backend disabled (built with no_backend_vm tag)")
}
