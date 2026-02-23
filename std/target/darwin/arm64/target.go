package arm64

import "j5.nz/rtg/std/target"

//rtg:target darwin/arm64
func darwinArm64TargetSpec() target.Spec {
	return target.Spec{
		Triple: "darwin/arm64",
		Defaults: target.Defaults{
			GOOS:     "darwin",
			GOARCH:   "arm64",
			PtrSize:  8,
			WordSize: 8,
			Backend:  "native",
		},
		Assembler: "aarch64",
		BinFormat: "macho64",
	}
}

//rtg:assembler aarch64
func darwinArm64AssemblerProvider() string { return "builtin.aarch64" }

//rtg:binfmt macho64
func darwinArm64BinFmtProvider() string { return "builtin.macho64" }

//rtg:targetabi darwin/arm64
func darwinArm64ABIConfigForTarget() target.GenericABI {
	return target.GenericABI{
		Kind: "darwin/arm64",
		U64: map[string]uint64{
			"image_base": 0x100000000,
		},
		I64: map[string]int64{
			"extra_globals":      3,
			"operand_stack_size": 1048576,
			"mmap_prot":          3,
			"mmap_flags":         0x1002,
			"mmap_fd":            -1,
			"mmap_offset":        0,
			"exit_code":          0,
			"fixup_skip_mask":    7,
		},
		Str: map[string]string{
			"mmap_symbol": "_mmap",
			"exit_symbol": "_exit",
		},
		Bool: map[string]bool{
			"with_got": true,
		},
	}
}
