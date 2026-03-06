package ir

// === Type System ===

// TypeKind represents the kind of a type.
type TypeKind int

const (
	TY_VOID TypeKind = iota
	TY_BOOL
	TY_BYTE
	TY_INT32
	TY_INT
	TY_FLOAT64
	TY_UINTPTR
	TY_STRING
	TY_POINTER
	TY_SLICE
	TY_STRUCT
	TY_INTERFACE
	TY_FUNC
	TY_MAP
)

// TypeInfo describes a resolved type.
type TypeInfo struct {
	Kind    TypeKind
	Name    string
	Pkg     string
	Size    int
	Align   int
	Elem    *TypeInfo
	Key     *TypeInfo
	Fields  []FieldInfo
	Params  []*TypeInfo
	Results []*TypeInfo
}

// FieldInfo describes a struct field.
type FieldInfo struct {
	Name   string
	Type   *TypeInfo
	Offset int
}

// === Stack Machine IR ===

// Opcode represents a stack machine instruction.
type Opcode int

const (
	OP_CONST_I64 Opcode = iota
	OP_CONST_F64
	OP_CONST_STR
	OP_CONST_BOOL
	OP_CONST_NIL

	OP_LOCAL_GET
	OP_LOCAL_SET
	OP_LOCAL_ADD_IMM
	OP_LOCAL_ADDR
	OP_GLOBAL_GET
	OP_GLOBAL_SET
	OP_GLOBAL_ADDR

	OP_DROP
	OP_DUP

	OP_ADD
	OP_SUB
	OP_MUL
	OP_DIV
	OP_MOD
	OP_NEG

	OP_AND
	OP_OR
	OP_XOR
	OP_SHL
	OP_SHR

	OP_EQ
	OP_NEQ
	OP_LT
	OP_GT
	OP_LEQ
	OP_GEQ
	OP_JMP_EQ
	OP_JMP_NEQ
	OP_JMP_LT
	OP_JMP_GT
	OP_JMP_LEQ
	OP_JMP_GEQ

	OP_NOT

	OP_LOAD
	OP_STORE
	OP_OFFSET

	OP_LABEL
	OP_JMP
	OP_JMP_IF
	OP_JMP_IF_NOT
	OP_CALL
	OP_CALL_INTRINSIC
	OP_RETURN

	OP_SLICE_GET
	OP_SLICE_MAKE
	OP_STRING_GET
	OP_STRING_MAKE
	OP_INDEX_ADDR
	OP_LEN

	OP_CONVERT

	OP_IFACE_BOX
	OP_IFACE_CALL

	OP_PANIC
	OP_CAP
)

// Inst annotation names used by backend-independent optimization passes.
const (
	// InstNonNilMemoryBase marks LOAD/LEN/CAP instructions whose pointer input
	// is provably non-nil (conservative local proof).
	InstNonNilMemoryBase = "$nonnull_base$"
)

// IsNonNilMemoryBase reports whether inst.Name carries the non-nil base marker.
// Keep this comparison in package ir to avoid imported-const comparison issues in
// selfhosted backend builds.
func IsNonNilMemoryBase(name string) bool {
	return name == InstNonNilMemoryBase
}

// Inst represents a single IR instruction.
type Inst struct {
	Op    Opcode
	Arg   int
	Width int // operand width in bytes: 0=word, 1=byte, 2=int16, 4=int32, 8=int64/float64
	Val   int64
	Name  string
}

// makeInst avoids keyed composite literal field corruption in selfhosted builds.
func makeInst(op Opcode, arg int, width int, val int64, name string) Inst {
	var inst Inst
	inst.Op = op
	inst.Arg = arg
	inst.Width = width
	inst.Val = val
	inst.Name = name
	return inst
}

// IRLocal represents a local variable in a function.
type IRLocal struct {
	Name      string
	Type      *TypeInfo
	Index     int
	Is64      bool // true for uint64/int64 locals (need i64 on wasm32)
	IsFloat64 bool // true for float64 locals (need f64 on wasm32)
	Width     int  // storage width: 0=word, 1=byte, 2=int16, 4=int32, 8=int64/float64
}

// IRFunc represents a compiled function.
type IRFunc struct {
	Name        string
	Params      int
	Locals      []IRLocal
	RetCount    int
	ResultKinds []TypeKind
	ResultIs64  []bool
	Code        []Inst
	Native      *NativeFunc
}

type NativeFixup struct {
	Kind   int
	Off    int
	Target string
}

const (
	NativeFixupCallRel32 = 1
)

type NativeFunc struct {
	Arch   string
	Code   []byte
	Fixups []NativeFixup
}

// IRGlobal represents a global variable.
type IRGlobal struct {
	Name  string
	Type  *TypeInfo
	Index int
}

// IRModule holds all compiled IR.
type IRModule struct {
	Funcs           []*IRFunc
	Globals         []IRGlobal
	Types           []*TypeInfo
	LinkStaticFuncs map[string]string   // intrinsic name → "library,symbol,mode"
	ZeroCallFuncs   map[string]bool     // function/method name → true when calls must be inlined
	TypeIDs         map[string]int      // concrete type → type ID
	MethodTable     map[string]string   // "pkg.Type.Method" → IR func name
	IfaceMethods    map[string][]string // interface name → method names
	IfaceMethodRets map[string]int      // iface+"\x00"+method → return count
	CallbackFuncs   map[string]bool     // function name → true if Win32 callback
}
