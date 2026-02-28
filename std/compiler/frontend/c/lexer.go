package frontend

import "fmt"

// Lexer tokenizes C preprocessing tokens.
type Lexer struct {
	file        string
	src         string
	pos         int
	line        int
	col         int
	startOfLine bool
}

func NewLexer(file string, src string) *Lexer {
	return &Lexer{
		file:        file,
		src:         normalizeSource(src),
		pos:         0,
		line:        1,
		col:         1,
		startOfLine: true,
	}
}

func normalizeSource(src string) string {
	// Normalize CRLF/CR to LF.
	out := make([]byte, 0, len(src))
	i := 0
	for i < len(src) {
		ch := src[i]
		if ch == '\r' {
			if i+1 < len(src) && src[i+1] == '\n' {
				i++
			}
			out = append(out, '\n')
			i++
			continue
		}
		out = append(out, ch)
		i++
	}

	// Remove escaped newlines (line splices).
	spliced := make([]byte, 0, len(out))
	i = 0
	for i < len(out) {
		if out[i] == '\\' && i+1 < len(out) && out[i+1] == '\n' {
			i += 2
			continue
		}
		spliced = append(spliced, out[i])
		i++
	}
	return string(spliced)
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
	if p < 0 || p >= len(l.src) {
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
		l.startOfLine = true
	} else {
		l.col++
	}
	return ch
}

func isAlpha(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isHexDigit(ch byte) bool {
	return isDigit(ch) || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

func isIdentStart(ch byte) bool {
	return isAlpha(ch) || ch == '_'
}

func isIdentContinue(ch byte) bool {
	return isIdentStart(ch) || isDigit(ch)
}

func (l *Lexer) scanIdent() Token {
	line := l.line
	col := l.col
	start := l.pos
	for !l.atEnd() && isIdentContinue(l.peek()) {
		l.advance()
	}
	return Token{Kind: TokIdent, Text: l.src[start:l.pos], File: l.file, Line: line, Col: col}
}

func (l *Lexer) scanNumber() Token {
	line := l.line
	col := l.col
	start := l.pos

	prev := byte(0)
	for !l.atEnd() {
		ch := l.peek()
		if isIdentContinue(ch) || ch == '.' {
			prev = ch
			l.advance()
			continue
		}
		if (ch == '+' || ch == '-') && (prev == 'e' || prev == 'E' || prev == 'p' || prev == 'P') {
			prev = ch
			l.advance()
			continue
		}
		break
	}

	return Token{Kind: TokNumber, Text: l.src[start:l.pos], File: l.file, Line: line, Col: col}
}

func (l *Lexer) scanQuoted(prefix string, delim byte, kind TokenKind) (Token, error) {
	line := l.line
	col := l.col - len(prefix) - 1
	start := l.pos - len(prefix) - 1
	for !l.atEnd() {
		ch := l.advance()
		if ch == '\\' {
			if l.atEnd() {
				break
			}
			l.advance()
			continue
		}
		if ch == delim {
			return Token{Kind: kind, Text: l.src[start:l.pos], File: l.file, Line: line, Col: col}, nil
		}
		if ch == '\n' {
			return Token{}, fmt.Errorf("%s:%d:%d: unterminated literal", l.file, line, col)
		}
	}
	return Token{}, fmt.Errorf("%s:%d:%d: unterminated literal", l.file, line, col)
}

func (l *Lexer) scanPunct() (Token, error) {
	line := l.line
	col := l.col

	multi := []string{
		"...",
		"<<=", ">>=",
		"->", "++", "--", "<<", ">>", "<=", ">=", "==", "!=", "&&", "||",
		"*=", "/=", "%=", "+=", "-=", "&=", "^=", "|=",
		"##",
	}
	for _, p := range multi {
		if len(l.src)-l.pos >= len(p) && l.src[l.pos:l.pos+len(p)] == p {
			l.pos += len(p)
			l.col += len(p)
			l.startOfLine = false
			return Token{Kind: TokPunct, Text: p, File: l.file, Line: line, Col: col}, nil
		}
	}

	ch := l.peek()
	single := "(){}[];:,?~!%^&*-=+|<>./#"
	i := 0
	for i < len(single) {
		if single[i] == ch {
			l.advance()
			return Token{Kind: TokPunct, Text: string(ch), File: l.file, Line: line, Col: col}, nil
		}
		i++
	}
	return Token{}, fmt.Errorf("%s:%d:%d: unexpected byte %q", l.file, line, col, ch)
}

func (l *Lexer) tokenizeOne(leadingSpace bool, startOfLine bool) (Token, error) {
	if l.atEnd() {
		return Token{Kind: TokEOF, File: l.file, Line: l.line, Col: l.col, LeadingSpace: leadingSpace, StartOfLine: startOfLine}, nil
	}
	ch := l.peek()

	if isDigit(ch) || (ch == '.' && isDigit(l.peekAt(1))) {
		tok := l.scanNumber()
		tok.LeadingSpace = leadingSpace
		tok.StartOfLine = startOfLine
		return tok, nil
	}

	if isIdentStart(ch) {
		// Prefixed string/char literals: L"...", u"...", U"...", u8"..."
		if ch == 'u' && l.peekAt(1) == '8' && (l.peekAt(2) == '"' || l.peekAt(2) == '\'') {
			prefixStart := l.pos
			l.advance()
			l.advance()
			delim := l.peek()
			l.advance()
			kind := TokString
			if delim == '\'' {
				kind = TokChar
			}
			tok, err := l.scanQuoted(l.src[prefixStart:l.pos], delim, kind)
			if err != nil {
				return Token{}, err
			}
			tok.LeadingSpace = leadingSpace
			tok.StartOfLine = startOfLine
			return tok, nil
		}
		if (ch == 'L' || ch == 'u' || ch == 'U') && (l.peekAt(1) == '"' || l.peekAt(1) == '\'') {
			prefixStart := l.pos
			l.advance()
			delim := l.peek()
			l.advance()
			kind := TokString
			if delim == '\'' {
				kind = TokChar
			}
			tok, err := l.scanQuoted(l.src[prefixStart:l.pos], delim, kind)
			if err != nil {
				return Token{}, err
			}
			tok.LeadingSpace = leadingSpace
			tok.StartOfLine = startOfLine
			return tok, nil
		}
		tok := l.scanIdent()
		tok.LeadingSpace = leadingSpace
		tok.StartOfLine = startOfLine
		return tok, nil
	}

	if ch == '"' || ch == '\'' {
		delim := ch
		l.advance()
		kind := TokString
		if delim == '\'' {
			kind = TokChar
		}
		tok, err := l.scanQuoted("", delim, kind)
		if err != nil {
			return Token{}, err
		}
		tok.LeadingSpace = leadingSpace
		tok.StartOfLine = startOfLine
		return tok, nil
	}

	tok, err := l.scanPunct()
	if err != nil {
		return Token{}, err
	}
	tok.LeadingSpace = leadingSpace
	tok.StartOfLine = startOfLine
	return tok, nil
}

// Tokenize tokenizes the whole input.
func (l *Lexer) Tokenize() ([]Token, error) {
	var out []Token
	leadingSpace := false
	for !l.atEnd() {
		ch := l.peek()
		if ch == ' ' || ch == '\t' || ch == 11 || ch == 12 || ch == '\r' {
			leadingSpace = true
			l.advance()
			continue
		}
		if ch == '\n' {
			tok := Token{Kind: TokNewline, Text: "\n", File: l.file, Line: l.line, Col: l.col, StartOfLine: true}
			out = append(out, tok)
			l.advance()
			leadingSpace = false
			continue
		}
		if ch == '/' && l.peekAt(1) == '/' {
			leadingSpace = true
			l.advance()
			l.advance()
			for !l.atEnd() && l.peek() != '\n' {
				l.advance()
			}
			continue
		}
		if ch == '/' && l.peekAt(1) == '*' {
			leadingSpace = true
			l.advance()
			l.advance()
			for !l.atEnd() {
				if l.peek() == '*' && l.peekAt(1) == '/' {
					l.advance()
					l.advance()
					break
				}
				if l.peek() == '\n' {
					tok := Token{Kind: TokNewline, Text: "\n", File: l.file, Line: l.line, Col: l.col, StartOfLine: true}
					out = append(out, tok)
				}
				l.advance()
			}
			continue
		}
		startOfLine := l.startOfLine
		tok, err := l.tokenizeOne(leadingSpace, startOfLine)
		if err != nil {
			return nil, err
		}
		if tok.Kind != TokEOF {
			out = append(out, tok)
		}
		leadingSpace = false
		l.startOfLine = false
	}
	out = append(out, Token{Kind: TokEOF, File: l.file, Line: l.line, Col: l.col, StartOfLine: l.startOfLine})
	return out, nil
}
