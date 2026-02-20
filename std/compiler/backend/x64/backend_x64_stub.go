//go:build no_backend_linux_amd64 && no_backend_windows_amd64

package main

import "fmt"

func generateAmd64ELF(irmod *IRModule, outputPath string) error {
	return fmt.Errorf("amd64 backend disabled (built with no_backend_linux_amd64 tag)")
}
