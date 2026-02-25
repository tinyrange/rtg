package elf

type Symbol struct {
	Name string
	Addr uint32
	Size uint32
	Info uint8
	Local bool
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

func alignUp(v int, a int) int {
	return (v + (a - 1)) & ^(a - 1)
}

func putSym32(b []byte, nameOff uint32, value uint32, size uint32, info uint8, other uint8, shndx uint16) {
	putU32(b[0:], nameOff)
	putU32(b[4:], value)
	putU32(b[8:], size)
	b[12] = info
	b[13] = other
	putU16(b[14:], shndx)
}

func putShdr32(b []byte, name uint32, typ uint32, flags uint32, addr uint32, off uint32, size uint32, link uint32, info uint32, addralign uint32, entsize uint32) {
	putU32(b[0:], name)
	putU32(b[4:], typ)
	putU32(b[8:], flags)
	putU32(b[12:], addr)
	putU32(b[16:], off)
	putU32(b[20:], size)
	putU32(b[24:], link)
	putU32(b[28:], info)
	putU32(b[32:], addralign)
	putU32(b[36:], entsize)
}

// BuildELF32Bringup builds a simple ARM ELF32 executable whose payload starts
// at the MPS2-AN505 flash base with a minimal vector table.
func BuildELF32Bringup(code []byte, initialSP uint32) []byte {
	return BuildELF32BringupWithSymbols(code, initialSP, nil)
}

func BuildELF32BringupWithSymbols(code []byte, initialSP uint32, symbols []Symbol) []byte {
	const (
		elfHeaderSize  = 52
		programHdrSize = 32
		payloadAlign   = 0x100
		vectorTableLen = 0x100
		flashBase      = uint32(0x10000000)
		shdrSize       = 40
		symEntSize     = 16
	)
	const (
		shtNull    = 0
		shtProgBit = 1
		shtSymTab  = 2
		shtStrTab  = 3
	)
	const (
		shfWrite     = 0x1
		shfAlloc     = 0x2
		shfExecInstr = 0x4
	)

	payloadOff := alignUp(elfHeaderSize+programHdrSize, payloadAlign)

	payload := make([]byte, vectorTableLen+len(code))
	resetAddr := flashBase + uint32(vectorTableLen)
	putU32(payload[0:], initialSP)
	putU32(payload[4:], resetAddr|1)
	copy(payload[vectorTableLen:], code)

	textOff := payloadOff + vectorTableLen
	textAddr := resetAddr

	strtab := make([]byte, 1) // first byte must be NUL
	symtab := make([]byte, symEntSize)
	localCount := 0
	for _, s := range symbols {
		if !s.Local {
			continue
		}
		nameOff := uint32(len(strtab))
		strtab = append(strtab, []byte(s.Name)...)
		strtab = append(strtab, 0)
		info := (s.Info & 0x0F) // STB_LOCAL
		ent := make([]byte, symEntSize)
		putSym32(ent, nameOff, s.Addr, s.Size, info, 0, 1)
		symtab = append(symtab, ent...)
		localCount = localCount + 1
	}
	for _, s := range symbols {
		if s.Local {
			continue
		}
		nameOff := uint32(len(strtab))
		strtab = append(strtab, []byte(s.Name)...)
		strtab = append(strtab, 0)
		info := uint8(0x10) | (s.Info & 0x0F) // STB_GLOBAL
		ent := make([]byte, symEntSize)
		putSym32(ent, nameOff, s.Addr, s.Size, info, 0, 1)
		symtab = append(symtab, ent...)
	}

	shstrtab := []byte{
		0,
		'.', 't', 'e', 'x', 't', 0,
		'.', 's', 'y', 'm', 't', 'a', 'b', 0,
		'.', 's', 't', 'r', 't', 'a', 'b', 0,
		'.', 's', 'h', 's', 't', 'r', 't', 'a', 'b', 0,
	}
	shNameText := uint32(1)
	shNameSymtab := uint32(7)
	shNameStrtab := uint32(15)
	shNameShstrtab := uint32(23)

	symtabOff := alignUp(payloadOff+len(payload), 4)
	strtabOff := symtabOff + len(symtab)
	shstrtabOff := strtabOff + len(strtab)
	shoff := alignUp(shstrtabOff+len(shstrtab), 4)
	shnum := 5
	totalSize := shoff + shnum*shdrSize

	bin := make([]byte, totalSize)

	// ELF ident.
	bin[0] = 0x7f
	bin[1] = 'E'
	bin[2] = 'L'
	bin[3] = 'F'
	bin[4] = 1 // ELFCLASS32
	bin[5] = 1 // ELFDATA2LSB
	bin[6] = 1 // EV_CURRENT
	bin[7] = 0 // ELFOSABI_NONE

	putU16(bin[16:], 2)           // ET_EXEC
	putU16(bin[18:], 40)          // EM_ARM
	putU32(bin[20:], 1)           // EV_CURRENT
	putU32(bin[24:], resetAddr|1) // e_entry
	putU32(bin[28:], elfHeaderSize)
	putU32(bin[32:], uint32(shoff))
	putU32(bin[36:], 0x05000000) // EF_ARM_EABI_VER5
	putU16(bin[40:], elfHeaderSize)
	putU16(bin[42:], programHdrSize)
	putU16(bin[44:], 1) // one PT_LOAD
	putU16(bin[46:], shdrSize)
	putU16(bin[48:], uint16(shnum))
	putU16(bin[50:], 4) // .shstrtab

	ph := bin[elfHeaderSize : elfHeaderSize+programHdrSize]
	putU32(ph[0:], 1)                     // PT_LOAD
	putU32(ph[4:], uint32(payloadOff))    // p_offset
	putU32(ph[8:], flashBase)             // p_vaddr
	putU32(ph[12:], flashBase)            // p_paddr
	putU32(ph[16:], uint32(len(payload))) // p_filesz
	putU32(ph[20:], uint32(len(payload))) // p_memsz
	putU32(ph[24:], 5)                    // PF_R|PF_X
	putU32(ph[28:], 0x1000)               // p_align

	copy(bin[payloadOff:], payload)
	copy(bin[symtabOff:], symtab)
	copy(bin[strtabOff:], strtab)
	copy(bin[shstrtabOff:], shstrtab)

	sh := bin[shoff:]
	// [0] SHT_NULL
	putShdr32(sh[0*shdrSize:], 0, shtNull, 0, 0, 0, 0, 0, 0, 0, 0)
	// [1] .text
	putShdr32(sh[1*shdrSize:], shNameText, shtProgBit, shfAlloc|shfExecInstr, textAddr, uint32(textOff), uint32(len(code)), 0, 0, 4, 0)
	// [2] .symtab
	putShdr32(sh[2*shdrSize:], shNameSymtab, shtSymTab, 0, 0, uint32(symtabOff), uint32(len(symtab)), 3, uint32(1+localCount), 4, symEntSize)
	// [3] .strtab
	putShdr32(sh[3*shdrSize:], shNameStrtab, shtStrTab, 0, 0, uint32(strtabOff), uint32(len(strtab)), 0, 0, 1, 0)
	// [4] .shstrtab
	putShdr32(sh[4*shdrSize:], shNameShstrtab, shtStrTab, 0, 0, uint32(shstrtabOff), uint32(len(shstrtab)), 0, 0, 1, 0)

	return bin
}
