//go:build no_backend_windows_amd64

package main

import "fmt"

func generateWinAmd64PE(irmod *IRModule, outputPath string) error {
	return fmt.Errorf("windows/amd64 backend disabled (built with no_backend_windows_amd64 tag)")
}
