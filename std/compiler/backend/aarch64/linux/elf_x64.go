//go:build !no_backend_linux_amd64 || !no_backend_arm64

package linux

import (
	"j5.nz/rtg/std/compiler/backend/aarch64"
	"j5.nz/rtg/std/compiler/ir"
)

// === ELF64 Binary Builder ===

type symEntry struct {
	nameOff int
	value   uint64
	size    uint64
}

func BuildELF64(g *aarch64.CodeGen, irmod *ir.IRModule) []byte {
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

	textSize := len(g.Code())
	rodataOffset := (textOffset + textSize + 7) & ^7 // 8-byte align for ARM64 LDR
	rodataSize := len(g.Rodata())
	dataOffset := (rodataOffset + rodataSize + 7) & ^7 // 8-byte align for ARM64 LDR
	dataSize := len(g.Data())

	loadedSize := dataOffset + dataSize // end of PT_LOAD segment

	// Virtual addresses
	textVAddr := g.BaseAddr() + uint64(textOffset)
	rodataVAddr := g.BaseAddr() + uint64(rodataOffset)
	dataVAddr := g.BaseAddr() + uint64(dataOffset)

	if g.IsArm64() {
		// ARM64: patch ADRP+ADD/LDR pairs with PC-relative offsets
		for i := 0; i < g.CallFixupCount(); i++ {
			codeOffset, targetName, value := g.CallFixupAt(i)
			if targetName == "$rodata_header$" {
				pcAddr := textVAddr + uint64(codeOffset)
				targetAddr := rodataVAddr + value
				g.PatchAdrpAddOrLdr(codeOffset, pcAddr, targetAddr)
			} else if targetName == "$data_addr$" {
				pcAddr := textVAddr + uint64(codeOffset)
				targetAddr := dataVAddr + value
				g.PatchAdrpAddOrLdr(codeOffset, pcAddr, targetAddr)
			}
		}
	} else {
		// x86-64: fix up string headers in rodata with absolute virtual addresses
		for _, headerOff := range g.StringMap() {
			dataOff := aarch64.GetU64(g.Rodata()[headerOff : headerOff+8])
			aarch64.PutU64(g.Rodata()[headerOff:headerOff+8], rodataVAddr+dataOff)
		}

		// Fix up code references to rodata headers and data section
		for i := 0; i < g.CallFixupCount(); i++ {
			codeOffset, targetName, _ := g.CallFixupAt(i)
			if targetName == "$rodata_header$" {
				headerOff := aarch64.GetU64(g.Code()[codeOffset : codeOffset+8])
				aarch64.PutU64(g.Code()[codeOffset:codeOffset+8], rodataVAddr+headerOff)
			} else if targetName == "$data_addr$" {
				dataOff := aarch64.GetU64(g.Code()[codeOffset : codeOffset+8])
				aarch64.PutU64(g.Code()[codeOffset:codeOffset+8], dataVAddr+dataOff)
			}
		}
	}

	// === Build .strtab (symbol name strings) ===
	var strtab []byte
	strtab = append(strtab, 0) // null byte at index 0

	// _start symbol
	startNameOff := len(strtab)
	strtab = append(strtab, []byte("_start")...)
	strtab = append(strtab, 0)

	// Function name offsets for symtab
	var syms []symEntry

	// _start entry
	startSize := uint64(0)
	if len(irmod.Funcs) > 0 {
		if v, ok := g.LookupFuncOffset(irmod.Funcs[0].Name); ok {
			startSize = uint64(v)
		}
	} else {
		startSize = uint64(textSize)
	}
	syms = append(syms, symEntry{startNameOff, textVAddr, startSize})

	// All compiled functions
	for i, f := range irmod.Funcs {
		nameOff := len(strtab)
		strtab = append(strtab, []byte(f.Name)...)
		strtab = append(strtab, 0)

		funcStart, _ := g.LookupFuncOffset(f.Name)
		var funcSize int
		if i+1 < len(irmod.Funcs) {
			nextStart, _ := g.LookupFuncOffset(irmod.Funcs[i+1].Name)
			funcSize = nextStart - funcStart
		} else {
			funcSize = textSize - funcStart
		}
		syms = append(syms, symEntry{nameOff, textVAddr + uint64(funcStart), uint64(funcSize)})
	}

	// === Build .symtab ===
	symEntrySize := 24
	// Entry 0: null symbol + 1 (_start) + len(irmod.Funcs)
	symtabSize := (1 + len(syms)) * symEntrySize
	symtab := make([]byte, symtabSize)

	// Entry 0 is all zeros (null symbol) — already zero from make()
	for i, sym := range syms {
		off := (i + 1) * symEntrySize
		aarch64.PutU32(symtab[off:], uint32(sym.nameOff)) // st_name
		symtab[off+4] = 0x12                              // st_info: STT_FUNC | STB_GLOBAL<<4
		symtab[off+5] = 0                                 // st_other
		aarch64.PutU16(symtab[off+6:], 1)                 // st_shndx: .text section index
		aarch64.PutU64(symtab[off+8:], sym.value)         // st_value
		aarch64.PutU64(symtab[off+16:], sym.size)         // st_size
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
	aarch64.PutU16(elf[16:], 2) // e_type: ET_EXEC
	var eMachine uint16 = 62    // EM_X86_64
	if g.IsArm64() {
		eMachine = 183 // EM_AARCH64
	}
	aarch64.PutU16(elf[18:], eMachine)
	aarch64.PutU32(elf[20:], 1)                     // e_version: EV_CURRENT
	aarch64.PutU64(elf[24:], entryAddr)             // e_entry
	aarch64.PutU64(elf[32:], uint64(elfHeaderSize)) // e_phoff
	if g.Target().StripBinary {
		aarch64.PutU64(elf[40:], 0) // e_shoff
	} else {
		aarch64.PutU64(elf[40:], uint64(shdrOffset)) // e_shoff
	}
	aarch64.PutU32(elf[48:], 0)                     // e_flags
	aarch64.PutU16(elf[52:], uint16(elfHeaderSize)) // e_ehsize
	aarch64.PutU16(elf[54:], uint16(phdrSize))      // e_phentsize
	aarch64.PutU16(elf[56:], 1)                     // e_phnum
	aarch64.PutU16(elf[58:], uint16(shdrEntrySize)) // e_shentsize
	if g.Target().StripBinary {
		aarch64.PutU16(elf[60:], 0) // e_shnum
		aarch64.PutU16(elf[62:], 0) // e_shstrndx
	} else {
		aarch64.PutU16(elf[60:], uint16(shdrCount)) // e_shnum
		aarch64.PutU16(elf[62:], 6)                 // e_shstrndx: index of .shstrtab
	}

	// Program header (single PT_LOAD, RWX)
	phdr := elf[elfHeaderSize:]
	aarch64.PutU32(phdr[0:], 1)                   // p_type: PT_LOAD
	aarch64.PutU32(phdr[4:], 7)                   // p_flags: PF_R|PF_W|PF_X
	aarch64.PutU64(phdr[8:], 0)                   // p_offset: 0 (load from start of file)
	aarch64.PutU64(phdr[16:], g.BaseAddr())       // p_vaddr
	aarch64.PutU64(phdr[24:], g.BaseAddr())       // p_paddr
	aarch64.PutU64(phdr[32:], uint64(loadedSize)) // p_filesz
	aarch64.PutU64(phdr[40:], uint64(loadedSize)) // p_memsz
	aarch64.PutU64(phdr[48:], 0x200000)           // p_align: 2MB

	// Copy loaded sections
	copy(elf[textOffset:], g.Code())
	copy(elf[rodataOffset:], g.Rodata())
	copy(elf[dataOffset:], g.Data())

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
		aarch64.PutU32(s[0:], uint32(shNameText))  // sh_name
		aarch64.PutU32(s[4:], 1)                   // sh_type: SHT_PROGBITS
		aarch64.PutU64(s[8:], 6)                   // sh_flags: SHF_ALLOC|SHF_EXECINSTR
		aarch64.PutU64(s[16:], textVAddr)          // sh_addr
		aarch64.PutU64(s[24:], uint64(textOffset)) // sh_offset
		aarch64.PutU64(s[32:], uint64(textSize))   // sh_size
		aarch64.PutU32(s[40:], 0)                  // sh_link
		aarch64.PutU32(s[44:], 0)                  // sh_info
		aarch64.PutU64(s[48:], 16)                 // sh_addralign
		aarch64.PutU64(s[56:], 0)                  // sh_entsize

		// Section 2: .rodata
		s = shdr[2*shdrEntrySize:]
		aarch64.PutU32(s[0:], uint32(shNameRodata))
		aarch64.PutU32(s[4:], 1) // SHT_PROGBITS
		aarch64.PutU64(s[8:], 2) // SHF_ALLOC
		aarch64.PutU64(s[16:], rodataVAddr)
		aarch64.PutU64(s[24:], uint64(rodataOffset))
		aarch64.PutU64(s[32:], uint64(rodataSize))
		aarch64.PutU64(s[48:], 8) // sh_addralign

		// Section 3: .data
		s = shdr[3*shdrEntrySize:]
		aarch64.PutU32(s[0:], uint32(shNameData))
		aarch64.PutU32(s[4:], 1) // SHT_PROGBITS
		aarch64.PutU64(s[8:], 3) // SHF_ALLOC|SHF_WRITE
		aarch64.PutU64(s[16:], dataVAddr)
		aarch64.PutU64(s[24:], uint64(dataOffset))
		aarch64.PutU64(s[32:], uint64(dataSize))
		aarch64.PutU64(s[48:], 8) // sh_addralign

		// Section 4: .symtab
		s = shdr[4*shdrEntrySize:]
		aarch64.PutU32(s[0:], uint32(shNameSymtab))
		aarch64.PutU32(s[4:], 2)  // SHT_SYMTAB
		aarch64.PutU64(s[8:], 0)  // no flags
		aarch64.PutU64(s[16:], 0) // sh_addr: not loaded
		aarch64.PutU64(s[24:], uint64(symtabOffset))
		aarch64.PutU64(s[32:], uint64(symtabSize))
		aarch64.PutU32(s[40:], 5)                    // sh_link: index of .strtab
		aarch64.PutU32(s[44:], 1)                    // sh_info: index of first global symbol (after null)
		aarch64.PutU64(s[48:], 8)                    // sh_addralign
		aarch64.PutU64(s[56:], uint64(symEntrySize)) // sh_entsize

		// Section 5: .strtab
		s = shdr[5*shdrEntrySize:]
		aarch64.PutU32(s[0:], uint32(shNameStrtab))
		aarch64.PutU32(s[4:], 3) // SHT_STRTAB
		aarch64.PutU64(s[8:], 0)
		aarch64.PutU64(s[16:], 0)
		aarch64.PutU64(s[24:], uint64(strtabOffset))
		aarch64.PutU64(s[32:], uint64(len(strtab)))
		aarch64.PutU64(s[48:], 1) // sh_addralign

		// Section 6: .shstrtab
		s = shdr[6*shdrEntrySize:]
		aarch64.PutU32(s[0:], uint32(shNameShstrtab))
		aarch64.PutU32(s[4:], 3) // SHT_STRTAB
		aarch64.PutU64(s[8:], 0)
		aarch64.PutU64(s[16:], 0)
		aarch64.PutU64(s[24:], uint64(shstrtabOffset))
		aarch64.PutU64(s[32:], uint64(len(shstrtab)))
		aarch64.PutU64(s[48:], 1) // sh_addralign
	}

	return elf
}
