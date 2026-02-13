//go:build !no_backend_linux_i386

package main

// === ELF32 Binary Builder ===

func (g *CodeGen) buildELF32(irmod *IRModule) []byte {
	// Layout:
	// [ELF header: 52 bytes]
	// [Program header: 32 bytes]
	// [Padding to 16-byte align]
	// [.text]
	// [.rodata]
	// [.data]
	// --- end of PT_LOAD segment ---
	// [.symtab]
	// [.strtab]
	// [.shstrtab]
	// [Section header table: 7 x 40 bytes]

	elfHeaderSize := 52
	phdrSize := 32
	headerTotal := elfHeaderSize + phdrSize
	// Align to 16 bytes
	textOffset := (headerTotal + 15) & ^15

	textSize := len(g.code)
	rodataOffset := textOffset + textSize
	rodataSize := len(g.rodata)
	dataOffset := rodataOffset + rodataSize
	dataSize := len(g.data)

	loadedSize := dataOffset + dataSize

	// Virtual addresses
	textVAddr := g.baseAddr + uint64(textOffset)
	rodataVAddr := g.baseAddr + uint64(rodataOffset)
	dataVAddr := g.baseAddr + uint64(dataOffset)

	// Fix up string headers in rodata: each header's data_ptr field (4 bytes)
	for _, headerOff := range g.stringMap {
		dataOff := getU32(g.rodata[headerOff : headerOff+4])
		putU32(g.rodata[headerOff:headerOff+4], uint32(rodataVAddr)+dataOff)
	}

	// Fix up code references to rodata headers and data section (4-byte imm32)
	for _, fix := range g.callFixups {
		if fix.Target == "$rodata_header$" {
			headerOff := getU32(g.code[fix.CodeOffset : fix.CodeOffset+4])
			putU32(g.code[fix.CodeOffset:fix.CodeOffset+4], uint32(rodataVAddr)+headerOff)
		} else if fix.Target == "$data_addr$" {
			dataOff := getU32(g.code[fix.CodeOffset : fix.CodeOffset+4])
			putU32(g.code[fix.CodeOffset:fix.CodeOffset+4], uint32(dataVAddr)+dataOff)
		}
	}

	// === Build .strtab ===
	var strtab []byte
	strtab = append(strtab, 0)

	startNameOff := len(strtab)
	strtab = append(strtab, []byte("_start")...)
	strtab = append(strtab, 0)

	var syms []symEntry

	startSize := uint64(0)
	if len(irmod.Funcs) > 0 {
		startSize = uint64(g.funcOffsets[irmod.Funcs[0].Name])
	} else {
		startSize = uint64(textSize)
	}
	syms = append(syms, symEntry{startNameOff, textVAddr, startSize})

	for i, f := range irmod.Funcs {
		nameOff := len(strtab)
		strtab = append(strtab, []byte(f.Name)...)
		strtab = append(strtab, 0)

		funcStart := g.funcOffsets[f.Name]
		var funcSize int
		if i+1 < len(irmod.Funcs) {
			funcSize = g.funcOffsets[irmod.Funcs[i+1].Name] - funcStart
		} else {
			funcSize = textSize - funcStart
		}
		syms = append(syms, symEntry{nameOff, textVAddr + uint64(funcStart), uint64(funcSize)})
	}

	// === Build .symtab (ELF32: 16 bytes per entry) ===
	symEntrySize := 16
	symtabSize := (1 + len(syms)) * symEntrySize
	symtab := make([]byte, symtabSize)

	// Entry 0 is all zeros (null symbol)
	for i, sym := range syms {
		off := (i + 1) * symEntrySize
		putU32(symtab[off:], uint32(sym.nameOff)) // st_name
		putU32(symtab[off+4:], uint32(sym.value)) // st_value
		putU32(symtab[off+8:], uint32(sym.size))  // st_size
		symtab[off+12] = 0x12                      // st_info: STT_FUNC | STB_GLOBAL<<4
		symtab[off+13] = 0                         // st_other
		putU16(symtab[off+14:], 1)                 // st_shndx: .text
	}

	// === Build .shstrtab ===
	shstrtab := []byte("\x00.text\x00.rodata\x00.data\x00.symtab\x00.strtab\x00.shstrtab\x00")
	shNameText := 1
	shNameRodata := 7
	shNameData := 15
	shNameSymtab := 21
	shNameStrtab := 29
	shNameShstrtab := 37

	// === Compute file offsets ===
	symtabOffset := loadedSize
	strtabOffset := symtabOffset + symtabSize
	shstrtabOffset := strtabOffset + len(strtab)
	shdrOffset := shstrtabOffset + len(shstrtab)

	shdrEntrySize := 40
	shdrCount := 7
	shdrTableSize := shdrCount * shdrEntrySize

	totalSize := shdrOffset + shdrTableSize

	entryAddr := textVAddr

	// Build the binary
	elf := make([]byte, totalSize)

	// ELF header (52 bytes)
	elf[0] = 0x7f
	elf[1] = 'E'
	elf[2] = 'L'
	elf[3] = 'F'
	elf[4] = 1 // ELFCLASS32
	elf[5] = 1 // ELFDATA2LSB
	elf[6] = 1 // EV_CURRENT
	elf[7] = 0 // ELFOSABI_NONE
	putU16(elf[16:], 2)                      // e_type: ET_EXEC
	putU16(elf[18:], 3)                      // e_machine: EM_386
	putU32(elf[20:], 1)                      // e_version: EV_CURRENT
	putU32(elf[24:], uint32(entryAddr))      // e_entry (4 bytes)
	putU32(elf[28:], uint32(elfHeaderSize))  // e_phoff (4 bytes)
	putU32(elf[32:], uint32(shdrOffset))     // e_shoff (4 bytes)
	putU32(elf[36:], 0)                      // e_flags
	putU16(elf[40:], uint16(elfHeaderSize))  // e_ehsize
	putU16(elf[42:], uint16(phdrSize))       // e_phentsize
	putU16(elf[44:], 1)                      // e_phnum
	putU16(elf[46:], uint16(shdrEntrySize))  // e_shentsize
	putU16(elf[48:], uint16(shdrCount))      // e_shnum
	putU16(elf[50:], 6)                      // e_shstrndx

	// Program header (32 bytes)
	phdr := elf[elfHeaderSize:]
	putU32(phdr[0:], 1)                     // p_type: PT_LOAD
	putU32(phdr[4:], 0)                     // p_offset: 0
	putU32(phdr[8:], uint32(g.baseAddr))    // p_vaddr
	putU32(phdr[12:], uint32(g.baseAddr))   // p_paddr
	putU32(phdr[16:], uint32(loadedSize))   // p_filesz
	putU32(phdr[20:], uint32(loadedSize))   // p_memsz
	putU32(phdr[24:], 7)                    // p_flags: PF_R|PF_W|PF_X
	putU32(phdr[28:], 0x1000)               // p_align: 4KB

	// Copy loaded sections
	copy(elf[textOffset:], g.code)
	copy(elf[rodataOffset:], g.rodata)
	copy(elf[dataOffset:], g.data)

	// Copy debug sections
	copy(elf[symtabOffset:], symtab)
	copy(elf[strtabOffset:], strtab)
	copy(elf[shstrtabOffset:], shstrtab)

	// === Write section header table (40 bytes each) ===
	shdr := elf[shdrOffset:]

	// Section 0: SHT_NULL (all zeros)

	// Section 1: .text
	s := shdr[1*shdrEntrySize:]
	putU32(s[0:], uint32(shNameText))
	putU32(s[4:], 1)                        // SHT_PROGBITS
	putU32(s[8:], 6)                        // SHF_ALLOC|SHF_EXECINSTR
	putU32(s[12:], uint32(textVAddr))
	putU32(s[16:], uint32(textOffset))
	putU32(s[20:], uint32(textSize))
	putU32(s[24:], 0)                       // sh_link
	putU32(s[28:], 0)                       // sh_info
	putU32(s[32:], 16)                      // sh_addralign
	putU32(s[36:], 0)                       // sh_entsize

	// Section 2: .rodata
	s = shdr[2*shdrEntrySize:]
	putU32(s[0:], uint32(shNameRodata))
	putU32(s[4:], 1)
	putU32(s[8:], 2)                        // SHF_ALLOC
	putU32(s[12:], uint32(rodataVAddr))
	putU32(s[16:], uint32(rodataOffset))
	putU32(s[20:], uint32(rodataSize))
	putU32(s[32:], 4)

	// Section 3: .data
	s = shdr[3*shdrEntrySize:]
	putU32(s[0:], uint32(shNameData))
	putU32(s[4:], 1)
	putU32(s[8:], 3)                        // SHF_ALLOC|SHF_WRITE
	putU32(s[12:], uint32(dataVAddr))
	putU32(s[16:], uint32(dataOffset))
	putU32(s[20:], uint32(dataSize))
	putU32(s[32:], 4)

	// Section 4: .symtab
	s = shdr[4*shdrEntrySize:]
	putU32(s[0:], uint32(shNameSymtab))
	putU32(s[4:], 2)                        // SHT_SYMTAB
	putU32(s[8:], 0)
	putU32(s[12:], 0)
	putU32(s[16:], uint32(symtabOffset))
	putU32(s[20:], uint32(symtabSize))
	putU32(s[24:], 5)                       // sh_link: .strtab
	putU32(s[28:], 1)                       // sh_info
	putU32(s[32:], 4)
	putU32(s[36:], uint32(symEntrySize))

	// Section 5: .strtab
	s = shdr[5*shdrEntrySize:]
	putU32(s[0:], uint32(shNameStrtab))
	putU32(s[4:], 3)                        // SHT_STRTAB
	putU32(s[8:], 0)
	putU32(s[12:], 0)
	putU32(s[16:], uint32(strtabOffset))
	putU32(s[20:], uint32(len(strtab)))
	putU32(s[32:], 1)

	// Section 6: .shstrtab
	s = shdr[6*shdrEntrySize:]
	putU32(s[0:], uint32(shNameShstrtab))
	putU32(s[4:], 3)                        // SHT_STRTAB
	putU32(s[8:], 0)
	putU32(s[12:], 0)
	putU32(s[16:], uint32(shstrtabOffset))
	putU32(s[20:], uint32(len(shstrtab)))
	putU32(s[32:], 1)

	return elf
}
