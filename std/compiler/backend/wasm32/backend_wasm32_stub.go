//go:build no_backend_wasi_wasm32

package wasm32

import (
	"fmt"

	"github.com/rtg-project/rtg/compiler/backend/common"
	"github.com/rtg-project/rtg/compiler/ir"
)

func Generate(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	return fmt.Errorf("wasm32 backend disabled (built with no_backend_wasi_wasm32 tag)")
}
