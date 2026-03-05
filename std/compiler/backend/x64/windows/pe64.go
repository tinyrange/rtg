//go:build !no_backend_arm64 || !no_backend_windows_amd64

package x64

import (
	core "j5.nz/rtg/std/compiler/backend/x64"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
	objpe "j5.nz/rtg/std/compiler/object/pe"
)

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

// getImportDirInfo returns the RVA and size of the Import Directory Table.
func getImportDirInfo(g *core.CodeGen, imports []winImport, idataRVA int) (int, int) {
	return objpe.GetImportDirInfo(toPEImportGroups(imports), idataRVA)
}

// makeCOFFSym creates an 18-byte COFF symbol entry.
func makeCOFFSym(name []byte, value uint32, section uint16, symType uint16, storageClass byte) []byte {
	return objpe.MakeCOFFSym(name, value, section, symType, storageClass)
}

// buildCOFFSymbols creates the COFF symbol table and string table.
func buildCOFFSymbols(g *core.CodeGen, irmod *ir.IRModule) ([]byte, []byte, int) {
	return objpe.BuildCOFFSymbols(irmod, g.FuncOffsets())
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

// buildPE64 assembles a PE32+ (64-bit) executable from the compiled code, rodata, data,
// and Windows import fixups.
func buildPE64(g *core.CodeGen, irmod *ir.IRModule) []byte {
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
		imageBase        = 0x400000
	)

	dosHeaderSize := 64
	dosStubSize := 64
	peSignatureSize := 4
	coffHeaderSize := 20
	optionalHeaderSize := 240
	numSections := 6
	if g.Target().StripBinary {
		numSections = numSections - 2 // strip .debug_abbrev and .debug_info
	}
	sectionTableSize := numSections * 40

	headersRawSize := dosHeaderSize + dosStubSize + peSignatureSize + coffHeaderSize + optionalHeaderSize + sectionTableSize
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

	// Section sizes
	textRawSize := common.AlignUp(len(g.Code), fileAlignment)
	rdataRawSize := common.AlignUp(len(rdataContent), fileAlignment)
	dataRawSize := common.AlignUp(len(dataContent), fileAlignment)

	imports := collectWinImportsFromFixups(g)

	// Build .idata section with 8-byte ILT/IAT entries
	idataContent := buildIData64(g, imports)
	idataRawSize := common.AlignUp(len(idataContent), fileAlignment)

	// RVAs
	textRVA := sectionAlignment // 0x1000
	rdataRVA := textRVA + common.SectionSpan(len(g.Code), sectionAlignment)
	dataRVA := rdataRVA + common.SectionSpan(len(rdataContent), sectionAlignment)
	idataRVA := dataRVA + common.SectionSpan(len(dataContent), sectionAlignment)

	// Fix up .idata internal RVAs
	fixupIData64(g, idataContent, idataRVA, imports)

	// Build DWARF debug sections with 8-byte addresses
	textVA := imageBase + textRVA
	debugAbbrev := []byte{}
	debugInfo := []byte{}
	debugAbbrevRawSize := 0
	debugInfoRawSize := 0
	if !g.Target().StripBinary {
		debugAbbrev, debugInfo = buildDWARF64(g, irmod, textVA, len(g.Code))
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
		common.PutU32(coffStrtab[0:], uint32(len(coffStrtab)))
	}

	symtabFileOff := debugInfoFileOff + debugInfoRawSize
	strtabFileOff := symtabFileOff + len(coffSyms)
	totalFileSize := strtabFileOff + len(coffStrtab)
	if g.Target().StripBinary {
		totalFileSize = idataFileOff + idataRawSize
	}

	imageSize := debugInfoRVA + common.SectionSpan(len(debugInfo), sectionAlignment)
	if g.Target().StripBinary {
		imageSize = idataRVA + common.SectionSpan(len(idataContent), sectionAlignment)
	}

	// Fix up string headers and code references
	iatOffsets := buildIATOffsets64(g, imports)

	// x64: string headers are in .rodata section
	g.PatchLinuxStringHeaders(uint64(imageBase + rdataRVA))

	// Fix up code references (movabs imm64 and RIP-relative call)
	for _, fix := range g.CallFixups() {
		targetName := core.CallFixupTarget(fix)
		codeOffset := core.CallFixupOffset(fix)
		if targetName == "$rodata_header$" {
			// Patch 8-byte movabs immediate with rodata VA
			headerOff := common.GetU64(g.Code[codeOffset : codeOffset+8])
			common.PutU64(g.Code[codeOffset:codeOffset+8], uint64(imageBase+rdataRVA)+headerOff)
		} else if targetName == "$data_addr$" {
			// Patch 8-byte movabs immediate with data VA
			dataOff := common.GetU64(g.Code[codeOffset : codeOffset+8])
			common.PutU64(g.Code[codeOffset:codeOffset+8], uint64(imageBase+dataRVA)+dataOff)
		} else if len(targetName) > 10 && targetName[0:10] == "$funcaddr$" {
			// Patch 8-byte movabs immediate with function virtual address
			refName := targetName[10:]
			funcOff, ok := g.MaybeGetFuncOffsets(refName)
			if !ok {
				panic("ICE: unresolved function address fixup: " + refName)
			}
			funcVA := uint64(imageBase+textRVA) + uint64(funcOff)
			common.PutU64(g.Code[codeOffset:codeOffset+8], funcVA)
		} else if libName, funcName, ok := decodeIATFixupTarget(targetName); ok {
			iatOff, ok := iatOffsets[winImportKey(libName, funcName)]
			if !ok {
				continue
			}
			// Patch RIP-relative disp32: target = iatVA, rip = textVA + codeOffset + 4
			iatVA := uint64(imageBase+idataRVA) + uint64(iatOff)
			rip := uint64(imageBase+textRVA) + uint64(codeOffset) + 4
			disp32 := int32(int64(iatVA) - int64(rip))
			common.PutU32(g.Code[codeOffset:codeOffset+4], uint32(disp32))
		}
	}

	// Assemble the file
	pe := make([]byte, totalFileSize)

	// === DOS Header (64 bytes) ===
	pe[0] = 'M'
	pe[1] = 'Z'
	common.PutU32(pe[0x3C:], 0x80)

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
	machineType := uint16(0x8664)                        // IMAGE_FILE_MACHINE_AMD64
	common.PutU16(coff[0:], machineType)                 // Machine
	common.PutU16(coff[2:], uint16(numSections))         // NumberOfSections
	common.PutU32(coff[4:], 0)                           // TimeDateStamp
	common.PutU32(coff[8:], uint32(symtabFileOff))       // PointerToSymbolTable
	common.PutU32(coff[12:], uint32(numSyms))            // NumberOfSymbols
	common.PutU16(coff[16:], uint16(optionalHeaderSize)) // SizeOfOptionalHeader
	common.PutU16(coff[18:], 0x0022)                     // Characteristics: EXECUTABLE_IMAGE | LARGE_ADDRESS_AWARE

	// === Optional Header (240 bytes at 0x98) ===
	opt := pe[0x98:]
	common.PutU16(opt[0:], 0x020B)                                                       // Magic: PE32+
	opt[2] = 1                                                                           // MajorLinkerVersion
	opt[3] = 0                                                                           // MinorLinkerVersion
	common.PutU32(opt[4:], uint32(len(g.Code)))                                          // SizeOfCode
	common.PutU32(opt[8:], uint32(len(rdataContent)+len(dataContent)+len(idataContent))) // SizeOfInitializedData
	common.PutU32(opt[12:], 0)                                                           // SizeOfUninitializedData
	common.PutU32(opt[16:], uint32(textRVA))                                             // AddressOfEntryPoint
	common.PutU32(opt[20:], uint32(textRVA))                                             // BaseOfCode
	// PE32+ has NO BaseOfData field — ImageBase is at offset 24
	common.PutU64(opt[24:], uint64(imageBase))        // ImageBase (8 bytes)
	common.PutU32(opt[32:], uint32(sectionAlignment)) // SectionAlignment
	common.PutU32(opt[36:], uint32(fileAlignment))    // FileAlignment
	common.PutU16(opt[40:], 6)                        // MajorOperatingSystemVersion
	common.PutU16(opt[42:], 0)                        // MinorOperatingSystemVersion
	common.PutU16(opt[44:], 0)                        // MajorImageVersion
	common.PutU16(opt[46:], 0)                        // MinorImageVersion
	common.PutU16(opt[48:], 6)                        // MajorSubsystemVersion
	common.PutU16(opt[50:], 0)                        // MinorSubsystemVersion
	common.PutU32(opt[52:], 0)                        // Win32VersionValue
	common.PutU32(opt[56:], uint32(imageSize))        // SizeOfImage
	common.PutU32(opt[60:], uint32(headersAligned))   // SizeOfHeaders
	common.PutU32(opt[64:], 0)                        // CheckSum
	common.PutU16(opt[68:], 3)                        // Subsystem: IMAGE_SUBSYSTEM_WINDOWS_CUI
	common.PutU16(opt[70:], 0x0100)                   // DllCharacteristics (NX_COMPAT)
	// PE32+: Stack/Heap sizes are 8 bytes each
	common.PutU64(opt[72:], 0x100000) // SizeOfStackReserve (1MB)
	common.PutU64(opt[80:], 0x1000)   // SizeOfStackCommit (4KB)
	common.PutU64(opt[88:], 0x100000) // SizeOfHeapReserve (1MB)
	common.PutU64(opt[96:], 0x1000)   // SizeOfHeapCommit (4KB)
	common.PutU32(opt[104:], 0)       // LoaderFlags
	common.PutU32(opt[108:], 16)      // NumberOfRvaAndSizes

	// Data directories (16 entries x 8 bytes = 128 bytes starting at opt[112])
	// [1] Import Table
	importDirRVA, importDirSize := getImportDirInfo(g, imports, idataRVA)
	common.PutU32(opt[112+1*8:], uint32(importDirRVA))
	common.PutU32(opt[112+1*8+4:], uint32(importDirSize))

	// [12] IAT
	iatRVA, iatSize := getIATInfo64(g, imports, idataRVA)
	common.PutU32(opt[112+12*8:], uint32(iatRVA))
	common.PutU32(opt[112+12*8+4:], uint32(iatSize))

	// === Section Table (at 0x188) ===
	sectBase := 0x188

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

	nextSect := sectBase + 160

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
	copy(pe[textFileOff:], g.Code)
	copy(pe[rdataFileOff:], rdataContent)
	copy(pe[dataFileOff:], dataContent)
	copy(pe[idataFileOff:], idataContent)
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
func buildIData64(g *core.CodeGen, imports []winImport) []byte {
	return objpe.BuildIData64(toPEImportGroups(imports))
}

func idataOffsetAfterIAT64(imports []winImport) int {
	return objpe.IdataOffsetAfterIAT64(toPEImportGroups(imports))
}

// fixupIData64 adjusts RVA fields in the .idata content to be actual RVAs.
func fixupIData64(g *core.CodeGen, idata []byte, idataRVA int, imports []winImport) {
	objpe.FixupIData64(idata, idataRVA, toPEImportGroups(imports))
}

// buildIATOffsets64 returns import key → offset within .idata of the IAT entry.
func buildIATOffsets64(g *core.CodeGen, imports []winImport) map[string]int {
	return objpe.BuildIATOffsets64(toPEImportGroups(imports))
}

// getIATInfo64 returns the RVA and size of the IAT (8-byte entries).
func getIATInfo64(g *core.CodeGen, imports []winImport, idataRVA int) (int, int) {
	return objpe.GetIATInfo64(toPEImportGroups(imports), idataRVA)
}

// buildDWARF64 generates DWARF2 sections with 8-byte addresses for PE32+.
func buildDWARF64(g *core.CodeGen, irmod *ir.IRModule, textVA int, textSize int) ([]byte, []byte) {
	symbols := objpe.BuildDWARFSymbols(irmod, textVA, textVA+textSize, g.FuncOffsets())
	return objpe.BuildDWARF64(textVA, textVA+textSize, symbols)
}

// buildBaseRelocations builds a .reloc section for 64-bit PE base relocations.
// sectionRVA is the RVA of the section containing the addresses to relocate.
// offsets are sorted offsets within that section of 8-byte absolute addresses.
func buildBaseRelocations(g *core.CodeGen, sectionRVA int, offsets []int) []byte {
	return objpe.BuildBaseRelocations64(sectionRVA, offsets)
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
