//go:build no_backend_linux_i386

package i386

import "fmt"

func generateI386ELF(irmod *IRModule, outputPath string) error {
	return fmt.Errorf("i386 backend disabled (built with no_backend_linux_i386 tag)")
}

func (g *CodeGen) emitStart_i386(irmod *IRModule) {
	panic("linux/i386 backend disabled")
}

func (g *CodeGen) compileSyscallIntrinsic_linux386(paramCount int) {
	panic("linux/i386 backend disabled")
}

func (g *CodeGen) compilePanic_linux386() {
	panic("linux/i386 backend disabled")
}

func (g *CodeGen) buildELF32(irmod *IRModule) []byte {
	panic("linux/i386 backend disabled")
}
