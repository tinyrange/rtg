//go:build exp_ir_binary

package binary

import (
	"fmt"
	"os"

	"j5.nz/rtg/std/compiler/ir"
)

const IrBinaryEnabled = true

var irBinaryMagic = []byte{'R', 'T', 'G', 'I', 'R', 'B', '1', 0}

type irBinWriter struct {
	buf []byte
}

func (w *irBinWriter) bytes() []byte {
	return w.buf
}

func (w *irBinWriter) writeByte(v byte) {
	w.buf = append(w.buf, v)
}

func (w *irBinWriter) writeBool(v bool) {
	if v {
		w.writeByte(1)
	} else {
		w.writeByte(0)
	}
}

func (w *irBinWriter) writeU32(v uint32) {
	u := int(v)
	b0 := byte(u % 256)
	u = u / 256
	b1 := byte(u % 256)
	u = u / 256
	b2 := byte(u % 256)
	u = u / 256
	b3 := byte(u % 256)
	w.buf = append(w.buf, b0, b1, b2, b3)
}

func (w *irBinWriter) writeI64(v int64) {
	u := uint64(v)
	w.buf = append(w.buf, byte(u), byte(u>>8), byte(u>>16), byte(u>>24),
		byte(u>>32), byte(u>>40), byte(u>>48), byte(u>>56))
}

func (w *irBinWriter) writeInt(v int) {
	w.writeI64(int64(v))
}

func (w *irBinWriter) writeString(s string) {
	w.writeU32(uint32(len(s)))
	w.buf = append(w.buf, []byte(s)...)
}

type irBinReader struct {
	data []byte
	off  int
}

func (r *irBinReader) need(n int) error {
	if r.off+n > len(r.data) {
		return fmt.Errorf("truncated IR binary")
	}
	return nil
}

func (r *irBinReader) readByte() (byte, error) {
	if err := r.need(1); err != nil {
		return 0, err
	}
	v := r.data[r.off]
	r.off++
	return v, nil
}

func (r *irBinReader) readBool() (bool, error) {
	b, err := r.readByte()
	if err != nil {
		return false, err
	}
	if b == 0 {
		return false, nil
	}
	if b == 1 {
		return true, nil
	}
	return false, fmt.Errorf("invalid bool value %d", int(b))
}

func (r *irBinReader) readU32() (uint32, error) {
	if err := r.need(4); err != nil {
		return 0, err
	}
	b := r.data[r.off : r.off+4]
	r.off += 4
	v := uint32(b[0]) + uint32(b[1])*256 + uint32(b[2])*65536 + uint32(b[3])*16777216
	return v, nil
}

func (r *irBinReader) readI64() (int64, error) {
	if err := r.need(8); err != nil {
		return 0, err
	}
	b := r.data[r.off : r.off+8]
	r.off += 8
	u := uint64(b[0]) |
		(uint64(b[1]) << 8) |
		(uint64(b[2]) << 16) |
		(uint64(b[3]) << 24) |
		(uint64(b[4]) << 32) |
		(uint64(b[5]) << 40) |
		(uint64(b[6]) << 48) |
		(uint64(b[7]) << 56)
	return int64(u), nil
}

func (r *irBinReader) readInt() (int, error) {
	v, err := r.readI64()
	if err != nil {
		return 0, err
	}
	return int(v), nil
}

func (r *irBinReader) readString() (string, error) {
	n, err := r.readU32()
	if err != nil {
		return "", err
	}
	if err := r.need(int(n)); err != nil {
		return "", err
	}
	s := string(r.data[r.off : r.off+int(n)])
	r.off += int(n)
	return s, nil
}

func typeIndex(idx map[*ir.TypeInfo]int, t *ir.TypeInfo) int {
	if t == nil {
		return -1
	}
	return idx[t]
}

func collectTypeInfo(t *ir.TypeInfo, idx map[*ir.TypeInfo]int, all *[]*ir.TypeInfo) {
	if t == nil {
		return
	}
	if _, ok := idx[t]; ok {
		return
	}
	idx[t] = len(*all)
	*all = append(*all, t)
	collectTypeInfo(t.Elem, idx, all)
	collectTypeInfo(t.Key, idx, all)
	for _, f := range t.Fields {
		collectTypeInfo(f.Type, idx, all)
	}
	for _, p := range t.Params {
		collectTypeInfo(p, idx, all)
	}
	for _, r := range t.Results {
		collectTypeInfo(r, idx, all)
	}
}

func WriteIRBinary(irmod *ir.IRModule, path string) error {
	if irmod == nil {
		return fmt.Errorf("nil IR module")
	}

	typeIdx := make(map[*ir.TypeInfo]int)
	var allTypes []*ir.TypeInfo
	for _, t := range irmod.Types {
		collectTypeInfo(t, typeIdx, &allTypes)
	}
	for i := 0; i < len(irmod.Globals); i++ {
		collectTypeInfo(irmod.Globals[i].Type, typeIdx, &allTypes)
	}
	for _, f := range irmod.Funcs {
		for i := 0; i < len(f.Locals); i++ {
			collectTypeInfo(f.Locals[i].Type, typeIdx, &allTypes)
		}
	}

	w := &irBinWriter{}
	for i := 0; i < len(irBinaryMagic); i++ {
		w.writeByte(irBinaryMagic[i])
	}

	w.writeU32(uint32(len(allTypes)))
	for i := 0; i < len(allTypes); i++ {
		t := allTypes[i]
		w.writeInt(int(t.Kind))
		w.writeString(t.Name)
		w.writeString(t.Pkg)
		w.writeInt(t.Size)
		w.writeInt(t.Align)
		w.writeInt(typeIndex(typeIdx, t.Elem))
		w.writeInt(typeIndex(typeIdx, t.Key))

		w.writeU32(uint32(len(t.Fields)))
		for j := 0; j < len(t.Fields); j++ {
			f := t.Fields[j]
			w.writeString(f.Name)
			w.writeInt(typeIndex(typeIdx, f.Type))
			w.writeInt(f.Offset)
		}

		w.writeU32(uint32(len(t.Params)))
		for j := 0; j < len(t.Params); j++ {
			w.writeInt(typeIndex(typeIdx, t.Params[j]))
		}

		w.writeU32(uint32(len(t.Results)))
		for j := 0; j < len(t.Results); j++ {
			w.writeInt(typeIndex(typeIdx, t.Results[j]))
		}
	}

	w.writeU32(uint32(len(irmod.Types)))
	for i := 0; i < len(irmod.Types); i++ {
		w.writeInt(typeIndex(typeIdx, irmod.Types[i]))
	}

	w.writeU32(uint32(len(irmod.Globals)))
	for i := 0; i < len(irmod.Globals); i++ {
		g := irmod.Globals[i]
		w.writeString(g.Name)
		w.writeInt(typeIndex(typeIdx, g.Type))
		w.writeInt(g.Index)
	}

	w.writeU32(uint32(len(irmod.Funcs)))
	for i := 0; i < len(irmod.Funcs); i++ {
		f := irmod.Funcs[i]
		w.writeString(f.Name)
		w.writeInt(f.Params)
		w.writeInt(f.RetCount)

		w.writeU32(uint32(len(f.Locals)))
		for j := 0; j < len(f.Locals); j++ {
			l := f.Locals[j]
			w.writeString(l.Name)
			w.writeInt(typeIndex(typeIdx, l.Type))
			w.writeInt(l.Index)
			w.writeBool(l.Is64)
			w.writeInt(l.Width)
		}

		w.writeU32(uint32(len(f.Code)))
		for j := 0; j < len(f.Code); j++ {
			in := f.Code[j]
			w.writeInt(int(in.Op))
			w.writeInt(in.Arg)
			w.writeInt(in.Width)
			w.writeI64(in.Val)
			w.writeString(in.Name)
		}
	}

	w.writeU32(uint32(len(irmod.LinkStaticFuncs)))
	for k := range irmod.LinkStaticFuncs {
		w.writeString(k)
		w.writeString(irmod.LinkStaticFuncs[k])
	}

	w.writeU32(uint32(len(irmod.TypeIDs)))
	for k := range irmod.TypeIDs {
		w.writeString(k)
		w.writeInt(irmod.TypeIDs[k])
	}

	w.writeU32(uint32(len(irmod.MethodTable)))
	for k := range irmod.MethodTable {
		w.writeString(k)
		w.writeString(irmod.MethodTable[k])
	}

	w.writeU32(uint32(len(irmod.IfaceMethods)))
	for k := range irmod.IfaceMethods {
		ms := irmod.IfaceMethods[k]
		w.writeString(k)
		w.writeU32(uint32(len(ms)))
		for j := 0; j < len(ms); j++ {
			w.writeString(ms[j])
		}
	}

	w.writeU32(uint32(len(irmod.IfaceMethodRets)))
	for k := range irmod.IfaceMethodRets {
		w.writeString(k)
		w.writeInt(irmod.IfaceMethodRets[k])
	}

	return os.WriteFile(path, w.bytes(), 0644)
}

func readIRMagic(r *irBinReader) error {
	for i := 0; i < len(irBinaryMagic); i++ {
		b, err := r.readByte()
		if err != nil {
			return err
		}
		if b != irBinaryMagic[i] {
			return fmt.Errorf("invalid IR binary magic")
		}
	}
	return nil
}

func readIRTypes(r *irBinReader) ([]*ir.TypeInfo, error) {
	typeCountU32, err := r.readU32()
	if err != nil {
		return nil, err
	}
	typeCount := int(typeCountU32)
	types := make([]*ir.TypeInfo, typeCount)
	for i := 0; i < typeCount; i++ {
		types[i] = &ir.TypeInfo{}
	}
	for i := 0; i < typeCount; i++ {
		t := types[i]
		kind, err := r.readInt()
		if err != nil {
			return nil, err
		}
		t.Kind = ir.TypeKind(kind)
		t.Name, err = r.readString()
		if err != nil {
			return nil, err
		}
		t.Pkg, err = r.readString()
		if err != nil {
			return nil, err
		}
		t.Size, err = r.readInt()
		if err != nil {
			return nil, err
		}
		t.Align, err = r.readInt()
		if err != nil {
			return nil, err
		}
		elemIdx, err := r.readInt()
		if err != nil {
			return nil, err
		}
		keyIdx, err := r.readInt()
		if err != nil {
			return nil, err
		}
		if elemIdx >= 0 && elemIdx < len(types) {
			t.Elem = types[elemIdx]
		}
		if keyIdx >= 0 && keyIdx < len(types) {
			t.Key = types[keyIdx]
		}
		fieldCountU32, err := r.readU32()
		if err != nil {
			return nil, err
		}
		fieldCount := int(fieldCountU32)
		fields := make([]ir.FieldInfo, 0, fieldCount)
		for j := 0; j < fieldCount; j++ {
			var f ir.FieldInfo
			name, err := r.readString()
			if err != nil {
				return nil, err
			}
			typeIdx, err := r.readInt()
			if err != nil {
				return nil, err
			}
			offset, err := r.readInt()
			if err != nil {
				return nil, err
			}
			f.Name = name
			f.Offset = offset
			if typeIdx >= 0 && typeIdx < len(types) {
				f.Type = types[typeIdx]
			}
			fields = append(fields, f)
		}
		t.Fields = fields
		paramCountU32, err := r.readU32()
		if err != nil {
			return nil, err
		}
		paramCount := int(paramCountU32)
		params := make([]*ir.TypeInfo, 0, paramCount)
		for j := 0; j < paramCount; j++ {
			typeIdx, err := r.readInt()
			if err != nil {
				return nil, err
			}
			var p *ir.TypeInfo
			if typeIdx >= 0 && typeIdx < len(types) {
				p = types[typeIdx]
			}
			params = append(params, p)
		}
		t.Params = params
		resultCountU32, err := r.readU32()
		if err != nil {
			return nil, err
		}
		resultCount := int(resultCountU32)
		results := make([]*ir.TypeInfo, 0, resultCount)
		for j := 0; j < resultCount; j++ {
			typeIdx, err := r.readInt()
			if err != nil {
				return nil, err
			}
			var res *ir.TypeInfo
			if typeIdx >= 0 && typeIdx < len(types) {
				res = types[typeIdx]
			}
			results = append(results, res)
		}
		t.Results = results
	}
	return types, nil
}

func readIRRootTypes(r *irBinReader, types []*ir.TypeInfo) ([]*ir.TypeInfo, error) {
	countU32, err := r.readU32()
	if err != nil {
		return nil, err
	}
	count := int(countU32)
	rootTypes := make([]*ir.TypeInfo, count)
	for i := 0; i < count; i++ {
		idx, err := r.readInt()
		if err != nil {
			return nil, err
		}
		if idx >= 0 && idx < len(types) {
			rootTypes[i] = types[idx]
		}
	}
	return rootTypes, nil
}

func readIRGlobals(r *irBinReader, types []*ir.TypeInfo) ([]ir.IRGlobal, error) {
	countU32, err := r.readU32()
	if err != nil {
		return nil, err
	}
	count := int(countU32)
	globals := make([]ir.IRGlobal, 0, count)
	for i := 0; i < count; i++ {
		var g ir.IRGlobal
		g.Name, err = r.readString()
		if err != nil {
			return nil, err
		}
		typeIdx, err := r.readInt()
		if err != nil {
			return nil, err
		}
		if typeIdx >= 0 && typeIdx < len(types) {
			g.Type = types[typeIdx]
		}
		g.Index, err = r.readInt()
		if err != nil {
			return nil, err
		}
		globals = append(globals, g)
	}
	return globals, nil
}

func readIRFuncs(r *irBinReader, types []*ir.TypeInfo) ([]*ir.IRFunc, error) {
	countU32, err := r.readU32()
	if err != nil {
		return nil, err
	}
	count := int(countU32)
	funcs := make([]*ir.IRFunc, 0, count)
	for i := 0; i < count; i++ {
		f := &ir.IRFunc{}
		f.Name, err = r.readString()
		if err != nil {
			return nil, err
		}
		f.Params, err = r.readInt()
		if err != nil {
			return nil, err
		}
		f.RetCount, err = r.readInt()
		if err != nil {
			return nil, err
		}
		localCountU32, err := r.readU32()
		if err != nil {
			return nil, err
		}
		localCount := int(localCountU32)
		locals := make([]ir.IRLocal, 0, localCount)
		for j := 0; j < localCount; j++ {
			var l ir.IRLocal
			l.Name, err = r.readString()
			if err != nil {
				return nil, err
			}
			typeIdx, err := r.readInt()
			if err != nil {
				return nil, err
			}
			if typeIdx >= 0 && typeIdx < len(types) {
				l.Type = types[typeIdx]
			}
			l.Index, err = r.readInt()
			if err != nil {
				return nil, err
			}
			l.Is64, err = r.readBool()
			if err != nil {
				return nil, err
			}
			l.Width, err = r.readInt()
			if err != nil {
				return nil, err
			}
			locals = append(locals, l)
		}
		f.Locals = locals
		instCountU32, err := r.readU32()
		if err != nil {
			return nil, err
		}
		instCount := int(instCountU32)
		code := make([]ir.Inst, 0, instCount)
		for j := 0; j < instCount; j++ {
			var in ir.Inst
			op, err := r.readInt()
			if err != nil {
				return nil, err
			}
			in.Op = ir.Opcode(op)
			in.Arg, err = r.readInt()
			if err != nil {
				return nil, err
			}
			in.Width, err = r.readInt()
			if err != nil {
				return nil, err
			}
			in.Val, err = r.readI64()
			if err != nil {
				return nil, err
			}
			in.Name, err = r.readString()
			if err != nil {
				return nil, err
			}
			code = append(code, in)
		}
		f.Code = code
		funcs = append(funcs, f)
	}
	return funcs, nil
}

func readStringIntMap(r *irBinReader) (map[string]int, error) {
	countU32, err := r.readU32()
	if err != nil {
		return nil, err
	}
	m := make(map[string]int)
	for i := 0; i < int(countU32); i++ {
		k, err := r.readString()
		if err != nil {
			return nil, err
		}
		v, err := r.readInt()
		if err != nil {
			return nil, err
		}
		m[k] = v
	}
	return m, nil
}

func readStringStringMap(r *irBinReader) (map[string]string, error) {
	countU32, err := r.readU32()
	if err != nil {
		return nil, err
	}
	m := make(map[string]string)
	for i := 0; i < int(countU32); i++ {
		k, err := r.readString()
		if err != nil {
			return nil, err
		}
		v, err := r.readString()
		if err != nil {
			return nil, err
		}
		m[k] = v
	}
	return m, nil
}

func readIfaceMethodsMap(r *irBinReader) (map[string][]string, error) {
	countU32, err := r.readU32()
	if err != nil {
		return nil, err
	}
	m := make(map[string][]string)
	for i := 0; i < int(countU32); i++ {
		k, err := r.readString()
		if err != nil {
			return nil, err
		}
		nU32, err := r.readU32()
		if err != nil {
			return nil, err
		}
		ms := make([]string, int(nU32))
		for j := 0; j < int(nU32); j++ {
			ms[j], err = r.readString()
			if err != nil {
				return nil, err
			}
		}
		m[k] = ms
	}
	return m, nil
}

func readIRBinaryFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return data, nil
	}
	// Some RTG-emitted artifacts may be owner-write-only; try fixing mode then retry.
	_ = os.Chmod(path, 0600)
	data, err2 := os.ReadFile(path)
	if err2 == nil {
		return data, nil
	}
	return nil, err
}

func ReadIRBinary(path string) (*ir.IRModule, error) {
	data, err := readIRBinaryFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	r := &irBinReader{data: data}
	if err := readIRMagic(r); err != nil {
		return nil, fmt.Errorf("read magic at %d: %w", r.off, err)
	}
	types, err := readIRTypes(r)
	if err != nil {
		return nil, fmt.Errorf("read types at %d: %w", r.off, err)
	}
	rootTypes, err := readIRRootTypes(r, types)
	if err != nil {
		return nil, fmt.Errorf("read root types at %d: %w", r.off, err)
	}
	globals, err := readIRGlobals(r, types)
	if err != nil {
		return nil, fmt.Errorf("read globals at %d: %w", r.off, err)
	}
	funcs, err := readIRFuncs(r, types)
	if err != nil {
		return nil, fmt.Errorf("read funcs at %d: %w", r.off, err)
	}
	linkStaticFuncs, err := readStringStringMap(r)
	if err != nil {
		return nil, fmt.Errorf("read linkstatic funcs at %d: %w", r.off, err)
	}
	typeIDs, err := readStringIntMap(r)
	if err != nil {
		return nil, fmt.Errorf("read typeIDs at %d: %w", r.off, err)
	}
	methodTable, err := readStringStringMap(r)
	if err != nil {
		return nil, fmt.Errorf("read method table at %d: %w", r.off, err)
	}
	ifaceMethods, err := readIfaceMethodsMap(r)
	if err != nil {
		return nil, fmt.Errorf("read iface methods at %d: %w", r.off, err)
	}
	ifaceMethodRets, err := readStringIntMap(r)
	if err != nil {
		return nil, fmt.Errorf("read iface method returns at %d: %w", r.off, err)
	}
	return &ir.IRModule{
		Funcs:           funcs,
		Globals:         globals,
		Types:           rootTypes,
		LinkStaticFuncs: linkStaticFuncs,
		TypeIDs:         typeIDs,
		MethodTable:     methodTable,
		IfaceMethods:    ifaceMethods,
		IfaceMethodRets: ifaceMethodRets,
	}, nil
}
