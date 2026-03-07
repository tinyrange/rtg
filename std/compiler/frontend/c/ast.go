package frontend

// NodeKind classifies C parse tree node kinds.
type NodeKind int

const (
	NTranslationUnit NodeKind = iota
	NExternalDecl
	NFunctionDef
	NCompoundStmt
	NDeclStmt
	NExprStmt
	NEmptyStmt
	NIfStmt
	NForStmt
	NWhileStmt
	NDoWhileStmt
	NSwitchStmt
	NCaseStmt
	NDefaultStmt
	NReturnStmt
	NBreakStmt
	NContinueStmt
	NGotoStmt
	NLabelStmt
)

func (k NodeKind) String() string {
	switch k {
	case NTranslationUnit:
		return "TranslationUnit"
	case NExternalDecl:
		return "ExternalDecl"
	case NFunctionDef:
		return "FunctionDef"
	case NCompoundStmt:
		return "CompoundStmt"
	case NDeclStmt:
		return "DeclStmt"
	case NExprStmt:
		return "ExprStmt"
	case NEmptyStmt:
		return "EmptyStmt"
	case NIfStmt:
		return "IfStmt"
	case NForStmt:
		return "ForStmt"
	case NWhileStmt:
		return "WhileStmt"
	case NDoWhileStmt:
		return "DoWhileStmt"
	case NSwitchStmt:
		return "SwitchStmt"
	case NCaseStmt:
		return "CaseStmt"
	case NDefaultStmt:
		return "DefaultStmt"
	case NReturnStmt:
		return "ReturnStmt"
	case NBreakStmt:
		return "BreakStmt"
	case NContinueStmt:
		return "ContinueStmt"
	case NGotoStmt:
		return "GotoStmt"
	case NLabelStmt:
		return "LabelStmt"
	default:
		return "?"
	}
}

// Node is a parse tree node for C frontend parse-only mode.
type Node struct {
	Kind     NodeKind
	Text     string
	Line     int
	Col      int
	Children []*Node
}
