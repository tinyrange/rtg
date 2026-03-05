//go:build !no_backend_linux_i386

package linux

import (
	core "j5.nz/rtg/std/compiler/backend/i386"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
	objelf "j5.nz/rtg/std/compiler/object/elf"
)

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
		dataOff := common.GetU32(g.Rodata[headerOff : headerOff+4])
		common.PutU32(g.Rodata[headerOff:headerOff+4], uint32(rodataVAddr)+dataOff)
	}

	for _, fix := range g.CallFixups {
		if fix.Target == "$rodata_header$" {
			headerOff := common.GetU32(g.Code[fix.CodeOffset : fix.CodeOffset+4])
			common.PutU32(g.Code[fix.CodeOffset:fix.CodeOffset+4], uint32(rodataVAddr)+headerOff)
		} else if fix.Target == "$data_addr$" {
			dataOff := common.GetU32(g.Code[fix.CodeOffset : fix.CodeOffset+4])
			common.PutU32(g.Code[fix.CodeOffset:fix.CodeOffset+4], uint32(dataVAddr)+dataOff)
		}
	}

	symtab, strtab := objelf.BuildSymtabAndStrtab32(irmod, textVAddr, textSize, g.FuncOffsets)
	symEntrySize := 16
	symtabSize := len(symtab)
	shstrtab, shNames := objelf.DefaultShStrtab()

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
	common.PutU16(elf[16:], 2)
	common.PutU16(elf[18:], 3)
	common.PutU32(elf[20:], 1)
	common.PutU32(elf[24:], uint32(textVAddr))
	common.PutU32(elf[28:], uint32(elfHeaderSize))
	if g.Target.StripBinary {
		common.PutU32(elf[32:], 0)
	} else {
		common.PutU32(elf[32:], uint32(shdrOffset))
	}
	common.PutU32(elf[36:], 0)
	common.PutU16(elf[40:], uint16(elfHeaderSize))
	common.PutU16(elf[42:], uint16(phdrSize))
	common.PutU16(elf[44:], 1)
	common.PutU16(elf[46:], uint16(shdrEntrySize))
	if g.Target.StripBinary {
		common.PutU16(elf[48:], 0)
		common.PutU16(elf[50:], 0)
	} else {
		common.PutU16(elf[48:], uint16(shdrCount))
		common.PutU16(elf[50:], 6)
	}

	phdr := elf[elfHeaderSize:]
	common.PutU32(phdr[0:], 1)
	common.PutU32(phdr[4:], 0)
	common.PutU32(phdr[8:], uint32(g.BaseAddr))
	common.PutU32(phdr[12:], uint32(g.BaseAddr))
	common.PutU32(phdr[16:], uint32(loadedSize))
	common.PutU32(phdr[20:], uint32(loadedSize))
	common.PutU32(phdr[24:], 7)
	common.PutU32(phdr[28:], 0x1000)

	copy(elf[textOffset:], g.Code)
	copy(elf[rodataOffset:], g.Rodata)
	copy(elf[dataOffset:], g.Data)

	if !g.Target.StripBinary {
		copy(elf[symtabOffset:], symtab)
		copy(elf[strtabOffset:], strtab)
		copy(elf[shstrtabOffset:], shstrtab)

		shdr := elf[shdrOffset:]

		s := shdr[1*shdrEntrySize:]
		common.PutU32(s[0:], uint32(shNames.Text))
		common.PutU32(s[4:], 1)
		common.PutU32(s[8:], 6)
		common.PutU32(s[12:], uint32(textVAddr))
		common.PutU32(s[16:], uint32(textOffset))
		common.PutU32(s[20:], uint32(textSize))
		common.PutU32(s[24:], 0)
		common.PutU32(s[28:], 0)
		common.PutU32(s[32:], 16)
		common.PutU32(s[36:], 0)

		s = shdr[2*shdrEntrySize:]
		common.PutU32(s[0:], uint32(shNames.Rodata))
		common.PutU32(s[4:], 1)
		common.PutU32(s[8:], 2)
		common.PutU32(s[12:], uint32(rodataVAddr))
		common.PutU32(s[16:], uint32(rodataOffset))
		common.PutU32(s[20:], uint32(rodataSize))
		common.PutU32(s[32:], 4)

		s = shdr[3*shdrEntrySize:]
		common.PutU32(s[0:], uint32(shNames.Data))
		common.PutU32(s[4:], 1)
		common.PutU32(s[8:], 3)
		common.PutU32(s[12:], uint32(dataVAddr))
		common.PutU32(s[16:], uint32(dataOffset))
		common.PutU32(s[20:], uint32(dataSize))
		common.PutU32(s[32:], 4)

		s = shdr[4*shdrEntrySize:]
		common.PutU32(s[0:], uint32(shNames.Symtab))
		common.PutU32(s[4:], 2)
		common.PutU32(s[8:], 0)
		common.PutU32(s[12:], 0)
		common.PutU32(s[16:], uint32(symtabOffset))
		common.PutU32(s[20:], uint32(symtabSize))
		common.PutU32(s[24:], 5)
		common.PutU32(s[28:], 1)
		common.PutU32(s[32:], 4)
		common.PutU32(s[36:], uint32(symEntrySize))

		s = shdr[5*shdrEntrySize:]
		common.PutU32(s[0:], uint32(shNames.Strtab))
		common.PutU32(s[4:], 3)
		common.PutU32(s[8:], 0)
		common.PutU32(s[12:], 0)
		common.PutU32(s[16:], uint32(strtabOffset))
		common.PutU32(s[20:], uint32(len(strtab)))
		common.PutU32(s[32:], 1)

		s = shdr[6*shdrEntrySize:]
		common.PutU32(s[0:], uint32(shNames.Shstrtab))
		common.PutU32(s[4:], 3)
		common.PutU32(s[8:], 0)
		common.PutU32(s[12:], 0)
		common.PutU32(s[16:], uint32(shstrtabOffset))
		common.PutU32(s[20:], uint32(len(shstrtab)))
		common.PutU32(s[32:], 1)
	}

	return elf
}
