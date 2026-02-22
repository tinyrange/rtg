//go:build !no_backend_dos_i386

package dos

import "j5.nz/rtg/std/compiler/ir"

// BuildTinyCOMFromIR emits a deterministic tiny DOS COM image suitable for
// bootstrapping/tests with hard-coded IR inputs.
//
// The generated program:
// - initializes DI operand stack pointer to 0xFF00
// - calls a tiny main that immediately returns
// - exits via INT 21h AH=4Ch
func BuildTinyCOMFromIR(irmod *ir.IRModule) ([]byte, bool) {
	if irmod == nil {
		return nil, false
	}
	_ = irmod
	return []byte{
		0xBF, 0x00, 0xFF, // mov di, 0xFF00
		0xE8, 0x05, 0x00, // call +5 (to ret at offset 0x000B)
		0xB8, 0x00, 0x4C, // mov ax, 0x4C00
		0xCD, 0x21, // int 21h
		0xC3, // ret
	}, true
}
