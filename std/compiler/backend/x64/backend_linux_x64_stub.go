//go:build no_backend_linux_amd64

package x64

func (g *CodeGen) emitStart(irmod *IRModule) {
	panic("linux/amd64 backend disabled")
}

func (g *CodeGen) compileSyscallIntrinsic(paramCount int) {
	panic("linux/amd64 backend disabled")
}

func (g *CodeGen) compilePanic() {
	panic("linux/amd64 backend disabled")
}
