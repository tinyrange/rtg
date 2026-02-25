//go:build no_backend_linux_i386

package i386

func (g *CodeGen) compileSyscallIntrinsic_linux386(paramCount int) {
	_ = paramCount
	panic("linux/386 backend disabled")
}

func (g *CodeGen) compilePanic_linux386() {
	panic("linux/386 backend disabled")
}
