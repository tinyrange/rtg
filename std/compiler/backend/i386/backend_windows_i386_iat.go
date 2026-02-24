//go:build !no_backend_i386 && !no_backend_windows_i386

package i386

// emitCallIAT emits `call dword ptr [abs32]` for calling Windows IAT entries.
func (g *CodeGen) emitCallIAT(funcName string) {
	g.emitCallIATInLib(winDefaultImportLibrary, funcName)
}

func (g *CodeGen) emitCallIATInLib(libName string, funcName string) {
	g.flush()
	g.emitBytes(0xFF, 0x15) // call dword ptr [abs32]
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: len(g.code),
		Target:     encodeIATFixupTarget(libName, funcName),
	})
	g.emitU32(0) // placeholder
}

// emitJmpIAT emits `jmp dword ptr [abs32]` for jumping to Windows IAT entries.
func (g *CodeGen) emitJmpIAT(funcName string) {
	g.emitJmpIATInLib(winDefaultImportLibrary, funcName)
}

func (g *CodeGen) emitJmpIATInLib(libName string, funcName string) {
	g.flush()
	g.emitBytes(0xFF, 0x25) // jmp dword ptr [abs32]
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: len(g.code),
		Target:     encodeIATFixupTarget(libName, funcName),
	})
	g.emitU32(0) // placeholder
}
