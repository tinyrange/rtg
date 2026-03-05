//go:build !no_backend_windows_i386

package windows

import (
	core "j5.nz/rtg/std/compiler/backend/i386"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
	objpe "j5.nz/rtg/std/compiler/object/pe"
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
	headersAligned := common.AlignUp(headersRawSize, fileAlignment)

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
	textRawSize := common.AlignUp(len(g.Code), fileAlignment)
	rdataRawSize := common.AlignUp(len(rdataContent), fileAlignment)
	dataRawSize := common.AlignUp(len(dataContent), fileAlignment)

	imports := collectWinImportsFromFixups(g.CallFixups)

	// Build .idata section
	idataContent := buildIData(g, imports)
	idataRawSize := common.AlignUp(len(idataContent), fileAlignment)

	// RVAs (section-aligned)
	textRVA := sectionAlignment // 0x1000
	rdataRVA := textRVA + common.SectionSpan(len(g.Code), sectionAlignment)
	dataRVA := rdataRVA + common.SectionSpan(len(rdataContent), sectionAlignment)
	idataRVA := dataRVA + common.SectionSpan(len(dataContent), sectionAlignment)

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
		debugAbbrevRawSize = common.AlignUp(len(debugAbbrev), fileAlignment)
		debugInfoRawSize = common.AlignUp(len(debugInfo), fileAlignment)
	}

	debugAbbrevRVA := idataRVA + common.SectionSpan(len(idataContent), sectionAlignment)
	debugInfoRVA := debugAbbrevRVA + common.SectionSpan(len(debugAbbrev), sectionAlignment)

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
		common.PutU32(coffStrtab[0:], uint32(len(coffStrtab)))
	}

	symtabFileOff := debugInfoFileOff + debugInfoRawSize
	strtabFileOff := symtabFileOff + len(coffSyms)
	totalFileSize := strtabFileOff + len(coffStrtab)
	if g.Target.StripBinary {
		totalFileSize = idataFileOff + idataRawSize
	}

	// Virtual size of image
	imageSize := debugInfoRVA + common.SectionSpan(len(debugInfo), sectionAlignment)
	if g.Target.StripBinary {
		imageSize = idataRVA + common.SectionSpan(len(idataContent), sectionAlignment)
	}

	// Fix up string headers in rodata
	for _, headerOff := range g.StringMap {
		dataOff := common.GetU32(g.Rodata[headerOff : headerOff+4])
		common.PutU32(g.Rodata[headerOff:headerOff+4], uint32(imageBase+rdataRVA)+dataOff)
	}

	// Fix up code references
	iatOffsets := buildIATOffsets(g, imports)
	for _, fix := range g.CallFixups {
		if fix.Target == "$rodata_header$" {
			headerOff := common.GetU32(g.Code[fix.CodeOffset : fix.CodeOffset+4])
			common.PutU32(g.Code[fix.CodeOffset:fix.CodeOffset+4], uint32(imageBase+rdataRVA)+headerOff)
		} else if fix.Target == "$data_addr$" {
			dataOff := common.GetU32(g.Code[fix.CodeOffset : fix.CodeOffset+4])
			common.PutU32(g.Code[fix.CodeOffset:fix.CodeOffset+4], uint32(imageBase+dataRVA)+dataOff)
		} else if len(fix.Target) > 10 && fix.Target[0:10] == "$funcaddr$" {
			refName := fix.Target[10:]
			funcOff, ok := g.FuncOffsets[refName]
			if !ok {
				panic("ICE: unresolved function address fixup: " + refName)
			}
			funcVA := uint32(imageBase+textRVA) + uint32(funcOff)
			common.PutU32(g.Code[fix.CodeOffset:fix.CodeOffset+4], funcVA)
		} else if libName, funcName, ok := decodeIATFixupTarget(fix.Target); ok {
			iatOff, ok := iatOffsets[winImportKey(libName, funcName)]
			if !ok {
				continue
			}
			common.PutU32(g.Code[fix.CodeOffset:fix.CodeOffset+4], uint32(imageBase+idataRVA)+uint32(iatOff))
		}
	}

	// Assemble the file
	pe := make([]byte, totalFileSize)

	// === DOS Header (64 bytes) ===
	pe[0] = 'M'
	pe[1] = 'Z'
	// e_lfanew at offset 0x3C (point to PE signature)
	common.PutU32(pe[0x3C:], 0x80)

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
	common.PutU16(coff[0:], 0x014C)              // Machine: IMAGE_FILE_MACHINE_I386
	common.PutU16(coff[2:], uint16(numSections)) // NumberOfSections
	common.PutU32(coff[4:], 0)                   // TimeDateStamp
	if g.Target.StripBinary {
		common.PutU32(coff[8:], 0) // PointerToSymbolTable
		common.PutU32(coff[12:], 0)
	} else {
		common.PutU32(coff[8:], uint32(symtabFileOff)) // PointerToSymbolTable
		common.PutU32(coff[12:], uint32(numSyms))      // NumberOfSymbols
	}
	common.PutU16(coff[16:], uint16(optionalHeaderSize)) // SizeOfOptionalHeader
	common.PutU16(coff[18:], 0x0103)                     // Characteristics: RELOCS_STRIPPED | EXECUTABLE_IMAGE | 32BIT_MACHINE

	// === Optional Header (224 bytes at 0x98) ===
	opt := pe[0x98:]
	common.PutU16(opt[0:], 0x010B)                                                       // Magic: PE32
	opt[2] = 1                                                                           // MajorLinkerVersion
	opt[3] = 0                                                                           // MinorLinkerVersion
	common.PutU32(opt[4:], uint32(len(g.Code)))                                          // SizeOfCode
	common.PutU32(opt[8:], uint32(len(rdataContent)+len(dataContent)+len(idataContent))) // SizeOfInitializedData
	common.PutU32(opt[12:], 0)                                                           // SizeOfUninitializedData
	common.PutU32(opt[16:], uint32(textRVA))                                             // AddressOfEntryPoint
	common.PutU32(opt[20:], uint32(textRVA))                                             // BaseOfCode
	common.PutU32(opt[24:], uint32(rdataRVA))                                            // BaseOfData
	common.PutU32(opt[28:], uint32(imageBase))                                           // ImageBase
	common.PutU32(opt[32:], uint32(sectionAlignment))                                    // SectionAlignment
	common.PutU32(opt[36:], uint32(fileAlignment))                                       // FileAlignment
	common.PutU16(opt[40:], 4)                                                           // MajorOperatingSystemVersion
	common.PutU16(opt[42:], 0)                                                           // MinorOperatingSystemVersion
	common.PutU16(opt[44:], 0)                                                           // MajorImageVersion
	common.PutU16(opt[46:], 0)                                                           // MinorImageVersion
	common.PutU16(opt[48:], 4)                                                           // MajorSubsystemVersion (4 = Win95/NT4)
	common.PutU16(opt[50:], 0)                                                           // MinorSubsystemVersion
	common.PutU32(opt[52:], 0)                                                           // Win32VersionValue
	common.PutU32(opt[56:], uint32(imageSize))                                           // SizeOfImage
	common.PutU32(opt[60:], uint32(headersAligned))                                      // SizeOfHeaders
	common.PutU32(opt[64:], 0)                                                           // CheckSum
	common.PutU16(opt[68:], 3)                                                           // Subsystem: IMAGE_SUBSYSTEM_WINDOWS_CUI
	common.PutU16(opt[70:], 0)                                                           // DllCharacteristics
	common.PutU32(opt[72:], 0x100000)                                                    // SizeOfStackReserve (1MB)
	common.PutU32(opt[76:], 0x1000)                                                      // SizeOfStackCommit (4KB)
	common.PutU32(opt[80:], 0x100000)                                                    // SizeOfHeapReserve (1MB)
	common.PutU32(opt[84:], 0x1000)                                                      // SizeOfHeapCommit (4KB)
	common.PutU32(opt[88:], 0)                                                           // LoaderFlags
	common.PutU32(opt[92:], 16)                                                          // NumberOfRvaAndSizes

	// Data directories (16 entries x 8 bytes = 128 bytes starting at opt[96])
	// [1] Import Table
	importDirRVA, importDirSize := getImportDirInfo(g, imports, idataRVA)
	common.PutU32(opt[96+1*8:], uint32(importDirRVA))
	common.PutU32(opt[96+1*8+4:], uint32(importDirSize))

	// [12] IAT
	iatRVA, iatSize := getIATInfo(g, imports, idataRVA)
	common.PutU32(opt[96+12*8:], uint32(iatRVA))
	common.PutU32(opt[96+12*8+4:], uint32(iatSize))

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
	objpe.WriteSection(buf, name, virtualSize, rva, rawSize, fileOff, characteristics)
}

// writeSectionLongName writes a section header with a long name referenced via
// the COFF string table. The name field is "/<decimal_offset>".
func writeSectionLongName(buf []byte, strtabOffset int, virtualSize, rva, rawSize, fileOff int, characteristics uint32) {
	objpe.WriteSectionLongName(buf, strtabOffset, virtualSize, rva, rawSize, fileOff, characteristics)
}

// formatSlashOffset formats an integer as "/<decimal>" for PE long section names.
func formatSlashOffset(n int) []byte {
	return objpe.FormatSlashOffset(n)
}

// buildIData builds the .idata section content.
// Layout: Import Directory Table | ILT blocks | IAT blocks | Hint/Name Table | DLL names
func buildIData(g *core.CodeGen, imports []winImport) []byte {
	return objpe.BuildIData32(toPEImportGroups(imports))
}

func idataOffsetAfterIAT(imports []winImport) int {
	return objpe.IdataOffsetAfterIAT32(toPEImportGroups(imports))
}

// fixupIData adjusts all RVA fields in the .idata content to be actual RVAs.
func fixupIData(g *core.CodeGen, idata []byte, idataRVA int, imports []winImport) {
	objpe.FixupIData32(idata, idataRVA, toPEImportGroups(imports))
}

// buildIATOffsets returns import key → offset within .idata of that function's IAT entry.
func buildIATOffsets(g *core.CodeGen, imports []winImport) map[string]int {
	return objpe.BuildIATOffsets32(toPEImportGroups(imports))
}

// getImportDirInfo returns the RVA and size of the Import Directory Table.
func getImportDirInfo(g *core.CodeGen, imports []winImport, idataRVA int) (int, int) {
	return objpe.GetImportDirInfo(toPEImportGroups(imports), idataRVA)
}

// getIATInfo returns the RVA and size of the IAT.
func getIATInfo(g *core.CodeGen, imports []winImport, idataRVA int) (int, int) {
	return objpe.GetIATInfo32(toPEImportGroups(imports), idataRVA)
}

// makeCOFFSym creates an 18-byte COFF symbol entry.
func makeCOFFSym(name []byte, value uint32, section uint16, symType uint16, storageClass byte) []byte {
	return objpe.MakeCOFFSym(name, value, section, symType, storageClass)
}

// buildCOFFSymbols creates the COFF symbol table and string table.
func buildCOFFSymbols(g *core.CodeGen, irmod *ir.IRModule) ([]byte, []byte, int) {
	return objpe.BuildCOFFSymbols(irmod, g.FuncOffsets)
}

func toPEImportGroups(imports []winImport) []objpe.ImportGroup {
	local := groupWinImports(imports)
	groups := make([]objpe.ImportGroup, len(local))
	for i, grp := range local {
		groups[i] = objpe.ImportGroup{
			Library: grp.Library,
			Symbols: grp.Symbols,
		}
	}
	return groups
}

// buildDWARF generates minimal DWARF2 .debug_abbrev and .debug_info sections
// so that WineDbg can resolve function names in backtraces.
func buildDWARF(g *core.CodeGen, irmod *ir.IRModule, textVA int, textSize int) ([]byte, []byte) {
	symbols := objpe.BuildDWARFSymbols(irmod, textVA, textVA+textSize, g.FuncOffsets)
	return objpe.BuildDWARF32(textVA, textVA+textSize, symbols)
}

// appendU32 appends a little-endian uint32 to a byte slice.
func appendU32(b []byte, v uint32) []byte {
	b = append(b, byte(v))
	b = append(b, byte(v>>8))
	b = append(b, byte(v>>16))
	b = append(b, byte(v>>24))
	return b
}
