//go:build !no_backend_windows_i386

package windows

import (
	core "j5.nz/rtg/std/compiler/backend/i386"
	"j5.nz/rtg/std/compiler/ir"
)

// buildPE32 assembles a PE32 executable from the compiled code, rodata, data,
// and Windows import fixups.
func BuildPE32(g *core.CodeGen, irmod *ir.IRModule) []byte {
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
	if g.Target.StripBinary {
		numSections = 4
	}
	sectionTableSize := numSections * 40

	headersRawSize := dosHeaderSize + dosStubSize + peSignatureSize + coffHeaderSize + optionalHeaderSize + sectionTableSize
	// Align headers to FileAlignment
	headersAligned := alignUp(headersRawSize, fileAlignment)

	// Ensure empty initialized-data sections still emit a minimal payload.
	// Some Windows loaders reject images with zero-sized initialized sections.
	rdataContent := g.Rodata
	if len(rdataContent) == 0 {
		rdataContent = []byte{0}
	}
	dataContent := g.Data
	if len(dataContent) == 0 {
		dataContent = []byte{0}
	}

	// Section sizes (raw = file-aligned, virtual = section-aligned)
	textRawSize := alignUp(len(g.Code), fileAlignment)
	rdataRawSize := alignUp(len(rdataContent), fileAlignment)
	dataRawSize := alignUp(len(dataContent), fileAlignment)

	imports := collectWinImportsFromFixups(g.CallFixups)

	// Build .idata section
	idataContent := buildIData(g, imports)
	idataRawSize := alignUp(len(idataContent), fileAlignment)

	// RVAs (section-aligned)
	textRVA := sectionAlignment // 0x1000
	rdataRVA := textRVA + sectionSpan(len(g.Code), sectionAlignment)
	dataRVA := rdataRVA + sectionSpan(len(rdataContent), sectionAlignment)
	idataRVA := dataRVA + sectionSpan(len(dataContent), sectionAlignment)

	// Fix up .idata internal RVAs
	fixupIData(g, idataContent, idataRVA, imports)

	// Build DWARF debug sections
	textVA := imageBase + textRVA
	debugAbbrev := []byte{}
	debugInfo := []byte{}
	debugAbbrevRawSize := 0
	debugInfoRawSize := 0
	if !g.Target.StripBinary {
		debugAbbrev, debugInfo = buildDWARF(g, irmod, textVA, len(g.Code))
		debugAbbrevRawSize = alignUp(len(debugAbbrev), fileAlignment)
		debugInfoRawSize = alignUp(len(debugInfo), fileAlignment)
	}

	debugAbbrevRVA := idataRVA + sectionSpan(len(idataContent), sectionAlignment)
	debugInfoRVA := debugAbbrevRVA + sectionSpan(len(debugAbbrev), sectionAlignment)

	// File offsets
	textFileOff := headersAligned
	rdataFileOff := textFileOff + textRawSize
	dataFileOff := rdataFileOff + rdataRawSize
	idataFileOff := dataFileOff + dataRawSize
	debugAbbrevFileOff := idataFileOff + idataRawSize
	debugInfoFileOff := debugAbbrevFileOff + debugAbbrevRawSize

	// Build COFF symbol table and string table
	// We also add long section names (.debug_abbrev, .debug_info) to the string table
	coffSyms := []byte{}
	coffStrtab := []byte{}
	numSyms := 0
	if !g.Target.StripBinary {
		coffSyms, coffStrtab, numSyms = buildCOFFSymbols(g, irmod)
	}

	// Add long section names to string table and record their offsets
	debugAbbrevNameOff := 0
	debugInfoNameOff := 0
	if !g.Target.StripBinary {
		debugAbbrevNameOff = len(coffStrtab)
		coffStrtab = append(coffStrtab, []byte(".debug_abbrev")...)
		coffStrtab = append(coffStrtab, 0)
		debugInfoNameOff = len(coffStrtab)
		coffStrtab = append(coffStrtab, []byte(".debug_info")...)
		coffStrtab = append(coffStrtab, 0)
		// Re-patch string table size
		putU32(coffStrtab[0:], uint32(len(coffStrtab)))
	}

	symtabFileOff := debugInfoFileOff + debugInfoRawSize
	strtabFileOff := symtabFileOff + len(coffSyms)
	totalFileSize := strtabFileOff + len(coffStrtab)
	if g.Target.StripBinary {
		totalFileSize = idataFileOff + idataRawSize
	}

	// Virtual size of image
	imageSize := debugInfoRVA + sectionSpan(len(debugInfo), sectionAlignment)
	if g.Target.StripBinary {
		imageSize = idataRVA + sectionSpan(len(idataContent), sectionAlignment)
	}

	// Fix up string headers in rodata
	for _, headerOff := range g.StringMap {
		dataOff := getU32(g.Rodata[headerOff : headerOff+4])
		putU32(g.Rodata[headerOff:headerOff+4], uint32(imageBase+rdataRVA)+dataOff)
	}

	// Fix up code references
	iatOffsets := buildIATOffsets(g, imports)
	for _, fix := range g.CallFixups {
		if fix.Target == "$rodata_header$" {
			headerOff := getU32(g.Code[fix.CodeOffset : fix.CodeOffset+4])
			putU32(g.Code[fix.CodeOffset:fix.CodeOffset+4], uint32(imageBase+rdataRVA)+headerOff)
		} else if fix.Target == "$data_addr$" {
			dataOff := getU32(g.Code[fix.CodeOffset : fix.CodeOffset+4])
			putU32(g.Code[fix.CodeOffset:fix.CodeOffset+4], uint32(imageBase+dataRVA)+dataOff)
		} else if libName, funcName, ok := decodeIATFixupTarget(fix.Target); ok {
			iatOff, ok := iatOffsets[winImportKey(libName, funcName)]
			if !ok {
				continue
			}
			putU32(g.Code[fix.CodeOffset:fix.CodeOffset+4], uint32(imageBase+idataRVA)+uint32(iatOff))
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
	putU16(coff[0:], 0x014C)              // Machine: IMAGE_FILE_MACHINE_I386
	putU16(coff[2:], uint16(numSections)) // NumberOfSections
	putU32(coff[4:], 0)                   // TimeDateStamp
	if g.Target.StripBinary {
		putU32(coff[8:], 0) // PointerToSymbolTable
		putU32(coff[12:], 0)
	} else {
		putU32(coff[8:], uint32(symtabFileOff)) // PointerToSymbolTable
		putU32(coff[12:], uint32(numSyms))      // NumberOfSymbols
	}
	putU16(coff[16:], uint16(optionalHeaderSize)) // SizeOfOptionalHeader
	putU16(coff[18:], 0x0103)                     // Characteristics: RELOCS_STRIPPED | EXECUTABLE_IMAGE | 32BIT_MACHINE

	// === Optional Header (224 bytes at 0x98) ===
	opt := pe[0x98:]
	putU16(opt[0:], 0x010B)                                                       // Magic: PE32
	opt[2] = 1                                                                    // MajorLinkerVersion
	opt[3] = 0                                                                    // MinorLinkerVersion
	putU32(opt[4:], uint32(len(g.Code)))                                          // SizeOfCode
	putU32(opt[8:], uint32(len(rdataContent)+len(dataContent)+len(idataContent))) // SizeOfInitializedData
	putU32(opt[12:], 0)                                                           // SizeOfUninitializedData
	putU32(opt[16:], uint32(textRVA))                                             // AddressOfEntryPoint
	putU32(opt[20:], uint32(textRVA))                                             // BaseOfCode
	putU32(opt[24:], uint32(rdataRVA))                                            // BaseOfData
	putU32(opt[28:], uint32(imageBase))                                           // ImageBase
	putU32(opt[32:], uint32(sectionAlignment))                                    // SectionAlignment
	putU32(opt[36:], uint32(fileAlignment))                                       // FileAlignment
	putU16(opt[40:], 4)                                                           // MajorOperatingSystemVersion
	putU16(opt[42:], 0)                                                           // MinorOperatingSystemVersion
	putU16(opt[44:], 0)                                                           // MajorImageVersion
	putU16(opt[46:], 0)                                                           // MinorImageVersion
	putU16(opt[48:], 4)                                                           // MajorSubsystemVersion (4 = Win95/NT4)
	putU16(opt[50:], 0)                                                           // MinorSubsystemVersion
	putU32(opt[52:], 0)                                                           // Win32VersionValue
	putU32(opt[56:], uint32(imageSize))                                           // SizeOfImage
	putU32(opt[60:], uint32(headersAligned))                                      // SizeOfHeaders
	putU32(opt[64:], 0)                                                           // CheckSum
	putU16(opt[68:], 3)                                                           // Subsystem: IMAGE_SUBSYSTEM_WINDOWS_CUI
	putU16(opt[70:], 0)                                                           // DllCharacteristics
	putU32(opt[72:], 0x100000)                                                    // SizeOfStackReserve (1MB)
	putU32(opt[76:], 0x1000)                                                      // SizeOfStackCommit (4KB)
	putU32(opt[80:], 0x100000)                                                    // SizeOfHeapReserve (1MB)
	putU32(opt[84:], 0x1000)                                                      // SizeOfHeapCommit (4KB)
	putU32(opt[88:], 0)                                                           // LoaderFlags
	putU32(opt[92:], 16)                                                          // NumberOfRvaAndSizes

	// Data directories (16 entries x 8 bytes = 128 bytes starting at opt[96])
	// [1] Import Table
	importDirRVA, importDirSize := getImportDirInfo(g, imports, idataRVA)
	putU32(opt[96+1*8:], uint32(importDirRVA))
	putU32(opt[96+1*8+4:], uint32(importDirSize))

	// [12] IAT
	iatRVA, iatSize := getIATInfo(g, imports, idataRVA)
	putU32(opt[96+12*8:], uint32(iatRVA))
	putU32(opt[96+12*8+4:], uint32(iatSize))

	// === Section Table (4 x 40 bytes at 0x178) ===
	sectBase := 0x178

	// .text
	writeSection(pe[sectBase:], ".text",
		len(g.Code), textRVA, textRawSize, textFileOff,
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

	if !g.Target.StripBinary {
		// .debug_abbrev — long name via COFF string table
		writeSectionLongName(pe[sectBase+160:], debugAbbrevNameOff,
			len(debugAbbrev), debugAbbrevRVA, debugAbbrevRawSize, debugAbbrevFileOff,
			0x42000040) // INITIALIZED_DATA | READ | DISCARDABLE

		// .debug_info — long name via COFF string table
		writeSectionLongName(pe[sectBase+200:], debugInfoNameOff,
			len(debugInfo), debugInfoRVA, debugInfoRawSize, debugInfoFileOff,
			0x42000040) // INITIALIZED_DATA | READ | DISCARDABLE
	}

	// Copy section data
	copy(pe[textFileOff:], g.Code)
	copy(pe[rdataFileOff:], rdataContent)
	copy(pe[dataFileOff:], dataContent)
	copy(pe[idataFileOff:], idataContent)
	if !g.Target.StripBinary {
		copy(pe[debugAbbrevFileOff:], debugAbbrev)
		copy(pe[debugInfoFileOff:], debugInfo)
	}

	// Copy COFF symbol table and string table
	if !g.Target.StripBinary {
		copy(pe[symtabFileOff:], coffSyms)
		copy(pe[strtabFileOff:], coffStrtab)
	}

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
	putU32(buf[8:], uint32(virtualSize)) // VirtualSize
	putU32(buf[12:], uint32(rva))        // VirtualAddress (RVA)
	putU32(buf[16:], uint32(rawSize))    // SizeOfRawData
	putU32(buf[20:], uint32(fileOff))    // PointerToRawData
	putU32(buf[24:], 0)                  // PointerToRelocations
	putU32(buf[28:], 0)                  // PointerToLinenumbers
	putU16(buf[32:], 0)                  // NumberOfRelocations
	putU16(buf[34:], 0)                  // NumberOfLinenumbers
	putU32(buf[36:], characteristics)    // Characteristics
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

// buildIData builds the .idata section content.
// Layout: Import Directory Table | ILT blocks | IAT blocks | Hint/Name Table | DLL names
func buildIData(g *core.CodeGen, imports []winImport) []byte {
	groups := groupWinImports(imports)
	numLibs := len(groups)

	// Import Directory Table: one descriptor per DLL, plus null terminator.
	idtSize := (numLibs + 1) * 20

	// Compute ILT and IAT block offsets.
	iltOffsets := make([]int, numLibs)
	iatOffsets := make([]int, numLibs)
	iltSize := 0
	for i, grp := range groups {
		iltOffsets[i] = idtSize + iltSize
		iltSize += (len(grp.Symbols) + 1) * 4
	}
	iatBase := idtSize + iltSize
	iatSize := 0
	for i, grp := range groups {
		iatOffsets[i] = iatBase + iatSize
		iatSize += (len(grp.Symbols) + 1) * 4
	}

	// Hint/Name entries.
	hntOffset := idataOffsetAfterIAT(imports)
	var hntEntries []byte
	hntOffsets := make(map[string]int)
	for _, grp := range groups {
		for _, sym := range grp.Symbols {
			off := hntOffset + len(hntEntries)
			hntOffsets[winImportKey(grp.Library, sym)] = off
			hntEntries = append(hntEntries, 0, 0) // Hint = 0
			hntEntries = append(hntEntries, []byte(sym)...)
			hntEntries = append(hntEntries, 0)
			if len(hntEntries)%2 != 0 {
				hntEntries = append(hntEntries, 0)
			}
		}
	}

	// DLL names.
	dllNameOffset := hntOffset + len(hntEntries)
	dllOffsets := make([]int, numLibs)
	var dllEntries []byte
	for i, grp := range groups {
		dllOffsets[i] = dllNameOffset + len(dllEntries)
		dllEntries = append(dllEntries, []byte(grp.Library)...)
		dllEntries = append(dllEntries, 0)
	}

	totalSize := dllNameOffset + len(dllEntries)
	idata := make([]byte, totalSize)

	// Import directory descriptors and thunk tables.
	for i, grp := range groups {
		base := i * 20
		putU32(idata[base+0:], uint32(iltOffsets[i]))  // OriginalFirstThunk
		putU32(idata[base+4:], 0)                      // TimeDateStamp
		putU32(idata[base+8:], 0)                      // ForwarderChain
		putU32(idata[base+12:], uint32(dllOffsets[i])) // Name
		putU32(idata[base+16:], uint32(iatOffsets[i])) // FirstThunk

		for j, sym := range grp.Symbols {
			key := winImportKey(grp.Library, sym)
			hnt := uint32(hntOffsets[key])
			putU32(idata[iltOffsets[i]+j*4:], hnt)
			putU32(idata[iatOffsets[i]+j*4:], hnt)
		}
	}

	copy(idata[hntOffset:], hntEntries)
	copy(idata[dllNameOffset:], dllEntries)
	return idata
}

func idataOffsetAfterIAT(imports []winImport) int {
	groups := groupWinImports(imports)
	idtSize := (len(groups) + 1) * 20
	iltSize := 0
	iatSize := 0
	for _, grp := range groups {
		iltSize += (len(grp.Symbols) + 1) * 4
		iatSize += (len(grp.Symbols) + 1) * 4
	}
	return idtSize + iltSize + iatSize
}

// fixupIData adjusts all RVA fields in the .idata content to be actual RVAs.
func fixupIData(g *core.CodeGen, idata []byte, idataRVA int, imports []winImport) {
	groups := groupWinImports(imports)
	numLibs := len(groups)
	idtSize := (numLibs + 1) * 20

	iltSize := 0
	for _, grp := range groups {
		iltSize += (len(grp.Symbols) + 1) * 4
	}
	iatBase := idtSize + iltSize

	for i := 0; i < numLibs; i++ {
		base := i * 20
		putU32(idata[base+0:], uint32(idataRVA)+getU32(idata[base+0:base+4]))    // OriginalFirstThunk
		putU32(idata[base+12:], uint32(idataRVA)+getU32(idata[base+12:base+16])) // Name
		putU32(idata[base+16:], uint32(idataRVA)+getU32(idata[base+16:base+20])) // FirstThunk
	}

	iltOff := idtSize
	for _, grp := range groups {
		for i := 0; i < len(grp.Symbols); i++ {
			off := iltOff + i*4
			putU32(idata[off:], uint32(idataRVA)+getU32(idata[off:off+4]))
		}
		iltOff += (len(grp.Symbols) + 1) * 4
	}

	iatOff := iatBase
	for _, grp := range groups {
		for i := 0; i < len(grp.Symbols); i++ {
			off := iatOff + i*4
			putU32(idata[off:], uint32(idataRVA)+getU32(idata[off:off+4]))
		}
		iatOff += (len(grp.Symbols) + 1) * 4
	}
}

// buildIATOffsets returns import key → offset within .idata of that function's IAT entry.
func buildIATOffsets(g *core.CodeGen, imports []winImport) map[string]int {
	groups := groupWinImports(imports)
	idtSize := (len(groups) + 1) * 20
	iltSize := 0
	for _, grp := range groups {
		iltSize += (len(grp.Symbols) + 1) * 4
	}
	iatBase := idtSize + iltSize

	offsets := make(map[string]int)
	cur := iatBase
	for _, grp := range groups {
		for i, sym := range grp.Symbols {
			offsets[winImportKey(grp.Library, sym)] = cur + i*4
		}
		cur += (len(grp.Symbols) + 1) * 4
	}
	return offsets
}

// getImportDirInfo returns the RVA and size of the Import Directory Table.
func getImportDirInfo(g *core.CodeGen, imports []winImport, idataRVA int) (int, int) {
	groups := groupWinImports(imports)
	if len(groups) == 0 {
		return 0, 0
	}
	return idataRVA, (len(groups) + 1) * 20
}

// getIATInfo returns the RVA and size of the IAT.
func getIATInfo(g *core.CodeGen, imports []winImport, idataRVA int) (int, int) {
	groups := groupWinImports(imports)
	if len(groups) == 0 {
		return 0, 0
	}
	idtSize := (len(groups) + 1) * 20
	iltSize := 0
	iatSize := 0
	for _, grp := range groups {
		iltSize += (len(grp.Symbols) + 1) * 4
		iatSize += (len(grp.Symbols) + 1) * 4
	}
	return idataRVA + idtSize + iltSize, iatSize
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
func buildCOFFSymbols(g *core.CodeGen, irmod *ir.IRModule) ([]byte, []byte, int) {
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
		funcOff := g.FuncOffsets[f.Name]
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
func buildDWARF(g *core.CodeGen, irmod *ir.IRModule, textVA int, textSize int) ([]byte, []byte) {
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
		startHighPC = textVA + g.FuncOffsets[irmod.Funcs[0].Name]
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
		funcStart := textVA + g.FuncOffsets[f.Name]
		funcEnd := textVA + textSize
		if i+1 < len(irmod.Funcs) {
			funcEnd = textVA + g.FuncOffsets[irmod.Funcs[i+1].Name]
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

func alignUp(v, align int) int {
	if align <= 0 {
		return v
	}
	mask := align - 1
	return (v + mask) & ^mask
}

func sectionSpan(size, align int) int {
	if size <= 0 {
		return align
	}
	return alignUp(size, align)
}
