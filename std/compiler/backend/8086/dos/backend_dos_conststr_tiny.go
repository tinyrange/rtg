//go:build !no_backend_dos_i386 && tiny_dos_backend

package dos

func (g *CodeGen) compileConstStr(raw string) {
	decoded := decodeTinyStringLiteral(raw)

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

func decodeTinyStringLiteral(raw string) string {
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		raw = raw[1 : len(raw)-1]
	}
	out := make([]byte, 0, len(raw))
	i := 0
	for i < len(raw) {
		c := raw[i]
		if c != '\\' || i+1 >= len(raw) {
			out = append(out, c)
			i++
			continue
		}
		n := raw[i+1]
		switch n {
		case 'n':
			out = append(out, '\n')
			i += 2
		case 'r':
			out = append(out, '\r')
			i += 2
		case 't':
			out = append(out, '\t')
			i += 2
		case '\\':
			out = append(out, '\\')
			i += 2
		case '"':
			out = append(out, '"')
			i += 2
		case 'x':
			if i+3 < len(raw) {
				h1 := unhex(raw[i+2])
				h2 := unhex(raw[i+3])
				if h1 >= 0 && h2 >= 0 {
					out = append(out, byte((h1<<4)|h2))
					i += 4
					continue
				}
			}
			out = append(out, '\\', 'x')
			i += 2
		default:
			out = append(out, n)
			i += 2
		}
	}
	return string(out)
}

func unhex(c byte) int {
	if c >= '0' && c <= '9' {
		return int(c - '0')
	}
	if c >= 'a' && c <= 'f' {
		return int(c-'a') + 10
	}
	if c >= 'A' && c <= 'F' {
		return int(c-'A') + 10
	}
	return -1
}
