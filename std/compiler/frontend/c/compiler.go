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
	Name          string
	IRName        string
	RetCount      int
	RetKind       cDeclKind
	RetBase       cScalarType
	RetPtrDepth   int
	RetOpaque     bool
	RetAggKeyword string
	RetAggTag     string
	ParamCount    int
	Variadic      bool
	ParamNames    []string
	ParamKinds    []cDeclKind
	ParamBases    []cScalarType
	ParamPtrDepth []int
	ParamOpaque   []bool
	ParamAggKey   []string
	ParamAggTag   []string
	ParamFuncSigs []*cFuncTypeSig
	Defined       bool
	Body          *Node
	File          string
	Line          int
	Col           int
}

type cFuncTypeSig struct {
	RetKind       cDeclKind
	RetBase       cScalarType
	RetPtrDepth   int
	RetIsVoid     bool
	RetOpaque     bool
	RetAggKeyword string
	RetAggTag     string
	ParamCount    int
	Variadic      bool
	ParamKinds    []cDeclKind
	ParamBases    []cScalarType
	ParamPtrDepth []int
	ParamOpaque   []bool
	ParamAggKey   []string
	ParamAggTag   []string
	ParamFuncSigs []*cFuncTypeSig
}

type cDeclItem struct {
	Name             string
	Init             []Token
	Kind             cDeclKind
	PtrDepth         int
	ArrayLen         int64
	IsVoid           bool
	Base             cScalarType
	FuncSig          *cFuncTypeSig
	OpaqueAggregate  bool
	AggregateKeyword string
	AggregateTag     string
}

type cGlobalInit struct {
	Name             string
	Index            int
	Kind             cDeclKind
	PtrDepth         int
	ArrayBase        int
	ArrayLen         int64
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
)

type cTypeInfo struct {
	Kind             cDeclKind
	PtrDepth         int
	ArrayLen         int64
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
	if cAggregateLookupFunc != nil && cAggregateLookupFunc.c != nil && cAggregateLookupFunc.c.target != nil {
		return int64(cAggregateLookupFunc.c.target.PtrSize)
	}
	if cAggregateLookupCompiler != nil && cAggregateLookupCompiler.target != nil {
		return int64(cAggregateLookupCompiler.target.PtrSize)
	}
	return 8
}

func currentCTargetLongSize() int64 {
	if cAggregateLookupFunc != nil && cAggregateLookupFunc.c != nil && cAggregateLookupFunc.c.target != nil {
		if cAggregateLookupFunc.c.target.GOOS == "windows" {
			return 4
		}
		return int64(cAggregateLookupFunc.c.target.PtrSize)
	}
	if cAggregateLookupCompiler != nil && cAggregateLookupCompiler.target != nil {
		if cAggregateLookupCompiler.target.GOOS == "windows" {
			return 4
		}
		return int64(cAggregateLookupCompiler.target.PtrSize)
	}
	return 8
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
	Base             cScalarType
	FuncSig          *cFuncTypeSig
	OpaqueAggregate  bool
	AggregateKeyword string
	AggregateTag     string
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
			TypeIDs:         make(map[string]int),
			MethodTable:     make(map[string]string),
			IfaceMethods:    make(map[string][]string),
			IfaceMethodRets: make(map[string]int),
		},
		funcs:          make(map[string]*cFuncSig),
		globalIndex:    make(map[string]int),
		globalKind:     make(map[string]cDeclKind),
		globalPtrDepth: make(map[string]int),
		globalElemStep: make(map[string]int64),
		globalArray:    make(map[string]int64),
		globalBase:     make(map[string]cScalarType),
		globalFunc:     make(map[string]*cFuncTypeSig),
		globalOpaque:   make(map[string]bool),
		globalAggKey:   make(map[string]string),
		globalAggTag:   make(map[string]string),
		aggregateTags:  make(map[string]*cAggregateInfo),
		enumConsts:     make(map[string]int64),
		typedefs:       make(map[string]cTypeInfo),
		funcIDs:        make(map[string]int64),
		intrinsics:     make(map[string]cIntrinsicWrapper),
		externFns:      make(map[string]string),
		nextLabelSeq:   1,
	}
	prevTypedefLookupCompiler := cTypedefLookupCompiler
	prevTypedefLookupFunc := cTypedefLookupFunc
	prevAggregateLookupCompiler := cAggregateLookupCompiler
	prevAggregateLookupFunc := cAggregateLookupFunc
	cTypedefLookupCompiler = c
	cTypedefLookupFunc = nil
	cAggregateLookupCompiler = c
	cAggregateLookupFunc = nil

	c.collectTopLevel()
	if len(c.errors) > 0 {
		cTypedefLookupCompiler = prevTypedefLookupCompiler
		cTypedefLookupFunc = prevTypedefLookupFunc
		cAggregateLookupCompiler = prevAggregateLookupCompiler
		cAggregateLookupFunc = prevAggregateLookupFunc
		return nil, c.errors
	}
	c.assignFunctionIDs()

	c.emitGlobalInit()
	for _, sig := range c.funcOrder {
		if !sig.Defined {
			continue
		}
		c.compileFunction(sig)
	}
	c.emitEntryWrapper()

	if len(c.errors) > 0 {
		cTypedefLookupCompiler = prevTypedefLookupCompiler
		cTypedefLookupFunc = prevTypedefLookupFunc
		cAggregateLookupCompiler = prevAggregateLookupCompiler
		cAggregateLookupFunc = prevAggregateLookupFunc
		return nil, c.errors
	}
	cTypedefLookupCompiler = prevTypedefLookupCompiler
	cTypedefLookupFunc = prevTypedefLookupFunc
	cAggregateLookupCompiler = prevAggregateLookupCompiler
	cAggregateLookupFunc = prevAggregateLookupFunc
	return c.irmod, nil
}

type compiler struct {
	target *common.Target
	units  []Unit
	irmod  *ir.IRModule

	errors []string

	funcs     map[string]*cFuncSig
	funcOrder []*cFuncSig

	globalIndex    map[string]int
	globalKind     map[string]cDeclKind
	globalPtrDepth map[string]int
	globalElemStep map[string]int64
	globalArray    map[string]int64
	globalBase     map[string]cScalarType
	globalFunc     map[string]*cFuncTypeSig
	globalOpaque   map[string]bool
	globalAggKey   map[string]string
	globalAggTag   map[string]string
	aggregateTags  map[string]*cAggregateInfo
	enumConsts     map[string]int64
	globalInits    []cGlobalInit
	typedefs       map[string]cTypeInfo
	funcIDs        map[string]int64

	intrinsics map[string]cIntrinsicWrapper
	externFns  map[string]string

	nextLabelSeq int
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
	_ = base
	return int64(c.target.PtrSize)
}

func (c *compiler) lookupEnumConst(name string) (int64, bool) {
	v, ok := c.enumConsts[name]
	return v, ok
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
		RetKind:       in.RetKind,
		RetBase:       in.RetBase,
		RetPtrDepth:   in.RetPtrDepth,
		RetIsVoid:     in.RetIsVoid,
		RetOpaque:     in.RetOpaque,
		RetAggKeyword: in.RetAggKeyword,
		RetAggTag:     in.RetAggTag,
		ParamCount:    in.ParamCount,
		Variadic:      in.Variadic,
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
		RetKind:       sig.RetKind,
		RetBase:       sig.RetBase,
		RetPtrDepth:   sig.RetPtrDepth,
		RetIsVoid:     sig.RetCount == 0,
		RetOpaque:     sig.RetOpaque,
		RetAggKeyword: sig.RetAggKeyword,
		RetAggTag:     sig.RetAggTag,
		ParamCount:    sig.ParamCount,
		Variadic:      sig.Variadic,
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
	sig, err := parseFunctionSignature(file, n.Line, n.Col, toks)
	if err != nil {
		c.errorf(file, n.Line, n.Col, "%v", err)
		return
	}
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
	if len(toks) == 0 {
		return
	}

	// Function prototype: first top-level '(' must follow the function name.
	fnLPar := topLevelPunctIndex(toks, "(")
	if fnLPar > 0 && toks[fnLPar-1].Kind == TokIdent && !isDeclarationKeyword(toks[fnLPar-1]) {
		sig, err := parseFunctionSignature(file, n.Line, n.Col, toks)
		if err == nil {
			if _, ok := c.funcs[sig.Name]; !ok {
				sig.Defined = false
				c.funcs[sig.Name] = sig
				c.funcOrder = append(c.funcOrder, sig)
			}
			return
		}
	}

	items, enumConsts, hasExtern, hasTypedef, err := parseDeclItems(toks, c.enumConsts)
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
			if it.Kind == cDeclArray {
				c.errorf(file, n.Line, n.Col, "typedef array declarations are not yet supported: %s", it.Name)
				continue
			}
			if it.Kind == cDeclPointer && it.PtrDepth == 0 {
				it.PtrDepth = 1
			}
			if _, exists := c.typedefs[it.Name]; exists {
				c.errorf(file, n.Line, n.Col, "duplicate typedef name %q", it.Name)
				continue
			}
			c.typedefs[it.Name] = cTypeInfo{
				Kind:             it.Kind,
				PtrDepth:         it.PtrDepth,
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
		if hasExtern && len(it.Init) == 0 {
			continue
		}
		if hasExtern && len(it.Init) > 0 {
			c.errorf(file, n.Line, n.Col, "extern declaration with initializer is not supported: %s", it.Name)
			continue
		}
		if _, exists := c.globalIndex[it.Name]; exists {
			c.errorf(file, n.Line, n.Col, "duplicate global declaration for %q", it.Name)
			continue
		}
		idx := len(c.irmod.Globals)
		irName := "c." + it.Name
		c.irmod.Globals = append(c.irmod.Globals, ir.IRGlobal{Name: irName, Index: idx})
		c.globalIndex[it.Name] = idx
		c.globalKind[it.Name] = it.Kind
		c.globalPtrDepth[it.Name] = it.PtrDepth
		c.globalBase[it.Name] = it.Base
		c.globalFunc[it.Name] = cloneFuncTypeSig(it.FuncSig)
		c.globalOpaque[it.Name] = it.OpaqueAggregate
		c.globalAggKey[it.Name] = it.AggregateKeyword
		c.globalAggTag[it.Name] = it.AggregateTag
		elemStep := c.pointerElemStep(it.Kind, it.PtrDepth, it.Base, it.IsVoid, it.OpaqueAggregate, it.AggregateKeyword, it.AggregateTag)
		if it.Kind == cDeclPointer && it.PtrDepth == 1 && isStringLiteralExpr(it.Init) {
			elemStep = 1
		}
		c.globalElemStep[it.Name] = elemStep
		if it.Kind == cDeclArray {
			c.globalArray[it.Name] = it.ArrayLen
			base := len(c.irmod.Globals)
			for i := int64(0); i < it.ArrayLen; i++ {
				elemIdx := len(c.irmod.Globals)
				elemName := fmt.Sprintf("%s$%d", irName, i)
				c.irmod.Globals = append(c.irmod.Globals, ir.IRGlobal{Name: elemName, Index: elemIdx})
			}
			c.globalInits = append(c.globalInits, cGlobalInit{
				Name:      it.Name,
				Index:     idx,
				Kind:      it.Kind,
				PtrDepth:  it.PtrDepth,
				ArrayBase: base,
				ArrayLen:  it.ArrayLen,
				Base:      it.Base,
				Init:      append([]Token{}, it.Init...),
				File:      file,
				Line:      n.Line,
				Col:       n.Col,
				IRName:    irName,
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
				Name:     it.Name,
				Index:    idx,
				Kind:     it.Kind,
				PtrDepth: it.PtrDepth,
				ArrayLen: it.ArrayLen,
				Base:     it.Base,
				Init:     append([]Token{}, it.Init...),
				File:     file,
				Line:     n.Line,
				Col:      n.Col,
				IRName:   irName,
			})
		}
	}
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
			fc.emit(ir.Inst{Op: ir.OP_GLOBAL_ADDR, Arg: g.ArrayBase})
			fc.emit(ir.Inst{Op: ir.OP_GLOBAL_SET, Arg: g.Index})
			initElems, err := parseArrayInitializerExprs(g.Init, g.ArrayLen)
			if err != nil {
				c.errorf(g.File, g.Line, g.Col, "invalid array initializer for %s: %v", g.Name, err)
				continue
			}
			for i, initExpr := range initElems {
				fc.emitExprTokens(g.File, g.Line, g.Col, initExpr)
				fc.emitCastToType(cTypeInfo{Kind: cDeclScalar, Base: g.Base})
				fc.emit(ir.Inst{Op: ir.OP_GLOBAL_GET, Arg: g.Index})
				if i > 0 {
					fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(i * c.target.PtrSize)})
					fc.emit(ir.Inst{Op: ir.OP_ADD})
				}
				fc.emit(ir.Inst{Op: ir.OP_STORE, Arg: int(fc.scalarSize(g.Base))})
			}
			continue
		}
		fc.emitExprTokens(g.File, g.Line, g.Col, g.Init)
		fc.emitCastToType(cTypeInfo{
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
	if sig.Variadic {
		paramSlots += 2
	}
	f := &ir.IRFunc{Name: sig.IRName, Params: paramSlots, RetCount: sig.RetCount}
	fc := &funcCompiler{
		c:             c,
		sig:           sig,
		fn:            f,
		scopes:        []map[string]cLocalBinding{{}},
		typedefScopes: []map[string]cTypeInfo{{}},
		enumScopes:    []map[string]int64{{}},
		aggregateTags: []map[string]*cAggregateInfo{{}},
		variadicCount: -1,
		variadicData:  -1,
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
		fc.addLocalDecl(name, kind, base, ptrDepth, elemStep, 0, pfunc, opaqueAggregate, aggregateKeyword, aggregateTag, sig.File, sig.Line, sig.Col)
	}
	if sig.Variadic {
		fc.variadicCount = fc.addLocalTyped("$va_count", cDeclScalar, cScalarInt, 0, int64(fc.c.target.PtrSize), nil, sig.File, sig.Line, sig.Col)
		fc.variadicData = fc.addLocalTyped("$va_data", cDeclPointer, cScalarInt, 1, int64(fc.c.target.PtrSize), nil, sig.File, sig.Line, sig.Col)
	}

	prevTypedefLookupFunc := cTypedefLookupFunc
	prevAggregateLookupFunc := cAggregateLookupFunc
	cTypedefLookupFunc = fc
	cAggregateLookupFunc = fc
	fc.compileCompound(sig.Body, true)
	cTypedefLookupFunc = prevTypedefLookupFunc
	cAggregateLookupFunc = prevAggregateLookupFunc
	if len(f.Code) == 0 || f.Code[len(f.Code)-1].Op != ir.OP_RETURN {
		if sig.RetCount > 0 {
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
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
		c.errorf("", 0, 0, "no C entrypoint found: expected function \"main\"")
		return
	}
	f := &ir.IRFunc{Name: "main.main", Params: 0, RetCount: 1}
	for i := 0; i < mainSig.ParamCount; i++ {
		f.Code = append(f.Code, ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
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
			}
			if t.Text == sep && depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
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

	lpar := -1
	depth := 0
	for i, t := range toks {
		if t.Kind != TokPunct {
			continue
		}
		if t.Text == "(" {
			if depth == 0 {
				lpar = i
				break
			}
			depth++
		}
	}
	if lpar < 0 {
		return nil, fmt.Errorf("not a function declaration")
	}

	rpar := -1
	depth = 1
	for i := lpar + 1; i < len(toks); i++ {
		t := toks[i]
		if t.Kind != TokPunct {
			continue
		}
		if t.Text == "(" {
			depth++
		} else if t.Text == ")" {
			depth--
			if depth == 0 {
				rpar = i
				break
			}
		}
	}
	if rpar < 0 {
		return nil, fmt.Errorf("unterminated function parameter list")
	}

	head := trimTokens(toks[:lpar])
	spec, decl, err := splitDeclSpecPrefix(head, "function declaration")
	if err != nil {
		return nil, err
	}
	retSpec, _, _, err := parseScalarTypeSpec(spec, "function declaration", true)
	if err != nil {
		return nil, err
	}
	name, retDeclKind, retDeclPtrDepth, _, _, err := parseDeclarator(decl, false)
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
	if isAggregateObjectType(retInfo) {
		return nil, fmt.Errorf("function %q returning %s %q by value is not yet supported", name, retInfo.AggregateKeyword, retInfo.AggregateTag)
	}
	retCount := 1
	if retInfo.IsVoid && retInfo.Kind == cDeclScalar {
		retCount = 0
	}

	paramTokens := toks[lpar+1 : rpar]
	paramTokens = trimTokens(paramTokens)
	var paramNames []string
	var paramKinds []cDeclKind
	var paramBases []cScalarType
	var paramPtrDepth []int
	var paramOpaque []bool
	var paramAggKey []string
	var paramAggTag []string
	var paramFuncSigs []*cFuncTypeSig
	var variadic bool
	paramCount := 0
	if len(paramTokens) > 0 {
		parts := splitTopLevel(paramTokens, ",")
		if len(parts) == 1 {
			p0 := trimTokens(parts[0])
			if len(p0) > 0 {
				spec, decl, err := splitDeclSpecPrefix(p0, "function parameter list")
				if err == nil {
					baseInfo, _, _, err := parseScalarTypeSpec(spec, "function parameter list", true)
					if err == nil {
						_, kind, ptrDepth, arrLen, _, err := parseDeclarator(decl, true)
						if err == nil {
							info, cerr := combineTypeAndDeclarator(baseInfo, kind, ptrDepth, arrLen, false, "function parameter list")
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
			pbaseInfo, _, _, err := parseScalarTypeSpec(spec, fmt.Sprintf("function parameter %d", i+1), true)
			if err != nil {
				return nil, err
			}
			pname, pdeclKind, pdeclPtrDepth, parrLen, pfnSig, err := parseDeclarator(decl, true)
			if err != nil {
				return nil, err
			}
			pinfo, err := combineTypeAndDeclarator(pbaseInfo, pdeclKind, pdeclPtrDepth, parrLen, false, fmt.Sprintf("function parameter %d", i+1))
			if err != nil {
				return nil, err
			}
			if pfnSig != nil {
				fnSig := cloneFuncTypeSig(pfnSig)
				fnSig.RetKind = pbaseInfo.Kind
				fnSig.RetBase = pbaseInfo.Base
				fnSig.RetPtrDepth = pbaseInfo.PtrDepth
				fnSig.RetIsVoid = pbaseInfo.IsVoid
				fnSig.RetOpaque = pbaseInfo.OpaqueAggregate
				fnSig.RetAggKeyword = pbaseInfo.AggregateKeyword
				fnSig.RetAggTag = pbaseInfo.AggregateTag
				pinfo.FuncSig = fnSig
			}
			if pname == "" {
				pname = fmt.Sprintf("$p%d", i)
			}
			if pinfo.Kind == cDeclArray {
				// Arrays in parameter lists decay to pointers.
				pinfo.Kind = cDeclPointer
				if pinfo.PtrDepth == 0 {
					pinfo.PtrDepth = 1
				}
			}
			if pinfo.IsVoid && pinfo.Kind == cDeclScalar {
				return nil, fmt.Errorf("function parameter %q cannot have type void", pname)
			}
			if isAggregateObjectType(pinfo) {
				return nil, fmt.Errorf("function parameter %q passing %s %q by value is not yet supported", pname, pinfo.AggregateKeyword, pinfo.AggregateTag)
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

	return &cFuncSig{
		Name:          name,
		IRName:        "c." + name,
		RetCount:      retCount,
		RetKind:       retInfo.Kind,
		RetBase:       retInfo.Base,
		RetPtrDepth:   retInfo.PtrDepth,
		RetOpaque:     retInfo.OpaqueAggregate,
		RetAggKeyword: retInfo.AggregateKeyword,
		RetAggTag:     retInfo.AggregateTag,
		ParamCount:    paramCount,
		Variadic:      variadic,
		ParamNames:    paramNames,
		ParamKinds:    paramKinds,
		ParamBases:    paramBases,
		ParamPtrDepth: paramPtrDepth,
		ParamOpaque:   paramOpaque,
		ParamAggKey:   paramAggKey,
		ParamAggTag:   paramAggTag,
		ParamFuncSigs: paramFuncSigs,
		Defined:       false,
		File:          file,
		Line:          line,
		Col:           col,
	}, nil
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
	case "const", "volatile", "restrict", "inline":
		return true
	default:
		return false
	}
}

func isUnsupportedCTypeKeyword(text string) bool {
	switch text {
	case "float", "double", "_Bool", "_Complex", "_Imaginary":
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
			if isTypeSpecifierKeyword(t.Text) {
				sawType = true
			}
			end++
			continue
		}
		if _, ok := lookupTypedefAlias(t.Text); ok {
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
		return nil, nil, fmt.Errorf("%s is missing a type specifier", context)
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
		elem := info
		elem.Kind = cDeclScalar
		elem.ArrayLen = 0
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

func parseAggregateFields(tokens []Token, keyword string, tag string, context string) ([]cAggregateField, int64, int64, error) {
	decls := splitTopLevel(trimTokens(tokens), ";")
	fields := make([]cAggregateField, 0, len(decls))
	used := make(map[string]bool)
	maxAlign := int64(1)
	maxSize := int64(0)
	nextOffset := int64(0)
	for i, rawDecl := range decls {
		decl := trimTokens(rawDecl)
		if len(decl) == 0 {
			continue
		}
		dctx := fmt.Sprintf("%s member declaration %d", context, i+1)
		spec, rest, err := splitDeclSpecPrefix(decl, dctx)
		if err != nil {
			return nil, 0, 0, err
		}
		baseInfo, hasExtern, hasTypedef, err := parseScalarTypeSpec(spec, dctx, true)
		if err != nil {
			return nil, 0, 0, err
		}
		if hasExtern {
			return nil, 0, 0, fmt.Errorf("%s does not allow extern members", dctx)
		}
		if hasTypedef {
			return nil, 0, 0, fmt.Errorf("%s does not allow typedef members", dctx)
		}
		if len(rest) == 0 {
			return nil, 0, 0, fmt.Errorf("%s requires at least one declarator", dctx)
		}
		items, err := parseDeclItemsWithBase(baseInfo, false, rest)
		if err != nil {
			return nil, 0, 0, err
		}
		for _, it := range items {
			if len(it.Init) > 0 {
				return nil, 0, 0, fmt.Errorf("%s member %q cannot have initializer", dctx, it.Name)
			}
			if it.FuncSig != nil {
				return nil, 0, 0, fmt.Errorf("%s member %q cannot be a function type", dctx, it.Name)
			}
			if it.Name == "" {
				return nil, 0, 0, fmt.Errorf("%s has unnamed member", dctx)
			}
			if used[it.Name] {
				return nil, 0, 0, fmt.Errorf("%s has duplicate member name %q", dctx, it.Name)
			}
			memberType := cTypeInfo{
				Kind:             it.Kind,
				PtrDepth:         it.PtrDepth,
				ArrayLen:         it.ArrayLen,
				IsVoid:           it.IsVoid,
				Base:             it.Base,
				FuncSig:          cloneFuncTypeSig(it.FuncSig),
				OpaqueAggregate:  it.OpaqueAggregate,
				AggregateKeyword: it.AggregateKeyword,
				AggregateTag:     it.AggregateTag,
			}
			size, align, err := cTypeLayout(memberType)
			if err != nil {
				return nil, 0, 0, fmt.Errorf("%s member %q has unsupported type: %v", dctx, it.Name, err)
			}
			if align <= 0 {
				align = 1
			}
			field := cAggregateField{
				Name:  it.Name,
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
			used[it.Name] = true
			fields = append(fields, field)
		}
	}
	if len(fields) == 0 {
		return nil, 0, 0, fmt.Errorf("%s %q requires at least one member declaration", keyword, tag)
	}
	maxSize = alignTo(maxSize, maxAlign)
	return fields, maxSize, maxAlign, nil
}

func parseAggregateTypeSpec(tokens []Token, start int, keyword string, context string) (int, cTypeInfo, error) {
	if start >= len(tokens) || tokens[start].Kind != TokIdent || tokens[start].Text != keyword {
		return start, cTypeInfo{}, fmt.Errorf("%s expected %s specifier", context, keyword)
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
			return start, cTypeInfo{}, fmt.Errorf("%s has unterminated %s definition", context, keyword)
		}
	}
	if tag == "" && !hasBody {
		return start, cTypeInfo{}, fmt.Errorf("%s %s specifier requires tag or body", context, keyword)
	}
	if hasBody {
		if tag == "" {
			tag = nextAnonAggregateTag(keyword)
		}
		placeholder := &cAggregateInfo{Keyword: keyword, Tag: tag, IsUnion: keyword == "union"}
		if err := registerAggregateAlias(placeholder); err != nil {
			return start, cTypeInfo{}, err
		}
		body := trimTokens(tokens[bodyOpen+1 : bodyClose])
		fields, size, align, err := parseAggregateFields(body, keyword, tag, context)
		if err != nil {
			return start, cTypeInfo{}, err
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
			return start, cTypeInfo{}, err
		}
		return i, cTypeInfo{
			Kind:             cDeclScalar,
			Base:             cScalarInt,
			OpaqueAggregate:  false,
			AggregateKeyword: keyword,
			AggregateTag:     tag,
		}, nil
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
	}, nil
}

func parseScalarTypeSpec(spec []Token, context string, allowVoid bool) (cTypeInfo, bool, bool, error) {
	var hasExtern bool
	var hasTypedef bool
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
	var sawSigned bool
	var sawUnsigned bool
	var aliasSet bool
	var aliasInfo cTypeInfo

	for i := 0; i < len(spec); {
		t := spec[i]
		if t.Kind == TokIdent && (t.Text == "struct" || t.Text == "union") {
			if aliasSet || sawEnum || sawAggregate || sawVoid || sawChar || sawShort || sawInt || sawLongCount > 0 || sawSigned || sawUnsigned {
				return cTypeInfo{}, false, false, fmt.Errorf("%s cannot combine %s with additional type specifiers", context, t.Text)
			}
			next, aggInfo, err := parseAggregateTypeSpec(spec, i, t.Text, context)
			if err != nil {
				return cTypeInfo{}, false, false, err
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
			if aliasSet || sawAggregate || sawVoid || sawChar || sawShort || sawInt || sawLongCount > 0 || sawSigned || sawUnsigned || sawEnum {
				return cTypeInfo{}, false, false, fmt.Errorf("%s cannot combine enum with additional type specifiers", context)
			}
			next, err := consumeEnumSpecifierTokens(spec, i, context)
			if err != nil {
				return cTypeInfo{}, false, false, err
			}
			sawType = true
			sawEnum = true
			i = next
			continue
		}
		if t.Kind != TokIdent {
			return cTypeInfo{}, false, false, fmt.Errorf("%s has invalid type token %q", context, t.Text)
		}
		if aliasSet {
			switch t.Text {
			case "extern":
				hasExtern = true
				continue
			case "typedef":
				hasTypedef = true
				continue
			case "const", "volatile", "restrict", "inline":
				continue
			}
			return cTypeInfo{}, false, false, fmt.Errorf("%s cannot combine typedef name %q with additional type specifiers", context, t.Text)
		}
		switch t.Text {
		case "extern":
			hasExtern = true
		case "typedef":
			hasTypedef = true
		case "auto", "register", "static":
			// Accepted for parser compatibility; not all storage semantics are modeled yet.
		case "const", "volatile", "restrict", "inline":
			// Qualifiers are currently ignored in this lowering stage.
		case "void":
			if sawEnum || sawAggregate {
				return cTypeInfo{}, false, false, fmt.Errorf("%s cannot combine aggregate/enum with void", context)
			}
			sawType = true
			sawVoid = true
		case "char":
			if sawEnum || sawAggregate {
				return cTypeInfo{}, false, false, fmt.Errorf("%s cannot combine aggregate/enum with char", context)
			}
			sawType = true
			sawChar = true
		case "short":
			if sawEnum || sawAggregate {
				return cTypeInfo{}, false, false, fmt.Errorf("%s cannot combine aggregate/enum with short", context)
			}
			sawType = true
			sawShort = true
		case "int":
			if sawEnum || sawAggregate {
				return cTypeInfo{}, false, false, fmt.Errorf("%s cannot combine aggregate/enum with int", context)
			}
			sawType = true
			sawInt = true
		case "long":
			if sawEnum || sawAggregate {
				return cTypeInfo{}, false, false, fmt.Errorf("%s cannot combine aggregate/enum with long", context)
			}
			sawType = true
			sawLongCount++
			if sawLongCount > 2 {
				return cTypeInfo{}, false, false, fmt.Errorf("%s has invalid long type combination", context)
			}
		case "signed":
			if sawEnum || sawAggregate {
				return cTypeInfo{}, false, false, fmt.Errorf("%s cannot combine aggregate/enum with signed", context)
			}
			sawType = true
			sawSigned = true
		case "unsigned":
			if sawEnum || sawAggregate {
				return cTypeInfo{}, false, false, fmt.Errorf("%s cannot combine aggregate/enum with unsigned", context)
			}
			sawType = true
			sawUnsigned = true
		default:
			if alias, ok := lookupTypedefAlias(t.Text); ok {
				if sawEnum || sawAggregate {
					return cTypeInfo{}, false, false, fmt.Errorf("%s cannot combine aggregate/enum with typedef name %q", context, t.Text)
				}
				if sawType || sawVoid || sawChar || sawShort || sawInt || sawLongCount > 0 || sawSigned || sawUnsigned {
					return cTypeInfo{}, false, false, fmt.Errorf("%s cannot combine typedef name %q with builtin type specifiers", context, t.Text)
				}
				sawType = true
				aliasSet = true
				aliasInfo = alias
				break
			}
			if isUnsupportedCTypeKeyword(t.Text) {
				return cTypeInfo{}, false, false, fmt.Errorf("%s uses unsupported type keyword %q", context, t.Text)
			}
			return cTypeInfo{}, false, false, fmt.Errorf("%s has unsupported type token %q", context, t.Text)
		}
		i++
	}

	if !sawType {
		return cTypeInfo{}, false, false, fmt.Errorf("%s is missing a type specifier", context)
	}
	if aliasSet {
		if aliasInfo.IsVoid && !allowVoid {
			return cTypeInfo{}, false, false, fmt.Errorf("%s cannot use void type here", context)
		}
		aliasInfo.FuncSig = cloneFuncTypeSig(aliasInfo.FuncSig)
		return aliasInfo, hasExtern, hasTypedef, nil
	}
	if sawSigned && sawUnsigned {
		return cTypeInfo{}, false, false, fmt.Errorf("%s cannot combine signed and unsigned", context)
	}
	if sawAggregate {
		return cTypeInfo{
			Kind:             cDeclScalar,
			Base:             cScalarInt,
			OpaqueAggregate:  aggregateOpaque,
			AggregateKeyword: aggregateKeyword,
			AggregateTag:     aggregateTag,
		}, hasExtern, hasTypedef, nil
	}
	if sawEnum {
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarInt}, hasExtern, hasTypedef, nil
	}
	if sawVoid {
		if sawChar || sawShort || sawInt || sawLongCount > 0 || sawSigned || sawUnsigned {
			return cTypeInfo{}, false, false, fmt.Errorf("%s has invalid void type combination", context)
		}
		if !allowVoid {
			return cTypeInfo{}, false, false, fmt.Errorf("%s cannot use void type here", context)
		}
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarInt, IsVoid: true}, hasExtern, hasTypedef, nil
	}
	if sawChar {
		if sawShort || sawInt || sawLongCount > 0 {
			return cTypeInfo{}, false, false, fmt.Errorf("%s has invalid char type combination", context)
		}
		if sawUnsigned {
			return cTypeInfo{Kind: cDeclScalar, Base: cScalarUChar}, hasExtern, hasTypedef, nil
		}
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarChar}, hasExtern, hasTypedef, nil
	}
	if sawShort {
		if sawLongCount > 0 {
			return cTypeInfo{}, false, false, fmt.Errorf("%s cannot combine short and long", context)
		}
		if sawUnsigned {
			return cTypeInfo{Kind: cDeclScalar, Base: cScalarUShort}, hasExtern, hasTypedef, nil
		}
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarShort}, hasExtern, hasTypedef, nil
	}
	if sawLongCount > 0 {
		if sawUnsigned {
			return cTypeInfo{Kind: cDeclScalar, Base: cScalarULong}, hasExtern, hasTypedef, nil
		}
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarLong}, hasExtern, hasTypedef, nil
	}
	if sawUnsigned {
		return cTypeInfo{Kind: cDeclScalar, Base: cScalarUInt}, hasExtern, hasTypedef, nil
	}
	_ = sawInt
	return cTypeInfo{Kind: cDeclScalar, Base: cScalarInt}, hasExtern, hasTypedef, nil
}

func combineTypeAndDeclarator(base cTypeInfo, declKind cDeclKind, declPtrDepth int, declArrayLen int64, allowOpaqueObject bool, context string) (cTypeInfo, error) {
	out := base
	if base.AggregateKeyword != "" && base.Kind != cDeclPointer && declKind == cDeclArray {
		if base.AggregateTag != "" {
			return cTypeInfo{}, fmt.Errorf("%s does not yet support array declarators over %s %q object types", context, base.AggregateKeyword, base.AggregateTag)
		}
		return cTypeInfo{}, fmt.Errorf("%s does not yet support array declarators over aggregate object types", context)
	}
	if base.AggregateKeyword != "" && base.OpaqueAggregate && base.Kind != cDeclPointer && declKind != cDeclPointer && !allowOpaqueObject {
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
		if base.Kind == cDeclPointer {
			return cTypeInfo{}, fmt.Errorf("%s does not support array declarators over pointer typedef bases", context)
		}
		out.Kind = cDeclArray
		out.ArrayLen = declArrayLen
		out.PtrDepth = 0
	case cDeclPointer:
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

func parseFunctionParamList(paramTokens []Token, context string) ([]cDeclKind, []cScalarType, []int, []bool, []string, []string, []*cFuncTypeSig, bool, error) {
	paramTokens = trimTokens(paramTokens)
	if len(paramTokens) == 0 {
		return nil, nil, nil, nil, nil, nil, nil, false, nil
	}
	parts := splitTopLevel(paramTokens, ",")
	if len(parts) == 1 {
		p0 := trimTokens(parts[0])
		if len(p0) > 0 {
			spec, decl, err := splitDeclSpecPrefix(p0, context)
			if err == nil {
				baseInfo, _, _, err := parseScalarTypeSpec(spec, context, true)
				if err == nil {
					_, kind, ptrDepth, arrLen, _, err := parseDeclarator(decl, true)
					if err == nil {
						info, cerr := combineTypeAndDeclarator(baseInfo, kind, ptrDepth, arrLen, false, context)
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
				return nil, nil, nil, nil, nil, nil, nil, false, fmt.Errorf("variadic marker must appear last after at least one named parameter")
			}
			variadic = true
			continue
		}
		pctx := fmt.Sprintf("%s parameter %d", context, i+1)
		spec, decl, err := splitDeclSpecPrefix(p, pctx)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, nil, false, err
		}
		pbaseInfo, _, _, err := parseScalarTypeSpec(spec, pctx, true)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, nil, false, err
		}
		pname, pdeclKind, pdeclPtrDepth, parrLen, pfnSig, err := parseDeclarator(decl, true)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, nil, false, err
		}
		pinfo, err := combineTypeAndDeclarator(pbaseInfo, pdeclKind, pdeclPtrDepth, parrLen, false, pctx)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, nil, false, err
		}
		if pinfo.Kind == cDeclArray {
			// Arrays in parameter lists decay to pointers.
			pinfo.Kind = cDeclPointer
			if pinfo.PtrDepth == 0 {
				pinfo.PtrDepth = 1
			}
		}
		if pinfo.IsVoid && pinfo.Kind == cDeclScalar {
			if pname == "" && i == 0 && len(parts) == 1 {
				// handled above for `void` empty parameter list; keep defensive fallback.
				return nil, nil, nil, nil, nil, nil, nil, false, nil
			}
			return nil, nil, nil, nil, nil, nil, nil, false, fmt.Errorf("%s parameter %q cannot have type void", context, pname)
		}
		if isAggregateObjectType(pinfo) {
			return nil, nil, nil, nil, nil, nil, nil, false, fmt.Errorf("%s does not support %s %q parameters passed by value", context, pinfo.AggregateKeyword, pinfo.AggregateTag)
		}
		if pfnSig != nil {
			pfn := cloneFuncTypeSig(pfnSig)
			pfn.RetKind = pbaseInfo.Kind
			pfn.RetBase = pbaseInfo.Base
			pfn.RetPtrDepth = pbaseInfo.PtrDepth
			pfn.RetIsVoid = pbaseInfo.IsVoid
			pfn.RetOpaque = pbaseInfo.OpaqueAggregate
			pfn.RetAggKeyword = pbaseInfo.AggregateKeyword
			pfn.RetAggTag = pbaseInfo.AggregateTag
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
	return paramKinds, paramBases, paramPtrDepth, paramOpaque, paramAggKey, paramAggTag, paramFuncSigs, variadic, nil
}

func parseDeclarator(tokens []Token, allowAbstract bool) (string, cDeclKind, int, int64, *cFuncTypeSig, error) {
	tokens = trimTokens(tokens)
	if len(tokens) == 0 {
		if allowAbstract {
			return "", cDeclScalar, 0, 0, nil, nil
		}
		return "", cDeclScalar, 0, 0, nil, fmt.Errorf("missing declarator")
	}

	// Support function-pointer declarators like `(*fp)(int)` and `(*)(int)`.
	if fnOpen := trailingFunctionSuffixOpen(tokens); fnOpen >= 0 {
		core := trimTokens(tokens[:fnOpen])
		if len(core) == 0 {
			return "", cDeclScalar, 0, 0, nil, fmt.Errorf("function declarator is missing pointer core")
		}
		name, ptrDepth, err := parseSimplePointerCore(core, allowAbstract)
		if err != nil {
			return "", cDeclScalar, 0, 0, nil, err
		}
		if ptrDepth == 0 {
			return "", cDeclScalar, 0, 0, nil, fmt.Errorf("function declarators are only supported through pointers")
		}
		paramKinds, paramBases, paramPtrDepth, paramOpaque, paramAggKey, paramAggTag, paramFuncSigs, variadic, err := parseFunctionParamList(tokens[fnOpen+1:len(tokens)-1], "function declarator")
		if err != nil {
			return "", cDeclScalar, 0, 0, nil, err
		}
		fnSig := &cFuncTypeSig{
			ParamCount:    len(paramKinds),
			Variadic:      variadic,
			ParamKinds:    paramKinds,
			ParamBases:    paramBases,
			ParamPtrDepth: paramPtrDepth,
			ParamOpaque:   paramOpaque,
			ParamAggKey:   paramAggKey,
			ParamAggTag:   paramAggTag,
			ParamFuncSigs: paramFuncSigs,
		}
		return name, cDeclPointer, ptrDepth, 0, fnSig, nil
	}

	if arrOpen := trailingArraySuffixOpen(tokens); arrOpen >= 0 {
		core := trimTokens(tokens[:arrOpen])
		name, ptrDepth, err := parseSimplePointerCore(core, allowAbstract)
		if err != nil {
			return "", cDeclScalar, 0, 0, nil, err
		}
		if ptrDepth > 0 {
			return "", cDeclScalar, 0, 0, nil, fmt.Errorf("pointer-to-array declarators are not yet supported")
		}
		n, err := parseArrayLength(tokens[arrOpen+1 : len(tokens)-1])
		if err != nil {
			return "", cDeclScalar, 0, 0, nil, err
		}
		return name, cDeclArray, 0, n, nil, nil
	}

	name, ptrDepth, err := parseSimplePointerCore(tokens, allowAbstract)
	if err != nil {
		return "", cDeclScalar, 0, 0, nil, err
	}
	if ptrDepth > 0 {
		return name, cDeclPointer, ptrDepth, 0, nil, nil
	}
	return name, cDeclScalar, 0, 0, nil, nil
}

func parseArrayLength(tokens []Token) (int64, error) {
	tokens = trimTokens(tokens)
	if len(tokens) != 1 || tokens[0].Kind != TokNumber {
		return 0, fmt.Errorf("array bounds must be integer literals")
	}
	n, err := parseCIntLiteral(tokens[0].Text)
	if err != nil {
		return 0, err
	}
	if n <= 0 {
		return 0, fmt.Errorf("array bounds must be positive")
	}
	return n, nil
}

type enumConstParser struct {
	toks   []Token
	pos    int
	lookup map[string]int64
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

func parseEnumConstExprTokens(toks []Token, lookup map[string]int64) (int64, error) {
	p := &enumConstParser{toks: trimTokens(toks), lookup: lookup}
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
			ev, err := parseEnumConstExprTokens(rhs, resolver)
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

func parseDeclItemsWithBase(baseInfo cTypeInfo, hasTypedef bool, rest []Token) ([]cDeclItem, error) {
	parts := splitTopLevel(rest, ",")
	items := make([]cDeclItem, 0, len(parts))
	for _, part := range parts {
		part = trimTokens(part)
		if len(part) == 0 {
			continue
		}

		eqIdx := -1
		depthParen := 0
		depthBracket := 0
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
				case "=":
					if depthParen == 0 && depthBracket == 0 {
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

		name, kind, ptrDepth, arrayLen, fnSig, err := parseDeclarator(lhs, false)
		if err != nil {
			return nil, fmt.Errorf("%s (%s)", err, tokenSliceText(lhs))
		}
		info, err := combineTypeAndDeclarator(baseInfo, kind, ptrDepth, arrayLen, hasTypedef, fmt.Sprintf("declaration of %q", name))
		if err != nil {
			return nil, err
		}
		if fnSig != nil {
			if isAggregateObjectType(baseInfo) {
				return nil, fmt.Errorf("declaration of %q uses function type returning %s %q by value, which is not yet supported", name, baseInfo.AggregateKeyword, baseInfo.AggregateTag)
			}
			sig := cloneFuncTypeSig(fnSig)
			sig.RetKind = baseInfo.Kind
			sig.RetBase = baseInfo.Base
			sig.RetPtrDepth = baseInfo.PtrDepth
			sig.RetIsVoid = baseInfo.IsVoid
			info.FuncSig = sig
		}
		if !hasTypedef && info.IsVoid && info.Kind != cDeclPointer {
			return nil, fmt.Errorf("declaration of %q cannot use void object type", name)
		}
		items = append(items, cDeclItem{
			Name:             name,
			Init:             init,
			Kind:             info.Kind,
			PtrDepth:         info.PtrDepth,
			ArrayLen:         info.ArrayLen,
			IsVoid:           info.IsVoid,
			Base:             info.Base,
			FuncSig:          cloneFuncTypeSig(info.FuncSig),
			OpaqueAggregate:  info.OpaqueAggregate,
			AggregateKeyword: info.AggregateKeyword,
			AggregateTag:     info.AggregateTag,
		})
	}
	return items, nil
}

func parseDeclItems(toks []Token, enumLookup map[string]int64) ([]cDeclItem, map[string]int64, bool, bool, error) {
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
		items, err := parseDeclItemsWithBase(cTypeInfo{Kind: cDeclScalar, Base: cScalarInt}, hasTypedef, rest)
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
	baseInfo, hasExtern, hasTypedef, err := parseScalarTypeSpec(spec, "declaration", true)
	if err != nil {
		return nil, nil, false, false, err
	}
	if len(rest) == 0 {
		if baseInfo.AggregateKeyword != "" && !hasTypedef {
			return nil, nil, hasExtern, hasTypedef, nil
		}
		return nil, nil, false, false, fmt.Errorf("missing declarator in declaration")
	}
	items, err := parseDeclItemsWithBase(baseInfo, hasTypedef, rest)
	if err != nil {
		return nil, nil, false, false, err
	}
	if len(items) == 0 {
		return nil, nil, false, false, fmt.Errorf("empty declaration")
	}
	return items, nil, hasExtern, hasTypedef, nil
}

func isTypeSpecifierKeyword(text string) bool {
	switch text {
	case "void", "char", "short", "int", "long", "signed", "unsigned":
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
	baseInfo, _, _, err := parseScalarTypeSpec(spec, "type name", true)
	if err != nil {
		return cTypeInfo{}, err
	}
	name, kind, ptrDepth, arrayLen, fnSig, err := parseDeclarator(decl, true)
	if err != nil {
		return cTypeInfo{}, err
	}
	if name != "" {
		return cTypeInfo{}, fmt.Errorf("named declarators are not supported in type names (%s)", name)
	}
	info, err := combineTypeAndDeclarator(baseInfo, kind, ptrDepth, arrayLen, true, "type name")
	if err != nil {
		return cTypeInfo{}, err
	}
	if fnSig != nil {
		sig := cloneFuncTypeSig(fnSig)
		sig.RetKind = baseInfo.Kind
		sig.RetBase = baseInfo.Base
		sig.RetPtrDepth = baseInfo.PtrDepth
		sig.RetIsVoid = baseInfo.IsVoid
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

func parseBraceInitializerExprs(init []Token) ([][]Token, error) {
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
			// Allow trailing comma.
			continue
		}
		if p[0].Kind == TokPunct && p[0].Text == "{" {
			return nil, fmt.Errorf("nested initializer lists are not yet supported")
		}
		out = append(out, p)
	}
	return out, nil
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
	if _, exists := cur[name]; exists {
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
		elem := info
		elem.Kind = cDeclScalar
		elem.ArrayLen = 0
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
	_ = base
	return int64(fc.c.target.PtrSize)
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
		if baseType.OpaqueAggregate {
			if diag {
				fc.errorf(fc.sig.File, 0, 0, "member access via '.' on incomplete %s %q", baseType.AggregateKeyword, baseType.AggregateTag)
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
		if baseType.OpaqueAggregate {
			if diag {
				fc.errorf(fc.sig.File, 0, 0, "member access via '->' on incomplete %s %q", baseType.AggregateKeyword, baseType.AggregateTag)
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
			fc.errorf(fc.sig.File, 0, 0, "member access on unknown/incomplete %s %q", baseType.AggregateKeyword, baseType.AggregateTag)
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
	return fc.addLocalDecl(name, cDeclScalar, cScalarInt, 0, int64(fc.c.target.PtrSize), 0, nil, false, "", "", file, line, col)
}

func (fc *funcCompiler) addLocalKind(name string, kind cDeclKind, file string, line int, col int) int {
	ptrDepth := 0
	if kind == cDeclPointer {
		ptrDepth = 1
	}
	return fc.addLocalDecl(name, kind, cScalarInt, ptrDepth, int64(fc.c.target.PtrSize), 0, nil, false, "", "", file, line, col)
}

func (fc *funcCompiler) addLocalTyped(name string, kind cDeclKind, base cScalarType, ptrDepth int, elemStep int64, funcSig *cFuncTypeSig, file string, line int, col int) int {
	return fc.addLocalDecl(name, kind, base, ptrDepth, elemStep, 0, funcSig, false, "", "", file, line, col)
}

func (fc *funcCompiler) addLocalDecl(name string, kind cDeclKind, base cScalarType, ptrDepth int, elemStep int64, arrayLen int64, funcSig *cFuncTypeSig, opaqueAggregate bool, aggregateKeyword string, aggregateTag string, file string, line int, col int) int {
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
	fc.fn.Locals = append(fc.fn.Locals, ir.IRLocal{Name: name, Index: idx})
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
	case NGotoStmt, NLabelStmt:
		fc.errorf(fc.sig.File, n.Line, n.Col, "goto/labels are not yet supported in C lowering")
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
	if body.Kind != NCompoundStmt {
		fc.errorf(fc.sig.File, n.Line, n.Col, "switch body must be a compound statement")
		return
	}

	tmpName := fmt.Sprintf("$switch%d", fc.c.nextLabel())
	switchVal := fc.addLocal(tmpName, fc.sig.File, n.Line, n.Col)
	fc.compileExprText(n.Text, n.Line, n.Col)
	fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: switchVal})

	endLabel := fc.c.nextLabel()
	defaultLabel := endLabel
	hasDefault := false
	caseLabels := make(map[*Node]int)

	for _, st := range body.Children {
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

	for _, st := range body.Children {
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
	for _, st := range body.Children {
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
	for i := int64(0); i < words; i++ {
		elemName := fmt.Sprintf("$%s$obj$%d$%d", name, idx, i)
		elemIdx := fc.addLocal(elemName, file, line, col)
		// Locals are laid out at decreasing stack addresses.
		// Keep base at the last-created slot so +offset addressing stays in-bounds.
		firstElem = elemIdx
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: elemIdx})
	}
	if firstElem < 0 {
		fc.errorf(file, line, col, "aggregate declaration requires non-zero size: %s", name)
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
		return
	}
	fc.emit(ir.Inst{Op: ir.OP_LOCAL_ADDR, Arg: firstElem})
	fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
	fc.emitAggregateObjectInitializer(name, idx, false, info, init, file, line, col)
}

func (fc *funcCompiler) emitAggregateObjectInitializer(name string, ptrIdx int, ptrIsGlobal bool, info cTypeInfo, init []Token, file string, line int, col int) {
	if len(init) == 0 {
		return
	}
	agg, ok := fc.lookupAggregate(info.AggregateKeyword, info.AggregateTag)
	if !ok || len(agg.Fields) == 0 {
		fc.errorf(file, line, col, "aggregate object initializer for %s %q uses unknown/incomplete type", info.AggregateKeyword, name)
		return
	}
	initElems, err := parseBraceInitializerExprs(init)
	if err != nil {
		fc.errorf(file, line, col, "invalid aggregate initializer for %s: %v", name, err)
		return
	}
	if agg.IsUnion && len(initElems) > 1 {
		fc.errorf(file, line, col, "union initializer for %s may only initialize the first member for now", name)
		return
	}
	if len(initElems) > len(agg.Fields) {
		fc.errorf(file, line, col, "too many aggregate initializer elements for %s (%d > %d)", name, len(initElems), len(agg.Fields))
		return
	}
	for i, initExpr := range initElems {
		field := agg.Fields[i]
		if field.Type.Kind == cDeclArray {
			fc.errorf(file, line, col, "aggregate initializer for %s member %q does not yet support array fields", name, field.Name)
			continue
		}
		if field.Type.Kind == cDeclScalar && field.Type.AggregateKeyword != "" && field.Type.AggregateTag != "" && field.Type.PtrDepth == 0 {
			fc.errorf(file, line, col, "aggregate initializer for %s member %q does not yet support nested aggregate values", name, field.Name)
			continue
		}
		width, _, ok := fc.typeInfoSizeAlign(field.Type)
		if !ok || width <= 0 {
			fc.errorf(file, line, col, "aggregate initializer for %s member %q has unsupported type", name, field.Name)
			continue
		}
		fc.emitExprTokens(file, line, col, initExpr)
		fc.emitCastToType(field.Type)
		if ptrIsGlobal {
			fc.emit(ir.Inst{Op: ir.OP_GLOBAL_GET, Arg: ptrIdx})
		} else {
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: ptrIdx})
		}
		if field.Offset != 0 {
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: field.Offset})
			fc.emit(ir.Inst{Op: ir.OP_ADD})
		}
		fc.emit(ir.Inst{Op: ir.OP_STORE, Arg: int(width)})
	}
}

func (fc *funcCompiler) compileDeclStmt(n *Node) {
	toks, err := lexSnippet(fc.sig.File, n.Text)
	if err != nil {
		fc.errorf(fc.sig.File, n.Line, n.Col, "invalid declaration: %v", err)
		return
	}
	items, enumConsts, hasExtern, hasTypedef, err := parseDeclItems(toks, fc.enumLookupMap())
	if err != nil {
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
			if it.Kind == cDeclArray {
				fc.errorf(fc.sig.File, n.Line, n.Col, "typedef array declarations are not yet supported: %s", it.Name)
				continue
			}
			fc.addLocalTypedef(it.Name, cTypeInfo{
				Kind:             it.Kind,
				PtrDepth:         it.PtrDepth,
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
		elemStep := fc.pointerElemStep(it.Kind, it.PtrDepth, it.Base, it.IsVoid, it.OpaqueAggregate, it.AggregateKeyword, it.AggregateTag)
		if it.Kind == cDeclPointer && it.PtrDepth == 1 && isStringLiteralExpr(it.Init) {
			elemStep = 1
		}
		idx := fc.addLocalDecl(it.Name, it.Kind, it.Base, it.PtrDepth, elemStep, it.ArrayLen, it.FuncSig, it.OpaqueAggregate, it.AggregateKeyword, it.AggregateTag, fc.sig.File, n.Line, n.Col)
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
			firstElem := -1
			for i := int64(0); i < it.ArrayLen; i++ {
				elemName := fmt.Sprintf("$%s$elem$%d$%d", it.Name, idx, i)
				elemIdx := fc.addLocal(elemName, fc.sig.File, n.Line, n.Col)
				// Locals are laid out at decreasing stack addresses.
				// Keep base at the last-created slot so +index addressing stays in-bounds.
				firstElem = elemIdx
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: elemIdx})
			}
			if firstElem < 0 {
				fc.errorf(fc.sig.File, n.Line, n.Col, "array declaration requires positive bounds: %s", it.Name)
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
				continue
			}
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_ADDR, Arg: firstElem})
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
			initElems, err := parseArrayInitializerExprs(it.Init, it.ArrayLen)
			if err != nil {
				fc.errorf(fc.sig.File, n.Line, n.Col, "invalid array initializer for %s: %v", it.Name, err)
				continue
			}
			for i, initExpr := range initElems {
				fc.emitExprTokens(fc.sig.File, n.Line, n.Col, initExpr)
				fc.emitCastToType(cTypeInfo{Kind: cDeclScalar, Base: it.Base})
				fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: idx})
				if i > 0 {
					fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(i * fc.c.target.PtrSize)})
					fc.emit(ir.Inst{Op: ir.OP_ADD})
				}
				fc.emit(ir.Inst{Op: ir.OP_STORE, Arg: int(fc.scalarSize(it.Base))})
			}
			continue
		}
		if len(it.Init) == 0 {
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
			continue
		}
		fc.emitExprTokens(fc.sig.File, n.Line, n.Col, it.Init)
		if it.Kind == cDeclScalar {
			fc.emitCastToType(cTypeInfo{Kind: cDeclScalar, Base: it.Base})
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
		if initToks[0].Kind == TokIdent && isDeclarationKeyword(initToks[0]) {
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
	if fc.sig.RetCount == 0 {
		if strings.TrimSpace(n.Text) != "" {
			fc.compileExprText(n.Text, n.Line, n.Col)
			fc.emit(ir.Inst{Op: ir.OP_DROP})
		}
		fc.emit(ir.Inst{Op: ir.OP_RETURN, Arg: 0})
		return
	}
	if strings.TrimSpace(n.Text) == "" {
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
	} else {
		fc.compileExprText(n.Text, n.Line, n.Col)
	}
	fc.emitCastToType(cTypeInfo{
		Kind:     fc.sig.RetKind,
		PtrDepth: fc.sig.RetPtrDepth,
		Base:     fc.sig.RetBase,
	})
	fc.emit(ir.Inst{Op: ir.OP_RETURN, Arg: 1})
}

func (fc *funcCompiler) compileDeclTokens(file string, n *Node, toks []Token) {
	items, enumConsts, hasExtern, hasTypedef, err := parseDeclItems(toks, fc.enumLookupMap())
	if err != nil {
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
			if it.Kind == cDeclArray {
				fc.errorf(file, n.Line, n.Col, "typedef array declarations are not yet supported: %s", it.Name)
				continue
			}
			fc.addLocalTypedef(it.Name, cTypeInfo{
				Kind:             it.Kind,
				PtrDepth:         it.PtrDepth,
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
		elemStep := fc.pointerElemStep(it.Kind, it.PtrDepth, it.Base, it.IsVoid, it.OpaqueAggregate, it.AggregateKeyword, it.AggregateTag)
		if it.Kind == cDeclPointer && it.PtrDepth == 1 && isStringLiteralExpr(it.Init) {
			elemStep = 1
		}
		idx := fc.addLocalDecl(it.Name, it.Kind, it.Base, it.PtrDepth, elemStep, it.ArrayLen, it.FuncSig, it.OpaqueAggregate, it.AggregateKeyword, it.AggregateTag, file, n.Line, n.Col)
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
			firstElem := -1
			for i := int64(0); i < it.ArrayLen; i++ {
				elemName := fmt.Sprintf("$%s$elem$%d$%d", it.Name, idx, i)
				elemIdx := fc.addLocal(elemName, file, n.Line, n.Col)
				// Locals are laid out at decreasing stack addresses.
				// Keep base at the last-created slot so +index addressing stays in-bounds.
				firstElem = elemIdx
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: elemIdx})
			}
			if firstElem < 0 {
				fc.errorf(file, n.Line, n.Col, "array declaration requires positive bounds: %s", it.Name)
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
				continue
			}
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_ADDR, Arg: firstElem})
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
			initElems, err := parseArrayInitializerExprs(it.Init, it.ArrayLen)
			if err != nil {
				fc.errorf(file, n.Line, n.Col, "invalid array initializer for %s: %v", it.Name, err)
				continue
			}
			for i, initExpr := range initElems {
				fc.emitExprTokens(file, n.Line, n.Col, initExpr)
				fc.emitCastToType(cTypeInfo{Kind: cDeclScalar, Base: it.Base})
				fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: idx})
				if i > 0 {
					fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(i * fc.c.target.PtrSize)})
					fc.emit(ir.Inst{Op: ir.OP_ADD})
				}
				fc.emit(ir.Inst{Op: ir.OP_STORE, Arg: int(fc.scalarSize(it.Base))})
			}
			continue
		}
		if len(it.Init) == 0 {
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
			continue
		}
		fc.emitExprTokens(file, n.Line, n.Col, it.Init)
		if it.Kind == cDeclScalar {
			fc.emitCastToType(cTypeInfo{Kind: cDeclScalar, Base: it.Base})
		}
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
	}
}

func (fc *funcCompiler) emitCondJumpFalse(file string, line int, col int, cond []Token, falseLabel int) {
	if len(cond) == 0 {
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 1})
	} else {
		fc.emitExprTokens(file, line, col, cond)
	}
	fc.emit(ir.Inst{Op: ir.OP_JMP_IF_NOT, Arg: falseLabel})
}

func (fc *funcCompiler) emitCondJumpTrue(file string, line int, col int, cond []Token, trueLabel int) {
	if len(cond) == 0 {
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 1})
	} else {
		fc.emitExprTokens(file, line, col, cond)
	}
	fc.emit(ir.Inst{Op: ir.OP_JMP_IF, Arg: trueLabel})
}

func (fc *funcCompiler) compileExprText(text string, line int, col int) {
	toks, err := lexSnippet(fc.sig.File, text)
	if err != nil {
		fc.errorf(fc.sig.File, line, col, "invalid expression: %v", err)
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		return
	}
	fc.emitExprTokens(fc.sig.File, line, col, toks)
}

func (fc *funcCompiler) emitExprTokens(file string, line int, col int, toks []Token) {
	ep := &cExprParser{fc: fc, file: file, line: line, col: col, toks: trimTokens(toks)}
	ex := ep.parseExpression()
	if ex == nil {
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		return
	}
	if ep.pos < len(ep.toks) {
		got := ep.toks[ep.pos]
		fc.errorf(file, line, col, "unexpected token in expression: %q", got.Text)
	}
	fc.emitExpr(ex)
}

func (fc *funcCompiler) emitIndexAddr(base *expr, index *expr) {
	step := fc.exprPointerStep(base)
	fc.emitExpr(base)
	fc.emitExpr(index)
	fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: step})
	fc.emit(ir.Inst{Op: ir.OP_MUL})
	fc.emit(ir.Inst{Op: ir.OP_ADD})
}

func (fc *funcCompiler) emitAddressOf(ex *expr) bool {
	if ex == nil {
		return false
	}
	switch ex.kind {
	case exprVar:
		if b, ok := fc.lookupLocalBinding(ex.name); ok {
			if isAggregateObjectDecl(b.Kind, b.PtrDepth, b.OpaqueAggregate, b.AggregateKeyword, b.AggregateTag) {
				fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: b.Index})
			} else {
				fc.emit(ir.Inst{Op: ir.OP_LOCAL_ADDR, Arg: b.Index})
			}
			return true
		}
		if idx, ok := fc.lookupGlobal(ex.name); ok {
			kind, _ := fc.lookupGlobalKind(ex.name)
			ptrDepth, _ := fc.lookupGlobalPtrDepth(ex.name)
			opaque, aggKey, aggTag := fc.lookupGlobalOpaqueAggregate(ex.name)
			if isAggregateObjectDecl(kind, ptrDepth, opaque, aggKey, aggTag) {
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
	default:
		return false
	}
}

func (fc *funcCompiler) longSize() int64 {
	if fc.c.target.GOOS == "windows" {
		return 4
	}
	return int64(fc.c.target.PtrSize)
}

func (fc *funcCompiler) scalarSize(base cScalarType) int64 {
	switch base {
	case cScalarChar, cScalarUChar:
		return 1
	case cScalarShort, cScalarUShort:
		return 2
	case cScalarInt, cScalarUInt:
		return 4
	case cScalarLong, cScalarULong:
		return fc.longSize()
	default:
		return 4
	}
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
		return info.ArrayLen * fc.typeScalarWidth(info)
	default:
		if info.IsVoid {
			return 1
		}
		return fc.typeScalarWidth(info)
	}
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
	default:
		return ""
	}
}

func (fc *funcCompiler) emitCastToType(info cTypeInfo) {
	switch info.Kind {
	case cDeclPointer:
		fc.emit(ir.Inst{Op: ir.OP_CONVERT, Name: "uintptr"})
		return
	case cDeclArray:
		return
	case cDeclScalar:
		if info.IsVoid {
			return
		}
		name := fc.convertNameForScalar(info.Base)
		if name != "" {
			fc.emit(ir.Inst{Op: ir.OP_CONVERT, Name: name})
		}
	}
}

func (fc *funcCompiler) varTypeInfo(name string) (cTypeInfo, bool) {
	if b, ok := fc.lookupLocalBinding(name); ok {
		return cTypeInfo{
			Kind:             b.Kind,
			PtrDepth:         b.PtrDepth,
			ArrayLen:         b.ArrayLen,
			Base:             b.Base,
			FuncSig:          cloneFuncTypeSig(b.FuncSig),
			OpaqueAggregate:  b.OpaqueAggregate,
			AggregateKeyword: b.AggregateKeyword,
			AggregateTag:     b.AggregateTag,
		}, true
	}
	if kind, ok := fc.lookupGlobalKind(name); ok {
		base, _ := fc.lookupGlobalBase(name)
		ptrDepth, _ := fc.lookupGlobalPtrDepth(name)
		n, _ := fc.lookupGlobalArrayLen(name)
		fsig, _ := fc.lookupGlobalFuncSig(name)
		opaque, aggKey, aggTag := fc.lookupGlobalOpaqueAggregate(name)
		return cTypeInfo{
			Kind:             kind,
			PtrDepth:         ptrDepth,
			ArrayLen:         n,
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
	i := 0
	for i < len(extraLocals) {
		elemName := fmt.Sprintf("$va_pack_elem$%d$%d", fc.c.nextLabel(), i)
		elemIdx := fc.addLocal(elemName, fc.sig.File, 0, 0)
		// Locals are laid out at decreasing stack addresses; keep base at
		// the last-created slot so +index addressing stays in-bounds.
		firstElem = elemIdx
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: elemIdx})
		i++
	}
	if firstElem < 0 {
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		return
	}
	ptrLocal := fc.addLocal(fmt.Sprintf("$va_pack_ptr$%d", fc.c.nextLabel()), fc.sig.File, 0, 0)
	fc.emit(ir.Inst{Op: ir.OP_LOCAL_ADDR, Arg: firstElem})
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
		return nil, false
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
	case exprStringLit:
		return cTypeInfo{Kind: cDeclPointer, PtrDepth: 1, Base: cScalarChar}, true
	case exprVar:
		return fc.varTypeInfo(ex.name)
	case exprAssign:
		return fc.exprTypeInfo(ex.left)
	case exprUnary:
		switch ex.op {
		case "&":
			if t, ok := fc.exprTypeInfo(ex.left); ok {
				if t.Kind == cDeclPointer {
					t.PtrDepth++
				} else if t.Kind == cDeclArray {
					t.Kind = cDeclPointer
					t.PtrDepth = 1
					t.ArrayLen = 0
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
					t.Kind = cDeclScalar
					t.PtrDepth = 0
					t.ArrayLen = 0
					t.FuncSig = nil
					return t, true
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
		default:
			return cTypeInfo{Kind: cDeclScalar, Base: cScalarInt}, true
		}
	case exprPostfix:
		if ex.op == "++" || ex.op == "--" {
			return fc.exprTypeInfo(ex.left)
		}
	case exprBinary:
		if ex.op == "+" || ex.op == "-" {
			if t, ok := fc.exprTypeInfo(ex.left); ok && (t.Kind == cDeclPointer || t.Kind == cDeclArray) {
				if t.Kind == cDeclArray {
					t.Kind = cDeclPointer
					if t.PtrDepth == 0 {
						t.PtrDepth = 1
					}
				}
				return t, true
			}
			if t, ok := fc.exprTypeInfo(ex.right); ok && (t.Kind == cDeclPointer || t.Kind == cDeclArray) {
				if t.Kind == cDeclArray {
					t.Kind = cDeclPointer
					if t.PtrDepth == 0 {
						t.PtrDepth = 1
					}
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
			if t.Kind == cDeclPointer || t.Kind == cDeclArray {
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
			if sig.RetCount == 0 {
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
			if ex.typeInfo.PtrDepth > 1 {
				return int64(fc.c.target.PtrSize)
			}
			if ex.typeInfo.IsVoid {
				return 1
			}
			if ex.typeInfo.AggregateKeyword != "" && ex.typeInfo.AggregateTag != "" && !ex.typeInfo.OpaqueAggregate {
				if agg, ok := fc.lookupAggregate(ex.typeInfo.AggregateKeyword, ex.typeInfo.AggregateTag); ok && len(agg.Fields) > 0 {
					return agg.Size
				}
			}
			return fc.scalarSize(ex.typeInfo.Base)
		}
	case exprCall:
		if sig, ok := fc.resolveDirectCallSig(ex); ok && sig.RetKind == cDeclPointer {
			if sig.RetPtrDepth > 1 {
				return int64(fc.c.target.PtrSize)
			}
			return fc.scalarSize(sig.RetBase)
		}
		if t, ok := fc.exprTypeInfo(ex.left); ok && t.Kind == cDeclPointer && t.PtrDepth == 1 && t.FuncSig != nil && t.FuncSig.RetKind == cDeclPointer {
			if t.FuncSig.RetPtrDepth > 1 {
				return int64(fc.c.target.PtrSize)
			}
			if t.FuncSig.RetIsVoid {
				return 1
			}
			return fc.scalarSize(t.FuncSig.RetBase)
		}
	case exprMember:
		if field, ok := fc.resolveMemberField(ex, false); ok && field.Type.Kind == cDeclPointer {
			if field.Type.PtrDepth > 1 {
				return int64(fc.c.target.PtrSize)
			}
			if field.Type.IsVoid {
				return 1
			}
			if field.Type.AggregateKeyword != "" && field.Type.AggregateTag != "" && !field.Type.OpaqueAggregate {
				if agg, ok := fc.lookupAggregate(field.Type.AggregateKeyword, field.Type.AggregateTag); ok && len(agg.Fields) > 0 {
					return agg.Size
				}
			}
			return fc.scalarSize(field.Type.Base)
		}
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
		return int(fc.scalarSize(t.Base))
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
		a.RetOpaque != b.RetOpaque ||
		a.RetAggKeyword != b.RetAggKeyword ||
		a.RetAggTag != b.RetAggTag {
		return false
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
		want.RetIsVoid != (sig.RetCount == 0) ||
		want.RetOpaque != sig.RetOpaque ||
		want.RetAggKeyword != sig.RetAggKeyword ||
		want.RetAggTag != sig.RetAggTag {
		return false
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

func (fc *funcCompiler) checkCallArgsByType(calleeName string, paramCount int, variadic bool, paramKinds []cDeclKind, paramFuncSigs []*cFuncTypeSig, call *expr) bool {
	if call == nil {
		return false
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
			fc.errorf(fc.sig.File, 0, 0, "argument %d of %q expects pointer value", i+1, calleeName)
			ok = false
			continue
		}
		if gotPtr {
			fc.errorf(fc.sig.File, 0, 0, "argument %d of %q expects scalar value", i+1, calleeName)
			ok = false
		}
	}
	return ok
}

func (fc *funcCompiler) checkCallArgs(sig *cFuncSig, call *expr) bool {
	if sig == nil || call == nil {
		return false
	}
	return fc.checkCallArgsByType(sig.Name, sig.ParamCount, sig.Variadic, sig.ParamKinds, sig.ParamFuncSigs, call)
}

func (fc *funcCompiler) emitExpr(ex *expr) {
	if ex == nil {
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		return
	}
	switch ex.kind {
	case exprIntLit:
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: ex.intVal})
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
		if idx, ok := fc.lookupGlobal(ex.name); ok {
			fc.emit(ir.Inst{Op: ir.OP_GLOBAL_GET, Arg: idx})
			return
		}
		if v, ok := fc.lookupEnumConst(ex.name); ok {
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: v})
			return
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
			fc.errorf(fc.sig.File, 0, 0, "aggregate assignment is not yet supported")
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			return
		}
		fc.emitExpr(ex.right)
		if t, ok := fc.exprTypeInfo(ex.left); ok {
			fc.emitCastToType(t)
		}
		fc.emit(ir.Inst{Op: ir.OP_DUP})
		if !fc.emitAddressOf(ex.left) {
			fc.errorf(fc.sig.File, 0, 0, "left-hand side of assignment is not assignable")
			fc.emit(ir.Inst{Op: ir.OP_DROP})
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			return
		}
		fc.emit(ir.Inst{Op: ir.OP_STORE, Arg: fc.exprLValueWidth(ex.left)})
	case exprUnary:
		if ex.op == "++" || ex.op == "--" {
			if ex.left == nil || ex.left.kind != exprVar {
				fc.errorf(fc.sig.File, 0, 0, "%s requires variable operand", ex.op)
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				return
			}
			name := ex.left.name
			if t, ok := fc.varTypeInfo(name); ok && isAggregateObjectType(t) {
				fc.errorf(fc.sig.File, 0, 0, "%s on aggregate object %q is not yet supported", ex.op, name)
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				return
			}
			step := int64(1)
			if fc.exprIsPointer(ex.left) {
				step = fc.exprPointerStep(ex.left)
			}
			if idx, ok := fc.lookupLocal(name); ok {
				fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: idx})
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: step})
				if ex.op == "++" {
					fc.emit(ir.Inst{Op: ir.OP_ADD})
				} else {
					fc.emit(ir.Inst{Op: ir.OP_SUB})
				}
				fc.emit(ir.Inst{Op: ir.OP_DUP})
				fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
				return
			}
			if idx, ok := fc.lookupGlobal(name); ok {
				fc.emit(ir.Inst{Op: ir.OP_GLOBAL_GET, Arg: idx})
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: step})
				if ex.op == "++" {
					fc.emit(ir.Inst{Op: ir.OP_ADD})
				} else {
					fc.emit(ir.Inst{Op: ir.OP_SUB})
				}
				fc.emit(ir.Inst{Op: ir.OP_DUP})
				fc.emit(ir.Inst{Op: ir.OP_GLOBAL_SET, Arg: idx})
				return
			}
			fc.errorf(fc.sig.File, 0, 0, "%s on undeclared variable %q", ex.op, name)
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
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
			fc.emit(ir.Inst{Op: ir.OP_NEG})
		case "*":
			if t, ok := fc.exprTypeInfo(ex.left); ok && t.Kind == cDeclPointer && t.PtrDepth == 1 && t.FuncSig != nil {
				// Function pointer dereference in value context remains a function designator.
				break
			}
			fc.emit(ir.Inst{Op: ir.OP_LOAD, Arg: fc.exprDerefWidth(ex.left)})
		case "!":
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			fc.emit(ir.Inst{Op: ir.OP_EQ})
		case "~":
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: -1})
			fc.emit(ir.Inst{Op: ir.OP_XOR})
		default:
			fc.errorf(fc.sig.File, 0, 0, "unsupported unary operator %q", ex.op)
		}
	case exprPostfix:
		if ex.left == nil || ex.left.kind != exprVar {
			fc.errorf(fc.sig.File, 0, 0, "%s requires variable operand", ex.op)
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			return
		}
		name := ex.left.name
		if t, ok := fc.varTypeInfo(name); ok && isAggregateObjectType(t) {
			fc.errorf(fc.sig.File, 0, 0, "%s on aggregate object %q is not yet supported", ex.op, name)
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			return
		}
		var idx int
		var isGlobal bool
		if v, ok := fc.lookupLocal(name); ok {
			idx = v
		} else if v, ok := fc.lookupGlobal(name); ok {
			idx = v
			isGlobal = true
		} else {
			fc.errorf(fc.sig.File, 0, 0, "%s on undeclared variable %q", ex.op, name)
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			return
		}
		if isGlobal {
			fc.emit(ir.Inst{Op: ir.OP_GLOBAL_GET, Arg: idx})
		} else {
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: idx})
		}
		fc.emit(ir.Inst{Op: ir.OP_DUP})
		step := int64(1)
		if fc.exprIsPointer(ex.left) {
			step = fc.exprPointerStep(ex.left)
		}
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: step})
		if ex.op == "++" {
			fc.emit(ir.Inst{Op: ir.OP_ADD})
		} else {
			fc.emit(ir.Inst{Op: ir.OP_SUB})
		}
		if isGlobal {
			fc.emit(ir.Inst{Op: ir.OP_GLOBAL_SET, Arg: idx})
		} else {
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
		}
	case exprBinary:
		if ex.op == "&&" || ex.op == "||" {
			fc.emitLogicalExpr(ex)
			return
		}
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
		fc.emit(ir.Inst{Op: ir.OP_LOAD, Arg: fc.exprDerefWidth(ex.left)})
	case exprMember:
		field, ok := fc.resolveMemberField(ex, true)
		if !ok || !fc.emitMemberAddress(ex, true) {
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			break
		}
		if field.Type.Kind == cDeclScalar && field.Type.AggregateKeyword != "" && field.Type.AggregateTag != "" && field.Type.PtrDepth == 0 {
			fc.errorf(fc.sig.File, 0, 0, "aggregate-valued member expression is not yet supported")
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			break
		}
		fc.emit(ir.Inst{Op: ir.OP_LOAD, Arg: fc.exprLValueWidth(ex)})
	case exprCall:
		if fc.emitBuiltinVariadicCall(ex) {
			return
		}
		if sig, ok := fc.resolveDirectCallSig(ex); ok {
			if !fc.checkCallArgs(sig, ex) {
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				return
			}
			if sig.Variadic && sig.Defined {
				argLocals := make([]int, 0, len(ex.args))
				i := 0
				for i < len(ex.args) {
					fc.emitExpr(ex.args[i])
					argTmp := fc.addLocal(fmt.Sprintf("$call_arg$%d$%d", fc.c.nextLabel(), i), fc.sig.File, 0, 0)
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
			for _, a := range ex.args {
				fc.emitExpr(a)
			}
			if !sig.Defined {
				if fc.c.target.Backend == "c" {
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
			if !fc.checkCallArgsByType("<indirect>", indirectSig.ParamCount, indirectSig.Variadic, indirectSig.ParamKinds, indirectSig.ParamFuncSigs, ex) {
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				return
			}
		}
		calleeTmp := fc.addLocal(fmt.Sprintf("$call_target$%d", fc.c.nextLabel()), fc.sig.File, 0, 0)
		fc.emitCallTargetValue(ex.left)
		fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: calleeTmp})

		argLocals := make([]int, 0, len(ex.args))
		for i, a := range ex.args {
			fc.emitExpr(a)
			argTmp := fc.addLocal(fmt.Sprintf("$call_arg$%d$%d", fc.c.nextLabel(), i), fc.sig.File, 0, 0)
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
				if fc.c.target.Backend == "c" {
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
		fc.emitExpr(ex.left)
		fc.emitCastToType(ex.typeInfo)
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
		fc.emitExpr(ex.left)
		fc.emit(ir.Inst{Op: ir.OP_JMP_IF_NOT, Arg: falseLabel})
		fc.emitExpr(ex.right)
		fc.emit(ir.Inst{Op: ir.OP_JMP_IF_NOT, Arg: falseLabel})
		fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: trueLabel})
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 1})
		fc.emit(ir.Inst{Op: ir.OP_JMP, Arg: endLabel})
		fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: falseLabel})
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		fc.emit(ir.Inst{Op: ir.OP_LABEL, Arg: endLabel})
		return
	}
	fc.emitExpr(ex.left)
	fc.emit(ir.Inst{Op: ir.OP_JMP_IF, Arg: trueLabel})
	fc.emitExpr(ex.right)
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
	exprStringLit
	exprVar
	exprAssign
	exprUnary
	exprPostfix
	exprBinary
	exprIndex
	exprMember
	exprCall
	exprCast
	exprSizeof
)

type expr struct {
	kind exprKind
	op   string

	intVal int64
	strVal string
	name   string

	left  *expr
	right *expr
	args  []*expr

	member string

	typeInfo cTypeInfo
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
	for _, t := range tokens {
		if t.Kind != TokIdent {
			continue
		}
		if isTypeSpecifierKeyword(t.Text) || isUnsupportedCTypeKeyword(t.Text) || isDeclarationKeyword(t) {
			return true
		}
		if _, ok := lookupTypedefAlias(t.Text); ok {
			return true
		}
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
	info, err := parseCTypeInfo(inner)
	if err != nil {
		if looksLikeTypeNameTokens(inner) {
			p.errorf("%v", err)
			return cTypeInfo{Kind: cDeclScalar}, end - start + 1, true
		}
		return cTypeInfo{}, 0, false
	}
	if !allowArray && info.Kind == cDeclArray {
		return cTypeInfo{}, 0, false
	}
	return info, end - start + 1, true
}

func (p *cExprParser) parseExpression() *expr {
	return p.parseAssignment()
}

func (p *cExprParser) parseAssignment() *expr {
	lhs := p.parseLogicalOr()
	if lhs == nil {
		return nil
	}
	if p.matchPunct("=") {
		rhs := p.parseAssignment()
		if rhs == nil {
			rhs = &expr{kind: exprIntLit, intVal: 0}
		}
		return &expr{kind: exprAssign, left: lhs, right: rhs}
	}
	return lhs
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
	if info, consumed, ok := p.tryParseParenType(false); ok {
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
					a := p.parseExpression()
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

func parseCIntLiteral(text string) (int64, error) {
	s := text
	if s == "" {
		return 0, fmt.Errorf("empty literal")
	}
	if strings.Contains(s, ".") || strings.Contains(s, "e") || strings.Contains(s, "E") || strings.Contains(s, "p") || strings.Contains(s, "P") {
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
