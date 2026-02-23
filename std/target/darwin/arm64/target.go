//go:build !no_backend_arm64

package arm64

import (
	aarch64macos "j5.nz/rtg/std/compiler/backend/aarch64/macos"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
	"j5.nz/rtg/std/target"
)

const packagePath = "j5.nz/rtg/std/target/darwin/arm64"

type darwinArm64Driver struct{}

func init() {
	target.Register(target.Spec{
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
	})
}

func (d darwinArm64Driver) Configure(_ *common.Target) error {
	return nil
}

func (d darwinArm64Driver) Generate(tgt *common.Target, irmod *ir.IRModule, outputPath string) error {
	return aarch64macos.Generate(tgt, irmod, outputPath)
}
