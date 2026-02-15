//go:build !no_backend_arm64 || !no_backend_windows_amd64

package main

// buildPE64 assembles a PE32+ (64-bit) executable from the compiled code, rodata, data,
// and a list of kernel32.dll imports. Used for windows/arm64.
func (g *CodeGen) buildPE64(irmod *IRModule, imports []string) []byte {
	// PE32+ Layout:
	// 0x000  DOS Header (64 bytes)
	// 0x040  DOS Stub (64 bytes)
	// 0x080  PE Signature (4 bytes)
	// 0x084  COFF Header (20 bytes)
	// 0x098  Optional Header (240 bytes)
	// 0x188  Section Table (6 sections x 40 bytes = 240 bytes)
	//        (pad to FileAlignment=0x200)
	// 0x200  .text / .rdata / .data / .idata / .debug_abbrev / .debug_info

	const (
		fileAlignment    = 0x200
		sectionAlignment = 0x1000
		imageBase        = 0x400000
	)

	dosHeaderSize := 64
	dosStubSize := 64
	peSignatureSize := 4
	coffHeaderSize := 20
	optionalHeaderSize := 240
	numSections := 6
	sectionTableSize := numSections * 40

	headersRawSize := dosHeaderSize + dosStubSize + peSignatureSize + coffHeaderSize + optionalHeaderSize + sectionTableSize
	headersAligned := alignUp(headersRawSize, fileAlignment)

	// Section sizes
	textRawSize := alignUp(len(g.code), fileAlignment)
	rdataRawSize := alignUp(len(g.rodata), fileAlignment)
	dataRawSize := alignUp(len(g.data), fileAlignment)

	// Build .idata section with 8-byte ILT/IAT entries
	idataContent := g.buildIData64(imports)
	idataRawSize := alignUp(len(idataContent), fileAlignment)

	// RVAs
	textRVA := sectionAlignment // 0x1000
	rdataRVA := textRVA + alignUp(len(g.code), sectionAlignment)
	dataRVA := rdataRVA + alignUp(len(g.rodata), sectionAlignment)
	idataRVA := dataRVA + alignUp(len(g.data), sectionAlignment)

	// Fix up .idata internal RVAs
	g.fixupIData64(idataContent, idataRVA, imports)

	// Build DWARF debug sections with 8-byte addresses
	textVA := imageBase + textRVA
	debugAbbrev, debugInfo := g.buildDWARF64(irmod, textVA, len(g.code))
	debugAbbrevRawSize := alignUp(len(debugAbbrev), fileAlignment)
	debugInfoRawSize := alignUp(len(debugInfo), fileAlignment)

	debugAbbrevRVA := idataRVA + alignUp(len(idataContent), sectionAlignment)
	debugInfoRVA := debugAbbrevRVA + alignUp(len(debugAbbrev), sectionAlignment)

	// File offsets
	textFileOff := headersAligned
	rdataFileOff := textFileOff + textRawSize
	dataFileOff := rdataFileOff + rdataRawSize
	idataFileOff := dataFileOff + dataRawSize
	debugAbbrevFileOff := idataFileOff + idataRawSize
	debugInfoFileOff := debugAbbrevFileOff + debugAbbrevRawSize

	// COFF symbols
	coffSyms, coffStrtab, numSyms := g.buildCOFFSymbols(irmod)

	debugAbbrevNameOff := len(coffStrtab)
	coffStrtab = append(coffStrtab, []byte(".debug_abbrev")...)
	coffStrtab = append(coffStrtab, 0)
	debugInfoNameOff := len(coffStrtab)
	coffStrtab = append(coffStrtab, []byte(".debug_info")...)
	coffStrtab = append(coffStrtab, 0)
	putU32(coffStrtab[0:], uint32(len(coffStrtab)))

	symtabFileOff := debugInfoFileOff + debugInfoRawSize
	strtabFileOff := symtabFileOff + len(coffSyms)
	totalFileSize := strtabFileOff + len(coffStrtab)

	imageSize := debugInfoRVA + alignUp(len(debugInfo), sectionAlignment)

	// Fix up string headers and code references
	iatOffsets := g.buildIATOffsets64(imports)
	if g.isArm64 {
		// ARM64: string headers are in .data section
		for _, headerOff := range g.stringMap {
			rodataOff := g.stringRodataMap[headerOff]
			putU64(g.data[headerOff:headerOff+8], uint64(imageBase+rdataRVA+rodataOff))
		}

		// Fix up code references (ADRP+ADD/LDR pairs)
		for _, fix := range g.callFixups {
			if fix.Target == "$rodata_header$" {
				pcAddr := uint64(imageBase + textRVA + fix.CodeOffset)
				targetAddr := uint64(imageBase+rdataRVA) + fix.Value
				g.patchAdrpAddOrLdr(fix.CodeOffset, pcAddr, targetAddr)
			} else if fix.Target == "$data_addr$" {
				pcAddr := uint64(imageBase + textRVA + fix.CodeOffset)
				targetAddr := uint64(imageBase+dataRVA) + fix.Value
				g.patchAdrpAddOrLdr(fix.CodeOffset, pcAddr, targetAddr)
			} else if len(fix.Target) > 5 && fix.Target[0:5] == "$iat$" {
				funcName := fix.Target[5:]
				iatOff, ok := iatOffsets[funcName]
				if ok {
					pcAddr := uint64(imageBase + textRVA + fix.CodeOffset)
					targetAddr := uint64(imageBase+idataRVA) + uint64(iatOff)
					g.patchAdrpLdr(fix.CodeOffset, pcAddr, targetAddr)
				}
			}
		}
	} else {
		// x64: string headers are in .rodata section
		for _, headerOff := range g.stringMap {
			dataOff := getU64(g.rodata[headerOff : headerOff+8])
			putU64(g.rodata[headerOff:headerOff+8], uint64(imageBase+rdataRVA)+dataOff)
		}

		// Fix up code references (movabs imm64 and RIP-relative call)
		for _, fix := range g.callFixups {
			if fix.Target == "$rodata_header$" {
				// Patch 8-byte movabs immediate with rodata VA
				headerOff := getU64(g.code[fix.CodeOffset : fix.CodeOffset+8])
				putU64(g.code[fix.CodeOffset:fix.CodeOffset+8], uint64(imageBase+rdataRVA)+headerOff)
			} else if fix.Target == "$data_addr$" {
				// Patch 8-byte movabs immediate with data VA
				dataOff := getU64(g.code[fix.CodeOffset : fix.CodeOffset+8])
				putU64(g.code[fix.CodeOffset:fix.CodeOffset+8], uint64(imageBase+dataRVA)+dataOff)
			} else if len(fix.Target) > 5 && fix.Target[0:5] == "$iat$" {
				funcName := fix.Target[5:]
				iatOff, ok := iatOffsets[funcName]
				if ok {
					// Patch RIP-relative disp32: target = iatVA, rip = textVA + codeOffset + 4
					iatVA := uint64(imageBase+idataRVA) + uint64(iatOff)
					rip := uint64(imageBase+textRVA) + uint64(fix.CodeOffset) + 4
					disp32 := int32(int64(iatVA) - int64(rip))
					putU32(g.code[fix.CodeOffset:fix.CodeOffset+4], uint32(disp32))
				}
			}
		}
	}

	// Assemble the file
	pe := make([]byte, totalFileSize)

	// === DOS Header (64 bytes) ===
	pe[0] = 'M'
	pe[1] = 'Z'
	putU32(pe[0x3C:], 0x80)

	// === DOS Stub (64 bytes at 0x40) ===
	dosStub := []byte{
		0x0e, 0x1f, 0xba, 0x0e, 0x00, 0xb4, 0x09, 0xcd,
		0x21, 0xb8, 0x01, 0x4c, 0xcd, 0x21, 0x54, 0x68,
		0x69, 0x73, 0x20, 0x70, 0x72, 0x6f, 0x67, 0x72,
		0x61, 0x6d, 0x20, 0x63, 0x61, 0x6e, 0x6e, 0x6f,
		0x74, 0x20, 0x62, 0x65, 0x20, 0x72, 0x75, 0x6e,
		0x20, 0x69, 0x6e, 0x20, 0x44, 0x4f, 0x53, 0x20,
		0x6d, 0x6f, 0x64, 0x65, 0x2e, 0x0d, 0x0d, 0x0a,
		0x24, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	copy(pe[0x40:], dosStub)

	// === PE Signature (4 bytes at 0x80) ===
	pe[0x80] = 'P'
	pe[0x81] = 'E'
	pe[0x82] = 0
	pe[0x83] = 0

	// === COFF Header (20 bytes at 0x84) ===
	coff := pe[0x84:]
	machineType := uint16(0x8664) // IMAGE_FILE_MACHINE_AMD64
	if g.isArm64 {
		machineType = 0xAA64 // IMAGE_FILE_MACHINE_ARM64
	}
	putU16(coff[0:], machineType)                  // Machine
	putU16(coff[2:], uint16(numSections))          // NumberOfSections
	putU32(coff[4:], 0)                            // TimeDateStamp
	putU32(coff[8:], uint32(symtabFileOff))        // PointerToSymbolTable
	putU32(coff[12:], uint32(numSyms))             // NumberOfSymbols
	putU16(coff[16:], uint16(optionalHeaderSize))  // SizeOfOptionalHeader
	putU16(coff[18:], 0x0022)                      // Characteristics: EXECUTABLE_IMAGE | LARGE_ADDRESS_AWARE

	// === Optional Header (240 bytes at 0x98) ===
	opt := pe[0x98:]
	putU16(opt[0:], 0x020B)                        // Magic: PE32+
	opt[2] = 1                                     // MajorLinkerVersion
	opt[3] = 0                                     // MinorLinkerVersion
	putU32(opt[4:], uint32(len(g.code)))           // SizeOfCode
	putU32(opt[8:], uint32(len(g.rodata)))         // SizeOfInitializedData
	putU32(opt[12:], 0)                            // SizeOfUninitializedData
	putU32(opt[16:], uint32(textRVA))              // AddressOfEntryPoint
	putU32(opt[20:], uint32(textRVA))              // BaseOfCode
	// PE32+ has NO BaseOfData field — ImageBase is at offset 24
	putU64(opt[24:], uint64(imageBase))            // ImageBase (8 bytes)
	putU32(opt[32:], uint32(sectionAlignment))     // SectionAlignment
	putU32(opt[36:], uint32(fileAlignment))         // FileAlignment
	putU16(opt[40:], 6)                            // MajorOperatingSystemVersion
	putU16(opt[42:], 0)                            // MinorOperatingSystemVersion
	putU16(opt[44:], 0)                            // MajorImageVersion
	putU16(opt[46:], 0)                            // MinorImageVersion
	putU16(opt[48:], 6)                            // MajorSubsystemVersion
	putU16(opt[50:], 0)                            // MinorSubsystemVersion
	putU32(opt[52:], 0)                            // Win32VersionValue
	putU32(opt[56:], uint32(imageSize))            // SizeOfImage
	putU32(opt[60:], uint32(headersAligned))       // SizeOfHeaders
	putU32(opt[64:], 0)                            // CheckSum
	putU16(opt[68:], 3)                            // Subsystem: IMAGE_SUBSYSTEM_WINDOWS_CUI
	dllChars := uint16(0x0100) // NX_COMPAT (x64: no ASLR since we use movabs absolute addresses)
	if g.isArm64 {
		dllChars = 0x0160 // DYNAMIC_BASE | NX_COMPAT | HIGH_ENTROPY_VA
	}
	putU16(opt[70:], dllChars)                     // DllCharacteristics
	// PE32+: Stack/Heap sizes are 8 bytes each
	putU64(opt[72:], 0x100000)                     // SizeOfStackReserve (1MB)
	putU64(opt[80:], 0x1000)                       // SizeOfStackCommit (4KB)
	putU64(opt[88:], 0x100000)                     // SizeOfHeapReserve (1MB)
	putU64(opt[96:], 0x1000)                       // SizeOfHeapCommit (4KB)
	putU32(opt[104:], 0)                           // LoaderFlags
	putU32(opt[108:], 16)                          // NumberOfRvaAndSizes

	// Data directories (16 entries x 8 bytes = 128 bytes starting at opt[112])
	// [1] Import Table
	importDirRVA, importDirSize := g.getImportDirInfo(imports, idataRVA)
	putU32(opt[112+1*8:], uint32(importDirRVA))
	putU32(opt[112+1*8+4:], uint32(importDirSize))

	// [12] IAT
	iatRVA, iatSize := g.getIATInfo64(imports, idataRVA)
	putU32(opt[112+12*8:], uint32(iatRVA))
	putU32(opt[112+12*8+4:], uint32(iatSize))

	// === Section Table (6 x 40 bytes at 0x188) ===
	sectBase := 0x188

	// .text
	writeSection(pe[sectBase:], ".text",
		len(g.code), textRVA, textRawSize, textFileOff,
		0x60000020) // CODE | EXECUTE | READ

	// .rdata
	writeSection(pe[sectBase+40:], ".rdata",
		len(g.rodata), rdataRVA, rdataRawSize, rdataFileOff,
		0x40000040) // INITIALIZED_DATA | READ

	// .data
	writeSection(pe[sectBase+80:], ".data",
		len(g.data), dataRVA, dataRawSize, dataFileOff,
		0xC0000040) // INITIALIZED_DATA | READ | WRITE

	// .idata
	writeSection(pe[sectBase+120:], ".idata",
		len(idataContent), idataRVA, idataRawSize, idataFileOff,
		0xC0000040) // INITIALIZED_DATA | READ | WRITE

	// .debug_abbrev
	writeSectionLongName(pe[sectBase+160:], debugAbbrevNameOff,
		len(debugAbbrev), debugAbbrevRVA, debugAbbrevRawSize, debugAbbrevFileOff,
		0x42000040) // INITIALIZED_DATA | READ | DISCARDABLE

	// .debug_info
	writeSectionLongName(pe[sectBase+200:], debugInfoNameOff,
		len(debugInfo), debugInfoRVA, debugInfoRawSize, debugInfoFileOff,
		0x42000040) // INITIALIZED_DATA | READ | DISCARDABLE

	// Copy section data
	copy(pe[textFileOff:], g.code)
	copy(pe[rdataFileOff:], g.rodata)
	copy(pe[dataFileOff:], g.data)
	copy(pe[idataFileOff:], idataContent)
	copy(pe[debugAbbrevFileOff:], debugAbbrev)
	copy(pe[debugInfoFileOff:], debugInfo)

	// Copy COFF symbol table and string table
	copy(pe[symtabFileOff:], coffSyms)
	copy(pe[strtabFileOff:], coffStrtab)

	return pe
}

// buildIData64 builds the .idata section with 8-byte ILT/IAT entries for PE32+.
func (g *CodeGen) buildIData64(imports []string) []byte {
	numImports := len(imports)

	// Import Directory Table: 1 real entry + 1 null terminator = 40 bytes
	idtSize := 40

	// ILT: (numImports + 1) * 8 bytes (null-terminated, 8 bytes per entry for PE32+)
	iltSize := (numImports + 1) * 8

	// IAT: identical to ILT
	iatSize := (numImports + 1) * 8

	// Hint/Name Table
	hntOffset := idtSize + iltSize + iatSize
	var hntEntries []byte
	var hntOffsets []int
	for _, name := range imports {
		hntOffsets = append(hntOffsets, hntOffset+len(hntEntries))
		hntEntries = append(hntEntries, 0, 0) // Hint = 0
		hntEntries = append(hntEntries, []byte(name)...)
		hntEntries = append(hntEntries, 0)
		if len(hntEntries)%2 != 0 {
			hntEntries = append(hntEntries, 0)
		}
	}

	// DLL name
	dllNameOffset := hntOffset + len(hntEntries)
	dllName := []byte("kernel32.dll\x00")

	totalSize := dllNameOffset + len(dllName)
	idata := make([]byte, totalSize)

	// Import Directory Table entry (20 bytes)
	iltRVAOffset := idtSize
	iatRVAOffset := idtSize + iltSize

	putU32(idata[0:], uint32(iltRVAOffset))  // OriginalFirstThunk — placeholder
	putU32(idata[4:], 0)                      // TimeDateStamp
	putU32(idata[8:], 0)                      // ForwarderChain
	putU32(idata[12:], uint32(dllNameOffset)) // Name — placeholder
	putU32(idata[16:], uint32(iatRVAOffset))  // FirstThunk — placeholder

	// ILT entries (8 bytes each for PE32+)
	for i := 0; i < numImports; i++ {
		off := iltRVAOffset + i*8
		putU64(idata[off:], uint64(hntOffsets[i])) // RVA of Hint/Name — placeholder
	}

	// IAT entries (8 bytes each, identical to ILT on disk)
	for i := 0; i < numImports; i++ {
		off := iatRVAOffset + i*8
		putU64(idata[off:], uint64(hntOffsets[i])) // RVA of Hint/Name — placeholder
	}

	// Hint/Name Table
	copy(idata[hntOffset:], hntEntries)

	// DLL name
	copy(idata[dllNameOffset:], dllName)

	return idata
}

// fixupIData64 adjusts RVA fields in the .idata content to be actual RVAs.
func (g *CodeGen) fixupIData64(idata []byte, idataRVA int, imports []string) {
	numImports := len(imports)
	idtSize := 40
	iltSize := (numImports + 1) * 8
	iltOff := idtSize
	iatOff := idtSize + iltSize

	// Fix Import Directory Table
	putU32(idata[0:], uint32(idataRVA)+getU32(idata[0:4]))   // OriginalFirstThunk
	putU32(idata[12:], uint32(idataRVA)+getU32(idata[12:16])) // Name
	putU32(idata[16:], uint32(idataRVA)+getU32(idata[16:20])) // FirstThunk

	// Fix ILT entries (8-byte)
	for i := 0; i < numImports; i++ {
		off := iltOff + i*8
		putU64(idata[off:], uint64(idataRVA)+getU64(idata[off:off+8]))
	}

	// Fix IAT entries (8-byte)
	for i := 0; i < numImports; i++ {
		off := iatOff + i*8
		putU64(idata[off:], uint64(idataRVA)+getU64(idata[off:off+8]))
	}
}

// buildIATOffsets64 returns func name → offset within .idata of the IAT entry (8-byte entries).
func (g *CodeGen) buildIATOffsets64(imports []string) map[string]int {
	idtSize := 40
	iltSize := (len(imports) + 1) * 8
	iatBaseOffset := idtSize + iltSize

	offsets := make(map[string]int)
	for i, name := range imports {
		offsets[name] = iatBaseOffset + i*8
	}
	return offsets
}

// getIATInfo64 returns the RVA and size of the IAT (8-byte entries).
func (g *CodeGen) getIATInfo64(imports []string, idataRVA int) (int, int) {
	idtSize := 40
	iltSize := (len(imports) + 1) * 8
	iatOffset := idtSize + iltSize
	iatSize := (len(imports) + 1) * 8
	return idataRVA + iatOffset, iatSize
}

// buildDWARF64 generates DWARF2 sections with 8-byte addresses for PE32+.
func (g *CodeGen) buildDWARF64(irmod *IRModule, textVA int, textSize int) ([]byte, []byte) {
	// === .debug_abbrev ===
	var abbrev []byte
	// Abbrev 1: compile_unit
	abbrev = append(abbrev, 1)    // abbrev number
	abbrev = append(abbrev, 0x11) // DW_TAG_compile_unit
	abbrev = append(abbrev, 1)    // DW_CHILDREN_yes
	abbrev = append(abbrev, 0x03) // DW_AT_name
	abbrev = append(abbrev, 0x08) // DW_FORM_string
	abbrev = append(abbrev, 0x11) // DW_AT_low_pc
	abbrev = append(abbrev, 0x01) // DW_FORM_addr
	abbrev = append(abbrev, 0x12) // DW_AT_high_pc
	abbrev = append(abbrev, 0x01) // DW_FORM_addr
	abbrev = append(abbrev, 0, 0) // end of attributes

	// Abbrev 2: subprogram
	abbrev = append(abbrev, 2)    // abbrev number
	abbrev = append(abbrev, 0x2e) // DW_TAG_subprogram
	abbrev = append(abbrev, 0)    // DW_CHILDREN_no
	abbrev = append(abbrev, 0x03) // DW_AT_name
	abbrev = append(abbrev, 0x08) // DW_FORM_string
	abbrev = append(abbrev, 0x11) // DW_AT_low_pc
	abbrev = append(abbrev, 0x01) // DW_FORM_addr
	abbrev = append(abbrev, 0x12) // DW_AT_high_pc
	abbrev = append(abbrev, 0x01) // DW_FORM_addr
	abbrev = append(abbrev, 0, 0) // end of attributes

	abbrev = append(abbrev, 0) // end of abbreviation table

	// === .debug_info ===
	var info []byte
	info = append(info, 0, 0, 0, 0) // unit_length (patched later)
	info = append(info, 2, 0)       // DWARF version 2
	info = append(info, 0, 0, 0, 0) // debug_abbrev_offset
	info = append(info, 8)          // address_size = 8 (64-bit)

	// DW_TAG_compile_unit
	info = append(info, 1) // abbrev 1
	info = append(info, []byte("rtg")...)
	info = append(info, 0)
	info = appendU64(info, uint64(textVA))
	info = appendU64(info, uint64(textVA+textSize))

	// _start
	startHighPC := textVA
	if len(irmod.Funcs) > 0 {
		startHighPC = textVA + g.funcOffsets[irmod.Funcs[0].Name]
	} else {
		startHighPC = textVA + textSize
	}
	info = append(info, 2) // abbrev 2
	info = append(info, []byte("_start")...)
	info = append(info, 0)
	info = appendU64(info, uint64(textVA))
	info = appendU64(info, uint64(startHighPC))

	// Functions
	i := 0
	for i < len(irmod.Funcs) {
		f := irmod.Funcs[i]
		funcStart := textVA + g.funcOffsets[f.Name]
		funcEnd := textVA + textSize
		if i+1 < len(irmod.Funcs) {
			funcEnd = textVA + g.funcOffsets[irmod.Funcs[i+1].Name]
		}

		info = append(info, 2)
		info = append(info, []byte(f.Name)...)
		info = append(info, 0)
		info = appendU64(info, uint64(funcStart))
		info = appendU64(info, uint64(funcEnd))
		i++
	}

	info = append(info, 0) // null terminator

	unitLen := len(info) - 4
	putU32(info[0:], uint32(unitLen))

	return abbrev, info
}

// appendU64 appends a little-endian uint64 to a byte slice.
func appendU64(b []byte, v uint64) []byte {
	b = append(b, byte(v))
	b = append(b, byte(v>>8))
	b = append(b, byte(v>>16))
	b = append(b, byte(v>>24))
	b = append(b, byte(v>>32))
	b = append(b, byte(v>>40))
	b = append(b, byte(v>>48))
	b = append(b, byte(v>>56))
	return b
}
