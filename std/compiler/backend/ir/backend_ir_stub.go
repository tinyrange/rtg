//go:build no_backend_ir

package main

import "fmt"

func generateIRText(irmod *IRModule, outputPath string) error {
	return fmt.Errorf("ir backend disabled (built with no_backend_ir tag)")
}
