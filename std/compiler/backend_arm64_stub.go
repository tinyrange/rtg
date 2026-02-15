//go:build no_backend_arm64

package main

import "fmt"

func generateDarwinArm64(irmod *IRModule, outputPath string) error {
	return fmt.Errorf("arm64 backend disabled (built with no_backend_arm64 tag)")
}

func generateLinuxArm64ELF(irmod *IRModule, outputPath string) error {
	return fmt.Errorf("arm64 backend disabled (built with no_backend_arm64 tag)")
}
