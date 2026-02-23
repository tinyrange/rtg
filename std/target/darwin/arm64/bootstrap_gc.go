//go:build gc

package arm64

import "j5.nz/rtg/std/target"

func init() {
	target.Register(darwinArm64TargetSpec())
	target.RegisterABI("darwin/arm64", darwinArm64ABIConfigForTarget())
}
