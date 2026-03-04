//go:build !no_backend_arm64

package ccmetal

import "j5.nz/rtg/std/compiler/backend/aarch64"

func alignUp(v int, align int) int {
	return (v + align - 1) & ^(align - 1)
}

// BuildFlatBinary emits a raw image with [text][rodata][data] layout.
func BuildFlatBinary(g *aarch64.CodeGen) []byte {
	textOffset := 0
	textSize := len(g.Code())
	rodataOffset := alignUp(textOffset+textSize, 8)
	rodataSize := len(g.Rodata())
	dataOffset := alignUp(rodataOffset+rodataSize, 8)
	dataSize := len(g.Data())
	totalSize := dataOffset + dataSize

	textVAddr := g.BaseAddr() + uint64(textOffset)
	rodataVAddr := g.BaseAddr() + uint64(rodataOffset)
	dataVAddr := g.BaseAddr() + uint64(dataOffset)

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

	bin := make([]byte, totalSize)
	copy(bin[textOffset:], g.Code())
	copy(bin[rodataOffset:], g.Rodata())
	copy(bin[dataOffset:], g.Data())
	return bin
}
