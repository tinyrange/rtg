package main

const (
	opConstI64 = 0
	opConstStr = 1
	opLocalGet = 4
	opLocalSet = 5
	opLocalAddr = 7
	opDrop = 11
	opAdd = 13
	opSub = 14
	opMul = 15
	opDiv = 16
	opNeg = 18
	opLoad = 37
	opStore = 38
	opOffset = 39
	opCall = 44
	opCallIntrinsic = 45
	opReturn = 46
	opLen = 52
	opConvert = 53
)

const (
	tokEOF = iota
	tokIdent
	tokNumber
	tokString
	tokPackage
	tokFunc
	tokVar
	tokType
	tokStruct
	tokReturn
	tokPrint
	tokInt
	tokStringType
	tokLParen
	tokRParen
	tokLBrace
	tokRBrace
	tokComma
	tokDot
	tokAssign
	tokColonAssign
	tokSemi
	tokPlus
	tokMinus
	tokStar
	tokSlash
)

const (
	kindInvalid = iota
	kindInt
	kindString
	kindStruct
	kindPtrStruct
)

type token struct {
	kind int
	text string
	ival int64
}

type inst struct {
	op int
	arg int
	val int64
	name string
}

type fieldDef struct {
	name string
	kind int
}

type structDef struct {
	name string
	fields []fieldDef
}

type varDef struct {
	name string
	kind int
	structIdx int
	local int
	base int
}

type funcSig struct {
	name string
	params int
	retCount int
	retKind int
}

type methodSig struct {
	structName string
	methodName string
	funcName string
	retCount int
	retKind int
}

type funcDef struct {
	name string
	params int
	retCount int
	retKind int
	locals []string
	vars []varDef
	code []inst
}

type parser struct {
	src []byte
	n int
	pos int
	tok token
	errCode int

	structs []structDef
	funcs []*funcDef
	sigs []funcSig
	methods []methodSig
	needPrintStr bool
	needPrintInt bool
	needSyscall bool
}

var inputName = [...]byte{'I', 'N', 'P', 'U', 'T', '.', 'G', 'O', 0}
var outputName = [...]byte{'P', 'R', 'O', 'G', '.', 'T', 'I', 'R', 0}

func isAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isAlnum(c byte) bool {
	return isAlpha(c) || (c >= '0' && c <= '9')
}

func eq(a string, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (p *parser) keywordKind(s string) int {
	if eq(s, "package") {
		return tokPackage
	}
	if eq(s, "func") {
		return tokFunc
	}
	if eq(s, "var") {
		return tokVar
	}
	if eq(s, "type") {
		return tokType
	}
	if eq(s, "struct") {
		return tokStruct
	}
	if eq(s, "return") {
		return tokReturn
	}
	if eq(s, "print") {
		return tokPrint
	}
	if eq(s, "int") {
		return tokInt
	}
	if eq(s, "string") {
		return tokStringType
	}
	return tokIdent
}

func (p *parser) skipSpaceAndComments() {
	for p.pos < p.n {
		c := p.src[p.pos]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			p.pos++
			continue
		}
		if c == '/' && p.pos+1 < p.n && p.src[p.pos+1] == '/' {
			p.pos += 2
			for p.pos < p.n && p.src[p.pos] != '\n' {
				p.pos++
			}
			continue
		}
		break
	}
}

func (p *parser) nextToken() token {
	p.skipSpaceAndComments()
	if p.pos >= p.n {
		return token{kind: tokEOF}
	}
	c := p.src[p.pos]
	if isAlpha(c) {
		s0 := p.pos
		p.pos++
		for p.pos < p.n && isAlnum(p.src[p.pos]) {
			p.pos++
		}
		s := string(p.src[s0:p.pos])
		return token{kind: p.keywordKind(s), text: s}
	}
	if c >= '0' && c <= '9' {
		v := int64(0)
		s0 := p.pos
		for p.pos < p.n {
			d := p.src[p.pos]
			if d < '0' || d > '9' {
				break
			}
			v = v*10 + int64(d-'0')
			p.pos++
		}
		return token{kind: tokNumber, text: string(p.src[s0:p.pos]), ival: v}
	}
	if c == '"' {
		p.pos++
		s0 := p.pos
		for p.pos < p.n {
			d := p.src[p.pos]
			if d == '\\' && p.pos+1 < p.n {
				p.pos += 2
				continue
			}
			if d == '"' {
				s := string(p.src[s0:p.pos])
				p.pos++
				return token{kind: tokString, text: s}
			}
			p.pos++
		}
		return token{kind: tokEOF}
	}

	p.pos++
	switch c {
	case '(':
		return token{kind: tokLParen}
	case ')':
		return token{kind: tokRParen}
	case '{':
		return token{kind: tokLBrace}
	case '}':
		return token{kind: tokRBrace}
	case ',':
		return token{kind: tokComma}
	case '.':
		return token{kind: tokDot}
	case ';':
		return token{kind: tokSemi}
	case '+':
		return token{kind: tokPlus}
	case '-':
		return token{kind: tokMinus}
	case '*':
		return token{kind: tokStar}
	case '/':
		return token{kind: tokSlash}
	case ':':
		if p.pos < p.n && p.src[p.pos] == '=' {
			p.pos++
			return token{kind: tokColonAssign}
		}
	case '=':
		return token{kind: tokAssign}
	}
	return token{kind: tokEOF}
}

func (p *parser) init(src []byte, n int) {
	p.src = src
	p.n = n
	p.pos = 0
	p.errCode = 0
	p.structs = nil
	p.funcs = nil
	p.sigs = nil
	p.methods = nil
	p.needPrintStr = false
	p.needPrintInt = false
	p.needSyscall = false
	p.tok = p.nextToken()
}

func (p *parser) adv() { p.tok = p.nextToken() }

func (p *parser) eat(k int) bool {
	if p.tok.kind != k {
		return false
	}
	p.adv()
	return true
}

func (p *parser) addSig(name string, params int, retCount int, retKind int) {
	p.sigs = append(p.sigs, funcSig{name: name, params: params, retCount: retCount, retKind: retKind})
}

func (p *parser) findSig(name string) (funcSig, bool) {
	for i := 0; i < len(p.sigs); i++ {
		if eq(p.sigs[i].name, name) {
			return p.sigs[i], true
		}
	}
	return funcSig{}, false
}

func (p *parser) findStruct(name string) int {
	for i := 0; i < len(p.structs); i++ {
		if eq(p.structs[i].name, name) {
			return i
		}
	}
	return -1
}

func (p *parser) findField(si int, field string) (int, int) {
	if si < 0 || si >= len(p.structs) {
		return -1, kindInvalid
	}
	for i := 0; i < len(p.structs[si].fields); i++ {
		if eq(p.structs[si].fields[i].name, field) {
			return i, p.structs[si].fields[i].kind
		}
	}
	return -1, kindInvalid
}

func (p *parser) findMethod(structName string, methodName string) (methodSig, bool) {
	for i := 0; i < len(p.methods); i++ {
		m := p.methods[i]
		if eq(m.structName, structName) && eq(m.methodName, methodName) {
			return m, true
		}
	}
	return methodSig{}, false
}

func (f *funcDef) emit(op int, arg int, val int64, name string) {
	f.code = append(f.code, inst{op: op, arg: arg, val: val, name: name})
}

func (f *funcDef) addLocal(name string) int {
	idx := len(f.locals)
	f.locals = append(f.locals, name)
	return idx
}

func (f *funcDef) addVarScalar(name string, kind int, structIdx int) varDef {
	li := f.addLocal(name)
	v := varDef{name: name, kind: kind, structIdx: structIdx, local: li, base: li}
	f.vars = append(f.vars, v)
	return v
}

func (f *funcDef) addVarStruct(name string, structIdx int, fields []fieldDef) varDef {
	base := len(f.locals)
	for i := 0; i < len(fields); i++ {
		f.locals = append(f.locals, name+"."+fields[i].name)
	}
	v := varDef{name: name, kind: kindStruct, structIdx: structIdx, base: base}
	f.vars = append(f.vars, v)
	return v
}

func (f *funcDef) findVar(name string) (varDef, bool) {
	for i := len(f.vars) - 1; i >= 0; i-- {
		if eq(f.vars[i].name, name) {
			return f.vars[i], true
		}
	}
	return varDef{}, false
}

func (p *parser) parseType() (int, int, bool) {
	if p.tok.kind == tokInt {
		p.adv()
		return kindInt, -1, true
	}
	if p.tok.kind == tokStringType {
		p.adv()
		return kindString, -1, true
	}
	if p.tok.kind == tokStar {
		p.adv()
		if p.tok.kind != tokIdent {
			return kindInvalid, -1, false
		}
		si := p.findStruct(p.tok.text)
		if si < 0 {
			return kindInvalid, -1, false
		}
		p.adv()
		return kindPtrStruct, si, true
	}
	if p.tok.kind == tokIdent {
		si := p.findStruct(p.tok.text)
		if si >= 0 {
			p.adv()
			return kindStruct, si, true
		}
	}
	return kindInvalid, -1, false
}

func (p *parser) parseTypeDecl() bool {
	if !p.eat(tokType) {
		return false
	}
	if p.tok.kind != tokIdent {
		p.errCode = 21
		return false
	}
	typeName := p.tok.text
	p.adv()
	if !p.eat(tokStruct) || !p.eat(tokLBrace) {
		p.errCode = 22
		return false
	}
	sd := structDef{name: typeName}
	for p.tok.kind != tokRBrace && p.tok.kind != tokEOF {
		if p.tok.kind == tokSemi {
			p.adv()
			continue
		}
		if p.tok.kind != tokIdent {
			p.errCode = 23
			return false
		}
		fname := p.tok.text
		p.adv()
		k, _, ok := p.parseType()
		if !ok || (k != kindInt && k != kindString) {
			p.errCode = 24
			return false
		}
		sd.fields = append(sd.fields, fieldDef{name: fname, kind: k})
		for p.tok.kind == tokSemi {
			p.adv()
		}
	}
	if !p.eat(tokRBrace) {
		p.errCode = 25
		return false
	}
	p.structs = append(p.structs, sd)
	return true
}

func (p *parser) parseCallArgs(f *funcDef) (int, bool) {
	argc := 0
	for p.tok.kind != tokRParen && p.tok.kind != tokEOF {
		_, _, ok := p.parseExpr(f)
		if !ok {
			return 0, false
		}
		argc++
		if p.tok.kind == tokComma {
			p.adv()
			continue
		}
		break
	}
	if !p.eat(tokRParen) {
		p.errCode = 61
		return 0, false
	}
	return argc, true
}

func (p *parser) emitSyscallCall(f *funcDef, argc int) int {
	p.needSyscall = true
	for argc < 7 {
		f.emit(opConstI64, 0, 0, "")
		argc++
	}
	f.emit(opCall, 7, 0, "main.__syscall")
	return 3
}

func (p *parser) parsePrimary(f *funcDef) (int, int, bool) {
	if p.tok.kind == tokNumber {
		f.emit(opConstI64, 0, p.tok.ival, "")
		p.adv()
		return kindInt, -1, true
	}
	if p.tok.kind == tokString {
		f.emit(opConstStr, 0, 0, p.tok.text)
		p.adv()
		return kindString, -1, true
	}
	if p.tok.kind == tokLParen {
		p.adv()
		k, si, ok := p.parseExpr(f)
		if !ok {
			return kindInvalid, -1, false
		}
		if !p.eat(tokRParen) {
			p.errCode = 62
			return kindInvalid, -1, false
		}
		return k, si, true
	}
	if p.tok.kind != tokIdent {
		p.errCode = 63
		return kindInvalid, -1, false
	}

	name := p.tok.text
	p.adv()

	if p.tok.kind == tokLParen {
		p.adv()
		argc, ok := p.parseCallArgs(f)
		if !ok {
			return kindInvalid, -1, false
		}
		if eq(name, "interrupt") || eq(name, "syscall") {
			p.emitSyscallCall(f, argc)
			return kindInt, -1, true
		}
		full := "main." + name
		sig, have := p.findSig(full)
		f.emit(opCall, argc, 0, full)
		if have && sig.retCount > 0 {
			return sig.retKind, -1, true
		}
		return kindInt, -1, true
	}

	if p.tok.kind == tokDot {
		p.adv()
		if p.tok.kind != tokIdent {
			p.errCode = 64
			return kindInvalid, -1, false
		}
		member := p.tok.text
		p.adv()
		v, ok := f.findVar(name)
		if !ok {
			p.errCode = 65
			return kindInvalid, -1, false
		}
		if p.tok.kind == tokLParen {
			p.adv()
			ms, mok := p.findMethod(p.structs[v.structIdx].name, member)
			if !mok {
				p.errCode = 66
				return kindInvalid, -1, false
			}
			if v.kind == kindStruct {
				f.emit(opLocalAddr, v.base, 0, "")
			} else {
				f.emit(opLocalGet, v.local, 0, "")
			}
			argc, ok := p.parseCallArgs(f)
			if !ok {
				return kindInvalid, -1, false
			}
			f.emit(opCall, argc+1, 0, ms.funcName)
			if ms.retCount > 0 {
				return ms.retKind, -1, true
			}
			return kindInvalid, -1, true
		}
		fi, fk := p.findField(v.structIdx, member)
		if fi < 0 {
			p.errCode = 67
			return kindInvalid, -1, false
		}
		if v.kind == kindStruct {
			f.emit(opLocalGet, v.base+fi, 0, "")
			return fk, -1, true
		}
		if v.kind == kindPtrStruct {
			f.emit(opLocalGet, v.local, 0, "")
			if fi != 0 {
				f.emit(opOffset, fi*2, 0, "")
			}
			f.emit(opLoad, 0, 0, "")
			return fk, -1, true
		}
		p.errCode = 68
		return kindInvalid, -1, false
	}

	v, ok := f.findVar(name)
	if !ok {
		p.errCode = 69
		return kindInvalid, -1, false
	}
	if v.kind == kindStruct {
		p.errCode = 70
		return kindInvalid, -1, false
	}
	f.emit(opLocalGet, v.local, 0, "")
	return v.kind, v.structIdx, true
}

func (p *parser) parseUnary(f *funcDef) (int, int, bool) {
	if p.tok.kind == tokMinus {
		p.adv()
		k, si, ok := p.parseUnary(f)
		if !ok {
			return kindInvalid, -1, false
		}
		f.emit(opNeg, 0, 0, "")
		return kindInt, si, k == kindInt
	}
	return p.parsePrimary(f)
}

func (p *parser) parseMul(f *funcDef) (int, int, bool) {
	k, si, ok := p.parseUnary(f)
	if !ok {
		return kindInvalid, -1, false
	}
	for p.tok.kind == tokStar || p.tok.kind == tokSlash {
		op := p.tok.kind
		p.adv()
		_, _, ok2 := p.parseUnary(f)
		if !ok2 {
			return kindInvalid, -1, false
		}
		if op == tokStar {
			f.emit(opMul, 0, 0, "")
		} else {
			f.emit(opDiv, 0, 0, "")
		}
		k = kindInt
		si = -1
	}
	return k, si, true
}

func (p *parser) parseExpr(f *funcDef) (int, int, bool) {
	k, si, ok := p.parseMul(f)
	if !ok {
		return kindInvalid, -1, false
	}
	for p.tok.kind == tokPlus || p.tok.kind == tokMinus {
		op := p.tok.kind
		p.adv()
		_, _, ok2 := p.parseMul(f)
		if !ok2 {
			return kindInvalid, -1, false
		}
		if op == tokPlus {
			f.emit(opAdd, 0, 0, "")
		} else {
			f.emit(opSub, 0, 0, "")
		}
		k = kindInt
		si = -1
	}
	return k, si, true
}

func (p *parser) parseVarDecl(f *funcDef) bool {
	if !p.eat(tokVar) {
		return false
	}
	if p.tok.kind != tokIdent {
		p.errCode = 81
		return false
	}
	name := p.tok.text
	p.adv()
	k, si, ok := p.parseType()
	if !ok {
		p.errCode = 82
		return false
	}
	if k == kindStruct {
		f.addVarStruct(name, si, p.structs[si].fields)
	} else {
		f.addVarScalar(name, k, si)
	}
	for p.tok.kind == tokSemi {
		p.adv()
	}
	return true
}

func (p *parser) assignTo(f *funcDef, lhs string, field string, rhsKind int) bool {
	v, ok := f.findVar(lhs)
	if !ok {
		p.errCode = 83
		return false
	}
	if field == "" {
		if v.kind == kindStruct {
			p.errCode = 84
			return false
		}
		_ = rhsKind
		f.emit(opLocalSet, v.local, 0, "")
		return true
	}
	fi, _ := p.findField(v.structIdx, field)
	if fi < 0 {
		p.errCode = 85
		return false
	}
	if v.kind == kindStruct {
		f.emit(opLocalSet, v.base+fi, 0, "")
		return true
	}
	if v.kind == kindPtrStruct {
		f.emit(opLocalGet, v.local, 0, "")
		if fi != 0 {
			f.emit(opOffset, fi*2, 0, "")
		}
		f.emit(opStore, 0, 0, "")
		return true
	}
	p.errCode = 86
	return false
}

func (p *parser) parsePrintStmt(f *funcDef) bool {
	if !p.eat(tokPrint) || !p.eat(tokLParen) {
		p.errCode = 87
		return false
	}
	k, _, ok := p.parseExpr(f)
	if !ok {
		return false
	}
	if !p.eat(tokRParen) {
		p.errCode = 88
		return false
	}
	if k == kindString {
		p.needPrintStr = true
		f.emit(opCall, 1, 0, "main.__print_str")
	} else {
		p.needPrintInt = true
		p.needPrintStr = true
		f.emit(opCall, 1, 0, "main.__print_int")
	}
	for p.tok.kind == tokSemi {
		p.adv()
	}
	return true
}

func (p *parser) parseStmt(f *funcDef) bool {
	if p.tok.kind == tokSemi {
		p.adv()
		return true
	}
	if p.tok.kind == tokVar {
		return p.parseVarDecl(f)
	}
	if p.tok.kind == tokReturn {
		p.adv()
		if f.retCount > 0 && p.tok.kind != tokSemi && p.tok.kind != tokRBrace {
			_, _, ok := p.parseExpr(f)
			if !ok {
				return false
			}
		}
		f.emit(opReturn, 0, 0, "")
		for p.tok.kind == tokSemi {
			p.adv()
		}
		return true
	}
	if p.tok.kind == tokPrint {
		return p.parsePrintStmt(f)
	}
	if p.tok.kind != tokIdent {
		p.errCode = 89
		return false
	}

	name := p.tok.text
	p.adv()
	field := ""
	if p.tok.kind == tokDot {
		p.adv()
		if p.tok.kind != tokIdent {
			p.errCode = 90
			return false
		}
		field = p.tok.text
		p.adv()
	}

	if p.tok.kind == tokAssign || p.tok.kind == tokColonAssign {
		short := p.tok.kind == tokColonAssign
		p.adv()
		rk, si, ok := p.parseExpr(f)
		if !ok {
			return false
		}
		if short {
			if field != "" {
				p.errCode = 91
				return false
			}
			if _, exists := f.findVar(name); !exists {
				if rk == kindStruct {
					f.addVarStruct(name, si, p.structs[si].fields)
				} else {
					if rk == kindInvalid {
						rk = kindInt
					}
					f.addVarScalar(name, rk, si)
				}
			}
		}
		if !p.assignTo(f, name, field, rk) {
			return false
		}
		for p.tok.kind == tokSemi {
			p.adv()
		}
		return true
	}

	if p.tok.kind != tokLParen {
		p.errCode = 92
		return false
	}
	p.adv()

	argc, ok := p.parseCallArgs(f)
	if !ok {
		return false
	}

	retCount := 0
	if field != "" {
		v, vok := f.findVar(name)
		if !vok {
			p.errCode = 93
			return false
		}
		ms, mok := p.findMethod(p.structs[v.structIdx].name, field)
		if !mok {
			p.errCode = 94
			return false
		}
		if v.kind == kindStruct {
			f.emit(opLocalAddr, v.base, 0, "")
		} else {
			f.emit(opLocalGet, v.local, 0, "")
		}
		f.emit(opCall, argc+1, 0, ms.funcName)
		retCount = ms.retCount
	} else {
		if eq(name, "interrupt") || eq(name, "syscall") {
			retCount = p.emitSyscallCall(f, argc)
		} else {
			full := "main." + name
			sig, have := p.findSig(full)
			f.emit(opCall, argc, 0, full)
			if have {
				retCount = sig.retCount
			} else {
				retCount = 1
			}
		}
	}
	for i := 0; i < retCount; i++ {
		f.emit(opDrop, 0, 0, "")
	}
	for p.tok.kind == tokSemi {
		p.adv()
	}
	return true
}

func (p *parser) parseFuncDecl() bool {
	if !p.eat(tokFunc) {
		return false
	}

	recvName := ""
	recvStruct := -1
	isMethod := false

	if p.tok.kind == tokLParen {
		p.adv()
		if p.tok.kind != tokIdent {
			p.errCode = 101
			return false
		}
		recvName = p.tok.text
		p.adv()
		if !p.eat(tokStar) || p.tok.kind != tokIdent {
			p.errCode = 102
			return false
		}
		recvStruct = p.findStruct(p.tok.text)
		if recvStruct < 0 {
			p.errCode = 103
			return false
		}
		p.adv()
		if !p.eat(tokRParen) {
			p.errCode = 104
			return false
		}
		isMethod = true
	}

	if p.tok.kind != tokIdent {
		p.errCode = 105
		return false
	}
	fnShort := p.tok.text
	p.adv()

	full := "main." + fnShort
	if isMethod {
		full = "main." + p.structs[recvStruct].name + "." + fnShort
	}

	if !p.eat(tokLParen) {
		p.errCode = 106
		return false
	}

	f := &funcDef{name: full, retKind: kindInvalid}
	if isMethod {
		f.params = 1
		f.addVarScalar(recvName, kindPtrStruct, recvStruct)
	}

	for p.tok.kind != tokRParen && p.tok.kind != tokEOF {
		if p.tok.kind != tokIdent {
			p.errCode = 107
			return false
		}
		pn := p.tok.text
		p.adv()
		k, si, ok := p.parseType()
		if !ok {
			p.errCode = 108
			return false
		}
		f.params++
		if k == kindStruct {
			f.addVarStruct(pn, si, p.structs[si].fields)
		} else {
			f.addVarScalar(pn, k, si)
		}
		if p.tok.kind == tokComma {
			p.adv()
			continue
		}
		break
	}
	if !p.eat(tokRParen) {
		p.errCode = 109
		return false
	}

	if p.tok.kind == tokInt || p.tok.kind == tokStringType {
		k, _, ok := p.parseType()
		if !ok {
			p.errCode = 110
			return false
		}
		f.retCount = 1
		f.retKind = k
	}

	p.addSig(full, f.params, f.retCount, f.retKind)
	if isMethod {
		p.methods = append(p.methods, methodSig{
			structName: p.structs[recvStruct].name,
			methodName: fnShort,
			funcName: full,
			retCount: f.retCount,
			retKind: f.retKind,
		})
	}

	if !p.eat(tokLBrace) {
		p.errCode = 111
		return false
	}
	for p.tok.kind != tokRBrace && p.tok.kind != tokEOF {
		if !p.parseStmt(f) {
			return false
		}
	}
	if !p.eat(tokRBrace) {
		p.errCode = 112
		return false
	}
	if len(f.code) == 0 || f.code[len(f.code)-1].op != opReturn {
		f.emit(opReturn, 0, 0, "")
	}
	p.funcs = append(p.funcs, f)
	return true
}

func (p *parser) addPrintHelpers() {
	if p.needPrintStr {
		p.addSig("main.__print_str", 1, 0, kindInvalid)
		f1 := &funcDef{name: "main.__print_str", params: 1, retCount: 0}
		f1.addVarScalar("s", kindString, -1)
		f1.emit(opConstI64, 0, 4, "")
		f1.emit(opConstI64, 0, 1, "")
		f1.emit(opLocalGet, 0, 0, "")
		f1.emit(opLoad, 0, 0, "")
		f1.emit(opLocalGet, 0, 0, "")
		f1.emit(opOffset, 2, 0, "")
		f1.emit(opLoad, 0, 0, "")
		f1.emit(opConvert, 0, 0, "uintptr")
		p.emitSyscallCall(f1, 4)
		f1.emit(opDrop, 0, 0, "")
		f1.emit(opDrop, 0, 0, "")
		f1.emit(opDrop, 0, 0, "")
		f1.emit(opReturn, 0, 0, "")
		p.funcs = append(p.funcs, f1)
	}

	if p.needPrintInt {
		p.addSig("main.__print_int", 1, 0, kindInvalid)
		f2 := &funcDef{name: "main.__print_int", params: 1, retCount: 0}
		f2.addVarScalar("n", kindInt, -1)
		f2.emit(opLocalGet, 0, 0, "")
		f2.emit(opCall, 1, 0, "runtime.IntToString")
		f2.emit(opCall, 1, 0, "main.__print_str")
		f2.emit(opReturn, 0, 0, "")
		p.funcs = append(p.funcs, f2)
	}
}

func (p *parser) addSyscallWrapper() {
	if !p.needSyscall {
		return
	}
	p.addSig("main.__syscall", 7, 3, kindInt)
	f := &funcDef{name: "main.__syscall", params: 7, retCount: 3}
	f.emit(opCallIntrinsic, 7, 0, "Syscall")
	f.emit(opReturn, 0, 0, "")
	p.funcs = append(p.funcs, f)
}

func (p *parser) parse() bool {
	if !p.eat(tokPackage) || p.tok.kind != tokIdent || !eq(p.tok.text, "main") {
		p.errCode = 11
		return false
	}
	p.adv()

	for p.tok.kind != tokEOF {
		for p.tok.kind == tokSemi {
			p.adv()
		}
		if p.tok.kind == tokType {
			if !p.parseTypeDecl() {
				return false
			}
			continue
		}
		if p.tok.kind == tokFunc {
			if !p.parseFuncDecl() {
				return false
			}
			continue
		}
		if p.tok.kind == tokEOF {
			break
		}
		p.errCode = 12
		return false
	}

	if _, ok := p.findSig("main.main"); !ok {
		p.errCode = 13
		return false
	}
	p.addPrintHelpers()
	p.addSyscallWrapper()
	return true
}

type enc struct {
	b []byte
	n int
}

func (e *enc) put8(v byte) bool {
	if e.n >= len(e.b) {
		return false
	}
	e.b[e.n] = v
	e.n++
	return true
}

func (e *enc) putU16(v int) bool {
	return e.put8(byte(v)) && e.put8(byte(v>>8))
}

func (e *enc) putI32(v int) bool {
	u := uint32(v)
	return e.put8(byte(u)) && e.put8(byte(u>>8)) && e.put8(byte(u>>16)) && e.put8(byte(u>>24))
}

func (e *enc) putI64(v int64) bool {
	u := uint64(v)
	return e.put8(byte(u)) && e.put8(byte(u>>8)) && e.put8(byte(u>>16)) && e.put8(byte(u>>24)) &&
		e.put8(byte(u>>32)) && e.put8(byte(u>>40)) && e.put8(byte(u>>48)) && e.put8(byte(u>>56))
}

func (e *enc) putStr(s string) bool {
	if !e.putU16(len(s)) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !e.put8(s[i]) {
			return false
		}
	}
	return true
}

func encodeTIR(p *parser, out []byte) int {
	e := enc{b: out}
	if !e.put8('T') || !e.put8('I') || !e.put8('R') || !e.put8('3') {
		return -1
	}
	if !e.putU16(len(p.funcs)) {
		return -1
	}
	for i := 0; i < len(p.funcs); i++ {
		f := p.funcs[i]
		if !e.putStr(f.name) {
			return -1
		}
		if !e.putU16(f.params) || !e.putU16(f.retCount) {
			return -1
		}
		if !e.putU16(len(f.locals)) {
			return -1
		}
		for li := 0; li < len(f.locals); li++ {
			if !e.putStr(f.locals[li]) {
				return -1
			}
		}
		if !e.putU16(len(f.code)) {
			return -1
		}
		for ci := 0; ci < len(f.code); ci++ {
			in := f.code[ci]
			if !e.putU16(in.op) || !e.putI32(in.arg) || !e.putI64(in.val) || !e.putStr(in.name) {
				return -1
			}
		}
	}
	return e.n
}

func estimateTIRSize(p *parser) uintptr {
	n := uintptr(0)
	n += 4 // magic
	n += 2 // func count
	for i := 0; i < len(p.funcs); i++ {
		f := p.funcs[i]
		n += 2 + uintptr(len(f.name))
		n += 2 + 2 // params, retCount
		n += 2     // local count
		for li := 0; li < len(f.locals); li++ {
			n += 2 + uintptr(len(f.locals[li]))
		}
		n += 2 // inst count
		for ci := 0; ci < len(f.code); ci++ {
			in := f.code[ci]
			_ = in
			n += 2  // op
			n += 4  // arg
			n += 8  // val
			n += 2  // name len
			n += uintptr(len(in.name))
		}
	}
	return n
}

func main() {
	inputBuf := make([]byte, 4096)
	outBuf := make([]byte, 1024)

	fd, _, errn := Syscall(5, Sliceptr(inputName[:]), 0, 0, 0, 0, 0)
	if errn != 0 {
		exitDOS(2)
	}
	readN, _, rerr := Syscall(3, fd, Sliceptr(inputBuf), uintptr(len(inputBuf)-1), 0, 0, 0)
	Syscall(6, fd, 0, 0, 0, 0, 0)
	if rerr != 0 {
		exitDOS(3)
	}
	n := int(readN)
	if n <= 0 {
		exitDOS(46)
	}
	var p parser
	p.init(inputBuf, n)
	if !p.parse() {
		if p.errCode != 0 {
			exitDOS(p.errCode)
		}
		exitDOS(5)
	}
	// Reuse input buffer as transient TIR output workspace to avoid extra heap growth.
	outBuf = inputBuf
	outN := encodeTIR(&p, outBuf)
	if outN <= 0 {
		exitDOS(6)
	}

	outFD, _, outErr := Syscall(5, Sliceptr(outputName[:]), 1, 0, 0, 0, 0)
	if outErr != 0 {
		exitDOS(7)
	}
	nw, _, werr := Syscall(4, outFD, Sliceptr(outBuf), uintptr(outN), 0, 0, 0)
	Syscall(6, outFD, 0, 0, 0, 0, 0)
	if werr != 0 || int(nw) != outN {
		exitDOS(8)
	}

	exitDOS(0)
}

//rtg:internal Syscall
func Syscall(num, a0, a1, a2, a3, a4, a5 uintptr) (r1 uintptr, r2 uintptr, err uintptr)

//rtg:internal Sliceptr
func Sliceptr(b []byte) uintptr

func exitDOS(code int) {
	Syscall(252, uintptr(code), 0, 0, 0, 0, 0)
	for {
	}
}
