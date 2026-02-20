//go:build no_backend_vm

package vm

import (
	"fmt"

	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

var ExitCode int

func SetArgs(args []string) {}

func Generate(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	return fmt.Errorf("vm backend disabled (built with no_backend_vm tag)")
}
