//go:build !no_backend_linux_i386

package linux

import (
	core "j5.nz/rtg/std/compiler/backend/i386"
	"j5.nz/rtg/std/compiler/ir"
)

type symEntry struct {
	nameOff int
	value   uint64
	size    uint64
}

// BuildELF32 assembles an ELF32 executable from the generated i386 module.
func BuildELF32(g *core.CodeGen, irmod *ir.IRModule) []byte {
	elfHeaderSize := 52
	phdrSize := 32
	headerTotal := elfHeaderSize + phdrSize
	textOffset := (headerTotal + 15) & ^15

	textSize := len(g.Code)
	rodataOffset := textOffset + textSize
	rodataSize := len(g.Rodata)
	dataOffset := rodataOffset + rodataSize
	dataSize := len(g.Data)
	loadedSize := dataOffset + dataSize

	textVAddr := g.BaseAddr + uint64(textOffset)
	rodataVAddr := g.BaseAddr + uint64(rodataOffset)
	dataVAddr := g.BaseAddr + uint64(dataOffset)

	for _, headerOff := range g.StringMap {
		dataOff := getU32(g.Rodata[headerOff : headerOff+4])
		putU32(g.Rodata[headerOff:headerOff+4], uint32(rodataVAddr)+dataOff)
	}

	for _, fix := range g.CallFixups {
		if fix.Target == "$rodata_header$" {
			headerOff := getU32(g.Code[fix.CodeOffset : fix.CodeOffset+4])
			putU32(g.Code[fix.CodeOffset:fix.CodeOffset+4], uint32(rodataVAddr)+headerOff)
		} else if fix.Target == "$data_addr$" {
			dataOff := getU32(g.Code[fix.CodeOffset : fix.CodeOffset+4])
			putU32(g.Code[fix.CodeOffset:fix.CodeOffset+4], uint32(dataVAddr)+dataOff)
		}
	}

	var strtab []byte
	strtab = append(strtab, 0)
	startNameOff := len(strtab)
	strtab = append(strtab, []byte("_start")...)
	strtab = append(strtab, 0)

	var syms []symEntry
	startSize := uint64(0)
	if len(irmod.Funcs) > 0 {
		startSize = uint64(g.FuncOffsets[irmod.Funcs[0].Name])
	} else {
		startSize = uint64(textSize)
	}
	syms = append(syms, symEntry{startNameOff, textVAddr, startSize})

	for i, f := range irmod.Funcs {
		nameOff := len(strtab)
		strtab = append(strtab, []byte(f.Name)...)
		strtab = append(strtab, 0)

		funcStart := g.FuncOffsets[f.Name]
		var funcSize int
		if i+1 < len(irmod.Funcs) {
			funcSize = g.FuncOffsets[irmod.Funcs[i+1].Name] - funcStart
		} else {
			funcSize = textSize - funcStart
		}
		syms = append(syms, symEntry{nameOff, textVAddr + uint64(funcStart), uint64(funcSize)})
	}

	symEntrySize := 16
	symtabSize := (1 + len(syms)) * symEntrySize
	symtab := make([]byte, symtabSize)
	for i, sym := range syms {
		off := (i + 1) * symEntrySize
		putU32(symtab[off:], uint32(sym.nameOff))
		putU32(symtab[off+4:], uint32(sym.value))
		putU32(symtab[off+8:], uint32(sym.size))
		symtab[off+12] = 0x12
		symtab[off+13] = 0
		putU16(symtab[off+14:], 1)
	}

	shstrtab := []byte("\x00.text\x00.rodata\x00.data\x00.symtab\x00.strtab\x00.shstrtab\x00")
	shNameText := 1
	shNameRodata := 7
	shNameData := 15
	shNameSymtab := 21
	shNameStrtab := 29
	shNameShstrtab := 37

	symtabOffset := loadedSize
	strtabOffset := symtabOffset + symtabSize
	shstrtabOffset := strtabOffset + len(strtab)
	shdrOffset := shstrtabOffset + len(shstrtab)

	shdrEntrySize := 40
	shdrCount := 7
	shdrTableSize := shdrCount * shdrEntrySize

	totalSize := shdrOffset + shdrTableSize
	if g.Target.StripBinary {
		totalSize = loadedSize
	}

	elf := make([]byte, totalSize)
	elf[0] = 0x7f
	elf[1] = 'E'
	elf[2] = 'L'
	elf[3] = 'F'
	elf[4] = 1
	elf[5] = 1
	elf[6] = 1
	elf[7] = 0
	putU16(elf[16:], 2)
	putU16(elf[18:], 3)
	putU32(elf[20:], 1)
	putU32(elf[24:], uint32(textVAddr))
	putU32(elf[28:], uint32(elfHeaderSize))
	if g.Target.StripBinary {
		putU32(elf[32:], 0)
	} else {
		putU32(elf[32:], uint32(shdrOffset))
	}
	putU32(elf[36:], 0)
	putU16(elf[40:], uint16(elfHeaderSize))
	putU16(elf[42:], uint16(phdrSize))
	putU16(elf[44:], 1)
	putU16(elf[46:], uint16(shdrEntrySize))
	if g.Target.StripBinary {
		putU16(elf[48:], 0)
		putU16(elf[50:], 0)
	} else {
		putU16(elf[48:], uint16(shdrCount))
		putU16(elf[50:], 6)
	}

	phdr := elf[elfHeaderSize:]
	putU32(phdr[0:], 1)
	putU32(phdr[4:], 0)
	putU32(phdr[8:], uint32(g.BaseAddr))
	putU32(phdr[12:], uint32(g.BaseAddr))
	putU32(phdr[16:], uint32(loadedSize))
	putU32(phdr[20:], uint32(loadedSize))
	putU32(phdr[24:], 7)
	putU32(phdr[28:], 0x1000)

	copy(elf[textOffset:], g.Code)
	copy(elf[rodataOffset:], g.Rodata)
	copy(elf[dataOffset:], g.Data)

	if !g.Target.StripBinary {
		copy(elf[symtabOffset:], symtab)
		copy(elf[strtabOffset:], strtab)
		copy(elf[shstrtabOffset:], shstrtab)

		shdr := elf[shdrOffset:]

		s := shdr[1*shdrEntrySize:]
		putU32(s[0:], uint32(shNameText))
		putU32(s[4:], 1)
		putU32(s[8:], 6)
		putU32(s[12:], uint32(textVAddr))
		putU32(s[16:], uint32(textOffset))
		putU32(s[20:], uint32(textSize))
		putU32(s[24:], 0)
		putU32(s[28:], 0)
		putU32(s[32:], 16)
		putU32(s[36:], 0)

		s = shdr[2*shdrEntrySize:]
		putU32(s[0:], uint32(shNameRodata))
		putU32(s[4:], 1)
		putU32(s[8:], 2)
		putU32(s[12:], uint32(rodataVAddr))
		putU32(s[16:], uint32(rodataOffset))
		putU32(s[20:], uint32(rodataSize))
		putU32(s[32:], 4)

		s = shdr[3*shdrEntrySize:]
		putU32(s[0:], uint32(shNameData))
		putU32(s[4:], 1)
		putU32(s[8:], 3)
		putU32(s[12:], uint32(dataVAddr))
		putU32(s[16:], uint32(dataOffset))
		putU32(s[20:], uint32(dataSize))
		putU32(s[32:], 4)

		s = shdr[4*shdrEntrySize:]
		putU32(s[0:], uint32(shNameSymtab))
		putU32(s[4:], 2)
		putU32(s[8:], 0)
		putU32(s[12:], 0)
		putU32(s[16:], uint32(symtabOffset))
		putU32(s[20:], uint32(symtabSize))
		putU32(s[24:], 5)
		putU32(s[28:], 1)
		putU32(s[32:], 4)
		putU32(s[36:], uint32(symEntrySize))

		s = shdr[5*shdrEntrySize:]
		putU32(s[0:], uint32(shNameStrtab))
		putU32(s[4:], 3)
		putU32(s[8:], 0)
		putU32(s[12:], 0)
		putU32(s[16:], uint32(strtabOffset))
		putU32(s[20:], uint32(len(strtab)))
		putU32(s[32:], 1)

		s = shdr[6*shdrEntrySize:]
		putU32(s[0:], uint32(shNameShstrtab))
		putU32(s[4:], 3)
		putU32(s[8:], 0)
		putU32(s[12:], 0)
		putU32(s[16:], uint32(shstrtabOffset))
		putU32(s[20:], uint32(len(shstrtab)))
		putU32(s[32:], 1)
	}

	return elf
}

func putU16(b []byte, v uint16) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
}

func putU32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}

func getU32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}
