//go:build !no_backend_arm64

package backend

import (
	"fmt"
	"os"

	"j5.nz/rtg/std/compiler/backend/aarch64"
	aarch64macos "j5.nz/rtg/std/compiler/backend/aarch64/macos"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
	targetcfg "j5.nz/rtg/std/target"
)

type builtinAArch64MachoABI struct {
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

func defaultBuiltinAArch64MachoABI() builtinAArch64MachoABI {
	return builtinAArch64MachoABI{
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
}

func parseBuiltinAArch64MachoABI(abi targetcfg.ABIProvider) (builtinAArch64MachoABI, error) {
	cfg := defaultBuiltinAArch64MachoABI()
	kind := targetcfg.ABIKind(abi)
	if kind != "none" && kind != "generic" && kind != "darwin/arm64" {
		return cfg, fmt.Errorf("unsupported ABI kind %q for builtin.aarch64+builtin.macho64", kind)
	}
	cfg.ImageBase = targetcfg.ABIUint64(abi, "image_base", cfg.ImageBase)
	cfg.ExtraGlobals = int(targetcfg.ABIInt64(abi, "extra_globals", int64(cfg.ExtraGlobals)))
	cfg.WithGOT = targetcfg.ABIBool(abi, "with_got", cfg.WithGOT)
	cfg.OperandStackSize = targetcfg.ABIInt64(abi, "operand_stack_size", cfg.OperandStackSize)
	cfg.MmapProt = targetcfg.ABIInt64(abi, "mmap_prot", cfg.MmapProt)
	cfg.MmapFlags = targetcfg.ABIInt64(abi, "mmap_flags", cfg.MmapFlags)
	cfg.MmapFD = targetcfg.ABIInt64(abi, "mmap_fd", cfg.MmapFD)
	cfg.MmapOffset = targetcfg.ABIInt64(abi, "mmap_offset", cfg.MmapOffset)
	cfg.ExitCode = targetcfg.ABIInt64(abi, "exit_code", cfg.ExitCode)
	cfg.FixupSkipMask = int(targetcfg.ABIInt64(abi, "fixup_skip_mask", int64(cfg.FixupSkipMask)))
	cfg.MmapSymbol = targetcfg.ABIString(abi, "mmap_symbol", cfg.MmapSymbol)
	cfg.ExitSymbol = targetcfg.ABIString(abi, "exit_symbol", cfg.ExitSymbol)
	return cfg, nil
}

func emitBuiltinAArch64MachoStart(g *aarch64.CodeGen, irmod *ir.IRModule, abi builtinAArch64MachoABI) {
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

func generateBuiltinAArch64Macho(tgt *common.Target, irmod *ir.IRModule, outputPath string, abi targetcfg.ABIProvider) error {
	cfg, err := parseBuiltinAArch64MachoABI(abi)
	if err != nil {
		return err
	}
	g := aarch64.NewCodeGen(tgt, irmod, cfg.ImageBase, cfg.ExtraGlobals, cfg.WithGOT)

	emitBuiltinAArch64MachoStart(g, irmod, cfg)
	g.CompileModuleFuncs(irmod)
	g.CollectNativeFuncSizes(irmod)
	if g.NeedTostringHelper() {
		g.EmitTostringHelperArm64()
	}

	unresolved := g.ResolveCallFixups(cfg.FixupSkipMask)
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

func generateRegisteredProfile(spec targetcfg.Spec, tgt *common.Target, irmod *ir.IRModule, outputPath string) (bool, error) {
	if spec.Assembler == "" || spec.BinFormat == "" {
		return false, nil
	}
	asmProvider := spec.Assembler
	if p, ok := targetcfg.LookupAssembler(spec.Assembler); ok && p != "" {
		asmProvider = p
	}
	fmtProvider := spec.BinFormat
	if p, ok := targetcfg.LookupBinFormat(spec.BinFormat); ok && p != "" {
		fmtProvider = p
	}

	if asmProvider == "builtin.aarch64" && fmtProvider == "builtin.macho64" {
		abi, _ := targetcfg.LookupABI(spec.Triple)
		return true, generateBuiltinAArch64Macho(tgt, irmod, outputPath, abi)
	}

	return true, fmt.Errorf(
		"unsupported target profile for %s: assembler=%s (%s), binfmt=%s (%s)",
		spec.Triple, spec.Assembler, asmProvider, spec.BinFormat, fmtProvider,
	)
}
