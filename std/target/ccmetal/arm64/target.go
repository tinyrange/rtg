package arm64

import "j5.nz/rtg/std/target"

//rtg:target ccmetal/arm64
func ccmetalArm64TargetSpec() target.Spec {
	return target.Spec{
		Triple: "ccmetal/arm64",
		Defaults: target.Defaults{
			GOOS:     "ccmetal",
			GOARCH:   "arm64",
			PtrSize:  8,
			WordSize: 8,
			Backend:  "native",
		},
	}
}

//rtg:targetabi ccmetal/arm64
func ccmetalArm64ABIConfigForTarget() target.GenericABI {
	return target.GenericABI{
		Kind: "ccmetal/arm64",
		U64: map[string]uint64{
			"image_base":         0x400000,
			"hypercall_mmio":     0x80000000,
			"operand_stack_size": 1048576,
		},
	}
}
