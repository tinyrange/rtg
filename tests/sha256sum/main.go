package main

import (
	"fmt"
	"os"
)

var k []int

func initK() {
	k = make([]int, 64)
	k[0] = 0x428a2f98
	k[1] = 0x71374491
	k[2] = 0xb5c0fbcf
	k[3] = 0xe9b5dba5
	k[4] = 0x3956c25b
	k[5] = 0x59f111f1
	k[6] = 0x923f82a4
	k[7] = 0xab1c5ed5
	k[8] = 0xd807aa98
	k[9] = 0x12835b01
	k[10] = 0x243185be
	k[11] = 0x550c7dc3
	k[12] = 0x72be5d74
	k[13] = 0x80deb1fe
	k[14] = 0x9bdc06a7
	k[15] = 0xc19bf174
	k[16] = 0xe49b69c1
	k[17] = 0xefbe4786
	k[18] = 0x0fc19dc6
	k[19] = 0x240ca1cc
	k[20] = 0x2de92c6f
	k[21] = 0x4a7484aa
	k[22] = 0x5cb0a9dc
	k[23] = 0x76f988da
	k[24] = 0x983e5152
	k[25] = 0xa831c66d
	k[26] = 0xb00327c8
	k[27] = 0xbf597fc7
	k[28] = 0xc6e00bf3
	k[29] = 0xd5a79147
	k[30] = 0x06ca6351
	k[31] = 0x14292967
	k[32] = 0x27b70a85
	k[33] = 0x2e1b2138
	k[34] = 0x4d2c6dfc
	k[35] = 0x53380d13
	k[36] = 0x650a7354
	k[37] = 0x766a0abb
	k[38] = 0x81c2c92e
	k[39] = 0x92722c85
	k[40] = 0xa2bfe8a1
	k[41] = 0xa81a664b
	k[42] = 0xc24b8b70
	k[43] = 0xc76c51a3
	k[44] = 0xd192e819
	k[45] = 0xd6990624
	k[46] = 0xf40e3585
	k[47] = 0x106aa070
	k[48] = 0x19a4c116
	k[49] = 0x1e376c08
	k[50] = 0x2748774c
	k[51] = 0x34b0bcb5
	k[52] = 0x391c0cb3
	k[53] = 0x4ed8aa4a
	k[54] = 0x5b9cca4f
	k[55] = 0x682e6ff3
	k[56] = 0x748f82ee
	k[57] = 0x78a5636f
	k[58] = 0x84c87814
	k[59] = 0x8cc70208
	k[60] = 0x90befffa
	k[61] = 0xa4506ceb
	k[62] = 0xbef9a3f7
	k[63] = 0xc67178f2
}

var m32 = 0xFFFFFFFF

func rotr(x int, n int) int {
	return ((x >> n) | (x << (32 - n))) & m32
}

type SHA256 struct {
	h0       int
	h1       int
	h2       int
	h3       int
	h4       int
	h5       int
	h6       int
	h7       int
	buf      []byte
	totalLen int
	w        []int
}

func newSHA256() *SHA256 {
	s := &SHA256{}
	s.h0 = 0x6a09e667
	s.h1 = 0xbb67ae85
	s.h2 = 0x3c6ef372
	s.h3 = 0xa54ff53a
	s.h4 = 0x510e527f
	s.h5 = 0x9b05688c
	s.h6 = 0x1f83d9ab
	s.h7 = 0x5be0cd19
	s.buf = make([]byte, 0, 64)
	s.w = make([]int, 64)
	return s
}

func (s *SHA256) processBlock(block []byte) {
	w := s.w
	i := 0
	for i < 16 {
		j := i * 4
		w[i] = (int(block[j])<<24 | int(block[j+1])<<16 | int(block[j+2])<<8 | int(block[j+3])) & m32
		i = i + 1
	}
	for i < 64 {
		s0 := rotr(w[i-15], 7) ^ rotr(w[i-15], 18) ^ ((w[i-15] >> 3) & m32)
		s1 := rotr(w[i-2], 17) ^ rotr(w[i-2], 19) ^ ((w[i-2] >> 10) & m32)
		w[i] = (w[i-16] + s0 + w[i-7] + s1) & m32
		i = i + 1
	}

	a := s.h0
	b := s.h1
	c := s.h2
	d := s.h3
	e := s.h4
	f := s.h5
	g := s.h6
	h := s.h7

	i = 0
	for i < 64 {
		S1 := rotr(e, 6) ^ rotr(e, 11) ^ rotr(e, 25)
		ch := (e & f) ^ ((^e) & g & m32)
		temp1 := (h + S1 + ch + k[i] + w[i]) & m32
		S0 := rotr(a, 2) ^ rotr(a, 13) ^ rotr(a, 22)
		maj := (a & b) ^ (a & c) ^ (b & c)
		temp2 := (S0 + maj) & m32

		h = g
		g = f
		f = e
		e = (d + temp1) & m32
		d = c
		c = b
		b = a
		a = (temp1 + temp2) & m32
		i = i + 1
	}

	s.h0 = (s.h0 + a) & m32
	s.h1 = (s.h1 + b) & m32
	s.h2 = (s.h2 + c) & m32
	s.h3 = (s.h3 + d) & m32
	s.h4 = (s.h4 + e) & m32
	s.h5 = (s.h5 + f) & m32
	s.h6 = (s.h6 + g) & m32
	s.h7 = (s.h7 + h) & m32
}

func (s *SHA256) update(data []byte) {
	s.totalLen = s.totalLen + len(data)
	// If we have buffered data, try to complete a block
	if len(s.buf) > 0 {
		need := 64 - len(s.buf)
		if len(data) < need {
			s.buf = append(s.buf, data...)
			return
		}
		s.buf = append(s.buf, data[0:need]...)
		s.processBlock(s.buf)
		s.buf = s.buf[0:0]
		data = data[need:]
	}
	// Process full blocks directly from data
	for len(data) >= 64 {
		s.processBlock(data[0:64])
		data = data[64:]
	}
	// Buffer remainder
	if len(data) > 0 {
		s.buf = append(s.buf, data...)
	}
}

func (s *SHA256) finish() []byte {
	// Padding
	bitLen := s.totalLen * 8
	s.buf = append(s.buf, 0x80)
	for len(s.buf)%64 != 56 {
		s.buf = append(s.buf, 0)
	}
	s.buf = append(s.buf, 0, 0, 0, 0)
	s.buf = append(s.buf, byte((bitLen>>24)&0xFF), byte((bitLen>>16)&0xFF), byte((bitLen>>8)&0xFF), byte(bitLen&0xFF))
	// Process remaining blocks (1 or 2)
	off := 0
	for off < len(s.buf) {
		s.processBlock(s.buf[off : off+64])
		off = off + 64
	}

	digest := make([]byte, 32)
	digest[0] = byte((s.h0 >> 24) & 0xFF)
	digest[1] = byte((s.h0 >> 16) & 0xFF)
	digest[2] = byte((s.h0 >> 8) & 0xFF)
	digest[3] = byte(s.h0 & 0xFF)
	digest[4] = byte((s.h1 >> 24) & 0xFF)
	digest[5] = byte((s.h1 >> 16) & 0xFF)
	digest[6] = byte((s.h1 >> 8) & 0xFF)
	digest[7] = byte(s.h1 & 0xFF)
	digest[8] = byte((s.h2 >> 24) & 0xFF)
	digest[9] = byte((s.h2 >> 16) & 0xFF)
	digest[10] = byte((s.h2 >> 8) & 0xFF)
	digest[11] = byte(s.h2 & 0xFF)
	digest[12] = byte((s.h3 >> 24) & 0xFF)
	digest[13] = byte((s.h3 >> 16) & 0xFF)
	digest[14] = byte((s.h3 >> 8) & 0xFF)
	digest[15] = byte(s.h3 & 0xFF)
	digest[16] = byte((s.h4 >> 24) & 0xFF)
	digest[17] = byte((s.h4 >> 16) & 0xFF)
	digest[18] = byte((s.h4 >> 8) & 0xFF)
	digest[19] = byte(s.h4 & 0xFF)
	digest[20] = byte((s.h5 >> 24) & 0xFF)
	digest[21] = byte((s.h5 >> 16) & 0xFF)
	digest[22] = byte((s.h5 >> 8) & 0xFF)
	digest[23] = byte(s.h5 & 0xFF)
	digest[24] = byte((s.h6 >> 24) & 0xFF)
	digest[25] = byte((s.h6 >> 16) & 0xFF)
	digest[26] = byte((s.h6 >> 8) & 0xFF)
	digest[27] = byte(s.h6 & 0xFF)
	digest[28] = byte((s.h7 >> 24) & 0xFF)
	digest[29] = byte((s.h7 >> 16) & 0xFF)
	digest[30] = byte((s.h7 >> 8) & 0xFF)
	digest[31] = byte(s.h7 & 0xFF)
	return digest
}

func main() {
	initK()

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: sha256sum <file>\n")
		os.Exit(1)
	}

	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "sha256sum: %s\n", err.Error())
		os.Exit(1)
	}

	s := newSHA256()
	buf := make([]byte, 4096)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			s.update(buf[0:n])
		}
		if err != nil {
			break
		}
		if n == 0 {
			break
		}
	}
	f.Close()

	digest := s.finish()

	hex := "0123456789abcdef"
	var out []byte
	i := 0
	for i < 32 {
		out = append(out, hex[digest[i]>>4])
		out = append(out, hex[digest[i]&0x0f])
		i = i + 1
	}
	fmt.Printf("%s  %s\n", string(out), os.Args[1])
}
