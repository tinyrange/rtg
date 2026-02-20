//go:build no_backend_wasi_wasm32

package wasm32

import (
	"fmt"

	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

func Generate(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	return fmt.Errorf("wasm32 backend disabled (built with no_backend_wasi_wasm32 tag)")
}
