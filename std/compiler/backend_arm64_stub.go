//go:build no_backend_arm64

package main

import "fmt"

func generateDarwinArm64(irmod *IRModule, outputPath string) error {
	return fmt.Errorf("arm64 backend disabled (built with no_backend_arm64 tag)")
}

func generateLinuxArm64ELF(irmod *IRModule, outputPath string) error {
	return fmt.Errorf("arm64 backend disabled (built with no_backend_arm64 tag)")
}

func generateWinArm64PE(irmod *IRModule, outputPath string) error {
	return fmt.Errorf("arm64 backend disabled (built with no_backend_arm64 tag)")
}

func (g *CodeGen) emitArm64(inst uint32)                            { panic("arm64 backend disabled") }
func (g *CodeGen) emitAddImm(rd, rn int, imm12 uint32)              { panic("arm64 backend disabled") }
func (g *CodeGen) emitSubImm(rd, rn int, imm12 uint32)              { panic("arm64 backend disabled") }
func (g *CodeGen) emitLdr(rt, rn int, offset int)                   { panic("arm64 backend disabled") }
func (g *CodeGen) emitStr(rt, rn int, offset int)                   { panic("arm64 backend disabled") }
func (g *CodeGen) emitBlr(rn int)                                   { panic("arm64 backend disabled") }
func (g *CodeGen) emitMovRRArm64(rd, rm int)                        { panic("arm64 backend disabled") }
func (g *CodeGen) emitAdrpLdr(rd int, target string, rawOff uint64) { panic("arm64 backend disabled") }
func (g *CodeGen) patchAdrpAddOrLdr(codeOffset int, pcAddr, targetAddr uint64) {
	panic("arm64 backend disabled")
}
func (g *CodeGen) patchAdrpAdd(codeOffset int, pcAddr, targetAddr uint64) {
	panic("arm64 backend disabled")
}
func (g *CodeGen) patchAdrpLdr(codeOffset int, pcAddr, targetAddr uint64) {
	panic("arm64 backend disabled")
}
