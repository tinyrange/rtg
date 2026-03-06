package frontend

import "fmt"

// Parser parses preprocessed C token streams into a lightweight AST.
type Parser struct {
	tokens []Token
	pos    int
	errors []string
}

func NewParser(tokens []Token) *Parser {
	return &Parser{tokens: tokens}
}

func (p *Parser) Errors() []string {
	return p.errors
}

func (p *Parser) errorf(tok Token, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if tok.File != "" {
		msg = fmt.Sprintf("%s:%d:%d: %s", tok.File, tok.Line, tok.Col, msg)
	}
	p.errors = append(p.errors, msg)
}

func (p *Parser) atEndRaw() bool {
	return p.pos >= len(p.tokens)
}

func (p *Parser) peekRaw() Token {
	if p.atEndRaw() {
		return Token{Kind: TokEOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) advanceRaw() Token {
	t := p.peekRaw()
	if !p.atEndRaw() {
		p.pos++
	}
	return t
}

func (p *Parser) skipNewlines() {
	for !p.atEndRaw() {
		t := p.peekRaw()
		if t.Kind != TokNewline {
			break
		}
		p.pos++
	}
}

func (p *Parser) atEnd() bool {
	p.skipNewlines()
	if p.atEndRaw() {
		return true
	}
	return p.peekRaw().Kind == TokEOF
}

func (p *Parser) peek() Token {
	p.skipNewlines()
	return p.peekRaw()
}

func (p *Parser) advance() Token {
	p.skipNewlines()
	return p.advanceRaw()
}

func (p *Parser) matchPunct(op string) bool {
	p.skipNewlines()
	if p.atEndRaw() {
		return false
	}
	t := p.peekRaw()
	if t.Kind == TokPunct && t.Text == op {
		p.pos++
		return true
	}
	return false
}

func (p *Parser) matchIdent(name string) bool {
	p.skipNewlines()
	if p.atEndRaw() {
		return false
	}
	t := p.peekRaw()
	if t.Kind == TokIdent && t.Text == name {
		p.pos++
		return true
	}
	return false
}

func (p *Parser) expectPunct(op string, ctx string) bool {
	if p.matchPunct(op) {
		return true
	}
	t := p.peek()
	p.errorf(t, "expected %q %s", op, ctx)
	return false
}

func isKeyword(t Token, kw string) bool {
	return t.Kind == TokIdent && t.Text == kw
}

func isDeclarationKeyword(tok Token) bool {
	if tok.Kind != TokIdent {
		return false
	}
	switch tok.Text {
	case "auto", "register", "static", "extern", "typedef":
		return true
	case "const", "volatile", "restrict", "inline":
		return true
	case "void", "char", "short", "int", "long", "float", "double", "signed", "unsigned", "_Bool", "_Complex", "_Imaginary":
		return true
	case "struct", "union", "enum":
		return true
	default:
		return false
	}
}

func filterNonNewline(tokens []Token) []Token {
	out := make([]Token, 0, len(tokens))
	for _, t := range tokens {
		if t.Kind != TokNewline && t.Kind != TokEOF {
			out = append(out, t)
		}
	}
	return out
}

func tokenSliceText(tokens []Token) string {
	tokens = filterNonNewline(tokens)
	if len(tokens) == 0 {
		return ""
	}
	var out string
	for i, t := range tokens {
		if i > 0 {
			out = out + " "
		}
		out = out + t.Text
	}
	return out
}

func (p *Parser) collectRange(start int, end int) []Token {
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	if end > len(p.tokens) {
		end = len(p.tokens)
	}
	return filterNonNewline(p.tokens[start:end])
}

func (p *Parser) recoverToStmtBoundary() {
	depthParen := 0
	depthBracket := 0
	for !p.atEndRaw() {
		t := p.peekRaw()
		if t.Kind == TokNewline {
			p.pos++
			continue
		}
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
			case ";":
				if depthParen == 0 && depthBracket == 0 {
					p.pos++
					return
				}
			case "}":
				if depthParen == 0 && depthBracket == 0 {
					return
				}
			}
		}
		p.pos++
	}
}

func (p *Parser) consumeBalancedUntil(terminator string) ([]Token, bool) {
	start := p.pos
	depthParen := 0
	depthBracket := 0
	depthBrace := 0
	for !p.atEndRaw() {
		t := p.peekRaw()
		if t.Kind == TokNewline {
			p.pos++
			continue
		}
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
			if t.Text == terminator && depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
				body := p.collectRange(start, p.pos)
				p.pos++
				return body, true
			}
		}
		p.pos++
	}
	return p.collectRange(start, p.pos), false
}

func (p *Parser) consumeStatementText() ([]Token, bool) {
	start := p.pos
	depthParen := 0
	depthBracket := 0
	depthBrace := 0
	for !p.atEndRaw() {
		t := p.peekRaw()
		if t.Kind == TokNewline {
			p.pos++
			continue
		}
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
					break
				}
				if depthParen == 0 && depthBracket == 0 {
					return p.collectRange(start, p.pos), true
				}
			case ";":
				if depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
					body := p.collectRange(start, p.pos)
					p.pos++
					return body, true
				}
			}
		}
		p.pos++
	}
	return p.collectRange(start, p.pos), false
}

func (p *Parser) parseParenText(ctx string) (string, bool) {
	if !p.expectPunct("(", ctx) {
		return "", false
	}
	start := p.pos
	depth := 1
	for !p.atEndRaw() {
		t := p.advanceRaw()
		if t.Kind == TokPunct {
			if t.Text == "(" {
				depth++
			} else if t.Text == ")" {
				depth--
				if depth == 0 {
					end := p.pos - 1
					return tokenSliceText(p.collectRange(start, end)), true
				}
			}
		}
	}
	t := p.peekRaw()
	p.errorf(t, "unterminated parenthesized expression in %s", ctx)
	return tokenSliceText(p.collectRange(start, p.pos)), false
}

func prevSignificant(tokens []Token) Token {
	i := len(tokens) - 1
	for i >= 0 {
		t := tokens[i]
		if t.Kind != TokNewline {
			return t
		}
		i--
	}
	return Token{Kind: TokEOF}
}

func looksLikeFunctionDefHead(tokens []Token) bool {
	tokens = filterNonNewline(tokens)
	if len(tokens) == 0 {
		return false
	}
	if hasTopLevelPunct(tokens, "=") {
		return false
	}
	prev := prevSignificant(tokens)
	if prev.Kind == TokPunct && prev.Text == ")" {
		return true
	}
	if prev.Kind == TokPunct && prev.Text == ";" {
		lpar := topLevelPunctIndex(tokens, "(")
		if lpar <= 0 {
			return false
		}
		rpar := matchingParenClose(tokens, lpar)
		if rpar <= lpar {
			return false
		}
		return rpar < len(tokens)-1
	}
	return false
}

func knrFunctionNameList(tokens []Token) (int, int, bool) {
	tokens = filterNonNewline(tokens)
	lpar := topLevelPunctIndex(tokens, "(")
	if lpar <= 0 || lpar >= len(tokens) {
		return -1, -1, false
	}
	if tokens[lpar-1].Kind != TokIdent || isDeclarationKeyword(tokens[lpar-1]) {
		return -1, -1, false
	}
	rpar := matchingParenClose(tokens, lpar)
	if rpar <= lpar {
		return -1, -1, false
	}
	parts := splitTopLevel(tokens[lpar+1:rpar], ",")
	if len(parts) == 0 {
		return -1, -1, false
	}
	for _, p := range parts {
		p = filterNonNewline(p)
		if len(p) == 0 {
			continue
		}
		if len(p) != 1 || p[0].Kind != TokIdent || isDeclarationKeyword(p[0]) {
			return -1, -1, false
		}
	}
	return lpar, rpar, true
}

func looksLikeKNRFunctionHeadPrefix(tokens []Token) bool {
	tokens = filterNonNewline(tokens)
	if len(tokens) == 0 || hasTopLevelPunct(tokens, "=") {
		return false
	}
	_, rpar, ok := knrFunctionNameList(tokens)
	if !ok {
		return false
	}
	return rpar < len(tokens)-1
}

// ParseTranslationUnit parses a whole C translation unit.
func (p *Parser) ParseTranslationUnit() *Node {
	root := &Node{Kind: NTranslationUnit}
	for !p.atEnd() {
		n := p.parseExternalDecl()
		if n != nil {
			root.Children = append(root.Children, n)
		} else {
			p.recoverToStmtBoundary()
		}
	}
	return root
}

func (p *Parser) parseExternalDecl() *Node {
	p.skipNewlines()
	if p.atEnd() {
		return nil
	}
	start := p.pos
	startTok := p.peekRaw()
	depthParen := 0
	depthBracket := 0
	depthBrace := 0
	for !p.atEndRaw() {
		t := p.peekRaw()
		if t.Kind == TokEOF {
			break
		}
		if t.Kind == TokNewline {
			p.pos++
			continue
		}
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
				if depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
					head := p.collectRange(start, p.pos)
					if looksLikeFunctionDefHead(head) {
						fn := &Node{Kind: NFunctionDef, Text: tokenSliceText(head), Line: startTok.Line, Col: startTok.Col}
						body := p.parseCompoundStmt()
						if body != nil {
							fn.Children = append(fn.Children, body)
						}
						return fn
					}
				}
				depthBrace++
			case "}":
				if depthBrace > 0 {
					depthBrace--
				}
			case ";":
				if depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
					head := p.collectRange(start, p.pos)
					if looksLikeKNRFunctionHeadPrefix(head) {
						p.pos++
						continue
					}
					p.pos++
					decl := &Node{Kind: NExternalDecl, Text: tokenSliceText(p.collectRange(start, p.pos-1)), Line: startTok.Line, Col: startTok.Col}
					return decl
				}
			}
		}
		p.pos++
	}
	if start < p.pos {
		decl := &Node{Kind: NExternalDecl, Text: tokenSliceText(p.collectRange(start, p.pos)), Line: startTok.Line, Col: startTok.Col}
		return decl
	}
	p.errorf(startTok, "unable to parse external declaration")
	return nil
}

func (p *Parser) parseCompoundStmt() *Node {
	if !p.matchPunct("{") {
		t := p.peek()
		p.errorf(t, "expected '{' to start compound statement")
		return nil
	}
	n := &Node{Kind: NCompoundStmt}
	for !p.atEndRaw() {
		p.skipNewlines()
		if p.matchPunct("}") {
			return n
		}
		if p.atEnd() {
			break
		}
		stmt := p.parseStatement()
		if stmt != nil {
			n.Children = append(n.Children, stmt)
		} else {
			p.recoverToStmtBoundary()
		}
	}
	t := p.peekRaw()
	p.errorf(t, "unterminated compound statement")
	return n
}

func (p *Parser) parseStatement() *Node {
	p.skipNewlines()
	t := p.peekRaw()
	if t.Kind == TokEOF {
		return nil
	}
	if t.Kind == TokPunct && t.Text == "{" {
		return p.parseCompoundStmt()
	}
	if t.Kind == TokPunct && t.Text == ";" {
		p.pos++
		return &Node{Kind: NEmptyStmt, Line: t.Line, Col: t.Col}
	}
	if t.Kind == TokIdent {
		switch t.Text {
		case "if":
			return p.parseIfStmt()
		case "for":
			return p.parseForStmt()
		case "while":
			return p.parseWhileStmt()
		case "do":
			return p.parseDoWhileStmt()
		case "switch":
			return p.parseSwitchStmt()
		case "case":
			return p.parseCaseStmt()
		case "default":
			return p.parseDefaultStmt()
		case "return":
			return p.parseReturnStmt()
		case "break":
			return p.parseSimpleKeywordStmt(NBreakStmt, "break")
		case "continue":
			return p.parseSimpleKeywordStmt(NContinueStmt, "continue")
		case "goto":
			return p.parseGotoStmt()
		}
		next := p.nextSignificantRaw(1)
		if next.Kind == TokPunct && next.Text == ":" {
			return p.parseLabelStmt()
		}
		if isDeclarationKeyword(t) {
			return p.parseDeclOrExprStmt(true)
		}
	}
	return p.parseDeclOrExprStmt(false)
}

func (p *Parser) nextSignificantRaw(offset int) Token {
	i := p.pos + offset
	for i < len(p.tokens) {
		t := p.tokens[i]
		if t.Kind != TokNewline {
			return t
		}
		i++
	}
	return Token{Kind: TokEOF}
}

func (p *Parser) parseIfStmt() *Node {
	start := p.advance()
	cond, _ := p.parseParenText("after if")
	n := &Node{Kind: NIfStmt, Text: cond, Line: start.Line, Col: start.Col}
	thenStmt := p.parseStatement()
	if thenStmt != nil {
		n.Children = append(n.Children, thenStmt)
	}
	if p.matchIdent("else") {
		elseStmt := p.parseStatement()
		if elseStmt != nil {
			n.Children = append(n.Children, elseStmt)
		}
	}
	return n
}

func (p *Parser) parseForStmt() *Node {
	start := p.advance()
	head, _ := p.parseParenText("after for")
	n := &Node{Kind: NForStmt, Text: head, Line: start.Line, Col: start.Col}
	body := p.parseStatement()
	if body != nil {
		n.Children = append(n.Children, body)
	}
	return n
}

func (p *Parser) parseWhileStmt() *Node {
	start := p.advance()
	cond, _ := p.parseParenText("after while")
	n := &Node{Kind: NWhileStmt, Text: cond, Line: start.Line, Col: start.Col}
	body := p.parseStatement()
	if body != nil {
		n.Children = append(n.Children, body)
	}
	return n
}

func (p *Parser) parseDoWhileStmt() *Node {
	start := p.advance()
	n := &Node{Kind: NDoWhileStmt, Line: start.Line, Col: start.Col}
	body := p.parseStatement()
	if body != nil {
		n.Children = append(n.Children, body)
	}
	if !p.matchIdent("while") {
		t := p.peek()
		p.errorf(t, "expected 'while' after do-body")
		return n
	}
	cond, _ := p.parseParenText("after do-while")
	n.Text = cond
	if !p.expectPunct(";", "after do-while") {
		p.recoverToStmtBoundary()
	}
	return n
}

func (p *Parser) parseSwitchStmt() *Node {
	start := p.advance()
	cond, _ := p.parseParenText("after switch")
	n := &Node{Kind: NSwitchStmt, Text: cond, Line: start.Line, Col: start.Col}
	body := p.parseStatement()
	if body != nil {
		n.Children = append(n.Children, body)
	}
	return n
}

func (p *Parser) parseCaseStmt() *Node {
	start := p.advance()
	body, ok := p.consumeBalancedUntil(":")
	if !ok {
		t := p.peekRaw()
		p.errorf(t, "expected ':' after case label")
	}
	n := &Node{Kind: NCaseStmt, Text: tokenSliceText(body), Line: start.Line, Col: start.Col}
	stmt := p.parseStatement()
	if stmt != nil {
		n.Children = append(n.Children, stmt)
	}
	return n
}

func (p *Parser) parseDefaultStmt() *Node {
	start := p.advance()
	if !p.expectPunct(":", "after default") {
		p.recoverToStmtBoundary()
	}
	n := &Node{Kind: NDefaultStmt, Line: start.Line, Col: start.Col}
	stmt := p.parseStatement()
	if stmt != nil {
		n.Children = append(n.Children, stmt)
	}
	return n
}

func (p *Parser) parseReturnStmt() *Node {
	start := p.advance()
	body, ok := p.consumeBalancedUntil(";")
	if !ok {
		t := p.peekRaw()
		p.errorf(t, "expected ';' after return statement")
	}
	return &Node{Kind: NReturnStmt, Text: tokenSliceText(body), Line: start.Line, Col: start.Col}
}

func (p *Parser) parseSimpleKeywordStmt(kind NodeKind, kw string) *Node {
	start := p.advance()
	if !p.expectPunct(";", "after "+kw) {
		p.recoverToStmtBoundary()
	}
	return &Node{Kind: kind, Line: start.Line, Col: start.Col}
}

func (p *Parser) parseGotoStmt() *Node {
	start := p.advance()
	p.skipNewlines()
	t := p.peekRaw()
	label := ""
	if t.Kind == TokIdent {
		label = t.Text
		p.pos++
	} else {
		p.errorf(t, "expected label after goto")
	}
	if !p.expectPunct(";", "after goto") {
		p.recoverToStmtBoundary()
	}
	return &Node{Kind: NGotoStmt, Text: label, Line: start.Line, Col: start.Col}
}

func (p *Parser) parseLabelStmt() *Node {
	start := p.advance()
	label := start.Text
	if !p.expectPunct(":", "after label") {
		p.recoverToStmtBoundary()
	}
	n := &Node{Kind: NLabelStmt, Text: label, Line: start.Line, Col: start.Col}
	stmt := p.parseStatement()
	if stmt != nil {
		n.Children = append(n.Children, stmt)
	}
	return n
}

func (p *Parser) parseDeclOrExprStmt(forceDecl bool) *Node {
	startTok := p.peekRaw()
	start := p.pos
	body, ok := p.consumeStatementText()
	if !ok {
		t := p.peekRaw()
		p.errorf(t, "expected ';' to terminate statement")
	}
	text := tokenSliceText(body)
	if text == "" {
		return &Node{Kind: NEmptyStmt, Line: startTok.Line, Col: startTok.Col}
	}
	if forceDecl {
		return &Node{Kind: NDeclStmt, Text: text, Line: startTok.Line, Col: startTok.Col}
	}
	if looksLikeDeclaration(body) {
		return &Node{Kind: NDeclStmt, Text: text, Line: startTok.Line, Col: startTok.Col}
	}
	if start == p.pos {
		return nil
	}
	return &Node{Kind: NExprStmt, Text: text, Line: startTok.Line, Col: startTok.Col}
}

func looksLikeDeclaration(body []Token) bool {
	if len(body) == 0 {
		return false
	}
	first := body[0]
	if isDeclarationKeyword(first) {
		return true
	}
	// Heuristic: type-like leading identifier followed by * or another identifier,
	// and no obvious assignment before declarator terminates.
	if first.Kind == TokIdent {
		if len(body) >= 2 {
			second := body[1]
			if second.Kind == TokPunct && second.Text == "*" {
				return true
			}
			if second.Kind == TokIdent {
				return true
			}
		}
	}
	return false
}
