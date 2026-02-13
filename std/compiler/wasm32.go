//go:build !no_backend_wasi_wasm32

package main

// === WASM Binary Format: Opcodes + LEB128 + Code Writer ===

// WASM section IDs
const (
	WASM_SEC_TYPE     = 1
	WASM_SEC_IMPORT   = 2
	WASM_SEC_FUNCTION = 3
	WASM_SEC_MEMORY   = 5
	WASM_SEC_GLOBAL   = 6
	WASM_SEC_EXPORT   = 7
	WASM_SEC_START    = 8
	WASM_SEC_CODE     = 10
	WASM_SEC_DATA     = 11
)

// WASM value types
const (
	WASM_TYPE_I32    = 0x7f
	WASM_TYPE_I64    = 0x7e
	WASM_TYPE_F32    = 0x7d
	WASM_TYPE_F64    = 0x7c
	WASM_TYPE_FUNC   = 0x60
	WASM_TYPE_VOID   = 0x40 // empty block type
)

// WASM opcodes
const (
	OP_WASM_UNREACHABLE = 0x00
	OP_WASM_NOP         = 0x01
	OP_WASM_BLOCK       = 0x02
	OP_WASM_LOOP        = 0x03
	OP_WASM_IF          = 0x04
	OP_WASM_ELSE        = 0x05
	OP_WASM_END         = 0x0b
	OP_WASM_BR          = 0x0c
	OP_WASM_BR_IF       = 0x0d
	OP_WASM_RETURN      = 0x0f
	OP_WASM_CALL        = 0x10
	OP_WASM_DROP        = 0x1a
	OP_WASM_SELECT      = 0x1b

	OP_WASM_LOCAL_GET  = 0x20
	OP_WASM_LOCAL_SET  = 0x21
	OP_WASM_LOCAL_TEE  = 0x22
	OP_WASM_GLOBAL_GET = 0x23
	OP_WASM_GLOBAL_SET = 0x24

	OP_WASM_I32_LOAD     = 0x28
	OP_WASM_I32_LOAD8_U  = 0x2d
	OP_WASM_I32_LOAD16_U = 0x2f
	OP_WASM_I32_STORE    = 0x36
	OP_WASM_I32_STORE8   = 0x3a
	OP_WASM_I32_STORE16  = 0x3b

	OP_WASM_MEMORY_SIZE = 0x3f
	OP_WASM_MEMORY_GROW = 0x40

	OP_WASM_I32_CONST = 0x41

	OP_WASM_I32_EQZ  = 0x45
	OP_WASM_I32_EQ   = 0x46
	OP_WASM_I32_NE   = 0x47
	OP_WASM_I32_LT_S = 0x48
	OP_WASM_I32_GT_S = 0x4a
	OP_WASM_I32_LE_S = 0x4c
	OP_WASM_I32_GE_S = 0x4e

	OP_WASM_I32_CLZ    = 0x67
	OP_WASM_I32_CTZ    = 0x68
	OP_WASM_I32_ADD    = 0x6a
	OP_WASM_I32_SUB    = 0x6b
	OP_WASM_I32_MUL    = 0x6c
	OP_WASM_I32_DIV_S  = 0x6d
	OP_WASM_I32_REM_S  = 0x6f
	OP_WASM_I32_AND    = 0x71
	OP_WASM_I32_OR     = 0x72
	OP_WASM_I32_XOR    = 0x73
	OP_WASM_I32_SHL    = 0x74
	OP_WASM_I32_SHR_S  = 0x75
	OP_WASM_I32_SHR_U  = 0x76

	OP_WASM_I32_WRAP_I64 = 0xa7

	// i64 opcodes
	OP_WASM_I64_CONST = 0x42
	OP_WASM_I64_EQZ   = 0x50
	OP_WASM_I64_EQ    = 0x51
	OP_WASM_I64_NE    = 0x52
	OP_WASM_I64_LT_S  = 0x53
	OP_WASM_I64_GT_S  = 0x55
	OP_WASM_I64_LE_S  = 0x57
	OP_WASM_I64_GE_S  = 0x59
	OP_WASM_I64_ADD   = 0x7c
	OP_WASM_I64_SUB   = 0x7d
	OP_WASM_I64_MUL   = 0x7e
	OP_WASM_I64_DIV_S = 0x7f
	OP_WASM_I64_REM_S = 0x81
	OP_WASM_I64_AND   = 0x83
	OP_WASM_I64_OR    = 0x84
	OP_WASM_I64_XOR   = 0x85
	OP_WASM_I64_SHL   = 0x86
	OP_WASM_I64_SHR_S = 0x87
	OP_WASM_I64_SHR_U = 0x88

	OP_WASM_I64_EXTEND_I32_S = 0xac
	OP_WASM_I64_EXTEND_I32_U = 0xad
)

// External kind for imports/exports
const (
	WASM_EXT_FUNC   = 0x00
	WASM_EXT_MEMORY = 0x02
	WASM_EXT_GLOBAL = 0x03
)

// === LEB128 encoding ===

func appendULEB128(buf []byte, v uint32) []byte {
	for {
		b := byte(v & 0x7f)
		v = v >> 7
		if v != 0 {
			b = b | 0x80
		}
		buf = append(buf, b)
		if v == 0 {
			break
		}
	}
	return buf
}

func appendSLEB128(buf []byte, v int32) []byte {
	more := true
	for more {
		b := byte(v & 0x7f)
		v = v >> 7
		if (v == 0 && (b&0x40) == 0) || (v == -1 && (b&0x40) != 0) {
			more = false
		} else {
			b = b | 0x80
		}
		buf = append(buf, b)
	}
	return buf
}

func appendSLEB128_64(buf []byte, v int64) []byte {
	more := true
	for more {
		b := byte(v & 0x7f)
		v = v >> 7
		if (v == 0 && (b&0x40) == 0) || (v == -1 && (b&0x40) != 0) {
			more = false
		} else {
			b = b | 0x80
		}
		buf = append(buf, b)
	}
	return buf
}

func ulebSize(v uint32) int {
	n := 0
	for {
		v = v >> 7
		n++
		if v == 0 {
			break
		}
	}
	return n
}

// === wasmCodeWriter: builds a WASM code body ===

type wasmCodeWriter struct {
	buf        []byte
	blockDepth int
}

func (w *wasmCodeWriter) byte(b byte) {
	w.buf = append(w.buf, b)
}

func (w *wasmCodeWriter) uleb(v uint32) {
	w.buf = appendULEB128(w.buf, v)
}

func (w *wasmCodeWriter) sleb(v int32) {
	w.buf = appendSLEB128(w.buf, v)
}

func (w *wasmCodeWriter) op(opcode byte) {
	w.buf = append(w.buf, opcode)
}

func (w *wasmCodeWriter) i32Const(v int32) {
	w.op(OP_WASM_I32_CONST)
	w.sleb(v)
}

func (w *wasmCodeWriter) localGet(idx uint32) {
	w.op(OP_WASM_LOCAL_GET)
	w.uleb(idx)
}

func (w *wasmCodeWriter) localSet(idx uint32) {
	w.op(OP_WASM_LOCAL_SET)
	w.uleb(idx)
}

func (w *wasmCodeWriter) localTee(idx uint32) {
	w.op(OP_WASM_LOCAL_TEE)
	w.uleb(idx)
}

func (w *wasmCodeWriter) globalGet(idx uint32) {
	w.op(OP_WASM_GLOBAL_GET)
	w.uleb(idx)
}

func (w *wasmCodeWriter) globalSet(idx uint32) {
	w.op(OP_WASM_GLOBAL_SET)
	w.uleb(idx)
}

func (w *wasmCodeWriter) call(funcIdx uint32) {
	w.op(OP_WASM_CALL)
	w.uleb(funcIdx)
}

func (w *wasmCodeWriter) br(depth uint32) {
	w.op(OP_WASM_BR)
	w.uleb(depth)
}

func (w *wasmCodeWriter) brIf(depth uint32) {
	w.op(OP_WASM_BR_IF)
	w.uleb(depth)
}

func (w *wasmCodeWriter) block(blockType byte) {
	w.op(OP_WASM_BLOCK)
	w.byte(blockType)
	w.blockDepth++
}

func (w *wasmCodeWriter) loop(blockType byte) {
	w.op(OP_WASM_LOOP)
	w.byte(blockType)
	w.blockDepth++
}

func (w *wasmCodeWriter) ifOp(blockType byte) {
	w.op(OP_WASM_IF)
	w.byte(blockType)
	w.blockDepth++
}

func (w *wasmCodeWriter) elseOp() {
	w.op(OP_WASM_ELSE)
}

func (w *wasmCodeWriter) end() {
	w.op(OP_WASM_END)
	w.blockDepth = w.blockDepth - 1
}

func (w *wasmCodeWriter) i32Load(align uint32, offset uint32) {
	w.op(OP_WASM_I32_LOAD)
	w.uleb(align)
	w.uleb(offset)
}

func (w *wasmCodeWriter) i32Load8u(align uint32, offset uint32) {
	w.op(OP_WASM_I32_LOAD8_U)
	w.uleb(align)
	w.uleb(offset)
}

func (w *wasmCodeWriter) i32Load16u(align uint32, offset uint32) {
	w.op(OP_WASM_I32_LOAD16_U)
	w.uleb(align)
	w.uleb(offset)
}

func (w *wasmCodeWriter) i32Store(align uint32, offset uint32) {
	w.op(OP_WASM_I32_STORE)
	w.uleb(align)
	w.uleb(offset)
}

func (w *wasmCodeWriter) i32Store8(align uint32, offset uint32) {
	w.op(OP_WASM_I32_STORE8)
	w.uleb(align)
	w.uleb(offset)
}

func (w *wasmCodeWriter) i32Store16(align uint32, offset uint32) {
	w.op(OP_WASM_I32_STORE16)
	w.uleb(align)
	w.uleb(offset)
}

func (w *wasmCodeWriter) drop() {
	w.op(OP_WASM_DROP)
}

func (w *wasmCodeWriter) returnOp() {
	w.op(OP_WASM_RETURN)
}

func (w *wasmCodeWriter) unreachable() {
	w.op(OP_WASM_UNREACHABLE)
}

// === i64 helpers ===

func (w *wasmCodeWriter) i64Const(v int64) {
	w.op(OP_WASM_I64_CONST)
	w.buf = appendSLEB128_64(w.buf, v)
}

func (w *wasmCodeWriter) i64ExtendI32U() {
	w.op(OP_WASM_I64_EXTEND_I32_U)
}

func (w *wasmCodeWriter) i64ExtendI32S() {
	w.op(OP_WASM_I64_EXTEND_I32_S)
}

func (w *wasmCodeWriter) i32WrapI64() {
	w.op(OP_WASM_I32_WRAP_I64)
}

func (w *wasmCodeWriter) i64Load(align uint32, offset uint32) {
	w.op(0x29) // i64.load
	w.uleb(align)
	w.uleb(offset)
}

func (w *wasmCodeWriter) i64Store(align uint32, offset uint32) {
	w.op(0x37) // i64.store
	w.uleb(align)
	w.uleb(offset)
}
