package frontend

import (
	"fmt"
	"strings"

	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// Unit is a single preprocessed/parsed C translation unit.
type Unit struct {
	File string
	Root *Node
}

type cFuncSig struct {
	Name             string
	IRName           string
	RetCount         int
	RetByPtr         bool
	RetKind          cDeclKind
	RetBase          cScalarType
	RetPtrDepth      int
	RetOpaque        bool
	RetAggKeyword    string
	RetAggTag        string
	ParamCount       int
	ParamUnspecified bool
	Variadic         bool
	ParamNames       []string
	ParamKinds       []cDeclKind
	ParamBases       []cScalarType
	ParamPtrDepth    []int
	ParamOpaque      []bool
	ParamAggKey      []string
	ParamAggTag      []string
	ParamFuncSigs    []*cFuncTypeSig
	Defined          bool
	Body             *Node
	File             string
	Line             int
	Col              int
}

type cFuncTypeSig struct {
	RetKind          cDeclKind
	RetBase          cScalarType
	RetPtrDepth      int
	RetIsVoid        bool
	RetByPtr         bool
	RetOpaque        bool
	RetAggKeyword    string
	RetAggTag        string
	ParamCount       int
	ParamUnspecified bool
	Variadic         bool
	ParamKinds       []cDeclKind
	ParamBases       []cScalarType
	ParamPtrDepth    []int
	ParamOpaque      []bool
	ParamAggKey      []string
	ParamAggTag      []string
	ParamFuncSigs    []*cFuncTypeSig
}

type cDeclItem struct {
	Name              string
	Init              []Token
	Kind              cDeclKind
	DirectFunc        bool
	PtrDepth          int
	ArrayLen          int64
	ArrayDims         []int64
	IsVoid            bool
	Base              cScalarType
	FuncSig           *cFuncTypeSig
	OpaqueAggregate   bool
	AggregateKeyword  string
	AggregateTag      string
	RuntimeArrayBound []Token
	RuntimeArrayElem  cTypeInfo
}

type cDeclaratorSuffix struct {
	IsFunction       bool
	ArrayLen         int64
	ParamKinds       []cDeclKind
	ParamUnspecified bool
	ParamBases       []cScalarType
	ParamPtrDepth    []int
	ParamOpaque      []bool
	ParamAggKey      []string
	ParamAggTag      []string
	ParamFuncSigs    []*cFuncTypeSig
	Variadic         bool
}

type cDeclaratorNode struct {
	Name      string
	PtrPrefix int
	Inner     *cDeclaratorNode
	Suffixes  []cDeclaratorSuffix
}

type cDeclaratorEntityKind int

const (
	cDeclaratorEntityBase cDeclaratorEntityKind = iota
	cDeclaratorEntityPointer
	cDeclaratorEntityArray
	cDeclaratorEntityFunction
)

type cDeclaratorEntity struct {
	Kind             cDeclaratorEntityKind
	Inner            *cDeclaratorEntity
	ArrayLen         int64
	ParamKinds       []cDeclKind
	ParamUnspecified bool
	ParamBases       []cScalarType
	ParamPtr         []int
	ParamOpaque      []bool
	ParamAggKey      []string
	ParamAggTag      []string
	ParamFuncs       []*cFuncTypeSig
	Variadic         bool
}

type cGlobalInit struct {
	Name             string
	Index            int
	Kind             cDeclKind
	PtrDepth         int
	ArrayBase        int
	ArrayLen         int64
	ArrayDims        []int64
	ObjectBase       int
	ObjectWords      int64
	AggregateKeyword string
	AggregateTag     string
	Base             cScalarType
	Init             []Token
	File             string
	Line             int
	Col              int
	IRName           string
}

type cDeclKind int

const (
	cDeclScalar cDeclKind = iota
	cDeclPointer
	cDeclArray
)

const cArrayLenUnspecified int64 = -1

type cScalarType int

const (
	cScalarInt cScalarType = iota
	cScalarUInt
	cScalarChar
	cScalarUChar
	cScalarShort
	cScalarUShort
	cScalarLong
	cScalarULong
	cScalarFloat
	cScalarDouble
)

type cTypeInfo struct {
	Kind             cDeclKind
	PtrDepth         int
	ArrayLen         int64
	ArrayDims        []int64
	IsVoid           bool
	Base             cScalarType
	FuncSig          *cFuncTypeSig
	OpaqueAggregate  bool
	AggregateKeyword string
	AggregateTag     string
}

type cAggregateField struct {
	Name string
	Type cTypeInfo

	Offset int64
	Size   int64
	Align  int64

	BitfieldWidth int64
}

type cAggregateInfo struct {
	Keyword string
	Tag     string
	IsUnion bool

	Size  int64
	Align int64

	Fields []cAggregateField
}

func isAggregateObjectType(info cTypeInfo) bool {
	return isAggregateObjectDecl(info.Kind, info.PtrDepth, info.OpaqueAggregate, info.AggregateKeyword, info.AggregateTag)
}

func isAggregateObjectDecl(kind cDeclKind, ptrDepth int, opaqueAggregate bool, aggregateKeyword string, aggregateTag string) bool {
	return kind == cDeclScalar &&
		ptrDepth == 0 &&
		!opaqueAggregate &&
		aggregateKeyword != "" &&
		aggregateTag != ""
}

var cTypedefLookupCompiler *compiler
var cTypedefLookupFunc *funcCompiler
var cAggregateLookupCompiler *compiler
var cAggregateLookupFunc *funcCompiler
var cConstSizeLookupCompiler *compiler
var cConstSizeLookupFunc *funcCompiler
var cConstSizeLookupDecl map[string]int64
var cAnonAggregateSeq int64

func lookupTypedefAlias(name string) (cTypeInfo, bool) {
	if cTypedefLookupFunc != nil {
		if info, ok := cTypedefLookupFunc.lookupTypedef(name); ok {
			return info, true
		}
	}
	if cTypedefLookupCompiler != nil {
		return cTypedefLookupCompiler.lookupTypedef(name)
	}
	return cTypeInfo{}, false
}

func lookupBuiltinTypedefAlias(name string) (cTypeInfo, bool) {
	switch name {
	case "__builtin_va_list":
		return cTypeInfo{Kind: cDeclPointer, PtrDepth: 1, IsVoid: true}, true
	case "int8_t":
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarChar}, true
	case "uint8_t":
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarUChar}, true
	case "int16_t":
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarShort}, true
	case "uint16_t":
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarUShort}, true
	case "int32_t":
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarInt}, true
	case "uint32_t":
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarUInt}, true
	case "int64_t":
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarLong}, true
	case "uint64_t":
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarULong}, true
	default:
		return cTypeInfo{}, false
	}
}

func lookupConstObjectSize(name string) (int64, bool) {
	if cConstSizeLookupFunc != nil {
		if n, ok := cConstSizeLookupFunc.lookupConstObjectSize(name); ok {
			return n, true
		}
	}
	if cConstSizeLookupDecl != nil {
		if n, ok := cConstSizeLookupDecl[name]; ok {
			return n, true
		}
	}
	if cConstSizeLookupCompiler != nil {
		return cConstSizeLookupCompiler.lookupConstObjectSize(name)
	}
	return 0, false
}

func lookupAggregateAlias(keyword string, tag string) (*cAggregateInfo, bool) {
	if cAggregateLookupFunc != nil {
		if info, ok := cAggregateLookupFunc.lookupAggregate(keyword, tag); ok {
			return info, true
		}
	}
	if cAggregateLookupCompiler != nil {
		return cAggregateLookupCompiler.lookupAggregate(keyword, tag)
	}
	return nil, false
}

func registerAggregateAlias(info *cAggregateInfo) error {
	if cAggregateLookupFunc != nil {
		return cAggregateLookupFunc.registerAggregate(info)
	}
	if cAggregateLookupCompiler != nil {
		return cAggregateLookupCompiler.registerAggregate(info)
	}
	return nil
}

func nextAnonAggregateTag(keyword string) string {
	cAnonAggregateSeq++
	return fmt.Sprintf("$anon_%s_%d", keyword, cAnonAggregateSeq)
}

func aggregateTypeKey(keyword string, tag string) string {
	return keyword + ":" + tag
}

func currentCTargetPtrSize() int64 {
	ptrSize := int64(8)
	if cAggregateLookupFunc != nil && cAggregateLookupFunc.c != nil && cAggregateLookupFunc.c.target != nil {
		ptrSize = int64(cAggregateLookupFunc.c.target.PtrSize)
	} else if cAggregateLookupCompiler != nil && cAggregateLookupCompiler.target != nil {
		ptrSize = int64(cAggregateLookupCompiler.target.PtrSize)
	}
	return ptrSize
}

func currentCTargetLongSize() int64 {
	longSize := int64(8)
	if cAggregateLookupFunc != nil && cAggregateLookupFunc.c != nil && cAggregateLookupFunc.c.target != nil {
		if cAggregateLookupFunc.c.target.GOOS == "windows" {
			longSize = 4
		} else {
			longSize = int64(cAggregateLookupFunc.c.target.PtrSize)
		}
	} else if cAggregateLookupCompiler != nil && cAggregateLookupCompiler.target != nil {
		if cAggregateLookupCompiler.target.GOOS == "windows" {
			longSize = 4
		} else {
			longSize = int64(cAggregateLookupCompiler.target.PtrSize)
		}
	}
	return longSize
}

func alignTo(n int64, align int64) int64 {
	if align <= 1 {
		return n
	}
	rem := n % align
	if rem == 0 {
		return n
	}
	return n + (align - rem)
}

type cIntrinsicWrapper struct {
	IRName   string
	Params   int
	RetCount int
}

type cLocalBinding struct {
	Index            int
	Kind             cDeclKind
	PtrDepth         int
	ElemStep         int64
	ArrayLen         int64
	ArrayDims        []int64
	Base             cScalarType
	FuncSig          *cFuncTypeSig
	OpaqueAggregate  bool
	AggregateKeyword string
	AggregateTag     string
}

type cUserLabel struct {
	Target int
	Line   int
	Col    int
}

// CompileUnits lowers parsed C units to RTG IR.
//
// Current scope intentionally targets a small executable subset:
// int/void functions, local/global int declarations, arithmetic/comparison,
// assignments, if/while/for/do, returns, and direct function calls.
func CompileUnits(target common.Target, units []Unit) (*ir.IRModule, []string) {
	c := &compiler{
		target: &target,
		units:  units,
		irmod: &ir.IRModule{
			EntryFunc:       ir.DefaultEntryFunc,
			FuncABIs:        make(map[string]string),
			FuncRetCounts:   make(map[string]int),
			TypeIDs:         make(map[string]int),
			MethodTable:     make(map[string]string),
			IfaceMethods:    make(map[string][]string),
			IfaceMethodRets: make(map[string]int),
		},
		funcs:           make(map[string]*cFuncSig),
		globalIndex:     make(map[string]int),
		globalHasInit:   make(map[string]bool),
		globalKind:      make(map[string]cDeclKind),
		globalPtrDepth:  make(map[string]int),
		globalElemStep:  make(map[string]int64),
		globalArray:     make(map[string]int64),
		globalArrayDims: make(map[string][]int64),
		globalBase:      make(map[string]cScalarType),
		globalFunc:      make(map[string]*cFuncTypeSig),
		globalOpaque:    make(map[string]bool),
		globalAggKey:    make(map[string]string),
		globalAggTag:    make(map[string]string),
		aggregateTags:   make(map[string]*cAggregateInfo),
		enumConsts:      make(map[string]int64),
		typedefs:        make(map[string]cTypeInfo),
		funcIDs:         make(map[string]int64),
		intrinsics:      make(map[string]cIntrinsicWrapper),
		externFns:       make(map[string]string),
		externDataFns:   make(map[string]string),
		externDataTypes: make(map[string]cTypeInfo),
		nextLabelSeq:    1,
	}
	prevTypedefLookupCompiler := cTypedefLookupCompiler
	prevTypedefLookupFunc := cTypedefLookupFunc
	prevAggregateLookupCompiler := cAggregateLookupCompiler
	prevAggregateLookupFunc := cAggregateLookupFunc
	prevConstSizeLookupCompiler := cConstSizeLookupCompiler
	prevConstSizeLookupFunc := cConstSizeLookupFunc
	cTypedefLookupCompiler = c
	cTypedefLookupFunc = nil
	cAggregateLookupCompiler = c
	cAggregateLookupFunc = nil
	cConstSizeLookupCompiler = c
	cConstSizeLookupFunc = nil

	c.collectTopLevel()
	if len(c.errors) > 0 {
		cTypedefLookupCompiler = prevTypedefLookupCompiler
		cTypedefLookupFunc = prevTypedefLookupFunc
		cAggregateLookupCompiler = prevAggregateLookupCompiler
		cAggregateLookupFunc = prevAggregateLookupFunc
		cConstSizeLookupCompiler = prevConstSizeLookupCompiler
		cConstSizeLookupFunc = prevConstSizeLookupFunc
		return nil, c.errors
	}
	c.assignFunctionIDs()
	c.registerNativeFunctionMetadata()

	c.finalizeTentativeGlobals()
	c.emitGlobalInit()
	for _, sig := range c.funcOrder {
		if !sig.Defined {
			continue
		}
		c.compileFunction(sig)
	}
	if !c.objectMode() {
		c.emitEntryWrapper()
	}

	if len(c.errors) > 0 {
		cTypedefLookupCompiler = prevTypedefLookupCompiler
		cTypedefLookupFunc = prevTypedefLookupFunc
		cAggregateLookupCompiler = prevAggregateLookupCompiler
		cAggregateLookupFunc = prevAggregateLookupFunc
		cConstSizeLookupCompiler = prevConstSizeLookupCompiler
		cConstSizeLookupFunc = prevConstSizeLookupFunc
		return nil, c.errors
	}
	cTypedefLookupCompiler = prevTypedefLookupCompiler
	cTypedefLookupFunc = prevTypedefLookupFunc
	cAggregateLookupCompiler = prevAggregateLookupCompiler
	cAggregateLookupFunc = prevAggregateLookupFunc
	cConstSizeLookupCompiler = prevConstSizeLookupCompiler
	cConstSizeLookupFunc = prevConstSizeLookupFunc
	return c.irmod, nil
}

type compiler struct {
	target *common.Target
	units  []Unit
	irmod  *ir.IRModule

	errors []string

	funcs     map[string]*cFuncSig
	funcOrder []*cFuncSig

	globalIndex     map[string]int
	globalHasInit   map[string]bool
	globalKind      map[string]cDeclKind
	globalPtrDepth  map[string]int
	globalElemStep  map[string]int64
	globalArray     map[string]int64
	globalArrayDims map[string][]int64
	globalBase      map[string]cScalarType
	globalFunc      map[string]*cFuncTypeSig
	globalOpaque    map[string]bool
	globalAggKey    map[string]string
	globalAggTag    map[string]string
	aggregateTags   map[string]*cAggregateInfo
	enumConsts      map[string]int64
	globalInits     []cGlobalInit
	typedefs        map[string]cTypeInfo
	funcIDs         map[string]int64

	intrinsics map[string]cIntrinsicWrapper
	externFns  map[string]string
	externDataFns   map[string]string
	externDataTypes map[string]cTypeInfo

	nextLabelSeq int
}

func (c *compiler) objectMode() bool {
	return c != nil && c.target != nil && c.target.RelocatableObject
}

func (c *compiler) cSymbolName(name string) string {
	if c.objectMode() {
		return name
	}
	return "c." + name
}

func (c *compiler) nativeFunctionABITag() string {
	if c == nil || c.target == nil || c.target.Backend != "native" {
		return ""
	}
	if c.target.GOOS == "darwin" && c.target.GOARCH == "arm64" {
		return "native-c-darwin-arm64"
	}
	if c.target.GOOS == "linux" && c.target.GOARCH == "386" && c.target.RelocatableObject {
		return "native-c-linux-386"
	}
	return ""
}

func (c *compiler) registerNativeFunctionMetadata() {
	if c.irmod == nil {
		return
	}
	if c.irmod.FuncABIs == nil {
		c.irmod.FuncABIs = make(map[string]string)
	}
	if c.irmod.FuncRetCounts == nil {
		c.irmod.FuncRetCounts = make(map[string]int)
	}
	abiTag := c.nativeFunctionABITag()
	if abiTag == "" {
		return
	}
	for _, sig := range c.funcOrder {
		if sig == nil || sig.Variadic {
			continue
		}
		if !sig.Defined && !c.objectMode() {
			continue
		}
		c.irmod.FuncABIs[sig.IRName] = abiTag
		c.irmod.FuncRetCounts[sig.IRName] = sig.RetCount
	}
}

func (c *compiler) errorf(file string, line int, col int, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if file != "" && line > 0 {
		msg = fmt.Sprintf("%s:%d:%d: %s", file, line, col, msg)
	}
	c.errors = append(c.errors, msg)
}

func (c *compiler) nextLabel() int {
	v := c.nextLabelSeq
	c.nextLabelSeq++
	return v
}

func (c *compiler) lookupTypedef(name string) (cTypeInfo, bool) {
	info, ok := c.typedefs[name]
	return info, ok
}

func (c *compiler) globalTypeInfo(name string) (cTypeInfo, bool) {
	kind, ok := c.globalKind[name]
	if !ok {
		return cTypeInfo{}, false
	}
	info := cTypeInfo{
		Kind:             kind,
		PtrDepth:         c.globalPtrDepth[name],
		Base:             c.globalBase[name],
		FuncSig:          cloneFuncTypeSig(c.globalFunc[name]),
		OpaqueAggregate:  c.globalOpaque[name],
		AggregateKeyword: c.globalAggKey[name],
		AggregateTag:     c.globalAggTag[name],
	}
	if kind == cDeclArray {
		info.ArrayLen = c.globalArray[name]
		info.ArrayDims = cloneInt64s(c.globalArrayDims[name])
	}
	return info, true
}

func cloneAggregateInfo(in *cAggregateInfo) *cAggregateInfo {
	if in == nil {
		return nil
	}
	out := &cAggregateInfo{
		Keyword: in.Keyword,
		Tag:     in.Tag,
		IsUnion: in.IsUnion,
		Size:    in.Size,
		Align:   in.Align,
	}
	if len(in.Fields) > 0 {
		out.Fields = make([]cAggregateField, len(in.Fields))
		i := 0
		for i < len(in.Fields) {
			f := in.Fields[i]
			out.Fields[i] = cAggregateField{
				Name:   f.Name,
				Type:   f.Type,
				Offset: f.Offset,
				Size:   f.Size,
				Align:  f.Align,
			}
			out.Fields[i].Type.FuncSig = cloneFuncTypeSig(f.Type.FuncSig)
			i++
		}
	}
	return out
}

func (c *compiler) lookupAggregate(keyword string, tag string) (*cAggregateInfo, bool) {
	if keyword == "" || tag == "" {
		return nil, false
	}
	key := aggregateTypeKey(keyword, tag)
	v, ok := c.aggregateTags[key]
	if !ok {
		return nil, false
	}
	return cloneAggregateInfo(v), true
}

func aggregateInfosCompatible(a *cAggregateInfo, b *cAggregateInfo) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Keyword != b.Keyword || a.Tag != b.Tag || a.IsUnion != b.IsUnion {
		return false
	}
	if len(a.Fields) != len(b.Fields) {
		return false
	}
	if a.Size != b.Size || a.Align != b.Align {
		return false
	}
	i := 0
	for i < len(a.Fields) {
		af := a.Fields[i]
		bf := b.Fields[i]
		if af.Name != bf.Name || af.Offset != bf.Offset || af.Size != bf.Size || af.Align != bf.Align {
			return false
		}
		if af.Type.Kind != bf.Type.Kind ||
			af.Type.PtrDepth != bf.Type.PtrDepth ||
			af.Type.ArrayLen != bf.Type.ArrayLen ||
			af.Type.IsVoid != bf.Type.IsVoid ||
			af.Type.Base != bf.Type.Base ||
			af.Type.OpaqueAggregate != bf.Type.OpaqueAggregate ||
			af.Type.AggregateKeyword != bf.Type.AggregateKeyword ||
			af.Type.AggregateTag != bf.Type.AggregateTag {
			return false
		}
		if !funcTypeSigEqual(af.Type.FuncSig, bf.Type.FuncSig) {
			return false
		}
		i++
	}
	return true
}

func (c *compiler) registerAggregate(info *cAggregateInfo) error {
	if info == nil || info.Keyword == "" || info.Tag == "" {
		return nil
	}
	key := aggregateTypeKey(info.Keyword, info.Tag)
	newInfo := cloneAggregateInfo(info)
	if prev, ok := c.aggregateTags[key]; ok {
		prevHasFields := len(prev.Fields) > 0
		newHasFields := len(newInfo.Fields) > 0
		if prevHasFields && newHasFields {
			if !aggregateInfosCompatible(prev, newInfo) {
				return fmt.Errorf("conflicting %s definition for %q", info.Keyword, info.Tag)
			}
			return nil
		}
		if prevHasFields {
			return nil
		}
		if !newHasFields {
			return nil
		}
	}
	c.aggregateTags[key] = newInfo
	return nil
}

func (c *compiler) pointerElemStep(kind cDeclKind, ptrDepth int, base cScalarType, isVoid bool, opaqueAggregate bool, aggregateKeyword string, aggregateTag string) int64 {
	if kind != cDeclPointer {
		return int64(c.target.PtrSize)
	}
	if ptrDepth > 1 {
		return int64(c.target.PtrSize)
	}
	if isVoid {
		return 1
	}
	if aggregateKeyword != "" && aggregateTag != "" {
		if !opaqueAggregate {
			if agg, ok := c.lookupAggregate(aggregateKeyword, aggregateTag); ok && len(agg.Fields) > 0 {
				return agg.Size
			}
		}
		return int64(c.target.PtrSize)
	}
	return cScalarSizeForType(base)
}

func (c *compiler) lookupEnumConst(name string) (int64, bool) {
	v, ok := c.enumConsts[name]
	return v, ok
}

func (c *compiler) lookupConstObjectSize(name string) (int64, bool) {
	info, ok := c.globalTypeInfo(name)
	if !ok {
		return 0, false
	}
	size, _, err := cTypeLayout(info)
	if err != nil {
		return 0, false
	}
	return size, true
}

func (c *compiler) addGlobalEnumConst(name string, val int64, file string, line int, col int) {
	if _, exists := c.enumConsts[name]; exists {
		c.errorf(file, line, col, "duplicate enum constant %q", name)
		return
	}
	c.enumConsts[name] = val
}

func (c *compiler) addGlobalEnumConsts(vals map[string]int64, file string, line int, col int) {
	for name, val := range vals {
		c.addGlobalEnumConst(name, val, file, line, col)
	}
}

func (c *compiler) finalizeTentativeGlobals() {
	for name, kind := range c.globalKind {
		if kind != cDeclArray {
			continue
		}
		if c.globalArray[name] != cArrayLenUnspecified {
			continue
		}
		c.globalArray[name] = 1
		if len(c.globalArrayDims[name]) > 0 {
			dims := cloneInt64s(c.globalArrayDims[name])
			dims[0] = 1
			c.globalArrayDims[name] = dims
		}
		for i := range c.globalInits {
			if c.globalInits[i].Name != name || c.globalInits[i].Kind != cDeclArray {
				continue
			}
			c.globalInits[i].ArrayLen = 1
			if len(c.globalInits[i].ArrayDims) > 0 {
				dims := cloneInt64s(c.globalInits[i].ArrayDims)
				dims[0] = 1
				c.globalInits[i].ArrayDims = dims
			}
		}
	}
}

func (c *compiler) assignFunctionIDs() {
	var next int64 = 1
	for _, sig := range c.funcOrder {
		if sig == nil || sig.Name == "" {
			continue
		}
		if _, exists := c.funcIDs[sig.Name]; exists {
			continue
		}
		c.funcIDs[sig.Name] = next
		next++
	}
}

func cloneFuncTypeSig(in *cFuncTypeSig) *cFuncTypeSig {
	if in == nil {
		return nil
	}
	out := &cFuncTypeSig{
		RetKind:          in.RetKind,
		RetBase:          in.RetBase,
		RetPtrDepth:      in.RetPtrDepth,
		RetIsVoid:        in.RetIsVoid,
		RetByPtr:         in.RetByPtr,
		RetOpaque:        in.RetOpaque,
		RetAggKeyword:    in.RetAggKeyword,
		RetAggTag:        in.RetAggTag,
		ParamCount:       in.ParamCount,
		ParamUnspecified: in.ParamUnspecified,
		Variadic:         in.Variadic,
	}
	out.ParamKinds = append([]cDeclKind{}, in.ParamKinds...)
	out.ParamBases = append([]cScalarType{}, in.ParamBases...)
	out.ParamPtrDepth = append([]int{}, in.ParamPtrDepth...)
	out.ParamOpaque = append([]bool{}, in.ParamOpaque...)
	out.ParamAggKey = append([]string{}, in.ParamAggKey...)
	out.ParamAggTag = append([]string{}, in.ParamAggTag...)
	if len(in.ParamFuncSigs) > 0 {
		out.ParamFuncSigs = make([]*cFuncTypeSig, len(in.ParamFuncSigs))
		for i, p := range in.ParamFuncSigs {
			out.ParamFuncSigs[i] = cloneFuncTypeSig(p)
		}
	}
	return out
}

func funcSigToTypeSig(sig *cFuncSig) *cFuncTypeSig {
	if sig == nil {
		return nil
	}
	out := &cFuncTypeSig{
		RetKind:          sig.RetKind,
		RetBase:          sig.RetBase,
		RetPtrDepth:      sig.RetPtrDepth,
		RetIsVoid:        sig.RetCount == 0 && !sig.RetByPtr,
		RetByPtr:         sig.RetByPtr,
		RetOpaque:        sig.RetOpaque,
		RetAggKeyword:    sig.RetAggKeyword,
		RetAggTag:        sig.RetAggTag,
		ParamCount:       sig.ParamCount,
		ParamUnspecified: sig.ParamUnspecified,
		Variadic:         sig.Variadic,
	}
	out.ParamKinds = append([]cDeclKind{}, sig.ParamKinds...)
	out.ParamBases = append([]cScalarType{}, sig.ParamBases...)
	out.ParamPtrDepth = append([]int{}, sig.ParamPtrDepth...)
	out.ParamOpaque = append([]bool{}, sig.ParamOpaque...)
	out.ParamAggKey = append([]string{}, sig.ParamAggKey...)
	out.ParamAggTag = append([]string{}, sig.ParamAggTag...)
	if len(sig.ParamFuncSigs) > 0 {
		out.ParamFuncSigs = make([]*cFuncTypeSig, len(sig.ParamFuncSigs))
		for i, p := range sig.ParamFuncSigs {
			out.ParamFuncSigs[i] = cloneFuncTypeSig(p)
		}
	}
	return out
}

func (c *compiler) ensureIntrinsicWrapper(name string, params int, retCount int) string {
	key := fmt.Sprintf("%s|%d|%d", name, params, retCount)
	if w, ok := c.intrinsics[key]; ok {
		return w.IRName
	}
	irName := fmt.Sprintf("c.intrinsic$%s$%d$%d", name, params, retCount)
	f := &ir.IRFunc{
		Name:     irName,
		Params:   params,
		RetCount: retCount,
	}
	f.Code = append(f.Code, ir.Inst{Op: ir.OP_CALL_INTRINSIC, Name: name, Arg: params})
	f.Code = append(f.Code, ir.Inst{Op: ir.OP_RETURN, Arg: retCount})
	c.irmod.Funcs = append(c.irmod.Funcs, f)
	c.intrinsics[key] = cIntrinsicWrapper{IRName: irName, Params: params, RetCount: retCount}
	return irName
}

func (c *compiler) ensureExternWrapper(name string, params int, retCount int) string {
	key := fmt.Sprintf("%s|%d|%d", name, params, retCount)
	if irName, ok := c.externFns[key]; ok {
		return irName
	}
	irName := fmt.Sprintf("c.extern$%s$%d$%d", name, params, retCount)
	intrinsic := fmt.Sprintf("c.extern.%s|%d|%d", name, params, retCount)
	if spec, ok := c.nativeExternLinkSpec(name, retCount); ok {
		if c.irmod.LinkStaticFuncs == nil {
			c.irmod.LinkStaticFuncs = make(map[string]string)
		}
		c.irmod.LinkStaticFuncs[intrinsic] = spec
	}
	f := &ir.IRFunc{
		Name:     irName,
		Params:   params,
		RetCount: retCount,
	}
	f.Code = append(f.Code, ir.Inst{Op: ir.OP_CALL_INTRINSIC, Name: intrinsic, Arg: params})
	f.Code = append(f.Code, ir.Inst{Op: ir.OP_RETURN, Arg: retCount})
	c.irmod.Funcs = append(c.irmod.Funcs, f)
	c.externFns[key] = irName
	return irName
}

func (c *compiler) ensureExternDataWrapper(name string, wantAddr bool) string {
	key := name + "|value"
	mode := "data"
	if wantAddr {
		key = name + "|addr"
		mode = "dataptr"
	}
	if irName, ok := c.externDataFns[key]; ok {
		return irName
	}
	spec, ok := c.nativeExternDataLinkSpec(name, wantAddr)
	if !ok {
		return ""
	}
	irName := fmt.Sprintf("c.externdata$%s$%s", name, mode)
	intrinsic := fmt.Sprintf("c.externdata.%s|%s", name, mode)
	if c.irmod.LinkStaticFuncs == nil {
		c.irmod.LinkStaticFuncs = make(map[string]string)
	}
	c.irmod.LinkStaticFuncs[intrinsic] = spec
	f := &ir.IRFunc{
		Name:     irName,
		Params:   0,
		RetCount: 1,
	}
	f.Code = append(f.Code, ir.Inst{Op: ir.OP_CALL_INTRINSIC, Name: intrinsic, Arg: 0})
	f.Code = append(f.Code, ir.Inst{Op: ir.OP_RETURN, Arg: 1})
	c.irmod.Funcs = append(c.irmod.Funcs, f)
	c.externDataFns[key] = irName
	return irName
}

func (c *compiler) nativeExternLinkSpec(name string, retCount int) (string, bool) {
	if c.target == nil || c.target.Backend == "c" {
		return "", false
	}
	mode := ""
	switch retCount {
	case 0:
		mode = "void"
	case 1:
		mode = "raw"
	default:
		return "", false
	}
	if name == "abort" || name == "exit" {
		mode = "noreturn"
	}
	if c.target.GOOS == "darwin" {
		switch name {
		case "abort", "exit", "close", "read", "write", "lseek", "unlink", "perror",
			"memset", "memmove", "bcopy", "bzero", "strcpy",
			"strncpy", "index":
			return "libSystem.dylib,_" + name + "," + mode, true
		case "open":
			return "libSystem.dylib,_open,rawvar2", true
		case "printf":
			return "libSystem.dylib,_printf,rawvar1", true
		case "fprintf":
			return "libSystem.dylib,_fprintf,rawvar2", true
		}
	}
	return "", false
}

func (c *compiler) nativeExternDataLinkSpec(name string, wantAddr bool) (string, bool) {
	if c.target == nil || c.target.Backend == "c" {
		return "", false
	}
	mode := "data"
	if wantAddr {
		mode = "dataptr"
	}
	if c.target.GOOS == "darwin" && c.target.GOARCH == "arm64" {
		switch name {
		case "stdin":
			return "libSystem.dylib,___stdinp," + mode, true
		case "stdout":
			return "libSystem.dylib,___stdoutp," + mode, true
		case "stderr":
			return "libSystem.dylib,___stderrp," + mode, true
		}
	}
	return "", false
}

func (c *compiler) canCallExternOnTarget(name string, retCount int) bool {
	if c.target != nil && c.target.Backend == "c" {
		return true
	}
	_, ok := c.nativeExternLinkSpec(name, retCount)
	return ok
}

func (c *compiler) canLoadExternDataOnTarget(name string) bool {
	_, ok := c.nativeExternDataLinkSpec(name, false)
	return ok
}

func encodeIRStringLiteral(raw string) string {
	var out []byte
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		switch ch {
		case '\\':
			out = append(out, '\\', '\\')
		case '"':
			out = append(out, '\\', '"')
		case '\n':
			out = append(out, '\\', 'n')
		case '\r':
			out = append(out, '\\', 'r')
		case '\t':
			out = append(out, '\\', 't')
		case 0:
			out = append(out, '\\', '0')
		default:
			if ch < 32 || ch >= 127 {
				out = append(out, '\\', 'x', common.HexDigit(ch>>4), common.HexDigit(ch&0x0f))
			} else {
				out = append(out, ch)
			}
		}
	}
	return string(out)
}

func (c *compiler) collectTopLevel() {
	for _, u := range c.units {
		if u.Root == nil {
			continue
		}
		for _, top := range u.Root.Children {
			switch top.Kind {
			case NFunctionDef:
				c.collectFunctionDef(u.File, top)
			case NExternalDecl:
				c.collectExternalDecl(u.File, top)
			}
		}
	}
}

func (c *compiler) collectFunctionDef(file string, n *Node) {
	toks, err := lexSnippet(file, n.Text)
	if err != nil {
		c.errorf(file, n.Line, n.Col, "invalid function header: %v", err)
		return
	}
	toks = trimLeadingDeclDecorators(toks)
	sig, err := parseFunctionSignature(file, n.Line, n.Col, toks)
	if err != nil {
		c.errorf(file, n.Line, n.Col, "%v", err)
		return
	}
	sig.IRName = c.cSymbolName(sig.Name)
	sig.Defined = true
	if len(n.Children) > 0 {
		sig.Body = n.Children[0]
	}

	if prev, ok := c.funcs[sig.Name]; ok {
		if prev.Defined {
			c.errorf(file, n.Line, n.Col, "duplicate function definition for %q", sig.Name)
			return
		}
		prev.Defined = true
		prev.RetCount = sig.RetCount
		prev.RetKind = sig.RetKind
		prev.RetBase = sig.RetBase
		prev.RetPtrDepth = sig.RetPtrDepth
		prev.RetOpaque = sig.RetOpaque
		prev.RetAggKeyword = sig.RetAggKeyword
		prev.RetAggTag = sig.RetAggTag
		prev.ParamCount = sig.ParamCount
		prev.Variadic = sig.Variadic
		prev.ParamNames = append([]string{}, sig.ParamNames...)
		prev.ParamKinds = append([]cDeclKind{}, sig.ParamKinds...)
		prev.ParamBases = append([]cScalarType{}, sig.ParamBases...)
		prev.ParamPtrDepth = append([]int{}, sig.ParamPtrDepth...)
		prev.ParamOpaque = append([]bool{}, sig.ParamOpaque...)
		prev.ParamAggKey = append([]string{}, sig.ParamAggKey...)
		prev.ParamAggTag = append([]string{}, sig.ParamAggTag...)
		prev.ParamFuncSigs = make([]*cFuncTypeSig, len(sig.ParamFuncSigs))
		for i, p := range sig.ParamFuncSigs {
			prev.ParamFuncSigs[i] = cloneFuncTypeSig(p)
		}
		prev.Body = sig.Body
		prev.File = file
		prev.Line = n.Line
		prev.Col = n.Col
		return
	}

	c.funcs[sig.Name] = sig
	c.funcOrder = append(c.funcOrder, sig)
}

func (c *compiler) collectExternalDecl(file string, n *Node) {
	toks, err := lexSnippet(file, n.Text)
	if err != nil {
		c.errorf(file, n.Line, n.Col, "invalid external declaration: %v", err)
		return
	}
	toks = trimLeadingDeclDecorators(toks)
	if len(toks) == 0 {
		return
	}

	// Function prototype path: skip typedef forms so function-pointer typedefs
	// keep flowing through the declaration parser, but otherwise keep the
	// permissive prototype detection for ordinary hosted headers.
	fnLPar := topLevelPunctIndex(toks, "(")
	hasTypedef := false
	for i := 0; i < fnLPar && i < len(toks); i++ {
		if toks[i].Kind == TokIdent && toks[i].Text == "typedef" {
			hasTypedef = true
			break
		}
	}
	if !hasTypedef && fnLPar > 0 && toks[fnLPar-1].Kind == TokIdent && !isDeclarationKeyword(toks[fnLPar-1]) && looksLikeStandaloneFunctionDecl(toks, fnLPar) {
		sig, err := parseFunctionSignature(file, n.Line, n.Col, toks)
		if err == nil {
			sig.IRName = c.cSymbolName(sig.Name)
			if _, ok := c.funcs[sig.Name]; !ok {
				sig.Defined = false
				c.funcs[sig.Name] = sig
				c.funcOrder = append(c.funcOrder, sig)
			}
			return
		}
	}

	items, enumConsts, hasExtern, hasTypedef, err := parseDeclItemsWithOptions(toks, c.enumConsts, false, true)
	if err != nil {
		c.errorf(file, n.Line, n.Col, "%v", err)
		return
	}
	if len(enumConsts) > 0 {
		c.addGlobalEnumConsts(enumConsts, file, n.Line, n.Col)
	}
	if hasTypedef {
		if hasExtern {
			c.errorf(file, n.Line, n.Col, "extern typedef declarations are not supported")
		}
		for _, it := range items {
			if len(it.Init) > 0 {
				c.errorf(file, n.Line, n.Col, "typedef declaration with initializer is not supported: %s", it.Name)
				continue
			}
			if it.Kind == cDeclPointer && it.PtrDepth == 0 {
				it.PtrDepth = 1
			}
			if existing, exists := c.typedefs[it.Name]; exists {
				candidate := cTypeInfo{
					Kind:             it.Kind,
					PtrDepth:         it.PtrDepth,
					ArrayLen:         it.ArrayLen,
					ArrayDims:        cloneInt64s(it.ArrayDims),
					IsVoid:           it.IsVoid,
					Base:             it.Base,
					FuncSig:          cloneFuncTypeSig(it.FuncSig),
					OpaqueAggregate:  it.OpaqueAggregate,
					AggregateKeyword: it.AggregateKeyword,
					AggregateTag:     it.AggregateTag,
				}
				if cTypeInfoEquivalent(existing, candidate) {
					continue
				}
				c.errorf(file, n.Line, n.Col, "duplicate typedef name %q", it.Name)
				continue
			}
			c.typedefs[it.Name] = cTypeInfo{
				Kind:             it.Kind,
				PtrDepth:         it.PtrDepth,
				ArrayLen:         it.ArrayLen,
				ArrayDims:        cloneInt64s(it.ArrayDims),
				IsVoid:           it.IsVoid,
				Base:             it.Base,
				FuncSig:          cloneFuncTypeSig(it.FuncSig),
				OpaqueAggregate:  it.OpaqueAggregate,
				AggregateKeyword: it.AggregateKeyword,
				AggregateTag:     it.AggregateTag,
			}
		}
		return
	}
	for _, it := range items {
		if it.FuncSig != nil && it.Kind == cDeclScalar && it.PtrDepth == 0 {
			sig := &cFuncSig{
				Name:             it.Name,
				IRName:           c.cSymbolName(it.Name),
				RetByPtr:         it.FuncSig.RetByPtr,
				RetKind:          it.FuncSig.RetKind,
				RetBase:          it.FuncSig.RetBase,
				RetPtrDepth:      it.FuncSig.RetPtrDepth,
				RetOpaque:        it.FuncSig.RetOpaque,
				RetAggKeyword:    it.FuncSig.RetAggKeyword,
				RetAggTag:        it.FuncSig.RetAggTag,
				ParamCount:       it.FuncSig.ParamCount,
				ParamUnspecified: it.FuncSig.ParamUnspecified,
				Variadic:         it.FuncSig.Variadic,
				ParamKinds:       append([]cDeclKind{}, it.FuncSig.ParamKinds...),
				ParamBases:       append([]cScalarType{}, it.FuncSig.ParamBases...),
				ParamPtrDepth:    append([]int{}, it.FuncSig.ParamPtrDepth...),
				ParamOpaque:      append([]bool{}, it.FuncSig.ParamOpaque...),
				ParamAggKey:      append([]string{}, it.FuncSig.ParamAggKey...),
				ParamAggTag:      append([]string{}, it.FuncSig.ParamAggTag...),
				ParamFuncSigs:    cloneFuncTypeSigs(it.FuncSig.ParamFuncSigs),
				Defined:          false,
				File:             file,
				Line:             n.Line,
				Col:              n.Col,
			}
			if it.FuncSig.RetIsVoid || it.FuncSig.RetByPtr {
				sig.RetCount = 0
			} else {
				sig.RetCount = 1
			}
			if prev, ok := c.funcs[sig.Name]; ok {
				if !funcTypeSigEqual(funcSigToTypeSig(prev), funcSigToTypeSig(sig)) {
					c.errorf(file, n.Line, n.Col, "conflicting function declaration for %q", sig.Name)
				}
			} else {
				c.funcs[sig.Name] = sig
				c.funcOrder = append(c.funcOrder, sig)
			}
			continue
		}
		if hasExtern && len(it.Init) == 0 {
			if _, exists := c.globalIndex[it.Name]; exists {
				continue
			}
		}
		if hasExtern && len(it.Init) > 0 {
			c.errorf(file, n.Line, n.Col, "extern declaration with initializer is not supported: %s", it.Name)
			continue
		}
		info := cTypeInfo{
			Kind:             it.Kind,
			PtrDepth:         it.PtrDepth,
			ArrayLen:         it.ArrayLen,
			ArrayDims:        cloneInt64s(it.ArrayDims),
			IsVoid:           it.IsVoid,
			Base:             it.Base,
			FuncSig:          cloneFuncTypeSig(it.FuncSig),
			OpaqueAggregate:  it.OpaqueAggregate,
			AggregateKeyword: it.AggregateKeyword,
			AggregateTag:     it.AggregateTag,
		}
		if hasExtern && len(it.Init) == 0 && c.canLoadExternDataOnTarget(it.Name) {
			if existing, ok := c.externDataTypes[it.Name]; ok {
				if !cTypeInfoEquivalent(existing, info) {
					c.errorf(file, n.Line, n.Col, "conflicting extern data declaration for %q", it.Name)
				}
			} else {
				c.externDataTypes[it.Name] = info
			}
			continue
		}
		if idx, exists := c.globalIndex[it.Name]; exists {
			existing, ok := c.globalTypeInfo(it.Name)
			merged, compatible := mergeRedeclaredTypeInfo(existing, info)
			if !ok || !compatible {
				c.errorf(file, n.Line, n.Col, "duplicate global declaration for %q", it.Name)
				continue
			}
			info = merged
			if it.Kind == cDeclArray && info.ArrayLen != cArrayLenUnspecified {
				c.globalArray[it.Name] = info.ArrayLen
				c.globalArrayDims[it.Name] = cloneInt64s(info.ArrayDims)
				for i := range c.globalInits {
					if c.globalInits[i].Name != it.Name || c.globalInits[i].Kind != cDeclArray {
						continue
					}
					c.globalInits[i].ArrayLen = info.ArrayLen
					c.globalInits[i].ArrayDims = cloneInt64s(info.ArrayDims)
				}
			}
			if len(it.Init) == 0 {
				continue
			}
			if c.globalHasInit[it.Name] {
				c.errorf(file, n.Line, n.Col, "duplicate global declaration for %q", it.Name)
				continue
			}
			if isAggregateObjectDecl(it.Kind, it.PtrDepth, it.OpaqueAggregate, it.AggregateKeyword, it.AggregateTag) {
				c.errorf(file, n.Line, n.Col, "duplicate global declaration for %q", it.Name)
				continue
			}
			c.globalHasInit[it.Name] = true
			irName := c.irmod.Globals[idx].Name
			if it.Kind == cDeclArray {
				replaced := false
				for i := range c.globalInits {
					if c.globalInits[i].Name != it.Name || c.globalInits[i].Kind != cDeclArray {
						continue
					}
					c.globalInits[i].PtrDepth = it.PtrDepth
					c.globalInits[i].ArrayLen = info.ArrayLen
					c.globalInits[i].ArrayDims = cloneInt64s(info.ArrayDims)
					c.globalInits[i].Base = it.Base
					c.globalInits[i].AggregateKeyword = it.AggregateKeyword
					c.globalInits[i].AggregateTag = it.AggregateTag
					c.globalInits[i].Init = append([]Token{}, it.Init...)
					c.globalInits[i].File = file
					c.globalInits[i].Line = n.Line
					c.globalInits[i].Col = n.Col
					c.globalInits[i].IRName = irName
					replaced = true
					break
				}
				if !replaced {
					c.globalInits = append(c.globalInits, cGlobalInit{
						Name:             it.Name,
						Index:            idx,
						Kind:             it.Kind,
						PtrDepth:         it.PtrDepth,
						ArrayLen:         info.ArrayLen,
						ArrayDims:        cloneInt64s(info.ArrayDims),
						Base:             it.Base,
						AggregateKeyword: it.AggregateKeyword,
						AggregateTag:     it.AggregateTag,
						Init:             append([]Token{}, it.Init...),
						File:             file,
						Line:             n.Line,
						Col:              n.Col,
						IRName:           irName,
					})
				}
				continue
			}
			c.globalInits = append(c.globalInits, cGlobalInit{
				Name:             it.Name,
				Index:            idx,
				Kind:             it.Kind,
				PtrDepth:         it.PtrDepth,
				Base:             it.Base,
				AggregateKeyword: it.AggregateKeyword,
				AggregateTag:     it.AggregateTag,
				Init:             append([]Token{}, it.Init...),
				File:             file,
				Line:             n.Line,
				Col:              n.Col,
				IRName:           irName,
			})
			continue
		}
		idx := len(c.irmod.Globals)
		irName := c.cSymbolName(it.Name)
		global := ir.IRGlobal{Name: irName, Index: idx}
		if it.Kind == cDeclScalar && it.PtrDepth == 0 && isFloatScalar(it.Base) {
			typeName := "float64"
			if it.Base == cScalarFloat {
				typeName = "float32"
			}
			global.Type = &ir.TypeInfo{
				Kind:  irFloatKindForCScalar(it.Base),
				Name:  typeName,
				Size:  int(scalarSizeForTarget(c.target, it.Base)),
				Align: int(scalarSizeForTarget(c.target, it.Base)),
			}
		}
		c.irmod.Globals = append(c.irmod.Globals, global)
		c.globalIndex[it.Name] = idx
		c.globalHasInit[it.Name] = len(it.Init) > 0
		c.globalKind[it.Name] = it.Kind
		c.globalPtrDepth[it.Name] = it.PtrDepth
		c.globalBase[it.Name] = it.Base
		c.globalFunc[it.Name] = cloneFuncTypeSig(it.FuncSig)
		c.globalOpaque[it.Name] = it.OpaqueAggregate
		c.globalAggKey[it.Name] = it.AggregateKeyword
		c.globalAggTag[it.Name] = it.AggregateTag
		elemStep := c.pointerElemStep(it.Kind, it.PtrDepth, it.Base, it.IsVoid, it.OpaqueAggregate, it.AggregateKeyword, it.AggregateTag)
		if it.Kind == cDeclArray {
			elemSize, _, err := cTypeLayout(arrayElementTypeInfo(info))
			if err == nil && elemSize > 0 {
				elemStep = elemSize
			}
		}
		if it.Kind == cDeclPointer && it.PtrDepth == 1 && isStringLiteralExpr(it.Init) {
			elemStep = 1
		}
		c.globalElemStep[it.Name] = elemStep
		if it.Kind == cDeclArray {
			c.globalArray[it.Name] = it.ArrayLen
			c.globalArrayDims[it.Name] = cloneInt64s(it.ArrayDims)
			c.globalInits = append(c.globalInits, cGlobalInit{
				Name:             it.Name,
				Index:            idx,
				Kind:             it.Kind,
				PtrDepth:         it.PtrDepth,
				ArrayLen:         it.ArrayLen,
				ArrayDims:        cloneInt64s(it.ArrayDims),
				Base:             it.Base,
				AggregateKeyword: it.AggregateKeyword,
				AggregateTag:     it.AggregateTag,
				Init:             append([]Token{}, it.Init...),
				File:             file,
				Line:             n.Line,
				Col:              n.Col,
				IRName:           irName,
			})
			continue
		}
		if isAggregateObjectDecl(it.Kind, it.PtrDepth, it.OpaqueAggregate, it.AggregateKeyword, it.AggregateTag) {
			size, _, err := cTypeLayout(cTypeInfo{
				Kind:             it.Kind,
				PtrDepth:         it.PtrDepth,
				Base:             it.Base,
				OpaqueAggregate:  it.OpaqueAggregate,
				AggregateKeyword: it.AggregateKeyword,
				AggregateTag:     it.AggregateTag,
			})
			if err != nil {
				c.errorf(file, n.Line, n.Col, "unsupported aggregate object declaration for %s: %v", it.Name, err)
				continue
			}
			word := int64(c.target.PtrSize)
			words := (size + word - 1) / word
			if words <= 0 {
				words = 1
			}
			base := len(c.irmod.Globals)
			for i := int64(0); i < words; i++ {
				elemIdx := len(c.irmod.Globals)
				elemName := fmt.Sprintf("%s$obj$%d", irName, i)
				c.irmod.Globals = append(c.irmod.Globals, ir.IRGlobal{Name: elemName, Index: elemIdx})
			}
			c.globalInits = append(c.globalInits, cGlobalInit{
				Name:             it.Name,
				Index:            idx,
				Kind:             it.Kind,
				PtrDepth:         it.PtrDepth,
				ObjectBase:       base,
				ObjectWords:      words,
				AggregateKeyword: it.AggregateKeyword,
				AggregateTag:     it.AggregateTag,
				Base:             it.Base,
				Init:             append([]Token{}, it.Init...),
				File:             file,
				Line:             n.Line,
				Col:              n.Col,
				IRName:           irName,
			})
			continue
		}
		if len(it.Init) > 0 {
			c.globalInits = append(c.globalInits, cGlobalInit{
				Name:      it.Name,
				Index:     idx,
				Kind:      it.Kind,
				PtrDepth:  it.PtrDepth,
				ArrayLen:  it.ArrayLen,
				ArrayDims: cloneInt64s(it.ArrayDims),
				Base:      it.Base,
				Init:      append([]Token{}, it.Init...),
				File:      file,
				Line:      n.Line,
				Col:       n.Col,
				IRName:    irName,
			})
		}
	}
}

func looksLikeStandaloneFunctionDecl(toks []Token, fnLPar int) bool {
	if fnLPar <= 0 || fnLPar >= len(toks) {
		return false
	}
	rpar := matchingParenClose(toks, fnLPar)
	if rpar < 0 {
		return false
	}
	for i := rpar + 1; i < len(toks); i++ {
		t := toks[i]
		if t.Kind == TokNewline {
			continue
		}
		if t.Kind != TokPunct {
			return false
		}
		switch t.Text {
		case ",", "[", "]", "=", "{", "}":
			return false
		}
	}
	return true
}

func trimLeadingDeclDecorators(tokens []Token) []Token {
	work := trimTokens(tokens)
	for len(work) > 0 {
		if work[0].Kind != TokIdent {
			break
		}
		switch work[0].Text {
		case "__attribute__", "__attribute":
			if len(work) < 2 || work[1].Kind != TokPunct || work[1].Text != "(" {
				return work
			}
			close := matchingParenClose(work, 1)
			if close < 0 {
				return work
			}
			work = trimTokens(work[close+1:])
			continue
		}
		break
	}
	return work
}

func (c *compiler) emitGlobalInit() {
	if len(c.globalInits) == 0 {
		return
	}
	f := &ir.IRFunc{Name: "c.init$globals", Params: 0, RetCount: 0}
	fc := &funcCompiler{
		c:             c,
		sig:           &cFuncSig{Name: "c.init$globals", IRName: "c.init$globals", RetCount: 0},
		fn:            f,
		scopes:        []map[string]cLocalBinding{{}},
		typedefScopes: []map[string]cTypeInfo{{}},
		enumScopes:    []map[string]int64{{}},
		aggregateTags: []map[string]*cAggregateInfo{{}},
		variadicCount: -1,
		variadicData:  -1,
	}
	for _, g := range c.globalInits {
		if g.ObjectWords > 0 {
			fc.emit(ir.Inst{Op: ir.OP_GLOBAL_ADDR, Arg: g.ObjectBase})
			fc.emit(ir.Inst{Op: ir.OP_GLOBAL_SET, Arg: g.Index})
			fc.emitAggregateObjectInitializer(g.Name, g.Index, true, cTypeInfo{
				Kind:             g.Kind,
				PtrDepth:         g.PtrDepth,
				Base:             g.Base,
				AggregateKeyword: g.AggregateKeyword,
				AggregateTag:     g.AggregateTag,
			}, g.Init, g.File, g.Line, g.Col)
			continue
		}
		if g.Kind == cDeclArray {
			arrInfo := cTypeInfo{
				Kind:             g.Kind,
				PtrDepth:         g.PtrDepth,
				ArrayLen:         g.ArrayLen,
				ArrayDims:        cloneInt64s(g.ArrayDims),
				Base:             g.Base,
				AggregateKeyword: g.AggregateKeyword,
				AggregateTag:     g.AggregateTag,
			}
			totalBytes := fc.typeByteSize(arrInfo)
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: totalBytes})
			alloc := fc.c.ensureIntrinsicWrapper("Alloc", 1, 1)
			fc.emit(ir.Inst{Op: ir.OP_CALL, Name: alloc, Arg: 1})
			fc.emit(ir.Inst{Op: ir.OP_GLOBAL_SET, Arg: g.Index})
			baseTmp := fc.allocTempLocal("$global_array_init")
			fc.emit(ir.Inst{Op: ir.OP_GLOBAL_GET, Arg: g.Index})
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: baseTmp})
			fc.emitArrayInitializerAt(g.Name, baseTmp, 0, arrInfo, g.Init, g.File, g.Line, g.Col)
			continue
		}
		fc.emitExprTokensAsType(g.File, g.Line, g.Col, g.Init, cTypeInfo{
			Kind:     g.Kind,
			PtrDepth: g.PtrDepth,
			Base:     g.Base,
		})
		fc.emit(ir.Inst{Op: ir.OP_GLOBAL_SET, Arg: g.Index})
	}
	fc.emit(ir.Inst{Op: ir.OP_RETURN, Arg: 0})
	c.irmod.Funcs = append(c.irmod.Funcs, f)
}

func (c *compiler) compileFunction(sig *cFuncSig) {
	if sig.Body == nil || sig.Body.Kind != NCompoundStmt {
		c.errorf(sig.File, sig.Line, sig.Col, "function %q is missing a compound body", sig.Name)
		return
	}

	paramSlots := sig.ParamCount
	if sig.RetByPtr {
		paramSlots++
	}
	if sig.Variadic {
		paramSlots += 2
	}
	f := &ir.IRFunc{Name: sig.IRName, Params: paramSlots, RetCount: sig.RetCount}
	if sig.RetCount > 0 && !sig.RetByPtr {
		retInfo := cTypeInfo{
			Kind:             sig.RetKind,
			PtrDepth:         sig.RetPtrDepth,
			Base:             sig.RetBase,
			OpaqueAggregate:  sig.RetOpaque,
			AggregateKeyword: sig.RetAggKeyword,
			AggregateTag:     sig.RetAggTag,
		}
		f.ResultKinds = []ir.TypeKind{irResultKindForCType(retInfo)}
		f.ResultIs64 = []bool{irResultIs64ForCType(c.target, retInfo)}
	}
	fc := &funcCompiler{
		c:             c,
		sig:           sig,
		fn:            f,
		scopes:        []map[string]cLocalBinding{{}},
		typedefScopes: []map[string]cTypeInfo{{}},
		enumScopes:    []map[string]int64{{}},
		aggregateTags: []map[string]*cAggregateInfo{{}},
		userLabels:    make(map[string]cUserLabel),
		retPtrLocal:   -1,
		variadicCount: -1,
		variadicData:  -1,
	}
	if sig.RetByPtr {
		fc.retPtrLocal = fc.addLocalTyped("$sret", cDeclPointer, cScalarInt, 1, int64(fc.c.target.PtrSize), nil, sig.File, sig.Line, sig.Col)
	}
	for i, p := range sig.ParamNames {
		name := p
		if name == "" {
			name = fmt.Sprintf("$p%d", i)
		}
		kind := cDeclScalar
		base := cScalarInt
		ptrDepth := 0
		opaqueAggregate := false
		aggregateKeyword := ""
		aggregateTag := ""
		elemStep := int64(fc.c.target.PtrSize)
		if i < len(sig.ParamKinds) {
			kind = sig.ParamKinds[i]
		}
		if i < len(sig.ParamBases) {
			base = sig.ParamBases[i]
		}
		if i < len(sig.ParamPtrDepth) {
			ptrDepth = sig.ParamPtrDepth[i]
		}
		if i < len(sig.ParamOpaque) {
			opaqueAggregate = sig.ParamOpaque[i]
		}
		if i < len(sig.ParamAggKey) {
			aggregateKeyword = sig.ParamAggKey[i]
		}
		if i < len(sig.ParamAggTag) {
			aggregateTag = sig.ParamAggTag[i]
		}
		if kind == cDeclArray {
			kind = cDeclPointer
			if ptrDepth == 0 {
				ptrDepth = 1
			}
		}
		elemStep = fc.pointerElemStep(kind, ptrDepth, base, false, opaqueAggregate, aggregateKeyword, aggregateTag)
		var pfunc *cFuncTypeSig
		if i < len(sig.ParamFuncSigs) {
			pfunc = sig.ParamFuncSigs[i]
		}
		if isAggregateObjectDecl(kind, ptrDepth, opaqueAggregate, aggregateKeyword, aggregateTag) {
			addrName := fmt.Sprintf("$p%d$addr", i)
			addrIdx := fc.addLocalDecl(addrName, cDeclPointer, cScalarInt, 1, int64(fc.c.target.PtrSize), 0, nil, nil, false, "", "", sig.File, sig.Line, sig.Col)
			info := cTypeInfo{
				Kind:             kind,
				PtrDepth:         ptrDepth,
				Base:             base,
				FuncSig:          cloneFuncTypeSig(pfunc),
				OpaqueAggregate:  opaqueAggregate,
				AggregateKeyword: aggregateKeyword,
				AggregateTag:     aggregateTag,
			}
			objIdx := fc.addLocalDecl(name, kind, base, ptrDepth, int64(fc.c.target.PtrSize), 0, nil, pfunc, opaqueAggregate, aggregateKeyword, aggregateTag, sig.File, sig.Line, sig.Col)
			fc.initLocalAggregateObject(name, objIdx, info, nil, sig.File, sig.Line, sig.Col)
			fc.emitCopyAggregateBytes(objIdx, addrIdx, fc.typeByteSize(info))
			continue
		}
		fc.addLocalDecl(name, kind, base, ptrDepth, elemStep, 0, nil, pfunc, opaqueAggregate, aggregateKeyword, aggregateTag, sig.File, sig.Line, sig.Col)
	}
	if sig.Variadic {
		fc.variadicCount = fc.addLocalTyped("$va_count", cDeclScalar, cScalarInt, 0, int64(fc.c.target.PtrSize), nil, sig.File, sig.Line, sig.Col)
		fc.variadicData = fc.addLocalTyped("$va_data", cDeclPointer, cScalarInt, 1, int64(fc.c.target.PtrSize), nil, sig.File, sig.Line, sig.Col)
	}

	prevTypedefLookupFunc := cTypedefLookupFunc
	prevConstSizeLookupFunc := cConstSizeLookupFunc
	prevAggregateLookupFunc := cAggregateLookupFunc
	cTypedefLookupFunc = fc
	cConstSizeLookupFunc = fc
	cAggregateLookupFunc = fc
	fc.indexUserLabels(sig.Body)
	fc.compileCompound(sig.Body, true)
	cTypedefLookupFunc = prevTypedefLookupFunc
	cConstSizeLookupFunc = prevConstSizeLookupFunc
	cAggregateLookupFunc = prevAggregateLookupFunc
	if len(f.Code) == 0 || f.Code[len(f.Code)-1].Op != ir.OP_RETURN {
		if sig.RetCount > 0 {
			fc.emitZeroValue(cTypeInfo{
				Kind:             sig.RetKind,
				PtrDepth:         sig.RetPtrDepth,
				Base:             sig.RetBase,
				OpaqueAggregate:  sig.RetOpaque,
				AggregateKeyword: sig.RetAggKeyword,
				AggregateTag:     sig.RetAggTag,
			})
			fc.emit(ir.Inst{Op: ir.OP_RETURN, Arg: 1})
		} else {
			fc.emit(ir.Inst{Op: ir.OP_RETURN, Arg: 0})
		}
	}

	c.irmod.Funcs = append(c.irmod.Funcs, f)
}

func (c *compiler) emitEntryWrapper() {
	mainSig, ok := c.funcs["main"]
	if !ok || !mainSig.Defined {
		return
	}
	f := &ir.IRFunc{Name: "main.main", Params: 0, RetCount: 1}
	useValueArgs := c.target.Backend == "c" || (c.target.GOOS == "darwin" && c.target.GOARCH == "arm64")
	for i := 0; i < mainSig.ParamCount; i++ {
		switch {
		case useValueArgs && i == 0:
			getArgc := c.ensureIntrinsicWrapper("SysArgcValue", 0, 1)
			f.Code = append(f.Code, ir.Inst{Op: ir.OP_CALL, Name: getArgc, Arg: 0})
		case useValueArgs && i == 1:
			getArgv := c.ensureIntrinsicWrapper("SysArgvBaseValue", 0, 1)
			f.Code = append(f.Code, ir.Inst{Op: ir.OP_CALL, Name: getArgv, Arg: 0})
		default:
			f.Code = append(f.Code, ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		}
	}
	f.Code = append(f.Code, ir.Inst{Op: ir.OP_CALL, Name: mainSig.IRName, Arg: mainSig.ParamCount})
	if mainSig.RetCount <= 0 {
		f.Code = append(f.Code, ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
	} else {
		for i := 1; i < mainSig.RetCount; i++ {
			f.Code = append(f.Code, ir.Inst{Op: ir.OP_DROP})
		}
	}
	f.Code = append(f.Code, ir.Inst{Op: ir.OP_RETURN, Arg: 1})
	c.irmod.Funcs = append(c.irmod.Funcs, f)
}

func lexSnippet(file string, src string) ([]Token, error) {
	lx := NewLexer(file, src)
	toks, err := lx.Tokenize()
	if err != nil {
		return nil, err
	}
	out := make([]Token, 0, len(toks))
	for _, t := range toks {
		if t.Kind == TokEOF || t.Kind == TokNewline {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

func hasTopLevelPunct(tokens []Token, punct string) bool {
	return topLevelPunctIndex(tokens, punct) >= 0
}

func topLevelPunctIndex(tokens []Token, punct string) int {
	depthParen := 0
	depthBracket := 0
	depthBrace := 0
	for i, t := range tokens {
		if t.Kind != TokPunct {
			continue
		}
		switch t.Text {
		case "(":
			if punct == "(" && depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
				return i
			}
			depthParen++
		case ")":
			if depthParen > 0 {
				depthParen--
			}
		case "[":
			if punct == "[" && depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
				return i
			}
			depthBracket++
		case "]":
			if depthBracket > 0 {
				depthBracket--
			}
		case "{":
			if punct == "{" && depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
				return i
			}
			depthBrace++
		case "}":
			if depthBrace > 0 {
				depthBrace--
			}
		default:
			if t.Text == punct && depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
				return i
			}
		}
	}
	return -1
}

func splitTopLevel(tokens []Token, sep string) [][]Token {
	parts := make([][]Token, 1)
	parts[0] = nil
	depthParen := 0
	depthBracket := 0
	depthBrace := 0
	depthTernary := 0
	for _, t := range tokens {
		if t.Kind == TokPunct {
			switch t.Text {
			case "(":
				depthParen++
			case ")":
				if depthParen > 0 {
					depthParen--
				}
			case "[":
				depthBracket++
			case "]":
				if depthBracket > 0 {
					depthBracket--
				}
			case "{":
				depthBrace++
			case "}":
				if depthBrace > 0 {
					depthBrace--
				}
			case "?":
				if depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
					depthTernary++
				}
			case ":":
				if depthParen == 0 && depthBracket == 0 && depthBrace == 0 && depthTernary > 0 {
					depthTernary--
				}
			}
			if t.Text == sep && depthParen == 0 && depthBracket == 0 && depthBrace == 0 && depthTernary == 0 {
				parts = append(parts, nil)
				continue
			}
		}
		parts[len(parts)-1] = append(parts[len(parts)-1], t)
	}
	return parts
}

func trimTokens(tokens []Token) []Token {
	if len(tokens) == 0 {
		return tokens
	}
	start := 0
	end := len(tokens)
	for start < end && tokens[start].Kind == TokNewline {
		start++
	}
	for end > start && tokens[end-1].Kind == TokNewline {
		end--
	}
	return tokens[start:end]
}

func parseFunctionSignature(file string, line int, col int, toks []Token) (*cFuncSig, error) {
	toks = trimTokens(toks)
	if len(toks) == 0 {
		return nil, fmt.Errorf("empty function declaration")
	}
	if brace := topLevelPunctIndex(toks, "{"); brace >= 0 {
		toks = trimTokens(toks[:brace])
	}

	lpar := trailingFunctionSuffixOpen(toks)
	oldStyle := false
	if lpar < 0 {
		lpar = topLevelPunctIndex(toks, "(")
		if lpar < 0 {
			return nil, fmt.Errorf("not a function declaration")
		}
		oldStyle = true
	}

	rpar := matchingParenClose(toks, lpar)
	if rpar < 0 {
		return nil, fmt.Errorf("unterminated function parameter list")
	}

	head := trimTokens(toks[:lpar])
	trailing := trimTokens(toks[rpar+1:])
	if !oldStyle && len(trailing) > 0 {
		oldStyle = true
	}
	spec, decl, err := splitDeclSpecPrefix(head, "function declaration")
	if err != nil {
		return nil, err
	}
	retSpec, _, _, _, err := parseScalarTypeSpec(spec, "function declaration", true)
	if err != nil {
		return nil, err
	}
	name, retDeclKind, retDeclPtrDepth, _, _, _, _, err := parseDeclarator(decl, false, nil)
	if err != nil {
		return nil, err
	}
	retInfo, err := combineTypeAndDeclarator(retSpec, retDeclKind, retDeclPtrDepth, 0, false, "function declaration")
	if err != nil {
		return nil, err
	}
	if retInfo.Kind == cDeclArray {
		return nil, fmt.Errorf("function %q cannot return array type", name)
	}
	retCount := 1
	if retInfo.IsVoid && retInfo.Kind == cDeclScalar {
		retCount = 0
	} else if isAggregateObjectType(retInfo) {
		retCount = 0
	}

	var paramNames []string
	var paramKinds []cDeclKind
	var paramBases []cScalarType
	var paramPtrDepth []int
	var paramOpaque []bool
	var paramAggKey []string
	var paramAggTag []string
	var paramFuncSigs []*cFuncTypeSig
	paramUnspecified := oldStyle
	var variadic bool
	paramCount := 0
	if oldStyle {
		paramNames, paramKinds, paramBases, paramPtrDepth, paramOpaque, paramAggKey, paramAggTag, paramFuncSigs, variadic, err =
			parseKNRFunctionParams(toks[lpar+1:rpar], trailing)
		if err != nil {
			return nil, err
		}
		paramCount = len(paramNames)
	} else {
		paramTokens := trimTokens(toks[lpar+1 : rpar])
		if len(paramTokens) == 0 {
			paramUnspecified = true
		} else {
			parts := splitTopLevel(paramTokens, ",")
			if len(parts) == 1 {
				p0 := trimTokens(parts[0])
				if len(p0) > 0 {
					spec, decl, err := splitDeclSpecPrefix(p0, "function parameter list")
					if err == nil {
						baseInfo, _, _, _, err := parseScalarTypeSpec(spec, "function parameter list", true)
						if err == nil {
							_, kind, ptrDepth, arrLen, arrDims, _, _, err := parseDeclarator(decl, true, nil)
							if err == nil {
								info, cerr := combineTypeAndDeclarator(baseInfo, kind, ptrDepth, arrLen, false, "function parameter list")
								if cerr == nil {
									applyDeclaratorArrayDims(&info, arrDims)
								}
								if cerr == nil && info.IsVoid && info.Kind == cDeclScalar {
									parts = nil
								}
							}
						}
					}
				}
			}
			for i, p := range parts {
				p = trimTokens(p)
				if len(p) == 0 {
					continue
				}
				if len(p) == 1 && p[0].Kind == TokPunct && p[0].Text == "..." {
					if i != len(parts)-1 || paramCount == 0 {
						return nil, fmt.Errorf("variadic marker must appear last after at least one named parameter")
					}
					variadic = true
					continue
				}
				spec, decl, err := splitDeclSpecPrefix(p, fmt.Sprintf("function parameter %d", i+1))
				if err != nil {
					return nil, err
				}
				pbaseInfo, _, _, _, err := parseScalarTypeSpec(spec, fmt.Sprintf("function parameter %d", i+1), true)
				if err != nil {
					return nil, err
				}
				pname, pdeclKind, pdeclPtrDepth, parrLen, parrDims, pfnSig, directFunc, err := parseDeclarator(decl, true, nil)
				if err != nil {
					return nil, err
				}
				if directFunc {
					pdeclKind = cDeclPointer
					pdeclPtrDepth = 1
				}
				pinfo, err := combineTypeAndDeclarator(pbaseInfo, pdeclKind, pdeclPtrDepth, parrLen, false, fmt.Sprintf("function parameter %d", i+1))
				if err != nil {
					return nil, err
				}
				applyDeclaratorArrayDims(&pinfo, parrDims)
				if pfnSig != nil {
					fnSig := cloneFuncTypeSig(pfnSig)
					applyFuncReturnBase(fnSig, pbaseInfo)
					pinfo.FuncSig = fnSig
				}
				if pname == "" {
					pname = fmt.Sprintf("$p%d", i)
				}
				if pinfo.Kind == cDeclArray {
					// Arrays in parameter lists decay to pointers.
					pinfo = decayArrayTypeInfo(pinfo)
				}
				if pinfo.IsVoid && pinfo.Kind == cDeclScalar {
					return nil, fmt.Errorf("function parameter %q cannot have type void", pname)
				}
				paramNames = append(paramNames, pname)
				paramKinds = append(paramKinds, pinfo.Kind)
				paramBases = append(paramBases, pinfo.Base)
				paramPtrDepth = append(paramPtrDepth, pinfo.PtrDepth)
				paramOpaque = append(paramOpaque, pinfo.OpaqueAggregate)
				paramAggKey = append(paramAggKey, pinfo.AggregateKeyword)
				paramAggTag = append(paramAggTag, pinfo.AggregateTag)
				paramFuncSigs = append(paramFuncSigs, cloneFuncTypeSig(pinfo.FuncSig))
				paramCount++
			}
		}
	}

	return &cFuncSig{
		Name:             name,
		IRName:           "c." + name,
		RetCount:         retCount,
		RetByPtr:         isAggregateObjectType(retInfo),
		RetKind:          retInfo.Kind,
		RetBase:          retInfo.Base,
		RetPtrDepth:      retInfo.PtrDepth,
		RetOpaque:        retInfo.OpaqueAggregate,
		RetAggKeyword:    retInfo.AggregateKeyword,
		RetAggTag:        retInfo.AggregateTag,
		ParamCount:       paramCount,
		ParamUnspecified: paramUnspecified,
		Variadic:         variadic,
		ParamNames:       paramNames,
		ParamKinds:       paramKinds,
		ParamBases:       paramBases,
		ParamPtrDepth:    paramPtrDepth,
		ParamOpaque:      paramOpaque,
		ParamAggKey:      paramAggKey,
		ParamAggTag:      paramAggTag,
		ParamFuncSigs:    paramFuncSigs,
		Defined:          false,
		File:             file,
		Line:             line,
		Col:              col,
	}, nil
}

func declItemTypeInfo(it cDeclItem) cTypeInfo {
	info := cTypeInfo{
		Kind:             it.Kind,
		PtrDepth:         it.PtrDepth,
		ArrayLen:         it.ArrayLen,
		ArrayDims:        cloneInt64s(it.ArrayDims),
		IsVoid:           it.IsVoid,
		Base:             it.Base,
		FuncSig:          cloneFuncTypeSig(it.FuncSig),
		OpaqueAggregate:  it.OpaqueAggregate,
		AggregateKeyword: it.AggregateKeyword,
		AggregateTag:     it.AggregateTag,
	}
	if it.DirectFunc {
		info.Kind = cDeclPointer
		if info.PtrDepth == 0 {
			info.PtrDepth = 1
		} else {
			info.PtrDepth++
		}
	}
	if info.Kind == cDeclArray {
		info = decayArrayTypeInfo(info)
	}
	return info
}

func parseKNRFunctionParams(paramNameTokens []Token, declTokens []Token) ([]string, []cDeclKind, []cScalarType, []int, []bool, []string, []string, []*cFuncTypeSig, bool, error) {
	paramNameTokens = trimTokens(paramNameTokens)
	declTokens = trimTokens(declTokens)
	parts := splitTopLevel(paramNameTokens, ",")
	paramNames := make([]string, 0, len(parts))
	for _, p := range parts {
		p = trimTokens(p)
		if len(p) == 0 {
			continue
		}
		if len(p) != 1 || p[0].Kind != TokIdent {
			return nil, nil, nil, nil, nil, nil, nil, nil, false, fmt.Errorf("K&R function parameter list must contain only identifiers")
		}
		paramNames = append(paramNames, p[0].Text)
	}
	paramTypes := make(map[string]cTypeInfo, len(paramNames))
	if len(declTokens) > 0 {
		decls := splitTopLevel(declTokens, ";")
		for _, rawDecl := range decls {
			rawDecl = trimTokens(rawDecl)
			if len(rawDecl) == 0 {
				continue
			}
			items, _, _, _, err := parseDeclItemsWithOptions(rawDecl, nil, false, true)
			if err != nil {
				return nil, nil, nil, nil, nil, nil, nil, nil, false, err
			}
			for _, it := range items {
				if len(it.Init) > 0 {
					return nil, nil, nil, nil, nil, nil, nil, nil, false, fmt.Errorf("K&R function parameter %q cannot have initializer", it.Name)
				}
				found := false
				for _, pname := range paramNames {
					if pname == it.Name {
						found = true
						break
					}
				}
				if !found {
					return nil, nil, nil, nil, nil, nil, nil, nil, false, fmt.Errorf("K&R function parameter declaration %q does not match parameter list", it.Name)
				}
				info := declItemTypeInfo(it)
				if info.IsVoid && info.Kind == cDeclScalar {
					return nil, nil, nil, nil, nil, nil, nil, nil, false, fmt.Errorf("function parameter %q cannot have type void", it.Name)
				}
				paramTypes[it.Name] = info
			}
		}
	}

	paramKinds := make([]cDeclKind, 0, len(paramNames))
	paramBases := make([]cScalarType, 0, len(paramNames))
	paramPtrDepth := make([]int, 0, len(paramNames))
	paramOpaque := make([]bool, 0, len(paramNames))
	paramAggKey := make([]string, 0, len(paramNames))
	paramAggTag := make([]string, 0, len(paramNames))
	paramFuncSigs := make([]*cFuncTypeSig, 0, len(paramNames))
	for _, name := range paramNames {
		info, ok := paramTypes[name]
		if !ok {
			info = cTypeInfo{Kind: cDeclScalar, Base: cScalarInt}
		}
		paramKinds = append(paramKinds, info.Kind)
		paramBases = append(paramBases, info.Base)
		paramPtrDepth = append(paramPtrDepth, info.PtrDepth)
		paramOpaque = append(paramOpaque, info.OpaqueAggregate)
		paramAggKey = append(paramAggKey, info.AggregateKeyword)
		paramAggTag = append(paramAggTag, info.AggregateTag)
		paramFuncSigs = append(paramFuncSigs, cloneFuncTypeSig(info.FuncSig))
	}
	return paramNames, paramKinds, paramBases, paramPtrDepth, paramOpaque, paramAggKey, paramAggTag, paramFuncSigs, false, nil
}

func isStorageClassKeyword(text string) bool {
	switch text {
	case "auto", "register", "static", "extern", "typedef":
		return true
	default:
		return false
	}
}

func isTypeQualifierKeyword(text string) bool {
	switch text {
	case "const", "volatile", "restrict", "inline", "__const", "__volatile", "__restrict", "__inline", "__inline__":
		return true
	default:
		return false
	}
}

func isUnsupportedCTypeKeyword(text string) bool {
	switch text {
	case "_Bool", "_Complex", "_Imaginary":
		return true
	default:
		return false
	}
}

func consumeTaggedSpecifierTokens(tokens []Token, start int, keyword string, context string) (int, string, bool, error) {
	if start >= len(tokens) || tokens[start].Kind != TokIdent || tokens[start].Text != keyword {
		return start, "", false, fmt.Errorf("%s expected %s specifier", context, keyword)
	}
	i := start + 1
	tag := ""
	hasBody := false
	if i < len(tokens) && tokens[i].Kind == TokIdent {
		tag = tokens[i].Text
		i++
	}
	if i < len(tokens) && tokens[i].Kind == TokPunct && tokens[i].Text == "{" {
		hasBody = true
		depth := 1
		i++
		for i < len(tokens) && depth > 0 {
			t := tokens[i]
			if t.Kind == TokPunct {
				if t.Text == "{" {
					depth++
				} else if t.Text == "}" {
					depth--
				}
			}
			i++
		}
		if depth != 0 {
			return start, "", false, fmt.Errorf("%s has unterminated %s specifier", context, keyword)
		}
	}
	if tag == "" && !hasBody {
		return start, "", false, fmt.Errorf("%s %s specifier requires tag or body", context, keyword)
	}
	return i, tag, hasBody, nil
}

func consumeEnumSpecifierTokens(tokens []Token, start int, context string) (int, error) {
	i, _, _, err := consumeTaggedSpecifierTokens(tokens, start, "enum", context)
	if err != nil {
		return start, err
	}
	return i, nil
}

func splitDeclSpecPrefix(tokens []Token, context string) ([]Token, []Token, error) {
	tokens = trimTokens(tokens)
	if len(tokens) == 0 {
		return nil, nil, fmt.Errorf("%s is empty", context)
	}
	end := 0
	sawType := false
	sawPrefix := false
	for end < len(tokens) {
		t := tokens[end]
		if t.Kind == TokIdent && t.Text == "enum" {
			next, err := consumeEnumSpecifierTokens(tokens, end, context)
			if err != nil {
				return nil, nil, err
			}
			sawType = true
			end = next
			continue
		}
		if t.Kind == TokIdent && (t.Text == "struct" || t.Text == "union") {
			next, _, _, err := consumeTaggedSpecifierTokens(tokens, end, t.Text, context)
			if err != nil {
				return nil, nil, err
			}
			sawType = true
			end = next
			continue
		}
		if t.Kind != TokIdent {
			break
		}
		if isStorageClassKeyword(t.Text) || isTypeQualifierKeyword(t.Text) || isTypeSpecifierKeyword(t.Text) {
			sawPrefix = true
			if isTypeSpecifierKeyword(t.Text) {
				sawType = true
			}
			end++
			continue
		}
		if _, ok := lookupBuiltinTypedefAlias(t.Text); ok {
			if sawType {
				break
			}
			sawPrefix = true
			sawType = true
			end++
			continue
		}
		if _, ok := lookupTypedefAlias(t.Text); ok {
			if sawType {
				break
			}
			sawPrefix = true
			sawType = true
			end++
			continue
		}
		if isUnsupportedCTypeKeyword(t.Text) {
			return nil, nil, fmt.Errorf("%s uses unsupported type keyword %q", context, t.Text)
		}
		break
	}
	if !sawType {
		if sawPrefix {
			return trimTokens(tokens[:end]), trimTokens(tokens[end:]), nil
		}
		return nil, tokens, nil
	}
	return trimTokens(tokens[:end]), trimTokens(tokens[end:]), nil
}

func cScalarSizeForType(base cScalarType) int64 {
	switch base {
	case cScalarChar, cScalarUChar:
		return 1
	case cScalarShort, cScalarUShort:
		return 2
	case cScalarInt, cScalarUInt:
		return 4
	case cScalarLong, cScalarULong:
		return currentCTargetLongSize()
	default:
		return 4
	}
}

func cTypeLayout(info cTypeInfo) (int64, int64, error) {
	switch info.Kind {
	case cDeclPointer:
		ps := currentCTargetPtrSize()
		return ps, ps, nil
	case cDeclArray:
		elem := arrayElementTypeInfo(info)
		elemSize, elemAlign, err := cTypeLayout(elem)
		if err != nil {
			return 0, 0, err
		}
		return elemSize * info.ArrayLen, elemAlign, nil
	case cDeclScalar:
		if info.IsVoid {
			return 1, 1, nil
		}
		if info.AggregateKeyword != "" && info.AggregateTag != "" {
			if info.OpaqueAggregate {
				return 0, 0, fmt.Errorf("incomplete %s %q type", info.AggregateKeyword, info.AggregateTag)
			}
			agg, ok := lookupAggregateAlias(info.AggregateKeyword, info.AggregateTag)
			if !ok || len(agg.Fields) == 0 {
				return 0, 0, fmt.Errorf("unknown/incomplete %s %q type", info.AggregateKeyword, info.AggregateTag)
			}
			align := agg.Align
			if align <= 0 {
				align = 1
			}
			size := agg.Size
			if size <= 0 {
				size = 1
			}
			return size, align, nil
		}
		sz := cScalarSizeForType(info.Base)
		return sz, sz, nil
	default:
		return 0, 0, fmt.Errorf("unsupported declaration kind")
	}
}

func isSupportedBitfieldType(info cTypeInfo) bool {
	if info.Kind != cDeclScalar || info.PtrDepth != 0 || info.IsVoid || info.FuncSig != nil {
		return false
	}
	return info.AggregateKeyword == "" && info.AggregateTag == ""
}

func parseBitfieldWidth(tokens []Token) (int64, error) {
	tokens = trimTokens(tokens)
	if len(tokens) == 0 {
		return 0, fmt.Errorf("bitfield width is empty")
	}
	n, err := parseEnumConstExprTokens(tokens, nil, nil)
	if err != nil {
		return 0, err
	}
	if n < 0 {
		return 0, fmt.Errorf("bitfield width must not be negative")
	}
	return n, nil
}

func parseAggregateFields(tokens []Token, keyword string, tag string, context string) ([]cAggregateField, int64, int64, map[string]int64, error) {
	decls := splitTopLevel(trimTokens(tokens), ";")
	fields := make([]cAggregateField, 0, len(decls))
	used := make(map[string]bool)
	var enumConsts map[string]int64
	prevDeclSizes := cConstSizeLookupDecl
	declSizes := make(map[string]int64)
	cConstSizeLookupDecl = declSizes
	defer func() {
		cConstSizeLookupDecl = prevDeclSizes
	}()
	maxAlign := int64(1)
	maxSize := int64(0)
	nextOffset := int64(0)
	bitfieldOpen := false
	bitfieldBase := cTypeInfo{}
	bitfieldUnitOffset := int64(0)
	bitfieldUnitSize := int64(0)
	bitfieldUnitAlign := int64(1)
	bitfieldBitsUsed := int64(0)
	for i, rawDecl := range decls {
		decl := trimTokens(rawDecl)
		if len(decl) == 0 {
			continue
		}
		dctx := fmt.Sprintf("%s member declaration %d", context, i+1)
		spec, rest, err := splitDeclSpecPrefix(decl, dctx)
		if err != nil {
			return nil, 0, 0, nil, err
		}
		baseInfo, memberEnumConsts, hasExtern, hasTypedef, err := parseScalarTypeSpec(spec, dctx, true)
		if err != nil {
			return nil, 0, 0, nil, err
		}
		enumConsts, err = mergeEnumConstMaps(enumConsts, memberEnumConsts)
		if err != nil {
			return nil, 0, 0, nil, fmt.Errorf("%s: %v", dctx, err)
		}
		if hasExtern {
			return nil, 0, 0, nil, fmt.Errorf("%s does not allow extern members", dctx)
		}
		if hasTypedef {
			return nil, 0, 0, nil, fmt.Errorf("%s does not allow typedef members", dctx)
		}
		if len(rest) == 0 {
			if baseInfo.AggregateKeyword != "" && baseInfo.AggregateTag != "" && !baseInfo.OpaqueAggregate {
				size, align, err := cTypeLayout(baseInfo)
				if err != nil {
					return nil, 0, 0, nil, fmt.Errorf("%s anonymous %s has unsupported type: %v", dctx, baseInfo.AggregateKeyword, err)
				}
				if align <= 0 {
					align = 1
				}
				agg, ok := lookupAggregateAlias(baseInfo.AggregateKeyword, baseInfo.AggregateTag)
				if !ok || len(agg.Fields) == 0 {
					return nil, 0, 0, nil, fmt.Errorf("%s anonymous %s is incomplete", dctx, baseInfo.AggregateKeyword)
				}
				baseOffset := int64(0)
				if keyword == "union" {
					if size > maxSize {
						maxSize = size
					}
				} else {
					bitfieldOpen = false
					bitfieldBitsUsed = 0
					nextOffset = alignTo(nextOffset, align)
					baseOffset = nextOffset
					nextOffset += size
					if nextOffset > maxSize {
						maxSize = nextOffset
					}
				}
				if align > maxAlign {
					maxAlign = align
				}
				for _, nested := range agg.Fields {
					if nested.Name == "" {
						continue
					}
					if used[nested.Name] {
						return nil, 0, 0, nil, fmt.Errorf("%s has duplicate member name %q", dctx, nested.Name)
					}
					field := nested
					field.Offset += baseOffset
					used[field.Name] = true
					fields = append(fields, field)
				}
				continue
			}
			return nil, 0, 0, nil, fmt.Errorf("%s requires at least one declarator", dctx)
		}
		parts := splitTopLevel(rest, ",")
		for j, part := range parts {
			part = trimTokens(part)
			if len(part) == 0 {
				continue
			}
			bctx := fmt.Sprintf("%s member declarator %d", dctx, j+1)
			eqIdx := topLevelPunctIndex(part, "=")
			if eqIdx >= 0 {
				return nil, 0, 0, nil, fmt.Errorf("%s cannot have initializer", bctx)
			}
			colonIdx := topLevelPunctIndex(part, ":")
			lhs := part
			var width int64
			if colonIdx >= 0 {
				lhs = trimTokens(part[:colonIdx])
				w, err := parseBitfieldWidth(part[colonIdx+1:])
				if err != nil {
					return nil, 0, 0, nil, fmt.Errorf("%s has invalid bitfield width: %v", bctx, err)
				}
				width = w
			}
			if len(lhs) == 0 {
				if colonIdx < 0 {
					return nil, 0, 0, nil, fmt.Errorf("%s has unnamed member", bctx)
				}
				lhs = nil
			}
			name := ""
			memberType := cTypeInfo{}
			field := cAggregateField{}
			if len(lhs) > 0 {
				var kind cDeclKind
				var ptrDepth int
				var arrayLen int64
				var arrayDims []int64
				var fnSig *cFuncTypeSig
				var directFunc bool
				name, kind, ptrDepth, arrayLen, arrayDims, fnSig, directFunc, err = parseDeclarator(lhs, false, enumConsts)
				if err != nil {
					return nil, 0, 0, nil, fmt.Errorf("%s: %w (%s)", bctx, err, tokenSliceText(lhs))
				}
				memberType, err = combineTypeAndDeclarator(baseInfo, kind, ptrDepth, arrayLen, false, bctx)
				if err != nil {
					return nil, 0, 0, nil, err
				}
				applyDeclaratorArrayDims(&memberType, arrayDims)
				if fnSig != nil {
					sig := cloneFuncTypeSig(fnSig)
					applyFuncReturnBase(sig, baseInfo)
					memberType.FuncSig = sig
				}
				if directFunc {
					memberType.Kind = cDeclPointer
					if memberType.PtrDepth == 0 {
						memberType.PtrDepth = 1
					} else {
						memberType.PtrDepth++
					}
				}
			} else {
				memberType = baseInfo
			}
			if name != "" {
				if size, _, err := cTypeLayout(memberType); err == nil {
					declSizes[name] = size
				}
			}
			if colonIdx >= 0 {
				if !isSupportedBitfieldType(memberType) {
					return nil, 0, 0, nil, fmt.Errorf("%s uses unsupported bitfield type", bctx)
				}
				size, align, err := cTypeLayout(memberType)
				if err != nil {
					return nil, 0, 0, nil, fmt.Errorf("%s has unsupported bitfield type: %v", bctx, err)
				}
				if align <= 0 {
					align = 1
				}
				unitBits := size * 8
				if width > unitBits {
					return nil, 0, 0, nil, fmt.Errorf("%s bitfield width %d exceeds storage width %d", bctx, width, unitBits)
				}
				field = cAggregateField{
					Name:          name,
					Type:          memberType,
					Size:          size,
					Align:         align,
					BitfieldWidth: width,
				}
				if keyword == "union" {
					field.Offset = 0
					if size > maxSize {
						maxSize = size
					}
				} else {
					if width == 0 || !bitfieldOpen || !cTypeInfoEquivalent(bitfieldBase, memberType) || bitfieldUnitSize != size || bitfieldUnitAlign != align || bitfieldBitsUsed+width > unitBits {
						nextOffset = alignTo(nextOffset, align)
						bitfieldUnitOffset = nextOffset
						bitfieldUnitSize = size
						bitfieldUnitAlign = align
						bitfieldBitsUsed = 0
						bitfieldBase = memberType
						bitfieldOpen = true
						nextOffset += size
						if nextOffset > maxSize {
							maxSize = nextOffset
						}
					}
					field.Offset = bitfieldUnitOffset
					bitfieldBitsUsed += width
					if width == 0 || bitfieldBitsUsed >= unitBits {
						bitfieldOpen = false
						bitfieldBitsUsed = 0
					}
				}
				if align > maxAlign {
					maxAlign = align
				}
				if name == "" {
					continue
				}
			} else {
				if name == "" {
					return nil, 0, 0, nil, fmt.Errorf("%s has unnamed member", bctx)
				}
				size, align, err := cTypeLayout(memberType)
				if err != nil {
					return nil, 0, 0, nil, fmt.Errorf("%s member %q has unsupported type: %v", bctx, name, err)
				}
				if align <= 0 {
					align = 1
				}
				field = cAggregateField{
					Name:  name,
					Type:  memberType,
					Size:  size,
					Align: align,
				}
				if keyword == "union" {
					field.Offset = 0
					if size > maxSize {
						maxSize = size
					}
				} else {
					bitfieldOpen = false
					bitfieldBitsUsed = 0
					nextOffset = alignTo(nextOffset, align)
					field.Offset = nextOffset
					nextOffset += size
					if nextOffset > maxSize {
						maxSize = nextOffset
					}
				}
				if align > maxAlign {
					maxAlign = align
				}
			}
			if used[name] {
				return nil, 0, 0, nil, fmt.Errorf("%s has duplicate member name %q", bctx, name)
			}
			used[name] = true
			fields = append(fields, field)
		}
	}
	if len(fields) == 0 {
		return nil, 0, 0, nil, fmt.Errorf("%s %q requires at least one member declaration", keyword, tag)
	}
	maxSize = alignTo(maxSize, maxAlign)
	return fields, maxSize, maxAlign, enumConsts, nil
}

func parseAggregateTypeSpec(tokens []Token, start int, keyword string, context string) (int, cTypeInfo, map[string]int64, error) {
	if start >= len(tokens) || tokens[start].Kind != TokIdent || tokens[start].Text != keyword {
		return start, cTypeInfo{}, nil, fmt.Errorf("%s expected %s specifier", context, keyword)
	}
	i := start + 1
	tag := ""
	if i < len(tokens) && tokens[i].Kind == TokIdent {
		tag = tokens[i].Text
		i++
	}
	hasBody := false
	bodyOpen := -1
	bodyClose := -1
	if i < len(tokens) && tokens[i].Kind == TokPunct && tokens[i].Text == "{" {
		hasBody = true
		bodyOpen = i
		depth := 1
		i++
		for i < len(tokens) && depth > 0 {
			t := tokens[i]
			if t.Kind == TokPunct {
				if t.Text == "{" {
					depth++
				} else if t.Text == "}" {
					depth--
					if depth == 0 {
						bodyClose = i
					}
				}
			}
			i++
		}
		if depth != 0 || bodyClose < 0 {
			return start, cTypeInfo{}, nil, fmt.Errorf("%s has unterminated %s definition", context, keyword)
		}
	}
	if tag == "" && !hasBody {
		return start, cTypeInfo{}, nil, fmt.Errorf("%s %s specifier requires tag or body", context, keyword)
	}
	if hasBody {
		if tag == "" {
			tag = nextAnonAggregateTag(keyword)
		}
		placeholder := &cAggregateInfo{Keyword: keyword, Tag: tag, IsUnion: keyword == "union"}
		if err := registerAggregateAlias(placeholder); err != nil {
			return start, cTypeInfo{}, nil, err
		}
		body := trimTokens(tokens[bodyOpen+1 : bodyClose])
		fields, size, align, enumConsts, err := parseAggregateFields(body, keyword, tag, context)
		if err != nil {
			return start, cTypeInfo{}, nil, err
		}
		info := &cAggregateInfo{
			Keyword: keyword,
			Tag:     tag,
			IsUnion: keyword == "union",
			Size:    size,
			Align:   align,
			Fields:  fields,
		}
		if err := registerAggregateAlias(info); err != nil {
			return start, cTypeInfo{}, nil, err
		}
		return i, cTypeInfo{
			Kind:             cDeclScalar,
			Base:             cScalarInt,
			OpaqueAggregate:  false,
			AggregateKeyword: keyword,
			AggregateTag:     tag,
		}, enumConsts, nil
	}
	opaque := true
	if agg, ok := lookupAggregateAlias(keyword, tag); ok && len(agg.Fields) > 0 {
		opaque = false
	}
	return i, cTypeInfo{
		Kind:             cDeclScalar,
		Base:             cScalarInt,
		OpaqueAggregate:  opaque,
		AggregateKeyword: keyword,
		AggregateTag:     tag,
	}, nil, nil
}

func parseScalarTypeSpec(spec []Token, context string, allowVoid bool) (cTypeInfo, map[string]int64, bool, bool, error) {
	var hasExtern bool
	var hasTypedef bool
	var enumConsts map[string]int64
	spec = trimTokens(spec)
	if len(spec) == 0 {
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarInt}, nil, false, false, nil
	}
	var sawType bool
	var sawEnum bool
	var sawAggregate bool
	var aggregateOpaque bool
	var aggregateKeyword string
	var aggregateTag string
	var sawVoid bool
	var sawChar bool
	var sawShort bool
	var sawInt bool
	var sawLongCount int
	var sawFloat bool
	var sawDouble bool
	var sawSigned bool
	var sawUnsigned bool
	var sawPrefixOnly bool
	var aliasSet bool
	var aliasInfo cTypeInfo

	for i := 0; i < len(spec); {
		t := spec[i]
		if t.Kind == TokIdent && (t.Text == "struct" || t.Text == "union") {
			if aliasSet || sawEnum || sawAggregate || sawVoid || sawChar || sawShort || sawInt || sawLongCount > 0 || sawFloat || sawDouble || sawSigned || sawUnsigned {
				return cTypeInfo{}, nil, false, false, fmt.Errorf("%s cannot combine %s with additional type specifiers", context, t.Text)
			}
			next, aggInfo, aggEnumConsts, err := parseAggregateTypeSpec(spec, i, t.Text, context)
			if err != nil {
				return cTypeInfo{}, nil, false, false, err
			}
			enumConsts, err = mergeEnumConstMaps(enumConsts, aggEnumConsts)
			if err != nil {
				return cTypeInfo{}, nil, false, false, fmt.Errorf("%s: %v", context, err)
			}
			sawType = true
			sawAggregate = true
			aggregateOpaque = aggInfo.OpaqueAggregate
			aggregateKeyword = aggInfo.AggregateKeyword
			aggregateTag = aggInfo.AggregateTag
			i = next
			continue
		}
		if t.Kind == TokIdent && t.Text == "enum" {
			if aliasSet || sawAggregate || sawVoid || sawChar || sawShort || sawInt || sawLongCount > 0 || sawFloat || sawDouble || sawSigned || sawUnsigned || sawEnum {
				return cTypeInfo{}, nil, false, false, fmt.Errorf("%s cannot combine enum with additional type specifiers", context)
			}
			consumed, localEnumConsts, err := parseEnumSpecifierAndConstants(spec[i:], enumConsts)
			if err != nil {
				return cTypeInfo{}, nil, false, false, err
			}
			enumConsts, err = mergeEnumConstMaps(enumConsts, localEnumConsts)
			if err != nil {
				return cTypeInfo{}, nil, false, false, fmt.Errorf("%s: %v", context, err)
			}
			sawType = true
			sawEnum = true
			i += consumed
			continue
		}
		if t.Kind != TokIdent {
			return cTypeInfo{}, nil, false, false, fmt.Errorf("%s has invalid type token %q", context, t.Text)
		}
		if aliasSet {
			switch t.Text {
			case "extern":
				hasExtern = true
				continue
			case "typedef":
				hasTypedef = true
				continue
			case "const", "volatile", "restrict", "inline", "__const", "__volatile", "__restrict", "__inline", "__inline__":
				continue
			}
			return cTypeInfo{}, nil, false, false, fmt.Errorf("%s cannot combine typedef name %q with additional type specifiers", context, t.Text)
		}
		switch t.Text {
		case "extern":
			hasExtern = true
			sawPrefixOnly = true
		case "typedef":
			hasTypedef = true
			sawPrefixOnly = true
		case "auto", "register", "static":
			// Accepted for parser compatibility; not all storage semantics are modeled yet.
			sawPrefixOnly = true
		case "const", "volatile", "restrict", "inline", "__const", "__volatile", "__restrict", "__inline", "__inline__":
			// Qualifiers are currently ignored in this lowering stage.
			sawPrefixOnly = true
		case "void":
			if sawEnum || sawAggregate {
				return cTypeInfo{}, nil, false, false, fmt.Errorf("%s cannot combine aggregate/enum with void", context)
			}
			sawType = true
			sawVoid = true
		case "char":
			if sawEnum || sawAggregate {
				return cTypeInfo{}, nil, false, false, fmt.Errorf("%s cannot combine aggregate/enum with char", context)
			}
			sawType = true
			sawChar = true
		case "short":
			if sawEnum || sawAggregate {
				return cTypeInfo{}, nil, false, false, fmt.Errorf("%s cannot combine aggregate/enum with short", context)
			}
			sawType = true
			sawShort = true
		case "int":
			if sawEnum || sawAggregate {
				return cTypeInfo{}, nil, false, false, fmt.Errorf("%s cannot combine aggregate/enum with int", context)
			}
			sawType = true
			sawInt = true
		case "long":
			if sawEnum || sawAggregate {
				return cTypeInfo{}, nil, false, false, fmt.Errorf("%s cannot combine aggregate/enum with long", context)
			}
			sawType = true
			sawLongCount++
			if sawLongCount > 2 {
				return cTypeInfo{}, nil, false, false, fmt.Errorf("%s has invalid long type combination", context)
			}
		case "float":
			if sawEnum || sawAggregate {
				return cTypeInfo{}, nil, false, false, fmt.Errorf("%s cannot combine aggregate/enum with float", context)
			}
			sawType = true
			sawFloat = true
		case "double":
			if sawEnum || sawAggregate {
				return cTypeInfo{}, nil, false, false, fmt.Errorf("%s cannot combine aggregate/enum with double", context)
			}
			sawType = true
			sawDouble = true
		case "signed", "__signed":
			if sawEnum || sawAggregate {
				return cTypeInfo{}, nil, false, false, fmt.Errorf("%s cannot combine aggregate/enum with signed", context)
			}
			sawType = true
			sawSigned = true
		case "unsigned":
			if sawEnum || sawAggregate {
				return cTypeInfo{}, nil, false, false, fmt.Errorf("%s cannot combine aggregate/enum with unsigned", context)
			}
			sawType = true
			sawUnsigned = true
		default:
			if builtin, ok := lookupBuiltinTypedefAlias(t.Text); ok {
				if sawEnum || sawAggregate {
					return cTypeInfo{}, nil, false, false, fmt.Errorf("%s cannot combine aggregate/enum with typedef name %q", context, t.Text)
				}
				if sawType || sawVoid || sawChar || sawShort || sawInt || sawLongCount > 0 || sawFloat || sawDouble || sawSigned || sawUnsigned {
					return cTypeInfo{}, nil, false, false, fmt.Errorf("%s cannot combine typedef name %q with builtin type specifiers", context, t.Text)
				}
				sawType = true
				aliasSet = true
				aliasInfo = builtin
				break
			}
			if alias, ok := lookupTypedefAlias(t.Text); ok {
				if sawEnum || sawAggregate {
					return cTypeInfo{}, nil, false, false, fmt.Errorf("%s cannot combine aggregate/enum with typedef name %q", context, t.Text)
				}
				if sawType || sawVoid || sawChar || sawShort || sawInt || sawLongCount > 0 || sawFloat || sawDouble || sawSigned || sawUnsigned {
					return cTypeInfo{}, nil, false, false, fmt.Errorf("%s cannot combine typedef name %q with builtin type specifiers", context, t.Text)
				}
				sawType = true
				aliasSet = true
				aliasInfo = alias
				break
			}
			if isUnsupportedCTypeKeyword(t.Text) {
				return cTypeInfo{}, nil, false, false, fmt.Errorf("%s uses unsupported type keyword %q", context, t.Text)
			}
			return cTypeInfo{}, nil, false, false, fmt.Errorf("%s has unsupported type token %q", context, t.Text)
		}
		i++
	}

	if !sawType {
		if sawPrefixOnly {
			return cTypeInfo{Kind: cDeclScalar, Base: cScalarInt}, enumConsts, hasExtern, hasTypedef, nil
		}
		return cTypeInfo{}, nil, false, false, fmt.Errorf("%s is missing a type specifier", context)
	}
	if aliasSet {
		if aliasInfo.IsVoid && !allowVoid {
			return cTypeInfo{}, nil, false, false, fmt.Errorf("%s cannot use void type here", context)
		}
		aliasInfo.FuncSig = cloneFuncTypeSig(aliasInfo.FuncSig)
		return aliasInfo, enumConsts, hasExtern, hasTypedef, nil
	}
	if sawSigned && sawUnsigned {
		return cTypeInfo{}, nil, false, false, fmt.Errorf("%s cannot combine signed and unsigned", context)
	}
	if sawFloat && sawDouble {
		return cTypeInfo{}, nil, false, false, fmt.Errorf("%s has invalid floating-point type combination", context)
	}
	if sawAggregate {
		return cTypeInfo{
			Kind:             cDeclScalar,
			Base:             cScalarInt,
			OpaqueAggregate:  aggregateOpaque,
			AggregateKeyword: aggregateKeyword,
			AggregateTag:     aggregateTag,
		}, enumConsts, hasExtern, hasTypedef, nil
	}
	if sawEnum {
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarInt}, enumConsts, hasExtern, hasTypedef, nil
	}
	if sawVoid {
		if sawChar || sawShort || sawInt || sawLongCount > 0 || sawFloat || sawDouble || sawSigned || sawUnsigned {
			return cTypeInfo{}, nil, false, false, fmt.Errorf("%s has invalid void type combination", context)
		}
		if !allowVoid {
			return cTypeInfo{}, nil, false, false, fmt.Errorf("%s cannot use void type here", context)
		}
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarInt, IsVoid: true}, enumConsts, hasExtern, hasTypedef, nil
	}
	if sawChar {
		if sawShort || sawInt || sawLongCount > 0 || sawFloat || sawDouble {
			return cTypeInfo{}, nil, false, false, fmt.Errorf("%s has invalid char type combination", context)
		}
		if sawUnsigned {
			return cTypeInfo{Kind: cDeclScalar, Base: cScalarUChar}, enumConsts, hasExtern, hasTypedef, nil
		}
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarChar}, enumConsts, hasExtern, hasTypedef, nil
	}
	if sawShort {
		if sawLongCount > 0 || sawFloat || sawDouble {
			return cTypeInfo{}, nil, false, false, fmt.Errorf("%s cannot combine short and long", context)
		}
		if sawUnsigned {
			return cTypeInfo{Kind: cDeclScalar, Base: cScalarUShort}, enumConsts, hasExtern, hasTypedef, nil
		}
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarShort}, enumConsts, hasExtern, hasTypedef, nil
	}
	if sawFloat || sawDouble {
		if sawChar || sawShort || sawInt || sawLongCount > 0 || sawSigned || sawUnsigned {
			return cTypeInfo{}, nil, false, false, fmt.Errorf("%s has invalid floating-point type combination", context)
		}
		if sawDouble {
			return cTypeInfo{Kind: cDeclScalar, Base: cScalarDouble}, enumConsts, hasExtern, hasTypedef, nil
		}
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarFloat}, enumConsts, hasExtern, hasTypedef, nil
	}
	if sawLongCount > 0 {
		if sawUnsigned {
			return cTypeInfo{Kind: cDeclScalar, Base: cScalarULong}, enumConsts, hasExtern, hasTypedef, nil
		}
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarLong}, enumConsts, hasExtern, hasTypedef, nil
	}
	if sawUnsigned {
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarUInt}, enumConsts, hasExtern, hasTypedef, nil
	}
	_ = sawInt
	return cTypeInfo{Kind: cDeclScalar, Base: cScalarInt}, enumConsts, hasExtern, hasTypedef, nil
}

func combineTypeAndDeclarator(base cTypeInfo, declKind cDeclKind, declPtrDepth int, declArrayLen int64, allowOpaqueObject bool, context string) (cTypeInfo, error) {
	out := base
	allowsOpaqueUse := allowOpaqueObject
	if base.AggregateKeyword != "" && base.OpaqueAggregate {
		if agg, ok := lookupAggregateAlias(base.AggregateKeyword, base.AggregateTag); ok && len(agg.Fields) > 0 {
			base.OpaqueAggregate = false
			out.OpaqueAggregate = false
		}
	}
	if base.AggregateKeyword != "" && base.OpaqueAggregate && base.Kind != cDeclPointer && declKind == cDeclArray && declPtrDepth > 0 {
		allowsOpaqueUse = true
	}
	if base.AggregateKeyword != "" && base.OpaqueAggregate && base.Kind != cDeclPointer && declKind != cDeclPointer && !allowsOpaqueUse {
		what := base.AggregateKeyword
		if what == "" {
			what = "aggregate"
		}
		flavor := "opaque "
		if base.AggregateTag != "" {
			return cTypeInfo{}, fmt.Errorf("%s only supports pointers to %s%s %q types for now", context, flavor, what, base.AggregateTag)
		}
		return cTypeInfo{}, fmt.Errorf("%s only supports pointers to %s%s types for now", context, flavor, what)
	}
	switch declKind {
	case cDeclArray:
		out.Kind = cDeclArray
		out.ArrayLen = declArrayLen
		if base.Kind == cDeclArray {
			out.ArrayDims = append([]int64{base.ArrayLen}, cloneInt64s(base.ArrayDims)...)
			out.PtrDepth = declPtrDepth
		} else if base.Kind == cDeclPointer {
			out.PtrDepth = base.PtrDepth + declPtrDepth
		} else {
			out.PtrDepth = declPtrDepth
		}
	case cDeclPointer:
		if base.Kind == cDeclArray {
			return cTypeInfo{}, fmt.Errorf("%s cannot form pointer-to-array type from typedef array base", context)
		}
		if base.Kind == cDeclPointer {
			out.Kind = cDeclPointer
			out.PtrDepth = base.PtrDepth + declPtrDepth
		} else {
			out.Kind = cDeclPointer
			out.PtrDepth = declPtrDepth
		}
	case cDeclScalar:
		// No additional declarator modifiers.
	default:
		return cTypeInfo{}, fmt.Errorf("%s has unsupported declarator kind", context)
	}
	if out.Kind == cDeclPointer && out.PtrDepth == 0 {
		out.PtrDepth = 1
	}
	if base.OpaqueAggregate {
		out.OpaqueAggregate = true
		out.AggregateKeyword = base.AggregateKeyword
		out.AggregateTag = base.AggregateTag
	}
	return out, nil
}

func arrayElementTypeInfo(info cTypeInfo) cTypeInfo {
	elem := info
	if len(elem.ArrayDims) > 0 {
		elem.Kind = cDeclArray
		elem.ArrayLen = elem.ArrayDims[0]
		elem.ArrayDims = append([]int64{}, elem.ArrayDims[1:]...)
		return elem
	}
	elem.ArrayLen = 0
	elem.ArrayDims = nil
	if elem.PtrDepth > 0 {
		elem.Kind = cDeclPointer
	} else {
		elem.Kind = cDeclScalar
	}
	return elem
}

func decayArrayTypeInfo(info cTypeInfo) cTypeInfo {
	out := info
	out.Kind = cDeclPointer
	out.ArrayLen = 0
	if out.PtrDepth == 0 {
		out.PtrDepth = 1
	} else {
		out.PtrDepth++
	}
	return out
}

func cloneInt64s(src []int64) []int64 {
	if len(src) == 0 {
		return nil
	}
	out := make([]int64, len(src))
	copy(out, src)
	return out
}

func mergeEnumConstMaps(dst map[string]int64, src map[string]int64) (map[string]int64, error) {
	if len(src) == 0 {
		return dst, nil
	}
	if dst == nil {
		dst = make(map[string]int64, len(src))
	}
	for name, val := range src {
		if _, exists := dst[name]; exists {
			return nil, fmt.Errorf("duplicate enum constant %q", name)
		}
		dst[name] = val
	}
	return dst, nil
}

func applyDeclaratorArrayDims(info *cTypeInfo, dims []int64) {
	if len(dims) == 0 {
		if info.Kind != cDeclArray {
			info.ArrayDims = nil
		}
		return
	}
	info.ArrayDims = cloneInt64s(dims)
}

func cTypeInfoEquivalent(a cTypeInfo, b cTypeInfo) bool {
	if a.Kind != b.Kind || a.PtrDepth != b.PtrDepth || a.ArrayLen != b.ArrayLen || a.IsVoid != b.IsVoid || a.Base != b.Base || a.OpaqueAggregate != b.OpaqueAggregate || a.AggregateKeyword != b.AggregateKeyword || a.AggregateTag != b.AggregateTag {
		return false
	}
	if len(a.ArrayDims) != len(b.ArrayDims) {
		return false
	}
	for i := range a.ArrayDims {
		if a.ArrayDims[i] != b.ArrayDims[i] {
			return false
		}
	}
	if a.FuncSig == nil || b.FuncSig == nil {
		return a.FuncSig == nil && b.FuncSig == nil
	}
	return funcTypeSigEqual(a.FuncSig, b.FuncSig)
}

func mergeRedeclaredTypeInfo(existing cTypeInfo, next cTypeInfo) (cTypeInfo, bool) {
	if cTypeInfoEquivalent(existing, next) {
		return next, true
	}
	if existing.Kind == next.Kind &&
		existing.PtrDepth == next.PtrDepth &&
		existing.ArrayLen == next.ArrayLen &&
		existing.IsVoid == next.IsVoid &&
		existing.Base == next.Base &&
		existing.AggregateKeyword != "" &&
		existing.AggregateKeyword == next.AggregateKeyword &&
		existing.AggregateTag == next.AggregateTag &&
		len(existing.ArrayDims) == len(next.ArrayDims) {
		sameDims := true
		for i := range existing.ArrayDims {
			if existing.ArrayDims[i] != next.ArrayDims[i] {
				sameDims = false
				break
			}
		}
		if sameDims {
			if existing.FuncSig == nil || next.FuncSig == nil {
				if existing.FuncSig == nil && next.FuncSig == nil {
					if existing.OpaqueAggregate && !next.OpaqueAggregate {
						return next, true
					}
					if !existing.OpaqueAggregate && next.OpaqueAggregate {
						merged := next
						merged.OpaqueAggregate = false
						return merged, true
					}
				}
			} else if funcTypeSigEqual(existing.FuncSig, next.FuncSig) {
				if existing.OpaqueAggregate && !next.OpaqueAggregate {
					return next, true
				}
				if !existing.OpaqueAggregate && next.OpaqueAggregate {
					merged := next
					merged.OpaqueAggregate = false
					return merged, true
				}
			}
		}
	}
	if existing.Kind != cDeclArray || next.Kind != cDeclArray {
		return cTypeInfo{}, false
	}
	if existing.PtrDepth != next.PtrDepth || existing.IsVoid != next.IsVoid || existing.Base != next.Base || existing.OpaqueAggregate != next.OpaqueAggregate || existing.AggregateKeyword != next.AggregateKeyword || existing.AggregateTag != next.AggregateTag {
		return cTypeInfo{}, false
	}
	if len(existing.ArrayDims) != len(next.ArrayDims) {
		return cTypeInfo{}, false
	}
	for i := range existing.ArrayDims {
		if existing.ArrayDims[i] != next.ArrayDims[i] {
			return cTypeInfo{}, false
		}
	}
	if existing.FuncSig == nil || next.FuncSig == nil {
		if existing.FuncSig != nil || next.FuncSig != nil {
			return cTypeInfo{}, false
		}
	} else if !funcTypeSigEqual(existing.FuncSig, next.FuncSig) {
		return cTypeInfo{}, false
	}
	merged := next
	switch {
	case existing.ArrayLen == cArrayLenUnspecified && next.ArrayLen != cArrayLenUnspecified:
		return merged, true
	case next.ArrayLen == cArrayLenUnspecified && existing.ArrayLen != cArrayLenUnspecified:
		merged.ArrayLen = existing.ArrayLen
		return merged, true
	default:
		return cTypeInfo{}, false
	}
}

func matchingParenClose(tokens []Token, open int) int {
	if open < 0 || open >= len(tokens) {
		return -1
	}
	if tokens[open].Kind != TokPunct || tokens[open].Text != "(" {
		return -1
	}
	depth := 1
	for i := open + 1; i < len(tokens); i++ {
		if tokens[i].Kind != TokPunct {
			continue
		}
		if tokens[i].Text == "(" {
			depth++
		} else if tokens[i].Text == ")" {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func matchingParenOpen(tokens []Token, close int) int {
	if close < 0 || close >= len(tokens) {
		return -1
	}
	if tokens[close].Kind != TokPunct || tokens[close].Text != ")" {
		return -1
	}
	depth := 1
	for i := close - 1; i >= 0; i-- {
		if tokens[i].Kind != TokPunct {
			continue
		}
		if tokens[i].Text == ")" {
			depth++
		} else if tokens[i].Text == "(" {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func trailingFunctionSuffixOpen(tokens []Token) int {
	tokens = trimTokens(tokens)
	if len(tokens) < 2 {
		return -1
	}
	last := len(tokens) - 1
	if tokens[last].Kind != TokPunct || tokens[last].Text != ")" {
		return -1
	}
	depth := 1
	for i := last - 1; i >= 0; i-- {
		if tokens[i].Kind != TokPunct {
			continue
		}
		if tokens[i].Text == ")" {
			depth++
		} else if tokens[i].Text == "(" {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func trailingArraySuffixOpen(tokens []Token) int {
	tokens = trimTokens(tokens)
	if len(tokens) < 2 {
		return -1
	}
	last := len(tokens) - 1
	if tokens[last].Kind != TokPunct || tokens[last].Text != "]" {
		return -1
	}
	depth := 1
	for i := last - 1; i >= 0; i-- {
		if tokens[i].Kind != TokPunct {
			continue
		}
		if tokens[i].Text == "]" {
			depth++
		} else if tokens[i].Text == "[" {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func parseSimplePointerCore(tokens []Token, allowAbstract bool) (string, int, error) {
	tokens = trimTokens(tokens)
	if len(tokens) == 0 {
		if allowAbstract {
			return "", 0, nil
		}
		return "", 0, fmt.Errorf("missing declarator")
	}

	if tokens[0].Kind == TokPunct && tokens[0].Text == "(" {
		close := matchingParenClose(tokens, 0)
		if close == len(tokens)-1 {
			return parseSimplePointerCore(tokens[1:close], allowAbstract)
		}
	}

	ptrDepth := 0
	i := 0
	for i < len(tokens) && tokens[i].Kind == TokPunct && tokens[i].Text == "*" {
		ptrDepth++
		i++
		for i < len(tokens) && tokens[i].Kind == TokIdent && isTypeQualifierKeyword(tokens[i].Text) {
			i++
		}
	}
	rest := trimTokens(tokens[i:])
	if len(rest) == 0 {
		if allowAbstract {
			return "", ptrDepth, nil
		}
		return "", 0, fmt.Errorf("unable to parse declarator name")
	}
	if len(rest) == 1 && rest[0].Kind == TokIdent && !isDeclarationKeyword(rest[0]) {
		return rest[0].Text, ptrDepth, nil
	}
	if rest[0].Kind == TokPunct && rest[0].Text == "(" {
		close := matchingParenClose(rest, 0)
		if close == len(rest)-1 {
			name, innerPtr, err := parseSimplePointerCore(rest[1:close], allowAbstract)
			if err != nil {
				return "", 0, err
			}
			return name, ptrDepth + innerPtr, nil
		}
	}
	return "", 0, fmt.Errorf("complex declarators are not yet supported")
}

func trimTrailingDeclaratorDecorators(tokens []Token) []Token {
	work := trimTokens(tokens)
	for len(work) >= 3 {
		last := len(work) - 1
		if work[last].Kind != TokPunct || work[last].Text != ")" {
			break
		}
		open := matchingParenOpen(work, last)
		if open < 1 {
			break
		}
		if work[open-1].Kind != TokIdent {
			if open >= 2 && work[open-1].Kind == TokPunct && work[open-1].Text == "(" && work[open-2].Kind == TokIdent {
				switch work[open-2].Text {
				case "__attribute__", "__attribute":
					work = trimTokens(work[:open-2])
					continue
				}
			}
			break
		}
		switch work[open-1].Text {
		case "__asm", "asm", "__attribute__", "__attribute":
			work = trimTokens(work[:open-1])
			continue
		}
		break
	}
	return work
}

func isGNUAsmStatementText(text string) bool {
	trimmed := strings.TrimSpace(text)
	if strings.Contains(trimmed, "asm") && strings.Contains(trimmed, "volatile") {
		return true
	}
	return strings.HasPrefix(trimmed, "asm(") ||
		strings.HasPrefix(trimmed, "asm ") ||
		strings.HasPrefix(trimmed, "asm\t") ||
		strings.HasPrefix(trimmed, "__asm(") ||
		strings.HasPrefix(trimmed, "__asm ") ||
		strings.HasPrefix(trimmed, "__asm\t") ||
		strings.HasPrefix(trimmed, "__asm__(") ||
		strings.HasPrefix(trimmed, "__asm__ ")
}

func parseFunctionParamList(paramTokens []Token, context string) ([]cDeclKind, []cScalarType, []int, []bool, []string, []string, []*cFuncTypeSig, bool, bool, error) {
	paramTokens = trimTokens(paramTokens)
	if len(paramTokens) == 0 {
		return nil, nil, nil, nil, nil, nil, nil, true, false, nil
	}
	parts := splitTopLevel(paramTokens, ",")
	if len(parts) == 1 {
		p0 := trimTokens(parts[0])
		if len(p0) > 0 {
			spec, decl, err := splitDeclSpecPrefix(p0, context)
			if err == nil {
				baseInfo, _, _, _, err := parseScalarTypeSpec(spec, context, true)
				if err == nil {
					_, kind, ptrDepth, arrLen, arrDims, _, _, err := parseDeclarator(decl, true, nil)
					if err == nil {
						info, cerr := combineTypeAndDeclarator(baseInfo, kind, ptrDepth, arrLen, false, context)
						if cerr == nil {
							applyDeclaratorArrayDims(&info, arrDims)
						}
						if cerr == nil && info.IsVoid && info.Kind == cDeclScalar {
							parts = nil
						}
					}
				}
			}
		}
	}

	variadic := false
	paramKinds := make([]cDeclKind, 0, len(parts))
	paramBases := make([]cScalarType, 0, len(parts))
	paramPtrDepth := make([]int, 0, len(parts))
	paramOpaque := make([]bool, 0, len(parts))
	paramAggKey := make([]string, 0, len(parts))
	paramAggTag := make([]string, 0, len(parts))
	paramFuncSigs := make([]*cFuncTypeSig, 0, len(parts))
	for i, p := range parts {
		p = trimTokens(p)
		if len(p) == 0 {
			continue
		}
		if len(p) == 1 && p[0].Kind == TokPunct && p[0].Text == "..." {
			if i != len(parts)-1 || len(paramKinds) == 0 {
				return nil, nil, nil, nil, nil, nil, nil, false, false, fmt.Errorf("variadic marker must appear last after at least one named parameter")
			}
			variadic = true
			continue
		}
		pctx := fmt.Sprintf("%s parameter %d", context, i+1)
		spec, decl, err := splitDeclSpecPrefix(p, pctx)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, nil, false, false, err
		}
		pbaseInfo, _, _, _, err := parseScalarTypeSpec(spec, pctx, true)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, nil, false, false, err
		}
		pname, pdeclKind, pdeclPtrDepth, parrLen, parrDims, pfnSig, directFunc, err := parseDeclarator(decl, true, nil)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, nil, false, false, err
		}
		if directFunc {
			pdeclKind = cDeclPointer
			pdeclPtrDepth = 1
		}
		pinfo, err := combineTypeAndDeclarator(pbaseInfo, pdeclKind, pdeclPtrDepth, parrLen, false, pctx)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, nil, false, false, err
		}
		applyDeclaratorArrayDims(&pinfo, parrDims)
		if pinfo.Kind == cDeclArray {
			// Arrays in parameter lists decay to pointers.
			pinfo = decayArrayTypeInfo(pinfo)
		}
		if pinfo.IsVoid && pinfo.Kind == cDeclScalar {
			if pname == "" && i == 0 && len(parts) == 1 {
				// handled above for `void` empty parameter list; keep defensive fallback.
				return nil, nil, nil, nil, nil, nil, nil, false, false, nil
			}
			return nil, nil, nil, nil, nil, nil, nil, false, false, fmt.Errorf("%s parameter %q cannot have type void", context, pname)
		}
		if pfnSig != nil {
			pfn := cloneFuncTypeSig(pfnSig)
			applyFuncReturnBase(pfn, pbaseInfo)
			pinfo.FuncSig = pfn
		}
		paramKinds = append(paramKinds, pinfo.Kind)
		paramBases = append(paramBases, pinfo.Base)
		paramPtrDepth = append(paramPtrDepth, pinfo.PtrDepth)
		paramOpaque = append(paramOpaque, pinfo.OpaqueAggregate)
		paramAggKey = append(paramAggKey, pinfo.AggregateKeyword)
		paramAggTag = append(paramAggTag, pinfo.AggregateTag)
		paramFuncSigs = append(paramFuncSigs, cloneFuncTypeSig(pinfo.FuncSig))
	}
	return paramKinds, paramBases, paramPtrDepth, paramOpaque, paramAggKey, paramAggTag, paramFuncSigs, false, variadic, nil
}

func parseTrailingArraySuffixes(tokens []Token) ([]Token, []int64, error) {
	work := trimTokens(tokens)
	var lens []int64
	for {
		arrOpen := trailingArraySuffixOpen(work)
		if arrOpen < 0 {
			break
		}
		n, err := parseArrayLength(work[arrOpen+1:len(work)-1], nil)
		if err != nil {
			return nil, nil, err
		}
		lens = append([]int64{n}, lens...)
		work = trimTokens(work[:arrOpen])
	}
	return work, lens, nil
}

func matchingBracketClose(tokens []Token, open int) int {
	if open < 0 || open >= len(tokens) {
		return -1
	}
	if tokens[open].Kind != TokPunct || tokens[open].Text != "[" {
		return -1
	}
	depth := 1
	for i := open + 1; i < len(tokens); i++ {
		if tokens[i].Kind != TokPunct {
			continue
		}
		if tokens[i].Text == "[" {
			depth++
		} else if tokens[i].Text == "]" {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func matchingBraceClose(tokens []Token, open int) int {
	if open < 0 || open >= len(tokens) {
		return -1
	}
	if tokens[open].Kind != TokPunct || tokens[open].Text != "{" {
		return -1
	}
	depth := 1
	for i := open + 1; i < len(tokens); i++ {
		if tokens[i].Kind != TokPunct {
			continue
		}
		if tokens[i].Text == "{" {
			depth++
		} else if tokens[i].Text == "}" {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func parseDeclaratorNode(tokens []Token, allowAbstract bool, enumLookup map[string]int64) (*cDeclaratorNode, int, error) {
	node := &cDeclaratorNode{}
	i := 0
	for i < len(tokens) && tokens[i].Kind == TokPunct && tokens[i].Text == "*" {
		node.PtrPrefix++
		i++
		for i < len(tokens) && tokens[i].Kind == TokIdent && isTypeQualifierKeyword(tokens[i].Text) {
			i++
		}
	}

	if i < len(tokens) && tokens[i].Kind == TokIdent && !isDeclarationKeyword(tokens[i]) {
		node.Name = tokens[i].Text
		i++
	} else if i < len(tokens) && tokens[i].Kind == TokPunct && tokens[i].Text == "(" {
		close := matchingParenClose(tokens, i)
		if close < 0 {
			return nil, 0, fmt.Errorf("unterminated parenthesized declarator")
		}
		sub, consumed, err := parseDeclaratorNode(tokens[i+1:close], allowAbstract, enumLookup)
		if err != nil {
			return nil, 0, err
		}
		if consumed != close-(i+1) {
			return nil, 0, fmt.Errorf("complex declarators are not yet supported")
		}
		node.Inner = sub
		i = close + 1
	} else if !allowAbstract {
		return nil, 0, fmt.Errorf("unable to parse declarator name")
	}

	for i < len(tokens) {
		if tokens[i].Kind != TokPunct {
			break
		}
		switch tokens[i].Text {
		case "[":
			close := matchingBracketClose(tokens, i)
			if close < 0 {
				return nil, 0, fmt.Errorf("unterminated array declarator")
			}
			n, err := parseArrayLength(tokens[i+1:close], enumLookup)
			if err != nil {
				return nil, 0, err
			}
			node.Suffixes = append(node.Suffixes, cDeclaratorSuffix{ArrayLen: n})
			i = close + 1
		case "(":
			close := matchingParenClose(tokens, i)
			if close < 0 {
				return nil, 0, fmt.Errorf("unterminated function declarator")
			}
			paramKinds, paramBases, paramPtrDepth, paramOpaque, paramAggKey, paramAggTag, paramFuncSigs, paramUnspecified, variadic, err := parseFunctionParamList(tokens[i+1:close], "function declarator")
			if err != nil {
				return nil, 0, err
			}
			node.Suffixes = append(node.Suffixes, cDeclaratorSuffix{
				IsFunction:       true,
				ParamKinds:       paramKinds,
				ParamUnspecified: paramUnspecified,
				ParamBases:       paramBases,
				ParamPtrDepth:    paramPtrDepth,
				ParamOpaque:      paramOpaque,
				ParamAggKey:      paramAggKey,
				ParamAggTag:      paramAggTag,
				ParamFuncSigs:    paramFuncSigs,
				Variadic:         variadic,
			})
			i = close + 1
		default:
			return node, i, nil
		}
	}
	return node, i, nil
}

func declaratorNodeName(node *cDeclaratorNode) string {
	if node == nil {
		return ""
	}
	if node.Inner != nil {
		return declaratorNodeName(node.Inner)
	}
	return node.Name
}

func buildDeclaratorEntity(node *cDeclaratorNode, base *cDeclaratorEntity) *cDeclaratorEntity {
	cur := base
	if node == nil {
		return cur
	}
	for i := 0; i < node.PtrPrefix; i++ {
		cur = &cDeclaratorEntity{Kind: cDeclaratorEntityPointer, Inner: cur}
	}
	for i := len(node.Suffixes) - 1; i >= 0; i-- {
		suf := node.Suffixes[i]
		if suf.IsFunction {
			cur = &cDeclaratorEntity{
				Kind:             cDeclaratorEntityFunction,
				Inner:            cur,
				ParamKinds:       append([]cDeclKind{}, suf.ParamKinds...),
				ParamUnspecified: suf.ParamUnspecified,
				ParamBases:       append([]cScalarType{}, suf.ParamBases...),
				ParamPtr:         append([]int{}, suf.ParamPtrDepth...),
				ParamOpaque:      append([]bool{}, suf.ParamOpaque...),
				ParamAggKey:      append([]string{}, suf.ParamAggKey...),
				ParamAggTag:      append([]string{}, suf.ParamAggTag...),
				ParamFuncs:       cloneFuncTypeSigs(suf.ParamFuncSigs),
				Variadic:         suf.Variadic,
			}
			continue
		}
		cur = &cDeclaratorEntity{Kind: cDeclaratorEntityArray, Inner: cur, ArrayLen: suf.ArrayLen}
	}
	if node.Inner != nil {
		return buildDeclaratorEntity(node.Inner, cur)
	}
	return cur
}

func cloneFuncTypeSigs(src []*cFuncTypeSig) []*cFuncTypeSig {
	if len(src) == 0 {
		return nil
	}
	out := make([]*cFuncTypeSig, len(src))
	for i := range src {
		out[i] = cloneFuncTypeSig(src[i])
	}
	return out
}

func applyFuncReturnBase(sig *cFuncTypeSig, baseInfo cTypeInfo) {
	if sig == nil {
		return
	}
	info := baseInfo
	if info.Kind == cDeclArray {
		info = decayArrayTypeInfo(info)
	}
	if sig.RetPtrDepth > 0 {
		if info.Kind == cDeclPointer {
			info.PtrDepth += sig.RetPtrDepth
		} else {
			info.Kind = cDeclPointer
			info.PtrDepth = sig.RetPtrDepth
		}
	}
	sig.RetKind = info.Kind
	sig.RetBase = info.Base
	sig.RetPtrDepth = info.PtrDepth
	sig.RetIsVoid = info.IsVoid
	sig.RetByPtr = isAggregateObjectType(info)
	sig.RetOpaque = info.OpaqueAggregate
	sig.RetAggKeyword = info.AggregateKeyword
	sig.RetAggTag = info.AggregateTag
}

func declaratorReturnPtrDepth(ent *cDeclaratorEntity) (int, error) {
	if ent == nil {
		return 0, nil
	}
	switch ent.Kind {
	case cDeclaratorEntityBase:
		return 0, nil
	case cDeclaratorEntityPointer:
		if ent.Inner != nil && ent.Inner.Kind == cDeclaratorEntityFunction {
			// Collapse function-returning-function-pointer to a plain pointer return.
			// This preserves old C hosted declarations like `(*signal())()`,
			// even though nested function return signatures are not modeled yet.
			return 1, nil
		}
		n, err := declaratorReturnPtrDepth(ent.Inner)
		if err != nil {
			return 0, err
		}
		return n + 1, nil
	case cDeclaratorEntityArray:
		return 0, fmt.Errorf("function cannot return array type")
	case cDeclaratorEntityFunction:
		return 0, fmt.Errorf("function declarators are only supported through pointers")
	default:
		return 0, fmt.Errorf("unsupported function return declarator")
	}
}

func buildDeclaratorFunctionSig(ent *cDeclaratorEntity) (*cFuncTypeSig, error) {
	if ent == nil || ent.Kind != cDeclaratorEntityFunction {
		return nil, fmt.Errorf("expected function declarator")
	}
	retPtrDepth, err := declaratorReturnPtrDepth(ent.Inner)
	if err != nil {
		return nil, err
	}
	return &cFuncTypeSig{
		RetPtrDepth:      retPtrDepth,
		ParamCount:       len(ent.ParamKinds),
		ParamUnspecified: ent.ParamUnspecified,
		Variadic:         ent.Variadic,
		ParamKinds:       append([]cDeclKind{}, ent.ParamKinds...),
		ParamBases:       append([]cScalarType{}, ent.ParamBases...),
		ParamPtrDepth:    append([]int{}, ent.ParamPtr...),
		ParamOpaque:      append([]bool{}, ent.ParamOpaque...),
		ParamAggKey:      append([]string{}, ent.ParamAggKey...),
		ParamAggTag:      append([]string{}, ent.ParamAggTag...),
		ParamFuncSigs:    cloneFuncTypeSigs(ent.ParamFuncs),
	}, nil
}

func flattenDeclaratorEntity(ent *cDeclaratorEntity) (cDeclKind, int, []int64, *cFuncTypeSig, bool, error) {
	if ent == nil {
		return cDeclScalar, 0, nil, nil, false, nil
	}
	switch ent.Kind {
	case cDeclaratorEntityBase:
		return cDeclScalar, 0, nil, nil, false, nil
	case cDeclaratorEntityPointer:
		if ent.Inner != nil && ent.Inner.Kind == cDeclaratorEntityFunction {
			fnSig, err := buildDeclaratorFunctionSig(ent.Inner)
			if err != nil {
				return cDeclScalar, 0, nil, nil, false, err
			}
			return cDeclPointer, 1, nil, fnSig, false, nil
		}
		kind, ptrDepth, lens, fnSig, directFunc, err := flattenDeclaratorEntity(ent.Inner)
		if err != nil {
			return cDeclScalar, 0, nil, nil, false, err
		}
		if kind == cDeclArray {
			return cDeclScalar, 0, nil, nil, false, fmt.Errorf("pointer-to-array declarators are not yet supported")
		}
		if kind == cDeclScalar {
			kind = cDeclPointer
		}
		return kind, ptrDepth + 1, lens, fnSig, directFunc, nil
	case cDeclaratorEntityArray:
		kind, ptrDepth, lens, fnSig, directFunc, err := flattenDeclaratorEntity(ent.Inner)
		if err != nil {
			return cDeclScalar, 0, nil, nil, false, err
		}
		if directFunc {
			return cDeclScalar, 0, nil, nil, false, fmt.Errorf("function declarators are only supported through pointers")
		}
		if kind != cDeclArray {
			kind = cDeclArray
		}
		lens = append([]int64{ent.ArrayLen}, lens...)
		return kind, ptrDepth, lens, fnSig, false, nil
	case cDeclaratorEntityFunction:
		fnSig, err := buildDeclaratorFunctionSig(ent)
		if err != nil {
			return cDeclScalar, 0, nil, nil, false, err
		}
		return cDeclScalar, 0, nil, fnSig, true, nil
	default:
		return cDeclScalar, 0, nil, nil, false, fmt.Errorf("unsupported declarator")
	}
}

func parseDeclarator(tokens []Token, allowAbstract bool, enumLookup map[string]int64) (string, cDeclKind, int, int64, []int64, *cFuncTypeSig, bool, error) {
	tokens = trimTrailingDeclaratorDecorators(tokens)
	if len(tokens) == 0 {
		if allowAbstract {
			return "", cDeclScalar, 0, 0, nil, nil, false, nil
		}
		return "", cDeclScalar, 0, 0, nil, nil, false, fmt.Errorf("missing declarator")
	}
	node, consumed, err := parseDeclaratorNode(tokens, allowAbstract, enumLookup)
	if err != nil {
		return "", cDeclScalar, 0, 0, nil, nil, false, err
	}
	if consumed != len(tokens) {
		return "", cDeclScalar, 0, 0, nil, nil, false, fmt.Errorf("complex declarators are not yet supported")
	}
	entity := buildDeclaratorEntity(node, &cDeclaratorEntity{Kind: cDeclaratorEntityBase})
	kind, ptrDepth, lens, fnSig, directFunc, err := flattenDeclaratorEntity(entity)
	if err != nil {
		return "", cDeclScalar, 0, 0, nil, nil, false, err
	}
	if len(lens) == 0 {
		return declaratorNodeName(node), kind, ptrDepth, 0, nil, fnSig, directFunc, nil
	}
	return declaratorNodeName(node), kind, ptrDepth, lens[0], lens[1:], fnSig, directFunc, nil
}

func parseArrayLength(tokens []Token, lookup map[string]int64) (int64, error) {
	tokens = trimTokens(tokens)
	if len(tokens) == 0 {
		return cArrayLenUnspecified, nil
	}
	filtered := make([]Token, 0, len(tokens))
	for _, t := range tokens {
		if t.Kind == TokIdent && (isTypeQualifierKeyword(t.Text) || t.Text == "static") {
			continue
		}
		filtered = append(filtered, t)
	}
	if len(filtered) == 0 {
		return cArrayLenUnspecified, nil
	}
	n, err := parseEnumConstExprTokens(filtered, lookup, nil)
	if err != nil {
		return 0, err
	}
	if n <= 0 {
		return 0, fmt.Errorf("array bounds must be positive")
	}
	return n, nil
}

func parseArrayDesignatorIndex(tokens []Token, lookup map[string]int64) (int64, error) {
	tokens = trimTokens(tokens)
	if len(tokens) == 0 {
		return 0, fmt.Errorf("initializer designator index is empty")
	}
	n, err := parseEnumConstExprTokens(tokens, lookup, nil)
	if err != nil {
		return 0, err
	}
	if n < 0 {
		return 0, fmt.Errorf("initializer designator index must not be negative")
	}
	return n, nil
}

type enumConstParser struct {
	toks   []Token
	pos    int
	lookup map[string]int64
	sizes  map[string]int64
}

func (p *enumConstParser) atEnd() bool {
	return p.pos >= len(p.toks)
}

func (p *enumConstParser) peek() Token {
	if p.atEnd() {
		return Token{Kind: TokEOF}
	}
	return p.toks[p.pos]
}

func (p *enumConstParser) advance() Token {
	t := p.peek()
	if !p.atEnd() {
		p.pos++
	}
	return t
}

func (p *enumConstParser) matchPunct(op string) bool {
	if p.atEnd() {
		return false
	}
	t := p.peek()
	if t.Kind == TokPunct && t.Text == op {
		p.pos++
		return true
	}
	return false
}

func (p *enumConstParser) findClosingParen(open int) int {
	if open < 0 || open >= len(p.toks) || p.toks[open].Kind != TokPunct || p.toks[open].Text != "(" {
		return -1
	}
	depth := 1
	for i := open + 1; i < len(p.toks); i++ {
		t := p.toks[i]
		if t.Kind != TokPunct {
			continue
		}
		switch t.Text {
		case "(":
			depth++
		case ")":
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func lookupConstExprSize(name string, sizes map[string]int64) (int64, bool) {
	if sizes != nil {
		if n, ok := sizes[name]; ok {
			return n, true
		}
	}
	return lookupConstObjectSize(name)
}

func parseConstExprSizeofOperand(tokens []Token, sizes map[string]int64) (int64, bool, error) {
	tokens = trimTokens(tokens)
	if len(tokens) == 0 {
		return 0, false, nil
	}
	if len(tokens) == 1 && tokens[0].Kind == TokIdent {
		if n, ok := lookupConstExprSize(tokens[0].Text, sizes); ok {
			return n, true, nil
		}
	}
	info, err := parseCTypeInfo(tokens)
	if err == nil {
		n, _, err := cTypeLayout(info)
		if err != nil {
			return 0, true, err
		}
		return n, true, nil
	}
	return 0, false, nil
}

func isEnumConstCastToken(t Token) bool {
	if t.Kind != TokIdent {
		return false
	}
	switch t.Text {
	case "char", "short", "int", "long", "signed", "unsigned", "__signed", "const", "volatile", "restrict", "__const", "__volatile", "__restrict":
		return true
	default:
		return false
	}
}

type enumConstCastInfo struct {
	apply    bool
	unsigned bool
	bits     int
}

func classifyEnumConstCast(tokens []Token) enumConstCastInfo {
	info := enumConstCastInfo{apply: true, bits: 32}
	var sawUnsigned bool
	var sawChar bool
	var sawShort bool
	var sawLong bool
	for _, t := range tokens {
		if t.Kind != TokIdent {
			continue
		}
		switch t.Text {
		case "unsigned":
			sawUnsigned = true
		case "char":
			sawChar = true
		case "short":
			sawShort = true
		case "long":
			sawLong = true
		}
	}
	info.unsigned = sawUnsigned
	switch {
	case sawChar:
		info.bits = 8
	case sawShort:
		info.bits = 16
	case sawLong:
		info.bits = int(currentCTargetLongSize() * 8)
	}
	return info
}

func applyEnumConstCast(v int64, info enumConstCastInfo) int64 {
	if !info.apply {
		return v
	}
	switch info.bits {
	case 8:
		if info.unsigned {
			return int64(uint8(v))
		}
		return int64(int8(v))
	case 16:
		if info.unsigned {
			return int64(uint16(v))
		}
		return int64(int16(v))
	case 32:
		if info.unsigned {
			return int64(uint32(v))
		}
		return int64(int32(v))
	default:
		if info.unsigned {
			return int64(uint64(v))
		}
		return v
	}
}

func (p *enumConstParser) consumeSimpleCast() (enumConstCastInfo, bool) {
	if p.atEnd() || p.peek().Kind != TokPunct || p.peek().Text != "(" {
		return enumConstCastInfo{}, false
	}
	depth := 0
	close := -1
	for i := p.pos; i < len(p.toks); i++ {
		t := p.toks[i]
		if t.Kind != TokPunct {
			continue
		}
		if t.Text == "(" {
			depth++
			continue
		}
		if t.Text == ")" {
			depth--
			if depth == 0 {
				close = i
				break
			}
		}
	}
	if close < 0 {
		return enumConstCastInfo{}, false
	}
	inner := trimTokens(p.toks[p.pos+1 : close])
	if len(inner) == 0 {
		return enumConstCastInfo{}, false
	}
	for _, t := range inner {
		if !isEnumConstCastToken(t) {
			return enumConstCastInfo{}, false
		}
	}
	p.pos = close + 1
	return classifyEnumConstCast(inner), true
}

func (p *enumConstParser) parsePrimary() (int64, error) {
	if p.atEnd() {
		return 0, fmt.Errorf("unexpected end of enum constant expression")
	}
	t := p.advance()
	switch t.Kind {
	case TokNumber:
		v, err := parseCIntLiteral(t.Text)
		if err != nil {
			return 0, err
		}
		return v, nil
	case TokChar:
		v, err := parseCCharLiteral(t.Text)
		if err != nil {
			return 0, err
		}
		return v, nil
	case TokIdent:
		if v, ok := p.lookup[t.Text]; ok {
			return v, nil
		}
		return 0, fmt.Errorf("unknown enum constant %q", t.Text)
	case TokPunct:
		if t.Text == "(" {
			v, err := p.parseExpr()
			if err != nil {
				return 0, err
			}
			if !p.matchPunct(")") {
				return 0, fmt.Errorf("expected ')' in enum constant expression")
			}
			return v, nil
		}
	}
	return 0, fmt.Errorf("invalid token %q in enum constant expression", t.Text)
}

func (p *enumConstParser) parseUnary() (int64, error) {
	if !p.atEnd() && p.peek().Kind == TokIdent && p.peek().Text == "sizeof" {
		p.pos++
		if p.matchPunct("(") {
			open := p.pos - 1
			close := p.findClosingParen(open)
			if close < 0 {
				return 0, fmt.Errorf("expected ')' after sizeof")
			}
			inner := p.toks[open+1 : close]
			p.pos = close + 1
			if n, ok, err := parseConstExprSizeofOperand(inner, p.sizes); ok || err != nil {
				return n, err
			}
			return 0, fmt.Errorf("unsupported sizeof operand in constant expression")
		}
		if p.atEnd() {
			return 0, fmt.Errorf("missing operand after sizeof")
		}
		tok := p.advance()
		if tok.Kind == TokIdent {
			if n, ok := lookupConstExprSize(tok.Text, p.sizes); ok {
				return n, nil
			}
		}
		return 0, fmt.Errorf("unsupported sizeof operand in constant expression")
	}
	if cast, ok := p.consumeSimpleCast(); ok {
		v, err := p.parseUnary()
		if err != nil {
			return 0, err
		}
		return applyEnumConstCast(v, cast), nil
	}
	if p.matchPunct("+") {
		return p.parseUnary()
	}
	if p.matchPunct("-") {
		v, err := p.parseUnary()
		return -v, err
	}
	if p.matchPunct("~") {
		v, err := p.parseUnary()
		return ^v, err
	}
	return p.parsePrimary()
}

func (p *enumConstParser) parseMul() (int64, error) {
	v, err := p.parseUnary()
	if err != nil {
		return 0, err
	}
	for {
		if p.matchPunct("*") {
			r, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			v = v * r
			continue
		}
		if p.matchPunct("/") {
			r, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			if r == 0 {
				return 0, fmt.Errorf("division by zero in enum constant expression")
			}
			v = v / r
			continue
		}
		if p.matchPunct("%") {
			r, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			if r == 0 {
				return 0, fmt.Errorf("modulo by zero in enum constant expression")
			}
			v = v % r
			continue
		}
		break
	}
	return v, nil
}

func (p *enumConstParser) parseAdd() (int64, error) {
	v, err := p.parseMul()
	if err != nil {
		return 0, err
	}
	for {
		if p.matchPunct("+") {
			r, err := p.parseMul()
			if err != nil {
				return 0, err
			}
			v = v + r
			continue
		}
		if p.matchPunct("-") {
			r, err := p.parseMul()
			if err != nil {
				return 0, err
			}
			v = v - r
			continue
		}
		break
	}
	return v, nil
}

func (p *enumConstParser) parseShift() (int64, error) {
	v, err := p.parseAdd()
	if err != nil {
		return 0, err
	}
	for {
		if p.matchPunct("<<") {
			r, err := p.parseAdd()
			if err != nil {
				return 0, err
			}
			v = v << uint64(r)
			continue
		}
		if p.matchPunct(">>") {
			r, err := p.parseAdd()
			if err != nil {
				return 0, err
			}
			v = v >> uint64(r)
			continue
		}
		break
	}
	return v, nil
}

func (p *enumConstParser) parseAnd() (int64, error) {
	v, err := p.parseShift()
	if err != nil {
		return 0, err
	}
	for p.matchPunct("&") {
		r, err := p.parseShift()
		if err != nil {
			return 0, err
		}
		v = v & r
	}
	return v, nil
}

func (p *enumConstParser) parseXor() (int64, error) {
	v, err := p.parseAnd()
	if err != nil {
		return 0, err
	}
	for p.matchPunct("^") {
		r, err := p.parseAnd()
		if err != nil {
			return 0, err
		}
		v = v ^ r
	}
	return v, nil
}

func (p *enumConstParser) parseOr() (int64, error) {
	v, err := p.parseXor()
	if err != nil {
		return 0, err
	}
	for p.matchPunct("|") {
		r, err := p.parseXor()
		if err != nil {
			return 0, err
		}
		v = v | r
	}
	return v, nil
}

func (p *enumConstParser) parseExpr() (int64, error) {
	return p.parseOr()
}

func parseEnumConstExprTokens(toks []Token, lookup map[string]int64, sizes map[string]int64) (int64, error) {
	p := &enumConstParser{toks: trimTokens(toks), lookup: lookup, sizes: sizes}
	v, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	if !p.atEnd() {
		return 0, fmt.Errorf("unexpected token %q in enum constant expression", p.peek().Text)
	}
	return v, nil
}

func copyEnumLookupMap(src map[string]int64) map[string]int64 {
	if len(src) == 0 {
		return make(map[string]int64)
	}
	out := make(map[string]int64, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func parseEnumSpecifierAndConstants(toks []Token, enumLookup map[string]int64) (int, map[string]int64, error) {
	toks = trimTokens(toks)
	if len(toks) == 0 || toks[0].Kind != TokIdent || toks[0].Text != "enum" {
		return 0, nil, fmt.Errorf("expected enum specifier")
	}
	i := 1
	if i < len(toks) && toks[i].Kind == TokIdent && toks[i].Text != "{" {
		i++
	}
	if i >= len(toks) || toks[i].Kind != TokPunct || toks[i].Text != "{" {
		if i <= 1 {
			return 0, nil, fmt.Errorf("enum specifier requires tag or enumerator list")
		}
		return i, nil, nil
	}
	depth := 1
	j := i + 1
	for j < len(toks) {
		t := toks[j]
		if t.Kind == TokPunct {
			if t.Text == "{" {
				depth++
			} else if t.Text == "}" {
				depth--
				if depth == 0 {
					break
				}
			}
		}
		j++
	}
	if j >= len(toks) || depth != 0 {
		return 0, nil, fmt.Errorf("unterminated enum enumerator list")
	}
	body := trimTokens(toks[i+1 : j])
	parts := splitTopLevel(body, ",")
	vals := make(map[string]int64)
	next := int64(0)
	for _, p := range parts {
		p = trimTokens(p)
		if len(p) == 0 {
			continue
		}
		eqIdx := -1
		dParen := 0
		for k, t := range p {
			if t.Kind != TokPunct {
				continue
			}
			if t.Text == "(" {
				dParen++
			} else if t.Text == ")" {
				if dParen > 0 {
					dParen--
				}
			} else if t.Text == "=" && dParen == 0 {
				eqIdx = k
				break
			}
		}
		lhs := p
		var rhs []Token
		if eqIdx >= 0 {
			lhs = trimTokens(p[:eqIdx])
			rhs = trimTokens(p[eqIdx+1:])
		}
		if len(lhs) != 1 || lhs[0].Kind != TokIdent {
			return 0, nil, fmt.Errorf("invalid enum enumerator declaration (%s)", tokenSliceText(p))
		}
		name := lhs[0].Text
		v := next
		if eqIdx >= 0 {
			resolver := copyEnumLookupMap(enumLookup)
			for name, val := range vals {
				resolver[name] = val
			}
			ev, err := parseEnumConstExprTokens(rhs, resolver, nil)
			if err != nil {
				return 0, nil, fmt.Errorf("invalid value for enum constant %q: %v", name, err)
			}
			v = ev
		}
		vals[name] = v
		next = v + 1
	}
	return j + 1, vals, nil
}

func parseDeclItemsWithBase(baseInfo cTypeInfo, enumLookup map[string]int64, hasTypedef bool, allowOpaqueObject bool, allowIncompleteArray bool, rest []Token, allowRuntimeArrays bool) ([]cDeclItem, error) {
	parts := splitTopLevel(rest, ",")
	items := make([]cDeclItem, 0, len(parts))
	prevDeclSizes := cConstSizeLookupDecl
	declSizes := make(map[string]int64)
	cConstSizeLookupDecl = declSizes
	defer func() {
		cConstSizeLookupDecl = prevDeclSizes
	}()
	for _, part := range parts {
		part = trimTokens(part)
		if len(part) == 0 {
			continue
		}

		eqIdx := -1
		depthParen := 0
		depthBracket := 0
		depthBrace := 0
		depthTernary := 0
		for i, t := range part {
			if t.Kind == TokPunct {
				switch t.Text {
				case "(":
					depthParen++
				case ")":
					if depthParen > 0 {
						depthParen--
					}
				case "[":
					depthBracket++
				case "]":
					if depthBracket > 0 {
						depthBracket--
					}
				case "{":
					depthBrace++
				case "}":
					if depthBrace > 0 {
						depthBrace--
					}
				case "?":
					if depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
						depthTernary++
					}
				case ":":
					if depthParen == 0 && depthBracket == 0 && depthBrace == 0 && depthTernary > 0 {
						depthTernary--
					}
				case "=":
					if eqIdx < 0 && depthParen == 0 && depthBracket == 0 && depthBrace == 0 && depthTernary == 0 {
						eqIdx = i
					}
				}
			}
		}

		lhs := part
		var init []Token
		if eqIdx >= 0 {
			lhs = part[:eqIdx]
			init = trimTokens(part[eqIdx+1:])
		}
		lhs = trimTokens(lhs)
		if len(lhs) == 0 {
			return nil, fmt.Errorf("missing declarator in declaration")
		}

		if allowRuntimeArrays && !hasTypedef {
			if item, ok, err := tryParseRuntimeArrayDeclItem(baseInfo, lhs, init); err != nil {
				return nil, err
			} else if ok {
				items = append(items, item)
				continue
			}
		}

		name, kind, ptrDepth, arrayLen, arrayDims, fnSig, directFunc, err := parseDeclarator(lhs, false, enumLookup)
		if err != nil {
			return nil, fmt.Errorf("%s (%s)", err, tokenSliceText(lhs))
		}
		info, err := combineTypeAndDeclarator(baseInfo, kind, ptrDepth, arrayLen, allowOpaqueObject || hasTypedef, fmt.Sprintf("declaration of %q", name))
		if err != nil {
			return nil, err
		}
		applyDeclaratorArrayDims(&info, arrayDims)
		if fnSig != nil {
			sig := cloneFuncTypeSig(fnSig)
			applyFuncReturnBase(sig, baseInfo)
			info.FuncSig = sig
		}
		if info.Kind == cDeclArray {
			for _, n := range info.ArrayDims {
				if n == cArrayLenUnspecified {
					return nil, fmt.Errorf("only the first array bound may be empty")
				}
			}
		}
		if info.Kind == cDeclArray && info.ArrayLen == cArrayLenUnspecified {
			n, ok, err := inferArrayLengthFromInit(info.Base, init, enumLookup)
			if err != nil {
				return nil, err
			}
			if ok {
				info.ArrayLen = n
			} else if !allowIncompleteArray {
				return nil, fmt.Errorf("array bounds must not be empty")
			}
		}
		if !hasTypedef && info.IsVoid && info.Kind != cDeclPointer && info.FuncSig == nil {
			return nil, fmt.Errorf("declaration of %q cannot use void object type", name)
		}
		items = append(items, cDeclItem{
			Name:             name,
			Init:             init,
			Kind:             info.Kind,
			DirectFunc:       directFunc,
			PtrDepth:         info.PtrDepth,
			ArrayLen:         info.ArrayLen,
			ArrayDims:        cloneInt64s(info.ArrayDims),
			IsVoid:           info.IsVoid,
			Base:             info.Base,
			FuncSig:          cloneFuncTypeSig(info.FuncSig),
			OpaqueAggregate:  info.OpaqueAggregate,
			AggregateKeyword: info.AggregateKeyword,
			AggregateTag:     info.AggregateTag,
		})
		if name != "" {
			if size, _, err := cTypeLayout(info); err == nil {
				declSizes[name] = size
			}
		}
	}
	return items, nil
}

func tryParseRuntimeArrayDeclItem(baseInfo cTypeInfo, lhs []Token, init []Token) (cDeclItem, bool, error) {
	lhs = trimTokens(lhs)
	if len(init) != 0 || len(lhs) < 4 {
		return cDeclItem{}, false, nil
	}
	last := len(lhs) - 1
	if lhs[last].Kind != TokPunct || lhs[last].Text != "]" {
		return cDeclItem{}, false, nil
	}
	open := -1
	for i := last - 1; i >= 0; i-- {
		if lhs[i].Kind == TokPunct && lhs[i].Text == "[" {
			if matchingBracketClose(lhs, i) == last {
				open = i
				break
			}
		}
	}
	if open <= 0 {
		return cDeclItem{}, false, nil
	}
	bound := trimTokens(lhs[open+1 : last])
	if len(bound) == 0 {
		return cDeclItem{}, false, nil
	}
	if _, err := parseArrayLength(bound, nil); err == nil {
		return cDeclItem{}, false, nil
	}

	baseDecl := trimTokens(lhs[:open])
	name, kind, ptrDepth, arrayLen, arrayDims, fnSig, directFunc, err := parseDeclarator(baseDecl, false, nil)
	if err != nil {
		return cDeclItem{}, false, nil
	}
	if directFunc || fnSig != nil || kind == cDeclArray || arrayLen != 0 || len(arrayDims) != 0 {
		return cDeclItem{}, false, nil
	}
	elemInfo, err := combineTypeAndDeclarator(baseInfo, kind, ptrDepth, 0, false, fmt.Sprintf("declaration of %q", name))
	if err != nil {
		return cDeclItem{}, false, err
	}
	info := elemInfo
	if info.Kind == cDeclPointer {
		info.PtrDepth++
	} else {
		info.Kind = cDeclPointer
		info.PtrDepth = 1
	}
	return cDeclItem{
		Name:              name,
		Kind:              info.Kind,
		PtrDepth:          info.PtrDepth,
		IsVoid:            info.IsVoid,
		Base:              info.Base,
		FuncSig:           cloneFuncTypeSig(info.FuncSig),
		OpaqueAggregate:   info.OpaqueAggregate,
		AggregateKeyword:  info.AggregateKeyword,
		AggregateTag:      info.AggregateTag,
		RuntimeArrayBound: cloneTokens(bound),
		RuntimeArrayElem:  elemInfo,
	}, true, nil
}

func parseDeclItemsWithOptions(toks []Token, enumLookup map[string]int64, allowRuntimeArrays bool, allowIncompleteArray bool) ([]cDeclItem, map[string]int64, bool, bool, error) {
	toks = trimTokens(toks)
	if len(toks) == 0 {
		return nil, nil, false, false, nil
	}

	// Enum declaration path (optionally with storage/qualifiers before `enum`).
	prefix := 0
	hasExtern := false
	hasTypedef := false
	for prefix < len(toks) {
		t := toks[prefix]
		if t.Kind != TokIdent {
			break
		}
		if t.Text == "enum" {
			break
		}
		if isStorageClassKeyword(t.Text) {
			if t.Text == "extern" {
				hasExtern = true
			}
			if t.Text == "typedef" {
				hasTypedef = true
			}
			prefix++
			continue
		}
		if isTypeQualifierKeyword(t.Text) {
			prefix++
			continue
		}
		break
	}
	if prefix < len(toks) && toks[prefix].Kind == TokIdent && toks[prefix].Text == "enum" {
		consumed, enumConsts, err := parseEnumSpecifierAndConstants(toks[prefix:], enumLookup)
		if err != nil {
			return nil, nil, false, false, err
		}
		rest := trimTokens(toks[prefix+consumed:])
		if len(rest) == 0 {
			return nil, enumConsts, hasExtern, hasTypedef, nil
		}
		mergedEnumLookup := make(map[string]int64, len(enumLookup)+len(enumConsts))
		for k, v := range enumLookup {
			mergedEnumLookup[k] = v
		}
		for k, v := range enumConsts {
			mergedEnumLookup[k] = v
		}
		items, err := parseDeclItemsWithBase(cTypeInfo{Kind: cDeclScalar, Base: cScalarInt}, mergedEnumLookup, hasTypedef, hasTypedef || hasExtern, hasTypedef || hasExtern || allowIncompleteArray, rest, allowRuntimeArrays)
		if err != nil {
			return nil, nil, false, false, err
		}
		if len(items) == 0 {
			return nil, enumConsts, hasExtern, hasTypedef, nil
		}
		return items, enumConsts, hasExtern, hasTypedef, nil
	}

	spec, rest, err := splitDeclSpecPrefix(toks, "declaration")
	if err != nil {
		return nil, nil, false, false, err
	}
	baseInfo, baseEnumConsts, hasExtern, hasTypedef, err := parseScalarTypeSpec(spec, "declaration", true)
	if err != nil {
		return nil, nil, false, false, err
	}
	if len(rest) == 0 {
		if baseInfo.AggregateKeyword != "" && !hasTypedef {
			return nil, baseEnumConsts, hasExtern, hasTypedef, nil
		}
		return nil, nil, false, false, fmt.Errorf("missing declarator in declaration")
	}
	mergedEnumLookup := enumLookup
	if len(baseEnumConsts) > 0 {
		mergedEnumLookup = make(map[string]int64, len(enumLookup)+len(baseEnumConsts))
		for k, v := range enumLookup {
			mergedEnumLookup[k] = v
		}
		for k, v := range baseEnumConsts {
			mergedEnumLookup[k] = v
		}
	}
	items, err := parseDeclItemsWithBase(baseInfo, mergedEnumLookup, hasTypedef, hasTypedef || hasExtern, hasTypedef || hasExtern || allowIncompleteArray, rest, allowRuntimeArrays)
	if err != nil {
		return nil, nil, false, false, err
	}
	if len(items) == 0 {
		return nil, nil, false, false, fmt.Errorf("empty declaration")
	}
	return items, baseEnumConsts, hasExtern, hasTypedef, nil
}

func parseDeclItems(toks []Token, enumLookup map[string]int64, allowRuntimeArrays bool) ([]cDeclItem, map[string]int64, bool, bool, error) {
	return parseDeclItemsWithOptions(toks, enumLookup, allowRuntimeArrays, false)
}

func isTypeSpecifierKeyword(text string) bool {
	switch text {
	case "void", "char", "short", "int", "long", "signed", "unsigned", "__signed", "float", "double":
		return true
	default:
		return false
	}
}

func parseCTypeInfo(tokens []Token) (cTypeInfo, error) {
	tokens = trimTokens(tokens)
	if len(tokens) == 0 {
		return cTypeInfo{}, fmt.Errorf("empty type name")
	}
	spec, decl, err := splitDeclSpecPrefix(tokens, "type name")
	if err != nil {
		return cTypeInfo{}, err
	}
	baseInfo, _, _, _, err := parseScalarTypeSpec(spec, "type name", true)
	if err != nil {
		return cTypeInfo{}, err
	}
	name, kind, ptrDepth, arrayLen, arrayDims, fnSig, directFunc, err := parseDeclarator(decl, true, nil)
	if err != nil {
		return cTypeInfo{}, err
	}
	if directFunc {
		kind = cDeclPointer
		ptrDepth = 1
	}
	if name != "" {
		return cTypeInfo{}, fmt.Errorf("named declarators are not supported in type names (%s)", name)
	}
	info, err := combineTypeAndDeclarator(baseInfo, kind, ptrDepth, arrayLen, true, "type name")
	if err != nil {
		return cTypeInfo{}, err
	}
	applyDeclaratorArrayDims(&info, arrayDims)
	if fnSig != nil {
		sig := cloneFuncTypeSig(fnSig)
		applyFuncReturnBase(sig, baseInfo)
		info.FuncSig = sig
	}
	if info.IsVoid && info.Kind == cDeclArray {
		return cTypeInfo{}, fmt.Errorf("array of void is not supported")
	}
	return info, nil
}

func parseArrayInitializerExprs(init []Token, arrayLen int64) ([][]Token, error) {
	out, err := parseBraceInitializerExprs(init)
	if err != nil {
		return nil, err
	}
	if int64(len(out)) > arrayLen {
		return nil, fmt.Errorf("too many initializer elements (%d > %d)", len(out), arrayLen)
	}
	return out, nil
}

func parseBraceInitializerParts(init []Token) ([][]Token, error) {
	init = trimTokens(init)
	if len(init) == 0 {
		return nil, nil
	}
	if len(init) < 2 || init[0].Kind != TokPunct || init[0].Text != "{" || init[len(init)-1].Kind != TokPunct || init[len(init)-1].Text != "}" {
		return nil, fmt.Errorf("expected brace initializer list")
	}
	inner := trimTokens(init[1 : len(init)-1])
	if len(inner) == 0 {
		return nil, nil
	}
	parts := splitTopLevel(inner, ",")
	out := make([][]Token, 0, len(parts))
	for _, p := range parts {
		p = trimTokens(p)
		if len(p) == 0 {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

func decodeStringLiteralSequence(tokens []Token) (string, error) {
	tokens = trimTokens(tokens)
	if len(tokens) == 0 {
		return "", nil
	}
	var b strings.Builder
	for _, t := range tokens {
		if t.Kind != TokString {
			return "", fmt.Errorf("expected string literal sequence")
		}
		part, err := decodeStringToken(t)
		if err != nil {
			return "", err
		}
		b.WriteString(part)
	}
	return b.String(), nil
}

func inferArrayLengthFromInit(base cScalarType, init []Token, enumLookup map[string]int64) (int64, bool, error) {
	init = trimTokens(init)
	if len(init) == 0 {
		return 0, false, nil
	}
	if isStringLiteralExpr(init) {
		if base != cScalarChar && base != cScalarUChar {
			return 0, false, fmt.Errorf("string literal initializer requires char array element type")
		}
		s, err := decodeStringLiteralSequence(init)
		if err != nil {
			return 0, false, err
		}
		return int64(len(s) + 1), true, nil
	}
	if parts, err := parseArrayInitializerElems(init, enumLookup); err == nil {
		nextIndex := int64(0)
		maxLen := int64(0)
		hasDesignator := false
		for _, part := range parts {
			if len(part.Indexes) > 0 {
				hasDesignator = true
				nextIndex = part.Indexes[0] + 1
			} else {
				nextIndex++
			}
			if nextIndex > maxLen {
				maxLen = nextIndex
			}
		}
		if hasDesignator && maxLen > 0 {
			return maxLen, true, nil
		}
	}
	elems, err := parseBraceInitializerExprs(init)
	if err == nil {
		return int64(len(elems)), true, nil
	}
	if n, ok := countTopLevelInitializerElems(init); ok {
		return n, true, nil
	}
	return 0, false, fmt.Errorf("array bounds may only be omitted with brace or string initializers")
}

func countTopLevelInitializerElems(init []Token) (int64, bool) {
	init = trimTokens(init)
	if len(init) < 2 || init[0].Kind != TokPunct || init[0].Text != "{" || init[len(init)-1].Kind != TokPunct || init[len(init)-1].Text != "}" {
		return 0, false
	}
	parts := splitTopLevel(trimTokens(init[1:len(init)-1]), ",")
	var count int64
	for _, p := range parts {
		if len(trimTokens(p)) == 0 {
			continue
		}
		count++
	}
	return count, true
}

func stringArrayInitializerValues(init []Token, base cScalarType, arrayLen int64) ([]int64, bool, error) {
	if !isStringLiteralExpr(init) {
		return nil, false, nil
	}
	if base != cScalarChar && base != cScalarUChar {
		return nil, false, fmt.Errorf("string literal initializer requires char array element type")
	}
	s, err := decodeStringLiteralSequence(init)
	if err != nil {
		return nil, true, err
	}
	vals := make([]int64, 0, len(s)+1)
	for i := 0; i < len(s); i++ {
		vals = append(vals, int64(uint8(s[i])))
	}
	if int64(len(vals)) > arrayLen {
		return nil, true, fmt.Errorf("string initializer is too long (%d > %d)", len(vals), arrayLen)
	}
	if int64(len(vals)) < arrayLen {
		vals = append(vals, 0)
	}
	return vals, true, nil
}

func parseBraceInitializerExprs(init []Token) ([][]Token, error) {
	parts, err := parseBraceInitializerParts(init)
	if err != nil {
		return nil, err
	}
	out := make([][]Token, 0, len(parts))
	for _, p := range parts {
		p = trimTokens(p)
		if p[0].Kind == TokPunct && p[0].Text == "{" {
			return nil, fmt.Errorf("nested initializer lists are not yet supported")
		}
		out = append(out, p)
	}
	return out, nil
}

func parseBraceInitializerItems(init []Token) ([][]Token, error) {
	init = trimTokens(init)
	if len(init) == 0 {
		return nil, nil
	}
	if len(init) < 2 || init[0].Kind != TokPunct || init[0].Text != "{" || init[len(init)-1].Kind != TokPunct || init[len(init)-1].Text != "}" {
		return nil, fmt.Errorf("expected brace initializer list")
	}
	inner := trimTokens(init[1 : len(init)-1])
	if len(inner) == 0 {
		return nil, nil
	}
	parts := splitTopLevel(inner, ",")
	out := make([][]Token, 0, len(parts))
	for _, p := range parts {
		p = trimTokens(p)
		if len(p) == 0 {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

func parseFieldDesignator(init []Token) (string, []Token, bool) {
	init = trimTokens(init)
	if len(init) < 4 {
		return "", init, false
	}
	if init[0].Kind != TokPunct || init[0].Text != "." {
		return "", init, false
	}
	if init[1].Kind != TokIdent {
		return "", init, false
	}
	if init[2].Kind != TokPunct || init[2].Text != "=" {
		return "", init, false
	}
	return init[1].Text, trimTokens(init[3:]), true
}

func isBraceInitializer(init []Token) bool {
	init = trimTokens(init)
	return len(init) >= 2 &&
		init[0].Kind == TokPunct &&
		init[0].Text == "{" &&
		init[len(init)-1].Kind == TokPunct &&
		init[len(init)-1].Text == "}"
}

func isZeroInitializerExpr(tokens []Token) bool {
	tokens = trimTokens(tokens)
	if len(tokens) == 0 {
		return false
	}
	if len(tokens) == 1 {
		switch tokens[0].Kind {
		case TokNumber:
			v, err := parseCIntLiteral(tokens[0].Text)
			return err == nil && v == 0
		case TokChar:
			v, err := parseCCharLiteral(tokens[0].Text)
			return err == nil && v == 0
		}
	}
	return false
}

func matchBraceClose(tokens []Token, open int) int {
	if open < 0 || open >= len(tokens) {
		return -1
	}
	if tokens[open].Kind != TokPunct || tokens[open].Text != "{" {
		return -1
	}
	depth := 1
	for i := open + 1; i < len(tokens); i++ {
		if tokens[i].Kind != TokPunct {
			continue
		}
		if tokens[i].Text == "{" {
			depth++
		} else if tokens[i].Text == "}" {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func isStringLiteralExpr(tokens []Token) bool {
	tokens = trimTokens(tokens)
	if len(tokens) == 0 {
		return false
	}
	for _, t := range tokens {
		if t.Kind != TokString {
			return false
		}
	}
	return true
}

type funcCompiler struct {
	c   *compiler
	sig *cFuncSig
	fn  *ir.IRFunc

	scopes        []map[string]cLocalBinding
	typedefScopes []map[string]cTypeInfo
	enumScopes    []map[string]int64
	aggregateTags []map[string]*cAggregateInfo

	breakTargets    []int
	continueTargets []int

	userLabels  map[string]cUserLabel
	retPtrLocal int

	variadicCount int
	variadicData  int
}

func (fc *funcCompiler) errorf(file string, line int, col int, format string, args ...interface{}) {
	fc.c.errorf(file, line, col, format, args...)
}

func (fc *funcCompiler) emit(inst ir.Inst) {
	fc.fn.Code = append(fc.fn.Code, inst)
}

func (fc *funcCompiler) pushScope() {
	fc.scopes = append(fc.scopes, make(map[string]cLocalBinding))
	fc.typedefScopes = append(fc.typedefScopes, make(map[string]cTypeInfo))
	fc.enumScopes = append(fc.enumScopes, make(map[string]int64))
	fc.aggregateTags = append(fc.aggregateTags, make(map[string]*cAggregateInfo))
}

func (fc *funcCompiler) popScope() {
	if len(fc.scopes) > 0 {
		fc.scopes = fc.scopes[:len(fc.scopes)-1]
	}
	if len(fc.typedefScopes) > 0 {
		fc.typedefScopes = fc.typedefScopes[:len(fc.typedefScopes)-1]
	}
	if len(fc.enumScopes) > 0 {
		fc.enumScopes = fc.enumScopes[:len(fc.enumScopes)-1]
	}
	if len(fc.aggregateTags) > 0 {
		fc.aggregateTags = fc.aggregateTags[:len(fc.aggregateTags)-1]
	}
}

func (fc *funcCompiler) addLocalTypedef(name string, info cTypeInfo, file string, line int, col int) {
	if len(fc.typedefScopes) == 0 {
		fc.typedefScopes = append(fc.typedefScopes, make(map[string]cTypeInfo))
	}
	cur := fc.typedefScopes[len(fc.typedefScopes)-1]
	if existing, exists := cur[name]; exists {
		if cTypeInfoEquivalent(existing, info) {
			return
		}
		fc.errorf(file, line, col, "duplicate typedef name %q in same scope", name)
		return
	}
	info.FuncSig = cloneFuncTypeSig(info.FuncSig)
	cur[name] = info
}

func (fc *funcCompiler) addLocalEnumConst(name string, val int64, file string, line int, col int) {
	if len(fc.enumScopes) == 0 {
		fc.enumScopes = append(fc.enumScopes, make(map[string]int64))
	}
	curEnums := fc.enumScopes[len(fc.enumScopes)-1]
	if _, exists := curEnums[name]; exists {
		fc.errorf(file, line, col, "duplicate enum constant %q in same scope", name)
		return
	}
	if len(fc.scopes) > 0 {
		if _, exists := fc.scopes[len(fc.scopes)-1][name]; exists {
			fc.errorf(file, line, col, "enum constant %q conflicts with local declaration in same scope", name)
			return
		}
	}
	curEnums[name] = val
}

func (fc *funcCompiler) addLocalEnumConsts(vals map[string]int64, file string, line int, col int) {
	for name, val := range vals {
		fc.addLocalEnumConst(name, val, file, line, col)
	}
}

func (fc *funcCompiler) lookupEnumConst(name string) (int64, bool) {
	for i := len(fc.enumScopes) - 1; i >= 0; i-- {
		if v, ok := fc.enumScopes[i][name]; ok {
			return v, true
		}
	}
	return fc.c.lookupEnumConst(name)
}

func (fc *funcCompiler) lookupConstObjectSize(name string) (int64, bool) {
	if b, ok := fc.lookupLocalBinding(name); ok {
		info := cTypeInfo{
			Kind:             b.Kind,
			PtrDepth:         b.PtrDepth,
			ArrayLen:         b.ArrayLen,
			ArrayDims:        cloneInt64s(b.ArrayDims),
			Base:             b.Base,
			FuncSig:          cloneFuncTypeSig(b.FuncSig),
			OpaqueAggregate:  b.OpaqueAggregate,
			AggregateKeyword: b.AggregateKeyword,
			AggregateTag:     b.AggregateTag,
		}
		size, _, err := cTypeLayout(info)
		if err == nil {
			return size, true
		}
	}
	return fc.c.lookupConstObjectSize(name)
}

func (fc *funcCompiler) enumLookupMap() map[string]int64 {
	out := copyEnumLookupMap(fc.c.enumConsts)
	for i := 0; i < len(fc.enumScopes); i++ {
		for name, val := range fc.enumScopes[i] {
			out[name] = val
		}
	}
	return out
}

func (fc *funcCompiler) aggregateLookupMap() map[string]*cAggregateInfo {
	out := make(map[string]*cAggregateInfo)
	for key, agg := range fc.c.aggregateTags {
		out[key] = cloneAggregateInfo(agg)
	}
	i := 0
	for i < len(fc.aggregateTags) {
		for key, agg := range fc.aggregateTags[i] {
			out[key] = cloneAggregateInfo(agg)
		}
		i++
	}
	return out
}

func (fc *funcCompiler) lookupAggregate(keyword string, tag string) (*cAggregateInfo, bool) {
	if keyword == "" || tag == "" {
		return nil, false
	}
	key := aggregateTypeKey(keyword, tag)
	i := len(fc.aggregateTags) - 1
	for i >= 0 {
		if agg, ok := fc.aggregateTags[i][key]; ok {
			return cloneAggregateInfo(agg), true
		}
		i--
	}
	return fc.c.lookupAggregate(keyword, tag)
}

func (fc *funcCompiler) registerAggregate(info *cAggregateInfo) error {
	if info == nil || info.Keyword == "" || info.Tag == "" {
		return nil
	}
	if len(fc.aggregateTags) == 0 {
		fc.aggregateTags = append(fc.aggregateTags, make(map[string]*cAggregateInfo))
	}
	key := aggregateTypeKey(info.Keyword, info.Tag)
	cur := fc.aggregateTags[len(fc.aggregateTags)-1]
	newInfo := cloneAggregateInfo(info)
	if prev, ok := cur[key]; ok {
		prevHasFields := len(prev.Fields) > 0
		newHasFields := len(newInfo.Fields) > 0
		if prevHasFields && newHasFields {
			if !aggregateInfosCompatible(prev, newInfo) {
				return fmt.Errorf("conflicting %s definition for %q", info.Keyword, info.Tag)
			}
			return nil
		}
		if prevHasFields || !newHasFields {
			return nil
		}
	}
	cur[key] = newInfo
	return nil
}

func (fc *funcCompiler) typeInfoSizeAlign(info cTypeInfo) (int64, int64, bool) {
	switch info.Kind {
	case cDeclPointer:
		ps := int64(fc.c.target.PtrSize)
		return ps, ps, true
	case cDeclArray:
		elem := arrayElementTypeInfo(info)
		elemSize, elemAlign, ok := fc.typeInfoSizeAlign(elem)
		if !ok {
			return 0, 0, false
		}
		return elemSize * info.ArrayLen, elemAlign, true
	case cDeclScalar:
		if info.IsVoid {
			return 1, 1, true
		}
		if info.AggregateKeyword != "" && info.AggregateTag != "" {
			if info.OpaqueAggregate {
				return 0, 0, false
			}
			if agg, ok := fc.lookupAggregate(info.AggregateKeyword, info.AggregateTag); ok && len(agg.Fields) > 0 {
				align := agg.Align
				if align <= 0 {
					align = 1
				}
				size := agg.Size
				if size <= 0 {
					size = 1
				}
				return size, align, true
			}
			return 0, 0, false
		}
		sz := fc.scalarSize(info.Base)
		return sz, sz, true
	}
	return 0, 0, false
}

func (fc *funcCompiler) pointerElemStep(kind cDeclKind, ptrDepth int, base cScalarType, isVoid bool, opaqueAggregate bool, aggregateKeyword string, aggregateTag string) int64 {
	if kind != cDeclPointer {
		return int64(fc.c.target.PtrSize)
	}
	if ptrDepth > 1 {
		return int64(fc.c.target.PtrSize)
	}
	if isVoid {
		return 1
	}
	if aggregateKeyword != "" && aggregateTag != "" {
		if !opaqueAggregate {
			if agg, ok := fc.lookupAggregate(aggregateKeyword, aggregateTag); ok && len(agg.Fields) > 0 {
				return agg.Size
			}
		}
		return int64(fc.c.target.PtrSize)
	}
	return fc.scalarSize(base)
}

func (fc *funcCompiler) resolveMemberField(ex *expr, diag bool) (cAggregateField, bool) {
	if ex == nil || ex.kind != exprMember {
		return cAggregateField{}, false
	}
	baseType, ok := fc.exprTypeInfo(ex.left)
	if !ok {
		if diag {
			fc.errorf(fc.sig.File, 0, 0, "member access requires aggregate expression")
		}
		return cAggregateField{}, false
	}
	switch ex.op {
	case ".":
		if baseType.Kind != cDeclScalar || baseType.AggregateKeyword == "" || baseType.AggregateTag == "" {
			if diag {
				fc.errorf(fc.sig.File, 0, 0, "member access via '.' requires struct/union value")
			}
			return cAggregateField{}, false
		}
	case "->":
		if baseType.Kind != cDeclPointer || baseType.PtrDepth != 1 || baseType.AggregateKeyword == "" || baseType.AggregateTag == "" {
			if diag {
				fc.errorf(fc.sig.File, 0, 0, "member access via '->' requires pointer to struct/union")
			}
			return cAggregateField{}, false
		}
	default:
		if diag {
			fc.errorf(fc.sig.File, 0, 0, "unsupported member access operator %q", ex.op)
		}
		return cAggregateField{}, false
	}
	agg, ok := fc.lookupAggregate(baseType.AggregateKeyword, baseType.AggregateTag)
	if !ok || len(agg.Fields) == 0 {
		if diag {
			if ex.op == "." {
				fc.errorf(fc.sig.File, 0, 0, "member access via '.' on incomplete %s %q", baseType.AggregateKeyword, baseType.AggregateTag)
			} else {
				fc.errorf(fc.sig.File, 0, 0, "member access via '->' on incomplete %s %q", baseType.AggregateKeyword, baseType.AggregateTag)
			}
		}
		return cAggregateField{}, false
	}
	for _, f := range agg.Fields {
		if f.Name == ex.member {
			return f, true
		}
	}
	if diag {
		fc.errorf(fc.sig.File, 0, 0, "%s %q has no member %q", agg.Keyword, agg.Tag, ex.member)
	}
	return cAggregateField{}, false
}

func (fc *funcCompiler) emitMemberAddress(ex *expr, diag bool) bool {
	field, ok := fc.resolveMemberField(ex, diag)
	if !ok {
		return false
	}
	if ex.op == "->" {
		fc.emitExpr(ex.left)
	} else {
		if !fc.emitAddressOf(ex.left) {
			if diag {
				fc.errorf(fc.sig.File, 0, 0, "member access via '.' requires addressable base expression")
			}
			return false
		}
	}
	if field.Offset != 0 {
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: field.Offset})
		fc.emit(ir.Inst{Op: ir.OP_ADD})
	}
	return true
}

func (fc *funcCompiler) lookupTypedef(name string) (cTypeInfo, bool) {
	for i := len(fc.typedefScopes) - 1; i >= 0; i-- {
		if info, ok := fc.typedefScopes[i][name]; ok {
			return info, true
		}
	}
	return fc.c.lookupTypedef(name)
}

func (fc *funcCompiler) addLocal(name string, file string, line int, col int) int {
	return fc.addLocalDecl(name, cDeclScalar, cScalarInt, 0, int64(fc.c.target.PtrSize), 0, nil, nil, false, "", "", file, line, col)
}

func (fc *funcCompiler) addLocalKind(name string, kind cDeclKind, file string, line int, col int) int {
	ptrDepth := 0
	if kind == cDeclPointer {
		ptrDepth = 1
	}
	return fc.addLocalDecl(name, kind, cScalarInt, ptrDepth, int64(fc.c.target.PtrSize), 0, nil, nil, false, "", "", file, line, col)
}

func (fc *funcCompiler) addLocalTyped(name string, kind cDeclKind, base cScalarType, ptrDepth int, elemStep int64, funcSig *cFuncTypeSig, file string, line int, col int) int {
	return fc.addLocalDecl(name, kind, base, ptrDepth, elemStep, 0, nil, funcSig, false, "", "", file, line, col)
}

func irFloatKindForCScalar(base cScalarType) ir.TypeKind {
	switch base {
	case cScalarFloat:
		return ir.TY_FLOAT32
	case cScalarDouble:
		return ir.TY_FLOAT64
	default:
		return ir.TY_VOID
	}
}

func irResultKindForCType(info cTypeInfo) ir.TypeKind {
	if isFloatTypeInfo(info) {
		return irFloatKindForCScalar(info.Base)
	}
	return ir.TY_VOID
}

func cLongSizeForTarget(target *common.Target) int64 {
	if target.GOOS == "windows" {
		return 4
	}
	return int64(target.PtrSize)
}

func scalarSizeForTarget(target *common.Target, base cScalarType) int64 {
	switch base {
	case cScalarChar, cScalarUChar:
		return 1
	case cScalarShort, cScalarUShort:
		return 2
	case cScalarInt, cScalarUInt:
		return 4
	case cScalarLong, cScalarULong:
		return cLongSizeForTarget(target)
	case cScalarFloat:
		return 4
	case cScalarDouble:
		return 8
	default:
		return 4
	}
}

func irResultIs64ForCType(target *common.Target, info cTypeInfo) bool {
	if isFloatTypeInfo(info) {
		return info.Base == cScalarDouble
	}
	if info.Kind == cDeclPointer || info.Kind == cDeclArray {
		return target.PtrSize == 8
	}
	return scalarSizeForTarget(target, info.Base) == 8
}

func (fc *funcCompiler) addLocalDecl(name string, kind cDeclKind, base cScalarType, ptrDepth int, elemStep int64, arrayLen int64, arrayDims []int64, funcSig *cFuncTypeSig, opaqueAggregate bool, aggregateKeyword string, aggregateTag string, file string, line int, col int) int {
	if len(fc.scopes) == 0 {
		fc.pushScope()
	}
	cur := fc.scopes[len(fc.scopes)-1]
	if _, exists := cur[name]; exists {
		fc.errorf(file, line, col, "redefinition of local %q", name)
	}
	if len(fc.enumScopes) > 0 {
		if _, exists := fc.enumScopes[len(fc.enumScopes)-1][name]; exists {
			fc.errorf(file, line, col, "local declaration %q conflicts with enum constant in same scope", name)
		}
	}
	idx := len(fc.fn.Locals)
	local := ir.IRLocal{Name: name, Index: idx}
	if kind == cDeclPointer || kind == cDeclArray {
		local.Width = fc.c.target.PtrSize
		local.Is64 = fc.c.target.PtrSize == 8
	} else if isFloatScalar(base) {
		local.Width = int(fc.scalarSize(base))
		local.FloatKind = irFloatKindForCScalar(base)
		local.IsFloat64 = base == cScalarDouble
	} else {
		local.Width = int(fc.scalarSize(base))
		local.Is64 = local.Width == 8
	}
	fc.fn.Locals = append(fc.fn.Locals, local)
	if kind == cDeclPointer && ptrDepth == 0 {
		ptrDepth = 1
	}
	if elemStep <= 0 {
		elemStep = int64(fc.c.target.PtrSize)
	}
	cur[name] = cLocalBinding{
		Index:            idx,
		Kind:             kind,
		PtrDepth:         ptrDepth,
		ElemStep:         elemStep,
		ArrayLen:         arrayLen,
		ArrayDims:        cloneInt64s(arrayDims),
		Base:             base,
		FuncSig:          cloneFuncTypeSig(funcSig),
		OpaqueAggregate:  opaqueAggregate,
		AggregateKeyword: aggregateKeyword,
		AggregateTag:     aggregateTag,
	}
	return idx
}

func (fc *funcCompiler) lookupLocal(name string) (int, bool) {
	b, ok := fc.lookupLocalBinding(name)
	if !ok {
		return 0, false
	}
	return b.Index, true
}

func (fc *funcCompiler) lookupLocalKind(name string) (cDeclKind, bool) {
	b, ok := fc.lookupLocalBinding(name)
	if !ok {
		return cDeclScalar, false
	}
	return b.Kind, true
}

func (fc *funcCompiler) lookupLocalArrayLen(name string) (int64, bool) {
	b, ok := fc.lookupLocalBinding(name)
	if !ok {
		return 0, false
	}
	return b.ArrayLen, b.Kind == cDeclArray
}

func (fc *funcCompiler) lookupLocalBase(name string) (cScalarType, bool) {
	b, ok := fc.lookupLocalBinding(name)
	if !ok {
		return cScalarInt, false
	}
	return b.Base, true
}

func (fc *funcCompiler) lookupLocalPtrDepth(name string) (int, bool) {
	b, ok := fc.lookupLocalBinding(name)
	if !ok {
		return 0, false
	}
	return b.PtrDepth, true
}

func (fc *funcCompiler) lookupLocalElemStep(name string) (int64, bool) {
	b, ok := fc.lookupLocalBinding(name)
	if !ok {
		return 0, false
	}
	return b.ElemStep, true
}

func (fc *funcCompiler) lookupLocalBinding(name string) (cLocalBinding, bool) {
	for i := len(fc.scopes) - 1; i >= 0; i-- {
		if b, ok := fc.scopes[i][name]; ok {
			return b, true
		}
	}
	return cLocalBinding{}, false
}

func (fc *funcCompiler) lookupGlobal(name string) (int, bool) {
	idx, ok := fc.c.globalIndex[name]
	return idx, ok
}

func (fc *funcCompiler) lookupGlobalKind(name string) (cDeclKind, bool) {
	kind, ok := fc.c.globalKind[name]
	if !ok {
		return cDeclScalar, false
	}
	return kind, true
}

func (fc *funcCompiler) lookupGlobalArrayLen(name string) (int64, bool) {
	n, ok := fc.c.globalArray[name]
	return n, ok
}

func (fc *funcCompiler) lookupGlobalArrayDims(name string) ([]int64, bool) {
	dims, ok := fc.c.globalArrayDims[name]
	if !ok {
		return nil, false
	}
	return cloneInt64s(dims), true
}

func (fc *funcCompiler) lookupGlobalBase(name string) (cScalarType, bool) {
	base, ok := fc.c.globalBase[name]
	if !ok {
		return cScalarInt, false
	}
	return base, true
}

func (fc *funcCompiler) lookupGlobalPtrDepth(name string) (int, bool) {
	depth, ok := fc.c.globalPtrDepth[name]
	if !ok {
		return 0, false
	}
	return depth, true
}

func (fc *funcCompiler) lookupGlobalElemStep(name string) (int64, bool) {
	step, ok := fc.c.globalElemStep[name]
	if !ok {
		return 0, false
	}
	return step, true
}

func (fc *funcCompiler) lookupGlobalFuncSig(name string) (*cFuncTypeSig, bool) {
	sig, ok := fc.c.globalFunc[name]
	if !ok {
		return nil, false
	}
	return cloneFuncTypeSig(sig), true
}

func (fc *funcCompiler) lookupGlobalOpaqueAggregate(name string) (bool, string, string) {
	return fc.c.globalOpaque[name], fc.c.globalAggKey[name], fc.c.globalAggTag[name]
}

func (fc *funcCompiler) compileCompound(n *Node, pushScope bool) {
	if n == nil {
		return
	}
	if pushScope {
		fc.pushScope()
		defer fc.popScope()
	}
	for _, st := range n.Children {
		fc.compileStmt(st)
	}
}

func (fc *funcCompiler) indexUserLabels(n *Node) {
	if n == nil {
		return
	}
	stack := []*Node{n}
	for len(stack) > 0 {
		last := len(stack) - 1
		cur := stack[last]
		stack = stack[:last]
		if cur == nil {
			continue
		}
		if cur.Kind == NLabelStmt {
			name := strings.TrimSpace(cur.Text)
			if name == "" {
				fc.errorf(fc.sig.File, cur.Line, cur.Col, "label name is missing")
			} else if prev, ok := fc.userLabels[name]; ok {
				fc.errorf(fc.sig.File, cur.Line, cur.Col, "duplicate label %q (previous at %d:%d)", name, prev.Line, prev.Col)
			} else {
				fc.userLabels[name] = cUserLabel{
					Target: fc.c.nextLabel(),
					Line:   cur.Line,
					Col:    cur.Col,
				}
			}
		}
		for i := len(cur.Children) - 1; i >= 0; i-- {
			stack = append(stack, cur.Children[i])
		}
	}
}

func (fc *funcCompiler) compileStmt(n *Node) {
	if n == nil {
		return
	}
	switch n.Kind {
	case NCompoundStmt:
		fc.compileCompound(n, true)
	case NDeclStmt:
		fc.compileDeclStmt(n)
	case NExprStmt:
		if isGNUAsmStatementText(n.Text) {
			return
		}
		fc.compileExprText(n.Text, n.Line, n.Col)
		fc.emit(ir.Inst{Op: ir.OP_DROP})
	case NEmptyStmt:
		return
	case NIfStmt:
		fc.compileIfStmt(n)
	case NWhileStmt:
		fc.compileWhileStmt(n)
	case NDoWhileStmt:
		fc.compileDoWhileStmt(n)
	case NForStmt:
		fc.compileForStmt(n)
	case NSwitchStmt:
		fc.compileSwitchStmt(n)
	case NReturnStmt:
		fc.compileReturnStmt(n)
	case NBreakStmt:
		if len(fc.breakTargets) == 0 {
			fc.errorf(fc.sig.File, n.Line, n.Col, "break used outside loop or switch")
			return
		}
		fc.emit(ir.Inst{Op: ir.OP_JMP, Arg: fc.breakTargets[len(fc.breakTargets)-1]})
	case NContinueStmt:
		if len(fc.continueTargets) == 0 {
			fc.errorf(fc.sig.File, n.Line, n.Col, "continue used outside loop")
			return
		}
		fc.emit(ir.Inst{Op: ir.OP_JMP, Arg: fc.continueTargets[len(fc.continueTargets)-1]})
	case NCaseStmt, NDefaultStmt:
		fc.errorf(fc.sig.File, n.Line, n.Col, "case/default label used outside switch")
	case NGotoStmt:
		name := strings.TrimSpace(n.Text)
		if name == "" {
			fc.errorf(fc.sig.File, n.Line, n.Col, "goto requires a label")
			return
		}
		lab, ok := fc.userLabels[name]
		if !ok {
			fc.errorf(fc.sig.File, n.Line, n.Col, "goto to undefined label %q", name)
			return
		}
		fc.emit(ir.Inst{Op: ir.OP_JMP, Arg: lab.Target})
	case NLabelStmt:
		name := strings.TrimSpace(n.Text)
		if name == "" {
			fc.errorf(fc.sig.File, n.Line, n.Col, "label name is missing")
		} else if lab, ok := fc.userLabels[name]; ok {
			// Duplicate labels are diagnosed in indexUserLabels. Emit the label
			// only for the first declaration to avoid conflicting IR labels.
			if lab.Line == n.Line && lab.Col == n.Col {
				fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: lab.Target})
			}
		} else {
			fc.errorf(fc.sig.File, n.Line, n.Col, "unknown label %q", name)
		}
		for _, child := range n.Children {
			fc.compileStmt(child)
		}
	default:
		fc.errorf(fc.sig.File, n.Line, n.Col, "unsupported statement kind: %s", n.Kind.String())
	}
}

func flattenSwitchLabelChain(n *Node) ([]*Node, *Node) {
	if n == nil {
		return nil, nil
	}
	if n.Kind != NCaseStmt && n.Kind != NDefaultStmt {
		return nil, n
	}
	var labels []*Node
	cur := n
	for cur != nil && (cur.Kind == NCaseStmt || cur.Kind == NDefaultStmt) {
		labels = append(labels, cur)
		if len(cur.Children) == 0 {
			return labels, nil
		}
		next := cur.Children[0]
		if next.Kind == NCaseStmt || next.Kind == NDefaultStmt {
			cur = next
			continue
		}
		return labels, next
	}
	return labels, nil
}

func (fc *funcCompiler) compileSwitchStmt(n *Node) {
	if len(n.Children) == 0 || n.Children[0] == nil {
		fc.errorf(fc.sig.File, n.Line, n.Col, "switch is missing body")
		return
	}
	body := n.Children[0]
	bodyChildren := body.Children
	if body.Kind != NCompoundStmt {
		bodyChildren = []*Node{body}
	}

	tmpName := fmt.Sprintf("$switch%d", fc.c.nextLabel())
	switchVal := fc.addLocal(tmpName, fc.sig.File, n.Line, n.Col)
	fc.compileExprText(n.Text, n.Line, n.Col)
	fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: switchVal})

	endLabel := fc.c.nextLabel()
	defaultLabel := endLabel
	hasDefault := false
	caseLabels := make(map[*Node]int)

	for _, st := range bodyChildren {
		labels, _ := flattenSwitchLabelChain(st)
		for _, lab := range labels {
			lbl := fc.c.nextLabel()
			caseLabels[lab] = lbl
			if lab.Kind == NDefaultStmt {
				if hasDefault {
					fc.errorf(fc.sig.File, lab.Line, lab.Col, "duplicate default label in switch")
					continue
				}
				hasDefault = true
				defaultLabel = lbl
			}
		}
	}

	for _, st := range bodyChildren {
		labels, _ := flattenSwitchLabelChain(st)
		for _, lab := range labels {
			if lab.Kind != NCaseStmt {
				continue
			}
			if strings.TrimSpace(lab.Text) == "" {
				fc.errorf(fc.sig.File, lab.Line, lab.Col, "case label requires expression")
				continue
			}
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: switchVal})
			fc.compileExprText(lab.Text, lab.Line, lab.Col)
			fc.emit(ir.Inst{Op: ir.OP_EQ})
			fc.emit(ir.Inst{Op: ir.OP_JMP_IF, Arg: caseLabels[lab]})
		}
	}
	fc.emit(ir.Inst{Op: ir.OP_JMP, Arg: defaultLabel})

	fc.pushScope()
	fc.breakTargets = append(fc.breakTargets, endLabel)
	for _, st := range bodyChildren {
		labels, tail := flattenSwitchLabelChain(st)
		if len(labels) == 0 {
			fc.compileStmt(st)
			continue
		}
		for _, lab := range labels {
			fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: caseLabels[lab]})
		}
		if tail != nil {
			fc.compileStmt(tail)
		}
	}
	fc.breakTargets = fc.breakTargets[:len(fc.breakTargets)-1]
	fc.popScope()
	fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: endLabel})
}

func (fc *funcCompiler) initLocalAggregateObject(name string, idx int, info cTypeInfo, init []Token, file string, line int, col int) {
	size, _, ok := fc.typeInfoSizeAlign(info)
	if !ok {
		fc.errorf(file, line, col, "unsupported aggregate object declaration for %s", name)
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
		return
	}
	word := int64(fc.c.target.PtrSize)
	words := (size + word - 1) / word
	if words <= 0 {
		words = 1
	}
	firstElem := -1
	lastElem := -1
	for i := int64(0); i < words; i++ {
		elemName := fmt.Sprintf("$%s$obj$%d$%d", name, idx, i)
		elemIdx := fc.addLocal(elemName, file, line, col)
		if firstElem < 0 {
			firstElem = elemIdx
		}
		lastElem = elemIdx
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: elemIdx})
	}
	if firstElem < 0 {
		fc.errorf(file, line, col, "aggregate declaration requires non-zero size: %s", name)
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
		return
	}
	baseElem := lastElem
	if fc.c.target.Backend == "c" {
		baseElem = firstElem
	}
	fc.emit(ir.Inst{Op: ir.OP_LOCAL_ADDR, Arg: baseElem})
	fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
	init = trimTokens(init)
	if len(init) == 0 {
		return
	}
	if init[0].Kind == TokPunct && init[0].Text == "{" {
		fc.emitAggregateObjectInitializer(name, idx, false, info, init, file, line, col)
		return
	}
	ex := fc.parseExprTokens(file, line, col, init)
	if ex == nil {
		return
	}
	srcTmp := fc.allocTempLocal("$agg_init_src")
	if !fc.emitAggregateExprAddress(ex) {
		fc.errorf(file, line, col, "aggregate initializer for %s requires brace initializer or aggregate-valued expression", name)
		return
	}
	fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: srcTmp})
	fc.emitCopyAggregateBytes(idx, srcTmp, size)
}

type cAggregateInitElem struct {
	Field   string
	Indexes []int64
	Init    []Token
}

type cArrayInitElem struct {
	Indexes []int64
	Init    []Token
}

func findTopLevelInitAssign(tokens []Token) int {
	depthParen := 0
	depthBracket := 0
	depthBrace := 0
	for i, t := range tokens {
		if t.Kind != TokPunct {
			continue
		}
		switch t.Text {
		case "(":
			depthParen++
		case ")":
			if depthParen > 0 {
				depthParen--
			}
		case "[":
			depthBracket++
		case "]":
			if depthBracket > 0 {
				depthBracket--
			}
		case "{":
			depthBrace++
		case "}":
			if depthBrace > 0 {
				depthBrace--
			}
		case "=":
			if depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
				return i
			}
		}
	}
	return -1
}

func parseAggregateInitializerElems(init []Token, enumLookup map[string]int64) ([]cAggregateInitElem, error) {
	parts, err := parseBraceInitializerParts(init)
	if err != nil {
		return nil, err
	}
	out := make([]cAggregateInitElem, 0, len(parts))
	for _, p := range parts {
		field := ""
		var indexes []int64
		value := trimTokens(p)
		if len(value) >= 4 && value[0].Kind == TokPunct && value[0].Text == "." && value[1].Kind == TokIdent {
			field = value[1].Text
			i := 2
			for i < len(value) && value[i].Kind == TokPunct && value[i].Text == "[" {
				close := matchingBracketClose(value, i)
				if close < 0 {
					return nil, fmt.Errorf("unterminated initializer designator index")
				}
				n, err := parseArrayDesignatorIndex(value[i+1:close], enumLookup)
				if err != nil {
					return nil, err
				}
				indexes = append(indexes, n)
				i = close + 1
			}
			if i < len(value) && value[i].Kind == TokPunct && value[i].Text == "=" {
				value = trimTokens(value[i+1:])
			} else {
				field = ""
				indexes = nil
			}
		}
		out = append(out, cAggregateInitElem{Field: field, Indexes: indexes, Init: value})
	}
	return out, nil
}

func parseArrayInitializerElems(init []Token, enumLookup map[string]int64) ([]cArrayInitElem, error) {
	parts, err := parseBraceInitializerParts(init)
	if err != nil {
		return nil, err
	}
	out := make([]cArrayInitElem, 0, len(parts))
	for _, p := range parts {
		value := trimTokens(p)
		var indexes []int64
		i := 0
		for i < len(value) && value[i].Kind == TokPunct && value[i].Text == "[" {
			close := matchingBracketClose(value, i)
			if close < 0 {
				return nil, fmt.Errorf("unterminated initializer designator index")
			}
			n, err := parseArrayDesignatorIndex(value[i+1:close], enumLookup)
			if err != nil {
				return nil, err
			}
			indexes = append(indexes, n)
			i = close + 1
		}
		if len(indexes) > 0 {
			if i < len(value) && value[i].Kind == TokPunct && value[i].Text == "=" {
				value = trimTokens(value[i+1:])
			} else {
				value = trimTokens(value[i:])
				if len(value) == 0 {
					indexes = nil
				}
			}
		}
		out = append(out, cArrayInitElem{Indexes: indexes, Init: value})
	}
	return out, nil
}

func (fc *funcCompiler) emitBaseAddr(baseIdx int, offset int64) {
	fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: baseIdx})
	if offset != 0 {
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: offset})
		fc.emit(ir.Inst{Op: ir.OP_ADD})
	}
}

func (fc *funcCompiler) emitScalarLikeInitializerAt(baseIdx int, offset int64, info cTypeInfo, init []Token, file string, line int, col int) {
	width, _, ok := fc.typeInfoSizeAlign(info)
	if !ok || width <= 0 {
		fc.errorf(file, line, col, "initializer target has unsupported type")
		return
	}
	fc.emitExprTokensAsType(file, line, col, init, info)
	fc.emitBaseAddr(baseIdx, offset)
	fc.emitStoreForType(int(width), info)
}

func (fc *funcCompiler) emitArrayInitializerAt(name string, baseIdx int, offset int64, info cTypeInfo, init []Token, file string, line int, col int) {
	if len(init) == 0 {
		return
	}
	if vals, ok, err := stringArrayInitializerValues(init, info.Base, info.ArrayLen); ok {
		if err != nil {
			fc.errorf(file, line, col, "invalid array initializer for %s: %v", name, err)
			return
		}
		elemInfo := arrayElementTypeInfo(info)
		elemStep := fc.typeByteSize(elemInfo)
		for i, v := range vals {
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: v})
			fc.emitCastValueToType(cTypeInfo{Kind: cDeclScalar, Base: cScalarInt}, elemInfo)
			fc.emitBaseAddr(baseIdx, offset+int64(i)*elemStep)
			fc.emitStoreForType(int(fc.typeByteSize(elemInfo)), elemInfo)
		}
		return
	}
	elems, err := parseArrayInitializerElems(init, fc.enumLookupMap())
	if err != nil {
		fc.errorf(file, line, col, "invalid array initializer for %s: %v", name, err)
		return
	}
	elemInfo := arrayElementTypeInfo(info)
	elemStep := fc.typeByteSize(elemInfo)
	nextIndex := int64(0)
	for _, elem := range elems {
		part := trimTokens(elem.Init)
		if len(part) == 0 {
			continue
		}
		curInfo := elemInfo
		curOff := offset
		topIndex := nextIndex
		if len(elem.Indexes) > 0 {
			topIndex = elem.Indexes[0]
			curInfo = info
			valid := true
			for _, index := range elem.Indexes {
				if curInfo.Kind != cDeclArray {
					fc.errorf(file, line, col, "initializer designator for %s indexes non-array type", name)
					valid = false
					break
				}
				if index < 0 || index >= curInfo.ArrayLen {
					fc.errorf(file, line, col, "initializer designator for %s has out-of-bounds index %d", name, index)
					valid = false
					break
				}
				nextInfo := arrayElementTypeInfo(curInfo)
				curOff += index * fc.typeByteSize(nextInfo)
				curInfo = nextInfo
			}
			if !valid {
				continue
			}
			nextIndex = topIndex + 1
		} else {
			if nextIndex >= info.ArrayLen {
				fc.errorf(file, line, col, "too many initializer elements for %s (%d > %d)", name, nextIndex+1, info.ArrayLen)
				return
			}
			curOff += nextIndex * elemStep
			nextIndex++
		}
		if curInfo.Kind == cDeclArray {
			if !isBraceInitializer(part) && !isStringLiteralExpr(part) {
				if isZeroInitializerExpr(part) {
					continue
				}
				fc.errorf(file, line, col, "invalid array initializer for %s: expected brace initializer list", name)
				continue
			}
			fc.emitArrayInitializerAt(name, baseIdx, curOff, curInfo, part, file, line, col)
			continue
		}
		if isAggregateObjectType(curInfo) {
			if !isBraceInitializer(part) {
				if isZeroInitializerExpr(part) {
					continue
				}
				srcTmp := fc.allocTempLocal("$arr_agg_init_src")
				ex := fc.parseExprTokens(file, line, col, part)
				if ex == nil || !fc.emitAggregateExprAddress(ex) {
					fc.errorf(file, line, col, "invalid aggregate initializer for %s: expected brace initializer list", name)
					continue
				}
				fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: srcTmp})
				dstTmp := fc.allocTempLocal("$arr_agg_init_dst")
				fc.emitBaseAddr(baseIdx, curOff)
				fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: dstTmp})
				fc.emitCopyAggregateBytes(dstTmp, srcTmp, fc.typeByteSize(curInfo))
				continue
			}
			fc.emitAggregateObjectInitializerAt(name, baseIdx, curOff, curInfo, part, file, line, col)
			continue
		}
		fc.emitScalarLikeInitializerAt(baseIdx, curOff, curInfo, part, file, line, col)
	}
}

func (fc *funcCompiler) emitAggregateObjectInitializerAt(name string, baseIdx int, offset int64, info cTypeInfo, init []Token, file string, line int, col int) {
	if len(init) == 0 {
		return
	}
	agg, ok := fc.lookupAggregate(info.AggregateKeyword, info.AggregateTag)
	if !ok || len(agg.Fields) == 0 {
		fc.errorf(file, line, col, "aggregate object initializer for %s %q uses unknown/incomplete type", info.AggregateKeyword, name)
		return
	}
	initElems, err := parseAggregateInitializerElems(init, fc.enumLookupMap())
	if err != nil {
		fc.errorf(file, line, col, "invalid aggregate initializer for %s: %v", name, err)
		return
	}
	if agg.IsUnion && len(initElems) > 1 {
		fc.errorf(file, line, col, "union initializer for %s may only initialize one member for now", name)
		return
	}
	nextField := 0
	for _, initElem := range initElems {
		fieldIdx := nextField
		if initElem.Field != "" {
			fieldIdx = -1
			for i, field := range agg.Fields {
				if field.Name == initElem.Field {
					fieldIdx = i
					break
				}
			}
			if fieldIdx < 0 {
				fc.errorf(file, line, col, "unknown aggregate initializer designator .%s for %s", initElem.Field, name)
				continue
			}
		}
		if fieldIdx >= len(agg.Fields) {
			fc.errorf(file, line, col, "too many aggregate initializer elements for %s (%d > %d)", name, fieldIdx+1, len(agg.Fields))
			return
		}
		field := agg.Fields[fieldIdx]
		nextField = fieldIdx + 1
		fieldOff := offset + field.Offset
		if field.Type.Kind == cDeclArray {
			if len(initElem.Indexes) > 0 {
				curInfo := field.Type
				curOff := fieldOff
				ok := true
				for _, index := range initElem.Indexes {
					if curInfo.Kind != cDeclArray {
						fc.errorf(file, line, col, "initializer designator for %s member %q indexes non-array type", name, field.Name)
						ok = false
						break
					}
					if index < 0 || index >= curInfo.ArrayLen {
						fc.errorf(file, line, col, "initializer designator for %s member %q has out-of-bounds index %d", name, field.Name, index)
						ok = false
						break
					}
					elemInfo := arrayElementTypeInfo(curInfo)
					curOff += index * fc.typeByteSize(elemInfo)
					curInfo = elemInfo
				}
				if !ok {
					continue
				}
				if curInfo.Kind == cDeclArray {
					fc.emitArrayInitializerAt(name, baseIdx, curOff, curInfo, initElem.Init, file, line, col)
				} else if isAggregateObjectType(curInfo) {
					fc.emitAggregateObjectInitializerAt(name, baseIdx, curOff, curInfo, initElem.Init, file, line, col)
				} else {
					fc.emitScalarLikeInitializerAt(baseIdx, curOff, curInfo, initElem.Init, file, line, col)
				}
				continue
			}
			fc.emitArrayInitializerAt(name, baseIdx, fieldOff, field.Type, initElem.Init, file, line, col)
			continue
		}
		if len(initElem.Indexes) > 0 {
			fc.errorf(file, line, col, "initializer designator for %s member %q indexes non-array type", name, field.Name)
			continue
		}
		if isAggregateObjectType(field.Type) {
			if !isBraceInitializer(initElem.Init) {
				if isZeroInitializerExpr(initElem.Init) {
					continue
				}
				srcTmp := fc.allocTempLocal("$field_agg_init_src")
				ex := fc.parseExprTokens(file, line, col, initElem.Init)
				if ex == nil || !fc.emitAggregateExprAddress(ex) {
					fc.errorf(file, line, col, "invalid aggregate initializer for %s: expected brace initializer list", name)
					continue
				}
				fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: srcTmp})
				dstTmp := fc.allocTempLocal("$field_agg_init_dst")
				fc.emitBaseAddr(baseIdx, fieldOff)
				fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: dstTmp})
				fc.emitCopyAggregateBytes(dstTmp, srcTmp, fc.typeByteSize(field.Type))
				continue
			}
			fc.emitAggregateObjectInitializerAt(name, baseIdx, fieldOff, field.Type, initElem.Init, file, line, col)
			continue
		}
		fc.emitScalarLikeInitializerAt(baseIdx, fieldOff, field.Type, initElem.Init, file, line, col)
	}
}

func (fc *funcCompiler) emitAggregateObjectInitializer(name string, ptrIdx int, ptrIsGlobal bool, info cTypeInfo, init []Token, file string, line int, col int) {
	if len(init) == 0 {
		return
	}
	baseTmp := ptrIdx
	if ptrIsGlobal {
		baseTmp = fc.allocTempLocal("$agg_init_base")
		fc.emit(ir.Inst{Op: ir.OP_GLOBAL_GET, Arg: ptrIdx})
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: baseTmp})
	}
	fc.emitAggregateObjectInitializerAt(name, baseTmp, 0, info, init, file, line, col)
}

func (fc *funcCompiler) compileDeclStmt(n *Node) {
	toks, err := lexSnippet(fc.sig.File, n.Text)
	if err != nil {
		fc.errorf(fc.sig.File, n.Line, n.Col, "invalid declaration: %v", err)
		return
	}
	if isGNUAsmStatementText(tokenSliceText(toks)) {
		return
	}
	items, enumConsts, hasExtern, hasTypedef, err := parseDeclItems(toks, fc.enumLookupMap(), true)
	if err != nil {
		if fc.shouldFallbackDeclToExpr(toks, err) {
			fc.emitExprTokens(fc.sig.File, n.Line, n.Col, toks)
			fc.emit(ir.Inst{Op: ir.OP_DROP})
			return
		}
		fc.errorf(fc.sig.File, n.Line, n.Col, "%v", err)
		return
	}
	if len(enumConsts) > 0 {
		fc.addLocalEnumConsts(enumConsts, fc.sig.File, n.Line, n.Col)
	}
	if hasTypedef {
		if hasExtern {
			fc.errorf(fc.sig.File, n.Line, n.Col, "extern typedef declarations are not supported")
		}
		for _, it := range items {
			if len(it.Init) > 0 {
				fc.errorf(fc.sig.File, n.Line, n.Col, "typedef declaration with initializer is not supported: %s", it.Name)
				continue
			}
			fc.addLocalTypedef(it.Name, cTypeInfo{
				Kind:             it.Kind,
				PtrDepth:         it.PtrDepth,
				ArrayLen:         it.ArrayLen,
				ArrayDims:        cloneInt64s(it.ArrayDims),
				IsVoid:           it.IsVoid,
				Base:             it.Base,
				FuncSig:          cloneFuncTypeSig(it.FuncSig),
				OpaqueAggregate:  it.OpaqueAggregate,
				AggregateKeyword: it.AggregateKeyword,
				AggregateTag:     it.AggregateTag,
			}, fc.sig.File, n.Line, n.Col)
		}
		return
	}
	for _, it := range items {
		info := cTypeInfo{
			Kind:             it.Kind,
			PtrDepth:         it.PtrDepth,
			ArrayLen:         it.ArrayLen,
			ArrayDims:        cloneInt64s(it.ArrayDims),
			IsVoid:           it.IsVoid,
			Base:             it.Base,
			FuncSig:          cloneFuncTypeSig(it.FuncSig),
			OpaqueAggregate:  it.OpaqueAggregate,
			AggregateKeyword: it.AggregateKeyword,
			AggregateTag:     it.AggregateTag,
		}
		elemStep := fc.pointerElemStep(it.Kind, it.PtrDepth, it.Base, it.IsVoid, it.OpaqueAggregate, it.AggregateKeyword, it.AggregateTag)
		if len(it.RuntimeArrayBound) > 0 {
			elemStep = fc.typeByteSize(it.RuntimeArrayElem)
		}
		if it.Kind == cDeclArray {
			elemStep = fc.typeByteSize(arrayElementTypeInfo(info))
		}
		if it.Kind == cDeclPointer && it.PtrDepth == 1 && isStringLiteralExpr(it.Init) {
			elemStep = 1
		}
		idx := fc.addLocalDecl(it.Name, it.Kind, it.Base, it.PtrDepth, elemStep, it.ArrayLen, it.ArrayDims, it.FuncSig, it.OpaqueAggregate, it.AggregateKeyword, it.AggregateTag, fc.sig.File, n.Line, n.Col)
		if len(it.RuntimeArrayBound) > 0 {
			fc.emitExprTokens(fc.sig.File, n.Line, n.Col, it.RuntimeArrayBound)
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: fc.typeByteSize(it.RuntimeArrayElem)})
			fc.emit(ir.Inst{Op: ir.OP_MUL})
			alloc := fc.c.ensureIntrinsicWrapper("Alloc", 1, 1)
			fc.emit(ir.Inst{Op: ir.OP_CALL, Name: alloc, Arg: 1})
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
			continue
		}
		if isAggregateObjectDecl(it.Kind, it.PtrDepth, it.OpaqueAggregate, it.AggregateKeyword, it.AggregateTag) {
			fc.initLocalAggregateObject(it.Name, idx, cTypeInfo{
				Kind:             it.Kind,
				PtrDepth:         it.PtrDepth,
				Base:             it.Base,
				OpaqueAggregate:  it.OpaqueAggregate,
				AggregateKeyword: it.AggregateKeyword,
				AggregateTag:     it.AggregateTag,
			}, it.Init, fc.sig.File, n.Line, n.Col)
			continue
		}
		if it.Kind == cDeclArray {
			arrInfo := cTypeInfo{
				Kind:             it.Kind,
				PtrDepth:         it.PtrDepth,
				ArrayLen:         it.ArrayLen,
				ArrayDims:        cloneInt64s(it.ArrayDims),
				Base:             it.Base,
				IsVoid:           it.IsVoid,
				FuncSig:          cloneFuncTypeSig(it.FuncSig),
				OpaqueAggregate:  it.OpaqueAggregate,
				AggregateKeyword: it.AggregateKeyword,
				AggregateTag:     it.AggregateTag,
			}
			totalBytes := fc.typeByteSize(arrInfo)
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: totalBytes})
			alloc := fc.c.ensureIntrinsicWrapper("Alloc", 1, 1)
			fc.emit(ir.Inst{Op: ir.OP_CALL, Name: alloc, Arg: 1})
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
			fc.emitArrayInitializerAt(it.Name, idx, 0, arrInfo, it.Init, fc.sig.File, n.Line, n.Col)
			continue
		}
		if len(it.Init) == 0 {
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
			continue
		}
		if it.Kind == cDeclScalar {
			fc.emitExprTokensAsType(fc.sig.File, n.Line, n.Col, it.Init, cTypeInfo{Kind: cDeclScalar, Base: it.Base})
		} else {
			fc.emitExprTokens(fc.sig.File, n.Line, n.Col, it.Init)
		}
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
	}
}

func (fc *funcCompiler) compileIfStmt(n *Node) {
	condToks, err := lexSnippet(fc.sig.File, n.Text)
	if err != nil {
		fc.errorf(fc.sig.File, n.Line, n.Col, "invalid if condition: %v", err)
		condToks = nil
	}
	elseLabel := fc.c.nextLabel()
	endLabel := fc.c.nextLabel()
	fc.emitCondJumpFalse(fc.sig.File, n.Line, n.Col, condToks, elseLabel)
	if len(n.Children) > 0 {
		fc.compileStmt(n.Children[0])
	}
	if len(n.Children) > 1 {
		fc.emit(ir.Inst{Op: ir.OP_JMP, Arg: endLabel})
		fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: elseLabel})
		fc.compileStmt(n.Children[1])
		fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: endLabel})
	} else {
		fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: elseLabel})
	}
}

func (fc *funcCompiler) compileWhileStmt(n *Node) {
	condToks, err := lexSnippet(fc.sig.File, n.Text)
	if err != nil {
		fc.errorf(fc.sig.File, n.Line, n.Col, "invalid while condition: %v", err)
		condToks = nil
	}
	startLabel := fc.c.nextLabel()
	endLabel := fc.c.nextLabel()
	fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: startLabel})
	fc.emitCondJumpFalse(fc.sig.File, n.Line, n.Col, condToks, endLabel)
	fc.breakTargets = append(fc.breakTargets, endLabel)
	fc.continueTargets = append(fc.continueTargets, startLabel)
	if len(n.Children) > 0 {
		fc.compileStmt(n.Children[0])
	}
	fc.breakTargets = fc.breakTargets[:len(fc.breakTargets)-1]
	fc.continueTargets = fc.continueTargets[:len(fc.continueTargets)-1]
	fc.emit(ir.Inst{Op: ir.OP_JMP, Arg: startLabel})
	fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: endLabel})
}

func (fc *funcCompiler) compileDoWhileStmt(n *Node) {
	condToks, err := lexSnippet(fc.sig.File, n.Text)
	if err != nil {
		fc.errorf(fc.sig.File, n.Line, n.Col, "invalid do-while condition: %v", err)
		condToks = nil
	}
	startLabel := fc.c.nextLabel()
	continueLabel := fc.c.nextLabel()
	endLabel := fc.c.nextLabel()
	fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: startLabel})
	fc.breakTargets = append(fc.breakTargets, endLabel)
	fc.continueTargets = append(fc.continueTargets, continueLabel)
	if len(n.Children) > 0 {
		fc.compileStmt(n.Children[0])
	}
	fc.breakTargets = fc.breakTargets[:len(fc.breakTargets)-1]
	fc.continueTargets = fc.continueTargets[:len(fc.continueTargets)-1]
	fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: continueLabel})
	fc.emitCondJumpTrue(fc.sig.File, n.Line, n.Col, condToks, startLabel)
	fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: endLabel})
}

func (fc *funcCompiler) compileForStmt(n *Node) {
	head, err := lexSnippet(fc.sig.File, n.Text)
	if err != nil {
		fc.errorf(fc.sig.File, n.Line, n.Col, "invalid for header: %v", err)
		head = nil
	}
	parts := splitTopLevel(head, ";")
	if len(parts) != 3 {
		fc.errorf(fc.sig.File, n.Line, n.Col, "for header must be init;cond;post")
		parts = [][]Token{nil, nil, nil}
	}
	initToks := trimTokens(parts[0])
	condToks := trimTokens(parts[1])
	postToks := trimTokens(parts[2])

	fc.pushScope()
	defer fc.popScope()

	if len(initToks) > 0 {
		if looksLikeConcreteDeclStart(initToks) {
			declNode := &Node{Line: n.Line, Col: n.Col}
			fc.compileDeclTokens(fc.sig.File, declNode, initToks)
		} else {
			fc.emitExprTokens(fc.sig.File, n.Line, n.Col, initToks)
			fc.emit(ir.Inst{Op: ir.OP_DROP})
		}
	}

	startLabel := fc.c.nextLabel()
	continueLabel := fc.c.nextLabel()
	endLabel := fc.c.nextLabel()

	fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: startLabel})
	if len(condToks) > 0 {
		fc.emitCondJumpFalse(fc.sig.File, n.Line, n.Col, condToks, endLabel)
	}

	fc.breakTargets = append(fc.breakTargets, endLabel)
	fc.continueTargets = append(fc.continueTargets, continueLabel)
	if len(n.Children) > 0 {
		fc.compileStmt(n.Children[0])
	}
	fc.breakTargets = fc.breakTargets[:len(fc.breakTargets)-1]
	fc.continueTargets = fc.continueTargets[:len(fc.continueTargets)-1]

	fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: continueLabel})
	if len(postToks) > 0 {
		fc.emitExprTokens(fc.sig.File, n.Line, n.Col, postToks)
		fc.emit(ir.Inst{Op: ir.OP_DROP})
	}
	fc.emit(ir.Inst{Op: ir.OP_JMP, Arg: startLabel})
	fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: endLabel})
}

func (fc *funcCompiler) compileReturnStmt(n *Node) {
	if fc.sig.RetByPtr {
		if strings.TrimSpace(n.Text) != "" {
			toks, err := lexSnippet(fc.sig.File, n.Text)
			if err != nil {
				fc.errorf(fc.sig.File, n.Line, n.Col, "invalid expression: %v", err)
			} else {
				ep := &cExprParser{fc: fc, file: fc.sig.File, line: n.Line, col: n.Col, toks: trimTokens(toks)}
				ex := ep.parseExpression()
				if ex != nil {
					if ep.pos < len(ep.toks) {
						got := ep.toks[ep.pos]
						fc.errorf(fc.sig.File, n.Line, n.Col, "unexpected token in expression: %q", got.Text)
					} else if srcInfo, ok := fc.exprTypeInfo(ex); ok && isAggregateObjectType(srcInfo) {
						srcTmp := fc.allocTempLocal("$ret_src")
						if fc.emitAggregateExprAddress(ex) {
							fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: srcTmp})
							size := fc.typeByteSize(srcInfo)
							fc.emitCopyAggregateBytes(fc.retPtrLocal, srcTmp, size)
						} else {
							fc.errorf(fc.sig.File, n.Line, n.Col, "aggregate return expression must be addressable")
						}
					} else if ex != nil {
						fc.errorf(fc.sig.File, n.Line, n.Col, "return expression must produce aggregate object")
					}
				}
			}
		}
		fc.emit(ir.Inst{Op: ir.OP_RETURN, Arg: 0})
		return
	}
	if fc.sig.RetCount == 0 {
		if strings.TrimSpace(n.Text) != "" {
			fc.compileExprText(n.Text, n.Line, n.Col)
			fc.emit(ir.Inst{Op: ir.OP_DROP})
		}
		fc.emit(ir.Inst{Op: ir.OP_RETURN, Arg: 0})
		return
	}
	if strings.TrimSpace(n.Text) == "" {
		fc.emitZeroValue(cTypeInfo{Kind: fc.sig.RetKind, PtrDepth: fc.sig.RetPtrDepth, Base: fc.sig.RetBase})
	} else {
		toks, err := lexSnippet(fc.sig.File, n.Text)
		if err != nil {
			fc.errorf(fc.sig.File, n.Line, n.Col, "invalid expression: %v", err)
			fc.emitZeroValue(cTypeInfo{Kind: fc.sig.RetKind, PtrDepth: fc.sig.RetPtrDepth, Base: fc.sig.RetBase})
		} else {
			fc.emitExprTokensAsType(fc.sig.File, n.Line, n.Col, toks, cTypeInfo{
				Kind:     fc.sig.RetKind,
				PtrDepth: fc.sig.RetPtrDepth,
				Base:     fc.sig.RetBase,
			})
		}
		fc.emit(ir.Inst{Op: ir.OP_RETURN, Arg: 1})
		return
	}
	fc.emitCastToType(cTypeInfo{
			Kind:     fc.sig.RetKind,
			PtrDepth: fc.sig.RetPtrDepth,
			Base:     fc.sig.RetBase,
		})
	fc.emit(ir.Inst{Op: ir.OP_RETURN, Arg: 1})
}

func (fc *funcCompiler) compileDeclTokens(file string, n *Node, toks []Token) {
	if isGNUAsmStatementText(tokenSliceText(toks)) {
		return
	}
	items, enumConsts, hasExtern, hasTypedef, err := parseDeclItems(toks, fc.enumLookupMap(), true)
	if err != nil {
		if fc.shouldFallbackDeclToExpr(toks, err) {
			fc.emitExprTokens(file, n.Line, n.Col, toks)
			fc.emit(ir.Inst{Op: ir.OP_DROP})
			return
		}
		fc.errorf(file, n.Line, n.Col, "%v", err)
		return
	}
	if len(enumConsts) > 0 {
		fc.addLocalEnumConsts(enumConsts, file, n.Line, n.Col)
	}
	if hasTypedef {
		if hasExtern {
			fc.errorf(file, n.Line, n.Col, "extern typedef declarations are not supported")
		}
		for _, it := range items {
			if len(it.Init) > 0 {
				fc.errorf(file, n.Line, n.Col, "typedef declaration with initializer is not supported: %s", it.Name)
				continue
			}
			fc.addLocalTypedef(it.Name, cTypeInfo{
				Kind:             it.Kind,
				PtrDepth:         it.PtrDepth,
				ArrayLen:         it.ArrayLen,
				ArrayDims:        cloneInt64s(it.ArrayDims),
				IsVoid:           it.IsVoid,
				Base:             it.Base,
				FuncSig:          cloneFuncTypeSig(it.FuncSig),
				OpaqueAggregate:  it.OpaqueAggregate,
				AggregateKeyword: it.AggregateKeyword,
				AggregateTag:     it.AggregateTag,
			}, file, n.Line, n.Col)
		}
		return
	}
	for _, it := range items {
		info := cTypeInfo{
			Kind:             it.Kind,
			PtrDepth:         it.PtrDepth,
			ArrayLen:         it.ArrayLen,
			ArrayDims:        cloneInt64s(it.ArrayDims),
			IsVoid:           it.IsVoid,
			Base:             it.Base,
			FuncSig:          cloneFuncTypeSig(it.FuncSig),
			OpaqueAggregate:  it.OpaqueAggregate,
			AggregateKeyword: it.AggregateKeyword,
			AggregateTag:     it.AggregateTag,
		}
		elemStep := fc.pointerElemStep(it.Kind, it.PtrDepth, it.Base, it.IsVoid, it.OpaqueAggregate, it.AggregateKeyword, it.AggregateTag)
		if len(it.RuntimeArrayBound) > 0 {
			elemStep = fc.typeByteSize(it.RuntimeArrayElem)
		}
		if it.Kind == cDeclArray {
			elemStep = fc.typeByteSize(arrayElementTypeInfo(info))
		}
		if it.Kind == cDeclPointer && it.PtrDepth == 1 && isStringLiteralExpr(it.Init) {
			elemStep = 1
		}
		idx := fc.addLocalDecl(it.Name, it.Kind, it.Base, it.PtrDepth, elemStep, it.ArrayLen, it.ArrayDims, it.FuncSig, it.OpaqueAggregate, it.AggregateKeyword, it.AggregateTag, file, n.Line, n.Col)
		if len(it.RuntimeArrayBound) > 0 {
			fc.emitExprTokens(file, n.Line, n.Col, it.RuntimeArrayBound)
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: fc.typeByteSize(it.RuntimeArrayElem)})
			fc.emit(ir.Inst{Op: ir.OP_MUL})
			alloc := fc.c.ensureIntrinsicWrapper("Alloc", 1, 1)
			fc.emit(ir.Inst{Op: ir.OP_CALL, Name: alloc, Arg: 1})
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
			continue
		}
		if isAggregateObjectDecl(it.Kind, it.PtrDepth, it.OpaqueAggregate, it.AggregateKeyword, it.AggregateTag) {
			fc.initLocalAggregateObject(it.Name, idx, cTypeInfo{
				Kind:             it.Kind,
				PtrDepth:         it.PtrDepth,
				Base:             it.Base,
				OpaqueAggregate:  it.OpaqueAggregate,
				AggregateKeyword: it.AggregateKeyword,
				AggregateTag:     it.AggregateTag,
			}, it.Init, file, n.Line, n.Col)
			continue
		}
		if it.Kind == cDeclArray {
			arrInfo := cTypeInfo{
				Kind:             it.Kind,
				PtrDepth:         it.PtrDepth,
				ArrayLen:         it.ArrayLen,
				ArrayDims:        cloneInt64s(it.ArrayDims),
				Base:             it.Base,
				IsVoid:           it.IsVoid,
				FuncSig:          cloneFuncTypeSig(it.FuncSig),
				OpaqueAggregate:  it.OpaqueAggregate,
				AggregateKeyword: it.AggregateKeyword,
				AggregateTag:     it.AggregateTag,
			}
			totalBytes := fc.typeByteSize(arrInfo)
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: totalBytes})
			alloc := fc.c.ensureIntrinsicWrapper("Alloc", 1, 1)
			fc.emit(ir.Inst{Op: ir.OP_CALL, Name: alloc, Arg: 1})
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
			fc.emitArrayInitializerAt(it.Name, idx, 0, arrInfo, it.Init, file, n.Line, n.Col)
			continue
		}
		if len(it.Init) == 0 {
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
			continue
		}
		if it.Kind == cDeclScalar {
			fc.emitExprTokensAsType(file, n.Line, n.Col, it.Init, cTypeInfo{Kind: cDeclScalar, Base: it.Base})
		} else {
			fc.emitExprTokens(file, n.Line, n.Col, it.Init)
		}
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
	}
}

func isDeclaratorParseError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "complex declarators are not yet supported") ||
		strings.Contains(msg, "unable to parse declarator name") ||
		strings.Contains(msg, "missing declarator")
}

func looksLikeConcreteDeclStart(tokens []Token) bool {
	tokens = trimTokens(tokens)
	if len(tokens) == 0 {
		return false
	}
	first := tokens[0]
	if first.Kind != TokIdent {
		return false
	}
	if isDeclarationKeyword(first) || isUnsupportedCTypeKeyword(first.Text) {
		return true
	}
	if _, ok := lookupBuiltinTypedefAlias(first.Text); ok {
		return true
	}
	if _, ok := lookupTypedefAlias(first.Text); ok {
		return true
	}
	return false
}

func (fc *funcCompiler) shouldFallbackDeclToExpr(toks []Token, err error) bool {
	if !isDeclaratorParseError(err) {
		return false
	}
	return !looksLikeConcreteDeclStart(toks)
}

func (fc *funcCompiler) emitZeroValue(info cTypeInfo) {
	if isFloatTypeInfo(info) {
		if info.Base == cScalarFloat {
			fc.emit(ir.Inst{Op: ir.OP_CONST_F32, Width: 4, Name: "0.0"})
		} else {
			fc.emit(ir.Inst{Op: ir.OP_CONST_F64, Width: 8, Name: "0.0"})
		}
		return
	}
	fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
}

func (fc *funcCompiler) emitConditionValue(ex *expr) {
	if ex == nil {
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		return
	}
	fc.emitExpr(ex)
	if t, ok := fc.exprTypeInfo(ex); ok && isFloatTypeInfo(t) {
		fc.emitZeroValue(t)
		if t.Base == cScalarFloat {
			fc.emit(ir.Inst{Op: ir.OP_NEQ, Width: 4, Name: "float32"})
		} else {
			fc.emit(ir.Inst{Op: ir.OP_NEQ, Width: 8, Name: "float64"})
		}
	}
}

func (fc *funcCompiler) emitTypedBinaryInst(op string, info cTypeInfo) {
	inst := ir.Inst{}
	switch op {
	case "+":
		inst.Op = ir.OP_ADD
	case "-":
		inst.Op = ir.OP_SUB
	case "*":
		inst.Op = ir.OP_MUL
	case "/":
		inst.Op = ir.OP_DIV
	case "==":
		inst.Op = ir.OP_EQ
	case "!=":
		inst.Op = ir.OP_NEQ
	case "<":
		inst.Op = ir.OP_LT
	case "<=":
		inst.Op = ir.OP_LEQ
	case ">":
		inst.Op = ir.OP_GT
	case ">=":
		inst.Op = ir.OP_GEQ
	default:
		fc.errorf(fc.sig.File, 0, 0, "unsupported typed binary operator %q", op)
		return
	}
	if isFloatTypeInfo(info) {
		inst.Width = int(fc.typeByteSize(info))
		inst.Name = fc.convertNameForScalar(info.Base)
	}
	fc.emit(inst)
}

func (fc *funcCompiler) emitCondJumpFalse(file string, line int, col int, cond []Token, falseLabel int) {
	if len(cond) == 0 {
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 1})
	} else {
		fc.emitConditionValue(fc.parseExprTokens(file, line, col, cond))
	}
	fc.emit(ir.Inst{Op: ir.OP_JMP_IF_NOT, Arg: falseLabel})
}

func (fc *funcCompiler) emitCondJumpTrue(file string, line int, col int, cond []Token, trueLabel int) {
	if len(cond) == 0 {
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 1})
	} else {
		fc.emitConditionValue(fc.parseExprTokens(file, line, col, cond))
	}
	fc.emit(ir.Inst{Op: ir.OP_JMP_IF, Arg: trueLabel})
}

func (fc *funcCompiler) compileExprText(text string, line int, col int) {
	if isGNUAsmStatementText(text) {
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		return
	}
	toks, err := lexSnippet(fc.sig.File, text)
	if err != nil {
		fc.errorf(fc.sig.File, line, col, "invalid expression: %v", err)
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		return
	}
	fc.emitExprTokens(fc.sig.File, line, col, toks)
}

func (fc *funcCompiler) parseExprTokens(file string, line int, col int, toks []Token) *expr {
	ep := &cExprParser{fc: fc, file: file, line: line, col: col, toks: trimTokens(toks)}
	ex := ep.parseExpression()
	if ex == nil {
		return nil
	}
	if ep.pos < len(ep.toks) {
		got := ep.toks[ep.pos]
		fc.errorf(file, line, col, "unexpected token in expression: %q", got.Text)
	}
	return ex
}

func (fc *funcCompiler) emitExprTokens(file string, line int, col int, toks []Token) {
	ex := fc.parseExprTokens(file, line, col, toks)
	if ex == nil {
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		return
	}
	fc.emitExpr(ex)
}

func (fc *funcCompiler) emitExprValueCast(ex *expr, dst cTypeInfo) {
	if ex == nil {
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		fc.emitCastToType(dst)
		return
	}
	fc.emitExpr(ex)
	if src, ok := fc.exprTypeInfo(ex); ok {
		fc.emitCastValueToType(src, dst)
		return
	}
	fc.emitCastToType(dst)
}

func (fc *funcCompiler) emitExprTokensAsType(file string, line int, col int, toks []Token, dst cTypeInfo) {
	ex := fc.parseExprTokens(file, line, col, toks)
	fc.emitExprValueCast(ex, dst)
}

func (fc *funcCompiler) emitIndexAddr(base *expr, index *expr) {
	step := fc.exprPointerStep(base)
	fc.emitExpr(base)
	fc.emitExpr(index)
	fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: step})
	fc.emit(ir.Inst{Op: ir.OP_MUL})
	fc.emit(ir.Inst{Op: ir.OP_ADD})
}

func (fc *funcCompiler) allocTempLocalForType(name string, info cTypeInfo) int {
	if isAggregateObjectType(info) || info.Kind == cDeclArray {
		return fc.allocTempLocal(name)
	}
	ptrDepth := info.PtrDepth
	if info.Kind == cDeclPointer && ptrDepth == 0 {
		ptrDepth = 1
	}
	elemStep := int64(fc.c.target.PtrSize)
	if info.Kind == cDeclPointer || info.Kind == cDeclArray {
		elemStep = fc.pointerStepForType(info)
	} else {
		elemStep = fc.typeByteSize(info)
	}
	return fc.addLocalDecl(fmt.Sprintf("%s$%d", name, fc.c.nextLabel()), info.Kind, info.Base, ptrDepth, elemStep, info.ArrayLen, info.ArrayDims, cloneFuncTypeSig(info.FuncSig), info.OpaqueAggregate, info.AggregateKeyword, info.AggregateTag, fc.sig.File, 0, 0)
}

func (fc *funcCompiler) emitLoadForType(width int, info cTypeInfo) {
	inst := ir.Inst{Op: ir.OP_LOAD, Arg: width}
	if isFloatTypeInfo(info) {
		inst.Name = fc.convertNameForScalar(info.Base)
	}
	fc.emit(inst)
}

func (fc *funcCompiler) emitStoreForType(width int, info cTypeInfo) {
	inst := ir.Inst{Op: ir.OP_STORE, Arg: width}
	if isFloatTypeInfo(info) {
		inst.Name = fc.convertNameForScalar(info.Base)
	}
	fc.emit(inst)
}

func (fc *funcCompiler) emitAddressOf(ex *expr) bool {
	if ex == nil {
		return false
	}
	switch ex.kind {
	case exprVar:
		if b, ok := fc.lookupLocalBinding(ex.name); ok {
			if b.Kind == cDeclArray || isAggregateObjectDecl(b.Kind, b.PtrDepth, b.OpaqueAggregate, b.AggregateKeyword, b.AggregateTag) {
				fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: b.Index})
			} else {
				fc.emit(ir.Inst{Op: ir.OP_LOCAL_ADDR, Arg: b.Index})
			}
			return true
		}
		if fc.c.canLoadExternDataOnTarget(ex.name) {
			wrap := fc.c.ensureExternDataWrapper(ex.name, true)
			if wrap != "" {
				fc.emit(ir.Inst{Op: ir.OP_CALL, Name: wrap, Arg: 0})
				return true
			}
		}
		if idx, ok := fc.lookupGlobal(ex.name); ok {
			kind, _ := fc.lookupGlobalKind(ex.name)
			ptrDepth, _ := fc.lookupGlobalPtrDepth(ex.name)
			opaque, aggKey, aggTag := fc.lookupGlobalOpaqueAggregate(ex.name)
			if kind == cDeclArray || isAggregateObjectDecl(kind, ptrDepth, opaque, aggKey, aggTag) {
				fc.emit(ir.Inst{Op: ir.OP_GLOBAL_GET, Arg: idx})
			} else {
				fc.emit(ir.Inst{Op: ir.OP_GLOBAL_ADDR, Arg: idx})
			}
			return true
		}
		if _, ok := fc.lookupEnumConst(ex.name); ok {
			return false
		}
		if id, ok := fc.c.funcIDs[ex.name]; ok {
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: id})
			return true
		}
		return false
	case exprUnary:
		if ex.op == "*" {
			fc.emitExpr(ex.left)
			return true
		}
		return false
	case exprIndex:
		fc.emitIndexAddr(ex.left, ex.right)
		return true
	case exprMember:
		return fc.emitMemberAddress(ex, false)
	case exprCompoundLit:
		info := ex.typeInfo
		tmpName := fmt.Sprintf("$compound_lit$%d", fc.c.nextLabel())
		if isAggregateObjectType(info) {
			idx := fc.addLocalDecl(tmpName, info.Kind, info.Base, info.PtrDepth, int64(fc.c.target.PtrSize), 0, nil, cloneFuncTypeSig(info.FuncSig), info.OpaqueAggregate, info.AggregateKeyword, info.AggregateTag, fc.sig.File, 0, 0)
			fc.initLocalAggregateObject(tmpName, idx, info, ex.initTok, fc.sig.File, 0, 0)
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: idx})
			return true
		}
		if info.Kind == cDeclArray {
			if info.ArrayLen == cArrayLenUnspecified {
				if n, ok, err := inferArrayLengthFromInit(info.Base, ex.initTok, fc.enumLookupMap()); err != nil {
					fc.errorf(fc.sig.File, 0, 0, "invalid compound literal array bound: %v", err)
					return false
				} else if ok {
					info.ArrayLen = n
				}
			}
			if info.ArrayLen == cArrayLenUnspecified {
				fc.errorf(fc.sig.File, 0, 0, "compound literal array bounds must not be empty")
				return false
			}
			idx := fc.addLocalDecl(tmpName, info.Kind, info.Base, info.PtrDepth, fc.typeByteSize(arrayElementTypeInfo(info)), info.ArrayLen, info.ArrayDims, cloneFuncTypeSig(info.FuncSig), info.OpaqueAggregate, info.AggregateKeyword, info.AggregateTag, fc.sig.File, 0, 0)
			totalBytes := fc.typeByteSize(info)
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: totalBytes})
			alloc := fc.c.ensureIntrinsicWrapper("Alloc", 1, 1)
			fc.emit(ir.Inst{Op: ir.OP_CALL, Name: alloc, Arg: 1})
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
			fc.emitArrayInitializerAt(tmpName, idx, 0, info, ex.initTok, fc.sig.File, 0, 0)
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: idx})
			return true
		}
		idx := fc.addLocalDecl(tmpName, info.Kind, info.Base, info.PtrDepth, int64(fc.c.target.PtrSize), 0, nil, cloneFuncTypeSig(info.FuncSig), info.OpaqueAggregate, info.AggregateKeyword, info.AggregateTag, fc.sig.File, 0, 0)
		parts, err := parseBraceInitializerParts(ex.initTok)
		if err != nil || len(parts) == 0 {
			fc.errorf(fc.sig.File, 0, 0, "invalid compound literal initializer")
			return false
		}
		fc.emitExprTokensAsType(fc.sig.File, 0, 0, parts[0], info)
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_ADDR, Arg: idx})
		return true
	case exprAssign:
		if info, ok := fc.exprTypeInfo(ex); ok && isAggregateObjectType(info) {
			fc.emitExpr(ex)
			return true
		}
		return false
	case exprCall, exprConditional, exprComma:
		return fc.emitAggregateExprAddress(ex)
	default:
		return false
	}
}

func (fc *funcCompiler) longSize() int64 {
	return cLongSizeForTarget(fc.c.target)
}

func (fc *funcCompiler) scalarSize(base cScalarType) int64 {
	return scalarSizeForTarget(fc.c.target, base)
}

func (fc *funcCompiler) typeScalarWidth(info cTypeInfo) int64 {
	if info.IsVoid {
		return 1
	}
	if info.Kind == cDeclPointer {
		return int64(fc.c.target.PtrSize)
	}
	if info.AggregateKeyword != "" && info.AggregateTag != "" && !info.OpaqueAggregate {
		if agg, ok := fc.lookupAggregate(info.AggregateKeyword, info.AggregateTag); ok && len(agg.Fields) > 0 {
			return agg.Size
		}
	}
	return fc.scalarSize(info.Base)
}

func (fc *funcCompiler) typeByteSize(info cTypeInfo) int64 {
	switch info.Kind {
	case cDeclPointer:
		return int64(fc.c.target.PtrSize)
	case cDeclArray:
		return info.ArrayLen * fc.typeByteSize(arrayElementTypeInfo(info))
	default:
		if info.IsVoid {
			return 1
		}
		return fc.typeScalarWidth(info)
	}
}

func (fc *funcCompiler) pointerStepForType(info cTypeInfo) int64 {
	if info.Kind == cDeclArray {
		return fc.typeByteSize(arrayElementTypeInfo(info))
	}
	if info.Kind != cDeclPointer {
		return int64(fc.c.target.PtrSize)
	}
	if info.PtrDepth > 1 {
		return int64(fc.c.target.PtrSize)
	}
	if info.IsVoid {
		return 1
	}
	if info.AggregateKeyword != "" && info.AggregateTag != "" && !info.OpaqueAggregate {
		if agg, ok := fc.lookupAggregate(info.AggregateKeyword, info.AggregateTag); ok && len(agg.Fields) > 0 {
			return agg.Size
		}
	}
	return fc.scalarSize(info.Base)
}

func isFloatScalar(base cScalarType) bool {
	return base == cScalarFloat || base == cScalarDouble
}

func isUnsignedScalar(base cScalarType) bool {
	switch base {
	case cScalarUInt, cScalarUChar, cScalarUShort, cScalarULong:
		return true
	default:
		return false
	}
}

func isFloatTypeInfo(info cTypeInfo) bool {
	return info.Kind == cDeclScalar &&
		info.PtrDepth == 0 &&
		!info.IsVoid &&
		!info.OpaqueAggregate &&
		info.AggregateKeyword == "" &&
		isFloatScalar(info.Base)
}

func (fc *funcCompiler) convertNameForScalar(base cScalarType) string {
	switch base {
	case cScalarChar:
		return "int8"
	case cScalarUChar:
		return "uint8"
	case cScalarShort:
		return "int16"
	case cScalarUShort:
		return "uint16"
	case cScalarInt:
		return "int32"
	case cScalarUInt:
		return "uint32"
	case cScalarLong:
		if fc.longSize() == 4 {
			return "int32"
		}
		return "int64"
	case cScalarULong:
		if fc.longSize() == 4 {
			return "uint32"
		}
		return "uint64"
	case cScalarFloat:
		return "float32"
	case cScalarDouble:
		return "float64"
	default:
		return ""
	}
}

func (fc *funcCompiler) convertSourceKindForType(info cTypeInfo) int64 {
	if isFloatTypeInfo(info) {
		if info.Base == cScalarFloat {
			return ir.CONVERT_SRC_FLOAT32
		}
		return ir.CONVERT_SRC_FLOAT64
	}
	if info.Kind == cDeclPointer || info.Kind == cDeclArray {
		return ir.CONVERT_SRC_UINT
	}
	if info.Kind == cDeclScalar && isUnsignedScalar(info.Base) {
		return ir.CONVERT_SRC_UINT
	}
	if info.Kind == cDeclScalar {
		return ir.CONVERT_SRC_INT
	}
	return ir.CONVERT_SRC_UNKNOWN
}

func (fc *funcCompiler) emitCastValueToType(src cTypeInfo, info cTypeInfo) {
	switch info.Kind {
	case cDeclPointer:
		fc.emit(ir.Inst{Op: ir.OP_CONVERT, Name: "uintptr", Width: int(fc.typeByteSize(src)), Val: fc.convertSourceKindForType(src)})
		return
	case cDeclArray:
		return
	case cDeclScalar:
		if info.IsVoid {
			return
		}
		name := fc.convertNameForScalar(info.Base)
		if name != "" {
			fc.emit(ir.Inst{Op: ir.OP_CONVERT, Name: name, Width: int(fc.typeByteSize(src)), Val: fc.convertSourceKindForType(src)})
		}
	}
}

func (fc *funcCompiler) emitCastToType(info cTypeInfo) {
	fc.emitCastValueToType(cTypeInfo{}, info)
}

func (fc *funcCompiler) varTypeInfo(name string) (cTypeInfo, bool) {
	if b, ok := fc.lookupLocalBinding(name); ok {
		return cTypeInfo{
			Kind:             b.Kind,
			PtrDepth:         b.PtrDepth,
			ArrayLen:         b.ArrayLen,
			ArrayDims:        cloneInt64s(b.ArrayDims),
			Base:             b.Base,
			FuncSig:          cloneFuncTypeSig(b.FuncSig),
			OpaqueAggregate:  b.OpaqueAggregate,
			AggregateKeyword: b.AggregateKeyword,
			AggregateTag:     b.AggregateTag,
		}, true
	}
	if info, ok := fc.c.externDataTypes[name]; ok {
		return cTypeInfo{
			Kind:             info.Kind,
			PtrDepth:         info.PtrDepth,
			ArrayLen:         info.ArrayLen,
			ArrayDims:        cloneInt64s(info.ArrayDims),
			Base:             info.Base,
			FuncSig:          cloneFuncTypeSig(info.FuncSig),
			OpaqueAggregate:  info.OpaqueAggregate,
			AggregateKeyword: info.AggregateKeyword,
			AggregateTag:     info.AggregateTag,
		}, true
	}
	if kind, ok := fc.lookupGlobalKind(name); ok {
		base, _ := fc.lookupGlobalBase(name)
		ptrDepth, _ := fc.lookupGlobalPtrDepth(name)
		n, _ := fc.lookupGlobalArrayLen(name)
		dims, _ := fc.lookupGlobalArrayDims(name)
		fsig, _ := fc.lookupGlobalFuncSig(name)
		opaque, aggKey, aggTag := fc.lookupGlobalOpaqueAggregate(name)
		return cTypeInfo{
			Kind:             kind,
			PtrDepth:         ptrDepth,
			ArrayLen:         n,
			ArrayDims:        cloneInt64s(dims),
			Base:             base,
			FuncSig:          fsig,
			OpaqueAggregate:  opaque,
			AggregateKeyword: aggKey,
			AggregateTag:     aggTag,
		}, true
	}
	if _, ok := fc.lookupEnumConst(name); ok {
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarInt}, true
	}
	if fs, ok := fc.c.funcs[name]; ok {
		return cTypeInfo{
			Kind:     cDeclPointer,
			PtrDepth: 1,
			Base:     fs.RetBase,
			IsVoid:   fs.RetCount == 0,
			FuncSig:  funcSigToTypeSig(fs),
		}, true
	}
	return cTypeInfo{}, false
}

func (fc *funcCompiler) pointerStepForVar(name string) int64 {
	if step, ok := fc.lookupLocalElemStep(name); ok && step > 0 {
		return step
	}
	if step, ok := fc.lookupGlobalElemStep(name); ok && step > 0 {
		return step
	}
	return int64(fc.c.target.PtrSize)
}

func (fc *funcCompiler) callDesignatorName(ex *expr) (string, bool) {
	for ex != nil && ex.kind == exprUnary && ex.op == "*" {
		ex = ex.left
	}
	if ex == nil || ex.kind != exprVar {
		return "", false
	}
	return ex.name, true
}

func (fc *funcCompiler) builtinVariadicCallName(call *expr) (string, bool) {
	if call == nil {
		return "", false
	}
	name, ok := fc.callDesignatorName(call.left)
	if !ok {
		return "", false
	}
	if name == "__builtin_va_count" || name == "__builtin_va_arg" {
		return name, true
	}
	return "", false
}

func (fc *funcCompiler) emitVariadicPackFromLocals(extraLocals []int) {
	fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(len(extraLocals))})
	if len(extraLocals) == 0 {
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		return
	}
	firstElem := -1
	lastElem := -1
	i := 0
	for i < len(extraLocals) {
		elemName := fmt.Sprintf("$va_pack_elem$%d$%d", fc.c.nextLabel(), i)
		elemIdx := fc.addLocal(elemName, fc.sig.File, 0, 0)
		if firstElem < 0 {
			firstElem = elemIdx
		}
		lastElem = elemIdx
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: elemIdx})
		i++
	}
	if firstElem < 0 {
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		return
	}
	ptrLocal := fc.addLocal(fmt.Sprintf("$va_pack_ptr$%d", fc.c.nextLabel()), fc.sig.File, 0, 0)
	baseElem := lastElem
	if fc.c.target.Backend == "c" {
		baseElem = firstElem
	}
	fc.emit(ir.Inst{Op: ir.OP_LOCAL_ADDR, Arg: baseElem})
	fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: ptrLocal})
	i = 0
	for i < len(extraLocals) {
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: extraLocals[i]})
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: ptrLocal})
		if i > 0 {
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(i * fc.c.target.PtrSize)})
			fc.emit(ir.Inst{Op: ir.OP_ADD})
		}
		fc.emit(ir.Inst{Op: ir.OP_STORE, Arg: fc.c.target.PtrSize})
		i++
	}
	fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: ptrLocal})
}

func (fc *funcCompiler) emitBuiltinVariadicCall(call *expr) bool {
	name, ok := fc.builtinVariadicCallName(call)
	if !ok {
		return false
	}
	switch name {
	case "__builtin_va_count":
		if len(call.args) != 0 {
			fc.errorf(fc.sig.File, 0, 0, "__builtin_va_count expects 0 arguments")
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			return true
		}
		if fc.variadicCount < 0 {
			fc.errorf(fc.sig.File, 0, 0, "__builtin_va_count can only be used inside variadic function definitions")
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			return true
		}
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: fc.variadicCount})
		return true
	case "__builtin_va_arg":
		if len(call.args) != 1 {
			fc.errorf(fc.sig.File, 0, 0, "__builtin_va_arg expects 1 argument")
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			return true
		}
		if fc.variadicCount < 0 || fc.variadicData < 0 {
			fc.errorf(fc.sig.File, 0, 0, "__builtin_va_arg can only be used inside variadic function definitions")
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			return true
		}
		idxLocal := fc.addLocal(fmt.Sprintf("$va_idx$%d", fc.c.nextLabel()), fc.sig.File, 0, 0)
		fc.emitExpr(call.args[0])
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idxLocal})
		zeroLabel := fc.c.nextLabel()
		endLabel := fc.c.nextLabel()
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: idxLocal})
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		fc.emit(ir.Inst{Op: ir.OP_LT})
		fc.emit(ir.Inst{Op: ir.OP_JMP_IF, Arg: zeroLabel})
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: idxLocal})
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: fc.variadicCount})
		fc.emit(ir.Inst{Op: ir.OP_GEQ})
		fc.emit(ir.Inst{Op: ir.OP_JMP_IF, Arg: zeroLabel})
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: fc.variadicData})
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: idxLocal})
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(fc.c.target.PtrSize)})
		fc.emit(ir.Inst{Op: ir.OP_MUL})
		fc.emit(ir.Inst{Op: ir.OP_ADD})
		fc.emit(ir.Inst{Op: ir.OP_LOAD, Arg: fc.c.target.PtrSize})
		fc.emit(ir.Inst{Op: ir.OP_JMP, Arg: endLabel})
		fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: zeroLabel})
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: endLabel})
		return true
	}
	return false
}

func (fc *funcCompiler) resolveDirectCallSig(call *expr) (*cFuncSig, bool) {
	if call == nil {
		return nil, false
	}
	name, ok := fc.callDesignatorName(call.left)
	if !ok || name == "" {
		return nil, false
	}
	// Locals/globals shadow function identifiers.
	if _, ok := fc.lookupLocal(name); ok {
		return nil, false
	}
	if _, ok := fc.lookupGlobal(name); ok {
		return nil, false
	}
	if _, ok := fc.lookupEnumConst(name); ok {
		return nil, false
	}
	sig, ok := fc.c.funcs[name]
	if !ok {
		return &cFuncSig{
			Name:             name,
			IRName:           name,
			ParamCount:       len(call.args),
			ParamUnspecified: true,
			RetCount:         1,
			RetKind:          cDeclScalar,
			RetBase:          cScalarInt,
			Variadic:         true,
		}, true
	}
	return sig, true
}

func (fc *funcCompiler) emitCallTargetValue(ex *expr) {
	if ex == nil {
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		return
	}
	if ex.kind == exprUnary && ex.op == "*" {
		if t, ok := fc.exprTypeInfo(ex.left); ok && t.Kind == cDeclPointer && t.PtrDepth == 1 {
			// In call position, dereferencing a function pointer yields a
			// function designator, which still carries the same call target value.
			fc.emitCallTargetValue(ex.left)
			return
		}
	}
	fc.emitExpr(ex)
}

func (fc *funcCompiler) exprTypeInfo(ex *expr) (cTypeInfo, bool) {
	if ex == nil {
		return cTypeInfo{}, false
	}
	switch ex.kind {
	case exprIntLit:
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarInt}, true
	case exprFloatLit:
		return ex.typeInfo, true
	case exprStringLit:
		return cTypeInfo{Kind: cDeclPointer, PtrDepth: 1, Base: cScalarChar}, true
	case exprVar:
		return fc.varTypeInfo(ex.name)
	case exprAssign:
		return fc.exprTypeInfo(ex.left)
	case exprComma:
		return fc.exprTypeInfo(ex.right)
	case exprConditional:
		if len(ex.args) >= 2 {
			if lt, ok := fc.exprTypeInfo(ex.args[0]); ok {
				if rt, rok := fc.exprTypeInfo(ex.args[1]); rok {
					if cTypeInfoEquivalent(lt, rt) {
						return lt, true
					}
					if isFloatTypeInfo(lt) || isFloatTypeInfo(rt) {
						if lt.Base == cScalarDouble || rt.Base == cScalarDouble {
							return cTypeInfo{Kind: cDeclScalar, Base: cScalarDouble}, true
						}
						return cTypeInfo{Kind: cDeclScalar, Base: cScalarFloat}, true
					}
				}
				return lt, true
			}
			if rt, ok := fc.exprTypeInfo(ex.args[1]); ok {
				return rt, true
			}
		}
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarInt}, true
	case exprUnary:
		switch ex.op {
		case "&":
			if t, ok := fc.exprTypeInfo(ex.left); ok {
				if t.Kind == cDeclPointer {
					t.PtrDepth++
				} else if t.Kind == cDeclArray {
					t = decayArrayTypeInfo(t)
				} else {
					t.Kind = cDeclPointer
					t.PtrDepth = 1
				}
				return t, true
			}
			return cTypeInfo{Kind: cDeclPointer, PtrDepth: 1, Base: cScalarInt}, true
		case "*":
			if t, ok := fc.exprTypeInfo(ex.left); ok {
				if t.Kind == cDeclArray {
					return arrayElementTypeInfo(t), true
				}
				if t.Kind == cDeclPointer && t.PtrDepth > 1 {
					t.PtrDepth--
					return t, true
				}
				if t.Kind == cDeclPointer {
					if t.FuncSig != nil {
						// Function pointer dereference yields a function designator.
						return t, true
					}
					t.Kind = cDeclScalar
					t.PtrDepth = 0
					t.ArrayLen = 0
					t.FuncSig = nil
					return t, true
				}
			}
			return cTypeInfo{Kind: cDeclScalar, Base: cScalarInt}, true
		case "++", "--":
			return fc.exprTypeInfo(ex.left)
		case "+", "-":
			if t, ok := fc.exprTypeInfo(ex.left); ok && isFloatTypeInfo(t) {
				return t, true
			}
			return cTypeInfo{Kind: cDeclScalar, Base: cScalarInt}, true
		default:
			return cTypeInfo{Kind: cDeclScalar, Base: cScalarInt}, true
		}
	case exprPostfix:
		if ex.op == "++" || ex.op == "--" {
			return fc.exprTypeInfo(ex.left)
		}
	case exprBinary:
		if lt, lok := fc.exprTypeInfo(ex.left); lok {
			if rt, rok := fc.exprTypeInfo(ex.right); rok {
				if isFloatTypeInfo(lt) || isFloatTypeInfo(rt) {
					switch ex.op {
					case "+", "-", "*", "/":
						if lt.Base == cScalarDouble || rt.Base == cScalarDouble {
							return cTypeInfo{Kind: cDeclScalar, Base: cScalarDouble}, true
						}
						return cTypeInfo{Kind: cDeclScalar, Base: cScalarFloat}, true
					case "==", "!=", "<", "<=", ">", ">=":
						return cTypeInfo{Kind: cDeclScalar, Base: cScalarInt}, true
					}
				}
			}
		}
		if ex.op == "+" || ex.op == "-" {
			if t, ok := fc.exprTypeInfo(ex.left); ok && (t.Kind == cDeclPointer || t.Kind == cDeclArray) {
				if t.Kind == cDeclArray {
					t = decayArrayTypeInfo(t)
				}
				return t, true
			}
			if t, ok := fc.exprTypeInfo(ex.right); ok && (t.Kind == cDeclPointer || t.Kind == cDeclArray) {
				if t.Kind == cDeclArray {
					t = decayArrayTypeInfo(t)
				}
				return t, true
			}
		}
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarInt}, true
	case exprIndex:
		if t, ok := fc.exprTypeInfo(ex.left); ok {
			if t.Kind == cDeclPointer && t.PtrDepth > 1 {
				t.PtrDepth--
				return t, true
			}
			if t.Kind == cDeclArray {
				return arrayElementTypeInfo(t), true
			}
			if t.Kind == cDeclPointer {
				t.Kind = cDeclScalar
				t.PtrDepth = 0
				t.ArrayLen = 0
				return t, true
			}
		}
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarInt}, true
	case exprMember:
		if field, ok := fc.resolveMemberField(ex, false); ok {
			out := field.Type
			out.FuncSig = cloneFuncTypeSig(out.FuncSig)
			return out, true
		}
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarInt}, true
	case exprCall:
		if name, ok := fc.builtinVariadicCallName(ex); ok {
			if name == "__builtin_va_count" {
				return cTypeInfo{Kind: cDeclScalar, Base: cScalarInt}, true
			}
			if name == "__builtin_va_arg" {
				return cTypeInfo{Kind: cDeclScalar, Base: cScalarULong}, true
			}
		}
		if sig, ok := fc.resolveDirectCallSig(ex); ok {
			if sig.RetCount == 0 && !sig.RetByPtr {
				return cTypeInfo{Kind: cDeclScalar, IsVoid: true}, true
			}
			return cTypeInfo{
				Kind:             sig.RetKind,
				PtrDepth:         sig.RetPtrDepth,
				Base:             sig.RetBase,
				OpaqueAggregate:  sig.RetOpaque,
				AggregateKeyword: sig.RetAggKeyword,
				AggregateTag:     sig.RetAggTag,
			}, true
		}
		if t, ok := fc.exprTypeInfo(ex.left); ok && t.Kind == cDeclPointer && t.PtrDepth == 1 {
			if t.FuncSig != nil {
				if t.FuncSig.RetByPtr {
					return cTypeInfo{
						Kind:             t.FuncSig.RetKind,
						PtrDepth:         t.FuncSig.RetPtrDepth,
						Base:             t.FuncSig.RetBase,
						OpaqueAggregate:  t.FuncSig.RetOpaque,
						AggregateKeyword: t.FuncSig.RetAggKeyword,
						AggregateTag:     t.FuncSig.RetAggTag,
					}, true
				}
				return cTypeInfo{
					Kind:             t.FuncSig.RetKind,
					PtrDepth:         t.FuncSig.RetPtrDepth,
					Base:             t.FuncSig.RetBase,
					IsVoid:           t.FuncSig.RetIsVoid,
					OpaqueAggregate:  t.FuncSig.RetOpaque,
					AggregateKeyword: t.FuncSig.RetAggKeyword,
					AggregateTag:     t.FuncSig.RetAggTag,
				}, true
			}
			return cTypeInfo{Kind: cDeclScalar, Base: t.Base, IsVoid: t.IsVoid}, true
		}
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarInt}, true
	case exprCast:
		return ex.typeInfo, true
	case exprCompoundLit:
		return ex.typeInfo, true
	case exprSizeof:
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarULong}, true
	}
	return cTypeInfo{}, false
}

func (fc *funcCompiler) exprPointerStep(ex *expr) int64 {
	if ex == nil {
		return int64(fc.c.target.PtrSize)
	}
	switch ex.kind {
	case exprStringLit:
		return 1
	case exprVar:
		return fc.pointerStepForVar(ex.name)
	case exprAssign:
		return fc.exprPointerStep(ex.left)
	case exprComma:
		return fc.exprPointerStep(ex.right)
	case exprConditional:
		if len(ex.args) >= 2 {
			if fc.exprIsPointer(ex.args[0]) {
				return fc.exprPointerStep(ex.args[0])
			}
			if fc.exprIsPointer(ex.args[1]) {
				return fc.exprPointerStep(ex.args[1])
			}
		}
	case exprUnary:
		if ex.op == "&" {
			if t, ok := fc.exprTypeInfo(ex.left); ok {
				if t.Kind == cDeclScalar {
					return fc.scalarSize(t.Base)
				}
				return int64(fc.c.target.PtrSize)
			}
			return int64(fc.c.target.PtrSize)
		}
		if ex.op == "++" || ex.op == "--" {
			return fc.exprPointerStep(ex.left)
		}
	case exprPostfix:
		if ex.op == "++" || ex.op == "--" {
			return fc.exprPointerStep(ex.left)
		}
	case exprBinary:
		if ex.op == "+" || ex.op == "-" {
			if fc.exprIsPointer(ex.left) {
				return fc.exprPointerStep(ex.left)
			}
			if fc.exprIsPointer(ex.right) {
				return fc.exprPointerStep(ex.right)
			}
		}
	case exprCast:
		if ex.typeInfo.Kind == cDeclPointer {
			return fc.pointerStepForType(ex.typeInfo)
		}
	case exprCall:
		if sig, ok := fc.resolveDirectCallSig(ex); ok && sig.RetKind == cDeclPointer {
			return fc.pointerStepForType(cTypeInfo{
				Kind:             sig.RetKind,
				PtrDepth:         sig.RetPtrDepth,
				Base:             sig.RetBase,
				IsVoid:           sig.RetCount == 0,
				OpaqueAggregate:  sig.RetOpaque,
				AggregateKeyword: sig.RetAggKeyword,
				AggregateTag:     sig.RetAggTag,
			})
		}
		if t, ok := fc.exprTypeInfo(ex.left); ok && t.Kind == cDeclPointer && t.PtrDepth == 1 && t.FuncSig != nil && t.FuncSig.RetKind == cDeclPointer {
			return fc.pointerStepForType(cTypeInfo{
				Kind:             t.FuncSig.RetKind,
				PtrDepth:         t.FuncSig.RetPtrDepth,
				Base:             t.FuncSig.RetBase,
				IsVoid:           t.FuncSig.RetIsVoid,
				OpaqueAggregate:  t.FuncSig.RetOpaque,
				AggregateKeyword: t.FuncSig.RetAggKeyword,
				AggregateTag:     t.FuncSig.RetAggTag,
			})
		}
	case exprMember:
		if field, ok := fc.resolveMemberField(ex, false); ok && field.Type.Kind == cDeclPointer {
			return fc.pointerStepForType(field.Type)
		}
	}
	if t, ok := fc.exprTypeInfo(ex); ok && (t.Kind == cDeclPointer || t.Kind == cDeclArray) {
		return fc.pointerStepForType(t)
	}
	return int64(fc.c.target.PtrSize)
}

func (fc *funcCompiler) exprDerefWidth(ex *expr) int {
	if ex == nil {
		return fc.c.target.PtrSize
	}
	t, ok := fc.exprTypeInfo(ex)
	if !ok {
		return fc.c.target.PtrSize
	}
	if t.Kind == cDeclArray {
		if t.IsVoid {
			fc.errorf(fc.sig.File, 0, 0, "cannot dereference void array")
			return 1
		}
		return int(fc.typeByteSize(arrayElementTypeInfo(t)))
	}
	if t.Kind != cDeclPointer {
		return fc.c.target.PtrSize
	}
	if t.PtrDepth > 1 {
		return fc.c.target.PtrSize
	}
	if t.IsVoid {
		fc.errorf(fc.sig.File, 0, 0, "cannot dereference void pointer")
		return 1
	}
	if t.OpaqueAggregate {
		if t.AggregateTag != "" {
			fc.errorf(fc.sig.File, 0, 0, "cannot dereference opaque %s %q pointer", t.AggregateKeyword, t.AggregateTag)
		} else if t.AggregateKeyword != "" {
			fc.errorf(fc.sig.File, 0, 0, "cannot dereference opaque %s pointer", t.AggregateKeyword)
		} else {
			fc.errorf(fc.sig.File, 0, 0, "cannot dereference opaque aggregate pointer")
		}
		return 1
	}
	if t.AggregateKeyword != "" && t.AggregateTag != "" {
		if agg, ok := fc.lookupAggregate(t.AggregateKeyword, t.AggregateTag); ok && len(agg.Fields) > 0 {
			return int(agg.Size)
		}
	}
	return int(fc.scalarSize(t.Base))
}

func (fc *funcCompiler) exprLValueWidth(ex *expr) int {
	if ex == nil {
		return fc.c.target.PtrSize
	}
	switch ex.kind {
	case exprVar:
		if t, ok := fc.varTypeInfo(ex.name); ok {
			if t.Kind == cDeclPointer {
				return fc.c.target.PtrSize
			}
			if t.Kind == cDeclArray {
				return fc.c.target.PtrSize
			}
			return int(fc.scalarSize(t.Base))
		}
	case exprUnary:
		if ex.op == "*" {
			return fc.exprDerefWidth(ex.left)
		}
	case exprIndex:
		return fc.exprDerefWidth(ex.left)
	case exprMember:
		if field, ok := fc.resolveMemberField(ex, false); ok {
			if size, _, ok := fc.typeInfoSizeAlign(field.Type); ok {
				return int(size)
			}
		}
		return fc.c.target.PtrSize
	}
	return fc.c.target.PtrSize
}

func (fc *funcCompiler) exprIsPointer(ex *expr) bool {
	t, ok := fc.exprTypeInfo(ex)
	return ok && (t.Kind == cDeclPointer || t.Kind == cDeclArray)
}

func (fc *funcCompiler) sizeofType(info cTypeInfo) int64 {
	if info.IsVoid && info.Kind == cDeclScalar {
		fc.errorf(fc.sig.File, 0, 0, "sizeof(void) is not supported")
		return 1
	}
	if info.OpaqueAggregate && info.Kind != cDeclPointer {
		if info.AggregateTag != "" {
			fc.errorf(fc.sig.File, 0, 0, "sizeof on opaque %s %q is not supported", info.AggregateKeyword, info.AggregateTag)
		} else if info.AggregateKeyword != "" {
			fc.errorf(fc.sig.File, 0, 0, "sizeof on opaque %s is not supported", info.AggregateKeyword)
		} else {
			fc.errorf(fc.sig.File, 0, 0, "sizeof on opaque aggregate is not supported")
		}
		return 1
	}
	return fc.typeByteSize(info)
}

func (fc *funcCompiler) sizeofExpr(ex *expr) int64 {
	if ex == nil {
		return int64(fc.c.target.PtrSize)
	}
	switch ex.kind {
	case exprStringLit:
		return int64(len(ex.strVal) + 1)
	case exprCast:
		return fc.sizeofType(ex.typeInfo)
	}
	if t, ok := fc.exprTypeInfo(ex); ok {
		if t.IsVoid && t.Kind == cDeclScalar {
			fc.errorf(fc.sig.File, 0, 0, "sizeof applied to void expression")
			return 1
		}
		return fc.typeByteSize(t)
	}
	return int64(fc.c.target.PtrSize)
}

func (fc *funcCompiler) isNullPointerLiteral(ex *expr) bool {
	return ex != nil && ex.kind == exprIntLit && ex.intVal == 0
}

func funcTypeSigEqual(a *cFuncTypeSig, b *cFuncTypeSig) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.RetKind != b.RetKind ||
		a.RetBase != b.RetBase ||
		a.RetPtrDepth != b.RetPtrDepth ||
		a.RetIsVoid != b.RetIsVoid ||
		a.RetByPtr != b.RetByPtr ||
		a.RetOpaque != b.RetOpaque ||
		a.RetAggKeyword != b.RetAggKeyword ||
		a.RetAggTag != b.RetAggTag {
		return false
	}
	if a.ParamUnspecified || b.ParamUnspecified {
		return true
	}
	if a.ParamCount != b.ParamCount {
		return false
	}
	if a.Variadic != b.Variadic {
		return false
	}
	for i := 0; i < a.ParamCount; i++ {
		if i >= len(a.ParamKinds) || i >= len(b.ParamKinds) || a.ParamKinds[i] != b.ParamKinds[i] {
			return false
		}
		if i >= len(a.ParamBases) || i >= len(b.ParamBases) || a.ParamBases[i] != b.ParamBases[i] {
			return false
		}
		if i >= len(a.ParamPtrDepth) || i >= len(b.ParamPtrDepth) || a.ParamPtrDepth[i] != b.ParamPtrDepth[i] {
			return false
		}
		if i >= len(a.ParamOpaque) || i >= len(b.ParamOpaque) || a.ParamOpaque[i] != b.ParamOpaque[i] {
			return false
		}
		if i >= len(a.ParamAggKey) || i >= len(b.ParamAggKey) || a.ParamAggKey[i] != b.ParamAggKey[i] {
			return false
		}
		if i >= len(a.ParamAggTag) || i >= len(b.ParamAggTag) || a.ParamAggTag[i] != b.ParamAggTag[i] {
			return false
		}
		var af, bf *cFuncTypeSig
		if i < len(a.ParamFuncSigs) {
			af = a.ParamFuncSigs[i]
		}
		if i < len(b.ParamFuncSigs) {
			bf = b.ParamFuncSigs[i]
		}
		if !funcTypeSigEqual(af, bf) {
			return false
		}
	}
	return true
}

func funcSigMatchesType(sig *cFuncSig, want *cFuncTypeSig) bool {
	if sig == nil || want == nil {
		return false
	}
	if want.RetKind != sig.RetKind ||
		want.RetBase != sig.RetBase ||
		want.RetPtrDepth != sig.RetPtrDepth ||
		want.RetIsVoid != (sig.RetCount == 0 && !sig.RetByPtr) ||
		want.RetByPtr != sig.RetByPtr ||
		want.RetOpaque != sig.RetOpaque ||
		want.RetAggKeyword != sig.RetAggKeyword ||
		want.RetAggTag != sig.RetAggTag {
		return false
	}
	if want.ParamUnspecified || sig.ParamUnspecified {
		return true
	}
	if want.ParamCount != sig.ParamCount {
		return false
	}
	if want.Variadic != sig.Variadic {
		return false
	}
	for i := 0; i < want.ParamCount; i++ {
		if i >= len(sig.ParamKinds) || want.ParamKinds[i] != sig.ParamKinds[i] {
			return false
		}
		if i >= len(sig.ParamBases) || want.ParamBases[i] != sig.ParamBases[i] {
			return false
		}
		if i >= len(sig.ParamPtrDepth) || want.ParamPtrDepth[i] != sig.ParamPtrDepth[i] {
			return false
		}
		if i >= len(sig.ParamOpaque) || want.ParamOpaque[i] != sig.ParamOpaque[i] {
			return false
		}
		if i >= len(sig.ParamAggKey) || want.ParamAggKey[i] != sig.ParamAggKey[i] {
			return false
		}
		if i >= len(sig.ParamAggTag) || want.ParamAggTag[i] != sig.ParamAggTag[i] {
			return false
		}
		var wf, sf *cFuncTypeSig
		if i < len(want.ParamFuncSigs) {
			wf = want.ParamFuncSigs[i]
		}
		if i < len(sig.ParamFuncSigs) {
			sf = sig.ParamFuncSigs[i]
		}
		if !funcTypeSigEqual(wf, sf) {
			return false
		}
	}
	return true
}

func (fc *funcCompiler) checkCallArgsByType(calleeName string, paramCount int, paramUnspecified bool, variadic bool, paramKinds []cDeclKind, paramFuncSigs []*cFuncTypeSig, call *expr) bool {
	if call == nil {
		return false
	}
	if paramUnspecified {
		return true
	}
	if !variadic && len(call.args) != paramCount {
		fc.errorf(fc.sig.File, 0, 0, "call to %q has %d arguments; expected %d", calleeName, len(call.args), paramCount)
		return false
	}
	if variadic && len(call.args) < paramCount {
		fc.errorf(fc.sig.File, 0, 0, "call to %q has %d arguments; expected at least %d", calleeName, len(call.args), paramCount)
		return false
	}
	ok := true
	for i, arg := range call.args {
		if i >= paramCount {
			// Variadic tail: accept scalar/pointer values as-is.
			continue
		}
		want := cDeclScalar
		if i < len(paramKinds) {
			want = paramKinds[i]
		}
		var wantFn *cFuncTypeSig
		if i < len(paramFuncSigs) {
			wantFn = paramFuncSigs[i]
		}
		gotPtr := fc.exprIsPointer(arg)
		if want == cDeclPointer {
			if gotPtr || fc.isNullPointerLiteral(arg) {
				if wantFn == nil || fc.isNullPointerLiteral(arg) {
					continue
				}
				at, aok := fc.exprTypeInfo(arg)
				if !aok || at.Kind != cDeclPointer || at.FuncSig == nil || !funcTypeSigEqual(at.FuncSig, wantFn) {
					fc.errorf(fc.sig.File, 0, 0, "argument %d of %q expects compatible function pointer", i+1, calleeName)
					ok = false
				}
				continue
			}
			if _, ok := fc.exprTypeInfo(arg); ok {
				// Preserve old C compatibility for integer-to-pointer calls;
				// lowering keeps both as plain machine values.
				continue
			}
			fc.errorf(fc.sig.File, 0, 0, "argument %d of %q expects pointer value", i+1, calleeName)
			ok = false
			continue
		}
		if i < paramCount {
			if got, ok := fc.exprTypeInfo(arg); ok && isAggregateObjectType(got) {
				continue
			}
		}
		if gotPtr {
			// Preserve old C compatibility for pointer-to-integer parameter
			// calls; later lowering keeps both as plain machine values.
			continue
		}
	}
	return ok
}

func (fc *funcCompiler) checkCallArgs(sig *cFuncSig, call *expr) bool {
	if sig == nil || call == nil {
		return false
	}
	if !sig.Defined {
		return true
	}
	return fc.checkCallArgsByType(sig.Name, sig.ParamCount, sig.ParamUnspecified, sig.Variadic, sig.ParamKinds, sig.ParamFuncSigs, call)
}

func (fc *funcCompiler) callArgTargetType(sig *cFuncSig, argIdx int) (cTypeInfo, bool) {
	if sig == nil || argIdx < 0 {
		return cTypeInfo{}, false
	}
	if sig.ParamUnspecified {
		return cTypeInfo{}, false
	}
	if argIdx < sig.ParamCount {
		kind := cDeclScalar
		if argIdx < len(sig.ParamKinds) {
			kind = sig.ParamKinds[argIdx]
		}
		info := cTypeInfo{Kind: kind, Base: cScalarInt}
		if argIdx < len(sig.ParamBases) {
			info.Base = sig.ParamBases[argIdx]
		}
		if argIdx < len(sig.ParamPtrDepth) {
			info.PtrDepth = sig.ParamPtrDepth[argIdx]
		}
		if argIdx < len(sig.ParamOpaque) {
			info.OpaqueAggregate = sig.ParamOpaque[argIdx]
		}
		if argIdx < len(sig.ParamAggKey) {
			info.AggregateKeyword = sig.ParamAggKey[argIdx]
		}
		if argIdx < len(sig.ParamAggTag) {
			info.AggregateTag = sig.ParamAggTag[argIdx]
		}
		if argIdx < len(sig.ParamFuncSigs) {
			info.FuncSig = cloneFuncTypeSig(sig.ParamFuncSigs[argIdx])
		}
		if info.Kind == cDeclArray {
			info.Kind = cDeclPointer
			if info.PtrDepth == 0 {
				info.PtrDepth = 1
			}
		}
		return info, true
	}
	if !sig.Variadic {
		return cTypeInfo{}, false
	}
	return cTypeInfo{}, false
}

func (fc *funcCompiler) callArgRuntimeType(sig *cFuncSig, argIdx int) (cTypeInfo, bool) {
	target, ok := fc.callArgTargetType(sig, argIdx)
	if !ok {
		return cTypeInfo{}, false
	}
	if isAggregateObjectType(target) {
		return cTypeInfo{Kind: cDeclPointer, PtrDepth: 1, Base: cScalarInt}, true
	}
	return target, true
}

func (fc *funcCompiler) emitCallArgValue(sig *cFuncSig, arg *expr, argIdx int) {
	if target, ok := fc.callArgTargetType(sig, argIdx); ok {
		if isAggregateObjectType(target) {
			if !fc.emitAggregateExprAddress(arg) {
				fc.errorf(fc.sig.File, 0, 0, "aggregate argument %d must be addressable", argIdx+1)
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			}
			return
		}
		fc.emitExprValueCast(arg, target)
		return
	}
	if sig != nil && sig.Variadic {
		if argInfo, ok := fc.exprTypeInfo(arg); ok && argInfo.Kind == cDeclScalar {
			if argInfo.Base == cScalarFloat {
				fc.emitExprValueCast(arg, cTypeInfo{Kind: cDeclScalar, Base: cScalarDouble})
				return
			}
			if argInfo.Base == cScalarChar || argInfo.Base == cScalarUChar || argInfo.Base == cScalarShort || argInfo.Base == cScalarUShort {
				fc.emitExprValueCast(arg, cTypeInfo{Kind: cDeclScalar, Base: cScalarInt})
				return
			}
		}
	}
	fc.emitExpr(arg)
}

func (fc *funcCompiler) callArgValueType(sig *cFuncSig, arg *expr, argIdx int) (cTypeInfo, bool) {
	if target, ok := fc.callArgRuntimeType(sig, argIdx); ok {
		return target, true
	}
	if sig != nil && sig.Variadic {
		if argInfo, ok := fc.exprTypeInfo(arg); ok && argInfo.Kind == cDeclScalar {
			if argInfo.Base == cScalarFloat {
				return cTypeInfo{Kind: cDeclScalar, Base: cScalarDouble}, true
			}
			if argInfo.Base == cScalarChar || argInfo.Base == cScalarUChar || argInfo.Base == cScalarShort || argInfo.Base == cScalarUShort {
				return cTypeInfo{Kind: cDeclScalar, Base: cScalarInt}, true
			}
		}
	}
	return fc.exprTypeInfo(arg)
}

func (fc *funcCompiler) allocTempLocal(name string) int {
	return fc.addLocal(fmt.Sprintf("%s$%d", name, fc.c.nextLabel()), fc.sig.File, 0, 0)
}

func (fc *funcCompiler) emitCopyAggregateBytes(dstAddrLocal int, srcAddrLocal int, size int64) {
	offset := int64(0)
	for offset < size {
		chunk := int64(fc.c.target.PtrSize)
		remain := size - offset
		if chunk > remain {
			switch {
			case remain >= 4:
				chunk = 4
			case remain >= 2:
				chunk = 2
			default:
				chunk = 1
			}
		}
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: srcAddrLocal})
		if offset != 0 {
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: offset})
			fc.emit(ir.Inst{Op: ir.OP_ADD})
		}
		fc.emit(ir.Inst{Op: ir.OP_LOAD, Arg: int(chunk)})
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: dstAddrLocal})
		if offset != 0 {
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: offset})
			fc.emit(ir.Inst{Op: ir.OP_ADD})
		}
		fc.emit(ir.Inst{Op: ir.OP_STORE, Arg: int(chunk)})
		offset += chunk
	}
}

func (fc *funcCompiler) allocLocalAggregateTemp(info cTypeInfo, name string) int {
	idx := fc.addLocalDecl(name, info.Kind, info.Base, info.PtrDepth, int64(fc.c.target.PtrSize), 0, nil, cloneFuncTypeSig(info.FuncSig), info.OpaqueAggregate, info.AggregateKeyword, info.AggregateTag, fc.sig.File, 0, 0)
	fc.initLocalAggregateObject(name, idx, info, nil, fc.sig.File, 0, 0)
	return idx
}

func (fc *funcCompiler) emitAggregateCallAddress(sig *cFuncSig, ex *expr) bool {
	if sig == nil || ex == nil || !sig.RetByPtr {
		return false
	}
	if sig.Variadic {
		fc.errorf(fc.sig.File, 0, 0, "aggregate return from variadic functions is not yet supported")
		return false
	}
	if !fc.checkCallArgs(sig, ex) {
		return false
	}
	if !sig.Defined {
		fc.errorf(fc.sig.File, 0, 0, "aggregate return from external function %q is not yet supported", sig.Name)
		return false
	}
	retInfo := cTypeInfo{
		Kind:             sig.RetKind,
		PtrDepth:         sig.RetPtrDepth,
		Base:             sig.RetBase,
		OpaqueAggregate:  sig.RetOpaque,
		AggregateKeyword: sig.RetAggKeyword,
		AggregateTag:     sig.RetAggTag,
	}
	tmpIdx := fc.allocLocalAggregateTemp(retInfo, fmt.Sprintf("$call$ret$%d", fc.c.nextLabel()))
	fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: tmpIdx})
	for i, a := range ex.args {
		fc.emitCallArgValue(sig, a, i)
	}
	fc.emit(ir.Inst{Op: ir.OP_CALL, Name: sig.IRName, Arg: sig.ParamCount + 1})
	fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: tmpIdx})
	return true
}

func (fc *funcCompiler) emitAggregateExprAddress(ex *expr) bool {
	if ex == nil {
		return false
	}
	switch ex.kind {
	case exprComma:
		return fc.emitAggregateExprAddress(ex.right)
	case exprAssign:
		if info, ok := fc.exprTypeInfo(ex); ok && isAggregateObjectType(info) {
			fc.emitExpr(ex)
			return true
		}
		return false
	case exprConditional:
		info, ok := fc.exprTypeInfo(ex)
		if !ok || !isAggregateObjectType(info) || len(ex.args) < 2 {
			return false
		}
		tmpIdx := fc.allocLocalAggregateTemp(info, fmt.Sprintf("$cond$ret$%d", fc.c.nextLabel()))
		dstTmp := fc.allocTempLocal("$cond_dst")
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: tmpIdx})
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: dstTmp})
		falseLabel := fc.c.nextLabel()
		endLabel := fc.c.nextLabel()
		fc.emitConditionValue(ex.left)
		fc.emit(ir.Inst{Op: ir.OP_JMP_IF_NOT, Arg: falseLabel})
		srcTmp := fc.allocTempLocal("$cond_src")
		if fc.emitAggregateExprAddress(ex.args[0]) {
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: srcTmp})
			fc.emitCopyAggregateBytes(dstTmp, srcTmp, fc.typeByteSize(info))
		} else {
			fc.errorf(fc.sig.File, 0, 0, "aggregate conditional expression requires aggregate-valued true branch")
		}
		fc.emit(ir.Inst{Op: ir.OP_JMP, Arg: endLabel})
		fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: falseLabel})
		if fc.emitAggregateExprAddress(ex.args[1]) {
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: srcTmp})
			fc.emitCopyAggregateBytes(dstTmp, srcTmp, fc.typeByteSize(info))
		} else {
			fc.errorf(fc.sig.File, 0, 0, "aggregate conditional expression requires aggregate-valued false branch")
		}
		fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: endLabel})
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: tmpIdx})
		return true
	case exprCall:
		if sig, ok := fc.resolveDirectCallSig(ex); ok && sig.RetByPtr {
			return fc.emitAggregateCallAddress(sig, ex)
		}
	}
	return fc.emitAddressOf(ex)
}

func (fc *funcCompiler) emitUpdateExpr(ex *expr, step int64, op string, postfix bool) {
	if ex == nil {
		fc.errorf(fc.sig.File, 0, 0, "%s requires assignable operand", op)
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		return
	}
	if t, ok := fc.exprTypeInfo(ex); ok && isAggregateObjectType(t) {
		fc.errorf(fc.sig.File, 0, 0, "%s on aggregate object is not yet supported", op)
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		return
	}
	addrTmp := fc.allocTempLocal("$lvalue_addr")
	if !fc.emitAddressOf(ex) {
		fc.errorf(fc.sig.File, 0, 0, "%s requires assignable operand", op)
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		return
	}
	fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: addrTmp})
	width := fc.exprLValueWidth(ex)
	valInfo, _ := fc.exprTypeInfo(ex)
	if postfix {
		oldTmp := fc.allocTempLocalForType("$lvalue_old", valInfo)
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: addrTmp})
		fc.emitLoadForType(width, valInfo)
		fc.emit(ir.Inst{Op: ir.OP_DUP})
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: oldTmp})
		if isFloatTypeInfo(valInfo) {
			if valInfo.Base == cScalarFloat {
				fc.emit(ir.Inst{Op: ir.OP_CONST_F32, Width: 4, Name: "1.0"})
			} else {
				fc.emit(ir.Inst{Op: ir.OP_CONST_F64, Width: 8, Name: "1.0"})
			}
			fc.emitTypedBinaryInst(op[:1], valInfo)
		} else {
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: step})
			if op == "++" {
				fc.emit(ir.Inst{Op: ir.OP_ADD})
			} else {
				fc.emit(ir.Inst{Op: ir.OP_SUB})
			}
		}
		fc.emitCastValueToType(valInfo, valInfo)
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: addrTmp})
		fc.emitStoreForType(width, valInfo)
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: oldTmp})
		return
	}
	fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: addrTmp})
	fc.emitLoadForType(width, valInfo)
	if isFloatTypeInfo(valInfo) {
		if valInfo.Base == cScalarFloat {
			fc.emit(ir.Inst{Op: ir.OP_CONST_F32, Width: 4, Name: "1.0"})
		} else {
			fc.emit(ir.Inst{Op: ir.OP_CONST_F64, Width: 8, Name: "1.0"})
		}
		fc.emitTypedBinaryInst(op[:1], valInfo)
	} else {
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: step})
		if op == "++" {
			fc.emit(ir.Inst{Op: ir.OP_ADD})
		} else {
			fc.emit(ir.Inst{Op: ir.OP_SUB})
		}
	}
	fc.emitCastValueToType(valInfo, valInfo)
	fc.emit(ir.Inst{Op: ir.OP_DUP})
	fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: addrTmp})
	fc.emitStoreForType(width, valInfo)
}

func (fc *funcCompiler) emitExpr(ex *expr) {
	if ex == nil {
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		return
	}
	switch ex.kind {
	case exprIntLit:
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: ex.intVal})
	case exprFloatLit:
		if ex.typeInfo.Base == cScalarFloat {
			fc.emit(ir.Inst{Op: ir.OP_CONST_F32, Width: 4, Name: ex.floatVal})
		} else {
			fc.emit(ir.Inst{Op: ir.OP_CONST_F64, Width: 8, Name: ex.floatVal})
		}
	case exprStringLit:
		// C string literals are NUL-terminated arrays that decay to pointers.
		fc.emit(ir.Inst{Op: ir.OP_CONST_STR, Name: encodeIRStringLiteral(ex.strVal + "\x00")})
		stringPtr := fc.c.ensureIntrinsicWrapper("Stringptr", 1, 1)
		fc.emit(ir.Inst{Op: ir.OP_CALL, Name: stringPtr, Arg: 1})
	case exprVar:
		if t, ok := fc.varTypeInfo(ex.name); ok && isAggregateObjectType(t) {
			fc.errorf(fc.sig.File, 0, 0, "aggregate-valued expression for %q is not yet supported", ex.name)
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			return
		}
		if idx, ok := fc.lookupLocal(ex.name); ok {
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: idx})
			return
		}
		if fc.c.canLoadExternDataOnTarget(ex.name) {
			wrap := fc.c.ensureExternDataWrapper(ex.name, false)
			if wrap != "" {
				fc.emit(ir.Inst{Op: ir.OP_CALL, Name: wrap, Arg: 0})
				return
			}
		}
		if idx, ok := fc.lookupGlobal(ex.name); ok {
			fc.emit(ir.Inst{Op: ir.OP_GLOBAL_GET, Arg: idx})
			return
		}
		if v, ok := fc.lookupEnumConst(ex.name); ok {
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: v})
			return
		}
		if _, ok := fc.c.funcs[ex.name]; ok {
			if _, haveID := fc.c.funcIDs[ex.name]; !haveID {
				fc.c.assignFunctionIDs()
			}
			if id, ok := fc.c.funcIDs[ex.name]; ok {
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: id})
				return
			}
		}
		if id, ok := fc.c.funcIDs[ex.name]; ok {
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: id})
			return
		}
		fc.errorf(fc.sig.File, 0, 0, "unknown identifier %q", ex.name)
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
	case exprAssign:
		if ex.left == nil {
			fc.errorf(fc.sig.File, 0, 0, "left-hand side of assignment must be assignable")
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			return
		}
		if t, ok := fc.exprTypeInfo(ex.left); ok && isAggregateObjectType(t) {
			addrTmp := fc.allocTempLocal("$assign_addr")
			if !fc.emitAddressOf(ex.left) {
				fc.errorf(fc.sig.File, 0, 0, "left-hand side of assignment is not assignable")
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				return
			}
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: addrTmp})
			srcTmp := fc.allocTempLocal("$assign_src")
			if !fc.emitAggregateExprAddress(ex.right) {
				fc.errorf(fc.sig.File, 0, 0, "aggregate assignment requires aggregate-valued right-hand side")
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				return
			}
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: srcTmp})
			fc.emitCopyAggregateBytes(addrTmp, srcTmp, fc.typeByteSize(t))
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: addrTmp})
			return
		}
		addrTmp := fc.allocTempLocal("$assign_addr")
		if !fc.emitAddressOf(ex.left) {
			fc.errorf(fc.sig.File, 0, 0, "left-hand side of assignment is not assignable")
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			return
		}
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: addrTmp})
		lhsInfo, _ := fc.exprTypeInfo(ex.left)
		resultInfo := lhsInfo
		if ex.op != "" && ex.op != "=" {
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: addrTmp})
			fc.emitLoadForType(fc.exprLValueWidth(ex.left), lhsInfo)
			rhsInfo, _ := fc.exprTypeInfo(ex.right)
			if isFloatTypeInfo(lhsInfo) || isFloatTypeInfo(rhsInfo) {
				fc.emitExprValueCast(ex.right, lhsInfo)
				switch ex.op {
				case "+=":
					fc.emitTypedBinaryInst("+", lhsInfo)
				case "-=":
					fc.emitTypedBinaryInst("-", lhsInfo)
				case "*=":
					fc.emitTypedBinaryInst("*", lhsInfo)
				case "/=":
					fc.emitTypedBinaryInst("/", lhsInfo)
				default:
					fc.errorf(fc.sig.File, 0, 0, "unsupported floating-point assignment operator %q", ex.op)
				}
			} else {
				fc.emitExpr(ex.right)
				switch ex.op {
				case "+=":
					fc.emit(ir.Inst{Op: ir.OP_ADD})
				case "-=":
					fc.emit(ir.Inst{Op: ir.OP_SUB})
				case "*=":
					fc.emit(ir.Inst{Op: ir.OP_MUL})
				case "/=":
					fc.emit(ir.Inst{Op: ir.OP_DIV})
				case "%=":
					fc.emit(ir.Inst{Op: ir.OP_MOD})
				case "&=":
					fc.emit(ir.Inst{Op: ir.OP_AND})
				case "^=":
					fc.emit(ir.Inst{Op: ir.OP_XOR})
				case "|=":
					fc.emit(ir.Inst{Op: ir.OP_OR})
				case "<<=":
					fc.emit(ir.Inst{Op: ir.OP_SHL})
				case ">>=":
					fc.emit(ir.Inst{Op: ir.OP_SHR})
				default:
					fc.errorf(fc.sig.File, 0, 0, "unsupported assignment operator %q", ex.op)
				}
			}
		} else {
			fc.emitExpr(ex.right)
			if rhsInfo, ok := fc.exprTypeInfo(ex.right); ok {
				resultInfo = rhsInfo
			}
		}
		if ex.op != "" && ex.op != "=" {
			fc.emitCastValueToType(resultInfo, lhsInfo)
		} else if rhsInfo, ok := fc.exprTypeInfo(ex.right); ok {
			fc.emitCastValueToType(rhsInfo, lhsInfo)
		} else {
			fc.emitCastToType(lhsInfo)
		}
		fc.emit(ir.Inst{Op: ir.OP_DUP})
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: addrTmp})
		fc.emitStoreForType(fc.exprLValueWidth(ex.left), lhsInfo)
	case exprComma:
		fc.emitExpr(ex.left)
		fc.emit(ir.Inst{Op: ir.OP_DROP})
		fc.emitExpr(ex.right)
	case exprConditional:
		if len(ex.args) < 2 {
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			return
		}
		falseLabel := fc.c.nextLabel()
		endLabel := fc.c.nextLabel()
		fc.emitConditionValue(ex.left)
		fc.emit(ir.Inst{Op: ir.OP_JMP_IF_NOT, Arg: falseLabel})
		if t, ok := fc.exprTypeInfo(ex); ok {
			fc.emitExprValueCast(ex.args[0], t)
		} else {
			fc.emitExpr(ex.args[0])
		}
		fc.emit(ir.Inst{Op: ir.OP_JMP, Arg: endLabel})
		fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: falseLabel})
		if t, ok := fc.exprTypeInfo(ex); ok {
			fc.emitExprValueCast(ex.args[1], t)
		} else {
			fc.emitExpr(ex.args[1])
		}
		fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: endLabel})
	case exprUnary:
		if ex.op == "++" || ex.op == "--" {
			step := int64(1)
			if fc.exprIsPointer(ex.left) {
				step = fc.exprPointerStep(ex.left)
			}
			fc.emitUpdateExpr(ex.left, step, ex.op, false)
			return
		}
		if ex.op == "&" {
			// &*x simplifies to x; otherwise take address of lvalue.
			if ex.left != nil && ex.left.kind == exprUnary && ex.left.op == "*" {
				fc.emitExpr(ex.left.left)
				break
			}
			if !fc.emitAddressOf(ex.left) {
				fc.errorf(fc.sig.File, 0, 0, "cannot take address of expression")
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			}
			break
		}
		fc.emitExpr(ex.left)
		switch ex.op {
		case "+":
			// no-op
		case "-":
			if t, ok := fc.exprTypeInfo(ex.left); ok && isFloatTypeInfo(t) {
				fc.emit(ir.Inst{Op: ir.OP_NEG, Width: int(fc.typeByteSize(t)), Name: fc.convertNameForScalar(t.Base)})
			} else {
				fc.emit(ir.Inst{Op: ir.OP_NEG})
			}
		case "*":
			if t, ok := fc.exprTypeInfo(ex.left); ok && t.Kind == cDeclPointer && t.PtrDepth == 1 && t.FuncSig != nil {
				// Function pointer dereference in value context remains a function designator.
				break
			}
			outInfo, _ := fc.exprTypeInfo(ex)
			fc.emitLoadForType(fc.exprDerefWidth(ex.left), outInfo)
		case "!":
			if t, ok := fc.exprTypeInfo(ex.left); ok && isFloatTypeInfo(t) {
				fc.emitZeroValue(t)
				fc.emitTypedBinaryInst("==", t)
			} else {
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				fc.emit(ir.Inst{Op: ir.OP_EQ})
			}
		case "~":
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: -1})
			fc.emit(ir.Inst{Op: ir.OP_XOR})
		default:
			fc.errorf(fc.sig.File, 0, 0, "unsupported unary operator %q", ex.op)
		}
	case exprPostfix:
		step := int64(1)
		if fc.exprIsPointer(ex.left) {
			step = fc.exprPointerStep(ex.left)
		}
		fc.emitUpdateExpr(ex.left, step, ex.op, true)
	case exprBinary:
		if ex.op == "&&" || ex.op == "||" {
			fc.emitLogicalExpr(ex)
			return
		}
		leftInfo, _ := fc.exprTypeInfo(ex.left)
		rightInfo, _ := fc.exprTypeInfo(ex.right)
		if ex.op == "+" || ex.op == "-" {
			leftPtr := fc.exprIsPointer(ex.left)
			rightPtr := fc.exprIsPointer(ex.right)
			if ex.op == "-" && leftPtr && rightPtr {
				step := fc.exprPointerStep(ex.left)
				fc.emitExpr(ex.left)
				fc.emitExpr(ex.right)
				fc.emit(ir.Inst{Op: ir.OP_SUB})
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: step})
				fc.emit(ir.Inst{Op: ir.OP_DIV})
				return
			}
			if leftPtr && !rightPtr {
				step := fc.exprPointerStep(ex.left)
				fc.emitExpr(ex.left)
				fc.emitExpr(ex.right)
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: step})
				fc.emit(ir.Inst{Op: ir.OP_MUL})
				if ex.op == "+" {
					fc.emit(ir.Inst{Op: ir.OP_ADD})
				} else {
					fc.emit(ir.Inst{Op: ir.OP_SUB})
				}
				return
			}
			if ex.op == "+" && !leftPtr && rightPtr {
				step := fc.exprPointerStep(ex.right)
				fc.emitExpr(ex.left)
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: step})
				fc.emit(ir.Inst{Op: ir.OP_MUL})
				fc.emitExpr(ex.right)
				fc.emit(ir.Inst{Op: ir.OP_ADD})
				return
			}
		}
		if isFloatTypeInfo(leftInfo) || isFloatTypeInfo(rightInfo) {
			floatInfo := cTypeInfo{Kind: cDeclScalar, Base: cScalarDouble}
			if leftInfo.Base != cScalarDouble && rightInfo.Base != cScalarDouble {
				floatInfo.Base = cScalarFloat
			}
			switch ex.op {
			case "+", "-", "*", "/", "==", "!=", "<", "<=", ">", ">=":
				fc.emitExprValueCast(ex.left, floatInfo)
				fc.emitExprValueCast(ex.right, floatInfo)
				fc.emitTypedBinaryInst(ex.op, floatInfo)
			default:
				fc.errorf(fc.sig.File, 0, 0, "unsupported floating-point binary operator %q", ex.op)
				fc.emitZeroValue(floatInfo)
			}
			return
		}
		fc.emitExpr(ex.left)
		fc.emitExpr(ex.right)
		switch ex.op {
		case "+":
			fc.emit(ir.Inst{Op: ir.OP_ADD})
		case "-":
			fc.emit(ir.Inst{Op: ir.OP_SUB})
		case "*":
			fc.emit(ir.Inst{Op: ir.OP_MUL})
		case "/":
			fc.emit(ir.Inst{Op: ir.OP_DIV})
		case "%":
			fc.emit(ir.Inst{Op: ir.OP_MOD})
		case "==":
			fc.emit(ir.Inst{Op: ir.OP_EQ})
		case "!=":
			fc.emit(ir.Inst{Op: ir.OP_NEQ})
		case "<":
			fc.emit(ir.Inst{Op: ir.OP_LT})
		case "<=":
			fc.emit(ir.Inst{Op: ir.OP_LEQ})
		case ">":
			fc.emit(ir.Inst{Op: ir.OP_GT})
		case ">=":
			fc.emit(ir.Inst{Op: ir.OP_GEQ})
		case "&":
			fc.emit(ir.Inst{Op: ir.OP_AND})
		case "|":
			fc.emit(ir.Inst{Op: ir.OP_OR})
		case "^":
			fc.emit(ir.Inst{Op: ir.OP_XOR})
		case "<<":
			fc.emit(ir.Inst{Op: ir.OP_SHL})
		case ">>":
			fc.emit(ir.Inst{Op: ir.OP_SHR})
		default:
			fc.errorf(fc.sig.File, 0, 0, "unsupported binary operator %q", ex.op)
		}
	case exprIndex:
		fc.emitIndexAddr(ex.left, ex.right)
		if t, ok := fc.exprTypeInfo(ex); ok && t.Kind == cDeclArray {
			return
		}
		outInfo, _ := fc.exprTypeInfo(ex)
		fc.emitLoadForType(fc.exprDerefWidth(ex.left), outInfo)
	case exprMember:
		field, ok := fc.resolveMemberField(ex, true)
		if !ok || !fc.emitMemberAddress(ex, true) {
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			break
		}
		if field.Type.Kind == cDeclArray {
			return
		}
		if field.Type.Kind == cDeclScalar && field.Type.AggregateKeyword != "" && field.Type.AggregateTag != "" && field.Type.PtrDepth == 0 {
			fc.errorf(fc.sig.File, 0, 0, "aggregate-valued member expression is not yet supported")
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			break
		}
		fc.emitLoadForType(fc.exprLValueWidth(ex), field.Type)
	case exprCall:
		if fc.emitBuiltinVariadicCall(ex) {
			return
		}
		if sig, ok := fc.resolveDirectCallSig(ex); ok {
			if !fc.checkCallArgs(sig, ex) {
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				return
			}
			if sig.RetByPtr {
				if !fc.emitAggregateCallAddress(sig, ex) {
					fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				}
				return
			}
			if sig.Variadic && sig.Defined {
				argLocals := make([]int, 0, len(ex.args))
				i := 0
				for i < len(ex.args) {
					fc.emitCallArgValue(sig, ex.args[i], i)
					argInfo, ok := fc.callArgValueType(sig, ex.args[i], i)
					argTmp := fc.allocTempLocal(fmt.Sprintf("$call_arg$%d$%d", fc.c.nextLabel(), i))
					if ok {
						argTmp = fc.allocTempLocalForType(fmt.Sprintf("$call_arg$%d$%d", fc.c.nextLabel(), i), argInfo)
					}
					fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: argTmp})
					argLocals = append(argLocals, argTmp)
					i++
				}
				i = 0
				for i < sig.ParamCount && i < len(argLocals) {
					fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: argLocals[i]})
					i++
				}
				fc.emitVariadicPackFromLocals(argLocals[sig.ParamCount:])
				fc.emit(ir.Inst{Op: ir.OP_CALL, Name: sig.IRName, Arg: sig.ParamCount + 2})
				if sig.RetCount == 0 {
					fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				}
				return
			}
			callArgCount := sig.ParamCount
			if sig.Variadic && !sig.Defined {
				callArgCount = len(ex.args)
			}
			for i, a := range ex.args {
				fc.emitCallArgValue(sig, a, i)
			}
			if !sig.Defined {
				if fc.c.objectMode() {
					fc.emit(ir.Inst{Op: ir.OP_CALL, Name: sig.IRName, Arg: callArgCount})
					if sig.RetCount == 0 {
						fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
					}
					return
				}
				if fc.c.canCallExternOnTarget(sig.Name, sig.RetCount) {
					wrap := fc.c.ensureExternWrapper(sig.Name, callArgCount, sig.RetCount)
					fc.emit(ir.Inst{Op: ir.OP_CALL, Name: wrap, Arg: callArgCount})
					if sig.RetCount == 0 {
						// Preserve expression stack shape for continued lowering.
						fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
					}
					return
				}
				fc.errorf(fc.sig.File, 0, 0, "calls to external function %q are only supported with -T c/* targets", sig.Name)
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				return
			}
			fc.emit(ir.Inst{Op: ir.OP_CALL, Name: sig.IRName, Arg: callArgCount})
			if sig.RetCount == 0 {
				// Preserve expression stack shape for continued lowering.
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			}
			return
		}

		// Indirect call through function pointer representation.
		var indirectSig *cFuncTypeSig
		if t, ok := fc.exprTypeInfo(ex.left); ok && t.Kind == cDeclPointer && t.PtrDepth == 1 {
			indirectSig = cloneFuncTypeSig(t.FuncSig)
		}
		if indirectSig != nil {
			if !fc.checkCallArgsByType("<indirect>", indirectSig.ParamCount, indirectSig.ParamUnspecified, indirectSig.Variadic, indirectSig.ParamKinds, indirectSig.ParamFuncSigs, ex) {
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				return
			}
		}
		calleeTmp := fc.addLocal(fmt.Sprintf("$call_target$%d", fc.c.nextLabel()), fc.sig.File, 0, 0)
		fc.emitCallTargetValue(ex.left)
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: calleeTmp})

		argLocals := make([]int, 0, len(ex.args))
		for i, a := range ex.args {
			var indirectCallSig *cFuncSig
			if indirectSig != nil {
				indirectCallSig = &cFuncSig{
					ParamCount:       indirectSig.ParamCount,
					ParamUnspecified: indirectSig.ParamUnspecified,
					Variadic:         indirectSig.Variadic,
					ParamKinds:       indirectSig.ParamKinds,
					ParamBases:       indirectSig.ParamBases,
					ParamPtrDepth:    indirectSig.ParamPtrDepth,
					ParamOpaque:      indirectSig.ParamOpaque,
					ParamAggKey:      indirectSig.ParamAggKey,
					ParamAggTag:      indirectSig.ParamAggTag,
					ParamFuncSigs:    indirectSig.ParamFuncSigs,
				}
				fc.emitCallArgValue(indirectCallSig, a, i)
			} else {
				fc.emitExpr(a)
			}
			argInfo, ok := fc.callArgValueType(indirectCallSig, a, i)
			argTmp := fc.allocTempLocal(fmt.Sprintf("$call_arg$%d$%d", fc.c.nextLabel(), i))
			if ok {
				argTmp = fc.allocTempLocalForType(fmt.Sprintf("$call_arg$%d$%d", fc.c.nextLabel(), i), argInfo)
			}
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: argTmp})
			argLocals = append(argLocals, argTmp)
		}

		var candidates []*cFuncSig
		for _, sig := range fc.c.funcOrder {
			if sig == nil {
				continue
			}
			if indirectSig != nil {
				if !funcSigMatchesType(sig, indirectSig) {
					continue
				}
				candidates = append(candidates, sig)
				continue
			}
			if sig.Variadic {
				if len(ex.args) < sig.ParamCount {
					continue
				}
				candidates = append(candidates, sig)
				continue
			}
			if sig.ParamCount != len(ex.args) {
				continue
			}
			candidates = append(candidates, sig)
		}
		if len(candidates) == 0 && indirectSig != nil {
			wantRetCount := 1
			if indirectSig.RetIsVoid {
				wantRetCount = 0
			}
			for _, sig := range fc.c.funcOrder {
				if sig == nil {
					continue
				}
				if sig.RetCount != wantRetCount {
					continue
				}
				if sig.Variadic {
					if len(ex.args) < sig.ParamCount {
						continue
					}
					candidates = append(candidates, sig)
					continue
				}
				if sig.ParamCount != len(ex.args) {
					continue
				}
				candidates = append(candidates, sig)
			}
		}
		if len(candidates) == 0 {
			if indirectSig != nil {
				fc.errorf(fc.sig.File, 0, 0, "indirect call has no matching function candidates for resolved signature")
			} else {
				fc.errorf(fc.sig.File, 0, 0, "indirect call has no matching function candidates with %d parameters", len(ex.args))
			}
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			return
		}

		endLabel := fc.c.nextLabel()
		fallbackLabel := fc.c.nextLabel()
		matchLabels := make([]int, len(candidates))
		for i := range candidates {
			matchLabels[i] = fc.c.nextLabel()
		}

		for i, sig := range candidates {
			id := fc.c.funcIDs[sig.Name]
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: calleeTmp})
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: id})
			fc.emit(ir.Inst{Op: ir.OP_EQ})
			fc.emit(ir.Inst{Op: ir.OP_JMP_IF, Arg: matchLabels[i]})
		}
		fc.emit(ir.Inst{Op: ir.OP_JMP, Arg: fallbackLabel})

		for i, sig := range candidates {
			fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: matchLabels[i]})
			callArgs := sig.ParamCount
			if sig.Variadic && !sig.Defined {
				callArgs = len(argLocals)
			}
			if sig.Variadic && sig.Defined {
				j := 0
				for j < sig.ParamCount && j < len(argLocals) {
					fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: argLocals[j]})
					j++
				}
				fc.emitVariadicPackFromLocals(argLocals[sig.ParamCount:])
				callArgs = sig.ParamCount + 2
			} else {
				j := 0
				for j < callArgs && j < len(argLocals) {
					fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: argLocals[j]})
					j++
				}
			}
			if !sig.Defined {
				if fc.c.objectMode() {
					fc.emit(ir.Inst{Op: ir.OP_CALL, Name: sig.IRName, Arg: callArgs})
					if sig.RetCount == 0 {
						fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
					}
					fc.emit(ir.Inst{Op: ir.OP_JMP, Arg: endLabel})
					continue
				}
				if fc.c.canCallExternOnTarget(sig.Name, sig.RetCount) {
					wrap := fc.c.ensureExternWrapper(sig.Name, callArgs, sig.RetCount)
					fc.emit(ir.Inst{Op: ir.OP_CALL, Name: wrap, Arg: callArgs})
					if sig.RetCount == 0 {
						fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
					}
					fc.emit(ir.Inst{Op: ir.OP_JMP, Arg: endLabel})
					continue
				}
				fc.errorf(fc.sig.File, 0, 0, "calls to external function %q are only supported with -T c/* targets", sig.Name)
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				fc.emit(ir.Inst{Op: ir.OP_JMP, Arg: endLabel})
				continue
			}
			fc.emit(ir.Inst{Op: ir.OP_CALL, Name: sig.IRName, Arg: callArgs})
			if sig.RetCount == 0 {
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			}
			fc.emit(ir.Inst{Op: ir.OP_JMP, Arg: endLabel})
		}

		fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: fallbackLabel})
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: endLabel})
	case exprCast:
		fc.emitExprValueCast(ex.left, ex.typeInfo)
	case exprCompoundLit:
		if isAggregateObjectType(ex.typeInfo) || ex.typeInfo.Kind == cDeclArray {
			if !fc.emitAddressOf(ex) {
				fc.errorf(fc.sig.File, 0, 0, "invalid compound literal")
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				break
			}
			break
		}
		if !fc.emitAddressOf(ex) {
			fc.errorf(fc.sig.File, 0, 0, "invalid compound literal")
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			break
		}
		fc.emitLoadForType(int(fc.typeByteSize(ex.typeInfo)), ex.typeInfo)
	case exprSizeof:
		if ex.left == nil {
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: fc.sizeofType(ex.typeInfo)})
		} else {
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: fc.sizeofExpr(ex.left)})
		}
	default:
		fc.errorf(fc.sig.File, 0, 0, "unsupported expression form")
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
	}
}

func (fc *funcCompiler) emitLogicalExpr(ex *expr) {
	falseLabel := fc.c.nextLabel()
	trueLabel := fc.c.nextLabel()
	endLabel := fc.c.nextLabel()
	if ex.op == "&&" {
		fc.emitConditionValue(ex.left)
		fc.emit(ir.Inst{Op: ir.OP_JMP_IF_NOT, Arg: falseLabel})
		fc.emitConditionValue(ex.right)
		fc.emit(ir.Inst{Op: ir.OP_JMP_IF_NOT, Arg: falseLabel})
		fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: trueLabel})
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 1})
		fc.emit(ir.Inst{Op: ir.OP_JMP, Arg: endLabel})
		fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: falseLabel})
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: endLabel})
		return
	}
	fc.emitConditionValue(ex.left)
	fc.emit(ir.Inst{Op: ir.OP_JMP_IF, Arg: trueLabel})
	fc.emitConditionValue(ex.right)
	fc.emit(ir.Inst{Op: ir.OP_JMP_IF, Arg: trueLabel})
	fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
	fc.emit(ir.Inst{Op: ir.OP_JMP, Arg: endLabel})
	fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: trueLabel})
	fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 1})
	fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: endLabel})
}

type exprKind int

const (
	exprIntLit exprKind = iota
	exprFloatLit
	exprStringLit
	exprVar
	exprAssign
	exprComma
	exprConditional
	exprUnary
	exprPostfix
	exprBinary
	exprIndex
	exprMember
	exprCall
	exprCast
	exprCompoundLit
	exprSizeof
)

type expr struct {
	kind exprKind
	op   string

	intVal int64
	floatVal string
	strVal string
	name   string

	left  *expr
	right *expr
	args  []*expr

	member string

	typeInfo cTypeInfo
	initTok  []Token
}

type cExprParser struct {
	fc   *funcCompiler
	file string
	line int
	col  int
	toks []Token
	pos  int
}

func looksLikeTypeNameTokens(tokens []Token) bool {
	tokens = trimTokens(tokens)
	if len(tokens) == 0 {
		return false
	}
	i := 0
	for i < len(tokens) {
		t := tokens[i]
		if t.Kind != TokIdent {
			break
		}
		if isStorageClassKeyword(t.Text) || isTypeQualifierKeyword(t.Text) {
			i++
			continue
		}
		if t.Text == "struct" || t.Text == "union" || t.Text == "enum" {
			return true
		}
		if isTypeSpecifierKeyword(t.Text) || isUnsupportedCTypeKeyword(t.Text) {
			return true
		}
		if _, ok := lookupTypedefAlias(t.Text); ok {
			return true
		}
		break
	}
	return false
}

func (p *cExprParser) errorf(format string, args ...interface{}) {
	p.fc.errorf(p.file, p.line, p.col, format, args...)
}

func (p *cExprParser) atEnd() bool {
	return p.pos >= len(p.toks)
}

func (p *cExprParser) peek() Token {
	if p.atEnd() {
		return Token{Kind: TokEOF}
	}
	return p.toks[p.pos]
}

func (p *cExprParser) advance() Token {
	t := p.peek()
	if !p.atEnd() {
		p.pos++
	}
	return t
}

func (p *cExprParser) matchPunct(op string) bool {
	if p.atEnd() {
		return false
	}
	t := p.peek()
	if t.Kind == TokPunct && t.Text == op {
		p.pos++
		return true
	}
	return false
}

func (p *cExprParser) matchIdent(name string) bool {
	if p.atEnd() {
		return false
	}
	t := p.peek()
	if t.Kind == TokIdent && t.Text == name {
		p.pos++
		return true
	}
	return false
}

func (p *cExprParser) tryParseParenType(allowArray bool) (cTypeInfo, int, bool) {
	if p.pos >= len(p.toks) {
		return cTypeInfo{}, 0, false
	}
	start := p.pos
	if p.toks[start].Kind != TokPunct || p.toks[start].Text != "(" {
		return cTypeInfo{}, 0, false
	}
	depth := 0
	end := -1
	for i := start; i < len(p.toks); i++ {
		t := p.toks[i]
		if t.Kind != TokPunct {
			continue
		}
		if t.Text == "(" {
			depth++
		} else if t.Text == ")" {
			depth--
			if depth == 0 {
				end = i
				break
			}
		}
	}
	if end < 0 {
		return cTypeInfo{}, 0, false
	}
	inner := trimTokens(p.toks[start+1 : end])
	if !looksLikeTypeNameTokens(inner) {
		return cTypeInfo{}, 0, false
	}
	info, err := parseCTypeInfo(inner)
	if err != nil {
		p.errorf("%v", err)
		return cTypeInfo{Kind: cDeclScalar}, end - start + 1, true
	}
	if !allowArray && info.Kind == cDeclArray {
		return cTypeInfo{}, 0, false
	}
	return info, end - start + 1, true
}

func (p *cExprParser) parseExpression() *expr {
	return p.parseComma()
}

func (p *cExprParser) parseComma() *expr {
	n := p.parseAssignment()
	for p.matchPunct(",") {
		r := p.parseAssignment()
		n = &expr{kind: exprComma, left: n, right: r}
	}
	return n
}

func (p *cExprParser) parseAssignment() *expr {
	lhs := p.parseConditional()
	if lhs == nil {
		return nil
	}
	assignOp := ""
	switch {
	case p.matchPunct("="):
		assignOp = "="
	case p.matchPunct("+="):
		assignOp = "+="
	case p.matchPunct("-="):
		assignOp = "-="
	case p.matchPunct("*="):
		assignOp = "*="
	case p.matchPunct("/="):
		assignOp = "/="
	case p.matchPunct("%="):
		assignOp = "%="
	case p.matchPunct("&="):
		assignOp = "&="
	case p.matchPunct("^="):
		assignOp = "^="
	case p.matchPunct("|="):
		assignOp = "|="
	case p.matchPunct("<<="):
		assignOp = "<<="
	case p.matchPunct(">>="):
		assignOp = ">>="
	}
	if assignOp != "" {
		rhs := p.parseAssignment()
		if rhs == nil {
			rhs = &expr{kind: exprIntLit, intVal: 0}
		}
		return &expr{kind: exprAssign, op: assignOp, left: lhs, right: rhs}
	}
	return lhs
}

func (p *cExprParser) parseConditional() *expr {
	cond := p.parseLogicalOr()
	if cond == nil {
		return nil
	}
	if !p.matchPunct("?") {
		return cond
	}
	thenExpr := p.parseExpression()
	if thenExpr == nil {
		thenExpr = &expr{kind: exprIntLit, intVal: 0}
	}
	if !p.matchPunct(":") {
		p.errorf("expected ':' in conditional expression")
		return thenExpr
	}
	elseExpr := p.parseConditional()
	if elseExpr == nil {
		elseExpr = &expr{kind: exprIntLit, intVal: 0}
	}
	return &expr{kind: exprConditional, left: cond, args: []*expr{thenExpr, elseExpr}}
}

func (p *cExprParser) parseLogicalOr() *expr {
	n := p.parseLogicalAnd()
	for p.matchPunct("||") {
		r := p.parseLogicalAnd()
		n = &expr{kind: exprBinary, op: "||", left: n, right: r}
	}
	return n
}

func (p *cExprParser) parseLogicalAnd() *expr {
	n := p.parseBitOr()
	for p.matchPunct("&&") {
		r := p.parseBitOr()
		n = &expr{kind: exprBinary, op: "&&", left: n, right: r}
	}
	return n
}

func (p *cExprParser) parseBitOr() *expr {
	n := p.parseBitXor()
	for p.matchPunct("|") {
		r := p.parseBitXor()
		n = &expr{kind: exprBinary, op: "|", left: n, right: r}
	}
	return n
}

func (p *cExprParser) parseBitXor() *expr {
	n := p.parseBitAnd()
	for p.matchPunct("^") {
		r := p.parseBitAnd()
		n = &expr{kind: exprBinary, op: "^", left: n, right: r}
	}
	return n
}

func (p *cExprParser) parseBitAnd() *expr {
	n := p.parseEquality()
	for p.matchPunct("&") {
		r := p.parseEquality()
		n = &expr{kind: exprBinary, op: "&", left: n, right: r}
	}
	return n
}

func (p *cExprParser) parseEquality() *expr {
	n := p.parseRelational()
	for {
		if p.matchPunct("==") {
			r := p.parseRelational()
			n = &expr{kind: exprBinary, op: "==", left: n, right: r}
			continue
		}
		if p.matchPunct("!=") {
			r := p.parseRelational()
			n = &expr{kind: exprBinary, op: "!=", left: n, right: r}
			continue
		}
		break
	}
	return n
}

func (p *cExprParser) parseRelational() *expr {
	n := p.parseShift()
	for {
		if p.matchPunct("<") {
			r := p.parseShift()
			n = &expr{kind: exprBinary, op: "<", left: n, right: r}
			continue
		}
		if p.matchPunct(">") {
			r := p.parseShift()
			n = &expr{kind: exprBinary, op: ">", left: n, right: r}
			continue
		}
		if p.matchPunct("<=") {
			r := p.parseShift()
			n = &expr{kind: exprBinary, op: "<=", left: n, right: r}
			continue
		}
		if p.matchPunct(">=") {
			r := p.parseShift()
			n = &expr{kind: exprBinary, op: ">=", left: n, right: r}
			continue
		}
		break
	}
	return n
}

func (p *cExprParser) parseShift() *expr {
	n := p.parseAdditive()
	for {
		if p.matchPunct("<<") {
			r := p.parseAdditive()
			n = &expr{kind: exprBinary, op: "<<", left: n, right: r}
			continue
		}
		if p.matchPunct(">>") {
			r := p.parseAdditive()
			n = &expr{kind: exprBinary, op: ">>", left: n, right: r}
			continue
		}
		break
	}
	return n
}

func (p *cExprParser) parseAdditive() *expr {
	n := p.parseMultiplicative()
	for {
		if p.matchPunct("+") {
			r := p.parseMultiplicative()
			n = &expr{kind: exprBinary, op: "+", left: n, right: r}
			continue
		}
		if p.matchPunct("-") {
			r := p.parseMultiplicative()
			n = &expr{kind: exprBinary, op: "-", left: n, right: r}
			continue
		}
		break
	}
	return n
}

func (p *cExprParser) parseMultiplicative() *expr {
	n := p.parseUnary()
	for {
		if p.matchPunct("*") {
			r := p.parseUnary()
			n = &expr{kind: exprBinary, op: "*", left: n, right: r}
			continue
		}
		if p.matchPunct("/") {
			r := p.parseUnary()
			n = &expr{kind: exprBinary, op: "/", left: n, right: r}
			continue
		}
		if p.matchPunct("%") {
			r := p.parseUnary()
			n = &expr{kind: exprBinary, op: "%", left: n, right: r}
			continue
		}
		break
	}
	return n
}

func (p *cExprParser) parseUnary() *expr {
	if p.matchIdent("sizeof") {
		if info, consumed, ok := p.tryParseParenType(true); ok {
			p.pos += consumed
			return &expr{kind: exprSizeof, typeInfo: info}
		}
		operand := p.parseUnary()
		if operand == nil {
			operand = &expr{kind: exprIntLit, intVal: 0}
		}
		return &expr{kind: exprSizeof, left: operand}
	}
	if info, consumed, ok := p.tryParseParenType(true); ok {
		bracePos := p.pos + consumed
		if bracePos < len(p.toks) && p.toks[bracePos].Kind == TokPunct && p.toks[bracePos].Text == "{" {
			close := matchBraceClose(p.toks, bracePos)
			if close < 0 {
				p.errorf("unterminated compound literal initializer")
				return &expr{kind: exprIntLit, intVal: 0}
			}
			p.pos = close + 1
			return &expr{kind: exprCompoundLit, typeInfo: info, initTok: append([]Token{}, p.toks[bracePos:close+1]...)}
		}
		if info.Kind == cDeclArray {
			// Preserve previous cast behavior: casts do not accept array types.
			return p.parsePostfix()
		}
		p.pos += consumed
		rhs := p.parseUnary()
		if rhs == nil {
			rhs = &expr{kind: exprIntLit, intVal: 0}
		}
		return &expr{kind: exprCast, left: rhs, typeInfo: info}
	}
	if p.matchPunct("+") {
		return &expr{kind: exprUnary, op: "+", left: p.parseUnary()}
	}
	if p.matchPunct("-") {
		return &expr{kind: exprUnary, op: "-", left: p.parseUnary()}
	}
	if p.matchPunct("!") {
		return &expr{kind: exprUnary, op: "!", left: p.parseUnary()}
	}
	if p.matchPunct("~") {
		return &expr{kind: exprUnary, op: "~", left: p.parseUnary()}
	}
	if p.matchPunct("&") {
		return &expr{kind: exprUnary, op: "&", left: p.parseUnary()}
	}
	if p.matchPunct("*") {
		return &expr{kind: exprUnary, op: "*", left: p.parseUnary()}
	}
	if p.matchPunct("++") {
		return &expr{kind: exprUnary, op: "++", left: p.parseUnary()}
	}
	if p.matchPunct("--") {
		return &expr{kind: exprUnary, op: "--", left: p.parseUnary()}
	}
	return p.parsePostfix()
}

func (p *cExprParser) parsePostfix() *expr {
	n := p.parsePrimary()
	if n == nil {
		return nil
	}
	for {
		if p.matchPunct("(") {
			var args []*expr
			if !p.matchPunct(")") {
				for {
					a := p.parseAssignment()
					if a == nil {
						a = &expr{kind: exprIntLit, intVal: 0}
					}
					args = append(args, a)
					if p.matchPunct(")") {
						break
					}
					if !p.matchPunct(",") {
						p.errorf("expected ',' or ')' in call argument list")
						break
					}
				}
			}
			callName := ""
			if n != nil && n.kind == exprVar {
				callName = n.name
			}
			n = &expr{kind: exprCall, left: n, name: callName, args: args}
			continue
		}
		if p.matchPunct("[") {
			idx := p.parseExpression()
			if idx == nil {
				idx = &expr{kind: exprIntLit, intVal: 0}
			}
			if !p.matchPunct("]") {
				p.errorf("expected ']' in index expression")
			}
			n = &expr{kind: exprIndex, left: n, right: idx}
			continue
		}
		if p.matchPunct(".") {
			if p.atEnd() {
				p.errorf("expected member name after '.'")
				return &expr{kind: exprIntLit, intVal: 0}
			}
			memberTok := p.advance()
			if memberTok.Kind != TokIdent {
				p.errorf("expected member name after '.', got %q", memberTok.Text)
				return &expr{kind: exprIntLit, intVal: 0}
			}
			n = &expr{kind: exprMember, op: ".", left: n, member: memberTok.Text}
			continue
		}
		if p.matchPunct("->") {
			if p.atEnd() {
				p.errorf("expected member name after '->'")
				return &expr{kind: exprIntLit, intVal: 0}
			}
			memberTok := p.advance()
			if memberTok.Kind != TokIdent {
				p.errorf("expected member name after '->', got %q", memberTok.Text)
				return &expr{kind: exprIntLit, intVal: 0}
			}
			n = &expr{kind: exprMember, op: "->", left: n, member: memberTok.Text}
			continue
		}
		if p.matchPunct("++") {
			n = &expr{kind: exprPostfix, op: "++", left: n}
			continue
		}
		if p.matchPunct("--") {
			n = &expr{kind: exprPostfix, op: "--", left: n}
			continue
		}
		break
	}
	return n
}

func (p *cExprParser) parsePrimary() *expr {
	if p.atEnd() {
		p.errorf("unexpected end of expression")
		return &expr{kind: exprIntLit, intVal: 0}
	}
	t := p.advance()
	switch t.Kind {
	case TokNumber:
		if lit, base, ok, err := parseCFloatLiteral(t.Text); ok {
			if err != nil {
				p.errorf("invalid floating-point literal %q: %v", t.Text, err)
				lit = "0.0"
				base = cScalarDouble
			}
			return &expr{kind: exprFloatLit, floatVal: lit, typeInfo: cTypeInfo{Kind: cDeclScalar, Base: base}}
		}
		v, err := parseCIntLiteral(t.Text)
		if err != nil {
			p.errorf("invalid integer literal %q: %v", t.Text, err)
			v = 0
		}
		return &expr{kind: exprIntLit, intVal: v}
	case TokChar:
		v, err := parseCCharLiteral(t.Text)
		if err != nil {
			p.errorf("invalid char literal %q: %v", t.Text, err)
			v = 0
		}
		return &expr{kind: exprIntLit, intVal: v}
	case TokString:
		s, err := decodeStringToken(t)
		if err != nil {
			p.errorf("invalid string literal %q: %v", t.Text, err)
			s = ""
		}
		for !p.atEnd() {
			next := p.peek()
			if next.Kind != TokString {
				break
			}
			t2 := p.advance()
			part, err := decodeStringToken(t2)
			if err != nil {
				p.errorf("invalid string literal %q: %v", t2.Text, err)
				continue
			}
			s += part
		}
		return &expr{kind: exprStringLit, strVal: s}
	case TokIdent:
		return &expr{kind: exprVar, name: t.Text}
	case TokPunct:
		if t.Text == "(" {
			e := p.parseExpression()
			if !p.matchPunct(")") {
				p.errorf("expected ')' to close parenthesized expression")
			}
			return e
		}
	}
	p.errorf("unexpected token %q in expression", t.Text)
	return &expr{kind: exprIntLit, intVal: 0}
}

func containsAnyByte(text string, chars string) bool {
	for i := 0; i < len(text); i++ {
		for j := 0; j < len(chars); j++ {
			if text[i] == chars[j] {
				return true
			}
		}
	}
	return false
}

func hasHexPrefix(text string) bool {
	return len(text) >= 2 && text[0] == '0' && (text[1] == 'x' || text[1] == 'X')
}

func isValidSimpleCFloatLiteral(text string) bool {
	if len(text) == 0 {
		return false
	}
	digits := 0
	fracDigits := 0
	i := 0
	if i < len(text) && (text[i] == '+' || text[i] == '-') {
		i++
	}
	for i < len(text) && text[i] >= '0' && text[i] <= '9' {
		digits++
		i++
	}
	if i < len(text) && text[i] == '.' {
		i++
		for i < len(text) && text[i] >= '0' && text[i] <= '9' {
			fracDigits++
			i++
		}
	}
	if digits == 0 && fracDigits == 0 {
		return false
	}
	if i < len(text) && (text[i] == 'e' || text[i] == 'E') {
		i++
		if i < len(text) && (text[i] == '+' || text[i] == '-') {
			i++
		}
		expDigits := 0
		for i < len(text) && text[i] >= '0' && text[i] <= '9' {
			expDigits++
			i++
		}
		if expDigits == 0 {
			return false
		}
	}
	return i == len(text)
}

func parseCFloatLiteral(text string) (string, cScalarType, bool, error) {
	s := text
	if s == "" {
		return "", cScalarDouble, false, nil
	}
	core := s
	base := cScalarDouble
	if hasHexPrefix(core) {
		last := core[len(core)-1]
		suffix := byte(0)
		if last == 'f' || last == 'F' || last == 'l' || last == 'L' {
			suffix = last
			core = core[:len(core)-1]
			if len(core) > 0 {
				prev := core[len(core)-1]
				if suffix == 'f' || suffix == 'F' {
					if prev == 'u' || prev == 'U' {
						return "", cScalarDouble, false, nil
					}
				} else {
					if prev == 'u' || prev == 'U' || prev == 'l' || prev == 'L' {
						return "", cScalarDouble, false, nil
					}
				}
			}
		}
		if core == "" {
			return "", cScalarDouble, true, fmt.Errorf("empty floating-point literal")
		}
		if !containsAnyByte(core, ".pP") {
			return "", cScalarDouble, false, nil
		}
		if suffix == 'f' || suffix == 'F' {
			base = cScalarFloat
		} else if suffix == 'l' || suffix == 'L' {
			return "", cScalarDouble, true, fmt.Errorf("long double literals are not supported")
		}
		return core, base, true, nil
	}
	if !containsAnyByte(core, ".eE") {
		return "", cScalarDouble, false, nil
	}
	last := core[len(core)-1]
	if last == 'f' || last == 'F' {
		base = cScalarFloat
		core = core[:len(core)-1]
	} else if last == 'l' || last == 'L' {
		return "", cScalarDouble, true, fmt.Errorf("long double literals are not supported")
	}
	if core == "" {
		return "", cScalarDouble, true, fmt.Errorf("empty floating-point literal")
	}
	if !isValidSimpleCFloatLiteral(core) {
		return "", cScalarDouble, true, fmt.Errorf("invalid floating-point literal")
	}
	return core, base, true, nil
}

func parseCIntLiteral(text string) (int64, error) {
	s := text
	if s == "" {
		return 0, fmt.Errorf("empty literal")
	}
	if strings.Contains(s, ".") {
		return 0, fmt.Errorf("floating-point literals are not supported")
	}

	end := len(s)
	for end > 0 {
		ch := s[end-1]
		if ch == 'u' || ch == 'U' || ch == 'l' || ch == 'L' {
			end--
			continue
		}
		break
	}
	s = s[:end]
	if s == "" {
		return 0, fmt.Errorf("invalid literal suffix")
	}
	if strings.Contains(s, ".") {
		return 0, fmt.Errorf("floating-point literals are not supported")
	}

	base := 10
	num := s
	if len(num) > 2 && (strings.HasPrefix(num, "0x") || strings.HasPrefix(num, "0X")) {
		base = 16
		num = num[2:]
	} else if len(num) > 2 && (strings.HasPrefix(num, "0b") || strings.HasPrefix(num, "0B")) {
		base = 2
		num = num[2:]
	} else if len(num) > 1 && num[0] == '0' {
		base = 8
		num = num[1:]
	}
	if base == 16 {
		if strings.Contains(num, "p") || strings.Contains(num, "P") {
			return 0, fmt.Errorf("floating-point literals are not supported")
		}
	} else if strings.Contains(s, "e") || strings.Contains(s, "E") || strings.Contains(s, "p") || strings.Contains(s, "P") {
		return 0, fmt.Errorf("floating-point literals are not supported")
	}
	if num == "" {
		return 0, nil
	}
	v, err := parseUintBase(num, base, 64)
	if err != nil {
		return 0, err
	}
	return int64(v), nil
}

func parseCCharLiteral(text string) (int64, error) {
	s := text
	if strings.HasPrefix(s, "u8") {
		s = s[2:]
	} else if strings.HasPrefix(s, "u") || strings.HasPrefix(s, "U") || strings.HasPrefix(s, "L") {
		s = s[1:]
	}
	if len(s) < 2 || s[0] != '\'' || s[len(s)-1] != '\'' {
		return 0, fmt.Errorf("expected quoted character literal")
	}
	body := s[1 : len(s)-1]
	if body == "" {
		return 0, fmt.Errorf("empty char literal")
	}
	if body[0] != '\\' {
		return int64(body[0]), nil
	}
	if len(body) == 1 {
		return 0, fmt.Errorf("invalid escape")
	}
	esc := body[1]
	switch esc {
	case 'n':
		return int64('\n'), nil
	case 'r':
		return int64('\r'), nil
	case 't':
		return int64('\t'), nil
	case '0':
		return 0, nil
	case '\\':
		return int64('\\'), nil
	case '\'':
		return int64('\''), nil
	case '"':
		return int64('"'), nil
	case 'x':
		if len(body) < 4 {
			return 0, fmt.Errorf("short hex escape")
		}
		v, err := parseUintBase(body[2:], 16, 8)
		if err != nil {
			return 0, err
		}
		return int64(v), nil
	default:
		if esc >= '0' && esc <= '7' {
			end := 2
			for end < len(body) && end < 4 && body[end] >= '0' && body[end] <= '7' {
				end++
			}
			v, err := parseUintBase(body[1:end], 8, 8)
			if err != nil {
				return 0, err
			}
			return int64(v), nil
		}
		return int64(esc), nil
	}
}
