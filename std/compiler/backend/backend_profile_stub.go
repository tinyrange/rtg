//go:build no_backend_arm64

package backend

import (
	"fmt"

	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
	targetcfg "j5.nz/rtg/std/target"
)

func generateRegisteredProfile(spec targetcfg.Spec, _ *common.Target, _ *ir.IRModule, _ string) (bool, error) {
	if spec.Assembler == "" || spec.BinFormat == "" {
		return false, nil
	}
	return true, fmt.Errorf(
		"target profile for %s requires arm64 backend support (built with no_backend_arm64 tag)",
		spec.Triple,
	)
}
