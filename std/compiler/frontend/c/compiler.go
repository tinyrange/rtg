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
	Name       string
	IRName     string
	RetCount   int
	ParamCount int
	ParamNames []string
	ParamKinds []cDeclKind
	Defined    bool
	Body       *Node
	File       string
	Line       int
	Col        int
}

type cDeclItem struct {
	Name     string
	Init     []Token
	Kind     cDeclKind
	ArrayLen int64
}

type cGlobalInit struct {
	Name      string
	Index     int
	Kind      cDeclKind
	ArrayBase int
	ArrayLen  int64
	Init      []Token
	File      string
	Line      int
	Col       int
	IRName    string
}

type cDeclKind int

const (
	cDeclScalar cDeclKind = iota
	cDeclPointer
	cDeclArray
)

type cLocalBinding struct {
	Index int
	Kind  cDeclKind
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
		funcs:        make(map[string]*cFuncSig),
		globalIndex:  make(map[string]int),
		globalKind:   make(map[string]cDeclKind),
		nextLabelSeq: 1,
	}

	c.collectTopLevel()
	if len(c.errors) > 0 {
		return nil, c.errors
	}

	c.emitGlobalInit()
	for _, sig := range c.funcOrder {
		if !sig.Defined {
			continue
		}
		c.compileFunction(sig)
	}
	c.emitEntryWrapper()

	if len(c.errors) > 0 {
		return nil, c.errors
	}
	return c.irmod, nil
}

type compiler struct {
	target *common.Target
	units  []Unit
	irmod  *ir.IRModule

	errors []string

	funcs     map[string]*cFuncSig
	funcOrder []*cFuncSig

	globalIndex map[string]int
	globalKind  map[string]cDeclKind
	globalInits []cGlobalInit

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
		prev.ParamCount = sig.ParamCount
		prev.ParamNames = append([]string{}, sig.ParamNames...)
		prev.ParamKinds = append([]cDeclKind{}, sig.ParamKinds...)
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

	// Function prototype.
	if hasTopLevelPunct(toks, "(") {
		sig, err := parseFunctionSignature(file, n.Line, n.Col, toks)
		if err != nil {
			// Keep prototype handling permissive; many unsupported declarations are harmless.
			return
		}
		if _, ok := c.funcs[sig.Name]; !ok {
			sig.Defined = false
			c.funcs[sig.Name] = sig
			c.funcOrder = append(c.funcOrder, sig)
		}
		return
	}

	items, hasExtern, err := parseDeclItems(toks)
	if err != nil {
		c.errorf(file, n.Line, n.Col, "%v", err)
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
		if it.Kind == cDeclArray {
			if len(it.Init) > 0 {
				c.errorf(file, n.Line, n.Col, "array initializers are not yet supported: %s", it.Name)
				continue
			}
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
				ArrayBase: base,
				ArrayLen:  it.ArrayLen,
				Init:      nil,
				File:      file,
				Line:      n.Line,
				Col:       n.Col,
				IRName:    irName,
			})
			continue
		}
		if len(it.Init) > 0 {
			c.globalInits = append(c.globalInits, cGlobalInit{
				Name:     it.Name,
				Index:    idx,
				Kind:     it.Kind,
				ArrayLen: it.ArrayLen,
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
		c:      c,
		sig:    &cFuncSig{Name: "c.init$globals", IRName: "c.init$globals", RetCount: 0},
		fn:     f,
		scopes: []map[string]cLocalBinding{{}},
	}
	for _, g := range c.globalInits {
		if g.Kind == cDeclArray {
			fc.emit(ir.Inst{Op: ir.OP_GLOBAL_ADDR, Arg: g.ArrayBase})
			fc.emit(ir.Inst{Op: ir.OP_GLOBAL_SET, Arg: g.Index})
			continue
		}
		fc.emitExprTokens(g.File, g.Line, g.Col, g.Init)
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

	f := &ir.IRFunc{Name: sig.IRName, Params: sig.ParamCount, RetCount: sig.RetCount}
	fc := &funcCompiler{
		c:      c,
		sig:    sig,
		fn:     f,
		scopes: []map[string]cLocalBinding{{}},
	}
	for i, p := range sig.ParamNames {
		name := p
		if name == "" {
			name = fmt.Sprintf("$p%d", i)
		}
		kind := cDeclScalar
		if i < len(sig.ParamKinds) {
			kind = sig.ParamKinds[i]
		}
		fc.addLocalKind(name, kind, sig.File, sig.Line, sig.Col)
	}

	fc.compileCompound(sig.Body, true)
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
	f := &ir.IRFunc{Name: "main.main", Params: 0, RetCount: 0}
	for i := 0; i < mainSig.ParamCount; i++ {
		f.Code = append(f.Code, ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
	}
	f.Code = append(f.Code, ir.Inst{Op: ir.OP_CALL, Name: mainSig.IRName, Arg: mainSig.ParamCount})
	for i := 0; i < mainSig.RetCount; i++ {
		f.Code = append(f.Code, ir.Inst{Op: ir.OP_DROP})
	}
	f.Code = append(f.Code, ir.Inst{Op: ir.OP_RETURN, Arg: 0})
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
	depthParen := 0
	depthBracket := 0
	depthBrace := 0
	for _, t := range tokens {
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
		default:
			if t.Text == punct && depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
				return true
			}
		}
	}
	return false
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

	nameIdx := -1
	for i := lpar - 1; i >= 0; i-- {
		if toks[i].Kind == TokIdent && !isDeclarationKeyword(toks[i]) {
			nameIdx = i
			break
		}
	}
	if nameIdx < 0 {
		for i := lpar - 1; i >= 0; i-- {
			if toks[i].Kind == TokIdent {
				nameIdx = i
				break
			}
		}
	}
	if nameIdx < 0 {
		return nil, fmt.Errorf("unable to determine function name")
	}
	name := toks[nameIdx].Text

	retCount := 1
	retTokens := toks[:nameIdx]
	if containsIdent(retTokens, "void") && !containsPunct(retTokens, "*") {
		retCount = 0
	}

	paramTokens := toks[lpar+1 : rpar]
	paramTokens = trimTokens(paramTokens)
	var paramNames []string
	var paramKinds []cDeclKind
	paramCount := 0
	if len(paramTokens) > 0 {
		parts := splitTopLevel(paramTokens, ",")
		if len(parts) == 1 && len(parts[0]) == 1 && parts[0][0].Kind == TokIdent && parts[0][0].Text == "void" {
			parts = nil
		}
		for i, p := range parts {
			p = trimTokens(p)
			if len(p) == 0 {
				continue
			}
			if len(p) == 1 && p[0].Kind == TokPunct && p[0].Text == "..." {
				return nil, fmt.Errorf("variadic functions are not supported")
			}
			pname := ""
			for j := len(p) - 1; j >= 0; j-- {
				if p[j].Kind == TokIdent && !isDeclarationKeyword(p[j]) {
					pname = p[j].Text
					break
				}
			}
			if pname == "" {
				pname = fmt.Sprintf("$p%d", i)
			}
			pkind := cDeclScalar
			if containsPunct(p, "*") || containsPunct(p, "[") {
				// In parameter lists, arrays decay to pointers.
				pkind = cDeclPointer
			}
			paramNames = append(paramNames, pname)
			paramKinds = append(paramKinds, pkind)
			paramCount++
		}
	}

	return &cFuncSig{
		Name:       name,
		IRName:     "c." + name,
		RetCount:   retCount,
		ParamCount: paramCount,
		ParamNames: paramNames,
		ParamKinds: paramKinds,
		Defined:    false,
		File:       file,
		Line:       line,
		Col:        col,
	}, nil
}

func containsIdent(tokens []Token, ident string) bool {
	for _, t := range tokens {
		if t.Kind == TokIdent && t.Text == ident {
			return true
		}
	}
	return false
}

func containsPunct(tokens []Token, punct string) bool {
	for _, t := range tokens {
		if t.Kind == TokPunct && t.Text == punct {
			return true
		}
	}
	return false
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

func parseDeclItems(toks []Token) ([]cDeclItem, bool, error) {
	toks = trimTokens(toks)
	if len(toks) == 0 {
		return nil, false, nil
	}
	hasExtern := false
	for _, t := range toks {
		if t.Kind == TokIdent && t.Text == "extern" {
			hasExtern = true
			break
		}
	}

	parts := splitTopLevel(toks, ",")
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
			return nil, false, fmt.Errorf("missing declarator in declaration")
		}

		name := ""
		namePos := -1
		for i := len(lhs) - 1; i >= 0; i-- {
			if lhs[i].Kind == TokIdent && !isDeclarationKeyword(lhs[i]) {
				name = lhs[i].Text
				namePos = i
				break
			}
		}
		if name == "" {
			return nil, false, fmt.Errorf("unable to parse declarator name")
		}

		kind := cDeclScalar
		var arrayLen int64
		ptrDepth := 0
		for i, t := range lhs {
			if t.Kind != TokPunct {
				continue
			}
			if t.Text == "*" {
				if i < namePos {
					ptrDepth++
					continue
				}
				return nil, false, fmt.Errorf("complex declarators are not yet supported (%s)", name)
			}
			if t.Text == "[" || t.Text == "]" {
				// Handled by suffix parsing below.
				continue
			}
			if t.Text == "(" && i >= namePos {
				return nil, false, fmt.Errorf("complex declarators are not yet supported (%s)", name)
			}
		}
		if ptrDepth > 0 {
			kind = cDeclPointer
		}

		suffixPos := namePos + 1
		for suffixPos < len(lhs) && lhs[suffixPos].Kind == TokPunct && lhs[suffixPos].Text == ")" {
			suffixPos++
		}
		if suffixPos < len(lhs) {
			if lhs[suffixPos].Kind != TokPunct || lhs[suffixPos].Text != "[" {
				return nil, false, fmt.Errorf("complex declarators are not yet supported (%s)", name)
			}
			if ptrDepth > 0 {
				return nil, false, fmt.Errorf("pointer-to-array declarators are not yet supported (%s)", name)
			}
			closeIdx := -1
			depth := 0
			for i := suffixPos; i < len(lhs); i++ {
				if lhs[i].Kind != TokPunct {
					continue
				}
				if lhs[i].Text == "[" {
					depth++
				} else if lhs[i].Text == "]" {
					depth--
					if depth == 0 {
						closeIdx = i
						break
					}
				}
			}
			if closeIdx < 0 {
				return nil, false, fmt.Errorf("unterminated array declarator (%s)", name)
			}
			if closeIdx != len(lhs)-1 {
				return nil, false, fmt.Errorf("complex declarators are not yet supported (%s)", name)
			}
			n, err := parseArrayLength(lhs[suffixPos+1 : closeIdx])
			if err != nil {
				return nil, false, fmt.Errorf("invalid array bounds for %s: %v", name, err)
			}
			kind = cDeclArray
			arrayLen = n
		}

		items = append(items, cDeclItem{Name: name, Init: init, Kind: kind, ArrayLen: arrayLen})
	}
	if len(items) == 0 {
		return nil, false, fmt.Errorf("empty declaration")
	}
	return items, hasExtern, nil
}

type funcCompiler struct {
	c   *compiler
	sig *cFuncSig
	fn  *ir.IRFunc

	scopes []map[string]cLocalBinding

	breakTargets    []int
	continueTargets []int
}

func (fc *funcCompiler) errorf(file string, line int, col int, format string, args ...interface{}) {
	fc.c.errorf(file, line, col, format, args...)
}

func (fc *funcCompiler) emit(inst ir.Inst) {
	fc.fn.Code = append(fc.fn.Code, inst)
}

func (fc *funcCompiler) pushScope() {
	fc.scopes = append(fc.scopes, make(map[string]cLocalBinding))
}

func (fc *funcCompiler) popScope() {
	if len(fc.scopes) > 0 {
		fc.scopes = fc.scopes[:len(fc.scopes)-1]
	}
}

func (fc *funcCompiler) addLocal(name string, file string, line int, col int) int {
	return fc.addLocalKind(name, cDeclScalar, file, line, col)
}

func (fc *funcCompiler) addLocalKind(name string, kind cDeclKind, file string, line int, col int) int {
	if len(fc.scopes) == 0 {
		fc.pushScope()
	}
	cur := fc.scopes[len(fc.scopes)-1]
	if _, exists := cur[name]; exists {
		fc.errorf(file, line, col, "redefinition of local %q", name)
	}
	idx := len(fc.fn.Locals)
	fc.fn.Locals = append(fc.fn.Locals, ir.IRLocal{Name: name, Index: idx})
	cur[name] = cLocalBinding{Index: idx, Kind: kind}
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

func (fc *funcCompiler) compileDeclStmt(n *Node) {
	toks, err := lexSnippet(fc.sig.File, n.Text)
	if err != nil {
		fc.errorf(fc.sig.File, n.Line, n.Col, "invalid declaration: %v", err)
		return
	}
	items, _, err := parseDeclItems(toks)
	if err != nil {
		fc.errorf(fc.sig.File, n.Line, n.Col, "%v", err)
		return
	}
	for _, it := range items {
		idx := fc.addLocalKind(it.Name, it.Kind, fc.sig.File, n.Line, n.Col)
		if it.Kind == cDeclArray {
			if len(it.Init) > 0 {
				fc.errorf(fc.sig.File, n.Line, n.Col, "array initializers are not yet supported: %s", it.Name)
			}
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
			continue
		}
		if len(it.Init) == 0 {
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
			continue
		}
		fc.emitExprTokens(fc.sig.File, n.Line, n.Col, it.Init)
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
	fc.emit(ir.Inst{Op: ir.OP_RETURN, Arg: 1})
}

func (fc *funcCompiler) compileDeclTokens(file string, n *Node, toks []Token) {
	items, _, err := parseDeclItems(toks)
	if err != nil {
		fc.errorf(file, n.Line, n.Col, "%v", err)
		return
	}
	for _, it := range items {
		idx := fc.addLocalKind(it.Name, it.Kind, file, n.Line, n.Col)
		if it.Kind == cDeclArray {
			if len(it.Init) > 0 {
				fc.errorf(file, n.Line, n.Col, "array initializers are not yet supported: %s", it.Name)
			}
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
			continue
		}
		if len(it.Init) == 0 {
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
			continue
		}
		fc.emitExprTokens(file, n.Line, n.Col, it.Init)
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
	fc.emitExpr(base)
	fc.emitExpr(index)
	fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(fc.c.target.PtrSize)})
	fc.emit(ir.Inst{Op: ir.OP_MUL})
	fc.emit(ir.Inst{Op: ir.OP_ADD})
}

func (fc *funcCompiler) emitAddressOf(ex *expr) bool {
	if ex == nil {
		return false
	}
	switch ex.kind {
	case exprVar:
		if idx, ok := fc.lookupLocal(ex.name); ok {
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_ADDR, Arg: idx})
			return true
		}
		if idx, ok := fc.lookupGlobal(ex.name); ok {
			fc.emit(ir.Inst{Op: ir.OP_GLOBAL_ADDR, Arg: idx})
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
	default:
		return false
	}
}

func (fc *funcCompiler) stepForKind(kind cDeclKind) int64 {
	if kind == cDeclPointer || kind == cDeclArray {
		return int64(fc.c.target.PtrSize)
	}
	return 1
}

func (fc *funcCompiler) variableKind(name string) cDeclKind {
	if kind, ok := fc.lookupLocalKind(name); ok {
		return kind
	}
	if kind, ok := fc.lookupGlobalKind(name); ok {
		return kind
	}
	return cDeclScalar
}

func (fc *funcCompiler) exprIsPointer(ex *expr) bool {
	if ex == nil {
		return false
	}
	switch ex.kind {
	case exprVar:
		kind := fc.variableKind(ex.name)
		return kind == cDeclPointer || kind == cDeclArray
	case exprAssign:
		return fc.exprIsPointer(ex.left)
	case exprUnary:
		switch ex.op {
		case "&":
			return true
		case "*":
			// The current subset models unary dereference as loading an int.
			// Pointer-to-pointer types are parsed, but depth is not tracked yet.
			return false
		case "++", "--":
			return fc.exprIsPointer(ex.left)
		default:
			return false
		}
	case exprPostfix:
		if ex.op == "++" || ex.op == "--" {
			return fc.exprIsPointer(ex.left)
		}
		return false
	case exprBinary:
		if ex.op == "+" {
			lp := fc.exprIsPointer(ex.left)
			rp := fc.exprIsPointer(ex.right)
			return (lp && !rp) || (!lp && rp)
		}
		if ex.op == "-" {
			lp := fc.exprIsPointer(ex.left)
			rp := fc.exprIsPointer(ex.right)
			return lp && !rp
		}
		return false
	default:
		return false
	}
}

func (fc *funcCompiler) emitExpr(ex *expr) {
	if ex == nil {
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		return
	}
	switch ex.kind {
	case exprIntLit:
		fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: ex.intVal})
	case exprVar:
		if idx, ok := fc.lookupLocal(ex.name); ok {
			fc.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: idx})
			return
		}
		if idx, ok := fc.lookupGlobal(ex.name); ok {
			fc.emit(ir.Inst{Op: ir.OP_GLOBAL_GET, Arg: idx})
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
		fc.emitExpr(ex.right)
		fc.emit(ir.Inst{Op: ir.OP_DUP})
		if !fc.emitAddressOf(ex.left) {
			fc.errorf(fc.sig.File, 0, 0, "left-hand side of assignment is not assignable")
			fc.emit(ir.Inst{Op: ir.OP_DROP})
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			return
		}
		fc.emit(ir.Inst{Op: ir.OP_STORE, Arg: fc.c.target.PtrSize})
	case exprUnary:
		if ex.op == "++" || ex.op == "--" {
			if ex.left == nil || ex.left.kind != exprVar {
				fc.errorf(fc.sig.File, 0, 0, "%s requires variable operand", ex.op)
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				return
			}
			name := ex.left.name
			step := fc.stepForKind(fc.variableKind(name))
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
			fc.emit(ir.Inst{Op: ir.OP_LOAD, Arg: fc.c.target.PtrSize})
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
		step := fc.stepForKind(fc.variableKind(name))
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
			if leftPtr && !rightPtr {
				fc.emitExpr(ex.left)
				fc.emitExpr(ex.right)
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(fc.c.target.PtrSize)})
				fc.emit(ir.Inst{Op: ir.OP_MUL})
				if ex.op == "+" {
					fc.emit(ir.Inst{Op: ir.OP_ADD})
				} else {
					fc.emit(ir.Inst{Op: ir.OP_SUB})
				}
				return
			}
			if ex.op == "+" && !leftPtr && rightPtr {
				fc.emitExpr(ex.left)
				fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(fc.c.target.PtrSize)})
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
		fc.emit(ir.Inst{Op: ir.OP_LOAD, Arg: fc.c.target.PtrSize})
	case exprCall:
		for _, a := range ex.args {
			fc.emitExpr(a)
		}
		sig, ok := fc.c.funcs[ex.name]
		if !ok {
			fc.errorf(fc.sig.File, 0, 0, "call to unknown function %q", ex.name)
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			return
		}
		if !sig.Defined {
			fc.errorf(fc.sig.File, 0, 0, "calls to external function %q are not yet supported", ex.name)
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			return
		}
		fc.emit(ir.Inst{Op: ir.OP_CALL, Name: sig.IRName, Arg: len(ex.args)})
		if sig.RetCount == 0 {
			// Preserve expression stack shape for continued lowering.
			fc.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
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
	exprVar
	exprAssign
	exprUnary
	exprPostfix
	exprBinary
	exprIndex
	exprCall
)

type expr struct {
	kind exprKind
	op   string

	intVal int64
	name   string

	left  *expr
	right *expr
	args  []*expr
}

type cExprParser struct {
	fc   *funcCompiler
	file string
	line int
	col  int
	toks []Token
	pos  int
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
			if n.kind != exprVar {
				p.errorf("only direct function calls are supported")
				n = &expr{kind: exprIntLit, intVal: 0}
			}
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
			n = &expr{kind: exprCall, name: n.name, args: args}
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
