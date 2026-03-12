package frontend

import (
	"fmt"
	"os"
	"strings"

	"j5.nz/rtg/std/compiler/backend/vm"
	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
	"j5.nz/rtg/std/compiler/stdlib"
)

// TokenKind represents the type of a lexical token.
type TokenKind int

const (
	// Literals
	TOKEN_EOF TokenKind = iota
	TOKEN_IDENT
	TOKEN_INT
	TOKEN_FLOAT
	TOKEN_IMAG
	TOKEN_STRING
	TOKEN_RAW_STRING
	TOKEN_RUNE
	TOKEN_COMMENT

	// Keywords
	TOKEN_PACKAGE
	TOKEN_IMPORT
	TOKEN_FUNC
	TOKEN_TYPE
	TOKEN_STRUCT
	TOKEN_INTERFACE
	TOKEN_VAR
	TOKEN_CONST
	TOKEN_IF
	TOKEN_ELSE
	TOKEN_FOR
	TOKEN_RANGE
	TOKEN_SWITCH
	TOKEN_CASE
	TOKEN_DEFAULT
	TOKEN_RETURN
	TOKEN_BREAK
	TOKEN_CONTINUE
	TOKEN_MAP
	TOKEN_NIL
	TOKEN_TRUE
	TOKEN_FALSE
	TOKEN_DEFER
	TOKEN_IOTA
	TOKEN_CHAN
	TOKEN_GO
	TOKEN_SELECT
	TOKEN_GOTO
	TOKEN_FALLTHROUGH

	// Operators
	TOKEN_PLUS
	TOKEN_MINUS
	TOKEN_STAR
	TOKEN_SLASH
	TOKEN_PERCENT
	TOKEN_EQ
	TOKEN_NEQ
	TOKEN_LT
	TOKEN_GT
	TOKEN_LEQ
	TOKEN_GEQ
	TOKEN_AND
	TOKEN_OR
	TOKEN_NOT
	TOKEN_AMPERSAND
	TOKEN_PIPE
	TOKEN_CARET
	TOKEN_SHL
	TOKEN_SHR

	// Assignment
	TOKEN_ASSIGN
	TOKEN_DEFINE
	TOKEN_PLUS_ASSIGN
	TOKEN_MINUS_ASSIGN
	TOKEN_STAR_ASSIGN
	TOKEN_SLASH_ASSIGN
	TOKEN_PERCENT_ASSIGN
	TOKEN_OR_ASSIGN
	TOKEN_AND_ASSIGN
	TOKEN_CARET_ASSIGN
	TOKEN_SHL_ASSIGN
	TOKEN_SHR_ASSIGN

	// Punctuation
	TOKEN_LPAREN
	TOKEN_RPAREN
	TOKEN_LBRACE
	TOKEN_RBRACE
	TOKEN_LBRACK
	TOKEN_RBRACK
	TOKEN_COMMA
	TOKEN_DOT
	TOKEN_COLON
	TOKEN_SEMICOLON
	TOKEN_ELLIPSIS
	TOKEN_INC
	TOKEN_DIRECTIVE
	TOKEN_DEC
)

var tokenNames = map[TokenKind]string{
	TOKEN_EOF: "EOF", TOKEN_IDENT: "IDENT", TOKEN_INT: "INT",
	TOKEN_FLOAT: "FLOAT", TOKEN_IMAG: "IMAG",
	TOKEN_STRING: "STRING", TOKEN_RAW_STRING: "RAW_STRING", TOKEN_RUNE: "RUNE", TOKEN_COMMENT: "COMMENT",
	TOKEN_PACKAGE: "package", TOKEN_IMPORT: "import", TOKEN_FUNC: "func",
	TOKEN_TYPE: "type", TOKEN_STRUCT: "struct", TOKEN_INTERFACE: "interface",
	TOKEN_VAR: "var", TOKEN_CONST: "const", TOKEN_IF: "if", TOKEN_ELSE: "else",
	TOKEN_FOR: "for", TOKEN_RANGE: "range", TOKEN_SWITCH: "switch",
	TOKEN_CASE: "case", TOKEN_DEFAULT: "default", TOKEN_RETURN: "return",
	TOKEN_BREAK: "break", TOKEN_CONTINUE: "continue", TOKEN_MAP: "map",
	TOKEN_NIL: "nil", TOKEN_TRUE: "true", TOKEN_FALSE: "false",
	TOKEN_DEFER: "defer", TOKEN_IOTA: "iota",
	TOKEN_CHAN: "chan", TOKEN_GO: "go", TOKEN_SELECT: "select",
	TOKEN_GOTO: "goto", TOKEN_FALLTHROUGH: "fallthrough",
	TOKEN_PLUS: "+", TOKEN_MINUS: "-", TOKEN_STAR: "*", TOKEN_SLASH: "/",
	TOKEN_PERCENT: "%", TOKEN_EQ: "==", TOKEN_NEQ: "!=",
	TOKEN_LT: "<", TOKEN_GT: ">", TOKEN_LEQ: "<=", TOKEN_GEQ: ">=",
	TOKEN_AND: "&&", TOKEN_OR: "||", TOKEN_NOT: "!",
	TOKEN_AMPERSAND: "&", TOKEN_PIPE: "|", TOKEN_CARET: "^",
	TOKEN_SHL: "<<", TOKEN_SHR: ">>",
	TOKEN_ASSIGN: "=", TOKEN_DEFINE: ":=", TOKEN_PLUS_ASSIGN: "+=", TOKEN_MINUS_ASSIGN: "-=",
	TOKEN_STAR_ASSIGN: "*=", TOKEN_SLASH_ASSIGN: "/=", TOKEN_PERCENT_ASSIGN: "%=",
	TOKEN_OR_ASSIGN: "|=", TOKEN_AND_ASSIGN: "&=", TOKEN_CARET_ASSIGN: "^=",
	TOKEN_SHL_ASSIGN: "<<=", TOKEN_SHR_ASSIGN: ">>=",
	TOKEN_LPAREN: "(", TOKEN_RPAREN: ")", TOKEN_LBRACE: "{", TOKEN_RBRACE: "}",
	TOKEN_LBRACK: "[", TOKEN_RBRACK: "]", TOKEN_COMMA: ",", TOKEN_DOT: ".",
	TOKEN_COLON: ":", TOKEN_SEMICOLON: ";", TOKEN_ELLIPSIS: "...",
	TOKEN_INC:       "++",
	TOKEN_DIRECTIVE: "directive",
	TOKEN_DEC:       "--",
}

func tokenName(k TokenKind) string {
	s, ok := tokenNames[k]
	if ok {
		return s
	}
	return "?"
}

var keywords = map[string]TokenKind{
	"package": TOKEN_PACKAGE, "import": TOKEN_IMPORT, "func": TOKEN_FUNC,
	"type": TOKEN_TYPE, "struct": TOKEN_STRUCT, "interface": TOKEN_INTERFACE,
	"var": TOKEN_VAR, "const": TOKEN_CONST, "if": TOKEN_IF, "else": TOKEN_ELSE,
	"for": TOKEN_FOR, "range": TOKEN_RANGE, "switch": TOKEN_SWITCH,
	"case": TOKEN_CASE, "default": TOKEN_DEFAULT, "return": TOKEN_RETURN,
	"break": TOKEN_BREAK, "continue": TOKEN_CONTINUE, "map": TOKEN_MAP,
	"nil": TOKEN_NIL, "true": TOKEN_TRUE, "false": TOKEN_FALSE,
	"defer": TOKEN_DEFER, "iota": TOKEN_IOTA,
	"chan": TOKEN_CHAN, "go": TOKEN_GO, "select": TOKEN_SELECT,
	"goto": TOKEN_GOTO, "fallthrough": TOKEN_FALLTHROUGH,
}

// Token represents a lexical token.
type Token struct {
	Kind TokenKind
	Val  string
	Line int
	Col  int
}

func (t Token) String() string {
	if t.Val != "" {
		return tokenName(t.Kind) + "(" + t.Val + ")"
	}
	return tokenName(t.Kind)
}

// Lexer tokenizes Go source code.
type Lexer struct {
	src  string
	pos  int
	line int
	col  int
}

func NewLexer(src string) *Lexer {
	return &Lexer{src: src, pos: 0, line: 1, col: 1}
}

//rtg:noprofile
func (l *Lexer) atEnd() bool {
	return l.pos >= len(l.src)
}

//rtg:noprofile
func (l *Lexer) peek() byte {
	if l.atEnd() {
		return 0
	}
	return l.src[l.pos]
}

//rtg:noprofile
func (l *Lexer) peekAt(offset int) byte {
	p := l.pos + offset
	if p >= len(l.src) {
		return 0
	}
	return l.src[p]
}

//rtg:noprofile
func (l *Lexer) advance() byte {
	ch := l.src[l.pos]
	l.pos++
	if ch == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return ch
}

func needsSemicolon(kind TokenKind) bool {
	if kind == TOKEN_IDENT || kind == TOKEN_INT || kind == TOKEN_FLOAT || kind == TOKEN_IMAG || kind == TOKEN_STRING || kind == TOKEN_RAW_STRING || kind == TOKEN_RUNE {
		return true
	}
	if kind == TOKEN_RPAREN || kind == TOKEN_RBRACK || kind == TOKEN_RBRACE {
		return true
	}
	if kind == TOKEN_INC || kind == TOKEN_DEC || kind == TOKEN_BREAK || kind == TOKEN_CONTINUE || kind == TOKEN_RETURN {
		return true
	}
	if kind == TOKEN_FALLTHROUGH {
		return true
	}
	if kind == TOKEN_TRUE || kind == TOKEN_FALSE || kind == TOKEN_NIL || kind == TOKEN_IOTA {
		return true
	}
	return false
}

func isLetter(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isExpDigitStart(ch byte, next byte) bool {
	if isDigit(ch) {
		return true
	}
	if (ch == '+' || ch == '-') && isDigit(next) {
		return true
	}
	return false
}

func (l *Lexer) skipWhitespaceAndComments() (bool, Token, bool) {
	sawNewline := false
	var directive Token
	hasDirective := false
	for !l.atEnd() {
		ch := l.peek()
		if ch == '\n' {
			sawNewline = true
			l.advance()
		} else if ch == ' ' || ch == '\t' || ch == '\r' {
			l.advance()
		} else if ch == '/' && l.peekAt(1) == '/' {
			cLine := l.line
			cCol := l.col
			l.advance()
			l.advance()
			start := l.pos
			for !l.atEnd() && l.peek() != '\n' && l.peek() != '\r' {
				l.advance()
			}
			val := l.src[start:l.pos]
			trimmed := val
			for len(trimmed) > 0 && (trimmed[0] == ' ' || trimmed[0] == '\t') {
				trimmed = trimmed[1:]
			}
			if len(trimmed) >= 4 && trimmed[0:4] == "rtg:" {
				directive = Token{Kind: TOKEN_DIRECTIVE, Val: trimmed[4:len(trimmed)], Line: cLine, Col: cCol}
				hasDirective = true
			} else if len(trimmed) >= 9 && trimmed[0:9] == "go:embed " {
				directive = Token{Kind: TOKEN_DIRECTIVE, Val: "embed " + trimmed[9:len(trimmed)], Line: cLine, Col: cCol}
				hasDirective = true
			}
			sawNewline = true
		} else {
			break
		}
	}
	return sawNewline, directive, hasDirective
}

func (l *Lexer) scanIdent() Token {
	line := l.line
	col := l.col
	start := l.pos
	pos := start
	src := l.src
	for pos < len(src) {
		ch := src[pos]
		isIdentChar := (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_' || (ch >= '0' && ch <= '9')
		if !isIdentChar {
			break
		}
		pos++
	}
	l.pos = pos
	l.col += pos - start
	val := l.src[start:l.pos]
	kind, isKeyword := keywords[val]
	if !isKeyword {
		kind = TOKEN_IDENT
	}
	return Token{Kind: kind, Val: val, Line: line, Col: col}
}

func (l *Lexer) scanNumber() Token {
	line := l.line
	col := l.col
	start := l.pos
	pos := start
	src := l.src
	n := len(src)
	isFloat := false
	isLetterByte := func(ch byte) bool {
		return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
	}
	if pos+1 < n && src[pos] == '0' && (src[pos+1] == 'x' || src[pos+1] == 'X') {
		pos += 2
		for pos < n {
			ch := src[pos]
			if !((ch >= '0' && ch <= '9') || isLetterByte(ch) || ch == '_') {
				break
			}
			pos++
		}
	} else if pos+1 < n && src[pos] == '0' && (src[pos+1] == 'b' || src[pos+1] == 'B') {
		pos += 2
		for pos < n {
			ch := src[pos]
			if !((ch >= '0' && ch <= '9') || isLetterByte(ch) || ch == '_') {
				break
			}
			pos++
		}
	} else if pos+1 < n && src[pos] == '0' && (src[pos+1] == 'o' || src[pos+1] == 'O') {
		pos += 2
		for pos < n {
			ch := src[pos]
			if !((ch >= '0' && ch <= '9') || isLetterByte(ch) || ch == '_') {
				break
			}
			pos++
		}
	} else {
		for pos < n && ((src[pos] >= '0' && src[pos] <= '9') || src[pos] == '_') {
			pos++
		}
		if pos+1 < n && src[pos] == '.' && (src[pos+1] >= '0' && src[pos+1] <= '9') {
			isFloat = true
			pos++
			for pos < n && ((src[pos] >= '0' && src[pos] <= '9') || src[pos] == '_') {
				pos++
			}
		}
		if pos < n && (src[pos] == 'e' || src[pos] == 'E') {
			next := byte(0)
			next2 := byte(0)
			if pos+1 < n {
				next = src[pos+1]
			}
			if pos+2 < n {
				next2 = src[pos+2]
			}
			if (next >= '0' && next <= '9') || ((next == '+' || next == '-') && (next2 >= '0' && next2 <= '9')) {
				isFloat = true
				pos++
				if pos < n && (src[pos] == '+' || src[pos] == '-') {
					pos++
				}
				for pos < n && ((src[pos] >= '0' && src[pos] <= '9') || src[pos] == '_') {
					pos++
				}
			}
		}
		if pos < n && src[pos] == 'i' {
			pos++
			l.pos = pos
			l.col += pos - start
			return Token{Kind: TOKEN_IMAG, Val: l.src[start:l.pos], Line: line, Col: col}
		}
	}
	l.pos = pos
	l.col += pos - start
	if isFloat {
		return Token{Kind: TOKEN_FLOAT, Val: l.src[start:l.pos], Line: line, Col: col}
	}
	return Token{Kind: TOKEN_INT, Val: l.src[start:l.pos], Line: line, Col: col}
}

func (l *Lexer) scanString() Token {
	line := l.line
	col := l.col
	l.advance() // skip opening "
	start := l.pos
	for !l.atEnd() && l.peek() != '"' {
		if l.peek() == '\\' {
			l.advance()
		}
		l.advance()
	}
	val := l.src[start:l.pos]
	if !l.atEnd() {
		l.advance() // skip closing "
	}
	return Token{Kind: TOKEN_STRING, Val: val, Line: line, Col: col}
}

func (l *Lexer) scanRawString() Token {
	line := l.line
	col := l.col
	l.advance() // skip opening `
	start := l.pos
	for !l.atEnd() && l.peek() != '`' {
		l.advance()
	}
	val := l.src[start:l.pos]
	if !l.atEnd() {
		l.advance() // skip closing `
	}
	return Token{Kind: TOKEN_RAW_STRING, Val: val, Line: line, Col: col}
}

func (l *Lexer) scanRune() Token {
	line := l.line
	col := l.col
	l.advance() // skip opening '
	start := l.pos
	for !l.atEnd() && l.peek() != '\'' && l.peek() != '\n' && l.peek() != '\r' {
		if l.peek() == '\\' && l.peekAt(1) != 0 {
			l.advance()
			l.advance()
			continue
		}
		l.advance()
	}
	val := l.src[start:l.pos]
	if !l.atEnd() && l.peek() == '\'' {
		l.advance() // skip closing '
	}
	return Token{Kind: TOKEN_RUNE, Val: val, Line: line, Col: col}
}

func (l *Lexer) Tokenize() []Token {
	capHint := len(l.src)/3 + 2
	if capHint < 32 {
		capHint = 32
	}
	tokens := make([]Token, 0, capHint)
	lastKind := TOKEN_EOF
	for {
		sawNewline, directive, hasDirective := l.skipWhitespaceAndComments()
		if sawNewline && needsSemicolon(lastKind) {
			tokens = append(tokens, Token{Kind: TOKEN_SEMICOLON, Val: "", Line: l.line, Col: l.col})
			lastKind = TOKEN_SEMICOLON
		}
		if hasDirective {
			tokens = append(tokens, directive)
			lastKind = TOKEN_DIRECTIVE
		}
		if l.atEnd() {
			if needsSemicolon(lastKind) {
				tokens = append(tokens, Token{Kind: TOKEN_SEMICOLON, Val: "", Line: l.line, Col: l.col})
			}
			tokens = append(tokens, Token{Kind: TOKEN_EOF, Line: l.line, Col: l.col})
			break
		}
		ch := l.peek()
		var tok Token
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_' {
			tok = l.scanIdent()
		} else if ch >= '0' && ch <= '9' {
			tok = l.scanNumber()
		} else if ch == '"' {
			tok = l.scanString()
		} else if ch == '`' {
			tok = l.scanRawString()
		} else if ch == '\'' {
			tok = l.scanRune()
		} else {
			tok = l.scanOperator()
		}
		tokens = append(tokens, tok)
		lastKind = tok.Kind
	}
	return tokens
}

func (l *Lexer) scanOperator() Token {
	line := l.line
	col := l.col
	ch := l.advance()
	switch ch {
	case '+':
		if l.peek() == '+' {
			l.advance()
			return Token{Kind: TOKEN_INC, Line: line, Col: col}
		}
		if l.peek() == '=' {
			l.advance()
			return Token{Kind: TOKEN_PLUS_ASSIGN, Line: line, Col: col}
		}
		return Token{Kind: TOKEN_PLUS, Line: line, Col: col}
	case '-':
		if l.peek() == '-' {
			l.advance()
			return Token{Kind: TOKEN_DEC, Line: line, Col: col}
		}
		if l.peek() == '=' {
			l.advance()
			return Token{Kind: TOKEN_MINUS_ASSIGN, Line: line, Col: col}
		}
		return Token{Kind: TOKEN_MINUS, Line: line, Col: col}
	case '*':
		if l.peek() == '=' {
			l.advance()
			return Token{Kind: TOKEN_STAR_ASSIGN, Line: line, Col: col}
		}
		return Token{Kind: TOKEN_STAR, Line: line, Col: col}
	case '/':
		if l.peek() == '=' {
			l.advance()
			return Token{Kind: TOKEN_SLASH_ASSIGN, Line: line, Col: col}
		}
		return Token{Kind: TOKEN_SLASH, Line: line, Col: col}
	case '%':
		if l.peek() == '=' {
			l.advance()
			return Token{Kind: TOKEN_PERCENT_ASSIGN, Line: line, Col: col}
		}
		return Token{Kind: TOKEN_PERCENT, Line: line, Col: col}
	case '=':
		if l.peek() == '=' {
			l.advance()
			return Token{Kind: TOKEN_EQ, Line: line, Col: col}
		}
		return Token{Kind: TOKEN_ASSIGN, Line: line, Col: col}
	case '!':
		if l.peek() == '=' {
			l.advance()
			return Token{Kind: TOKEN_NEQ, Line: line, Col: col}
		}
		return Token{Kind: TOKEN_NOT, Line: line, Col: col}
	case '<':
		if l.peek() == '=' {
			l.advance()
			return Token{Kind: TOKEN_LEQ, Line: line, Col: col}
		}
		if l.peek() == '<' {
			l.advance()
			if l.peek() == '=' {
				l.advance()
				return Token{Kind: TOKEN_SHL_ASSIGN, Line: line, Col: col}
			}
			return Token{Kind: TOKEN_SHL, Line: line, Col: col}
		}
		return Token{Kind: TOKEN_LT, Line: line, Col: col}
	case '>':
		if l.peek() == '=' {
			l.advance()
			return Token{Kind: TOKEN_GEQ, Line: line, Col: col}
		}
		if l.peek() == '>' {
			l.advance()
			if l.peek() == '=' {
				l.advance()
				return Token{Kind: TOKEN_SHR_ASSIGN, Line: line, Col: col}
			}
			return Token{Kind: TOKEN_SHR, Line: line, Col: col}
		}
		return Token{Kind: TOKEN_GT, Line: line, Col: col}
	case '&':
		if l.peek() == '&' {
			l.advance()
			return Token{Kind: TOKEN_AND, Line: line, Col: col}
		}
		if l.peek() == '=' {
			l.advance()
			return Token{Kind: TOKEN_AND_ASSIGN, Line: line, Col: col}
		}
		return Token{Kind: TOKEN_AMPERSAND, Line: line, Col: col}
	case '|':
		if l.peek() == '|' {
			l.advance()
			return Token{Kind: TOKEN_OR, Line: line, Col: col}
		}
		if l.peek() == '=' {
			l.advance()
			return Token{Kind: TOKEN_OR_ASSIGN, Line: line, Col: col}
		}
		return Token{Kind: TOKEN_PIPE, Line: line, Col: col}
	case '^':
		if l.peek() == '=' {
			l.advance()
			return Token{Kind: TOKEN_CARET_ASSIGN, Line: line, Col: col}
		}
		return Token{Kind: TOKEN_CARET, Line: line, Col: col}
	case '(':
		return Token{Kind: TOKEN_LPAREN, Line: line, Col: col}
	case ')':
		return Token{Kind: TOKEN_RPAREN, Line: line, Col: col}
	case '{':
		return Token{Kind: TOKEN_LBRACE, Line: line, Col: col}
	case '}':
		return Token{Kind: TOKEN_RBRACE, Line: line, Col: col}
	case '[':
		return Token{Kind: TOKEN_LBRACK, Line: line, Col: col}
	case ']':
		return Token{Kind: TOKEN_RBRACK, Line: line, Col: col}
	case ',':
		return Token{Kind: TOKEN_COMMA, Line: line, Col: col}
	case '.':
		if l.peek() == '.' && l.peekAt(1) == '.' {
			l.advance()
			l.advance()
			return Token{Kind: TOKEN_ELLIPSIS, Line: line, Col: col}
		}
		return Token{Kind: TOKEN_DOT, Line: line, Col: col}
	case ':':
		if l.peek() == '=' {
			l.advance()
			return Token{Kind: TOKEN_DEFINE, Line: line, Col: col}
		}
		return Token{Kind: TOKEN_COLON, Line: line, Col: col}
	case ';':
		return Token{Kind: TOKEN_SEMICOLON, Line: line, Col: col}
	}
	return Token{Kind: TOKEN_EOF, Val: string(ch), Line: line, Col: col}
}

// NodeKind represents the type of an AST node.
type NodeKind int

const (
	NFile NodeKind = iota
	NImport
	NDeclGroup
	NFunc
	NTypeDecl
	NField
	NBlock
	NIf
	NFor
	NSwitch
	NCase
	NReturn
	NAssign
	NVarDecl
	NConstDecl
	NExprStmt
	NBranch
	NIdent
	NIntLit
	NFloatLit
	NStringLit
	NRuneLit
	NBasicLit
	NBinaryExpr
	NUnaryExpr
	NCallExpr
	NIndexExpr
	NSelectorExpr
	NTypeAssertExpr
	NCompositeLit
	NKeyValue
	NPointerType
	NSliceType
	NArrayType
	NMapType
	NFuncType
	NStructType
	NInterfaceType
	NIncStmt
	NDeferStmt
	NSliceExpr
	NDirective
	NDecStmt
)

// Node is the universal AST node.
type Node struct {
	Kind  NodeKind
	Pos   int
	Name  string
	Nodes []*Node
	X     *Node
	Y     *Node
	Body  *Node
	Type  *Node
}

func newDeclGroup(name string, pos int, nodes []*Node) *Node {
	return &Node{Kind: NDeclGroup, Name: name, Nodes: nodes, Pos: pos}
}

func isDeclGroup(node *Node, name string) bool {
	if node == nil || node.Kind != NDeclGroup {
		return false
	}
	return name == "" || node.Name == name
}

// Parser parses a sequence of tokens into an AST.
type Parser struct {
	tokens    []Token
	pos       int
	errors    []string
	noCompLit bool
}

func NewParser(tokens []Token) *Parser {
	return &Parser{tokens: tokens, pos: 0}
}

func (p *Parser) peek() Token {
	if p.pos >= len(p.tokens) {
		return Token{Kind: TOKEN_EOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) advance() Token {
	if p.pos >= len(p.tokens) {
		return Token{Kind: TOKEN_EOF}
	}
	tok := p.tokens[p.pos]
	p.pos++
	return tok
}

func (p *Parser) at(kind TokenKind) bool {
	if p.pos >= len(p.tokens) {
		return TOKEN_EOF == kind
	}
	return p.tokens[p.pos].Kind == kind
}

func (p *Parser) expect(kind TokenKind) Token {
	tok := p.advance()
	if tok.Kind != kind {
		p.errorf("expected %s, got %s at line %d col %d", tokenName(kind), tok.String(), tok.Line, tok.Col)
	}
	return tok
}

func (p *Parser) errorf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	p.errors = append(p.errors, msg)
}

func (p *Parser) skipSemicolon() {
	if p.at(TOKEN_SEMICOLON) {
		p.advance()
	}
}

func (p *Parser) skipBlock() {
	p.expect(TOKEN_LBRACE)
	depth := 1
	for depth > 0 && !p.at(TOKEN_EOF) {
		if p.at(TOKEN_LBRACE) {
			depth++
		}
		if p.at(TOKEN_RBRACE) {
			depth = depth - 1
		}
		p.advance()
	}
}

// ParseFile parses a complete Go source file.
func (p *Parser) ParseFile() *Node {
	file := &Node{Kind: NFile, Pos: p.peek().Line}

	// package clause
	p.expect(TOKEN_PACKAGE)
	pkgName := p.expect(TOKEN_IDENT)
	file.Name = pkgName.Val
	p.skipSemicolon()

	// imports
	for p.at(TOKEN_IMPORT) {
		file.Nodes = append(file.Nodes, p.parseImportGroup())
		p.skipSemicolon()
	}

	// top-level declarations
	for !p.at(TOKEN_EOF) {
		decl := p.parseTopDecl()
		if decl != nil {
			file.Nodes = append(file.Nodes, decl)
		}
		p.skipSemicolon()
	}

	return file
}

func (p *Parser) parseImportGroup() *Node {
	pos := p.peek().Line
	p.expect(TOKEN_IMPORT)
	imports := make([]*Node, 0, 1)
	if p.at(TOKEN_LPAREN) {
		p.advance()
		for !p.at(TOKEN_RPAREN) && !p.at(TOKEN_EOF) {
			spec := p.parseImportSpec()
			imports = append(imports, spec)
			p.skipSemicolon()
		}
		p.expect(TOKEN_RPAREN)
		return newDeclGroup("import", pos, imports)
	}
	spec := p.parseImportSpec()
	return spec
}

func (p *Parser) parseImportSpec() *Node {
	alias := ""
	aliasPos := p.peek().Line
	if p.at(TOKEN_IDENT) {
		aliasTok := p.advance()
		alias = aliasTok.Val
		aliasPos = aliasTok.Line
	} else if p.at(TOKEN_DOT) {
		aliasTok := p.advance()
		alias = "."
		aliasPos = aliasTok.Line
	}

	tok := p.expect(TOKEN_STRING)
	n := &Node{Kind: NImport, Name: tok.Val, Pos: tok.Line}
	if alias != "" {
		n.X = &Node{Kind: NIdent, Name: alias, Pos: aliasPos}
	}
	return n
}

func (p *Parser) parseTopDecl() *Node {
	switch p.peek().Kind {
	case TOKEN_DIRECTIVE:
		dir := p.advance()
		decl := p.parseTopDecl()
		return &Node{Kind: NDirective, Name: dir.Val, X: decl, Pos: dir.Line}
	case TOKEN_FUNC:
		return p.parseFuncDecl()
	case TOKEN_TYPE:
		return p.parseTypeDecl()
	case TOKEN_VAR:
		return p.parseVarDecl()
	case TOKEN_CONST:
		return p.parseConstDecl()
	}
	tok := p.advance()
	p.errorf("unexpected top-level token: %s at line %d", tok.String(), tok.Line)
	return nil
}

func (p *Parser) parseFuncDecl() *Node {
	pos := p.peek().Line
	p.expect(TOKEN_FUNC)
	node := &Node{Kind: NFunc, Pos: pos}

	// optional receiver
	if p.at(TOKEN_LPAREN) {
		p.advance()
		node.X = p.parseReceiver()
		p.expect(TOKEN_RPAREN)
	}

	// function name
	name := p.expect(TOKEN_IDENT)
	node.Name = name.Val

	// reject generic type parameters
	if p.at(TOKEN_LBRACK) {
		p.errorf("generic type parameters are not supported at line %d", p.peek().Line)
		p.advance()
		for !p.at(TOKEN_RBRACK) && !p.at(TOKEN_EOF) {
			p.advance()
		}
		if p.at(TOKEN_RBRACK) {
			p.advance()
		}
	}

	// parameters
	node.Nodes = p.parseFieldList(true)

	// result type(s)
	if canStartResult(p.peek().Kind) {
		node.Type = p.parseResultType()
	}

	// body
	if p.at(TOKEN_LBRACE) {
		node.Body = p.parseBlock()
	}
	return node
}

func (p *Parser) parseReceiver() *Node {
	node := &Node{Kind: NField, Pos: p.peek().Line}
	name := p.expect(TOKEN_IDENT)
	node.Name = name.Val
	node.Type = p.parseType()
	return node
}

func isTypeStart(kind TokenKind) bool {
	switch kind {
	case TOKEN_IDENT, TOKEN_STAR, TOKEN_LBRACK, TOKEN_MAP, TOKEN_FUNC, TOKEN_STRUCT, TOKEN_INTERFACE, TOKEN_CHAN:
		return true
	}
	return false
}

func canStartResult(kind TokenKind) bool {
	return kind == TOKEN_LPAREN || isTypeStart(kind)
}

func isAssignOp(kind TokenKind) bool {
	switch kind {
	case TOKEN_ASSIGN, TOKEN_DEFINE, TOKEN_PLUS_ASSIGN, TOKEN_MINUS_ASSIGN,
		TOKEN_STAR_ASSIGN, TOKEN_SLASH_ASSIGN, TOKEN_PERCENT_ASSIGN,
		TOKEN_OR_ASSIGN, TOKEN_AND_ASSIGN, TOKEN_CARET_ASSIGN,
		TOKEN_SHL_ASSIGN, TOKEN_SHR_ASSIGN:
		return true
	}
	return false
}

func isSimpleAssignOp(kind TokenKind) bool {
	return kind == TOKEN_ASSIGN || kind == TOKEN_DEFINE
}

func isStmtTerminator(kind TokenKind) bool {
	return kind == TOKEN_SEMICOLON || kind == TOKEN_RBRACE || kind == TOKEN_EOF
}

func (p *Parser) parseFieldList(allowVariadic bool) []*Node {
	p.expect(TOKEN_LPAREN)
	params := make([]*Node, 0, 8)
	for !p.at(TOKEN_RPAREN) && !p.at(TOKEN_EOF) {
		params = append(params, p.parseFieldDecl(allowVariadic)...)
		if p.at(TOKEN_COMMA) {
			p.advance()
		}
	}
	p.expect(TOKEN_RPAREN)
	return params
}

func (p *Parser) parseFieldDecl(allowVariadic bool) []*Node {
	pos := p.peek().Line
	if !p.at(TOKEN_IDENT) || p.nextKind() == TOKEN_DOT {
		return []*Node{p.parseUnnamedField(pos, allowVariadic)}
	}
	if p.nextKind() != TOKEN_COMMA && p.nextKind() != TOKEN_RPAREN {
		return []*Node{p.parseNamedField(allowVariadic)}
	}
	names := []Token{p.advance()}
	for p.at(TOKEN_COMMA) && p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].Kind == TOKEN_IDENT {
		if p.pos+2 < len(p.tokens) && p.tokens[p.pos+2].Kind == TOKEN_DOT {
			break
		}
		p.advance()
		names = append(names, p.advance())
	}
	if (allowVariadic && p.at(TOKEN_ELLIPSIS)) || isTypeStart(p.peek().Kind) {
		typ, variadic := p.parseFieldType(allowVariadic)
		out := make([]*Node, 0, len(names))
		for i, name := range names {
			fieldName := name.Val
			if variadic && i == len(names)-1 {
				fieldName = "..." + fieldName
			}
			out = append(out, &Node{Kind: NField, Pos: name.Line, Name: fieldName, Type: typ})
		}
		if variadic && len(names) > 1 {
			p.errorf("variadic parameter must have exactly one name at line %d", pos)
		}
		return out
	}
	out := make([]*Node, 0, len(names))
	for _, name := range names {
		out = append(out, &Node{Kind: NField, Pos: name.Line, Type: p.bareIdentType(name)})
	}
	return out
}

func (p *Parser) parseNamedField(allowVariadic bool) *Node {
	name := p.expect(TOKEN_IDENT)
	typ, variadic := p.parseFieldType(allowVariadic)
	fieldName := name.Val
	if variadic {
		fieldName = "..." + fieldName
	}
	return &Node{Kind: NField, Pos: name.Line, Name: fieldName, Type: typ}
}

func (p *Parser) parseUnnamedField(pos int, allowVariadic bool) *Node {
	typ, variadic := p.parseFieldType(allowVariadic)
	name := ""
	if variadic {
		name = "..."
	}
	return &Node{Kind: NField, Pos: pos, Name: name, Type: typ}
}

func (p *Parser) parseFieldType(allowVariadic bool) (*Node, bool) {
	if allowVariadic && p.at(TOKEN_ELLIPSIS) {
		p.advance()
		return p.parseType(), true
	}
	return p.parseType(), false
}

func (p *Parser) bareIdentType(tok Token) *Node {
	if tok.Val == "any" {
		return &Node{Kind: NIdent, Name: "interface{}", Pos: tok.Line}
	}
	if tok.Val == "complex64" || tok.Val == "complex128" {
		p.errorf("%s type is not supported at line %d", tok.Val, tok.Line)
		return &Node{Kind: NIdent, Name: "error", Pos: tok.Line}
	}
	return &Node{Kind: NIdent, Name: tok.Val, Pos: tok.Line}
}

func (p *Parser) nextKind() TokenKind {
	if p.pos+1 >= len(p.tokens) {
		return TOKEN_EOF
	}
	return p.tokens[p.pos+1].Kind
}

func (p *Parser) parseResultType() *Node {
	if !canStartResult(p.peek().Kind) {
		return nil
	}
	if p.at(TOKEN_LPAREN) {
		pos := p.peek().Line
		return &Node{Kind: NFuncType, Pos: pos, Nodes: p.parseFieldList(false)}
	}
	return p.parseType()
}

func (p *Parser) parseTypeDecl() *Node {
	pos := p.peek().Line
	p.expect(TOKEN_TYPE)

	// Handle grouped type declarations: type ( ... )
	if p.at(TOKEN_LPAREN) {
		p.advance()
		var decls []*Node
		for !p.at(TOKEN_RPAREN) && !p.at(TOKEN_EOF) {
			name := p.expect(TOKEN_IDENT)
			node := &Node{Kind: NTypeDecl, Name: name.Val, Pos: name.Line}
			if p.at(TOKEN_ASSIGN) {
				p.advance()
			}
			node.Type = p.parseType()
			decls = append(decls, node)
			p.skipSemicolon()
		}
		p.expect(TOKEN_RPAREN)
		return newDeclGroup("type", pos, decls)
	}

	name := p.expect(TOKEN_IDENT)
	node := &Node{Kind: NTypeDecl, Name: name.Val, Pos: pos}
	if p.at(TOKEN_ASSIGN) {
		p.advance()
	}
	node.Type = p.parseType()
	return node
}

func (p *Parser) parseVarDecl() *Node {
	pos := p.peek().Line
	p.expect(TOKEN_VAR)
	if p.at(TOKEN_LPAREN) {
		p.advance()
		var decls []*Node
		for !p.at(TOKEN_RPAREN) && !p.at(TOKEN_EOF) {
			spec := p.parseVarDeclSpec()
			if spec != nil {
				decls = append(decls, spec...)
			}
			p.skipSemicolon()
		}
		p.expect(TOKEN_RPAREN)
		return newDeclGroup("var", pos, decls)
	}

	decls := p.parseVarDeclSpec()
	if len(decls) == 1 {
		return decls[0]
	}
	return &Node{Kind: NVarDecl, Nodes: decls, Pos: pos}
}

func (p *Parser) parseVarDeclSpec() []*Node {
	specPos := p.peek().Line
	names := make([]string, 0, 2)
	first := p.expect(TOKEN_IDENT)
	names = append(names, first.Val)
	for p.at(TOKEN_COMMA) {
		p.advance()
		name := p.expect(TOKEN_IDENT)
		names = append(names, name.Val)
	}

	var typ *Node
	if !p.at(TOKEN_ASSIGN) && !p.at(TOKEN_SEMICOLON) && !p.at(TOKEN_RPAREN) && !p.at(TOKEN_EOF) {
		typ = p.parseType()
	}

	rhs := make([]*Node, 0, 1)
	if p.at(TOKEN_ASSIGN) {
		p.advance()
		rhs = append(rhs, p.parseExpr())
		for p.at(TOKEN_COMMA) {
			p.advance()
			rhs = append(rhs, p.parseExpr())
		}
		if len(rhs) > 1 && len(rhs) != len(names) {
			p.errorf("invalid var declaration at line %d: %d values for %d variables", specPos, len(rhs), len(names))
		}
	}

	decls := make([]*Node, 0, len(names))
	for i, name := range names {
		node := &Node{Kind: NVarDecl, Name: name, Pos: specPos, Type: typ}
		if len(rhs) == len(names) {
			node.X = rhs[i]
		} else if len(rhs) == 1 {
			node.X = rhs[0]
		}
		decls = append(decls, node)
	}
	return decls
}

func (p *Parser) parseConstDecl() *Node {
	pos := p.peek().Line
	p.expect(TOKEN_CONST)
	if p.at(TOKEN_LPAREN) {
		p.advance()
		decls := make([]*Node, 0, 4)
		for !p.at(TOKEN_RPAREN) && !p.at(TOKEN_EOF) {
			name := p.expect(TOKEN_IDENT)
			spec := &Node{Kind: NConstDecl, Name: name.Val, Pos: name.Line}
			if p.at(TOKEN_IDENT) && !p.at(TOKEN_SEMICOLON) {
				spec.Type = p.parseType()
			}
			if p.at(TOKEN_ASSIGN) {
				p.advance()
				spec.X = p.parseExpr()
			}
			decls = append(decls, spec)
			p.skipSemicolon()
		}
		p.expect(TOKEN_RPAREN)
		return newDeclGroup("const", pos, decls)
	}
	name := p.expect(TOKEN_IDENT)
	node := &Node{Kind: NConstDecl, Name: name.Val, Pos: pos}
	if p.at(TOKEN_IDENT) {
		node.Type = p.parseType()
	}
	if p.at(TOKEN_ASSIGN) {
		p.advance()
		node.X = p.parseExpr()
	}
	return node
}

// Type parsing

func (p *Parser) parseType() *Node {
	switch p.peek().Kind {
	case TOKEN_IDENT:
		tok := p.advance()
		if tok.Val == "any" {
			return &Node{Kind: NIdent, Name: "interface{}", Pos: tok.Line}
		}
		if tok.Val == "complex64" || tok.Val == "complex128" {
			p.errorf("%s type is not supported at line %d", tok.Val, tok.Line)
			return &Node{Kind: NIdent, Name: "error", Pos: tok.Line}
		}
		node := &Node{Kind: NIdent, Name: tok.Val, Pos: tok.Line}
		if p.at(TOKEN_DOT) {
			p.advance()
			name := p.expect(TOKEN_IDENT)
			node = &Node{Kind: NSelectorExpr, X: node, Name: name.Val, Pos: tok.Line}
		}
		return node
	case TOKEN_STAR:
		pos := p.peek().Line
		p.advance()
		inner := p.parseType()
		return &Node{Kind: NPointerType, X: inner, Pos: pos}
	case TOKEN_LBRACK:
		return p.parseSliceOrArrayType()
	case TOKEN_MAP:
		return p.parseMapType()
	case TOKEN_FUNC:
		return p.parseFuncType()
	case TOKEN_STRUCT:
		return p.parseStructType()
	case TOKEN_INTERFACE:
		return p.parseInterfaceType()
	case TOKEN_CHAN:
		tok := p.advance()
		p.errorf("chan types are not supported at line %d", tok.Line)
		if !p.at(TOKEN_LBRACE) && !p.at(TOKEN_SEMICOLON) && !p.at(TOKEN_RPAREN) && !p.at(TOKEN_COMMA) && !p.at(TOKEN_EOF) {
			p.parseType() // consume element type for recovery
		}
		return &Node{Kind: NIdent, Name: "error", Pos: tok.Line}
	}
	tok := p.advance()
	p.errorf("expected type, got %s at line %d", tok.String(), tok.Line)
	return &Node{Kind: NIdent, Name: "error", Pos: tok.Line}
}

func (p *Parser) parseSliceOrArrayType() *Node {
	pos := p.peek().Line
	p.expect(TOKEN_LBRACK)
	if p.at(TOKEN_RBRACK) {
		p.advance()
		elem := p.parseType()
		return &Node{Kind: NSliceType, X: elem, Pos: pos}
	}
	var arrayLen *Node
	name := ""
	if p.at(TOKEN_ELLIPSIS) {
		p.advance()
		name = "..."
	} else {
		arrayLen = p.parseExpr()
	}
	p.expect(TOKEN_RBRACK)
	elem := p.parseType()
	return &Node{Kind: NArrayType, X: elem, Y: arrayLen, Name: name, Pos: pos}
}

func (p *Parser) parseMapType() *Node {
	pos := p.peek().Line
	p.expect(TOKEN_MAP)
	p.expect(TOKEN_LBRACK)
	key := p.parseType()
	p.expect(TOKEN_RBRACK)
	val := p.parseType()
	return &Node{Kind: NMapType, X: key, Y: val, Pos: pos}
}

func (p *Parser) parseFuncType() *Node {
	pos := p.peek().Line
	p.expect(TOKEN_FUNC)
	node := &Node{Kind: NFuncType, Pos: pos}
	node.Nodes = p.parseFieldList(true)
	// optional return type
	if canStartResult(p.peek().Kind) {
		node.Type = p.parseResultType()
	}
	return node
}

func (p *Parser) parseStructType() *Node {
	pos := p.peek().Line
	p.expect(TOKEN_STRUCT)
	p.expect(TOKEN_LBRACE)
	node := &Node{Kind: NStructType, Pos: pos}
	for !p.at(TOKEN_RBRACE) && !p.at(TOKEN_EOF) {
		field := p.parseStructField()
		node.Nodes = append(node.Nodes, field)
		p.skipSemicolon()
	}
	p.expect(TOKEN_RBRACE)
	return node
}

func (p *Parser) parseStructField() *Node {
	node := &Node{Kind: NField, Pos: p.peek().Line}
	name := p.expect(TOKEN_IDENT)
	node.Name = name.Val
	if !p.at(TOKEN_SEMICOLON) && !p.at(TOKEN_RBRACE) && !p.at(TOKEN_EOF) {
		node.Type = p.parseType()
	} else {
		// Embedded field: `struct { Base }`
		node.Type = &Node{Kind: NIdent, Name: node.Name, Pos: name.Line}
	}
	return node
}

func (p *Parser) parseInterfaceType() *Node {
	pos := p.peek().Line
	p.expect(TOKEN_INTERFACE)
	p.expect(TOKEN_LBRACE)
	node := &Node{Kind: NInterfaceType, Pos: pos}
	for !p.at(TOKEN_RBRACE) && !p.at(TOKEN_EOF) {
		// Parse method signature: MethodName(params) returnType
		meth := &Node{Kind: NFunc, Pos: p.peek().Line}
		name := p.expect(TOKEN_IDENT)
		meth.Name = name.Val
		meth.Nodes = p.parseFieldList(true)
		// Parse return type(s)
		if canStartResult(p.peek().Kind) {
			meth.Type = p.parseResultType()
		}
		node.Nodes = append(node.Nodes, meth)
		p.skipSemicolon()
	}
	p.expect(TOKEN_RBRACE)
	return node
}

// Statement parsing

func (p *Parser) parseBlock() *Node {
	pos := p.peek().Line
	p.expect(TOKEN_LBRACE)
	block := &Node{Kind: NBlock, Pos: pos, Nodes: make([]*Node, 0, 8)}
	for !p.at(TOKEN_RBRACE) && !p.at(TOKEN_EOF) {
		stmt := p.parseStmt()
		if stmt != nil {
			block.Nodes = append(block.Nodes, stmt)
		}
	}
	p.expect(TOKEN_RBRACE)
	return block
}

func (p *Parser) parseStmt() *Node {
	if p.at(TOKEN_SEMICOLON) {
		p.advance()
		return nil
	}

	var node *Node
	switch p.peek().Kind {
	case TOKEN_LBRACE:
		node = p.parseBlock()
	case TOKEN_IF:
		node = p.parseIfStmt()
	case TOKEN_FOR:
		node = p.parseForStmt()
	case TOKEN_SWITCH:
		node = p.parseSwitchStmt()
	case TOKEN_RETURN:
		node = p.parseReturnStmt()
	case TOKEN_VAR:
		node = p.parseVarDecl()
	case TOKEN_CONST:
		node = p.parseConstDecl()
	case TOKEN_TYPE:
		node = p.parseTypeDecl()
	case TOKEN_GOTO:
		pos := p.peek().Line
		p.advance()
		name := p.expect(TOKEN_IDENT)
		node = &Node{Kind: NBranch, Name: "goto", X: &Node{Kind: NIdent, Name: name.Val, Pos: name.Line}, Pos: pos}
	case TOKEN_BREAK:
		pos := p.peek().Line
		p.advance()
		var target *Node
		if p.at(TOKEN_IDENT) {
			name := p.advance()
			target = &Node{Kind: NIdent, Name: name.Val, Pos: name.Line}
		}
		node = &Node{Kind: NBranch, Name: "break", X: target, Pos: pos}
	case TOKEN_CONTINUE:
		pos := p.peek().Line
		p.advance()
		var target *Node
		if p.at(TOKEN_IDENT) {
			name := p.advance()
			target = &Node{Kind: NIdent, Name: name.Val, Pos: name.Line}
		}
		node = &Node{Kind: NBranch, Name: "continue", X: target, Pos: pos}
	case TOKEN_FALLTHROUGH:
		pos := p.peek().Line
		p.advance()
		node = &Node{Kind: NBranch, Name: "fallthrough", Pos: pos}
	case TOKEN_DEFER:
		node = p.parseDeferStmt()
	case TOKEN_GO:
		p.errorf("go statements (goroutines) are not supported at line %d", p.peek().Line)
		p.advance()
		depth := 0
		for !p.at(TOKEN_EOF) {
			if p.at(TOKEN_LBRACE) {
				depth++
			}
			if p.at(TOKEN_RBRACE) {
				if depth == 0 {
					break
				}
				depth = depth - 1
			}
			if p.at(TOKEN_SEMICOLON) && depth == 0 {
				break
			}
			p.advance()
		}
	case TOKEN_SELECT:
		p.errorf("select statements are not supported at line %d", p.peek().Line)
		p.advance()
		if p.at(TOKEN_LBRACE) {
			p.skipBlock()
		}
	default:
		// Label declaration: name:
		if p.at(TOKEN_IDENT) && p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].Kind == TOKEN_COLON {
			name := p.advance()
			p.expect(TOKEN_COLON)
			node = &Node{Kind: NBranch, Name: "label", X: &Node{Kind: NIdent, Name: name.Val, Pos: name.Line}, Pos: name.Line}
			break
		}
		node = p.parseSimpleStmtNoSemicolon()
	}
	p.skipSemicolon()
	return node
}

func (p *Parser) parseIfStmt() *Node {
	pos := p.peek().Line
	p.expect(TOKEN_IF)
	node := &Node{Kind: NIf, Pos: pos}

	// Parse condition or init statement (might be multi-value like a, b := expr)
	old := p.noCompLit
	p.noCompLit = true
	initOrCond := p.parseSimpleStmtNoSemicolon()
	p.noCompLit = old

	// Check for semicolon indicating init statement
	if p.at(TOKEN_SEMICOLON) {
		p.advance()
		// initOrCond was the init statement
		node.Nodes = append(node.Nodes, initOrCond)
		node.X = p.parseExprNoBrace()
	} else {
		// initOrCond was the condition; extract expression
		if initOrCond.Kind == NExprStmt {
			node.X = initOrCond.X
		} else {
			node.X = initOrCond
		}
	}

	node.Body = p.parseBlock()

	if p.at(TOKEN_ELSE) {
		p.advance()
		if p.at(TOKEN_IF) {
			node.Y = p.parseIfStmt()
		} else {
			node.Y = p.parseBlock()
		}
	}
	return node
}

func (p *Parser) parseForStmt() *Node {
	pos := p.peek().Line
	p.expect(TOKEN_FOR)
	node := &Node{Kind: NFor, Pos: pos}

	// Check for bare "for {"
	if p.at(TOKEN_LBRACE) {
		node.Body = p.parseBlock()
		return node
	}

	// Check for bare "for range x {"
	if p.at(TOKEN_RANGE) {
		p.advance()
		iterable := p.parseExprNoBrace()
		node.Name = "range"
		node.Type = iterable
		node.Body = p.parseBlock()
		return node
	}

	// Empty-init 3-clause form: for ; cond; post { ... }
	if p.at(TOKEN_SEMICOLON) {
		p.advance()
		if !p.at(TOKEN_SEMICOLON) {
			node.Y = p.parseExprNoBrace()
		}
		p.expect(TOKEN_SEMICOLON)
		if !p.at(TOKEN_LBRACE) {
			node.Type = p.parseSimpleStmtNoSemicolon()
		}
		node.Body = p.parseBlock()
		return node
	}

	// Try to detect range-based for loop
	// Patterns: for _, x := range y { ... } or for i := range y { ... }
	first := p.parseExprNoBrace()

	if p.at(TOKEN_COMMA) {
		// Multi-value: for a, b := range ...
		p.advance()
		second := p.parseExprNoBrace()
		if p.at(TOKEN_DEFINE) || p.at(TOKEN_ASSIGN) {
			op := p.advance()
			_ = op
			p.expect(TOKEN_RANGE)
			iterable := p.parseExprNoBrace()
			node.Name = "range"
			node.X = first
			node.Y = second
			node.Type = iterable
			node.Body = p.parseBlock()
			return node
		}
	} else if p.at(TOKEN_DEFINE) || p.at(TOKEN_ASSIGN) {
		// Could be: for i := range ... or for i := 0; ...
		savedPos := p.pos
		op := p.advance()
		if p.at(TOKEN_RANGE) {
			p.advance()
			iterable := p.parseExprNoBrace()
			node.Name = "range"
			node.X = first
			node.Type = iterable
			node.Body = p.parseBlock()
			return node
		}
		// It's a 3-clause for: restore and parse as simple stmt
		p.pos = savedPos
		p.advance() // consume the := or =
		rhs := p.parseExprNoBrace()
		init := &Node{Kind: NAssign, Name: tokenVal(op), X: first, Y: rhs, Pos: first.Pos}
		node.X = init
		p.expect(TOKEN_SEMICOLON)
		node.Y = p.parseExprNoBrace()
		p.expect(TOKEN_SEMICOLON)
		if !p.at(TOKEN_LBRACE) {
			node.Type = p.parseSimpleStmtNoSemicolon()
		}
		node.Body = p.parseBlock()
		return node
	} else if p.at(TOKEN_SEMICOLON) {
		// 3-clause for with expression init
		init := &Node{Kind: NExprStmt, X: first, Pos: first.Pos}
		node.X = init
		p.advance()
		if !p.at(TOKEN_SEMICOLON) {
			node.Y = p.parseExprNoBrace()
		}
		p.expect(TOKEN_SEMICOLON)
		if !p.at(TOKEN_LBRACE) {
			node.Type = p.parseSimpleStmtNoSemicolon()
		}
		node.Body = p.parseBlock()
		return node
	}

	// Simple condition for loop: for cond { ... }
	node.Y = first
	node.Body = p.parseBlock()
	return node
}

func (p *Parser) parseSwitchStmt() *Node {
	pos := p.peek().Line
	p.expect(TOKEN_SWITCH)
	node := &Node{Kind: NSwitch, Pos: pos}

	// Optional init statement and/or tag expression
	if !p.at(TOKEN_LBRACE) {
		old := p.noCompLit
		p.noCompLit = true
		first := p.parseSimpleStmtNoSemicolon()
		p.noCompLit = old

		if p.at(TOKEN_SEMICOLON) {
			p.advance()
			node.X = first
			if !p.at(TOKEN_LBRACE) {
				tag := p.parseExprNoBrace()
				if tag != nil && tag.Kind == NTypeAssertExpr && tag.Name == "type" {
					node.Name = "typeswitch"
					node.Y = tag.X
				} else {
					node.Y = tag
				}
			}
		} else {
			if first != nil && first.Kind == NAssign && first.Y != nil && first.Y.Kind == NTypeAssertExpr && first.Y.Name == "type" {
				node.Name = "typeswitch"
				node.Y = first.Y.X
				if first.X != nil && first.X.Kind == NIdent {
					node.Type = &Node{Kind: NIdent, Name: first.X.Name, Pos: first.X.Pos}
				}
				node.X = nil
				goto switchBody
			}
			if first != nil && first.Kind == NExprStmt {
				tag := first.X
				if tag != nil && tag.Kind == NTypeAssertExpr && tag.Name == "type" {
					node.Name = "typeswitch"
					node.Y = tag.X
				} else {
					node.Y = tag
				}
			} else {
				node.Y = first
			}
		}
	}

switchBody:
	p.expect(TOKEN_LBRACE)
	for !p.at(TOKEN_RBRACE) && !p.at(TOKEN_EOF) {
		c := p.parseCaseClause()
		node.Nodes = append(node.Nodes, c)
	}
	p.expect(TOKEN_RBRACE)
	return node
}

func (p *Parser) parseCaseClause() *Node {
	pos := p.peek().Line
	node := &Node{Kind: NCase, Pos: pos}
	if p.at(TOKEN_CASE) {
		p.advance()
		// Parse case expressions (comma-separated)
		node.X = p.parseExpr()
		for p.at(TOKEN_COMMA) {
			p.advance()
			extra := p.parseExpr()
			node.Nodes = append(node.Nodes, extra)
		}
	} else {
		p.expect(TOKEN_DEFAULT)
		node.Name = "default"
	}
	p.expect(TOKEN_COLON)

	// Parse statements until next case/default/}
	var stmts []*Node
	for !p.at(TOKEN_CASE) && !p.at(TOKEN_DEFAULT) && !p.at(TOKEN_RBRACE) && !p.at(TOKEN_EOF) {
		stmt := p.parseStmt()
		if stmt != nil {
			stmts = append(stmts, stmt)
		}
	}
	if len(stmts) > 0 {
		node.Body = &Node{Kind: NBlock, Nodes: stmts, Pos: pos}
	}
	return node
}

func (p *Parser) parseReturnStmt() *Node {
	pos := p.peek().Line
	p.expect(TOKEN_RETURN)
	node := &Node{Kind: NReturn, Pos: pos}
	if !isStmtTerminator(p.peek().Kind) {
		node.X = p.parseExpr()
		for p.at(TOKEN_COMMA) {
			p.advance()
			node.Nodes = append(node.Nodes, p.parseExpr())
		}
	}
	return node
}

func (p *Parser) parseDeferStmt() *Node {
	pos := p.peek().Line
	p.expect(TOKEN_DEFER)
	expr := p.parseExpr()
	return &Node{Kind: NDeferStmt, X: expr, Pos: pos}
}

func (p *Parser) parseSimpleStmtNoSemicolon() *Node {
	expr := p.parseExpr()

	// Check for increment
	if p.at(TOKEN_INC) {
		p.advance()
		return &Node{Kind: NIncStmt, X: expr, Pos: expr.Pos}
	}
	if p.at(TOKEN_DEC) {
		p.advance()
		return &Node{Kind: NDecStmt, X: expr, Pos: expr.Pos}
	}

	// Check for assignment / short var decl
	if isAssignOp(p.peek().Kind) {
		op := p.advance()
		rhs := p.parseExpr()
		return &Node{Kind: NAssign, Name: tokenVal(op), X: expr, Y: rhs, Pos: expr.Pos}
	}

	// Check for multi-value assignment: a, b = ... or a, b := ...
	if p.at(TOKEN_COMMA) {
		var lhs []*Node
		lhs = append(lhs, expr)
		for p.at(TOKEN_COMMA) {
			p.advance()
			lhs = append(lhs, p.parseExpr())
		}
		if isSimpleAssignOp(p.peek().Kind) {
			op := p.advance()
			rhs := p.parseExpr()
			node := &Node{Kind: NAssign, Name: tokenVal(op), Y: rhs, Pos: expr.Pos}
			node.Nodes = lhs
			// Check for comma-separated RHS: a, b := 1, 2
			if p.at(TOKEN_COMMA) {
				var rhsList []*Node
				rhsList = append(rhsList, rhs)
				for p.at(TOKEN_COMMA) {
					p.advance()
					rhsList = append(rhsList, p.parseExpr())
				}
				node.Y = nil
				node.Body = &Node{Kind: NBlock, Nodes: rhsList, Pos: expr.Pos}
			}
			return node
		}
	}

	return &Node{Kind: NExprStmt, X: expr, Pos: expr.Pos}
}

// Expression parsing

func (p *Parser) parseExpr() *Node {
	return p.parseBinaryExpr(1)
}

func (p *Parser) parseExprNoBrace() *Node {
	old := p.noCompLit
	p.noCompLit = true
	expr := p.parseExpr()
	p.noCompLit = old
	return expr
}

func precedence(kind TokenKind) int {
	switch kind {
	case TOKEN_OR:
		return 1
	case TOKEN_AND:
		return 2
	case TOKEN_EQ, TOKEN_NEQ, TOKEN_LT, TOKEN_GT, TOKEN_LEQ, TOKEN_GEQ:
		return 3
	case TOKEN_PLUS, TOKEN_MINUS, TOKEN_PIPE, TOKEN_CARET:
		return 4
	case TOKEN_STAR, TOKEN_SLASH, TOKEN_PERCENT, TOKEN_AMPERSAND, TOKEN_SHL, TOKEN_SHR:
		return 5
	}
	return 0
}

func (p *Parser) parseBinaryExpr(minPrec int) *Node {
	left := p.parseUnaryExpr()
	for {
		prec := precedence(p.peek().Kind)
		if prec < minPrec {
			break
		}
		op := p.advance()
		right := p.parseBinaryExpr(prec + 1)
		left = &Node{Kind: NBinaryExpr, Name: tokenVal(op), X: left, Y: right, Pos: left.Pos}
	}
	return left
}

func (p *Parser) parseUnaryExpr() *Node {
	kind := p.peek().Kind
	if kind == TOKEN_NOT || kind == TOKEN_MINUS || kind == TOKEN_CARET || kind == TOKEN_STAR || kind == TOKEN_AMPERSAND {
		op := p.advance()
		expr := p.parseUnaryExpr()
		name := tokenVal(op)
		if kind == TOKEN_STAR {
			name = "*"
		} else if kind == TOKEN_AMPERSAND {
			name = "&"
		}
		return &Node{Kind: NUnaryExpr, Name: name, X: expr, Pos: op.Line}
	}
	return p.parsePrimaryExpr()
}

func (p *Parser) parsePrimaryExpr() *Node {
	var node *Node
	switch p.peek().Kind {
	case TOKEN_IDENT:
		tok := p.advance()
		node = &Node{Kind: NIdent, Name: tok.Val, Pos: tok.Line}
	case TOKEN_INT:
		tok := p.advance()
		node = &Node{Kind: NIntLit, Name: tok.Val, Pos: tok.Line}
	case TOKEN_FLOAT:
		tok := p.advance()
		node = &Node{Kind: NFloatLit, Name: tok.Val, Pos: tok.Line}
	case TOKEN_IMAG:
		tok := p.advance()
		p.errorf("imaginary literals are not supported at line %d col %d", tok.Line, tok.Col)
		return &Node{Kind: NIdent, Name: "error", Pos: tok.Line}
	case TOKEN_STRING:
		tok := p.advance()
		node = &Node{Kind: NStringLit, Name: tok.Val, Pos: tok.Line}
	case TOKEN_RAW_STRING:
		tok := p.advance()
		// Raw strings carry literal bytes; normalize to escaped form so
		// backend string-literal decoding preserves those bytes verbatim.
		node = &Node{Kind: NStringLit, Name: encodeStringLiteral(tok.Val), Pos: tok.Line}
	case TOKEN_RUNE:
		tok := p.advance()
		node = &Node{Kind: NRuneLit, Name: tok.Val, Pos: tok.Line}
	case TOKEN_TRUE, TOKEN_FALSE, TOKEN_NIL, TOKEN_IOTA:
		tok := p.advance()
		node = &Node{Kind: NBasicLit, Name: tok.Val, Pos: tok.Line}
	case TOKEN_LPAREN:
		p.advance()
		node = p.parseExpr()
		p.expect(TOKEN_RPAREN)
	case TOKEN_LBRACK:
		// Slice type used as expression (composite literal)
		node = p.parseSliceOrArrayType()
	case TOKEN_MAP:
		node = p.parseMapType()
	case TOKEN_FUNC:
		// Function literal or function type
		node = p.parseFuncType()
		if p.at(TOKEN_LBRACE) {
			// Function literal
			body := p.parseBlock()
			node.Body = body
		}
	case TOKEN_CHAN:
		tok := p.advance()
		p.errorf("chan types are not supported at line %d", tok.Line)
		if p.at(TOKEN_IDENT) {
			p.advance()
		}
		return &Node{Kind: NIdent, Name: "error", Pos: tok.Line}
	case TOKEN_GO:
		tok := p.advance()
		p.errorf("go statements (goroutines) are not supported at line %d", tok.Line)
		return &Node{Kind: NIdent, Name: "error", Pos: tok.Line}
	case TOKEN_SELECT:
		tok := p.advance()
		p.errorf("select statements are not supported at line %d", tok.Line)
		return &Node{Kind: NIdent, Name: "error", Pos: tok.Line}
	default:
		tok := p.advance()
		p.errorf("unexpected token in expression: %s at line %d col %d", tok.String(), tok.Line, tok.Col)
		return &Node{Kind: NIdent, Name: "error", Pos: tok.Line}
	}
	return p.parsePostfixOps(node)
}

func (p *Parser) isTypeLikeNode(node *Node) bool {
	if node.Kind == NIdent || node.Kind == NSliceType || node.Kind == NArrayType || node.Kind == NMapType || node.Kind == NPointerType {
		return true
	}
	if node.Kind == NSelectorExpr {
		return true
	}
	return false
}

func (p *Parser) parsePostfixOps(node *Node) *Node {
	for {
		switch p.peek().Kind {
		case TOKEN_DOT:
			node = p.parsePostfixDot(node)
			continue
		case TOKEN_LPAREN:
			node = p.parsePostfixCall(node)
			continue
		case TOKEN_LBRACK:
			node = p.parsePostfixIndexOrSlice(node)
			continue
		case TOKEN_LBRACE:
			if p.canParseCompositeLit(node) {
				node = p.parseCompositeLit(node)
				continue
			}
		}
		return node
	}
}

func (p *Parser) parsePostfixDot(node *Node) *Node {
	p.advance()
	if p.at(TOKEN_LPAREN) {
		// Type assertion: x.(T) or x.(type) (for type switches).
		p.advance()
		assertNode := &Node{Kind: NTypeAssertExpr, X: node, Pos: node.Pos}
		if p.at(TOKEN_TYPE) {
			p.advance()
			assertNode.Name = "type"
		} else {
			assertNode.Type = p.parseType()
		}
		p.expect(TOKEN_RPAREN)
		return assertNode
	}
	name := p.expect(TOKEN_IDENT)
	return &Node{Kind: NSelectorExpr, X: node, Name: name.Val, Pos: node.Pos}
}

func (p *Parser) parsePostfixCall(node *Node) *Node {
	p.advance()
	call := &Node{Kind: NCallExpr, X: node, Pos: node.Pos, Nodes: make([]*Node, 0, 2)}
	for !p.at(TOKEN_RPAREN) && !p.at(TOKEN_EOF) {
		arg := p.parseExpr()
		if p.at(TOKEN_ELLIPSIS) {
			p.advance()
			call.Name = "spread"
		}
		call.Nodes = append(call.Nodes, arg)
		if p.at(TOKEN_COMMA) {
			p.advance()
		}
	}
	p.expect(TOKEN_RPAREN)
	return call
}

func (p *Parser) parsePostfixIndexOrSlice(node *Node) *Node {
	p.advance()
	if p.at(TOKEN_COLON) {
		// s[:hi] — empty low bound, defaults to 0
		p.advance()
		var hi *Node
		if !p.at(TOKEN_RBRACK) {
			hi = p.parseExpr()
		}
		var max *Node
		if p.at(TOKEN_COLON) {
			p.advance()
			if !p.at(TOKEN_RBRACK) {
				max = p.parseExpr()
			}
		}
		p.expect(TOKEN_RBRACK)
		lo := &Node{Kind: NIntLit, Name: "0", Pos: node.Pos}
		return &Node{Kind: NSliceExpr, X: node, Y: lo, Body: hi, Type: max, Pos: node.Pos}
	}

	index := p.parseExpr()
	if p.at(TOKEN_COLON) {
		p.advance()
		var hi *Node
		if !p.at(TOKEN_RBRACK) {
			hi = p.parseExpr()
		}
		var max *Node
		if p.at(TOKEN_COLON) {
			p.advance()
			if !p.at(TOKEN_RBRACK) {
				max = p.parseExpr()
			}
		}
		p.expect(TOKEN_RBRACK)
		return &Node{Kind: NSliceExpr, X: node, Y: index, Body: hi, Type: max, Pos: node.Pos}
	}

	p.expect(TOKEN_RBRACK)
	return &Node{Kind: NIndexExpr, X: node, Y: index, Pos: node.Pos}
}

func (p *Parser) canParseCompositeLit(node *Node) bool {
	allowCompLit := !p.noCompLit
	if p.noCompLit {
		// In noCompLit mode (e.g. if/switch headers), only allow
		// composite literals with unambiguous non-identifier type forms.
		// Do NOT allow selector expressions here, because values like
		// `tok.Kind` in `switch tok.Kind { ... }` are ambiguous.
		if node.Kind == NSliceType || node.Kind == NArrayType || node.Kind == NMapType {
			allowCompLit = true
		}
	}
	return allowCompLit && p.isTypeLikeNode(node)
}

func (p *Parser) parseCompositeLit(typeNode *Node) *Node {
	pos := typeNode.Pos
	p.expect(TOKEN_LBRACE)
	node := &Node{Kind: NCompositeLit, Type: typeNode, Pos: pos, Nodes: make([]*Node, 0, 4)}
	// Infer element type for nested composite literals
	var elemType *Node
	if typeNode.Kind == NSliceType || typeNode.Kind == NArrayType {
		elemType = typeNode.X
	} else if typeNode.Kind == NMapType {
		elemType = typeNode.Y
	}
	for !p.at(TOKEN_RBRACE) && !p.at(TOKEN_EOF) {
		if p.at(TOKEN_LBRACE) && elemType != nil {
			// Nested composite literal with inferred type: {X: 1, Y: 2}
			val := p.parseCompositeLit(elemType)
			node.Nodes = append(node.Nodes, val)
		} else {
			val := p.parseExpr()
			if p.at(TOKEN_COLON) {
				p.advance()
				var v *Node
				if p.at(TOKEN_LBRACE) && elemType != nil {
					v = p.parseCompositeLit(elemType)
				} else {
					v = p.parseExpr()
				}
				kv := &Node{Kind: NKeyValue, X: val, Y: v, Pos: val.Pos}
				node.Nodes = append(node.Nodes, kv)
			} else {
				node.Nodes = append(node.Nodes, val)
			}
		}
		if p.at(TOKEN_COMMA) {
			p.advance()
		}
	}
	p.expect(TOKEN_RBRACE)
	return node
}

// tokenVal returns the string representation of a token.
// For tokens with a Val (identifiers, literals), returns Val.
// For operators and keywords, returns the canonical string from tokenNames.
func tokenVal(tok Token) string {
	if tok.Val != "" {
		return tok.Val
	}
	return tokenName(tok.Kind)
}

const (
	comptimePkgPath           = "j5.nz/rtg/x/comptime"
	comptimePkgPrefix         = "j5.nz/rtg/x/comptime."
	targetPkgPath             = "j5.nz/rtg/std/target"
	targetRegisterFn          = targetPkgPath + ".Register"
	targetRegisterABI         = targetPkgPath + ".RegisterABI"
	targetRegisterABIExternal = targetPkgPath + ".RegisterExternalABI"
	targetRegisterAsm         = targetPkgPath + ".RegisterAssembler"
	targetRegisterFmt         = targetPkgPath + ".RegisterBinFormat"
)

// Defer lowering is enabled by default.
var featureDeferEnabled = true

type closureCaptureSpec struct {
	Name          string
	Width         int
	ConcreteType  string
	InterfaceType string
}

type closureCaptureBinding struct {
	LocalIdx int
	Width    int
	IsPtr    bool
}

type assembleInfo struct {
	Arch        string
	BuilderName string
	Params      int
	RetCount    int
}

type deferSite struct {
	callOp          ir.Opcode
	callName        string
	argCount        int
	retCount        int
	fixedCount      int
	isVariadic      bool
	variadicElemSz  int
	variadicIsIface bool
}

type structTypeLookupResult struct {
	typeNode *Node
	pkgPath  string
	ok       bool
}

const structTypeLookupMaxEntries = 8192

// === Compiler ===

// Compiler lowers AST from a Module into stack machine IR.
type Compiler struct {
	target                *common.Target
	mod                   *Module
	irmod                 *ir.IRModule
	curFunc               *ir.IRFunc
	scopes                []map[string]int
	labelSeq              int
	breaks                []int
	continues             []int
	fallthroughs          []int
	pendingStmtLabels     []string
	breakLabelTargets     map[string][]int
	continueLabelTargets  map[string][]int
	globals               map[string]int
	types                 map[string]*ir.TypeInfo
	curPkg                *Package
	errors                []string
	funcRets              map[string]int      // function name → return count
	funcParams            map[string]int      // function name → param count
	funcParamTypes        map[string][]string // function name → param type names (profile parent hash is synthetic arg 0 for profiled plain funcs; follows receiver for methods)
	funcProfileParentABI  map[string]bool     // function/method name → true when call ABI includes synthetic parent hash param in -profile mode
	funcVariadic          map[string]int      // variadic function name → count of fixed params
	funcVariadicIface     map[string]bool     // variadic function name → true if ...interface{}
	funcVariadicElem      map[string]int      // variadic function name → variadic elem size (1 for ...byte, 8 otherwise)
	funcIsInternal        map[string]bool     // function name → true if declared via //rtg:internal
	funcIsLinkStatic      map[string]bool     // function name → true if declared via //rtg:linkstatic
	funcIsProfiled        map[string]bool     // function/method name → true when profiling is enabled (methods/functions default-on unless //rtg:noprofile)
	funcIsCallback        map[string]bool     // function name → true if declared via //rtg:callback
	funcIsZeroCall        map[string]bool     // function/method name → true if calls must be inlined at callsites
	typeIsZeroCall        map[string]bool     // qualified type name → true if methods default to zerocall
	comptimeFuncs         map[string]bool     // function/method name → true if marked //rtg:comptime
	funcRetTypeNodes      map[string]*Node    // function name → first return type node (for comptime literal synthesis)
	localElemSizes        map[string]int      // variable name → slice element size (1 for byte, 8 otherwise)
	globalElemSizes       map[string]int      // qualified global name → slice element size
	ifaceMethods          map[string][]string // interface name → method names
	ifaceMethodRets       map[string]int      // iface+"\x00"+method → return count
	ifaceMethodRetTypes   map[string]string   // iface+"\x00"+method → first return type name
	ifaceMethodRetLists   map[string][]string // iface+"\x00"+method → full return type names
	methodTable           map[string]string   // "pkg.Type.Method" → qualified IR func name
	methodFuncNames       map[string]bool     // qualified method function names
	typeIDs               map[string]int      // concrete type qualified name → unique int
	nextTypeID            int
	localTypes            map[string]string   // local var name → type name (for interface-typed locals)
	localTypeDecls        map[string]*Node    // local type name → type declaration node (function scope)
	localStringVars       map[string]bool     // local var name → true if the local is a string
	localConcreteTypes    map[string]string   // local var name → qualified type name for method resolution
	funcRetTypes          map[string][]string // function name → return type names
	localMapVars          map[string]int      // local var name → keyKind (0=int, 1=string) if it's a map
	localMapValueTypes    map[string]string   // local map var name → value type name (e.g. "*Package")
	globalMapVars         map[string]int      // qualified global name → keyKind if it's a map
	globalConcreteTypes   map[string]string   // qualified global name → qualified type name
	constValues           map[string]int64    // qualified const name → precomputed value
	constStringValues     map[string]string   // qualified const name → precomputed string value
	localAddrOf           map[string]bool     // local var name → true if assigned from &var (pointer-to-pointer)
	stackDepth            int                 // operand stack depth tracking for balance checks
	deferSites            []deferSite
	deferHeadLocal        int
	panicUnwindLabel      int
	namedResultNames      []string
	labelIDs              map[string]int
	funcLitSeq            int
	localFuncTargets      map[string]string
	localMethodTargets    map[string]string
	localMethodRecv       map[string]int
	funcLiteralCaptures   map[string][]closureCaptureSpec
	localFuncCaptures     map[string][]closureCaptureBinding
	activeCaptures        map[string]closureCaptureBinding
	captureConcreteTypes  map[string]string
	captureIfaceTypes     map[string]string
	dotJoinCache          map[string]map[string]string // a → b → "a.b"
	qualifyTypeCache      map[string]string            // "typeName\x00pkgPath" → qualified result
	structTypeLookup      map[string]structTypeLookupResult
	comptimeSeq           int
	comptimeDisabled      bool
	inComptimeFunc        bool
	profileStartLocal     int
	profileParentLocal    int
	profileMethodHash     uint32
	profileFlushOnExit    bool
	currentMethodHash     uint32
	inIfInit              bool
	ifInitLeakedNames     map[string]bool
	assembleFuncs         map[string]assembleInfo
	inAssembleBuilder     bool
	entryFunc             string
	panicCheckSlowLabels  map[int]int
	panicCheckSlowDepths  []int
	deferRecoverWrapFuncs map[string]bool // function name → keep DeferRecoverBefore/AfterCall wrappers
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
	entryFunc := target.EntryFunc
	if entryFunc == "" {
		entryFunc = "main.main"
	}
	c := &Compiler{
		target:                &target,
		mod:                   mod,
		irmod:                 &ir.IRModule{},
		globals:               make(map[string]int),
		types:                 make(map[string]*ir.TypeInfo),
		funcRets:              make(map[string]int),
		funcParams:            make(map[string]int),
		funcParamTypes:        make(map[string][]string),
		funcProfileParentABI:  make(map[string]bool),
		funcVariadic:          make(map[string]int),
		funcVariadicIface:     make(map[string]bool),
		funcVariadicElem:      make(map[string]int),
		funcIsInternal:        make(map[string]bool),
		funcIsLinkStatic:      make(map[string]bool),
		funcIsProfiled:        make(map[string]bool),
		funcIsCallback:        make(map[string]bool),
		funcIsZeroCall:        make(map[string]bool),
		typeIsZeroCall:        make(map[string]bool),
		comptimeFuncs:         make(map[string]bool),
		funcRetTypeNodes:      make(map[string]*Node),
		globalElemSizes:       make(map[string]int),
		ifaceMethods:          make(map[string][]string),
		ifaceMethodRets:       make(map[string]int),
		ifaceMethodRetTypes:   make(map[string]string),
		ifaceMethodRetLists:   make(map[string][]string),
		methodTable:           make(map[string]string),
		methodFuncNames:       make(map[string]bool),
		typeIDs:               make(map[string]int),
		nextTypeID:            4, // 1=int, 2=string, 3=bool are reserved
		funcRetTypes:          make(map[string][]string),
		globalMapVars:         make(map[string]int),
		globalConcreteTypes:   make(map[string]string),
		constValues:           make(map[string]int64),
		constStringValues:     make(map[string]string),
		funcLiteralCaptures:   make(map[string][]closureCaptureSpec),
		localFuncCaptures:     make(map[string][]closureCaptureBinding),
		dotJoinCache:          make(map[string]map[string]string),
		qualifyTypeCache:      make(map[string]string),
		structTypeLookup:      make(map[string]structTypeLookupResult),
		assembleFuncs:         make(map[string]assembleInfo),
		entryFunc:             entryFunc,
		deferRecoverWrapFuncs: make(map[string]bool),
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
			global := ir.IRGlobal{Name: qname, Index: idx}
			if sym.Node != nil && sym.Node.Type != nil && sym.Node.Type.Kind == NIdent {
				if t, ok := c.types[sym.Node.Type.Name]; ok {
					global.Type = t
				}
			} else if sym.Node != nil && sym.Node.X != nil {
				switch c.exprConcreteType(sym.Node.X) {
				case "float32":
					global.Type = c.types["float32"]
				case "float64":
					global.Type = c.types["float64"]
				}
			}
			c.irmod.Globals = append(c.irmod.Globals, global)
			if sym.Node != nil && sym.Node.Type != nil && (sym.Node.Type.Kind == NSliceType || sym.Node.Type.Kind == NArrayType) {
				c.globalElemSizes[qname] = c.sliceElemSize(sym.Node.Type)
			}
			// Also detect slice composite literal initializers (no explicit type on var)
			if sym.Node != nil && sym.Node.X != nil && sym.Node.X.Kind == NCompositeLit && sym.Node.X.Type != nil &&
				(sym.Node.X.Type.Kind == NSliceType || sym.Node.X.Type.Kind == NArrayType) {
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

	c.compileAssembledFunctions()
	c.rewriteProfileParentCalls()
	c.insertDeferRecoverCallWrappers()
	c.prunePanicPropagationChecks()

	// Pass dispatch data to backend
	c.irmod.TypeIDs = c.typeIDs
	c.irmod.MethodTable = c.methodTable
	c.irmod.IfaceMethods = c.ifaceMethods
	c.irmod.IfaceMethodRets = c.ifaceMethodRets
	if len(c.funcIsZeroCall) > 0 {
		c.irmod.ZeroCallFuncs = make(map[string]bool, len(c.funcIsZeroCall))
		for qname := range c.funcIsZeroCall {
			c.irmod.ZeroCallFuncs[qname] = true
		}
	}
	optErrs := ir.OptimizeIRModule(c.target, c.irmod)
	if len(optErrs) > 0 {
		c.errors = append(c.errors, optErrs...)
	}

	return c.irmod, c.errors
}

func (c *Compiler) initBuiltinTypes() {
	c.types["bool"] = &ir.TypeInfo{Kind: ir.TY_BOOL, Name: "bool", Size: 1, Align: 1}
	c.types["byte"] = &ir.TypeInfo{Kind: ir.TY_BYTE, Name: "byte", Size: 1, Align: 1}
	c.types["int16"] = &ir.TypeInfo{Kind: ir.TY_INT32, Name: "int16", Size: 2, Align: 2}
	c.types["uint16"] = &ir.TypeInfo{Kind: ir.TY_INT32, Name: "uint16", Size: 2, Align: 2}
	c.types["int32"] = &ir.TypeInfo{Kind: ir.TY_INT32, Name: "int32", Size: 4, Align: 4}
	c.types["int"] = &ir.TypeInfo{Kind: ir.TY_INT, Name: "int", Size: 8, Align: 8}
	c.types["float32"] = &ir.TypeInfo{Kind: ir.TY_FLOAT32, Name: "float32", Size: 4, Align: 4}
	c.types["float64"] = &ir.TypeInfo{Kind: ir.TY_FLOAT64, Name: "float64", Size: 8, Align: 8}
	c.types["uintptr"] = &ir.TypeInfo{Kind: ir.TY_UINTPTR, Name: "uintptr", Size: 8, Align: 8}
	c.types["string"] = &ir.TypeInfo{Kind: ir.TY_STRING, Name: "string", Size: 16, Align: 8}
	c.types["error"] = &ir.TypeInfo{Kind: ir.TY_INTERFACE, Name: "error", Size: 16, Align: 8}
	c.types["int64"] = &ir.TypeInfo{Kind: ir.TY_INT, Name: "int64", Size: 8, Align: 8}
	c.typeIDs["bool"] = 3
	c.ifaceMethods["interface{}"] = []string{}
	c.ifaceMethods["error"] = []string{"Error"}
	c.ifaceMethodRets["error\x00Error"] = 1
	c.ifaceMethodRetLists["error\x00Error"] = []string{"string"}
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
		return name == "int" || name == "int8" || name == "int16" || name == "int32" || name == "int64" || name == "iota" || name == "imag"
	case 'u':
		return name == "uint" || name == "uint8" || name == "uint16" || name == "uint32" || name == "uint64" || name == "uintptr"
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
	if cached, ok := c.structTypeLookup[qualifiedType]; ok {
		if cached.ok {
			return cached.typeNode, cached.pkgPath
		}
		return nil, ""
	}
	dotIdx := -1
	i := 0
	for i < len(qualifiedType) {
		if qualifiedType[i] == '.' {
			dotIdx = i
		}
		i++
	}
	if dotIdx < 0 {
		c.structTypeLookup[qualifiedType] = structTypeLookupResult{}
		return nil, ""
	}
	pkgPath := qualifiedType[0:dotIdx]
	typeName := qualifiedType[dotIdx+1 : len(qualifiedType)]
	if len(typeName) > 0 && typeName[0] == '*' {
		typeName = typeName[1:len(typeName)]
	}
	pkg, ok := c.mod.Packages[pkgPath]
	if !ok {
		c.structTypeLookup[qualifiedType] = structTypeLookupResult{}
		return nil, ""
	}
	if pkgPath == c.curPkg.Path {
		if localDecl, ok := c.localTypeDecls[typeName]; ok && localDecl != nil && localDecl.Type != nil {
			return localDecl.Type, pkgPath
		}
	}
	sym, ok := pkg.Symbols[typeName]
	if !ok || sym.Kind != SymType || sym.Node == nil {
		c.structTypeLookup[qualifiedType] = structTypeLookupResult{}
		return nil, ""
	}
	typeNode := sym.Node.Type
	if typeNode == nil {
		c.structTypeLookup[qualifiedType] = structTypeLookupResult{}
		return nil, ""
	}
	if len(c.structTypeLookup) >= structTypeLookupMaxEntries {
		for key := range c.structTypeLookup {
			delete(c.structTypeLookup, key)
		}
	}
	c.structTypeLookup[qualifiedType] = structTypeLookupResult{
		typeNode: typeNode,
		pkgPath:  pkgPath,
		ok:       true,
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

func (c *Compiler) storageSizeForTypeName(typeName string) int {
	typeName = c.resolveStorageTypeName(typeName, 0)
	if typeName == "float32" {
		return 4
	}
	if typeName == "int64" || typeName == "uint64" || typeName == "float64" {
		return 8
	}
	return c.target.PtrSize
}

func (c *Compiler) structFieldStorageSize(field *Node, pkgPath string) int {
	if field == nil || field.Type == nil {
		return c.target.PtrSize
	}
	return c.storageSizeForTypeName(c.qualifyTypeName(nodeTypeName(field.Type), pkgPath))
}

func (c *Compiler) emitZeroValueForTypeName(typeName string) {
	typeName = c.resolveStorageTypeName(typeName, 0)
	if typeName == "float32" {
		c.emit(makeInst(ir.OP_CONST_F32, 0, 4, 0, "0.0"))
		return
	}
	if typeName == "float64" {
		c.emit(makeInst(ir.OP_CONST_F64, 0, 8, 0, "0.0"))
		return
	}
	if typeName == "int64" || typeName == "uint64" {
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0, Width: 8})
		return
	}
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
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
	offset := 0
	for _, field := range typeNode.Nodes {
		if field.Kind != NField {
			continue
		}
		if field.Name == fieldName {
			return []int{offset}, true
		}
		offset = offset + c.structFieldStorageSize(field, pkgPath)
	}

	// Promoted field match through embedded fields.
	offset = 0
	for _, field := range typeNode.Nodes {
		if field.Kind != NField {
			continue
		}
		if field.Type != nil && field.Name == nodeTypeName(field.Type) {
			embeddedType := c.qualifyTypeName(nodeTypeName(field.Type), pkgPath)
			if subPath, ok := c.resolveFieldPathRec(embeddedType, fieldName, visited); ok {
				path := []int{offset}
				for _, off := range subPath {
					path = append(path, off)
				}
				return path, true
			}
		}
		offset = offset + c.structFieldStorageSize(field, pkgPath)
	}
	return nil, false
}

func (c *Compiler) resolveFieldPath(qualifiedType string, fieldName string) ([]int, bool) {
	return c.resolveFieldPathRec(qualifiedType, fieldName, make(map[string]bool))
}

// resolveFieldType looks up the type of a struct field given a qualified type name and field name.
func (c *Compiler) resolveFieldType(qualifiedType string, fieldName string) string {
	qualifiedType = c.qualifyTypeName(qualifiedType, "")
	field, pkgPath := c.lookupStructField(qualifiedType, fieldName)
	if field != nil && field.Type != nil {
		return c.qualifyTypeName(nodeTypeName(field.Type), pkgPath)
	}
	typeNode, ownerPkg := c.lookupStructTypeNode(qualifiedType)
	if typeNode == nil {
		return ""
	}
	for _, embedded := range typeNode.Nodes {
		if embedded.Kind != NField || embedded.Type == nil || embedded.Name != nodeTypeName(embedded.Type) {
			continue
		}
		embeddedType := c.qualifyTypeName(nodeTypeName(embedded.Type), ownerPkg)
		if t := c.resolveFieldType(embeddedType, fieldName); t != "" {
			return t
		}
	}
	return ""
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

func (c *Compiler) structFieldIsInterface(typeName string, fieldName string) bool {
	if typeName == "" || fieldName == "" {
		return false
	}
	qualifiedType := c.qualifyTypeName(typeName, "")
	field, pkgPath := c.lookupStructField(qualifiedType, fieldName)
	if field == nil || field.Type == nil {
		return false
	}
	fieldTypeName := nodeTypeName(field.Type)
	fieldTypeQualified := c.qualifyTypeName(fieldTypeName, pkgPath)
	return c.isInterfaceTypeName(fieldTypeName) || c.isInterfaceTypeName(fieldTypeQualified)
}

// resolveFieldOffset looks up the byte offset of a struct field given a qualified type name and field name.
func (c *Compiler) resolveFieldOffset(qualifiedType string, fieldName string) int {
	if path, ok := c.resolveFieldPath(qualifiedType, fieldName); ok && len(path) > 0 {
		return path[0]
	}
	return -1
}

func (c *Compiler) resolveStructSize(qualifiedType string) int {
	typeNode, pkgPath := c.lookupStructTypeNode(qualifiedType)
	if typeNode == nil {
		return 0
	}
	size := 0
	for _, field := range typeNode.Nodes {
		if field.Kind == NField {
			size = size + c.structFieldStorageSize(field, pkgPath)
		}
	}
	return size
}

// typeElemSize returns storage size in bytes for values of typeName when used as
// slice elements in this compiler's lowered representation.
func (c *Compiler) typeElemSize(typeName string) int {
	if typeName == "" {
		return c.target.PtrSize
	}
	if typeName == "byte" {
		return 1
	}
	if typeName == "float32" {
		return 4
	}
	if typeName == "float64" {
		return 8
	}
	return c.target.PtrSize
}

// resolveFieldElemSize looks up a struct field's type and returns its element size for indexing.
func (c *Compiler) resolveFieldElemSize(qualifiedType string, fieldName string) int {
	field, _ := c.lookupStructField(qualifiedType, fieldName)
	if field == nil || field.Type == nil {
		return 0
	}
	if (field.Type.Kind == NSliceType || field.Type.Kind == NArrayType) && field.Type.X != nil {
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
		if c.curPkg != nil {
			if sym, ok := c.curPkg.Symbols[mapExpr.Name]; ok && sym.Kind == SymVar && sym.Node != nil {
				if sym.Node.Type != nil && sym.Node.Type.Kind == NMapType && sym.Node.Type.Y != nil {
					return nodeTypeName(sym.Node.Type.Y)
				}
				if sym.Node.X != nil && sym.Node.X.Kind == NCompositeLit && sym.Node.X.Type != nil &&
					sym.Node.X.Type.Kind == NMapType && sym.Node.X.Type.Y != nil {
					return nodeTypeName(sym.Node.X.Type.Y)
				}
			}
		}
	}
	if mapExpr.Kind == NSelectorExpr && mapExpr.X != nil {
		if mapExpr.X.Kind == NIdent {
			pkg := c.resolvePackage(mapExpr.X.Name)
			if pkg != nil {
				if sym, ok := pkg.Symbols[mapExpr.Name]; ok && sym.Kind == SymVar && sym.Node != nil {
					if sym.Node.Type != nil && sym.Node.Type.Kind == NMapType && sym.Node.Type.Y != nil {
						return nodeTypeName(sym.Node.Type.Y)
					}
					if sym.Node.X != nil && sym.Node.X.Kind == NCompositeLit && sym.Node.X.Type != nil &&
						sym.Node.X.Type.Kind == NMapType && sym.Node.X.Type.Y != nil {
						return nodeTypeName(sym.Node.X.Type.Y)
					}
				}
			}
		}
		recvType := c.resolveExprType(mapExpr.X)
		if recvType == "" {
			return ""
		}
		return c.resolveFieldMapValueType(recvType, mapExpr.Name)
	}
	if vt := qualifiedMapValueTypeName(c.resolveExprType(mapExpr)); vt != "" {
		return vt
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

func qualifiedMapKeyTypeName(t string) string {
	if len(t) <= 4 || t[0] != 'm' || t[1] != 'a' || t[2] != 'p' || t[3] != '[' {
		return ""
	}
	depth := 1
	i := 4
	for i < len(t) && depth > 0 {
		if t[i] == '[' {
			depth = depth + 1
		}
		if t[i] == ']' {
			depth = depth - 1
		}
		if depth > 0 {
			i = i + 1
		}
	}
	if depth != 0 || i > len(t) {
		return ""
	}
	return t[4:i]
}

func qualifiedMapValueTypeName(t string) string {
	if len(t) <= 4 || t[0] != 'm' || t[1] != 'a' || t[2] != 'p' || t[3] != '[' {
		return ""
	}
	depth := 1
	i := 4
	for i < len(t) && depth > 0 {
		if t[i] == '[' {
			depth = depth + 1
		}
		if t[i] == ']' {
			depth = depth - 1
		}
		i = i + 1
	}
	if depth != 0 || i > len(t) {
		return ""
	}
	return t[i:len(t)]
}

// resolveFieldSliceElemType returns the qualified element type of a struct field that is a slice or array.
func (c *Compiler) resolveFieldSliceElemType(qualifiedType string, fieldName string) string {
	field, pkgPath := c.lookupStructField(qualifiedType, fieldName)
	if field == nil || field.Type == nil || field.Type.X == nil {
		return ""
	}
	if field.Type.Kind != NSliceType && field.Type.Kind != NArrayType {
		return ""
	}
	return c.qualifyTypeName(nodeTypeName(field.Type.X), pkgPath)
}

func splitBracketType(typeName string) (string, bool) {
	if len(typeName) >= 3 && typeName[0] != '[' {
		dotIdx := -1
		for i := 0; i < len(typeName); i++ {
			if typeName[i] == '.' {
				dotIdx = i
			}
		}
		if dotIdx >= 0 && dotIdx+1 < len(typeName) && typeName[dotIdx+1] == '[' {
			typeName = typeName[dotIdx+1:]
		}
	}
	if len(typeName) < 3 || typeName[0] != '[' {
		return "", false
	}
	if typeName[1] == ']' {
		return typeName[2:], true
	}
	i := 1
	for i < len(typeName) && typeName[i] != ']' {
		i++
	}
	if i >= len(typeName) || i == 1 {
		return "", false
	}
	return typeName[i+1:], true
}

func isArrayTypeName(typeName string) bool {
	if len(typeName) < 3 || typeName[0] != '[' {
		return false
	}
	return typeName[1] != ']'
}

func arrayTypeNestingDepth(typeName string) int {
	depth := 0
	for isArrayTypeName(typeName) {
		depth++
		elemType, ok := splitBracketType(typeName)
		if !ok {
			break
		}
		typeName = elemType
	}
	return depth
}

// resolveExprType returns the concrete qualified type of an expression, or "" if unknown.
func (c *Compiler) resolveExprType(node *Node) string {
	if node == nil {
		return ""
	}
	if node.Kind == NFloatLit {
		return "float64"
	}
	if node.Kind == NIdent {
		if sym, ok := c.curPkg.Symbols[node.Name]; ok && sym.Kind == SymConst && sym.Node != nil && c.isConstFloatExpr(sym.Node.X) {
			return c.exprConcreteType(sym.Node.X)
		}
		if ct, ok := c.localConcreteTypes[node.Name]; ok {
			return ct
		}
		if ifaceType, ok := c.localTypes[node.Name]; ok {
			return ifaceType
		}
		qname := c.curPkg.QualName(node.Name)
		if ct, ok := c.globalConcreteTypes[qname]; ok {
			return ct
		}
		// Fallback: infer global variable type from initializer when the
		// declaration omits an explicit type.
		if sym, ok := c.curPkg.Symbols[node.Name]; ok && sym.Kind == SymVar && sym.Node != nil {
			if sym.Node.Type != nil {
				t := c.qualifyTypeName(nodeTypeName(sym.Node.Type), c.curPkg.Path)
				if t != "" {
					c.globalConcreteTypes[qname] = t
					return t
				}
			}
			if sym.Node.X != nil && !(sym.Node.X.Kind == NIdent && sym.Node.X.Name == node.Name) {
				if t := c.exprConcreteType(sym.Node.X); t != "" {
					c.globalConcreteTypes[qname] = t
					return t
				}
				if t := c.resolveExprType(sym.Node.X); t != "" {
					c.globalConcreteTypes[qname] = t
					return t
				}
			}
		}
		return ""
	}
	if node.Kind == NCompositeLit && node.Type != nil {
		return c.qualifyTypeName(nodeTypeName(node.Type), "")
	}
	if node.Kind == NTypeAssertExpr && node.Type != nil {
		return c.qualifiedTypeFromTypeNode(node.Type, "")
	}
	if node.Kind == NUnaryExpr {
		if node.Name == "*" {
			inner := c.resolveExprType(node.X)
			if inner == "" {
				inner = c.exprConcreteType(node.X)
			}
			return derefQualifiedTypeName(inner)
		}
		if node.Name == "&" {
			return c.exprConcreteType(node)
		}
	}
	// Index expression: determine element type from collection type
	if node.Kind == NIndexExpr && node.X != nil {
		collType := c.resolveExprType(node.X)
		if elemType, ok := splitBracketType(collType); ok {
			return elemType
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
		if node.X != nil && node.X.Kind == NIdent {
			switch node.X.Name {
			case "int", "uintptr", "uint", "byte", "int8", "uint8", "int16", "int32", "int64", "uint16", "uint32", "uint64", "float32", "float64":
				return node.X.Name
			}
		}
		if node.X != nil && node.X.Kind == NSelectorExpr && node.X.X != nil {
			if ifaceType := c.resolveExprType(node.X.X); ifaceType != "" {
				if retType, ok := c.ifaceMethodFirstReturnType(ifaceType, node.X.Name); ok {
					return retType
				}
			}
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
			calleePkg := ""
			if node.X != nil && node.X.Kind == NSelectorExpr && node.X.X != nil && node.X.X.Kind == NIdent {
				pkg := c.resolvePackage(node.X.X.Name)
				if pkg != nil {
					calleePkg = pkg.Path
				}
			}
			return c.qualifyTypeName(retTypes[0], calleePkg)
		}
		// Fallback: read return type directly from function declarations when
		// funcRetTypes has not yet been populated for this callee.
		if node.X != nil && node.X.Kind == NSelectorExpr && node.X.X != nil && node.X.X.Kind == NIdent {
			pkg := c.resolvePackage(node.X.X.Name)
			if pkg != nil {
				if sym, ok := pkg.Symbols[node.X.Name]; ok && sym.Kind == SymFunc && sym.Node != nil && sym.Node.Type != nil {
					if sym.Node.Type.Kind == NFuncType && len(sym.Node.Type.Nodes) > 0 {
						return c.qualifyTypeName(nodeTypeName(sym.Node.Type.Nodes[0]), pkg.Path)
					}
					return c.qualifyTypeName(nodeTypeName(sym.Node.Type), pkg.Path)
				}
			}
		}
		if node.X != nil && node.X.Kind == NIdent {
			if sym, ok := c.curPkg.Symbols[node.X.Name]; ok && sym.Kind == SymFunc && sym.Node != nil && sym.Node.Type != nil {
				if sym.Node.Type.Kind == NFuncType && len(sym.Node.Type.Nodes) > 0 {
					return c.qualifyTypeName(nodeTypeName(sym.Node.Type.Nodes[0]), c.curPkg.Path)
				}
				return c.qualifyTypeName(nodeTypeName(sym.Node.Type), c.curPkg.Path)
			}
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

func derefQualifiedTypeName(typeName string) string {
	if typeName == "" {
		return ""
	}
	if len(typeName) > 0 && typeName[0] == '*' {
		return typeName[1:len(typeName)]
	}
	i := -1
	j := 0
	for j+1 < len(typeName) {
		if typeName[j] == '.' && typeName[j+1] == '*' {
			i = j
		}
		j = j + 1
	}
	if i >= 0 {
		return typeName[0:i+1] + typeName[i+2:len(typeName)]
	}
	return ""
}

func isNonStructPointerTargetType(typeName string) bool {
	if typeName == "" {
		return false
	}
	if len(typeName) > 0 && typeName[0] == '[' {
		return true
	}
	switch typeName {
	case "int", "int16", "int32", "int64",
		"uint", "uint16", "uint32", "uint64",
		"uintptr", "byte", "bool", "string":
		return true
	}
	return strings.HasPrefix(typeName, "map[") || strings.HasPrefix(typeName, "func(") || strings.HasPrefix(typeName, "*")
}

func qualifiedPointerTargetInfo(typeName string) (pkgPath string, targetName string, ok bool) {
	targetName = derefQualifiedTypeName(typeName)
	if targetName == "" {
		return "", "", false
	}
	if isNonStructPointerTargetType(targetName) {
		return "", targetName, true
	}
	dotIdx := -1
	for i := 0; i < len(targetName); i++ {
		if targetName[i] == '.' {
			dotIdx = i
		}
	}
	if dotIdx < 0 {
		return "", targetName, true
	}
	return targetName[0:dotIdx], targetName[dotIdx+1 : len(targetName)], true
}

func isBuiltinBoolTypeName(typeName string) bool {
	if typeName == "bool" {
		return true
	}
	if len(typeName) <= len(".bool") || typeName[len(typeName)-len(".bool"):len(typeName)] != ".bool" {
		return false
	}
	prefix := typeName[0 : len(typeName)-len(".bool")]
	return prefix != ""
}

func isFloatTypeName(name string) bool {
	for len(name) > 0 && name[0] == '*' {
		name = name[1:]
	}
	i := len(name) - 1
	for i >= 0 {
		if name[i] == '.' {
			name = name[i+1:]
			break
		}
		i--
	}
	return name == "float32" || name == "float64"
}

// typeWidth returns the byte width for a named type.
// Returns 0 for word-sized types (int, uintptr, pointers, etc).
func typeWidth(name string) int {
	switch name {
	case "byte", "int8", "uint8":
		return 1
	case "int16", "uint16":
		return 2
	case "int32", "uint32", "float32":
		return 4
	case "int64", "uint64", "float64":
		return 8
	}
	return 0
}

func (c *Compiler) setLocalTypeFlags(idx int, typeName string) {
	if idx < 0 || idx >= len(c.curFunc.Locals) {
		return
	}
	typeName = c.resolveStorageTypeName(typeName, 0)
	w := typeWidth(typeName)
	if w != 0 {
		c.curFunc.Locals[idx].Width = w
	}
	c.curFunc.Locals[idx].Is64 = typeName == "uint64" || typeName == "int64"
	c.curFunc.Locals[idx].IsFloat64 = typeName == "float64"
	c.curFunc.Locals[idx].FloatKind = ir.TY_VOID
	if typeName == "float32" {
		c.curFunc.Locals[idx].FloatKind = ir.TY_FLOAT32
	} else if typeName == "float64" {
		c.curFunc.Locals[idx].FloatKind = ir.TY_FLOAT64
	}
}

func (c *Compiler) resolveStorageTypeName(typeName string, depth int) string {
	if typeName == "" || depth > 16 {
		return typeName
	}
	if isBuiltinTypeName(typeName) || c.isInterfaceTypeName(typeName) {
		return typeName
	}
	if len(typeName) > 0 && (typeName[0] == '*' || strings.HasPrefix(typeName, "[]") || strings.HasPrefix(typeName, "map[")) {
		return typeName
	}
	dot := len(typeName) - 1
	for dot >= 0 {
		if typeName[dot] == '.' {
			break
		}
		dot--
	}
	if dot > 0 {
		typeShort := typeName[dot+1:]
		if isBuiltinTypeName(typeShort) || c.isInterfaceTypeName(typeShort) {
			return typeShort
		}
	}
	if decl, ok := c.lookupCurrentTypeDecl(typeName); ok && decl != nil && decl.Type != nil {
		next := c.qualifiedTypeFromTypeNode(decl.Type, c.curPkg.Path)
		if next == "" || next == typeName {
			next = nodeTypeName(decl.Type)
		}
		if next != "" && next != typeName {
			return c.resolveStorageTypeName(next, depth+1)
		}
	}
	dot = len(typeName) - 1
	for dot >= 0 {
		if typeName[dot] == '.' {
			break
		}
		dot--
	}
	if dot > 0 {
		pkgPath := typeName[:dot]
		typeShort := typeName[dot+1:]
		if pkg, ok := c.mod.Packages[pkgPath]; ok && pkg != nil {
			if sym, ok := pkg.Symbols[typeShort]; ok && sym.Kind == SymType && sym.Node != nil && sym.Node.Type != nil {
				next := c.qualifiedTypeFromTypeNode(sym.Node.Type, pkg.Path)
				if next == "" || next == typeName {
					next = nodeTypeName(sym.Node.Type)
				}
				if next != "" && next != typeName {
					return c.resolveStorageTypeName(next, depth+1)
				}
			}
		}
	}
	return typeName
}

func (c *Compiler) irResultKindForTypeNode(node *Node) ir.TypeKind {
	if node == nil {
		return ir.TY_INT
	}
	typeName := c.resolveStorageTypeName(nodeTypeName(node), 0)
	if typeName == "" {
		return ir.TY_INT
	}
	if len(typeName) > 0 && typeName[0] == '*' {
		return ir.TY_POINTER
	}
	i := len(typeName) - 1
	for i >= 0 {
		if typeName[i] == '.' {
			typeName = typeName[i+1:]
			break
		}
		i--
	}
	switch typeName {
	case "bool":
		return ir.TY_BOOL
	case "byte":
		return ir.TY_BYTE
	case "int16", "uint16", "int32", "uint32":
		return ir.TY_INT32
	case "int", "int64", "uint", "uint64":
		return ir.TY_INT
	case "float32":
		return ir.TY_FLOAT32
	case "float64":
		return ir.TY_FLOAT64
	case "uintptr":
		return ir.TY_UINTPTR
	case "string":
		return ir.TY_STRING
	case "interface{}", "error":
		return ir.TY_INTERFACE
	}
	return ir.TY_INT
}

func (c *Compiler) irResultIs64ForTypeNode(node *Node) bool {
	if node == nil {
		return false
	}
	typeName := c.resolveStorageTypeName(nodeTypeName(node), 0)
	if typeName == "" {
		return false
	}
	i := len(typeName) - 1
	for i >= 0 {
		if typeName[i] == '.' {
			typeName = typeName[i+1:]
			break
		}
		i--
	}
	return typeName == "int64" || typeName == "uint64"
}

func isUnsignedTypeName(name string) bool {
	for len(name) > 0 && name[0] == '*' {
		name = name[1:]
	}
	i := len(name) - 1
	for i >= 0 {
		if name[i] == '.' {
			name = name[i+1:]
			break
		}
		i--
	}
	switch name {
	case "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "byte":
		return true
	}
	return false
}

func (c *Compiler) isUnsignedComparison(node *Node) bool {
	if node == nil || node.X == nil || node.Y == nil {
		return false
	}
	left := c.resolveExprType(node.X)
	if left == "" {
		left = c.exprConcreteType(node.X)
	}
	right := c.resolveExprType(node.Y)
	if right == "" {
		right = c.exprConcreteType(node.Y)
	}
	return isUnsignedTypeName(left) || isUnsignedTypeName(right)
}

func (c *Compiler) isUnsignedExpr(node *Node) bool {
	if node == nil {
		return false
	}
	typ := c.resolveExprType(node)
	if typ == "" {
		typ = c.exprConcreteType(node)
	}
	return isUnsignedTypeName(typ)
}

func (c *Compiler) isFloatExpr(node *Node) bool {
	if node == nil {
		return false
	}
	if node.Kind == NFloatLit {
		return true
	}
	typ := c.resolveExprType(node)
	if typ == "" {
		typ = c.exprConcreteType(node)
	}
	return isFloatTypeName(typ)
}

func (c *Compiler) floatExprTypeName(node *Node) string {
	if node == nil {
		return ""
	}
	if node.Kind == NFloatLit {
		// Untyped float literals default to float64 unless a surrounding typed
		// context narrows them explicitly.
		return ""
	}
	typ := c.resolveExprType(node)
	if typ == "" {
		typ = c.exprConcreteType(node)
	}
	return floatInstName(c.resolveStorageTypeName(typ, 0))
}

func mergeFloatTypeNames(left string, right string) string {
	if left == "float64" || right == "float64" {
		return "float64"
	}
	if left == "float32" || right == "float32" {
		return "float32"
	}
	return "float64"
}

func floatInstName(typeName string) string {
	typeName = strings.TrimSpace(typeName)
	if typeName == "" {
		return ""
	}
	for len(typeName) > 0 && typeName[0] == '*' {
		typeName = typeName[1:]
	}
	i := len(typeName) - 1
	for i >= 0 {
		if typeName[i] == '.' {
			typeName = typeName[i+1:]
			break
		}
		i--
	}
	if typeName == "float32" || typeName == "float64" {
		return typeName
	}
	return ""
}

func (c *Compiler) floatInstNameForTypeName(typeName string) string {
	return floatInstName(c.resolveStorageTypeName(typeName, 0))
}

func (c *Compiler) resolvedFloatInstName(node *Node) string {
	if node == nil {
		return ""
	}
	if typ := c.floatExprTypeName(node); typ != "" {
		return typ
	}
	if c.isConstFloatExpr(node) {
		return "float64"
	}
	return ""
}

func (c *Compiler) convertSourceKind(node *Node) int64 {
	if node == nil {
		return ir.CONVERT_SRC_UNKNOWN
	}
	typeName := c.resolveExprType(node)
	if typeName == "" {
		typeName = c.exprConcreteType(node)
	}
	if typeName == "" && c.isConstFloatExpr(node) {
		typeName = "float64"
	}
	switch c.resolveStorageTypeName(typeName, 0) {
	case "float32":
		return ir.CONVERT_SRC_FLOAT32
	case "float64":
		return ir.CONVERT_SRC_FLOAT64
	case "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "byte":
		return ir.CONVERT_SRC_UINT
	case "bool", "int", "int8", "int16", "int32", "int64", "rune":
		return ir.CONVERT_SRC_INT
	default:
		return ir.CONVERT_SRC_UNKNOWN
	}
}

func (c *Compiler) emitConvertForExpr(node *Node, targetType string) {
	c.emit(makeInst(ir.OP_CONVERT, 0, c.exprWidth(node), c.convertSourceKind(node), targetType))
}

func (c *Compiler) maybeConvertArgForParamType(arg *Node, paramType string) {
	paramType = c.resolveStorageTypeName(paramType, 0)
	if isFloatTypeName(paramType) {
		argType := c.floatExprTypeName(arg)
		if argType == "" || argType != paramType {
			c.emitConvertForExpr(arg, paramType)
		}
	}
}

// exprWidth infers the operand width from an AST expression.
// Returns 0 for word-sized, or 1/2/4/8 for explicitly sized types.
func (c *Compiler) exprWidth(node *Node) int {
	if node == nil {
		return 0
	}
	switch node.Kind {
	case NFloatLit:
		return 8
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
		// Type conversions: uint64(), int64(), int32(), byte(), etc.
		calleeName := c.resolveCallName(node.X)
		tw := typeWidth(calleeName)
		if tw != 0 {
			return tw
		}
		if retTypes, ok := c.callReturnTypes(node); ok && len(retTypes) > 0 {
			return typeWidth(retTypes[0])
		}
	case NBinaryExpr:
		switch node.Name {
		case "==", "!=", "<", ">", "<=", ">=", "&&", "||":
			return typeWidth("bool")
		}
		lw := c.exprWidth(node.X)
		rw := c.exprWidth(node.Y)
		if lw > rw {
			return lw
		}
		return rw
	case NUnaryExpr:
		if node.Name == "!" {
			return typeWidth("bool")
		}
		return c.exprWidth(node.X)
	}
	return 0
}

// precomputeConsts walks all const declarations in a package, tracking iota,
// and stores computed values in c.constValues.
func (c *Compiler) precomputeConsts(pkg *Package) {
	for _, file := range pkg.Files {
		for _, node := range file.Nodes {
			base, _ := unwrapDirectiveNode(node)
			if base == nil {
				continue
			}
			node = base
			if isDeclGroup(node, "const") {
				// Grouped const block: iota increments for each child.
				var lastExpr *Node
				iotaVal := int64(0)
				for _, child := range node.Nodes {
					qname := pkg.QualName(child.Name)
					if child.X != nil {
						lastExpr = child.X
					}
					if c.isConstStringExpr(lastExpr) {
						c.constStringValues[qname] = c.evalConstString(lastExpr)
					} else if c.isConstFloatExpr(lastExpr) {
						// Float constants are compiled from their AST on demand.
					} else {
						val := c.evalConstExprWithIota(lastExpr, iotaVal)
						c.constValues[qname] = val
					}
					iotaVal++
				}
			} else if node.Kind == NConstDecl {
				// Single const: iota = 0.
				qname := pkg.QualName(node.Name)
				if c.isConstStringExpr(node.X) {
					c.constStringValues[qname] = c.evalConstString(node.X)
				} else if c.isConstFloatExpr(node.X) {
					// Float constants are compiled from their AST on demand.
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
			if right == 0 {
				c.errorf("constant division by zero")
				return 0
			}
			return left / right
		case "%":
			if right == 0 {
				c.errorf("constant modulo by zero")
				return 0
			}
			return left % right
		case "<<":
			if right < 0 || right >= 64 {
				c.errorf("constant shift overflow")
				return 0
			}
			return int64(uint64(left) << uint(right))
		case ">>":
			if v, ok := c.evalRightShiftViaConstLeftShift(node.X, right, iotaVal); ok {
				return v
			}
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

func (c *Compiler) evalRightShiftViaConstLeftShift(leftExpr *Node, rightShift int64, iotaVal int64) (int64, bool) {
	if leftExpr == nil || leftExpr.Kind != NIdent || rightShift < 0 {
		return 0, false
	}
	sym, ok := c.curPkg.Symbols[leftExpr.Name]
	if !ok || sym.Kind != SymConst || sym.Node == nil || sym.Node.X == nil {
		return 0, false
	}
	shiftExpr := sym.Node.X
	if shiftExpr.Kind != NBinaryExpr || shiftExpr.Name != "<<" || shiftExpr.Y == nil {
		return 0, false
	}
	leftShift := c.evalConstExprWithIota(shiftExpr.Y, iotaVal)
	if leftShift < 0 {
		return 0, false
	}
	base := c.evalConstExprWithIota(shiftExpr.X, iotaVal)
	if leftShift >= rightShift {
		netShift := leftShift - rightShift
		if netShift >= 63 {
			return 0, false
		}
		return base << uint(netShift), true
	}
	return base >> uint(rightShift-leftShift), true
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

func (c *Compiler) isConstFloatExpr(node *Node) bool {
	if node == nil {
		return false
	}
	switch node.Kind {
	case NFloatLit:
		return true
	case NUnaryExpr:
		return node.Name == "-" && c.isConstFloatExpr(node.X)
	case NBinaryExpr:
		switch node.Name {
		case "+", "-", "*", "/":
			return c.isConstFloatExpr(node.X) || c.isConstFloatExpr(node.Y)
		}
	case NIdent:
		sym, ok := c.curPkg.Symbols[node.Name]
		return ok && sym.Kind == SymConst && sym.Node != nil && c.isConstFloatExpr(sym.Node.X)
	case NCallExpr:
		if node.X != nil && node.X.Kind == NIdent && len(node.Nodes) > 0 {
			return node.X.Name == "float32" || node.X.Name == "float64" || c.isConstFloatExpr(node.Nodes[0])
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
	c.checkTopLevelRedeclarations(pkg)
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

func appendTopDeclNames(node *Node, out []string) []string {
	if node == nil {
		return out
	}
	switch node.Kind {
	case NDirective:
		if node.X != nil {
			return appendTopDeclNames(node.X, out)
		}
	case NDeclGroup:
		for _, child := range node.Nodes {
			out = appendTopDeclNames(child, out)
		}
	case NFunc:
		// Methods have their own receiver namespace and are not package-level decls.
		if node.X != nil {
			break
		}
		if node.Name != "" {
			out = append(out, node.Name)
		}
	case NTypeDecl:
		if node.Name != "" {
			out = append(out, node.Name)
		}
	case NVarDecl, NConstDecl:
		if len(node.Nodes) > 0 {
			for _, child := range node.Nodes {
				if child != nil && child.Name != "" {
					out = append(out, child.Name)
				}
			}
		} else if node.Name != "" {
			out = append(out, node.Name)
		}
	}
	return out
}

func (c *Compiler) checkTopLevelRedeclarations(pkg *Package) {
	seen := make(map[string]bool)
	for _, file := range pkg.Files {
		for _, node := range file.Nodes {
			names := appendTopDeclNames(node, nil)
			for _, name := range names {
				if name == "_" {
					continue
				}
				if seen[name] {
					c.errorf("%s: %s redeclared in this package", pkg.Path, name)
					continue
				}
				seen[name] = true
			}
		}
	}
}

func receiverBaseTypeName(typeName string) string {
	for len(typeName) > 0 && typeName[0] == '*' {
		typeName = typeName[1:len(typeName)]
	}
	return typeName
}

func hasZeroCallDirective(directives []string) bool {
	for _, d := range directives {
		if isZeroCallDirective(d) {
			return true
		}
	}
	return false
}

func isStrictDirectiveAllowed(directive string) bool {
	trimmed := strings.TrimSpace(directive)
	if parseProfileDirective(trimmed) {
		return true
	}
	if parseNoProfileDirective(trimmed) {
		return true
	}
	return len(trimmed) > len("embed ") && strings.HasPrefix(trimmed, "embed ")
}

func isRTGImplementationPackagePath(path string) bool {
	if path == "" {
		return false
	}
	if strings.HasPrefix(path, "j5.nz/rtg/std/") || strings.HasPrefix(path, "j5.nz/rtg/x/") {
		return true
	}
	if path == "runtime" || path == "os" || path == "target" || path == "compiler" {
		return true
	}
	return strings.HasPrefix(path, "compiler/")
}

func isProfileDefaultOnPackagePath(path string) bool {
	if path == "" {
		return false
	}
	if path == "main" || path == "compiler" || path == "target" {
		return true
	}
	if strings.HasPrefix(path, "compiler/") || strings.HasPrefix(path, "target/") {
		return true
	}
	if strings.HasPrefix(path, "j5.nz/rtg/std/compiler/") || strings.HasPrefix(path, "j5.nz/rtg/std/target/") {
		return true
	}
	return false
}

func (c *Compiler) strictDirectiveChecksEnabled(pkg *Package) bool {
	if c == nil || c.target == nil || !c.target.Strict || pkg == nil {
		return false
	}
	return !isRTGImplementationPackagePath(pkg.Path)
}

func isNilExpr(node *Node) bool {
	return node != nil && node.Kind == NBasicLit && node.Name == "nil"
}

func isComparisonOp(op string) bool {
	switch op {
	case "==", "!=", "<", ">", "<=", ">=":
		return true
	}
	return false
}

func isPointerTypeNameForStrict(typeName string) bool {
	if typeName == "" {
		return false
	}
	if typeName[0] == '*' {
		return true
	}
	dot := -1
	i := 0
	for i < len(typeName) {
		if typeName[i] == '.' {
			dot = i
		}
		i++
	}
	return dot >= 0 && dot+1 < len(typeName) && typeName[dot+1] == '*'
}

func (c *Compiler) isPointerExprForStrict(node *Node) bool {
	if node == nil {
		return false
	}
	if node.Kind == NUnaryExpr && node.Name == "&" {
		return true
	}
	t := c.resolveExprType(node)
	if t == "" {
		t = c.exprConcreteType(node)
	}
	return isPointerTypeNameForStrict(t)
}

func (c *Compiler) isSliceExprForStrict(node *Node) bool {
	if node == nil {
		return false
	}
	if c.isStringTypedExpr(node) || isStringExpr(node) {
		return false
	}
	if node.Kind == NCompositeLit && node.Type != nil && node.Type.Kind == NSliceType {
		return true
	}
	if node.Kind == NCallExpr && node.X != nil && node.X.Kind == NSliceType {
		return true
	}
	if node.Kind == NSliceExpr {
		if c.isStringTypedExpr(node.X) || isStringExpr(node.X) {
			return false
		}
		return true
	}
	t := c.resolveExprType(node)
	return strings.HasPrefix(t, "[]")
}

func (c *Compiler) isFuncExprForStrict(node *Node) bool {
	if node == nil {
		return false
	}
	if node.Kind == NFuncType {
		return true
	}
	if node.Kind == NIdent {
		if _, ok := c.localFuncTargets[node.Name]; ok {
			return true
		}
		if _, ok := c.localMethodTargets[node.Name]; ok {
			return true
		}
		if _, ok := c.lookupLocal(node.Name); ok {
			t := c.resolveExprType(node)
			return strings.HasPrefix(t, "func(")
		}
		if c.curPkg != nil {
			if sym, ok := c.curPkg.Symbols[node.Name]; ok {
				if sym.Kind == SymFunc {
					return true
				}
				if sym.Kind == SymVar && sym.Node != nil && sym.Node.Type != nil && sym.Node.Type.Kind == NFuncType {
					return true
				}
			}
		}
	}
	if node.Kind == NSelectorExpr && node.X != nil && node.X.Kind == NIdent {
		pkg := c.resolvePackage(node.X.Name)
		if pkg != nil {
			if sym, ok := pkg.Symbols[node.Name]; ok {
				if sym.Kind == SymFunc {
					return true
				}
				if sym.Kind == SymVar && sym.Node != nil && sym.Node.Type != nil && sym.Node.Type.Kind == NFuncType {
					return true
				}
			}
		}
	}
	t := c.resolveExprType(node)
	return strings.HasPrefix(t, "func(")
}

func (c *Compiler) strictCheckComparison(op string, left *Node, right *Node) {
	if !isComparisonOp(op) {
		return
	}

	leftNil := isNilExpr(left)
	rightNil := isNilExpr(right)
	mapCmp := c.isMapExpr(left) || c.isMapExpr(right)
	sliceCmp := c.isSliceExprForStrict(left) || c.isSliceExprForStrict(right)
	funcCmp := c.isFuncExprForStrict(left) || c.isFuncExprForStrict(right)

	if mapCmp {
		if op != "==" && op != "!=" {
			c.errorf("%s: invalid operation: map values are not ordered", c.curFunc.Name)
			return
		}
		if !leftNil && !rightNil {
			c.errorf("%s: invalid operation: map can only be compared to nil", c.curFunc.Name)
		}
		return
	}
	if sliceCmp {
		if op != "==" && op != "!=" {
			c.errorf("%s: invalid operation: slice values are not ordered", c.curFunc.Name)
			return
		}
		if !leftNil && !rightNil {
			c.errorf("%s: invalid operation: slice can only be compared to nil", c.curFunc.Name)
		}
		return
	}
	if funcCmp {
		if op != "==" && op != "!=" {
			c.errorf("%s: invalid operation: function values are not ordered", c.curFunc.Name)
			return
		}
		if !leftNil && !rightNil {
			c.errorf("%s: invalid operation: function can only be compared to nil", c.curFunc.Name)
		}
		return
	}

	if (op == "<" || op == ">" || op == "<=" || op == ">=") &&
		(c.isPointerExprForStrict(left) || c.isPointerExprForStrict(right)) {
		c.errorf("%s: invalid operation: pointer values are not ordered", c.curFunc.Name)
	}
}

func (c *Compiler) strictCheckPointerArithmetic(op string, left *Node, right *Node) {
	switch op {
	case "+", "-", "*", "/", "%", "&", "|", "^", "<<", ">>":
	default:
		return
	}
	if c.isPointerExprForStrict(left) || c.isPointerExprForStrict(right) {
		c.errorf("%s: pointer arithmetic is not allowed", c.curFunc.Name)
	}
}

func (c *Compiler) filterDirectivesForStrict(pkg *Package, node *Node, directives []string, report bool) []string {
	if !c.strictDirectiveChecksEnabled(pkg) || len(directives) == 0 {
		return directives
	}
	filtered := make([]string, 0, len(directives))
	for _, directive := range directives {
		if isStrictDirectiveAllowed(directive) {
			filtered = append(filtered, directive)
			continue
		}
		if report {
			line := 0
			if node != nil {
				line = node.Pos
			}
			c.errorf("%s: line %d: directive //rtg:%s is not allowed in -strict mode", pkg.Path, line, strings.TrimSpace(directive))
		}
	}
	return filtered
}

func (c *Compiler) collectZeroCallTypeDirectives(pkg *Package) {
	for _, file := range pkg.Files {
		for _, node := range file.Nodes {
			base, directives := unwrapDirectiveNode(node)
			directives = c.filterDirectivesForStrict(pkg, node, directives, false)
			if base == nil || !hasZeroCallDirective(directives) {
				continue
			}
			switch base.Kind {
			case NTypeDecl:
				if base.Name != "" {
					c.typeIsZeroCall[pkg.QualName(base.Name)] = true
				}
			case NDeclGroup:
				for _, child := range base.Nodes {
					if child != nil && child.Kind == NTypeDecl && child.Name != "" {
						c.typeIsZeroCall[pkg.QualName(child.Name)] = true
					}
				}
			}
		}
	}
}

func (c *Compiler) collectFuncRetTypes(pkg *Package) {
	c.collectZeroCallTypeDirectives(pkg)

	for _, file := range pkg.Files {
		for _, node := range file.Nodes {
			fn, directives := unwrapDirectiveNode(node)
			directives = c.filterDirectivesForStrict(pkg, node, directives, false)
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
			delete(c.funcProfileParentABI, qname)
			delete(c.funcIsProfiled, qname)
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
			isZeroCallFunc := false
			hasProfileDirective := false
			hasNoProfileDirective := false
			assembleArch := ""
			for _, d := range directives {
				if isComptimeDirective(d) {
					isComptimeFunc = true
				}
				if isZeroCallDirective(d) {
					isZeroCallFunc = true
				}
				if parseProfileDirective(d) {
					hasProfileDirective = true
				}
				if parseNoProfileDirective(d) {
					hasNoProfileDirective = true
				}
				if arch, ok := parseAssembleDirective(d); ok {
					assembleArch = arch
				}
				if parseInternalDirective(d) != "" {
					c.funcIsInternal[qname] = true
				}
				if _, ok := parseLinkStaticDirective(d); ok {
					c.funcIsLinkStatic[qname] = true
				}
				if isCallbackDirective(d) {
					c.funcIsCallback[qname] = true
				}
			}
			if !isZeroCallFunc && fn.X != nil && fn.X.Type != nil {
				recvBase := receiverBaseTypeName(nodeTypeName(fn.X.Type))
				if recvBase != "" && c.typeIsZeroCall[pkg.QualName(recvBase)] {
					isZeroCallFunc = true
				}
			}
			hasBody := fn.Body != nil
			defaultOn := isProfileDefaultOnPackagePath(pkg.Path)
			// Pre-register variadic info and param count
			profileParentABI := false
			if c.target != nil && c.target.Profile {
				if !hasBody || hasNoProfileDirective {
					profileParentABI = false
				} else if !defaultOn && !hasProfileDirective {
					// Outside default-on packages, callables remain opt-in.
					profileParentABI = false
				} else if fn.X == nil && fn.Name == "init" {
					// init functions are invoked indirectly by runtime init machinery.
					// Keep ABI zero-arg for compatibility.
					profileParentABI = false
				} else if qname != c.entryFunc {
					// Entry point ABI must stay zero-arg for startup.
					// All other callables receive caller hash in profiling mode.
					profileParentABI = true
				}
			}
			if profileParentABI {
				c.funcProfileParentABI[qname] = true
			}
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
				if profileParentABI {
					paramCount++
					fixedParams++
					paramTypeNames = append(paramTypeNames, "uint32")
				}
			} else if profileParentABI {
				// For profiled plain functions, caller hash is synthetic arg 0.
				paramCount++
				fixedParams++
				paramTypeNames = append(paramTypeNames, "uint32")
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
			if isZeroCallFunc {
				c.funcIsZeroCall[qname] = true
			}
			if c.target != nil && c.target.Profile {
				if !hasBody || hasNoProfileDirective {
					// Explicit opt-out for hot methods/functions.
				} else if !defaultOn && !hasProfileDirective {
					// Outside default-on packages, callables remain opt-in.
				} else {
					// Methods/functions are default-on in selected profiling packages.
					c.funcIsProfiled[qname] = true
				}
			}
			if assembleArch != "" {
				c.assembleFuncs[qname] = assembleInfo{
					Arch:     assembleArch,
					Params:   paramCount,
					RetCount: len(retTypeNames),
				}
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
	if node.Kind == NDirective {
		c.collectInterfaceDecl(pkg, node.X)
		return
	}
	if node.Kind == NTypeDecl && node.Type != nil && node.Type.Kind == NInterfaceType {
		qname := pkg.QualName(node.Name)
		var methods []string
		for _, meth := range node.Type.Nodes {
			if meth.Kind == NFunc {
				methods = append(methods, meth.Name)
				retCount := 0
				firstRetType := ""
				var retTypes []string
				if meth.Type != nil {
					if meth.Type.Kind == NFuncType {
						retCount = len(meth.Type.Nodes)
						if len(meth.Type.Nodes) > 0 {
							firstRetNode := meth.Type.Nodes[0]
							if firstRetNode != nil && firstRetNode.Type != nil {
								firstRetNode = firstRetNode.Type
							}
							firstRetType = c.qualifiedTypeFromTypeNode(firstRetNode, pkg.Path)
						}
						for _, ret := range meth.Type.Nodes {
							retNode := ret
							if retNode != nil && retNode.Type != nil {
								retNode = retNode.Type
							}
							retTypes = append(retTypes, c.qualifiedTypeFromTypeNode(retNode, pkg.Path))
						}
					} else {
						retCount = 1
						firstRetType = c.qualifiedTypeFromTypeNode(meth.Type, pkg.Path)
						retTypes = append(retTypes, firstRetType)
					}
				}
				c.ifaceMethodRets[node.Name+"\x00"+meth.Name] = retCount
				c.ifaceMethodRets[qname+"\x00"+meth.Name] = retCount
				if firstRetType != "" {
					c.ifaceMethodRetTypes[node.Name+"\x00"+meth.Name] = firstRetType
					c.ifaceMethodRetTypes[qname+"\x00"+meth.Name] = firstRetType
				}
				if len(retTypes) > 0 {
					c.ifaceMethodRetLists[node.Name+"\x00"+meth.Name] = retTypes
					c.ifaceMethodRetLists[qname+"\x00"+meth.Name] = retTypes
				}
			}
		}
		c.ifaceMethods[node.Name] = methods
		c.ifaceMethods[qname] = methods
	}
	if node.Kind == NDeclGroup {
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

func (c *Compiler) ifaceMethodFirstReturnType(ifaceType string, methodName string) (string, bool) {
	if ifaceType == "" || methodName == "" {
		return "", false
	}
	key := ifaceType + "\x00" + methodName
	if retType, ok := c.ifaceMethodRetTypes[key]; ok && retType != "" {
		return retType, true
	}
	return "", false
}

func (c *Compiler) ifaceMethodReturnTypes(ifaceType string, methodName string) ([]string, bool) {
	if ifaceType == "" || methodName == "" {
		return nil, false
	}
	key := ifaceType + "\x00" + methodName
	if retTypes, ok := c.ifaceMethodRetLists[key]; ok && len(retTypes) > 0 {
		return retTypes, true
	}
	return nil, false
}

// registerAnonInterfaceType assigns a stable synthetic name for an anonymous
// interface type and records its method set/return counts for interface calls.
func (c *Compiler) registerAnonInterfaceType(typeNode *Node) string {
	if typeNode == nil || typeNode.Kind != NInterfaceType {
		return ""
	}
	if len(typeNode.Nodes) == 0 {
		return "interface{}"
	}
	var methodNames []string
	var retCounts []int
	var firstRetTypes []string
	var allRetTypes [][]string
	for _, meth := range typeNode.Nodes {
		if meth == nil || meth.Kind != NFunc || meth.Name == "" {
			continue
		}
		retCount := 0
		var methodRetTypes []string
		if meth.Type != nil {
			if meth.Type.Kind == NFuncType {
				retCount = len(meth.Type.Nodes)
				for _, ret := range meth.Type.Nodes {
					retNode := ret
					if retNode != nil && retNode.Type != nil {
						retNode = retNode.Type
					}
					methodRetTypes = append(methodRetTypes, c.qualifiedTypeFromTypeNode(retNode, ""))
				}
			} else {
				retCount = 1
				methodRetTypes = append(methodRetTypes, c.qualifiedTypeFromTypeNode(meth.Type, ""))
			}
		}
		methodNames = append(methodNames, meth.Name)
		retCounts = append(retCounts, retCount)
		firstRetType := ""
		if len(methodRetTypes) > 0 {
			firstRetType = methodRetTypes[0]
		}
		firstRetTypes = append(firstRetTypes, firstRetType)
		allRetTypes = append(allRetTypes, methodRetTypes)
	}
	if len(methodNames) == 0 {
		return "interface{}"
	}
	key := "interface{"
	i := 0
	for i < len(methodNames) {
		if i > 0 {
			key = key + ";"
		}
		key = fmt.Sprintf("%s%s:%d", key, methodNames[i], retCounts[i])
		i = i + 1
	}
	key = key + "}"
	if _, ok := c.ifaceMethods[key]; !ok {
		c.ifaceMethods[key] = methodNames
	}
	i = 0
	for i < len(methodNames) {
		c.ifaceMethodRets[key+"\x00"+methodNames[i]] = retCounts[i]
		if firstRetTypes[i] != "" {
			c.ifaceMethodRetTypes[key+"\x00"+methodNames[i]] = firstRetTypes[i]
		}
		if len(allRetTypes[i]) > 0 {
			c.ifaceMethodRetLists[key+"\x00"+methodNames[i]] = allRetTypes[i]
		}
		i = i + 1
	}
	return key
}

func (c *Compiler) callReturnTypes(node *Node) ([]string, bool) {
	if node == nil || node.Kind != NCallExpr || node.X == nil {
		return nil, false
	}
	if node.X.Kind == NSelectorExpr && node.X.X != nil {
		ifaceType := c.resolveExprType(node.X.X)
		if ifaceType == "" {
			ifaceType = c.exprConcreteType(node.X.X)
		}
		if retTypes, ok := c.ifaceMethodReturnTypes(ifaceType, node.X.Name); ok {
			return retTypes, true
		}
	}
	calleeName := c.resolveCallName(node.X)
	if node.X.Kind == NIdent {
		if target, ok := c.localFuncTargets[node.X.Name]; ok {
			calleeName = target
		} else if target, ok := c.localMethodTargets[node.X.Name]; ok {
			calleeName = target
		}
	}
	if retTypes, ok := c.funcRetTypes[calleeName]; ok && len(retTypes) > 0 {
		calleePkg := ""
		if node.X.Kind == NSelectorExpr && node.X.X != nil && node.X.X.Kind == NIdent {
			pkg := c.resolvePackage(node.X.X.Name)
			if pkg != nil {
				calleePkg = pkg.Path
			}
		}
		qualified := make([]string, 0, len(retTypes))
		for _, retType := range retTypes {
			qualified = append(qualified, c.qualifyTypeName(retType, calleePkg))
		}
		return qualified, true
	}
	return nil, false
}

func (c *Compiler) qualifiedTypeFromTypeNode(typeNode *Node, pkgPath string) string {
	if typeNode == nil {
		return ""
	}
	if typeNode.Kind == NInterfaceType {
		return c.registerAnonInterfaceType(typeNode)
	}
	typeName := nodeTypeName(typeNode)
	if typeName == "" {
		return ""
	}
	return c.qualifyTypeName(typeName, pkgPath)
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
		c.methodFuncNames[qname] = true
		// Assign type ID if not yet assigned
		if _, ok := c.typeIDs[qtype]; !ok {
			c.typeIDs[qtype] = c.nextTypeID
			c.nextTypeID++
		}
	}
}

type directiveInit struct {
	key      string
	qname    string
	registry string
}

type artifactInit struct {
	id    string
	qname string
}

func sortDirectiveInits(inits []directiveInit) {
	i := 1
	for i < len(inits) {
		j := i
		for j > 0 {
			prev := inits[j-1]
			cur := inits[j]
			if stringLess(cur.key, prev.key) {
				inits[j-1] = cur
				inits[j] = prev
				j = j - 1
				continue
			}
			if cur.key == prev.key && stringLess(cur.qname, prev.qname) {
				inits[j-1] = cur
				inits[j] = prev
				j = j - 1
				continue
			}
			break
		}
		i = i + 1
	}
}

func abiDirectiveRegistry(retType string) (string, bool) {
	if retType == "string" {
		return targetRegisterABI, true
	}
	if retType == "ABIProvider" || retType == "GenericABI" {
		return targetRegisterABIExternal, true
	}
	if strings.HasSuffix(retType, ".ABIProvider") || strings.HasSuffix(retType, ".GenericABI") {
		return targetRegisterABIExternal, true
	}
	return "", false
}

func (c *Compiler) collectTargetDirectiveInits(pkg *Package) ([]directiveInit, []directiveInit, []directiveInit, []directiveInit) {
	var targetInits []directiveInit
	var abiInits []directiveInit
	var asmInits []directiveInit
	var fmtInits []directiveInit
	targetByTriple := make(map[string]string)
	abiByTriple := make(map[string]string)
	asmByName := make(map[string]string)
	fmtByName := make(map[string]string)

	for _, file := range pkg.Files {
		for _, node := range file.Nodes {
			base, directives := unwrapDirectiveNode(node)
			directives = c.filterDirectivesForStrict(pkg, node, directives, false)
			if base == nil || base.Kind != NFunc {
				continue
			}
			qname := pkg.QualName(base.Name)
			for _, d := range directives {
				if triple, ok := parseTargetDirective(d); ok {
					if base.X != nil {
						c.errorf("%s: //rtg:target %s cannot be used on methods", qname, triple)
						continue
					}
					if len(base.Nodes) != 0 {
						c.errorf("%s: //rtg:target %s requires a zero-argument function", qname, triple)
						continue
					}
					if retCount, found := c.funcRets[qname]; !found || retCount != 1 {
						c.errorf("%s: //rtg:target %s requires exactly one return value", qname, triple)
						continue
					}
					if prev, exists := targetByTriple[triple]; exists {
						if prev != qname {
							c.errorf("%s: duplicate //rtg:target %s (already declared by %s)", qname, triple, prev)
						}
						continue
					}
					targetByTriple[triple] = qname
					targetInits = append(targetInits, directiveInit{key: triple, qname: qname})
				}
				if triple, ok := parseTargetABIDirective(d); ok {
					if base.X != nil {
						c.errorf("%s: //rtg:targetabi %s cannot be used on methods", qname, triple)
						continue
					}
					if len(base.Nodes) != 0 {
						c.errorf("%s: //rtg:targetabi %s requires a zero-argument function", qname, triple)
						continue
					}
					if retCount, found := c.funcRets[qname]; !found || retCount != 1 {
						c.errorf("%s: //rtg:targetabi %s requires exactly one return value", qname, triple)
						continue
					}
					retTypes := c.funcRetTypes[qname]
					if len(retTypes) != 1 {
						c.errorf("%s: //rtg:targetabi %s requires exactly one return value", qname, triple)
						continue
					}
					registry, ok := abiDirectiveRegistry(retTypes[0])
					if !ok {
						c.errorf("%s: //rtg:targetabi %s requires return type string, target.ABIProvider, or target.GenericABI", qname, triple)
						continue
					}
					if prev, exists := abiByTriple[triple]; exists {
						if prev != qname {
							c.errorf("%s: duplicate //rtg:targetabi %s (already declared by %s)", qname, triple, prev)
						}
						continue
					}
					abiByTriple[triple] = qname
					abiInits = append(abiInits, directiveInit{key: triple, qname: qname, registry: registry})
				}
				if name, ok := parseAssemblerDirective(d); ok {
					if base.X != nil {
						c.errorf("%s: //rtg:assembler %s cannot be used on methods", qname, name)
						continue
					}
					if len(base.Nodes) != 0 {
						c.errorf("%s: //rtg:assembler %s requires a zero-argument function", qname, name)
						continue
					}
					if retCount, found := c.funcRets[qname]; !found || retCount != 1 {
						c.errorf("%s: //rtg:assembler %s requires exactly one return value", qname, name)
						continue
					}
					retTypes := c.funcRetTypes[qname]
					if len(retTypes) != 1 || retTypes[0] != "string" {
						c.errorf("%s: //rtg:assembler %s requires return type string", qname, name)
						continue
					}
					if prev, exists := asmByName[name]; exists {
						if prev != qname {
							c.errorf("%s: duplicate //rtg:assembler %s (already declared by %s)", qname, name, prev)
						}
						continue
					}
					asmByName[name] = qname
					asmInits = append(asmInits, directiveInit{key: name, qname: qname})
				}
				if name, ok := parseBinFormatDirective(d); ok {
					if base.X != nil {
						c.errorf("%s: //rtg:binfmt %s cannot be used on methods", qname, name)
						continue
					}
					if len(base.Nodes) != 0 {
						c.errorf("%s: //rtg:binfmt %s requires a zero-argument function", qname, name)
						continue
					}
					if retCount, found := c.funcRets[qname]; !found || retCount != 1 {
						c.errorf("%s: //rtg:binfmt %s requires exactly one return value", qname, name)
						continue
					}
					retTypes := c.funcRetTypes[qname]
					if len(retTypes) != 1 || retTypes[0] != "string" {
						c.errorf("%s: //rtg:binfmt %s requires return type string", qname, name)
						continue
					}
					if prev, exists := fmtByName[name]; exists {
						if prev != qname {
							c.errorf("%s: duplicate //rtg:binfmt %s (already declared by %s)", qname, name, prev)
						}
						continue
					}
					fmtByName[name] = qname
					fmtInits = append(fmtInits, directiveInit{key: name, qname: qname})
				}
			}
		}
	}

	sortDirectiveInits(targetInits)
	sortDirectiveInits(abiInits)
	sortDirectiveInits(asmInits)
	sortDirectiveInits(fmtInits)
	return targetInits, abiInits, asmInits, fmtInits
}

func (c *Compiler) collectArtifactDirectiveInits(pkg *Package) []artifactInit {
	var out []artifactInit
	seenByID := make(map[string]string)
	for _, file := range pkg.Files {
		for _, node := range file.Nodes {
			base, directives := unwrapDirectiveNode(node)
			directives = c.filterDirectivesForStrict(pkg, node, directives, false)
			if base == nil || base.Kind != NVarDecl || base.Name == "" || len(base.Nodes) != 0 {
				continue
			}
			qname := pkg.QualName(base.Name)
			for _, d := range directives {
				id, ok := parseArtifactDirective(d)
				if !ok {
					continue
				}
				if prev, exists := seenByID[id]; exists && prev != qname {
					c.errorf("%s: duplicate //rtg:artifact id=%s (already declared by %s)", qname, id, prev)
					continue
				}
				seenByID[id] = qname
				out = append(out, artifactInit{id: id, qname: qname})
			}
		}
	}
	return out
}

func collectVarInitDecls(node *Node, inits *[]*Node) {
	if node == nil {
		return
	}
	if node.Kind == NDirective {
		collectVarInitDecls(node.X, inits)
		return
	}
	if node.Kind == NDeclGroup {
		for _, child := range node.Nodes {
			collectVarInitDecls(child, inits)
		}
		return
	}
	if node.Kind != NVarDecl {
		return
	}
	if node.X != nil {
		*inits = append(*inits, node)
		return
	}
	for _, child := range node.Nodes {
		if child != nil && child.X != nil {
			*inits = append(*inits, child)
		}
	}
}

func (c *Compiler) compileGlobalInits(pkg *Package) {
	// Collect all global var decls with initializers
	var inits []*Node
	for _, file := range pkg.Files {
		for _, node := range file.Nodes {
			collectVarInitDecls(node, &inits)
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
	targetInits, abiInits, asmInits, fmtInits := c.collectTargetDirectiveInits(pkg)
	artifactInits := c.collectArtifactDirectiveInits(pkg)

	if len(inits) == 0 && len(embeds) == 0 && len(targetInits) == 0 && len(abiInits) == 0 && len(asmInits) == 0 && len(fmtInits) == 0 && len(artifactInits) == 0 {
		return
	}
	// Create a synthetic init function for global var initialization
	f := &ir.IRFunc{Name: pkg.Path + ".init$globals"}
	savedPanicUnwindLabel := c.panicUnwindLabel
	savedPanicCheckSlowLabels := c.panicCheckSlowLabels
	savedPanicCheckSlowDepths := c.panicCheckSlowDepths
	savedNamedResultNames := c.namedResultNames
	c.curFunc = f
	c.scopes = nil
	c.localElemSizes = make(map[string]int)
	c.localStringVars = make(map[string]bool)
	c.localAddrOf = make(map[string]bool)
	c.localConcreteTypes = make(map[string]string)
	c.localMapVars = make(map[string]int)
	c.localMapValueTypes = make(map[string]string)
	c.stackDepth = 0
	c.namedResultNames = nil
	c.panicUnwindLabel = c.newLabel()
	c.resetPanicPropagationOutlineState()
	c.pushScope()
	for _, node := range inits {
		qname := pkg.QualName(node.Name)
		gidx, ok := c.globals[qname]
		if !ok {
			continue
		}
		if defineValue, ok := c.lookupDefineValue(qname, node.Name); ok {
			c.emit(makeInst(ir.OP_CONST_STR, 0, 0, 0, encodeStringLiteral(defineValue)))
			c.emit(ir.Inst{Op: ir.OP_GLOBAL_SET, Arg: gidx})
			continue
		}
		c.compileExpr(node.X)
		if node.Type != nil && node.Type.Kind == NArrayType {
			c.maybeCloneArrayForTypeName(nodeTypeName(node.Type))
		} else {
			c.maybeCloneArrayForTypeName(c.exprConcreteType(node.X))
		}
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

	if c.target != nil && c.target.CompileAsArtifacts != nil {
		for _, art := range artifactInits {
			payload, ok := c.target.CompileAsArtifacts[art.qname]
			if !ok {
				continue
			}
			gidx, exists := c.globals[art.qname]
			if !exists {
				continue
			}
			c.emit(makeInst(ir.OP_CONST_STR, 0, 0, 0, encodeStringLiteral(payload)))
			c.emit(makeInst(ir.OP_CONVERT, 0, 0, 0, "[]byte"))
			c.emit(ir.Inst{Op: ir.OP_GLOBAL_SET, Arg: gidx})
		}
	}

	for _, reg := range targetInits {
		c.emit(makeInst(ir.OP_CALL, 0, 0, 0, reg.qname))
		c.emitKnownCall(targetRegisterFn, 1, 0)
	}
	for _, reg := range abiInits {
		registry := reg.registry
		if registry == "" {
			registry = targetRegisterABI
		}
		c.emit(makeInst(ir.OP_CONST_STR, 0, 0, 0, encodeStringLiteral(reg.key)))
		c.emit(makeInst(ir.OP_CALL, 0, 0, 0, reg.qname))
		c.emitKnownCall(registry, 2, 0)
	}
	for _, reg := range asmInits {
		c.emit(makeInst(ir.OP_CONST_STR, 0, 0, 0, encodeStringLiteral(reg.key)))
		c.emit(makeInst(ir.OP_CALL, 0, 0, 0, reg.qname))
		c.emitKnownCall(targetRegisterAsm, 2, 0)
	}
	for _, reg := range fmtInits {
		c.emit(makeInst(ir.OP_CONST_STR, 0, 0, 0, encodeStringLiteral(reg.key)))
		c.emit(makeInst(ir.OP_CALL, 0, 0, 0, reg.qname))
		c.emitKnownCall(targetRegisterFmt, 2, 0)
	}
	c.emit(ir.Inst{Op: ir.OP_RETURN, Arg: 0})
	if c.panicUnwindLabel >= 0 {
		c.emitPanicPropagationSlowPaths()
		// Panic-unwind path shared by call-site panic propagation checks.
		c.emitLabel(c.panicUnwindLabel)
		recoveredLabel := c.newLabel()
		c.emitKnownCall("runtime.PanicWasRecovered", 0, 1)
		c.emit(ir.Inst{Op: ir.OP_JMP_IF, Arg: recoveredLabel})
		c.emitRecoveredPanicReturn()
		c.emitLabel(recoveredLabel)
		c.emitKnownCall("runtime.PanicReset", 0, 0)
		c.emitRecoveredPanicReturn()
	}
	if c.stackDepth != 0 {
		panic(fmt.Sprintf("ICE: stack not balanced at end of function %s (depth=%d)", c.curFunc.Name, c.stackDepth))
	}
	c.irmod.Funcs = append(c.irmod.Funcs, f)
	c.curFunc = nil
	c.panicUnwindLabel = savedPanicUnwindLabel
	c.panicCheckSlowLabels = savedPanicCheckSlowLabels
	c.panicCheckSlowDepths = savedPanicCheckSlowDepths
	c.namedResultNames = savedNamedResultNames
}

func (c *Compiler) lookupDefineValue(qualifiedName string, shortName string) (string, bool) {
	if c == nil || c.target == nil || c.target.Defines == nil {
		return "", false
	}
	if v, ok := c.target.Defines[qualifiedName]; ok {
		return v, true
	}
	if v, ok := c.target.Defines[shortName]; ok {
		return v, true
	}
	return "", false
}

func (c *Compiler) compileEmbedInit(pkg *Package, gidx int, pattern string) {
	patterns := splitEmbedPatterns(pattern)
	if len(patterns) == 0 {
		patterns = []string{pattern}
	}

	var names []string
	var data []string
	seen := make(map[string]bool)
	for _, pat := range patterns {
		// Resolve each embed pattern relative to the package directory.
		embedDir := cleanPath(pkg.Dir + "/" + pat)
		prefix := embedPatternPrefix(pat)

		// Try embedded FS first (when self-hosting from embedded std),
		// then fall back to disk.
		partNames, partData := stdlib.WalkEmbedFromFS(embedDir)
		if partNames == nil {
			partNames, partData = common.WalkDirectory(embedDir, embedDir)
		}
		// Embedded std package paths are often stored without the "std/" prefix
		// (for example "compiler/stdlib"). Retry with a std-prefixed base so
		// patterns like "../../../x" can still resolve to top-level x/.
		if len(partNames) == 0 && !strings.HasPrefix(pkg.Dir, "std/") {
			altDir := cleanPath("std/" + pkg.Dir + "/" + pat)
			if altDir != embedDir {
				altNames, altData := stdlib.WalkEmbedFromFS(altDir)
				if altNames == nil {
					altNames, altData = common.WalkDirectory(altDir, altDir)
				}
				if len(altNames) > 0 {
					partNames = altNames
					partData = altData
				}
			}
		}
		i := 0
		for i < len(partNames) && i < len(partData) {
			name := partNames[i]
			if prefix != "" {
				if name == "" {
					name = prefix
				} else if !strings.HasPrefix(name, prefix+"/") && name != prefix {
					name = prefix + "/" + name
				}
			}
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
				data = append(data, partData[i])
			}
			i = i + 1
		}
	}

	// Sort for deterministic order
	sortEmbedFiles(names, data)

	// Create empty FS struct: push 2 nil fields (names, data slices)
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0}) // nil names slice
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0}) // nil data slice
	c.emitCompositeCall("embed.FS", 2)
	c.emit(ir.Inst{Op: ir.OP_GLOBAL_SET, Arg: gidx})

	// For each file, call embed.AddFile(fs, name, data)
	for i := 0; i < len(names); i++ {
		c.emit(ir.Inst{Op: ir.OP_GLOBAL_GET, Arg: gidx})
		c.emit(makeInst(ir.OP_CONST_STR, 0, 0, 0, encodeStringLiteral(names[i])))
		c.emit(makeInst(ir.OP_CONST_STR, 0, 0, 0, encodeStringLiteral(data[i])))
		c.emitKnownCall("embed.AddFile", 3, 0)
	}
}

func splitEmbedPatterns(pattern string) []string {
	fields := strings.Fields(pattern)
	var out []string
	i := 0
	for i < len(fields) {
		if fields[i] != "" {
			out = append(out, fields[i])
		}
		i = i + 1
	}
	return out
}

func embedPatternPrefix(pattern string) string {
	parts := strings.Split(pattern, "/")
	var clean []string
	i := 0
	for i < len(parts) {
		p := parts[i]
		if p == "" || p == "." || p == ".." {
			i = i + 1
			continue
		}
		clean = append(clean, p)
		i = i + 1
	}
	return strings.Join(clean, "/")
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
		directives = c.filterDirectivesForStrict(c.curPkg, node, directives, true)
		if base != nil && base.Kind == NFunc {
			intern := ""
			var linkspec LinkStaticDirective
			hasLinkStatic := false
			assembleArch := ""
			hasCallback := false
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
				if arch, ok := parseAssembleDirective(d); ok {
					assembleArch = arch
				}
				if isCallbackDirective(d) {
					hasCallback = true
				}
			}
			if intern != "" {
				c.compileIntrinsicFunc(base, intern)
			} else if hasLinkStatic {
				c.compileLinkStaticFunc(base, linkspec)
			} else if assembleArch != "" {
				c.compileAssembleFunc(base, assembleArch)
			} else {
				c.compileFunc(base)
			}
			if hasCallback {
				qname := c.curPkg.QualName(base.Name)
				if c.irmod.CallbackFuncs == nil {
					c.irmod.CallbackFuncs = make(map[string]bool)
				}
				c.irmod.CallbackFuncs[qname] = true
			}
		}
	case NVarDecl:
		// Global var — init handled separately
	case NConstDecl, NTypeDecl, NDeclGroup, NImport:
		// No code to emit
	default:
		panic("ICE: unhandled top-level declaration kind in compileTopDecl")
	}
}

func containsDeferStmt(node *Node) bool {
	if node == nil {
		return false
	}
	stack := make([]*Node, 0, 64)
	stack = append(stack, node)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n == nil {
			continue
		}
		// Defer statements inside nested function literals belong to the nested
		// function, not the current one.
		if n.Kind == NFuncType && n.Body != nil {
			continue
		}
		if n.Kind == NDeferStmt {
			return true
		}
		if n.X != nil {
			stack = append(stack, n.X)
		}
		if n.Y != nil {
			stack = append(stack, n.Y)
		}
		if n.Body != nil {
			stack = append(stack, n.Body)
		}
		if n.Type != nil {
			stack = append(stack, n.Type)
		}
		for i := len(n.Nodes) - 1; i >= 0; i-- {
			child := n.Nodes[i]
			if child != nil {
				stack = append(stack, child)
			}
		}
	}
	return false
}

func (c *Compiler) compileFunc(node *Node) {
	qname := c.curPkg.QualName(node.Name)
	if node.X != nil {
		// Method with receiver
		recvType := nodeTypeName(node.X.Type)
		qname = c.dotJoin(c.curPkg.QualName(recvType), node.Name)
	}
	codeCap := 64
	if node.Body != nil {
		codeCap += len(node.Body.Nodes) * 12
	}
	f := &ir.IRFunc{Name: qname, Code: make([]ir.Inst, 0, codeCap)}
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
	c.deferSites = nil
	c.deferHeadLocal = -1
	c.panicUnwindLabel = -1
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
	c.profileStartLocal = -1
	c.profileParentLocal = -1
	c.profileMethodHash = 0
	c.profileFlushOnExit = false
	c.currentMethodHash = 0
	c.inIfInit = false
	c.ifInitLeakedNames = make(map[string]bool)
	for name, concreteType := range c.captureConcreteTypes {
		c.localConcreteTypes[name] = concreteType
	}
	for name, ifaceType := range c.captureIfaceTypes {
		c.localTypes[name] = ifaceType
	}
	c.pushScope()

	profileParentABI := c.target != nil && c.target.Profile && c.funcProfileParentABI[qname]
	if c.target != nil && c.target.Profile {
		c.currentMethodHash = profileHash32FNV(qname)
	}

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
		recvIdx := c.addLocal(node.X.Name)
		f.Params++
		if profileParentABI {
			c.profileParentLocal = c.addLocal("$profile_parent")
			f.Params++
		}
		// Track concrete type of receiver for self-method calls
		if node.X.Type != nil {
			recvType := nodeTypeName(node.X.Type)
			c.setLocalTypeFlags(recvIdx, recvType)
			c.localConcreteTypes[node.X.Name] = c.qualifyTypeName(recvType, "")
		}
	} else if profileParentABI {
		c.profileParentLocal = c.addLocal("$profile_parent")
		f.Params++
	}

	// Register params
	isVariadic := false
	isIfaceVariadic := false
	varElemSize := c.target.PtrSize
	fixedParams := 0
	if node.X != nil {
		fixedParams = 1 // receiver counts as fixed
	}
	if profileParentABI {
		fixedParams++
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
			if param.Type != nil && param.Type.Kind == NIdent {
				c.setLocalTypeFlags(localIdx, param.Type.Name)
			}
			// Track elem size for slice params
			if isVarParam {
				c.localElemSizes[pname] = varElemSize
			} else if param.Type != nil && (param.Type.Kind == NSliceType || param.Type.Kind == NArrayType) {
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
				// Also track slice/array elem sizes from type
				if elemType, ok := splitBracketType(qualifiedType); ok {
					c.localElemSizes[pname] = c.typeElemSize(elemType)
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
			f.ResultKinds = make([]ir.TypeKind, 0, len(node.Type.Nodes))
			f.ResultIs64 = make([]bool, 0, len(node.Type.Nodes))
			for _, ret := range node.Type.Nodes {
				retTypeNode := ret.Type
				if retTypeNode == nil {
					retTypeNode = ret
				}
				f.ResultKinds = append(f.ResultKinds, c.irResultKindForTypeNode(retTypeNode))
				f.ResultIs64 = append(f.ResultIs64, c.irResultIs64ForTypeNode(retTypeNode))
				if ret.Name != "" {
					retIdx := c.addLocal(ret.Name)
					c.namedResultNames = append(c.namedResultNames, ret.Name)
					if retTypeNode != nil {
						if retTypeNode.Kind == NIdent {
							c.setLocalTypeFlags(retIdx, retTypeNode.Name)
							if retTypeNode.Name == "string" {
								c.localStringVars[ret.Name] = true
							}
						}
						typeName := nodeTypeName(retTypeNode)
						if typeName != "" {
							qualifiedType := c.qualifyTypeName(typeName, "")
							if c.isInterfaceTypeName(typeName) || c.isInterfaceTypeName(qualifiedType) {
								c.localTypes[ret.Name] = qualifiedType
							}
							c.localConcreteTypes[ret.Name] = qualifiedType
							if retTypeNode.Kind == NSliceType || retTypeNode.Kind == NArrayType {
								c.localElemSizes[ret.Name] = c.sliceElemSize(retTypeNode)
							}
							if retTypeNode.Kind == NMapType {
								c.localMapVars[ret.Name] = c.mapKeyKind(retTypeNode.X)
								if retTypeNode.Y != nil {
									c.localMapValueTypes[ret.Name] = nodeTypeName(retTypeNode.Y)
								}
							}
						}
					}
				}
			}
		} else {
			f.RetCount = 1
			f.ResultKinds = []ir.TypeKind{c.irResultKindForTypeNode(node.Type)}
			f.ResultIs64 = []bool{c.irResultIs64ForTypeNode(node.Type)}
		}
	}

	// Pre-register funcRets before compiling body so recursive calls resolve correctly
	c.funcRets[f.Name] = f.RetCount
	c.stackDepth = 0
	if c.funcIsZeroCall[f.Name] {
		c.panicUnwindLabel = -1
	} else {
		c.panicUnwindLabel = c.newLabel()
	}
	c.resetPanicPropagationOutlineState()
	if c.funcIsProfiled[f.Name] {
		c.profileMethodHash = c.currentMethodHash
		if c.profileMethodHash == 0 {
			c.profileMethodHash = profileHash32FNV(f.Name)
		}
		c.profileStartLocal = c.addLocal("$profile_start")
		c.emitKnownCall("runtime.Now", 0, 1)
		c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: c.profileStartLocal})
		arenaMethodHash := c.currentMethodHash
		if arenaMethodHash == 0 {
			arenaMethodHash = profileHash32FNV(f.Name)
		}
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(arenaMethodHash)})
		if c.profileParentLocal >= 0 {
			c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: c.profileParentLocal})
		} else {
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		}
		c.emit(makeInst(ir.OP_CONST_STR, 0, 0, 0, qname))
		c.emit(makeInst(ir.OP_CALL, 3, 0, 0, "runtime.ArenaEnter"))
	}
	if c.target != nil && c.target.Profile && f.Name == c.entryFunc {
		c.profileFlushOnExit = true
	}

	for _, name := range c.namedResultNames {
		if idx, ok := c.lookupLocal(name); ok {
			w := 0
			if idx < len(c.curFunc.Locals) {
				w = c.curFunc.Locals[idx].Width
			}
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx, Width: w})
		}
	}

	if featureDeferEnabled && containsDeferStmt(node.Body) {
		c.deferHeadLocal = c.addLocal("$defer_head")
		c.emit(ir.Inst{Op: ir.OP_CONST_NIL})
		c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: c.deferHeadLocal})
	}

	// Compile body
	if node.Body != nil {
		c.compileBlock(node.Body)
	}

	// Ensure function ends with a return
	codeLen := len(f.Code)
	if codeLen == 0 || f.Code[codeLen-1].Op != ir.OP_RETURN {
		if len(c.namedResultNames) > 0 {
			c.compileReturn(&Node{Kind: NReturn})
		} else {
			if len(c.deferSites) > 0 {
				c.emitDeferredCalls()
			}
			c.emitProfileExit()
			c.emitProfileFinalize()
			c.emit(ir.Inst{Op: ir.OP_RETURN, Arg: 0})
		}
	}

	if c.panicUnwindLabel >= 0 {
		c.emitPanicPropagationSlowPaths()
		// Panic-unwind path shared by call-site panic propagation checks.
		c.emitLabel(c.panicUnwindLabel)
		if len(c.deferSites) > 0 {
			c.emitDeferredCalls()
		}
		recoveredLabel := c.newLabel()
		c.emitKnownCall("runtime.PanicWasRecovered", 0, 1)
		c.emit(ir.Inst{Op: ir.OP_JMP_IF, Arg: recoveredLabel})
		if f.Name == c.entryFunc {
			c.emitProfileExit()
			c.emitProfileFinalize()
			c.emitKnownCall("runtime.PanicValueToString", 0, 1)
			c.emit(ir.Inst{Op: ir.OP_PANIC})
		} else {
			c.emitRecoveredPanicReturn()
		}
		c.emitLabel(recoveredLabel)
		c.emitKnownCall("runtime.PanicReset", 0, 0)
		c.emitRecoveredPanicReturn()
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

func (c *Compiler) compileAssembleFunc(node *Node, arch string) {
	if node == nil {
		return
	}
	if node.X != nil {
		c.errorf("assemble methods are not supported (%s)", node.Name)
		return
	}
	qname := c.curPkg.QualName(node.Name)
	info, ok := c.assembleFuncs[qname]
	if !ok {
		info = assembleInfo{Arch: arch}
	}
	if assemblePkgForArch(arch) == "" {
		c.errorf("%s: unsupported assemble arch %q", qname, arch)
		return
	}
	if c.target.GOARCH != arch {
		c.errorf("%s: assemble %s used when target is %s", qname, arch, c.target.GOARCH)
		return
	}
	if c.target.Backend != "native" {
		c.errorf("%s: assemble functions require native backend (got %s)", qname, c.target.Backend)
		return
	}

	stub := &ir.IRFunc{
		Name:     qname,
		Params:   info.Params,
		RetCount: info.RetCount,
		Code: []ir.Inst{
			{Op: ir.OP_CONST_STR, Name: "native-only function called in VM: " + qname},
			{Op: ir.OP_PANIC},
		},
	}
	c.irmod.Funcs = append(c.irmod.Funcs, stub)

	builderNode := cloneTypeNode(node)
	builderNode.Name = "__rtg_asm_builder_" + node.Name
	builderNode.Type = nil
	builderQname := c.curPkg.QualName(builderNode.Name)

	saved := c.inAssembleBuilder
	c.inAssembleBuilder = true
	c.compileFunc(builderNode)
	c.inAssembleBuilder = saved

	info.Arch = arch
	info.BuilderName = builderQname
	c.assembleFuncs[qname] = info
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
			f.ResultKinds = make([]ir.TypeKind, 0, len(node.Type.Nodes))
			f.ResultIs64 = make([]bool, 0, len(node.Type.Nodes))
			for _, ret := range node.Type.Nodes {
				retTypeNode := ret.Type
				if retTypeNode == nil {
					retTypeNode = ret
				}
				f.ResultKinds = append(f.ResultKinds, c.irResultKindForTypeNode(retTypeNode))
				f.ResultIs64 = append(f.ResultIs64, c.irResultIs64ForTypeNode(retTypeNode))
			}
		} else {
			f.RetCount = 1
			f.ResultKinds = []ir.TypeKind{c.irResultKindForTypeNode(node.Type)}
			f.ResultIs64 = []bool{c.irResultIs64ForTypeNode(node.Type)}
		}
	}

	// Emit single intrinsic call
	c.stackDepth = 0
	c.emit(makeInst(ir.OP_CALL_INTRINSIC, paramCount, 0, 0, intern))
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
			f.ResultKinds = make([]ir.TypeKind, 0, len(node.Type.Nodes))
			f.ResultIs64 = make([]bool, 0, len(node.Type.Nodes))
			for _, ret := range node.Type.Nodes {
				retTypeNode := ret.Type
				if retTypeNode == nil {
					retTypeNode = ret
				}
				f.ResultKinds = append(f.ResultKinds, c.irResultKindForTypeNode(retTypeNode))
				f.ResultIs64 = append(f.ResultIs64, c.irResultIs64ForTypeNode(retTypeNode))
			}
		} else {
			f.RetCount = 1
			f.ResultKinds = []ir.TypeKind{c.irResultKindForTypeNode(node.Type)}
			f.ResultIs64 = []bool{c.irResultIs64ForTypeNode(node.Type)}
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
	c.emit(makeInst(ir.OP_CALL_INTRINSIC, paramCount, 0, 0, intrinsicName))
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
	if c.inIfInit {
		if c.ifInitLeakedNames == nil {
			c.ifInitLeakedNames = make(map[string]bool)
		}
		c.ifInitLeakedNames[name] = true
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

func makeInst(op ir.Opcode, arg int, width int, val int64, name string) ir.Inst {
	var inst ir.Inst
	inst.Op = op
	inst.Arg = arg
	inst.Width = width
	inst.Val = val
	inst.Name = name
	return inst
}

func (c *Compiler) emitKnownCall(name string, argCount int, retCount int) {
	c.curFunc.Code = append(c.curFunc.Code, makeInst(ir.OP_CALL, argCount, 0, 0, name))
	c.stackDepth = c.stackDepth - argCount + retCount
}

func (c *Compiler) emitCompositeCall(typeName string, argCount int) {
	c.curFunc.Code = append(c.curFunc.Code, makeInst(ir.OP_CALL, argCount, 0, 0, "builtin.composite."+typeName))
	c.stackDepth = c.stackDepth - argCount + 1
}

func knownCallRetCount(name string) (int, bool) {
	if len(name) > len("builtin.composite.") {
		match := true
		prefix := "builtin.composite."
		i := 0
		for i < len(prefix) {
			if name[i] != prefix[i] {
				match = false
				break
			}
			i++
		}
		if match {
			return 1, true
		}
	}
	switch name {
	case "runtime.Now",
		"runtime.SliceCloneArray",
		"runtime.SliceMake",
		"runtime.SliceMakeCap",
		"runtime.SliceAppend",
		"runtime.SliceAppendSlice",
		"runtime.SliceCopy",
		"runtime.SliceReslice",
		"runtime.SliceResliceFull",
		"runtime.StringSlice",
		"runtime.StringConcat",
		"runtime.Stringptr",
		"runtime.Tostring",
		"runtime.ByteToString",
		"runtime.RuneToString",
		"runtime.Alloc",
		"runtime.MapMake",
		"runtime.MapSet",
		"runtime.MapLen",
		"runtime.MapEntryKey",
		"runtime.MapEntryValue",
		"runtime.StringEqual",
		"runtime.StringLess",
		"runtime.PanicWasRecovered",
		"runtime.PanicValueToString",
		"runtime.Recover":
		return 1, true
	case "runtime.MapGet",
		"runtime.StringDecodeRune":
		return 2, true
	case "runtime.SysWrite":
		return 3, true
	case "runtime.ArenaEnter",
		"runtime.Memzero",
		"runtime.DeferRecoverEnter",
		"runtime.DeferRecoverExit",
		"runtime.ProfileAllocHash",
		"runtime.PanicBegin",
		"runtime.MapDelete",
		"runtime.ProfileHashNow",
		"runtime.ProfileFlush",
		"runtime.ArenaFlush",
		"runtime.ArenaLeave",
		"runtime.PanicReset",
		"runtime.PanicShouldUnwind":
		return 0, true
	}
	return 0, false
}

func countReturnTypes(fn *Node) int {
	if fn == nil || fn.Type == nil {
		return 0
	}
	if fn.Type.Kind == NFuncType {
		return len(fn.Type.Nodes)
	}
	return 1
}

func hasPackagePrefix(qname string, pkgPath string) bool {
	if len(qname) <= len(pkgPath) || len(pkgPath) == 0 {
		return false
	}
	i := 0
	for i < len(pkgPath) {
		if qname[i] != pkgPath[i] {
			return false
		}
		i++
	}
	return qname[len(pkgPath)] == '.'
}

func (c *Compiler) resolvedCallRetCount(callName string) int {
	if retCount, ok := knownCallRetCount(callName); ok {
		return retCount
	}
	if retCount, ok := c.funcRets[callName]; ok {
		return retCount
	}
	if retTypes, ok := c.funcRetTypes[callName]; ok {
		return len(retTypes)
	}
	if c.mod != nil {
		for _, pkg := range c.mod.Packages {
			if pkg == nil || !hasPackagePrefix(callName, pkg.Path) {
				continue
			}
			shortName := callName[len(pkg.Path)+1:]
			dot := false
			i := 0
			for i < len(shortName) {
				if shortName[i] == '.' {
					dot = true
					break
				}
				i++
			}
			if dot {
				continue
			}
			if sym, ok := pkg.Symbols[shortName]; ok && sym.Kind == SymFunc {
				return countReturnTypes(sym.Node)
			}
		}
	}
	return 1
}

func (c *Compiler) instStackDelta(inst ir.Inst) int {
	switch inst.Op {
	case ir.OP_CONST_I64, ir.OP_CONST_F32, ir.OP_CONST_F64, ir.OP_CONST_STR, ir.OP_CONST_BOOL, ir.OP_CONST_NIL:
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
		return -inst.Arg + c.resolvedCallRetCount(inst.Name)
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
		// consumes receiver + args, produces interface-method return count
		retCount := 1
		if len(inst.Name) > 0 {
			dot := -1
			i := len(inst.Name) - 1
			for i >= 0 {
				if inst.Name[i] == '.' {
					dot = i
					break
				}
				i--
			}
			if dot > 0 && dot+1 < len(inst.Name) {
				ifaceType := inst.Name[:dot]
				methodName := inst.Name[dot+1:]
				if n, ok := c.ifaceMethodReturnCount(ifaceType, methodName); ok {
					retCount = n
				}
			}
		}
		return -(inst.Arg + 1) + retCount
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
			if rhs, ok := sharedVarDeclRHS(node.Nodes); ok {
				lhs := make([]*Node, 0, len(node.Nodes))
				for _, child := range node.Nodes {
					lhs = append(lhs, &Node{Kind: NIdent, Name: child.Name, Pos: child.Pos})
				}
				c.compileAssign(&Node{Kind: NAssign, Name: ":=", Nodes: lhs, Y: rhs, Pos: node.Pos})
				return
			}
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
		c.compileDeferStmt(node)
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
	case NDeclGroup:
		for _, child := range node.Nodes {
			c.compileStmt(child)
		}
	case NBlock:
		c.compileBlock(node)
	default:
		panic("ICE: unhandled statement kind in compileStmt")
	}
}

func (c *Compiler) compileDeferStmt(node *Node) {
	if node == nil {
		return
	}
	if !featureDeferEnabled {
		c.errorf("%s: line %d: defer is not supported", c.curFunc.Name, node.Pos)
		return
	}
	if node.X == nil || node.X.Kind != NCallExpr {
		c.errorf("%s: line %d: defer expects a function call", c.curFunc.Name, node.Pos)
		return
	}
	call := node.X
	site := deferSite{
		callOp:         ir.OP_CALL,
		callName:       c.resolveCallName(call.X),
		variadicElemSz: c.target.PtrSize,
	}
	calleeName := ""
	localFuncTarget := ""
	localMethodTarget := ""
	var selectorRecv *Node
	selectorIfaceRetCount := -1
	if call.X != nil && call.X.Kind == NIdent {
		calleeName = call.X.Name
		if target, ok := c.localFuncTargets[calleeName]; ok {
			localFuncTarget = target
			site.callName = target
		} else if target, ok := c.localMethodTargets[calleeName]; ok {
			localMethodTarget = target
			site.callName = target
		}
	} else if call.X != nil && call.X.Kind == NSelectorExpr && call.X.X != nil {
		// Selector calls can be package-qualified functions (no receiver) or
		// method calls (receiver is the selector base expression).
		isPkgSelector := false
		if call.X.X.Kind == NIdent && c.resolvePackage(call.X.X.Name) != nil {
			isPkgSelector = true
		}
		if !isPkgSelector {
			selectorRecv = call.X.X
			recvType := c.resolveExprType(selectorRecv)
			if recvType != "" {
				if retCount, ok := c.ifaceMethodReturnCount(recvType, call.X.Name); ok {
					site.callOp = ir.OP_IFACE_CALL
					site.callName = c.dotJoin(recvType, call.X.Name)
					selectorIfaceRetCount = retCount
				} else if resolved, ok := c.resolveMethodByConcreteType(recvType, call.X.Name); ok {
					site.callName = resolved
				}
			}
		}
	}
	site.fixedCount, site.isVariadic = c.funcVariadic[site.callName]
	site.variadicIsIface = site.isVariadic && c.funcVariadicIface[site.callName]
	if esz, ok := c.funcVariadicElem[site.callName]; ok {
		site.variadicElemSz = esz
	}
	if n, ok := c.funcRets[site.callName]; ok {
		site.retCount = n
	}
	if selectorIfaceRetCount >= 0 {
		site.retCount = selectorIfaceRetCount
	}
	c.markDeferRecoverWrapTarget(site.callOp, site.callName)
	site.argCount = len(call.Nodes)
	captureArgs := c.localFuncCaptures[calleeName]
	if localFuncTarget != "" {
		site.argCount = site.argCount + len(captureArgs)
	}
	if localMethodTarget != "" {
		site.argCount = site.argCount + 1
	}
	if selectorRecv != nil {
		site.argCount = site.argCount + 1
	}

	if c.deferHeadLocal < 0 {
		c.deferHeadLocal = c.addLocal("$defer_head")
		c.emit(ir.Inst{Op: ir.OP_CONST_NIL})
		c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: c.deferHeadLocal})
	}

	siteID := len(c.deferSites)
	c.deferSites = append(c.deferSites, site)
	recIdx := c.addLocal(fmt.Sprintf("$defer_rec_%d", siteID))
	recordSize := 2*c.target.PtrSize + site.argCount*c.target.PtrSize
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(recordSize)})
	c.emitRuntimeAllocCall()
	c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: recIdx})

	// rec.next = head
	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: c.deferHeadLocal})
	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: recIdx})
	c.emit(ir.Inst{Op: ir.OP_STORE, Arg: c.target.PtrSize})
	// rec.siteID = siteID
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(siteID)})
	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: recIdx})
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(c.target.PtrSize)})
	c.emit(ir.Inst{Op: ir.OP_ADD})
	c.emit(ir.Inst{Op: ir.OP_STORE, Arg: c.target.PtrSize})

	argIndex := 0
	if localFuncTarget != "" {
		for _, capture := range captureArgs {
			if capture.IsPtr {
				c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: capture.LocalIdx})
			} else {
				c.emit(ir.Inst{Op: ir.OP_LOCAL_ADDR, Arg: capture.LocalIdx})
			}
			c.emitStoreDeferredArg(recIdx, argIndex)
			argIndex++
		}
	}
	if localMethodTarget != "" {
		recvIdx, ok := c.localMethodRecv[calleeName]
		if !ok {
			recvIdx, ok = c.lookupLocal(calleeName)
		}
		if ok {
			w := 0
			if recvIdx < len(c.curFunc.Locals) {
				w = c.curFunc.Locals[recvIdx].Width
			}
			c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: recvIdx, Width: w})
		} else {
			c.emit(ir.Inst{Op: ir.OP_CONST_NIL})
		}
		c.emitStoreDeferredArg(recIdx, argIndex)
		argIndex++
	}
	if selectorRecv != nil {
		c.compileExpr(selectorRecv)
		c.emitStoreDeferredArg(recIdx, argIndex)
		argIndex++
	}
	fixedCallArgs := site.fixedCount
	if localFuncTarget != "" {
		fixedCallArgs -= len(captureArgs)
	}
	if localMethodTarget != "" {
		fixedCallArgs--
	}
	// Concrete selector-method calls include receiver in fixedCount; interface
	// method calls do not.
	if selectorRecv != nil && site.callOp == ir.OP_CALL {
		fixedCallArgs--
	}
	if fixedCallArgs < 0 {
		fixedCallArgs = 0
	}
	callArgIndex := 0
	for _, arg := range call.Nodes {
		c.compileExpr(arg)
		if site.variadicIsIface && callArgIndex >= fixedCallArgs {
			if typeID := c.exprPrimitiveTypeID(arg); typeID > 0 {
				c.emit(ir.Inst{Op: ir.OP_IFACE_BOX, Arg: typeID})
			}
		}
		c.emitStoreDeferredArg(recIdx, argIndex)
		argIndex++
		callArgIndex++
	}

	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: recIdx})
	c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: c.deferHeadLocal})
}

func (c *Compiler) markDeferRecoverWrapTarget(callOp ir.Opcode, callName string) {
	if c == nil || callName == "" {
		return
	}
	if c.deferRecoverWrapFuncs == nil {
		c.deferRecoverWrapFuncs = make(map[string]bool)
	}
	if callOp == ir.OP_CALL {
		c.deferRecoverWrapFuncs[callName] = true
		return
	}
	if callOp != ir.OP_IFACE_CALL {
		return
	}
	dot := lastIndexByteInString(callName, '.')
	if dot < 0 || dot+1 >= len(callName) {
		return
	}
	methodName := callName[dot+1:]
	for key, resolved := range c.methodTable {
		kdot := lastIndexByteInString(key, '.')
		if kdot < 0 || kdot+1 >= len(key) {
			continue
		}
		if key[kdot+1:] != methodName {
			continue
		}
		c.deferRecoverWrapFuncs[resolved] = true
	}
}

func lastIndexByteInString(s string, ch byte) int {
	i := len(s) - 1
	for i >= 0 {
		if s[i] == ch {
			return i
		}
		i--
	}
	return -1
}

func (c *Compiler) emitStoreDeferredArg(recLocal int, argIndex int) {
	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: recLocal})
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(2*c.target.PtrSize + argIndex*c.target.PtrSize)})
	c.emit(ir.Inst{Op: ir.OP_ADD})
	c.emit(ir.Inst{Op: ir.OP_STORE, Arg: c.target.PtrSize})
}

func (c *Compiler) emitLoadDeferredArg(recLocal int, argIndex int) {
	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: recLocal})
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(2*c.target.PtrSize + argIndex*c.target.PtrSize)})
	c.emit(ir.Inst{Op: ir.OP_ADD})
	c.emit(ir.Inst{Op: ir.OP_LOAD, Arg: c.target.PtrSize})
}

func sharedVarDeclRHS(decls []*Node) (*Node, bool) {
	if len(decls) < 2 {
		return nil, false
	}
	rhs := decls[0].X
	if rhs == nil {
		return nil, false
	}
	for _, decl := range decls {
		if decl == nil || decl.Kind != NVarDecl || decl.X != rhs || decl.Type != nil {
			return nil, false
		}
	}
	return rhs, true
}

func (c *Compiler) assignStackValuesToLHS(lhsNodes []*Node, define bool) {
	i := len(lhsNodes) - 1
	for i >= 0 {
		lhs := lhsNodes[i]
		if define {
			idx := c.addLocal(lhs.Name)
			if ct, ok := c.localConcreteTypes[lhs.Name]; ok {
				c.maybeCloneArrayForTypeName(ct)
			}
			c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
		} else {
			c.maybeCloneArrayForLValue(lhs)
			c.compileLValueSet(lhs)
		}
		i = i - 1
	}
}

func (c *Compiler) lvalueTypeName(node *Node) string {
	if node == nil {
		return ""
	}
	switch node.Kind {
	case NIdent:
		if ct, ok := c.localConcreteTypes[node.Name]; ok {
			return ct
		}
		if c.curPkg != nil {
			qname := c.curPkg.QualName(node.Name)
			if ct, ok := c.globalConcreteTypes[qname]; ok {
				return ct
			}
		}
	case NSelectorExpr:
		return c.resolveExprType(node)
	}
	return ""
}

func (c *Compiler) maybeCloneArrayForTypeName(typeName string) {
	depth := arrayTypeNestingDepth(typeName)
	if depth > 0 {
		// nestedDepth = number of array layers below the current one.
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(depth - 1)})
		c.emitKnownCall("runtime.SliceCloneArray", 2, 1)
	}
}

func (c *Compiler) maybeCloneArrayForLValue(node *Node) {
	c.maybeCloneArrayForTypeName(c.lvalueTypeName(node))
}

func (c *Compiler) emitLocalZeroSet(localIdx int, width int) {
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
	c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: localIdx, Width: width})
}

func (c *Compiler) compileCompoundAssign(node *Node, op ir.Opcode) {
	if op == ir.OP_ADD || op == ir.OP_SUB || op == ir.OP_MUL || op == ir.OP_DIV || op == ir.OP_MOD || op == ir.OP_AND || op == ir.OP_OR || op == ir.OP_XOR || op == ir.OP_SHL || op == ir.OP_SHR {
		c.strictCheckPointerArithmetic("+", node.X, node.Y)
	}
	w := c.exprWidth(node.X)
	floatType := c.floatInstNameForTypeName(c.resolveExprType(node.X))
	c.compileLValueGet(node.X)
	c.compileExpr(node.Y)
	if floatType != "" {
		rhsFloatType := c.floatExprTypeName(node.Y)
		if rhsFloatType == "" || rhsFloatType != floatType {
			c.emitConvertForExpr(node.Y, floatType)
		}
	}
	inst := ir.Inst{Op: op, Width: w}
	if floatType != "" {
		inst.Name = floatType
	} else if (op == ir.OP_ADD || op == ir.OP_SUB || op == ir.OP_MUL || op == ir.OP_SHR || op == ir.OP_DIV || op == ir.OP_MOD) && c.isUnsignedExpr(node.X) {
		inst.Name = "unsigned"
	}
	c.emit(inst)
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

func (c *Compiler) setLocalMapMetadataFromMapType(name string, mapType *Node) {
	if mapType == nil || mapType.Kind != NMapType || name == "" {
		return
	}
	c.localMapVars[name] = c.mapKeyKind(mapType.X)
	if mapType.Y != nil {
		c.localMapValueTypes[name] = nodeTypeName(mapType.Y)
	}
}

func (c *Compiler) isShortDeclNameNew(scope map[string]int, name string) bool {
	if scope == nil || name == "" {
		return false
	}
	if _, exists := scope[name]; !exists {
		return true
	}
	return c.ifInitLeakedNames != nil && c.ifInitLeakedNames[name]
}

func (c *Compiler) bindLocalFuncValue(localName string, rhs *Node, localIdx int, width int, allowMethod bool) bool {
	if localName == "" {
		return false
	}
	if allowMethod && c.registerMethodValueBinding(localName, rhs, localIdx) {
		return true
	}
	if c.registerFuncValueBinding(localName, rhs) {
		c.emitLocalZeroSet(localIdx, width)
		return true
	}
	if rhs != nil && rhs.Kind == NFuncType && rhs.Body != nil {
		target := c.compileFuncLiteral(rhs)
		c.localFuncTargets[localName] = target
		c.bindFuncCaptures(localName, target)
		delete(c.localMethodTargets, localName)
		delete(c.localMethodRecv, localName)
		c.emitLocalZeroSet(localIdx, width)
		return true
	}
	return false
}

func (c *Compiler) compileVarDecl(node *Node) {
	if len(c.scopes) > 0 {
		scope := c.scopes[len(c.scopes)-1]
		if _, exists := scope[node.Name]; exists {
			c.errorf("%s: %s redeclared in this block", c.curFunc.Name, node.Name)
			return
		}
	}
	if node.Type != nil && node.Type.Kind == NIdent {
		tname := node.Type.Name
		if !isBuiltinTypeName(tname) {
			if _, ok := c.lookupCurrentTypeDecl(tname); !ok {
				c.errorf("%s: undefined type: %s", c.curFunc.Name, tname)
			}
		}
	}
	idx := c.addLocal(node.Name)
	if node.Type != nil && node.Type.Kind == NIdent {
		c.setLocalTypeFlags(idx, node.Type.Name)
	}
	// Track element size for slice/array variables
	if node.Type != nil && (node.Type.Kind == NSliceType || node.Type.Kind == NArrayType) {
		c.localElemSizes[node.Name] = c.sliceElemSize(node.Type)
	}
	// Track string-typed variables
	if node.Type != nil && node.Type.Kind == NIdent && node.Type.Name == "string" {
		c.localStringVars[node.Name] = true
	}
	// Track map-typed variables
	c.setLocalMapMetadataFromMapType(node.Name, node.Type)
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
		if c.bindLocalFuncValue(node.Name, node.X, idx, c.curFunc.Locals[idx].Width, false) {
			return
		}
		c.compileExpr(node.X)
		if node.Type != nil {
			c.maybeCloneArrayForTypeName(nodeTypeName(node.Type))
		}
		if node.Type == nil {
			if ct := c.exprConcreteType(node.X); ct != "" {
				c.localConcreteTypes[node.Name] = ct
				if ct == "string" {
					c.localStringVars[node.Name] = true
				}
				c.setLocalTypeFlags(idx, ct)
				if elemType, ok := splitBracketType(ct); ok {
					c.localElemSizes[node.Name] = c.typeElemSize(elemType)
				}
				c.maybeCloneArrayForTypeName(ct)
			}
		}
		if node.Type != nil {
			if c.isInterfaceTypeName(nodeTypeName(node.Type)) {
				c.maybeBoxValueForInterface(node.X)
			}
		}
		if node.Type != nil {
			c.maybeConvertArgForParamType(node.X, nodeTypeName(node.Type))
		}
		c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx, Width: c.curFunc.Locals[idx].Width})
	} else {
		// Fixed-size arrays are represented as slice-header handles with a fixed
		// len/cap and cloned on value copies.
		if node.Type != nil && node.Type.Kind == NArrayType {
			arrLen := c.evalConstExprWithIota(node.Type.Y, 0)
			if arrLen < 0 {
				arrLen = 0
			}
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: arrLen})
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(c.sliceElemSize(node.Type))})
			c.emitKnownCall("runtime.SliceMake", 2, 1)
			c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
			return
		}

		// Struct locals are represented as pointers to heap-allocated storage.
		// A zero-value struct var must still be addressable and non-nil.
		if node.Type != nil {
			rawTypeName := nodeTypeName(node.Type)
			typeName := c.qualifyTypeName(rawTypeName, "")
			typeNode, _ := c.lookupStructTypeNode(typeName)
			// Only value-struct locals get implicit storage. Pointer locals
			// (e.g. *Parser) must remain nil-zero by default.
			if typeNode != nil && (len(rawTypeName) == 0 || rawTypeName[0] != '*') {
				size := c.resolveStructSize(typeName)
				if size <= 0 {
					size = c.target.PtrSize
				}
				c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(size)})
				c.emitRuntimeAllocCall()
				c.emit(ir.Inst{Op: ir.OP_DUP})
				c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(size)})
				c.emit(makeInst(ir.OP_CALL, 2, 0, 0, "runtime.Memzero"))
				c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
				return
			}
		}
		// Zero-initialize the local to avoid stack garbage.
		if node.Type != nil {
			c.emitZeroValueForTypeName(nodeTypeName(node.Type))
		} else {
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		}
		c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx})
	}
}

func isBuiltinTypeName(t string) bool {
	switch t {
	case "bool",
		"int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64",
		"uintptr", "byte", "rune",
		"string", "error", "interface{}":
		return true
	}
	return false
}

func (c *Compiler) compileAssign(node *Node) {
	if len(node.Nodes) > 0 {
		isDefine := node.Name == ":="
		if isDefine && !c.inIfInit && len(c.scopes) > 0 {
			scope := c.scopes[len(c.scopes)-1]
			hasNew := false
			for _, lhs := range node.Nodes {
				if lhs == nil || lhs.Kind != NIdent || lhs.Name == "_" {
					continue
				}
				if c.isShortDeclNameNew(scope, lhs.Name) {
					hasNew = true
					break
				}
			}
			if !hasNew {
				c.errorf("%s: no new variables on left side of :=", c.curFunc.Name)
				return
			}
		}
		// Multi-value assignment with comma-separated RHS: a, b := 1, 2
		if node.Body != nil && node.Body.Kind == NBlock && len(node.Body.Nodes) > 0 {
			if len(node.Body.Nodes) != len(node.Nodes) {
				c.errorf("%s: assignment count mismatch: %d variables but %d values", c.curFunc.Name, len(node.Nodes), len(node.Body.Nodes))
				return
			}
			if isDefine {
				for i, rhs := range node.Body.Nodes {
					if i >= len(node.Nodes) || node.Nodes[i] == nil || node.Nodes[i].Kind != NIdent {
						continue
					}
					if ct := c.exprConcreteType(rhs); ct != "" {
						c.localConcreteTypes[node.Nodes[i].Name] = ct
						if elemType, ok := splitBracketType(ct); ok {
							c.localElemSizes[node.Nodes[i].Name] = c.typeElemSize(elemType)
						}
					}
				}
			}
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
			c.emitKnownCall("runtime.MapGet", 2, 2)
			// MapGet returns (value, ok) — both on stack
			// Assign in reverse order: ok first (top of stack), then value
			c.assignStackValuesToLHS(node.Nodes, isDefine)
			// Track concrete type of the map value variable (node.Nodes[0])
			if isDefine && len(node.Nodes) >= 1 {
				valType := c.resolveMapValueType(node.Y.X)
				if valType != "" {
					c.localConcreteTypes[node.Nodes[0].Name] = c.qualifyTypeName(valType, "")
					if idx, ok := c.lookupLocal(node.Nodes[0].Name); ok {
						c.setLocalTypeFlags(idx, valType)
					}
				}
			}
			return
		}

		// Multi-value type assertion: v, ok := x.(T)
		if node.Y != nil && node.Y.Kind == NTypeAssertExpr && len(node.Nodes) == 2 {
			c.compileTypeAssertCommaOk(node.Y)
			c.assignStackValuesToLHS(node.Nodes, isDefine)
			if isDefine && len(node.Nodes) > 0 {
				assertedType := c.qualifiedTypeFromTypeNode(node.Y.Type, "")
				if assertedType != "" && node.Nodes[0] != nil && node.Nodes[0].Kind == NIdent {
					lhsName := node.Nodes[0].Name
					c.localConcreteTypes[lhsName] = assertedType
					if c.isInterfaceTypeName(assertedType) {
						c.localTypes[lhsName] = assertedType
					}
					if idx, ok := c.lookupLocal(lhsName); ok {
						c.setLocalTypeFlags(idx, assertedType)
					}
					if elemType, ok := splitBracketType(assertedType); ok {
						c.localElemSizes[lhsName] = c.typeElemSize(elemType)
					}
				}
				if len(node.Nodes) > 1 && node.Nodes[1] != nil && node.Nodes[1].Kind == NIdent {
					c.localConcreteTypes[node.Nodes[1].Name] = "bool"
				}
			}
			return
		}

		// Multi-value assignment: a, b = expr or a, b := expr
		if node.Y != nil && c.exprReturnCount(node.Y) != len(node.Nodes) {
			c.errorf("%s: assignment count mismatch: %d variables but %d values", c.curFunc.Name, len(node.Nodes), c.exprReturnCount(node.Y))
			return
		}
		c.compileExpr(node.Y)

		var callRetTypes []string
		// Track interface-typed, string-typed, and concrete-typed locals from multi-value := assignments
		if isDefine && node.Y != nil && node.Y.Kind == NCallExpr {
			if retTypes, ok := c.callReturnTypes(node.Y); ok {
				callRetTypes = retTypes
				for j, lhs := range node.Nodes {
					if j >= len(retTypes) || lhs == nil || lhs.Kind != NIdent {
						continue
					}
					qret := retTypes[j]
					if c.isInterfaceTypeName(qret) {
						c.localTypes[lhs.Name] = qret
					}
					if qret == "string" {
						c.localStringVars[lhs.Name] = true
					}
					if elemType, ok := splitBracketType(qret); ok {
						c.localElemSizes[lhs.Name] = c.typeElemSize(elemType)
					}
					c.setLocalMapMetadataFromQualified(lhs.Name, qret)
					// Track concrete type for method resolution
					c.localConcreteTypes[lhs.Name] = qret
				}
			}
		}

		// Assign to each LHS in reverse order (values are on stack)
		c.assignStackValuesToLHS(node.Nodes, isDefine)
		if isDefine && len(callRetTypes) > 0 {
			for j, lhs := range node.Nodes {
				if j >= len(callRetTypes) || lhs == nil || lhs.Kind != NIdent {
					continue
				}
				if idx, found := c.lookupLocal(lhs.Name); found {
					c.setLocalTypeFlags(idx, callRetTypes[j])
				}
			}
		}
		return
	}

	if node.Name == ":=" {
		if !c.inIfInit && len(c.scopes) > 0 {
			scope := c.scopes[len(c.scopes)-1]
			if idx, exists := scope[node.X.Name]; exists {
				isCommaOkForm := node.Y != nil && ((node.Y.Kind == NIndexExpr && c.isMapExpr(node.Y.X)) || node.Y.Kind == NTypeAssertExpr)
				isParam := idx >= 0 && idx < c.curFunc.Params
				if !isParam && !isCommaOkForm && !c.isShortDeclNameNew(scope, node.X.Name) {
					c.errorf("%s: no new variables on left side of :=", c.curFunc.Name)
					return
				}
			}
		}
		if node.Y != nil && c.exprReturnCount(node.Y) != 1 {
			c.errorf("%s: assignment count mismatch: 1 variable but %d values", c.curFunc.Name, c.exprReturnCount(node.Y))
			return
		}
		// Short var decl
		idx := c.addLocal(node.X.Name)
		w := c.exprWidth(node.Y)
		if w != 0 {
			c.curFunc.Locals[idx].Width = w
			c.curFunc.Locals[idx].Is64 = false
			c.curFunc.Locals[idx].IsFloat64 = false
			c.curFunc.Locals[idx].FloatKind = ir.TY_VOID
			switch c.resolvedFloatInstName(node.Y) {
			case "float32":
				c.curFunc.Locals[idx].FloatKind = ir.TY_FLOAT32
			case "float64":
				c.curFunc.Locals[idx].FloatKind = ir.TY_FLOAT64
				c.curFunc.Locals[idx].IsFloat64 = true
			}
			if w == 8 && c.curFunc.Locals[idx].FloatKind == ir.TY_VOID {
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
			c.setLocalTypeFlags(idx, ct)
			if ct == "string" {
				c.localStringVars[node.X.Name] = true
			}
			if c.isInterfaceTypeName(ct) {
				c.localTypes[node.X.Name] = ct
			}
			// Track slice/array elem sizes
			if elemType, ok := splitBracketType(ct); ok {
				c.localElemSizes[node.X.Name] = c.typeElemSize(elemType)
			}
			// Track map variables from concrete return type
			c.setLocalMapMetadataFromQualified(node.X.Name, ct)
		}
		if node.Y != nil {
			if node.Y.Kind == NIntLit || node.Y.Kind == NRuneLit {
				c.localConcreteTypes[node.X.Name] = "int"
			} else if node.Y.Kind == NFloatLit {
				c.localConcreteTypes[node.X.Name] = "float64"
			} else if node.Y.Kind == NStringLit {
				c.localConcreteTypes[node.X.Name] = "string"
			} else if node.Y.Kind == NBasicLit && (node.Y.Name == "true" || node.Y.Name == "false") {
				c.localConcreteTypes[node.X.Name] = "bool"
			}
		}
		if node.Y != nil && node.Y.Kind == NTypeAssertExpr {
			if rt := c.resolveExprType(node.Y); rt != "" {
				c.localConcreteTypes[node.X.Name] = rt
				if c.isInterfaceTypeName(rt) {
					c.localTypes[node.X.Name] = rt
				}
				if elemType, ok := splitBracketType(rt); ok {
					c.localElemSizes[node.X.Name] = c.typeElemSize(elemType)
				}
				c.setLocalMapMetadataFromQualified(node.X.Name, rt)
				c.setLocalTypeFlags(idx, rt)
			}
		}
		// Track map variables from composite literals: m := map[K]V{...}
		if node.Y != nil && node.Y.Kind == NCompositeLit {
			c.setLocalMapMetadataFromMapType(node.X.Name, node.Y.Type)
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
				c.setLocalMapMetadataFromMapType(node.X.Name, node.Y.Nodes[0])
				if node.Y.Nodes[0].Y != nil {
					valType := nodeTypeName(node.Y.Nodes[0].Y)
					c.localConcreteTypes[node.X.Name] = "map[" + c.qualifyTypeName(keyType, "") + "]" + c.qualifyTypeName(valType, "")
				}
			}
		}
		// Function literals are lowered to private named functions and bound
		// through localFuncTargets; the local slot stores a placeholder value.
		if c.bindLocalFuncValue(node.X.Name, node.Y, idx, w, true) {
			return
		}
		c.compileExpr(node.Y)
		if lhsType := c.resolveExprType(node.X); isFloatTypeName(lhsType) {
			c.maybeConvertArgForParamType(node.Y, lhsType)
		}
		if ct, ok := c.localConcreteTypes[node.X.Name]; ok {
			c.maybeCloneArrayForTypeName(ct)
		}
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
		mapValueType := c.resolveMapValueType(node.X.X)
		mapValueTypeQualified := c.qualifyTypeName(mapValueType, "")
		mapValueIsInterface := c.isInterfaceTypeName(mapValueType) || c.isInterfaceTypeName(mapValueTypeQualified)
		c.compileExpr(node.X.X) // push map
		c.compileExpr(node.X.Y) // push key
		c.compileExpr(node.Y)   // push value
		if isFloatTypeName(mapValueTypeQualified) || isFloatTypeName(mapValueType) {
			c.maybeConvertArgForParamType(node.Y, mapValueTypeQualified)
		}
		if mapValueIsInterface {
			c.maybeBoxValueForInterface(node.Y)
		}
		c.emitKnownCall("runtime.MapSet", 3, 1)
		c.emit(ir.Inst{Op: ir.OP_DROP}) // discard returned header (unchanged)
		return
	}

	// Regular assignment
	if node.Y != nil && c.exprReturnCount(node.Y) != 1 {
		c.errorf("%s: assignment count mismatch: 1 variable but %d values", c.curFunc.Name, c.exprReturnCount(node.Y))
		return
	}
	c.compileExpr(node.Y)
	if lhsType := c.resolveExprType(node.X); isFloatTypeName(lhsType) {
		c.maybeConvertArgForParamType(node.Y, lhsType)
	}
	if _, ok := c.lvalueInterfaceType(node.X); ok {
		c.maybeBoxValueForInterface(node.Y)
	}
	c.maybeCloneArrayForLValue(node.X)
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
			spec := closureCaptureSpec{Name: name, Width: localWidth(c.curFunc.Locals, idx)}
			if concreteType, ok := c.localConcreteTypes[name]; ok {
				spec.ConcreteType = concreteType
			}
			if ifaceType, ok := c.localTypes[name]; ok {
				spec.InterfaceType = ifaceType
			}
			captures = append(captures, spec)
			seen[name] = true
		}
	}
	return captures
}

func (c *Compiler) compileFuncLiteral(lit *Node) string {
	captures := c.collectFuncLiteralCaptures(lit)
	return c.compileFuncLiteralWithCaptures(lit, captures)
}

func (c *Compiler) compileFuncLiteralNoCapture(lit *Node) string {
	return c.compileFuncLiteralWithCaptures(lit, nil)
}

func (c *Compiler) compileFuncLiteralWithCaptures(lit *Node, captures []closureCaptureSpec) string {
	name := fmt.Sprintf("$lit_%d", c.funcLitSeq)
	c.funcLitSeq++

	params := make([]*Node, 0, len(captures)+len(lit.Nodes))
	activeCaptures := make(map[string]closureCaptureBinding)
	captureConcreteTypes := make(map[string]string)
	captureIfaceTypes := make(map[string]string)
	for i, capture := range captures {
		pname := "$cap_" + capture.Name
		params = append(params, &Node{Kind: NVarDecl, Name: pname})
		activeCaptures[capture.Name] = closureCaptureBinding{LocalIdx: i, Width: capture.Width, IsPtr: true}
		if capture.ConcreteType != "" {
			captureConcreteTypes[capture.Name] = capture.ConcreteType
		}
		if capture.InterfaceType != "" {
			captureIfaceTypes[capture.Name] = capture.InterfaceType
		}
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
	savedDeferSites := c.deferSites
	savedDeferHeadLocal := c.deferHeadLocal
	savedPanicUnwindLabel := c.panicUnwindLabel
	savedPanicCheckSlowLabels := c.panicCheckSlowLabels
	savedPanicCheckSlowDepths := c.panicCheckSlowDepths
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
	savedCaptureConcreteTypes := c.captureConcreteTypes
	savedCaptureIfaceTypes := c.captureIfaceTypes
	savedProfileStartLocal := c.profileStartLocal
	savedProfileParentLocal := c.profileParentLocal
	savedProfileMethodHash := c.profileMethodHash
	savedProfileFlushOnExit := c.profileFlushOnExit
	savedCurrentMethodHash := c.currentMethodHash
	savedInIfInit := c.inIfInit
	savedIfInitLeakedNames := c.ifInitLeakedNames

	c.activeCaptures = activeCaptures
	c.captureConcreteTypes = captureConcreteTypes
	c.captureIfaceTypes = captureIfaceTypes
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
	c.deferSites = savedDeferSites
	c.deferHeadLocal = savedDeferHeadLocal
	c.panicUnwindLabel = savedPanicUnwindLabel
	c.panicCheckSlowLabels = savedPanicCheckSlowLabels
	c.panicCheckSlowDepths = savedPanicCheckSlowDepths
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
	c.captureConcreteTypes = savedCaptureConcreteTypes
	c.captureIfaceTypes = savedCaptureIfaceTypes
	c.profileStartLocal = savedProfileStartLocal
	c.profileParentLocal = savedProfileParentLocal
	c.profileMethodHash = savedProfileMethodHash
	c.profileFlushOnExit = savedProfileFlushOnExit
	c.currentMethodHash = savedCurrentMethodHash
	c.inIfInit = savedInIfInit
	c.ifInitLeakedNames = savedIfInitLeakedNames
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
			} else {
				c.errorf("%s: undefined: %s", c.curFunc.Name, node.Name)
				c.emit(ir.Inst{Op: ir.OP_DROP})
			}
		}
	case NIndexExpr:
		elemSize := c.exprElemSize(node.X)
		c.compileExpr(node.X)
		c.compileExpr(node.Y)
		c.emit(ir.Inst{Op: ir.OP_INDEX_ADDR, Arg: elemSize})
		inst := ir.Inst{Op: ir.OP_STORE, Arg: elemSize}
		inst.Name = floatInstName(c.resolveExprType(node))
		c.emit(inst)
	case NSelectorExpr:
		if node.X != nil && node.X.Kind == NIdent {
			pkg := c.resolvePackage(node.X.Name)
			if pkg != nil {
				qname := pkg.QualName(node.Name)
				gidx, ok := c.globals[qname]
				if ok {
					c.emit(ir.Inst{Op: ir.OP_GLOBAL_SET, Arg: gidx})
				} else {
					c.emit(makeInst(ir.OP_GLOBAL_SET, 0, 0, 0, qname))
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
		fieldType := c.resolveFieldType(recvType, node.Name)
		inst := ir.Inst{Op: ir.OP_STORE, Arg: c.storageSizeForTypeName(fieldType)}
		inst.Name = c.floatInstNameForTypeName(fieldType)
		c.emit(inst)
	case NUnaryExpr:
		if node.Name == "*" {
			c.compileExpr(node.X)
			inst := ir.Inst{Op: ir.OP_STORE, Arg: c.exprWidth(node)}
			inst.Name = floatInstName(c.resolveExprType(node))
			c.emit(inst)
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
	if len(c.deferSites) == 0 || c.deferHeadLocal < 0 {
		return
	}

	recIdx := c.addLocal("$defer_pop_rec")
	siteIdx := c.addLocal("$defer_pop_site")
	loopLabel := c.newLabel()
	doneLabel := c.newLabel()

	c.emitLabel(loopLabel)
	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: c.deferHeadLocal})
	c.emit(ir.Inst{Op: ir.OP_JMP_IF_NOT, Arg: doneLabel})

	// rec = head; head = rec.next
	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: c.deferHeadLocal})
	c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: recIdx})
	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: recIdx})
	c.emit(ir.Inst{Op: ir.OP_LOAD, Arg: c.target.PtrSize})
	c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: c.deferHeadLocal})

	// site = rec.siteID
	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: recIdx})
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(c.target.PtrSize)})
	c.emit(ir.Inst{Op: ir.OP_ADD})
	c.emit(ir.Inst{Op: ir.OP_LOAD, Arg: c.target.PtrSize})
	c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: siteIdx})

	for idx, site := range c.deferSites {
		nextCase := c.newLabel()
		c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: siteIdx})
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(idx)})
		c.emit(ir.Inst{Op: ir.OP_JMP_NEQ, Arg: nextCase})
		c.emitDeferredSiteCall(site, recIdx)
		c.emit(ir.Inst{Op: ir.OP_JMP, Arg: loopLabel})
		c.emitLabel(nextCase)
	}
	c.emit(ir.Inst{Op: ir.OP_JMP, Arg: loopLabel})
	c.emitLabel(doneLabel)
}

func (c *Compiler) emitDeferredSiteCall(site deferSite, recIdx int) {
	argCount := site.argCount
	c.emit(makeInst(ir.OP_CALL, 0, 0, 0, "runtime.DeferRecoverEnter"))
	if site.isVariadic {
		fixedCount := site.fixedCount
		if fixedCount > argCount {
			fixedCount = argCount
		}
		k := 0
		for k < fixedCount {
			c.emitLoadDeferredArg(recIdx, k)
			k++
		}
		variadicCount := argCount - fixedCount
		if variadicCount <= 0 {
			c.emit(ir.Inst{Op: ir.OP_CONST_NIL})
		} else {
			varElemSz := site.variadicElemSz
			sliceHdrSize := 4 * c.target.PtrSize
			allocSize := sliceHdrSize + variadicCount*varElemSz
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(allocSize)})
			c.emitRuntimeAllocCall()
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
				c.emitLoadDeferredArg(recIdx, fixedCount+j)
				c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: tmpIdx})
				c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(sliceHdrSize + j*varElemSz)})
				c.emit(ir.Inst{Op: ir.OP_ADD})
				c.emit(ir.Inst{Op: ir.OP_STORE, Arg: varElemSz})
				j++
			}
			c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: tmpIdx})
		}
		switch site.callOp {
		case ir.OP_IFACE_CALL:
			c.emit(makeInst(ir.OP_IFACE_CALL, fixedCount, 0, 0, site.callName))
		default:
			c.emit(makeInst(ir.OP_CALL, fixedCount+1, 0, 0, site.callName))
		}
	} else {
		k := 0
		for k < argCount {
			c.emitLoadDeferredArg(recIdx, k)
			k++
		}
		switch site.callOp {
		case ir.OP_IFACE_CALL:
			if argCount > 0 {
				c.emit(makeInst(ir.OP_IFACE_CALL, argCount-1, 0, 0, site.callName))
			} else {
				c.emit(makeInst(ir.OP_IFACE_CALL, 0, 0, 0, site.callName))
			}
		default:
			c.emit(makeInst(ir.OP_CALL, argCount, 0, 0, site.callName))
		}
	}
	c.emit(makeInst(ir.OP_CALL, 0, 0, 0, "runtime.DeferRecoverExit"))
	dropCount := site.retCount
	for dropCount > 0 {
		c.emit(ir.Inst{Op: ir.OP_DROP})
		dropCount--
	}
}

func (c *Compiler) emitNamedReturnValues(retTypes []string) int {
	count := 0
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
		if i < len(retTypes) {
			c.maybeCloneArrayForTypeName(retTypes[i])
		}
		if i < len(retTypes) && c.isInterfaceTypeName(retTypes[i]) {
			if typeID := c.typeIDForTypeName(c.localConcreteTypes[name]); typeID > 0 {
				c.emit(ir.Inst{Op: ir.OP_IFACE_BOX, Arg: typeID})
			}
		}
		count++
	}
	return count
}

func (c *Compiler) emitProfileExit() {
	if c.profileStartLocal >= 0 {
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(c.profileMethodHash)})
		if c.profileParentLocal >= 0 {
			c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: c.profileParentLocal})
		} else {
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		}
		c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: c.profileStartLocal})
		c.emitKnownCall("runtime.ProfileHashNow", 3, 0)
		if c.target != nil && c.target.Profile {
			c.emitKnownCall("runtime.ArenaLeave", 0, 0)
		}
	}
}

func (c *Compiler) emitProfileAllocSample() {
	if c.target == nil || !c.target.Profile {
		return
	}
	if c.profileStartLocal < 0 {
		return
	}
	if c.profileMethodHash == 0 {
		return
	}
	// Stack before: [..., size]
	// Stack after:  [..., size]
	// ProfileAllocHash signature is (size, methodHash, parentHash).
	c.emit(ir.Inst{Op: ir.OP_DUP})
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(c.profileMethodHash)})
	if c.profileParentLocal >= 0 {
		c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: c.profileParentLocal})
	} else {
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
	}
	c.emit(makeInst(ir.OP_CALL, 3, 0, 0, "runtime.ProfileAllocHash"))
}

func (c *Compiler) emitRuntimeAllocCall() {
	c.emitProfileAllocSample()
	c.emitKnownCall("runtime.Alloc", 1, 1)
}

func profileHash32FNV(name string) uint32 {
	var h uint32 = (uint32(0x811c) << 16) | uint32(0x9dc5)
	i := 0
	for i < len(name) {
		h = h ^ uint32(name[i])
		h = h * 16777619
		i++
	}
	return h
}

func (c *Compiler) resetPanicPropagationOutlineState() {
	c.panicCheckSlowLabels = nil
	c.panicCheckSlowDepths = nil
}

func (c *Compiler) panicPropagationSlowLabel(depth int) int {
	if c.panicCheckSlowLabels == nil {
		c.panicCheckSlowLabels = make(map[int]int)
	}
	if label, ok := c.panicCheckSlowLabels[depth]; ok {
		return label
	}
	label := c.newLabel()
	c.panicCheckSlowLabels[depth] = label
	c.panicCheckSlowDepths = append(c.panicCheckSlowDepths, depth)
	return label
}

func (c *Compiler) emitPanicPropagationSlowPaths() {
	if c.panicUnwindLabel < 0 || len(c.panicCheckSlowDepths) == 0 {
		return
	}
	for _, depth := range c.panicCheckSlowDepths {
		label, ok := c.panicCheckSlowLabels[depth]
		if !ok {
			continue
		}
		c.stackDepth = depth
		c.emitLabel(label)
		dropCount := depth
		for dropCount > 0 {
			c.emit(ir.Inst{Op: ir.OP_DROP})
			dropCount--
		}
		c.emit(ir.Inst{Op: ir.OP_JMP, Arg: c.panicUnwindLabel})
	}
	c.stackDepth = 0
}

func (c *Compiler) emitProfileFinalize() {
	if !c.profileFlushOnExit {
		return
	}
	c.emitKnownCall("runtime.ProfileFlush", 0, 0)
	c.emitKnownCall("runtime.ArenaFlush", 0, 0)
}

func (c *Compiler) emitRecoveredPanicReturn() {
	retTypes := c.funcRetTypes[c.curFunc.Name]
	count := 0
	if len(c.namedResultNames) > 0 {
		count = c.emitNamedReturnValues(retTypes)
	} else {
		want := c.curFunc.RetCount
		i := 0
		for i < want {
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			count++
			i++
		}
	}
	c.emitProfileExit()
	c.emitProfileFinalize()
	c.emit(ir.Inst{Op: ir.OP_RETURN, Arg: count})
}

func (c *Compiler) emitPanicPropagationCheck(_ int) {
	if c.panicUnwindLabel < 0 {
		return
	}
	savedDepth := c.stackDepth
	c.emitKnownCall("runtime.PanicShouldUnwind", 0, 1)
	if c.target != nil && c.target.GOOS == "wasi" && c.target.GOARCH == "wasm32" {
		// The wasm stackifier still depends on the original inline panic-unwind
		// shape here; outlined slow labels can leave branch targets with the
		// wrong stack state during validation.
		continueLabel := c.newLabel()
		c.emit(ir.Inst{Op: ir.OP_JMP_IF_NOT, Arg: continueLabel})
		dropCount := savedDepth
		for dropCount > 0 {
			c.emit(ir.Inst{Op: ir.OP_DROP})
			dropCount--
		}
		c.emit(ir.Inst{Op: ir.OP_JMP, Arg: c.panicUnwindLabel})
		c.stackDepth = savedDepth
		c.emitLabel(continueLabel)
		return
	}
	slowLabel := c.panicUnwindLabel
	if savedDepth > 0 {
		// Reuse one outlined slow path per transient stack depth so callsites
		// only need a single conditional branch on the unwind result.
		slowLabel = c.panicPropagationSlowLabel(savedDepth)
	}
	c.emit(ir.Inst{Op: ir.OP_JMP_IF, Arg: slowLabel})
}

func (c *Compiler) emitCallWithPanicCheck(callName string, argCount int) {
	if callName == "runtime.Alloc" && argCount == 1 {
		c.emitProfileAllocSample()
	}
	retCount := c.resolvedCallRetCount(callName)
	c.curFunc.Code = append(c.curFunc.Code, makeInst(ir.OP_CALL, argCount, 0, 0, callName))
	c.stackDepth = c.stackDepth - argCount + retCount
	c.emitPanicPropagationCheck(retCount)
}

func (c *Compiler) emitIfaceCallWithPanicCheck(callName string, argCount int, retCount int) {
	c.emit(makeInst(ir.OP_IFACE_CALL, argCount, 0, 0, callName))
	c.emitPanicPropagationCheck(retCount)
}

func (c *Compiler) compileReturn(node *Node) {
	count := 0
	retTypes := c.funcRetTypes[c.curFunc.Name]
	expectedCount := c.funcRets[c.curFunc.Name]
	bareReturn := node.X == nil && len(node.Nodes) == 0
	explicitCount := 0
	if node.X != nil {
		explicitCount += c.exprReturnCount(node.X)
	}
	for _, extra := range node.Nodes {
		explicitCount += c.exprReturnCount(extra)
	}
	if bareReturn && len(c.namedResultNames) == 0 && expectedCount > 0 {
		c.errorf("%s: not enough return values: got 0, want %d", c.curFunc.Name, expectedCount)
	}
	if !bareReturn && explicitCount != expectedCount {
		c.errorf("%s: wrong number of return values: got %d, want %d", c.curFunc.Name, explicitCount, expectedCount)
	}

	if len(c.namedResultNames) > 0 {
		if !bareReturn {
			retExprs := make([]*Node, 0, 1+len(node.Nodes))
			if node.X != nil {
				retExprs = append(retExprs, node.X)
			}
			retExprs = append(retExprs, node.Nodes...)
			if len(retExprs) == len(c.namedResultNames) && len(retExprs) == expectedCount {
				for i, retExpr := range retExprs {
					c.compileExpr(retExpr)
					if i < len(retTypes) {
						c.maybeCloneArrayForTypeName(retTypes[i])
					}
					c.maybeBoxInterface(retExpr, retTypes, i)
				}
				i := len(retExprs) - 1
				for i >= 0 {
					name := c.namedResultNames[i]
					idx, ok := c.lookupLocal(name)
					if ok {
						w := 0
						if idx < len(c.curFunc.Locals) {
							w = c.curFunc.Locals[idx].Width
						}
						c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: idx, Width: w})
					} else {
						c.emit(ir.Inst{Op: ir.OP_DROP})
					}
					i--
				}
			} else {
				if node.X != nil {
					retIdx := count
					c.compileExpr(node.X)
					if retIdx < len(retTypes) {
						c.maybeCloneArrayForTypeName(retTypes[retIdx])
					}
					c.maybeBoxInterface(node.X, retTypes, retIdx)
					count++
				}
				for _, extra := range node.Nodes {
					retIdx := count
					c.compileExpr(extra)
					if retIdx < len(retTypes) {
						c.maybeCloneArrayForTypeName(retTypes[retIdx])
					}
					c.maybeBoxInterface(extra, retTypes, retIdx)
					count++
				}
				if len(c.deferSites) > 0 {
					c.emitDeferredCalls()
				}
				c.emitProfileExit()
				c.emitProfileFinalize()
				c.emit(ir.Inst{Op: ir.OP_RETURN, Arg: count})
				return
			}
		}
		if len(c.deferSites) > 0 {
			c.emitDeferredCalls()
		}
		count = c.emitNamedReturnValues(retTypes)
		c.emitProfileExit()
		c.emitProfileFinalize()
		c.emit(ir.Inst{Op: ir.OP_RETURN, Arg: count})
		return
	}

	if node.X != nil {
		retIdx := count
		c.compileExpr(node.X)
		if retIdx < len(retTypes) {
			c.maybeCloneArrayForTypeName(retTypes[retIdx])
		}
		c.maybeBoxInterface(node.X, retTypes, retIdx)
		count++
	}
	for _, extra := range node.Nodes {
		retIdx := count
		c.compileExpr(extra)
		if retIdx < len(retTypes) {
			c.maybeCloneArrayForTypeName(retTypes[retIdx])
		}
		c.maybeBoxInterface(extra, retTypes, retIdx)
		count++
	}
	if len(c.deferSites) > 0 {
		c.emitDeferredCalls()
	}
	c.emitProfileExit()
	c.emitProfileFinalize()
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
	if _, ok := splitBracketType(t); ok {
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
	if expr.Kind == NFloatLit {
		return "float64"
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
		if expr.X != nil && expr.X.Kind == NIdent {
			switch expr.X.Name {
			case "int", "uintptr", "uint", "byte", "int8", "uint8", "int16", "int32", "int64", "uint16", "uint32", "uint64", "float32", "float64":
				return expr.X.Name
			}
		}
		// append returns the same slice type as its first argument
		if expr.X != nil && expr.X.Kind == NIdent && expr.X.Name == "append" && len(expr.Nodes) > 0 {
			return c.exprConcreteType(expr.Nodes[0])
		}
		if expr.X != nil && expr.X.Kind == NSelectorExpr && expr.X.X != nil {
			if ifaceType := c.resolveExprType(expr.X.X); ifaceType != "" {
				if retType, ok := c.ifaceMethodFirstReturnType(ifaceType, expr.X.Name); ok {
					return retType
				}
			}
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
		// Fallback: read return type directly from function declarations when
		// funcRetTypes has not yet been populated for this callee.
		if expr.X != nil && expr.X.Kind == NSelectorExpr && expr.X.X != nil && expr.X.X.Kind == NIdent {
			pkg := c.resolvePackage(expr.X.X.Name)
			if pkg != nil {
				if sym, ok := pkg.Symbols[expr.X.Name]; ok && sym.Kind == SymFunc && sym.Node != nil && sym.Node.Type != nil {
					if sym.Node.Type.Kind == NFuncType && len(sym.Node.Type.Nodes) > 0 {
						return c.qualifyTypeName(nodeTypeName(sym.Node.Type.Nodes[0]), pkg.Path)
					}
					return c.qualifyTypeName(nodeTypeName(sym.Node.Type), pkg.Path)
				}
			}
		}
		if expr.X != nil && expr.X.Kind == NIdent {
			if sym, ok := c.curPkg.Symbols[expr.X.Name]; ok && sym.Kind == SymFunc && sym.Node != nil && sym.Node.Type != nil {
				if sym.Node.Type.Kind == NFuncType && len(sym.Node.Type.Nodes) > 0 {
					return c.qualifyTypeName(nodeTypeName(sym.Node.Type.Nodes[0]), c.curPkg.Path)
				}
				return c.qualifyTypeName(nodeTypeName(sym.Node.Type), c.curPkg.Path)
			}
		}
	}
	// Variable reference: check localConcreteTypes
	if expr.Kind == NIdent {
		if sym, ok := c.curPkg.Symbols[expr.Name]; ok && sym.Kind == SymConst && sym.Node != nil && c.isConstFloatExpr(sym.Node.X) {
			return c.exprConcreteType(sym.Node.X)
		}
		if ct, ok := c.localConcreteTypes[expr.Name]; ok {
			return ct
		}
		qname := c.curPkg.QualName(expr.Name)
		if ct, ok := c.globalConcreteTypes[qname]; ok {
			return ct
		}
		if sym, ok := c.curPkg.Symbols[expr.Name]; ok && sym.Kind == SymVar && sym.Node != nil {
			if sym.Node.Type != nil {
				t := c.qualifyTypeName(nodeTypeName(sym.Node.Type), c.curPkg.Path)
				if t != "" {
					c.globalConcreteTypes[qname] = t
					return t
				}
			}
			if sym.Node.X != nil && !(sym.Node.X.Kind == NIdent && sym.Node.X.Name == expr.Name) {
				if t := c.exprConcreteType(sym.Node.X); t != "" {
					c.globalConcreteTypes[qname] = t
					return t
				}
				if t := c.resolveExprType(sym.Node.X); t != "" {
					c.globalConcreteTypes[qname] = t
					return t
				}
			}
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
	// Basic arithmetic/bitwise operations preserve operand type.
	if expr.Kind == NBinaryExpr {
		switch expr.Name {
		case "==", "!=", "<", ">", "<=", ">=", "&&", "||":
			return "bool"
		case "+", "-", "*", "/", "%", "&", "|", "^", "<<", ">>":
			left := c.exprConcreteType(expr.X)
			if left != "" {
				return left
			}
			return c.exprConcreteType(expr.Y)
		}
	}
	return ""
}

func (c *Compiler) compileIf(node *Node) {
	elseLabel := c.newLabel()
	endLabel := c.newLabel()
	var initScopeNames []string

	// Compile init statement if present (e.g. if x, ok := m[k]; ok { ... })
	if len(node.Nodes) > 0 {
		existing := make(map[string]bool)
		if len(c.scopes) > 0 {
			scope := c.scopes[len(c.scopes)-1]
			for name := range scope {
				existing[name] = true
			}
		}
		savedInIfInit := c.inIfInit
		c.inIfInit = true
		c.compileStmt(node.Nodes[0])
		c.inIfInit = savedInIfInit
		if len(c.scopes) > 0 {
			scope := c.scopes[len(c.scopes)-1]
			for name := range scope {
				if !existing[name] {
					initScopeNames = append(initScopeNames, name)
				}
			}
		}
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
	if len(c.scopes) > 0 {
		scope := c.scopes[len(c.scopes)-1]
		for _, name := range initScopeNames {
			delete(scope, name)
		}
	}
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
	isFloat := c.isFloatExpr(node.X) || c.isFloatExpr(node.Y)
	floatType := ""
	if isFloat {
		floatType = mergeFloatTypeNames(c.floatExprTypeName(node.X), c.floatExprTypeName(node.Y))
	}
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
	if floatType != "" {
		leftFloatType := c.floatExprTypeName(node.X)
		if leftFloatType == "" || leftFloatType != floatType {
			c.emitConvertForExpr(node.X, floatType)
		}
	}
	c.compileExpr(node.Y)
	if floatType != "" {
		rightFloatType := c.floatExprTypeName(node.Y)
		if rightFloatType == "" || rightFloatType != floatType {
			c.emitConvertForExpr(node.Y, floatType)
		}
	}
	inst := ir.Inst{Op: irOp, Arg: targetLabel, Width: c.exprWidth(node)}
	if floatType != "" {
		inst.Name = floatType
	} else if c.isUnsignedComparison(node) {
		inst.Name = "unsigned"
	}
	c.emit(inst)
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
	if c.isDefinitelyNonBoolExpr(cond) {
		c.errorf("%s: condition must be bool", c.curFunc.Name)
	}

	if cond.Kind == NUnaryExpr && cond.Name == "!" {
		c.compileCondJump(cond.X, !jumpIfTrue, targetLabel)
		return
	}

	if cond.Kind == NBinaryExpr {
		switch cond.Name {
		case "&&":
			if c.isDefinitelyNonBoolExpr(cond.X) || c.isDefinitelyNonBoolExpr(cond.Y) {
				c.errorf("%s: && requires boolean operands", c.curFunc.Name)
			}
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
			if c.isDefinitelyNonBoolExpr(cond.X) || c.isDefinitelyNonBoolExpr(cond.Y) {
				c.errorf("%s: || requires boolean operands", c.curFunc.Name)
			}
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
			c.strictCheckComparison(cond.Name, cond.X, cond.Y)
			if c.boolOperandMismatch(cond.X, cond.Y) {
				c.errorf("%s: invalid comparison between bool and non-bool", c.curFunc.Name)
			}
			if c.isStringTypedExpr(cond.X) || c.isStringTypedExpr(cond.Y) || isStringExpr(cond.X) || isStringExpr(cond.Y) {
				if c.emitStringCompareResult(cond.Name, cond.X, cond.Y) {
					if jumpIfTrue {
						c.emit(ir.Inst{Op: ir.OP_JMP_IF, Arg: targetLabel})
					} else {
						c.emit(ir.Inst{Op: ir.OP_JMP_IF_NOT, Arg: targetLabel})
					}
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
	if c.isDefinitelyInvalidRangeArg(node.Type) {
		c.errorf("%s: cannot range over non-iterable expression", c.curFunc.Name)
		return
	}

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
		c.emitKnownCall("runtime.MapLen", 1, 1)
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
			c.emitKnownCall("runtime.MapEntryKey", 2, 1)
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
		c.emitKnownCall("runtime.StringDecodeRune", 2, 2)
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
			c.emitKnownCall("runtime.MapEntryValue", 2, 1)
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
	typeSwitchVarName := ""
	if isTypeSwitch && node.Type != nil && node.Type.Kind == NIdent && node.Type.Name != "_" {
		typeSwitchVarName = node.Type.Name
	}
	needsScope := node.X != nil || typeSwitchVarName != ""
	if needsScope {
		c.pushScope()
		c.compileStmt(node.X)
	}
	typeSwitchVarIdx := -1
	typeSwitchIfaceIdx := -1
	if typeSwitchVarName != "" {
		typeSwitchVarIdx = c.addLocal(typeSwitchVarName)
	}

	// Compile tag if present
	hasTag := node.Y != nil
	if hasTag {
		if isTypeSwitch {
			c.compileExpr(node.Y)
			typeSwitchIfaceIdx = c.addLocal("$typeswitch_iface")
			c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: typeSwitchIfaceIdx})
			c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: typeSwitchIfaceIdx})
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
			c.bindTypeSwitchVar(typeSwitchVarName, typeSwitchVarIdx, typeSwitchIfaceIdx, nil, true)
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
					if !isTypeSwitch {
						c.strictCheckComparison("==", node.Y, expr)
					}
					c.emit(ir.Inst{Op: ir.OP_DUP})
					if isTypeSwitch {
						c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(c.typeIDForTypeNode(expr))})
					} else {
						c.compileExpr(expr)
						if isStringSwitch {
							c.emitKnownCall("runtime.StringEqual", 2, 1)
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
			c.bindTypeSwitchVar(typeSwitchVarName, typeSwitchVarIdx, typeSwitchIfaceIdx, caseExprs, false)
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

func (c *Compiler) bindTypeSwitchVar(typeSwitchVarName string, typeSwitchVarIdx int, typeSwitchIfaceIdx int, caseExprs []*Node, isDefault bool) {
	if typeSwitchVarName == "" || typeSwitchVarIdx < 0 || typeSwitchIfaceIdx < 0 {
		return
	}
	delete(c.localConcreteTypes, typeSwitchVarName)
	delete(c.localStringVars, typeSwitchVarName)
	delete(c.localMapVars, typeSwitchVarName)
	delete(c.localMapValueTypes, typeSwitchVarName)
	assignIface := isDefault || len(caseExprs) != 1
	ifaceType := "interface{}"
	if !assignIface {
		caseType := nodeTypeName(caseExprs[0])
		caseQType := c.qualifyTypeName(caseType, "")
		if c.isInterfaceTypeName(caseType) || c.isInterfaceTypeName(caseQType) {
			if caseQType != "" {
				ifaceType = caseQType
			}
			assignIface = true
		} else if caseQType != "" {
			delete(c.localTypes, typeSwitchVarName)
			c.localConcreteTypes[typeSwitchVarName] = caseQType
			if caseQType == "string" {
				c.localStringVars[typeSwitchVarName] = true
			}
			if len(caseQType) >= 4 && caseQType[0:4] == "map[" {
				c.setLocalMapMetadataFromQualified(typeSwitchVarName, caseQType)
			}
			c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: typeSwitchIfaceIdx})
			c.emit(ir.Inst{Op: ir.OP_OFFSET, Arg: c.target.PtrSize})
			c.emit(ir.Inst{Op: ir.OP_LOAD, Arg: c.target.PtrSize})
			c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: typeSwitchVarIdx})
			return
		}
		assignIface = true
	}
	if assignIface {
		c.localTypes[typeSwitchVarName] = ifaceType
		c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: typeSwitchIfaceIdx})
		c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: typeSwitchVarIdx})
	}
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
	assertedType := c.qualifiedTypeFromTypeNode(node.Type, "")
	payloadSize := c.storageSizeForTypeName(assertedType)
	payloadLoad := ir.Inst{Op: ir.OP_LOAD, Arg: payloadSize}
	payloadLoad.Name = c.floatInstNameForTypeName(assertedType)

	if commaOk {
		ifaceIdx := c.addLocal("$typeassert_iface")
		valIdx := c.addLocal("$typeassert_val")
		okIdx := c.addLocal("$typeassert_ok")
		c.setLocalTypeFlags(valIdx, assertedType)
		c.curFunc.Locals[valIdx].Width = payloadSize

		c.compileExpr(node.X)
		c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: ifaceIdx})

		c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: ifaceIdx})
		c.emit(ir.Inst{Op: ir.OP_OFFSET, Arg: 0})
		c.emit(ir.Inst{Op: ir.OP_LOAD, Arg: c.target.PtrSize})
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(typeID)})

		failLabel := c.newLabel()
		endLabel := c.newLabel()
		c.emit(ir.Inst{Op: ir.OP_JMP_NEQ, Arg: failLabel})

		// Success: extract payload + true.
		c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: ifaceIdx})
		c.emit(ir.Inst{Op: ir.OP_OFFSET, Arg: c.target.PtrSize})
		c.emit(payloadLoad)
		c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: valIdx})
		c.emit(ir.Inst{Op: ir.OP_CONST_BOOL, Arg: 1})
		c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: okIdx})
		c.emit(ir.Inst{Op: ir.OP_JMP, Arg: endLabel})

		// Failure: zero value + false.
		c.emitLabel(failLabel)
		c.emitZeroValueForTypeName(assertedType)
		c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: valIdx})
		c.emit(ir.Inst{Op: ir.OP_CONST_BOOL, Arg: 0})
		c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: okIdx})
		c.emitLabel(endLabel)

		c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: valIdx})
		c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: okIdx})
		return
	}

	ifaceIdx := c.addLocal("$typeassert_iface")
	valIdx := c.addLocal("$typeassert_val")
	c.setLocalTypeFlags(valIdx, assertedType)
	c.curFunc.Locals[valIdx].Width = payloadSize

	c.compileExpr(node.X)
	c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: ifaceIdx})
	c.emitZeroValueForTypeName(assertedType)
	c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: valIdx})
	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: ifaceIdx})
	c.emit(ir.Inst{Op: ir.OP_OFFSET, Arg: 0})
	c.emit(ir.Inst{Op: ir.OP_LOAD, Arg: c.target.PtrSize})
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(typeID)})

	failLabel := c.newLabel()
	endLabel := c.newLabel()
	c.emit(ir.Inst{Op: ir.OP_JMP_NEQ, Arg: failLabel})
	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: ifaceIdx})
	c.emit(ir.Inst{Op: ir.OP_OFFSET, Arg: c.target.PtrSize})
	c.emit(payloadLoad)
	c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: valIdx})
	c.emit(ir.Inst{Op: ir.OP_JMP, Arg: endLabel})

	c.emitLabel(failLabel)
	c.emit(makeInst(ir.OP_CONST_STR, 0, 0, 0, "type assertion failed"))
	c.emit(ir.Inst{Op: ir.OP_PANIC})
	c.emitLabel(endLabel)
	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: valIdx})
}

func (c *Compiler) compileTypeAssertExpr(node *Node) {
	c.compileTypeAssert(node, false)
}

func (c *Compiler) compileTypeAssertCommaOk(node *Node) {
	c.compileTypeAssert(node, true)
}

func (c *Compiler) compileInc(node *Node) {
	if c.isPointerExprForStrict(node.X) {
		c.errorf("%s: pointer arithmetic is not allowed", c.curFunc.Name)
	}
	w := c.exprWidth(node.X)
	c.compileLValueGet(node.X)
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 1})
	inst := ir.Inst{Op: ir.OP_ADD, Width: w}
	if c.isUnsignedExpr(node.X) {
		inst.Name = "unsigned"
	}
	c.emit(inst)
	c.compileLValueSet(node.X)
}

func (c *Compiler) compileDec(node *Node) {
	if c.isPointerExprForStrict(node.X) {
		c.errorf("%s: pointer arithmetic is not allowed", c.curFunc.Name)
	}
	w := c.exprWidth(node.X)
	c.compileLValueGet(node.X)
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 1})
	inst := ir.Inst{Op: ir.OP_SUB, Width: w}
	if c.isUnsignedExpr(node.X) {
		inst.Name = "unsigned"
	}
	c.emit(inst)
	c.compileLValueSet(node.X)
}

func (c *Compiler) compileBranch(node *Node) {
	switch node.Name {
	case "break":
		if node.X != nil && node.X.Kind == NIdent {
			targets := c.breakLabelTargets[node.X.Name]
			if len(targets) > 0 {
				c.emit(ir.Inst{Op: ir.OP_JMP, Arg: targets[len(targets)-1]})
			} else {
				c.errorf("%s: invalid break label %s", c.curFunc.Name, node.X.Name)
			}
		} else if len(c.breaks) > 0 {
			c.emit(ir.Inst{Op: ir.OP_JMP, Arg: c.breaks[len(c.breaks)-1]})
		} else {
			c.errorf("%s: break is not in a loop or switch", c.curFunc.Name)
		}
	case "continue":
		if node.X != nil && node.X.Kind == NIdent {
			targets := c.continueLabelTargets[node.X.Name]
			if len(targets) > 0 {
				c.emit(ir.Inst{Op: ir.OP_JMP, Arg: targets[len(targets)-1]})
			} else {
				c.errorf("%s: invalid continue label %s", c.curFunc.Name, node.X.Name)
			}
		} else if len(c.continues) > 0 {
			c.emit(ir.Inst{Op: ir.OP_JMP, Arg: c.continues[len(c.continues)-1]})
		} else {
			c.errorf("%s: continue is not in a loop", c.curFunc.Name)
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
	case NFloatLit:
		c.compileFloatLit(node)
	case NStringLit:
		c.compileStringLit(node)
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
	val, ok := parseIntLiteralChecked(node.Name)
	if !ok {
		c.errorf("%s: invalid integer literal %q", c.curFunc.Name, node.Name)
		val = 0
	}
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: val})
}

func (c *Compiler) compileFloatLit(node *Node) {
	if !isValidFloatLiteral(node.Name) {
		c.errorf("%s: invalid float literal %q", c.curFunc.Name, node.Name)
		c.emit(makeInst(ir.OP_CONST_F64, 0, 0, 0, "0.0"))
		return
	}
	c.emit(makeInst(ir.OP_CONST_F64, 0, 8, 0, node.Name))
}

func (c *Compiler) compileStringLit(node *Node) {
	if !isValidStringLiteralContents(node.Name) {
		c.errorf("%s: invalid string escape in literal", c.curFunc.Name)
		c.emit(makeInst(ir.OP_CONST_STR, 0, 0, 0, ""))
		return
	}
	c.emit(makeInst(ir.OP_CONST_STR, 0, 0, 0, node.Name))
}

func (c *Compiler) compileRuneLit(node *Node) {
	val, ok := parseRuneLiteralChecked(node.Name)
	if !ok {
		c.errorf("%s: invalid rune literal %q", c.curFunc.Name, node.Name)
		val = 0
	}
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
	if node.Name == "recover" {
		// Predeclared builtin function value; represented as nil placeholder.
		c.emit(ir.Inst{Op: ir.OP_CONST_NIL})
		return
	}
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
		c.emit(makeInst(ir.OP_CONST_STR, 0, 0, 0, sval))
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
			c.emit(makeInst(ir.OP_CONST_STR, 0, 0, 0, c.evalConstString(sym.Node.X)))
			return
		}
		if c.isConstFloatExpr(sym.Node.X) {
			c.compileExpr(sym.Node.X)
			return
		}
		val := c.resolveConstValue(sym.Node)
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: val})
		return
	}
	if c.inAssembleBuilder && symOk && sym.Kind == SymFunc {
		c.emit(makeInst(ir.OP_CONST_STR, 0, 0, 0, c.curPkg.QualName(node.Name)))
		return
	}
	c.errorf("%s: undefined: %s", c.curFunc.Name, node.Name)
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
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
		if c.localConcreteTypes[node.Name] == "string" {
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
		if c.isMapExpr(node.X) {
			return c.qualifyTypeName(c.resolveMapValueType(node.X), "") == "string"
		}
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

func (c *Compiler) isBoolTypedExpr(node *Node) bool {
	if node == nil {
		return false
	}
	switch node.Kind {
	case NBasicLit:
		return node.Name == "true" || node.Name == "false"
	case NUnaryExpr:
		if node.Name == "!" {
			return c.isBoolTypedExpr(node.X)
		}
		if node.Name == "*" {
			t := c.resolveExprType(node)
			return t == "bool" || isBuiltinBoolTypeName(t)
		}
		return false
	case NBinaryExpr:
		switch node.Name {
		case "&&", "||", "==", "!=", "<", ">", "<=", ">=":
			return true
		}
	case NCallExpr:
		if node.X != nil {
			calleeName := c.resolveCallName(node.X)
			if retTypes, ok := c.funcRetTypes[calleeName]; ok && len(retTypes) > 0 {
				return retTypes[0] == "bool"
			}
		}
	}
	t := c.resolveExprType(node)
	return t == "bool" || isBuiltinBoolTypeName(t)
}

func (c *Compiler) isDefinitelyNonBoolExpr(node *Node) bool {
	if node == nil {
		return false
	}
	switch node.Kind {
	case NIntLit, NRuneLit, NStringLit:
		return true
	case NBasicLit:
		return node.Name != "true" && node.Name != "false"
	case NUnaryExpr:
		if node.Name == "!" {
			return false
		}
		if node.Name == "*" {
			t := c.resolveExprType(node)
			return t != "" && t != "bool" && !isBuiltinBoolTypeName(t)
		}
		return true
	case NBinaryExpr:
		switch node.Name {
		case "&&", "||", "==", "!=", "<", ">", "<=", ">=":
			return false
		default:
			return true
		}
	}
	t := c.resolveExprType(node)
	return t != "" && t != "bool" && !isBuiltinBoolTypeName(t)
}

func (c *Compiler) boolOperandMismatch(left *Node, right *Node) bool {
	leftBool := c.isBoolTypedExpr(left)
	rightBool := c.isBoolTypedExpr(right)
	return (leftBool && !rightBool) || (!leftBool && rightBool)
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
	c.emitKnownCall("runtime.StringEqual", 2, 1)
}

func (c *Compiler) emitStringLessCall(x *Node, y *Node) {
	c.compileExpr(x)
	c.compileExpr(y)
	c.emitKnownCall("runtime.StringLess", 2, 1)
}

// emitStringCompareResult emits code that leaves a bool result on the stack for
// string comparison op x <op> y.
func (c *Compiler) emitStringCompareResult(op string, x *Node, y *Node) bool {
	switch op {
	case "==":
		c.emitStringEqualCall(x, y)
		return true
	case "!=":
		c.emitStringEqualCall(x, y)
		c.emit(ir.Inst{Op: ir.OP_NOT})
		return true
	case "<":
		c.emitStringLessCall(x, y)
		return true
	case ">":
		c.emitStringLessCall(y, x)
		return true
	case "<=":
		c.emitStringLessCall(y, x)
		c.emit(ir.Inst{Op: ir.OP_NOT})
		return true
	case ">=":
		c.emitStringLessCall(x, y)
		c.emit(ir.Inst{Op: ir.OP_NOT})
		return true
	default:
		return false
	}
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

	c.strictCheckComparison(node.Name, node.X, node.Y)
	c.strictCheckPointerArithmetic(node.Name, node.X, node.Y)

	// String operations: concatenation and comparison
	isStr := isStringExpr(node.X) || isStringExpr(node.Y) || c.isStringTypedExpr(node.X) || c.isStringTypedExpr(node.Y)
	if isStr && node.Name == "+" {
		c.compileExpr(node.X)
		c.compileExpr(node.Y)
		c.emitKnownCall("runtime.StringConcat", 2, 1)
		return
	}
	if isStr && c.emitStringCompareResult(node.Name, node.X, node.Y) {
		return
	}
	isFloat := c.isFloatExpr(node.X) || c.isFloatExpr(node.Y)
	floatType := ""
	if isFloat {
		floatType = mergeFloatTypeNames(c.floatExprTypeName(node.X), c.floatExprTypeName(node.Y))
	}
	if isFloat {
		switch node.Name {
		case "%", "&", "|", "^", "<<", ">>":
			c.errorf("%s: operator %s is not supported for %s", c.curFunc.Name, node.Name, floatType)
			c.emitZeroValueForTypeName(floatType)
			return
		}
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
	if floatType != "" {
		leftFloatType := c.floatExprTypeName(node.X)
		if leftFloatType == "" || leftFloatType != floatType {
			c.emitConvertForExpr(node.X, floatType)
		}
	}
	c.compileExpr(node.Y)
	if floatType != "" {
		rightFloatType := c.floatExprTypeName(node.Y)
		if rightFloatType == "" || rightFloatType != floatType {
			c.emitConvertForExpr(node.Y, floatType)
		}
	}

	w := c.exprWidth(node)

	switch node.Name {
	case "+":
		inst := ir.Inst{Op: ir.OP_ADD, Width: w}
		if isFloat {
			inst.Name = floatType
		} else if c.isUnsignedExpr(node.X) {
			inst.Name = "unsigned"
		}
		c.emit(inst)
	case "-":
		inst := ir.Inst{Op: ir.OP_SUB, Width: w}
		if isFloat {
			inst.Name = floatType
		} else if c.isUnsignedExpr(node.X) {
			inst.Name = "unsigned"
		}
		c.emit(inst)
	case "*":
		inst := ir.Inst{Op: ir.OP_MUL, Width: w}
		if isFloat {
			inst.Name = floatType
		} else if c.isUnsignedExpr(node.X) {
			inst.Name = "unsigned"
		}
		c.emit(inst)
	case "/":
		inst := ir.Inst{Op: ir.OP_DIV, Width: w}
		if isFloat {
			inst.Name = floatType
		} else if c.isUnsignedExpr(node.X) {
			inst.Name = "unsigned"
		}
		c.emit(inst)
	case "%":
		inst := ir.Inst{Op: ir.OP_MOD, Width: w}
		if c.isUnsignedExpr(node.X) {
			inst.Name = "unsigned"
		}
		c.emit(inst)
	case "&":
		c.emit(ir.Inst{Op: ir.OP_AND, Width: w})
	case "|":
		c.emit(ir.Inst{Op: ir.OP_OR, Width: w})
	case "^":
		c.emit(ir.Inst{Op: ir.OP_XOR, Width: w})
	case "<<":
		c.emit(ir.Inst{Op: ir.OP_SHL, Width: w})
	case ">>":
		inst := ir.Inst{Op: ir.OP_SHR, Width: w}
		if c.isUnsignedExpr(node.X) {
			inst.Name = "unsigned"
		}
		c.emit(inst)
	case "==":
		inst := ir.Inst{Op: ir.OP_EQ, Width: w}
		if isFloat {
			inst.Name = floatType
		} else if c.isUnsignedComparison(node) {
			inst.Name = "unsigned"
		}
		c.emit(inst)
	case "!=":
		inst := ir.Inst{Op: ir.OP_NEQ, Width: w}
		if isFloat {
			inst.Name = floatType
		} else if c.isUnsignedComparison(node) {
			inst.Name = "unsigned"
		}
		c.emit(inst)
	case "<":
		inst := ir.Inst{Op: ir.OP_LT, Width: w}
		if isFloat {
			inst.Name = floatType
		} else if c.isUnsignedComparison(node) {
			inst.Name = "unsigned"
		}
		c.emit(inst)
	case ">":
		inst := ir.Inst{Op: ir.OP_GT, Width: w}
		if isFloat {
			inst.Name = floatType
		} else if c.isUnsignedComparison(node) {
			inst.Name = "unsigned"
		}
		c.emit(inst)
	case "<=":
		inst := ir.Inst{Op: ir.OP_LEQ, Width: w}
		if isFloat {
			inst.Name = floatType
		} else if c.isUnsignedComparison(node) {
			inst.Name = "unsigned"
		}
		c.emit(inst)
	case ">=":
		inst := ir.Inst{Op: ir.OP_GEQ, Width: w}
		if isFloat {
			inst.Name = floatType
		} else if c.isUnsignedComparison(node) {
			inst.Name = "unsigned"
		}
		c.emit(inst)
	default:
		panic("ICE: unhandled binary operator in compileBinaryExpr")
	}
}

func (c *Compiler) compileUnaryExpr(node *Node) {
	switch node.Name {
	case "!":
		c.compileExpr(node.X)
		c.emit(makeInst(ir.OP_NOT, 0, 0, 0, ""))
	case "-":
		w := c.exprWidth(node.X)
		c.compileExpr(node.X)
		inst := makeInst(ir.OP_NEG, 0, w, 0, "")
		if c.isFloatExpr(node.X) {
			inst.Name = c.resolvedFloatInstName(node.X)
		}
		c.emit(inst)
	case "*":
		if c.isDefinitelyNonPointerExpr(node.X) {
			c.errorf("%s: cannot indirect non-pointer expression", c.curFunc.Name)
		}
		c.compileExpr(node.X)
		if !c.isPointerToStructDeref(node.X) {
			inst := ir.Inst{Op: ir.OP_LOAD, Arg: c.exprWidth(node)}
			inst.Name = floatInstName(c.resolveExprType(node))
			c.emit(inst)
		}
	case "&":
		c.compileAddrOf(node.X)
	case "^":
		if c.isFloatExpr(node.X) {
			floatType := c.resolvedFloatInstName(node.X)
			c.errorf("%s: operator ^ is not supported for %s", c.curFunc.Name, floatType)
			c.emitZeroValueForTypeName(floatType)
			return
		}
		w := c.exprWidth(node.X)
		c.compileExpr(node.X)
		c.emit(makeInst(ir.OP_CONST_I64, 0, w, -1, ""))
		c.emit(makeInst(ir.OP_XOR, 0, w, 0, ""))
		if w == 1 {
			c.emit(makeInst(ir.OP_CONST_I64, 0, 0, 0xFF, ""))
			c.emit(makeInst(ir.OP_AND, 0, 0, 0, ""))
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
	pkgPath, tName, ok := qualifiedPointerTargetInfo(ct)
	if !ok {
		return false
	}
	if isNonStructPointerTargetType(tName) || pkgPath == "" {
		return false
	}
	pkg, ok := c.mod.Packages[pkgPath]
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
	if node == nil {
		return false
	}
	ptrType := c.resolveExprType(node)
	if ptrType == "" {
		ptrType = c.exprConcreteType(node)
	}
	if ptrType == "" {
		// In later self-host stages we can miss concrete type metadata for
		// pointer locals; default to no-op deref to preserve handle semantics.
		return true
	}
	pointeeType := derefQualifiedTypeName(ptrType)
	if pointeeType == "" {
		return false
	}
	// Pointers to slices, arrays, maps, funcs, pointers, and scalar forms
	// still need an actual load on dereference.
	if strings.HasPrefix(pointeeType, "[]") || strings.HasPrefix(pointeeType, "[") ||
		strings.HasPrefix(pointeeType, "map[") || strings.HasPrefix(pointeeType, "func(") ||
		strings.HasPrefix(pointeeType, "*") {
		return false
	}
	if pointeeType == "int" || pointeeType == "int16" || pointeeType == "int32" || pointeeType == "int64" ||
		pointeeType == "uint" || pointeeType == "uint16" || pointeeType == "uint32" || pointeeType == "uint64" ||
		pointeeType == "uintptr" || pointeeType == "byte" || pointeeType == "bool" || pointeeType == "string" ||
		pointeeType == "float32" || pointeeType == "float64" {
		return false
	}
	typeNode, _ := c.lookupStructTypeNode(pointeeType)
	if typeNode == nil {
		// Missing package/type metadata in later self-host stages: prefer no-op
		// for named pointer types to preserve struct-handle semantics.
		return true
	}
	return typeNode.Kind == NStructType
}

func (c *Compiler) isDefinitelyNonPointerExpr(node *Node) bool {
	if node == nil {
		return false
	}
	switch node.Kind {
	case NIntLit, NRuneLit, NStringLit:
		return true
	case NUnaryExpr:
		if node.Name == "&" {
			return false
		}
	}
	if node.Kind == NIdent && c.localAddrOf[node.Name] {
		return false
	}
	t := c.resolveExprType(node)
	if t == "" {
		return false
	}
	if strings.HasPrefix(t, "*") || strings.Contains(t, ".*") {
		return false
	}
	return true
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
	c.emitRuntimeAllocCall()
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

func (c *Compiler) profileParentHashForCalls() uint32 {
	if c.target == nil || !c.target.Profile {
		return 0
	}
	return c.currentMethodHash
}

//rtg:noprofile
func (c *Compiler) callNeedsProfileParent(callName string) bool {
	if c.target == nil || !c.target.Profile {
		return false
	}
	if callName == "" {
		return false
	}
	return c.funcProfileParentABI[callName]
}

func (c *Compiler) rewriteProfileParentCalls() {
	if c.target == nil || !c.target.Profile {
		return
	}
	for _, f := range c.irmod.Funcs {
		if f == nil || len(f.Code) == 0 {
			continue
		}
		maxArgs := 0
		extraInst := 0
		rewriteCount := 0
		for _, inst := range f.Code {
			if inst.Op == ir.OP_CALL && c.funcProfileParentABI[inst.Name] {
				rewriteCount++
				if inst.Arg > maxArgs {
					maxArgs = inst.Arg
				}
				extraInst += inst.Arg*2 + 1
			}
		}
		if rewriteCount == 0 {
			continue
		}
		tempBase := len(f.Locals)
		if maxArgs > 0 {
			i := 0
			for i < maxArgs {
				f.Locals = append(f.Locals, ir.IRLocal{
					Name:  "",
					Index: tempBase + i,
				})
				i++
			}
		}
		callerHash := profileHash32FNV(f.Name)
		rewritten := make([]ir.Inst, 0, len(f.Code)+extraInst)
		for _, inst := range f.Code {
			if inst.Op == ir.OP_CALL && c.funcProfileParentABI[inst.Name] {
				argCount := inst.Arg
				i := argCount - 1
				for i >= 0 {
					rewritten = append(rewritten, ir.Inst{Op: ir.OP_LOCAL_SET, Arg: tempBase + i})
					i--
				}
				if c.methodFuncNames[inst.Name] {
					if argCount > 0 {
						rewritten = append(rewritten, ir.Inst{Op: ir.OP_LOCAL_GET, Arg: tempBase})
					}
					rewritten = append(rewritten, ir.Inst{Op: ir.OP_CONST_I64, Val: int64(callerHash)})
					i = 1
					for i < argCount {
						rewritten = append(rewritten, ir.Inst{Op: ir.OP_LOCAL_GET, Arg: tempBase + i})
						i++
					}
				} else {
					rewritten = append(rewritten, ir.Inst{Op: ir.OP_CONST_I64, Val: int64(callerHash)})
					i = 0
					for i < argCount {
						rewritten = append(rewritten, ir.Inst{Op: ir.OP_LOCAL_GET, Arg: tempBase + i})
						i++
					}
				}
				inst.Arg = argCount + 1
			}
			rewritten = append(rewritten, inst)
		}
		f.Code = rewritten
	}
}

func (c *Compiler) buildRecoverReachability() map[string]bool {
	mayRecover := make(map[string]bool)
	if c == nil || c.irmod == nil {
		return mayRecover
	}
	// Seed: functions that directly call runtime.Recover.
	for _, f := range c.irmod.Funcs {
		if f == nil {
			continue
		}
		for _, inst := range f.Code {
			if inst.Op == ir.OP_CALL && inst.Name == "runtime.Recover" {
				mayRecover[f.Name] = true
				break
			}
		}
	}
	changed := true
	for changed {
		changed = false
		for _, f := range c.irmod.Funcs {
			if f == nil || mayRecover[f.Name] {
				continue
			}
			i := 0
			for i < len(f.Code) {
				inst := f.Code[i]
				switch inst.Op {
				case ir.OP_CALL:
					if mayRecover[inst.Name] {
						mayRecover[f.Name] = true
						changed = true
						i = len(f.Code)
						continue
					}
				case ir.OP_IFACE_CALL:
					if c.ifaceCallMayReachRecover(inst.Name, mayRecover) {
						mayRecover[f.Name] = true
						changed = true
						i = len(f.Code)
						continue
					}
				}
				i++
			}
		}
	}
	return mayRecover
}

func (c *Compiler) buildKnownFuncSet() map[string]bool {
	known := make(map[string]bool)
	if c == nil || c.irmod == nil {
		return known
	}
	for _, f := range c.irmod.Funcs {
		if f != nil {
			known[f.Name] = true
		}
	}
	return known
}

func (c *Compiler) ifaceCallMayReachRecover(ifaceCallName string, mayRecover map[string]bool) bool {
	dot := lastIndexByteInString(ifaceCallName, '.')
	if dot < 0 || dot+1 >= len(ifaceCallName) {
		return true
	}
	methodName := ifaceCallName[dot+1:]
	found := false
	for key, resolved := range c.methodTable {
		kdot := lastIndexByteInString(key, '.')
		if kdot < 0 || kdot+1 >= len(key) {
			continue
		}
		if key[kdot+1:] != methodName {
			continue
		}
		found = true
		if mayRecover[resolved] {
			return true
		}
	}
	if !found {
		// Unknown dynamic target set: keep wrappers conservatively.
		return true
	}
	return false
}

func (c *Compiler) callMayReachRecover(inst ir.Inst, mayRecover map[string]bool, known map[string]bool) bool {
	switch inst.Op {
	case ir.OP_CALL:
		if inst.Name == "" {
			return true
		}
		// Unknown callee names are treated conservatively.
		if mayRecover[inst.Name] {
			return true
		}
		return !known[inst.Name]
	case ir.OP_IFACE_CALL:
		return c.ifaceCallMayReachRecover(inst.Name, mayRecover)
	default:
		return false
	}
}

func (c *Compiler) buildPanicReachability() map[string]bool {
	mayPanic := make(map[string]bool)
	if c == nil || c.irmod == nil {
		return mayPanic
	}
	// Seed: functions that directly start panic-unwind.
	for _, f := range c.irmod.Funcs {
		if f == nil {
			continue
		}
		for _, inst := range f.Code {
			if inst.Op == ir.OP_PANIC {
				mayPanic[f.Name] = true
				break
			}
			if inst.Op == ir.OP_CALL && inst.Name == "runtime.PanicBegin" {
				mayPanic[f.Name] = true
				break
			}
		}
	}
	known := c.buildKnownFuncSet()
	changed := true
	for changed {
		changed = false
		for _, f := range c.irmod.Funcs {
			if f == nil || mayPanic[f.Name] {
				continue
			}
			i := 0
			for i < len(f.Code) {
				inst := f.Code[i]
				switch inst.Op {
				case ir.OP_CALL:
					if inst.Name == "" {
						mayPanic[f.Name] = true
						changed = true
						i = len(f.Code)
						continue
					}
					if mayPanic[inst.Name] {
						mayPanic[f.Name] = true
						changed = true
						i = len(f.Code)
						continue
					}
					if !known[inst.Name] {
						// Unknown calls are treated conservatively.
						mayPanic[f.Name] = true
						changed = true
						i = len(f.Code)
						continue
					}
				case ir.OP_IFACE_CALL:
					// Interface dispatch is conservative for panic propagation: keep
					// unwind checks through dynamic call chains.
					mayPanic[f.Name] = true
					changed = true
					i = len(f.Code)
					continue
				}
				i++
			}
		}
	}
	return mayPanic
}

func (c *Compiler) ifaceCallMayReachPanic(ifaceCallName string, mayPanic map[string]bool) bool {
	dot := lastIndexByteInString(ifaceCallName, '.')
	if dot < 0 || dot+1 >= len(ifaceCallName) {
		return true
	}
	methodName := ifaceCallName[dot+1:]
	found := false
	for key, resolved := range c.methodTable {
		kdot := lastIndexByteInString(key, '.')
		if kdot < 0 || kdot+1 >= len(key) {
			continue
		}
		if key[kdot+1:] != methodName {
			continue
		}
		found = true
		if mayPanic[resolved] {
			return true
		}
	}
	if !found {
		// Unknown dynamic target set: keep checks conservatively.
		return true
	}
	return false
}

func (c *Compiler) callMayTriggerPanic(inst ir.Inst, mayPanic map[string]bool, known map[string]bool) bool {
	switch inst.Op {
	case ir.OP_CALL:
		if inst.Name == "" {
			return true
		}
		if mayPanic[inst.Name] {
			return true
		}
		return !known[inst.Name]
	case ir.OP_IFACE_CALL:
		return true
	default:
		return false
	}
}

func (c *Compiler) prunePanicPropagationChecks() {
	if c == nil || c.irmod == nil {
		return
	}
	if c.target != nil && c.target.GOARCH == "wasm32" {
		// WASM stackification currently relies on fully conservative panic
		// propagation checks in self-hosted compiler builds.
		return
	}
	mayPanic := c.buildPanicReachability()
	known := c.buildKnownFuncSet()
	for _, f := range c.irmod.Funcs {
		if f == nil || len(f.Code) < 3 {
			continue
		}
		out := make([]ir.Inst, 0, len(f.Code))
		i := 0
		for i < len(f.Code) {
			if i+2 < len(f.Code) &&
				(f.Code[i].Op == ir.OP_CALL || f.Code[i].Op == ir.OP_IFACE_CALL) &&
				f.Code[i+1].Op == ir.OP_CALL &&
				f.Code[i+1].Name == "runtime.PanicShouldUnwind" &&
				(f.Code[i+2].Op == ir.OP_JMP_IF_NOT || f.Code[i+2].Op == ir.OP_JMP_IF) {
				callInst := f.Code[i]
				if f.Code[i+2].Op == ir.OP_JMP_IF {
					if c.callMayTriggerPanic(callInst, mayPanic, known) {
						out = append(out, f.Code[i:i+3]...)
					} else {
						out = append(out, callInst)
					}
					i = i + 3
					continue
				}
				continueLabel := f.Code[i+2].Arg
				j := i + 3
				for j < len(f.Code) && f.Code[j].Op == ir.OP_DROP {
					j++
				}
				if j+1 < len(f.Code) &&
					f.Code[j].Op == ir.OP_JMP &&
					f.Code[j+1].Op == ir.OP_LABEL &&
					f.Code[j+1].Arg == continueLabel {
					if c.callMayTriggerPanic(callInst, mayPanic, known) {
						out = append(out, f.Code[i:j+2]...)
					} else {
						out = append(out, callInst)
					}
					i = j + 2
					continue
				}
			}
			out = append(out, f.Code[i])
			i++
		}
		f.Code = out
	}
}

func (c *Compiler) insertDeferRecoverCallWrappers() {
	if c == nil || c.irmod == nil {
		return
	}
	mayRecover := c.buildRecoverReachability()
	known := c.buildKnownFuncSet()
	for _, f := range c.irmod.Funcs {
		if f == nil || len(f.Code) < 3 {
			continue
		}
		if !c.deferRecoverWrapFuncs[f.Name] {
			continue
		}
		out := make([]ir.Inst, 0, len(f.Code))
		i := 0
		for i < len(f.Code) {
			if i+2 < len(f.Code) &&
				(f.Code[i].Op == ir.OP_CALL || f.Code[i].Op == ir.OP_IFACE_CALL) &&
				f.Code[i+1].Op == ir.OP_CALL &&
				f.Code[i+1].Name == "runtime.PanicShouldUnwind" &&
				(f.Code[i+2].Op == ir.OP_JMP_IF_NOT || f.Code[i+2].Op == ir.OP_JMP_IF) &&
				c.callMayReachRecover(f.Code[i], mayRecover, known) {
				out = append(out, makeInst(ir.OP_CALL, 0, 0, 0, "runtime.DeferRecoverBeforeCall"))
				out = append(out, f.Code[i])
				out = append(out, makeInst(ir.OP_CALL, 0, 0, 0, "runtime.DeferRecoverAfterCall"))
				i++
				continue
			}
			out = append(out, f.Code[i])
			i++
		}
		f.Code = out
	}
}

func (c *Compiler) methodCallNeedsProfileParent(callName string) bool {
	return c.methodFuncNames[callName] && c.callNeedsProfileParent(callName)
}

func (c *Compiler) emitCallWithReceiver(receiver *Node, args []*Node, callName string) {
	paramTypes := c.funcParamTypes[callName]
	c.compileExpr(receiver)
	if len(paramTypes) > 0 {
		c.maybeCloneArrayForTypeName(paramTypes[0])
	}
	paramOffset := 1
	argCount := len(args) + 1
	if c.methodCallNeedsProfileParent(callName) {
		paramOffset++
	}
	for i, arg := range args {
		c.compileExpr(arg)
		if i+paramOffset < len(paramTypes) {
			c.maybeConvertArgForParamType(arg, paramTypes[i+paramOffset])
			c.maybeCloneArrayForTypeName(paramTypes[i+paramOffset])
			if c.isInterfaceTypeName(paramTypes[i+paramOffset]) {
				c.maybeBoxValueForInterface(arg)
			}
		}
	}
	c.emitCallWithPanicCheck(callName, argCount)
}

func (c *Compiler) emitIfaceMethodCall(recvExpr *Node, args []*Node, ifaceType string, methodName string) {
	paramTypes := c.funcParamTypes[c.dotJoin(ifaceType, methodName)]
	c.compileExpr(recvExpr)
	argCount := len(args)
	if c.target != nil && c.target.Profile {
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(c.profileParentHashForCalls())})
		argCount++
	}
	for i, arg := range args {
		c.compileExpr(arg)
		if i < len(paramTypes) {
			c.maybeConvertArgForParamType(arg, paramTypes[i])
			c.maybeCloneArrayForTypeName(paramTypes[i])
			if c.isInterfaceTypeName(paramTypes[i]) {
				c.maybeBoxValueForInterface(arg)
			}
		}
	}
	retCount, _ := c.ifaceMethodReturnCount(ifaceType, methodName)
	c.emitIfaceCallWithPanicCheck(c.dotJoin(ifaceType, methodName), argCount, retCount)
}

func (c *Compiler) emitPromotedMethodCall(recvExpr *Node, args []*Node, pm promotedMethodMatch) {
	c.compileExpr(recvExpr)
	i := 0
	for i < len(pm.Offsets) {
		c.emit(ir.Inst{Op: ir.OP_OFFSET, Arg: pm.Offsets[i]})
		c.emit(ir.Inst{Op: ir.OP_LOAD, Arg: c.target.PtrSize})
		i++
	}
	paramTypes := c.funcParamTypes[pm.Target]
	if len(paramTypes) > 0 {
		c.maybeCloneArrayForTypeName(paramTypes[0])
	}
	paramOffset := 1
	argCount := len(args) + 1
	if c.methodCallNeedsProfileParent(pm.Target) {
		paramOffset++
	}
	for i, arg := range args {
		c.compileExpr(arg)
		if i+paramOffset < len(paramTypes) {
			c.maybeConvertArgForParamType(arg, paramTypes[i+paramOffset])
			c.maybeCloneArrayForTypeName(paramTypes[i+paramOffset])
			if c.isInterfaceTypeName(paramTypes[i+paramOffset]) {
				c.maybeBoxValueForInterface(arg)
			}
		}
	}
	c.emitCallWithPanicCheck(pm.Target, argCount)
}

func (c *Compiler) emitResolvedMethodCall(node *Node, recvExpr *Node, resolvedName string) {
	fixedCount, isVariadic := c.funcVariadic[resolvedName]
	isSpread := node.Name == "spread"
	if isVariadic && !isSpread {
		c.compileExpr(recvExpr)
		paramTypes := c.funcParamTypes[resolvedName]
		if len(paramTypes) > 0 {
			c.maybeCloneArrayForTypeName(paramTypes[0])
		}
		paramOffset := 1
		if c.methodCallNeedsProfileParent(resolvedName) {
			paramOffset++
		}
		fixedArgCount := fixedCount - paramOffset
		if fixedArgCount < 0 {
			fixedArgCount = 0
		}
		i := 0
		for i < fixedArgCount && i < len(node.Nodes) {
			c.compileExpr(node.Nodes[i])
			if i+paramOffset < len(paramTypes) {
				c.maybeCloneArrayForTypeName(paramTypes[i+paramOffset])
				if c.isInterfaceTypeName(paramTypes[i+paramOffset]) {
					c.maybeBoxValueForInterface(node.Nodes[i])
				}
			}
			i++
		}
		variadicCount := len(node.Nodes) - fixedArgCount
		if variadicCount < 0 {
			variadicCount = 0
		}
		mVarElemSz := c.target.PtrSize
		if mesz, ok := c.funcVariadicElem[resolvedName]; ok {
			mVarElemSz = mesz
		}
		c.packVariadicSlice(node.Nodes, fixedArgCount, variadicCount, mVarElemSz, resolvedName)
		c.emitCallWithPanicCheck(resolvedName, fixedArgCount+2)
		return
	}
	c.emitCallWithReceiver(recvExpr, node.Nodes, resolvedName)
}

func runtimeMemBuiltinReturnCount(name string) (int, bool) {
	if name == "runtime.ReadPtr" || name == "runtime.Funcptr" {
		return 1, true
	}
	if name == "runtime.WritePtr" || name == "runtime.WriteByte" {
		return 0, true
	}
	return 0, false
}

func (c *Compiler) resolveFuncptrTarget(node *Node) (string, bool) {
	if node == nil {
		c.errorf("%s: runtime.Funcptr expects exactly one function argument", c.curFunc.Name)
		return "", false
	}
	switch node.Kind {
	case NIdent:
		if target, ok := c.localFuncTargets[node.Name]; ok {
			if len(c.localFuncCaptures[node.Name]) > 0 {
				c.errorf("%s: runtime.Funcptr does not support closures with captures (%s)", c.curFunc.Name, node.Name)
				return "", false
			}
			return target, true
		}
		if _, ok := c.localMethodTargets[node.Name]; ok {
			c.errorf("%s: runtime.Funcptr does not support bound method values (%s)", c.curFunc.Name, node.Name)
			return "", false
		}
		if c.curPkg != nil {
			if sym, ok := c.curPkg.Symbols[node.Name]; ok && sym.Kind == SymFunc {
				return c.curPkg.QualName(node.Name), true
			}
		}
	case NSelectorExpr:
		if node.X != nil && node.X.Kind == NIdent {
			if pkg := c.resolvePackage(node.X.Name); pkg != nil {
				if sym, ok := pkg.Symbols[node.Name]; ok && sym.Kind == SymFunc {
					return pkg.QualName(node.Name), true
				}
			}
		}
	case NFuncType:
		if node.Body != nil {
			if len(c.collectFuncLiteralCaptures(node)) > 0 {
				c.errorf("%s: runtime.Funcptr anonymous callbacks cannot capture local variables", c.curFunc.Name)
				return "", false
			}
			target := c.compileFuncLiteralNoCapture(node)
			return target, true
		}
	}
	c.errorf("%s: runtime.Funcptr expects a function symbol (or non-capturing function literal)", c.curFunc.Name)
	return "", false
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
	if callName == "runtime.Funcptr" && len(args) == 1 {
		target, ok := c.resolveFuncptrTarget(args[0])
		if !ok {
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			return true
		}
		c.emit(makeInst(ir.OP_CONST_I64, 0, 0, 0, "$funcaddr$"+target))
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
		structSize := c.resolveStructSize(qualified)
		if structSize > 0 {
			size = structSize
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
	c.emitKnownCall("runtime.Alloc", 1, 1)
	c.emit(ir.Inst{Op: ir.OP_DUP})
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(size)})
	c.emit(makeInst(ir.OP_CALL, 2, 0, 0, "runtime.Memzero"))
	return true
}

func (c *Compiler) emitSysWriteStringLocal(localIdx int) {
	c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 1})
	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: localIdx})
	c.emitKnownCall("runtime.Stringptr", 1, 1)
	c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: localIdx})
	c.emit(ir.Inst{Op: ir.OP_LEN})
	c.emit(makeInst(ir.OP_CONVERT, 0, 0, 0, "uintptr"))
	c.emitKnownCall("runtime.SysWrite", 3, 3)
	c.emit(ir.Inst{Op: ir.OP_DROP})
	c.emit(ir.Inst{Op: ir.OP_DROP})
	c.emit(ir.Inst{Op: ir.OP_DROP})
}

func (c *Compiler) compilePrintBuiltin(node *Node, withNewline bool) {
	tmpIdx := c.addLocal(fmt.Sprintf("$print_%d", len(c.curFunc.Locals)))
	for i, arg := range node.Nodes {
		c.compileExpr(arg)
		c.maybeBoxValueForInterface(arg)
		c.emitKnownCall("runtime.Tostring", 1, 1)
		c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: tmpIdx})
		c.emitSysWriteStringLocal(tmpIdx)
		if withNewline && i < len(node.Nodes)-1 {
			c.emit(makeInst(ir.OP_CONST_STR, 0, 0, 0, " "))
			c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: tmpIdx})
			c.emitSysWriteStringLocal(tmpIdx)
		}
	}
	if withNewline {
		c.emit(makeInst(ir.OP_CONST_STR, 0, 0, 0, "\n"))
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
	paramTypes := c.funcParamTypes[target]
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
	argCount := len(node.Nodes) + 1
	for i, arg := range node.Nodes {
		c.compileExpr(arg)
		paramIdx := i + 1
		if c.methodCallNeedsProfileParent(target) {
			paramIdx++
		}
		if paramIdx < len(paramTypes) {
			c.maybeCloneArrayForTypeName(paramTypes[paramIdx])
			if c.isInterfaceTypeName(paramTypes[paramIdx]) {
				c.maybeBoxValueForInterface(arg)
			}
		}
	}
	c.emitCallWithPanicCheck(target, argCount)
	return true
}

func (c *Compiler) callNodeForCurrentPkgTarget(target string) (*Node, bool) {
	if target == "" || c.curPkg == nil {
		return nil, false
	}
	prefix := c.curPkg.Path + "."
	if len(target) <= len(prefix) || target[0:len(prefix)] != prefix {
		return nil, false
	}
	return astIdent(target[len(prefix):len(target)]), true
}

func (c *Compiler) resolveStaticCallbackCallee(arg *Node) (*Node, bool) {
	if arg == nil {
		return nil, false
	}
	switch arg.Kind {
	case NIdent:
		if target, ok := c.localFuncTargets[arg.Name]; ok {
			return c.callNodeForCurrentPkgTarget(target)
		}
		if _, ok := c.localMethodTargets[arg.Name]; ok {
			return nil, false
		}
		if c.curPkg != nil {
			if sym, ok := c.curPkg.Symbols[arg.Name]; ok && sym.Kind == SymFunc {
				return astIdent(arg.Name), true
			}
		}
		return nil, false
	case NSelectorExpr:
		if arg.X != nil && arg.X.Kind == NIdent {
			pkg := c.resolvePackage(arg.X.Name)
			if pkg != nil {
				if sym, ok := pkg.Symbols[arg.Name]; ok && sym.Kind == SymFunc {
					return cloneTypeNode(arg), true
				}
			}
		}
		return nil, false
	case NFuncType:
		if arg.Body == nil {
			return nil, false
		}
		target := c.compileFuncLiteralNoCapture(arg)
		return c.callNodeForCurrentPkgTarget(target)
	}
	return nil, false
}

func (c *Compiler) buildRunTestHelper(callbackCallee *Node) string {
	if callbackCallee == nil {
		return ""
	}
	helper := &Node{
		Kind: NFuncType,
		Nodes: []*Node{
			{Kind: NField, Name: "name", Type: astIdent("string")},
			{Kind: NField, Name: "verbose", Type: astIdent("bool")},
		},
		Type: astIdent("bool"),
		Body: astBlock(
			astAssign(":=", astIdent("t"), astCall(astSelector("testing", "BeginTest"), astIdent("name"), astIdent("verbose"))),
			&Node{
				Kind: NDeferStmt,
				X: astCall(
					astSelector("testing", "FinishTest"),
					astIdent("t"),
					astIdent("name"),
					astIdent("verbose"),
				),
			},
			astExprStmt(astCall(cloneTypeNode(callbackCallee), astIdent("t"))),
			&Node{
				Kind: NReturn,
				X:    astUnary("!", astCall(astSelect(astIdent("t"), "Failed"))),
			},
		),
	}
	return c.compileFuncLiteralNoCapture(helper)
}

func (c *Compiler) buildRunBenchmarkHelper(callbackCallee *Node) string {
	if callbackCallee == nil {
		return ""
	}
	helper := &Node{
		Kind: NFuncType,
		Nodes: []*Node{
			{Kind: NField, Name: "name", Type: astIdent("string")},
			{Kind: NField, Name: "verbose", Type: astIdent("bool")},
		},
		Type: astIdent("bool"),
		Body: astBlock(
			astAssign(":=", astIdent("b"), astCall(astSelector("testing", "BeginBenchmark"), astIdent("name"), astIdent("verbose"))),
			&Node{
				Kind: NDeferStmt,
				X: astCall(
					astSelector("testing", "FinishBenchmark"),
					astIdent("b"),
					astIdent("name"),
					astIdent("verbose"),
				),
			},
			astExprStmt(astCall(astSelect(astIdent("b"), "ResetTimer"))),
			astExprStmt(astCall(cloneTypeNode(callbackCallee), astIdent("b"))),
			astExprStmt(astCall(astSelect(astIdent("b"), "StopTimer"))),
			astIf(
				astBinary("<=", astSelect(astIdent("b"), "N"), astInt(0)),
				astBlock(astAssign("=", astSelect(astIdent("b"), "N"), astInt(1))),
			),
			astAssign(":=", astIdent("nsPerOp"), astBinary("/", astCall(astSelect(astIdent("b"), "Elapsed")), astSelect(astIdent("b"), "N"))),
			astExprStmt(astCall(astSelector("testing", "PrintBenchmarkResult"), astIdent("name"), astSelect(astIdent("b"), "N"), astIdent("nsPerOp"))),
			&Node{
				Kind: NReturn,
				X:    astUnary("!", astCall(astSelect(astIdent("b"), "Failed"))),
			},
		),
	}
	return c.compileFuncLiteralNoCapture(helper)
}

func (c *Compiler) tryCompileTestingRunCall(node *Node, callName string) bool {
	if node == nil || callName == "" {
		return false
	}
	if callName != "testing.RunTest" && callName != "testing.RunBenchmark" {
		return false
	}
	if len(node.Nodes) != 3 {
		return false
	}
	callbackCallee, ok := c.resolveStaticCallbackCallee(node.Nodes[2])
	if !ok {
		return false
	}
	helperTarget := ""
	if callName == "testing.RunTest" {
		helperTarget = c.buildRunTestHelper(callbackCallee)
	} else {
		helperTarget = c.buildRunBenchmarkHelper(callbackCallee)
	}
	if helperTarget == "" {
		return false
	}
	c.compileExpr(node.Nodes[0])
	c.compileExpr(node.Nodes[1])
	c.emitCallWithPanicCheck(helperTarget, 2)
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
			callArgCount := len(node.Nodes) + len(captureArgs)
			for i, arg := range node.Nodes {
				c.compileExpr(arg)
				paramTypes := c.funcParamTypes[target]
				paramIdx := i + len(captureArgs)
				if paramIdx < len(paramTypes) {
					c.maybeConvertArgForParamType(arg, paramTypes[paramIdx])
				}
			}
			c.emitCallWithPanicCheck(target, callArgCount)
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
			if c.panicUnwindLabel < 0 {
				c.errorf("%s: recover is not supported in zerocall functions", c.curFunc.Name)
				c.emit(ir.Inst{Op: ir.OP_CONST_NIL})
				return
			}
			if len(node.Nodes) != 0 {
				c.errorf("%s: recover expects no arguments", c.curFunc.Name)
			}
			c.emitKnownCall("runtime.Recover", 0, 1)
			return
		}
		if name == "complex" || name == "real" || name == "imag" {
			c.errorf("%s: %s builtin is not supported (complex numbers are not implemented)", c.curFunc.Name, name)
			c.emit(ir.Inst{Op: ir.OP_CONST_NIL})
			return
		}
		if name == "len" {
			if len(node.Nodes) != 1 {
				c.errorf("%s: len expects exactly one argument", c.curFunc.Name)
				c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				return
			}
			if c.isDefinitelyInvalidLenArg(node.Nodes[0]) {
				c.errorf("%s: invalid argument to len", c.curFunc.Name)
				c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				return
			}
			if len(node.Nodes) > 0 && c.isMapExpr(node.Nodes[0]) {
				c.compileExpr(node.Nodes[0])
				c.emitKnownCall("runtime.MapLen", 1, 1)
				return
			}
			c.compileExpr(node.Nodes[0])
			c.emit(ir.Inst{Op: ir.OP_LEN})
			return
		}
		if name == "cap" {
			if len(node.Nodes) != 1 {
				c.errorf("%s: cap expects exactly one argument", c.curFunc.Name)
				c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				return
			}
			if c.isDefinitelyInvalidCapArg(node.Nodes[0]) {
				c.errorf("%s: invalid argument to cap", c.curFunc.Name)
				c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				return
			}
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
				c.emit(makeInst(ir.OP_CALL, 2, 0, 0, "runtime.MapDelete"))
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
					c.emitKnownCall("runtime.MapMake", 1, 1)
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
			if c.panicUnwindLabel < 0 {
				if len(node.Nodes) != 1 {
					c.errorf("%s: panic expects exactly one argument", c.curFunc.Name)
					c.emit(makeInst(ir.OP_CONST_STR, 0, 0, 0, "panic"))
					c.emit(ir.Inst{Op: ir.OP_PANIC})
					return
				}
				arg := node.Nodes[0]
				c.compileExpr(arg)
				if !c.isStringTypedExpr(arg) && !isStringExpr(arg) {
					c.maybeBoxValueForInterface(arg)
					c.emitKnownCall("runtime.Tostring", 1, 1)
				}
				c.emit(ir.Inst{Op: ir.OP_PANIC})
				return
			}
			if len(node.Nodes) != 1 {
				c.errorf("%s: panic expects exactly one argument", c.curFunc.Name)
				c.emit(makeInst(ir.OP_CONST_STR, 0, 0, 0, "panic"))
				c.emit(ir.Inst{Op: ir.OP_IFACE_BOX, Arg: c.typeIDForTypeName("string")})
				c.emit(makeInst(ir.OP_CONST_STR, 0, 0, 0, "panic"))
				c.emit(makeInst(ir.OP_CALL, 2, 0, 0, "runtime.PanicBegin"))
				c.emit(ir.Inst{Op: ir.OP_JMP, Arg: c.panicUnwindLabel})
				return
			}
			arg := node.Nodes[0]
			c.compileExpr(arg)
			argIdx := c.addLocal("$panic_arg")
			c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: argIdx})
			// First argument: original panic value as interface{}.
			c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: argIdx})
			c.maybeBoxValueForInterface(arg)
			// Second argument: panic text used by OP_PANIC path.
			c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: argIdx})
			if !c.isStringTypedExpr(arg) && !isStringExpr(arg) {
				argType := c.resolveExprType(arg)
				if !c.isInterfaceTypeName(argType) {
					c.maybeBoxValueForInterface(arg)
				}
				c.emitKnownCall("runtime.Tostring", 1, 1)
			}
			c.emit(makeInst(ir.OP_CALL, 2, 0, 0, "runtime.PanicBegin"))
			c.emit(ir.Inst{Op: ir.OP_JMP, Arg: c.panicUnwindLabel})
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
		if name == "int" || name == "uintptr" || name == "uint" || name == "byte" || name == "int8" || name == "uint8" || name == "string" || name == "int16" || name == "int32" || name == "int64" || name == "uint16" || name == "uint32" || name == "uint64" || name == "float32" || name == "float64" {
			arg := node.Nodes[0]
			c.compileExpr(arg)
			if name == "string" {
				if c.isExprByte(arg) {
					c.emitKnownCall("runtime.ByteToString", 1, 1)
				} else if c.isExprByteSlice(arg) {
					c.emitConvertForExpr(arg, name)
				} else if c.isStringTypedExpr(arg) {
					// string(string) is a no-op.
				} else if c.isExprIntegerLike(arg) {
					// string(int/rune) conversion.
					c.emitKnownCall("runtime.RuneToString", 1, 1)
				} else {
					// Prefer slice->string semantics unless we know this is integer-like.
					c.emitConvertForExpr(arg, name)
				}
			} else {
				c.emitConvertForExpr(arg, name)
			}
			return
		}
	}

	// Check for []byte() conversion
	if node.X != nil && node.X.Kind == NSliceType {
		c.compileExpr(node.Nodes[0])
		c.emit(makeInst(ir.OP_CONVERT, 0, 0, 0, "[]byte"))
		return
	}

	// Check for user-defined type conversions (e.g. Errno(val))
	if node.X != nil && node.X.Kind == NIdent && len(node.Nodes) == 1 {
		if _, ok := c.lookupCurrentTypeDecl(node.X.Name); ok {
			c.compileExpr(node.Nodes[0])
			targetType := c.resolveStorageTypeName(node.X.Name, 0)
			if targetType == "" {
				targetType = node.X.Name
			}
			c.emitConvertForExpr(node.Nodes[0], targetType)
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
				targetType := c.resolveStorageTypeName(impPkg.QualName(typeName), 0)
				if targetType == "" {
					targetType = typeName
				}
				c.emitConvertForExpr(node.Nodes[0], targetType)
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
				c.emitIfaceMethodCall(node.X.X, node.Nodes, ifaceType, methodName)
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
					c.emitPromotedMethodCall(node.X.X, node.Nodes, pm)
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
				c.emitResolvedMethodCall(node, node.X.X, resolvedName)
				return
			}
		}
	}

	// Check for concrete/interface method call with arbitrary receiver expressions
	// (e.g. os.Stderr.Write(...), ptr.Field.Method(...)).
	if node.X != nil && node.X.Kind == NSelectorExpr && node.X.X != nil {
		recvExpr := node.X.X
		methodName := node.X.Name

		// Skip package-qualified function calls (pkg.Func(...)).
		if !(recvExpr.Kind == NIdent && c.resolvePackage(recvExpr.Name) != nil) {
			recvType := c.resolveExprType(recvExpr)
			if recvType == "" {
				recvType = c.exprConcreteType(recvExpr)
			}
			if recvType != "" {
				if _, hasMethod := c.ifaceMethodReturnCount(recvType, methodName); hasMethod {
					c.emitIfaceMethodCall(recvExpr, node.Nodes, recvType, methodName)
					return
				}

				resolvedName, ok := c.resolveMethodByConcreteType(recvType, methodName)
				if !ok {
					if pm, found := c.findPromotedMethod(recvType, methodName); found {
						if !c.isComptimeCallAllowed(pm.Target) {
							return
						}
						c.emitPromotedMethodCall(recvExpr, node.Nodes, pm)
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
					c.emitResolvedMethodCall(node, recvExpr, resolvedName)
					return
				}
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
	if node.X != nil && node.X.Kind == NSelectorExpr && node.X.X != nil && node.X.X.Kind == NIdent {
		if c.methodFuncNames[callName] {
			if !c.isComptimeCallAllowed(callName) {
				return
			}
			if c.tryCompileComptimeCall(node, callName) {
				return
			}
			c.emitResolvedMethodCall(node, node.X.X, callName)
			return
		}
	}
	if !c.isComptimeCallAllowed(callName) {
		return
	}
	if c.emitRuntimeMemBuiltinCall(callName, node.Nodes) {
		return
	}
	if c.tryCompileComptimeCall(node, callName) {
		return
	}
	if c.tryCompileTestingRunCall(node, callName) {
		return
	}

	paramTypes := c.funcParamTypes[callName]
	profileParentOffset := 0
	if c.callNeedsProfileParent(callName) {
		profileParentOffset = 1
	}

	// Check if this is a variadic function call
	fixedCount, isVariadic := c.funcVariadic[callName]
	isSpread := node.Name == "spread"
	argCount := len(node.Nodes)
	callFixedCount := fixedCount - profileParentOffset
	if callFixedCount < 0 {
		callFixedCount = 0
	}
	if expected, ok := c.funcParams[callName]; ok {
		callExpected := expected - profileParentOffset
		if callExpected < 0 {
			callExpected = 0
		}
		if isVariadic {
			if isSpread {
				if argCount != callFixedCount+1 {
					c.errorf("%s: wrong number of arguments in variadic spread call to %s: got %d, want %d", c.curFunc.Name, callName, argCount, callFixedCount+1)
					return
				}
			} else if argCount < callFixedCount {
				c.errorf("%s: not enough arguments in call to %s: got %d, need at least %d", c.curFunc.Name, callName, argCount, callFixedCount)
				return
			}
		} else if argCount != callExpected {
			c.errorf("%s: wrong number of arguments in call to %s: got %d, want %d", c.curFunc.Name, callName, argCount, callExpected)
			return
		}
	}

	if isVariadic && !isSpread {
		// Compile fixed args normally
		i := 0
		for i < callFixedCount && i < len(node.Nodes) {
			arg := node.Nodes[i]
			c.compileExpr(arg)
			paramTypeIdx := i + profileParentOffset
			if paramTypeIdx < len(paramTypes) {
				c.maybeConvertArgForParamType(arg, paramTypes[paramTypeIdx])
				c.maybeCloneArrayForTypeName(paramTypes[paramTypeIdx])
			}
			if paramTypeIdx < len(paramTypes) && c.isInterfaceTypeName(paramTypes[paramTypeIdx]) {
				c.maybeBoxValueForInterface(arg)
			}
			i++
		}

		// Package variadic args into an inline slice
		variadicCount := len(node.Nodes) - callFixedCount
		if variadicCount < 0 {
			variadicCount = 0
		}

		varElemSz := c.target.PtrSize
		if esz, ok := c.funcVariadicElem[callName]; ok {
			varElemSz = esz
		}

		c.packVariadicSlice(node.Nodes, callFixedCount, variadicCount, varElemSz, callName)

		// Call with fixedCount + 1 args (fixed params + one slice)
		c.emitCallWithPanicCheck(callName, callFixedCount+1)
	} else {
		// Non-variadic call, or spread call — compile all args normally.
		for i, arg := range node.Nodes {
			c.compileExpr(arg)
			paramTypeIdx := i + profileParentOffset
			if paramTypeIdx < len(paramTypes) {
				c.maybeConvertArgForParamType(arg, paramTypes[paramTypeIdx])
				c.maybeCloneArrayForTypeName(paramTypes[paramTypeIdx])
			}
			// For variadic spread calls, the last arg is already a variadic
			// slice value and must not be boxed as interface{}.
			shouldBox := true
			if isVariadic && isSpread && i == len(node.Nodes)-1 {
				shouldBox = false
			}
			if shouldBox && paramTypeIdx < len(paramTypes) && c.isInterfaceTypeName(paramTypes[paramTypeIdx]) {
				c.maybeBoxValueForInterface(arg)
			}
		}
		c.emitCallWithPanicCheck(callName, argCount)
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
	savedDeferSites := c.deferSites
	savedDeferHeadLocal := c.deferHeadLocal
	savedPanicUnwindLabel := c.panicUnwindLabel
	savedPanicCheckSlowLabels := c.panicCheckSlowLabels
	savedPanicCheckSlowDepths := c.panicCheckSlowDepths
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
	savedInIfInit := c.inIfInit
	savedIfInitLeakedNames := c.ifInitLeakedNames

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
	c.deferSites = nil
	c.deferHeadLocal = -1
	c.panicUnwindLabel = -1
	c.resetPanicPropagationOutlineState()
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
	c.inIfInit = false
	c.ifInitLeakedNames = make(map[string]bool)
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
	c.deferSites = savedDeferSites
	c.deferHeadLocal = savedDeferHeadLocal
	c.panicUnwindLabel = savedPanicUnwindLabel
	c.panicCheckSlowLabels = savedPanicCheckSlowLabels
	c.panicCheckSlowDepths = savedPanicCheckSlowDepths
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
	c.inIfInit = savedInIfInit
	c.ifInitLeakedNames = savedIfInitLeakedNames

	if len(c.errors) > prevErrs {
		return "", nil, fmt.Errorf("wrapper compilation produced errors")
	}
	return wrapName, f, nil
}

func (c *Compiler) findIRFunc(name string) *ir.IRFunc {
	for _, f := range c.irmod.Funcs {
		if f != nil && f.Name == name {
			return f
		}
	}
	return nil
}

func decodeAsmFixupBytes(data []byte) ([]ir.NativeFixup, error) {
	var out []ir.NativeFixup
	i := 0
	for i < len(data) {
		if i+8 > len(data) {
			return nil, fmt.Errorf("truncated fixup record")
		}
		off := int(uint32(data[i]) | uint32(data[i+1])<<8 | uint32(data[i+2])<<16 | uint32(data[i+3])<<24)
		i = i + 4
		n := int(uint32(data[i]) | uint32(data[i+1])<<8 | uint32(data[i+2])<<16 | uint32(data[i+3])<<24)
		i = i + 4
		if n < 0 || i+n > len(data) {
			return nil, fmt.Errorf("invalid fixup target length")
		}
		target := string(data[i : i+n])
		i = i + n
		out = append(out, ir.NativeFixup{
			Kind:   ir.NativeFixupCallRel32,
			Off:    off,
			Target: target,
		})
	}
	return out, nil
}

func evalSliceBytes(eval *vm.EvalState, raw uint64) ([]byte, error) {
	if raw == 0 {
		return nil, nil
	}
	ws := uint64(vm.EvalWordSize(eval))
	dataPtr := vm.EvalLoadWord(eval, raw)
	n := int(vm.EvalLoadWord(eval, raw+ws))
	if n < 0 {
		return nil, fmt.Errorf("negative slice length")
	}
	if n == 0 || dataPtr == 0 {
		return []byte{}, nil
	}
	return vm.EvalLoadBytes(eval, dataPtr, n)
}

func (c *Compiler) buildAssembleWrapper(runtimeName string, info assembleInfo) *ir.IRFunc {
	c.comptimeSeq = c.comptimeSeq + 1
	pkgPath := runtimeName
	dot := -1
	i := 0
	for i < len(runtimeName) {
		if runtimeName[i] == '.' {
			dot = i
		}
		i = i + 1
	}
	if dot >= 0 {
		pkgPath = runtimeName[0:dot]
	}
	asmPkg := assemblePkgForArch(info.Arch)
	if asmPkg == "" {
		return nil
	}
	wrapName := fmt.Sprintf("%s.assemble$%d", pkgPath, c.comptimeSeq)
	f := &ir.IRFunc{Name: wrapName, RetCount: 2}
	f.Code = append(f.Code,
		makeInst(ir.OP_CONST_STR, 0, 0, 0, runtimeName),
		ir.Inst{Op: ir.OP_CONST_I64, Val: int64(info.Params)},
		ir.Inst{Op: ir.OP_CONST_I64, Val: int64(info.RetCount)},
		makeInst(ir.OP_CALL, 3, 0, 0, asmPkg+".__rtg_asm_begin"),
	)
	for i := 0; i < info.Params; i++ {
		f.Code = append(f.Code, ir.Inst{Op: ir.OP_CONST_I64, Val: int64(-1 - i)})
	}
	f.Code = append(f.Code,
		makeInst(ir.OP_CALL, info.Params, 0, 0, info.BuilderName),
		makeInst(ir.OP_CALL, 0, 0, 0, asmPkg+".__rtg_asm_take_code"),
		makeInst(ir.OP_CALL, 0, 0, 0, asmPkg+".__rtg_asm_take_fixups"),
		ir.Inst{Op: ir.OP_RETURN, Arg: 2},
	)
	return f
}

func (c *Compiler) compileAssembledFunctions() {
	if len(c.assembleFuncs) == 0 {
		return
	}
	if c.target.Backend != "native" {
		c.errorf("native assembled functions are not supported on backend %s", c.target.Backend)
		return
	}

	c.irmod.TypeIDs = c.typeIDs
	c.irmod.MethodTable = c.methodTable
	c.irmod.IfaceMethods = c.ifaceMethods
	c.irmod.IfaceMethodRets = c.ifaceMethodRets

	for qname, info := range c.assembleFuncs {
		if info.Arch != c.target.GOARCH {
			c.errorf("%s: assemble %s used when target is %s", qname, info.Arch, c.target.GOARCH)
			continue
		}
		if assemblePkgForArch(info.Arch) == "" {
			c.errorf("%s: unsupported assemble arch %q", qname, info.Arch)
			continue
		}
		if info.BuilderName == "" {
			continue
		}
		runtimeFunc := c.findIRFunc(qname)
		if runtimeFunc == nil {
			c.errorf("%s: assemble placeholder function missing", qname)
			continue
		}
		wrap := c.buildAssembleWrapper(qname, info)
		if wrap == nil {
			c.errorf("%s: unsupported assemble arch %q", qname, info.Arch)
			continue
		}
		c.irmod.Funcs = append(c.irmod.Funcs, wrap)
		eval, err := vm.NewEvalStateNoInit(c.target, c.irmod)
		if err != nil {
			c.irmod.Funcs = c.irmod.Funcs[0 : len(c.irmod.Funcs)-1]
			c.errorf("%s: assemble init failed: %v", qname, err)
			continue
		}
		rets, err := vm.EvalCall(eval, wrap.Name, nil, 2)
		c.irmod.Funcs = c.irmod.Funcs[0 : len(c.irmod.Funcs)-1]
		if err != nil {
			c.errorf("%s: assemble execution failed: %v", qname, err)
			continue
		}
		if len(rets) != 2 {
			c.errorf("%s: assemble execution returned %d values", qname, len(rets))
			continue
		}
		code, err := evalSliceBytes(eval, rets[0])
		if err != nil {
			c.errorf("%s: assemble code decode failed: %v", qname, err)
			continue
		}
		fixRaw, err := evalSliceBytes(eval, rets[1])
		if err != nil {
			c.errorf("%s: assemble fixup decode failed: %v", qname, err)
			continue
		}
		fixups, err := decodeAsmFixupBytes(fixRaw)
		if err != nil {
			c.errorf("%s: assemble fixup parse failed: %v", qname, err)
			continue
		}
		if len(code) == 0 {
			c.errorf("%s: assembled function produced no code (missing Ret?)", qname)
			continue
		}
		ok := true
		for _, fx := range fixups {
			if fx.Off < 0 || fx.Off+4 > len(code) {
				c.errorf("%s: native fixup offset out of bounds (%d)", qname, fx.Off)
				ok = false
			}
		}
		if !ok {
			continue
		}
		runtimeFunc.Native = &ir.NativeFunc{
			Arch:   info.Arch,
			Code:   code,
			Fixups: fixups,
		}
	}
}

func assemblePkgForArch(arch string) string {
	switch arch {
	case "amd64":
		return "j5.nz/rtg/x/asm/amd64"
	case "386":
		return "j5.nz/rtg/x/asm/i386"
	case "arm64":
		return "j5.nz/rtg/x/asm/arm64"
	default:
		return ""
	}
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
	if bits <= 8 {
		return int64(int8(raw))
	}
	if bits <= 16 {
		return int64(int16(raw))
	}
	if bits <= 32 {
		return int64(int32(raw))
	}
	return int64(raw)
}

func maskBits(raw uint64, bits int) uint64 {
	if bits <= 0 || bits >= 64 {
		return raw
	}
	if bits <= 8 {
		return uint64(uint8(raw))
	}
	if bits <= 16 {
		return uint64(uint16(raw))
	}
	if bits <= 32 {
		return uint64(uint32(raw))
	}
	return raw
}

func decimalI64(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	var u uint64
	if neg {
		u = uint64(-(v + 1))
		u = u + 1
	} else {
		u = uint64(v)
	}
	var buf [32]byte
	i := len(buf)
	for u != 0 {
		i--
		buf[i] = byte('0' + (u % 10))
		u = u / 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:len(buf)])
}

func intNode(v int64) *Node {
	return &Node{Kind: NIntLit, Name: decimalI64(v)}
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

	if aliasNode.Kind == NSliceType || aliasNode.Kind == NArrayType {
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
		if litType == nil || (litType.Kind != NSliceType && litType.Kind != NArrayType) {
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
	if typeName == "" || typeName == "string" || typeName == "int" || typeName == "bool" || typeName == "byte" ||
		typeName == "float32" || typeName == "float64" || typeName == "error" || typeName == "interface{}" {
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
	// Fixed array prefix: [N]T
	if len(typeName) > 2 && typeName[0] == '[' && typeName[1] != ']' {
		i := 1
		for i < len(typeName) && typeName[i] != ']' {
			i++
		}
		if i < len(typeName) {
			return typeName[0:i+1] + c.qualifyTypeName(typeName[i+1:len(typeName)], pkgPath)
		}
	}
	// Pointer prefix: keep * after package name to match method table format (e.g. "main.*Parser")
	if len(typeName) > 1 && typeName[0] == '*' {
		inner := typeName[1:len(typeName)]
		if inner == "string" || inner == "int" || inner == "bool" || inner == "byte" ||
			inner == "float32" || inner == "float64" ||
			inner == "int8" || inner == "uint8" || inner == "int16" || inner == "uint16" ||
			inner == "int32" || inner == "uint32" || inner == "int64" || inner == "uint64" ||
			inner == "uint" || inner == "uintptr" || inner == "error" || inner == "interface{}" {
			return "*" + inner
		}
		if strings.HasPrefix(inner, "[]") || strings.HasPrefix(inner, "[") || strings.HasPrefix(inner, "map[") || strings.HasPrefix(inner, "func(") {
			return "*" + c.qualifyTypeName(inner, pkgPath)
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

// sliceElemType extracts the element type from a slice/array type string.
func sliceElemType(typeName string) string {
	if elemType, ok := splitBracketType(typeName); ok {
		return elemType
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
	return name == "ReadPtr" || name == "WritePtr" || name == "WriteByte" || name == "Funcptr"
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
		if c.curPkg != nil {
			qname := c.curPkg.QualName(node.Name)
			if _, ok := c.funcRets[qname]; ok {
				return qname
			}
		}
		if _, ok := c.funcRets[node.Name]; ok {
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
		methodName := node.Name
		fieldName := node.X.Name
		recvType := c.resolveExprType(node.X)
		if recvType == "" {
			recvType = c.exprConcreteType(node.X)
		}
		if recvType != "" {
			if _, hasMethod := c.ifaceMethodReturnCount(recvType, methodName); hasMethod {
				return c.dotJoin(recvType, methodName)
			}
			if resolved, ok := c.resolveMethodByConcreteType(recvType, methodName); ok {
				return resolved
			}
			if pm, found := c.findPromotedMethod(recvType, methodName); found {
				return pm.Target
			}
		}
		// Legacy fallback: derive field type from root concrete receiver.
		// Walk X chain to find the root ident
		root := node.X.X
		for root != nil && root.Kind == NSelectorExpr {
			root = root.X
		}
		if root != nil && root.Kind == NIdent {
			if concreteType, ok := c.localConcreteTypes[root.Name]; ok {
				fieldType := c.resolveFieldType(concreteType, fieldName)
				if fieldType != "" {
					if _, hasMethod := c.ifaceMethodReturnCount(fieldType, methodName); hasMethod {
						return c.dotJoin(fieldType, methodName)
					}
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
		c.emitKnownCall("runtime.SliceAppendSlice", 2, 1)
	} else {
		// Append one element at a time, chaining the result
		i := 1
		for i < len(node.Nodes) {
			c.compileExpr(node.Nodes[i])
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(elemSize)})
			c.emitKnownCall("runtime.SliceAppend", 3, 1)
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
	c.emitKnownCall("runtime.SliceCopy", 2, 1)
}

func (c *Compiler) compileMake(node *Node) {
	// make([]T, len) or make([]T, len, cap) or make(map[K]V)
	if node.Nodes[0].Kind == NMapType {
		// Map creation: make(map[K]V)
		keyKind := c.mapKeyKind(node.Nodes[0].X)
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(keyKind)})
		c.emitKnownCall("runtime.MapMake", 1, 1)
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
		c.emitKnownCall("runtime.SliceMakeCap", 3, 1)
	} else {
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(elemSize)})
		c.emitKnownCall("runtime.SliceMake", 2, 1)
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
			if bname == "delete" || bname == "close" || bname == "clear" || bname == "panic" || bname == "print" || bname == "println" {
				return 0
			}
		}
		// Interface method calls: use the declared interface method signature.
		if node.X != nil && node.X.Kind == NSelectorExpr && node.X.X != nil {
			recvExpr := node.X.X
			ifaceType := ""
			if recvExpr.Kind == NIdent {
				if t, ok := c.localTypes[recvExpr.Name]; ok {
					ifaceType = t
				}
			}
			if ifaceType == "" {
				ifaceType = c.resolveExprType(recvExpr)
			}
			if ifaceType == "" {
				ifaceType = c.exprConcreteType(recvExpr)
			}
			if ifaceType != "" {
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
		// Check concrete type for slice/array elem size
		if ct, ok := c.localConcreteTypes[node.Name]; ok {
			if elemType, ok := splitBracketType(ct); ok {
				return c.typeElemSize(elemType)
			}
		}
		// Not a known slice variable — assume string indexing (elem size 1)
		return 1
	case NCallExpr:
		// Function call: resolve return type and determine elem size
		calleeName := c.resolveCallName(node.X)
		if retTypes, ok := c.funcRetTypes[calleeName]; ok && len(retTypes) > 0 {
			retType := c.qualifyTypeName(retTypes[0], "")
			if elemType, ok := splitBracketType(retType); ok {
				return c.typeElemSize(elemType)
			}
		}
		return 1
	case NIndexExpr:
		// Chained indexing: e.g., matrix[i] where matrix is [][]int
		// Determine elem size of the result of indexing the base
		if node.X != nil {
			baseCT := c.exprConcreteType(node.X)
			if resultType, ok := splitBracketType(baseCT); ok {
				if elemType, ok := splitBracketType(resultType); ok {
					return c.typeElemSize(elemType)
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
	if t := c.resolveExprType(node); t != "" {
		if elemType, ok := splitBracketType(t); ok {
			return c.typeElemSize(elemType)
		}
		if t == "string" {
			return 1
		}
	}
	if ct := c.exprConcreteType(node); ct != "" {
		if elemType, ok := splitBracketType(ct); ok {
			return c.typeElemSize(elemType)
		}
		if ct == "string" {
			return 1
		}
	}
	return 1
}

// sliceElemSize returns the element size for a slice type node.
func (c *Compiler) sliceElemSize(typeNode *Node) int {
	if typeNode == nil {
		return c.target.PtrSize
	}
	if (typeNode.Kind == NSliceType || typeNode.Kind == NArrayType) && typeNode.X != nil {
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
			if c.inAssembleBuilder {
				if sym, ok := pkg.Symbols[node.Name]; ok && sym.Kind == SymFunc {
					c.emit(makeInst(ir.OP_CONST_STR, 0, 0, 0, qname))
					return
				}
			}
			// Check if it's a precomputed constant
			if val, ok := c.constValues[qname]; ok {
				c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: val})
				return
			}
			// Check if it's a constant in the target package
			if sym, ok := pkg.Symbols[node.Name]; ok && sym.Kind == SymConst {
				if c.isConstFloatExpr(sym.Node.X) {
					c.compileExpr(sym.Node.X)
					return
				}
				val := c.resolveConstValue(sym.Node)
				c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: val})
				return
			}
			gidx, gok := c.globals[qname]
			if gok {
				c.emit(ir.Inst{Op: ir.OP_GLOBAL_GET, Arg: gidx})
				return
			}
			c.emit(makeInst(ir.OP_GLOBAL_GET, 0, 0, 0, qname))
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
	fieldType := c.resolveFieldType(recvType, node.Name)
	i := 0
	for i < len(offsets) {
		c.emit(ir.Inst{Op: ir.OP_OFFSET, Arg: offsets[i]})
		inst := ir.Inst{Op: ir.OP_LOAD, Arg: c.target.PtrSize}
		if i == len(offsets)-1 {
			inst.Arg = c.storageSizeForTypeName(fieldType)
			inst.Name = c.floatInstNameForTypeName(fieldType)
		}
		c.emit(inst)
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

	offset := 0
	for _, field := range typeNode.Nodes {
		if field.Kind != NField {
			continue
		}
		if field.Type != nil && field.Name == nodeTypeName(field.Type) {
			embeddedType := c.qualifyTypeName(nodeTypeName(field.Type), pkgPath)
			candidate := c.dotJoin(embeddedType, methodName)
			if resolved, ok := c.methodTable[candidate]; ok {
				return promotedMethodMatch{Offsets: []int{offset}, Target: resolved}, true
			}
			ptrCandidate := c.dotJoin(pointerMethodTypeName(embeddedType), methodName)
			if resolved, ok := c.methodTable[ptrCandidate]; ok {
				return promotedMethodMatch{Offsets: []int{offset}, Target: resolved}, true
			}
			if sub, ok := c.findPromotedMethodRec(embeddedType, methodName, visited); ok {
				offsets := []int{offset}
				for _, off := range sub.Offsets {
					offsets = append(offsets, off)
				}
				return promotedMethodMatch{Offsets: offsets, Target: sub.Target}, true
			}
		}
		offset = offset + c.structFieldStorageSize(field, pkgPath)
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
		c.emitKnownCall("runtime.MapGet", 2, 2)
		// MapGet returns (value, ok) — drop ok for single-value context
		// (multi-value context is handled in compileAssign)
		c.emit(ir.Inst{Op: ir.OP_DROP})
		return
	}
	if c.isDefinitelyNonIndexableExpr(node.X) {
		c.errorf("%s: cannot index non-indexable expression", c.curFunc.Name)
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
		return
	}
	elemSize := c.exprElemSize(node.X)
	if node.X != nil && node.X.Kind == NIdent {
		if idx, ok := c.lookupLocal(node.X.Name); ok && idx >= 0 && idx < len(c.curFunc.Locals) {
			switch c.curFunc.Locals[idx].FloatKind {
			case ir.TY_FLOAT32:
				elemSize = 4
			case ir.TY_FLOAT64:
				elemSize = 8
			}
		}
	}
	c.compileExpr(node.X)
	c.compileExpr(node.Y)
	c.emit(ir.Inst{Op: ir.OP_INDEX_ADDR, Arg: elemSize})
	inst := ir.Inst{Op: ir.OP_LOAD, Arg: elemSize}
	inst.Name = floatInstName(c.resolveExprType(node))
	c.emit(inst)
}

func (c *Compiler) isDefinitelyNonIndexableExpr(node *Node) bool {
	if node == nil {
		return false
	}
	if c.isMapExpr(node) || c.isStringTypedExpr(node) || isStringExpr(node) {
		return false
	}
	t := c.resolveExprType(node)
	if t == "" {
		return false
	}
	if t == "string" || strings.HasPrefix(t, "[]") || strings.HasPrefix(t, "[") || strings.HasPrefix(t, "map[") {
		return false
	}
	return true
}

func (c *Compiler) isDefinitelyInvalidLenArg(node *Node) bool {
	if node == nil {
		return true
	}
	switch node.Kind {
	case NIntLit, NRuneLit, NStringLit:
		return node.Kind != NStringLit
	}
	if c.isMapExpr(node) || c.isStringTypedExpr(node) || isStringExpr(node) {
		return false
	}
	t := c.resolveExprType(node)
	if t == "" {
		return false
	}
	if t == "string" || strings.HasPrefix(t, "[]") || strings.HasPrefix(t, "[") || strings.HasPrefix(t, "map[") {
		return false
	}
	return isDefinitelyScalarTypeName(t)
}

func (c *Compiler) isDefinitelyInvalidCapArg(node *Node) bool {
	if node == nil {
		return true
	}
	switch node.Kind {
	case NIntLit, NRuneLit, NStringLit:
		return true
	}
	if c.isMapExpr(node) || c.isStringTypedExpr(node) || isStringExpr(node) {
		return true
	}
	t := c.resolveExprType(node)
	if t == "" {
		return false
	}
	if strings.HasPrefix(t, "[]") || strings.HasPrefix(t, "[") {
		return false
	}
	if t == "string" || strings.HasPrefix(t, "map[") {
		return true
	}
	return isDefinitelyScalarTypeName(t)
}

func (c *Compiler) isDefinitelyInvalidRangeArg(node *Node) bool {
	if node == nil {
		return true
	}
	switch node.Kind {
	case NIntLit, NRuneLit:
		return true
	case NStringLit:
		return false
	}
	if c.isMapExpr(node) || c.isStringTypedExpr(node) || isStringExpr(node) {
		return false
	}
	t := c.resolveExprType(node)
	if t == "" {
		return false
	}
	if t == "string" || strings.HasPrefix(t, "[]") || strings.HasPrefix(t, "[") || strings.HasPrefix(t, "map[") {
		return false
	}
	return isDefinitelyScalarTypeName(t)
}

func isDefinitelyScalarTypeName(t string) bool {
	switch t {
	case "bool",
		"int", "int16", "int32", "int64",
		"uint", "uint16", "uint32", "uint64",
		"uintptr", "byte", "rune", "float32", "float64":
		return true
	}
	return false
}

// mapExprKeyKind returns the key kind of a map expression (0=int, 1=string, -1=not a map).
func (c *Compiler) mapExprKeyKind(node *Node) int {
	if node == nil {
		return -1
	}
	if keyType := qualifiedMapKeyTypeName(c.resolveExprType(node)); keyType != "" {
		if keyType == "string" {
			return 1
		}
		return 0
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
		if elemType, ok := splitBracketType(collType); ok && len(elemType) > 4 &&
			elemType[0] == 'm' && elemType[1] == 'a' && elemType[2] == 'p' && elemType[3] == '[' {
			// Extract key type from "[]map[K]V" or "[N]map[K]V"
			keyType := elemType[4:len(elemType)]
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
	if qualifiedMapValueTypeName(c.resolveExprType(node)) != "" {
		return true
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
		if elemType, ok := splitBracketType(collType); ok && len(elemType) > 4 &&
			elemType[0] == 'm' && elemType[1] == 'a' && elemType[2] == 'p' && elemType[3] == '[' {
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
		c.emitKnownCall("runtime.StringSlice", 3, 1)
	} else if node.Type != nil {
		c.compileExpr(node.Type)
		c.emitKnownCall("runtime.SliceResliceFull", 4, 1)
	} else {
		c.emitKnownCall("runtime.SliceReslice", 3, 1)
	}
}

func (c *Compiler) compileCompositeLit(node *Node) {
	// Handle map composite literals: map[K]V{k1: v1, k2: v2, ...}
	if node.Type != nil && node.Type.Kind == NMapType {
		keyKind := c.mapKeyKind(node.Type.X)
		valueTypeName := ""
		if node.Type.Y != nil {
			valueTypeName = nodeTypeName(node.Type.Y)
		}
		valueTypeQualified := c.qualifyTypeName(valueTypeName, "")
		valueIsInterface := c.isInterfaceTypeName(valueTypeName) || c.isInterfaceTypeName(valueTypeQualified)
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(keyKind)})
		c.emitKnownCall("runtime.MapMake", 1, 1)
		// For each key-value pair, call MapSet
		for _, elem := range node.Nodes {
			if elem.Kind == NKeyValue {
				// Stack: map_hdr
				// Dup map header, push key, push value, call MapSet
				c.emit(ir.Inst{Op: ir.OP_DUP})
				c.compileExpr(elem.X)
				c.compileExpr(elem.Y)
				if valueIsInterface {
					c.maybeBoxValueForInterface(elem.Y)
				}
				c.emitKnownCall("runtime.MapSet", 3, 1)
				c.emit(ir.Inst{Op: ir.OP_DROP}) // drop the returned header (same as input)
				// Original map_hdr still on stack
			}
		}
		return
	}

	// Handle fixed array composite literals: [N]T{...} / [...]T{...}
	if node.Type != nil && node.Type.Kind == NArrayType {
		elemSize := c.sliceElemSize(node.Type)
		arrLen := int64(0)
		if node.Type.Name == "..." {
			arrLen = int64(len(node.Nodes))
			for _, elem := range node.Nodes {
				if elem.Kind == NKeyValue && elem.X != nil {
					k := c.evalConstExprWithIota(elem.X, 0)
					if k+1 > arrLen {
						arrLen = k + 1
					}
				}
			}
		} else {
			arrLen = c.evalConstExprWithIota(node.Type.Y, 0)
			if arrLen < 0 {
				arrLen = 0
			}
		}
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: arrLen})
		c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(elemSize)})
		c.emitKnownCall("runtime.SliceMake", 2, 1)
		tmpIdx := c.addLocal("$arrlit")
		c.emit(ir.Inst{Op: ir.OP_LOCAL_SET, Arg: tmpIdx})

		nextPos := int64(0)
		for _, elem := range node.Nodes {
			elemExpr := elem
			idx := nextPos
			if elem.Kind == NKeyValue {
				elemExpr = elem.Y
				idx = c.evalConstExprWithIota(elem.X, 0)
			}
			if idx < 0 || idx >= arrLen {
				c.errorf("%s: array index %d out of bounds for length %d", c.curFunc.Name, idx, arrLen)
				continue
			}
			c.compileExpr(elemExpr)
			c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: tmpIdx})
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: idx})
			c.emit(ir.Inst{Op: ir.OP_INDEX_ADDR, Arg: elemSize})
			c.emit(ir.Inst{Op: ir.OP_STORE, Arg: elemSize})
			nextPos = idx + 1
		}
		c.emit(ir.Inst{Op: ir.OP_LOCAL_GET, Arg: tmpIdx})
		return
	}

	// Handle slice composite literals: []T{e1, e2, ...}
	if node.Type != nil && node.Type.Kind == NSliceType {
		elemSize := c.sliceElemSize(node.Type)
		if len(node.Nodes) == 0 {
			// Empty slice literal: use SliceMake with length 0
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(elemSize)})
			c.emitKnownCall("runtime.SliceMake", 2, 1)
		} else {
			// Build slice by appending each element
			c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0}) // nil slice
			for _, elem := range node.Nodes {
				c.compileExpr(elem) // push element value
				c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: int64(elemSize)})
				c.emitKnownCall("runtime.SliceAppend", 3, 1)
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
			// Push values in struct field declaration order
			for _, fname := range structFields {
				var val *Node
				found := false
				for _, elem := range node.Nodes {
					if elem.Kind == NKeyValue && elem.X != nil && elem.X.Name == fname {
						val = elem.Y
						found = true
						break
					}
				}
				if found {
					c.compileExpr(val)
					if c.structFieldIsInterface(typeName, fname) {
						c.maybeBoxValueForInterface(val)
					}
				} else {
					c.emitZeroValueForTypeName(c.resolveFieldType(typeName, fname))
				}
			}
			c.emitCompositeCall(typeName, len(structFields))
		} else {
			// Fallback: push values in literal order
			for _, elem := range node.Nodes {
				if elem.Kind == NKeyValue {
					c.compileExpr(elem.Y)
				} else {
					c.compileExpr(elem)
				}
			}
			c.emitCompositeCall(typeName, len(node.Nodes))
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
			for i < len(structFields) {
				c.emitZeroValueForTypeName(c.resolveFieldType(typeName, structFields[i]))
				i++
			}
			for i < nfields {
				c.emit(ir.Inst{Op: ir.OP_CONST_I64, Val: 0})
				i++
			}
			c.emitCompositeCall(typeName, nfields)
		} else {
			// Positional: push values in literal order
			structFields := c.getStructFields(typeName)
			for i, elem := range node.Nodes {
				c.compileExpr(elem)
				if i < len(structFields) && c.structFieldIsInterface(typeName, structFields[i]) {
					c.maybeBoxValueForInterface(elem)
				}
			}
			c.emitCompositeCall(typeName, len(node.Nodes))
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
	case NArrayType:
		lenExpr := "0"
		if node.Name == "..." {
			lenExpr = "..."
		} else if node.Y != nil {
			switch node.Y.Kind {
			case NIntLit:
				lenExpr = node.Y.Name
			case NIdent:
				lenExpr = node.Y.Name
			default:
				lenExpr = "?"
			}
		}
		return "[" + lenExpr + "]" + nodeTypeName(node.X)
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

func parseIntLiteralChecked(s string) (int64, bool) {
	if !isValidIntLiteral(s) {
		return 0, false
	}
	return parseIntLiteral(s), true
}

func isValidIntLiteral(s string) bool {
	if len(s) == 0 {
		return false
	}
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		return validDigitsWithUnderscore(s[2:len(s)], 16)
	}
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'b' || s[1] == 'B') {
		return validDigitsWithUnderscore(s[2:len(s)], 2)
	}
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'o' || s[1] == 'O') {
		return validDigitsWithUnderscore(s[2:len(s)], 8)
	}
	// Legacy leading-0 integers are octal.
	if len(s) > 1 && s[0] == '0' && s[1] >= '0' && s[1] <= '9' {
		return validDigitsWithUnderscore(s[1:len(s)], 8)
	}
	return validDigitsWithUnderscore(s, 10)
}

func isValidFloatLiteral(s string) bool {
	if len(s) == 0 {
		return false
	}
	i := 0
	digits := 0
	fracDigits := 0
	underscore := false
	for i < len(s) {
		ch := s[i]
		if ch == '_' {
			if underscore || (digits+fracDigits) == 0 {
				return false
			}
			underscore = true
			i++
			continue
		}
		if ch == '.' {
			break
		}
		if ch < '0' || ch > '9' {
			break
		}
		underscore = false
		digits++
		i++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) {
			ch := s[i]
			if ch == '_' {
				if underscore || (digits+fracDigits) == 0 {
					return false
				}
				underscore = true
				i++
				continue
			}
			if ch == 'e' || ch == 'E' {
				break
			}
			if ch < '0' || ch > '9' {
				return false
			}
			underscore = false
			fracDigits++
			i++
		}
	}
	if digits == 0 && fracDigits == 0 {
		return false
	}
	if underscore {
		return false
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		expDigits := 0
		expUnderscore := false
		for i < len(s) {
			ch := s[i]
			if ch == '_' {
				if expUnderscore || expDigits == 0 {
					return false
				}
				expUnderscore = true
				i++
				continue
			}
			if ch < '0' || ch > '9' {
				return false
			}
			expUnderscore = false
			expDigits++
			i++
		}
		if expDigits == 0 || expUnderscore {
			return false
		}
	}
	return i == len(s)
}

func validDigitsWithUnderscore(s string, base int) bool {
	if len(s) == 0 {
		return false
	}
	prevUnderscore := false
	digits := 0
	i := 0
	for i < len(s) {
		ch := s[i]
		if ch == '_' {
			if prevUnderscore || digits == 0 {
				return false
			}
			prevUnderscore = true
			i++
			continue
		}
		d := hexDigitValue(ch)
		if d < 0 || d >= base {
			return false
		}
		prevUnderscore = false
		digits++
		i++
	}
	return digits > 0 && !prevUnderscore
}

func hexDigitValue(ch byte) int {
	if ch >= '0' && ch <= '9' {
		return int(ch - '0')
	}
	if ch >= 'a' && ch <= 'f' {
		return int(ch-'a') + 10
	}
	if ch >= 'A' && ch <= 'F' {
		return int(ch-'A') + 10
	}
	return -1
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

func parseRuneLiteralChecked(s string) (int, bool) {
	if len(s) == 0 {
		return 0, false
	}
	if s[0] == '\\' {
		if len(s) == 2 {
			switch s[1] {
			case 'n':
				return 10, true
			case 't':
				return 9, true
			case 'r':
				return 13, true
			case '\\':
				return 92, true
			case '\'':
				return 39, true
			case '"':
				return 34, true
			case '0':
				return 0, true
			}
			return 0, false
		}
		if len(s) == 4 && s[1] == 'x' {
			hi := hexDigitValue(s[2])
			lo := hexDigitValue(s[3])
			if hi >= 0 && lo >= 0 {
				return hi<<4 | lo, true
			}
		}
		return 0, false
	}
	b0 := s[0]
	if b0 < 0x80 {
		if len(s) != 1 {
			return 0, false
		}
		return int(b0), true
	}
	if (b0&0xE0) == 0xC0 && len(s) == 2 {
		return int(b0&0x1F)<<6 | int(s[1]&0x3F), true
	}
	if (b0&0xF0) == 0xE0 && len(s) == 3 {
		return int(b0&0x0F)<<12 | int(s[1]&0x3F)<<6 | int(s[2]&0x3F), true
	}
	if (b0&0xF8) == 0xF0 && len(s) == 4 {
		return int(b0&0x07)<<18 | int(s[1]&0x3F)<<12 | int(s[2]&0x3F)<<6 | int(s[3]&0x3F), true
	}
	return 0, false
}

func parseRuneLiteral(s string) int {
	val, _ := parseRuneLiteralChecked(s)
	return val
}

func isValidStringLiteralContents(s string) bool {
	i := 0
	for i < len(s) {
		if s[i] != '\\' {
			i++
			continue
		}
		if i+1 >= len(s) {
			return false
		}
		switch s[i+1] {
		case 'n', 't', 'r', '\\', '"', '\'', '0':
			i += 2
		case 'x':
			if i+3 >= len(s) {
				return false
			}
			hi := hexDigitValue(s[i+2])
			lo := hexDigitValue(s[i+3])
			if hi < 0 || lo < 0 {
				return false
			}
			i += 4
		default:
			return false
		}
	}
	return true
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

// SymKind represents the kind of a symbol.
type SymKind int

const (
	SymFunc SymKind = iota
	SymType
	SymVar
	SymConst
)

// Symbol represents a named entity in a package.
type Symbol struct {
	Name       string
	Kind       SymKind
	Node       *Node
	Pkg        *Package
	Intern     string
	Embed      string
	LinkStatic bool
}

// Package represents a parsed Go package.
type Package struct {
	Name          string
	Path          string
	Dir           string
	Files         []*Node
	Imports       []string
	ImportAliases map[string]string
	Symbols       map[string]*Symbol
	Inits         []*Node
	qualNames     map[string]string // name → "Path.name"
	qualPtrNames  map[string]string // name → "Path.*name"
}

func (pkg *Package) QualName(name string) string {
	if q, ok := pkg.qualNames[name]; ok {
		return q
	}
	q := pkg.Path + "." + name
	if pkg.qualNames == nil {
		pkg.qualNames = make(map[string]string)
	}
	pkg.qualNames[name] = q
	return q
}

func (pkg *Package) QualPtrName(name string) string {
	if q, ok := pkg.qualPtrNames[name]; ok {
		return q
	}
	q := pkg.Path + ".*" + name
	if pkg.qualPtrNames == nil {
		pkg.qualPtrNames = make(map[string]string)
	}
	pkg.qualPtrNames[name] = q
	return q
}

// Module represents the complete module with all resolved packages.
type Module struct {
	BaseDir  string
	Packages map[string]*Package
	Order    []string
	Entry    *Package
}

const modulePathPrefix = "j5.nz/rtg/"

var discoveredBuildTags []string

func importPathCandidates(importPath string) []string {
	candidates := []string{importPath}
	if strings.HasPrefix(importPath, modulePathPrefix) {
		stripped := importPath[len(modulePathPrefix):]
		candidates = common.AppendUnique(candidates, stripped)
		if strings.HasPrefix(stripped, "std/") {
			candidates = common.AppendUnique(candidates, stripped[len("std/"):])
		}
	}
	return candidates
}

func ResetDiscoveredBuildTags() {
	discoveredBuildTags = nil
}

func addDiscoveredBuildTag(tag string) {
	if tag == "" || tag == "true" || tag == "false" {
		return
	}
	for _, existing := range discoveredBuildTags {
		if existing == tag {
			return
		}
	}
	discoveredBuildTags = append(discoveredBuildTags, tag)
}

func GetDiscoveredBuildTags() []string {
	var tags []string
	for _, tag := range discoveredBuildTags {
		if isKnownOS(tag) || isKnownArch(tag) {
			continue
		}
		tags = append(tags, tag)
	}
	sortStrings(tags)
	return tags
}

// ResolveModule parses entry files and recursively resolves all imports.
func ResolveModule(target *common.Target, baseDir string, entryFiles []string) *Module {
	p := &Preprocessor{
		target: target,
	}

	mod := &Module{
		BaseDir:  baseDir,
		Packages: make(map[string]*Package),
	}

	// Parse entry package.
	// If specific .go files are given, parse only those files (like go run file.go).
	// If a directory or bare package name is given, parse all .go files in it.
	entryDir := dirOfPath(entryFiles[0])
	var mainPkg *Package
	arg := entryFiles[0]
	if isGoFile(arg) {
		// Specific .go files: parse only the named files
		mainPkg = &Package{
			Name:    "main",
			Path:    "main",
			Dir:     entryDir,
			Symbols: make(map[string]*Symbol),
		}
		for _, f := range entryFiles {
			node := parseFile(f)
			if node != nil {
				mainPkg.Files = append(mainPkg.Files, node)
			}
		}
		mainPkg.Imports = collectImports(mainPkg)
	} else if arg != "." {
		// Bare package name: try embedded std first, then directory scan
		mainPkg = p.parsePackageFromStdlibSources(baseDir, arg)
		if mainPkg == nil {
			mainPkg = p.parsePackageDir(entryDir, "main")
		}
	} else {
		// "." or directory: scan the directory for all .go files
		mainPkg = p.parsePackageDir(entryDir, "main")
	}
	if mainPkg == nil {
		fmt.Fprintf(os.Stderr, "error: no Go files found in %s\n", entryDir)
		os.Exit(1)
	}
	if target.TestMode {
		injectSyntheticTestRunner(mainPkg)
	}
	mainPkg.Path = "main"
	mod.Packages["main"] = mainPkg
	mod.Entry = mainPkg

	// Worklist loop: resolve imports recursively
	var worklist []string
	for _, imp := range mainPkg.Imports {
		worklist = append(worklist, imp)
	}
	// Runtime is required by compiler-emitted helpers (alloc/map/string/etc),
	// even for programs that do not explicitly import it.
	if mainPkg.Path != "runtime" {
		hasRuntime := false
		for _, imp := range mainPkg.Imports {
			if imp == "runtime" {
				hasRuntime = true
				break
			}
		}
		if !hasRuntime {
			mainPkg.Imports = append(mainPkg.Imports, "runtime")
			worklist = append(worklist, "runtime")
		}
	}

	for len(worklist) > 0 {
		importPath := worklist[0]
		worklist = worklist[1:len(worklist)]

		_, already := mod.Packages[importPath]
		if already {
			continue
		}

		pkg := p.parsePackageFromStdlibSources(baseDir, importPath)
		if pkg == nil {
			dirs := p.resolveImportDirs(baseDir, importPath)
			if len(dirs) == 0 {
				fmt.Fprintf(os.Stderr, "error: cannot resolve import %s\n", importPath)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "error: no Go files for import %s\n", importPath)
			os.Exit(1)
		}
		mod.Packages[importPath] = pkg

		for _, imp := range pkg.Imports {
			_, seen := mod.Packages[imp]
			if !seen {
				worklist = append(worklist, imp)
			}
		}
	}

	// Topological sort
	mod.Order = topologicalSort(mod.Packages)

	// Collect symbols for each package
	for _, path := range mod.Order {
		pkg, ok := mod.Packages[path]
		if ok {
			collectSymbols(pkg)
		}
	}

	return mod
}

func ShouldUseEmbeddedStdlib(target *common.Target) bool {
	if !stdlib.HasEmbeddedStd() {
		return false
	}
	if !target.StdlibIncludeExplicit {
		return true
	}
	return target.StdlibIncludeEmbedded
}

func appendStdlibDirCandidates(candidates []string, root string, importPath string) []string {
	if root == "" {
		return candidates
	}
	root = common.TrimTrailingSlash(common.NormalizePath(root))
	if root == "" {
		return candidates
	}
	candidates = common.AppendUnique(candidates, root+"/"+importPath)
	candidates = common.AppendUnique(candidates, root+"/std/"+importPath)
	return candidates
}

// resolveImportDirs maps an import path to possible directories on disk.
func (c *Preprocessor) resolveImportDirs(baseDir string, importPath string) []string {
	var dirs []string
	importCandidates := importPathCandidates(importPath)
	if c.target.StdlibIncludeExplicit {
		for _, include := range c.target.StdlibIncludePaths {
			for _, candidate := range importCandidates {
				dirs = appendStdlibDirCandidates(dirs, include, candidate)
			}
		}
		return dirs
	}
	for _, candidate := range importCandidates {
		dirs = appendStdlibDirCandidates(dirs, baseDir, candidate)
	}
	return dirs
}

func (c *Preprocessor) parsePackageFromStdlibSources(baseDir string, importPath string) *Package {
	importCandidates := importPathCandidates(importPath)
	if ShouldUseEmbeddedStdlib(c.target) {
		for _, candidate := range importCandidates {
			pkg := c.parsePackageFromEmbed(candidate)
			if pkg != nil {
				// Preserve canonical import path for symbol qualification while
				// keeping embed-relative package directory for //go:embed patterns.
				pkg.Path = importPath
				return pkg
			}
		}
	}
	if c.target.StdlibIncludeExplicit {
		for _, include := range c.target.StdlibIncludePaths {
			for _, candidate := range importCandidates {
				dirs := appendStdlibDirCandidates(nil, include, candidate)
				for _, dir := range dirs {
					pkg := c.parsePackageDir(dir, importPath)
					if pkg != nil {
						return pkg
					}
				}
			}
		}
		return nil
	}
	for _, candidate := range importCandidates {
		dirs := appendStdlibDirCandidates(nil, baseDir, candidate)
		for _, dir := range dirs {
			pkg := c.parsePackageDir(dir, importPath)
			if pkg != nil {
				return pkg
			}
		}
	}
	return nil
}

// stringLess compares two strings lexicographically (byte-by-byte).
// This is needed because the RTG compiler's < operator does integer comparison,
// not string content comparison.
func stringLess(a string, b string) bool {
	la := len(a)
	lb := len(b)
	n := la
	if lb < n {
		n = lb
	}
	i := 0
	for i < n {
		if a[i] < b[i] {
			return true
		}
		if a[i] > b[i] {
			return false
		}
		i = i + 1
	}
	return la < lb
}

// sortStrings sorts a string slice in-place using insertion sort.
func sortStrings(s []string) {
	i := 1
	for i < len(s) {
		j := i
		for j > 0 && stringLess(s[j], s[j-1]) {
			tmp := s[j]
			s[j] = s[j-1]
			s[j-1] = tmp
			j = j - 1
		}
		i = i + 1
	}
}

// shouldIncludeFile checks if a .go file should be included based on build tags.
// If a //go:build directive exists, it takes precedence over filename-based filtering.
// Otherwise, filename-based GOOS/GOARCH conventions are used.
func (c *Preprocessor) shouldIncludeFile(path string, name string) bool {
	// 1. Check //go:build directive in file content (takes precedence)
	src, err := os.ReadFile(path)
	if err != nil {
		return true // if can't read, include by default
	}
	content := string(src)
	collectBuildTagsFromContent(content)
	collectBuildTagsFromFilename(name)

	// Scan first few lines for //go:build
	pos := 0
	for pos < len(content) {
		// Find end of line
		eol := pos
		for eol < len(content) && content[eol] != '\n' {
			eol++
		}
		line := content[pos:eol]

		// Skip blank lines and comments at top of file
		trimmed := trimLeftSpace(line)
		if len(trimmed) == 0 {
			pos = eol + 1
			continue
		}

		// Check for //go:build
		if len(trimmed) >= 11 && trimmed[0:11] == "//go:build " {
			expr := trimmed[11:len(trimmed)]
			return c.evalBuildExpr(expr)
		}

		// Check for regular comments (skip them)
		if len(trimmed) >= 2 && trimmed[0:2] == "//" {
			pos = eol + 1
			continue
		}

		// First non-comment, non-blank line — stop looking
		break
	}

	// 2. Filename-based tag filtering (only if no //go:build directive)
	// Strip .go suffix
	base := name[0 : len(name)-3]

	// Check for _GOOS_GOARCH.go, _GOOS.go, _GOARCH.go patterns
	// Find last underscore segment(s)
	parts := splitString(base, '_')
	if len(parts) >= 3 {
		// Could be name_GOOS_GOARCH.go
		maybearch := parts[len(parts)-1]
		maybeos := parts[len(parts)-2]
		if isKnownOS(maybeos) && isKnownArch(maybearch) {
			if !c.hasTag(maybeos) || !c.hasTag(maybearch) {
				return false
			}
		} else if isKnownOS(maybearch) || isKnownArch(maybearch) {
			if !c.hasTag(maybearch) {
				return false
			}
		}
	} else if len(parts) >= 2 {
		last := parts[len(parts)-1]
		if isKnownOS(last) || isKnownArch(last) {
			if !c.hasTag(last) {
				return false
			}
		}
	}

	return true
}

// splitString splits a string by a separator byte.
func splitString(s string, sep byte) []string {
	var result []string
	start := 0
	i := 0
	for i < len(s) {
		if s[i] == sep {
			result = append(result, s[start:i])
			start = i + 1
		}
		i++
	}
	result = append(result, s[start:len(s)])
	return result
}

func collectBuildTagsFromContent(content string) {
	// Scan first few lines for //go:build expression.
	pos := 0
	for pos < len(content) {
		eol := pos
		for eol < len(content) && content[eol] != '\n' {
			eol++
		}
		line := content[pos:eol]
		trimmed := trimLeftSpace(line)
		if len(trimmed) == 0 {
			pos = eol + 1
			continue
		}
		if len(trimmed) >= 11 && trimmed[0:11] == "//go:build " {
			collectBuildTagsFromExpr(trimmed[11:len(trimmed)])
			return
		}
		if len(trimmed) >= 2 && trimmed[0:2] == "//" {
			pos = eol + 1
			continue
		}
		return
	}
}

func collectBuildTagsFromExpr(expr string) {
	i := 0
	for i < len(expr) {
		if isAlphaNum(expr[i]) || expr[i] == '_' {
			start := i
			for i < len(expr) && (isAlphaNum(expr[i]) || expr[i] == '_') {
				i++
			}
			addDiscoveredBuildTag(expr[start:i])
			continue
		}
		i++
	}
}

func collectBuildTagsFromFilename(name string) {
	if !isGoFile(name) {
		return
	}
	base := name[0 : len(name)-3]
	parts := splitString(base, '_')
	if len(parts) >= 3 {
		maybearch := parts[len(parts)-1]
		maybeos := parts[len(parts)-2]
		if isKnownOS(maybeos) && isKnownArch(maybearch) {
			addDiscoveredBuildTag(maybeos)
			addDiscoveredBuildTag(maybearch)
			return
		}
	}
	if len(parts) >= 2 {
		last := parts[len(parts)-1]
		if isKnownOS(last) || isKnownArch(last) {
			addDiscoveredBuildTag(last)
		}
	}
}

// trimLeftSpace trims leading spaces and tabs.
func trimLeftSpace(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return s[i:len(s)]
}

// isKnownOS returns true if s is a known GOOS value.
func isKnownOS(s string) bool {
	return s == "linux" || s == "darwin" || s == "windows" || s == "freebsd" || s == "wasi" || s == "dos" || s == "c"
}

// isKnownArch returns true if s is a known GOARCH value.
func isKnownArch(s string) bool {
	return s == "amd64" || s == "386" || s == "arm64" || s == "arm" || s == "rv64" || s == "rv32" || s == "wasm32" || s == "dos16" || s == "c8" || s == "c16" || s == "c32" || s == "c64"
}

type Preprocessor struct {
	target *common.Target
}

// hasTag checks if a tag is in the active build tag set.
func (c *Preprocessor) hasTag(tag string) bool {
	i := 0
	for i < len(c.target.BuildTags) {
		if c.target.BuildTags[i] == tag {
			return true
		}
		i++
	}
	return false
}

// evalBuildExpr evaluates a //go:build expression against the active tag set.
// Supports: bare tags, &&, ||, !, and parentheses.
func (c *Preprocessor) evalBuildExpr(expr string) bool {
	expr = trimLeftSpace(expr)
	result, _ := c.parseBuildOr(expr)
	return result
}

// parseBuildOr parses: term (|| term)*
func (c *Preprocessor) parseBuildOr(expr string) (bool, string) {
	left, rest := c.parseBuildAnd(expr)
	for {
		rest = trimLeftSpace(rest)
		if len(rest) >= 2 && rest[0] == '|' && rest[1] == '|' {
			var right bool
			right, rest = c.parseBuildAnd(rest[2:len(rest)])
			left = left || right
		} else {
			break
		}
	}
	return left, rest
}

// parseBuildAnd parses: unary (&& unary)*
func (c *Preprocessor) parseBuildAnd(expr string) (bool, string) {
	left, rest := c.parseBuildUnary(expr)
	for {
		rest = trimLeftSpace(rest)
		if len(rest) >= 2 && rest[0] == '&' && rest[1] == '&' {
			var right bool
			right, rest = c.parseBuildUnary(rest[2:len(rest)])
			left = left && right
		} else {
			break
		}
	}
	return left, rest
}

// parseBuildUnary parses: !unary | atom
func (c *Preprocessor) parseBuildUnary(expr string) (bool, string) {
	expr = trimLeftSpace(expr)
	if len(expr) > 0 && expr[0] == '!' {
		val, rest := c.parseBuildUnary(expr[1:len(expr)])
		return !val, rest
	}
	return c.parseBuildAtom(expr)
}

// parseBuildAtom parses: (expr) | tag
func (c *Preprocessor) parseBuildAtom(expr string) (bool, string) {
	expr = trimLeftSpace(expr)
	if len(expr) > 0 && expr[0] == '(' {
		val, rest := c.parseBuildOr(expr[1:len(expr)])
		rest = trimLeftSpace(rest)
		if len(rest) > 0 && rest[0] == ')' {
			rest = rest[1:len(rest)]
		}
		return val, rest
	}
	// Parse a bare tag identifier (alphanumeric + _)
	i := 0
	for i < len(expr) && (isAlphaNum(expr[i]) || expr[i] == '_') {
		i++
	}
	if i == 0 {
		return false, expr
	}
	tag := expr[0:i]
	return c.hasTag(tag), expr[i:len(expr)]
}

// isAlphaNum returns true if c is a letter or digit.
func isAlphaNum(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// parsePackageDir lists .go files in a directory, parses each, and merges into one Package.
func (c *Preprocessor) parsePackageDir(dir string, importPath string) *Package {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	// Collect .go file names and sort for deterministic order.
	// Go's os.ReadDir sorts by name, but RTG's ReadDir returns filesystem order.
	var goFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !isGoFile(entry.Name()) {
			continue
		}
		if !c.target.TestMode && isGoTestFile(entry.Name()) {
			continue
		}
		// Check build tags before including
		if !c.shouldIncludeFile(dir+"/"+entry.Name(), entry.Name()) {
			continue
		}
		goFiles = append(goFiles, entry.Name())
	}
	sortStrings(goFiles)

	pkg := &Package{
		Path:    importPath,
		Dir:     dir,
		Symbols: make(map[string]*Symbol),
	}

	for _, name := range goFiles {
		path := dir + "/" + name
		node := parseFile(path)
		if node != nil {
			if pkg.Name == "" {
				pkg.Name = node.Name
			}
			pkg.Files = append(pkg.Files, node)
		}
	}

	if len(pkg.Files) == 0 {
		return nil
	}

	pkg.Imports = collectImports(pkg)
	return pkg
}

// dirOfPath returns the directory portion of a file path.
func dirOfPath(path string) string {
	hasSep := false
	j := 0
	for j < len(path) {
		if path[j] == '/' || path[j] == '\\' {
			hasSep = true
			break
		}
		j = j + 1
	}
	if !isGoFile(path) && hasSep {
		trimmed := strings.TrimRight(path, "/\\")
		if trimmed == "" {
			return path
		}
		return trimmed
	}
	i := len(path) - 1
	for i >= 0 {
		if path[i] == '/' || path[i] == '\\' {
			if i == 0 {
				return "/"
			}
			return path[0:i]
		}
		i = i - 1
	}
	return "."
}

// isGoFile checks if a filename ends with ".go".
func isGoFile(name string) bool {
	if len(name) < 4 {
		return false
	}
	return name[len(name)-3:len(name)] == ".go"
}

func isGoTestFile(name string) bool {
	return len(name) > 8 && strings.HasSuffix(name, "_test.go")
}

func isUpperASCII(ch byte) bool {
	return ch >= 'A' && ch <= 'Z'
}

func isTopLevelTestFuncName(name string, prefix string) bool {
	if len(name) <= len(prefix) {
		return false
	}
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	return isUpperASCII(name[len(prefix)])
}

func collectTopLevelFuncsWithPrefix(pkg *Package, prefix string) []string {
	var names []string
	seen := make(map[string]bool)
	for _, file := range pkg.Files {
		if file == nil || file.Kind != NFile {
			continue
		}
		for _, node := range file.Nodes {
			if node == nil || node.Kind != NFunc || node.X != nil {
				continue
			}
			if !isTopLevelTestFuncName(node.Name, prefix) {
				continue
			}
			if !seen[node.Name] {
				seen[node.Name] = true
				names = append(names, node.Name)
			}
		}
	}
	sortStrings(names)
	return names
}

func astIdent(name string) *Node {
	return &Node{Kind: NIdent, Name: name}
}

func astString(value string) *Node {
	return &Node{Kind: NStringLit, Name: value}
}

func astInt(value int) *Node {
	return &Node{Kind: NIntLit, Name: fmt.Sprintf("%d", value)}
}

func astSelector(base string, field string) *Node {
	return &Node{Kind: NSelectorExpr, X: astIdent(base), Name: field}
}

func astSelect(base *Node, field string) *Node {
	return &Node{Kind: NSelectorExpr, X: base, Name: field}
}

func astCall(callee *Node, args ...*Node) *Node {
	return &Node{Kind: NCallExpr, X: callee, Nodes: args}
}

func astExprStmt(expr *Node) *Node {
	return &Node{Kind: NExprStmt, X: expr}
}

func astAssign(op string, lhs *Node, rhs *Node) *Node {
	return &Node{Kind: NAssign, Name: op, X: lhs, Y: rhs}
}

func astMultiAssign(op string, lhs []*Node, rhs *Node) *Node {
	return &Node{Kind: NAssign, Name: op, Nodes: lhs, Y: rhs}
}

func astBinary(op string, left *Node, right *Node) *Node {
	return &Node{Kind: NBinaryExpr, Name: op, X: left, Y: right}
}

func astUnary(op string, expr *Node) *Node {
	return &Node{Kind: NUnaryExpr, Name: op, X: expr}
}

func astBlock(stmts ...*Node) *Node {
	var out []*Node
	for _, stmt := range stmts {
		if stmt != nil {
			out = append(out, stmt)
		}
	}
	return &Node{Kind: NBlock, Nodes: out}
}

func astIf(cond *Node, thenBlock *Node) *Node {
	return &Node{Kind: NIf, X: cond, Body: thenBlock}
}

func injectSyntheticTestRunner(pkg *Package) {
	if pkg == nil {
		return
	}
	testNames := collectTopLevelFuncsWithPrefix(pkg, "Test")
	benchNames := collectTopLevelFuncsWithPrefix(pkg, "Benchmark")
	packageName := pkg.Name
	if packageName == "" {
		packageName = "main"
	}
	var decls []*Node
	decls = append(decls, &Node{Kind: NImport, Name: "testing"})

	for _, testName := range testNames {
		fn := &Node{
			Kind: NFunc,
			Name: "__rtg_run_" + testName,
			Nodes: []*Node{
				{Kind: NField, Name: "verbose", Type: astIdent("bool")},
			},
			Type: astIdent("bool"),
			Body: astBlock(
				astAssign(":=", astIdent("t"), astCall(astSelector("testing", "BeginTest"), astString(testName), astIdent("verbose"))),
				&Node{
					Kind: NDeferStmt,
					X: astCall(
						astSelector("testing", "FinishTest"),
						astIdent("t"),
						astString(testName),
						astIdent("verbose"),
					),
				},
				astExprStmt(astCall(astIdent(testName), astIdent("t"))),
				&Node{
					Kind: NReturn,
					X:    astUnary("!", astCall(astSelect(astIdent("t"), "Failed"))),
				},
			),
		}
		decls = append(decls, fn)
	}

	for _, benchName := range benchNames {
		fn := &Node{
			Kind: NFunc,
			Name: "__rtg_bench_" + benchName,
			Nodes: []*Node{
				{Kind: NField, Name: "verbose", Type: astIdent("bool")},
			},
			Type: astIdent("bool"),
			Body: astBlock(
				astAssign(":=", astIdent("b"), astCall(astSelector("testing", "BeginBenchmark"), astString(benchName), astIdent("verbose"))),
				&Node{
					Kind: NDeferStmt,
					X: astCall(
						astSelector("testing", "FinishBenchmark"),
						astIdent("b"),
						astString(benchName),
						astIdent("verbose"),
					),
				},
				astExprStmt(astCall(astSelect(astIdent("b"), "ResetTimer"))),
				astExprStmt(astCall(astIdent(benchName), astIdent("b"))),
				astExprStmt(astCall(astSelect(astIdent("b"), "StopTimer"))),
				astIf(
					astBinary("<=", astSelect(astIdent("b"), "N"), astInt(0)),
					astBlock(astAssign("=", astSelect(astIdent("b"), "N"), astInt(1))),
				),
				astAssign(":=", astIdent("nsPerOp"), astBinary("/", astCall(astSelect(astIdent("b"), "Elapsed")), astSelect(astIdent("b"), "N"))),
				astExprStmt(astCall(astSelector("testing", "PrintBenchmarkResult"), astString(benchName), astSelect(astIdent("b"), "N"), astIdent("nsPerOp"))),
				&Node{
					Kind: NReturn,
					X:    astUnary("!", astCall(astSelect(astIdent("b"), "Failed"))),
				},
			),
		}
		decls = append(decls, fn)
	}

	var stmts []*Node
	stmts = append(stmts, astMultiAssign(":=", []*Node{astIdent("runPattern"), astIdent("benchPattern"), astIdent("verbose")}, astCall(astSelector("testing", "ParseTestArgs"))))
	stmts = append(stmts, astAssign(":=", astIdent("failures"), astInt(0)))
	stmts = append(stmts, astAssign(":=", astIdent("testsRun"), astInt(0)))
	stmts = append(stmts, astAssign(":=", astIdent("benchesRun"), astInt(0)))

	for _, testName := range testNames {
		stmts = append(stmts, astIf(
			astCall(astSelector("testing", "Match"), astString(testName), astIdent("runPattern")),
			astBlock(
				astAssign("=", astIdent("testsRun"), astBinary("+", astIdent("testsRun"), astInt(1))),
				astIf(
					astUnary("!", astCall(astIdent("__rtg_run_"+testName), astIdent("verbose"))),
					astBlock(astAssign("=", astIdent("failures"), astBinary("+", astIdent("failures"), astInt(1)))),
				),
			),
		))
	}

	for _, benchName := range benchNames {
		stmts = append(stmts, astIf(
			astBinary("&&",
				astBinary("!=", astIdent("benchPattern"), astString("")),
				astCall(astSelector("testing", "Match"), astString(benchName), astIdent("benchPattern")),
			),
			astBlock(
				astAssign("=", astIdent("benchesRun"), astBinary("+", astIdent("benchesRun"), astInt(1))),
				astIf(
					astUnary("!", astCall(astIdent("__rtg_bench_"+benchName), astIdent("verbose"))),
					astBlock(astAssign("=", astIdent("failures"), astBinary("+", astIdent("failures"), astInt(1)))),
				),
			),
		))
	}

	stmts = append(stmts, astIf(
		astBinary("!=", astIdent("failures"), astInt(0)),
		astBlock(astExprStmt(astCall(astSelector("testing", "FailAndExit"), astIdent("failures")))),
	))
	stmts = append(stmts, astExprStmt(astCall(astSelector("testing", "PassAndExit"), astIdent("verbose"), astIdent("testsRun"), astIdent("benchesRun"))))
	decls = append(decls, &Node{Kind: NFunc, Name: "init", Body: astBlock(stmts...)})

	file := &Node{Kind: NFile, Name: packageName, Nodes: decls}

	pkg.Files = append(pkg.Files, file)
	pkg.Imports = collectImports(pkg)
}

// parseFile reads, lexes, and parses a single Go source file.
func parseFile(path string) *Node {
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading %s: %v\n", path, err)
		return nil
	}

	// fmt.Fprintf(os.Stderr, "  parsing %s (%d bytes, %d tokens)...\n", path, len(src), 0)
	lexer := NewLexer(string(src))
	tokens := lexer.Tokenize()
	// fmt.Fprintf(os.Stderr, "  tokenized %s: %d tokens\n", path, len(tokens))

	parser := NewParser(tokens)
	file := parser.ParseFile()

	if len(parser.errors) > 0 {
		fmt.Fprintf(os.Stderr, "parse errors in %s:\n", path)
		for _, e := range parser.errors {
			fmt.Fprintf(os.Stderr, "  %s\n", e)
		}
		return nil
	}

	return file
}

// ParseFile is an exported wrapper used by non-frontend packages that need
// RTG-compatible parsing of Go source files with //rtg: directives.
func ParseFile(path string) *Node {
	return parseFile(path)
}

// parseSource lexes and parses source code from a string.
func parseSource(name string, src string) *Node {
	lexer := NewLexer(src)
	tokens := lexer.Tokenize()
	parser := NewParser(tokens)
	file := parser.ParseFile()

	if len(parser.errors) > 0 {
		fmt.Fprintf(os.Stderr, "parse errors in %s:\n", name)
		for _, e := range parser.errors {
			fmt.Fprintf(os.Stderr, "  %s\n", e)
		}
		return nil
	}

	return file
}

// ParseSource is an exported wrapper used by tooling that needs AST access.
func ParseSource(name string, src string) *Node {
	return parseSource(name, src)
}

// shouldIncludeContent checks if source content should be included based on build tags.
// This is like shouldIncludeFile but takes content directly instead of reading from disk.
func (c *Preprocessor) shouldIncludeContent(content string, name string) bool {
	collectBuildTagsFromContent(content)
	collectBuildTagsFromFilename(name)

	// 1. Check //go:build directive in content (takes precedence)
	pos := 0
	for pos < len(content) {
		eol := pos
		for eol < len(content) && content[eol] != '\n' {
			eol++
		}
		line := content[pos:eol]
		trimmed := trimLeftSpace(line)
		if len(trimmed) == 0 {
			pos = eol + 1
			continue
		}
		if len(trimmed) >= 11 && trimmed[0:11] == "//go:build " {
			expr := trimmed[11:len(trimmed)]
			return c.evalBuildExpr(expr)
		}
		if len(trimmed) >= 2 && trimmed[0:2] == "//" {
			pos = eol + 1
			continue
		}
		break
	}

	// 2. Filename-based tag filtering
	base := name[0 : len(name)-3]
	parts := splitString(base, '_')
	if len(parts) >= 3 {
		maybearch := parts[len(parts)-1]
		maybeos := parts[len(parts)-2]
		if isKnownOS(maybeos) && isKnownArch(maybearch) {
			if !c.hasTag(maybeos) || !c.hasTag(maybearch) {
				return false
			}
		} else if isKnownOS(maybearch) || isKnownArch(maybearch) {
			if !c.hasTag(maybearch) {
				return false
			}
		}
	} else if len(parts) >= 2 {
		last := parts[len(parts)-1]
		if isKnownOS(last) || isKnownArch(last) {
			if !c.hasTag(last) {
				return false
			}
		}
	}
	return true
}

// collectImports walks NFile.Nodes for NImport nodes and returns deduplicated import paths.
func collectImports(pkg *Package) []string {
	seen := make(map[string]bool)
	aliases := make(map[string]string)
	var result []string
	for _, file := range pkg.Files {
		for _, node := range file.Nodes {
			collectImportsFromNode(node, seen, aliases, &result)
		}
	}
	pkg.ImportAliases = aliases
	return result
}

func collectImportsFromNode(node *Node, seen map[string]bool, aliases map[string]string, result *[]string) {
	if node == nil {
		return
	}
	if node.Kind == NDeclGroup {
		for _, child := range node.Nodes {
			collectImportsFromNode(child, seen, aliases, result)
		}
		return
	}
	if node.Kind != NImport {
		return
	}
	path := node.Name
	if !seen[path] {
		seen[path] = true
		*result = append(*result, path)
	}
	if node.X != nil && node.X.Kind == NIdent {
		alias := node.X.Name
		if alias != "" && alias != "_" && alias != "." {
			aliases[alias] = path
		}
	}
}

// topologicalSort performs a DFS-based topological sort on the import graph.
type topoState struct {
	pkgs    map[string]*Package
	visited map[string]bool
	order   []string
}

func (ts *topoState) visit(path string) {
	if ts.visited[path] {
		return
	}
	ts.visited[path] = true
	pkg, ok := ts.pkgs[path]
	if ok {
		for _, imp := range pkg.Imports {
			ts.visit(imp)
		}
	}
	ts.order = append(ts.order, path)
}

func topologicalSort(pkgs map[string]*Package) []string {
	ts := &topoState{
		pkgs:    pkgs,
		visited: make(map[string]bool),
	}
	// Sort map keys to keep package visitation deterministic across runtimes.
	var paths []string
	for path := range pkgs {
		paths = append(paths, path)
	}
	sortStrings(paths)
	for _, path := range paths {
		ts.visit(path)
	}
	return ts.order
}

// collectSymbols walks top-level declarations and populates pkg.Symbols.
func collectSymbols(pkg *Package) {
	for _, file := range pkg.Files {
		for _, node := range file.Nodes {
			collectDeclSymbol(pkg, node)
		}
	}
}

// collectDeclSymbol registers a single top-level declaration as a symbol.
func collectDeclSymbol(pkg *Package, node *Node) {
	if node == nil {
		return
	}

	switch node.Kind {
	case NFunc:
		sym := &Symbol{Name: node.Name, Kind: SymFunc, Node: node, Pkg: pkg}
		pkg.Symbols[node.Name] = sym
		if node.Name == "init" {
			pkg.Inits = append(pkg.Inits, node)
		}
	case NTypeDecl:
		sym := &Symbol{Name: node.Name, Kind: SymType, Node: node, Pkg: pkg}
		pkg.Symbols[node.Name] = sym
	case NVarDecl:
		if len(node.Nodes) > 0 {
			for _, child := range node.Nodes {
				sym := &Symbol{Name: child.Name, Kind: SymVar, Node: child, Pkg: pkg}
				pkg.Symbols[child.Name] = sym
			}
		} else {
			sym := &Symbol{Name: node.Name, Kind: SymVar, Node: node, Pkg: pkg}
			pkg.Symbols[node.Name] = sym
		}
	case NConstDecl:
		if len(node.Nodes) > 0 {
			// Grouped const declaration
			for _, child := range node.Nodes {
				sym := &Symbol{Name: child.Name, Kind: SymConst, Node: child, Pkg: pkg}
				pkg.Symbols[child.Name] = sym
			}
		} else {
			sym := &Symbol{Name: node.Name, Kind: SymConst, Node: node, Pkg: pkg}
			pkg.Symbols[node.Name] = sym
		}
	case NDirective:
		// Unwrap the directive, register the inner decl, and mark intrinsic
		if node.X != nil {
			base := node.X
			collectDeclSymbol(pkg, base)
			if base.Kind == NDeclGroup {
				return
			}
			// Parse directive name for "internal FuncName"
			intern := parseInternalDirective(node.Name)
			if intern != "" && base.Name != "" {
				sym, ok := pkg.Symbols[base.Name]
				if ok {
					sym.Intern = intern
				}
			}
			// Check for embed directive
			if len(node.Name) > 6 && node.Name[0:6] == "embed " && base.Name != "" {
				sym, ok := pkg.Symbols[base.Name]
				if ok {
					sym.Embed = node.Name[6:len(node.Name)]
				}
			}
			if _, ok := parseLinkStaticDirective(node.Name); ok && base.Name != "" {
				sym, exists := pkg.Symbols[base.Name]
				if exists {
					sym.LinkStatic = true
				}
			}
		}
	case NDeclGroup:
		// Grouped declarations.
		for _, child := range node.Nodes {
			collectDeclSymbol(pkg, child)
		}
	case NImport:
		// Skip imports
	}
}

// ValidateModule checks cross-package references and returns errors.
func ValidateModule(mod *Module) []string {
	var errors []string
	for _, path := range mod.Order {
		pkg, ok := mod.Packages[path]
		if !ok {
			continue
		}
		// Build import map: package name → *Package
		importMap := make(map[string]*Package)
		for _, imp := range pkg.Imports {
			ipkg, iok := mod.Packages[imp]
			if iok {
				importMap[ipkg.Name] = ipkg
			}
		}
		for alias, imp := range pkg.ImportAliases {
			ipkg, iok := mod.Packages[imp]
			if iok {
				importMap[alias] = ipkg
			}
		}
		for _, file := range pkg.Files {
			for _, node := range file.Nodes {
				validateNode(pkg, importMap, node, &errors)
			}
		}
	}
	return errors
}

func validateNode(pkg *Package, imports map[string]*Package, node *Node, errors *[]string) {
	if node == nil {
		return
	}
	stack := make([]*Node, 0, 64)
	stack = append(stack, node)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n == nil {
			continue
		}

		// Check selector expressions: pkg.Name references
		if n.Kind == NSelectorExpr && n.X != nil && n.X.Kind == NIdent {
			target, isImport := imports[n.X.Name]
			if isImport {
				_, hasSym := target.Symbols[n.Name]
				allowRuntimeMemBuiltin := target.Path == "runtime" && isRuntimeMemBuiltinName(n.Name)
				if !hasSym && !allowRuntimeMemBuiltin {
					*errors = append(*errors, fmt.Sprintf("%s: %s.%s undefined (package %s has no symbol %s)", pkg.Path, n.X.Name, n.Name, target.Path, n.Name))
				}
			}
		}

		// Check that imported package names used as bare identifiers are valid
		if n.Kind == NCallExpr && n.X != nil && n.X.Kind == NIdent {
			name := n.X.Name
			// If calling an identifier that matches an import name, that's wrong
			_, isImport := imports[name]
			if isImport {
				*errors = append(*errors, fmt.Sprintf("%s: %s used as function (is a package name)", pkg.Path, name))
			}
		}

		if n.X != nil {
			stack = append(stack, n.X)
		}
		if n.Y != nil {
			stack = append(stack, n.Y)
		}
		if n.Body != nil {
			stack = append(stack, n.Body)
		}
		if n.Type != nil {
			stack = append(stack, n.Type)
		}
		for i := len(n.Nodes) - 1; i >= 0; i-- {
			child := n.Nodes[i]
			if child != nil {
				stack = append(stack, child)
			}
		}
	}
}

// parseInternalDirective parses a directive value like "internal Syscall"
// and returns the intrinsic name ("Syscall"), or "" if not an internal directive.
func parseInternalDirective(val string) string {
	prefix := "internal "
	if len(val) <= len(prefix) {
		return ""
	}
	if val[0:len(prefix)] != prefix {
		return ""
	}
	return val[len(prefix):len(val)]
}

func isComptimeDirective(val string) bool {
	return strings.TrimSpace(val) == "comptime"
}

func isZeroCallDirective(val string) bool {
	return strings.TrimSpace(val) == "zerocall"
}

func parseProfileDirective(val string) bool {
	return strings.TrimSpace(val) == "profile"
}

func parseNoProfileDirective(val string) bool {
	return strings.TrimSpace(val) == "noprofile"
}

// CollectProfileMethodQualNames returns qualified method names marked with //rtg:profile.
func CollectProfileMethodQualNames(mod *Module) []string {
	return collectCallableQualNames(mod, true, true)
}

// CollectMethodQualNames returns qualified method names for all methods in the module.
func CollectMethodQualNames(mod *Module) []string {
	return collectCallableQualNames(mod, true, false)
}

// CollectCallableQualNames returns qualified function and method names in the module.
func CollectCallableQualNames(mod *Module) []string {
	return collectCallableQualNames(mod, false, false)
}

func collectCallableQualNames(mod *Module, methodsOnly bool, profileOnly bool) []string {
	if mod == nil {
		return nil
	}
	var out []string
	seen := make(map[string]bool)
	for _, pkgPath := range mod.Order {
		pkg, ok := mod.Packages[pkgPath]
		if !ok {
			continue
		}
		for _, file := range pkg.Files {
			for _, node := range file.Nodes {
				base, directives := unwrapDirectiveNode(node)
				if base == nil || base.Kind != NFunc {
					continue
				}
				if profileOnly {
					hasProfile := false
					for _, d := range directives {
						if parseProfileDirective(d) {
							hasProfile = true
							break
						}
					}
					if !hasProfile {
						continue
					}
				}
				var qname string
				if base.X != nil && base.X.Type != nil {
					recvType := nodeTypeName(base.X.Type)
					if recvType == "" {
						continue
					}
					qname = pkg.QualName(recvType) + "." + base.Name
				} else {
					if methodsOnly {
						continue
					}
					qname = pkg.QualName(base.Name)
				}
				if !seen[qname] {
					seen[qname] = true
					out = append(out, qname)
				}
			}
		}
	}
	return out
}

func isCallbackDirective(val string) bool {
	return strings.TrimSpace(val) == "callback"
}

func parseAssembleDirective(val string) (string, bool) {
	prefix := "assemble "
	trimmed := strings.TrimSpace(val)
	if len(trimmed) <= len(prefix) || trimmed[0:len(prefix)] != prefix {
		return "", false
	}
	arch := strings.TrimSpace(trimmed[len(prefix):len(trimmed)])
	if arch == "" {
		return "", false
	}
	return arch, true
}

func parseTargetDirective(val string) (string, bool) {
	prefix := "target "
	trimmed := strings.TrimSpace(val)
	if len(trimmed) <= len(prefix) || trimmed[0:len(prefix)] != prefix {
		return "", false
	}
	triple := strings.TrimSpace(trimmed[len(prefix):len(trimmed)])
	if triple == "" || strings.Index(triple, "/") < 0 {
		return "", false
	}
	return triple, true
}

func parseTargetABIDirective(val string) (string, bool) {
	prefix := "targetabi "
	trimmed := strings.TrimSpace(val)
	if len(trimmed) <= len(prefix) || trimmed[0:len(prefix)] != prefix {
		return "", false
	}
	triple := strings.TrimSpace(trimmed[len(prefix):len(trimmed)])
	if triple == "" || strings.Index(triple, "/") < 0 {
		return "", false
	}
	return triple, true
}

func parseAssemblerDirective(val string) (string, bool) {
	prefix := "assembler "
	trimmed := strings.TrimSpace(val)
	if len(trimmed) <= len(prefix) || trimmed[0:len(prefix)] != prefix {
		return "", false
	}
	name := strings.TrimSpace(trimmed[len(prefix):len(trimmed)])
	if name == "" {
		return "", false
	}
	return name, true
}

func parseBinFormatDirective(val string) (string, bool) {
	prefix := "binfmt "
	trimmed := strings.TrimSpace(val)
	if len(trimmed) <= len(prefix) || trimmed[0:len(prefix)] != prefix {
		return "", false
	}
	name := strings.TrimSpace(trimmed[len(prefix):len(trimmed)])
	if name == "" {
		return "", false
	}
	return name, true
}

type CompileAsDirective struct {
	ID     string
	Target string
}

func parseDirectiveKVArgs(rest string) map[string]string {
	parts := strings.Fields(strings.TrimSpace(rest))
	values := make(map[string]string)
	for _, part := range parts {
		eq := strings.Index(part, "=")
		if eq <= 0 || eq >= len(part)-1 {
			continue
		}
		key := strings.TrimSpace(part[0:eq])
		val := strings.TrimSpace(part[eq+1:])
		if key == "" || val == "" {
			continue
		}
		values[key] = val
	}
	return values
}

func parseCompileAsDirective(val string) (CompileAsDirective, bool) {
	prefix := "compileas "
	trimmed := strings.TrimSpace(val)
	if len(trimmed) <= len(prefix) || trimmed[0:len(prefix)] != prefix {
		return CompileAsDirective{}, false
	}
	rest := strings.TrimSpace(trimmed[len(prefix):len(trimmed)])
	if rest == "" {
		return CompileAsDirective{}, false
	}
	kv := parseDirectiveKVArgs(rest)
	id := kv["id"]
	target := kv["target"]
	if id == "" || target == "" {
		return CompileAsDirective{}, false
	}
	if strings.Index(target, "/") < 0 {
		return CompileAsDirective{}, false
	}
	return CompileAsDirective{ID: id, Target: target}, true
}

func parseArtifactDirective(val string) (string, bool) {
	prefix := "artifact "
	trimmed := strings.TrimSpace(val)
	if len(trimmed) <= len(prefix) || trimmed[0:len(prefix)] != prefix {
		return "", false
	}
	rest := strings.TrimSpace(trimmed[len(prefix):len(trimmed)])
	if rest == "" {
		return "", false
	}
	kv := parseDirectiveKVArgs(rest)
	id := kv["id"]
	if id == "" {
		return "", false
	}
	return id, true
}

func funcReturnCount(node *Node) int {
	if node == nil || node.Type == nil {
		return 0
	}
	if node.Type.Kind == NFuncType {
		return len(node.Type.Nodes)
	}
	return 1
}

func isByteSliceType(node *Node) bool {
	if node == nil || node.Kind != NSliceType || node.X == nil {
		return false
	}
	return node.X.Kind == NIdent && node.X.Name == "byte"
}

type CompileAsSpec struct {
	ID          string
	Target      string
	EntryFunc   string
	ArtifactVar string
}

func collectCompileAsDirectives(pkg *Package) ([]CompileAsSpec, []CompileAsSpec, []string) {
	var compileSpecs []CompileAsSpec
	var artifactSpecs []CompileAsSpec
	var errs []string

	for _, file := range pkg.Files {
		for _, node := range file.Nodes {
			base, directives := unwrapDirectiveNode(node)
			if base == nil {
				continue
			}
			for _, d := range directives {
				if spec, ok := parseCompileAsDirective(d); ok {
					if base.Kind != NFunc {
						errs = append(errs, fmt.Sprintf("%s: line %d: //rtg:compileas must annotate a function", pkg.Path, node.Pos))
						continue
					}
					if base.X != nil {
						errs = append(errs, fmt.Sprintf("%s.%s: //rtg:compileas cannot annotate methods", pkg.Path, base.Name))
						continue
					}
					if len(base.Nodes) != 0 {
						errs = append(errs, fmt.Sprintf("%s.%s: //rtg:compileas requires zero parameters", pkg.Path, base.Name))
						continue
					}
					if funcReturnCount(base) != 0 {
						errs = append(errs, fmt.Sprintf("%s.%s: //rtg:compileas requires zero return values", pkg.Path, base.Name))
						continue
					}
					compileSpecs = append(compileSpecs, CompileAsSpec{
						ID:        spec.ID,
						Target:    spec.Target,
						EntryFunc: pkg.QualName(base.Name),
					})
				}
				if id, ok := parseArtifactDirective(d); ok {
					if base.Kind != NVarDecl {
						errs = append(errs, fmt.Sprintf("%s: line %d: //rtg:artifact must annotate a variable", pkg.Path, node.Pos))
						continue
					}
					if len(base.Nodes) != 0 || base.Name == "" {
						errs = append(errs, fmt.Sprintf("%s: line %d: //rtg:artifact must annotate a single variable declaration", pkg.Path, node.Pos))
						continue
					}
					if base.X != nil {
						errs = append(errs, fmt.Sprintf("%s.%s: //rtg:artifact variable cannot have an initializer", pkg.Path, base.Name))
						continue
					}
					if !isByteSliceType(base.Type) {
						errs = append(errs, fmt.Sprintf("%s.%s: //rtg:artifact requires type []byte", pkg.Path, base.Name))
						continue
					}
					artifactSpecs = append(artifactSpecs, CompileAsSpec{
						ID:          id,
						ArtifactVar: pkg.QualName(base.Name),
					})
				}
			}
		}
	}
	return compileSpecs, artifactSpecs, errs
}

func sortCompileAsSpecs(specs []CompileAsSpec) {
	i := 1
	for i < len(specs) {
		j := i
		for j > 0 {
			if specs[j-1].ID <= specs[j].ID {
				break
			}
			specs[j-1], specs[j] = specs[j], specs[j-1]
			j--
		}
		i++
	}
}

func CollectCompileAsSpecs(mod *Module) ([]CompileAsSpec, []string) {
	if mod == nil {
		return nil, nil
	}

	compileByID := make(map[string]CompileAsSpec)
	artifactByID := make(map[string]CompileAsSpec)
	var errs []string

	for _, pkgPath := range mod.Order {
		pkg, ok := mod.Packages[pkgPath]
		if !ok {
			continue
		}
		compileSpecs, artifactSpecs, pkgErrs := collectCompileAsDirectives(pkg)
		if len(pkgErrs) > 0 {
			errs = append(errs, pkgErrs...)
		}
		for _, spec := range compileSpecs {
			if prev, exists := compileByID[spec.ID]; exists {
				errs = append(errs, fmt.Sprintf("duplicate //rtg:compileas id=%s (%s, %s)", spec.ID, prev.EntryFunc, spec.EntryFunc))
				continue
			}
			compileByID[spec.ID] = spec
		}
		for _, spec := range artifactSpecs {
			if prev, exists := artifactByID[spec.ID]; exists {
				errs = append(errs, fmt.Sprintf("duplicate //rtg:artifact id=%s (%s, %s)", spec.ID, prev.ArtifactVar, spec.ArtifactVar))
				continue
			}
			artifactByID[spec.ID] = spec
		}
	}

	var out []CompileAsSpec
	for id, compileSpec := range compileByID {
		artifactSpec, ok := artifactByID[id]
		if !ok {
			errs = append(errs, fmt.Sprintf("//rtg:compileas id=%s is missing matching //rtg:artifact", id))
			continue
		}
		out = append(out, CompileAsSpec{
			ID:          id,
			Target:      compileSpec.Target,
			EntryFunc:   compileSpec.EntryFunc,
			ArtifactVar: artifactSpec.ArtifactVar,
		})
	}
	for id := range artifactByID {
		if _, ok := compileByID[id]; !ok {
			errs = append(errs, fmt.Sprintf("//rtg:artifact id=%s is missing matching //rtg:compileas", id))
		}
	}
	sortCompileAsSpecs(out)
	return out, errs
}

// LinkStaticDirective describes a static-link external symbol target.
type LinkStaticDirective struct {
	Library string
	Symbol  string
	Mode    string
}

// parseLinkStaticDirective parses a directive value like:
//
//	"linkstatic libSystem.dylib,_write"
//	"linkstatic libSystem.dylib,_getcwd,ptr"
//
// and returns metadata plus true on success.
func parseLinkStaticDirective(val string) (LinkStaticDirective, bool) {
	prefix := "linkstatic "
	if len(val) <= len(prefix) || val[0:len(prefix)] != prefix {
		return LinkStaticDirective{}, false
	}
	rest := strings.TrimSpace(val[len(prefix):len(val)])
	if rest == "" {
		return LinkStaticDirective{}, false
	}
	parts := strings.Split(rest, ",")
	if len(parts) < 2 || len(parts) > 3 {
		return LinkStaticDirective{}, false
	}
	lib := strings.TrimSpace(parts[0])
	sym := strings.TrimSpace(parts[1])
	mode := ""
	if len(parts) == 3 {
		mode = strings.TrimSpace(parts[2])
	}
	if lib == "" || sym == "" {
		return LinkStaticDirective{}, false
	}
	return LinkStaticDirective{Library: lib, Symbol: sym, Mode: mode}, true
}
