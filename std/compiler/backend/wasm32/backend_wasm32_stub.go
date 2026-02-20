//go:build no_backend_wasi_wasm32

package main

import "fmt"

func generateWasm32(irmod *IRModule, outputPath string) error {
	return fmt.Errorf("wasm32 backend disabled (built with no_backend_wasi_wasm32 tag)")
}
