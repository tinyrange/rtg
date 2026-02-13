//go:build no_backend_darwin_arm64

package main

import "fmt"

func generateDarwinArm64(irmod *IRModule, outputPath string) error {
	return fmt.Errorf("darwin/arm64 backend disabled (built with no_backend_darwin_arm64 tag)")
}
