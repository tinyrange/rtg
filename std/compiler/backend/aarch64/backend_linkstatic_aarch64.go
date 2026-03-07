//go:build !no_backend_arm64

package aarch64

import (
	"strings"

	"j5.nz/rtg/std/compiler/ir"
)

func (g *CodeGen) loadLinkStaticArgsArm64(paramCount int) {
	maxRegs := paramCount
	if maxRegs > 8 {
		maxRegs = 8
	}
	i := 0
	for i < maxRegs {
		g.emitLoadLocalArm64((i+1)*8, REG_X0+i)
		i++
	}
}

func (g *CodeGen) emitRawPtrReturnArm64() {
	g.rawPush(REG_X0)
	g.EmitMovZ(REG_X1, 0, 0)
	g.rawPush(REG_X1)
	g.rawPush(REG_X1)
	g.ClearOperandCache()
}

func (g *CodeGen) emitRawReturnArm64() {
	g.rawPush(REG_X0)
	g.ClearOperandCache()
}

func (g *CodeGen) emitVoidReturnArm64() {
	g.ClearOperandCache()
}

func (g *CodeGen) emitDataPtrReturnArm64(sym string) {
	g.EmitLoadGOT(sym, REG_X0)
	g.rawPush(REG_X0)
	g.ClearOperandCache()
}

func (g *CodeGen) emitDataReturnArm64(sym string) {
	g.EmitLoadGOT(sym, REG_X0)
	g.emitLdr(REG_X0, REG_X0, 0)
	g.rawPush(REG_X0)
	g.ClearOperandCache()
}

func (g *CodeGen) emitVariadicLinkStaticCallArm64(paramCount int, fixedCount int, sym string) {
	if fixedCount < 0 || fixedCount > paramCount {
		panic("ICE: invalid darwin variadic linkstatic signature")
	}
	maxRegs := fixedCount
	if maxRegs > 8 {
		maxRegs = 8
	}
	i := 0
	for i < maxRegs {
		g.emitLoadLocalArm64((i+1)*8, REG_X0+i)
		i++
	}
	stackArgs := paramCount - fixedCount
	frame := stackArgs * 8
	if frame%16 != 0 {
		frame += 8
	}
	if frame > 0 {
		g.emitSubImm(REG_SP, REG_SP, uint32(frame))
		i = 0
		for i < stackArgs {
			g.emitLoadLocalArm64((fixedCount+i+1)*8, REG_X16)
			g.EmitStr(REG_X16, REG_SP, i*8)
			i++
		}
	}
	g.EmitCallGOT(sym)
	if frame > 0 {
		g.emitAddImm(REG_SP, REG_SP, uint32(frame))
	}
}

func parseVariadicLinkStaticMode(mode string) (string, int, bool) {
	var prefix string
	switch {
	case strings.HasPrefix(mode, "rawvar"):
		prefix = "rawvar"
	case strings.HasPrefix(mode, "voidvar"):
		prefix = "voidvar"
	default:
		return "", 0, false
	}
	digits := mode[len(prefix):]
	if digits == "" {
		return "", 0, false
	}
	n := 0
	for i := 0; i < len(digits); i++ {
		ch := digits[i]
		if ch < '0' || ch > '9' {
			return "", 0, false
		}
		n = n*10 + int(ch-'0')
	}
	return prefix, n, true
}

func decodeLinkStaticSpec(raw string) (string, string, string, bool) {
	c1 := -1
	i := 0
	for i < len(raw) {
		if raw[i] == ',' {
			c1 = i
			break
		}
		i++
	}
	if c1 < 0 {
		return "", "", "", false
	}
	c2 := -1
	i = c1 + 1
	for i < len(raw) {
		if raw[i] == ',' {
			c2 = i
			break
		}
		i++
	}
	if c2 < 0 {
		return "", "", "", false
	}
	// Reject unexpected extra separators to keep metadata strict.
	i = c2 + 1
	for i < len(raw) {
		if raw[i] == ',' {
			return "", "", "", false
		}
		i++
	}
	lib := strings.TrimSpace(raw[0:c1])
	sym := strings.TrimSpace(raw[c1+1 : c2])
	mode := strings.TrimSpace(raw[c2+1:])
	if lib == "" || sym == "" {
		return "", "", "", false
	}
	return lib, sym, mode, true
}

func (g *CodeGen) compileLinkStaticIntrinsicArm64(inst ir.Inst) bool {
	if g.target.GOOS != "darwin" || g.irmod == nil || g.irmod.LinkStaticFuncs == nil {
		return false
	}
	raw, ok := g.irmod.LinkStaticFuncs[inst.Name]
	if !ok {
		return false
	}
	lib, sym, mode, ok := decodeLinkStaticSpec(raw)
	if !ok {
		panic("ICE: invalid linkstatic metadata for '" + inst.Name + "'")
	}
	if lib != "libSystem.dylib" {
		panic("ICE: unsupported darwin linkstatic library '" + lib + "'")
	}
	switch mode {
	case "data":
		g.emitDataReturnArm64(sym)
		return true
	case "dataptr":
		g.emitDataPtrReturnArm64(sym)
		return true
	}
	if vmode, fixedCount, ok := parseVariadicLinkStaticMode(mode); ok {
		g.emitVariadicLinkStaticCallArm64(inst.Arg, fixedCount, sym)
		switch vmode {
		case "rawvar":
			g.emitRawReturnArm64()
		case "voidvar":
			g.emitVoidReturnArm64()
		default:
			panic("ICE: unknown darwin variadic linkstatic mode '" + vmode + "'")
		}
		return true
	}
	g.loadLinkStaticArgsArm64(inst.Arg)
	g.EmitCallGOT(sym)

	if mode == "" {
		mode = "syscall"
	}
	switch mode {
	case "syscall":
		g.emitSyscallReturnArm64()
	case "ptr":
		g.emitSyscallReturnPtrArm64()
	case "rawptr":
		g.emitRawPtrReturnArm64()
	case "raw":
		g.emitRawReturnArm64()
	case "void":
		g.emitVoidReturnArm64()
	case "noreturn":
		g.ClearOperandCache()
	default:
		panic("ICE: unknown linkstatic mode '" + mode + "'")
	}
	return true
}
