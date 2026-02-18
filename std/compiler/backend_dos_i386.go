//go:build !no_backend_dos_i386

package main

import (
	"fmt"
	"os"
)

const (
	comLoadAddr386 = 0x100
	comMaxImage    = 65536 - comLoadAddr386
)

// generateDOSCOM386 compiles an IRModule to a DOS .COM image.
//
// The image is laid out as:
//
//	[code][_rodata][_data]
//
// loaded by DOS at segment:0100h.
func generateDOSCOM386(irmod *IRModule, outputPath string) error {
	g := &CodeGen{
		funcOffsets:   make(map[string]int),
		labelOffsets:  make(map[int]int),
		stringMap:     make(map[string]int),
		globalOffsets: make([]int, len(irmod.Globals)),
		irmod:         irmod,
		wordSize:      2,
	}

	slot := g.slotBytes_i386()
	for i := range irmod.Globals {
		g.globalOffsets[i] = i * slot
	}
	g.data = make([]byte, len(irmod.Globals)*slot)

	g.emitStart_dos386(irmod)

	for _, f := range irmod.Funcs {
		g.funcOffsets[f.Name] = len(g.code)
		g.compileFunc_i386(f)
	}
	collectNativeFuncSizes(irmod, g.funcOffsets, len(g.code))
	if g.needTostringHelper {
		g.emitTostringHelperI386()
	}

	var unresolved []string
	for _, fix := range g.callFixups {
		if fix.Target == "$rodata_header$" || fix.Target == "$data_addr$" {
			continue
		}
		target, ok := g.funcOffsets[fix.Target]
		if !ok {
			unresolved = append(unresolved, fix.Target)
			continue
		}
		g.patchRel32At(fix.CodeOffset, target)
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

	com, err := g.buildCOM386()
	if err != nil {
		return err
	}
	if err := os.WriteFile(outputPath, com, 0755); err != nil {
		return fmt.Errorf("write output: %v", err)
	}
	return nil
}

// emitStart_dos386 emits a minimal COM entry sequence.
func (g *CodeGen) emitStart_dos386(irmod *IRModule) {
	// Operand stack starts near the top of the 64KB segment.
	g.emitMovRegImm32(REG32_EDI, 0xff00)

	for _, f := range irmod.Funcs {
		if isInitFunc(f.Name) {
			g.emitCallPlaceholder(f.Name)
		}
	}
	g.emitCallPlaceholder("main.main")

	// DOS terminate process (INT 21h AH=4Ch, AL=0).
	g.emitMovRegImm32(REG32_EAX, 0x4c00)
	g.emitBytes(0xcd, 0x21)
}

func (g *CodeGen) buildCOM386() ([]byte, error) {
	textSize := len(g.code)
	rodataSize := len(g.rodata)
	dataSize := len(g.data)
	total := textSize + rodataSize + dataSize
	if total > comMaxImage {
		return nil, fmt.Errorf("COM image too large: %d bytes (max %d)", total, comMaxImage)
	}

	rodataAddr := uint32(comLoadAddr386 + textSize)
	dataAddr := uint32(comLoadAddr386 + textSize + rodataSize)

	// Patch string headers to hold near offsets in the loaded COM segment.
	for _, headerOff := range g.stringMap {
		if g.wordSize == 2 {
			dataOff := uint32(uint16(g.rodata[headerOff]) | uint16(g.rodata[headerOff+1])<<8)
			putU16(g.rodata[headerOff:headerOff+2], uint16(rodataAddr+dataOff))
		} else {
			dataOff := getU32(g.rodata[headerOff : headerOff+4])
			putU32(g.rodata[headerOff:headerOff+4], rodataAddr+dataOff)
		}
	}

	// Patch code references to rodata/data with segment-relative offsets.
	for _, fix := range g.callFixups {
		if fix.Target == "$rodata_header$" {
			if g.wordSize == 2 {
				headerOff := uint32(uint16(g.code[fix.CodeOffset]) | uint16(g.code[fix.CodeOffset+1])<<8)
				putU16(g.code[fix.CodeOffset:fix.CodeOffset+2], uint16(rodataAddr+headerOff))
			} else {
				headerOff := getU32(g.code[fix.CodeOffset : fix.CodeOffset+4])
				putU32(g.code[fix.CodeOffset:fix.CodeOffset+4], rodataAddr+headerOff)
			}
		} else if fix.Target == "$data_addr$" {
			if g.wordSize == 2 {
				off := uint32(uint16(g.code[fix.CodeOffset]) | uint16(g.code[fix.CodeOffset+1])<<8)
				putU16(g.code[fix.CodeOffset:fix.CodeOffset+2], uint16(dataAddr+off))
			} else {
				off := getU32(g.code[fix.CodeOffset : fix.CodeOffset+4])
				putU32(g.code[fix.CodeOffset:fix.CodeOffset+4], dataAddr+off)
			}
		}
	}

	com := make([]byte, total)
	copy(com, g.code)
	copy(com[textSize:], g.rodata)
	copy(com[textSize+rodataSize:], g.data)
	return com, nil
}
