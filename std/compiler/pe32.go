//go:build !no_backend_windows_i386 || !no_backend_arm64 || !no_backend_windows_amd64

package main

// buildPE32 assembles a PE32 executable from the compiled code, rodata, data,
// and a list of kernel32.dll imports.
func (g *CodeGen) buildPE32(irmod *IRModule, imports []string) []byte {
	// PE32 Layout:
	// 0x000  DOS Header (64 bytes)
	// 0x040  DOS Stub (64 bytes)
	// 0x080  PE Signature (4 bytes)
	// 0x084  COFF Header (20 bytes)
	// 0x098  Optional Header (224 bytes)
	// 0x178  Section Table (6 sections x 40 bytes = 240 bytes)
	//        (pad to FileAlignment=0x200)
	// 0x200  .text
	//        .rdata
	//        .data
	//        .idata
	//        .debug_abbrev (DWARF abbreviations)
	//        .debug_info   (DWARF compilation unit + subprograms)

	const (
		fileAlignment    = 0x200
		sectionAlignment = 0x1000
		imageBase        = 0x400000
	)

	dosHeaderSize := 64
	dosStubSize := 64
	peSignatureSize := 4
	coffHeaderSize := 20
	optionalHeaderSize := 224
	numSections := 6
	sectionTableSize := numSections * 40

	headersRawSize := dosHeaderSize + dosStubSize + peSignatureSize + coffHeaderSize + optionalHeaderSize + sectionTableSize
	// Align headers to FileAlignment
	headersAligned := alignUp(headersRawSize, fileAlignment)

	// Section sizes (raw = file-aligned, virtual = section-aligned)
	textRawSize := alignUp(len(g.code), fileAlignment)
	rdataRawSize := alignUp(len(g.rodata), fileAlignment)
	dataRawSize := alignUp(len(g.data), fileAlignment)

	// Build .idata section
	idataContent := g.buildIData(imports)
	idataRawSize := alignUp(len(idataContent), fileAlignment)

	// RVAs (section-aligned)
	textRVA := sectionAlignment // 0x1000
	rdataRVA := textRVA + alignUp(len(g.code), sectionAlignment)
	dataRVA := rdataRVA + alignUp(len(g.rodata), sectionAlignment)
	idataRVA := dataRVA + alignUp(len(g.data), sectionAlignment)

	// Fix up .idata internal RVAs
	g.fixupIData(idataContent, idataRVA, imports)

	// Build DWARF debug sections
	textVA := imageBase + textRVA
	debugAbbrev, debugInfo := g.buildDWARF(irmod, textVA, len(g.code))
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

	// Build COFF symbol table and string table
	// We also add long section names (.debug_abbrev, .debug_info) to the string table
	coffSyms, coffStrtab, numSyms := g.buildCOFFSymbols(irmod)

	// Add long section names to string table and record their offsets
	debugAbbrevNameOff := len(coffStrtab)
	coffStrtab = append(coffStrtab, []byte(".debug_abbrev")...)
	coffStrtab = append(coffStrtab, 0)
	debugInfoNameOff := len(coffStrtab)
	coffStrtab = append(coffStrtab, []byte(".debug_info")...)
	coffStrtab = append(coffStrtab, 0)
	// Re-patch string table size
	putU32(coffStrtab[0:], uint32(len(coffStrtab)))

	symtabFileOff := debugInfoFileOff + debugInfoRawSize
	strtabFileOff := symtabFileOff + len(coffSyms)
	totalFileSize := strtabFileOff + len(coffStrtab)

	// Virtual size of image
	imageSize := debugInfoRVA + alignUp(len(debugInfo), sectionAlignment)

	// Fix up string headers in rodata
	for _, headerOff := range g.stringMap {
		dataOff := getU32(g.rodata[headerOff : headerOff+4])
		putU32(g.rodata[headerOff:headerOff+4], uint32(imageBase+rdataRVA)+dataOff)
	}

	// Fix up code references
	iatOffsets := g.buildIATOffsets(imports)
	for _, fix := range g.callFixups {
		if fix.Target == "$rodata_header$" {
			headerOff := getU32(g.code[fix.CodeOffset : fix.CodeOffset+4])
			putU32(g.code[fix.CodeOffset:fix.CodeOffset+4], uint32(imageBase+rdataRVA)+headerOff)
		} else if fix.Target == "$data_addr$" {
			dataOff := getU32(g.code[fix.CodeOffset : fix.CodeOffset+4])
			putU32(g.code[fix.CodeOffset:fix.CodeOffset+4], uint32(imageBase+dataRVA)+dataOff)
		} else if len(fix.Target) > 5 && fix.Target[0:5] == "$iat$" {
			funcName := fix.Target[5:]
			iatOff, ok := iatOffsets[funcName]
			if ok {
				putU32(g.code[fix.CodeOffset:fix.CodeOffset+4], uint32(imageBase+idataRVA)+uint32(iatOff))
			}
		}
	}

	// Assemble the file
	pe := make([]byte, totalFileSize)

	// === DOS Header (64 bytes) ===
	pe[0] = 'M'
	pe[1] = 'Z'
	// e_lfanew at offset 0x3C (point to PE signature)
	putU32(pe[0x3C:], 0x80)

	// === DOS Stub (64 bytes at 0x40) ===
	// "This program cannot be run in DOS mode" message
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
	putU16(coff[0:], 0x014C)                  // Machine: IMAGE_FILE_MACHINE_I386
	putU16(coff[2:], uint16(numSections))      // NumberOfSections
	putU32(coff[4:], 0)                        // TimeDateStamp
	putU32(coff[8:], uint32(symtabFileOff))    // PointerToSymbolTable
	putU32(coff[12:], uint32(numSyms))         // NumberOfSymbols
	putU16(coff[16:], uint16(optionalHeaderSize)) // SizeOfOptionalHeader
	putU16(coff[18:], 0x0103)                  // Characteristics: RELOCS_STRIPPED | EXECUTABLE_IMAGE | 32BIT_MACHINE

	// === Optional Header (224 bytes at 0x98) ===
	opt := pe[0x98:]
	putU16(opt[0:], 0x010B)                    // Magic: PE32
	opt[2] = 1                                 // MajorLinkerVersion
	opt[3] = 0                                 // MinorLinkerVersion
	putU32(opt[4:], uint32(len(g.code)))       // SizeOfCode
	putU32(opt[8:], uint32(len(g.rodata)))     // SizeOfInitializedData
	putU32(opt[12:], 0)                        // SizeOfUninitializedData
	putU32(opt[16:], uint32(textRVA))          // AddressOfEntryPoint
	putU32(opt[20:], uint32(textRVA))          // BaseOfCode
	putU32(opt[24:], uint32(rdataRVA))         // BaseOfData
	putU32(opt[28:], uint32(imageBase))        // ImageBase
	putU32(opt[32:], uint32(sectionAlignment)) // SectionAlignment
	putU32(opt[36:], uint32(fileAlignment))    // FileAlignment
	putU16(opt[40:], 4)                        // MajorOperatingSystemVersion
	putU16(opt[42:], 0)                        // MinorOperatingSystemVersion
	putU16(opt[44:], 0)                        // MajorImageVersion
	putU16(opt[46:], 0)                        // MinorImageVersion
	putU16(opt[48:], 4)                        // MajorSubsystemVersion (4 = Win95/NT4)
	putU16(opt[50:], 0)                        // MinorSubsystemVersion
	putU32(opt[52:], 0)                        // Win32VersionValue
	putU32(opt[56:], uint32(imageSize))        // SizeOfImage
	putU32(opt[60:], uint32(headersAligned))   // SizeOfHeaders
	putU32(opt[64:], 0)                        // CheckSum
	putU16(opt[68:], 3)                        // Subsystem: IMAGE_SUBSYSTEM_WINDOWS_CUI
	putU16(opt[70:], 0)                        // DllCharacteristics
	putU32(opt[72:], 0x100000)                 // SizeOfStackReserve (1MB)
	putU32(opt[76:], 0x1000)                   // SizeOfStackCommit (4KB)
	putU32(opt[80:], 0x100000)                 // SizeOfHeapReserve (1MB)
	putU32(opt[84:], 0x1000)                   // SizeOfHeapCommit (4KB)
	putU32(opt[88:], 0)                        // LoaderFlags
	putU32(opt[92:], 16)                       // NumberOfRvaAndSizes

	// Data directories (16 entries x 8 bytes = 128 bytes starting at opt[96])
	// [1] Import Table
	importDirRVA, importDirSize := g.getImportDirInfo(imports, idataRVA)
	putU32(opt[96+1*8:], uint32(importDirRVA))
	putU32(opt[96+1*8+4:], uint32(importDirSize))

	// [12] IAT
	iatRVA, iatSize := g.getIATInfo(imports, idataRVA)
	putU32(opt[96+12*8:], uint32(iatRVA))
	putU32(opt[96+12*8+4:], uint32(iatSize))

	// === Section Table (4 x 40 bytes at 0x178) ===
	sectBase := 0x178

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

	// .debug_abbrev — long name via COFF string table
	writeSectionLongName(pe[sectBase+160:], debugAbbrevNameOff,
		len(debugAbbrev), debugAbbrevRVA, debugAbbrevRawSize, debugAbbrevFileOff,
		0x42000040) // INITIALIZED_DATA | READ | DISCARDABLE

	// .debug_info — long name via COFF string table
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

// writeSection writes a 40-byte section header entry.
func writeSection(buf []byte, name string, virtualSize, rva, rawSize, fileOff int, characteristics uint32) {
	// Name (8 bytes, null-padded)
	i := 0
	for i < len(name) && i < 8 {
		buf[i] = name[i]
		i++
	}
	putU32(buf[8:], uint32(virtualSize))       // VirtualSize
	putU32(buf[12:], uint32(rva))              // VirtualAddress (RVA)
	putU32(buf[16:], uint32(rawSize))          // SizeOfRawData
	putU32(buf[20:], uint32(fileOff))          // PointerToRawData
	putU32(buf[24:], 0)                        // PointerToRelocations
	putU32(buf[28:], 0)                        // PointerToLinenumbers
	putU16(buf[32:], 0)                        // NumberOfRelocations
	putU16(buf[34:], 0)                        // NumberOfLinenumbers
	putU32(buf[36:], characteristics)          // Characteristics
}

// writeSectionLongName writes a section header with a long name referenced via
// the COFF string table. The name field is "/<decimal_offset>".
func writeSectionLongName(buf []byte, strtabOffset int, virtualSize, rva, rawSize, fileOff int, characteristics uint32) {
	// Format: "/<decimal_offset>" in 8 bytes
	s := formatSlashOffset(strtabOffset)
	i := 0
	for i < len(s) && i < 8 {
		buf[i] = s[i]
		i++
	}
	putU32(buf[8:], uint32(virtualSize))
	putU32(buf[12:], uint32(rva))
	putU32(buf[16:], uint32(rawSize))
	putU32(buf[20:], uint32(fileOff))
	putU32(buf[24:], 0)
	putU32(buf[28:], 0)
	putU16(buf[32:], 0)
	putU16(buf[34:], 0)
	putU32(buf[36:], characteristics)
}

// formatSlashOffset formats an integer as "/<decimal>" for PE long section names.
func formatSlashOffset(n int) []byte {
	if n == 0 {
		return []byte("/0")
	}
	// Build digits in reverse
	var digits []byte
	v := n
	for v > 0 {
		digits = append(digits, byte('0'+v%10))
		v = v / 10
	}
	result := []byte("/")
	i := len(digits) - 1
	for i >= 0 {
		result = append(result, digits[i])
		i = i - 1
	}
	return result
}

// buildIData builds the .idata section content for kernel32.dll imports.
// Layout: Import Directory Table | ILT | IAT | Hint/Name Table | DLL Name
func (g *CodeGen) buildIData(imports []string) []byte {
	numImports := len(imports)

	// Import Directory Table: 1 real entry + 1 null terminator = 40 bytes
	idtSize := 40

	// ILT: (numImports + 1) * 4 bytes (null-terminated)
	iltSize := (numImports + 1) * 4

	// IAT: identical to ILT
	iatSize := (numImports + 1) * 4

	// Hint/Name Table: for each import, 2 bytes hint + name + null + padding
	hntOffset := idtSize + iltSize + iatSize
	var hntEntries []byte
	var hntOffsets []int // offset within idata of each hint/name entry
	for _, name := range imports {
		hntOffsets = append(hntOffsets, hntOffset+len(hntEntries))
		hntEntries = append(hntEntries, 0, 0) // Hint = 0
		hntEntries = append(hntEntries, []byte(name)...)
		hntEntries = append(hntEntries, 0) // null terminator
		// Pad to 2-byte alignment
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
	iltRVAOffset := idtSize         // ILT follows IDT
	iatRVAOffset := idtSize + iltSize // IAT follows ILT
	// These will be fixed up as RVAs relative to idata section start
	// The caller must add idataRVA to get the actual RVA
	putU32(idata[0:], uint32(iltRVAOffset))   // OriginalFirstThunk (RVA of ILT) — placeholder, needs idataRVA added
	putU32(idata[4:], 0)                       // TimeDateStamp
	putU32(idata[8:], 0)                       // ForwarderChain
	putU32(idata[12:], uint32(dllNameOffset)) // Name (RVA of DLL name) — placeholder
	putU32(idata[16:], uint32(iatRVAOffset))  // FirstThunk (RVA of IAT) — placeholder
	// Entry 2 (null terminator): already zero

	// ILT entries
	for i := 0; i < numImports; i++ {
		off := iltRVAOffset + i*4
		putU32(idata[off:], uint32(hntOffsets[i])) // RVA of Hint/Name — placeholder
	}
	// Null terminator already zero

	// IAT entries (identical to ILT on disk)
	for i := 0; i < numImports; i++ {
		off := iatRVAOffset + i*4
		putU32(idata[off:], uint32(hntOffsets[i])) // RVA of Hint/Name — placeholder
	}

	// Hint/Name Table
	copy(idata[hntOffset:], hntEntries)

	// DLL name
	copy(idata[dllNameOffset:], dllName)

	return idata
}

// fixupIData adjusts all RVA fields in the .idata content to be actual RVAs.
func (g *CodeGen) fixupIData(idata []byte, idataRVA int, imports []string) {
	numImports := len(imports)
	idtSize := 40
	iltSize := (numImports + 1) * 4
	iltOff := idtSize
	iatOff := idtSize + iltSize

	// Fix Import Directory Table
	putU32(idata[0:], uint32(idataRVA)+getU32(idata[0:4]))   // OriginalFirstThunk
	putU32(idata[12:], uint32(idataRVA)+getU32(idata[12:16])) // Name
	putU32(idata[16:], uint32(idataRVA)+getU32(idata[16:20])) // FirstThunk

	// Fix ILT entries
	for i := 0; i < numImports; i++ {
		off := iltOff + i*4
		putU32(idata[off:], uint32(idataRVA)+getU32(idata[off:off+4]))
	}

	// Fix IAT entries
	for i := 0; i < numImports; i++ {
		off := iatOff + i*4
		putU32(idata[off:], uint32(idataRVA)+getU32(idata[off:off+4]))
	}
}

// buildIATOffsets returns a map of function name → offset within .idata of that function's IAT entry.
func (g *CodeGen) buildIATOffsets(imports []string) map[string]int {
	idtSize := 40
	iltSize := (len(imports) + 1) * 4
	iatBaseOffset := idtSize + iltSize

	offsets := make(map[string]int)
	for i, name := range imports {
		offsets[name] = iatBaseOffset + i*4
	}
	return offsets
}

// getImportDirInfo returns the RVA and size of the Import Directory Table.
func (g *CodeGen) getImportDirInfo(imports []string, idataRVA int) (int, int) {
	return idataRVA, 40 // 1 entry + null terminator = 40 bytes
}

// getIATInfo returns the RVA and size of the IAT.
func (g *CodeGen) getIATInfo(imports []string, idataRVA int) (int, int) {
	idtSize := 40
	iltSize := (len(imports) + 1) * 4
	iatOffset := idtSize + iltSize
	iatSize := (len(imports) + 1) * 4
	return idataRVA + iatOffset, iatSize
}

// makeCOFFSym creates an 18-byte COFF symbol entry.
func makeCOFFSym(name []byte, value uint32, section uint16, symType uint16, storageClass byte) []byte {
	sym := make([]byte, 18)
	if len(name) <= 8 {
		i := 0
		for i < len(name) && i < 8 {
			sym[i] = name[i]
			i++
		}
	} else {
		// Long name marker: first 4 bytes zero, next 4 = strtab offset
		// Caller must set bytes 4..7 to the strtab offset after calling this
		putU32(sym[0:], 0)
		putU32(sym[4:], 0) // placeholder
	}
	putU32(sym[8:], value)
	putU16(sym[12:], section)
	putU16(sym[14:], symType)
	sym[16] = storageClass
	sym[17] = 0
	return sym
}

// buildCOFFSymbols creates the COFF symbol table and string table.
func (g *CodeGen) buildCOFFSymbols(irmod *IRModule) ([]byte, []byte, int) {
	var coffSyms []byte
	var coffStrtab []byte
	coffStrtab = append(coffStrtab, 0, 0, 0, 0) // placeholder for string table size
	numSyms := 0

	// Add _start symbol
	sym := makeCOFFSym([]byte("_start"), 0, 1, 0x20, 2)
	coffSyms = append(coffSyms, sym...)
	numSyms++

	// Add function symbols
	i := 0
	for i < len(irmod.Funcs) {
		f := irmod.Funcs[i]
		funcOff := g.funcOffsets[f.Name]
		nameBytes := []byte(f.Name)
		sym = makeCOFFSym(nameBytes, uint32(funcOff), 1, 0x20, 2)
		if len(nameBytes) > 8 {
			// Patch long name offset
			putU32(sym[4:], uint32(len(coffStrtab)))
			coffStrtab = append(coffStrtab, nameBytes...)
			coffStrtab = append(coffStrtab, 0)
		}
		coffSyms = append(coffSyms, sym...)
		numSyms++
		i++
	}

	// Patch string table size
	putU32(coffStrtab[0:], uint32(len(coffStrtab)))

	return coffSyms, coffStrtab, numSyms
}

// buildDWARF generates minimal DWARF2 .debug_abbrev and .debug_info sections
// so that WineDbg can resolve function names in backtraces.
func (g *CodeGen) buildDWARF(irmod *IRModule, textVA int, textSize int) ([]byte, []byte) {
	// === .debug_abbrev ===
	// Abbrev 1: DW_TAG_compile_unit, has children
	//   DW_AT_name (string), DW_AT_low_pc (addr), DW_AT_high_pc (addr)
	// Abbrev 2: DW_TAG_subprogram, no children
	//   DW_AT_name (string), DW_AT_low_pc (addr), DW_AT_high_pc (addr)
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
	// Compilation unit header (DWARF2, 32-bit):
	//   unit_length: 4 bytes
	//   version: 2 bytes (= 2)
	//   debug_abbrev_offset: 4 bytes (= 0)
	//   address_size: 1 byte (= 4)
	// Total header: 11 bytes

	var info []byte
	// Reserve 4 bytes for unit_length (patched at end)
	info = append(info, 0, 0, 0, 0)
	// Version
	info = append(info, 2, 0) // DWARF version 2 (little-endian)
	// debug_abbrev_offset
	info = append(info, 0, 0, 0, 0) // offset 0 into .debug_abbrev
	// address_size
	info = append(info, 4) // 32-bit addresses

	// DW_TAG_compile_unit (abbrev 1)
	info = append(info, 1) // abbrev number 1
	// DW_AT_name: inline string
	info = append(info, []byte("rtg")...)
	info = append(info, 0)
	// DW_AT_low_pc
	info = appendU32(info, uint32(textVA))
	// DW_AT_high_pc
	info = appendU32(info, uint32(textVA+textSize))

	// Add _start as subprogram
	startHighPC := textVA
	if len(irmod.Funcs) > 0 {
		startHighPC = textVA + g.funcOffsets[irmod.Funcs[0].Name]
	} else {
		startHighPC = textVA + textSize
	}
	info = append(info, 2) // abbrev number 2
	info = append(info, []byte("_start")...)
	info = append(info, 0)
	info = appendU32(info, uint32(textVA))
	info = appendU32(info, uint32(startHighPC))

	// Add each function as DW_TAG_subprogram
	i := 0
	for i < len(irmod.Funcs) {
		f := irmod.Funcs[i]
		funcStart := textVA + g.funcOffsets[f.Name]
		funcEnd := textVA + textSize
		if i+1 < len(irmod.Funcs) {
			funcEnd = textVA + g.funcOffsets[irmod.Funcs[i+1].Name]
		}

		info = append(info, 2) // abbrev number 2
		info = append(info, []byte(f.Name)...)
		info = append(info, 0)
		info = appendU32(info, uint32(funcStart))
		info = appendU32(info, uint32(funcEnd))
		i++
	}

	// Null terminator (end of compile_unit children)
	info = append(info, 0)

	// Patch unit_length (total size minus the 4-byte length field itself)
	unitLen := len(info) - 4
	putU32(info[0:], uint32(unitLen))

	return abbrev, info
}

// appendU32 appends a little-endian uint32 to a byte slice.
func appendU32(b []byte, v uint32) []byte {
	b = append(b, byte(v))
	b = append(b, byte(v>>8))
	b = append(b, byte(v>>16))
	b = append(b, byte(v>>24))
	return b
}

