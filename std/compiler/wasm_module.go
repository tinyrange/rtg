//go:build !no_backend_wasi_wasm32

package main

// === WASM Binary Module Builder ===
// Builds a complete .wasm binary from types, imports, functions, exports, etc.

// wasmFuncType describes a function signature.
type wasmFuncType struct {
	params  []byte // value types for parameters
	results []byte // value types for results
}

// wasmImport describes an imported function.
type wasmImport struct {
	module  string
	name    string
	typeIdx int
}

// wasmExport describes an exported function or memory.
type wasmExport struct {
	name string
	kind byte   // WASM_EXT_FUNC or WASM_EXT_MEMORY
	idx  uint32 // function or memory index
}

// wasmGlobal describes a WASM global variable.
type wasmGlobal struct {
	valType byte
	mutable bool
	initVal int32
}

// wasmDataSeg describes a data segment to initialize linear memory.
type wasmDataSeg struct {
	offset int32  // memory offset (constant expression)
	data   []byte // bytes to place
}

// wasmModule builds a complete WASM binary.
type wasmModule struct {
	types    []wasmFuncType
	imports  []wasmImport
	funcs    []int    // type index for each function
	exports  []wasmExport
	globals  []wasmGlobal
	codes    [][]byte // encoded function bodies (with local decls)
	datasegs []wasmDataSeg
	memMin   uint32 // minimum memory pages
	memMax   uint32 // maximum memory pages (0 = no max)
}

// typeIdx registers a function type and returns its index, deduplicating.
func (m *wasmModule) typeIdx(params []byte, results []byte) int {
	for i, t := range m.types {
		if len(t.params) == len(params) && len(t.results) == len(results) {
			match := true
			j := 0
			for j < len(t.params) {
				if t.params[j] != params[j] {
					match = false
					break
				}
				j++
			}
			if match {
				j = 0
				for j < len(t.results) {
					if t.results[j] != results[j] {
						match = false
						break
					}
					j++
				}
			}
			if match {
				return i
			}
		}
	}
	idx := len(m.types)
	m.types = append(m.types, wasmFuncType{params: params, results: results})
	return idx
}

// addImport adds an imported function and returns its function index.
func (m *wasmModule) addImport(module string, name string, params []byte, results []byte) int {
	tidx := m.typeIdx(params, results)
	idx := len(m.imports)
	m.imports = append(m.imports, wasmImport{module: module, name: name, typeIdx: tidx})
	return idx
}

// addFunc adds a function (code body added separately) and returns its function index.
func (m *wasmModule) addFunc(params []byte, results []byte) int {
	tidx := m.typeIdx(params, results)
	m.funcs = append(m.funcs, tidx)
	return len(m.imports) + len(m.funcs) - 1
}

// addExport adds an export entry.
func (m *wasmModule) addExport(name string, kind byte, idx uint32) {
	m.exports = append(m.exports, wasmExport{name: name, kind: kind, idx: idx})
}

// addGlobal adds a WASM global and returns its index.
func (m *wasmModule) addGlobal(valType byte, mutable bool, initVal int32) int {
	idx := len(m.globals)
	m.globals = append(m.globals, wasmGlobal{valType: valType, mutable: mutable, initVal: initVal})
	return idx
}

// addData adds a data segment.
func (m *wasmModule) addData(offset int32, data []byte) {
	m.datasegs = append(m.datasegs, wasmDataSeg{offset: offset, data: data})
}

// encode produces the complete .wasm binary.
func (m *wasmModule) encode() []byte {
	var out []byte

	// Magic number + version
	out = append(out, 0x00, 0x61, 0x73, 0x6d) // \0asm
	out = append(out, 0x01, 0x00, 0x00, 0x00) // version 1

	// Type section
	if len(m.types) > 0 {
		out = m.encodeSection(out, WASM_SEC_TYPE, m.encodeTypeSection())
	}

	// Import section
	if len(m.imports) > 0 {
		out = m.encodeSection(out, WASM_SEC_IMPORT, m.encodeImportSection())
	}

	// Function section
	if len(m.funcs) > 0 {
		out = m.encodeSection(out, WASM_SEC_FUNCTION, m.encodeFuncSection())
	}

	// Memory section
	out = m.encodeSection(out, WASM_SEC_MEMORY, m.encodeMemorySection())

	// Global section
	if len(m.globals) > 0 {
		out = m.encodeSection(out, WASM_SEC_GLOBAL, m.encodeGlobalSection())
	}

	// Export section
	if len(m.exports) > 0 {
		out = m.encodeSection(out, WASM_SEC_EXPORT, m.encodeExportSection())
	}

	// Code section
	if len(m.codes) > 0 {
		out = m.encodeSection(out, WASM_SEC_CODE, m.encodeCodeSection())
	}

	// Data section
	if len(m.datasegs) > 0 {
		out = m.encodeSection(out, WASM_SEC_DATA, m.encodeDataSection())
	}

	return out
}

func (m *wasmModule) encodeSection(out []byte, id int, payload []byte) []byte {
	out = append(out, byte(id))
	out = appendULEB128(out, uint32(len(payload)))
	out = append(out, payload...)
	return out
}

func (m *wasmModule) encodeTypeSection() []byte {
	var buf []byte
	buf = appendULEB128(buf, uint32(len(m.types)))
	for _, t := range m.types {
		buf = append(buf, WASM_TYPE_FUNC)
		buf = appendULEB128(buf, uint32(len(t.params)))
		buf = append(buf, t.params...)
		buf = appendULEB128(buf, uint32(len(t.results)))
		buf = append(buf, t.results...)
	}
	return buf
}

func (m *wasmModule) encodeImportSection() []byte {
	var buf []byte
	buf = appendULEB128(buf, uint32(len(m.imports)))
	for _, imp := range m.imports {
		buf = appendULEB128(buf, uint32(len(imp.module)))
		buf = append(buf, []byte(imp.module)...)
		buf = appendULEB128(buf, uint32(len(imp.name)))
		buf = append(buf, []byte(imp.name)...)
		buf = append(buf, WASM_EXT_FUNC)
		buf = appendULEB128(buf, uint32(imp.typeIdx))
	}
	return buf
}

func (m *wasmModule) encodeFuncSection() []byte {
	var buf []byte
	buf = appendULEB128(buf, uint32(len(m.funcs)))
	for _, tidx := range m.funcs {
		buf = appendULEB128(buf, uint32(tidx))
	}
	return buf
}

func (m *wasmModule) encodeMemorySection() []byte {
	var buf []byte
	buf = appendULEB128(buf, 1) // 1 memory
	if m.memMax > 0 {
		buf = append(buf, 0x01) // has max
		buf = appendULEB128(buf, m.memMin)
		buf = appendULEB128(buf, m.memMax)
	} else {
		buf = append(buf, 0x00) // no max
		buf = appendULEB128(buf, m.memMin)
	}
	return buf
}

func (m *wasmModule) encodeGlobalSection() []byte {
	var buf []byte
	buf = appendULEB128(buf, uint32(len(m.globals)))
	for _, g := range m.globals {
		buf = append(buf, g.valType)
		if g.mutable {
			buf = append(buf, 0x01)
		} else {
			buf = append(buf, 0x00)
		}
		// init expression: i32.const val, end
		buf = append(buf, OP_WASM_I32_CONST)
		buf = appendSLEB128(buf, g.initVal)
		buf = append(buf, OP_WASM_END)
	}
	return buf
}

func (m *wasmModule) encodeExportSection() []byte {
	var buf []byte
	buf = appendULEB128(buf, uint32(len(m.exports)))
	for _, exp := range m.exports {
		buf = appendULEB128(buf, uint32(len(exp.name)))
		buf = append(buf, []byte(exp.name)...)
		buf = append(buf, exp.kind)
		buf = appendULEB128(buf, exp.idx)
	}
	return buf
}

func (m *wasmModule) encodeCodeSection() []byte {
	var buf []byte
	buf = appendULEB128(buf, uint32(len(m.codes)))
	for _, body := range m.codes {
		buf = appendULEB128(buf, uint32(len(body)))
		buf = append(buf, body...)
	}
	return buf
}

func (m *wasmModule) encodeDataSection() []byte {
	var buf []byte
	buf = appendULEB128(buf, uint32(len(m.datasegs)))
	for _, seg := range m.datasegs {
		buf = append(buf, 0x00) // memory index 0 (active)
		// offset expression: i32.const offset, end
		buf = append(buf, OP_WASM_I32_CONST)
		buf = appendSLEB128(buf, seg.offset)
		buf = append(buf, OP_WASM_END)
		buf = appendULEB128(buf, uint32(len(seg.data)))
		buf = append(buf, seg.data...)
	}
	return buf
}

// encodeFuncBody builds a complete code entry: local declarations + body + end.
func encodeFuncBody(localCounts []uint32, localTypes []byte, body []byte) []byte {
	var buf []byte
	// Number of local declaration groups
	buf = appendULEB128(buf, uint32(len(localCounts)))
	for i, count := range localCounts {
		buf = appendULEB128(buf, count)
		buf = append(buf, localTypes[i])
	}
	buf = append(buf, body...)
	buf = append(buf, OP_WASM_END) // function end
	return buf
}
