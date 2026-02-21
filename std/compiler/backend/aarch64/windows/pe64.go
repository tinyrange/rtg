//go:build !no_backend_arm64 || !no_backend_windows_amd64

package windows

import (
	"j5.nz/rtg/std/compiler/backend/aarch64"
	"j5.nz/rtg/std/compiler/ir"
)

// writeSection writes a 40-byte section header entry.
func writeSection(buf []byte, name string, virtualSize, rva, rawSize, fileOff int, characteristics uint32) {
	// Name (8 bytes, null-padded)
	i := 0
	for i < len(name) && i < 8 {
		buf[i] = name[i]
		i++
	}
	aarch64.PutU32(buf[8:], uint32(virtualSize)) // VirtualSize
	aarch64.PutU32(buf[12:], uint32(rva))        // VirtualAddress (RVA)
	aarch64.PutU32(buf[16:], uint32(rawSize))    // SizeOfRawData
	aarch64.PutU32(buf[20:], uint32(fileOff))    // PointerToRawData
	aarch64.PutU32(buf[24:], 0)                  // PointerToRelocations
	aarch64.PutU32(buf[28:], 0)                  // PointerToLinenumbers
	aarch64.PutU16(buf[32:], 0)                  // NumberOfRelocations
	aarch64.PutU16(buf[34:], 0)                  // NumberOfLinenumbers
	aarch64.PutU32(buf[36:], characteristics)    // Characteristics
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
	aarch64.PutU32(buf[8:], uint32(virtualSize))
	aarch64.PutU32(buf[12:], uint32(rva))
	aarch64.PutU32(buf[16:], uint32(rawSize))
	aarch64.PutU32(buf[20:], uint32(fileOff))
	aarch64.PutU32(buf[24:], 0)
	aarch64.PutU32(buf[28:], 0)
	aarch64.PutU16(buf[32:], 0)
	aarch64.PutU16(buf[34:], 0)
	aarch64.PutU32(buf[36:], characteristics)
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

// getImportDirInfo returns the RVA and size of the Import Directory Table.
func getImportDirInfo(g *aarch64.CodeGen, imports []string, idataRVA int) (int, int) {
	return idataRVA, 40 // 1 entry + null terminator = 40 bytes
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
		aarch64.PutU32(sym[0:], 0)
		aarch64.PutU32(sym[4:], 0) // placeholder
	}
	aarch64.PutU32(sym[8:], value)
	aarch64.PutU16(sym[12:], section)
	aarch64.PutU16(sym[14:], symType)
	sym[16] = storageClass
	sym[17] = 0
	return sym
}

// buildCOFFSymbols creates the COFF symbol table and string table.
func buildCOFFSymbols(g *aarch64.CodeGen, irmod *ir.IRModule) ([]byte, []byte, int) {
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
		funcOff, _ := g.LookupFuncOffset(f.Name)
		nameBytes := []byte(f.Name)
		sym = makeCOFFSym(nameBytes, uint32(funcOff), 1, 0x20, 2)
		if len(nameBytes) > 8 {
			// Patch long name offset
			aarch64.PutU32(sym[4:], uint32(len(coffStrtab)))
			coffStrtab = append(coffStrtab, nameBytes...)
			coffStrtab = append(coffStrtab, 0)
		}
		coffSyms = append(coffSyms, sym...)
		numSyms++
		i++
	}

	// Patch string table size
	aarch64.PutU32(coffStrtab[0:], uint32(len(coffStrtab)))

	return coffSyms, coffStrtab, numSyms
}

// buildPE64 assembles a PE32+ (64-bit) executable from the compiled code, rodata, data,
// and a list of kernel32.dll imports. Used for windows/arm64.
func BuildPE64(g *aarch64.CodeGen, irmod *ir.IRModule, imports []string) []byte {
	// PE32+ Layout:
	// 0x000  DOS Header (64 bytes)
	// 0x040  DOS Stub (64 bytes)
	// 0x080  PE Signature (4 bytes)
	// 0x084  COFF Header (20 bytes)
	// 0x098  Optional Header (240 bytes)
	// 0x188  Section Table (6 or 7 sections x 40 bytes)
	//        (pad to FileAlignment=0x200)
	// 0x200  .text / .rdata / .data / .idata / [.reloc] / .debug_abbrev / .debug_info

	const (
		fileAlignment    = 0x200
		sectionAlignment = 0x1000
	)
	imageBase := g.BaseAddr()
	hasReloc := g.IsArm64()

	dosHeaderSize := 64
	dosStubSize := 64
	peSignatureSize := 4
	coffHeaderSize := 20
	optionalHeaderSize := 240
	numSections := 6
	if hasReloc {
		numSections = 7
	}
	if g.Target().StripBinary {
		numSections = numSections - 2 // strip .debug_abbrev and .debug_info
	}
	sectionTableSize := numSections * 40

	headersRawSize := dosHeaderSize + dosStubSize + peSignatureSize + coffHeaderSize + optionalHeaderSize + sectionTableSize
	headersAligned := aarch64.AlignUp(headersRawSize, fileAlignment)

	// Ensure empty initialized-data sections still emit a minimal payload.
	// Some Windows loaders reject images with zero-sized initialized sections.
	rdataContent := g.Rodata()
	if len(rdataContent) == 0 {
		rdataContent = []byte{0}
	}
	dataContent := g.Data()
	if len(dataContent) == 0 {
		dataContent = []byte{0}
	}

	// Section sizes
	textRawSize := aarch64.AlignUp(len(g.Code()), fileAlignment)
	rdataRawSize := aarch64.AlignUp(len(rdataContent), fileAlignment)
	dataRawSize := aarch64.AlignUp(len(dataContent), fileAlignment)

	// Build .idata section with 8-byte ILT/IAT entries
	idataContent := buildIData64(g, imports)
	idataRawSize := aarch64.AlignUp(len(idataContent), fileAlignment)

	// RVAs
	textRVA := sectionAlignment // 0x1000
	rdataRVA := textRVA + aarch64.SectionSpan(len(g.Code()), sectionAlignment)
	dataRVA := rdataRVA + aarch64.SectionSpan(len(rdataContent), sectionAlignment)
	idataRVA := dataRVA + aarch64.SectionSpan(len(dataContent), sectionAlignment)

	// Fix up .idata internal RVAs
	fixupIData64(g, idataContent, idataRVA, imports)

	// Build .reloc section for ARM64 (required for ASLR).
	var relocContent []byte
	relocRVA := 0
	relocRawSize := 0
	if hasReloc {
		// Only relocate known absolute addresses emitted into .data.
		// String headers are the only intentional absolute-pointer payloads today.
		relocOffsets := make([]int, 0, len(g.StringHeaderOffsets()))
		for _, headerOff := range g.StringHeaderOffsets() {
			relocOffsets = append(relocOffsets, headerOff)
		}
		sortRelocOffsets(relocOffsets)
		relocContent = buildBaseRelocations(g, dataRVA, relocOffsets)
		relocRVA = idataRVA + aarch64.SectionSpan(len(idataContent), sectionAlignment)
		relocRawSize = aarch64.AlignUp(len(relocContent), fileAlignment)
	}

	// Build DWARF debug sections with 8-byte addresses
	textVA := imageBase + uint64(textRVA)
	textVAInt := int(textVA)
	debugAbbrev := []byte{}
	debugInfo := []byte{}
	debugAbbrevRawSize := 0
	debugInfoRawSize := 0
	if !g.Target().StripBinary {
		debugAbbrev, debugInfo = buildDWARF64(g, irmod, textVAInt, len(g.Code()))
		debugAbbrevRawSize = aarch64.AlignUp(len(debugAbbrev), fileAlignment)
		debugInfoRawSize = aarch64.AlignUp(len(debugInfo), fileAlignment)
	}

	debugAbbrevRVA := idataRVA + aarch64.SectionSpan(len(idataContent), sectionAlignment)
	if hasReloc {
		debugAbbrevRVA = relocRVA + aarch64.SectionSpan(len(relocContent), sectionAlignment)
	}
	debugInfoRVA := debugAbbrevRVA + aarch64.SectionSpan(len(debugAbbrev), sectionAlignment)

	// File offsets
	textFileOff := headersAligned
	rdataFileOff := textFileOff + textRawSize
	dataFileOff := rdataFileOff + rdataRawSize
	idataFileOff := dataFileOff + dataRawSize
	relocFileOff := idataFileOff + idataRawSize
	debugAbbrevFileOff := idataFileOff + idataRawSize
	if hasReloc {
		debugAbbrevFileOff = relocFileOff + relocRawSize
	}
	debugInfoFileOff := debugAbbrevFileOff + debugAbbrevRawSize

	// COFF symbols
	coffSyms := []byte{}
	coffStrtab := []byte{}
	numSyms := 0
	if !g.Target().StripBinary {
		coffSyms, coffStrtab, numSyms = buildCOFFSymbols(g, irmod)
	}

	debugAbbrevNameOff := 0
	debugInfoNameOff := 0
	if !g.Target().StripBinary {
		debugAbbrevNameOff = len(coffStrtab)
		coffStrtab = append(coffStrtab, []byte(".debug_abbrev")...)
		coffStrtab = append(coffStrtab, 0)
		debugInfoNameOff = len(coffStrtab)
		coffStrtab = append(coffStrtab, []byte(".debug_info")...)
		coffStrtab = append(coffStrtab, 0)
		aarch64.PutU32(coffStrtab[0:], uint32(len(coffStrtab)))
	}

	symtabFileOff := debugInfoFileOff + debugInfoRawSize
	strtabFileOff := symtabFileOff + len(coffSyms)
	totalFileSize := strtabFileOff + len(coffStrtab)
	if g.Target().StripBinary {
		if hasReloc {
			totalFileSize = relocFileOff + relocRawSize
		} else {
			totalFileSize = idataFileOff + idataRawSize
		}
	}

	imageSize := debugInfoRVA + aarch64.SectionSpan(len(debugInfo), sectionAlignment)
	if g.Target().StripBinary {
		if hasReloc {
			imageSize = relocRVA + aarch64.SectionSpan(len(relocContent), sectionAlignment)
		} else {
			imageSize = idataRVA + aarch64.SectionSpan(len(idataContent), sectionAlignment)
		}
	}

	// Fix up string headers and code references
	iatOffsets := buildIATOffsets64(g, imports)
	if g.IsArm64() {
		// ARM64: string headers are in .data section
		for _, headerOff := range g.StringHeaderOffsets() {
			rodataOff := g.StringRodataMap()[headerOff]
			aarch64.PutU64(g.Data()[headerOff:headerOff+8], imageBase+uint64(rdataRVA+rodataOff))
		}

		// Fix up code references (ADRP+ADD/LDR pairs)
		for i := 0; i < g.CallFixupCount(); i++ {
			codeOffset, targetName, value := g.CallFixupAt(i)
			if targetName == "$rodata_header$" {
				pcAddr := imageBase + uint64(textRVA+codeOffset)
				targetAddr := imageBase + uint64(rdataRVA) + value
				g.PatchAdrpAddOrLdr(codeOffset, pcAddr, targetAddr)
			} else if targetName == "$data_addr$" {
				pcAddr := imageBase + uint64(textRVA+codeOffset)
				targetAddr := imageBase + uint64(dataRVA) + value
				g.PatchAdrpAddOrLdr(codeOffset, pcAddr, targetAddr)
			} else if len(targetName) > 5 && targetName[0:5] == "$iat$" {
				funcName := targetName[5:]
				iatOff, ok := iatOffsets[funcName]
				if ok {
					pcAddr := imageBase + uint64(textRVA+codeOffset)
					targetAddr := imageBase + uint64(idataRVA) + uint64(iatOff)
					g.PatchAdrpLdr(codeOffset, pcAddr, targetAddr)
				}
			}
		}
	} else {
		// x64: string headers are in .rodata section
		for _, headerOff := range g.StringHeaderOffsets() {
			dataOff := aarch64.GetU64(g.Rodata()[headerOff : headerOff+8])
			aarch64.PutU64(g.Rodata()[headerOff:headerOff+8], imageBase+uint64(rdataRVA)+dataOff)
		}

		// Fix up code references (movabs imm64 and RIP-relative call)
		for i := 0; i < g.CallFixupCount(); i++ {
			codeOffset, targetName, _ := g.CallFixupAt(i)
			if targetName == "$rodata_header$" {
				// Patch 8-byte movabs immediate with rodata VA
				headerOff := aarch64.GetU64(g.Code()[codeOffset : codeOffset+8])
				aarch64.PutU64(g.Code()[codeOffset:codeOffset+8], imageBase+uint64(rdataRVA)+headerOff)
			} else if targetName == "$data_addr$" {
				// Patch 8-byte movabs immediate with data VA
				dataOff := aarch64.GetU64(g.Code()[codeOffset : codeOffset+8])
				aarch64.PutU64(g.Code()[codeOffset:codeOffset+8], imageBase+uint64(dataRVA)+dataOff)
			} else if len(targetName) > 5 && targetName[0:5] == "$iat$" {
				funcName := targetName[5:]
				iatOff, ok := iatOffsets[funcName]
				if ok {
					// Patch RIP-relative disp32: target = iatVA, rip = textVA + codeOffset + 4
					iatVA := imageBase + uint64(idataRVA) + uint64(iatOff)
					rip := imageBase + uint64(textRVA) + uint64(codeOffset) + 4
					disp32 := int32(int64(iatVA) - int64(rip))
					aarch64.PutU32(g.Code()[codeOffset:codeOffset+4], uint32(disp32))
				}
			}
		}
	}

	// Assemble the file
	pe := make([]byte, totalFileSize)

	// === DOS Header (64 bytes) ===
	pe[0] = 'M'
	pe[1] = 'Z'
	aarch64.PutU32(pe[0x3C:], 0x80)

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
	if g.IsArm64() {
		machineType = 0xAA64 // IMAGE_FILE_MACHINE_ARM64
	}
	aarch64.PutU16(coff[0:], machineType)         // Machine
	aarch64.PutU16(coff[2:], uint16(numSections)) // NumberOfSections
	aarch64.PutU32(coff[4:], 0)                   // TimeDateStamp
	if g.Target().StripBinary {
		aarch64.PutU32(coff[8:], 0) // PointerToSymbolTable
		aarch64.PutU32(coff[12:], 0)
	} else {
		aarch64.PutU32(coff[8:], uint32(symtabFileOff)) // PointerToSymbolTable
		aarch64.PutU32(coff[12:], uint32(numSyms))      // NumberOfSymbols
	}
	aarch64.PutU16(coff[16:], uint16(optionalHeaderSize)) // SizeOfOptionalHeader
	aarch64.PutU16(coff[18:], 0x0022)                     // Characteristics: EXECUTABLE_IMAGE | LARGE_ADDRESS_AWARE

	// === Optional Header (240 bytes at 0x98) ===
	opt := pe[0x98:]
	aarch64.PutU16(opt[0:], 0x020B)                                                       // Magic: PE32+
	opt[2] = 1                                                                            // MajorLinkerVersion
	opt[3] = 0                                                                            // MinorLinkerVersion
	aarch64.PutU32(opt[4:], uint32(len(g.Code())))                                        // SizeOfCode
	aarch64.PutU32(opt[8:], uint32(len(rdataContent)+len(dataContent)+len(idataContent))) // SizeOfInitializedData
	aarch64.PutU32(opt[12:], 0)                                                           // SizeOfUninitializedData
	aarch64.PutU32(opt[16:], uint32(textRVA))                                             // AddressOfEntryPoint
	aarch64.PutU32(opt[20:], uint32(textRVA))                                             // BaseOfCode
	// PE32+ has NO BaseOfData field — ImageBase is at offset 24
	aarch64.PutU64(opt[24:], imageBase)               // ImageBase (8 bytes)
	aarch64.PutU32(opt[32:], uint32(sectionAlignment)) // SectionAlignment
	aarch64.PutU32(opt[36:], uint32(fileAlignment))    // FileAlignment
	aarch64.PutU16(opt[40:], 6)                        // MajorOperatingSystemVersion
	aarch64.PutU16(opt[42:], 0)                        // MinorOperatingSystemVersion
	aarch64.PutU16(opt[44:], 0)                        // MajorImageVersion
	aarch64.PutU16(opt[46:], 0)                        // MinorImageVersion
	aarch64.PutU16(opt[48:], 6)                        // MajorSubsystemVersion
	aarch64.PutU16(opt[50:], 0)                        // MinorSubsystemVersion
	aarch64.PutU32(opt[52:], 0)                        // Win32VersionValue
	aarch64.PutU32(opt[56:], uint32(imageSize))        // SizeOfImage
	aarch64.PutU32(opt[60:], uint32(headersAligned))   // SizeOfHeaders
	aarch64.PutU32(opt[64:], 0)                        // CheckSum
	aarch64.PutU16(opt[68:], 3)                        // Subsystem: IMAGE_SUBSYSTEM_WINDOWS_CUI
	dllChars := uint16(0x0100) // NX_COMPAT
	if hasReloc {
		dllChars = 0x0140 // DYNAMIC_BASE | NX_COMPAT
	}
	aarch64.PutU16(opt[70:], dllChars) // DllCharacteristics
	// PE32+: Stack/Heap sizes are 8 bytes each
	aarch64.PutU64(opt[72:], 0x100000) // SizeOfStackReserve (1MB)
	aarch64.PutU64(opt[80:], 0x1000)   // SizeOfStackCommit (4KB)
	aarch64.PutU64(opt[88:], 0x100000) // SizeOfHeapReserve (1MB)
	aarch64.PutU64(opt[96:], 0x1000)   // SizeOfHeapCommit (4KB)
	aarch64.PutU32(opt[104:], 0)       // LoaderFlags
	aarch64.PutU32(opt[108:], 16)      // NumberOfRvaAndSizes

	// Data directories (16 entries x 8 bytes = 128 bytes starting at opt[112])
	// [1] Import Table
	importDirRVA, importDirSize := getImportDirInfo(g, imports, idataRVA)
	aarch64.PutU32(opt[112+1*8:], uint32(importDirRVA))
	aarch64.PutU32(opt[112+1*8+4:], uint32(importDirSize))

	// [5] Base Relocation Table
	if hasReloc {
		aarch64.PutU32(opt[112+5*8:], uint32(relocRVA))
		aarch64.PutU32(opt[112+5*8+4:], uint32(len(relocContent)))
	}

	// [12] IAT
	iatRVA, iatSize := getIATInfo64(g, imports, idataRVA)
	aarch64.PutU32(opt[112+12*8:], uint32(iatRVA))
	aarch64.PutU32(opt[112+12*8+4:], uint32(iatSize))

	// === Section Table (at 0x188) ===
	sectBase := 0x188

	// .text
	writeSection(pe[sectBase:], ".text",
		len(g.Code()), textRVA, textRawSize, textFileOff,
		0x60000020) // CODE | EXECUTE | READ

	// .rdata
	writeSection(pe[sectBase+40:], ".rdata",
		len(rdataContent), rdataRVA, rdataRawSize, rdataFileOff,
		0x40000040) // INITIALIZED_DATA | READ

	// .data
	writeSection(pe[sectBase+80:], ".data",
		len(dataContent), dataRVA, dataRawSize, dataFileOff,
		0xC0000040) // INITIALIZED_DATA | READ | WRITE

	// .idata
	writeSection(pe[sectBase+120:], ".idata",
		len(idataContent), idataRVA, idataRawSize, idataFileOff,
		0xC0000040) // INITIALIZED_DATA | READ | WRITE

	nextSect := sectBase + 160
	if hasReloc {
		// .reloc
		writeSection(pe[nextSect:], ".reloc",
			len(relocContent), relocRVA, relocRawSize, relocFileOff,
			0x42000040) // INITIALIZED_DATA | DISCARDABLE | READ
		nextSect += 40
	}

	if !g.Target().StripBinary {
		// .debug_abbrev
		writeSectionLongName(pe[nextSect:], debugAbbrevNameOff,
			len(debugAbbrev), debugAbbrevRVA, debugAbbrevRawSize, debugAbbrevFileOff,
			0x42000040) // INITIALIZED_DATA | READ | DISCARDABLE

		// .debug_info
		writeSectionLongName(pe[nextSect+40:], debugInfoNameOff,
			len(debugInfo), debugInfoRVA, debugInfoRawSize, debugInfoFileOff,
			0x42000040) // INITIALIZED_DATA | READ | DISCARDABLE
	}

	// Copy section data
	copy(pe[textFileOff:], g.Code())
	copy(pe[rdataFileOff:], rdataContent)
	copy(pe[dataFileOff:], dataContent)
	copy(pe[idataFileOff:], idataContent)
	if hasReloc {
		copy(pe[relocFileOff:], relocContent)
	}
	if !g.Target().StripBinary {
		copy(pe[debugAbbrevFileOff:], debugAbbrev)
		copy(pe[debugInfoFileOff:], debugInfo)
	}

	// Copy COFF symbol table and string table
	if !g.Target().StripBinary {
		copy(pe[symtabFileOff:], coffSyms)
		copy(pe[strtabFileOff:], coffStrtab)
	}

	return pe
}

// buildIData64 builds the .idata section with 8-byte ILT/IAT entries for PE32+.
func buildIData64(g *aarch64.CodeGen, imports []string) []byte {
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

	aarch64.PutU32(idata[0:], uint32(iltRVAOffset))   // OriginalFirstThunk — placeholder
	aarch64.PutU32(idata[4:], 0)                      // TimeDateStamp
	aarch64.PutU32(idata[8:], 0)                      // ForwarderChain
	aarch64.PutU32(idata[12:], uint32(dllNameOffset)) // Name — placeholder
	aarch64.PutU32(idata[16:], uint32(iatRVAOffset))  // FirstThunk — placeholder

	// ILT entries (8 bytes each for PE32+)
	for i := 0; i < numImports; i++ {
		off := iltRVAOffset + i*8
		aarch64.PutU64(idata[off:], uint64(hntOffsets[i])) // RVA of Hint/Name — placeholder
	}

	// IAT entries (8 bytes each, identical to ILT on disk)
	for i := 0; i < numImports; i++ {
		off := iatRVAOffset + i*8
		aarch64.PutU64(idata[off:], uint64(hntOffsets[i])) // RVA of Hint/Name — placeholder
	}

	// Hint/Name Table
	copy(idata[hntOffset:], hntEntries)

	// DLL name
	copy(idata[dllNameOffset:], dllName)

	return idata
}

// fixupIData64 adjusts RVA fields in the .idata content to be actual RVAs.
func fixupIData64(g *aarch64.CodeGen, idata []byte, idataRVA int, imports []string) {
	numImports := len(imports)
	idtSize := 40
	iltSize := (numImports + 1) * 8
	iltOff := idtSize
	iatOff := idtSize + iltSize

	// Fix Import Directory Table
	aarch64.PutU32(idata[0:], uint32(idataRVA)+aarch64.GetU32(idata[0:4]))    // OriginalFirstThunk
	aarch64.PutU32(idata[12:], uint32(idataRVA)+aarch64.GetU32(idata[12:16])) // Name
	aarch64.PutU32(idata[16:], uint32(idataRVA)+aarch64.GetU32(idata[16:20])) // FirstThunk

	// Fix ILT entries (8-byte)
	for i := 0; i < numImports; i++ {
		off := iltOff + i*8
		aarch64.PutU64(idata[off:], uint64(idataRVA)+aarch64.GetU64(idata[off:off+8]))
	}

	// Fix IAT entries (8-byte)
	for i := 0; i < numImports; i++ {
		off := iatOff + i*8
		aarch64.PutU64(idata[off:], uint64(idataRVA)+aarch64.GetU64(idata[off:off+8]))
	}
}

// buildIATOffsets64 returns func name → offset within .idata of the IAT entry (8-byte entries).
func buildIATOffsets64(g *aarch64.CodeGen, imports []string) map[string]int {
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
func getIATInfo64(g *aarch64.CodeGen, imports []string, idataRVA int) (int, int) {
	idtSize := 40
	iltSize := (len(imports) + 1) * 8
	iatOffset := idtSize + iltSize
	iatSize := (len(imports) + 1) * 8
	return idataRVA + iatOffset, iatSize
}

// buildDWARF64 generates DWARF2 sections with 8-byte addresses for PE32+.
func buildDWARF64(g *aarch64.CodeGen, irmod *ir.IRModule, textVA int, textSize int) ([]byte, []byte) {
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
		if v, ok := g.LookupFuncOffset(irmod.Funcs[0].Name); ok {
			startHighPC = textVA + v
		}
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
		funcStartOff, _ := g.LookupFuncOffset(f.Name)
		funcStart := textVA + funcStartOff
		funcEnd := textVA + textSize
		if i+1 < len(irmod.Funcs) {
			if nextOff, ok := g.LookupFuncOffset(irmod.Funcs[i+1].Name); ok {
				funcEnd = textVA + nextOff
			}
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
	aarch64.PutU32(info[0:], uint32(unitLen))

	return abbrev, info
}

func sortRelocOffsets(offsets []int) {
	// Insertion sort for deterministic output (critical for self-hosting)
	i := 1
	for i < len(offsets) {
		j := i
		for j > 0 && offsets[j] < offsets[j-1] {
			tmp := offsets[j]
			offsets[j] = offsets[j-1]
			offsets[j-1] = tmp
			j = j - 1
		}
		i = i + 1
	}
}

// buildBaseRelocations builds a .reloc section for 64-bit PE base relocations.
// sectionRVA is the RVA of the section containing the addresses to relocate.
// offsets are sorted offsets within that section of 8-byte absolute addresses.
func buildBaseRelocations(g *aarch64.CodeGen, sectionRVA int, offsets []int) []byte {
	if len(offsets) == 0 {
		return nil
	}

	var reloc []byte

	// Group relocations by 4KB page
	i := 0
	for i < len(offsets) {
		rva := sectionRVA + offsets[i]
		pageRVA := (rva / 0x1000) * 0x1000

		// Reserve space for block header
		blockStart := len(reloc)
		reloc = append(reloc, 0, 0, 0, 0) // PageRVA placeholder
		reloc = append(reloc, 0, 0, 0, 0) // BlockSize placeholder

		// Add entries for all relocations in this page
		for i < len(offsets) {
			rva = sectionRVA + offsets[i]
			if (rva/0x1000)*0x1000 != pageRVA {
				break
			}
			offsetInPage := rva % 0x1000
			entry := uint16(0xA000) | uint16(offsetInPage) // IMAGE_REL_BASED_DIR64
			reloc = append(reloc, byte(entry), byte(entry>>8))
			i++
		}

		// Pad to 4-byte alignment
		blockSize := len(reloc) - blockStart
		if blockSize%4 != 0 {
			reloc = append(reloc, 0, 0) // IMAGE_REL_BASED_ABSOLUTE padding
			blockSize += 2
		}

		// Patch block header
		aarch64.PutU32(reloc[blockStart:], uint32(pageRVA))
		aarch64.PutU32(reloc[blockStart+4:], uint32(blockSize))
	}

	return reloc
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
