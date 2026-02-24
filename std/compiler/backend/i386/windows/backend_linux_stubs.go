//go:build !no_backend_i386 && !no_backend_windows_i386

package windows

func (g *CodeGen) compileSyscallIntrinsic_linux386(paramCount int) {
	panic("ICE: linux/386 syscall path in windows/386 backend")
}

func (g *CodeGen) compilePanic_linux386() {
	panic("ICE: linux/386 panic path in windows/386 backend")
}
