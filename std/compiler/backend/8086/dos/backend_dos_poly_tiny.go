//go:build !no_backend_dos_i386 && tiny_dos_backend

package dos

func (g *CodeGen) tostringIntrinsic() {
	panic("ICE: Tostring intrinsic disabled in tiny_dos_backend")
}

func (g *CodeGen) emitTostringHelper() {
	panic("ICE: tostring helper disabled in tiny_dos_backend")
}

func (g *CodeGen) compileTostringBody() {
	panic("ICE: tostring body disabled in tiny_dos_backend")
}

func (g *CodeGen) ifaceBox(typeID int) {
	panic("ICE: iface box disabled in tiny_dos_backend")
}

func (g *CodeGen) ifaceCall(methodName string, argCount int) {
	panic("ICE: iface call disabled in tiny_dos_backend")
}
