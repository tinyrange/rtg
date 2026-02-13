//go:build no_backend_c

package main

import "fmt"

func generateCSource(irmod *IRModule, outputPath string) error {
	return fmt.Errorf("c backend disabled (built with no_backend_c tag)")
}
