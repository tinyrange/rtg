//go:build !no_backend_linux_amd64 || !no_backend_arm64

package x64

import (
	"j5.nz/rtg/std/compiler/backend/x64/core"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// === ELF64 Binary Builder ===

func buildELF64(g *core.CodeGen, irmod *ir.IRModule) []byte {
	// Layout:
	// [ELF header: 64 bytes]
	// [Program header: 56 bytes]
	// [Padding to 16-byte align: variable]
	// [.text]
	// [.rodata]
	// [.data]
	// --- end of PT_LOAD segment ---
	// [.symtab]
	// [.strtab]
	// [.shstrtab]
	// [Section header table: 7 × 64 bytes]

	elfHeaderSize := 64
	phdrSize := 56
	headerTotal := elfHeaderSize + phdrSize
	// Align to 16 bytes
	textOffset := (headerTotal + 15) & ^15

	textSize := len(g.Code)
	rodataOffset := (textOffset + textSize + 7) & ^7 // 8-byte align for ARM64 LDR
	rodataSize := len(g.Rodata)
	dataOffset := (rodataOffset + rodataSize + 7) & ^7 // 8-byte align for ARM64 LDR
	dataSize := len(g.Data)

	loadedSize := dataOffset + dataSize // end of PT_LOAD segment

	// Virtual addresses
	textVAddr := g.BaseAddr + uint64(textOffset)
	rodataVAddr := g.BaseAddr + uint64(rodataOffset)
	dataVAddr := g.BaseAddr + uint64(dataOffset)

	// x86-64: fix up string headers in rodata with absolute virtual addresses.
	g.PatchLinuxStringHeaders(rodataVAddr)

	// Fix up code references to rodata headers and data section.
	g.PatchLinuxDataAndRodataFixups(rodataVAddr, dataVAddr)

	// === Build .strtab (symbol name strings) ===
	var strtab []byte
	strtab = append(strtab, 0) // null byte at index 0

	// _start symbol
	startNameOff := len(strtab)
	strtab = append(strtab, []byte("_start")...)
	strtab = append(strtab, 0)

	// Function name offsets for symtab
	var syms []core.SymEntry

	// _start entry
	startSize := uint64(0)
	if len(irmod.Funcs) > 0 {
		startSize = uint64(g.GetFuncOffset(irmod.Funcs[0].Name))
	} else {
		startSize = uint64(textSize)
	}
	syms = append(syms, core.SymEntry{startNameOff, textVAddr, startSize})

	// All compiled functions
	for i, f := range irmod.Funcs {
		nameOff := len(strtab)
		strtab = append(strtab, []byte(f.Name)...)
		strtab = append(strtab, 0)

		funcStart := g.GetFuncOffset(f.Name)
		var funcSize int
		if i+1 < len(irmod.Funcs) {
			funcSize = g.GetFuncOffset(irmod.Funcs[i+1].Name) - funcStart
		} else {
			funcSize = textSize - funcStart
		}
		syms = append(syms, core.SymEntry{nameOff, textVAddr + uint64(funcStart), uint64(funcSize)})
	}

	// === Build .symtab ===
	symEntrySize := 24
	// Entry 0: null symbol + 1 (_start) + len(irmod.Funcs)
	symtabSize := (1 + len(syms)) * symEntrySize
	symtab := make([]byte, symtabSize)

	// Entry 0 is all zeros (null symbol) — already zero from make()
	for i, sym := range syms {
		off := (i + 1) * symEntrySize
		common.PutU32(symtab[off:], uint32(sym.NameOff)) // st_name
		symtab[off+4] = 0x12                             // st_info: STT_FUNC | STB_GLOBAL<<4
		symtab[off+5] = 0                                // st_other
		common.PutU16(symtab[off+6:], 1)                 // st_shndx: .text section index
		common.PutU64(symtab[off+8:], sym.Value)         // st_value
		common.PutU64(symtab[off+16:], sym.Size)         // st_size
	}

	// === Build .shstrtab (section name strings) ===
	// \0.text\0.rodata\0.data\0.symtab\0.strtab\0.shstrtab\0
	shstrtab := []byte("\x00.text\x00.rodata\x00.data\x00.symtab\x00.strtab\x00.shstrtab\x00")
	// Offsets within shstrtab:
	shNameText := 1      // ".text"
	shNameRodata := 7    // ".rodata"
	shNameData := 15     // ".data"
	shNameSymtab := 21   // ".symtab"
	shNameStrtab := 29   // ".strtab"
	shNameShstrtab := 37 // ".shstrtab"

	// === Compute file offsets for new sections ===
	symtabOffset := loadedSize
	strtabOffset := symtabOffset + symtabSize
	shstrtabOffset := strtabOffset + len(strtab)
	shdrOffset := shstrtabOffset + len(shstrtab)

	shdrEntrySize := 64
	shdrCount := 7
	shdrTableSize := shdrCount * shdrEntrySize

	totalSize := shdrOffset + shdrTableSize
	if g.Target().StripBinary {
		totalSize = loadedSize
	}

	// Entry point
	entryAddr := textVAddr // _start is at beginning of .text

	// Build the binary
	elf := make([]byte, totalSize)

	// ELF header
	elf[0] = 0x7f
	elf[1] = 'E'
	elf[2] = 'L'
	elf[3] = 'F'
	elf[4] = 2 // ELFCLASS64
	elf[5] = 1 // ELFDATA2LSB
	elf[6] = 1 // EV_CURRENT
	elf[7] = 0 // ELFOSABI_NONE
	// bytes 8-15: padding (zero)
	common.PutU16(elf[16:], 2) // e_type: ET_EXEC
	var eMachine uint16 = 62   // EM_X86_64
	common.PutU16(elf[18:], eMachine)
	common.PutU32(elf[20:], 1)                     // e_version: EV_CURRENT
	common.PutU64(elf[24:], entryAddr)             // e_entry
	common.PutU64(elf[32:], uint64(elfHeaderSize)) // e_phoff
	if g.Target().StripBinary {
		common.PutU64(elf[40:], 0) // e_shoff
	} else {
		common.PutU64(elf[40:], uint64(shdrOffset)) // e_shoff
	}
	common.PutU32(elf[48:], 0)                     // e_flags
	common.PutU16(elf[52:], uint16(elfHeaderSize)) // e_ehsize
	common.PutU16(elf[54:], uint16(phdrSize))      // e_phentsize
	common.PutU16(elf[56:], 1)                     // e_phnum
	common.PutU16(elf[58:], uint16(shdrEntrySize)) // e_shentsize
	if g.Target().StripBinary {
		common.PutU16(elf[60:], 0) // e_shnum
		common.PutU16(elf[62:], 0) // e_shstrndx
	} else {
		common.PutU16(elf[60:], uint16(shdrCount)) // e_shnum
		common.PutU16(elf[62:], 6)                 // e_shstrndx: index of .shstrtab
	}

	// Program header (single PT_LOAD, RWX)
	phdr := elf[elfHeaderSize:]
	common.PutU32(phdr[0:], 1)                   // p_type: PT_LOAD
	common.PutU32(phdr[4:], 7)                   // p_flags: PF_R|PF_W|PF_X
	common.PutU64(phdr[8:], 0)                   // p_offset: 0 (load from start of file)
	common.PutU64(phdr[16:], g.BaseAddr)         // p_vaddr
	common.PutU64(phdr[24:], g.BaseAddr)         // p_paddr
	common.PutU64(phdr[32:], uint64(loadedSize)) // p_filesz
	common.PutU64(phdr[40:], uint64(loadedSize)) // p_memsz
	common.PutU64(phdr[48:], 0x200000)           // p_align: 2MB

	// Copy loaded sections
	copy(elf[textOffset:], g.Code)
	copy(elf[rodataOffset:], g.Rodata)
	copy(elf[dataOffset:], g.Data)

	if !g.Target().StripBinary {
		// Copy debug sections (not part of PT_LOAD)
		copy(elf[symtabOffset:], symtab)
		copy(elf[strtabOffset:], strtab)
		copy(elf[shstrtabOffset:], shstrtab)

		// === Write section header table ===
		shdr := elf[shdrOffset:]

		// Section 0: SHT_NULL (all zeros — already zero from make())

		// Section 1: .text
		s := shdr[1*shdrEntrySize:]
		common.PutU32(s[0:], uint32(shNameText))  // sh_name
		common.PutU32(s[4:], 1)                   // sh_type: SHT_PROGBITS
		common.PutU64(s[8:], 6)                   // sh_flags: SHF_ALLOC|SHF_EXECINSTR
		common.PutU64(s[16:], textVAddr)          // sh_addr
		common.PutU64(s[24:], uint64(textOffset)) // sh_offset
		common.PutU64(s[32:], uint64(textSize))   // sh_size
		common.PutU32(s[40:], 0)                  // sh_link
		common.PutU32(s[44:], 0)                  // sh_info
		common.PutU64(s[48:], 16)                 // sh_addralign
		common.PutU64(s[56:], 0)                  // sh_entsize

		// Section 2: .rodata
		s = shdr[2*shdrEntrySize:]
		common.PutU32(s[0:], uint32(shNameRodata))
		common.PutU32(s[4:], 1) // SHT_PROGBITS
		common.PutU64(s[8:], 2) // SHF_ALLOC
		common.PutU64(s[16:], rodataVAddr)
		common.PutU64(s[24:], uint64(rodataOffset))
		common.PutU64(s[32:], uint64(rodataSize))
		common.PutU64(s[48:], 8) // sh_addralign

		// Section 3: .data
		s = shdr[3*shdrEntrySize:]
		common.PutU32(s[0:], uint32(shNameData))
		common.PutU32(s[4:], 1) // SHT_PROGBITS
		common.PutU64(s[8:], 3) // SHF_ALLOC|SHF_WRITE
		common.PutU64(s[16:], dataVAddr)
		common.PutU64(s[24:], uint64(dataOffset))
		common.PutU64(s[32:], uint64(dataSize))
		common.PutU64(s[48:], 8) // sh_addralign

		// Section 4: .symtab
		s = shdr[4*shdrEntrySize:]
		common.PutU32(s[0:], uint32(shNameSymtab))
		common.PutU32(s[4:], 2)  // SHT_SYMTAB
		common.PutU64(s[8:], 0)  // no flags
		common.PutU64(s[16:], 0) // sh_addr: not loaded
		common.PutU64(s[24:], uint64(symtabOffset))
		common.PutU64(s[32:], uint64(symtabSize))
		common.PutU32(s[40:], 5)                    // sh_link: index of .strtab
		common.PutU32(s[44:], 1)                    // sh_info: index of first global symbol (after null)
		common.PutU64(s[48:], 8)                    // sh_addralign
		common.PutU64(s[56:], uint64(symEntrySize)) // sh_entsize

		// Section 5: .strtab
		s = shdr[5*shdrEntrySize:]
		common.PutU32(s[0:], uint32(shNameStrtab))
		common.PutU32(s[4:], 3) // SHT_STRTAB
		common.PutU64(s[8:], 0)
		common.PutU64(s[16:], 0)
		common.PutU64(s[24:], uint64(strtabOffset))
		common.PutU64(s[32:], uint64(len(strtab)))
		common.PutU64(s[48:], 1) // sh_addralign

		// Section 6: .shstrtab
		s = shdr[6*shdrEntrySize:]
		common.PutU32(s[0:], uint32(shNameShstrtab))
		common.PutU32(s[4:], 3) // SHT_STRTAB
		common.PutU64(s[8:], 0)
		common.PutU64(s[16:], 0)
		common.PutU64(s[24:], uint64(shstrtabOffset))
		common.PutU64(s[32:], uint64(len(shstrtab)))
		common.PutU64(s[48:], 1) // sh_addralign
	}

	return elf
}
