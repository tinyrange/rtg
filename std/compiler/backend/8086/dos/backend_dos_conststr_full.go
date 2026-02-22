//go:build !no_backend_dos_i386 && !tiny_dos_backend

package dos

import "j5.nz/rtg/std/compiler/backend/becommon"

func (g *CodeGen) compileConstStr(raw string) {
	decoded := becommon.DecodeStringLiteral(raw)

	headerOff, ok := g.stringMap[decoded]
	if !ok {
		dataOff := len(g.rodata)
		g.rodata = append(g.rodata, []byte(decoded)...)

		headerOff = len(g.rodata)
		g.rodata = append(g.rodata, 0, 0, byte(len(decoded)), byte(len(decoded)>>8))

		g.stringMap[decoded] = headerOff
		putU16(g.rodata[headerOff:headerOff+2], uint16(dataOff))
	}

	g.emitMovImm16(REG16_AX, uint16(headerOff))
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: len(g.code) - 2,
		Target:     "$rodata_header$",
	})
	g.opPush(REG16_AX)
}
