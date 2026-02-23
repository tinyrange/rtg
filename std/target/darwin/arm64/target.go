//go:build !no_backend_arm64

package arm64

import (
	"fmt"
	"os"

	"j5.nz/rtg/std/compiler/backend/aarch64"
	aarch64macos "j5.nz/rtg/std/compiler/backend/aarch64/macos"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
	"j5.nz/rtg/std/target"
)

type darwinArm64Driver struct{}

func (d darwinArm64Driver) Configure(_ *common.Target) error {
	return nil
}

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

type darwinArm64ABIConfig struct {
	ImageBase        uint64
	ExtraGlobals     int
	WithGOT          bool
	OperandStackSize int64
	MmapProt         int64
	MmapFlags        int64
	MmapFD           int64
	MmapOffset       int64
	ExitCode         int64
	FixupSkipMask    int
	MmapSymbol       string
	ExitSymbol       string
}

func darwinArm64ABIConfigFromProvider(provider target.ABIProvider) darwinArm64ABIConfig {
	cfg := darwinArm64ABIConfig{
		ImageBase:        0x100000000,
		ExtraGlobals:     3,
		WithGOT:          true,
		OperandStackSize: 1048576,
		MmapProt:         3,
		MmapFlags:        0x1002,
		MmapFD:           -1,
		MmapOffset:       0,
		ExitCode:         0,
		FixupSkipMask: aarch64.FixupSkipRodataHeader |
			aarch64.FixupSkipDataAddr |
			aarch64.FixupSkipGotAddr,
		MmapSymbol: "_mmap",
		ExitSymbol: "_exit",
	}
	cfg.ImageBase = target.ABIUint64(provider, "image_base", cfg.ImageBase)
	cfg.ExtraGlobals = int(target.ABIInt64(provider, "extra_globals", int64(cfg.ExtraGlobals)))
	cfg.WithGOT = target.ABIBool(provider, "with_got", cfg.WithGOT)
	cfg.OperandStackSize = target.ABIInt64(provider, "operand_stack_size", cfg.OperandStackSize)
	cfg.MmapProt = target.ABIInt64(provider, "mmap_prot", cfg.MmapProt)
	cfg.MmapFlags = target.ABIInt64(provider, "mmap_flags", cfg.MmapFlags)
	cfg.MmapFD = target.ABIInt64(provider, "mmap_fd", cfg.MmapFD)
	cfg.MmapOffset = target.ABIInt64(provider, "mmap_offset", cfg.MmapOffset)
	cfg.ExitCode = target.ABIInt64(provider, "exit_code", cfg.ExitCode)
	cfg.FixupSkipMask = int(target.ABIInt64(provider, "fixup_skip_mask", int64(cfg.FixupSkipMask)))
	cfg.MmapSymbol = target.ABIString(provider, "mmap_symbol", cfg.MmapSymbol)
	cfg.ExitSymbol = target.ABIString(provider, "exit_symbol", cfg.ExitSymbol)
	return cfg
}

func (d darwinArm64Driver) Generate(tgt *common.Target, irmod *ir.IRModule, outputPath string) error {
	abi := darwinArm64ABIConfigFromProvider(darwinArm64ABIProviderForCodegen())
	g := aarch64.NewCodeGen(tgt, irmod, abi.ImageBase, abi.ExtraGlobals, abi.WithGOT)

	emitDarwinArm64Start(g, irmod, abi)
	g.CompileModuleFuncs(irmod)
	g.CollectNativeFuncSizes(irmod)
	if g.NeedTostringHelper() {
		g.EmitTostringHelperArm64()
	}

	unresolved := g.ResolveCallFixups(abi.FixupSkipMask)
	if len(unresolved) > 0 {
		fmt.Fprintf(os.Stderr, "error: %d unresolved calls:\n", len(unresolved))
		seen := make(map[string]bool)
		for _, name := range unresolved {
			if !seen[name] {
				fmt.Fprintf(os.Stderr, "  %s\n", name)
				seen[name] = true
			}
		}
		return fmt.Errorf("%d unresolved calls", len(unresolved))
	}

	binName := outputPath
	lastSlash := -1
	for i := 0; i < len(outputPath); i++ {
		if outputPath[i] == '/' {
			lastSlash = i
		}
	}
	if lastSlash >= 0 {
		binName = outputPath[lastSlash+1:]
	}

	macho := aarch64macos.BuildMachO64(g, irmod, binName)
	if err := os.WriteFile(outputPath, macho, 0755); err != nil {
		return fmt.Errorf("write output: %v", err)
	}
	_ = os.Chmod(outputPath, 0755)
	return nil
}

func emitDarwinArm64Start(g *aarch64.CodeGen, irmod *ir.IRModule, abi darwinArm64ABIConfig) {
	// dyld enters LC_MAIN as a normal function.
	g.EmitStp(aarch64.REG_FP, aarch64.REG_LR, aarch64.REG_SP, -16)
	g.EmitMovRRArm64(aarch64.REG_FP, aarch64.REG_SP)

	argcGlobalOff := len(irmod.Globals) * 8
	argvGlobalOff := (len(irmod.Globals) + 1) * 8
	envpGlobalOff := (len(irmod.Globals) + 2) * 8

	g.EmitAdrpAdd(aarch64.REG_X3, "$data_addr$", uint64(argcGlobalOff))
	g.EmitStr(aarch64.REG_X0, aarch64.REG_X3, 0)
	g.EmitAdrpAdd(aarch64.REG_X3, "$data_addr$", uint64(argvGlobalOff))
	g.EmitStr(aarch64.REG_X1, aarch64.REG_X3, 0)
	g.EmitAdrpAdd(aarch64.REG_X3, "$data_addr$", uint64(envpGlobalOff))
	g.EmitStr(aarch64.REG_X2, aarch64.REG_X3, 0)

	g.EmitMovZ(aarch64.REG_X0, 0, 0)
	g.EmitLoadImm64Compact(aarch64.REG_X1, uint64(abi.OperandStackSize))
	g.EmitLoadImm64Compact(aarch64.REG_X2, uint64(abi.MmapProt))
	g.EmitLoadImm64Compact(aarch64.REG_X3, uint64(abi.MmapFlags))
	g.EmitLoadImm64Compact(aarch64.REG_X4, uint64(abi.MmapFD))
	g.EmitLoadImm64Compact(aarch64.REG_X5, uint64(abi.MmapOffset))
	g.EmitCallGOT(abi.MmapSymbol)

	g.EmitLoadImm64Compact(aarch64.REG_X1, uint64(abi.OperandStackSize))
	g.EmitAddRR(aarch64.REG_X28, aarch64.REG_X0, aarch64.REG_X1)

	for _, f := range irmod.Funcs {
		if ir.IsInitFunc(f.Name) {
			g.EmitCallPlaceholderArm64(f.Name)
		}
	}
	g.EmitCallPlaceholderArm64("main.main")

	g.EmitMovZ(aarch64.REG_X0, uint16(abi.ExitCode), 0)
	g.EmitCallGOT(abi.ExitSymbol)

	g.EmitMovRRArm64(aarch64.REG_SP, aarch64.REG_FP)
	g.EmitLdp(aarch64.REG_FP, aarch64.REG_LR, aarch64.REG_SP, 16)
	g.EmitRet()
}

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

func darwinArm64ABIProviderForCodegen() target.ABIProvider {
	return target.ABIProvider{
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
			"fixup_skip_mask": int64(aarch64.FixupSkipRodataHeader |
				aarch64.FixupSkipDataAddr |
				aarch64.FixupSkipGotAddr),
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
