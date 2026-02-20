//go:build no_backend_linux_amd64 && no_backend_arm64

package main

func (g *CodeGen) buildELF64(irmod *IRModule) []byte {
	panic("linux/amd64 backend disabled")
}
