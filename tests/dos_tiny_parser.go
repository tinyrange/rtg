package main

const (
	opConstStr = 1
	opLocalGet = 4
	opLocalSet = 5
	opDrop     = 11
	opReturn   = 46
)

const (
	tokEOF = iota
	tokIdent
	tokString
	tokPackage
	tokFunc
	tokVar
	tokMain
	tokPrint
	tokStringType
	tokLParen
	tokRParen
	tokLBrace
	tokRBrace
	tokAssign
	tokSemi
)

type token struct {
	kind int
	s0   int
	s1   int
}

type inst struct {
	op  int
	arg int
	s0  int
	s1  int
}

type parser struct {
	src []byte
	n   int
	pos int
	tok token

	errCode int
}

var gHaveMsg bool
var gCode []inst
var gCodeN int

var inputName = [...]byte{'I', 'N', 'P', 'U', 'T', '.', 'G', 'O', 0}
var outputName = [...]byte{'P', 'R', 'O', 'G', '.', 'T', 'I', 'R', 0}

func isAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isAlnum(c byte) bool {
	return isAlpha(c) || (c >= '0' && c <= '9')
}

func eqBytes(src []byte, a int, b int, lit string) bool {
	if b-a != len(lit) {
		return false
	}
	for i := 0; i < len(lit); i++ {
		if src[a+i] != lit[i] {
			return false
		}
	}
	return true
}

func keywordKind(src []byte, s0 int, s1 int) int {
	if eqBytes(src, s0, s1, "package") {
		return tokPackage
	}
	if eqBytes(src, s0, s1, "func") {
		return tokFunc
	}
	if eqBytes(src, s0, s1, "var") {
		return tokVar
	}
	if eqBytes(src, s0, s1, "main") {
		return tokMain
	}
	if eqBytes(src, s0, s1, "print") {
		return tokPrint
	}
	if eqBytes(src, s0, s1, "string") {
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
		s1 := p.pos
		return token{kind: keywordKind(p.src, s0, s1), s0: s0, s1: s1}
	}
	if c == '"' {
		p.pos++
		s0 := p.pos
		for p.pos < p.n {
			ch := p.src[p.pos]
			if ch == '\\' && p.pos+1 < p.n {
				p.pos += 2
				continue
			}
			if ch == '"' {
				s1 := p.pos
				p.pos++
				return token{kind: tokString, s0: s0, s1: s1}
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
	case '=':
		return token{kind: tokAssign}
	case ';':
		return token{kind: tokSemi}
	default:
		return token{kind: tokEOF}
	}
}

func (p *parser) init(src []byte, n int) {
	p.src = src
	p.n = n
	p.pos = 0
	p.tok = p.nextToken()
	gHaveMsg = false
	if len(gCode) == 0 {
		gCode = make([]inst, 64)
	}
	gCodeN = 0
}

func (p *parser) adv() {
	p.tok = p.nextToken()
}

func (p *parser) eat(k int) bool {
	if p.tok.kind != k {
		return false
	}
	p.adv()
	return true
}

func (p *parser) emit(op int, arg int, s0 int, s1 int) bool {
	if gCodeN >= len(gCode) {
		p.errCode = 190
		return false
	}
	gCode[gCodeN] = inst{op: op, arg: arg, s0: s0, s1: s1}
	gCodeN++
	return true
}

func (p *parser) parsePrimary() bool {
	if p.tok.kind == tokString {
		t := p.tok
		p.adv()
		return p.emit(opConstStr, 0, t.s0, t.s1)
	}
	if p.tok.kind == tokIdent || p.tok.kind == tokMain {
		if !eqBytes(p.src, p.tok.s0, p.tok.s1, "msg") {
			p.errCode = 31
			return false
		}
		gHaveMsg = true
		p.adv()
		return p.emit(opLocalGet, 0, 0, 0)
	}
	p.errCode = 32
	return false
}

func (p *parser) parseAssignStringRHS() bool {
	// tok currently '='; p.pos is right after '=' byte.
	i := p.pos
	for i < p.n {
		c := p.src[i]
		if c == ' ' || c == '\t' || c == '\r' {
			i++
			continue
		}
		break
	}
	if i >= p.n || p.src[i] != '"' {
		p.errCode = 44
		return false
	}
	i++
	s0 := i
	for i < p.n {
		c := p.src[i]
		if c == '\\' && i+1 < p.n {
			i += 2
			continue
		}
		if c == '"' {
			s1 := i
	if gCodeN != 0 {
		p.errCode = 191
		return false
	}
			if !p.emit(opConstStr, 0, s0, s1) {
				if p.errCode == 0 {
					p.errCode = 45
				}
				return false
			}
			p.pos = i + 1
			p.tok = p.nextToken()
			return true
		}
		i++
	}
	p.errCode = 44
	return false
}

func (p *parser) parseStmt() bool {
	if p.tok.kind == tokVar {
		p.adv()
		if p.tok.kind != tokIdent || !eqBytes(p.src, p.tok.s0, p.tok.s1, "msg") {
			p.errCode = 41
			return false
		}
		gHaveMsg = true
		p.adv()
		if !p.eat(tokStringType) {
			p.errCode = 42
			return false
		}
		if p.tok.kind == tokSemi {
			p.adv()
		}
		return true
	}
	if p.tok.kind == tokIdent || p.tok.kind == tokMain {
		if !eqBytes(p.src, p.tok.s0, p.tok.s1, "msg") {
			p.errCode = 43
			return false
		}
		gHaveMsg = true
		p.adv()
		if p.tok.kind != tokAssign {
			p.errCode = 43
			return false
		}
		if !p.parseAssignStringRHS() {
			if p.errCode == 0 {
				p.errCode = 44
			}
			return false
		}
		if !p.emit(opLocalSet, 0, 0, 0) {
			p.errCode = 45
			return false
		}
		if p.tok.kind == tokSemi {
			p.adv()
		}
		return true
	}
	if p.tok.kind == tokPrint {
		p.adv()
		if !p.eat(tokLParen) {
			p.errCode = 46
			return false
		}
		if !p.parsePrimary() {
			if p.errCode == 0 {
				p.errCode = 47
			}
			return false
		}
		if !p.eat(tokRParen) {
			p.errCode = 48
			return false
		}
		if !p.emit(opDrop, 0, 0, 0) {
			p.errCode = 49
			return false
		}
		if p.tok.kind == tokSemi {
			p.adv()
		}
		return true
	}
	p.errCode = 40
	return false
}

func (p *parser) parse() bool {
	if !p.eat(tokPackage) {
		p.errCode = 11
		return false
	}
	if !p.eat(tokMain) {
		p.errCode = 12
		return false
	}
	if !p.eat(tokFunc) {
		p.errCode = 13
		return false
	}
	if !p.eat(tokMain) {
		p.errCode = 14
		return false
	}
	if !p.eat(tokLParen) || !p.eat(tokRParen) || !p.eat(tokLBrace) {
		p.errCode = 15
		return false
	}
	for p.tok.kind == tokSemi {
		p.adv()
	}
	// var msg string
	if !p.eat(tokVar) {
		p.errCode = 16
		return false
	}
	if p.tok.kind != tokIdent || !eqBytes(p.src, p.tok.s0, p.tok.s1, "msg") {
		p.errCode = 41
		return false
	}
	gHaveMsg = true
	p.adv()
	if !p.eat(tokStringType) {
		p.errCode = 42
		return false
	}
	for p.tok.kind == tokSemi {
		p.adv()
	}
	// msg = \"...\"
	if p.tok.kind != tokIdent || !eqBytes(p.src, p.tok.s0, p.tok.s1, "msg") {
		p.errCode = 43
		return false
	}
	p.adv()
	if p.tok.kind != tokAssign {
		p.errCode = 43
		return false
	}
	if !p.parseAssignStringRHS() {
		if p.errCode == 0 {
			p.errCode = 44
		}
		return false
	}
	if !p.emit(opLocalSet, 0, 0, 0) {
		p.errCode = 45
		return false
	}
	for p.tok.kind == tokSemi {
		p.adv()
	}
	// print(msg)
	if !p.eat(tokPrint) || !p.eat(tokLParen) {
		p.errCode = 46
		return false
	}
	if p.tok.kind != tokIdent || !eqBytes(p.src, p.tok.s0, p.tok.s1, "msg") {
		p.errCode = 47
		return false
	}
	p.adv()
	if !p.eat(tokRParen) {
		p.errCode = 48
		return false
	}
	if !p.emit(opLocalGet, 0, 0, 0) || !p.emit(opDrop, 0, 0, 0) {
		p.errCode = 49
		return false
	}
	if !p.eat(tokRBrace) {
		p.errCode = 17
		return false
	}
	if !p.emit(opReturn, 0, 0, 0) {
		p.errCode = 18
		return false
	}
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

func (e *enc) putStrLit(s string) bool {
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

func (e *enc) putStrRange(src []byte, s0 int, s1 int) bool {
	n := s1 - s0
	if n < 0 {
		return false
	}
	if !e.putU16(n) {
		return false
	}
	for i := 0; i < n; i++ {
		if !e.put8(src[s0+i]) {
			return false
		}
	}
	return true
}

func encodeTIR(src []byte, p *parser, out []byte) int {
	e := enc{b: out, n: 0}
	if !e.put8('T') || !e.put8('I') || !e.put8('R') || !e.put8('3') {
		return -1
	}
	if !e.putU16(1) {
		return -1
	}
	if !e.putStrLit("main.main") {
		return -1
	}
	if !e.putU16(0) || !e.putU16(0) {
		return -1
	}
	if gHaveMsg {
		if !e.putU16(1) || !e.putStrLit("msg") {
			return -1
		}
	} else {
		if !e.putU16(0) {
			return -1
		}
	}
	if !e.putU16(gCodeN) {
		return -1
	}
	for i := 0; i < gCodeN; i++ {
		in := gCode[i]
		if !e.putU16(in.op) {
			return -1
		}
		if !e.putI32(in.arg) {
			return -1
		}
		if !e.putI64(0) {
			return -1
		}
		if in.op == opConstStr {
			if !e.putStrRange(src, in.s0, in.s1) {
				return -1
			}
		} else {
			if !e.putU16(0) {
				return -1
			}
		}
	}
	return e.n
}

func main() {
	inputBuf := make([]byte, 256)
	outBuf := make([]byte, 512)
	fd, _, errn := Syscall(5, Sliceptr(inputName[:]), 0, 0, 0, 0, 0)
	if errn != 0 {
		exitDOS(2)
	}
	readN, _, rerr := Syscall(3, fd, Sliceptr(inputBuf), 90, 0, 0, 0)
	if rerr != 0 {
		Syscall(6, fd, 0, 0, 0, 0, 0)
		exitDOS(2)
	}
	Syscall(6, fd, 0, 0, 0, 0, 0)
	n := int(readN)
	if n <= 0 {
		exitDOS(61)
	}

	var p parser
	p.init(inputBuf, n)
	if gCodeN != 0 {
		exitDOS(180 + gCodeN)
	}
	if !p.parse() {
		if p.errCode != 0 {
			exitDOS(p.errCode)
		}
		exitDOS(3)
	}

	outN := encodeTIR(inputBuf, &p, outBuf)
	if outN <= 0 {
		exitDOS(4)
	}

	outFD, _, outErr := Syscall(5, Sliceptr(outputName[:]), 1, 0, 0, 0, 0)
	if outErr != 0 {
		exitDOS(5)
	}
	nw, _, werr := Syscall(4, outFD, Sliceptr(outBuf), uintptr(outN), 0, 0, 0)
	Syscall(6, outFD, 0, 0, 0, 0, 0)
	if werr != 0 || int(nw) != outN {
		exitDOS(6)
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
