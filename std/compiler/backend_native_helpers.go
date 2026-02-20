package main

import (
	"fmt"
	"os"
)

func newNativeCodeGen(irmod *IRModule, wordSize int, baseAddr uint64, isArm64 bool) *CodeGen {
	return &CodeGen{
		funcOffsets:   make(map[string]int),
		labelOffsets:  make(map[int]int),
		stringMap:     make(map[string]int),
		globalOffsets: make([]int, len(irmod.Globals)),
		baseAddr:      baseAddr,
		irmod:         irmod,
		wordSize:      wordSize,
		isArm64:       isArm64,
	}
}

func initNativeGlobalsData(g *CodeGen, globals int, slotSize int) {
	for i := 0; i < globals; i++ {
		g.globalOffsets[i] = i * slotSize
	}
	g.data = make([]byte, globals*slotSize)
}

func skipNativeCallFixupTarget(target string, allowIAT bool) bool {
	if target == "$rodata_header$" || target == "$data_addr$" {
		return true
	}
	if allowIAT && len(target) > 5 && target[0:5] == "$iat$" {
		return true
	}
	return false
}

const (
	nativeCompileModeX64 = iota
	nativeCompileModeI386
	nativeCompileModeArm64
)

func compileNativeModuleFuncs(g *CodeGen, irmod *IRModule, mode int) {
	for _, f := range irmod.Funcs {
		g.funcOffsets[f.Name] = len(g.code)
		switch mode {
		case nativeCompileModeX64:
			g.compileFunc(f)
		case nativeCompileModeI386:
			g.compileFunc_i386(f)
		case nativeCompileModeArm64:
			g.compileFuncArm64(f)
		default:
			panic("unsupported native compile mode")
		}
	}

	collectNativeFuncSizes(irmod, g.funcOffsets, len(g.code))
	if g.needTostringHelper {
		switch mode {
		case nativeCompileModeX64:
			g.emitTostringHelperX64()
		case nativeCompileModeI386:
			g.emitTostringHelperI386()
		case nativeCompileModeArm64:
			g.emitTostringHelperArm64()
		}
	}
}

func resolveNativeCallFixups(g *CodeGen, allowIAT bool, arm64 bool) error {
	var unresolved []string
	for _, fix := range g.callFixups {
		if skipNativeCallFixupTarget(fix.Target, allowIAT) {
			continue
		}
		target, ok := g.funcOffsets[fix.Target]
		if !ok {
			unresolved = append(unresolved, fix.Target)
			continue
		}
		if arm64 {
			g.patchArm64BAt(fix.CodeOffset, target)
		} else {
			g.patchRel32At(fix.CodeOffset, target)
		}
	}
	if len(unresolved) > 0 {
		fmt.Fprintf(os.Stderr, "error: %d unresolved calls:\n", len(unresolved))
		seen := make(map[string]bool)
		for _, name := range unresolved {
			if !seen[name] {
				fmt.Fprintf(os.Stderr, "  %s\n", name)
				seen[name] = true
			}
		}
		return fmt.Errorf("%d unresolved calls", len(unresolved))
	}
	return nil
}
