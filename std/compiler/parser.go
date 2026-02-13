package main

import "fmt"

// TokenKind represents the type of a lexical token.
type TokenKind int

const (
	// Literals
	TOKEN_EOF TokenKind = iota
	TOKEN_IDENT
	TOKEN_INT
	TOKEN_STRING
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
	TOKEN_OR_ASSIGN

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
)

var tokenNames = map[TokenKind]string{
	TOKEN_EOF: "EOF", TOKEN_IDENT: "IDENT", TOKEN_INT: "INT",
	TOKEN_STRING: "STRING", TOKEN_RUNE: "RUNE", TOKEN_COMMENT: "COMMENT",
	TOKEN_PACKAGE: "package", TOKEN_IMPORT: "import", TOKEN_FUNC: "func",
	TOKEN_TYPE: "type", TOKEN_STRUCT: "struct", TOKEN_INTERFACE: "interface",
	TOKEN_VAR: "var", TOKEN_CONST: "const", TOKEN_IF: "if", TOKEN_ELSE: "else",
	TOKEN_FOR: "for", TOKEN_RANGE: "range", TOKEN_SWITCH: "switch",
	TOKEN_CASE: "case", TOKEN_DEFAULT: "default", TOKEN_RETURN: "return",
	TOKEN_BREAK: "break", TOKEN_CONTINUE: "continue", TOKEN_MAP: "map",
	TOKEN_NIL: "nil", TOKEN_TRUE: "true", TOKEN_FALSE: "false",
	TOKEN_DEFER: "defer", TOKEN_IOTA: "iota",
	TOKEN_PLUS: "+", TOKEN_MINUS: "-", TOKEN_STAR: "*", TOKEN_SLASH: "/",
	TOKEN_PERCENT: "%", TOKEN_EQ: "==", TOKEN_NEQ: "!=",
	TOKEN_LT: "<", TOKEN_GT: ">", TOKEN_LEQ: "<=", TOKEN_GEQ: ">=",
	TOKEN_AND: "&&", TOKEN_OR: "||", TOKEN_NOT: "!",
	TOKEN_AMPERSAND: "&", TOKEN_PIPE: "|", TOKEN_CARET: "^",
	TOKEN_SHL: "<<", TOKEN_SHR: ">>",
	TOKEN_ASSIGN: "=", TOKEN_DEFINE: ":=", TOKEN_PLUS_ASSIGN: "+=", TOKEN_OR_ASSIGN: "|=",
	TOKEN_LPAREN: "(", TOKEN_RPAREN: ")", TOKEN_LBRACE: "{", TOKEN_RBRACE: "}",
	TOKEN_LBRACK: "[", TOKEN_RBRACK: "]", TOKEN_COMMA: ",", TOKEN_DOT: ".",
	TOKEN_COLON: ":", TOKEN_SEMICOLON: ";", TOKEN_ELLIPSIS: "...",
	TOKEN_INC:       "++",
	TOKEN_DIRECTIVE: "directive",
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
	src  []byte
	pos  int
	line int
	col  int
}

func NewLexer(src []byte) *Lexer {
	return &Lexer{src: src, pos: 0, line: 1, col: 1}
}

func (l *Lexer) atEnd() bool {
	return l.pos >= len(l.src)
}

func (l *Lexer) peek() byte {
	if l.atEnd() {
		return 0
	}
	return l.src[l.pos]
}

func (l *Lexer) peekAt(offset int) byte {
	p := l.pos + offset
	if p >= len(l.src) {
		return 0
	}
	return l.src[p]
}

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
	if kind == TOKEN_IDENT || kind == TOKEN_INT || kind == TOKEN_STRING || kind == TOKEN_RUNE {
		return true
	}
	if kind == TOKEN_RPAREN || kind == TOKEN_RBRACK || kind == TOKEN_RBRACE {
		return true
	}
	if kind == TOKEN_INC || kind == TOKEN_BREAK || kind == TOKEN_CONTINUE || kind == TOKEN_RETURN {
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

func (l *Lexer) skipWhitespaceAndComments() (bool, *Token) {
	sawNewline := false
	var directive *Token
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
			for !l.atEnd() && l.peek() != '\n' {
				l.advance()
			}
			val := string(l.src[start:l.pos])
			if len(val) >= 4 && val[0:4] == "rtg:" {
				directive = &Token{Kind: TOKEN_DIRECTIVE, Val: val[4:len(val)], Line: cLine, Col: cCol}
			} else if len(val) >= 9 && val[0:9] == "go:embed " {
				directive = &Token{Kind: TOKEN_DIRECTIVE, Val: "embed " + val[9:len(val)], Line: cLine, Col: cCol}
			}
			sawNewline = true
		} else {
			break
		}
	}
	return sawNewline, directive
}

func (l *Lexer) scanIdent() Token {
	line := l.line
	col := l.col
	start := l.pos
	for !l.atEnd() && (isLetter(l.peek()) || isDigit(l.peek())) {
		l.advance()
	}
	val := string(l.src[start:l.pos])
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
	if l.peek() == '0' && l.peekAt(1) == 'x' {
		l.advance()
		l.advance()
		for !l.atEnd() && (isDigit(l.peek()) || (l.peek() >= 'a' && l.peek() <= 'f') || (l.peek() >= 'A' && l.peek() <= 'F')) {
			l.advance()
		}
	} else {
		for !l.atEnd() && isDigit(l.peek()) {
			l.advance()
		}
	}
	return Token{Kind: TOKEN_INT, Val: string(l.src[start:l.pos]), Line: line, Col: col}
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
	val := string(l.src[start:l.pos])
	if !l.atEnd() {
		l.advance() // skip closing "
	}
	return Token{Kind: TOKEN_STRING, Val: val, Line: line, Col: col}
}

func (l *Lexer) scanRune() Token {
	line := l.line
	col := l.col
	l.advance() // skip opening '
	start := l.pos
	if l.peek() == '\\' {
		l.advance()
	}
	l.advance()
	val := string(l.src[start:l.pos])
	if !l.atEnd() {
		l.advance() // skip closing '
	}
	return Token{Kind: TOKEN_RUNE, Val: val, Line: line, Col: col}
}

func (l *Lexer) Tokenize() []Token {
	var tokens []Token
	lastKind := TOKEN_EOF
	for {
		sawNewline, directive := l.skipWhitespaceAndComments()
		if sawNewline && needsSemicolon(lastKind) {
			tokens = append(tokens, Token{Kind: TOKEN_SEMICOLON, Val: "", Line: l.line, Col: l.col})
			lastKind = TOKEN_SEMICOLON
		}
		if directive != nil {
			tok := Token{Kind: directive.Kind, Val: directive.Val, Line: directive.Line, Col: directive.Col}
			tokens = append(tokens, tok)
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
		if isLetter(ch) {
			tok = l.scanIdent()
		} else if isDigit(ch) {
			tok = l.scanNumber()
		} else if ch == '"' {
			tok = l.scanString()
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
		return Token{Kind: TOKEN_MINUS, Line: line, Col: col}
	case '*':
		return Token{Kind: TOKEN_STAR, Line: line, Col: col}
	case '/':
		return Token{Kind: TOKEN_SLASH, Line: line, Col: col}
	case '%':
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
			return Token{Kind: TOKEN_SHR, Line: line, Col: col}
		}
		return Token{Kind: TOKEN_GT, Line: line, Col: col}
	case '&':
		if l.peek() == '&' {
			l.advance()
			return Token{Kind: TOKEN_AND, Line: line, Col: col}
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
	NStringLit
	NRuneLit
	NBasicLit
	NBinaryExpr
	NUnaryExpr
	NCallExpr
	NIndexExpr
	NSelectorExpr
	NCompositeLit
	NKeyValue
	NPointerType
	NSliceType
	NMapType
	NFuncType
	NStructType
	NInterfaceType
	NIncStmt
	NDeferStmt
	NSliceExpr
	NDirective
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
	tok := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return tok
}

func (p *Parser) at(kind TokenKind) bool {
	if p.pos >= len(p.tokens) {
		return TOKEN_EOF == kind
	}
	return p.tokens[p.pos].Kind == kind
}

func (p *Parser) match(kinds ...TokenKind) bool {
	k := TOKEN_EOF
	if p.pos < len(p.tokens) {
		k = p.tokens[p.pos].Kind
	}
	for _, kind := range kinds {
		if k == kind {
			return true
		}
	}
	return false
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
		imports := p.parseImportGroup()
		file.Nodes = append(file.Nodes, imports...)
	}

	// top-level declarations
	for !p.at(TOKEN_EOF) {
		decl := p.parseTopDecl()
		if decl != nil {
			file.Nodes = append(file.Nodes, decl)
		}
	}

	return file
}

func (p *Parser) parseImportGroup() []*Node {
	p.expect(TOKEN_IMPORT)
	var imports []*Node
	if p.at(TOKEN_LPAREN) {
		p.advance()
		for !p.at(TOKEN_RPAREN) && !p.at(TOKEN_EOF) {
			tok := p.expect(TOKEN_STRING)
			imports = append(imports, &Node{Kind: NImport, Name: tok.Val, Pos: tok.Line})
			p.skipSemicolon()
		}
		p.expect(TOKEN_RPAREN)
	} else {
		tok := p.expect(TOKEN_STRING)
		imports = append(imports, &Node{Kind: NImport, Name: tok.Val, Pos: tok.Line})
	}
	p.skipSemicolon()
	return imports
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

	// parameters
	node.Nodes = p.parseParamList()

	// result type(s)
	if !p.at(TOKEN_LBRACE) && !p.at(TOKEN_SEMICOLON) && !p.at(TOKEN_EOF) {
		if p.at(TOKEN_LPAREN) {
			// multiple return values (possibly named)
			node.Type = &Node{Kind: NFuncType}
			p.advance()
			for !p.at(TOKEN_RPAREN) && !p.at(TOKEN_EOF) {
				t := p.parseParam()
				node.Type.Nodes = append(node.Type.Nodes, t)
				if p.at(TOKEN_COMMA) {
					p.advance()
				}
			}
			p.expect(TOKEN_RPAREN)
		} else {
			node.Type = p.parseType()
		}
	}

	// body
	if p.at(TOKEN_LBRACE) {
		node.Body = p.parseBlock()
	}
	p.skipSemicolon()
	return node
}

func (p *Parser) parseReceiver() *Node {
	node := &Node{Kind: NField, Pos: p.peek().Line}
	name := p.expect(TOKEN_IDENT)
	node.Name = name.Val
	node.Type = p.parseType()
	return node
}

func (p *Parser) parseParamList() []*Node {
	p.expect(TOKEN_LPAREN)
	var params []*Node
	for !p.at(TOKEN_RPAREN) && !p.at(TOKEN_EOF) {
		param := p.parseParam()
		params = append(params, param)
		if p.at(TOKEN_COMMA) {
			p.advance()
		}
	}
	p.expect(TOKEN_RPAREN)

	// Fix grouped parameters: (a, b int) → two NField nodes each with type int.
	// parseParam sees "a" followed by comma and treats it as a type-only param.
	// If a later param has both name and type, preceding type-only params whose
	// "type" is a bare ident are actually names sharing that type.
	var result []*Node
	i := 0
	for i < len(params) {
		if params[i].Name != "" || i+1 >= len(params) {
			// Already has a name, or last param — keep as-is
			result = append(result, params[i])
			i = i + 1
			continue
		}
		// params[i] has no name. Check if it's a bare ident that might be a
		// grouped name. Collect consecutive unnamed bare-ident params.
		groupStart := i
		for i < len(params) && params[i].Name == "" && params[i].Type != nil && params[i].Type.Kind == NIdent {
			if i+1 < len(params) && params[i+1].Name != "" && params[i+1].Type != nil {
				// Next param has name+type — this group of bare idents are names
				i = i + 1
				break
			}
			if i+1 >= len(params) {
				// Last param — it's a real type-only param, not a grouped name
				break
			}
			i = i + 1
		}
		if i < len(params) && params[i].Name != "" && params[i].Type != nil {
			// The params from groupStart..i-1 are names sharing params[i]'s type
			j := groupStart
			for j < i {
				node := &Node{Kind: NField, Pos: params[j].Pos}
				node.Name = params[j].Type.Name // the "type" was actually the name
				node.Type = params[i].Type
				result = append(result, node)
				j = j + 1
			}
			result = append(result, params[i])
			i = i + 1
		} else {
			// Not a group — emit as-is
			j := groupStart
			for j <= i && j < len(params) {
				result = append(result, params[j])
				j = j + 1
			}
			i = j
		}
	}
	return result
}

func (p *Parser) parseParam() *Node {
	node := &Node{Kind: NField, Pos: p.peek().Line}
	// Check if this is "name type" or just "type"
	if p.at(TOKEN_IDENT) && p.pos+1 < len(p.tokens) {
		next := p.tokens[p.pos+1]
		if next.Kind != TOKEN_COMMA && next.Kind != TOKEN_RPAREN {
			name := p.advance()
			node.Name = name.Val
			if p.at(TOKEN_ELLIPSIS) {
				p.advance()
				node.Name = "..." + node.Name
			}
			node.Type = p.parseType()
			return node
		}
	}
	node.Type = p.parseType()
	return node
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
			node.Type = p.parseType()
			decls = append(decls, node)
			p.skipSemicolon()
		}
		p.expect(TOKEN_RPAREN)
		p.skipSemicolon()
		if len(decls) == 1 {
			return decls[0]
		}
		group := &Node{Kind: NBlock, Nodes: decls, Pos: pos}
		return group
	}

	name := p.expect(TOKEN_IDENT)
	node := &Node{Kind: NTypeDecl, Name: name.Val, Pos: pos}
	node.Type = p.parseType()
	p.skipSemicolon()
	return node
}

func (p *Parser) parseVarDecl() *Node {
	pos := p.peek().Line
	p.expect(TOKEN_VAR)
	name := p.expect(TOKEN_IDENT)
	node := &Node{Kind: NVarDecl, Name: name.Val, Pos: pos}
	if !p.at(TOKEN_ASSIGN) && !p.at(TOKEN_SEMICOLON) && !p.at(TOKEN_EOF) {
		node.Type = p.parseType()
	}
	if p.at(TOKEN_ASSIGN) {
		p.advance()
		node.X = p.parseExpr()
	}
	p.skipSemicolon()
	return node
}

func (p *Parser) parseConstDecl() *Node {
	pos := p.peek().Line
	p.expect(TOKEN_CONST)
	if p.at(TOKEN_LPAREN) {
		p.advance()
		group := &Node{Kind: NConstDecl, Pos: pos}
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
			group.Nodes = append(group.Nodes, spec)
			p.skipSemicolon()
		}
		p.expect(TOKEN_RPAREN)
		p.skipSemicolon()
		return group
	}
	name := p.expect(TOKEN_IDENT)
	node := &Node{Kind: NConstDecl, Name: name.Val, Pos: pos}
	if p.at(TOKEN_ASSIGN) {
		p.advance()
		node.X = p.parseExpr()
	}
	p.skipSemicolon()
	return node
}

// Type parsing

func (p *Parser) parseType() *Node {
	switch p.peek().Kind {
	case TOKEN_IDENT:
		tok := p.advance()
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
	}
	tok := p.advance()
	p.errorf("expected type, got %s at line %d", tok.String(), tok.Line)
	return &Node{Kind: NIdent, Name: "error", Pos: tok.Line}
}

func (p *Parser) parseSliceOrArrayType() *Node {
	pos := p.peek().Line
	p.expect(TOKEN_LBRACK)
	p.expect(TOKEN_RBRACK)
	elem := p.parseType()
	return &Node{Kind: NSliceType, X: elem, Pos: pos}
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
	node.Nodes = p.parseParamList()
	// optional return type
	if !p.at(TOKEN_SEMICOLON) && !p.at(TOKEN_COMMA) && !p.at(TOKEN_RPAREN) && !p.at(TOKEN_LBRACE) && !p.at(TOKEN_EOF) {
		node.Type = p.parseType()
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
		meth.Nodes = p.parseParamList()
		// Parse return type(s)
		if !p.at(TOKEN_SEMICOLON) && !p.at(TOKEN_RBRACE) && !p.at(TOKEN_EOF) {
			if p.at(TOKEN_LPAREN) {
				meth.Type = &Node{Kind: NFuncType}
				p.advance()
				for !p.at(TOKEN_RPAREN) && !p.at(TOKEN_EOF) {
					t := p.parseParam()
					meth.Type.Nodes = append(meth.Type.Nodes, t)
					if p.at(TOKEN_COMMA) {
						p.advance()
					}
				}
				p.expect(TOKEN_RPAREN)
			} else {
				meth.Type = p.parseType()
			}
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
	block := &Node{Kind: NBlock, Pos: pos}
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
	switch p.peek().Kind {
	case TOKEN_IF:
		return p.parseIfStmt()
	case TOKEN_FOR:
		return p.parseForStmt()
	case TOKEN_SWITCH:
		return p.parseSwitchStmt()
	case TOKEN_RETURN:
		return p.parseReturnStmt()
	case TOKEN_VAR:
		return p.parseVarDecl()
	case TOKEN_CONST:
		return p.parseConstDecl()
	case TOKEN_BREAK:
		pos := p.peek().Line
		p.advance()
		p.skipSemicolon()
		return &Node{Kind: NBranch, Name: "break", Pos: pos}
	case TOKEN_CONTINUE:
		pos := p.peek().Line
		p.advance()
		p.skipSemicolon()
		return &Node{Kind: NBranch, Name: "continue", Pos: pos}
	case TOKEN_DEFER:
		return p.parseDeferStmt()
	case TOKEN_SEMICOLON:
		p.advance()
		return nil
	}
	return p.parseSimpleStmt()
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
	p.skipSemicolon()
	return node
}

func (p *Parser) parseForStmt() *Node {
	pos := p.peek().Line
	p.expect(TOKEN_FOR)
	node := &Node{Kind: NFor, Pos: pos}

	// Check for bare "for {"
	if p.at(TOKEN_LBRACE) {
		node.Body = p.parseBlock()
		p.skipSemicolon()
		return node
	}

	// Try to detect range-based for loop
	// Patterns: for _, x := range y { ... } or for i := range y { ... }
	// or for range y { ... }
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
			p.skipSemicolon()
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
			p.skipSemicolon()
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
		p.skipSemicolon()
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
		p.skipSemicolon()
		return node
	}

	// Simple condition for loop: for cond { ... }
	node.Y = first
	node.Body = p.parseBlock()
	p.skipSemicolon()
	return node
}

func (p *Parser) parseSwitchStmt() *Node {
	pos := p.peek().Line
	p.expect(TOKEN_SWITCH)
	node := &Node{Kind: NSwitch, Pos: pos}

	// Optional tag expression
	if !p.at(TOKEN_LBRACE) {
		tag := p.parseExprNoBrace()
		if p.at(TOKEN_SEMICOLON) {
			// It was an init statement
			p.advance()
			node.X = tag
			if !p.at(TOKEN_LBRACE) {
				node.Y = p.parseExprNoBrace()
			}
		} else {
			node.Y = tag
		}
	}

	p.expect(TOKEN_LBRACE)
	for !p.at(TOKEN_RBRACE) && !p.at(TOKEN_EOF) {
		c := p.parseCaseClause()
		node.Nodes = append(node.Nodes, c)
	}
	p.expect(TOKEN_RBRACE)
	p.skipSemicolon()
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
	if !p.at(TOKEN_SEMICOLON) && !p.at(TOKEN_RBRACE) && !p.at(TOKEN_EOF) {
		node.X = p.parseExpr()
		for p.at(TOKEN_COMMA) {
			p.advance()
			node.Nodes = append(node.Nodes, p.parseExpr())
		}
	}
	p.skipSemicolon()
	return node
}

func (p *Parser) parseDeferStmt() *Node {
	pos := p.peek().Line
	p.expect(TOKEN_DEFER)
	expr := p.parseExpr()
	p.skipSemicolon()
	return &Node{Kind: NDeferStmt, X: expr, Pos: pos}
}

func (p *Parser) parseSimpleStmt() *Node {
	node := p.parseSimpleStmtNoSemicolon()
	p.skipSemicolon()
	return node
}

func (p *Parser) parseSimpleStmtNoSemicolon() *Node {
	expr := p.parseExpr()

	// Check for increment
	if p.at(TOKEN_INC) {
		p.advance()
		return &Node{Kind: NIncStmt, X: expr, Pos: expr.Pos}
	}

	// Check for assignment / short var decl
	if p.match(TOKEN_ASSIGN, TOKEN_DEFINE, TOKEN_PLUS_ASSIGN, TOKEN_OR_ASSIGN) {
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
		if p.match(TOKEN_ASSIGN, TOKEN_DEFINE) {
			op := p.advance()
			rhs := p.parseExpr()
			node := &Node{Kind: NAssign, Name: tokenVal(op), Y: rhs, Pos: expr.Pos}
			node.Nodes = lhs
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
	if p.at(TOKEN_NOT) || p.at(TOKEN_MINUS) || p.at(TOKEN_CARET) {
		op := p.advance()
		expr := p.parseUnaryExpr()
		return &Node{Kind: NUnaryExpr, Name: tokenVal(op), X: expr, Pos: op.Line}
	}
	if p.at(TOKEN_STAR) {
		op := p.advance()
		expr := p.parseUnaryExpr()
		return &Node{Kind: NUnaryExpr, Name: "*", X: expr, Pos: op.Line}
	}
	if p.at(TOKEN_AMPERSAND) {
		op := p.advance()
		expr := p.parseUnaryExpr()
		return &Node{Kind: NUnaryExpr, Name: "&", X: expr, Pos: op.Line}
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
	case TOKEN_STRING:
		tok := p.advance()
		node = &Node{Kind: NStringLit, Name: tok.Val, Pos: tok.Line}
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
	default:
		tok := p.advance()
		p.errorf("unexpected token in expression: %s at line %d col %d", tok.String(), tok.Line, tok.Col)
		return &Node{Kind: NIdent, Name: "error", Pos: tok.Line}
	}
	return p.parsePostfixOps(node)
}

func (p *Parser) isTypeLikeNode(node *Node) bool {
	if node.Kind == NIdent || node.Kind == NSliceType || node.Kind == NMapType || node.Kind == NPointerType {
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
			p.advance()
			name := p.expect(TOKEN_IDENT)
			node = &Node{Kind: NSelectorExpr, X: node, Name: name.Val, Pos: node.Pos}
		case TOKEN_LPAREN:
			p.advance()
			call := &Node{Kind: NCallExpr, X: node, Pos: node.Pos}
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
			node = call
		case TOKEN_LBRACK:
			p.advance()
			index := p.parseExpr()
			if p.at(TOKEN_COLON) {
				p.advance()
				var hi *Node
				if !p.at(TOKEN_RBRACK) {
					hi = p.parseExpr()
				}
				p.expect(TOKEN_RBRACK)
				node = &Node{Kind: NSliceExpr, X: node, Y: index, Body: hi, Pos: node.Pos}
			} else {
				p.expect(TOKEN_RBRACK)
				node = &Node{Kind: NIndexExpr, X: node, Y: index, Pos: node.Pos}
			}
		case TOKEN_LBRACE:
			if !p.noCompLit && p.isTypeLikeNode(node) {
				node = p.parseCompositeLit(node)
			} else {
				return node
			}
		default:
			return node
		}
	}
}

func (p *Parser) parseCompositeLit(typeNode *Node) *Node {
	pos := typeNode.Pos
	p.expect(TOKEN_LBRACE)
	node := &Node{Kind: NCompositeLit, Type: typeNode, Pos: pos}
	for !p.at(TOKEN_RBRACE) && !p.at(TOKEN_EOF) {
		val := p.parseExpr()
		if p.at(TOKEN_COLON) {
			p.advance()
			v := p.parseExpr()
			kv := &Node{Kind: NKeyValue, X: val, Y: v, Pos: val.Pos}
			node.Nodes = append(node.Nodes, kv)
		} else {
			node.Nodes = append(node.Nodes, val)
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
