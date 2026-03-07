//go:build !no_backend_linux_i386

package linux

import (
	"fmt"
	"os"

	core "j5.nz/rtg/std/compiler/backend/i386"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

const (
	elf32TypeRel     = 1
	elf32MachineI386 = 3

	shtNull     = 0
	shtProgbits = 1
	shtSymtab   = 2
	shtStrtab   = 3
	shtRel      = 9

	shfWrite     = 0x1
	shfAlloc     = 0x2
	shfExecinstr = 0x4

	stbLocal  = 0
	stbGlobal = 1
	sttObject = 1
	sttFunc   = 2
	sttSection = 3

	r386_32  = 1
	r386PC32 = 2
)

type elf32Rel struct {
	Offset uint32
	Sym    int
	Type   int
}

type elf32Sym struct {
	NameOff int
	Value   uint32
	Size    uint32
	Info    byte
	Other   byte
	Shndx   uint16
}

type elf32ShNames struct {
	Text      int
	Rodata    int
	Data      int
	RelText   int
	RelRodata int
	Symtab    int
	Strtab    int
	Shstrtab  int
}

func GenerateObjectELF(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	g := core.NewCodeGen(target, irmod, 0)
	g.CompileModuleFuncs(irmod)
	g.CollectNativeFuncSizes(irmod)
	if g.NeedTostringHelper {
		g.EmitTostringHelperI386()
	}
	obj, err := BuildELF32Object(g, irmod)
	if err != nil {
		return err
	}
	if err := os.WriteFile(outputPath, obj, 0644); err != nil {
		return fmt.Errorf("write output: %v", err)
	}
	return nil
}

func BuildELF32Object(g *core.CodeGen, irmod *ir.IRModule) ([]byte, error) {
	code := append([]byte{}, g.Code...)
	rodata := append([]byte{}, g.Rodata...)
	data := append([]byte{}, g.Data...)

	shstrtab, shNames := elf32ObjectShStrtab()
	strtab := []byte{0}

	var localSyms []elf32Sym
	var globalSyms []elf32Sym
	symIndex := 1
	localSyms = append(localSyms, elf32Sym{Info: byte(sttSection), Shndx: 1})
	symIndex++
	rodataSectionSym := symIndex
	localSyms = append(localSyms, elf32Sym{Info: byte(sttSection), Shndx: 2})
	symIndex++
	dataSectionSym := symIndex
	localSyms = append(localSyms, elf32Sym{Info: byte(sttSection), Shndx: 3})
	symIndex++

	var funcNames []string
	for name := range g.FuncOffsets {
		funcNames = append(funcNames, name)
	}
	sortStringSliceByI386Offset(funcNames, g.FuncOffsets)
	funcSymIndex := make(map[string]int)
	for i, name := range funcNames {
		start := g.FuncOffsets[name]
		end := len(code)
		if i+1 < len(funcNames) {
			end = g.FuncOffsets[funcNames[i+1]]
		}
		sym := elf32Sym{
			NameOff: appendELF32String(&strtab, name),
			Value:   uint32(start),
			Size:    uint32(end - start),
			Shndx:   1,
		}
		idx := symIndex
		symIndex++
		if len(name) > 0 && name[0] == '$' {
			sym.Info = byte((stbLocal << 4) | sttFunc)
			funcSymIndex[name] = idx
			localSyms = append(localSyms, sym)
		} else {
			sym.Info = byte((stbGlobal << 4) | sttFunc)
			funcSymIndex[name] = idx
			globalSyms = append(globalSyms, sym)
		}
	}

	for _, gl := range irmod.Globals {
		sym := elf32Sym{
			NameOff: appendELF32String(&strtab, gl.Name),
			Value:   uint32(gl.Index * g.SlotBytesI386()),
			Size:    uint32(g.SlotBytesI386()),
			Info:    byte((stbGlobal << 4) | sttObject),
			Shndx:   3,
		}
		symIndex++
		globalSyms = append(globalSyms, sym)
	}

	undefFuncSym := make(map[string]int)

	var relText []elf32Rel
	for _, fx := range g.CallFixups {
		switch {
		case fx.Target == "$rodata_header$":
			relText = append(relText, elf32Rel{Offset: uint32(fx.CodeOffset), Sym: rodataSectionSym, Type: r386_32})
		case fx.Target == "$data_addr$":
			relText = append(relText, elf32Rel{Offset: uint32(fx.CodeOffset), Sym: dataSectionSym, Type: r386_32})
		case len(fx.Target) > 10 && fx.Target[0:10] == "$funcaddr$":
			name := fx.Target[10:]
			sym := addELF32UndefinedFunc(undefFuncSym, &globalSyms, &strtab, &symIndex, name)
			if idx, ok := funcSymIndex[name]; ok {
				sym = idx
			}
			putU32(code[fx.CodeOffset:fx.CodeOffset+4], 0)
			relText = append(relText, elf32Rel{Offset: uint32(fx.CodeOffset), Sym: sym, Type: r386_32})
		default:
			if off, ok := g.FuncOffsets[fx.Target]; ok {
				rel := int32(off - (fx.CodeOffset + 4))
				putU32(code[fx.CodeOffset:fx.CodeOffset+4], uint32(rel))
				continue
			}
			putU32(code[fx.CodeOffset:fx.CodeOffset+4], 0xFFFFFFFC)
			sym := addELF32UndefinedFunc(undefFuncSym, &globalSyms, &strtab, &symIndex, fx.Target)
			relText = append(relText, elf32Rel{Offset: uint32(fx.CodeOffset), Sym: sym, Type: r386PC32})
		}
	}

	var relRodata []elf32Rel
	var headerOffs []int
	for _, off := range g.StringMap {
		headerOffs = append(headerOffs, off)
	}
	sortI386Ints(headerOffs)
	for _, headerOff := range headerOffs {
		relRodata = append(relRodata, elf32Rel{Offset: uint32(headerOff), Sym: rodataSectionSym, Type: r386_32})
	}

	symtab := make([]byte, (1+len(localSyms)+len(globalSyms))*16)
	writeSym := func(idx int, sym elf32Sym) {
		off := idx * 16
		putU32(symtab[off:], uint32(sym.NameOff))
		putU32(symtab[off+4:], sym.Value)
		putU32(symtab[off+8:], sym.Size)
		symtab[off+12] = sym.Info
		symtab[off+13] = sym.Other
		putU16(symtab[off+14:], sym.Shndx)
	}
	nextSym := 1
	for _, sym := range localSyms {
		writeSym(nextSym, sym)
		nextSym++
	}
	firstGlobalSym := nextSym
	for _, sym := range globalSyms {
		writeSym(nextSym, sym)
		nextSym++
	}

	relTextBytes := buildELF32Relocs(relText)
	relRodataBytes := buildELF32Relocs(relRodata)

	elfHeaderSize := 52
	textOffset := common.AlignUp(elfHeaderSize, 16)
	rodataOffset := common.AlignUp(textOffset+len(code), 4)
	dataOffset := common.AlignUp(rodataOffset+len(rodata), 4)
	relTextOffset := common.AlignUp(dataOffset+len(data), 4)
	relRodataOffset := common.AlignUp(relTextOffset+len(relTextBytes), 4)
	symtabOffset := common.AlignUp(relRodataOffset+len(relRodataBytes), 4)
	strtabOffset := common.AlignUp(symtabOffset+len(symtab), 4)
	shstrtabOffset := strtabOffset + len(strtab)
	shdrOffset := common.AlignUp(shstrtabOffset+len(shstrtab), 4)

	sectionCount := 9
	shdrSize := 40
	totalSize := shdrOffset + sectionCount*shdrSize
	elf := make([]byte, totalSize)

	elf[0] = 0x7f
	elf[1] = 'E'
	elf[2] = 'L'
	elf[3] = 'F'
	elf[4] = 1
	elf[5] = 1
	elf[6] = 1
	putU16(elf[16:], elf32TypeRel)
	putU16(elf[18:], elf32MachineI386)
	putU32(elf[20:], 1)
	putU32(elf[24:], 0)
	putU32(elf[28:], 0)
	putU32(elf[32:], uint32(shdrOffset))
	putU32(elf[36:], 0)
	putU16(elf[40:], uint16(elfHeaderSize))
	putU16(elf[42:], 0)
	putU16(elf[44:], 0)
	putU16(elf[46:], uint16(shdrSize))
	putU16(elf[48:], uint16(sectionCount))
	putU16(elf[50:], 8)

	copy(elf[textOffset:], code)
	copy(elf[rodataOffset:], rodata)
	copy(elf[dataOffset:], data)
	copy(elf[relTextOffset:], relTextBytes)
	copy(elf[relRodataOffset:], relRodataBytes)
	copy(elf[symtabOffset:], symtab)
	copy(elf[strtabOffset:], strtab)
	copy(elf[shstrtabOffset:], shstrtab)

	shdr := elf[shdrOffset:]
	writeShdr := func(idx int, name, typ, flags, addr, off, size, link, info, addralign, entsize uint32) {
		s := shdr[idx*shdrSize:]
		putU32(s[0:], name)
		putU32(s[4:], typ)
		putU32(s[8:], flags)
		putU32(s[12:], addr)
		putU32(s[16:], off)
		putU32(s[20:], size)
		putU32(s[24:], link)
		putU32(s[28:], info)
		putU32(s[32:], addralign)
		putU32(s[36:], entsize)
	}

	writeShdr(1, uint32(shNames.Text), shtProgbits, shfAlloc|shfExecinstr, 0, uint32(textOffset), uint32(len(code)), 0, 0, 16, 0)
	writeShdr(2, uint32(shNames.Rodata), shtProgbits, shfAlloc, 0, uint32(rodataOffset), uint32(len(rodata)), 0, 0, 4, 0)
	writeShdr(3, uint32(shNames.Data), shtProgbits, shfAlloc|shfWrite, 0, uint32(dataOffset), uint32(len(data)), 0, 0, 4, 0)
	writeShdr(4, uint32(shNames.RelText), shtRel, 0, 0, uint32(relTextOffset), uint32(len(relTextBytes)), 6, 1, 4, 8)
	writeShdr(5, uint32(shNames.RelRodata), shtRel, 0, 0, uint32(relRodataOffset), uint32(len(relRodataBytes)), 6, 2, 4, 8)
	writeShdr(6, uint32(shNames.Symtab), shtSymtab, 0, 0, uint32(symtabOffset), uint32(len(symtab)), 7, uint32(firstGlobalSym), 4, 16)
	writeShdr(7, uint32(shNames.Strtab), shtStrtab, 0, 0, uint32(strtabOffset), uint32(len(strtab)), 0, 0, 1, 0)
	writeShdr(8, uint32(shNames.Shstrtab), shtStrtab, 0, 0, uint32(shstrtabOffset), uint32(len(shstrtab)), 0, 0, 1, 0)

	return elf, nil
}

func buildELF32Relocs(rels []elf32Rel) []byte {
	out := make([]byte, len(rels)*8)
	for i, rel := range rels {
		off := i * 8
		putU32(out[off:], rel.Offset)
		putU32(out[off+4:], uint32((rel.Sym<<8)|(rel.Type&0xff)))
	}
	return out
}

func elf32ObjectShStrtab() ([]byte, elf32ShNames) {
	return []byte("\x00.text\x00.rodata\x00.data\x00.rel.text\x00.rel.rodata\x00.symtab\x00.strtab\x00.shstrtab\x00"), elf32ShNames{
		Text:      1,
		Rodata:    7,
		Data:      15,
		RelText:   21,
		RelRodata: 31,
		Symtab:    43,
		Strtab:    51,
		Shstrtab:  59,
	}
}

func appendELF32String(strtab *[]byte, name string) int {
	off := len(*strtab)
	*strtab = append(*strtab, []byte(name)...)
	*strtab = append(*strtab, 0)
	return off
}

func addELF32UndefinedFunc(undef map[string]int, globalSyms *[]elf32Sym, strtab *[]byte, symIndex *int, name string) int {
	if idx, ok := undef[name]; ok {
		return idx
	}
	sym := elf32Sym{
		NameOff: appendELF32String(strtab, name),
		Info:    byte((stbGlobal << 4) | sttFunc),
		Shndx:   0,
	}
	idx := *symIndex
	*symIndex = *symIndex + 1
	undef[name] = idx
	*globalSyms = append(*globalSyms, sym)
	return idx
}

func putU16(buf []byte, v uint16) {
	buf[0] = byte(v)
	buf[1] = byte(v >> 8)
}

func putU32(buf []byte, v uint32) {
	buf[0] = byte(v)
	buf[1] = byte(v >> 8)
	buf[2] = byte(v >> 16)
	buf[3] = byte(v >> 24)
}

func sortStringSliceByI386Offset(names []string, offsets map[string]int) {
	for i := 1; i < len(names); i++ {
		j := i
		for j > 0 {
			aj := offsets[names[j-1]]
			bj := offsets[names[j]]
			if aj < bj || (aj == bj && names[j-1] <= names[j]) {
				break
			}
			names[j-1], names[j] = names[j], names[j-1]
			j--
		}
	}
}

func sortI386Ints(values []int) {
	for i := 1; i < len(values); i++ {
		j := i
		for j > 0 && values[j-1] > values[j] {
			values[j-1], values[j] = values[j], values[j-1]
			j--
		}
	}
}
