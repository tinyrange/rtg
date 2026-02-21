//go:build !no_backend_arm64

package aarch64

import (
	"strings"

	"j5.nz/rtg/std/compiler/ir"
)

func (g *CodeGen) loadLinkStaticArgsArm64(paramCount int) {
	if paramCount > 8 {
		panic("ICE: linkstatic intrinsic has too many args for arm64")
	}
	i := 0
	for i < paramCount {
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

func decodeLinkStaticSpec(raw string) (string, string, string, bool) {
	parts := strings.Split(raw, ",")
	if len(parts) != 3 {
		return "", "", "", false
	}
	lib := strings.TrimSpace(parts[0])
	sym := strings.TrimSpace(parts[1])
	mode := strings.TrimSpace(parts[2])
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
	case "noreturn":
		g.ClearOperandCache()
	default:
		panic("ICE: unknown linkstatic mode '" + mode + "'")
	}
	return true
}
