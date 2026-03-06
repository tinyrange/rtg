package linux

import (
	"j5.nz/rtg/std/compiler/backend/riscv"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
	objelf "j5.nz/rtg/std/compiler/object/elf"
)

func BuildELF(g *riscv.CodeGen, irmod *ir.IRModule) []byte {
	if g.Target().WordSize == 8 {
		return buildELF64(g, irmod)
	}
	return buildELF32(g, irmod)
}

func buildELF64(g *riscv.CodeGen, irmod *ir.IRModule) []byte {
	elfHeaderSize := 64
	phdrSize := 56
	headerTotal := elfHeaderSize + phdrSize
	textOffset := common.AlignUp(headerTotal, 16)
	textSize := len(g.Code())
	rodataOffset := common.AlignUp(textOffset+textSize, 8)
	rodataSize := len(g.Rodata())
	dataOffset := common.AlignUp(rodataOffset+rodataSize, 8)
	dataSize := len(g.Data())
	loadedSize := dataOffset + dataSize
	textVAddr := g.BaseAddr() + uint64(textOffset)
	rodataVAddr := g.BaseAddr() + uint64(rodataOffset)
	dataVAddr := g.BaseAddr() + uint64(dataOffset)
	g.PatchSectionFixups(textVAddr, rodataVAddr, dataVAddr)
	symtab, strtab := objelf.BuildSymtabAndStrtab64(irmod, textVAddr, textSize, g.FuncOffsets())
	symtabOffset := loadedSize
	strtabOffset := symtabOffset + len(symtab)
	shstrtab, names := objelf.DefaultShStrtab()
	shstrtabOffset := strtabOffset + len(strtab)
	shdrOffset := shstrtabOffset + len(shstrtab)
	shdrEntrySize := 64
	shdrCount := 7
	totalSize := shdrOffset + shdrEntrySize*shdrCount
	if g.Target().StripBinary {
		totalSize = loadedSize
	}
	elf := make([]byte, totalSize)
	elf[0], elf[1], elf[2], elf[3] = 0x7f, 'E', 'L', 'F'
	elf[4], elf[5], elf[6] = 2, 1, 1
	common.PutU16(elf[16:], 2)
	common.PutU16(elf[18:], 243)
	common.PutU32(elf[20:], 1)
	common.PutU64(elf[24:], textVAddr)
	common.PutU64(elf[32:], uint64(elfHeaderSize))
	if !g.Target().StripBinary {
		common.PutU64(elf[40:], uint64(shdrOffset))
	}
	common.PutU32(elf[48:], 0)
	common.PutU16(elf[52:], uint16(elfHeaderSize))
	common.PutU16(elf[54:], uint16(phdrSize))
	common.PutU16(elf[56:], 1)
	common.PutU16(elf[58:], uint16(shdrEntrySize))
	if g.Target().StripBinary {
		common.PutU16(elf[60:], 0)
		common.PutU16(elf[62:], 0)
	} else {
		common.PutU16(elf[60:], uint16(shdrCount))
		common.PutU16(elf[62:], 6)
	}
	phdr := elf[elfHeaderSize:]
	common.PutU32(phdr[0:], 1)
	common.PutU32(phdr[4:], 7)
	common.PutU64(phdr[8:], 0)
	common.PutU64(phdr[16:], g.BaseAddr())
	common.PutU64(phdr[24:], g.BaseAddr())
	common.PutU64(phdr[32:], uint64(loadedSize))
	common.PutU64(phdr[40:], uint64(loadedSize))
	common.PutU64(phdr[48:], 0x1000)
	copy(elf[textOffset:], g.Code())
	copy(elf[rodataOffset:], g.Rodata())
	copy(elf[dataOffset:], g.Data())
	if !g.Target().StripBinary {
		copy(elf[symtabOffset:], symtab)
		copy(elf[strtabOffset:], strtab)
		copy(elf[shstrtabOffset:], shstrtab)
		shdr := elf[shdrOffset:]
		s := shdr[1*shdrEntrySize:]
		common.PutU32(s[0:], uint32(names.Text)); common.PutU32(s[4:], 1); common.PutU64(s[8:], 6); common.PutU64(s[16:], textVAddr); common.PutU64(s[24:], uint64(textOffset)); common.PutU64(s[32:], uint64(textSize)); common.PutU64(s[48:], 16)
		s = shdr[2*shdrEntrySize:]
		common.PutU32(s[0:], uint32(names.Rodata)); common.PutU32(s[4:], 1); common.PutU64(s[8:], 2); common.PutU64(s[16:], rodataVAddr); common.PutU64(s[24:], uint64(rodataOffset)); common.PutU64(s[32:], uint64(rodataSize)); common.PutU64(s[48:], 8)
		s = shdr[3*shdrEntrySize:]
		common.PutU32(s[0:], uint32(names.Data)); common.PutU32(s[4:], 1); common.PutU64(s[8:], 3); common.PutU64(s[16:], dataVAddr); common.PutU64(s[24:], uint64(dataOffset)); common.PutU64(s[32:], uint64(dataSize)); common.PutU64(s[48:], 8)
		s = shdr[4*shdrEntrySize:]
		common.PutU32(s[0:], uint32(names.Symtab)); common.PutU32(s[4:], 2); common.PutU64(s[24:], uint64(symtabOffset)); common.PutU64(s[32:], uint64(len(symtab))); common.PutU32(s[40:], 5); common.PutU32(s[44:], 1); common.PutU64(s[48:], 8); common.PutU64(s[56:], 24)
		s = shdr[5*shdrEntrySize:]
		common.PutU32(s[0:], uint32(names.Strtab)); common.PutU32(s[4:], 3); common.PutU64(s[24:], uint64(strtabOffset)); common.PutU64(s[32:], uint64(len(strtab))); common.PutU64(s[48:], 1)
		s = shdr[6*shdrEntrySize:]
		common.PutU32(s[0:], uint32(names.Shstrtab)); common.PutU32(s[4:], 3); common.PutU64(s[24:], uint64(shstrtabOffset)); common.PutU64(s[32:], uint64(len(shstrtab))); common.PutU64(s[48:], 1)
	}
	return elf
}

func buildELF32(g *riscv.CodeGen, irmod *ir.IRModule) []byte {
	elfHeaderSize := 52
	phdrSize := 32
	headerTotal := elfHeaderSize + phdrSize
	textOffset := common.AlignUp(headerTotal, 16)
	textSize := len(g.Code())
	rodataOffset := common.AlignUp(textOffset+textSize, 4)
	rodataSize := len(g.Rodata())
	dataOffset := common.AlignUp(rodataOffset+rodataSize, 4)
	dataSize := len(g.Data())
	loadedSize := dataOffset + dataSize
	textVAddr := g.BaseAddr() + uint64(textOffset)
	rodataVAddr := g.BaseAddr() + uint64(rodataOffset)
	dataVAddr := g.BaseAddr() + uint64(dataOffset)
	g.PatchSectionFixups(textVAddr, rodataVAddr, dataVAddr)
	symtab, strtab := objelf.BuildSymtabAndStrtab32(irmod, textVAddr, textSize, g.FuncOffsets())
	symtabOffset := loadedSize
	strtabOffset := symtabOffset + len(symtab)
	shstrtab, names := objelf.DefaultShStrtab()
	shstrtabOffset := strtabOffset + len(strtab)
	shdrOffset := shstrtabOffset + len(shstrtab)
	shdrEntrySize := 40
	shdrCount := 7
	totalSize := shdrOffset + shdrEntrySize*shdrCount
	if g.Target().StripBinary { totalSize = loadedSize }
	elf := make([]byte, totalSize)
	elf[0], elf[1], elf[2], elf[3] = 0x7f, 'E', 'L', 'F'
	elf[4], elf[5], elf[6] = 1, 1, 1
	common.PutU16(elf[16:], 2)
	common.PutU16(elf[18:], 243)
	common.PutU32(elf[20:], 1)
	common.PutU32(elf[24:], uint32(textVAddr))
	common.PutU32(elf[28:], uint32(elfHeaderSize))
	if !g.Target().StripBinary { common.PutU32(elf[32:], uint32(shdrOffset)) }
	common.PutU16(elf[40:], uint16(elfHeaderSize))
	common.PutU16(elf[42:], uint16(phdrSize))
	common.PutU16(elf[44:], 1)
	common.PutU16(elf[46:], uint16(shdrEntrySize))
	if g.Target().StripBinary {
		common.PutU16(elf[48:], 0); common.PutU16(elf[50:], 0)
	} else {
		common.PutU16(elf[48:], uint16(shdrCount)); common.PutU16(elf[50:], 6)
	}
	phdr := elf[elfHeaderSize:]
	common.PutU32(phdr[0:], 1); common.PutU32(phdr[4:], 0); common.PutU32(phdr[8:], uint32(g.BaseAddr())); common.PutU32(phdr[12:], uint32(g.BaseAddr())); common.PutU32(phdr[16:], uint32(loadedSize)); common.PutU32(phdr[20:], uint32(loadedSize)); common.PutU32(phdr[24:], 7); common.PutU32(phdr[28:], 0x1000)
	copy(elf[textOffset:], g.Code())
	copy(elf[rodataOffset:], g.Rodata())
	copy(elf[dataOffset:], g.Data())
	if !g.Target().StripBinary {
		copy(elf[symtabOffset:], symtab)
		copy(elf[strtabOffset:], strtab)
		copy(elf[shstrtabOffset:], shstrtab)
		shdr := elf[shdrOffset:]
		s := shdr[1*shdrEntrySize:]
		common.PutU32(s[0:], uint32(names.Text)); common.PutU32(s[4:], 1); common.PutU32(s[8:], 6); common.PutU32(s[12:], uint32(textVAddr)); common.PutU32(s[16:], uint32(textOffset)); common.PutU32(s[20:], uint32(textSize)); common.PutU32(s[32:], 16)
		s = shdr[2*shdrEntrySize:]
		common.PutU32(s[0:], uint32(names.Rodata)); common.PutU32(s[4:], 1); common.PutU32(s[8:], 2); common.PutU32(s[12:], uint32(rodataVAddr)); common.PutU32(s[16:], uint32(rodataOffset)); common.PutU32(s[20:], uint32(rodataSize)); common.PutU32(s[32:], 4)
		s = shdr[3*shdrEntrySize:]
		common.PutU32(s[0:], uint32(names.Data)); common.PutU32(s[4:], 1); common.PutU32(s[8:], 3); common.PutU32(s[12:], uint32(dataVAddr)); common.PutU32(s[16:], uint32(dataOffset)); common.PutU32(s[20:], uint32(dataSize)); common.PutU32(s[32:], 4)
		s = shdr[4*shdrEntrySize:]
		common.PutU32(s[0:], uint32(names.Symtab)); common.PutU32(s[4:], 2); common.PutU32(s[16:], uint32(symtabOffset)); common.PutU32(s[20:], uint32(len(symtab))); common.PutU32(s[24:], 5); common.PutU32(s[28:], 1); common.PutU32(s[32:], 4); common.PutU32(s[36:], 16)
		s = shdr[5*shdrEntrySize:]
		common.PutU32(s[0:], uint32(names.Strtab)); common.PutU32(s[4:], 3); common.PutU32(s[16:], uint32(strtabOffset)); common.PutU32(s[20:], uint32(len(strtab))); common.PutU32(s[32:], 1)
		s = shdr[6*shdrEntrySize:]
		common.PutU32(s[0:], uint32(names.Shstrtab)); common.PutU32(s[4:], 3); common.PutU32(s[16:], uint32(shstrtabOffset)); common.PutU32(s[20:], uint32(len(shstrtab))); common.PutU32(s[32:], 1)
	}
	return elf
}
