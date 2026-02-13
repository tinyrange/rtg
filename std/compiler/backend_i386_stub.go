//go:build no_backend_linux_i386

package main

import "fmt"

func generateI386ELF(irmod *IRModule, outputPath string) error {
	return fmt.Errorf("i386 backend disabled (built with no_backend_linux_i386 tag)")
}
