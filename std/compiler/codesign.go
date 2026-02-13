//go:build !no_backend_darwin_arm64

package main


// Code signature layout for ad-hoc signing of Mach-O files.
// Adapted from the Go toolchain's codesign package.
// Includes embedded SHA-256 to avoid cross-package method call issues in rtg.

const (
	csPageSizeBits = 12
	csPageSize     = 1 << csPageSizeBits
	csHashSize     = 32
)

const (
	CSMAGIC_CODEDIRECTORY      = 0xfade0c02
	CSMAGIC_EMBEDDED_SIGNATURE = 0xfade0cc0
	CSMAGIC_REQUIREMENTS       = 0xfade0c01
	CSMAGIC_BLOBWRAPPER        = 0xfade0b01
)

const (
	CSSLOT_CODEDIRECTORY = 0x00000
	CSSLOT_REQUIREMENTS  = 0x00002
	CSSLOT_SIGNATURESLOT = 0x10000
)

const (
	CS_HASHTYPE_SHA256     = 2
	CS_EXECSEG_MAIN_BINARY = 0x1
)

const csBlobIndexSize = 2 * 4
const csFixedSuperBlobSize = 3 * 4
const csCodeDirectorySize = 13*4 + 4*1 + 4*8
const csGenericBlobSize = 2 * 4

// --- Embedded SHA-256 implementation ---

var csK []uint32

func init() {
	csK = make([]uint32, 64)
	csK[0] = 0x428a2f98
	csK[1] = 0x71374491
	csK[2] = 0xb5c0fbcf
	csK[3] = 0xe9b5dba5
	csK[4] = 0x3956c25b
	csK[5] = 0x59f111f1
	csK[6] = 0x923f82a4
	csK[7] = 0xab1c5ed5
	csK[8] = 0xd807aa98
	csK[9] = 0x12835b01
	csK[10] = 0x243185be
	csK[11] = 0x550c7dc3
	csK[12] = 0x72be5d74
	csK[13] = 0x80deb1fe
	csK[14] = 0x9bdc06a7
	csK[15] = 0xc19bf174
	csK[16] = 0xe49b69c1
	csK[17] = 0xefbe4786
	csK[18] = 0x0fc19dc6
	csK[19] = 0x240ca1cc
	csK[20] = 0x2de92c6f
	csK[21] = 0x4a7484aa
	csK[22] = 0x5cb0a9dc
	csK[23] = 0x76f988da
	csK[24] = 0x983e5152
	csK[25] = 0xa831c66d
	csK[26] = 0xb00327c8
	csK[27] = 0xbf597fc7
	csK[28] = 0xc6e00bf3
	csK[29] = 0xd5a79147
	csK[30] = 0x06ca6351
	csK[31] = 0x14292967
	csK[32] = 0x27b70a85
	csK[33] = 0x2e1b2138
	csK[34] = 0x4d2c6dfc
	csK[35] = 0x53380d13
	csK[36] = 0x650a7354
	csK[37] = 0x766a0abb
	csK[38] = 0x81c2c92e
	csK[39] = 0x92722c85
	csK[40] = 0xa2bfe8a1
	csK[41] = 0xa81a664b
	csK[42] = 0xc24b8b70
	csK[43] = 0xc76c51a3
	csK[44] = 0xd192e819
	csK[45] = 0xd6990624
	csK[46] = 0xf40e3585
	csK[47] = 0x106aa070
	csK[48] = 0x19a4c116
	csK[49] = 0x1e376c08
	csK[50] = 0x2748774c
	csK[51] = 0x34b0bcb5
	csK[52] = 0x391c0cb3
	csK[53] = 0x4ed8aa4a
	csK[54] = 0x5b9cca4f
	csK[55] = 0x682e6ff3
	csK[56] = 0x748f82ee
	csK[57] = 0x78a5636f
	csK[58] = 0x84c87814
	csK[59] = 0x8cc70208
	csK[60] = 0x90befffa
	csK[61] = 0xa4506ceb
	csK[62] = 0xbef9a3f7
	csK[63] = 0xc67178f2
}

const csMask32 = 0xFFFFFFFF

func csRotr(x uint32, n uint32) uint32 {
	return ((x >> n) | (x << (32 - n))) & csMask32
}

// csSha256Block processes a single 64-byte block, updating state in place.
func csSha256Block(st []uint32, data []byte) {
	w := make([]uint32, 64)
	i := 0
	for i < 16 {
		w[i] = uint32(data[i*4])<<24 | uint32(data[i*4+1])<<16 | uint32(data[i*4+2])<<8 | uint32(data[i*4+3])
		i++
	}
	for i < 64 {
		s0 := csRotr(w[i-15], 7) ^ csRotr(w[i-15], 18) ^ (w[i-15] >> 3)
		s1 := csRotr(w[i-2], 17) ^ csRotr(w[i-2], 19) ^ (w[i-2] >> 10)
		w[i] = (w[i-16] + s0 + w[i-7] + s1) & csMask32
		i++
	}

	a := st[0]
	b := st[1]
	c := st[2]
	dd := st[3]
	e := st[4]
	f := st[5]
	g := st[6]
	h := st[7]

	i = 0
	for i < 64 {
		S1 := csRotr(e, 6) ^ csRotr(e, 11) ^ csRotr(e, 25)
		ch := (e & f) ^ (((^e) & csMask32) & g)
		temp1 := (h + S1 + ch + csK[i] + w[i]) & csMask32
		S0 := csRotr(a, 2) ^ csRotr(a, 13) ^ csRotr(a, 22)
		maj := (a & b) ^ (a & c) ^ (b & c)
		temp2 := (S0 + maj) & csMask32

		h = g
		g = f
		f = e
		e = (dd + temp1) & csMask32
		dd = c
		c = b
		b = a
		a = (temp1 + temp2) & csMask32
		i++
	}

	st[0] = (st[0] + a) & csMask32
	st[1] = (st[1] + b) & csMask32
	st[2] = (st[2] + c) & csMask32
	st[3] = (st[3] + dd) & csMask32
	st[4] = (st[4] + e) & csMask32
	st[5] = (st[5] + f) & csMask32
	st[6] = (st[6] + g) & csMask32
	st[7] = (st[7] + h) & csMask32
}

// csSha256 computes the SHA-256 hash of data and returns a 32-byte digest.
func csSha256(data []byte) []byte {
	st := make([]uint32, 8)
	st[0] = 0x6a09e667
	st[1] = 0xbb67ae85
	st[2] = 0x3c6ef372
	st[3] = 0xa54ff53a
	st[4] = 0x510e527f
	st[5] = 0x9b05688c
	st[6] = 0x1f83d9ab
	st[7] = 0x5be0cd19

	// Process full 64-byte blocks
	pos := 0
	nblocks := 0
	for pos+64 <= len(data) {
		csSha256Block(st, data[pos:pos+64])
		pos = pos + 64
		nblocks++
	}

	// Padding
	buf := make([]byte, 0)
	remaining := data[pos:len(data)]
	buf = append(buf, remaining...)
	buf = append(buf, 0x80)
	for len(buf)%64 != 56 {
		buf = append(buf, 0)
	}
	bitLen := uint64(len(data)) * 8
	buf = append(buf, byte(bitLen>>56))
	buf = append(buf, byte(bitLen>>48))
	buf = append(buf, byte(bitLen>>40))
	buf = append(buf, byte(bitLen>>32))
	buf = append(buf, byte(bitLen>>24))
	buf = append(buf, byte(bitLen>>16))
	buf = append(buf, byte(bitLen>>8))
	buf = append(buf, byte(bitLen))

	// Process remaining padded blocks
	pos = 0
	for pos < len(buf) {
		csSha256Block(st, buf[pos:pos+64])
		pos = pos + 64
	}

	out := make([]byte, 32)
	j := 0
	for j < 8 {
		out[j*4] = byte(st[j] >> 24)
		out[j*4+1] = byte(st[j] >> 16)
		out[j*4+2] = byte(st[j] >> 8)
		out[j*4+3] = byte(st[j])
		j++
	}
	return out
}

// --- Code signing helpers ---

func csPut32be(b []byte, x uint32) []byte {
	b[0] = byte(x >> 24)
	b[1] = byte(x >> 16)
	b[2] = byte(x >> 8)
	b[3] = byte(x)
	return b[4:]
}

func csPut64be(b []byte, x uint64) []byte {
	b[0] = byte(x >> 56)
	b[1] = byte(x >> 48)
	b[2] = byte(x >> 40)
	b[3] = byte(x >> 32)
	b[4] = byte(x >> 24)
	b[5] = byte(x >> 16)
	b[6] = byte(x >> 8)
	b[7] = byte(x)
	return b[8:]
}

func csPut8(b []byte, x byte) []byte {
	b[0] = x
	return b[1:]
}

func csPuts(b []byte, s []byte) []byte {
	n := copy(b, s)
	return b[n:]
}

func csSbSize(nblobs uint32) uint32 {
	return csFixedSuperBlobSize + nblobs*csBlobIndexSize
}

func csCdSize(nslots int64, nspecial int64, id string) int64 {
	sz := int64(csCodeDirectorySize)
	sz = sz + int64(len(id)+1)
	sz = sz + (nslots + nspecial) * int64(csHashSize)
	return sz
}

// CodeSignSize computes the size of the code signature.
func CodeSignSize(codeSize int64, id string) int64 {
	nslots := (codeSize + csPageSize - 1) / csPageSize
	nspecial := int64(CSSLOT_REQUIREMENTS)

	nblobs := uint32(3) // code directory + requirements + signature slot

	sz := int64(csSbSize(nblobs))
	sz = sz + csCdSize(nslots, nspecial, id)
	sz = sz + int64(csGenericBlobSize) + 4 // empty requirements
	sz = sz + int64(csGenericBlobSize)     // empty certificate blob wrapper
	return sz
}

// CodeSign generates an ad-hoc code signature and writes it to out.
func CodeSign(out []byte, data []byte, codeSize int64, textOff int64, textSize int64, isMain bool, id string) {
	nslots := (codeSize + csPageSize - 1) / csPageSize
	nspecial := int64(CSSLOT_REQUIREMENTS)

	off := uint32(0)
	idOff := int64(csCodeDirectorySize)
	hashOff := idOff + int64(len(id)+1) + nspecial*int64(csHashSize)
	sz := len(out)

	nblobs := uint32(3)

	// SuperBlob header
	off = off + csSbSize(nblobs)

	// CodeDirectory
	cdLen := uint32(csCdSize(nslots, nspecial, id))
	cdOff := off
	off = off + cdLen

	// Requirements blob
	reqOff := off
	reqLen := uint32(csGenericBlobSize + 4)
	off = off + reqLen

	// Empty certificate blob wrapper
	certOff := off
	certLen := uint32(csGenericBlobSize)
	off = off + certLen
	_ = off

	// --- Emit SuperBlob ---
	outp := out
	outp = csPut32be(outp, CSMAGIC_EMBEDDED_SIGNATURE)
	outp = csPut32be(outp, uint32(sz))
	outp = csPut32be(outp, nblobs)

	// BlobIndex: CodeDirectory
	outp = csPut32be(outp, CSSLOT_CODEDIRECTORY)
	outp = csPut32be(outp, cdOff)

	// BlobIndex: Requirements
	outp = csPut32be(outp, CSSLOT_REQUIREMENTS)
	outp = csPut32be(outp, reqOff)

	// BlobIndex: SignatureSlot
	outp = csPut32be(outp, CSSLOT_SIGNATURESLOT)
	outp = csPut32be(outp, certOff)

	// --- Emit CodeDirectory ---
	execSegFlags := uint64(0)
	if isMain {
		execSegFlags = CS_EXECSEG_MAIN_BINARY
	}
	outp = csPut32be(outp, CSMAGIC_CODEDIRECTORY)
	outp = csPut32be(outp, cdLen)
	outp = csPut32be(outp, 0x20400)                // version
	outp = csPut32be(outp, 0x20002)                // flags: adhoc | linkerSigned
	outp = csPut32be(outp, uint32(hashOff))        // hashOffset
	outp = csPut32be(outp, uint32(idOff))          // identOffset
	outp = csPut32be(outp, uint32(nspecial))       // nSpecialSlots
	outp = csPut32be(outp, uint32(nslots))         // nCodeSlots
	outp = csPut32be(outp, uint32(codeSize))       // codeLimit
	outp = csPut8(outp, csHashSize)                // hashSize
	outp = csPut8(outp, CS_HASHTYPE_SHA256)        // hashType
	outp = csPut8(outp, 0)                         // pad1
	outp = csPut8(outp, byte(csPageSizeBits))      // pageSize
	outp = csPut32be(outp, 0)                      // pad2
	outp = csPut32be(outp, 0)                      // scatterOffset
	outp = csPut32be(outp, 0)                      // teamOffset
	outp = csPut32be(outp, 0)                      // pad3
	outp = csPut64be(outp, 0)                      // codeLimit64
	outp = csPut64be(outp, uint64(textOff))        // execSegBase
	outp = csPut64be(outp, uint64(textSize))       // execSegLimit
	outp = csPut64be(outp, execSegFlags)           // execSegFlags

	// Identifier string (null terminated)
	outp = csPuts(outp, []byte(id))
	outp = csPut8(outp, 0)

	// Special slot hashes (in reverse order, from -nspecial to -1)
	// Compute requirements blob hash for slot -2 (CSSLOT_REQUIREMENTS)
	reqBlob := make([]byte, reqLen)
	rp := reqBlob
	rp = csPut32be(rp, CSMAGIC_REQUIREMENTS)
	rp = csPut32be(rp, reqLen)
	rp = csPut32be(rp, 0) // empty requirements data
	_ = rp
	reqHash := csSha256(reqBlob)

	i := -int(nspecial)
	for i < 0 {
		if -i == int(CSSLOT_REQUIREMENTS) {
			outp = csPuts(outp, reqHash)
		} else {
			outp = csPuts(outp, make([]byte, csHashSize))
		}
		i++
	}

	// Code slot hashes (page-by-page SHA-256)
	p := 0
	for p < int(codeSize) {
		end := p + csPageSize
		if end > int(codeSize) {
			end = int(codeSize)
		}
		pageHash := csSha256(data[p:end])
		outp = csPuts(outp, pageHash)
		p = end
	}

	// --- Emit Requirements blob ---
	outp = csPut32be(outp, CSMAGIC_REQUIREMENTS)
	outp = csPut32be(outp, reqLen)
	outp = csPut32be(outp, 0) // empty requirements

	// --- Emit empty certificate blob ---
	outp = csPut32be(outp, CSMAGIC_BLOBWRAPPER)
	outp = csPut32be(outp, certLen)
}
