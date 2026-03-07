//go:build !no_backend_arm64

package macos

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"j5.nz/rtg/std/compiler/backend/aarch64"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

type arm64FuncSpan struct {
	Name  string
	Start int
	End   int
}

func GenerateDarwinObject(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	g := aarch64.NewCodeGen(target, irmod, 0, 0, true)
	g.CompileModuleFuncs(irmod)
	g.CollectNativeFuncSizes(irmod)
	if g.NeedTostringHelper() {
		g.EmitTostringHelperArm64()
	}
	asm, err := buildDarwinObjectAssembly(g, irmod)
	if err != nil {
		return err
	}
	tmpPath := tempAssemblyPath()
	defer os.RemoveAll(tmpPath)
	if err := os.WriteFile(tmpPath, []byte(asm), 0644); err != nil {
		return fmt.Errorf("write temp assembly: %v", err)
	}
	cmd := exec.Command("cc", "-c", "-arch", "arm64", "-x", "assembler", "-o", outputPath, tmpPath)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("assemble object: %v", err)
	}
	return nil
}

func buildDarwinObjectAssembly(g *aarch64.CodeGen, irmod *ir.IRModule) (string, error) {
	funcs := collectArm64FuncSpans(g, irmod)
	funcByName := make(map[string]arm64FuncSpan)
	funcLabels := make(map[string]string)
	for i, fn := range funcs {
		funcByName[fn.Name] = fn
		if len(fn.Name) > 0 && fn.Name[0] == '$' {
			funcLabels[fn.Name] = fmt.Sprintf("Lrtg_func_%d", i)
		} else {
			funcLabels[fn.Name] = darwinAsmSymbol(fn.Name)
		}
	}
	fixups := make(map[int]aarch64.CallFixup)
	for i := 0; i < g.CallFixupCount(); i++ {
		off, target, value := g.CallFixupAt(i)
		fixups[off] = aarch64.CallFixup{CodeOffset: off, Target: target, Value: value}
	}

	roLabels := make(map[int]string)
	dataLabels := make(map[int]string)
	for _, gl := range irmod.Globals {
		dataLabels[gl.Index*8] = darwinGlobalSymbol(gl.Name)
	}
	for _, fx := range fixups {
		switch fx.Target {
		case "$rodata_header$":
			roLabels[int(fx.Value)] = localSectionLabel("rodata", int(fx.Value))
		case "$data_addr$":
			dataLabels[int(fx.Value)] = localSectionLabel("data", int(fx.Value))
		case "$got_addr$":
			slot := int(fx.Value / 8)
			if slot < 0 || slot >= len(g.GotSymbols()) {
				return "", fmt.Errorf("invalid GOT slot %d in object assembly", slot)
			}
		}
	}

	var out strings.Builder
	out.WriteString(".section __TEXT,__text,regular,pure_instructions\n")
	out.WriteString(".p2align 2\n")
	for _, fn := range funcs {
		sym := funcLabels[fn.Name]
		if !strings.HasPrefix(fn.Name, "$") {
			out.WriteString(".globl ")
			out.WriteString(sym)
			out.WriteByte('\n')
		}
		out.WriteString(sym)
		out.WriteString(":\n")
		off := fn.Start
		for off < fn.End {
			if fx, ok := fixups[off]; ok {
				lineCount, err := emitArm64FixupAsm(&out, g, fx, funcByName, funcLabels, roLabels, dataLabels)
				if err != nil {
					return "", err
				}
				off += lineCount * 4
				continue
			}
			if off+4 > len(g.Code()) {
				return "", fmt.Errorf("truncated instruction stream at %#x", off)
			}
			inst := common.GetU32(g.Code()[off : off+4])
			fmt.Fprintf(&out, ".long 0x%08x\n", inst)
			off += 4
		}
	}

	if len(g.Rodata()) > 0 {
		out.WriteString(".section __TEXT,__const\n")
		out.WriteString(".p2align 3\n")
		emitLabeledBytes(&out, g.Rodata(), roLabels)
	}
	if len(g.Data()) > 0 {
		out.WriteString(".section __DATA,__data\n")
		out.WriteString(".p2align 3\n")
		emitLabeledBytes(&out, g.Data(), dataLabels)
	}
	out.WriteString(".subsections_via_symbols\n")
	return out.String(), nil
}

func collectArm64FuncSpans(g *aarch64.CodeGen, irmod *ir.IRModule) []arm64FuncSpan {
	var names []string
	for name := range g.FuncOffsets() {
		names = append(names, name)
	}
	sortArm64FuncNames(names, g)
	out := make([]arm64FuncSpan, 0, len(names))
	for i, name := range names {
		start, _ := g.LookupFuncOffset(name)
		end := len(g.Code())
		if i+1 < len(names) {
			end, _ = g.LookupFuncOffset(names[i+1])
		}
		out = append(out, arm64FuncSpan{Name: name, Start: start, End: end})
	}
	return out
}

func emitArm64FixupAsm(out *strings.Builder, g *aarch64.CodeGen, fx aarch64.CallFixup, funcs map[string]arm64FuncSpan, funcLabels map[string]string, roLabels map[int]string, dataLabels map[int]string) (int, error) {
	switch fx.Target {
	case "$rodata_header$", "$data_addr$", "$got_addr$":
		if fx.CodeOffset+8 > len(g.Code()) {
			return 0, fmt.Errorf("truncated special fixup at %#x", fx.CodeOffset)
		}
		first := common.GetU32(g.Code()[fx.CodeOffset : fx.CodeOffset+4])
		second := common.GetU32(g.Code()[fx.CodeOffset+4 : fx.CodeOffset+8])
		rd := int(first & 31)
		label := ""
		switch fx.Target {
		case "$rodata_header$":
			label = roLabels[int(fx.Value)]
		case "$data_addr$":
			label = dataLabels[int(fx.Value)]
		case "$got_addr$":
			slot := int(fx.Value / 8)
			label = darwinAsmSymbol(g.GotSymbols()[slot])
		}
		if label == "" {
			return 0, fmt.Errorf("missing fixup label for %s at %#x", fx.Target, fx.CodeOffset)
		}
		fmt.Fprintf(out, "adrp %s, %s%s\n", arm64AsmReg(rd), label, pageSuffixForFixup(fx.Target))
		if isArm64LdrUnsigned64(second) {
			rt := int(second & 31)
			rn := int((second >> 5) & 31)
			fmt.Fprintf(out, "ldr %s, [%s, %s%s]\n", arm64AsmReg(rt), arm64AsmReg(rn), label, pageOffSuffixForFixup(fx.Target))
			return 2, nil
		}
		if isArm64AddImm(second) {
			rd2 := int(second & 31)
			rn := int((second >> 5) & 31)
			fmt.Fprintf(out, "add %s, %s, %s%s\n", arm64AsmReg(rd2), arm64AsmReg(rn), label, pageOffSuffixForFixup(fx.Target))
			return 2, nil
		}
		return 0, fmt.Errorf("unsupported arm64 special fixup instruction pair at %#x", fx.CodeOffset)
	default:
		sym := darwinAsmSymbol(fx.Target)
		if _, ok := funcs[fx.Target]; ok {
			sym = funcLabels[fx.Target]
		}
		fmt.Fprintf(out, "bl %s\n", sym)
		return 1, nil
	}
}

func emitLabeledBytes(out *strings.Builder, data []byte, labels map[int]string) {
	if len(labels) == 0 {
		writeByteRuns(out, data)
		return
	}
	var offsets []int
	for off := range labels {
		if off >= 0 && off <= len(data) {
			offsets = append(offsets, off)
		}
	}
	sortInts(offsets)
	pos := 0
	for _, off := range offsets {
		if off > pos {
			writeByteRuns(out, data[pos:off])
		}
		out.WriteString(labels[off])
		out.WriteString(":\n")
		pos = off
	}
	if pos < len(data) {
		writeByteRuns(out, data[pos:])
	}
}

func writeByteRuns(out *strings.Builder, data []byte) {
	for i := 0; i < len(data); {
		end := i + 16
		if end > len(data) {
			end = len(data)
		}
		out.WriteString(".byte ")
		for j := i; j < end; j++ {
			if j > i {
				out.WriteString(", ")
			}
			fmt.Fprintf(out, "0x%02x", data[j])
		}
		out.WriteByte('\n')
		i = end
	}
}

func darwinGlobalSymbol(name string) string {
	return darwinAsmSymbol(name)
}

func darwinAsmSymbol(name string) string {
	if name == "" {
		return name
	}
	if name[0] == '_' || name[0] == 'L' {
		return name
	}
	return "_" + name
}

func localSectionLabel(section string, off int) string {
	return fmt.Sprintf("Lrtg_%s_%x", section, off)
}

func arm64AsmReg(reg int) string {
	if reg == 31 {
		return "sp"
	}
	return fmt.Sprintf("x%d", reg)
}

func isArm64LdrUnsigned64(inst uint32) bool {
	return inst&0xFFC00000 == 0xF9400000
}

func isArm64AddImm(inst uint32) bool {
	return inst&0xFF000000 == 0x91000000
}

func pageSuffixForFixup(target string) string {
	if target == "$got_addr$" {
		return "@GOTPAGE"
	}
	return "@PAGE"
}

func pageOffSuffixForFixup(target string) string {
	if target == "$got_addr$" {
		return "@GOTPAGEOFF"
	}
	return "@PAGEOFF"
}

func tempAssemblyPath() string {
	return "/tmp/rtg-arm64-object-" + fmt.Sprintf("%d", os.Getpid()) + ".s"
}

func sortArm64FuncNames(names []string, g *aarch64.CodeGen) {
	for i := 1; i < len(names); i++ {
		j := i
		for j > 0 {
			aj, _ := g.LookupFuncOffset(names[j-1])
			bj, _ := g.LookupFuncOffset(names[j])
			if aj < bj || (aj == bj && names[j-1] <= names[j]) {
				break
			}
			names[j-1], names[j] = names[j], names[j-1]
			j--
		}
	}
}

func sortInts(values []int) {
	for i := 1; i < len(values); i++ {
		j := i
		for j > 0 && values[j-1] > values[j] {
			values[j-1], values[j] = values[j], values[j-1]
			j--
		}
	}
}
