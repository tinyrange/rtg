//go:build no_backend_dos_i386

package i386

import "fmt"

func generateDOSCOM386(irmod *IRModule, outputPath string) error {
	return fmt.Errorf("dos/8086 backend disabled (built with no_backend_dos_i386 tag)")
}

func (g *CodeGen) compileSyscallIntrinsic_dos386(paramCount int) {
	panic("dos/8086 backend disabled")
}
