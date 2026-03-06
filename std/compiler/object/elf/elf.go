package elf

import (
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

type SectionNameOffsets struct {
	Text     int
	Rodata   int
	Data     int
	Symtab   int
	Strtab   int
	Shstrtab int
}

func DefaultShStrtab() ([]byte, SectionNameOffsets) {
	return []byte("\x00.text\x00.rodata\x00.data\x00.symtab\x00.strtab\x00.shstrtab\x00"), SectionNameOffsets{
		Text:     1,
		Rodata:   7,
		Data:     15,
		Symtab:   21,
		Strtab:   29,
		Shstrtab: 37,
	}
}

func BuildSymtabAndStrtab64(irmod *ir.IRModule, textVAddr uint64, textSize int, funcOffsets map[string]int) ([]byte, []byte) {
	type symEntry struct {
		nameOff int
		value   uint64
		size    uint64
	}

	var strtab []byte
	strtab = append(strtab, 0)
	startNameOff := len(strtab)
	strtab = append(strtab, []byte("_start")...)
	strtab = append(strtab, 0)
	spans := ir.ComputeNativeFuncSpans(irmod, funcOffsets, textSize)

	var syms []symEntry
	startSize := uint64(textSize)
	if off, ok := ir.FirstNativeFuncOffset(irmod, funcOffsets, textSize); ok {
		startSize = uint64(off)
	}
	syms = append(syms, symEntry{nameOff: startNameOff, value: textVAddr, size: startSize})

	for _, f := range irmod.Funcs {
		span, ok := spans[f.Name]
		if !ok {
			continue
		}
		nameOff := len(strtab)
		strtab = append(strtab, []byte(f.Name)...)
		strtab = append(strtab, 0)
		syms = append(syms, symEntry{nameOff: nameOff, value: textVAddr + uint64(span.Start), size: uint64(span.End - span.Start)})
	}

	symEntrySize := 24
	symtab := make([]byte, (1+len(syms))*symEntrySize)
	for i, sym := range syms {
		off := (i + 1) * symEntrySize
		common.PutU32(symtab[off:], uint32(sym.nameOff))
		symtab[off+4] = 0x12
		symtab[off+5] = 0
		common.PutU16(symtab[off+6:], 1)
		common.PutU64(symtab[off+8:], sym.value)
		common.PutU64(symtab[off+16:], sym.size)
	}

	return symtab, strtab
}

func BuildSymtabAndStrtab32(irmod *ir.IRModule, textVAddr uint64, textSize int, funcOffsets map[string]int) ([]byte, []byte) {
	type symEntry struct {
		nameOff int
		value   uint32
		size    uint32
	}

	var strtab []byte
	strtab = append(strtab, 0)
	startNameOff := len(strtab)
	strtab = append(strtab, []byte("_start")...)
	strtab = append(strtab, 0)
	spans := ir.ComputeNativeFuncSpans(irmod, funcOffsets, textSize)

	var syms []symEntry
	startSize := uint32(textSize)
	if off, ok := ir.FirstNativeFuncOffset(irmod, funcOffsets, textSize); ok {
		startSize = uint32(off)
	}
	syms = append(syms, symEntry{nameOff: startNameOff, value: uint32(textVAddr), size: startSize})

	for _, f := range irmod.Funcs {
		span, ok := spans[f.Name]
		if !ok {
			continue
		}
		nameOff := len(strtab)
		strtab = append(strtab, []byte(f.Name)...)
		strtab = append(strtab, 0)
		syms = append(syms, symEntry{nameOff: nameOff, value: uint32(textVAddr) + uint32(span.Start), size: uint32(span.End - span.Start)})
	}

	symEntrySize := 16
	symtab := make([]byte, (1+len(syms))*symEntrySize)
	for i, sym := range syms {
		off := (i + 1) * symEntrySize
		common.PutU32(symtab[off:], uint32(sym.nameOff))
		common.PutU32(symtab[off+4:], sym.value)
		common.PutU32(symtab[off+8:], sym.size)
		symtab[off+12] = 0x12
		symtab[off+13] = 0
		common.PutU16(symtab[off+14:], 1)
	}

	return symtab, strtab
}
