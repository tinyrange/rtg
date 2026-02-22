package frontend

import (
	"fmt"
	"strings"

	"j5.nz/rtg/std/compiler/backend/vm"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
	"j5.nz/rtg/std/compiler/stdlib"
)

const (
	comptimePkgPath   = "j5.nz/rtg/x/comptime"
	comptimePkgPrefix = "j5.nz/rtg/x/comptime."
)

type closureCaptureSpec struct {
	Name  string
	Width int
}

type closureCaptureBinding struct {
	LocalIdx int
	Width    int
	IsPtr    bool
}

// === Compiler ===

// Compiler lowers AST from a Module into stack machine IR.
type Compiler struct {
	target               *common.Target
	mod                  *Module
	irmod                *ir.IRModule
	curFunc              *ir.IRFunc
	scopes               []map[string]int
	labelSeq             int
	breaks               []int
	continues            []int
	fallthroughs         []int
	pendingStmtLabels    []string
	breakLabelTargets    map[string][]int
	continueLabelTargets map[string][]int
	globals              map[string]int
	types                map[string]*ir.TypeInfo
	curPkg               *Package
	errors               []string
	funcRets             map[string]int      // function name → return count
	funcParams           map[string]int      // function name → param count
	funcParamTypes       map[string][]string // function name → param type names (receiver first for methods)
	funcVariadic         map[string]int      // variadic function name → count of fixed params
	funcVariadicIface    map[string]bool     // variadic function name → true if ...interface{}
	funcVariadicElem     map[string]int      // variadic function name → variadic elem size (1 for ...byte, 8 otherwise)
	funcIsInternal       map[string]bool     // function name → true if declared via //rtg:internal
	funcIsLinkStatic     map[string]bool     // function name → true if declared via //rtg:linkstatic
	comptimeFuncs        map[string]bool     // function/method name → true if marked //rtg:comptime
	funcRetTypeNodes     map[string]*Node    // function name → first return type node (for comptime literal synthesis)
	localElemSizes       map[string]int      // variable name → slice element size (1 for byte, 8 otherwise)
	globalElemSizes      map[string]int      // qualified global name → slice element size
	ifaceMethods         map[string][]string // interface name → method names
	ifaceMethodRets      map[string]int      // iface+"\x00"+method → return count
	methodTable          map[string]string   // "pkg.Type.Method" → qualified IR func name
	typeIDs              map[string]int      // concrete type qualified name → unique int
	nextTypeID           int
	localTypes           map[string]string   // local var name → type name (for interface-typed locals)
	localTypeDecls       map[string]*Node    // local type name → type declaration node (function scope)
	localStringVars      map[string]bool     // local var name → true if the local is a string
	localConcreteTypes   map[string]string   // local var name → qualified type name for method resolution
	funcRetTypes         map[string][]string // function name → return type names
	localMapVars         map[string]int      // local var name → keyKind (0=int, 1=string) if it's a map
	localMapValueTypes   map[string]string   // local map var name → value type name (e.g. "*Package")
	globalMapVars        map[string]int      // qualified global name → keyKind if it's a map
	globalConcreteTypes  map[string]string   // qualified global name → qualified type name
	constValues          map[string]int64    // qualified const name → precomputed value
	constStringValues    map[string]string   // qualified const name → precomputed string value
	localAddrOf          map[string]bool     // local var name → true if assigned from &var (pointer-to-pointer)
	stackDepth           int                 // operand stack depth tracking for balance checks
	deferNames           []string
	deferArgStarts       []int
	deferArgCounts       []int
	deferRetCounts       []int
	namedResultNames     []string
	labelIDs             map[string]int
	funcLitSeq           int
	localFuncTargets     map[string]string
	localMethodTargets   map[string]string
	localMethodRecv      map[string]int
	funcLiteralCaptures  map[string][]closureCaptureSpec
	localFuncCaptures    map[string][]closureCaptureBinding
	activeCaptures       map[string]closureCaptureBinding
	dotJoinCache         map[string]map[string]string // a → b → "a.b"
	qualifyTypeCache     map[string]string            // "typeName\x00pkgPath" → qualified result
	comptimeSeq          int
	comptimeDisabled     bool
	inComptimeFunc       bool
}

func (c *Compiler) dotJoin(a string, b string) string {
	m := c.dotJoinCache[a]
	if m != nil {
		if v, ok := m[b]; ok {
			return v
		}
	} else {
		m = make(map[string]string)
		c.dotJoinCache[a] = m
	}
	v := a + "." + b
	m[b] = v
	return v
}

// CompileModule compiles an entire resolved module to IR.
func CompileModule(target common.Target, mod *Module) (*ir.IRModule, []string) {
	c := &Compiler{
		target:              &target,
		mod:                 mod,
		irmod:               &ir.IRModule{},
		globals:             make(map[string]int),
		types:               make(map[string]*ir.TypeInfo),
		funcRets:            make(map[string]int),
		funcParams:          make(map[string]int),
		funcParamTypes:      make(map[string][]string),
		funcVariadic:        make(map[string]int),
		funcVariadicIface:   make(map[string]bool),
		funcVariadicElem:    make(map[string]int),
		funcIsInternal:      make(map[string]bool),
		funcIsLinkStatic:    make(map[string]bool),
		comptimeFuncs:       make(map[string]bool),
		funcRetTypeNodes:    make(map[string]*Node),
		globalElemSizes:     make(map[string]int),
		ifaceMethods:        make(map[string][]string),
		ifaceMethodRets:     make(map[string]int),
		methodTable:         make(map[string]string),
		typeIDs:             make(map[string]int),
		nextTypeID:          4, // 1=int, 2=string, 3=bool are reserved
		funcRetTypes:        make(map[string][]string),
		globalMapVars:       make(map[string]int),
		globalConcreteTypes: make(map[string]string),
		constValues:         make(map[string]int64),
		constStringValues:   make(map[string]string),
		funcLiteralCaptures: make(map[string][]closureCaptureSpec),
		localFuncCaptures:   make(map[string][]closureCaptureBinding),
		dotJoinCache:        make(map[string]map[string]string),
		qualifyTypeCache:    make(map[string]string),
	}
	c.initBuiltinTypes()

	// Pre-pass: collect interface and method declarations for all packages so
	// interface method lowering does not depend on per-package compile order.
	for _, path := range mod.Order {
		pkg, ok := mod.Packages[path]
		if !ok {
			continue
		}
		c.curPkg = pkg
		c.buildInterfaceTable(pkg)
	}

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
			qname := pkg.QualName(name)
			idx := len(c.irmod.Globals)
			c.globals[qname] = idx
			c.irmod.Globals = append(c.irmod.Globals, ir.IRGlobal{Name: qname, Index: idx})
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
			} else if sym.Node != nil && sym.Node.X != nil && sym.Node.X.Kind == NCompositeLit && sym.Node.X.Type != nil {
				tn := nodeTypeName(sym.Node.X.Type)
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
	c.irmod.IfaceMethodRets = c.ifaceMethodRets
	ir.OptimizeIRModule(c.target, c.irmod)

	return c.irmod, c.errors
}

func (c *Compiler) initBuiltinTypes() {
	c.types["bool"] = &ir.TypeInfo{Kind: ir.TY_BOOL, Name: "bool", Size: 1, Align: 1}
	c.types["byte"] = &ir.TypeInfo{Kind: ir.TY_BYTE, Name: "byte", Size: 1, Align: 1}
	c.types["int16"] = &ir.TypeInfo{Kind: ir.TY_INT32, Name: "int16", Size: 2, Align: 2}
	c.types["uint16"] = &ir.TypeInfo{Kind: ir.TY_INT32, Name: "uint16", Size: 2, Align: 2}
	c.types["int32"] = &ir.TypeInfo{Kind: ir.TY_INT32, Name: "int32", Size: 4, Align: 4}
	c.types["int"] = &ir.TypeInfo{Kind: ir.TY_INT, Name: "int", Size: 8, Align: 8}
	c.types["uintptr"] = &ir.TypeInfo{Kind: ir.TY_UINTPTR, Name: "uintptr", Size: 8, Align: 8}
	c.types["string"] = &ir.TypeInfo{Kind: ir.TY_STRING, Name: "string", Size: 16, Align: 8}
	c.types["error"] = &ir.TypeInfo{Kind: ir.TY_INTERFACE, Name: "error", Size: 16, Align: 8}
	c.types["int64"] = &ir.TypeInfo{Kind: ir.TY_INT, Name: "int64", Size: 8, Align: 8}
	c.typeIDs["bool"] = 3
	c.ifaceMethods["interface{}"] = []string{}
	c.ifaceMethods["error"] = []string{"Error"}
	c.ifaceMethodRets["error\x00Error"] = 1
}

func (c *Compiler) errorf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	c.errors = append(c.errors, msg)
}

func (c *Compiler) lookupCurrentTypeDecl(name string) (*Node, bool) {
	if c.localTypeDecls != nil {
		if n, ok := c.localTypeDecls[name]; ok && n != nil {
			return n, true
		}
	}
	if c.curPkg != nil {
		if sym, ok := c.curPkg.Symbols[name]; ok && sym.Kind == SymType && sym.Node != nil {
			return sym.Node, true
		}
	}
	return nil, false
}

func (c *Compiler) registerLocalTypeDecl(node *Node) {
	if node == nil || node.Kind != NTypeDecl || node.Name == "" {
		return
	}
	if c.localTypeDecls == nil {
		c.localTypeDecls = make(map[string]*Node)
	}
	c.localTypeDecls[node.Name] = node
}

func isBuiltinName(name string) bool {
	if len(name) == 0 {
		return false
	}
	switch name[0] {
	case 'l':
		return name == "len"
	case 'c':
		return name == "cap" || name == "copy" || name == "close" || name == "complex" || name == "clear"
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
		return name == "int" || name == "int16" || name == "int32" || name == "int64" || name == "iota" || name == "imag"
	case 'u':
		return name == "uint" || name == "uint16" || name == "uint32" || name == "uint64" || name == "uintptr"
	case 'b':
		return name == "byte" || name == "bool"
	case 's':
		return name == "string"
	case 'f':
		return name == "false" || name == "float32" || name == "float64"
	case 'r':
		return name == "rune" || name == "recover" || name == "real"
	case 'e':
		return name == "error"
	case 't':
		return name == "true"
	}
	return false
}

func (c *Compiler) resolvePackage(pkgName string) *Package {
	if c.curPkg != nil && c.curPkg.ImportAliases != nil {
		if path, ok := c.curPkg.ImportAliases[pkgName]; ok {
			if pkg, ok := c.mod.Packages[path]; ok {
				return pkg
			}
		}
	}
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
	if pkgPath == c.curPkg.Path {
		if localDecl, ok := c.localTypeDecls[typeName]; ok && localDecl != nil && localDecl.Type != nil {
			return localDecl.Type, pkgPath
		}
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

func (c *Compiler) resolveFieldPathRec(qualifiedType string, fieldName string, visited map[string]bool) ([]int, bool) {
	if qualifiedType == "" || visited[qualifiedType] {
		return nil, false
	}
	visited[qualifiedType] = true

	typeNode, pkgPath := c.lookupStructTypeNode(qualifiedType)
	if typeNode == nil {
		return nil, false
	}

	// Direct field match.
	fieldIdx := 0
	for _, field := range typeNode.Nodes {
		if field.Kind != NField {
			continue
		}
		if field.Name == fieldName {
			return []int{fieldIdx * c.target.PtrSize}, true
		}
		fieldIdx++
	}

	// Promoted field match through embedded fields.
	fieldIdx = 0
	for _, field := range typeNode.Nodes {
		if field.Kind != NField {
			continue
		}
		if field.Type != nil && field.Name == nodeTypeName(field.Type) {
			embeddedType := c.qualifyTypeName(nodeTypeName(field.Type), pkgPath)
			if subPath, ok := c.resolveFieldPathRec(embeddedType, fieldName, visited); ok {
				path := []int{fieldIdx * c.target.PtrSize}
				for _, off := range subPath {
					path = append(path, off)
				}
				return path, true
			}
		}
		fieldIdx++
	}
	return nil, false
}

func (c *Compiler) resolveFieldPath(qualifiedType string, fieldName string) ([]int, bool) {
	return c.resolveFieldPathRec(qualifiedType, fieldName, make(map[string]bool))
}

// resolveFieldType looks up the type of a struct field given a qualified type name and field name.
func (c *Compiler) resolveFieldType(qualifiedType string, fieldName string) string {
	if path, ok := c.resolveFieldPath(qualifiedType, fieldName); ok {
		curType := qualifiedType
		i := 0
		for i < len(path) {
			typeNode, pkgPath := c.lookupStructTypeNode(curType)
			if typeNode == nil {
				return ""
			}
			targetIdx := path[i] / c.target.PtrSize
			fieldIdx := 0
			var match *Node
			for _, field := range typeNode.Nodes {
				if field.Kind != NField {
					continue
				}
				if fieldIdx == targetIdx {
					match = field
					break
				}
				fieldIdx++
			}
			if match == nil || match.Type == nil {
				return ""
			}
			tname := c.qualifyTypeName(nodeTypeName(match.Type), pkgPath)
			if i == len(path)-1 {
				return tname
			}
			curType = tname
			i++
		}
	}
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
	if path, ok := c.resolveFieldPath(qualifiedType, fieldName); ok && len(path) > 0 {
		return path[0]
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
		return c.target.PtrSize
	}
	if typeName == "byte" {
		return 1
	}
	return c.target.PtrSize
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
		qname := c.curPkg.QualName(node.Name)
		if ct, ok := c.globalConcreteTypes[qname]; ok {
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
		if node.X != nil && node.X.Kind == NIdent && node.X.Name == "recover" {
			return "interface{}"
		}
		if node.X != nil && node.X.Kind == NIdent && node.X.Name == "new" && len(node.Nodes) == 1 {
			typeName := nodeTypeName(node.Nodes[0])
			if typeName != "" {
				return c.qualifyTypeName("*"+typeName, "")
			}
		}
		if node.X != nil && node.X.Kind == NSliceType && node.X.X != nil {
			return "[]" + c.qualifyTypeName(nodeTypeName(node.X.X), "")
		}
		if node.X != nil && node.X.Kind == NIdent && node.X.Name == "string" {
			return "string"
		}
		calleeName := c.resolveCallName(node.X)
		if node.X != nil && node.X.Kind == NIdent {
			if target, ok := c.localFuncTargets[node.X.Name]; ok {
				calleeName = target
			} else if target, ok := c.localMethodTargets[node.X.Name]; ok {
				calleeName = target
			}
		}
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
	case "int16", "uint16":
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
					qname := pkg.QualName(child.Name)
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
				qname := pkg.QualName(node.Name)
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
		qname := c.curPkg.QualName(node.Name)
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
		qname := c.curPkg.QualName(node.Name)
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
		qname := c.curPkg.QualName(node.Name)
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
			fn, directives := unwrapDirectiveNode(node)
			if fn == nil {
				continue
			}
			if fn.Kind != NFunc {
				continue
			}
			qname := pkg.QualName(fn.Name)
			if fn.X != nil {
				// Method with receiver
				recvType := nodeTypeName(fn.X.Type)
				qname = c.dotJoin(pkg.QualName(recvType), fn.Name)
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
			var firstRet *Node
			if fn.Type != nil {
				if fn.Type.Kind == NFuncType && len(fn.Type.Nodes) > 0 {
					if fn.Type.Nodes[0].Type != nil {
						firstRet = fn.Type.Nodes[0].Type
					} else {
						firstRet = fn.Type.Nodes[0]
					}
				} else {
					firstRet = fn.Type
				}
			}
			c.funcRetTypeNodes[qname] = firstRet
			isComptimeFunc := false
			for _, d := range directives {
				if isComptimeDirective(d) {
					isComptimeFunc = true
				}
				if parseInternalDirective(d) != "" {
					c.funcIsInternal[qname] = true
				}
				if _, ok := parseLinkStaticDirective(d); ok {
					c.funcIsLinkStatic[qname] = true
				}
			}

			// Pre-register variadic info and param count
			paramCount := len(fn.Nodes)
			fixedParams := 0
			var paramTypeNames []string
			if fn.X != nil {
				paramCount++
				fixedParams = 1 // receiver counts as fixed
				if fn.X.Type != nil {
					paramTypeNames = append(paramTypeNames, nodeTypeName(fn.X.Type))
				} else {
					paramTypeNames = append(paramTypeNames, "")
				}
			}
			isVariadic := false
			isIfaceVariadic := false
			varElemSize := c.target.PtrSize
			for _, param := range fn.Nodes {
				paramTypeName := ""
				if param.Type != nil {
					paramTypeName = nodeTypeName(param.Type)
				}
				if len(param.Name) > 3 && param.Name[0:3] == "..." {
					if paramTypeName != "" {
						paramTypeName = "[]" + paramTypeName
					}
					paramTypeNames = append(paramTypeNames, paramTypeName)
					isVariadic = true
					if param.Type != nil && param.Type.Kind == NInterfaceType {
						isIfaceVariadic = true
					}
					if param.Type != nil && param.Type.Kind == NIdent && param.Type.Name == "byte" {
						varElemSize = 1
					}
				} else {
					paramTypeNames = append(paramTypeNames, paramTypeName)
					fixedParams++
				}
			}
			c.funcParams[qname] = paramCount
			c.funcParamTypes[qname] = paramTypeNames
			if isComptimeFunc {
				c.comptimeFuncs[qname] = true
			}
			if isVariadic {
				c.funcVariadic[qname] = fixedParams
				c.funcVariadicIface[qname] = isIfaceVariadic
				c.funcVariadicElem[qname] = varElemSize
			}
		}
	}
}

func (c *Compiler) isComptimeCallAllowed(callName string) bool {
	if !c.inComptimeFunc {
		return true
	}
	if callName == "" {
		return true
	}
	if c.funcIsInternal[callName] || c.funcIsLinkStatic[callName] {
		if strings.HasPrefix(callName, comptimePkgPrefix) {
			return true
		}
		c.errorf("%s: comptime functions may only access host operations via j5.nz/rtg/x/comptime (disallowed call: %s)", c.curFunc.Name, callName)
		return false
	}
	return true
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
		qname := pkg.QualName(node.Name)
		var methods []string
		for _, meth := range node.Type.Nodes {
			if meth.Kind == NFunc {
				methods = append(methods, meth.Name)
				retCount := 0
				if meth.Type != nil {
					if meth.Type.Kind == NFuncType {
						retCount = len(meth.Type.Nodes)
					} else {
						retCount = 1
					}
				}
				c.ifaceMethodRets[node.Name+"\x00"+meth.Name] = retCount
				c.ifaceMethodRets[qname+"\x00"+meth.Name] = retCount
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

func (c *Compiler) ifaceMethodReturnCount(ifaceType string, methodName string) (int, bool) {
	if ifaceType == "" || methodName == "" {
		return 0, false
	}
	key := ifaceType + "\x00" + methodName
	if ret, ok := c.ifaceMethodRets[key]; ok {
		return ret, true
	}
	return 0, false
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
		qtype := pkg.QualName(recvType)
		qname := c.dotJoin(qtype, node.Name)
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
				continue
			}
			if node.Kind == NDirective && node.X != nil && node.X.Kind == NVarDecl {
				directiveVar := node.X
				if directiveVar.X != nil {
					inits = append(inits, directiveVar)
				} else if len(directiveVar.Nodes) > 0 {
					for _, child := range directiveVar.Nodes {
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
	f := &ir.IRFunc{Name: pkg.Path + ".init$globals"}
	c.curFunc = f
	c.scopes = nil
	c.localElemSizes = make(map[string]int)
	c.localStringVars = make(map[string]bool)
	c.localAddrOf = make(map[string]bool)
	c.localConcreteTypes = make(map[string]string)
	c.localMapVars = make(map[string]int)
	c.localMapValueTypes = make(map[string]string)
	c.stackDepth = 0
	c.pushScope()
	for _, node := range inits {
		qname := pkg.QualName(node.Name)
		gidx, ok := c.globals[qname]
		if !ok {
			continue
		}
		c.compileExpr(node.X)
		c.emit(ir.Inst{Op: ir.OP_GLOBAL_SET, Arg: gidx})
	}

	// Generate embed init code
	for _, emb := range embeds {
		qname := pkg.QualName(emb.name)
		gidx, ok := c.globals[qname]
		if !ok {
			continue
		}
		c.compileEmbedInit(pkg, gidx, emb.pattern)
	}

	c.emit(ir.Inst{Op: ir.OP_RETURN, Arg: 0})
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
	names, data := stdlib.WalkEmbedFromFS(embedDir)
	if names == nil {
		names, data = common.WalkDirectory(embedDir, embedDir)
	}

	// Sort for deterministic order
	sortEmbedFiles(names, data)

	// Create empty FS struct: push 2 nil fields (names, data slices)
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0}) // nil names slice
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0}) // nil data slice
	c.emit(ir.Inst{Op: ir.OP_CALL, Name: "builtin.composite.embed.FS", Arg: 2})
	c.emit(ir.Inst{Op: ir.OP_GLOBAL_SET, Arg: gidx})

	// For each file, call embed.AddFile(fs, name, data)
	for i := 0; i < len(names); i++ {
		c.emit(ir.Inst{Op: ir.OP_GLOBAL_GET, Arg: gidx})
		c.emit(ir.Inst{Op: ir.OP_CONST_STR, Name: encodeStringLiteral(names[i])})
		c.emit(ir.Inst{Op: ir.OP_CONST_STR, Name: encodeStringLiteral(data[i])})
		c.emit(ir.Inst{Op: ir.OP_CALL, Name: "embed.AddFile", Arg: 3})
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
		for j > 0 && stringLess(embeds[j].name, embeds[j-1].name) {
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
		for j > 0 && stringLess(names[j], names[j-1]) {
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

func unwrapDirectiveNode(node *Node) (*Node, []string) {
	var directives []string
	cur := node
	for cur != nil && cur.Kind == NDirective {
		directives = append(directives, cur.Name)
		cur = cur.X
	}
	return cur, directives
}

func validLinkStaticMode(mode string) bool {
	return mode == "" || mode == "syscall" || mode == "ptr" || mode == "rawptr" || mode == "noreturn"
}

func encodeLinkStaticDirective(spec LinkStaticDirective) string {
	return spec.Library + "," + spec.Symbol + "," + spec.Mode
}

func (c *Compiler) compileTopDecl(node *Node) {
	if node == nil {
		return
	}
	switch node.Kind {
	case NFunc:
		c.compileFunc(node)
	case NDirective:
		base, directives := unwrapDirectiveNode(node)
		if base != nil && base.Kind == NFunc {
			intern := ""
			var linkspec LinkStaticDirective
			hasLinkStatic := false
			for _, d := range directives {
				in := parseInternalDirective(d)
				if in != "" {
					intern = in
				}
				ls, ok := parseLinkStaticDirective(d)
				if ok {
					linkspec = ls
					hasLinkStatic = true
				}
			}
			if intern != "" {
				c.compileIntrinsicFunc(base, intern)
			} else if hasLinkStatic {
				c.compileLinkStaticFunc(base, linkspec)
			} else {
				c.compileFunc(base)
			}
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
	qname := c.curPkg.QualName(node.Name)
	if node.X != nil {
		// Method with receiver
		recvType := nodeTypeName(node.X.Type)
		qname = c.dotJoin(c.curPkg.QualName(recvType), node.Name)
	}
	f := &ir.IRFunc{Name: qname}
	savedInComptimeFunc := c.inComptimeFunc
	if savedInComptimeFunc || c.comptimeFuncs[qname] {
		c.inComptimeFunc = true
	} else {
		c.inComptimeFunc = false
	}
	c.curFunc = f
	c.scopes = nil
	c.localElemSizes = make(map[string]int)
	c.localTypes = make(map[string]string)
	c.localTypeDecls = make(map[string]*Node)
	c.localStringVars = make(map[string]bool)
	c.localAddrOf = make(map[string]bool)
	c.localConcreteTypes = make(map[string]string)
	c.localMapVars = make(map[string]int)
	c.localMapValueTypes = make(map[string]string)
	c.deferNames = nil
	c.deferArgStarts = nil
	c.deferArgCounts = nil
	c.deferRetCounts = nil
	c.namedResultNames = nil
	c.fallthroughs = nil
	c.pendingStmtLabels = nil
	c.labelIDs = make(map[string]int)
	c.breakLabelTargets = make(map[string][]int)
	c.continueLabelTargets = make(map[string][]int)
	c.localFuncTargets = make(map[string]string)
	c.localMethodTargets = make(map[string]string)
	c.localMethodRecv = make(map[string]int)
	c.localFuncCaptures = make(map[string][]closureCaptureBinding)
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
	varElemSize := c.target.PtrSize
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
				if isVarParam {
					typeName = "[]" + typeName
				}
				qualifiedType := c.qualifyTypeName(typeName, "")
				// Track interface-typed params
				if c.isInterfaceTypeName(typeName) || c.isInterfaceTypeName(qualifiedType) {
					c.localTypes[pname] = qualifiedType
				}
				c.localConcreteTypes[pname] = qualifiedType
				// Also track slice elem sizes from type
				if len(qualifiedType) > 2 && qualifiedType[0] == '[' && qualifiedType[1] == ']' {
					c.localElemSizes[pname] = c.typeElemSize(qualifiedType[2:len(qualifiedType)])
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
					c.namedResultNames = append(c.namedResultNames, ret.Name)
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
	if codeLen == 0 || f.Code[codeLen-1].Op != ir.OP_RETURN {
		if len(c.deferNames) > 0 {
			c.emitDeferredCalls()
		}
		c.emit(ir.Inst{Op: ir.OP_RETURN, Arg: 0})
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
	c.inComptimeFunc = savedInComptimeFunc
}

func (c *Compiler) compileIntrinsicFunc(node *Node, intern string) {
	qname := c.curPkg.QualName(node.Name)

	f := &ir.IRFunc{Name: qname}
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
	varElemSizeI := c.target.PtrSize
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
	c.emit(ir.Inst{Op: ir.OP_CALL_INTRINSIC, Name: intern, Arg: paramCount})
	c.emit(ir.Inst{Op: ir.OP_RETURN, Arg: f.RetCount})

	c.funcRets[f.Name] = f.RetCount
	c.funcParams[f.Name] = f.Params
	if isVariadic {
		c.funcVariadic[f.Name] = fixedParams
		c.funcVariadicElem[f.Name] = varElemSizeI
	}
	c.irmod.Funcs = append(c.irmod.Funcs, f)
	c.curFunc = nil
}

func (c *Compiler) compileLinkStaticFunc(node *Node, spec LinkStaticDirective) {
	if node.X != nil {
		c.errors = append(c.errors, "linkstatic methods are not supported")
		return
	}
	if !validLinkStaticMode(spec.Mode) {
		c.errors = append(c.errors, fmt.Sprintf("invalid linkstatic mode %q on %s", spec.Mode, node.Name))
		return
	}

	qname := c.curPkg.QualName(node.Name)
	intrinsicName := qname

	f := &ir.IRFunc{Name: qname}
	c.curFunc = f

	// Count params and detect variadic
	paramCount := len(node.Nodes)
	f.Params = paramCount
	isVariadic := false
	fixedParams := 0
	varElemSizeI := c.target.PtrSize
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
	mode := spec.Mode
	if mode == "" {
		mode = "syscall"
	}
	if mode == "noreturn" {
		if f.RetCount != 0 {
			c.errors = append(c.errors, fmt.Sprintf("linkstatic noreturn function %s must return no values", qname))
			c.curFunc = nil
			return
		}
	} else if f.RetCount != 3 {
		c.errors = append(c.errors, fmt.Sprintf("linkstatic function %s must return (uintptr, uintptr, int32)", qname))
		c.curFunc = nil
		return
	}

	if c.irmod.LinkStaticFuncs == nil {
		c.irmod.LinkStaticFuncs = make(map[string]string)
	}
	c.irmod.LinkStaticFuncs[intrinsicName] = encodeLinkStaticDirective(spec)

	// Emit single intrinsic call
	c.stackDepth = 0
	c.emit(ir.Inst{Op: ir.OP_CALL_INTRINSIC, Name: intrinsicName, Arg: paramCount})
	c.emit(ir.Inst{Op: ir.OP_RETURN, Arg: f.RetCount})

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
	c.curFunc.Locals = append(c.curFunc.Locals, ir.IRLocal{Name: name, Index: idx})
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

func (c *Compiler) emit(inst ir.Inst) {
	c.curFunc.Code = append(c.curFunc.Code, inst)
	c.stackDepth = c.stackDepth + c.instStackDelta(inst)
}

func (c *Compiler) instStackDelta(inst ir.Inst) int {
	switch inst.Op {
	case ir.OP_CONST_I64, ir.OP_CONST_STR, ir.OP_CONST_BOOL, ir.OP_CONST_NIL:
		return 1
	case ir.OP_LOCAL_GET, ir.OP_GLOBAL_GET, ir.OP_LOCAL_ADDR, ir.OP_GLOBAL_ADDR:
		return 1
	case ir.OP_LOCAL_SET, ir.OP_GLOBAL_SET:
		return -1
	case ir.OP_LOCAL_ADD_IMM:
		return 0
	case ir.OP_DROP:
		return -1
	case ir.OP_DUP:
		return 1
	case ir.OP_ADD, ir.OP_SUB, ir.OP_MUL, ir.OP_DIV, ir.OP_MOD:
		return -1
	case ir.OP_AND, ir.OP_OR, ir.OP_XOR, ir.OP_SHL, ir.OP_SHR:
		return -1
	case ir.OP_EQ, ir.OP_NEQ, ir.OP_LT, ir.OP_GT, ir.OP_LEQ, ir.OP_GEQ:
		return -1
	case ir.OP_JMP_EQ, ir.OP_JMP_NEQ, ir.OP_JMP_LT, ir.OP_JMP_GT, ir.OP_JMP_LEQ, ir.OP_JMP_GEQ:
		return -2
	case ir.OP_NEG, ir.OP_NOT:
		return 0
	case ir.OP_LOAD:
		return 0 // pop addr, push value
	case ir.OP_STORE:
		return -2 // pop addr + value
	case ir.OP_OFFSET:
		return 0
	case ir.OP_LABEL, ir.OP_JMP:
		return 0
	case ir.OP_JMP_IF, ir.OP_JMP_IF_NOT:
		return -1
	case ir.OP_CALL:
		retCount := 0
		if len(inst.Name) > 18 && inst.Name[0:18] == "builtin.composite." {
			retCount = 1 // composite literal calls consume N fields, produce 1 pointer
		} else if n, ok := c.funcRets[inst.Name]; ok {
			retCount = n
		}
		return -inst.Arg + retCount
	case ir.OP_CALL_INTRINSIC:
		// Intrinsics read params from frame, only push results
		if c.curFunc != nil {
			return c.curFunc.RetCount
		}
		return 0
	case ir.OP_RETURN:
		return -inst.Arg
	case ir.OP_INDEX_ADDR:
		return -1 // pop base + index, push addr
	case ir.OP_LEN:
		return 0 // pop header, push len
	case ir.OP_CAP:
		return 0 // pop header, push cap
	case ir.OP_CONVERT:
		return 0
	case ir.OP_IFACE_BOX:
		return 0 // pop value, push boxed
	case ir.OP_IFACE_CALL:
		// consumes receiver + args, produces 1 result
		return -(inst.Arg + 1) + 1
	case ir.OP_PANIC:
		return -1
	case ir.OP_SLICE_GET, ir.OP_SLICE_MAKE, ir.OP_STRING_GET, ir.OP_STRING_MAKE:
		return 0
	}
	panic("ICE: unknown opcode in instStackDelta")
}

func (c *Compiler) blockEndsWithReturn() bool {
	if c.curFunc == nil || len(c.curFunc.Code) == 0 {
		return false
	}
	last := c.curFunc.Code[len(c.curFunc.Code)-1]
	return last.Op == ir.OP_RETURN || last.Op == ir.OP_PANIC
}

func (c *Compiler) emitLabel(label int) {
	c.emit(ir.Inst{Op: ir.OP_LABEL, Arg: label})
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
	stmtLabels := c.pendingStmtLabels
	if node.Kind != NBranch || node.Name != "label" {
		c.pendingStmtLabels = nil
	}
	switch node.Kind {
	case NVarDecl:
		if len(node.Nodes) > 0 {
			for _, child := range node.Nodes {
				c.compileVarDecl(child)
			}
		} else {
			c.compileVarDecl(node)
		}
	case NAssign:
		c.compileAssign(node)
	case NReturn:
		c.compileReturn(node)
	case NIf:
		c.compileIf(node)
	case NFor:
		c.compileFor(node, stmtLabels)
	case NSwitch:
		c.compileSwitch(node, stmtLabels)
	case NExprStmt:
		c.compileExpr(node.X)
		// Drop return values left on the operand stack
		retCount := c.exprReturnCount(node.X)
		i := 0
		for i < retCount {
			c.emit(ir.Inst{Op: ir.OP_DROP})
			i++
		}
	case NIncStmt:
		c.compileInc(node)
	case NDecStmt:
		c.compileDec(node)
	case NBranch:
		if node.Name == "label" && node.X != nil && node.X.Kind == NIdent {
			c.pendingStmtLabels = append(c.pendingStmtLabels, node.X.Name)
		}
		c.compileBranch(node)
	case NDeferStmt:
		if node.X != nil && node.X.Kind == NCallExpr {
			name := c.resolveCallName(node.X.X)
			fixedCount, isVariadic := c.funcVariadic[name]
			isIfaceVar := isVariadic && c.funcVariadicIface[name]
			argStart := -1
			argCount := 0
			for _, arg := range node.X.Nodes {
				c.compileExpr(arg)
				if isIfaceVar && argCount >= fixedCount {
					if typeID := c.exprPrimitiveTypeID(arg); typeID > 0 {
						c.emit(ir.Inst{Op: ir.OP_IFACE_BOX, Arg: typeID})
					}
				}
				idx := c.addLocal(fmt.Sprintf("_defer_%d_%d", len(c.deferNames), argCount))
				c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
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
			retCount := 0
			if n, ok := c.funcRets[name]; ok {
				retCount = n
			}
			c.deferRetCounts = append(c.deferRetCounts, retCount)
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
	case NTypeDecl:
		// Local type declaration
		c.registerLocalTypeDecl(node)
	case NBlock:
		c.compileBlock(node)
	default:
		panic("ICE: unhandled statement kind in compileStmt")
	}
}

func (c *Compiler) assignStackValuesToLHS(lhsNodes []*Node, define bool) {
	i := len(lhsNodes) - 1
	for i >= 0 {
		lhs := lhsNodes[i]
		if define {
			idx := c.addLocal(lhs.Name)
			c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
		} else {
			c.compileLValueSet(lhs)
		}
		i = i - 1
	}
}

func (c *Compiler) compileCompoundAssign(node *Node, op ir.Opcode) {
	w := c.exprWidth(node.X)
	c.compileLValueGet(node.X)
	c.compileExpr(node.Y)
	c.emit(ir.Inst{Op: op, Width: w})
	c.compileLValueSet(node.X)
}

func (c *Compiler) setLocalMapMetadata(name string, keyType string, valType string) {
	if keyType == "string" {
		c.localMapVars[name] = 1
	} else {
		c.localMapVars[name] = 0
	}
	c.localMapValueTypes[name] = valType
}

func (c *Compiler) setLocalMapMetadataFromQualified(name string, qtype string) {
	if keyType, valType, ok := parseMapTypeName(qtype); ok {
		c.setLocalMapMetadata(name, keyType, valType)
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
		// Track concrete type for struct field access and method resolution
		ct := c.qualifyTypeName(typeName, "")
		if c.isInterfaceTypeName(typeName) || c.isInterfaceTypeName(ct) {
			c.localTypes[node.Name] = ct
		}
		c.localConcreteTypes[node.Name] = ct
	}
	if node.X != nil {
		if c.registerFuncValueBinding(node.Name, node.X) {
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx, Width: c.curFunc.Locals[idx].Width})
			return
		}
		// Lower function literals to generated package-scope functions.
		if node.X.Kind == NFuncType && node.X.Body != nil {
			target := c.compileFuncLiteral(node.X)
			c.localFuncTargets[node.Name] = target
			c.bindFuncCaptures(node.Name, target)
			delete(c.localMethodTargets, node.Name)
			delete(c.localMethodRecv, node.Name)
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx, Width: c.curFunc.Locals[idx].Width})
			return
		}
		c.compileExpr(node.X)
		if node.Type != nil {
			if c.isInterfaceTypeName(nodeTypeName(node.Type)) {
				c.maybeBoxValueForInterface(node.X)
			}
		}
		c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx, Width: c.curFunc.Locals[idx].Width})
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
				size := slots * c.target.PtrSize
				if size <= 0 {
					size = c.target.PtrSize
				}
				c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(size)})
				c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.Alloc", Arg: 1})
				c.emit(ir.Inst{Op: ir.OP_DUP})
				c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(size)})
				c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.Memzero", Arg: 2})
				c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
				return
			}
		}
		// Zero-initialize the local to avoid stack garbage
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
	}
}

func (c *Compiler) compileAssign(node *Node) {
	if len(node.Nodes) > 0 {
		isDefine := node.Name == ":="
		// Multi-value assignment with comma-separated RHS: a, b := 1, 2
		if node.Body != nil && node.Body.Kind == NBlock && len(node.Body.Nodes) > 0 {
			for _, rhs := range node.Body.Nodes {
				c.compileExpr(rhs)
			}
			c.assignStackValuesToLHS(node.Nodes, isDefine)
			return
		}

		// Multi-value map index: v, ok := m[key]
		if node.Y != nil && node.Y.Kind == NIndexExpr && c.isMapExpr(node.Y.X) {
			c.compileExpr(node.Y.X) // push map
			c.compileExpr(node.Y.Y) // push key
			c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.MapGet", Arg: 2})
			// MapGet returns (value, ok) — both on stack
			// Assign in reverse order: ok first (top of stack), then value
			c.assignStackValuesToLHS(node.Nodes, isDefine)
			// Track concrete type of the map value variable (node.Nodes[0])
			if isDefine && len(node.Nodes) >= 1 {
				valType := c.resolveMapValueType(node.Y.X)
				if valType != "" {
					c.localConcreteTypes[node.Nodes[0].Name] = c.qualifyTypeName(valType, "")
				}
			}
			return
		}

		// Multi-value type assertion: v, ok := x.(T)
		if node.Y != nil && node.Y.Kind == NTypeAssertExpr && len(node.Nodes) == 2 {
			c.compileTypeAssertCommaOk(node.Y)
			c.assignStackValuesToLHS(node.Nodes, isDefine)
			return
		}

		// Multi-value assignment: a, b = expr or a, b := expr
		c.compileExpr(node.Y)

		// Track interface-typed, string-typed, and concrete-typed locals from multi-value := assignments
		if isDefine && node.Y != nil && node.Y.Kind == NCallExpr {
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
						if c.isInterfaceTypeName(retTypes[j]) || c.isInterfaceTypeName(qret) {
							c.localTypes[lhs.Name] = qret
						}
						if retTypes[j] == "string" {
							c.localStringVars[lhs.Name] = true
						}
						if len(retTypes[j]) > 2 && retTypes[j][0] == '[' && retTypes[j][1] == ']' {
							elemType := qret[2:len(qret)]
							c.localElemSizes[lhs.Name] = c.typeElemSize(elemType)
						}
						c.setLocalMapMetadataFromQualified(lhs.Name, qret)
						// Track concrete type for method resolution
						c.localConcreteTypes[lhs.Name] = qret
					}
				}
			}
		}

		// Assign to each LHS in reverse order (values are on stack)
		c.assignStackValuesToLHS(node.Nodes, isDefine)
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
		// Track address-of locals for selector auto-deref.
		// Skip struct-handle sources because &structLocal preserves the handle.
		if node.Y != nil && node.Y.Kind == NUnaryExpr && node.Y.Name == "&" && node.Y.X != nil && node.Y.X.Kind == NIdent {
			baseName := node.Y.X.Name
			needsAutoDeref := true
			if ct, ok := c.localConcreteTypes[baseName]; ok {
				if typeNode, _ := c.lookupStructTypeNode(ct); typeNode != nil {
					needsAutoDeref = false
				}
			} else {
				gqname := c.curPkg.QualName(baseName)
				if ct, ok := c.globalConcreteTypes[gqname]; ok {
					if typeNode, _ := c.lookupStructTypeNode(ct); typeNode != nil {
						needsAutoDeref = false
					}
				}
			}
			if needsAutoDeref {
				c.localAddrOf[node.X.Name] = true
			}
		}
		// Track concrete type and elem size for method resolution and indexing
		if ct := c.exprConcreteType(node.Y); ct != "" {
			c.localConcreteTypes[node.X.Name] = ct
			if c.isInterfaceTypeName(ct) {
				c.localTypes[node.X.Name] = ct
			}
			// Track slice elem sizes
			if len(ct) > 2 && ct[0] == '[' && ct[1] == ']' {
				c.localElemSizes[node.X.Name] = c.typeElemSize(ct[2:len(ct)])
			}
			// Track map variables from concrete return type
			c.setLocalMapMetadataFromQualified(node.X.Name, ct)
		}
		if node.Y != nil {
			if node.Y.Kind == NIntLit || node.Y.Kind == NRuneLit {
				c.localConcreteTypes[node.X.Name] = "int"
			} else if node.Y.Kind == NStringLit {
				c.localConcreteTypes[node.X.Name] = "string"
			} else if node.Y.Kind == NBasicLit && (node.Y.Name == "true" || node.Y.Name == "false") {
				c.localConcreteTypes[node.X.Name] = "bool"
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
				if node.Y.Nodes[0].X != nil {
					elemType := c.qualifyTypeName(nodeTypeName(node.Y.Nodes[0].X), "")
					c.localConcreteTypes[node.X.Name] = "[]" + elemType
				}
			}
			if len(node.Y.Nodes) > 0 && node.Y.Nodes[0].Kind == NMapType {
				keyType := nodeTypeName(node.Y.Nodes[0].X)
				c.localMapVars[node.X.Name] = c.mapKeyKind(node.Y.Nodes[0].X)
				if node.Y.Nodes[0].Y != nil {
					valType := nodeTypeName(node.Y.Nodes[0].Y)
					c.localMapValueTypes[node.X.Name] = valType
					c.localConcreteTypes[node.X.Name] = "map[" + c.qualifyTypeName(keyType, "") + "]" + c.qualifyTypeName(valType, "")
				}
			}
		}
		// Function literals are lowered to private named functions and bound
		// through localFuncTargets; the local slot stores a placeholder value.
		if c.registerMethodValueBinding(node.X.Name, node.Y, idx) {
			return
		}
		if c.registerFuncValueBinding(node.X.Name, node.Y) {
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx, Width: w})
			return
		}
		if node.Y != nil && node.Y.Kind == NFuncType && node.Y.Body != nil {
			target := c.compileFuncLiteral(node.Y)
			c.localFuncTargets[node.X.Name] = target
			c.bindFuncCaptures(node.X.Name, target)
			delete(c.localMethodTargets, node.X.Name)
			delete(c.localMethodRecv, node.X.Name)
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx, Width: w})
			return
		}
		c.compileExpr(node.Y)
		c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx, Width: w})
		return
	}

	switch node.Name {
	case "+=":
		c.compileCompoundAssign(node, ir.OP_ADD)
		return
	case "-=":
		c.compileCompoundAssign(node, ir.OP_SUB)
		return
	case "*=":
		c.compileCompoundAssign(node, ir.OP_MUL)
		return
	case "/=":
		c.compileCompoundAssign(node, ir.OP_DIV)
		return
	case "%=":
		c.compileCompoundAssign(node, ir.OP_MOD)
		return
	case "|=":
		c.compileCompoundAssign(node, ir.OP_OR)
		return
	case "&=":
		c.compileCompoundAssign(node, ir.OP_AND)
		return
	case "^=":
		c.compileCompoundAssign(node, ir.OP_XOR)
		return
	case "<<=":
		c.compileCompoundAssign(node, ir.OP_SHL)
		return
	case ">>=":
		c.compileCompoundAssign(node, ir.OP_SHR)
		return
	}

	// Map index assignment: m[key] = val
	if node.X != nil && node.X.Kind == NIndexExpr && c.isMapExpr(node.X.X) {
		c.compileExpr(node.X.X) // push map
		c.compileExpr(node.X.Y) // push key
		c.compileExpr(node.Y)   // push value
		c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.MapSet", Arg: 3})
		c.emit(ir.Inst{Op: ir.OP_DROP}) // discard returned header (unchanged)
		return
	}

	// Regular assignment
	c.compileExpr(node.Y)
	if _, ok := c.lvalueInterfaceType(node.X); ok {
		c.maybeBoxValueForInterface(node.Y)
	}
	c.compileLValueSet(node.X)
}

func trimVariadicName(name string) string {
	if len(name) > 3 && name[0:3] == "..." {
		return name[3:]
	}
	return name
}

func localWidth(locals []ir.IRLocal, idx int) int {
	if idx >= 0 && idx < len(locals) {
		return locals[idx].Width
	}
	return 0
}

func (c *Compiler) bindFuncCaptures(localName string, target string) {
	captures, ok := c.funcLiteralCaptures[target]
	if !ok || len(captures) == 0 {
		delete(c.localFuncCaptures, localName)
		return
	}
	bindings := make([]closureCaptureBinding, 0, len(captures))
	for _, capture := range captures {
		if idx, found := c.lookupLocal(capture.Name); found {
			bindings = append(bindings, closureCaptureBinding{LocalIdx: idx, Width: capture.Width})
			continue
		}
		if parentCapture, found := c.activeCaptures[capture.Name]; found {
			bindings = append(bindings, closureCaptureBinding{LocalIdx: parentCapture.LocalIdx, Width: capture.Width, IsPtr: true})
		}
	}
	c.localFuncCaptures[localName] = bindings
}

func (c *Compiler) collectFuncLiteralCaptures(lit *Node) []closureCaptureSpec {
	_ = lit
	var captures []closureCaptureSpec
	seen := make(map[string]bool)
	for i := len(c.scopes) - 1; i >= 0; i-- {
		var names []string
		for name := range c.scopes[i] {
			if name != "_" && !seen[name] {
				names = append(names, name)
			}
		}
		sortStrings(names)
		for _, name := range names {
			idx, ok := c.lookupLocal(name)
			if !ok {
				continue
			}
			captures = append(captures, closureCaptureSpec{Name: name, Width: localWidth(c.curFunc.Locals, idx)})
			seen[name] = true
		}
	}
	return captures
}

func (c *Compiler) compileFuncLiteral(lit *Node) string {
	name := fmt.Sprintf("$lit_%d", c.funcLitSeq)
	c.funcLitSeq++

	captures := c.collectFuncLiteralCaptures(lit)
	params := make([]*Node, 0, len(captures)+len(lit.Nodes))
	activeCaptures := make(map[string]closureCaptureBinding)
	for i, capture := range captures {
		pname := "$cap_" + capture.Name
		params = append(params, &Node{Kind: NVarDecl, Name: pname})
		activeCaptures[capture.Name] = closureCaptureBinding{LocalIdx: i, Width: capture.Width, IsPtr: true}
	}
	params = append(params, lit.Nodes...)

	fn := &Node{
		Kind: NFunc, Name: name, Pos: lit.Pos,
		Nodes: params, Type: lit.Type, Body: lit.Body,
	}
	// Save current function compilation state.
	savedCurFunc := c.curFunc
	savedScopes := c.scopes
	savedLocalElem := c.localElemSizes
	savedLocalTypes := c.localTypes
	savedLocalTypeDecls := c.localTypeDecls
	savedLocalStrings := c.localStringVars
	savedLocalAddr := c.localAddrOf
	savedLocalConcrete := c.localConcreteTypes
	savedLocalMapVars := c.localMapVars
	savedLocalMapVals := c.localMapValueTypes
	savedDefNames := c.deferNames
	savedDefStarts := c.deferArgStarts
	savedDefCounts := c.deferArgCounts
	savedDefRetCounts := c.deferRetCounts
	savedNamed := c.namedResultNames
	savedPendingStmtLabels := c.pendingStmtLabels
	savedLabelIDs := c.labelIDs
	savedBreakLabelTargets := c.breakLabelTargets
	savedContinueLabelTargets := c.continueLabelTargets
	savedStackDepth := c.stackDepth
	savedFuncTargets := c.localFuncTargets
	savedMethodTargets := c.localMethodTargets
	savedMethodRecv := c.localMethodRecv
	savedLocalFuncCaptures := c.localFuncCaptures
	savedActiveCaptures := c.activeCaptures

	c.activeCaptures = activeCaptures
	c.compileFunc(fn)

	// Restore caller function state.
	c.curFunc = savedCurFunc
	c.scopes = savedScopes
	c.localElemSizes = savedLocalElem
	c.localTypes = savedLocalTypes
	c.localTypeDecls = savedLocalTypeDecls
	c.localStringVars = savedLocalStrings
	c.localAddrOf = savedLocalAddr
	c.localConcreteTypes = savedLocalConcrete
	c.localMapVars = savedLocalMapVars
	c.localMapValueTypes = savedLocalMapVals
	c.deferNames = savedDefNames
	c.deferArgStarts = savedDefStarts
	c.deferArgCounts = savedDefCounts
	c.deferRetCounts = savedDefRetCounts
	c.namedResultNames = savedNamed
	c.pendingStmtLabels = savedPendingStmtLabels
	c.labelIDs = savedLabelIDs
	c.breakLabelTargets = savedBreakLabelTargets
	c.continueLabelTargets = savedContinueLabelTargets
	c.stackDepth = savedStackDepth
	c.localFuncTargets = savedFuncTargets
	c.localMethodTargets = savedMethodTargets
	c.localMethodRecv = savedMethodRecv
	c.localFuncCaptures = savedLocalFuncCaptures
	c.activeCaptures = savedActiveCaptures
	target := c.curPkg.QualName(name)
	c.funcLiteralCaptures[target] = captures
	return target
}

func (c *Compiler) registerFuncValueBinding(localName string, rhs *Node) bool {
	if rhs == nil || localName == "" {
		return false
	}
	if rhs.Kind == NIdent {
		if target, ok := c.localFuncTargets[rhs.Name]; ok {
			c.localFuncTargets[localName] = target
			if captures, ok := c.localFuncCaptures[rhs.Name]; ok {
				c.localFuncCaptures[localName] = captures
			} else {
				delete(c.localFuncCaptures, localName)
			}
			delete(c.localMethodTargets, localName)
			delete(c.localMethodRecv, localName)
			return true
		}
		if c.curPkg != nil {
			if sym, ok := c.curPkg.Symbols[rhs.Name]; ok && sym.Kind == SymFunc {
				c.localFuncTargets[localName] = c.curPkg.QualName(rhs.Name)
				delete(c.localFuncCaptures, localName)
				delete(c.localMethodTargets, localName)
				delete(c.localMethodRecv, localName)
				return true
			}
		}
	}
	return false
}

func (c *Compiler) registerMethodValueBinding(localName string, rhs *Node, localIdx int) bool {
	if rhs == nil || localName == "" || localIdx < 0 {
		return false
	}
	if rhs.Kind != NSelectorExpr || rhs.X == nil {
		return false
	}
	// pkg.Func selectors are not method values.
	if rhs.X.Kind == NIdent && c.resolvePackage(rhs.X.Name) != nil {
		return false
	}

	methodName := rhs.Name
	concreteType := c.resolveExprType(rhs.X)
	if concreteType == "" {
		return false
	}
	target, ok := c.resolveMethodByConcreteType(concreteType, methodName)
	if !ok {
		return false
	}

	// Method values capture the receiver expression at binding time.
	c.compileExpr(rhs.X)
	c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: localIdx})

	delete(c.localFuncTargets, localName)
	delete(c.localFuncCaptures, localName)
	c.localMethodTargets[localName] = target
	c.localMethodRecv[localName] = localIdx
	return true
}

func (c *Compiler) lvalueInterfaceType(node *Node) (string, bool) {
	if node == nil {
		return "", false
	}
	if node.Kind == NIdent {
		if t, ok := c.localTypes[node.Name]; ok {
			return t, true
		}
		if c.curPkg != nil {
			if sym, ok := c.curPkg.Symbols[node.Name]; ok && sym.Kind == SymVar && sym.Node != nil && sym.Node.Type != nil {
				tname := nodeTypeName(sym.Node.Type)
				if c.isInterfaceTypeName(tname) {
					return tname, true
				}
			}
		}
	}
	return "", false
}

func (c *Compiler) compileLValueSet(node *Node) {
	if node == nil {
		return
	}
	switch node.Kind {
	case NIdent:
		if node.Name == "_" {
			c.emit(ir.Inst{Op: ir.OP_DROP})
			return
		}
		idx, ok := c.lookupLocal(node.Name)
		if ok {
			w := 0
			if idx < len(c.curFunc.Locals) {
				w = c.curFunc.Locals[idx].Width
			}
			c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx, Width: w})
		} else {
			if capture, ok := c.activeCaptures[node.Name]; ok {
				c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: capture.LocalIdx})
				c.emit(ir.Inst{Op: ir.OP_STORE, Arg: capture.Width})
				return
			}
			gidx, gok := c.lookupGlobal(node.Name)
			if gok {
				c.emit(ir.Inst{Op: ir.OP_GLOBAL_SET, Arg: gidx})
			}
		}
	case NIndexExpr:
		elemSize := c.exprElemSize(node.X)
		c.compileExpr(node.X)
		c.compileExpr(node.Y)
		c.emit(ir.Inst{Op: ir.OP_INDEX_ADDR, Arg: elemSize})
		c.emit(ir.Inst{Op: ir.OP_STORE, Arg: elemSize})
	case NSelectorExpr:
		if node.X != nil && node.X.Kind == NIdent {
			pkg := c.resolvePackage(node.X.Name)
			if pkg != nil {
				qname := pkg.QualName(node.Name)
				gidx, ok := c.globals[qname]
				if ok {
					c.emit(ir.Inst{Op: ir.OP_GLOBAL_SET, Arg: gidx})
				} else {
					c.emit(ir.Inst{Op: ir.OP_GLOBAL_SET, Name: qname})
				}
				return
			}
		}
		recvType := c.resolveExprType(node.X)
		if recvType == "" {
			c.errorf("%s: cannot resolve selector %s (unknown receiver type)", c.curFunc.Name, node.Name)
			c.emit(ir.Inst{Op: ir.OP_DROP})
			return
		}
		offset := c.resolveFieldOffset(recvType, node.Name)
		if offset < 0 {
			c.errorf("%s: cannot resolve selector %s on %s", c.curFunc.Name, node.Name, recvType)
			c.emit(ir.Inst{Op: ir.OP_DROP})
			return
		}
		c.compileExpr(node.X)
		// Auto-deref pointer-to-struct for field write (e.g., pp.X = 100)
		if node.X != nil && c.needsSelectorDeref(node.X) {
			c.emit(ir.Inst{Op: ir.OP_LOAD, Arg: c.target.PtrSize})
		}
		c.emit(ir.Inst{Op: ir.OP_OFFSET, Arg: offset})
		c.emit(ir.Inst{Op: ir.OP_STORE, Arg: c.target.PtrSize})
	case NUnaryExpr:
		if node.Name == "*" {
			c.compileExpr(node.X)
			c.emit(ir.Inst{Op: ir.OP_STORE, Arg: c.target.PtrSize})
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
		fixedCount, isVariadic := c.funcVariadic[name]
		if isVariadic {
			if fixedCount > argCount {
				fixedCount = argCount
			}
			k := 0
			for k < fixedCount {
				c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: argStart + k})
				k++
			}
			variadicCount := argCount - fixedCount
			if variadicCount < 0 {
				variadicCount = 0
			}
			varElemSz := c.target.PtrSize
			if esz, ok := c.funcVariadicElem[name]; ok {
				varElemSz = esz
			}
			sliceHdrSize := 4 * c.target.PtrSize
			allocSize := sliceHdrSize + variadicCount*varElemSz
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(allocSize)})
			c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.Alloc", Arg: 1})
			tmpIdx := c.addLocal("$defer_varslice")
			c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: tmpIdx})
			c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: tmpIdx})
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(sliceHdrSize)})
			c.emit(ir.Inst{Op: ir.OP_ADD})
			c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: tmpIdx})
			c.emit(ir.Inst{Op: ir.OP_STORE})
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(variadicCount)})
			c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: tmpIdx})
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(c.target.PtrSize)})
			c.emit(ir.Inst{Op: ir.OP_ADD})
			c.emit(ir.Inst{Op: ir.OP_STORE})
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(variadicCount)})
			c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: tmpIdx})
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(2 * c.target.PtrSize)})
			c.emit(ir.Inst{Op: ir.OP_ADD})
			c.emit(ir.Inst{Op: ir.OP_STORE})
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(varElemSz)})
			c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: tmpIdx})
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(3 * c.target.PtrSize)})
			c.emit(ir.Inst{Op: ir.OP_ADD})
			c.emit(ir.Inst{Op: ir.OP_STORE})
			j := 0
			for j < variadicCount {
				c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: argStart + fixedCount + j})
				c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: tmpIdx})
				c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(sliceHdrSize + j*varElemSz)})
				c.emit(ir.Inst{Op: ir.OP_ADD})
				c.emit(ir.Inst{Op: ir.OP_STORE, Arg: varElemSz})
				j++
			}
			c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: tmpIdx})
			c.emit(ir.Inst{Op: ir.OP_CALL, Name: name, Arg: fixedCount + 1})
		} else {
			k := 0
			for k < argCount {
				c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: argStart + k})
				k++
			}
			c.emit(ir.Inst{Op: ir.OP_CALL, Name: name, Arg: argCount})
		}
		dropCount := 0
		if idx < len(c.deferRetCounts) {
			dropCount = c.deferRetCounts[idx]
		}
		for dropCount > 0 {
			c.emit(ir.Inst{Op: ir.OP_DROP})
			dropCount--
		}
		di++
	}
}

func (c *Compiler) compileReturn(node *Node) {
	count := 0
	retTypes := c.funcRetTypes[c.curFunc.Name]
	bareReturn := node.X == nil && len(node.Nodes) == 0
	if bareReturn && len(c.namedResultNames) > 0 {
		for i, name := range c.namedResultNames {
			idx, ok := c.lookupLocal(name)
			if !ok {
				continue
			}
			w := 0
			if idx < len(c.curFunc.Locals) {
				w = c.curFunc.Locals[idx].Width
			}
			c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: idx, Width: w})
			if i < len(retTypes) && c.isInterfaceTypeName(retTypes[i]) {
				if typeID := c.typeIDForTypeName(c.localConcreteTypes[name]); typeID > 0 {
					c.emit(ir.Inst{Op: ir.OP_IFACE_BOX, Arg: typeID})
				}
			}
			count++
		}
	}

	if node.X != nil {
		retIdx := count
		c.compileExpr(node.X)
		c.maybeBoxInterface(node.X, retTypes, retIdx)
		count++
	}
	for _, extra := range node.Nodes {
		retIdx := count
		c.compileExpr(extra)
		c.maybeBoxInterface(extra, retTypes, retIdx)
		count++
	}
	if len(c.deferNames) > 0 {
		c.emitDeferredCalls()
	}
	c.emit(ir.Inst{Op: ir.OP_RETURN, Arg: count})
}

// maybeBoxInterface checks if the return value at position idx needs boxing
// (i.e., the expected return type is an interface). If so, emits OP_IFACE_BOX.
func (c *Compiler) maybeBoxInterface(expr *Node, retTypes []string, idx int) {
	if idx >= len(retTypes) {
		return
	}
	expectedType := retTypes[idx]
	if !c.isInterfaceTypeName(expectedType) && !c.isInterfaceTypeName(c.qualifyTypeName(expectedType, "")) {
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
				gotType := calleeRetTypes[idx]
				if c.isInterfaceTypeName(gotType) || c.isInterfaceTypeName(c.qualifyTypeName(gotType, "")) {
					return // callee already boxes
				}
			}
		}
	}
	if typeID := c.resolveConcreteTypeID(expr); typeID > 0 {
		c.emit(ir.Inst{Op: ir.OP_IFACE_BOX, Arg: typeID})
		return
	}
	if ct := c.exprConcreteType(expr); ct != "" {
		if c.isInterfaceTypeName(ct) {
			return
		}
		c.emit(ir.Inst{Op: ir.OP_IFACE_BOX, Arg: c.typeIDForTypeName(ct)})
		return
	}
	shouldCheckPrimitive := false
	switch expr.Kind {
	case NIntLit, NRuneLit, NStringLit, NBasicLit, NUnaryExpr, NSliceExpr:
		shouldCheckPrimitive = true
	}
	if shouldCheckPrimitive {
		if typeID := c.exprPrimitiveTypeID(expr); typeID > 0 {
			c.emit(ir.Inst{Op: ir.OP_IFACE_BOX, Arg: typeID})
		}
	}
}

// exprPrimitiveTypeID returns the type ID for boxing a primitive value as interface{}.
// Returns 1 for int, 2 for string, or the concrete type ID for named types.
// Returns 0 if the value is already an interface or doesn't need boxing.
func (c *Compiler) exprPrimitiveTypeID(expr *Node) int {
	if expr == nil {
		return 0
	}
	if typeID := c.resolveConcreteTypeID(expr); typeID > 0 {
		return typeID
	}
	switch expr.Kind {
	case NIntLit, NRuneLit:
		return 1
	case NStringLit:
		return 2
	case NBasicLit:
		if expr.Name == "true" || expr.Name == "false" {
			return 3
		}
		return 0
	case NIdent:
		if expr.Name == "true" || expr.Name == "false" {
			return 3
		}
		if expr.Name == "nil" {
			return 0
		}
	case NUnaryExpr:
		if expr.Name == "&" {
			return 0
		}
		if expr.Name == "!" || expr.Name == "-" || expr.Name == "^" {
			return 1
		}
	case NSliceExpr:
		if c.isStringTypedExpr(expr.X) {
			return 2
		}
		return 0
	}
	if c.isStringTypedExpr(expr) {
		return 2
	}
	t := c.resolveExprType(expr)
	if t == "bool" {
		return 3
	}
	if t == "string" {
		return 2
	}
	if t == "error" || t == "interface{}" {
		return 0
	}
	if len(t) > 1 && t[0] == '[' && t[1] == ']' {
		return 0
	}
	return 1
}

// resolveConcreteTypeID detects the concrete type from AST patterns and returns its type ID.
func (c *Compiler) resolveConcreteTypeID(expr *Node) int {
	if expr == nil {
		return 0
	}
	// Pattern: Errno(x) — type conversion call (unqualified)
	if expr.Kind == NCallExpr && expr.X != nil && expr.X.Kind == NIdent {
		if _, ok := c.lookupCurrentTypeDecl(expr.X.Name); ok {
			qtype := c.curPkg.QualName(expr.X.Name)
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
				qtype := impPkg.QualName(typeName)
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
		qtype := c.curPkg.QualPtrName(typeName)
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
		if expr.X != nil && expr.X.Kind == NIdent && expr.X.Name == "recover" {
			return "interface{}"
		}
		if expr.X != nil && expr.X.Kind == NIdent && expr.X.Name == "new" && len(expr.Nodes) == 1 {
			typeName := nodeTypeName(expr.Nodes[0])
			if typeName != "" {
				return c.qualifyTypeName("*"+typeName, "")
			}
		}
		if expr.X != nil && expr.X.Kind == NSliceType && expr.X.X != nil {
			return "[]" + c.qualifyTypeName(nodeTypeName(expr.X.X), "")
		}
		if expr.X != nil && expr.X.Kind == NIdent && expr.X.Name == "string" {
			return "string"
		}
		// append returns the same slice type as its first argument
		if expr.X != nil && expr.X.Kind == NIdent && expr.X.Name == "append" && len(expr.Nodes) > 0 {
			return c.exprConcreteType(expr.Nodes[0])
		}
		calleeName := c.resolveCallName(expr.X)
		if expr.X != nil && expr.X.Kind == NIdent {
			if target, ok := c.localFuncTargets[expr.X.Name]; ok {
				calleeName = target
			} else if target, ok := c.localMethodTargets[expr.X.Name]; ok {
				calleeName = target
			}
		}
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

	c.compileCondJump(node.X, false, elseLabel)

	branchDepth := c.stackDepth
	c.compileBlock(node.Body)
	thenDepth := c.stackDepth
	thenReturns := c.blockEndsWithReturn()
	c.emit(ir.Inst{Op: ir.OP_JMP, Arg: endLabel})

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

func (c *Compiler) invertCmpOp(op string) string {
	switch op {
	case "==":
		return "!="
	case "!=":
		return "=="
	case "<":
		return ">="
	case ">":
		return "<="
	case "<=":
		return ">"
	case ">=":
		return "<"
	default:
		return ""
	}
}

func (c *Compiler) emitCmpJump(op string, node *Node, targetLabel int) bool {
	var irOp ir.Opcode
	switch op {
	case "==":
		irOp = ir.OP_JMP_EQ
	case "!=":
		irOp = ir.OP_JMP_NEQ
	case "<":
		irOp = ir.OP_JMP_LT
	case ">":
		irOp = ir.OP_JMP_GT
	case "<=":
		irOp = ir.OP_JMP_LEQ
	case ">=":
		irOp = ir.OP_JMP_GEQ
	default:
		return false
	}
	c.compileExpr(node.X)
	c.compileExpr(node.Y)
	c.emit(ir.Inst{Op: irOp, Arg: targetLabel, Width: c.exprWidth(node)})
	return true
}

// compileCondJump emits control flow for a boolean condition.
// If jumpIfTrue is true, it jumps to targetLabel when cond is true.
// Otherwise it jumps when cond is false.
func (c *Compiler) compileCondJump(cond *Node, jumpIfTrue bool, targetLabel int) {
	if cond == nil {
		if !jumpIfTrue {
			c.emit(ir.Inst{Op: ir.OP_JMP, Arg: targetLabel})
		}
		return
	}

	if cond.Kind == NUnaryExpr && cond.Name == "!" {
		c.compileCondJump(cond.X, !jumpIfTrue, targetLabel)
		return
	}

	if cond.Kind == NBinaryExpr {
		switch cond.Name {
		case "&&":
			if jumpIfTrue {
				skipLabel := c.newLabel()
				c.compileCondJump(cond.X, false, skipLabel)
				c.compileCondJump(cond.Y, true, targetLabel)
				c.emitLabel(skipLabel)
			} else {
				c.compileCondJump(cond.X, false, targetLabel)
				c.compileCondJump(cond.Y, false, targetLabel)
			}
			return
		case "||":
			if jumpIfTrue {
				c.compileCondJump(cond.X, true, targetLabel)
				c.compileCondJump(cond.Y, true, targetLabel)
			} else {
				skipLabel := c.newLabel()
				c.compileCondJump(cond.X, true, skipLabel)
				c.compileCondJump(cond.Y, false, targetLabel)
				c.emitLabel(skipLabel)
			}
			return
		case "==", "!=", "<", ">", "<=", ">=":
			if (cond.Name == "==" || cond.Name == "!=") && (c.isStringTypedExpr(cond.X) || c.isStringTypedExpr(cond.Y) || isStringExpr(cond.X) || isStringExpr(cond.Y)) {
				c.emitStringEqualCall(cond.X, cond.Y)
				if (cond.Name == "==" && jumpIfTrue) || (cond.Name == "!=" && !jumpIfTrue) {
					c.emit(ir.Inst{Op: ir.OP_JMP_IF, Arg: targetLabel})
				} else {
					c.emit(ir.Inst{Op: ir.OP_JMP_IF_NOT, Arg: targetLabel})
				}
				return
			}
			cmpOp := cond.Name
			if !jumpIfTrue {
				inv := c.invertCmpOp(cmpOp)
				if inv != "" {
					cmpOp = inv
				}
			}
			if c.emitCmpJump(cmpOp, cond, targetLabel) {
				return
			}
		}
	}

	c.compileExpr(cond)
	if jumpIfTrue {
		c.emit(ir.Inst{Op: ir.OP_JMP_IF, Arg: targetLabel})
	} else {
		c.emit(ir.Inst{Op: ir.OP_JMP_IF_NOT, Arg: targetLabel})
	}
}

func (c *Compiler) compileFor(node *Node, stmtLabels []string) {
	savedDepth := c.stackDepth
	loopLabel := c.newLabel()
	continueLabel := c.newLabel()
	breakLabel := c.newLabel()

	c.breaks = append(c.breaks, breakLabel)
	c.continues = append(c.continues, continueLabel)
	for _, name := range stmtLabels {
		c.breakLabelTargets[name] = append(c.breakLabelTargets[name], breakLabel)
		c.continueLabelTargets[name] = append(c.continueLabelTargets[name], continueLabel)
	}

	if node.Name == "range" {
		c.compileForRange(node, loopLabel, continueLabel, breakLabel)
	} else if node.X != nil || node.Type != nil {
		// 3-clause for (init and/or post)
		c.pushScope()
		if node.X != nil {
			c.compileStmt(node.X)
		}
		c.emitLabel(loopLabel)
		if node.Y != nil {
			c.compileCondJump(node.Y, false, breakLabel)
		}
		if node.Body != nil {
			c.compileBlock(node.Body)
		}
		c.emitLabel(continueLabel)
		if node.Type != nil {
			c.compileStmt(node.Type)
		}
		c.emit(ir.Inst{Op: ir.OP_JMP, Arg: loopLabel})
		c.emitLabel(breakLabel)
		c.popScope()
	} else if node.Y != nil {
		// Condition-only for loop
		c.emitLabel(loopLabel)
		c.compileCondJump(node.Y, false, breakLabel)
		if node.Body != nil {
			c.compileBlock(node.Body)
		}
		c.emitLabel(continueLabel)
		c.emit(ir.Inst{Op: ir.OP_JMP, Arg: loopLabel})
		c.emitLabel(breakLabel)
	} else {
		// Bare for loop (infinite)
		c.emitLabel(loopLabel)
		if node.Body != nil {
			c.compileBlock(node.Body)
		}
		c.emitLabel(continueLabel)
		c.emit(ir.Inst{Op: ir.OP_JMP, Arg: loopLabel})
		c.emitLabel(breakLabel)
	}

	c.breaks = c.breaks[0 : len(c.breaks)-1]
	c.continues = c.continues[0 : len(c.continues)-1]
	for _, name := range stmtLabels {
		bt := c.breakLabelTargets[name]
		c.breakLabelTargets[name] = bt[0 : len(bt)-1]
		ct := c.continueLabelTargets[name]
		c.continueLabelTargets[name] = ct[0 : len(ct)-1]
	}
	c.stackDepth = savedDepth // for loops should have net-zero effect
}

func (c *Compiler) compileForRange(node *Node, loopLabel int, continueLabel int, breakLabel int) {
	c.pushScope()

	isMap := c.isMapExpr(node.Type)
	isString := !isMap && c.isStringTypedExpr(node.Type)

	// Compile the iterable and store it
	c.compileExpr(node.Type)
	iterIdx := c.addLocal("$iter")
	c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: iterIdx})

	// Initialize index to 0
	idxIdx := c.addLocal("$idx")
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
	c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idxIdx})

	c.emitLabel(loopLabel)

	// Compare index < len(iterable)
	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: idxIdx})
	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: iterIdx})
	if isMap {
		c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.MapLen", Arg: 1})
	} else {
		c.emit(ir.Inst{Op: ir.OP_LEN})
	}
	c.emit(ir.Inst{Op: ir.OP_JMP_GEQ, Arg: breakLabel})

	// Bind loop variables
	if node.X != nil {
		keyIdx := c.addLocal(node.X.Name)
		if isMap {
			// For maps, key = MapEntryKey(hdr, idx)
			c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: iterIdx})
			c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: idxIdx})
			c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.MapEntryKey", Arg: 2})
			// Track string-typed key vars for interface boxing
			if c.mapExprKeyKind(node.Type) == 1 {
				c.localStringVars[node.X.Name] = true
			}
		} else {
			c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: idxIdx})
		}
		c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: keyIdx})
	}
	if isString {
		sizeIdx := c.addLocal("$rsize")
		runeIdx := c.addLocal("$rrune")
		c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: iterIdx})
		c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: idxIdx})
		c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.StringDecodeRune", Arg: 2})
		c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: sizeIdx})
		c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: runeIdx})
		if node.Y != nil {
			valIdx := c.addLocal(node.Y.Name)
			c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: runeIdx})
			c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: valIdx})
			c.localConcreteTypes[node.Y.Name] = "int"
		}
		if node.Body != nil {
			c.compileBlock(node.Body)
		}
		c.emitLabel(continueLabel)
		c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: idxIdx})
		c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: sizeIdx})
		c.emit(ir.Inst{Op: ir.OP_ADD})
		c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idxIdx})
		c.emit(ir.Inst{Op: ir.OP_JMP, Arg: loopLabel})
		c.emitLabel(breakLabel)
		c.popScope()
		return
	}
	if node.Y != nil {
		valIdx := c.addLocal(node.Y.Name)
		c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: iterIdx})
		c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: idxIdx})
		if isMap {
			// For maps, value = MapEntryValue(hdr, idx)
			c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.MapEntryValue", Arg: 2})
		} else {
			elemSize := c.exprElemSize(node.Type)
			c.emit(ir.Inst{Op: ir.OP_INDEX_ADDR, Arg: elemSize})
			c.emit(ir.Inst{Op: ir.OP_LOAD, Arg: elemSize})
		}
		c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: valIdx})
		// Track value type from collection element type for method resolution
		if isMap && node.Type != nil {
			valType := c.resolveMapValueType(node.Type)
			if valType != "" {
				qvalType := c.qualifyTypeName(valType, "")
				c.localConcreteTypes[node.Y.Name] = qvalType
				if valType == "string" {
					c.localStringVars[node.Y.Name] = true
				}
				// Range values can themselves be maps (map[K]V); preserve that
				// metadata so downstream indexing compiles as map access.
				c.setLocalMapMetadataFromQualified(node.Y.Name, qvalType)
			}
		}
		if !isMap && node.Type != nil {
			elemType := ""
			if node.Type.Kind == NIdent {
				collType := c.localConcreteTypes[node.Type.Name]
				if collType == "" {
					gqname := c.curPkg.QualName(node.Type.Name)
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
	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: idxIdx})
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 1})
	c.emit(ir.Inst{Op: ir.OP_ADD})
	c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idxIdx})
	c.emit(ir.Inst{Op: ir.OP_JMP, Arg: loopLabel})
	c.emitLabel(breakLabel)
	c.popScope()
}

func (c *Compiler) compileSwitch(node *Node, stmtLabels []string) {
	savedDepth := c.stackDepth
	endLabel := c.newLabel()
	c.breaks = append(c.breaks, endLabel)
	for _, name := range stmtLabels {
		c.breakLabelTargets[name] = append(c.breakLabelTargets[name], endLabel)
	}
	isTypeSwitch := node.Name == "typeswitch"
	needsScope := node.X != nil
	if needsScope {
		c.pushScope()
		c.compileStmt(node.X)
	}

	// Compile tag if present
	hasTag := node.Y != nil
	if hasTag {
		if isTypeSwitch {
			c.compileExpr(node.Y)
			c.emit(ir.Inst{Op: ir.OP_OFFSET, Arg: 0})
			c.emit(ir.Inst{Op: ir.OP_LOAD, Arg: c.target.PtrSize})
		} else {
			c.compileExpr(node.Y)
		}
	}
	caseCheckDepth := c.stackDepth // depth with tag on stack (if any)
	caseCount := len(node.Nodes)
	caseBodyLabels := make([]int, caseCount)
	caseExecLabels := make([]int, caseCount)
	caseNextLabels := make([]int, caseCount)
	for i := 0; i < caseCount; i++ {
		caseBodyLabels[i] = c.newLabel()
		caseExecLabels[i] = c.newLabel()
		caseNextLabels[i] = c.newLabel()
	}

	// Detect string switch: check tag and first case value
	isStringSwitch := false
	if hasTag && !isTypeSwitch {
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

	for i, cas := range node.Nodes {
		bodyLabel := caseBodyLabels[i]
		execLabel := caseExecLabels[i]
		nextLabel := caseNextLabels[i]
		fallthroughLabel := endLabel
		if i+1 < caseCount {
			fallthroughLabel = caseExecLabels[i+1]
		}

		if cas.Name == "default" {
			c.emitLabel(bodyLabel)
			if hasTag {
				c.emit(ir.Inst{Op: ir.OP_DROP})
			}
			c.emitLabel(execLabel)
			c.fallthroughs = append(c.fallthroughs, fallthroughLabel)
			if cas.Body != nil {
				c.compileBlock(cas.Body)
			}
			c.fallthroughs = c.fallthroughs[0 : len(c.fallthroughs)-1]
			c.emit(ir.Inst{Op: ir.OP_JMP, Arg: endLabel})
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
					c.emit(ir.Inst{Op: ir.OP_DUP})
					if isTypeSwitch {
						c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(c.typeIDForTypeNode(expr))})
					} else {
						c.compileExpr(expr)
						if isStringSwitch {
							c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.StringEqual", Arg: 2})
							c.emit(ir.Inst{Op: ir.OP_JMP_IF, Arg: bodyLabel})
							continue
						}
					}
					c.emit(ir.Inst{Op: ir.OP_JMP_EQ, Arg: bodyLabel})
				}
			} else {
				// No tag — each case expr is a bool condition, OR them
				for _, expr := range caseExprs {
					c.compileCondJump(expr, true, bodyLabel)
				}
			}
			c.emit(ir.Inst{Op: ir.OP_JMP, Arg: nextLabel})

			// Body is reached from JMP_IF; depth = caseCheckDepth
			c.stackDepth = caseCheckDepth
			c.emitLabel(bodyLabel)
			if hasTag {
				c.emit(ir.Inst{Op: ir.OP_DROP})
			}
			c.emitLabel(execLabel)
			c.fallthroughs = append(c.fallthroughs, fallthroughLabel)
			if cas.Body != nil {
				c.compileBlock(cas.Body)
			}
			c.fallthroughs = c.fallthroughs[0 : len(c.fallthroughs)-1]
			c.emit(ir.Inst{Op: ir.OP_JMP, Arg: endLabel})
			// Reset depth for next case's check path
			c.stackDepth = caseCheckDepth
			c.emitLabel(nextLabel)
		}
	}

	if hasTag {
		c.emit(ir.Inst{Op: ir.OP_DROP})
	}
	c.emitLabel(endLabel)
	if needsScope {
		c.popScope()
	}
	c.breaks = c.breaks[0 : len(c.breaks)-1]
	for _, name := range stmtLabels {
		bt := c.breakLabelTargets[name]
		c.breakLabelTargets[name] = bt[0 : len(bt)-1]
	}
	c.stackDepth = savedDepth // switch should have net-zero effect
}

func (c *Compiler) typeIDForTypeName(tname string) int {
	if tname == "" {
		return 0
	}
	if tname == "int" {
		return 1
	}
	if tname == "string" {
		return 2
	}
	if tname == "bool" {
		return 3
	}
	qt := c.qualifyTypeName(tname, "")
	if id, ok := c.typeIDs[qt]; ok {
		return id
	}
	id := c.nextTypeID
	c.nextTypeID++
	c.typeIDs[qt] = id
	return id
}

func (c *Compiler) typeIDForTypeNode(t *Node) int {
	if t == nil {
		return 0
	}
	return c.typeIDForTypeName(nodeTypeName(t))
}

func (c *Compiler) isInterfaceTypeName(typeName string) bool {
	if typeName == "" {
		return false
	}
	if typeName == "interface{}" {
		return true
	}
	_, ok := c.ifaceMethods[typeName]
	return ok
}

func (c *Compiler) maybeBoxValueForInterface(expr *Node) {
	if expr == nil {
		return
	}
	// Already an interface-typed expression/value.
	if ct := c.exprConcreteType(expr); c.isInterfaceTypeName(ct) {
		return
	}
	if typeID := c.resolveConcreteTypeID(expr); typeID > 0 {
		c.emit(ir.Inst{Op: ir.OP_IFACE_BOX, Arg: typeID})
		return
	}
	if ct := c.exprConcreteType(expr); ct != "" {
		c.emit(ir.Inst{Op: ir.OP_IFACE_BOX, Arg: c.typeIDForTypeName(ct)})
		return
	}
	if typeID := c.exprPrimitiveTypeID(expr); typeID > 0 {
		c.emit(ir.Inst{Op: ir.OP_IFACE_BOX, Arg: typeID})
		return
	}
}

func (c *Compiler) compileTypeAssert(node *Node, commaOk bool) {
	if node == nil || node.X == nil || node.Type == nil {
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		if commaOk {
			c.emit(ir.Inst{Op: ir.OP_CONST_BOOL, Arg: 0})
		}
		return
	}
	typeID := c.typeIDForTypeNode(node.Type)

	c.compileExpr(node.X)
	c.emit(ir.Inst{Op: ir.OP_DUP})
	c.emit(ir.Inst{Op: ir.OP_OFFSET, Arg: 0})
	c.emit(ir.Inst{Op: ir.OP_LOAD, Arg: c.target.PtrSize})
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(typeID)})

	failLabel := c.newLabel()
	endLabel := c.newLabel()
	c.emit(ir.Inst{Op: ir.OP_JMP_NEQ, Arg: failLabel})
	c.emit(ir.Inst{Op: ir.OP_OFFSET, Arg: c.target.PtrSize})
	c.emit(ir.Inst{Op: ir.OP_LOAD, Arg: c.target.PtrSize})
	if commaOk {
		c.emit(ir.Inst{Op: ir.OP_CONST_BOOL, Arg: 1})
	}
	c.emit(ir.Inst{Op: ir.OP_JMP, Arg: endLabel})

	c.emitLabel(failLabel)
	c.emit(ir.Inst{Op: ir.OP_DROP})
	if commaOk {
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		c.emit(ir.Inst{Op: ir.OP_CONST_BOOL, Arg: 0})
	} else {
		c.emit(ir.Inst{Op: ir.OP_CONST_STR, Name: "type assertion failed"})
		c.emit(ir.Inst{Op: ir.OP_PANIC})
	}
	c.emitLabel(endLabel)
}

func (c *Compiler) compileTypeAssertExpr(node *Node) {
	c.compileTypeAssert(node, false)
}

func (c *Compiler) compileTypeAssertCommaOk(node *Node) {
	c.compileTypeAssert(node, true)
}

func (c *Compiler) compileInc(node *Node) {
	c.compileLValueGet(node.X)
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 1})
	c.emit(ir.Inst{Op: ir.OP_ADD})
	c.compileLValueSet(node.X)
}

func (c *Compiler) compileDec(node *Node) {
	c.compileLValueGet(node.X)
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 1})
	c.emit(ir.Inst{Op: ir.OP_SUB})
	c.compileLValueSet(node.X)
}

func (c *Compiler) compileBranch(node *Node) {
	switch node.Name {
	case "break":
		if node.X != nil && node.X.Kind == NIdent {
			targets := c.breakLabelTargets[node.X.Name]
			if len(targets) > 0 {
				c.emit(ir.Inst{Op: ir.OP_JMP, Arg: targets[len(targets)-1]})
			}
		} else if len(c.breaks) > 0 {
			c.emit(ir.Inst{Op: ir.OP_JMP, Arg: c.breaks[len(c.breaks)-1]})
		}
	case "continue":
		if node.X != nil && node.X.Kind == NIdent {
			targets := c.continueLabelTargets[node.X.Name]
			if len(targets) > 0 {
				c.emit(ir.Inst{Op: ir.OP_JMP, Arg: targets[len(targets)-1]})
			}
		} else if len(c.continues) > 0 {
			c.emit(ir.Inst{Op: ir.OP_JMP, Arg: c.continues[len(c.continues)-1]})
		}
	case "fallthrough":
		if len(c.fallthroughs) > 0 {
			c.emit(ir.Inst{Op: ir.OP_JMP, Arg: c.fallthroughs[len(c.fallthroughs)-1]})
		}
	case "goto":
		if node.X == nil || node.X.Kind != NIdent {
			return
		}
		labelID, ok := c.labelIDs[node.X.Name]
		if !ok {
			labelID = c.newLabel()
			c.labelIDs[node.X.Name] = labelID
		}
		c.emit(ir.Inst{Op: ir.OP_JMP, Arg: labelID})
	case "label":
		if node.X == nil || node.X.Kind != NIdent {
			return
		}
		labelID, ok := c.labelIDs[node.X.Name]
		if !ok {
			labelID = c.newLabel()
			c.labelIDs[node.X.Name] = labelID
		}
		c.emitLabel(labelID)
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
		c.emit(ir.Inst{Op: ir.OP_CONST_STR, Name: node.Name})
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
	case NTypeAssertExpr:
		c.compileTypeAssertExpr(node)
	case NIndexExpr:
		c.compileIndexExpr(node)
	case NSliceExpr:
		c.compileSliceExpr(node)
	case NCompositeLit:
		c.compileCompositeLit(node)
	case NFuncType:
		if node.Body != nil {
			c.compileFuncLiteral(node)
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		} else {
			panic("ICE: bare function type in compileExpr")
		}
	default:
		panic("ICE: unhandled expression kind in compileExpr")
	}
}

func (c *Compiler) compileIntLit(node *Node) {
	val := parseIntLiteral(node.Name)
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: val})
}

func (c *Compiler) compileRuneLit(node *Node) {
	val := parseRuneLiteral(node.Name)
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(val)})
}

func (c *Compiler) compileBasicLit(node *Node) {
	if node.Name == "true" {
		c.emit(ir.Inst{Op: ir.OP_CONST_BOOL, Arg: 1})
	} else if node.Name == "false" {
		c.emit(ir.Inst{Op: ir.OP_CONST_BOOL, Arg: 0})
	} else if node.Name == "nil" {
		c.emit(ir.Inst{Op: ir.OP_CONST_NIL})
	} else if node.Name == "iota" {
		// Iota is resolved at const-eval time; emit 0 as placeholder
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
	}
}

func (c *Compiler) compileIdent(node *Node) {
	idx, ok := c.lookupLocal(node.Name)
	if ok {
		w := 0
		if idx < len(c.curFunc.Locals) {
			w = c.curFunc.Locals[idx].Width
		}
		c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: idx, Width: w})
		return
	}
	if capture, ok := c.activeCaptures[node.Name]; ok {
		c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: capture.LocalIdx})
		c.emit(ir.Inst{Op: ir.OP_LOAD, Arg: capture.Width})
		return
	}
	gidx, gok := c.lookupGlobal(node.Name)
	if gok {
		c.emit(ir.Inst{Op: ir.OP_GLOBAL_GET, Arg: gidx})
		return
	}
	// Check if it's a precomputed constant
	qname2 := c.curPkg.QualName(node.Name)
	if sval, ok2 := c.constStringValues[qname2]; ok2 {
		c.emit(ir.Inst{Op: ir.OP_CONST_STR, Name: sval})
		return
	}
	if val, ok2 := c.constValues[qname2]; ok2 {
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: val})
		return
	}
	// Check if it's a constant in the current package
	sym, symOk := c.curPkg.Symbols[node.Name]
	if symOk && sym.Kind == SymConst {
		if c.isConstStringExpr(sym.Node.X) {
			c.emit(ir.Inst{Op: ir.OP_CONST_STR, Name: c.evalConstString(sym.Node.X)})
			return
		}
		val := c.resolveConstValue(sym.Node)
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: val})
		return
	}
	// Could be a package name or unresolved — emit as global reference
	c.emit(ir.Inst{Op: ir.OP_GLOBAL_GET, Name: node.Name})
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
	qname := c.curPkg.QualName(name)
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
		qname := c.curPkg.QualName(node.Name)
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

// isExprByteSlice returns true if the expression is known to produce []byte.
func (c *Compiler) isExprByteSlice(node *Node) bool {
	if node == nil {
		return false
	}
	if node.Kind == NCallExpr && node.X != nil && node.X.Kind == NSliceType && node.X.X != nil {
		return nodeTypeName(node.X.X) == "byte"
	}
	if c.resolveExprType(node) == "[]byte" {
		return true
	}
	return c.exprConcreteType(node) == "[]byte"
}

func isIntegerTypeName(t string) bool {
	return t == "int" || t == "uintptr" || t == "uint" || t == "byte" ||
		t == "int16" || t == "int32" || t == "int64" ||
		t == "uint16" || t == "uint32" || t == "uint64" || t == "rune"
}

// isExprIntegerLike reports whether expression is known to be integer-like.
func (c *Compiler) isExprIntegerLike(node *Node) bool {
	if node == nil {
		return false
	}
	switch node.Kind {
	case NIntLit, NRuneLit:
		return true
	case NCallExpr:
		if node.X != nil && node.X.Kind == NIdent {
			return isIntegerTypeName(node.X.Name)
		}
	}
	t := c.resolveExprType(node)
	if isIntegerTypeName(t) {
		return true
	}
	return isIntegerTypeName(c.exprConcreteType(node))
}

func (c *Compiler) constIntArg(node *Node) (int, bool) {
	if node == nil {
		return 0, false
	}
	switch node.Kind {
	case NIntLit:
		v := parseIntLiteral(node.Name)
		iv := int(v)
		return iv, int64(iv) == v
	case NRuneLit:
		v := int64(parseRuneLiteral(node.Name))
		iv := int(v)
		return iv, int64(iv) == v
	case NIdent:
		qname := c.curPkg.QualName(node.Name)
		if v, ok := c.constValues[qname]; ok {
			iv := int(v)
			return iv, int64(iv) == v
		}
	case NCallExpr:
		if node.X != nil && node.X.Kind == NIdent && len(node.Nodes) == 1 {
			name := node.X.Name
			if name == "uintptr" || name == "uint" || name == "int" || name == "int64" || name == "uint64" {
				return c.constIntArg(node.Nodes[0])
			}
		}
	}
	return 0, false
}

func (c *Compiler) emitStringEqualCall(x *Node, y *Node) {
	c.compileExpr(x)
	c.compileExpr(y)
	c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.StringEqual", Arg: 2})
}

func (c *Compiler) compileLogicalBinary(node *Node, isAnd bool) {
	branchLabel := c.newLabel()
	endLabel := c.newLabel()
	c.compileExpr(node.X)
	if isAnd {
		c.emit(ir.Inst{Op: ir.OP_JMP_IF_NOT, Arg: branchLabel})
	} else {
		c.emit(ir.Inst{Op: ir.OP_JMP_IF, Arg: branchLabel})
	}
	// Conditional jump popped condition; now compile Y (pushes 1 value)
	savedDepth := c.stackDepth
	c.compileExpr(node.Y)
	c.emit(ir.Inst{Op: ir.OP_JMP, Arg: endLabel})
	// Alternate branch starts at same depth after conditional jump
	c.stackDepth = savedDepth
	c.emitLabel(branchLabel)
	if isAnd {
		c.emit(ir.Inst{Op: ir.OP_CONST_BOOL, Arg: 0})
	} else {
		c.emit(ir.Inst{Op: ir.OP_CONST_BOOL, Arg: 1})
	}
	c.emitLabel(endLabel)
}

func (c *Compiler) compileBinaryExpr(node *Node) {
	// Short-circuit for && and ||
	if node.Name == "&&" {
		c.compileLogicalBinary(node, true)
		return
	}
	if node.Name == "||" {
		c.compileLogicalBinary(node, false)
		return
	}

	// String operations: concatenation and comparison
	isStr := isStringExpr(node.X) || isStringExpr(node.Y) || c.isStringTypedExpr(node.X) || c.isStringTypedExpr(node.Y)
	if isStr && node.Name == "+" {
		c.compileExpr(node.X)
		c.compileExpr(node.Y)
		c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.StringConcat", Arg: 2})
		return
	}
	if isStr && node.Name == "==" {
		c.emitStringEqualCall(node.X, node.Y)
		return
	}
	if isStr && node.Name == "!=" {
		c.emitStringEqualCall(node.X, node.Y)
		c.emit(ir.Inst{Op: ir.OP_NOT})
		return
	}

	// Word-sized + constant => OFFSET (smaller than const+add stack sequence).
	// This covers pointer/uintptr arithmetic and other word-sized arithmetic forms.
	if node.Name == "+" && c.exprWidth(node) == 0 {
		if off, ok := c.constIntArg(node.Y); ok {
			c.compileExpr(node.X)
			c.emit(ir.Inst{Op: ir.OP_OFFSET, Arg: off})
			return
		}
		if off, ok := c.constIntArg(node.X); ok {
			c.compileExpr(node.Y)
			c.emit(ir.Inst{Op: ir.OP_OFFSET, Arg: off})
			return
		}
	}

	c.compileExpr(node.X)
	c.compileExpr(node.Y)

	w := c.exprWidth(node)

	switch node.Name {
	case "+":
		c.emit(ir.Inst{Op: ir.OP_ADD, Width: w})
	case "-":
		c.emit(ir.Inst{Op: ir.OP_SUB, Width: w})
	case "*":
		c.emit(ir.Inst{Op: ir.OP_MUL, Width: w})
	case "/":
		c.emit(ir.Inst{Op: ir.OP_DIV, Width: w})
	case "%":
		c.emit(ir.Inst{Op: ir.OP_MOD, Width: w})
	case "&":
		c.emit(ir.Inst{Op: ir.OP_AND, Width: w})
	case "|":
		c.emit(ir.Inst{Op: ir.OP_OR, Width: w})
	case "^":
		c.emit(ir.Inst{Op: ir.OP_XOR, Width: w})
	case "<<":
		c.emit(ir.Inst{Op: ir.OP_SHL, Width: w})
	case ">>":
		c.emit(ir.Inst{Op: ir.OP_SHR, Width: w})
	case "==":
		c.emit(ir.Inst{Op: ir.OP_EQ, Width: w})
	case "!=":
		c.emit(ir.Inst{Op: ir.OP_NEQ, Width: w})
	case "<":
		c.emit(ir.Inst{Op: ir.OP_LT, Width: w})
	case ">":
		c.emit(ir.Inst{Op: ir.OP_GT, Width: w})
	case "<=":
		c.emit(ir.Inst{Op: ir.OP_LEQ, Width: w})
	case ">=":
		c.emit(ir.Inst{Op: ir.OP_GEQ, Width: w})
	default:
		panic("ICE: unhandled binary operator in compileBinaryExpr")
	}
}

func (c *Compiler) compileUnaryExpr(node *Node) {
	switch node.Name {
	case "!":
		c.compileExpr(node.X)
		c.emit(ir.Inst{Op: ir.OP_NOT})
	case "-":
		w := c.exprWidth(node.X)
		c.compileExpr(node.X)
		c.emit(ir.Inst{Op: ir.OP_NEG, Width: w})
	case "*":
		c.compileExpr(node.X)
		if !c.isPointerToStructDeref(node.X) {
			c.emit(ir.Inst{Op: ir.OP_LOAD, Arg: c.target.PtrSize})
		}
	case "&":
		c.compileAddrOf(node.X)
	case "^":
		w := c.exprWidth(node.X)
		c.compileExpr(node.X)
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: -1, Width: w})
		c.emit(ir.Inst{Op: ir.OP_XOR, Width: w})
		if w == 1 {
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0xFF})
			c.emit(ir.Inst{Op: ir.OP_AND})
		}
	default:
		panic("ICE: unhandled unary operator in compileUnaryExpr")
	}
}

// needsSelectorDeref checks if a selector base needs an extra LOAD for auto-deref.
// For pp.X where pp is *Point, we need to load through pp to get the struct pointer.
// Returns false for unknowns (conservative — no extra deref).
func (c *Compiler) needsSelectorDeref(node *Node) bool {
	if node == nil || node.Kind != NIdent {
		return false
	}
	// Only auto-deref variables created from & (address-of local)
	if !c.localAddrOf[node.Name] {
		return false
	}
	ct, ok := c.localConcreteTypes[node.Name]
	if !ok {
		return false
	}
	dotIdx := -1
	for i := 0; i < len(ct); i++ {
		if ct[i] == '.' {
			dotIdx = i
		}
	}
	if dotIdx < 0 {
		return false
	}
	rest := ct[dotIdx+1 : len(ct)]
	if len(rest) == 0 || rest[0] != '*' {
		return false
	}
	tName := rest[1:len(rest)]
	if len(tName) > 0 && tName[0] == '[' {
		return false
	}
	if tName == "int" || tName == "int16" || tName == "int32" || tName == "int64" ||
		tName == "uint" || tName == "uint16" || tName == "uint32" || tName == "uint64" ||
		tName == "uintptr" || tName == "byte" || tName == "bool" || tName == "string" {
		return false
	}
	if strings.HasPrefix(tName, "map[") || strings.HasPrefix(tName, "func(") || strings.HasPrefix(tName, "*") {
		return false
	}
	pkg, ok := c.mod.Packages[ct[0:dotIdx]]
	if !ok {
		return false
	}
	if pkg.Path == c.curPkg.Path {
		if localDecl, ok := c.localTypeDecls[tName]; ok && localDecl != nil && localDecl.Type != nil {
			return localDecl.Type.Kind == NStructType
		}
	}
	sym, ok := pkg.Symbols[tName]
	if !ok || sym.Kind != SymType || sym.Node == nil {
		return false
	}
	typeNode := sym.Node.Type
	if typeNode == nil {
		return false
	}
	return typeNode.Kind == NStructType
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
	if tName == "int" || tName == "int16" || tName == "int32" || tName == "int64" ||
		tName == "uint" || tName == "uint16" || tName == "uint32" || tName == "uint64" ||
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
	if pkg.Path == c.curPkg.Path {
		if localDecl, ok := c.localTypeDecls[tName]; ok && localDecl != nil && localDecl.Type != nil {
			return localDecl.Type.Kind == NStructType
		}
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
		if capture, ok := c.activeCaptures[node.Name]; ok {
			c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: capture.LocalIdx})
			return
		}
		idx, ok := c.lookupLocal(node.Name)
		if ok {
			// Struct-typed locals are represented as heap handles in the slot.
			// Taking their address should preserve that handle, not the slot address.
			if ct, hasType := c.localConcreteTypes[node.Name]; hasType {
				if typeNode, _ := c.lookupStructTypeNode(ct); typeNode != nil {
					c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: idx})
					return
				}
			}
			c.emit(ir.Inst{Op: ir.OP_LOCAL_ADDR, Arg: idx})
		} else {
			gidx, gok := c.lookupGlobal(node.Name)
			if gok {
				// Struct-typed globals are already represented as heap handles.
				// Taking their address should preserve that handle, not the global slot address.
				qname := c.curPkg.QualName(node.Name)
				if ct, ok := c.globalConcreteTypes[qname]; ok {
					if typeNode, _ := c.lookupStructTypeNode(ct); typeNode != nil {
						c.emit(ir.Inst{Op: ir.OP_GLOBAL_GET, Arg: gidx})
						return
					}
				}
				c.emit(ir.Inst{Op: ir.OP_GLOBAL_ADDR, Arg: gidx})
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
		c.emit(ir.Inst{Op: ir.OP_CONST_NIL})
		return
	}
	sliceHdrSize := 4 * c.target.PtrSize // 32 on amd64, 16 on i386
	allocSize := sliceHdrSize + varCount*elemSz
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(allocSize)})
	c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.Alloc", Arg: 1})
	tmpIdx := c.addLocal("$varslice")
	c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: tmpIdx})
	// header[0] = data_ptr (header + sliceHdrSize)
	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: tmpIdx})
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(sliceHdrSize)})
	c.emit(ir.Inst{Op: ir.OP_ADD})
	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: tmpIdx})
	c.emit(ir.Inst{Op: ir.OP_STORE})
	// header[ptrSize] = len
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(varCount)})
	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: tmpIdx})
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(c.target.PtrSize)})
	c.emit(ir.Inst{Op: ir.OP_ADD})
	c.emit(ir.Inst{Op: ir.OP_STORE})
	// header[2*ptrSize] = cap
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(varCount)})
	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: tmpIdx})
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(2 * c.target.PtrSize)})
	c.emit(ir.Inst{Op: ir.OP_ADD})
	c.emit(ir.Inst{Op: ir.OP_STORE})
	// header[3*ptrSize] = elem_size
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(elemSz)})
	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: tmpIdx})
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(3 * c.target.PtrSize)})
	c.emit(ir.Inst{Op: ir.OP_ADD})
	c.emit(ir.Inst{Op: ir.OP_STORE})
	// Store each variadic arg into data region
	isIfaceVar := c.funcVariadicIface[ifaceKey]
	j := 0
	for j < varCount {
		arg := args[firstArgIdx+j]
		c.compileExpr(arg)
		if isIfaceVar {
			typeID := c.exprPrimitiveTypeID(arg)
			if typeID > 0 {
				c.emit(ir.Inst{Op: ir.OP_IFACE_BOX, Arg: typeID})
			}
		}
		c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: tmpIdx})
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(sliceHdrSize + j*elemSz)})
		c.emit(ir.Inst{Op: ir.OP_ADD})
		c.emit(ir.Inst{Op: ir.OP_STORE, Arg: elemSz})
		j++
	}
	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: tmpIdx})
}

func (c *Compiler) emitCallWithReceiver(receiver *Node, args []*Node, callName string) {
	c.compileExpr(receiver)
	for _, arg := range args {
		c.compileExpr(arg)
	}
	c.emit(ir.Inst{Op: ir.OP_CALL, Name: callName, Arg: len(args) + 1})
}

func runtimeMemBuiltinReturnCount(name string) (int, bool) {
	if name == "runtime.ReadPtr" {
		return 1, true
	}
	if name == "runtime.WritePtr" || name == "runtime.WriteByte" {
		return 0, true
	}
	return 0, false
}

func (c *Compiler) emitRuntimeMemBuiltinCall(callName string, args []*Node) bool {
	if callName == "runtime.ReadPtr" && len(args) == 1 {
		c.compileExpr(args[0])
		c.emit(ir.Inst{Op: ir.OP_LOAD, Arg: c.target.PtrSize})
		return true
	}
	if callName == "runtime.WritePtr" && len(args) == 2 {
		c.compileExpr(args[1])
		c.compileExpr(args[0])
		c.emit(ir.Inst{Op: ir.OP_STORE, Arg: c.target.PtrSize})
		return true
	}
	if callName == "runtime.WriteByte" && len(args) == 2 {
		c.compileExpr(args[1])
		c.compileExpr(args[0])
		c.emit(ir.Inst{Op: ir.OP_STORE, Arg: 1})
		return true
	}
	return false
}

func (c *Compiler) compileNewBuiltin(node *Node) bool {
	if node == nil || len(node.Nodes) != 1 {
		return false
	}
	typeArg := node.Nodes[0]
	typeName := nodeTypeName(typeArg)
	qualified := c.qualifyTypeName(typeName, "")
	size := c.target.PtrSize
	if typeNode, _ := c.lookupStructTypeNode(qualified); typeNode != nil {
		slots := c.resolveStructSlotCount(qualified)
		if slots > 0 {
			size = slots * c.target.PtrSize
		}
	} else if typeArg.Kind == NIdent {
		if w := typeWidth(typeArg.Name); w > 0 {
			size = w
		}
	}
	if size <= 0 {
		size = c.target.PtrSize
	}
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(size)})
	c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.Alloc", Arg: 1})
	c.emit(ir.Inst{Op: ir.OP_DUP})
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(size)})
	c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.Memzero", Arg: 2})
	return true
}

func (c *Compiler) emitSysWriteStringLocal(localIdx int) {
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 1})
	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: localIdx})
	c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.Stringptr", Arg: 1})
	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: localIdx})
	c.emit(ir.Inst{Op: ir.OP_LEN})
	c.emit(ir.Inst{Op: ir.OP_CONVERT, Name: "uintptr"})
	c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.SysWrite", Arg: 3})
	c.emit(ir.Inst{Op: ir.OP_DROP})
	c.emit(ir.Inst{Op: ir.OP_DROP})
	c.emit(ir.Inst{Op: ir.OP_DROP})
}

func (c *Compiler) compilePrintBuiltin(node *Node, withNewline bool) {
	tmpIdx := c.addLocal(fmt.Sprintf("$print_%d", len(c.curFunc.Locals)))
	for i, arg := range node.Nodes {
		c.compileExpr(arg)
		c.maybeBoxValueForInterface(arg)
		c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.Tostring", Arg: 1})
		c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: tmpIdx})
		c.emitSysWriteStringLocal(tmpIdx)
		if withNewline && i < len(node.Nodes)-1 {
			c.emit(ir.Inst{Op: ir.OP_CONST_STR, Name: " "})
			c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: tmpIdx})
			c.emitSysWriteStringLocal(tmpIdx)
		}
	}
	if withNewline {
		c.emit(ir.Inst{Op: ir.OP_CONST_STR, Name: "\n"})
		c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: tmpIdx})
		c.emitSysWriteStringLocal(tmpIdx)
	}
}

func (c *Compiler) compileBoundMethodValueCall(node *Node) bool {
	if node == nil || node.X == nil || node.X.Kind != NIdent {
		return false
	}
	name := node.X.Name
	target, ok := c.localMethodTargets[name]
	if !ok {
		return false
	}
	recvIdx, hasRecv := c.localMethodRecv[name]
	if !hasRecv {
		recvIdx, hasRecv = c.lookupLocal(name)
	}
	if hasRecv {
		w := 0
		if recvIdx < len(c.curFunc.Locals) {
			w = c.curFunc.Locals[recvIdx].Width
		}
		c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: recvIdx, Width: w})
	} else {
		c.errorf("%s: missing method receiver for %s", c.curFunc.Name, name)
		c.emit(ir.Inst{Op: ir.OP_CONST_NIL})
	}
	for _, arg := range node.Nodes {
		c.compileExpr(arg)
	}
	c.emit(ir.Inst{Op: ir.OP_CALL, Name: target, Arg: len(node.Nodes) + 1})
	return true
}

func (c *Compiler) compileCallExpr(node *Node) {
	if node.X != nil && node.X.Kind == NIdent {
		if target, ok := c.localFuncTargets[node.X.Name]; ok {
			captureArgs := c.localFuncCaptures[node.X.Name]
			for _, capture := range captureArgs {
				if capture.IsPtr {
					c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: capture.LocalIdx})
				} else {
					c.emit(ir.Inst{Op: ir.OP_LOCAL_ADDR, Arg: capture.LocalIdx})
				}
			}
			for _, arg := range node.Nodes {
				c.compileExpr(arg)
			}
			c.emit(ir.Inst{Op: ir.OP_CALL, Name: target, Arg: len(node.Nodes) + len(captureArgs)})
			return
		}
		if c.compileBoundMethodValueCall(node) {
			return
		}
	}

	// Check for builtins
	if node.X != nil && node.X.Kind == NIdent {
		name := node.X.Name
		if name == "recover" {
			// Minimal recover stub: outside panic unwinding this returns nil.
			// Full panic/recover semantics remain unimplemented.
			c.emit(ir.Inst{Op: ir.OP_CONST_NIL})
			return
		}
		if name == "complex" || name == "real" || name == "imag" {
			c.errorf("%s: %s builtin is not supported (complex numbers are not implemented)", c.curFunc.Name, name)
			c.emit(ir.Inst{Op: ir.OP_CONST_NIL})
			return
		}
		if name == "float32" || name == "float64" {
			c.errorf("%s: %s conversion is not supported (floating-point support is not implemented)", c.curFunc.Name, name)
			c.emit(ir.Inst{Op: ir.OP_CONST_NIL})
			return
		}
		if name == "len" {
			if len(node.Nodes) > 0 && c.isMapExpr(node.Nodes[0]) {
				c.compileExpr(node.Nodes[0])
				c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.MapLen", Arg: 1})
				return
			}
			c.compileExpr(node.Nodes[0])
			c.emit(ir.Inst{Op: ir.OP_LEN})
			return
		}
		if name == "cap" {
			c.compileExpr(node.Nodes[0])
			c.emit(ir.Inst{Op: ir.OP_CAP})
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
				c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.MapDelete", Arg: 2})
			}
			return
		}
		if name == "clear" {
			if len(node.Nodes) >= 1 {
				target := node.Nodes[0]
				if c.isMapExpr(target) && target.Kind == NIdent {
					keyKind := 0
					if k, ok := c.localMapVars[target.Name]; ok {
						keyKind = k
					} else {
						gq := c.curPkg.QualName(target.Name)
						if k, ok := c.globalMapVars[gq]; ok {
							keyKind = k
						}
					}
					c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(keyKind)})
					c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.MapMake", Arg: 1})
					c.compileLValueSet(target)
					return
				}
			}
			c.errorf("%s: clear is currently only supported for local/global map variables", c.curFunc.Name)
			return
		}
		if name == "make" {
			c.compileMake(node)
			return
		}
		if name == "panic" {
			if len(c.deferNames) > 0 {
				c.emitDeferredCalls()
			}
			if len(node.Nodes) > 0 {
				c.compileExpr(node.Nodes[0])
			} else {
				c.emit(ir.Inst{Op: ir.OP_CONST_STR, Name: "panic"})
			}
			c.emit(ir.Inst{Op: ir.OP_PANIC})
			return
		}
		if name == "new" {
			if c.compileNewBuiltin(node) {
				return
			}
		}
		if name == "print" {
			c.compilePrintBuiltin(node, false)
			return
		}
		if name == "println" {
			c.compilePrintBuiltin(node, true)
			return
		}
		// Type conversions: int(), uintptr(), byte(), string(), int16(), int32()
		if name == "int" || name == "uintptr" || name == "uint" || name == "byte" || name == "string" || name == "int16" || name == "int32" || name == "int64" || name == "uint16" || name == "uint32" || name == "uint64" {
			arg := node.Nodes[0]
			c.compileExpr(arg)
			if name == "string" {
				if c.isExprByte(arg) {
					c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.ByteToString", Arg: 1})
				} else if c.isExprByteSlice(arg) {
					c.emit(ir.Inst{Op: ir.OP_CONVERT, Name: name})
				} else if c.isStringTypedExpr(arg) {
					// string(string) is a no-op.
				} else if c.isExprIntegerLike(arg) {
					// string(int/rune) conversion.
					c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.RuneToString", Arg: 1})
				} else {
					// Prefer slice->string semantics unless we know this is integer-like.
					c.emit(ir.Inst{Op: ir.OP_CONVERT, Name: name})
				}
			} else {
				c.emit(ir.Inst{Op: ir.OP_CONVERT, Name: name})
			}
			return
		}
	}

	// Check for []byte() conversion
	if node.X != nil && node.X.Kind == NSliceType {
		c.compileExpr(node.Nodes[0])
		c.emit(ir.Inst{Op: ir.OP_CONVERT, Name: "[]byte"})
		return
	}

	// Check for user-defined type conversions (e.g. Errno(val))
	if node.X != nil && node.X.Kind == NIdent && len(node.Nodes) == 1 {
		if _, ok := c.lookupCurrentTypeDecl(node.X.Name); ok {
			c.compileExpr(node.Nodes[0])
			c.emit(ir.Inst{Op: ir.OP_CONVERT, Name: node.X.Name})
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
				c.emit(ir.Inst{Op: ir.OP_CONVERT, Name: typeName})
				return
			}
		}
	}

	// Check for interface method call: e.g. err.Error()
	if node.X != nil && node.X.Kind == NSelectorExpr && node.X.X != nil && node.X.X.Kind == NIdent {
		recvName := node.X.X.Name
		methodName := node.X.Name
		if ifaceType, ok := c.localTypes[recvName]; ok {
			if _, hasMethod := c.ifaceMethodReturnCount(ifaceType, methodName); hasMethod {
				// Push receiver (interface pointer) then args
				c.compileExpr(node.X.X)
				for _, arg := range node.Nodes {
					c.compileExpr(arg)
				}
				c.emit(ir.Inst{Op: ir.OP_IFACE_CALL, Name: c.dotJoin(ifaceType, methodName), Arg: len(node.Nodes)})
				return
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
			gqname := c.curPkg.QualName(recvName)
			concreteType, ok = c.globalConcreteTypes[gqname]
		}
		if ok {
			resolvedName, ok := c.resolveMethodByConcreteType(concreteType, methodName)
			if !ok {
				if pm, found := c.findPromotedMethod(concreteType, methodName); found {
					if !c.isComptimeCallAllowed(pm.Target) {
						return
					}
					// Build receiver from embedded field path.
					c.compileExpr(node.X.X)
					i := 0
					for i < len(pm.Offsets) {
						c.emit(ir.Inst{Op: ir.OP_OFFSET, Arg: pm.Offsets[i]})
						c.emit(ir.Inst{Op: ir.OP_LOAD, Arg: c.target.PtrSize})
						i++
					}
					for _, arg := range node.Nodes {
						c.compileExpr(arg)
					}
					c.emit(ir.Inst{Op: ir.OP_CALL, Name: pm.Target, Arg: len(node.Nodes) + 1})
					return
				}
			}
			if ok {
				if !c.isComptimeCallAllowed(resolvedName) {
					return
				}
				if c.tryCompileComptimeCall(node, resolvedName) {
					return
				}
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
					mVarElemSz := c.target.PtrSize
					if mesz, ok := c.funcVariadicElem[resolvedName]; ok {
						mVarElemSz = mesz
					}
					c.packVariadicSlice(node.Nodes, fixedCount-1, variadicCount, mVarElemSz, resolvedName)
					c.emit(ir.Inst{Op: ir.OP_CALL, Name: resolvedName, Arg: fixedCount + 1})
				} else {
					// Non-variadic or spread: push receiver first, then args
					c.emitCallWithReceiver(node.X.X, node.Nodes, resolvedName)
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
					resolvedName, ok := c.resolveMethodByConcreteType(fieldType, methodName)
					if ok {
						if !c.isComptimeCallAllowed(resolvedName) {
							return
						}
						if c.tryCompileComptimeCall(node, resolvedName) {
							return
						}
						// Push receiver (the field access) first, then args
						c.emitCallWithReceiver(node.X.X, node.Nodes, resolvedName)
						return
					}
				}
			}
		}
	}

	// Determine the function to call
	callName := c.resolveCallName(node.X)
	if !c.isComptimeCallAllowed(callName) {
		return
	}
	if c.emitRuntimeMemBuiltinCall(callName, node.Nodes) {
		return
	}
	if c.tryCompileComptimeCall(node, callName) {
		return
	}

	paramTypes := c.funcParamTypes[callName]

	// Check if this is a variadic function call
	fixedCount, isVariadic := c.funcVariadic[callName]
	isSpread := node.Name == "spread"

	if isVariadic && !isSpread {
		// Compile fixed args normally
		i := 0
		for i < fixedCount && i < len(node.Nodes) {
			arg := node.Nodes[i]
			c.compileExpr(arg)
			if i < len(paramTypes) && c.isInterfaceTypeName(paramTypes[i]) {
				c.maybeBoxValueForInterface(arg)
			}
			i++
		}

		// Package variadic args into an inline slice
		variadicCount := len(node.Nodes) - fixedCount
		if variadicCount < 0 {
			variadicCount = 0
		}

		varElemSz := c.target.PtrSize
		if esz, ok := c.funcVariadicElem[callName]; ok {
			varElemSz = esz
		}

		c.packVariadicSlice(node.Nodes, fixedCount, variadicCount, varElemSz, callName)

		// Call with fixedCount + 1 args (fixed params + one slice)
		c.emit(ir.Inst{Op: ir.OP_CALL, Name: callName, Arg: fixedCount + 1})
	} else {
		// Non-variadic call, or spread call — compile all args normally.
		for i, arg := range node.Nodes {
			c.compileExpr(arg)
			// For variadic spread calls, the last arg is already a variadic
			// slice value and must not be boxed as interface{}.
			shouldBox := true
			if isVariadic && isSpread && i == len(node.Nodes)-1 {
				shouldBox = false
			}
			if shouldBox && i < len(paramTypes) && c.isInterfaceTypeName(paramTypes[i]) {
				c.maybeBoxValueForInterface(arg)
			}
		}

		argCount := len(node.Nodes)

		// Pad missing args with nil
		if expected, ok := c.funcParams[callName]; ok && argCount < expected {
			for argCount < expected {
				c.emit(ir.Inst{Op: ir.OP_CONST_NIL})
				argCount++
			}
		}

		c.emit(ir.Inst{Op: ir.OP_CALL, Name: callName, Arg: argCount})
	}
}

func (c *Compiler) tryCompileComptimeCall(node *Node, callName string) bool {
	if node == nil || callName == "" || c.comptimeDisabled {
		return false
	}
	if !c.comptimeFuncs[callName] {
		return false
	}
	retCount, ok := c.funcRets[callName]
	if !ok || retCount != 1 {
		c.errorf("%s: comptime call %s must return exactly one value", c.curFunc.Name, callName)
		return false
	}
	retType := c.funcRetTypeNodes[callName]
	if retType == nil {
		c.errorf("%s: comptime call %s has unknown return type", c.curFunc.Name, callName)
		return false
	}
	if c.exprUsesLocalIdentifier(node) {
		c.errorf("%s: comptime call %s may only use compile-time constants and globals", c.curFunc.Name, callName)
		return false
	}

	wrapName, wrapFunc, err := c.buildComptimeWrapper(node, retCount)
	if err != nil {
		c.errorf("%s: comptime wrapper build failed for %s: %v", c.curFunc.Name, callName, err)
		return false
	}

	c.irmod.Funcs = append(c.irmod.Funcs, wrapFunc)
	c.irmod.TypeIDs = c.typeIDs
	c.irmod.MethodTable = c.methodTable
	c.irmod.IfaceMethods = c.ifaceMethods
	c.irmod.IfaceMethodRets = c.ifaceMethodRets
	eval, err := vm.NewEvalState(c.target, c.irmod)
	c.irmod.Funcs = c.irmod.Funcs[0 : len(c.irmod.Funcs)-1]
	if err != nil {
		c.errorf("%s: comptime init failed for %s: %v", c.curFunc.Name, callName, err)
		return false
	}
	rets, err := vm.EvalCall(eval, wrapName, nil, retCount)
	if err != nil {
		c.errorf("%s: comptime execution failed for %s: %v", c.curFunc.Name, callName, err)
		return false
	}
	if len(rets) != 1 {
		c.errorf("%s: comptime execution failed for %s: expected 1 return, got %d", c.curFunc.Name, callName, len(rets))
		return false
	}

	lit, err := c.decodeComptimeValue(rets[0], retType, eval, 0)
	if err != nil {
		c.errorf("%s: comptime decode failed for %s: %v", c.curFunc.Name, callName, err)
		return false
	}
	c.compileExpr(lit)
	return true
}

func (c *Compiler) buildComptimeWrapper(call *Node, retCount int) (string, *ir.IRFunc, error) {
	c.comptimeSeq = c.comptimeSeq + 1
	wrapName := fmt.Sprintf("%s.comptime$%d", c.curPkg.Path, c.comptimeSeq)
	f := &ir.IRFunc{Name: wrapName, RetCount: retCount}
	prevErrs := len(c.errors)

	savedCurFunc := c.curFunc
	savedScopes := c.scopes
	savedLocalElemSizes := c.localElemSizes
	savedLocalTypes := c.localTypes
	savedLocalTypeDecls := c.localTypeDecls
	savedLocalStringVars := c.localStringVars
	savedLocalConcreteTypes := c.localConcreteTypes
	savedLocalMapVars := c.localMapVars
	savedLocalMapValueTypes := c.localMapValueTypes
	savedLocalAddrOf := c.localAddrOf
	savedDeferNames := c.deferNames
	savedDeferArgStarts := c.deferArgStarts
	savedDeferArgCounts := c.deferArgCounts
	savedDeferRetCounts := c.deferRetCounts
	savedNamedResultNames := c.namedResultNames
	savedPendingStmtLabels := c.pendingStmtLabels
	savedLabelIDs := c.labelIDs
	savedBreakLabelTargets := c.breakLabelTargets
	savedContinueLabelTargets := c.continueLabelTargets
	savedStackDepth := c.stackDepth
	savedLocalFuncTargets := c.localFuncTargets
	savedLocalMethodTargets := c.localMethodTargets
	savedLocalMethodRecv := c.localMethodRecv
	savedLocalFuncCaptures := c.localFuncCaptures
	savedActiveCaptures := c.activeCaptures
	savedComptimeDisabled := c.comptimeDisabled

	c.curFunc = f
	c.scopes = nil
	c.localElemSizes = make(map[string]int)
	c.localTypes = make(map[string]string)
	c.localTypeDecls = make(map[string]*Node)
	c.localStringVars = make(map[string]bool)
	c.localConcreteTypes = make(map[string]string)
	c.localMapVars = make(map[string]int)
	c.localMapValueTypes = make(map[string]string)
	c.localAddrOf = make(map[string]bool)
	c.deferNames = nil
	c.deferArgStarts = nil
	c.deferArgCounts = nil
	c.deferRetCounts = nil
	c.namedResultNames = nil
	c.fallthroughs = nil
	c.pendingStmtLabels = nil
	c.labelIDs = make(map[string]int)
	c.breakLabelTargets = make(map[string][]int)
	c.continueLabelTargets = make(map[string][]int)
	c.stackDepth = 0
	c.localFuncTargets = make(map[string]string)
	c.localMethodTargets = make(map[string]string)
	c.localMethodRecv = make(map[string]int)
	c.localFuncCaptures = make(map[string][]closureCaptureBinding)
	c.activeCaptures = nil
	c.comptimeDisabled = true
	c.pushScope()
	c.compileExpr(call)
	c.emit(ir.Inst{Op: ir.OP_RETURN, Arg: retCount})
	c.popScope()

	c.curFunc = savedCurFunc
	c.scopes = savedScopes
	c.localElemSizes = savedLocalElemSizes
	c.localTypes = savedLocalTypes
	c.localTypeDecls = savedLocalTypeDecls
	c.localStringVars = savedLocalStringVars
	c.localConcreteTypes = savedLocalConcreteTypes
	c.localMapVars = savedLocalMapVars
	c.localMapValueTypes = savedLocalMapValueTypes
	c.localAddrOf = savedLocalAddrOf
	c.deferNames = savedDeferNames
	c.deferArgStarts = savedDeferArgStarts
	c.deferArgCounts = savedDeferArgCounts
	c.deferRetCounts = savedDeferRetCounts
	c.namedResultNames = savedNamedResultNames
	c.pendingStmtLabels = savedPendingStmtLabels
	c.labelIDs = savedLabelIDs
	c.breakLabelTargets = savedBreakLabelTargets
	c.continueLabelTargets = savedContinueLabelTargets
	c.stackDepth = savedStackDepth
	c.localFuncTargets = savedLocalFuncTargets
	c.localMethodTargets = savedLocalMethodTargets
	c.localMethodRecv = savedLocalMethodRecv
	c.localFuncCaptures = savedLocalFuncCaptures
	c.activeCaptures = savedActiveCaptures
	c.comptimeDisabled = savedComptimeDisabled

	if len(c.errors) > prevErrs {
		return "", nil, fmt.Errorf("wrapper compilation produced errors")
	}
	return wrapName, f, nil
}

func (c *Compiler) exprUsesLocalIdentifier(node *Node) bool {
	if node == nil {
		return false
	}
	if node.Kind == NIdent {
		_, isLocal := c.lookupLocal(node.Name)
		if isLocal {
			return true
		}
	}
	if c.exprUsesLocalIdentifier(node.X) {
		return true
	}
	if c.exprUsesLocalIdentifier(node.Y) {
		return true
	}
	if c.exprUsesLocalIdentifier(node.Body) {
		return true
	}
	if c.exprUsesLocalIdentifier(node.Type) {
		return true
	}
	for _, child := range node.Nodes {
		if c.exprUsesLocalIdentifier(child) {
			return true
		}
	}
	return false
}

func cloneTypeNode(node *Node) *Node {
	if node == nil {
		return nil
	}
	out := &Node{
		Kind: node.Kind,
		Pos:  node.Pos,
		Name: node.Name,
	}
	out.X = cloneTypeNode(node.X)
	out.Y = cloneTypeNode(node.Y)
	out.Body = cloneTypeNode(node.Body)
	out.Type = cloneTypeNode(node.Type)
	if len(node.Nodes) > 0 {
		out.Nodes = make([]*Node, len(node.Nodes))
		i := 0
		for i < len(node.Nodes) {
			out.Nodes[i] = cloneTypeNode(node.Nodes[i])
			i = i + 1
		}
	}
	return out
}

func (c *Compiler) resolveTypeAliasNode(typeNode *Node) *Node {
	if typeNode == nil {
		return nil
	}
	if typeNode.Kind == NIdent {
		if decl, ok := c.lookupCurrentTypeDecl(typeNode.Name); ok && decl != nil && decl.Type != nil {
			return decl.Type
		}
		return typeNode
	}
	if typeNode.Kind == NSelectorExpr && typeNode.X != nil && typeNode.X.Kind == NIdent {
		pkg := c.resolvePackage(typeNode.X.Name)
		if pkg != nil {
			if sym, ok := pkg.Symbols[typeNode.Name]; ok && sym.Kind == SymType && sym.Node != nil && sym.Node.Type != nil {
				return sym.Node.Type
			}
		}
	}
	return typeNode
}

func signExtendBits(raw uint64, bits int) int64 {
	if bits <= 0 {
		return int64(raw)
	}
	if bits >= 64 {
		return int64(raw)
	}
	mask := (uint64(1) << uint(bits)) - 1
	v := raw & mask
	sign := uint64(1) << uint(bits-1)
	if v&sign != 0 {
		v = v | ^mask
	}
	return int64(v)
}

func maskBits(raw uint64, bits int) uint64 {
	if bits <= 0 || bits >= 64 {
		return raw
	}
	return raw & ((uint64(1) << uint(bits)) - 1)
}

func intNode(v int64) *Node {
	return &Node{Kind: NIntLit, Name: fmt.Sprintf("%d", v)}
}

func (c *Compiler) decodeComptimeValue(raw uint64, typeNode *Node, eval *vm.EvalState, depth int) (*Node, error) {
	if depth > 64 {
		return nil, fmt.Errorf("value nesting too deep")
	}
	if typeNode == nil {
		return intNode(int64(raw)), nil
	}
	aliasNode := c.resolveTypeAliasNode(typeNode)
	if aliasNode == nil {
		return intNode(int64(raw)), nil
	}

	if aliasNode.Kind == NIdent {
		name := aliasNode.Name
		wordBits := vm.EvalWordSize(eval) * 8
		if name == "bool" {
			if raw != 0 {
				return &Node{Kind: NBasicLit, Name: "true"}, nil
			}
			return &Node{Kind: NBasicLit, Name: "false"}, nil
		}
		if name == "string" {
			if raw == 0 {
				return &Node{Kind: NStringLit, Name: ""}, nil
			}
			dataPtr := vm.EvalLoadWord(eval, raw)
			slen := int(vm.EvalLoadWord(eval, raw+uint64(vm.EvalWordSize(eval))))
			if slen <= 0 || dataPtr == 0 {
				return &Node{Kind: NStringLit, Name: ""}, nil
			}
			data, err := vm.EvalLoadBytes(eval, dataPtr, slen)
			if err != nil {
				return nil, err
			}
			return &Node{Kind: NStringLit, Name: string(data)}, nil
		}
		if name == "byte" {
			return intNode(signExtendBits(maskBits(raw, 8), 8)), nil
		}
		if name == "int16" {
			return intNode(signExtendBits(raw, 16)), nil
		}
		if name == "int32" || name == "rune" {
			return intNode(signExtendBits(raw, 32)), nil
		}
		if name == "int64" || name == "int" {
			return intNode(signExtendBits(raw, wordBits)), nil
		}
		if name == "uint16" {
			return intNode(int64(maskBits(raw, 16))), nil
		}
		if name == "uint32" {
			return intNode(int64(maskBits(raw, 32))), nil
		}
		if name == "uint64" || name == "uint" || name == "uintptr" {
			return intNode(int64(maskBits(raw, wordBits))), nil
		}
		// Named non-builtin type: decode via alias if possible.
		if aliasNode != typeNode {
			return c.decodeComptimeValue(raw, aliasNode, eval, depth+1)
		}
	}

	if aliasNode.Kind == NSliceType {
		if raw == 0 {
			return &Node{Kind: NBasicLit, Name: "nil"}, nil
		}
		ws := uint64(vm.EvalWordSize(eval))
		dataPtr := vm.EvalLoadWord(eval, raw)
		slen := int(vm.EvalLoadWord(eval, raw+ws))
		elemSize := int(vm.EvalLoadWord(eval, raw+3*ws))
		var elems []*Node
		i := 0
		for i < slen {
			var elemRaw uint64
			if elemSize == 1 {
				bs, err := vm.EvalLoadBytes(eval, dataPtr+uint64(i), 1)
				if err != nil {
					return nil, err
				}
				elemRaw = uint64(bs[0])
			} else {
				elemRaw = vm.EvalLoadWord(eval, dataPtr+uint64(i*elemSize))
			}
			ev, err := c.decodeComptimeValue(elemRaw, aliasNode.X, eval, depth+1)
			if err != nil {
				return nil, err
			}
			elems = append(elems, ev)
			i = i + 1
		}
		litType := typeNode
		if litType == nil || litType.Kind != NSliceType {
			litType = aliasNode
		}
		return &Node{Kind: NCompositeLit, Type: cloneTypeNode(litType), Nodes: elems}, nil
	}

	if aliasNode.Kind == NMapType {
		if raw == 0 {
			return &Node{Kind: NBasicLit, Name: "nil"}, nil
		}
		ws := uint64(vm.EvalWordSize(eval))
		dataPtr := vm.EvalLoadWord(eval, raw)
		mlen := int(vm.EvalLoadWord(eval, raw+ws))
		entrySize := int(2 * ws)
		var elems []*Node
		i := 0
		for i < mlen {
			entryAddr := dataPtr + uint64(i*entrySize)
			keyRaw := vm.EvalLoadWord(eval, entryAddr)
			valRaw := vm.EvalLoadWord(eval, entryAddr+ws)
			keyNode, err := c.decodeComptimeValue(keyRaw, aliasNode.X, eval, depth+1)
			if err != nil {
				return nil, err
			}
			valNode, err := c.decodeComptimeValue(valRaw, aliasNode.Y, eval, depth+1)
			if err != nil {
				return nil, err
			}
			elems = append(elems, &Node{Kind: NKeyValue, X: keyNode, Y: valNode})
			i = i + 1
		}
		litType := typeNode
		if litType == nil || litType.Kind != NMapType {
			litType = aliasNode
		}
		return &Node{Kind: NCompositeLit, Type: cloneTypeNode(litType), Nodes: elems}, nil
	}

	structType := aliasNode
	litType := typeNode
	if aliasNode.Kind == NStructType {
		// keep litType as declared type node so codegen emits named composite constructors.
		if typeNode.Kind != NIdent && typeNode.Kind != NSelectorExpr {
			litType = aliasNode
		}
	}
	if structType.Kind == NStructType {
		ws := uint64(vm.EvalWordSize(eval))
		var fields []*Node
		slot := 0
		for _, field := range structType.Nodes {
			if field == nil || field.Kind != NField {
				continue
			}
			var fieldRaw uint64
			if raw != 0 {
				fieldRaw = vm.EvalLoadWord(eval, raw+uint64(slot)*ws)
			}
			fv, err := c.decodeComptimeValue(fieldRaw, field.Type, eval, depth+1)
			if err != nil {
				return nil, err
			}
			fields = append(fields, fv)
			slot = slot + 1
		}
		return &Node{Kind: NCompositeLit, Type: cloneTypeNode(litType), Nodes: fields}, nil
	}

	if aliasNode.Kind == NPointerType {
		return intNode(int64(raw)), nil
	}

	return intNode(int64(raw)), nil
}

// qualifyTypeName qualifies a type name with a package path if not already qualified.
func (c *Compiler) qualifyTypeName(typeName string, pkgPath string) string {
	if typeName == "" || typeName == "string" || typeName == "int" || typeName == "bool" || typeName == "byte" || typeName == "error" || typeName == "interface{}" {
		return typeName
	}
	// Unqualified names (pkgPath=="") are resolved relative to c.curPkg.
	// Include that context in the cache key to avoid cross-package collisions
	// (e.g. "*CodeGen" in backend/i386 vs backend/x64).
	cachePkg := pkgPath
	if cachePkg == "" && c.curPkg != nil {
		cachePkg = c.curPkg.Path
	}
	cacheKey := typeName + "\x00" + cachePkg
	if cached, ok := c.qualifyTypeCache[cacheKey]; ok {
		return cached
	}
	result := c.qualifyTypeNameInner(typeName, pkgPath)
	c.qualifyTypeCache[cacheKey] = result
	return result
}

func (c *Compiler) qualifyTypeNameInner(typeName string, pkgPath string) string {
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
		// Check if inner is already qualified (e.g. "*os.File" → "os.*File")
		j := 0
		for j < len(inner) {
			if inner[j] == '.' {
				pkgAlias := inner[0:j]
				typePart := inner[j+1 : len(inner)]
				// Resolve package alias to full path
				impPkg := c.resolvePackage(pkgAlias)
				if impPkg != nil {
					return impPkg.QualPtrName(typePart)
				}
				return pkgAlias + ".*" + typePart
			}
			j++
		}
		if pkgPath != "" {
			if resolvedPkg, ok := c.mod.Packages[pkgPath]; ok {
				return resolvedPkg.QualPtrName(inner)
			}
			return pkgPath + ".*" + inner
		}
		return c.curPkg.QualPtrName(inner)
	}
	// Already qualified (contains '.') — but might be an import alias, resolve it
	i := 0
	for i < len(typeName) {
		if typeName[i] == '.' {
			pkgAlias := typeName[0:i]
			rest := typeName[i+1 : len(typeName)]
			impPkg := c.resolvePackage(pkgAlias)
			if impPkg != nil {
				return impPkg.QualName(rest)
			}
			return typeName
		}
		i++
	}
	// Qualify with package
	if pkgPath != "" {
		if resolvedPkg, ok := c.mod.Packages[pkgPath]; ok {
			return resolvedPkg.QualName(typeName)
		}
		return c.dotJoin(pkgPath, typeName)
	}
	return c.curPkg.QualName(typeName)
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

func (c *Compiler) resolveMethodByConcreteType(concreteType string, methodName string) (string, bool) {
	candidate := c.dotJoin(concreteType, methodName)
	if resolved, ok := c.methodTable[candidate]; ok {
		return resolved, true
	}
	ptrCandidate := c.dotJoin(pointerMethodTypeName(concreteType), methodName)
	resolved, ok := c.methodTable[ptrCandidate]
	return resolved, ok
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

func isRuntimeMemBuiltinName(name string) bool {
	return name == "ReadPtr" || name == "WritePtr" || name == "WriteByte"
}

func (c *Compiler) resolveCallName(node *Node) string {
	if node == nil {
		return ""
	}
	if node.Kind == NIdent {
		if c.curPkg != nil && c.curPkg.Path == "runtime" && isRuntimeMemBuiltinName(node.Name) {
			return "runtime." + node.Name
		}
		// Check if it's a local variable (e.g. function literal)
		_, isLocal := c.lookupLocal(node.Name)
		if isLocal {
			return node.Name
		}
		// Check if it's a function or type in current package
		if _, ok := c.lookupCurrentTypeDecl(node.Name); ok {
			return c.curPkg.QualName(node.Name)
		}
		sym, ok := c.curPkg.Symbols[node.Name]
		if ok {
			if sym.Kind != SymFunc && sym.Kind != SymType {
				c.errorf("%s: %s is not callable (not a function or type)", c.curFunc.Name, node.Name)
			}
			return c.curPkg.QualName(node.Name)
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
				if pkg.Path == "runtime" && isRuntimeMemBuiltinName(node.Name) {
					return pkg.QualName(node.Name)
				}
				c.errorf("%s: %s.%s not found in package %s", c.curFunc.Name, node.X.Name, node.Name, pkg.Path)
			} else if sym.Kind != SymFunc && sym.Kind != SymType {
				c.errorf("%s: %s.%s is not callable", c.curFunc.Name, node.X.Name, node.Name)
			}
			return pkg.QualName(node.Name)
		}
		// Interface method call (e.g. err.Error()).
		if ifaceType, ok := c.localTypes[node.X.Name]; ok {
			if _, hasMethod := c.ifaceMethodReturnCount(ifaceType, node.Name); hasMethod {
				return c.dotJoin(ifaceType, node.Name)
			}
		}
		// Could be a method call — try to resolve using concrete type
		concreteType := ""
		if ct, ok := c.localConcreteTypes[node.X.Name]; ok {
			concreteType = ct
		} else {
			gqname := c.curPkg.QualName(node.X.Name)
			if ct, ok := c.globalConcreteTypes[gqname]; ok {
				concreteType = ct
			}
		}
		if concreteType != "" {
			if resolved, ok := c.resolveMethodByConcreteType(concreteType, node.Name); ok {
				return resolved
			}
		}
		if resolved, ok := c.findUniqueMethodByName(node.Name); ok {
			return resolved
		}
		c.errorf("%s: cannot resolve selector call %s.%s (unknown receiver type)", c.curFunc.Name, node.X.Name, node.Name)
		return "unknown"
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
					if resolved, ok := c.resolveMethodByConcreteType(fieldType, methodName); ok {
						return resolved
					}
				}
			}
		}
		if resolved, ok := c.findUniqueMethodByName(methodName); ok {
			return resolved
		}
		c.errorf("%s: cannot resolve selector call %s on chained receiver field %s", c.curFunc.Name, methodName, fieldName)
		return "unknown"
	}
	return "unknown"
}

func (c *Compiler) compileAppend(node *Node) {
	if len(node.Nodes) < 2 {
		return
	}
	// Determine element size from the slice argument
	elemSize := c.target.PtrSize // default: pointer-sized elements
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
		c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.SliceAppendSlice", Arg: 2})
	} else {
		// Append one element at a time, chaining the result
		i := 1
		for i < len(node.Nodes) {
			c.compileExpr(node.Nodes[i])
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(elemSize)})
			c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.SliceAppend", Arg: 3})
			i++
		}
	}
}

func (c *Compiler) compileCopy(node *Node) {
	if len(node.Nodes) < 2 {
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		return
	}
	// copy(dst, src) → runtime.SliceCopy(dst, src)
	c.compileExpr(node.Nodes[0])
	c.compileExpr(node.Nodes[1])
	c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.SliceCopy", Arg: 2})
}

func (c *Compiler) compileMake(node *Node) {
	// make([]T, len) or make([]T, len, cap) or make(map[K]V)
	if node.Nodes[0].Kind == NMapType {
		// Map creation: make(map[K]V)
		keyKind := c.mapKeyKind(node.Nodes[0].X)
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(keyKind)})
		c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.MapMake", Arg: 1})
		return
	}
	// Slice creation: make([]T, len) or make([]T, len, cap)
	if len(node.Nodes) >= 2 {
		c.compileExpr(node.Nodes[1]) // length
	} else {
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
	}
	elemSize := c.sliceElemSize(node.Nodes[0])
	if len(node.Nodes) >= 3 {
		c.compileExpr(node.Nodes[2]) // capacity
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(elemSize)})
		c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.SliceMakeCap", Arg: 3})
	} else {
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(elemSize)})
		c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.SliceMake", Arg: 2})
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
		// Interface method calls: use the declared interface method signature.
		if node.X != nil && node.X.Kind == NSelectorExpr && node.X.X != nil && node.X.X.Kind == NIdent {
			recvName := node.X.X.Name
			if ifaceType, ok := c.localTypes[recvName]; ok {
				if retCount, ok := c.ifaceMethodReturnCount(ifaceType, node.X.Name); ok {
					return retCount
				}
			}
		}
		// Look up the callee's return count (node.X is the callee)
		name := c.resolveCallName(node.X)
		if node.X != nil && node.X.Kind == NIdent {
			if target, ok := c.localFuncTargets[node.X.Name]; ok {
				name = target
			} else if target, ok := c.localMethodTargets[node.X.Name]; ok {
				name = target
			}
		}
		if ret, ok := runtimeMemBuiltinReturnCount(name); ok {
			return ret
		}
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
		qname := c.curPkg.QualName(node.Name)
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
	case NIndexExpr:
		// Chained indexing: e.g., matrix[i] where matrix is [][]int
		// Determine elem size of the result of indexing the base
		if node.X != nil {
			baseCT := c.exprConcreteType(node.X)
			if len(baseCT) > 2 && baseCT[0] == '[' && baseCT[1] == ']' {
				resultType := baseCT[2:len(baseCT)]
				if len(resultType) > 2 && resultType[0] == '[' && resultType[1] == ']' {
					return c.typeElemSize(resultType[2:len(resultType)])
				}
			}
		}
		return 1
	case NSelectorExpr:
		// pkg.Name — look up qualified global
		if node.X != nil && node.X.Kind == NIdent {
			pkg := c.resolvePackage(node.X.Name)
			if pkg != nil {
				qname := pkg.QualName(node.Name)
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
		return c.target.PtrSize
	}
	return 1
}

// sliceElemSize returns the element size for a slice type node.
func (c *Compiler) sliceElemSize(typeNode *Node) int {
	if typeNode == nil {
		return c.target.PtrSize
	}
	if typeNode.Kind == NSliceType && typeNode.X != nil {
		return c.typeElemSize(nodeTypeName(typeNode.X))
	}
	return c.target.PtrSize
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
			qname := pkg.QualName(node.Name)
			// Check if it's a precomputed constant
			if val, ok := c.constValues[qname]; ok {
				c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: val})
				return
			}
			// Check if it's a constant in the target package
			if sym, ok := pkg.Symbols[node.Name]; ok && sym.Kind == SymConst {
				val := c.resolveConstValue(sym.Node)
				c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: val})
				return
			}
			gidx, gok := c.globals[qname]
			if gok {
				c.emit(ir.Inst{Op: ir.OP_GLOBAL_GET, Arg: gidx})
				return
			}
			c.emit(ir.Inst{Op: ir.OP_GLOBAL_GET, Name: qname})
			return
		}
	}
	recvType := c.resolveExprType(node.X)
	if recvType == "" {
		c.errorf("%s: cannot resolve selector %s (unknown receiver type)", c.curFunc.Name, node.Name)
		c.emit(ir.Inst{Op: ir.OP_CONST_NIL})
		return
	}
	offsets, ok := c.resolveFieldPath(recvType, node.Name)
	if !ok || len(offsets) == 0 {
		c.errorf("%s: cannot resolve selector %s on %s", c.curFunc.Name, node.Name, recvType)
		c.emit(ir.Inst{Op: ir.OP_CONST_NIL})
		return
	}
	c.compileExpr(node.X)
	// Auto-deref pointer-to-struct for field access (e.g., pp.X where pp is *Point)
	if node.X != nil && c.needsSelectorDeref(node.X) {
		c.emit(ir.Inst{Op: ir.OP_LOAD, Arg: c.target.PtrSize})
	}
	i := 0
	for i < len(offsets) {
		c.emit(ir.Inst{Op: ir.OP_OFFSET, Arg: offsets[i]})
		c.emit(ir.Inst{Op: ir.OP_LOAD, Arg: c.target.PtrSize})
		i++
	}
}

type promotedMethodMatch struct {
	Offsets []int
	Target  string
}

func (c *Compiler) findPromotedMethodRec(qualifiedType string, methodName string, visited map[string]bool) (promotedMethodMatch, bool) {
	if qualifiedType == "" || visited[qualifiedType] {
		return promotedMethodMatch{}, false
	}
	visited[qualifiedType] = true

	typeNode, pkgPath := c.lookupStructTypeNode(qualifiedType)
	if typeNode == nil {
		return promotedMethodMatch{}, false
	}

	fieldIdx := 0
	for _, field := range typeNode.Nodes {
		if field.Kind != NField {
			continue
		}
		if field.Type != nil && field.Name == nodeTypeName(field.Type) {
			embeddedType := c.qualifyTypeName(nodeTypeName(field.Type), pkgPath)
			candidate := c.dotJoin(embeddedType, methodName)
			if resolved, ok := c.methodTable[candidate]; ok {
				return promotedMethodMatch{Offsets: []int{fieldIdx * c.target.PtrSize}, Target: resolved}, true
			}
			ptrCandidate := c.dotJoin(pointerMethodTypeName(embeddedType), methodName)
			if resolved, ok := c.methodTable[ptrCandidate]; ok {
				return promotedMethodMatch{Offsets: []int{fieldIdx * c.target.PtrSize}, Target: resolved}, true
			}
			if sub, ok := c.findPromotedMethodRec(embeddedType, methodName, visited); ok {
				offsets := []int{fieldIdx * c.target.PtrSize}
				for _, off := range sub.Offsets {
					offsets = append(offsets, off)
				}
				return promotedMethodMatch{Offsets: offsets, Target: sub.Target}, true
			}
		}
		fieldIdx++
	}
	return promotedMethodMatch{}, false
}

func (c *Compiler) findPromotedMethod(qualifiedType string, methodName string) (promotedMethodMatch, bool) {
	return c.findPromotedMethodRec(qualifiedType, methodName, make(map[string]bool))
}

func (c *Compiler) compileIndexExpr(node *Node) {
	// Check for map index read: m[key]
	if c.isMapExpr(node.X) {
		c.compileExpr(node.X)
		c.compileExpr(node.Y)
		c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.MapGet", Arg: 2})
		// MapGet returns (value, ok) — drop ok for single-value context
		// (multi-value context is handled in compileAssign)
		c.emit(ir.Inst{Op: ir.OP_DROP})
		return
	}
	elemSize := c.exprElemSize(node.X)
	c.compileExpr(node.X)
	c.compileExpr(node.Y)
	c.emit(ir.Inst{Op: ir.OP_INDEX_ADDR, Arg: elemSize})
	c.emit(ir.Inst{Op: ir.OP_LOAD, Arg: elemSize})
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
		qname := c.curPkg.QualName(node.Name)
		if kk, ok := c.globalMapVars[qname]; ok {
			return kk
		}
	}
	if node.Kind == NSelectorExpr && node.X != nil {
		if node.X.Kind == NIdent {
			pkg := c.resolvePackage(node.X.Name)
			if pkg != nil {
				qname := pkg.QualName(node.Name)
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
		qname := c.curPkg.QualName(node.Name)
		_, ok = c.globalMapVars[qname]
		return ok
	}
	// Check for pkg.mapVar or struct.mapField
	if node.Kind == NSelectorExpr && node.X != nil {
		if node.X.Kind == NIdent {
			pkg := c.resolvePackage(node.X.Name)
			if pkg != nil {
				qname := pkg.QualName(node.Name)
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
		c.emit(ir.Inst{Op: ir.OP_LEN})
	}

	// Use StringSlice for string-typed targets, SliceReslice for slices
	if c.isStringTypedExpr(node.X) {
		c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.StringSlice", Arg: 3})
	} else if node.Type != nil {
		c.compileExpr(node.Type)
		c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.SliceResliceFull", Arg: 4})
	} else {
		c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.SliceReslice", Arg: 3})
	}
}

func (c *Compiler) compileCompositeLit(node *Node) {
	// Handle map composite literals: map[K]V{k1: v1, k2: v2, ...}
	if node.Type != nil && node.Type.Kind == NMapType {
		keyKind := c.mapKeyKind(node.Type.X)
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(keyKind)})
		c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.MapMake", Arg: 1})
		// For each key-value pair, call MapSet
		for _, elem := range node.Nodes {
			if elem.Kind == NKeyValue {
				// Stack: map_hdr
				// Dup map header, push key, push value, call MapSet
				c.emit(ir.Inst{Op: ir.OP_DUP})
				c.compileExpr(elem.X)
				c.compileExpr(elem.Y)
				c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.MapSet", Arg: 3})
				c.emit(ir.Inst{Op: ir.OP_DROP}) // drop the returned header (same as input)
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
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(elemSize)})
			c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.SliceMake", Arg: 2})
		} else {
			// Build slice by appending each element
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0}) // nil slice
			for _, elem := range node.Nodes {
				c.compileExpr(elem) // push element value
				c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(elemSize)})
				c.emit(ir.Inst{Op: ir.OP_CALL, Name: "runtime.SliceAppend", Arg: 3})
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
					c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				}
			}
			c.emit(ir.Inst{Op: ir.OP_CALL, Name: "builtin.composite." + typeName, Arg: len(structFields)})
		} else {
			// Fallback: push values in literal order
			for _, elem := range node.Nodes {
				if elem.Kind == NKeyValue {
					c.compileExpr(elem.Y)
				} else {
					c.compileExpr(elem)
				}
			}
			c.emit(ir.Inst{Op: ir.OP_CALL, Name: "builtin.composite." + typeName, Arg: len(node.Nodes)})
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
				c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				i++
			}
			c.emit(ir.Inst{Op: ir.OP_CALL, Name: "builtin.composite." + typeName, Arg: nfields})
		} else {
			// Positional: push values in literal order
			for _, elem := range node.Nodes {
				c.compileExpr(elem)
			}
			c.emit(ir.Inst{Op: ir.OP_CALL, Name: "builtin.composite." + typeName, Arg: len(node.Nodes)})
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
	case NInterfaceType:
		return "interface{}"
	}
	return ""
}

func parseIntLiteral(s string) int64 {
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		return parseHexLiteral(s[2:len(s)])
	}
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'b' || s[1] == 'B') {
		return parseBaseLiteral(s[2:len(s)], 2)
	}
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'o' || s[1] == 'O') {
		return parseBaseLiteral(s[2:len(s)], 8)
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
			if s[i] == '_' {
				i++
				continue
			}
			result = result*8 + int64(s[i]-'0')
			i++
		}
	} else {
		for i < len(s) {
			if s[i] == '_' {
				i++
				continue
			}
			result = result*10 + int64(s[i]-'0')
			i++
		}
	}
	if neg {
		result = 0 - result
	}
	return result
}

func parseBaseLiteral(s string, base int64) int64 {
	var result int64
	i := 0
	for i < len(s) {
		ch := s[i]
		if ch == '_' {
			i++
			continue
		}
		d := int64(0)
		if ch >= '0' && ch <= '9' {
			d = int64(ch - '0')
		} else if ch >= 'a' && ch <= 'f' {
			d = int64(ch-'a') + 10
		} else if ch >= 'A' && ch <= 'F' {
			d = int64(ch-'A') + 10
		}
		result = result*base + d
		i++
	}
	return result
}

func parseHexLiteral(s string) int64 {
	var result int64
	i := 0
	for i < len(s) {
		ch := s[i]
		if ch == '_' {
			i++
			continue
		}
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
	// Decode UTF-8 leading rune.
	b0 := s[0]
	if b0 < 0x80 {
		return int(b0)
	}
	if (b0&0xE0) == 0xC0 && len(s) >= 2 {
		return int(b0&0x1F)<<6 | int(s[1]&0x3F)
	}
	if (b0&0xF0) == 0xE0 && len(s) >= 3 {
		return int(b0&0x0F)<<12 | int(s[1]&0x3F)<<6 | int(s[2]&0x3F)
	}
	if (b0&0xF8) == 0xF0 && len(s) >= 4 {
		return int(b0&0x07)<<18 | int(s[1]&0x3F)<<12 | int(s[2]&0x3F)<<6 | int(s[3]&0x3F)
	}
	return int(b0)
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
			buf = append(buf, '\\', 'x', common.HexDigit(ch>>4), common.HexDigit(ch&0x0f))
		} else {
			buf = append(buf, ch)
		}
		i++
	}
	return string(buf)
}
