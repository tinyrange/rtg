package pe

import (
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

type ImportGroup struct {
	Library string
	Symbols []string
}

type DwarfSymbol struct {
	Name   string
	LowPC  int
	HighPC int
}

func FormatSlashOffset(n int) []byte {
	if n == 0 {
		return []byte("/0")
	}
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

func WriteSection(buf []byte, name string, virtualSize, rva, rawSize, fileOff int, characteristics uint32) {
	i := 0
	for i < len(name) && i < 8 {
		buf[i] = name[i]
		i++
	}
	common.PutU32(buf[8:], uint32(virtualSize))
	common.PutU32(buf[12:], uint32(rva))
	common.PutU32(buf[16:], uint32(rawSize))
	common.PutU32(buf[20:], uint32(fileOff))
	common.PutU32(buf[24:], 0)
	common.PutU32(buf[28:], 0)
	common.PutU16(buf[32:], 0)
	common.PutU16(buf[34:], 0)
	common.PutU32(buf[36:], characteristics)
}

func WriteSectionLongName(buf []byte, strtabOffset int, virtualSize, rva, rawSize, fileOff int, characteristics uint32) {
	s := FormatSlashOffset(strtabOffset)
	i := 0
	for i < len(s) && i < 8 {
		buf[i] = s[i]
		i++
	}
	common.PutU32(buf[8:], uint32(virtualSize))
	common.PutU32(buf[12:], uint32(rva))
	common.PutU32(buf[16:], uint32(rawSize))
	common.PutU32(buf[20:], uint32(fileOff))
	common.PutU32(buf[24:], 0)
	common.PutU32(buf[28:], 0)
	common.PutU16(buf[32:], 0)
	common.PutU16(buf[34:], 0)
	common.PutU32(buf[36:], characteristics)
}

func MakeCOFFSym(name []byte, value uint32, section uint16, symType uint16, storageClass byte) []byte {
	sym := make([]byte, 18)
	if len(name) <= 8 {
		i := 0
		for i < len(name) && i < 8 {
			sym[i] = name[i]
			i++
		}
	} else {
		common.PutU32(sym[0:], 0)
		common.PutU32(sym[4:], 0)
	}
	common.PutU32(sym[8:], value)
	common.PutU16(sym[12:], section)
	common.PutU16(sym[14:], symType)
	sym[16] = storageClass
	sym[17] = 0
	return sym
}

func BuildCOFFSymbols(irmod *ir.IRModule, funcOffsets map[string]int) ([]byte, []byte, int) {
	var coffSyms []byte
	var coffStrtab []byte
	coffStrtab = append(coffStrtab, 0, 0, 0, 0)
	numSyms := 0

	sym := MakeCOFFSym([]byte("_start"), 0, 1, 0x20, 2)
	coffSyms = append(coffSyms, sym...)
	numSyms++

	for _, f := range irmod.Funcs {
		funcOff, ok := funcOffsets[f.Name]
		if !ok {
			continue
		}
		nameBytes := []byte(f.Name)
		sym = MakeCOFFSym(nameBytes, uint32(funcOff), 1, 0x20, 2)
		if len(nameBytes) > 8 {
			common.PutU32(sym[4:], uint32(len(coffStrtab)))
			coffStrtab = append(coffStrtab, nameBytes...)
			coffStrtab = append(coffStrtab, 0)
		}
		coffSyms = append(coffSyms, sym...)
		numSyms++
	}

	common.PutU32(coffStrtab[0:], uint32(len(coffStrtab)))
	return coffSyms, coffStrtab, numSyms
}

func ImportKey(lib string, sym string) string {
	return lib + "|" + sym
}

func GetImportDirInfo(groups []ImportGroup, idataRVA int) (int, int) {
	if len(groups) == 0 {
		return 0, 0
	}
	return idataRVA, (len(groups) + 1) * 20
}

func idataOffsetAfterIAT(groups []ImportGroup, thunkBytes int) int {
	idtSize := (len(groups) + 1) * 20
	iltSize := 0
	iatSize := 0
	for _, grp := range groups {
		iltSize += (len(grp.Symbols) + 1) * thunkBytes
		iatSize += (len(grp.Symbols) + 1) * thunkBytes
	}
	return idtSize + iltSize + iatSize
}

func IdataOffsetAfterIAT32(groups []ImportGroup) int {
	return idataOffsetAfterIAT(groups, 4)
}

func IdataOffsetAfterIAT64(groups []ImportGroup) int {
	return idataOffsetAfterIAT(groups, 8)
}

func buildIData(groups []ImportGroup, thunkBytes int) []byte {
	numLibs := len(groups)
	idtSize := (numLibs + 1) * 20

	iltOffsets := make([]int, numLibs)
	iatOffsets := make([]int, numLibs)
	iltSize := 0
	for i, grp := range groups {
		iltOffsets[i] = idtSize + iltSize
		iltSize += (len(grp.Symbols) + 1) * thunkBytes
	}
	iatBase := idtSize + iltSize
	iatSize := 0
	for i, grp := range groups {
		iatOffsets[i] = iatBase + iatSize
		iatSize += (len(grp.Symbols) + 1) * thunkBytes
	}

	hntOffset := idataOffsetAfterIAT(groups, thunkBytes)
	var hntEntries []byte
	hntOffsets := make(map[string]int)
	for _, grp := range groups {
		for _, sym := range grp.Symbols {
			off := hntOffset + len(hntEntries)
			hntOffsets[ImportKey(grp.Library, sym)] = off
			hntEntries = append(hntEntries, 0, 0)
			hntEntries = append(hntEntries, []byte(sym)...)
			hntEntries = append(hntEntries, 0)
			if len(hntEntries)%2 != 0 {
				hntEntries = append(hntEntries, 0)
			}
		}
	}

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

	for i, grp := range groups {
		base := i * 20
		common.PutU32(idata[base+0:], uint32(iltOffsets[i]))
		common.PutU32(idata[base+4:], 0)
		common.PutU32(idata[base+8:], 0)
		common.PutU32(idata[base+12:], uint32(dllOffsets[i]))
		common.PutU32(idata[base+16:], uint32(iatOffsets[i]))

		for j, sym := range grp.Symbols {
			key := ImportKey(grp.Library, sym)
			hnt := hntOffsets[key]
			offILT := iltOffsets[i] + j*thunkBytes
			offIAT := iatOffsets[i] + j*thunkBytes
			if thunkBytes == 8 {
				common.PutU64(idata[offILT:], uint64(hnt))
				common.PutU64(idata[offIAT:], uint64(hnt))
			} else {
				common.PutU32(idata[offILT:], uint32(hnt))
				common.PutU32(idata[offIAT:], uint32(hnt))
			}
		}
	}

	copy(idata[hntOffset:], hntEntries)
	copy(idata[dllNameOffset:], dllEntries)
	return idata
}

func BuildIData32(groups []ImportGroup) []byte {
	return buildIData(groups, 4)
}

func BuildIData64(groups []ImportGroup) []byte {
	return buildIData(groups, 8)
}

func fixupIData(idata []byte, idataRVA int, groups []ImportGroup, thunkBytes int) {
	numLibs := len(groups)
	idtSize := (numLibs + 1) * 20

	iltSize := 0
	for _, grp := range groups {
		iltSize += (len(grp.Symbols) + 1) * thunkBytes
	}
	iatBase := idtSize + iltSize

	for i := 0; i < numLibs; i++ {
		base := i * 20
		common.PutU32(idata[base+0:], uint32(idataRVA)+common.GetU32(idata[base+0:base+4]))
		common.PutU32(idata[base+12:], uint32(idataRVA)+common.GetU32(idata[base+12:base+16]))
		common.PutU32(idata[base+16:], uint32(idataRVA)+common.GetU32(idata[base+16:base+20]))
	}

	iltOff := idtSize
	for _, grp := range groups {
		for i := 0; i < len(grp.Symbols); i++ {
			off := iltOff + i*thunkBytes
			if thunkBytes == 8 {
				common.PutU64(idata[off:], uint64(idataRVA)+common.GetU64(idata[off:off+8]))
			} else {
				common.PutU32(idata[off:], uint32(idataRVA)+common.GetU32(idata[off:off+4]))
			}
		}
		iltOff += (len(grp.Symbols) + 1) * thunkBytes
	}

	iatOff := iatBase
	for _, grp := range groups {
		for i := 0; i < len(grp.Symbols); i++ {
			off := iatOff + i*thunkBytes
			if thunkBytes == 8 {
				common.PutU64(idata[off:], uint64(idataRVA)+common.GetU64(idata[off:off+8]))
			} else {
				common.PutU32(idata[off:], uint32(idataRVA)+common.GetU32(idata[off:off+4]))
			}
		}
		iatOff += (len(grp.Symbols) + 1) * thunkBytes
	}
}

func FixupIData32(idata []byte, idataRVA int, groups []ImportGroup) {
	fixupIData(idata, idataRVA, groups, 4)
}

func FixupIData64(idata []byte, idataRVA int, groups []ImportGroup) {
	fixupIData(idata, idataRVA, groups, 8)
}

func buildIATOffsets(groups []ImportGroup, thunkBytes int) map[string]int {
	idtSize := (len(groups) + 1) * 20
	iltSize := 0
	for _, grp := range groups {
		iltSize += (len(grp.Symbols) + 1) * thunkBytes
	}
	iatBase := idtSize + iltSize

	offsets := make(map[string]int)
	cur := iatBase
	for _, grp := range groups {
		for i, sym := range grp.Symbols {
			offsets[ImportKey(grp.Library, sym)] = cur + i*thunkBytes
		}
		cur += (len(grp.Symbols) + 1) * thunkBytes
	}
	return offsets
}

func BuildIATOffsets32(groups []ImportGroup) map[string]int {
	return buildIATOffsets(groups, 4)
}

func BuildIATOffsets64(groups []ImportGroup) map[string]int {
	return buildIATOffsets(groups, 8)
}

func getIATInfo(groups []ImportGroup, idataRVA int, thunkBytes int) (int, int) {
	if len(groups) == 0 {
		return 0, 0
	}
	idtSize := (len(groups) + 1) * 20
	iltSize := 0
	iatSize := 0
	for _, grp := range groups {
		iltSize += (len(grp.Symbols) + 1) * thunkBytes
		iatSize += (len(grp.Symbols) + 1) * thunkBytes
	}
	return idataRVA + idtSize + iltSize, iatSize
}

func GetIATInfo32(groups []ImportGroup, idataRVA int) (int, int) {
	return getIATInfo(groups, idataRVA, 4)
}

func GetIATInfo64(groups []ImportGroup, idataRVA int) (int, int) {
	return getIATInfo(groups, idataRVA, 8)
}

func BuildDWARFSymbols(irmod *ir.IRModule, textStart int, textEnd int, funcOffsets map[string]int) []DwarfSymbol {
	symbols := make([]DwarfSymbol, 0, len(irmod.Funcs)+1)
	spans := ir.ComputeNativeFuncSpans(irmod, funcOffsets, textEnd-textStart)

	startHighPC := textEnd
	if off, ok := ir.FirstNativeFuncOffset(irmod, funcOffsets, textEnd-textStart); ok {
		startHighPC = textStart + off
	}
	symbols = append(symbols, DwarfSymbol{Name: "_start", LowPC: textStart, HighPC: startHighPC})

	for _, f := range irmod.Funcs {
		span, ok := spans[f.Name]
		if !ok {
			continue
		}
		symbols = append(symbols, DwarfSymbol{Name: f.Name, LowPC: textStart + span.Start, HighPC: textStart + span.End})
	}

	return symbols
}

func BuildDWARF32(textStart int, textEnd int, symbols []DwarfSymbol) ([]byte, []byte) {
	var abbrev []byte
	abbrev = append(abbrev, 1, 0x11, 1, 0x03, 0x08, 0x11, 0x01, 0x12, 0x01, 0, 0)
	abbrev = append(abbrev, 2, 0x2e, 0, 0x03, 0x08, 0x11, 0x01, 0x12, 0x01, 0, 0)
	abbrev = append(abbrev, 0)

	var info []byte
	info = append(info, 0, 0, 0, 0)
	info = append(info, 2, 0)
	info = append(info, 0, 0, 0, 0)
	info = append(info, 4)

	info = append(info, 1)
	info = append(info, []byte("rtg")...)
	info = append(info, 0)
	info = appendU32(info, uint32(textStart))
	info = appendU32(info, uint32(textEnd))

	for _, sym := range symbols {
		info = append(info, 2)
		info = append(info, []byte(sym.Name)...)
		info = append(info, 0)
		info = appendU32(info, uint32(sym.LowPC))
		info = appendU32(info, uint32(sym.HighPC))
	}

	info = append(info, 0)
	unitLen := len(info) - 4
	common.PutU32(info[0:], uint32(unitLen))
	return abbrev, info
}

func BuildDWARF64(textStart int, textEnd int, symbols []DwarfSymbol) ([]byte, []byte) {
	var abbrev []byte
	abbrev = append(abbrev, 1, 0x11, 1, 0x03, 0x08, 0x11, 0x01, 0x12, 0x01, 0, 0)
	abbrev = append(abbrev, 2, 0x2e, 0, 0x03, 0x08, 0x11, 0x01, 0x12, 0x01, 0, 0)
	abbrev = append(abbrev, 0)

	var info []byte
	info = append(info, 0, 0, 0, 0)
	info = append(info, 2, 0)
	info = append(info, 0, 0, 0, 0)
	info = append(info, 8)

	info = append(info, 1)
	info = append(info, []byte("rtg")...)
	info = append(info, 0)
	info = appendU64(info, uint64(textStart))
	info = appendU64(info, uint64(textEnd))

	for _, sym := range symbols {
		info = append(info, 2)
		info = append(info, []byte(sym.Name)...)
		info = append(info, 0)
		info = appendU64(info, uint64(sym.LowPC))
		info = appendU64(info, uint64(sym.HighPC))
	}

	info = append(info, 0)
	unitLen := len(info) - 4
	common.PutU32(info[0:], uint32(unitLen))
	return abbrev, info
}

func SortOffsets(offsets []int) {
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

func BuildBaseRelocations64(sectionRVA int, offsets []int) []byte {
	if len(offsets) == 0 {
		return nil
	}

	var reloc []byte
	i := 0
	for i < len(offsets) {
		rva := sectionRVA + offsets[i]
		pageRVA := (rva / 0x1000) * 0x1000

		blockStart := len(reloc)
		reloc = append(reloc, 0, 0, 0, 0)
		reloc = append(reloc, 0, 0, 0, 0)

		for i < len(offsets) {
			rva = sectionRVA + offsets[i]
			if (rva/0x1000)*0x1000 != pageRVA {
				break
			}
			offsetInPage := rva % 0x1000
			entry := uint16(0xA000) | uint16(offsetInPage)
			reloc = append(reloc, byte(entry), byte(entry>>8))
			i++
		}

		blockSize := len(reloc) - blockStart
		if blockSize%4 != 0 {
			reloc = append(reloc, 0, 0)
			blockSize += 2
		}

		common.PutU32(reloc[blockStart:], uint32(pageRVA))
		common.PutU32(reloc[blockStart+4:], uint32(blockSize))
	}

	return reloc
}

func appendU32(b []byte, v uint32) []byte {
	b = append(b, byte(v))
	b = append(b, byte(v>>8))
	b = append(b, byte(v>>16))
	b = append(b, byte(v>>24))
	return b
}

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
