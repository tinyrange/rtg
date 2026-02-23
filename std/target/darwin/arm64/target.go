//go:build !no_backend_arm64

package arm64

import (
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/target"
)

type darwinArm64Driver struct{}

func (d darwinArm64Driver) Configure(_ *common.Target) error {
	return nil
}

const packagePath = "j5.nz/rtg/std/target/darwin/arm64"

//rtg:target darwin/arm64
func darwinArm64TargetSpec() target.Spec {
	return target.Spec{
		Triple:      "darwin/arm64",
		PackagePath: packagePath,
		Defaults: target.Defaults{
			GOOS:     "darwin",
			GOARCH:   "arm64",
			PtrSize:  8,
			WordSize: 8,
			Backend:  "native",
		},
		Driver: darwinArm64Driver{},
	}
}
