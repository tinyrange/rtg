package main

import (
	"os"

	"j5.nz/rtg/std/compiler/backend/8086/dos"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

type tinyReader struct {
	b []byte
	i int
}

func (r *tinyReader) need(n int) bool {
	return r.i+n <= len(r.b)
}

func (r *tinyReader) u16() (int, bool) {
	if !r.need(2) {
		return 0, false
	}
	v := int(r.b[r.i]) | (int(r.b[r.i+1]) << 8)
	r.i += 2
	return v, true
}

func (r *tinyReader) i32() (int, bool) {
	if !r.need(4) {
		return 0, false
	}
	v := int(uint32(r.b[r.i]) | (uint32(r.b[r.i+1]) << 8) | (uint32(r.b[r.i+2]) << 16) | (uint32(r.b[r.i+3]) << 24))
	r.i += 4
	return v, true
}

func (r *tinyReader) i64() (int64, bool) {
	if !r.need(8) {
		return 0, false
	}
	u := uint64(r.b[r.i]) |
		(uint64(r.b[r.i+1]) << 8) |
		(uint64(r.b[r.i+2]) << 16) |
		(uint64(r.b[r.i+3]) << 24) |
		(uint64(r.b[r.i+4]) << 32) |
		(uint64(r.b[r.i+5]) << 40) |
		(uint64(r.b[r.i+6]) << 48) |
		(uint64(r.b[r.i+7]) << 56)
	r.i += 8
	return int64(u), true
}

func (r *tinyReader) str() (string, bool) {
	n, ok := r.u16()
	if !ok || !r.need(n) {
		return "", false
	}
	s := string(r.b[r.i : r.i+n])
	r.i += n
	return s, true
}

func decodeTinyIR(data []byte) (*ir.IRModule, bool) {
	if len(data) < 6 {
		return nil, false
	}
	if data[0] != 'T' || data[1] != 'I' || data[2] != 'R' || data[3] != '3' {
		return nil, false
	}
	r := tinyReader{b: data, i: 4}
	fnCount, ok := r.u16()
	if !ok {
		return nil, false
	}
	mod := &ir.IRModule{Funcs: make([]*ir.IRFunc, 0, fnCount)}
	for f := 0; f < fnCount; f++ {
		name, ok := r.str()
		if !ok {
			return nil, false
		}
		params, ok := r.u16()
		if !ok {
			return nil, false
		}
		retCount, ok := r.u16()
		if !ok {
			return nil, false
		}
		localCount, ok := r.u16()
		if !ok {
			return nil, false
		}
		locals := make([]ir.IRLocal, 0, localCount)
		for i := 0; i < localCount; i++ {
			ln, ok := r.str()
			if !ok {
				return nil, false
			}
			locals = append(locals, ir.IRLocal{Name: ln, Index: i})
		}
		instCount, ok := r.u16()
		if !ok {
			return nil, false
		}
		code := make([]ir.Inst, 0, instCount)
		for i := 0; i < instCount; i++ {
			op, ok := r.u16()
			if !ok {
				return nil, false
			}
			arg, ok := r.i32()
			if !ok {
				return nil, false
			}
			val, ok := r.i64()
			if !ok {
				return nil, false
			}
			nameField, ok := r.str()
			if !ok {
				return nil, false
			}
			code = append(code, ir.Inst{Op: ir.Opcode(op), Arg: arg, Val: val, Name: nameField})
		}
		mod.Funcs = append(mod.Funcs, &ir.IRFunc{Name: name, Params: params, RetCount: retCount, Locals: locals, Code: code})
	}
	if r.i != len(data) {
		return nil, false
	}
	return mod, true
}

func main() {
	tir, err := os.ReadFile("PROG.TIR")
	if err != nil {
		os.Exit(2)
	}
	mod, ok := decodeTinyIR(tir)
	if !ok || mod == nil {
		os.Exit(2)
	}
	target := &common.Target{GOOS: "dos", GOARCH: "dos16", PtrSize: 2, Backend: "native"}
	if dos.GenerateDOSCOM(target, mod, "CHILD.COM") != nil {
		os.Exit(3)
	}
	os.Exit(0)
}
