//go:build no_backend_windows_i386

package main

import "fmt"

func generateWin386PE(irmod *IRModule, outputPath string) error {
	return fmt.Errorf("windows/386 backend disabled (built with no_backend_windows_i386 tag)")
}
