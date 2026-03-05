//go:build !no_backend_darwin_arm64

package macos

import (
	"j5.nz/rtg/std/compiler/backend/aarch64"
	"j5.nz/rtg/std/compiler/backend/aarch64/codesign"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// === Mach-O 64-bit Builder for ARM64 ===

type machoSymEntry struct {
	nameOff int
	value   uint64
	size    uint64
	ntype   byte
}

const (
	machoPageSize = 0x4000 // 16KB for ARM64 macOS
)

// buildMachO64 builds a Mach-O 64-bit executable for macOS ARM64.
func BuildMachO64(g *aarch64.CodeGen, irmod *ir.IRModule, outputName string) []byte {
	// In Mach-O, the __TEXT segment starts at file offset 0 and includes
	// the Mach-O header and load commands. This is different from ELF.
	//
	// Layout:
	// [__TEXT segment: fileoff=0]
	//   [Mach-O Header: 32 bytes]
	//   [Load Commands: variable]
	//   [Padding]
	//   [__text section: machine code]
	//   [__const section: rodata]
	//   [Padding to page boundary]
	// [__DATA segment]
	//   [__data section: globals]
	//   [__got section: GOT entries]
	//   [Padding to page boundary]
	// [__LINKEDIT segment]
	//   [bind opcodes, export trie, symtab, strtab]
	//   [Padding to page boundary]

	textSize := len(g.Code())
	rodataSize := len(g.Rodata())
	dataSize := len(g.Data())
	gotSize := len(g.GotSymbols()) * 8

	bindOpcodes := buildBindOpcodes(g)

	// === Build string table and symbol entries ===
	strtab := []byte{}
	var syms []machoSymEntry
	if !g.Target().StripBinary {
		strtab = append(strtab, 0) // index 0 = empty string

		// _main entry point (the code before the first compiled function)
		mainNameOff := len(strtab)
		strtab = append(strtab, []byte("_main")...)
		strtab = append(strtab, 0)
		mainSize := uint64(0)
		if len(irmod.Funcs) > 0 {
			if v, ok := g.LookupFuncOffset(irmod.Funcs[0].Name); ok {
				mainSize = uint64(v)
			}
		} else {
			mainSize = uint64(textSize)
		}
		syms = append(syms, machoSymEntry{mainNameOff, 0, mainSize, 0x0F}) // N_SECT|N_EXT

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
			syms = append(syms, machoSymEntry{nameOff, uint64(funcStart), uint64(funcSize), 0x0E}) // N_SECT (local)
		}
	}

	// Build export trie for _main
	exportTrie := buildExportTrie(0) // _main at offset 0 from image base __TEXT

	// Dylinker and dylib paths (padded to 8-byte alignment)
	// "/usr/lib/dyld\x00" = 14 bytes, padded to 16
	dylinkerPath := "/usr/lib/dyld\x00\x00\x00"
	// "/usr/lib/libSystem.B.dylib\x00" = 27 bytes, padded to 32
	dylibPath := "/usr/lib/libSystem.B.dylib\x00\x00\x00\x00\x00\x00"

	// Load command sizes (all must be multiples of 8 for 64-bit Mach-O)
	lcSegSize := 72
	lcSectSize := 80
	lcDylinkerSize := common.AlignUp(12+len(dylinkerPath), 8)
	lcDylibSize := common.AlignUp(24+len(dylibPath), 8)
	lcMainSize := 24
	lcSymtabSize := 0
	lcDysymtabSize := 0
	if !g.Target().StripBinary {
		lcSymtabSize = 24
		lcDysymtabSize = 80
	}
	lcDyldInfoSize := 48
	lcCodeSigSize := 16

	ncmds := 11
	if g.Target().StripBinary {
		ncmds = ncmds - 2
	}
	lcTotal := lcSegSize + // PAGEZERO
		lcSegSize + 2*lcSectSize + // TEXT
		lcSegSize + 2*lcSectSize + // DATA
		lcSegSize + // LINKEDIT
		lcDylinkerSize +
		lcDylibSize +
		lcMainSize +
		lcSymtabSize +
		lcDysymtabSize +
		lcDyldInfoSize +
		lcCodeSigSize

	headerSize := 32 + lcTotal

	// __TEXT segment starts at file offset 0, includes header
	// __text section starts after header (aligned)
	textSectionOff := common.AlignUp(headerSize, 16)
	constSectionOff := textSectionOff + textSize
	textSegEnd := common.AlignUp(constSectionOff+rodataSize, machoPageSize)
	if textSegEnd < machoPageSize {
		textSegEnd = machoPageSize
	}

	// __DATA segment
	dataSegStart := textSegEnd
	dataSectionOff := dataSegStart
	gotSectionOff := dataSectionOff + common.AlignUp(dataSize, 8)
	dataSegEnd := common.AlignUp(gotSectionOff+gotSize, machoPageSize)
	if dataSegEnd == dataSegStart {
		dataSegEnd = dataSegStart + machoPageSize
	}

	// __LINKEDIT segment
	linkeditStart := dataSegEnd

	// Layout within __LINKEDIT: bind opcodes, export trie, symtab nlist, strtab
	bindOff := linkeditStart
	bindSize := len(bindOpcodes)

	exportOff := common.AlignUp(bindOff+bindSize, 8)
	exportSize := len(exportTrie)

	nlistSize := 16
	symtabOff := common.AlignUp(exportOff+exportSize, 8)
	symtabNEntries := len(syms)
	symtabDataSize := symtabNEntries * nlistSize

	strtabOff := symtabOff + symtabDataSize
	strtabSize := len(strtab)

	// Code signature goes at the end of __LINKEDIT, 16-byte aligned
	codeSignOff := common.AlignUp(strtabOff+strtabSize, 16)
	codeSignID := outputName
	if codeSignID == "" {
		codeSignID = "a.out"
	}
	sigSize := int(codesign.CodeSignSize(int64(codeSignOff), codeSignID))

	linkeditEnd := common.AlignUp(codeSignOff+sigSize, machoPageSize)
	if linkeditEnd == linkeditStart {
		linkeditEnd = linkeditStart + machoPageSize
	}

	totalFileSize := linkeditEnd

	// Virtual addresses
	pagezeroVMSize := uint64(0x100000000)
	textSegVAddr := pagezeroVMSize
	textSegVMSize := uint64(textSegEnd)

	textSectionVAddr := textSegVAddr + uint64(textSectionOff)
	constSectionVAddr := textSegVAddr + uint64(constSectionOff)

	dataSegVAddr := textSegVAddr + uint64(dataSegStart)
	dataSegVMSize := uint64(dataSegEnd - dataSegStart)
	dataSectionVAddr := textSegVAddr + uint64(dataSectionOff)
	gotSectionVAddr := textSegVAddr + uint64(gotSectionOff)

	linkeditVAddr := textSegVAddr + uint64(linkeditStart)
	linkeditVMSize := uint64(linkeditEnd - linkeditStart)

	// String header data_ptr fields are computed at runtime via ADRP+ADD (ASLR-safe).
	// No link-time fixup or rebase needed.

	// Fix up code references (ADRP+ADD or ADRP+LDR pairs)
	for i := 0; i < g.CallFixupCount(); i++ {
		codeOffset, targetName, value := g.CallFixupAt(i)
		pcAddr := textSectionVAddr + uint64(codeOffset)
		switch targetName {
		case "$rodata_header$":
			targetAddr := constSectionVAddr + value
			g.PatchAdrpAdd(codeOffset, pcAddr, targetAddr)
		case "$data_addr$":
			targetAddr := dataSectionVAddr + value
			// Check if this is an ADRP+LDR or ADRP+ADD by inspecting the second instruction
			secondInst := common.GetU32(g.Code()[codeOffset+4:])
			if secondInst&0xFFC00000 == 0xF9400000 {
				g.PatchAdrpLdr(codeOffset, pcAddr, targetAddr)
			} else {
				g.PatchAdrpAdd(codeOffset, pcAddr, targetAddr)
			}
		case "$got_addr$":
			targetAddr := gotSectionVAddr + value
			g.PatchAdrpLdr(codeOffset, pcAddr, targetAddr)
		}
	}

	// Entry point: LC_MAIN entryoff is offset from start of __TEXT segment (= file offset 0)
	entryOff := uint64(textSectionOff)

	// Build the binary
	bin := make([]byte, totalFileSize)

	// === Mach-O Header (32 bytes) ===
	common.PutU32(bin[0:], 0xFEEDFACF)
	common.PutU32(bin[4:], 0x0100000C) // CPU_TYPE_ARM64
	common.PutU32(bin[8:], 0x00000000) // CPU_SUBTYPE_ALL
	common.PutU32(bin[12:], 0x02)      // MH_EXECUTE
	common.PutU32(bin[16:], uint32(ncmds))
	common.PutU32(bin[20:], uint32(lcTotal))
	common.PutU32(bin[24:], 0x00200085) // MH_NOUNDEFS|MH_DYLDLINK|MH_TWOLEVEL|MH_PIE
	common.PutU32(bin[28:], 0)

	off := 32

	// LC_SEGMENT_64: __PAGEZERO
	common.PutU32(bin[off:], 0x19)
	common.PutU32(bin[off+4:], uint32(lcSegSize))
	copy(bin[off+8:], "__PAGEZERO\x00\x00\x00\x00\x00\x00")
	common.PutU64(bin[off+24:], 0)
	common.PutU64(bin[off+32:], pagezeroVMSize)
	off += lcSegSize

	// LC_SEGMENT_64: __TEXT (fileoff=0, covers header + code + rodata)
	textSegCmdSize := lcSegSize + 2*lcSectSize
	common.PutU32(bin[off:], 0x19)
	common.PutU32(bin[off+4:], uint32(textSegCmdSize))
	copy(bin[off+8:], "__TEXT\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	common.PutU64(bin[off+24:], textSegVAddr)
	common.PutU64(bin[off+32:], textSegVMSize)
	common.PutU64(bin[off+40:], 0)                  // fileoff = 0
	common.PutU64(bin[off+48:], uint64(textSegEnd)) // filesize
	common.PutU32(bin[off+56:], 5)                  // maxprot: r-x
	common.PutU32(bin[off+60:], 5)                  // initprot: r-x
	common.PutU32(bin[off+64:], 2)                  // nsects
	common.PutU32(bin[off+68:], 0)
	off += lcSegSize

	// Section: __text
	copy(bin[off:], "__text\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	copy(bin[off+16:], "__TEXT\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	common.PutU64(bin[off+32:], textSectionVAddr)
	common.PutU64(bin[off+40:], uint64(textSize))
	common.PutU32(bin[off+48:], uint32(textSectionOff))
	common.PutU32(bin[off+52:], 2) // align 2^2=4
	common.PutU32(bin[off+64:], 0x80000400)
	off += lcSectSize

	// Section: __const
	copy(bin[off:], "__const\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	copy(bin[off+16:], "__TEXT\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	common.PutU64(bin[off+32:], constSectionVAddr)
	common.PutU64(bin[off+40:], uint64(rodataSize))
	common.PutU32(bin[off+48:], uint32(constSectionOff))
	common.PutU32(bin[off+52:], 3)
	common.PutU32(bin[off+64:], 0)
	off += lcSectSize

	// LC_SEGMENT_64: __DATA
	dataSegCmdSize := lcSegSize + 2*lcSectSize
	common.PutU32(bin[off:], 0x19)
	common.PutU32(bin[off+4:], uint32(dataSegCmdSize))
	copy(bin[off+8:], "__DATA\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	common.PutU64(bin[off+24:], dataSegVAddr)
	common.PutU64(bin[off+32:], dataSegVMSize)
	common.PutU64(bin[off+40:], uint64(dataSegStart))
	common.PutU64(bin[off+48:], uint64(dataSegEnd-dataSegStart))
	common.PutU32(bin[off+56:], 3) // maxprot: rw-
	common.PutU32(bin[off+60:], 3) // initprot: rw-
	common.PutU32(bin[off+64:], 2) // nsects
	off += lcSegSize

	// Section: __data
	copy(bin[off:], "__data\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	copy(bin[off+16:], "__DATA\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	common.PutU64(bin[off+32:], dataSectionVAddr)
	common.PutU64(bin[off+40:], uint64(dataSize))
	common.PutU32(bin[off+48:], uint32(dataSectionOff))
	common.PutU32(bin[off+52:], 3)
	common.PutU32(bin[off+64:], 0)
	off += lcSectSize

	// Section: __got
	copy(bin[off:], "__got\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	copy(bin[off+16:], "__DATA\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
	common.PutU64(bin[off+32:], gotSectionVAddr)
	common.PutU64(bin[off+40:], uint64(gotSize))
	common.PutU32(bin[off+48:], uint32(gotSectionOff))
	common.PutU32(bin[off+52:], 3)
	common.PutU32(bin[off+64:], 0x06) // S_NON_LAZY_SYMBOL_POINTERS
	off += lcSectSize

	// LC_SEGMENT_64: __LINKEDIT
	common.PutU32(bin[off:], 0x19)
	common.PutU32(bin[off+4:], uint32(lcSegSize))
	copy(bin[off+8:], "__LINKEDIT\x00\x00\x00\x00\x00\x00")
	common.PutU64(bin[off+24:], linkeditVAddr)
	common.PutU64(bin[off+32:], linkeditVMSize)
	common.PutU64(bin[off+40:], uint64(linkeditStart))
	common.PutU64(bin[off+48:], uint64(linkeditEnd-linkeditStart))
	common.PutU32(bin[off+56:], 1) // maxprot: r--
	common.PutU32(bin[off+60:], 1) // initprot: r--
	off += lcSegSize

	// LC_LOAD_DYLINKER
	common.PutU32(bin[off:], 0x0E)
	common.PutU32(bin[off+4:], uint32(lcDylinkerSize))
	common.PutU32(bin[off+8:], 12)
	copy(bin[off+12:], dylinkerPath)
	off += lcDylinkerSize

	// LC_LOAD_DYLIB
	common.PutU32(bin[off:], 0x0C)
	common.PutU32(bin[off+4:], uint32(lcDylibSize))
	common.PutU32(bin[off+8:], 24)
	common.PutU32(bin[off+12:], 2)        // timestamp
	common.PutU32(bin[off+16:], 0x010000) // current_version
	common.PutU32(bin[off+20:], 0x010000) // compat_version
	copy(bin[off+24:], dylibPath)
	off += lcDylibSize

	// LC_MAIN
	common.PutU32(bin[off:], 0x80000028)
	common.PutU32(bin[off+4:], uint32(lcMainSize))
	common.PutU64(bin[off+8:], entryOff)
	common.PutU64(bin[off+16:], 0) // stacksize
	off += lcMainSize

	if !g.Target().StripBinary {
		// LC_SYMTAB
		common.PutU32(bin[off:], 0x02)
		common.PutU32(bin[off+4:], uint32(lcSymtabSize))
		common.PutU32(bin[off+8:], uint32(symtabOff))
		common.PutU32(bin[off+12:], uint32(symtabNEntries))
		common.PutU32(bin[off+16:], uint32(strtabOff))
		common.PutU32(bin[off+20:], uint32(strtabSize))
		off += lcSymtabSize

		// LC_DYSYMTAB
		common.PutU32(bin[off:], 0x0B)
		common.PutU32(bin[off+4:], uint32(lcDysymtabSize))
		// ilocalsym = 0, nlocalsym = number of local symbols
		nLocalSyms := 0
		nExtSyms := 0
		for _, sym := range syms {
			if sym.ntype&0x01 != 0 { // N_EXT
				nExtSyms++
			} else {
				nLocalSyms++
			}
		}
		common.PutU32(bin[off+8:], 0)                       // ilocalsym
		common.PutU32(bin[off+12:], uint32(nLocalSyms))     // nlocalsym
		common.PutU32(bin[off+16:], uint32(nLocalSyms))     // iextdefsym (starts after locals)
		common.PutU32(bin[off+20:], uint32(nExtSyms))       // nextdefsym
		common.PutU32(bin[off+24:], uint32(symtabNEntries)) // iundefsym (no undefs)
		common.PutU32(bin[off+28:], 0)                      // nundefsym
		off += lcDysymtabSize
	}

	// LC_DYLD_INFO_ONLY
	common.PutU32(bin[off:], 0x80000022)
	common.PutU32(bin[off+4:], uint32(lcDyldInfoSize))
	// bind info
	common.PutU32(bin[off+16:], uint32(bindOff))
	common.PutU32(bin[off+20:], uint32(bindSize))
	// export info
	common.PutU32(bin[off+40:], uint32(exportOff))
	common.PutU32(bin[off+44:], uint32(exportSize))
	off += lcDyldInfoSize

	// LC_CODE_SIGNATURE
	common.PutU32(bin[off:], 0x1d)
	common.PutU32(bin[off+4:], uint32(lcCodeSigSize))
	common.PutU32(bin[off+8:], uint32(codeSignOff))
	common.PutU32(bin[off+12:], uint32(sigSize))
	off += lcCodeSigSize

	_ = off

	// Copy section data
	copy(bin[textSectionOff:], g.Code())
	copy(bin[constSectionOff:], g.Rodata())
	copy(bin[dataSectionOff:], g.Data())

	// __LINKEDIT content
	copy(bin[bindOff:], bindOpcodes)
	copy(bin[exportOff:], exportTrie)

	if !g.Target().StripBinary {
		// Symbol table: nlist_64 entries sorted with locals first, then externals
		// Sort: locals first, then externals (required by LC_DYSYMTAB)
		symOff := symtabOff
		// Write local symbols first
		for _, sym := range syms {
			if sym.ntype&0x01 != 0 { // N_EXT — skip for now
				continue
			}
			nlist := bin[symOff:]
			common.PutU32(nlist[0:], uint32(sym.nameOff))
			nlist[4] = sym.ntype
			nlist[5] = 1 // n_sect: 1 = __text
			common.PutU64(nlist[8:], textSectionVAddr+sym.value)
			symOff += nlistSize
		}
		// Write external symbols
		for _, sym := range syms {
			if sym.ntype&0x01 == 0 { // not N_EXT — skip
				continue
			}
			nlist := bin[symOff:]
			common.PutU32(nlist[0:], uint32(sym.nameOff))
			nlist[4] = sym.ntype
			nlist[5] = 1 // n_sect: 1 = __text
			common.PutU64(nlist[8:], textSectionVAddr+sym.value)
			symOff += nlistSize
		}

		// String table
		copy(bin[strtabOff:], strtab)
	}

	// Compute and embed ad-hoc code signature.
	codeSignEnd := codeSignOff + sigSize
	codesign.CodeSign(bin[codeSignOff:codeSignEnd], bin[0:codeSignOff], int64(codeSignOff), 0, int64(textSegEnd), true, codeSignID)

	return bin
}

// buildExportTrie builds a minimal export trie containing just _main.
func buildExportTrie(mainEntryOff uint64) []byte {
	// Terminal info for _main: flags=0 (regular), address=entryOff
	terminalInfo := encodeULEB128(0) // flags = EXPORT_SYMBOL_FLAGS_KIND_REGULAR
	terminalInfo = append(terminalInfo, encodeULEB128(mainEntryOff)...)

	// Terminal node: [terminal_size, terminal_info..., child_count=0]
	var termNode []byte
	termNode = append(termNode, byte(len(terminalInfo)))
	termNode = append(termNode, terminalInfo...)
	termNode = append(termNode, 0) // no children

	// Root node partial: [terminal_size=0, child_count=1, "_main\0"]
	// Then append ULEB128(child_offset) where child_offset = len(root_node)
	rootPartial := []byte{0, 1} // terminal_size=0, child_count=1
	rootPartial = append(rootPartial, []byte("_main")...)
	rootPartial = append(rootPartial, 0) // null terminator

	// child_offset = len(rootPartial) + len(ULEB128(child_offset))
	// Since rootPartial is 8 bytes and child_offset < 128, ULEB is 1 byte → offset = 9
	childOffset := len(rootPartial) + 1 // 8 + 1 = 9
	rootPartial = append(rootPartial, encodeULEB128(uint64(childOffset))...)

	var trie []byte
	trie = append(trie, rootPartial...)
	trie = append(trie, termNode...)
	return trie
}

// buildRebaseOpcodes generates DYLD rebase opcodes for pointers in __DATA
// that need to be adjusted by the ASLR slide. This includes string header
// data_ptr fields that point into __TEXT,__const.
// buildBindOpcodes generates DYLD bind opcodes for the GOT entries.
func buildBindOpcodes(g *aarch64.CodeGen) []byte {
	var ops []byte

	dataSegIdx := 2
	dataSize := common.AlignUp(len(g.Data()), 8)
	gotOffsetInDataSeg := dataSize

	for i, sym := range g.GotSymbols() {
		ops = append(ops, 0x10|1) // SET_DYLIB_ORDINAL_IMM(1)

		ops = append(ops, 0x40) // SET_SYMBOL_TRAILING_FLAGS_IMM(0)
		ops = append(ops, []byte(sym)...)
		ops = append(ops, 0)

		ops = append(ops, 0x50|1) // SET_TYPE_IMM(BIND_TYPE_POINTER)

		segOffset := gotOffsetInDataSeg + i*8
		ops = append(ops, 0x70|byte(dataSegIdx)) // SET_SEGMENT_AND_OFFSET_ULEB
		ops = append(ops, encodeULEB128(uint64(segOffset))...)

		ops = append(ops, 0x90) // DO_BIND
	}

	ops = append(ops, 0x00) // DONE
	return ops
}

// encodeULEB128 encodes a uint64 as ULEB128.
func encodeULEB128(val uint64) []byte {
	var result []byte
	for {
		b := byte(val & 0x7F)
		val = val >> 7
		if val != 0 {
			b |= 0x80
		}
		result = append(result, b)
		if val == 0 {
			break
		}
	}
	return result
}
