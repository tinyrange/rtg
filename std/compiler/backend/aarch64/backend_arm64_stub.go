//go:build no_backend_arm64

package aarch64

import (
	"fmt"

	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

func GenerateDarwin(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	return fmt.Errorf("arm64 backend disabled (built with no_backend_arm64 tag)")
}

func GenerateLinuxELF(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	return fmt.Errorf("arm64 backend disabled (built with no_backend_arm64 tag)")
}

func GenerateWinPE(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	return fmt.Errorf("arm64 backend disabled (built with no_backend_arm64 tag)")
}

func generateDarwinArm64(irmod *IRModule, outputPath string) error {
	return fmt.Errorf("arm64 backend disabled (built with no_backend_arm64 tag)")
}

func generateLinuxArm64ELF(irmod *IRModule, outputPath string) error {
	return fmt.Errorf("arm64 backend disabled (built with no_backend_arm64 tag)")
}

func generateWinArm64PE(irmod *IRModule, outputPath string) error {
	return fmt.Errorf("arm64 backend disabled (built with no_backend_arm64 tag)")
}

func (g *CodeGen) EmitArm64(inst uint32)                            { panic("arm64 backend disabled") }
func (g *CodeGen) emitAddImm(rd, rn int, imm12 uint32)              { panic("arm64 backend disabled") }
func (g *CodeGen) emitSubImm(rd, rn int, imm12 uint32)              { panic("arm64 backend disabled") }
func (g *CodeGen) emitLdr(rt, rn int, offset int)                   { panic("arm64 backend disabled") }
func (g *CodeGen) EmitStr(rt, rn int, offset int)                   { panic("arm64 backend disabled") }
//rtg:profile
func (g *CodeGen) EmitBlr(rn int)                                   { panic("arm64 backend disabled") }
//rtg:profile
func (g *CodeGen) EmitMovRRArm64(rd, rm int)                        { panic("arm64 backend disabled") }
//rtg:profile
func (g *CodeGen) emitAdrpLdr(rd int, target string, rawOff uint64) { panic("arm64 backend disabled") }
//rtg:profile
func (g *CodeGen) PatchAdrpAddOrLdr(codeOffset int, pcAddr, targetAddr uint64) {
	panic("arm64 backend disabled")
}
//rtg:profile
func (g *CodeGen) PatchAdrpAdd(codeOffset int, pcAddr, targetAddr uint64) {
	panic("arm64 backend disabled")
}
//rtg:profile
func (g *CodeGen) PatchAdrpLdr(codeOffset int, pcAddr, targetAddr uint64) {
	panic("arm64 backend disabled")
}
