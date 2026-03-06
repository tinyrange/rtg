package frontend

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Macro is a C preprocessor macro definition.
type Macro struct {
	Name         string
	FunctionLike bool
	Params       []string
	Variadic     bool
	Body         []Token
}

// Options configures preprocessing behavior.
type Options struct {
	IncludePaths          []string
	SystemIncludePaths    []string
	Defines               []string // NAME or NAME=VALUE
	Undefs                []string
	MaxIncludeDepth       int
	TargetOS              string
	TargetArch            string
	PtrSize               int
	Hosted                bool
	DisableBuiltinHeaders bool
}

// Preprocessor preprocesses token streams.
type Preprocessor struct {
	includePaths        []string
	systemIncludePaths  []string
	builtinIncludePaths []string
	macros              map[string]*Macro
	once                map[string]bool
	headerGuards        map[string]string
	included            map[string]bool
	activeFiles         map[string]bool
	maxIncludeDepth     int
}

func NewPreprocessor(opts Options) *Preprocessor {
	maxDepth := opts.MaxIncludeDepth
	if maxDepth <= 0 {
		maxDepth = 64
	}
	targetOS := opts.TargetOS
	if targetOS == "" {
		targetOS = runtime.GOOS
	}
	targetArch := opts.TargetArch
	if targetArch == "" {
		targetArch = runtime.GOARCH
	}
	ptrSize := opts.PtrSize
	if ptrSize <= 0 {
		ptrSize = defaultTargetPtrSize(targetArch)
	}
	p := &Preprocessor{
		includePaths:        append([]string{}, opts.IncludePaths...),
		systemIncludePaths:  append([]string{}, opts.SystemIncludePaths...),
		builtinIncludePaths: builtinIncludeSearchPaths(!opts.DisableBuiltinHeaders),
		macros:              make(map[string]*Macro),
		once:                make(map[string]bool),
		headerGuards:        make(map[string]string),
		included:            make(map[string]bool),
		activeFiles:         make(map[string]bool),
		maxIncludeDepth:     maxDepth,
	}
	for _, d := range builtinPredefineSpecs(targetOS, targetArch, ptrSize, opts.Hosted) {
		p.applyCommandLineDefine(d)
	}
	for _, d := range opts.Defines {
		p.applyCommandLineDefine(d)
	}
	for _, u := range opts.Undefs {
		p.macros[u] = nil
	}
	return p
}

func cloneTokens(in []Token) []Token {
	if len(in) == 0 {
		return nil
	}
	out := make([]Token, len(in))
	copy(out, in)
	return out
}

func parenBalance(tokens []Token) int {
	depth := 0
	for _, t := range tokens {
		if t.Kind != TokPunct {
			continue
		}
		switch t.Text {
		case "(":
			depth++
		case ")":
			if depth > 0 {
				depth--
			}
		}
	}
	return depth
}

func copyDisabled(disabled map[string]bool, name string) map[string]bool {
	out := make(map[string]bool)
	for k, v := range disabled {
		if v {
			out[k] = true
		}
	}
	out[name] = true
	return out
}

func tokenizeDefineBody(spec string) []Token {
	lx := NewLexer("<define>", spec)
	toks, err := lx.Tokenize()
	if err != nil {
		return nil
	}
	if len(toks) > 0 && toks[len(toks)-1].Kind == TokEOF {
		toks = toks[:len(toks)-1]
	}
	var out []Token
	for _, t := range toks {
		if t.Kind != TokNewline {
			out = append(out, t)
		}
	}
	return out
}

func (p *Preprocessor) applyCommandLineDefine(spec string) {
	name := spec
	value := "1"
	if eq := strings.Index(spec, "="); eq >= 0 {
		name = spec[:eq]
		value = spec[eq+1:]
	}
	if name == "" {
		return
	}
	p.macros[name] = &Macro{Name: name, Body: tokenizeDefineBody(value)}
}

func fileExists(path string) bool {
	_, err := os.ReadFile(path)
	return err == nil
}

func detectHeaderGuard(path string, src string) string {
	lx := NewLexer(path, src)
	toks, err := lx.Tokenize()
	if err != nil {
		return ""
	}
	i := 0
	for i < len(toks) && toks[i].Kind == TokNewline {
		i++
	}
	if i+5 >= len(toks) {
		return ""
	}
	if toks[i].Kind != TokPunct || toks[i].Text != "#" {
		return ""
	}
	if toks[i+1].Kind != TokIdent || toks[i+1].Text != "ifndef" {
		return ""
	}
	if toks[i+2].Kind != TokIdent {
		return ""
	}
	guard := toks[i+2].Text
	if toks[i+3].Kind != TokNewline {
		return ""
	}
	i += 4
	for i < len(toks) && toks[i].Kind == TokNewline {
		i++
	}
	if i+3 >= len(toks) {
		return ""
	}
	if toks[i].Kind != TokPunct || toks[i].Text != "#" {
		return ""
	}
	if toks[i+1].Kind != TokIdent || toks[i+1].Text != "define" {
		return ""
	}
	if toks[i+2].Kind != TokIdent || toks[i+2].Text != guard {
		return ""
	}
	return guard
}

func (p *Preprocessor) headerGuardFor(path string, src string) string {
	if guard, ok := p.headerGuards[path]; ok {
		return guard
	}
	guard := detectHeaderGuard(path, src)
	p.headerGuards[path] = guard
	return guard
}

func (p *Preprocessor) resolveInclude(include string, quoted bool, currentFile string) (string, error) {
	if isAbsPath(include) {
		if fileExists(include) {
			return absPath(include)
		}
		return "", fmt.Errorf("include not found: %s", include)
	}
	var search []string
	if quoted && currentFile != "" {
		search = append(search, filepath.Dir(currentFile))
	}
	search = append(search, p.includePaths...)
	search = append(search, p.builtinIncludePaths...)
	search = append(search, p.systemIncludePaths...)
	for _, dir := range search {
		if dir == "" {
			continue
		}
		candidate := cleanPath(filepath.Join(dir, include))
		if _, ok := readBuiltinInclude(candidate); ok {
			return candidate, nil
		}
		if fileExists(candidate) {
			abs, err := absPath(candidate)
			if err != nil {
				return "", err
			}
			return abs, nil
		}
	}
	if fileExists(include) {
		return absPath(include)
	}
	return "", fmt.Errorf("include not found: %s", include)
}

func (p *Preprocessor) processPath(path string, depth int) ([]Token, error) {
	if depth > p.maxIncludeDepth {
		return nil, fmt.Errorf("preprocessor: include depth exceeded (%d)", p.maxIncludeDepth)
	}
	path = cleanPath(path)
	if src, ok := readBuiltinInclude(path); ok {
		if p.once[path] && p.included[path] {
			return nil, nil
		}
		if p.activeFiles[path] {
			if guard := p.headerGuardFor(path, src); guard != "" && p.isDefined(guard) {
				return nil, nil
			}
			return nil, fmt.Errorf("preprocessor: recursive include cycle: %s", path)
		}
		p.activeFiles[path] = true
		p.included[path] = true
		out, procErr := p.processSource(path, src, depth)
		p.activeFiles[path] = false
		return out, procErr
	}
	abs, err := absPath(path)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	src := string(content)
	if p.once[abs] && p.included[abs] {
		return nil, nil
	}
	if p.activeFiles[abs] {
		if guard := p.headerGuardFor(abs, src); guard != "" && p.isDefined(guard) {
			return nil, nil
		}
		return nil, fmt.Errorf("preprocessor: recursive include cycle: %s", abs)
	}
	p.activeFiles[abs] = true
	p.included[abs] = true
	out, procErr := p.processSource(abs, src, depth)
	p.activeFiles[abs] = false
	return out, procErr
}

// ProcessFile preprocesses a C source file and emits tokens.
func (p *Preprocessor) ProcessFile(path string) ([]Token, error) {
	toks, err := p.processPath(path, 0)
	if err != nil {
		return nil, err
	}
	toks = append(toks, Token{Kind: TokEOF, File: path})
	return toks, nil
}

// ProcessSource preprocesses source content with the given logical filename.
func (p *Preprocessor) ProcessSource(file string, src string) ([]Token, error) {
	toks, err := p.processSource(file, src, 0)
	if err != nil {
		return nil, err
	}
	toks = append(toks, Token{Kind: TokEOF, File: file})
	return toks, nil
}

type condFrame struct {
	parentActive bool
	active       bool
	branchTaken  bool
	sawElse      bool
}

type condState struct {
	frames []condFrame
	active bool
}

func (p *Preprocessor) processSource(file string, src string, depth int) ([]Token, error) {
	lx := NewLexer(file, src)
	toks, err := lx.Tokenize()
	if err != nil {
		return nil, err
	}
	if len(toks) > 0 && toks[len(toks)-1].Kind == TokEOF {
		toks = toks[:len(toks)-1]
	}

	state := condState{active: true}
	var out []Token
	i := 0
	for i < len(toks) {
		lineStart := i
		for i < len(toks) && toks[i].Kind != TokNewline {
			i++
		}
		line := toks[lineStart:i]
		hasNL := i < len(toks) && toks[i].Kind == TokNewline
		var nlTok Token
		if hasNL {
			nlTok = toks[i]
		}

		if len(line) > 0 && line[0].Kind == TokPunct && line[0].Text == "#" {
			emitted, err := p.handleDirective(file, line, &state, depth)
			if err != nil {
				return nil, err
			}
			if len(emitted) > 0 {
				out = append(out, emitted...)
			}
			if hasNL {
				i++
			}
			continue
		}

		if state.active {
			group := cloneTokens(line)
			newlines := make([]Token, 0, 1)
			next := i
			if hasNL {
				newlines = append(newlines, nlTok)
				next = i + 1
			}
			for parenBalance(group) > 0 && next < len(toks) {
				lineStart = next
				for next < len(toks) && toks[next].Kind != TokNewline {
					next++
				}
				group = append(group, toks[lineStart:next]...)
				if next < len(toks) && toks[next].Kind == TokNewline {
					newlines = append(newlines, toks[next])
					next++
				}
			}
			if len(group) > 0 {
				expanded, err := p.expandTokens(file, group, nil, 0)
				if err != nil {
					return nil, err
				}
				out = append(out, expanded...)
			}
			out = append(out, newlines...)
			i = next
			continue
		}

		if hasNL {
			i++
		}
	}

	if len(state.frames) > 0 {
		return nil, fmt.Errorf("%s: unterminated conditional directive", file)
	}

	return out, nil
}

func isDirectiveName(line []Token, name string) bool {
	return len(line) >= 2 && line[1].Kind == TokIdent && line[1].Text == name
}

func lineToText(toks []Token) string {
	if len(toks) == 0 {
		return ""
	}
	var b strings.Builder
	for i, t := range toks {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(t.Text)
	}
	return b.String()
}

func (p *Preprocessor) handleDirective(file string, line []Token, state *condState, depth int) ([]Token, error) {
	if len(line) < 2 || line[1].Kind != TokIdent {
		return nil, nil
	}
	name := line[1].Text
	args := line[2:]

	switch name {
	case "if":
		parent := state.active
		cond := false
		if parent {
			expanded, err := p.expandIfExprTokens(file, args, nil, 0)
			if err != nil {
				return nil, err
			}
			cond, err = p.evalIfExpression(file, expanded)
			if err != nil {
				return nil, err
			}
		}
		f := condFrame{parentActive: parent, active: parent && cond, branchTaken: parent && cond}
		state.frames = append(state.frames, f)
		state.active = f.active
		return nil, nil
	case "ifdef":
		parent := state.active
		cond := parent && len(args) > 0 && args[0].Kind == TokIdent && p.isDefined(args[0].Text)
		f := condFrame{parentActive: parent, active: parent && cond, branchTaken: parent && cond}
		state.frames = append(state.frames, f)
		state.active = f.active
		return nil, nil
	case "ifndef":
		parent := state.active
		cond := parent && len(args) > 0 && args[0].Kind == TokIdent && !p.isDefined(args[0].Text)
		f := condFrame{parentActive: parent, active: parent && cond, branchTaken: parent && cond}
		state.frames = append(state.frames, f)
		state.active = f.active
		return nil, nil
	case "elif":
		if len(state.frames) == 0 {
			return nil, fmt.Errorf("%s: #elif without #if", file)
		}
		idx := len(state.frames) - 1
		f := state.frames[idx]
		if f.sawElse {
			return nil, fmt.Errorf("%s: #elif after #else", file)
		}
		if !f.parentActive {
			f.active = false
		} else if f.branchTaken {
			f.active = false
		} else {
			expanded, err := p.expandIfExprTokens(file, args, nil, 0)
			if err != nil {
				return nil, err
			}
			cond, err := p.evalIfExpression(file, expanded)
			if err != nil {
				return nil, err
			}
			f.active = cond
			if cond {
				f.branchTaken = true
			}
		}
		state.frames[idx] = f
		state.active = f.active
		return nil, nil
	case "else":
		if len(state.frames) == 0 {
			return nil, fmt.Errorf("%s: #else without #if", file)
		}
		idx := len(state.frames) - 1
		f := state.frames[idx]
		if f.sawElse {
			return nil, fmt.Errorf("%s: duplicate #else", file)
		}
		f.sawElse = true
		if !f.parentActive || f.branchTaken {
			f.active = false
		} else {
			f.active = true
			f.branchTaken = true
		}
		state.frames[idx] = f
		state.active = f.active
		return nil, nil
	case "endif":
		if len(state.frames) == 0 {
			return nil, fmt.Errorf("%s: #endif without #if", file)
		}
		state.frames = state.frames[:len(state.frames)-1]
		if len(state.frames) == 0 {
			state.active = true
		} else {
			top := state.frames[len(state.frames)-1]
			state.active = top.active
		}
		return nil, nil
	}

	if state.active == false {
		return nil, nil
	}

	switch name {
	case "define":
		return nil, p.handleDefine(file, args)
	case "undef":
		return nil, p.handleUndef(file, args)
	case "include":
		return p.handleInclude(file, args, depth+1)
	case "pragma":
		if len(args) > 0 && args[0].Kind == TokIdent && args[0].Text == "once" {
			abs, err := absPath(file)
			if err != nil {
				return nil, err
			}
			p.once[abs] = true
		}
		return nil, nil
	case "error":
		return nil, fmt.Errorf("%s: #error %s", file, lineToText(args))
	default:
		return nil, nil
	}
}

func (p *Preprocessor) handleDefine(file string, args []Token) error {
	if len(args) == 0 || args[0].Kind != TokIdent {
		return fmt.Errorf("%s: invalid #define", file)
	}
	m := &Macro{Name: args[0].Text}
	i := 1
	if i < len(args) && args[i].Kind == TokPunct && args[i].Text == "(" && !args[i].LeadingSpace {
		m.FunctionLike = true
		i++
		if i < len(args) && args[i].Kind == TokPunct && args[i].Text == ")" {
			i++
		} else {
			for {
				if i >= len(args) {
					return fmt.Errorf("%s: unterminated macro parameter list", file)
				}
				if args[i].Kind == TokPunct && args[i].Text == "..." {
					m.Variadic = true
					m.Params = append(m.Params, "__VA_ARGS__")
					i++
					if i >= len(args) || args[i].Kind != TokPunct || args[i].Text != ")" {
						return fmt.Errorf("%s: variadic macro must end with )", file)
					}
					i++
					break
				}
				if args[i].Kind != TokIdent {
					return fmt.Errorf("%s: invalid macro parameter", file)
				}
				m.Params = append(m.Params, args[i].Text)
				i++
				if i >= len(args) {
					return fmt.Errorf("%s: unterminated macro parameter list", file)
				}
				if args[i].Kind == TokPunct && args[i].Text == ")" {
					i++
					break
				}
				if args[i].Kind == TokPunct && args[i].Text == "," {
					i++
					continue
				}
				return fmt.Errorf("%s: expected ',' or ')' in macro parameter list", file)
			}
		}
	}
	if i < len(args) {
		m.Body = cloneTokens(args[i:])
	}
	p.macros[m.Name] = m
	return nil
}

func (p *Preprocessor) handleUndef(file string, args []Token) error {
	if len(args) == 0 || args[0].Kind != TokIdent {
		return fmt.Errorf("%s: invalid #undef", file)
	}
	p.macros[args[0].Text] = nil
	return nil
}

func decodeStringToken(tok Token) (string, error) {
	if len(tok.Text) < 2 {
		return "", fmt.Errorf("invalid string token %q", tok.Text)
	}
	q := tok.Text
	for len(q) > 0 {
		switch q[0] {
		case ' ', '\t', '\r', '\n':
			q = q[1:]
		default:
			goto trimmed
		}
	}
trimmed:
	if len(q) < 2 {
		return "", fmt.Errorf("invalid string token %q", tok.Text)
	}
	if q[0] == 'L' || q[0] == 'u' || q[0] == 'U' {
		q = q[1:]
	} else if len(q) >= 2 && q[0] == 'u' && q[1] == '8' {
		q = q[2:]
	}
	if len(q) < 2 || q[0] != '"' {
		return "", fmt.Errorf("invalid include string %q", tok.Text)
	}
	v, err := unquoteCString(q)
	if err != nil {
		return "", err
	}
	return v, nil
}

func (p *Preprocessor) handleInclude(file string, args []Token, depth int) ([]Token, error) {
	expanded, err := p.expandTokens(file, args, nil, 0)
	if err != nil {
		return nil, err
	}
	if len(expanded) == 0 {
		return nil, fmt.Errorf("%s: empty #include", file)
	}

	include := ""
	quoted := false
	if expanded[0].Kind == TokString {
		v, err := decodeStringToken(expanded[0])
		if err != nil {
			return nil, fmt.Errorf("%s: %v", file, err)
		}
		include = v
		quoted = true
	} else if expanded[0].Kind == TokPunct && expanded[0].Text == "<" {
		j := 1
		var b strings.Builder
		for j < len(expanded) {
			if expanded[j].Kind == TokPunct && expanded[j].Text == ">" {
				break
			}
			b.WriteString(expanded[j].Text)
			j++
		}
		if j >= len(expanded) {
			return nil, fmt.Errorf("%s: unterminated #include <...>", file)
		}
		include = b.String()
		quoted = false
	} else {
		return nil, fmt.Errorf("%s: invalid #include operand", file)
	}

	resolved, err := p.resolveInclude(include, quoted, file)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", file, err)
	}
	return p.processPath(resolved, depth)
}

func (p *Preprocessor) isDefined(name string) bool {
	m, ok := p.macros[name]
	return ok && m != nil
}

func (p *Preprocessor) expandTokens(file string, in []Token, disabled map[string]bool, depth int) ([]Token, error) {
	if depth > 128 {
		return nil, fmt.Errorf("preprocessor: macro expansion depth exceeded")
	}
	var out []Token
	for i := 0; i < len(in); i++ {
		tok := in[i]
		if tok.Kind == TokIdent {
			if tok.Text == "__LINE__" {
				out = append(out, Token{Kind: TokNumber, Text: decimalItoa(tok.Line), File: file, Line: tok.Line, Col: tok.Col})
				continue
			}
			if tok.Text == "__FILE__" {
				out = append(out, Token{Kind: TokString, Text: quoteTokenText(file), File: file, Line: tok.Line, Col: tok.Col})
				continue
			}
			m, ok := p.macros[tok.Text]
			if ok && m != nil && !disabled[tok.Text] {
				if m.FunctionLike {
					open := i + 1
					for open < len(in) && in[open].Kind == TokNewline {
						open++
					}
					if open >= len(in) || in[open].Kind != TokPunct || in[open].Text != "(" {
						out = append(out, tok)
						continue
					}
					args, end, ok := parseMacroArgs(in, open)
					if !ok {
						return nil, fmt.Errorf("%s:%d:%d: unterminated macro call", file, tok.Line, tok.Col)
					}
					repl, err := p.applyMacro(file, tok, m, args)
					if err != nil {
						return nil, err
					}
					nextDisabled := copyDisabled(disabled, tok.Text)
					rawRepl := cloneTokens(repl)
					repl, err = p.expandTokens(file, repl, nextDisabled, depth+1)
					if err != nil && strings.Contains(err.Error(), "unterminated macro call") {
						tail := append(rawRepl, cloneTokens(in[end+1:])...)
						repl, err = p.expandTokens(file, tail, nextDisabled, depth+1)
						if err == nil {
							out = append(out, repl...)
							return out, nil
						}
					}
					if err != nil {
						return nil, err
					}
					out = append(out, repl...)
					i = end
					continue
				}
				nextDisabled := copyDisabled(disabled, tok.Text)
				repl, err := p.expandTokens(file, cloneTokens(m.Body), nextDisabled, depth+1)
				if err != nil && strings.Contains(err.Error(), "unterminated macro call") {
					tail := append(cloneTokens(m.Body), cloneTokens(in[i+1:])...)
					repl, err = p.expandTokens(file, tail, nextDisabled, depth+1)
					if err == nil {
						out = append(out, repl...)
						return out, nil
					}
				}
				if err != nil {
					return nil, err
				}
				out = append(out, repl...)
				continue
			}
		}
		out = append(out, tok)
	}
	return out, nil
}

func (p *Preprocessor) expandIfExprTokens(file string, in []Token, disabled map[string]bool, depth int) ([]Token, error) {
	if depth > 128 {
		return nil, fmt.Errorf("preprocessor: macro expansion depth exceeded")
	}
	var out []Token
	for i := 0; i < len(in); i++ {
		tok := in[i]
		if tok.Kind == TokIdent && tok.Text == "defined" {
			out = append(out, tok)
			j := i + 1
			for j < len(in) && in[j].Kind == TokNewline {
				out = append(out, in[j])
				j++
			}
			if j < len(in) && in[j].Kind == TokPunct && in[j].Text == "(" {
				out = append(out, in[j])
				j++
				for j < len(in) && in[j].Kind == TokNewline {
					out = append(out, in[j])
					j++
				}
				if j < len(in) {
					out = append(out, in[j])
					j++
				}
				for j < len(in) && in[j].Kind == TokNewline {
					out = append(out, in[j])
					j++
				}
				if j < len(in) && in[j].Kind == TokPunct && in[j].Text == ")" {
					out = append(out, in[j])
					i = j
					continue
				}
				i = j - 1
				continue
			}
			if j < len(in) {
				out = append(out, in[j])
				i = j
				continue
			}
			i = j - 1
			continue
		}
		if tok.Kind == TokIdent {
			if tok.Text == "__LINE__" {
				out = append(out, Token{Kind: TokNumber, Text: decimalItoa(tok.Line), File: file, Line: tok.Line, Col: tok.Col})
				continue
			}
			if tok.Text == "__FILE__" {
				out = append(out, Token{Kind: TokString, Text: quoteTokenText(file), File: file, Line: tok.Line, Col: tok.Col})
				continue
			}
			m, ok := p.macros[tok.Text]
			if ok && m != nil && !disabled[tok.Text] {
				if m.FunctionLike {
					open := i + 1
					for open < len(in) && in[open].Kind == TokNewline {
						open++
					}
					if open >= len(in) || in[open].Kind != TokPunct || in[open].Text != "(" {
						out = append(out, tok)
						continue
					}
					args, end, ok := parseMacroArgs(in, open)
					if !ok {
						return nil, fmt.Errorf("%s:%d:%d: unterminated macro call", file, tok.Line, tok.Col)
					}
					repl, err := p.applyMacro(file, tok, m, args)
					if err != nil {
						return nil, err
					}
					nextDisabled := copyDisabled(disabled, tok.Text)
					rawRepl := cloneTokens(repl)
					repl, err = p.expandIfExprTokens(file, repl, nextDisabled, depth+1)
					if err != nil && strings.Contains(err.Error(), "unterminated macro call") {
						tail := append(rawRepl, cloneTokens(in[end+1:])...)
						repl, err = p.expandIfExprTokens(file, tail, nextDisabled, depth+1)
						if err == nil {
							out = append(out, repl...)
							return out, nil
						}
					}
					if err != nil {
						return nil, err
					}
					out = append(out, repl...)
					i = end
					continue
				}
				nextDisabled := copyDisabled(disabled, tok.Text)
				repl, err := p.expandIfExprTokens(file, cloneTokens(m.Body), nextDisabled, depth+1)
				if err != nil && strings.Contains(err.Error(), "unterminated macro call") {
					tail := append(cloneTokens(m.Body), cloneTokens(in[i+1:])...)
					repl, err = p.expandIfExprTokens(file, tail, nextDisabled, depth+1)
					if err == nil {
						out = append(out, repl...)
						return out, nil
					}
				}
				if err != nil {
					return nil, err
				}
				out = append(out, repl...)
				continue
			}
		}
		out = append(out, tok)
	}
	return out, nil
}

func parseMacroArgs(tokens []Token, openParen int) ([][]Token, int, bool) {
	if openParen >= len(tokens) || tokens[openParen].Kind != TokPunct || tokens[openParen].Text != "(" {
		return nil, 0, false
	}
	depthParen := 0
	depthBracket := 0
	depthBrace := 0
	j := openParen + 1
	var args [][]Token
	var cur []Token
	sawAny := false
	for j < len(tokens) {
		t := tokens[j]
		if t.Kind == TokPunct {
			switch t.Text {
			case "(":
				depthParen++
				cur = append(cur, t)
				sawAny = true
				j++
				continue
			case ")":
				if depthParen == 0 {
					if sawAny || len(args) > 0 {
						args = append(args, cur)
					}
					return args, j, true
				}
				if depthParen > 0 {
					depthParen--
				}
				cur = append(cur, t)
				sawAny = true
				j++
				continue
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
			case ",":
				if depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
					args = append(args, cur)
					cur = nil
					sawAny = true
					j++
					continue
				}
			}
		}
		cur = append(cur, t)
		sawAny = true
		j++
	}
	return nil, 0, false
}

func stringifyTokens(tokens []Token) string {
	if len(tokens) == 0 {
		return "\"\""
	}
	var b strings.Builder
	for i, t := range tokens {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(t.Text)
	}
	return quoteTokenText(b.String())
}

func (p *Preprocessor) applyMacro(file string, callTok Token, m *Macro, args [][]Token) ([]Token, error) {
	paramMap := make(map[string][]Token)
	if m.FunctionLike {
		if !m.Variadic {
			for i, name := range m.Params {
				if i < len(args) {
					paramMap[name] = cloneTokens(args[i])
				} else {
					paramMap[name] = nil
				}
			}
		} else {
			fixed := len(m.Params) - 1
			if fixed < 0 {
				fixed = 0
			}
			for i := 0; i < fixed; i++ {
				name := m.Params[i]
				if i < len(args) {
					paramMap[name] = cloneTokens(args[i])
				} else {
					paramMap[name] = nil
				}
			}
			var varg []Token
			for i := fixed; i < len(args); i++ {
				if i > fixed {
					varg = append(varg, Token{Kind: TokPunct, Text: ",", File: file, Line: callTok.Line, Col: callTok.Col})
				}
				varg = append(varg, cloneTokens(args[i])...)
			}
			paramMap["__VA_ARGS__"] = varg
		}
	}

	var out []Token
	for i := 0; i < len(m.Body); i++ {
		t := m.Body[i]
		if t.Kind == TokPunct && t.Text == "#" && i+1 < len(m.Body) {
			next := m.Body[i+1]
			if next.Kind == TokIdent {
				if arg, ok := paramMap[next.Text]; ok {
					out = append(out, Token{Kind: TokString, Text: stringifyTokens(arg), File: file, Line: callTok.Line, Col: callTok.Col})
					i++
					continue
				}
			}
		}
		if t.Kind == TokPunct && t.Text == "##" {
			if len(out) == 0 || i+1 >= len(m.Body) {
				continue
			}
			right := []Token{m.Body[i+1]}
			if m.Body[i+1].Kind == TokIdent {
				if arg, ok := paramMap[m.Body[i+1].Text]; ok {
					right = cloneTokens(arg)
				}
			}
			if len(right) == 0 {
				i++
				continue
			}
			merged := out[len(out)-1]
			merged.Text = merged.Text + right[0].Text
			out[len(out)-1] = merged
			if len(right) > 1 {
				out = append(out, right[1:]...)
			}
			i++
			continue
		}
		if t.Kind == TokIdent {
			if arg, ok := paramMap[t.Text]; ok {
				out = append(out, cloneTokens(arg)...)
				continue
			}
		}
		out = append(out, t)
	}
	return out, nil
}

func parseIntConstant(text string) (int64, error) {
	s := text
	for len(s) > 0 {
		c := s[len(s)-1]
		if c == 'u' || c == 'U' || c == 'l' || c == 'L' {
			s = s[:len(s)-1]
			continue
		}
		break
	}
	if s == "" {
		return 0, fmt.Errorf("invalid integer constant %q", text)
	}
	base := 10
	start := 0
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		base = 16
		start = 2
	} else if len(s) > 1 && s[0] == '0' {
		base = 8
		start = 1
	}
	if start >= len(s) {
		return 0, nil
	}
	u, err := parseUintBase(s[start:], base, 64)
	if err != nil {
		return 0, err
	}
	return int64(u), nil
}

type exprParser struct {
	tokens []Token
	pos    int
	pp     *Preprocessor
	file   string
}

func (e *exprParser) atEnd() bool {
	return e.pos >= len(e.tokens)
}

func (e *exprParser) peek() Token {
	if e.atEnd() {
		return Token{Kind: TokEOF}
	}
	return e.tokens[e.pos]
}

func (e *exprParser) advance() Token {
	t := e.peek()
	if !e.atEnd() {
		e.pos++
	}
	return t
}

func (e *exprParser) matchPunct(op string) bool {
	if e.atEnd() {
		return false
	}
	t := e.tokens[e.pos]
	if t.Kind == TokPunct && t.Text == op {
		e.pos++
		return true
	}
	return false
}

func (e *exprParser) parsePrimary() (int64, error) {
	if e.matchPunct("(") {
		v, err := e.parseComma()
		if err != nil {
			return 0, err
		}
		if !e.matchPunct(")") {
			return 0, fmt.Errorf("unterminated parenthesized expression")
		}
		return v, nil
	}
	t := e.peek()
	if t.Kind == TokNumber {
		e.advance()
		v, err := parseIntConstant(t.Text)
		if err != nil {
			return 0, err
		}
		return v, nil
	}
	if t.Kind == TokChar {
		e.advance()
		v, err := parseCCharLiteral(t.Text)
		if err != nil {
			return 0, err
		}
		return v, nil
	}
	if t.Kind == TokIdent {
		e.advance()
		if m, ok := e.pp.macros[t.Text]; ok && m != nil && !m.FunctionLike && len(m.Body) == 1 && m.Body[0].Kind == TokNumber {
			v, err := parseIntConstant(m.Body[0].Text)
			if err == nil {
				return v, nil
			}
		}
		return 0, nil
	}
	return 0, fmt.Errorf("unexpected token in #if expression: %s", t.String())
}

func (e *exprParser) parseUnary() (int64, error) {
	if e.matchPunct("!") {
		v, err := e.parseUnary()
		if err != nil {
			return 0, err
		}
		if v == 0 {
			return 1, nil
		}
		return 0, nil
	}
	if e.matchPunct("+") {
		return e.parseUnary()
	}
	if e.matchPunct("-") {
		v, err := e.parseUnary()
		if err != nil {
			return 0, err
		}
		return -v, nil
	}
	if e.matchPunct("~") {
		v, err := e.parseUnary()
		if err != nil {
			return 0, err
		}
		return ^v, nil
	}
	if !e.atEnd() && e.peek().Kind == TokIdent && e.peek().Text == "defined" {
		e.advance()
		if e.matchPunct("(") {
			if e.atEnd() || e.peek().Kind != TokIdent {
				return 0, fmt.Errorf("defined() expects identifier")
			}
			name := e.advance().Text
			if !e.matchPunct(")") {
				return 0, fmt.Errorf("defined() missing closing )")
			}
			if e.pp.isDefined(name) {
				return 1, nil
			}
			return 0, nil
		}
		if e.atEnd() || e.peek().Kind != TokIdent {
			return 0, fmt.Errorf("defined expects identifier")
		}
		name := e.advance().Text
		if e.pp.isDefined(name) {
			return 1, nil
		}
		return 0, nil
	}
	if !e.atEnd() && e.peek().Kind == TokIdent {
		name := e.peek().Text
		switch name {
		case "__has_include":
			e.advance()
			return e.parseHasInclude()
		case "__has_include_next", "__has_feature", "__has_extension", "__has_builtin", "__has_attribute", "__has_c_attribute", "__has_warning", "__has_cpp_attribute", "__has_declspec_attribute":
			name := e.advance().Text
			if _, err := e.parseFeatureOperand(name); err != nil {
				return 0, err
			}
			return 0, nil
		}
		if strings.HasPrefix(name, "__is_") || name == "__building_module" {
			name = e.advance().Text
			if _, err := e.parseFeatureOperand(name); err != nil {
				return 0, err
			}
			return 0, nil
		}
	}
	return e.parsePrimary()
}

func (e *exprParser) parseFeatureOperand(name string) ([]Token, error) {
	if !e.matchPunct("(") {
		return nil, fmt.Errorf("%s expects (...)", name)
	}
	start := e.pos
	depth := 1
	for !e.atEnd() {
		tok := e.advance()
		if tok.Kind != TokPunct {
			continue
		}
		switch tok.Text {
		case "(":
			depth++
		case ")":
			depth--
			if depth == 0 {
				return cloneTokens(e.tokens[start : e.pos-1]), nil
			}
		}
	}
	return nil, fmt.Errorf("%s missing closing )", name)
}

func (e *exprParser) parseHasInclude() (int64, error) {
	operand, err := e.parseFeatureOperand("__has_include")
	if err != nil {
		return 0, err
	}
	if len(operand) == 0 {
		return 0, fmt.Errorf("__has_include expects a header operand")
	}

	include := ""
	quoted := false
	if len(operand) == 1 && operand[0].Kind == TokString {
		v, err := decodeStringToken(operand[0])
		if err != nil {
			return 0, err
		}
		include = v
		quoted = true
	} else if operand[0].Kind == TokPunct && operand[0].Text == "<" {
		j := 1
		var b strings.Builder
		for j < len(operand) {
			if operand[j].Kind == TokPunct && operand[j].Text == ">" {
				break
			}
			b.WriteString(operand[j].Text)
			j++
		}
		if j >= len(operand) || j != len(operand)-1 {
			return 0, fmt.Errorf("invalid __has_include operand")
		}
		include = b.String()
	} else {
		return 0, fmt.Errorf("invalid __has_include operand")
	}

	_, err = e.pp.resolveInclude(include, quoted, e.file)
	if err == nil {
		return 1, nil
	}
	if strings.Contains(err.Error(), "include not found:") {
		return 0, nil
	}
	return 0, nil
}

func (e *exprParser) parseMul() (int64, error) {
	v, err := e.parseUnary()
	if err != nil {
		return 0, err
	}
	for {
		if e.matchPunct("*") {
			r, err := e.parseUnary()
			if err != nil {
				return 0, err
			}
			v = v * r
			continue
		}
		if e.matchPunct("/") {
			r, err := e.parseUnary()
			if err != nil {
				return 0, err
			}
			if r == 0 {
				return 0, fmt.Errorf("division by zero in #if expression")
			}
			v = v / r
			continue
		}
		if e.matchPunct("%") {
			r, err := e.parseUnary()
			if err != nil {
				return 0, err
			}
			if r == 0 {
				return 0, fmt.Errorf("modulo by zero in #if expression")
			}
			v = v % r
			continue
		}
		break
	}
	return v, nil
}

func (e *exprParser) parseAdd() (int64, error) {
	v, err := e.parseMul()
	if err != nil {
		return 0, err
	}
	for {
		if e.matchPunct("+") {
			r, err := e.parseMul()
			if err != nil {
				return 0, err
			}
			v = v + r
			continue
		}
		if e.matchPunct("-") {
			r, err := e.parseMul()
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

func (e *exprParser) parseShift() (int64, error) {
	v, err := e.parseAdd()
	if err != nil {
		return 0, err
	}
	for {
		if e.matchPunct("<<") {
			r, err := e.parseAdd()
			if err != nil {
				return 0, err
			}
			v = v << uint(r)
			continue
		}
		if e.matchPunct(">>") {
			r, err := e.parseAdd()
			if err != nil {
				return 0, err
			}
			v = v >> uint(r)
			continue
		}
		break
	}
	return v, nil
}

func (e *exprParser) parseRel() (int64, error) {
	v, err := e.parseShift()
	if err != nil {
		return 0, err
	}
	for {
		if e.matchPunct("<") {
			r, err := e.parseShift()
			if err != nil {
				return 0, err
			}
			if v < r {
				v = 1
			} else {
				v = 0
			}
			continue
		}
		if e.matchPunct(">") {
			r, err := e.parseShift()
			if err != nil {
				return 0, err
			}
			if v > r {
				v = 1
			} else {
				v = 0
			}
			continue
		}
		if e.matchPunct("<=") {
			r, err := e.parseShift()
			if err != nil {
				return 0, err
			}
			if v <= r {
				v = 1
			} else {
				v = 0
			}
			continue
		}
		if e.matchPunct(">=") {
			r, err := e.parseShift()
			if err != nil {
				return 0, err
			}
			if v >= r {
				v = 1
			} else {
				v = 0
			}
			continue
		}
		break
	}
	return v, nil
}

func (e *exprParser) parseEq() (int64, error) {
	v, err := e.parseRel()
	if err != nil {
		return 0, err
	}
	for {
		if e.matchPunct("==") {
			r, err := e.parseRel()
			if err != nil {
				return 0, err
			}
			if v == r {
				v = 1
			} else {
				v = 0
			}
			continue
		}
		if e.matchPunct("!=") {
			r, err := e.parseRel()
			if err != nil {
				return 0, err
			}
			if v != r {
				v = 1
			} else {
				v = 0
			}
			continue
		}
		break
	}
	return v, nil
}

func (e *exprParser) parseBitAnd() (int64, error) {
	v, err := e.parseEq()
	if err != nil {
		return 0, err
	}
	for e.matchPunct("&") {
		r, err := e.parseEq()
		if err != nil {
			return 0, err
		}
		v = v & r
	}
	return v, nil
}

func (e *exprParser) parseBitXor() (int64, error) {
	v, err := e.parseBitAnd()
	if err != nil {
		return 0, err
	}
	for e.matchPunct("^") {
		r, err := e.parseBitAnd()
		if err != nil {
			return 0, err
		}
		v = v ^ r
	}
	return v, nil
}

func (e *exprParser) parseBitOr() (int64, error) {
	v, err := e.parseBitXor()
	if err != nil {
		return 0, err
	}
	for e.matchPunct("|") {
		r, err := e.parseBitXor()
		if err != nil {
			return 0, err
		}
		v = v | r
	}
	return v, nil
}

func (e *exprParser) parseAnd() (int64, error) {
	v, err := e.parseBitOr()
	if err != nil {
		return 0, err
	}
	for e.matchPunct("&&") {
		r, err := e.parseBitOr()
		if err != nil {
			return 0, err
		}
		if v != 0 && r != 0 {
			v = 1
		} else {
			v = 0
		}
	}
	return v, nil
}

func (e *exprParser) parseOr() (int64, error) {
	v, err := e.parseAnd()
	if err != nil {
		return 0, err
	}
	for e.matchPunct("||") {
		r, err := e.parseAnd()
		if err != nil {
			return 0, err
		}
		if v != 0 || r != 0 {
			v = 1
		} else {
			v = 0
		}
	}
	return v, nil
}

func (e *exprParser) parseConditional() (int64, error) {
	v, err := e.parseOr()
	if err != nil {
		return 0, err
	}
	if !e.matchPunct("?") {
		return v, nil
	}
	trueVal, err := e.parseComma()
	if err != nil {
		return 0, err
	}
	if !e.matchPunct(":") {
		return 0, fmt.Errorf("expected ':' in conditional expression")
	}
	falseVal, err := e.parseConditional()
	if err != nil {
		return 0, err
	}
	if v != 0 {
		return trueVal, nil
	}
	return falseVal, nil
}

func (e *exprParser) parseComma() (int64, error) {
	v, err := e.parseConditional()
	if err != nil {
		return 0, err
	}
	for e.matchPunct(",") {
		v, err = e.parseConditional()
		if err != nil {
			return 0, err
		}
	}
	return v, nil
}

func (p *Preprocessor) evalIfExpression(file string, tokens []Token) (bool, error) {
	if len(tokens) == 0 {
		return false, nil
	}
	e := &exprParser{tokens: tokens, pp: p, file: file}
	v, err := e.parseComma()
	if err != nil {
		return false, err
	}
	if !e.atEnd() {
		return false, fmt.Errorf("trailing tokens in #if expression (%s)", tokenSliceText(e.tokens[e.pos:]))
	}
	return v != 0, nil
}
