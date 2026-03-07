package frontend

// TokenKind classifies C preprocessing tokens.
type TokenKind int

const (
	TokEOF TokenKind = iota
	TokNewline
	TokIdent
	TokNumber
	TokString
	TokChar
	TokPunct
)

func (k TokenKind) String() string {
	switch k {
	case TokEOF:
		return "EOF"
	case TokNewline:
		return "NEWLINE"
	case TokIdent:
		return "IDENT"
	case TokNumber:
		return "NUMBER"
	case TokString:
		return "STRING"
	case TokChar:
		return "CHAR"
	case TokPunct:
		return "PUNCT"
	default:
		return "?"
	}
}

// Token is a single C preprocessing token.
type Token struct {
	Kind         TokenKind
	Text         string
	File         string
	Line         int
	Col          int
	LeadingSpace bool
	StartOfLine  bool
}

func (t Token) String() string {
	return t.Kind.String() + ":" + quoteTokenText(t.Text)
}
