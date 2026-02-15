package main

import (
	"fmt"
	"os"
	"strings"
)

// === Type System ===

// TypeKind represents the kind of a type.
type TypeKind int

const (
	TY_VOID TypeKind = iota
	TY_BOOL
	TY_BYTE
	TY_INT32
	TY_INT
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
	OP_CONST_STR
	OP_CONST_BOOL
	OP_CONST_NIL

	OP_LOCAL_GET
	OP_LOCAL_SET
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

// Inst represents a single IR instruction.
type Inst struct {
	Op    Opcode
	Arg   int
	Width int // operand width in bytes: 0=word, 1=byte, 2=int16, 4=int32, 8=int64
	Val   int64
	Name  string
}

// IRLocal represents a local variable in a function.
type IRLocal struct {
	Name  string
	Type  *TypeInfo
	Index int
	Is64  bool // true for uint64/int64 locals (need i64 on wasm32)
	Width int  // storage width: 0=word, 1=byte, 2=int16, 4=int32, 8=int64
}

// IRFunc represents a compiled function.
type IRFunc struct {
	Name     string
	Params   int
	Locals   []IRLocal
	RetCount int
	Code     []Inst
}

// IRGlobal represents a global variable.
type IRGlobal struct {
	Name  string
	Type  *TypeInfo
	Index int
}

// IRModule holds all compiled IR.
type IRModule struct {
	Funcs        []*IRFunc
	Globals      []IRGlobal
	Types        []*TypeInfo
	TypeIDs      map[string]int      // concrete type → type ID
	MethodTable  map[string]string   // "pkg.Type.Method" → IR func name
	IfaceMethods map[string][]string // interface name → method names
}

// === Compiler ===

// Compiler lowers AST from a Module into stack machine IR.
type Compiler struct {
	mod                *Module
	irmod              *IRModule
	curFunc            *IRFunc
	scopes             []map[string]int
	labelSeq           int
	breaks             []int
	continues          []int
	globals            map[string]int
	types              map[string]*TypeInfo
	curPkg             *Package
	errors             []string
	funcRets           map[string]int      // function name → return count
	funcParams         map[string]int      // function name → param count
	funcVariadic       map[string]int      // variadic function name → count of fixed params
	funcVariadicIface  map[string]bool     // variadic function name → true if ...interface{}
	funcVariadicElem   map[string]int      // variadic function name → variadic elem size (1 for ...byte, 8 otherwise)
	localElemSizes     map[string]int      // variable name → slice element size (1 for byte, 8 otherwise)
	globalElemSizes    map[string]int      // qualified global name → slice element size
	ifaceMethods       map[string][]string // interface name → method names
	methodTable        map[string]string   // "pkg.Type.Method" → qualified IR func name
	typeIDs            map[string]int      // concrete type qualified name → unique int
	nextTypeID         int
	localTypes         map[string]string   // local var name → type name (for interface-typed locals)
	localStringVars    map[string]bool     // local var name → true if the local is a string
	localConcreteTypes map[string]string   // local var name → qualified type name for method resolution
	funcRetTypes       map[string][]string // function name → return type names
	localMapVars       map[string]int      // local var name → keyKind (0=int, 1=string) if it's a map
	localMapValueTypes map[string]string   // local map var name → value type name (e.g. "*Package")
	globalMapVars      map[string]int      // qualified global name → keyKind if it's a map
	globalConcreteTypes map[string]string  // qualified global name → qualified type name
	constValues        map[string]int64    // qualified const name → precomputed value
	constStringValues  map[string]string   // qualified const name → precomputed string value
	stackDepth         int                 // operand stack depth tracking for balance checks
	deferNames         []string
	deferArgStarts     []int
	deferArgCounts     []int
}

// CompileModule compiles an entire resolved module to IR.
func CompileModule(mod *Module) (*IRModule, []string) {
	c := &Compiler{
		mod:               mod,
		irmod:             &IRModule{},
		globals:           make(map[string]int),
		types:             make(map[string]*TypeInfo),
		funcRets:          make(map[string]int),
		funcParams:        make(map[string]int),
		funcVariadic:      make(map[string]int),
		funcVariadicIface: make(map[string]bool),
		funcVariadicElem:  make(map[string]int),
		globalElemSizes:   make(map[string]int),
		ifaceMethods:      make(map[string][]string),
		methodTable:       make(map[string]string),
		typeIDs:           make(map[string]int),
		nextTypeID:        3, // 1=int, 2=string are reserved
		funcRetTypes:      make(map[string][]string),
		globalMapVars:      make(map[string]int),
		globalConcreteTypes: make(map[string]string),
		constValues:       make(map[string]int64),
		constStringValues: make(map[string]string),
	}
	c.initBuiltinTypes()

	// Register globals for all packages in topological order
	for _, path := range mod.Order {
		pkg, ok := mod.Packages[path]
		if !ok {
			continue
		}
		c.curPkg = pkg
		// Collect and sort variable names for deterministic global ordering.
		// Map iteration order is non-deterministic between Go and RTG runtimes.
		var varNames []string
		for name, sym := range pkg.Symbols {
			if sym.Kind == SymVar {
				varNames = append(varNames, name)
			}
		}
		sortStrings(varNames)
		for _, name := range varNames {
			sym := pkg.Symbols[name]
			qname := pkg.Path + "." + name
			idx := len(c.irmod.Globals)
			c.globals[qname] = idx
			c.irmod.Globals = append(c.irmod.Globals, IRGlobal{Name: qname, Index: idx})
			if sym.Node != nil && sym.Node.Type != nil && sym.Node.Type.Kind == NSliceType {
				c.globalElemSizes[qname] = c.sliceElemSize(sym.Node.Type)
			}
			// Also detect slice composite literal initializers (no explicit type on var)
			if sym.Node != nil && sym.Node.X != nil && sym.Node.X.Kind == NCompositeLit && sym.Node.X.Type != nil && sym.Node.X.Type.Kind == NSliceType {
				c.globalElemSizes[qname] = c.sliceElemSize(sym.Node.X.Type)
			}
			// Track map globals
			if sym.Node != nil && sym.Node.Type != nil && sym.Node.Type.Kind == NMapType {
				c.globalMapVars[qname] = c.mapKeyKind(sym.Node.Type.X)
			}
			// Also detect map composite literal initializers (no explicit type on var)
			if sym.Node != nil && sym.Node.X != nil && sym.Node.X.Kind == NCompositeLit && sym.Node.X.Type != nil && sym.Node.X.Type.Kind == NMapType {
				c.globalMapVars[qname] = c.mapKeyKind(sym.Node.X.Type.X)
			}
			// Track concrete type for struct-typed globals (for method resolution)
			if sym.Node != nil && sym.Node.Type != nil {
				tn := nodeTypeName(sym.Node.Type)
				if tn != "" {
					c.globalConcreteTypes[qname] = c.qualifyTypeName(tn, pkg.Path)
				}
			}
		}
	}

	// Precompute all constant values (with iota tracking)
	for _, path := range mod.Order {
		pkg, ok := mod.Packages[path]
		if !ok {
			continue
		}
		c.curPkg = pkg
		c.precomputeConsts(pkg)
	}

	// Compile functions for all packages in topological order
	for _, path := range mod.Order {
		pkg, ok := mod.Packages[path]
		if !ok {
			continue
		}
		c.curPkg = pkg
		c.compilePackage(pkg)
	}

	// Pass dispatch data to backend
	c.irmod.TypeIDs = c.typeIDs
	c.irmod.MethodTable = c.methodTable
	c.irmod.IfaceMethods = c.ifaceMethods

	return c.irmod, c.errors
}

func (c *Compiler) initBuiltinTypes() {
	c.types["bool"] = &TypeInfo{Kind: TY_BOOL, Name: "bool", Size: 1, Align: 1}
	c.types["byte"] = &TypeInfo{Kind: TY_BYTE, Name: "byte", Size: 1, Align: 1}
	c.types["int32"] = &TypeInfo{Kind: TY_INT32, Name: "int32", Size: 4, Align: 4}
	c.types["int"] = &TypeInfo{Kind: TY_INT, Name: "int", Size: 8, Align: 8}
	c.types["uintptr"] = &TypeInfo{Kind: TY_UINTPTR, Name: "uintptr", Size: 8, Align: 8}
	c.types["string"] = &TypeInfo{Kind: TY_STRING, Name: "string", Size: 16, Align: 8}
	c.types["error"] = &TypeInfo{Kind: TY_INTERFACE, Name: "error", Size: 16, Align: 8}
	c.types["int64"] = &TypeInfo{Kind: TY_INT, Name: "int64", Size: 8, Align: 8}
	c.ifaceMethods["error"] = []string{"Error"}
}

func (c *Compiler) errorf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	c.errors = append(c.errors, msg)
}

func isBuiltinName(name string) bool {
	if len(name) == 0 {
		return false
	}
	switch name[0] {
	case 'l':
		return name == "len"
	case 'c':
		return name == "cap" || name == "copy" || name == "close"
	case 'a':
		return name == "append"
	case 'm':
		return name == "make"
	case 'n':
		return name == "new" || name == "nil"
	case 'p':
		return name == "panic" || name == "print" || name == "println"
	case 'd':
		return name == "delete"
	case 'i':
		return name == "int" || name == "int32" || name == "int64" || name == "iota"
	case 'u':
		return name == "uint" || name == "uint16" || name == "uint32" || name == "uint64" || name == "uintptr"
	case 'b':
		return name == "byte" || name == "bool"
	case 's':
		return name == "string"
	case 'f':
		return name == "float64" || name == "false"
	case 'r':
		return name == "rune"
	case 'e':
		return name == "error"
	case 't':
		return name == "true"
	}
	return false
}

func (c *Compiler) resolvePackage(pkgName string) *Package {
	for _, imp := range c.curPkg.Imports {
		pkg, ok := c.mod.Packages[imp]
		if ok && pkg.Name == pkgName {
			return pkg
		}
	}
	return nil
}

// lookupStructTypeNode parses a qualified type name and returns the struct's type node
// and the package path. Returns nil, "" if not found.
func (c *Compiler) lookupStructTypeNode(qualifiedType string) (*Node, string) {
	dotIdx := -1
	i := 0
	for i < len(qualifiedType) {
		if qualifiedType[i] == '.' {
			dotIdx = i
		}
		i++
	}
	if dotIdx < 0 {
		return nil, ""
	}
	pkgPath := qualifiedType[0:dotIdx]
	typeName := qualifiedType[dotIdx+1 : len(qualifiedType)]
	if len(typeName) > 0 && typeName[0] == '*' {
		typeName = typeName[1:len(typeName)]
	}
	pkg, ok := c.mod.Packages[pkgPath]
	if !ok {
		return nil, ""
	}
	sym, ok := pkg.Symbols[typeName]
	if !ok || sym.Kind != SymType || sym.Node == nil {
		return nil, ""
	}
	typeNode := sym.Node.Type
	if typeNode == nil {
		return nil, ""
	}
	return typeNode, pkgPath
}

// lookupStructField parses a qualified type name and returns the matching field node
// and the package path. Returns nil, "" if not found.
func (c *Compiler) lookupStructField(qualifiedType string, fieldName string) (*Node, string) {
	typeNode, pkgPath := c.lookupStructTypeNode(qualifiedType)
	if typeNode == nil {
		return nil, ""
	}
	for _, field := range typeNode.Nodes {
		if field.Kind == NField && field.Name == fieldName {
			return field, pkgPath
		}
	}
	return nil, ""
}

// resolveFieldType looks up the type of a struct field given a qualified type name and field name.
func (c *Compiler) resolveFieldType(qualifiedType string, fieldName string) string {
	field, pkgPath := c.lookupStructField(qualifiedType, fieldName)
	if field == nil || field.Type == nil {
		return ""
	}
	return c.qualifyTypeName(nodeTypeName(field.Type), pkgPath)
}

// getStructFields returns the field names of a struct type in declaration order.
func (c *Compiler) getStructFields(typeName string) []string {
	qualifiedType := c.qualifyTypeName(typeName, "")
	typeNode, _ := c.lookupStructTypeNode(qualifiedType)
	if typeNode == nil {
		return nil
	}
	var fields []string
	for _, field := range typeNode.Nodes {
		if field.Kind == NField {
			fields = append(fields, field.Name)
		}
	}
	return fields
}

// resolveFieldOffset looks up the byte offset of a struct field given a qualified type name and field name.
func (c *Compiler) resolveFieldOffset(qualifiedType string, fieldName string) int {
	typeNode, _ := c.lookupStructTypeNode(qualifiedType)
	if typeNode == nil {
		return -1
	}
	fieldIdx := 0
	for _, field := range typeNode.Nodes {
		if field.Kind == NField {
			if field.Name == fieldName {
				return fieldIdx * targetPtrSize
			}
			fieldIdx++
		}
	}
	return -1
}

func (c *Compiler) resolveStructSlotCount(qualifiedType string) int {
	typeNode, _ := c.lookupStructTypeNode(qualifiedType)
	if typeNode == nil {
		return 0
	}
	count := 0
	for _, field := range typeNode.Nodes {
		if field.Kind == NField {
			count = count + 1
		}
	}
	return count
}

// typeElemSize returns storage size in bytes for values of typeName when used as
// slice elements in this compiler's lowered representation.
// Non-byte elements are pointer-sized handles to values.
func (c *Compiler) typeElemSize(typeName string) int {
	if typeName == "" {
		return targetPtrSize
	}
	if typeName == "byte" {
		return 1
	}
	return targetPtrSize
}

// resolveFieldElemSize looks up a struct field's type and returns its element size for indexing.
func (c *Compiler) resolveFieldElemSize(qualifiedType string, fieldName string) int {
	field, _ := c.lookupStructField(qualifiedType, fieldName)
	if field == nil || field.Type == nil {
		return 0
	}
	if field.Type.Kind == NSliceType && field.Type.X != nil {
		return c.sliceElemSize(field.Type)
	}
	if field.Type.Kind == NIdent && field.Type.X == nil {
		if field.Type.Name == "string" {
			return 1
		}
	}
	return 0
}

// resolveFieldIsMap checks if a struct field is a map type.
func (c *Compiler) resolveFieldIsMap(qualifiedType string, fieldName string) bool {
	field, _ := c.lookupStructField(qualifiedType, fieldName)
	if field == nil || field.Type == nil {
		return false
	}
	return field.Type.Kind == NMapType
}

// resolveFieldMapKeyKind returns the key kind (0=int, 1=string) for a struct field that is a map.
func (c *Compiler) resolveFieldMapKeyKind(qualifiedType string, fieldName string) int {
	field, _ := c.lookupStructField(qualifiedType, fieldName)
	if field == nil || field.Type == nil || field.Type.Kind != NMapType {
		return -1
	}
	return c.mapKeyKind(field.Type.X)
}

// resolveMapValueType returns the value type name for a map expression.
// For example, if the map is a struct field like ts.pkgs (map[string]*Package), returns "*Package".
func (c *Compiler) resolveMapValueType(mapExpr *Node) string {
	if mapExpr == nil {
		return ""
	}
	if mapExpr.Kind == NIdent {
		if vt, ok := c.localMapValueTypes[mapExpr.Name]; ok {
			return vt
		}
	}
	if mapExpr.Kind == NSelectorExpr && mapExpr.X != nil {
		recvType := c.resolveExprType(mapExpr.X)
		if recvType == "" {
			return ""
		}
		return c.resolveFieldMapValueType(recvType, mapExpr.Name)
	}
	return ""
}

// resolveFieldMapValueType returns the value type name for a struct field that is a map.
func (c *Compiler) resolveFieldMapValueType(qualifiedType string, fieldName string) string {
	field, _ := c.lookupStructField(qualifiedType, fieldName)
	if field == nil || field.Type == nil || field.Type.Kind != NMapType || field.Type.Y == nil {
		return ""
	}
	return nodeTypeName(field.Type.Y)
}

// resolveFieldSliceElemType returns the qualified element type of a struct field that is a slice.
func (c *Compiler) resolveFieldSliceElemType(qualifiedType string, fieldName string) string {
	field, pkgPath := c.lookupStructField(qualifiedType, fieldName)
	if field == nil || field.Type == nil || field.Type.Kind != NSliceType || field.Type.X == nil {
		return ""
	}
	return c.qualifyTypeName(nodeTypeName(field.Type.X), pkgPath)
}

// resolveExprType returns the concrete qualified type of an expression, or "" if unknown.
func (c *Compiler) resolveExprType(node *Node) string {
	if node == nil {
		return ""
	}
	if node.Kind == NIdent {
		if ct, ok := c.localConcreteTypes[node.Name]; ok {
			return ct
		}
		return ""
	}
	// Index expression: determine element type from collection type
	if node.Kind == NIndexExpr && node.X != nil {
		collType := c.resolveExprType(node.X)
		if len(collType) > 2 && collType[0] == '[' && collType[1] == ']' {
			// Slice element type: strip []
			return collType[2:len(collType)]
		}
		// Map value type: strip map[K] to get V
		if len(collType) > 4 && collType[0] == 'm' && collType[1] == 'a' && collType[2] == 'p' && collType[3] == '[' {
			depth := 1
			i := 4
			for i < len(collType) && depth > 0 {
				if collType[i] == '[' {
					depth = depth + 1
				}
				if collType[i] == ']' {
					depth = depth - 1
				}
				i = i + 1
			}
			if i <= len(collType) {
				return collType[i:len(collType)]
			}
		}
		return ""
	}
	// Call expression: check return type
	if node.Kind == NCallExpr {
		calleeName := c.resolveCallName(node.X)
		if retTypes, ok := c.funcRetTypes[calleeName]; ok && len(retTypes) > 0 {
			return c.qualifyTypeName(retTypes[0], "")
		}
		return ""
	}
	if node.Kind == NSelectorExpr && node.X != nil {
		// Check if it's a package-qualified access (not a field access)
		if node.X.Kind == NIdent {
			pkg := c.resolvePackage(node.X.Name)
			if pkg != nil {
				// Package-qualified: look up the symbol type
				sym, ok := pkg.Symbols[node.Name]
				if ok && sym.Kind == SymVar && sym.Node != nil && sym.Node.Type != nil {
					typeName := nodeTypeName(sym.Node.Type)
					return c.qualifyTypeName(typeName, pkg.Path)
				}
				return ""
			}
		}
		// Field access: resolve receiver type, then field type
		recvType := c.resolveExprType(node.X)
		if recvType != "" {
			return c.resolveFieldType(recvType, node.Name)
		}
	}
	return ""
}

// typeWidth returns the byte width for a named type.
// Returns 0 for word-sized types (int, uintptr, pointers, etc).
func typeWidth(name string) int {
	switch name {
	case "byte":
		return 1
	case "uint16":
		return 2
	case "int32", "uint32":
		return 4
	case "int64", "uint64":
		return 8
	}
	return 0
}

// exprWidth infers the operand width from an AST expression.
// Returns 0 for word-sized, or 1/2/4/8 for explicitly sized types.
func (c *Compiler) exprWidth(node *Node) int {
	if node == nil {
		return 0
	}
	switch node.Kind {
	case NIdent:
		// Check if this local has a known concrete type
		if ct, ok := c.localConcreteTypes[node.Name]; ok {
			w := typeWidth(ct)
			if w != 0 {
				return w
			}
		}
		// Check if local has Width set
		if idx, ok := c.lookupLocal(node.Name); ok {
			if idx < len(c.curFunc.Locals) {
				w := c.curFunc.Locals[idx].Width
				if w != 0 {
					return w
				}
			}
		}
	case NCallExpr:
		calleeName := c.resolveCallName(node.X)
		// Type conversions: uint64(), int64(), int32(), byte(), etc.
		tw := typeWidth(calleeName)
		if tw != 0 {
			return tw
		}
		if retTypes, ok := c.funcRetTypes[calleeName]; ok && len(retTypes) > 0 {
			return typeWidth(retTypes[0])
		}
	case NBinaryExpr:
		lw := c.exprWidth(node.X)
		rw := c.exprWidth(node.Y)
		if lw > rw {
			return lw
		}
		return rw
	case NUnaryExpr:
		return c.exprWidth(node.X)
	}
	return 0
}

// precomputeConsts walks all const declarations in a package, tracking iota,
// and stores computed values in c.constValues.
func (c *Compiler) precomputeConsts(pkg *Package) {
	for _, file := range pkg.Files {
		for _, node := range file.Nodes {
			if node.Kind == NConstDecl && len(node.Nodes) > 0 {
				// Grouped const block: iota increments for each child
				var lastExpr *Node
				iotaVal := int64(0)
				for _, child := range node.Nodes {
					qname := pkg.Path + "." + child.Name
					if child.X != nil {
						lastExpr = child.X
					}
					if c.isConstStringExpr(lastExpr) {
						c.constStringValues[qname] = c.evalConstString(lastExpr)
					} else {
						val := c.evalConstExprWithIota(lastExpr, iotaVal)
						c.constValues[qname] = val
					}
					iotaVal++
				}
			} else if node.Kind == NConstDecl {
				// Single const: iota = 0
				qname := pkg.Path + "." + node.Name
				if c.isConstStringExpr(node.X) {
					c.constStringValues[qname] = c.evalConstString(node.X)
				} else {
					c.constValues[qname] = c.evalConstExprWithIota(node.X, 0)
				}
			}
		}
	}
}

// evalConstExprWithIota evaluates a constant expression, substituting the given iota value.
func (c *Compiler) evalConstExprWithIota(node *Node, iotaVal int64) int64 {
	if node == nil {
		return iotaVal
	}
	switch node.Kind {
	case NIntLit:
		return parseIntLiteral(node.Name)
	case NRuneLit:
		return int64(parseRuneLiteral(node.Name))
	case NBasicLit:
		if node.Name == "true" {
			return 1
		}
		if node.Name == "false" {
			return 0
		}
		if node.Name == "iota" {
			return iotaVal
		}
		return 0
	case NIdent:
		// Look up another constant
		qname := c.curPkg.Path + "." + node.Name
		if val, ok := c.constValues[qname]; ok {
			return val
		}
		sym, ok := c.curPkg.Symbols[node.Name]
		if ok && sym.Kind == SymConst {
			return c.resolveConstValue(sym.Node)
		}
		return 0
	case NBinaryExpr:
		left := c.evalConstExprWithIota(node.X, iotaVal)
		right := c.evalConstExprWithIota(node.Y, iotaVal)
		switch node.Name {
		case "+":
			return left + right
		case "-":
			return left - right
		case "*":
			return left * right
		case "/":
			if right != 0 {
				return left / right
			}
			return 0
		case "%":
			if right != 0 {
				return left % right
			}
			return 0
		case "<<":
			return left << uint(right)
		case ">>":
			return left >> uint(right)
		case "|":
			return left | right
		case "&":
			return left & right
		case "^":
			return left ^ right
		default:
			panic("ICE: unhandled binary operator in evalConstExprWithIota")
		}
	case NUnaryExpr:
		val := c.evalConstExprWithIota(node.X, iotaVal)
		if node.Name == "-" {
			return -val
		}
		if node.Name == "^" {
			return ^val
		}
		if node.Name == "!" {
			if val == 0 {
				return 1
			}
			return 0
		}
		panic("ICE: unhandled unary operator in evalConstExprWithIota")
	case NCallExpr:
		// Type conversion in const context
		if node.X != nil && node.X.Kind == NIdent && len(node.Nodes) > 0 {
			return c.evalConstExprWithIota(node.Nodes[0], iotaVal)
		}
		return 0
	}
	return 0
}

func (c *Compiler) isConstStringExpr(node *Node) bool {
	if node == nil {
		return false
	}
	if node.Kind == NStringLit {
		return true
	}
	if node.Kind == NBinaryExpr && node.Name == "+" {
		return c.isConstStringExpr(node.X) || c.isConstStringExpr(node.Y)
	}
	if node.Kind == NIdent {
		qname := c.curPkg.Path + "." + node.Name
		if _, ok := c.constStringValues[qname]; ok {
			return true
		}
	}
	return false
}

func (c *Compiler) evalConstString(node *Node) string {
	if node == nil {
		return ""
	}
	if node.Kind == NStringLit {
		return node.Name
	}
	if node.Kind == NBinaryExpr && node.Name == "+" {
		return c.evalConstString(node.X) + c.evalConstString(node.Y)
	}
	if node.Kind == NIdent {
		qname := c.curPkg.Path + "." + node.Name
		if s, ok := c.constStringValues[qname]; ok {
			return s
		}
	}
	return ""
}

func (c *Compiler) compilePackage(pkg *Package) {
	// Build interface and method tables for this package
	c.buildInterfaceTable(pkg)
	// Pre-pass: collect function return types so they're available during compilation
	c.collectFuncRetTypes(pkg)
	// First, generate init code for global variables with initializers
	c.compileGlobalInits(pkg)
	// Then compile all functions
	for _, file := range pkg.Files {
		for _, node := range file.Nodes {
			c.compileTopDecl(node)
		}
	}
}

func (c *Compiler) collectFuncRetTypes(pkg *Package) {
	for _, file := range pkg.Files {
		for _, node := range file.Nodes {
			fn := node
			if fn.Kind == NDirective && fn.X != nil {
				fn = fn.X
			}
			if fn.Kind != NFunc {
				continue
			}
			qname := pkg.Path + "." + fn.Name
			if fn.X != nil {
				// Method with receiver
				recvType := nodeTypeName(fn.X.Type)
				qname = pkg.Path + "." + recvType + "." + fn.Name
			}
			var retTypeNames []string
			if fn.Type != nil {
				if fn.Type.Kind == NFuncType && len(fn.Type.Nodes) > 0 {
					for _, ret := range fn.Type.Nodes {
						if ret.Type != nil {
							retTypeNames = append(retTypeNames, nodeTypeName(ret.Type))
						} else {
							retTypeNames = append(retTypeNames, nodeTypeName(ret))
						}
					}
				} else {
					retTypeNames = append(retTypeNames, nodeTypeName(fn.Type))
				}
			}
			c.funcRetTypes[qname] = retTypeNames
			c.funcRets[qname] = len(retTypeNames)

			// Pre-register variadic info and param count
			paramCount := len(fn.Nodes)
			fixedParams := 0
			if fn.X != nil {
				paramCount++
				fixedParams = 1 // receiver counts as fixed
			}
			isVariadic := false
			isIfaceVariadic := false
			varElemSize := targetPtrSize
			for _, param := range fn.Nodes {
				if len(param.Name) > 3 && param.Name[0:3] == "..." {
					isVariadic = true
					if param.Type != nil && param.Type.Kind == NInterfaceType {
						isIfaceVariadic = true
					}
					if param.Type != nil && param.Type.Kind == NIdent && param.Type.Name == "byte" {
						varElemSize = 1
					}
				} else {
					fixedParams++
				}
			}
			c.funcParams[qname] = paramCount
			if isVariadic {
				c.funcVariadic[qname] = fixedParams
				c.funcVariadicIface[qname] = isIfaceVariadic
				c.funcVariadicElem[qname] = varElemSize
			}
		}
	}
}

func (c *Compiler) buildInterfaceTable(pkg *Package) {
	for _, file := range pkg.Files {
		for _, node := range file.Nodes {
			c.collectInterfaceDecl(pkg, node)
			c.collectMethodDecl(pkg, node)
		}
	}
}

func (c *Compiler) collectInterfaceDecl(pkg *Package, node *Node) {
	if node == nil {
		return
	}
	if node.Kind == NTypeDecl && node.Type != nil && node.Type.Kind == NInterfaceType {
		qname := pkg.Path + "." + node.Name
		var methods []string
		for _, meth := range node.Type.Nodes {
			if meth.Kind == NFunc {
				methods = append(methods, meth.Name)
			}
		}
		c.ifaceMethods[node.Name] = methods
		c.ifaceMethods[qname] = methods
	}
	if node.Kind == NBlock {
		for _, child := range node.Nodes {
			c.collectInterfaceDecl(pkg, child)
		}
	}
}

func (c *Compiler) collectMethodDecl(pkg *Package, node *Node) {
	if node == nil {
		return
	}
	// Unwrap directives
	if node.Kind == NDirective && node.X != nil {
		c.collectMethodDecl(pkg, node.X)
		return
	}
	if node.Kind == NFunc && node.X != nil {
		// Method with receiver
		recvType := nodeTypeName(node.X.Type)
		qtype := pkg.Path + "." + recvType
		qname := qtype + "." + node.Name
		c.methodTable[qname] = qname
		// Assign type ID if not yet assigned
		if _, ok := c.typeIDs[qtype]; !ok {
			c.typeIDs[qtype] = c.nextTypeID
			c.nextTypeID++
		}
	}
}

func (c *Compiler) compileGlobalInits(pkg *Package) {
	// Collect all global var decls with initializers
	var inits []*Node
	for _, file := range pkg.Files {
		for _, node := range file.Nodes {
			if node.Kind == NVarDecl {
				if node.X != nil {
					inits = append(inits, node)
				} else if len(node.Nodes) > 0 {
					for _, child := range node.Nodes {
						if child.X != nil {
							inits = append(inits, child)
						}
					}
				}
			}
		}
	}

	// Collect embed vars
	var embeds []embedInfo
	for name, sym := range pkg.Symbols {
		if sym.Embed != "" {
			embeds = append(embeds, embedInfo{name: name, pattern: sym.Embed})
		}
	}
	sortEmbeds(embeds)

	if len(inits) == 0 && len(embeds) == 0 {
		return
	}
	// Create a synthetic init function for global var initialization
	f := &IRFunc{Name: pkg.Path + ".init$globals"}
	c.curFunc = f
	c.scopes = nil
	c.localElemSizes = make(map[string]int)
	c.localStringVars = make(map[string]bool)
	c.localConcreteTypes = make(map[string]string)
	c.localMapVars = make(map[string]int)
	c.localMapValueTypes = make(map[string]string)
	c.stackDepth = 0
	c.pushScope()
	for _, node := range inits {
		qname := pkg.Path + "." + node.Name
		gidx, ok := c.globals[qname]
		if !ok {
			continue
		}
		c.compileExpr(node.X)
		c.emit(Inst{Op: OP_GLOBAL_SET, Arg: gidx})
	}

	// Generate embed init code
	for _, emb := range embeds {
		qname := pkg.Path + "." + emb.name
		gidx, ok := c.globals[qname]
		if !ok {
			continue
		}
		c.compileEmbedInit(pkg, gidx, emb.pattern)
	}

	c.emit(Inst{Op: OP_RETURN, Arg: 0})
	if c.stackDepth != 0 {
		panic("ICE: stack not balanced at end of function")
	}
	c.irmod.Funcs = append(c.irmod.Funcs, f)
	c.curFunc = nil
}

func (c *Compiler) compileEmbedInit(pkg *Package, gidx int, pattern string) {
	// Resolve the embed path relative to the package directory
	embedDir := pkg.Dir + "/" + pattern
	// Normalize .. in paths
	embedDir = cleanPath(embedDir)

	// Try embedded FS first (when self-hosting from embedded std),
	// then fall back to disk.
	names, data := walkEmbedFromFS(embedDir)
	if names == nil {
		names, data = walkEmbedDir(embedDir, embedDir)
	}

	// Sort for deterministic order
	sortEmbedFiles(names, data)

	// Create empty FS struct: push 2 nil fields (names, data slices)
	c.emit(Inst{Op: OP_CONST_I64, Val: 0}) // nil names slice
	c.emit(Inst{Op: OP_CONST_I64, Val: 0}) // nil data slice
	c.emit(Inst{Op: OP_CALL, Name: "builtin.composite.embed.FS", Arg: 2})
	c.emit(Inst{Op: OP_GLOBAL_SET, Arg: gidx})

	// For each file, call embed.AddFile(fs, name, data)
	for i := 0; i < len(names); i++ {
		c.emit(Inst{Op: OP_GLOBAL_GET, Arg: gidx})
		c.emit(Inst{Op: OP_CONST_STR, Name: encodeStringLiteral(names[i])})
		c.emit(Inst{Op: OP_CONST_STR, Name: encodeStringLiteral(data[i])})
		c.emit(Inst{Op: OP_CALL, Name: "embed.AddFile", Arg: 3})
	}
}

// cleanPath resolves . and .. in a path.
func cleanPath(path string) string {
	parts := strings.Split(path, "/")
	var clean []string
	for _, p := range parts {
		if p == "." || p == "" {
			continue
		}
		if p == ".." && len(clean) > 0 && clean[len(clean)-1] != ".." {
			clean = clean[0 : len(clean)-1]
		} else {
			clean = append(clean, p)
		}
	}
	result := strings.Join(clean, "/")
	if len(path) > 0 && path[0] == '/' {
		return "/" + result
	}
	return result
}

type embedInfo struct {
	name    string
	pattern string
}

func sortEmbeds(embeds []embedInfo) {
	i := 1
	for i < len(embeds) {
		j := i
		for j > 0 && embeds[j].name < embeds[j-1].name {
			tmp := embeds[j]
			embeds[j] = embeds[j-1]
			embeds[j-1] = tmp
			j = j - 1
		}
		i = i + 1
	}
}

// sortEmbedFiles sorts names and data slices together by name.
func sortEmbedFiles(names []string, data []string) {
	i := 1
	for i < len(names) {
		j := i
		for j > 0 && names[j] < names[j-1] {
			tmpN := names[j]
			names[j] = names[j-1]
			names[j-1] = tmpN
			tmpD := data[j]
			data[j] = data[j-1]
			data[j-1] = tmpD
			j = j - 1
		}
		i = i + 1
	}
}

func (c *Compiler) compileTopDecl(node *Node) {
	if node == nil {
		return
	}
	switch node.Kind {
	case NFunc:
		c.compileFunc(node)
	case NDirective:
		if node.X != nil && node.X.Kind == NFunc {
			c.compileIntrinsicFunc(node)
		}
	case NVarDecl:
		// Global var — init handled separately
	case NConstDecl, NTypeDecl, NBlock, NImport:
		// No code to emit
	default:
		panic("ICE: unhandled top-level declaration kind in compileTopDecl")
	}
}

func (c *Compiler) compileFunc(node *Node) {
	qname := c.curPkg.Path + "." + node.Name
	if node.X != nil {
		// Method with receiver
		recvType := nodeTypeName(node.X.Type)
		qname = c.curPkg.Path + "." + recvType + "." + node.Name
	}
	f := &IRFunc{Name: qname}
	c.curFunc = f
	c.scopes = nil
	c.localElemSizes = make(map[string]int)
	c.localTypes = make(map[string]string)
	c.localStringVars = make(map[string]bool)
	c.localConcreteTypes = make(map[string]string)
	c.localMapVars = make(map[string]int)
	c.localMapValueTypes = make(map[string]string)
	c.deferNames = nil
	c.deferArgStarts = nil
	c.deferArgCounts = nil
	c.pushScope()

	// Extract return type names for interface boxing
	var retTypeNames []string
	if node.Type != nil {
		if node.Type.Kind == NFuncType && len(node.Type.Nodes) > 0 {
			for _, ret := range node.Type.Nodes {
				if ret.Type != nil {
					retTypeNames = append(retTypeNames, nodeTypeName(ret.Type))
				} else {
					retTypeNames = append(retTypeNames, nodeTypeName(ret))
				}
			}
		} else {
			retTypeNames = append(retTypeNames, nodeTypeName(node.Type))
		}
	}
	c.funcRetTypes[qname] = retTypeNames

	// Register receiver as first param
	if node.X != nil {
		c.addLocal(node.X.Name)
		f.Params++
		// Track concrete type of receiver for self-method calls
		if node.X.Type != nil {
			recvType := nodeTypeName(node.X.Type)
			c.localConcreteTypes[node.X.Name] = c.qualifyTypeName(recvType, "")
		}
	}

	// Register params
	isVariadic := false
	isIfaceVariadic := false
	varElemSize := targetPtrSize
	fixedParams := 0
	if node.X != nil {
		fixedParams = 1 // receiver counts as fixed
	}
	for _, param := range node.Nodes {
		pname := param.Name
		isVarParam := false
		if len(pname) > 3 && pname[0:3] == "..." {
			pname = pname[3:]
			isVariadic = true
			isVarParam = true
			if param.Type != nil && param.Type.Kind == NInterfaceType {
				isIfaceVariadic = true
			}
			if param.Type != nil && param.Type.Kind == NIdent && param.Type.Name == "byte" {
				varElemSize = 1
			}
		} else {
			fixedParams++
		}
		if pname != "" {
			localIdx := c.addLocal(pname)
			// Mark uint64/int64 params for i64 on wasm32
			if param.Type != nil && param.Type.Kind == NIdent && (param.Type.Name == "uint64" || param.Type.Name == "int64") {
				c.curFunc.Locals[localIdx].Is64 = true
			}
			// Set Width for explicitly sized params
			if param.Type != nil && param.Type.Kind == NIdent {
				w := typeWidth(param.Type.Name)
				if w != 0 {
					c.curFunc.Locals[localIdx].Width = w
				}
			}
			// Track elem size for slice params
			if isVarParam {
				c.localElemSizes[pname] = varElemSize
			} else if param.Type != nil && param.Type.Kind == NSliceType {
				c.localElemSizes[pname] = c.sliceElemSize(param.Type)
			}
			// Track string-typed params
			if param.Type != nil && param.Type.Kind == NIdent && param.Type.Name == "string" {
				c.localStringVars[pname] = true
			}
			// Track concrete type for method resolution on params
			if param.Type != nil {
				typeName := nodeTypeName(param.Type)
				// Track interface-typed params
				if _, isIface := c.ifaceMethods[typeName]; isIface {
					c.localTypes[pname] = typeName
				}
				ct := c.qualifyTypeName(typeName, "")
				c.localConcreteTypes[pname] = ct
				// Also track slice elem sizes from type
				if len(ct) > 2 && ct[0] == '[' && ct[1] == ']' {
					c.localElemSizes[pname] = c.typeElemSize(ct[2:len(ct)])
				}
				// Track map-typed params
				if param.Type.Kind == NMapType {
					c.localMapVars[pname] = c.mapKeyKind(param.Type.X)
					if param.Type.Y != nil {
						c.localMapValueTypes[pname] = nodeTypeName(param.Type.Y)
					}
				}
			}
		}
		f.Params++
	}

	// Count returns and add named return values as locals
	if node.Type != nil {
		if node.Type.Kind == NFuncType && len(node.Type.Nodes) > 0 {
			f.RetCount = len(node.Type.Nodes)
			for _, ret := range node.Type.Nodes {
				if ret.Name != "" {
					c.addLocal(ret.Name)
				}
			}
		} else {
			f.RetCount = 1
		}
	}

	// Pre-register funcRets before compiling body so recursive calls resolve correctly
	c.funcRets[f.Name] = f.RetCount
	c.stackDepth = 0

	// Compile body
	if node.Body != nil {
		c.compileBlock(node.Body)
	}

	// Ensure function ends with a return
	codeLen := len(f.Code)
	if codeLen == 0 || f.Code[codeLen-1].Op != OP_RETURN {
		if len(c.deferNames) > 0 {
			c.emitDeferredCalls()
		}
		c.emit(Inst{Op: OP_RETURN, Arg: 0})
	}

	c.popScope()
	c.funcRets[f.Name] = f.RetCount
	c.funcParams[f.Name] = f.Params
	if isVariadic {
		c.funcVariadic[f.Name] = fixedParams
		c.funcVariadicIface[f.Name] = isIfaceVariadic
		c.funcVariadicElem[f.Name] = varElemSize
	}
	c.irmod.Funcs = append(c.irmod.Funcs, f)
	c.curFunc = nil
}

func (c *Compiler) compileIntrinsicFunc(directive *Node) {
	node := directive.X
	qname := c.curPkg.Path + "." + node.Name
	intern := parseInternalDirective(directive.Name)

	f := &IRFunc{Name: qname}
	c.curFunc = f

	// Count params and detect variadic
	paramCount := len(node.Nodes)
	if node.X != nil {
		paramCount++
	}
	f.Params = paramCount
	isVariadic := false
	fixedParams := 0
	if node.X != nil {
		fixedParams = 1
	}
	varElemSizeI := targetPtrSize
	for _, param := range node.Nodes {
		if len(param.Name) > 3 && param.Name[0:3] == "..." {
			isVariadic = true
			if param.Type != nil && param.Type.Kind == NIdent && param.Type.Name == "byte" {
				varElemSizeI = 1
			}
		} else {
			fixedParams++
		}
	}

	// Count returns
	if node.Type != nil {
		if node.Type.Kind == NFuncType && len(node.Type.Nodes) > 0 {
			f.RetCount = len(node.Type.Nodes)
		} else {
			f.RetCount = 1
		}
	}

	// Emit single intrinsic call
	c.stackDepth = 0
	c.emit(Inst{Op: OP_CALL_INTRINSIC, Name: intern, Arg: paramCount})
	c.emit(Inst{Op: OP_RETURN, Arg: f.RetCount})

	c.funcRets[f.Name] = f.RetCount
	c.funcParams[f.Name] = f.Params
	if isVariadic {
		c.funcVariadic[f.Name] = fixedParams
		c.funcVariadicElem[f.Name] = varElemSizeI
	}
	c.irmod.Funcs = append(c.irmod.Funcs, f)
	c.curFunc = nil
}

// === Scope management ===

func (c *Compiler) pushScope() {
	c.scopes = append(c.scopes, make(map[string]int))
}

func (c *Compiler) popScope() {
	if len(c.scopes) > 0 {
		c.scopes = c.scopes[0 : len(c.scopes)-1]
	}
}

func (c *Compiler) addLocal(name string) int {
	idx := len(c.curFunc.Locals)
	c.curFunc.Locals = append(c.curFunc.Locals, IRLocal{Name: name, Index: idx})
	if len(c.scopes) > 0 {
		c.scopes[len(c.scopes)-1][name] = idx
	}
	return idx
}

func (c *Compiler) lookupLocal(name string) (int, bool) {
	i := len(c.scopes) - 1
	for i >= 0 {
		idx, ok := c.scopes[i][name]
		if ok {
			return idx, true
		}
		i = i - 1
	}
	return 0, false
}

func (c *Compiler) newLabel() int {
	l := c.labelSeq
	c.labelSeq++
	return l
}

func (c *Compiler) emit(inst Inst) {
	c.curFunc.Code = append(c.curFunc.Code, inst)
	c.stackDepth = c.stackDepth + c.instStackDelta(inst)
}

func (c *Compiler) instStackDelta(inst Inst) int {
	switch inst.Op {
	case OP_CONST_I64, OP_CONST_STR, OP_CONST_BOOL, OP_CONST_NIL:
		return 1
	case OP_LOCAL_GET, OP_GLOBAL_GET, OP_LOCAL_ADDR, OP_GLOBAL_ADDR:
		return 1
	case OP_LOCAL_SET, OP_GLOBAL_SET:
		return -1
	case OP_DROP:
		return -1
	case OP_DUP:
		return 1
	case OP_ADD, OP_SUB, OP_MUL, OP_DIV, OP_MOD:
		return -1
	case OP_AND, OP_OR, OP_XOR, OP_SHL, OP_SHR:
		return -1
	case OP_EQ, OP_NEQ, OP_LT, OP_GT, OP_LEQ, OP_GEQ:
		return -1
	case OP_NEG, OP_NOT:
		return 0
	case OP_LOAD:
		return 0 // pop addr, push value
	case OP_STORE:
		return -2 // pop addr + value
	case OP_OFFSET:
		return 0
	case OP_LABEL, OP_JMP:
		return 0
	case OP_JMP_IF, OP_JMP_IF_NOT:
		return -1
	case OP_CALL:
		retCount := 0
		if len(inst.Name) > 18 && inst.Name[0:18] == "builtin.composite." {
			retCount = 1 // composite literal calls consume N fields, produce 1 pointer
		} else if n, ok := c.funcRets[inst.Name]; ok {
			retCount = n
		}
		return -inst.Arg + retCount
	case OP_CALL_INTRINSIC:
		// Intrinsics read params from frame, only push results
		if c.curFunc != nil {
			return c.curFunc.RetCount
		}
		return 0
	case OP_RETURN:
		return -inst.Arg
	case OP_INDEX_ADDR:
		return -1 // pop base + index, push addr
	case OP_LEN:
		return 0 // pop header, push len
	case OP_CAP:
		return 0 // pop header, push cap
	case OP_CONVERT:
		return 0
	case OP_IFACE_BOX:
		return 0 // pop value, push boxed
	case OP_IFACE_CALL:
		// consumes receiver + args, produces 1 result
		return -(inst.Arg + 1) + 1
	case OP_PANIC:
		return -1
	case OP_SLICE_GET, OP_SLICE_MAKE, OP_STRING_GET, OP_STRING_MAKE:
		return 0
	}
	panic("ICE: unknown opcode in instStackDelta")
}

func (c *Compiler) blockEndsWithReturn() bool {
	if c.curFunc == nil || len(c.curFunc.Code) == 0 {
		return false
	}
	last := c.curFunc.Code[len(c.curFunc.Code)-1]
	return last.Op == OP_RETURN || last.Op == OP_PANIC
}

func (c *Compiler) emitLabel(label int) {
	c.emit(Inst{Op: OP_LABEL, Arg: label})
}

// === Statement compilation ===

func (c *Compiler) compileBlock(node *Node) {
	c.pushScope()
	for _, stmt := range node.Nodes {
		c.compileStmt(stmt)
	}
	c.popScope()
}

func (c *Compiler) compileStmt(node *Node) {
	if node == nil {
		return
	}
	switch node.Kind {
	case NVarDecl:
		c.compileVarDecl(node)
	case NAssign:
		c.compileAssign(node)
	case NReturn:
		c.compileReturn(node)
	case NIf:
		c.compileIf(node)
	case NFor:
		c.compileFor(node)
	case NSwitch:
		c.compileSwitch(node)
	case NExprStmt:
		c.compileExpr(node.X)
		// Drop return values left on the operand stack
		retCount := c.exprReturnCount(node.X)
		i := 0
		for i < retCount {
			c.emit(Inst{Op: OP_DROP})
			i++
		}
	case NIncStmt:
		c.compileInc(node)
	case NBranch:
		c.compileBranch(node)
	case NDeferStmt:
		if node.X != nil && node.X.Kind == NCallExpr {
			name := c.resolveCallName(node.X.X)
			argStart := -1
			argCount := 0
			for _, arg := range node.X.Nodes {
				c.compileExpr(arg)
				idx := c.addLocal(fmt.Sprintf("_defer_%d_%d", len(c.deferNames), argCount))
				c.emit(Inst{Op: OP_LOCAL_SET, Arg: idx})
				if argStart < 0 {
					argStart = idx
				}
				argCount++
			}
			if argStart < 0 {
				argStart = 0
			}
			c.deferNames = append(c.deferNames, name)
			c.deferArgStarts = append(c.deferArgStarts, argStart)
			c.deferArgCounts = append(c.deferArgCounts, argCount)
		}
	case NConstDecl:
		// Local const — treat like var
		if len(node.Nodes) > 0 {
			for _, child := range node.Nodes {
				c.compileVarDecl(child)
			}
		} else {
			c.compileVarDecl(node)
		}
	case NBlock:
		c.compileBlock(node)
	default:
		panic("ICE: unhandled statement kind in compileStmt")
	}
}

func (c *Compiler) compileVarDecl(node *Node) {
	idx := c.addLocal(node.Name)
	// Mark uint64/int64 locals for i64 on wasm32
	if node.Type != nil && node.Type.Kind == NIdent && (node.Type.Name == "uint64" || node.Type.Name == "int64") {
		c.curFunc.Locals[idx].Is64 = true
	}
	// Set Width for explicitly sized locals
	if node.Type != nil && node.Type.Kind == NIdent {
		w := typeWidth(node.Type.Name)
		if w != 0 {
			c.curFunc.Locals[idx].Width = w
		}
	}
	// Track element size for slice variables
	if node.Type != nil && node.Type.Kind == NSliceType {
		c.localElemSizes[node.Name] = c.sliceElemSize(node.Type)
	}
	// Track string-typed variables
	if node.Type != nil && node.Type.Kind == NIdent && node.Type.Name == "string" {
		c.localStringVars[node.Name] = true
	}
	// Track map-typed variables
	if node.Type != nil && node.Type.Kind == NMapType {
		c.localMapVars[node.Name] = c.mapKeyKind(node.Type.X)
		if node.Type.Y != nil {
			c.localMapValueTypes[node.Name] = nodeTypeName(node.Type.Y)
		}
	}
	// Track interface-typed variables
	if node.Type != nil {
		typeName := nodeTypeName(node.Type)
		if _, isIface := c.ifaceMethods[typeName]; isIface {
			c.localTypes[node.Name] = typeName
		}
		// Track concrete type for struct field access and method resolution
		ct := c.qualifyTypeName(typeName, "")
		c.localConcreteTypes[node.Name] = ct
	}
	if node.X != nil {
		c.compileExpr(node.X)
		c.emit(Inst{Op: OP_LOCAL_SET, Arg: idx, Width: c.curFunc.Locals[idx].Width})
	} else {
		// Struct locals are represented as pointers to heap-allocated storage.
		// A zero-value struct var must still be addressable and non-nil.
		if node.Type != nil {
			rawTypeName := nodeTypeName(node.Type)
			typeName := c.qualifyTypeName(rawTypeName, "")
			typeNode, _ := c.lookupStructTypeNode(typeName)
			// Only value-struct locals get implicit storage. Pointer locals
			// (e.g. *Parser) must remain nil-zero by default.
			if typeNode != nil && (len(rawTypeName) == 0 || rawTypeName[0] != '*') {
				slots := c.resolveStructSlotCount(typeName)
				size := slots * targetPtrSize
				if size <= 0 {
					size = targetPtrSize
				}
				c.emit(Inst{Op: OP_CONST_I64, Val: int64(size)})
				c.emit(Inst{Op: OP_CALL, Name: "runtime.Alloc", Arg: 1})
				c.emit(Inst{Op: OP_DUP})
				c.emit(Inst{Op: OP_CONST_I64, Val: int64(size)})
				c.emit(Inst{Op: OP_CALL, Name: "runtime.Memzero", Arg: 2})
				c.emit(Inst{Op: OP_LOCAL_SET, Arg: idx})
				return
			}
		}
		// Zero-initialize the local to avoid stack garbage
		c.emit(Inst{Op: OP_CONST_I64, Val: 0})
		c.emit(Inst{Op: OP_LOCAL_SET, Arg: idx})
	}
}

func (c *Compiler) compileAssign(node *Node) {
	if len(node.Nodes) > 0 {
		// Multi-value assignment with comma-separated RHS: a, b := 1, 2
		if node.Body != nil && node.Body.Kind == NBlock && len(node.Body.Nodes) > 0 {
			for _, rhs := range node.Body.Nodes {
				c.compileExpr(rhs)
			}
			i := len(node.Nodes) - 1
			for i >= 0 {
				lhs := node.Nodes[i]
				if node.Name == ":=" {
					idx := c.addLocal(lhs.Name)
					c.emit(Inst{Op: OP_LOCAL_SET, Arg: idx})
				} else {
					c.compileLValueSet(lhs)
				}
				i = i - 1
			}
			return
		}

		// Multi-value map index: v, ok := m[key]
		if node.Y != nil && node.Y.Kind == NIndexExpr && c.isMapExpr(node.Y.X) {
			c.compileExpr(node.Y.X) // push map
			c.compileExpr(node.Y.Y) // push key
			c.emit(Inst{Op: OP_CALL, Name: "runtime.MapGet", Arg: 2})
			// MapGet returns (value, ok) — both on stack
			// Assign in reverse order: ok first (top of stack), then value
			i := len(node.Nodes) - 1
			for i >= 0 {
				lhs := node.Nodes[i]
				if node.Name == ":=" {
					idx := c.addLocal(lhs.Name)
					c.emit(Inst{Op: OP_LOCAL_SET, Arg: idx})
				} else {
					c.compileLValueSet(lhs)
				}
				i = i - 1
			}
			// Track concrete type of the map value variable (node.Nodes[0])
			if node.Name == ":=" && len(node.Nodes) >= 1 {
				valType := c.resolveMapValueType(node.Y.X)
				if valType != "" {
					c.localConcreteTypes[node.Nodes[0].Name] = c.qualifyTypeName(valType, "")
				}
			}
			return
		}

		// Multi-value assignment: a, b = expr or a, b := expr
		c.compileExpr(node.Y)

		// Track interface-typed, string-typed, and concrete-typed locals from multi-value := assignments
		if node.Name == ":=" && node.Y != nil && node.Y.Kind == NCallExpr {
			calleeName := c.resolveCallName(node.Y.X)
			if retTypes, ok := c.funcRetTypes[calleeName]; ok {
				// Determine the package of the callee for type qualification
				calleePkg := ""
				if node.Y.X != nil && node.Y.X.Kind == NSelectorExpr && node.Y.X.X != nil {
					pkg := c.resolvePackage(node.Y.X.X.Name)
					if pkg != nil {
						calleePkg = pkg.Path
					}
				}
				for j, lhs := range node.Nodes {
					if j < len(retTypes) {
						qret := c.qualifyTypeName(retTypes[j], calleePkg)
						if _, isIface := c.ifaceMethods[retTypes[j]]; isIface {
							c.localTypes[lhs.Name] = retTypes[j]
						}
						if retTypes[j] == "string" {
							c.localStringVars[lhs.Name] = true
						}
						if len(retTypes[j]) > 2 && retTypes[j][0] == '[' && retTypes[j][1] == ']' {
							elemType := qret[2:len(qret)]
							c.localElemSizes[lhs.Name] = c.typeElemSize(elemType)
						}
						if keyType, valType, ok := parseMapTypeName(qret); ok {
							if keyType == "string" {
								c.localMapVars[lhs.Name] = 1
							} else {
								c.localMapVars[lhs.Name] = 0
							}
							c.localMapValueTypes[lhs.Name] = valType
						}
						// Track concrete type for method resolution
						c.localConcreteTypes[lhs.Name] = qret
					}
				}
			}
		}

		// Assign to each LHS in reverse order (values are on stack)
		i := len(node.Nodes) - 1
		for i >= 0 {
			lhs := node.Nodes[i]
			if node.Name == ":=" {
				idx := c.addLocal(lhs.Name)
				c.emit(Inst{Op: OP_LOCAL_SET, Arg: idx})
			} else {
				c.compileLValueSet(lhs)
			}
			i = i - 1
		}
		return
	}

	if node.Name == ":=" {
		// Short var decl
		idx := c.addLocal(node.X.Name)
		// Infer width from RHS expression for int64/uint64/etc.
		w := c.exprWidth(node.Y)
		if w != 0 {
			c.curFunc.Locals[idx].Width = w
			if w == 8 {
				c.curFunc.Locals[idx].Is64 = true
			}
		}
		// Track string-typed short vars
		if c.isStringTypedExpr(node.Y) {
			c.localStringVars[node.X.Name] = true
		}
		// Track concrete type and elem size for method resolution and indexing
		if ct := c.exprConcreteType(node.Y); ct != "" {
			c.localConcreteTypes[node.X.Name] = ct
			// Track slice elem sizes
			if len(ct) > 2 && ct[0] == '[' && ct[1] == ']' {
				c.localElemSizes[node.X.Name] = c.typeElemSize(ct[2:len(ct)])
			}
			// Track map variables from concrete return type
			if keyType, valType, ok := parseMapTypeName(ct); ok {
				if keyType == "string" {
					c.localMapVars[node.X.Name] = 1
				} else {
					c.localMapVars[node.X.Name] = 0
				}
				c.localMapValueTypes[node.X.Name] = valType
			}
		}
		// Track map variables from composite literals: m := map[K]V{...}
		if node.Y != nil && node.Y.Kind == NCompositeLit && node.Y.Type != nil && node.Y.Type.Kind == NMapType {
			c.localMapVars[node.X.Name] = c.mapKeyKind(node.Y.Type.X)
			if node.Y.Type.Y != nil {
				c.localMapValueTypes[node.X.Name] = nodeTypeName(node.Y.Type.Y)
			}
		}
		// Track slice and map variables from make() calls
		if node.Y != nil && node.Y.Kind == NCallExpr && node.Y.X != nil && node.Y.X.Kind == NIdent && node.Y.X.Name == "make" {
			if len(node.Y.Nodes) > 0 && node.Y.Nodes[0].Kind == NSliceType {
				c.localElemSizes[node.X.Name] = c.sliceElemSize(node.Y.Nodes[0])
			}
			if len(node.Y.Nodes) > 0 && node.Y.Nodes[0].Kind == NMapType {
				c.localMapVars[node.X.Name] = c.mapKeyKind(node.Y.Nodes[0].X)
				if node.Y.Nodes[0].Y != nil {
					c.localMapValueTypes[node.X.Name] = nodeTypeName(node.Y.Nodes[0].Y)
				}
			}
		}
		c.compileExpr(node.Y)
		c.emit(Inst{Op: OP_LOCAL_SET, Arg: idx, Width: w})
		return
	}

	if node.Name == "+=" {
		w := c.exprWidth(node.X)
		c.compileLValueGet(node.X)
		c.compileExpr(node.Y)
		c.emit(Inst{Op: OP_ADD, Width: w})
		c.compileLValueSet(node.X)
		return
	}

	if node.Name == "-=" {
		w := c.exprWidth(node.X)
		c.compileLValueGet(node.X)
		c.compileExpr(node.Y)
		c.emit(Inst{Op: OP_SUB, Width: w})
		c.compileLValueSet(node.X)
		return
	}

	if node.Name == "*=" {
		w := c.exprWidth(node.X)
		c.compileLValueGet(node.X)
		c.compileExpr(node.Y)
		c.emit(Inst{Op: OP_MUL, Width: w})
		c.compileLValueSet(node.X)
		return
	}

	if node.Name == "/=" {
		w := c.exprWidth(node.X)
		c.compileLValueGet(node.X)
		c.compileExpr(node.Y)
		c.emit(Inst{Op: OP_DIV, Width: w})
		c.compileLValueSet(node.X)
		return
	}

	if node.Name == "%=" {
		w := c.exprWidth(node.X)
		c.compileLValueGet(node.X)
		c.compileExpr(node.Y)
		c.emit(Inst{Op: OP_MOD, Width: w})
		c.compileLValueSet(node.X)
		return
	}

	if node.Name == "|=" {
		w := c.exprWidth(node.X)
		c.compileLValueGet(node.X)
		c.compileExpr(node.Y)
		c.emit(Inst{Op: OP_OR, Width: w})
		c.compileLValueSet(node.X)
		return
	}

	if node.Name == "&=" {
		w := c.exprWidth(node.X)
		c.compileLValueGet(node.X)
		c.compileExpr(node.Y)
		c.emit(Inst{Op: OP_AND, Width: w})
		c.compileLValueSet(node.X)
		return
	}

	if node.Name == "^=" {
		w := c.exprWidth(node.X)
		c.compileLValueGet(node.X)
		c.compileExpr(node.Y)
		c.emit(Inst{Op: OP_XOR, Width: w})
		c.compileLValueSet(node.X)
		return
	}

	if node.Name == "<<=" {
		w := c.exprWidth(node.X)
		c.compileLValueGet(node.X)
		c.compileExpr(node.Y)
		c.emit(Inst{Op: OP_SHL, Width: w})
		c.compileLValueSet(node.X)
		return
	}

	if node.Name == ">>=" {
		w := c.exprWidth(node.X)
		c.compileLValueGet(node.X)
		c.compileExpr(node.Y)
		c.emit(Inst{Op: OP_SHR, Width: w})
		c.compileLValueSet(node.X)
		return
	}

	// Map index assignment: m[key] = val
	if node.X != nil && node.X.Kind == NIndexExpr && c.isMapExpr(node.X.X) {
		c.compileExpr(node.X.X) // push map
		c.compileExpr(node.X.Y) // push key
		c.compileExpr(node.Y)   // push value
		c.emit(Inst{Op: OP_CALL, Name: "runtime.MapSet", Arg: 3})
		c.emit(Inst{Op: OP_DROP}) // discard returned header (unchanged)
		return
	}

	// Regular assignment
	c.compileExpr(node.Y)
	c.compileLValueSet(node.X)
}

func (c *Compiler) compileLValueSet(node *Node) {
	if node == nil {
		return
	}
	switch node.Kind {
	case NIdent:
		if node.Name == "_" {
			c.emit(Inst{Op: OP_DROP})
			return
		}
		idx, ok := c.lookupLocal(node.Name)
		if ok {
			w := 0
			if idx < len(c.curFunc.Locals) {
				w = c.curFunc.Locals[idx].Width
			}
			c.emit(Inst{Op: OP_LOCAL_SET, Arg: idx, Width: w})
		} else {
			gidx, gok := c.lookupGlobal(node.Name)
			if gok {
				c.emit(Inst{Op: OP_GLOBAL_SET, Arg: gidx})
			}
		}
	case NIndexExpr:
		elemSize := c.exprElemSize(node.X)
		c.compileExpr(node.X)
		c.compileExpr(node.Y)
		c.emit(Inst{Op: OP_INDEX_ADDR, Arg: elemSize})
		c.emit(Inst{Op: OP_STORE, Arg: elemSize})
	case NSelectorExpr:
		offset := 0
		recvType := c.resolveExprType(node.X)
		if recvType != "" {
			off := c.resolveFieldOffset(recvType, node.Name)
			if off >= 0 {
				offset = off
			}
		}
		// NOTE: resolveFieldOffsetByName fallback removed - too ambiguous
		c.compileExpr(node.X)
		c.emit(Inst{Op: OP_OFFSET, Arg: offset})
		c.emit(Inst{Op: OP_STORE, Arg: targetPtrSize})
	case NUnaryExpr:
		if node.Name == "*" {
			c.compileExpr(node.X)
			c.emit(Inst{Op: OP_STORE, Arg: targetPtrSize})
		}
	default:
		panic("ICE: unhandled lvalue kind in compileLValueSet")
	}
}

func (c *Compiler) compileLValueGet(node *Node) {
	if node == nil {
		return
	}
	switch node.Kind {
	case NIdent:
		c.compileExpr(node)
	case NIndexExpr:
		c.compileExpr(node)
	case NSelectorExpr:
		c.compileExpr(node)
	default:
		panic("ICE: unhandled lvalue kind in compileLValueGet")
	}
}

func (c *Compiler) emitDeferredCalls() {
	n := len(c.deferNames)
	di := 0
	for di < n {
		idx := n - 1 - di
		name := c.deferNames[idx]
		argStart := c.deferArgStarts[idx]
		argCount := c.deferArgCounts[idx]
		k := 0
		for k < argCount {
			c.emit(Inst{Op: OP_LOCAL_GET, Arg: argStart + k})
			k++
		}
		c.emit(Inst{Op: OP_CALL, Name: name, Arg: argCount})
		di++
	}
}

func (c *Compiler) compileReturn(node *Node) {
	count := 0
	retTypes := c.funcRetTypes[c.curFunc.Name]

	if node.X != nil {
		c.compileExpr(node.X)
		c.maybeBoxInterface(node.X, retTypes, 0)
		count++
	}
	for i, extra := range node.Nodes {
		c.compileExpr(extra)
		c.maybeBoxInterface(extra, retTypes, i+1)
		count++
	}
	if len(c.deferNames) > 0 {
		c.emitDeferredCalls()
	}
	c.emit(Inst{Op: OP_RETURN, Arg: count})
}

// maybeBoxInterface checks if the return value at position idx needs boxing
// (i.e., the expected return type is an interface). If so, emits OP_IFACE_BOX.
func (c *Compiler) maybeBoxInterface(expr *Node, retTypes []string, idx int) {
	if idx >= len(retTypes) {
		return
	}
	expectedType := retTypes[idx]
	_, isIface := c.ifaceMethods[expectedType]
	if !isIface {
		return
	}
	// Don't box nil
	if expr.Kind == NBasicLit && expr.Name == "nil" {
		return
	}
	// Don't box passthrough calls that already return the interface type
	if expr.Kind == NCallExpr {
		calleeName := c.resolveCallName(expr.X)
		if calleeRetTypes, ok := c.funcRetTypes[calleeName]; ok {
			if idx < len(calleeRetTypes) {
				if _, calleeReturnsIface := c.ifaceMethods[calleeRetTypes[idx]]; calleeReturnsIface {
					return // callee already boxes
				}
			}
		}
	}
	typeID := c.resolveConcreteTypeID(expr)
	if typeID > 0 {
		c.emit(Inst{Op: OP_IFACE_BOX, Arg: typeID})
	}
}

// exprPrimitiveTypeID returns the type ID for boxing a primitive value as interface{}.
// Returns 1 for int, 2 for string, or the concrete type ID for named types.
// Returns 0 if the value is already an interface or doesn't need boxing.
func (c *Compiler) exprPrimitiveTypeID(expr *Node) int {
	if expr == nil {
		return 0
	}
	switch expr.Kind {
	case NIntLit, NRuneLit:
		return 1 // int
	case NStringLit:
		return 2 // string
	case NIdent:
		if expr.Name == "true" || expr.Name == "false" {
			return 1 // bool treated as int
		}
		if expr.Name == "nil" {
			return 0 // nil doesn't need boxing
		}
		if c.localStringVars[expr.Name] {
			return 2 // string variable
		}
		// Check if it's a local — assume int if not a known slice/interface
		if _, isSlice := c.localElemSizes[expr.Name]; isSlice {
			return 0 // slice, pass as-is
		}
		if _, isIface := c.localTypes[expr.Name]; isIface {
			return 0 // already interface
		}
		// Check if it's a global string var
		if c.curPkg != nil {
			if sym, ok := c.curPkg.Symbols[expr.Name]; ok && sym.Kind == SymVar && sym.Node != nil && sym.Node.Type != nil {
				if nodeTypeName(sym.Node.Type) == "string" {
					return 2
				}
			}
		}
		return 1 // default to int for unknown locals
	case NBinaryExpr:
		if expr.Name == "+" || expr.Name == "-" || expr.Name == "*" || expr.Name == "/" || expr.Name == "%" {
			if c.isStringTypedExpr(expr) {
				return 2
			}
			return 1
		}
		if expr.Name == "==" || expr.Name == "!=" || expr.Name == "<" || expr.Name == ">" || expr.Name == "<=" || expr.Name == ">=" || expr.Name == "&&" || expr.Name == "||" {
			return 1 // comparison returns int/bool
		}
	case NCallExpr:
		if expr.X != nil && expr.X.Kind == NIdent {
			name := expr.X.Name
			if name == "len" || name == "cap" || name == "int" || name == "uintptr" || name == "byte" || name == "int32" || name == "int64" {
				return 1
			}
			if name == "string" {
				return 2
			}
		}
		calleeName := c.resolveCallName(expr.X)
		if retTypes, ok := c.funcRetTypes[calleeName]; ok && len(retTypes) > 0 {
			if retTypes[0] == "string" {
				return 2
			}
			if retTypes[0] == "error" || retTypes[0] == "interface{}" {
				return 0 // already interface
			}
		}
		// Check if callee returns a known named type with a type_id
		typeID := c.resolveConcreteTypeID(expr)
		if typeID > 0 {
			return typeID
		}
		return 1 // default to int for unknown calls
	case NUnaryExpr:
		if expr.Name == "&" {
			typeID := c.resolveConcreteTypeID(expr)
			if typeID > 0 {
				return typeID
			}
			return 0 // pointer, might be interface already
		}
		if expr.Name == "*" {
			// Pointer dereference: check the pointed-to type
			if expr.X != nil && expr.X.Kind == NIdent {
				if ct, ok := c.localConcreteTypes[expr.X.Name]; ok {
					// ct is like "main.*int" — extract the pointed-to type
					dotIdx := -1
					for i := 0; i < len(ct); i++ {
						if ct[i] == '.' {
							dotIdx = i
						}
					}
					if dotIdx >= 0 {
						rest := ct[dotIdx+1 : len(ct)]
						if len(rest) > 1 && rest[0] == '*' {
							inner := rest[1:len(rest)]
							if inner == "string" {
								return 2
							}
							if inner == "int" || inner == "byte" || inner == "bool" || inner == "int32" || inner == "int64" || inner == "uintptr" {
								return 1
							}
						}
					}
				}
			}
			return 1 // default: deref of pointer to scalar
		}
		if expr.Name == "!" || expr.Name == "-" || expr.Name == "^" {
			return 1
		}
	case NSelectorExpr:
		// Struct field access — check if it's a string field
		if c.isStringTypedExpr(expr) {
			return 2
		}
		return 1 // default to int for unknown fields
	case NIndexExpr:
		// Determine element type from the base expression
		if expr.X != nil && expr.X.Kind == NIdent {
			// String indexing returns byte (int)
			if c.localStringVars[expr.X.Name] {
				return 1
			}
			// Check if it's a known slice type
			if _, isSlice := c.localElemSizes[expr.X.Name]; isSlice {
				return 1 // scalar element from known slice
			}
			// Check concrete type — if it's a slice of strings, return 2
			if ct, ok := c.localConcreteTypes[expr.X.Name]; ok {
				if c.concreteTypeIsStringSlice(ct) {
					return 2
				}
				if c.concreteTypeIsStructSlice(ct) {
					return 0 // struct element — don't box
				}
			}
		}
		return 1 // default: assume scalar element
	case NSliceExpr:
		if c.isStringTypedExpr(expr.X) {
			return 2 // string slice produces string
		}
		return 0 // slice reslice produces slice
	}
	return 0
}

func (c *Compiler) concreteTypeIsStringSlice(ct string) bool {
	return ct == "[]string"
}

func (c *Compiler) concreteTypeIsStructSlice(ct string) bool {
	if len(ct) <= 2 {
		return false
	}
	if ct[0] != '[' || ct[1] != ']' {
		return false
	}
	elem := ct[2:len(ct)]
	if elem == "string" || elem == "int" || elem == "byte" || elem == "bool" || elem == "int32" || elem == "int64" || elem == "uintptr" {
		return false
	}
	return true
}

// resolveConcreteTypeID detects the concrete type from AST patterns and returns its type ID.
func (c *Compiler) resolveConcreteTypeID(expr *Node) int {
	if expr == nil {
		return 0
	}
	// Pattern: Errno(x) — type conversion call (unqualified)
	if expr.Kind == NCallExpr && expr.X != nil && expr.X.Kind == NIdent {
		sym, ok := c.curPkg.Symbols[expr.X.Name]
		if ok && sym.Kind == SymType {
			qtype := c.curPkg.Path + "." + expr.X.Name
			if id, ok := c.typeIDs[qtype]; ok {
				return id
			}
		}
	}
	// Pattern: os.Errno(x) — qualified type conversion call
	if expr.Kind == NCallExpr && expr.X != nil && expr.X.Kind == NSelectorExpr && expr.X.X != nil && expr.X.X.Kind == NIdent {
		pkgAlias := expr.X.X.Name
		typeName := expr.X.Name
		impPkg := c.resolvePackage(pkgAlias)
		if impPkg != nil {
			if sym, ok := impPkg.Symbols[typeName]; ok && sym.Kind == SymType {
				qtype := impPkg.Path + "." + typeName
				if id, ok := c.typeIDs[qtype]; ok {
					return id
				}
			}
		}
	}
	// Pattern: &fmtError{...} — address-of composite literal
	if expr.Kind == NUnaryExpr && expr.Name == "&" && expr.X != nil && expr.X.Kind == NCompositeLit {
		typeName := ""
		if expr.X.Type != nil {
			typeName = nodeTypeName(expr.X.Type)
		}
		qtype := c.curPkg.Path + ".*" + typeName
		if id, ok := c.typeIDs[qtype]; ok {
			return id
		}
	}
	return 0
}

// exprConcreteType returns the qualified concrete type name for an expression, or "".
func (c *Compiler) exprConcreteType(expr *Node) string {
	if expr == nil {
		return ""
	}
	// Composite literal: Greeting{...}
	if expr.Kind == NCompositeLit && expr.Type != nil {
		typeName := nodeTypeName(expr.Type)
		return c.qualifyTypeName(typeName, "")
	}
	// Address-of composite literal: &File{...}
	if expr.Kind == NUnaryExpr && expr.Name == "&" && expr.X != nil && expr.X.Kind == NCompositeLit {
		if expr.X.Type != nil {
			typeName := nodeTypeName(expr.X.Type)
			return c.qualifyTypeName("*"+typeName, "")
		}
	}
	// Address-of variable: &x where x has a known concrete type
	if expr.Kind == NUnaryExpr && expr.Name == "&" && expr.X != nil && expr.X.Kind == NIdent {
		if ct, ok := c.localConcreteTypes[expr.X.Name]; ok {
			// Strip package prefix, prepend *, re-qualify
			dotIdx := -1
			for i := 0; i < len(ct); i++ {
				if ct[i] == '.' {
					dotIdx = i
				}
			}
			if dotIdx >= 0 {
				return ct[0:dotIdx+1] + "*" + ct[dotIdx+1:len(ct)]
			}
			return "*" + ct
		}
	}
	// Address-of any expression: when inner type is unknown, default to *int
	// so that isPointerToStructDeref returns false (requiring LOAD on deref).
	// Struct composite literals and typed idents are already handled above.
	if expr.Kind == NUnaryExpr && expr.Name == "&" {
		return c.qualifyTypeName("*int", "")
	}
	// Function call: check return type
	if expr.Kind == NCallExpr {
		// append returns the same slice type as its first argument
		if expr.X != nil && expr.X.Kind == NIdent && expr.X.Name == "append" && len(expr.Nodes) > 0 {
			return c.exprConcreteType(expr.Nodes[0])
		}
		calleeName := c.resolveCallName(expr.X)
		if retTypes, ok := c.funcRetTypes[calleeName]; ok && len(retTypes) > 0 {
			// Extract package path from callee name for proper qualification
			// For pkg.Func calls, find the package containing this function
			calleePkg := ""
			if expr.X != nil && expr.X.Kind == NSelectorExpr && expr.X.X != nil && expr.X.X.Kind == NIdent {
				pkg := c.resolvePackage(expr.X.X.Name)
				if pkg != nil {
					calleePkg = pkg.Path
				}
			}
			return c.qualifyTypeName(retTypes[0], calleePkg)
		}
	}
	// Variable reference: check localConcreteTypes
	if expr.Kind == NIdent {
		if ct, ok := c.localConcreteTypes[expr.Name]; ok {
			return ct
		}
	}
	// Slice expression: e.g. args[1:], s[lo:hi] — type is same as target
	if expr.Kind == NSliceExpr && expr.X != nil {
		return c.exprConcreteType(expr.X)
	}
	// Index expression: e.g. nodes[i], slice[idx]
	if expr.Kind == NIndexExpr {
		return c.resolveExprType(expr)
	}
	// Selector expression: e.g. directive.X, node.Type
	if expr.Kind == NSelectorExpr {
		return c.resolveExprType(expr)
	}
	return ""
}

func (c *Compiler) compileIf(node *Node) {
	elseLabel := c.newLabel()
	endLabel := c.newLabel()

	// Compile init statement if present (e.g. if x, ok := m[k]; ok { ... })
	if len(node.Nodes) > 0 {
		c.compileStmt(node.Nodes[0])
	}

	c.compileExpr(node.X)
	c.emit(Inst{Op: OP_JMP_IF_NOT, Arg: elseLabel})

	branchDepth := c.stackDepth
	c.compileBlock(node.Body)
	thenDepth := c.stackDepth
	thenReturns := c.blockEndsWithReturn()
	c.emit(Inst{Op: OP_JMP, Arg: endLabel})

	c.stackDepth = branchDepth
	c.emitLabel(elseLabel)
	if node.Y != nil {
		if node.Y.Kind == NIf {
			c.compileStmt(node.Y)
		} else {
			c.compileBlock(node.Y)
		}
	}
	elseDepth := c.stackDepth
	elseReturns := c.blockEndsWithReturn()
	// Check balance if neither branch returns (returning branches don't merge)
	if !thenReturns && !elseReturns && thenDepth != elseDepth {
		// fmt.Fprintf(os.Stderr, "warning: unbalanced if branches in %s: then=%d else=%d (entry=%d)\n",
		// 	c.curFunc.Name, thenDepth, elseDepth, branchDepth)
	}
	// If one branch returns, use the other's depth at the merge point
	if thenReturns && !elseReturns {
		c.stackDepth = elseDepth
	} else if !thenReturns && elseReturns {
		c.stackDepth = thenDepth
	}
	c.emitLabel(endLabel)
}

func (c *Compiler) compileFor(node *Node) {
	savedDepth := c.stackDepth
	loopLabel := c.newLabel()
	continueLabel := c.newLabel()
	breakLabel := c.newLabel()

	c.breaks = append(c.breaks, breakLabel)
	c.continues = append(c.continues, continueLabel)

	if node.Name == "range" {
		c.compileForRange(node, loopLabel, continueLabel, breakLabel)
	} else if node.X != nil && node.X.Kind == NAssign {
		// 3-clause for
		c.pushScope()
		c.compileStmt(node.X)
		c.emitLabel(loopLabel)
		if node.Y != nil {
			c.compileExpr(node.Y)
			c.emit(Inst{Op: OP_JMP_IF_NOT, Arg: breakLabel})
		}
		if node.Body != nil {
			c.compileBlock(node.Body)
		}
		c.emitLabel(continueLabel)
		if node.Type != nil {
			c.compileStmt(node.Type)
		}
		c.emit(Inst{Op: OP_JMP, Arg: loopLabel})
		c.emitLabel(breakLabel)
		c.popScope()
	} else if node.Y != nil {
		// Condition-only for loop
		c.emitLabel(loopLabel)
		c.compileExpr(node.Y)
		c.emit(Inst{Op: OP_JMP_IF_NOT, Arg: breakLabel})
		if node.Body != nil {
			c.compileBlock(node.Body)
		}
		c.emitLabel(continueLabel)
		c.emit(Inst{Op: OP_JMP, Arg: loopLabel})
		c.emitLabel(breakLabel)
	} else {
		// Bare for loop (infinite)
		c.emitLabel(loopLabel)
		if node.Body != nil {
			c.compileBlock(node.Body)
		}
		c.emitLabel(continueLabel)
		c.emit(Inst{Op: OP_JMP, Arg: loopLabel})
		c.emitLabel(breakLabel)
	}

	c.breaks = c.breaks[0 : len(c.breaks)-1]
	c.continues = c.continues[0 : len(c.continues)-1]
	c.stackDepth = savedDepth // for loops should have net-zero effect
}

func (c *Compiler) compileForRange(node *Node, loopLabel int, continueLabel int, breakLabel int) {
	c.pushScope()

	isMap := c.isMapExpr(node.Type)

	// Compile the iterable and store it
	c.compileExpr(node.Type)
	iterIdx := c.addLocal("$iter")
	c.emit(Inst{Op: OP_LOCAL_SET, Arg: iterIdx})

	// Initialize index to 0
	idxIdx := c.addLocal("$idx")
	c.emit(Inst{Op: OP_CONST_I64, Val: 0})
	c.emit(Inst{Op: OP_LOCAL_SET, Arg: idxIdx})

	c.emitLabel(loopLabel)

	// Compare index < len(iterable)
	c.emit(Inst{Op: OP_LOCAL_GET, Arg: idxIdx})
	if isMap {
		c.emit(Inst{Op: OP_LOCAL_GET, Arg: iterIdx})
		c.emit(Inst{Op: OP_CALL, Name: "runtime.MapLen", Arg: 1})
	} else {
		c.emit(Inst{Op: OP_LOCAL_GET, Arg: iterIdx})
		c.emit(Inst{Op: OP_LEN})
	}
	c.emit(Inst{Op: OP_LT})
	c.emit(Inst{Op: OP_JMP_IF_NOT, Arg: breakLabel})

	// Bind loop variables
	if node.X != nil {
		keyIdx := c.addLocal(node.X.Name)
		if isMap {
			// For maps, key = MapEntryKey(hdr, idx)
			c.emit(Inst{Op: OP_LOCAL_GET, Arg: iterIdx})
			c.emit(Inst{Op: OP_LOCAL_GET, Arg: idxIdx})
			c.emit(Inst{Op: OP_CALL, Name: "runtime.MapEntryKey", Arg: 2})
			// Track string-typed key vars for interface boxing
			if c.mapExprKeyKind(node.Type) == 1 {
				c.localStringVars[node.X.Name] = true
			}
		} else {
			c.emit(Inst{Op: OP_LOCAL_GET, Arg: idxIdx})
		}
		c.emit(Inst{Op: OP_LOCAL_SET, Arg: keyIdx})
	}
	if node.Y != nil {
		valIdx := c.addLocal(node.Y.Name)
		if isMap {
			// For maps, value = MapEntryValue(hdr, idx)
			c.emit(Inst{Op: OP_LOCAL_GET, Arg: iterIdx})
			c.emit(Inst{Op: OP_LOCAL_GET, Arg: idxIdx})
			c.emit(Inst{Op: OP_CALL, Name: "runtime.MapEntryValue", Arg: 2})
		} else {
			elemSize := c.exprElemSize(node.Type)
			c.emit(Inst{Op: OP_LOCAL_GET, Arg: iterIdx})
			c.emit(Inst{Op: OP_LOCAL_GET, Arg: idxIdx})
			c.emit(Inst{Op: OP_INDEX_ADDR, Arg: elemSize})
			c.emit(Inst{Op: OP_LOAD, Arg: elemSize})
		}
		c.emit(Inst{Op: OP_LOCAL_SET, Arg: valIdx})
		// Track value type from collection element type for method resolution
		if isMap && node.Type != nil {
			valType := c.resolveMapValueType(node.Type)
			if valType != "" {
				c.localConcreteTypes[node.Y.Name] = c.qualifyTypeName(valType, "")
				if valType == "string" {
					c.localStringVars[node.Y.Name] = true
				}
			}
		}
		if !isMap && node.Type != nil {
			elemType := ""
			if node.Type.Kind == NIdent {
				collType := c.localConcreteTypes[node.Type.Name]
				if collType == "" {
					gqname := c.curPkg.Path + "." + node.Type.Name
					collType = c.globalConcreteTypes[gqname]
				}
				elemType = sliceElemType(collType)
			} else if node.Type.Kind == NSelectorExpr && node.Type.X != nil {
				// Range over struct field: e.g. pkg.Files or fn.Type.Nodes
				recvType := c.resolveExprType(node.Type.X)
				if recvType != "" {
					elemType = c.resolveFieldSliceElemType(recvType, node.Type.Name)
				}
			} else if node.Type.Kind == NCallExpr {
				// Range over function call result: e.g. strings.Fields(s)
				calleeName := c.resolveCallName(node.Type.X)
				if retTypes, ok := c.funcRetTypes[calleeName]; ok && len(retTypes) > 0 {
					retType := c.qualifyTypeName(retTypes[0], "")
					elemType = sliceElemType(retType)
				}
			}
			if elemType != "" {
				c.localConcreteTypes[node.Y.Name] = elemType
				if elemType == "string" {
					c.localStringVars[node.Y.Name] = true
				}
			}
		}
	}

	if node.Body != nil {
		c.compileBlock(node.Body)
	}

	c.emitLabel(continueLabel)

	// Increment index
	c.emit(Inst{Op: OP_LOCAL_GET, Arg: idxIdx})
	c.emit(Inst{Op: OP_CONST_I64, Val: 1})
	c.emit(Inst{Op: OP_ADD})
	c.emit(Inst{Op: OP_LOCAL_SET, Arg: idxIdx})
	c.emit(Inst{Op: OP_JMP, Arg: loopLabel})

	c.emitLabel(breakLabel)
	c.popScope()
}

func (c *Compiler) compileSwitch(node *Node) {
	savedDepth := c.stackDepth
	endLabel := c.newLabel()

	// Compile tag if present
	hasTag := node.Y != nil
	if hasTag {
		c.compileExpr(node.Y)
	}
	caseCheckDepth := c.stackDepth // depth with tag on stack (if any)

	// Detect string switch: check tag and first case value
	isStringSwitch := false
	if hasTag {
		if isStringExpr(node.Y) || c.isStringTypedExpr(node.Y) {
			isStringSwitch = true
		}
		if !isStringSwitch {
			for _, cas := range node.Nodes {
				if cas.Name != "default" && cas.X != nil {
					if isStringExpr(cas.X) || c.isStringTypedExpr(cas.X) {
						isStringSwitch = true
					}
					break
				}
			}
		}
	}

	for _, cas := range node.Nodes {
		bodyLabel := c.newLabel()
		nextLabel := c.newLabel()

		if cas.Name == "default" {
			c.emitLabel(bodyLabel)
			if hasTag {
				c.emit(Inst{Op: OP_DROP})
			}
			if cas.Body != nil {
				c.compileBlock(cas.Body)
			}
			c.emit(Inst{Op: OP_JMP, Arg: endLabel})
			c.stackDepth = caseCheckDepth // reset for next case
		} else {
			// Collect all case values: first in cas.X, rest in cas.Nodes
			var caseExprs []*Node
			caseExprs = append(caseExprs, cas.X)
			for _, extra := range cas.Nodes {
				caseExprs = append(caseExprs, extra)
			}

			if hasTag {
				// Check each case value with OR logic
				// DUP/expr/EQ/JMP_IF is net-zero on the fallthrough path
				for _, expr := range caseExprs {
					c.emit(Inst{Op: OP_DUP})
					c.compileExpr(expr)
					if isStringSwitch {
						c.emit(Inst{Op: OP_CALL, Name: "runtime.StringEqual", Arg: 2})
					} else {
						c.emit(Inst{Op: OP_EQ})
					}
					c.emit(Inst{Op: OP_JMP_IF, Arg: bodyLabel})
				}
			} else {
				// No tag — each case expr is a bool condition, OR them
				for _, expr := range caseExprs {
					c.compileExpr(expr)
					c.emit(Inst{Op: OP_JMP_IF, Arg: bodyLabel})
				}
			}
			c.emit(Inst{Op: OP_JMP, Arg: nextLabel})

			// Body is reached from JMP_IF; depth = caseCheckDepth
			c.stackDepth = caseCheckDepth
			c.emitLabel(bodyLabel)
			if hasTag {
				c.emit(Inst{Op: OP_DROP})
			}
			if cas.Body != nil {
				c.compileBlock(cas.Body)
			}
			c.emit(Inst{Op: OP_JMP, Arg: endLabel})
			// Reset depth for next case's check path
			c.stackDepth = caseCheckDepth
			c.emitLabel(nextLabel)
		}
	}

	if hasTag {
		c.emit(Inst{Op: OP_DROP})
	}
	c.emitLabel(endLabel)
	c.stackDepth = savedDepth // switch should have net-zero effect
}

func (c *Compiler) compileInc(node *Node) {
	c.compileLValueGet(node.X)
	c.emit(Inst{Op: OP_CONST_I64, Val: 1})
	c.emit(Inst{Op: OP_ADD})
	c.compileLValueSet(node.X)
}

func (c *Compiler) compileBranch(node *Node) {
	if node.Name == "break" && len(c.breaks) > 0 {
		c.emit(Inst{Op: OP_JMP, Arg: c.breaks[len(c.breaks)-1]})
	} else if node.Name == "continue" && len(c.continues) > 0 {
		c.emit(Inst{Op: OP_JMP, Arg: c.continues[len(c.continues)-1]})
	}
}

// === Expression compilation ===

func (c *Compiler) compileExpr(node *Node) {
	if node == nil {
		return
	}
	switch node.Kind {
	case NIntLit:
		c.compileIntLit(node)
	case NStringLit:
		c.emit(Inst{Op: OP_CONST_STR, Name: node.Name})
	case NRuneLit:
		c.compileRuneLit(node)
	case NBasicLit:
		c.compileBasicLit(node)
	case NIdent:
		c.compileIdent(node)
	case NBinaryExpr:
		c.compileBinaryExpr(node)
	case NUnaryExpr:
		c.compileUnaryExpr(node)
	case NCallExpr:
		c.compileCallExpr(node)
	case NSelectorExpr:
		c.compileSelectorExpr(node)
	case NIndexExpr:
		c.compileIndexExpr(node)
	case NSliceExpr:
		c.compileSliceExpr(node)
	case NCompositeLit:
		c.compileCompositeLit(node)
	default:
		panic("ICE: unhandled expression kind in compileExpr")
	}
}

func (c *Compiler) compileIntLit(node *Node) {
	val := parseIntLiteral(node.Name)
	c.emit(Inst{Op: OP_CONST_I64, Val: val})
}

func (c *Compiler) compileRuneLit(node *Node) {
	val := parseRuneLiteral(node.Name)
	c.emit(Inst{Op: OP_CONST_I64, Val: int64(val)})
}

func (c *Compiler) compileBasicLit(node *Node) {
	if node.Name == "true" {
		c.emit(Inst{Op: OP_CONST_BOOL, Arg: 1})
	} else if node.Name == "false" {
		c.emit(Inst{Op: OP_CONST_BOOL, Arg: 0})
	} else if node.Name == "nil" {
		c.emit(Inst{Op: OP_CONST_NIL})
	} else if node.Name == "iota" {
		// Iota is resolved at const-eval time; emit 0 as placeholder
		c.emit(Inst{Op: OP_CONST_I64, Val: 0})
	}
}

func (c *Compiler) compileIdent(node *Node) {
	idx, ok := c.lookupLocal(node.Name)
	if ok {
		w := 0
		if idx < len(c.curFunc.Locals) {
			w = c.curFunc.Locals[idx].Width
		}
		c.emit(Inst{Op: OP_LOCAL_GET, Arg: idx, Width: w})
		return
	}
	gidx, gok := c.lookupGlobal(node.Name)
	if gok {
		c.emit(Inst{Op: OP_GLOBAL_GET, Arg: gidx})
		return
	}
	// Check if it's a precomputed constant
	qname2 := c.curPkg.Path + "." + node.Name
	if sval, ok2 := c.constStringValues[qname2]; ok2 {
		c.emit(Inst{Op: OP_CONST_STR, Name: sval})
		return
	}
	if val, ok2 := c.constValues[qname2]; ok2 {
		c.emit(Inst{Op: OP_CONST_I64, Val: val})
		return
	}
	// Check if it's a constant in the current package
	sym, symOk := c.curPkg.Symbols[node.Name]
	if symOk && sym.Kind == SymConst {
		if c.isConstStringExpr(sym.Node.X) {
			c.emit(Inst{Op: OP_CONST_STR, Name: c.evalConstString(sym.Node.X)})
			return
		}
		val := c.resolveConstValue(sym.Node)
		c.emit(Inst{Op: OP_CONST_I64, Val: val})
		return
	}
	// Could be a package name or unresolved — emit as global reference
	c.emit(Inst{Op: OP_GLOBAL_GET, Name: node.Name})
}

// resolveConstValue evaluates a constant declaration's value at compile time.
func (c *Compiler) resolveConstValue(node *Node) int64 {
	if node == nil {
		return 0
	}
	if node.X != nil {
		return c.evalConstExprWithIota(node.X, 0)
	}
	return 0
}

func (c *Compiler) lookupGlobal(name string) (int, bool) {
	// Try qualified name with current package
	qname := c.curPkg.Path + "." + name
	idx, ok := c.globals[qname]
	if ok {
		return idx, true
	}
	// Try bare name
	idx, ok = c.globals[name]
	return idx, ok
}

// isStringExpr returns true if the expression is known to produce a string value (AST-only check).
func isStringExpr(node *Node) bool {
	if node == nil {
		return false
	}
	switch node.Kind {
	case NStringLit:
		return true
	case NBinaryExpr:
		if node.Name == "+" {
			return isStringExpr(node.X) || isStringExpr(node.Y)
		}
	}
	return false
}

// isStringTypedExpr returns true if the expression produces a string value (uses compiler context).
func (c *Compiler) isStringTypedExpr(node *Node) bool {
	if node == nil {
		return false
	}
	switch node.Kind {
	case NStringLit:
		return true
	case NIdent:
		if c.localStringVars[node.Name] {
			return true
		}
		// Check string constants
		qname := c.curPkg.Path + "." + node.Name
		if _, ok := c.constStringValues[qname]; ok {
			return true
		}
		// Check global string vars
		if c.curPkg != nil {
			if sym, ok := c.curPkg.Symbols[node.Name]; ok && sym.Kind == SymVar && sym.Node != nil && sym.Node.Type != nil {
				return nodeTypeName(sym.Node.Type) == "string"
			}
		}
		return false
	case NBinaryExpr:
		if node.Name == "+" {
			return c.isStringTypedExpr(node.X) || c.isStringTypedExpr(node.Y)
		}
	case NCallExpr:
		// Check if function returns string
		if node.X != nil {
			calleeName := c.resolveCallName(node.X)
			if retTypes, ok := c.funcRetTypes[calleeName]; ok && len(retTypes) > 0 {
				return retTypes[0] == "string"
			}
			// string() conversion
			if node.X.Kind == NIdent && node.X.Name == "string" {
				return true
			}
		}
	case NSelectorExpr:
		// Struct field access — check if the field is a string type
		if node.X != nil {
			recvType := c.resolveExprType(node.X)
			if recvType != "" {
				fieldType := c.resolveFieldType(recvType, node.Name)
				return fieldType == "string"
			}
		}
	case NSliceExpr:
		// String slice expression s[lo:hi] — string if target is string
		return c.isStringTypedExpr(node.X)
	case NIndexExpr:
		// Index into []string → string
		if node.X != nil {
			ct := ""
			if node.X.Kind == NIdent {
				ct = c.localConcreteTypes[node.X.Name]
			} else {
				ct = c.resolveExprType(node.X)
			}
			if ct == "[]string" {
				return true
			}
		}
	}
	return false
}

// isExprByte returns true if the expression is known to produce a single byte value.
func (c *Compiler) isExprByte(node *Node) bool {
	if node == nil {
		return false
	}
	switch node.Kind {
	case NIndexExpr:
		// Index into a byte slice → single byte
		if node.X != nil && node.X.Kind == NIdent {
			if es, ok := c.localElemSizes[node.X.Name]; ok && es == 1 {
				return true
			}
		}
	case NIdent:
		if ct, ok := c.localConcreteTypes[node.Name]; ok && ct == "byte" {
			return true
		}
	case NCallExpr:
		if node.X != nil {
			calleeName := c.resolveCallName(node.X)
			if retTypes, ok := c.funcRetTypes[calleeName]; ok && len(retTypes) > 0 {
				return retTypes[0] == "byte"
			}
		}
	}
	return false
}

func (c *Compiler) compileBinaryExpr(node *Node) {
	// Short-circuit for && and ||
	if node.Name == "&&" {
		falseLabel := c.newLabel()
		endLabel := c.newLabel()
		c.compileExpr(node.X)
		c.emit(Inst{Op: OP_JMP_IF_NOT, Arg: falseLabel})
		// JMP_IF_NOT popped condition; now compile Y (pushes 1 value)
		savedDepth := c.stackDepth
		c.compileExpr(node.Y)
		c.emit(Inst{Op: OP_JMP, Arg: endLabel})
		// False branch starts at the same depth as after JMP_IF_NOT
		c.stackDepth = savedDepth
		c.emitLabel(falseLabel)
		c.emit(Inst{Op: OP_CONST_BOOL, Arg: 0})
		c.emitLabel(endLabel)
		return
	}
	if node.Name == "||" {
		trueLabel := c.newLabel()
		endLabel := c.newLabel()
		c.compileExpr(node.X)
		c.emit(Inst{Op: OP_JMP_IF, Arg: trueLabel})
		// JMP_IF popped condition; now compile Y (pushes 1 value)
		savedDepth := c.stackDepth
		c.compileExpr(node.Y)
		c.emit(Inst{Op: OP_JMP, Arg: endLabel})
		// True branch starts at the same depth as after JMP_IF
		c.stackDepth = savedDepth
		c.emitLabel(trueLabel)
		c.emit(Inst{Op: OP_CONST_BOOL, Arg: 1})
		c.emitLabel(endLabel)
		return
	}

	// String operations: concatenation and comparison
	isStr := isStringExpr(node.X) || isStringExpr(node.Y) || c.isStringTypedExpr(node.X) || c.isStringTypedExpr(node.Y)
	if isStr && node.Name == "+" {
		c.compileExpr(node.X)
		c.compileExpr(node.Y)
		c.emit(Inst{Op: OP_CALL, Name: "runtime.StringConcat", Arg: 2})
		return
	}
	if isStr && node.Name == "==" {
		c.compileExpr(node.X)
		c.compileExpr(node.Y)
		c.emit(Inst{Op: OP_CALL, Name: "runtime.StringEqual", Arg: 2})
		return
	}
	if isStr && node.Name == "!=" {
		c.compileExpr(node.X)
		c.compileExpr(node.Y)
		c.emit(Inst{Op: OP_CALL, Name: "runtime.StringEqual", Arg: 2})
		c.emit(Inst{Op: OP_NOT})
		return
	}

	c.compileExpr(node.X)
	c.compileExpr(node.Y)

	w := c.exprWidth(node)

	switch node.Name {
	case "+":
		c.emit(Inst{Op: OP_ADD, Width: w})
	case "-":
		c.emit(Inst{Op: OP_SUB, Width: w})
	case "*":
		c.emit(Inst{Op: OP_MUL, Width: w})
	case "/":
		c.emit(Inst{Op: OP_DIV, Width: w})
	case "%":
		c.emit(Inst{Op: OP_MOD, Width: w})
	case "&":
		c.emit(Inst{Op: OP_AND, Width: w})
	case "|":
		c.emit(Inst{Op: OP_OR, Width: w})
	case "^":
		c.emit(Inst{Op: OP_XOR, Width: w})
	case "<<":
		c.emit(Inst{Op: OP_SHL, Width: w})
	case ">>":
		c.emit(Inst{Op: OP_SHR, Width: w})
	case "==":
		c.emit(Inst{Op: OP_EQ, Width: w})
	case "!=":
		c.emit(Inst{Op: OP_NEQ, Width: w})
	case "<":
		c.emit(Inst{Op: OP_LT, Width: w})
	case ">":
		c.emit(Inst{Op: OP_GT, Width: w})
	case "<=":
		c.emit(Inst{Op: OP_LEQ, Width: w})
	case ">=":
		c.emit(Inst{Op: OP_GEQ, Width: w})
	default:
		panic("ICE: unhandled binary operator in compileBinaryExpr")
	}
}

func (c *Compiler) compileUnaryExpr(node *Node) {
	switch node.Name {
	case "!":
		c.compileExpr(node.X)
		c.emit(Inst{Op: OP_NOT})
	case "-":
		w := c.exprWidth(node.X)
		c.compileExpr(node.X)
		c.emit(Inst{Op: OP_NEG, Width: w})
	case "*":
		c.compileExpr(node.X)
		if !c.isPointerToStructDeref(node.X) {
			c.emit(Inst{Op: OP_LOAD, Arg: targetPtrSize})
		}
	case "&":
		c.compileAddrOf(node.X)
	case "^":
		w := c.exprWidth(node.X)
		c.compileExpr(node.X)
		c.emit(Inst{Op: OP_CONST_I64, Val: -1, Width: w})
		c.emit(Inst{Op: OP_XOR, Width: w})
		if w == 1 {
			c.emit(Inst{Op: OP_CONST_I64, Val: 0xFF})
			c.emit(Inst{Op: OP_AND})
		}
	default:
		panic("ICE: unhandled unary operator in compileUnaryExpr")
	}
}

// isPointerToStructDeref checks if a node represents a variable of pointer-to-struct type.
// In this compiler, struct values are heap-allocated pointers, so *ptr where ptr is *StructType
// should be a no-op (the value IS the pointer). For non-struct pointer types (*[]string, *int, etc.),
// a LOAD is needed to read the pointed-to value.
func (c *Compiler) isPointerToStructDeref(node *Node) bool {
	if node == nil || node.Kind != NIdent {
		return false
	}
	ct, ok := c.localConcreteTypes[node.Name]
	if !ok {
		// In later self-host stages we can miss concrete type metadata for
		// pointer locals; default to no-op deref to preserve handle semantics.
		return true
	}
	// ct is like "main.*Token" or "main.*[]string"
	// Find the last dot to split package path from type
	dotIdx := -1
	for i := 0; i < len(ct); i++ {
		if ct[i] == '.' {
			dotIdx = i
		}
	}
	if dotIdx < 0 {
		return false
	}
	pkgPath := ct[0:dotIdx]
	rest := ct[dotIdx+1 : len(ct)]
	// rest should start with '*' for a pointer type
	if len(rest) == 0 || rest[0] != '*' {
		return false
	}
	tName := rest[1:len(rest)]
	// If tName starts with '[' it's a slice/array, not a struct
	if len(tName) > 0 && tName[0] == '[' {
		return false
	}
	// Pointers to primitives and well-known scalar forms should still load.
	if tName == "int" || tName == "int32" || tName == "int64" ||
		tName == "uint" || tName == "uint32" || tName == "uint64" ||
		tName == "uintptr" || tName == "byte" || tName == "bool" || tName == "string" {
		return false
	}
	if strings.HasPrefix(tName, "map[") || strings.HasPrefix(tName, "func(") || strings.HasPrefix(tName, "*") {
		return false
	}
	// Look up the type in the package
	pkg, ok := c.mod.Packages[pkgPath]
	if !ok {
		// Missing package/type metadata in later self-host stages: prefer no-op
		// for named pointer types to preserve struct-handle semantics.
		return true
	}
	sym, ok := pkg.Symbols[tName]
	if !ok || sym.Kind != SymType || sym.Node == nil {
		return true
	}
	typeNode := sym.Node.Type
	if typeNode == nil {
		return true
	}
	return typeNode.Kind == NStructType
}

func (c *Compiler) compileAddrOf(node *Node) {
	if node == nil {
		return
	}
	switch node.Kind {
	case NIdent:
		idx, ok := c.lookupLocal(node.Name)
		if ok {
			c.emit(Inst{Op: OP_LOCAL_ADDR, Arg: idx})
		} else {
			gidx, gok := c.lookupGlobal(node.Name)
			if gok {
				c.emit(Inst{Op: OP_GLOBAL_ADDR, Arg: gidx})
			}
		}
	case NCompositeLit:
		c.compileCompositeLit(node)
		// The composite lit value is on the stack; in a real compiler
		// we'd allocate and store, then push the address
	default:
		c.compileExpr(node)
	}
}

// packVariadicSlice emits IR to pack variadic args into a slice.
// args is the list of argument nodes, firstArgIdx is the index of the first variadic arg,
// varCount is the number of variadic args, elemSz is the element size,
// and ifaceKey is the function name to check in funcVariadicIface.
func (c *Compiler) packVariadicSlice(args []*Node, firstArgIdx int, varCount int, elemSz int, ifaceKey string) {
	if varCount == 0 {
		c.emit(Inst{Op: OP_CONST_NIL})
		return
	}
	sliceHdrSize := 4 * targetPtrSize // 32 on amd64, 16 on i386
	allocSize := sliceHdrSize + varCount*elemSz
	c.emit(Inst{Op: OP_CONST_I64, Val: int64(allocSize)})
	c.emit(Inst{Op: OP_CALL, Name: "runtime.Alloc", Arg: 1})
	tmpIdx := c.addLocal("$varslice")
	c.emit(Inst{Op: OP_LOCAL_SET, Arg: tmpIdx})
	// header[0] = data_ptr (header + sliceHdrSize)
	c.emit(Inst{Op: OP_LOCAL_GET, Arg: tmpIdx})
	c.emit(Inst{Op: OP_CONST_I64, Val: int64(sliceHdrSize)})
	c.emit(Inst{Op: OP_ADD})
	c.emit(Inst{Op: OP_LOCAL_GET, Arg: tmpIdx})
	c.emit(Inst{Op: OP_STORE})
	// header[ptrSize] = len
	c.emit(Inst{Op: OP_CONST_I64, Val: int64(varCount)})
	c.emit(Inst{Op: OP_LOCAL_GET, Arg: tmpIdx})
	c.emit(Inst{Op: OP_CONST_I64, Val: int64(targetPtrSize)})
	c.emit(Inst{Op: OP_ADD})
	c.emit(Inst{Op: OP_STORE})
	// header[2*ptrSize] = cap
	c.emit(Inst{Op: OP_CONST_I64, Val: int64(varCount)})
	c.emit(Inst{Op: OP_LOCAL_GET, Arg: tmpIdx})
	c.emit(Inst{Op: OP_CONST_I64, Val: int64(2 * targetPtrSize)})
	c.emit(Inst{Op: OP_ADD})
	c.emit(Inst{Op: OP_STORE})
	// header[3*ptrSize] = elem_size
	c.emit(Inst{Op: OP_CONST_I64, Val: int64(elemSz)})
	c.emit(Inst{Op: OP_LOCAL_GET, Arg: tmpIdx})
	c.emit(Inst{Op: OP_CONST_I64, Val: int64(3 * targetPtrSize)})
	c.emit(Inst{Op: OP_ADD})
	c.emit(Inst{Op: OP_STORE})
	// Store each variadic arg into data region
	isIfaceVar := c.funcVariadicIface[ifaceKey]
	j := 0
	for j < varCount {
		arg := args[firstArgIdx+j]
		c.compileExpr(arg)
		if isIfaceVar {
			typeID := c.exprPrimitiveTypeID(arg)
			if typeID > 0 {
				c.emit(Inst{Op: OP_IFACE_BOX, Arg: typeID})
			}
		}
		c.emit(Inst{Op: OP_LOCAL_GET, Arg: tmpIdx})
		c.emit(Inst{Op: OP_CONST_I64, Val: int64(sliceHdrSize + j*elemSz)})
		c.emit(Inst{Op: OP_ADD})
		c.emit(Inst{Op: OP_STORE, Arg: elemSz})
		j++
	}
	c.emit(Inst{Op: OP_LOCAL_GET, Arg: tmpIdx})
}

func (c *Compiler) compileCallExpr(node *Node) {
	// Check for builtins
	if node.X != nil && node.X.Kind == NIdent {
		name := node.X.Name
		if name == "len" {
			if len(node.Nodes) > 0 && c.isMapExpr(node.Nodes[0]) {
				c.compileExpr(node.Nodes[0])
				c.emit(Inst{Op: OP_CALL, Name: "runtime.MapLen", Arg: 1})
				return
			}
			c.compileExpr(node.Nodes[0])
			c.emit(Inst{Op: OP_LEN})
			return
		}
		if name == "cap" {
			c.compileExpr(node.Nodes[0])
			c.emit(Inst{Op: OP_CAP})
			return
		}
		if name == "append" {
			c.compileAppend(node)
			return
		}
		if name == "copy" {
			c.compileCopy(node)
			return
		}
		if name == "delete" {
			if len(node.Nodes) >= 2 {
				c.compileExpr(node.Nodes[0])
				c.compileExpr(node.Nodes[1])
				c.emit(Inst{Op: OP_CALL, Name: "runtime.MapDelete", Arg: 2})
			}
			return
		}
		if name == "make" {
			c.compileMake(node)
			return
		}
		if name == "panic" {
			if len(node.Nodes) > 0 {
				c.compileExpr(node.Nodes[0])
			} else {
				c.emit(Inst{Op: OP_CONST_STR, Name: "panic"})
			}
			c.emit(Inst{Op: OP_PANIC})
			return
		}
		// Type conversions: int(), uintptr(), byte(), string(), int32()
		if name == "int" || name == "uintptr" || name == "uint" || name == "byte" || name == "string" || name == "int32" || name == "int64" || name == "uint16" || name == "uint32" || name == "uint64" {
			c.compileExpr(node.Nodes[0])
			if name == "string" && c.isExprByte(node.Nodes[0]) {
				c.emit(Inst{Op: OP_CALL, Name: "runtime.ByteToString", Arg: 1})
			} else {
				c.emit(Inst{Op: OP_CONVERT, Name: name})
			}
			return
		}
	}

	// Check for []byte() conversion
	if node.X != nil && node.X.Kind == NSliceType {
		c.compileExpr(node.Nodes[0])
		c.emit(Inst{Op: OP_CONVERT, Name: "[]byte"})
		return
	}

	// Check for user-defined type conversions (e.g. Errno(val))
	if node.X != nil && node.X.Kind == NIdent && len(node.Nodes) == 1 {
		sym, ok := c.curPkg.Symbols[node.X.Name]
		if ok && sym.Kind == SymType {
			c.compileExpr(node.Nodes[0])
			c.emit(Inst{Op: OP_CONVERT, Name: node.X.Name})
			return
		}
	}

	// Check for qualified type conversions (e.g. os.Errno(val))
	if node.X != nil && node.X.Kind == NSelectorExpr && node.X.X != nil && node.X.X.Kind == NIdent && len(node.Nodes) == 1 {
		pkgAlias := node.X.X.Name
		typeName := node.X.Name
		impPkg := c.resolvePackage(pkgAlias)
		if impPkg != nil {
			if sym, ok := impPkg.Symbols[typeName]; ok && sym.Kind == SymType {
				c.compileExpr(node.Nodes[0])
				c.emit(Inst{Op: OP_CONVERT, Name: typeName})
				return
			}
		}
	}

	// Check for interface method call: e.g. err.Error()
	if node.X != nil && node.X.Kind == NSelectorExpr && node.X.X != nil && node.X.X.Kind == NIdent {
		recvName := node.X.X.Name
		methodName := node.X.Name
		if ifaceType, ok := c.localTypes[recvName]; ok {
			if methods, ok := c.ifaceMethods[ifaceType]; ok {
				isIfaceMethod := false
				for _, m := range methods {
					if m == methodName {
						isIfaceMethod = true
					}
				}
				if isIfaceMethod {
					// Push receiver (interface pointer) then args
					c.compileExpr(node.X.X)
					for _, arg := range node.Nodes {
						c.compileExpr(arg)
					}
					c.emit(Inst{Op: OP_IFACE_CALL, Name: ifaceType + "." + methodName, Arg: len(node.Nodes)})
					return
				}
			}
		}
	}

	// Check for concrete method call: e.g. entry.Name()
	if node.X != nil && node.X.Kind == NSelectorExpr && node.X.X != nil && node.X.X.Kind == NIdent {
		recvName := node.X.X.Name
		methodName := node.X.Name
		concreteType, ok := c.localConcreteTypes[recvName]
		if !ok {
			// Try global concrete types
			gqname := c.curPkg.Path + "." + recvName
			concreteType, ok = c.globalConcreteTypes[gqname]
		}
		if ok {
			candidate := concreteType + "." + methodName
			resolvedName, ok := c.methodTable[candidate]
			if !ok {
				ptrCandidate := pointerMethodTypeName(concreteType) + "." + methodName
				resolvedName, ok = c.methodTable[ptrCandidate]
			}
			if ok {
				// Check if this method is variadic
				fixedCount, isVariadic := c.funcVariadic[resolvedName]
				isSpread := node.Name == "spread"
				if isVariadic && !isSpread {
					// Push receiver (counts as first fixed arg)
					c.compileExpr(node.X.X)
					// Compile other fixed args (fixedCount includes receiver)
					i := 0
					for i < fixedCount-1 && i < len(node.Nodes) {
						c.compileExpr(node.Nodes[i])
						i++
					}
					// Package variadic args into a slice
					variadicCount := len(node.Nodes) - (fixedCount - 1)
					if variadicCount < 0 {
						variadicCount = 0
					}
					mVarElemSz := targetPtrSize
					if mesz, ok := c.funcVariadicElem[resolvedName]; ok {
						mVarElemSz = mesz
					}
					c.packVariadicSlice(node.Nodes, fixedCount-1, variadicCount, mVarElemSz, resolvedName)
					c.emit(Inst{Op: OP_CALL, Name: resolvedName, Arg: fixedCount + 1})
				} else {
					// Non-variadic or spread: push receiver first, then args
					c.compileExpr(node.X.X)
					for _, arg := range node.Nodes {
						c.compileExpr(arg)
					}
					c.emit(Inst{Op: OP_CALL, Name: resolvedName, Arg: len(node.Nodes) + 1})
				}
				return
			}
		}
	}

	// Check for chained selector method call: e.g. node.Kind.String()
	// node.X = SelectorExpr{Name: "String", X: SelectorExpr{Name: "Kind", X: Ident{Name: "node"}}}
	if node.X != nil && node.X.Kind == NSelectorExpr && node.X.X != nil && node.X.X.Kind == NSelectorExpr {
		methodName := node.X.Name
		fieldName := node.X.X.Name
		// Walk X chain to find the root ident
		root := node.X.X.X
		for root != nil && root.Kind == NSelectorExpr {
			root = root.X
		}
		if root != nil && root.Kind == NIdent {
			if concreteType, ok := c.localConcreteTypes[root.Name]; ok {
				fieldType := c.resolveFieldType(concreteType, fieldName)
				if fieldType != "" {
					candidate := fieldType + "." + methodName
					resolvedName, ok := c.methodTable[candidate]
					if !ok {
						ptrCandidate := pointerMethodTypeName(fieldType) + "." + methodName
						resolvedName, ok = c.methodTable[ptrCandidate]
					}
					if ok {
						// Push receiver (the field access) first, then args
						c.compileExpr(node.X.X)
						for _, arg := range node.Nodes {
							c.compileExpr(arg)
						}
						c.emit(Inst{Op: OP_CALL, Name: resolvedName, Arg: len(node.Nodes) + 1})
						return
					}
				}
			}
		}
	}

	// Determine the function to call
	callName := c.resolveCallName(node.X)

	// Check if this is a variadic function call
	fixedCount, isVariadic := c.funcVariadic[callName]
	isSpread := node.Name == "spread"

	if isVariadic && !isSpread {
		// Compile fixed args normally
		i := 0
		for i < fixedCount && i < len(node.Nodes) {
			c.compileExpr(node.Nodes[i])
			i++
		}

		// Package variadic args into an inline slice
		variadicCount := len(node.Nodes) - fixedCount
		if variadicCount < 0 {
			variadicCount = 0
		}

		varElemSz := targetPtrSize
		if esz, ok := c.funcVariadicElem[callName]; ok {
			varElemSz = esz
		}

		c.packVariadicSlice(node.Nodes, fixedCount, variadicCount, varElemSz, callName)

		// Call with fixedCount + 1 args (fixed params + one slice)
		c.emit(Inst{Op: OP_CALL, Name: callName, Arg: fixedCount + 1})
	} else {
		// Non-variadic call, or spread call — compile all args normally
		for _, arg := range node.Nodes {
			c.compileExpr(arg)
		}

		argCount := len(node.Nodes)

		// Pad missing args with nil
		if expected, ok := c.funcParams[callName]; ok && argCount < expected {
			for argCount < expected {
				c.emit(Inst{Op: OP_CONST_NIL})
				argCount++
			}
		}

		c.emit(Inst{Op: OP_CALL, Name: callName, Arg: argCount})
	}
}

// qualifyTypeName qualifies a type name with a package path if not already qualified.
func (c *Compiler) qualifyTypeName(typeName string, pkgPath string) string {
	if typeName == "" || typeName == "string" || typeName == "int" || typeName == "bool" || typeName == "byte" || typeName == "error" || typeName == "interface{}" {
		return typeName
	}
	// Map types: recursively qualify key and value types
	if len(typeName) >= 4 && typeName[0] == 'm' && typeName[1] == 'a' && typeName[2] == 'p' && typeName[3] == '[' {
		depth := 1
		i := 4
		for i < len(typeName) && depth > 0 {
			if typeName[i] == '[' {
				depth = depth + 1
			}
			if typeName[i] == ']' {
				depth = depth - 1
			}
			i = i + 1
		}
		keyPart := typeName[4 : i-1]
		valPart := typeName[i:len(typeName)]
		return "map[" + c.qualifyTypeName(keyPart, pkgPath) + "]" + c.qualifyTypeName(valPart, pkgPath)
	}
	// Strip slice prefix to get element type
	if len(typeName) > 2 && typeName[0] == '[' && typeName[1] == ']' {
		return "[]" + c.qualifyTypeName(typeName[2:len(typeName)], pkgPath)
	}
	// Pointer prefix: keep * after package name to match method table format (e.g. "main.*Parser")
	if len(typeName) > 1 && typeName[0] == '*' {
		inner := typeName[1:len(typeName)]
		pkg := pkgPath
		if pkg == "" {
			pkg = c.curPkg.Path
		}
		// Check if inner is already qualified (e.g. "*os.File" → "os.*File")
		j := 0
		for j < len(inner) {
			if inner[j] == '.' {
				pkgAlias := inner[0:j]
				typePart := inner[j+1 : len(inner)]
				// Resolve package alias to full path
				impPkg := c.resolvePackage(pkgAlias)
				if impPkg != nil {
					return impPkg.Path + ".*" + typePart
				}
				return pkgAlias + ".*" + typePart
			}
			j++
		}
		return pkg + ".*" + inner
	}
	// Already qualified (contains '.') — but might be an import alias, resolve it
	i := 0
	for i < len(typeName) {
		if typeName[i] == '.' {
			pkgAlias := typeName[0:i]
			rest := typeName[i+1 : len(typeName)]
			impPkg := c.resolvePackage(pkgAlias)
			if impPkg != nil {
				return impPkg.Path + "." + rest
			}
			return typeName
		}
		i++
	}
	// Qualify with package
	if pkgPath != "" {
		return pkgPath + "." + typeName
	}
	return c.curPkg.Path + "." + typeName
}

// pointerMethodTypeName converts a qualified value receiver type name like
// "strings.Builder" into the method-table pointer form "strings.*Builder".
func pointerMethodTypeName(typeName string) string {
	if typeName == "" {
		return ""
	}
	if strings.Contains(typeName, ".*") {
		return typeName
	}
	if strings.HasPrefix(typeName, "*") {
		return typeName
	}
	dot := -1
	i := len(typeName) - 1
	for i >= 0 {
		if typeName[i] == '.' {
			dot = i
			break
		}
		i = i - 1
	}
	if dot < 0 {
		return "*" + typeName
	}
	if dot+1 < len(typeName) && typeName[dot+1] == '*' {
		return typeName
	}
	return typeName[0:dot+1] + "*" + typeName[dot+1:len(typeName)]
}

// findUniqueMethodByName finds a method implementation by bare method name.
// Returns (resolvedName, true) only when exactly one method matches.
func (c *Compiler) findUniqueMethodByName(methodName string) (string, bool) {
	found := ""
	for key, resolved := range c.methodTable {
		if strings.HasSuffix(key, "."+methodName) {
			if found != "" && found != resolved {
				return "", false
			}
			found = resolved
		}
	}
	if found == "" {
		return "", false
	}
	return found, true
}

// sliceElemType extracts the element type from a slice type string like "[]os.DirEntry".
func sliceElemType(typeName string) string {
	if len(typeName) > 2 && typeName[0] == '[' && typeName[1] == ']' {
		return typeName[2:len(typeName)]
	}
	return ""
}

// parseMapTypeName parses a qualified map type like "map[string]int" and
// returns key and value type names.
func parseMapTypeName(typeName string) (string, string, bool) {
	if len(typeName) < 5 || typeName[0] != 'm' || typeName[1] != 'a' || typeName[2] != 'p' || typeName[3] != '[' {
		return "", "", false
	}
	depth := 1
	i := 4
	for i < len(typeName) && depth > 0 {
		if typeName[i] == '[' {
			depth = depth + 1
		} else if typeName[i] == ']' {
			depth = depth - 1
		}
		i = i + 1
	}
	if depth != 0 || i <= 5 || i > len(typeName) {
		return "", "", false
	}
	keyType := typeName[4 : i-1]
	valType := typeName[i:len(typeName)]
	if len(keyType) == 0 || len(valType) == 0 {
		return "", "", false
	}
	return keyType, valType, true
}

func (c *Compiler) resolveCallName(node *Node) string {
	if node == nil {
		return ""
	}
	if node.Kind == NIdent {
		// Check if it's a local variable (e.g. function literal)
		_, isLocal := c.lookupLocal(node.Name)
		if isLocal {
			return node.Name
		}
		// Check if it's a function or type in current package
		sym, ok := c.curPkg.Symbols[node.Name]
		if ok {
			if sym.Kind != SymFunc && sym.Kind != SymType {
				c.errorf("%s: %s is not callable (not a function or type)", c.curFunc.Name, node.Name)
			}
			return c.curPkg.Path + "." + node.Name
		}
		if !isBuiltinName(node.Name) {
			c.errorf("%s: undefined: %s (used as function)", c.curFunc.Name, node.Name)
		}
		return node.Name
	}
	if node.Kind == NSelectorExpr && node.X != nil && node.X.Kind == NIdent {
		// pkg.Func or receiver.Method
		pkg := c.resolvePackage(node.X.Name)
		if pkg != nil {
			sym, hasSym := pkg.Symbols[node.Name]
			if !hasSym {
				c.errorf("%s: %s.%s not found in package %s", c.curFunc.Name, node.X.Name, node.Name, pkg.Path)
			} else if sym.Kind != SymFunc && sym.Kind != SymType {
				c.errorf("%s: %s.%s is not callable", c.curFunc.Name, node.X.Name, node.Name)
			}
			return pkg.Path + "." + node.Name
		}
		// Could be a method call — try to resolve using concrete type
		concreteType := ""
		if ct, ok := c.localConcreteTypes[node.X.Name]; ok {
			concreteType = ct
		} else {
			gqname := c.curPkg.Path + "." + node.X.Name
			if ct, ok := c.globalConcreteTypes[gqname]; ok {
				concreteType = ct
			}
		}
		if concreteType != "" {
			candidate := concreteType + "." + node.Name
			if resolved, ok := c.methodTable[candidate]; ok {
				return resolved
			}
			ptrCandidate := pointerMethodTypeName(concreteType) + "." + node.Name
			if resolved, ok := c.methodTable[ptrCandidate]; ok {
				return resolved
			}
		}
		if resolved, ok := c.findUniqueMethodByName(node.Name); ok {
			return resolved
		}
		return node.X.Name + "." + node.Name
	}
	// Handle []byte, []int, etc. type conversions
	if node.Kind == NSliceType {
		return "[]" + nodeTypeName(node.X)
	}
	// Handle chained selector: e.g. node.Kind.String() → receiver is SelectorExpr
	if node.Kind == NSelectorExpr && node.X != nil && node.X.Kind == NSelectorExpr {
		// Try to resolve the field type and look up the method
		methodName := node.Name
		fieldName := node.X.Name
		// Walk X chain to find the root ident
		root := node.X.X
		for root != nil && root.Kind == NSelectorExpr {
			root = root.X
		}
		if root != nil && root.Kind == NIdent {
			if concreteType, ok := c.localConcreteTypes[root.Name]; ok {
				fieldType := c.resolveFieldType(concreteType, fieldName)
				if fieldType != "" {
					candidate := fieldType + "." + methodName
					if resolved, ok := c.methodTable[candidate]; ok {
						return resolved
					}
					ptrCandidate := pointerMethodTypeName(fieldType) + "." + methodName
					if resolved, ok := c.methodTable[ptrCandidate]; ok {
						return resolved
					}
				}
			}
		}
		if resolved, ok := c.findUniqueMethodByName(methodName); ok {
			return resolved
		}
	}
	return "unknown"
}

func (c *Compiler) compileAppend(node *Node) {
	if len(node.Nodes) < 2 {
		return
	}
	// Determine element size from the slice argument
	elemSize := targetPtrSize // default: pointer-sized elements
	if node.Nodes[0].Kind == NIdent {
		name := node.Nodes[0].Name
		if es, ok := c.localElemSizes[name]; ok {
			elemSize = es
		} else if ct, ok := c.localConcreteTypes[name]; ok {
			if ct == "[]byte" {
				elemSize = 1
			}
		}
	} else {
		// For selector expressions, index expressions, etc., use exprElemSize
		elemSize = c.exprElemSize(node.Nodes[0])
	}
	// Compile slice arg
	c.compileExpr(node.Nodes[0])
	if node.Name == "spread" {
		// append(dst, src...) — append all elements from src slice
		c.compileExpr(node.Nodes[1])
		c.emit(Inst{Op: OP_CALL, Name: "runtime.SliceAppendSlice", Arg: 2})
	} else {
		// Append one element at a time, chaining the result
		i := 1
		for i < len(node.Nodes) {
			c.compileExpr(node.Nodes[i])
			c.emit(Inst{Op: OP_CONST_I64, Val: int64(elemSize)})
			c.emit(Inst{Op: OP_CALL, Name: "runtime.SliceAppend", Arg: 3})
			i++
		}
	}
}

func (c *Compiler) compileCopy(node *Node) {
	if len(node.Nodes) < 2 {
		c.emit(Inst{Op: OP_CONST_I64, Val: 0})
		return
	}
	// copy(dst, src) → runtime.SliceCopy(dst, src)
	c.compileExpr(node.Nodes[0])
	c.compileExpr(node.Nodes[1])
	c.emit(Inst{Op: OP_CALL, Name: "runtime.SliceCopy", Arg: 2})
}

func (c *Compiler) compileMake(node *Node) {
	// make([]T, len) or make([]T, len, cap) or make(map[K]V)
	if node.Nodes[0].Kind == NMapType {
		// Map creation: make(map[K]V)
		keyKind := c.mapKeyKind(node.Nodes[0].X)
		c.emit(Inst{Op: OP_CONST_I64, Val: int64(keyKind)})
		c.emit(Inst{Op: OP_CALL, Name: "runtime.MapMake", Arg: 1})
		return
	}
	// Slice creation: make([]T, len) or make([]T, len, cap)
	if len(node.Nodes) >= 2 {
		c.compileExpr(node.Nodes[1]) // length
	} else {
		c.emit(Inst{Op: OP_CONST_I64, Val: 0})
	}
	elemSize := c.sliceElemSize(node.Nodes[0])
	if len(node.Nodes) >= 3 {
		c.compileExpr(node.Nodes[2]) // capacity
		c.emit(Inst{Op: OP_CONST_I64, Val: int64(elemSize)})
		c.emit(Inst{Op: OP_CALL, Name: "runtime.SliceMakeCap", Arg: 3})
	} else {
		c.emit(Inst{Op: OP_CONST_I64, Val: int64(elemSize)})
		c.emit(Inst{Op: OP_CALL, Name: "runtime.SliceMake", Arg: 2})
	}
}

// mapKeyKind returns the key kind for a map key type node: 0=int/pointer, 1=string.
func (c *Compiler) mapKeyKind(keyTypeNode *Node) int {
	if keyTypeNode != nil && keyTypeNode.Kind == NIdent && keyTypeNode.Name == "string" {
		return 1
	}
	return 0
}

// exprReturnCount returns how many values an expression pushes onto the operand stack.
func (c *Compiler) exprReturnCount(node *Node) int {
	if node == nil {
		return 1
	}
	if node.Kind == NCallExpr {
		// Builtins that return nothing
		if node.X != nil && node.X.Kind == NIdent {
			bname := node.X.Name
			if bname == "delete" || bname == "close" {
				return 0
			}
		}
		// Look up the callee's return count (node.X is the callee)
		name := c.resolveCallName(node.X)
		if retCount, ok := c.funcRets[name]; ok {
			return retCount
		}
		// Unknown function — assume 1 return value
		return 1
	}
	// All other expressions produce 1 value
	return 1
}

// exprElemSize determines the element size for indexing an expression.
// Returns 1 for strings and []byte, 8 for all other slice types.
func (c *Compiler) exprElemSize(node *Node) int {
	if node == nil {
		return 1
	}
	switch node.Kind {
	case NIdent:
		if es, ok := c.localElemSizes[node.Name]; ok {
			return es
		}
		// Check globals with qualified name
		qname := c.curPkg.Path + "." + node.Name
		if es, ok := c.globalElemSizes[qname]; ok {
			return es
		}
		// Check concrete type for slice elem size
		if ct, ok := c.localConcreteTypes[node.Name]; ok {
			if len(ct) > 2 && ct[0] == '[' && ct[1] == ']' {
				return c.typeElemSize(ct[2:len(ct)])
			}
		}
		// Not a known slice variable — assume string indexing (elem size 1)
		return 1
	case NCallExpr:
		// Function call: resolve return type and determine elem size
		calleeName := c.resolveCallName(node.X)
		if retTypes, ok := c.funcRetTypes[calleeName]; ok && len(retTypes) > 0 {
			retType := c.qualifyTypeName(retTypes[0], "")
			if len(retType) > 2 && retType[0] == '[' && retType[1] == ']' {
				return c.typeElemSize(retType[2:len(retType)])
			}
		}
		return 1
	case NSelectorExpr:
		// pkg.Name — look up qualified global
		if node.X != nil && node.X.Kind == NIdent {
			pkg := c.resolvePackage(node.X.Name)
			if pkg != nil {
				qname := pkg.Path + "." + node.Name
				if es, ok := c.globalElemSizes[qname]; ok {
					return es
				}
			}
		}
		// Field access — resolve field type (handles chained selectors)
		if node.X != nil {
			recvType := c.resolveExprType(node.X)
			if recvType != "" {
				es := c.resolveFieldElemSize(recvType, node.Name)
				if es > 0 {
					return es
				}
			}
		}
		return targetPtrSize
	}
	return 1
}

// sliceElemSize returns the element size for a slice type node.
func (c *Compiler) sliceElemSize(typeNode *Node) int {
	if typeNode == nil {
		return targetPtrSize
	}
	if typeNode.Kind == NSliceType && typeNode.X != nil {
		return c.typeElemSize(nodeTypeName(typeNode.X))
	}
	return targetPtrSize
}

func (c *Compiler) compileSelectorExpr(node *Node) {
	if node.X != nil && node.X.Kind == NIdent {
		// Check if it's a package-qualified access
		pkg := c.resolvePackage(node.X.Name)
		if pkg != nil {
			_, hasSym := pkg.Symbols[node.Name]
			if !hasSym {
				c.errorf("%s: %s.%s not found in package %s", c.curFunc.Name, node.X.Name, node.Name, pkg.Path)
			}
			qname := pkg.Path + "." + node.Name
			// Check if it's a precomputed constant
			if val, ok := c.constValues[qname]; ok {
				c.emit(Inst{Op: OP_CONST_I64, Val: val})
				return
			}
			// Check if it's a constant in the target package
			if sym, ok := pkg.Symbols[node.Name]; ok && sym.Kind == SymConst {
				val := c.resolveConstValue(sym.Node)
				c.emit(Inst{Op: OP_CONST_I64, Val: val})
				return
			}
			gidx, gok := c.globals[qname]
			if gok {
				c.emit(Inst{Op: OP_GLOBAL_GET, Arg: gidx})
				return
			}
			c.emit(Inst{Op: OP_GLOBAL_GET, Name: qname})
			return
		}
	}
	// Field access — resolve byte offset from concrete type
	offset := 0
	recvType := c.resolveExprType(node.X)
	if recvType != "" {
		off := c.resolveFieldOffset(recvType, node.Name)
		if off >= 0 {
			offset = off
		}
	}
	c.compileExpr(node.X)
	c.emit(Inst{Op: OP_OFFSET, Arg: offset})
	c.emit(Inst{Op: OP_LOAD, Arg: targetPtrSize})
}

func (c *Compiler) compileIndexExpr(node *Node) {
	// Check for map index read: m[key]
	if c.isMapExpr(node.X) {
		c.compileExpr(node.X)
		c.compileExpr(node.Y)
		c.emit(Inst{Op: OP_CALL, Name: "runtime.MapGet", Arg: 2})
		// MapGet returns (value, ok) — drop ok for single-value context
		// (multi-value context is handled in compileAssign)
		c.emit(Inst{Op: OP_DROP})
		return
	}
	elemSize := c.exprElemSize(node.X)
	c.compileExpr(node.X)
	c.compileExpr(node.Y)
	c.emit(Inst{Op: OP_INDEX_ADDR, Arg: elemSize})
	c.emit(Inst{Op: OP_LOAD, Arg: elemSize})
}

// mapExprKeyKind returns the key kind of a map expression (0=int, 1=string, -1=not a map).
func (c *Compiler) mapExprKeyKind(node *Node) int {
	if node == nil {
		return -1
	}
	if node.Kind == NIdent {
		if kk, ok := c.localMapVars[node.Name]; ok {
			return kk
		}
		qname := c.curPkg.Path + "." + node.Name
		if kk, ok := c.globalMapVars[qname]; ok {
			return kk
		}
	}
	if node.Kind == NSelectorExpr && node.X != nil {
		if node.X.Kind == NIdent {
			pkg := c.resolvePackage(node.X.Name)
			if pkg != nil {
				qname := pkg.Path + "." + node.Name
				if kk, ok := c.globalMapVars[qname]; ok {
					return kk
				}
			}
		}
		// Check struct field map type (handles chained selectors)
		recvType := c.resolveExprType(node.X)
		if recvType != "" {
			return c.resolveFieldMapKeyKind(recvType, node.Name)
		}
	}
	// Indexing a slice of maps: determine key kind from the map element type
	if node.Kind == NIndexExpr && node.X != nil {
		collType := c.resolveExprType(node.X)
		if len(collType) > 6 && collType[0] == '[' && collType[1] == ']' && collType[2] == 'm' && collType[3] == 'a' && collType[4] == 'p' && collType[5] == '[' {
			// Extract key type from "[]map[K]V"
			keyType := collType[6:len(collType)]
			// Find closing ]
			depth := 1
			ki := 0
			for ki < len(keyType) && depth > 0 {
				if keyType[ki] == '[' {
					depth++
				} else if keyType[ki] == ']' {
					depth = depth - 1
				}
				if depth > 0 {
					ki++
				}
			}
			keyName := keyType[0:ki]
			if keyName == "string" {
				return 1
			}
			return 0
		}
	}
	return -1
}

// isMapExpr returns true if the expression evaluates to a map value.
func (c *Compiler) isMapExpr(node *Node) bool {
	if node == nil {
		return false
	}
	if node.Kind == NIdent {
		_, ok := c.localMapVars[node.Name]
		if ok {
			return true
		}
		// Check qualified global
		qname := c.curPkg.Path + "." + node.Name
		_, ok = c.globalMapVars[qname]
		return ok
	}
	// Check for pkg.mapVar or struct.mapField
	if node.Kind == NSelectorExpr && node.X != nil {
		if node.X.Kind == NIdent {
			pkg := c.resolvePackage(node.X.Name)
			if pkg != nil {
				qname := pkg.Path + "." + node.Name
				_, ok := c.globalMapVars[qname]
				return ok
			}
		}
		// Check if this is a struct field that is a map type (handles chained selectors)
		recvType := c.resolveExprType(node.X)
		if recvType != "" {
			return c.resolveFieldIsMap(recvType, node.Name)
		}
	}
	// Check for indexing a slice-of-maps: scopes[i] where scopes is []map[K]V
	if node.Kind == NIndexExpr && node.X != nil {
		collType := c.resolveExprType(node.X)
		if len(collType) > 6 && collType[0] == '[' && collType[1] == ']' && collType[2] == 'm' && collType[3] == 'a' && collType[4] == 'p' && collType[5] == '[' {
			return true
		}
	}
	return false
}

func (c *Compiler) compileSliceExpr(node *Node) {
	c.compileExpr(node.X)
	c.compileExpr(node.Y)
	if node.Body != nil {
		c.compileExpr(node.Body)
	} else {
		c.compileExpr(node.X)
		c.emit(Inst{Op: OP_LEN})
	}

	// Use StringSlice for string-typed targets, SliceReslice for slices
	if c.isStringTypedExpr(node.X) {
		c.emit(Inst{Op: OP_CALL, Name: "runtime.StringSlice", Arg: 3})
	} else {
		c.emit(Inst{Op: OP_CALL, Name: "runtime.SliceReslice", Arg: 3})
	}
}

func (c *Compiler) compileCompositeLit(node *Node) {
	// Handle map composite literals: map[K]V{k1: v1, k2: v2, ...}
	if node.Type != nil && node.Type.Kind == NMapType {
		keyKind := c.mapKeyKind(node.Type.X)
		c.emit(Inst{Op: OP_CONST_I64, Val: int64(keyKind)})
		c.emit(Inst{Op: OP_CALL, Name: "runtime.MapMake", Arg: 1})
		// For each key-value pair, call MapSet
		for _, elem := range node.Nodes {
			if elem.Kind == NKeyValue {
				// Stack: map_hdr
				// Dup map header, push key, push value, call MapSet
				c.emit(Inst{Op: OP_DUP})
				c.compileExpr(elem.X)
				c.compileExpr(elem.Y)
				c.emit(Inst{Op: OP_CALL, Name: "runtime.MapSet", Arg: 3})
				c.emit(Inst{Op: OP_DROP}) // drop the returned header (same as input)
				// Original map_hdr still on stack
			}
		}
		return
	}

	// Handle slice composite literals: []T{e1, e2, ...}
	if node.Type != nil && node.Type.Kind == NSliceType {
		elemSize := c.sliceElemSize(node.Type)
		if len(node.Nodes) == 0 {
			// Empty slice literal: use SliceMake with length 0
			c.emit(Inst{Op: OP_CONST_I64, Val: 0})
			c.emit(Inst{Op: OP_CONST_I64, Val: int64(elemSize)})
			c.emit(Inst{Op: OP_CALL, Name: "runtime.SliceMake", Arg: 2})
		} else {
			// Build slice by appending each element
			c.emit(Inst{Op: OP_CONST_I64, Val: 0}) // nil slice
			for _, elem := range node.Nodes {
				c.compileExpr(elem) // push element value
				c.emit(Inst{Op: OP_CONST_I64, Val: int64(elemSize)})
				c.emit(Inst{Op: OP_CALL, Name: "runtime.SliceAppend", Arg: 3})
			}
		}
		return
	}

	// Struct composite literals
	typeName := ""
	if node.Type != nil {
		typeName = nodeTypeName(node.Type)
	}

	// Check if this is a key-value composite literal (named fields)
	hasKeyValue := false
	for _, elem := range node.Nodes {
		if elem.Kind == NKeyValue {
			hasKeyValue = true
			break
		}
	}

	if hasKeyValue {
		// Look up the struct type to get all field names
		structFields := c.getStructFields(typeName)
		if len(structFields) > 0 {
			// Build a map of field name → expression
			fieldVals := make(map[string]*Node)
			for _, elem := range node.Nodes {
				if elem.Kind == NKeyValue && elem.X != nil {
					fieldVals[elem.X.Name] = elem.Y
				}
			}
			// Push values in struct field declaration order
			for _, fname := range structFields {
				val, ok := fieldVals[fname]
				if ok {
					c.compileExpr(val)
				} else {
					c.emit(Inst{Op: OP_CONST_I64, Val: 0})
				}
			}
			c.emit(Inst{Op: OP_CALL, Name: "builtin.composite." + typeName, Arg: len(structFields)})
		} else {
			// Fallback: push values in literal order
			for _, elem := range node.Nodes {
				if elem.Kind == NKeyValue {
					c.compileExpr(elem.Y)
				} else {
					c.compileExpr(elem)
				}
			}
			c.emit(Inst{Op: OP_CALL, Name: "builtin.composite." + typeName, Arg: len(node.Nodes)})
		}
	} else {
		if len(node.Nodes) == 0 {
			// Empty struct literal like &Foo{}: allocate zero-initialized struct
			structFields := c.getStructFields(typeName)
			nfields := len(structFields)
			if nfields == 0 {
				nfields = 1 // at minimum allocate something
			}
			i := 0
			for i < nfields {
				c.emit(Inst{Op: OP_CONST_I64, Val: 0})
				i++
			}
			c.emit(Inst{Op: OP_CALL, Name: "builtin.composite." + typeName, Arg: nfields})
		} else {
			// Positional: push values in literal order
			for _, elem := range node.Nodes {
				c.compileExpr(elem)
			}
			c.emit(Inst{Op: OP_CALL, Name: "builtin.composite." + typeName, Arg: len(node.Nodes)})
		}
	}
}

// === Helper functions ===

func nodeTypeName(node *Node) string {
	if node == nil {
		return ""
	}
	switch node.Kind {
	case NIdent:
		return node.Name
	case NSelectorExpr:
		if node.X != nil {
			return nodeTypeName(node.X) + "." + node.Name
		}
		return node.Name
	case NPointerType:
		return "*" + nodeTypeName(node.X)
	case NSliceType:
		return "[]" + nodeTypeName(node.X)
	case NMapType:
		return "map[" + nodeTypeName(node.X) + "]" + nodeTypeName(node.Y)
	}
	return ""
}

func parseIntLiteral(s string) int64 {
	if len(s) >= 2 && s[0] == '0' && s[1] == 'x' {
		return parseHexLiteral(s[2:len(s)])
	}
	var result int64
	i := 0
	neg := false
	if i < len(s) && s[i] == '-' {
		neg = true
		i++
	}
	// Octal: starts with 0 and has more digits
	if i < len(s) && s[i] == '0' && i+1 < len(s) {
		i++
		for i < len(s) {
			result = result*8 + int64(s[i]-'0')
			i++
		}
	} else {
		for i < len(s) {
			result = result*10 + int64(s[i]-'0')
			i++
		}
	}
	if neg {
		result = 0 - result
	}
	return result
}

func parseHexLiteral(s string) int64 {
	var result int64
	i := 0
	for i < len(s) {
		ch := s[i]
		if ch >= '0' && ch <= '9' {
			result = result*16 + int64(ch-'0')
		} else if ch >= 'a' && ch <= 'f' {
			result = result*16 + int64(ch-'a'+10)
		} else if ch >= 'A' && ch <= 'F' {
			result = result*16 + int64(ch-'A'+10)
		}
		i++
	}
	return result
}

func parseRuneLiteral(s string) int {
	if len(s) == 0 {
		return 0
	}
	if s[0] == '\\' && len(s) >= 2 {
		switch s[1] {
		case 'n':
			return 10
		case 't':
			return 9
		case 'r':
			return 13
		case '\\':
			return 92
		case '\'':
			return 39
		case '"':
			return 34
		case '0':
			return 0
		}
		return int(s[1])
	}
	return int(s[0])
}

// encodeStringLiteral converts raw bytes to an escaped string literal format
// suitable for OP_CONST_STR. This is the inverse of decodeStringLiteral.
func encodeStringLiteral(raw string) string {
	var buf []byte
	i := 0
	for i < len(raw) {
		ch := raw[i]
		if ch == '\\' {
			buf = append(buf, '\\', '\\')
		} else if ch == '"' {
			buf = append(buf, '\\', '"')
		} else if ch == '\n' {
			buf = append(buf, '\\', 'n')
		} else if ch == '\r' {
			buf = append(buf, '\\', 'r')
		} else if ch == '\t' {
			buf = append(buf, '\\', 't')
		} else if ch == 0 {
			buf = append(buf, '\\', '0')
		} else if ch < 32 || ch >= 127 {
			buf = append(buf, '\\', 'x', hexDigit(ch>>4), hexDigit(ch&0x0f))
		} else {
			buf = append(buf, ch)
		}
		i++
	}
	return string(buf)
}

func hexDigit(v byte) byte {
	if v < 10 {
		return '0' + v
	}
	return 'a' + v - 10
}

// walkEmbedDir recursively collects all files under dir, returning
// relative paths (relative to base) and their contents.
func walkEmbedDir(base string, dir string) ([]string, []string) {
	var names []string
	var data []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return names, data
	}
	for _, entry := range entries {
		path := dir + "/" + entry.Name()
		if entry.IsDir() {
			subNames, subData := walkEmbedDir(base, path)
			names = append(names, subNames...)
			data = append(data, subData...)
		} else {
			content, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			// Compute relative path from base
			rel := path[len(base)+1 : len(path)]
			names = append(names, rel)
			data = append(data, string(content))
		}
	}
	return names, data
}
