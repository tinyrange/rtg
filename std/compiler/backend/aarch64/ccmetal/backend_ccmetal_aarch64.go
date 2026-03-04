//go:build !no_backend_arm64

package ccmetal

import (
	"fmt"
	"os"

	"j5.nz/rtg/std/compiler/backend/aarch64"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
	targetcfg "j5.nz/rtg/std/target"
)

const (
	defaultImageBase      = uint64(0x400000)
	defaultHypercallMMIO  = uint64(0x80000000)
	defaultOperandStackSz = uint64(1048576)
)

type config struct {
	imageBase        uint64
	hypercallMMIO    uint64
	operandStackSize uint64
}

func parseUnsignedDefine(raw string) (uint64, bool) {
	if raw == "" {
		return 0, false
	}
	base := uint64(10)
	i := 0
	if len(raw) > 2 && raw[0] == '0' {
		if raw[1] == 'x' || raw[1] == 'X' {
			base = 16
			i = 2
		} else if raw[1] == 'b' || raw[1] == 'B' {
			base = 2
			i = 2
		} else if raw[1] == 'o' || raw[1] == 'O' {
			base = 8
			i = 2
		}
	}
	var v uint64
	digits := 0
	for i < len(raw) {
		ch := raw[i]
		i = i + 1
		if ch == '_' {
			continue
		}
		d := int64(-1)
		if ch >= '0' && ch <= '9' {
			d = int64(ch - '0')
		} else if ch >= 'a' && ch <= 'f' {
			d = int64(ch-'a') + 10
		} else if ch >= 'A' && ch <= 'F' {
			d = int64(ch-'A') + 10
		}
		if d < 0 || uint64(d) >= base {
			return 0, false
		}
		v = v*base + uint64(d)
		digits = digits + 1
	}
	if digits == 0 {
		return 0, false
	}
	return v, true
}

func applyUnsignedDefine(defines map[string]string, key string, cur uint64) uint64 {
	if defines == nil {
		return cur
	}
	raw, ok := defines[key]
	if !ok {
		return cur
	}
	v, ok := parseUnsignedDefine(raw)
	if !ok {
		return cur
	}
	return v
}

func configForTarget(tgt *common.Target) config {
	cfg := config{
		imageBase:        defaultImageBase,
		hypercallMMIO:    defaultHypercallMMIO,
		operandStackSize: defaultOperandStackSz,
	}
	if tgt == nil {
		return cfg
	}
	if abi, ok := targetcfg.LookupABI(tgt.Triple); ok {
		cfg.imageBase = targetcfg.ABIUint64(abi, "image_base", cfg.imageBase)
		cfg.hypercallMMIO = targetcfg.ABIUint64(abi, "hypercall_mmio", cfg.hypercallMMIO)
		cfg.operandStackSize = targetcfg.ABIUint64(abi, "operand_stack_size", cfg.operandStackSize)
	}
	cfg.imageBase = applyUnsignedDefine(tgt.Defines, "ccmetal.image_base", cfg.imageBase)
	cfg.imageBase = applyUnsignedDefine(tgt.Defines, "image_base", cfg.imageBase)
	cfg.hypercallMMIO = applyUnsignedDefine(tgt.Defines, "ccmetal.hypercall_mmio", cfg.hypercallMMIO)
	cfg.hypercallMMIO = applyUnsignedDefine(tgt.Defines, "hypercall_mmio", cfg.hypercallMMIO)
	cfg.operandStackSize = applyUnsignedDefine(tgt.Defines, "ccmetal.operand_stack_size", cfg.operandStackSize)
	cfg.operandStackSize = applyUnsignedDefine(tgt.Defines, "operand_stack_size", cfg.operandStackSize)
	return cfg
}

func emitHypercall(g *aarch64.CodeGen, mmio uint64) {
	g.EmitLoadImm64Compact(aarch64.REG_X16, mmio)
	// The VMM observes a native-width store to the doorbell MMIO address.
	g.EmitStr(aarch64.REG_X8, aarch64.REG_X16, 0)
}

func emitStart(g *aarch64.CodeGen, irmod *ir.IRModule, cfg config) {
	// mmap(NULL, operandStackSize, PROT_READ|PROT_WRITE, MAP_PRIVATE|MAP_ANON, -1, 0)
	g.EmitMovZ(aarch64.REG_X0, 0, 0)
	g.EmitLoadImm64Compact(aarch64.REG_X1, cfg.operandStackSize)
	g.EmitLoadImm64Compact(aarch64.REG_X2, 3)
	g.EmitLoadImm64Compact(aarch64.REG_X3, 0x22)
	g.EmitLoadImm64Compact(aarch64.REG_X4, 0xFFFFFFFFFFFFFFFF)
	g.EmitMovZ(aarch64.REG_X5, 0, 0)
	g.EmitLoadImm64Compact(aarch64.REG_X8, 222)
	emitHypercall(g, cfg.hypercallMMIO)

	// X28 = mmap result + stackSize (operand stack grows down).
	g.EmitLoadImm64Compact(aarch64.REG_X1, cfg.operandStackSize)
	g.EmitAddRR(aarch64.REG_X28, aarch64.REG_X0, aarch64.REG_X1)

	for _, f := range irmod.Funcs {
		if ir.IsInitFunc(f.Name) {
			g.EmitCallPlaceholderArm64(f.Name)
		}
	}

	g.EmitCallPlaceholderArm64("main.main")

	// exit_group(0)
	g.EmitMovZ(aarch64.REG_X0, 0, 0)
	g.EmitLoadImm64Compact(aarch64.REG_X8, 94)
	emitHypercall(g, cfg.hypercallMMIO)
}

// GenerateFlatBinary compiles an IRModule to a flat ARM64 binary for ccmetal.
func GenerateFlatBinary(tgt *common.Target, irmod *ir.IRModule, outputPath string) error {
	flat, err := GenerateFlatBinaryToBytes(tgt, irmod)
	if err != nil {
		return fmt.Errorf("generate flat binary: %w", err)
	}

	if err := os.WriteFile(outputPath, flat, 0755); err != nil {
		return fmt.Errorf("write output: %v", err)
	}
	return nil
}

func GenerateFlatBinaryToBytes(tgt *common.Target, irmod *ir.IRModule) ([]byte, error) {
	cfg := configForTarget(tgt)
	if tgt.Defines == nil {
		tgt.Defines = make(map[string]string)
	}
	tgt.Defines["ccmetal.hypercall_mmio"] = fmt.Sprintf("0x%x", cfg.hypercallMMIO)
	if _, ok := tgt.Defines["runtime.GOOS"]; !ok {
		tgt.Defines["runtime.GOOS"] = "ccmetal"
	}

	g := aarch64.NewCodeGen(tgt, irmod, cfg.imageBase, 0, false)

	emitStart(g, irmod, cfg)
	g.CompileModuleFuncs(irmod)
	g.CollectNativeFuncSizes(irmod)
	if g.NeedTostringHelper() {
		g.EmitTostringHelperArm64()
	}

	unresolved := g.ResolveCallFixups(aarch64.FixupSkipRodataHeader | aarch64.FixupSkipDataAddr)
	if len(unresolved) > 0 {
		fmt.Fprintf(os.Stderr, "error: %d unresolved calls:\n", len(unresolved))
		seen := make(map[string]bool)
		for _, name := range unresolved {
			if !seen[name] {
				fmt.Fprintf(os.Stderr, "  %s\n", name)
				seen[name] = true
			}
		}
		return nil, fmt.Errorf("%d unresolved calls", len(unresolved))
	}

	flat := BuildFlatBinary(g)
	return flat, nil
}
