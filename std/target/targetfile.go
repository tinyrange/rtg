package target

import (
	"fmt"
	"os"

	frontend "j5.nz/rtg/std/compiler/frontend/go"
)

func unwrapDirectiveNode(node *frontend.Node) (*frontend.Node, []string) {
	var directives []string
	cur := node
	for cur != nil && cur.Kind == frontend.NDirective {
		directives = append(directives, cur.Name)
		cur = cur.X
	}
	return cur, directives
}

func parseTargetDirective(val string) (string, bool) {
	prefix := "target "
	if len(val) <= len(prefix) || val[0:len(prefix)] != prefix {
		return "", false
	}
	triple := val[len(prefix):len(val)]
	if triple == "" {
		return "", false
	}
	return triple, true
}

func parseTargetABIDirective(val string) (string, bool) {
	prefix := "targetabi "
	if len(val) <= len(prefix) || val[0:len(prefix)] != prefix {
		return "", false
	}
	triple := val[len(prefix):len(val)]
	if triple == "" {
		return "", false
	}
	return triple, true
}

func parseAssemblerDirective(val string) (string, bool) {
	prefix := "assembler "
	if len(val) <= len(prefix) || val[0:len(prefix)] != prefix {
		return "", false
	}
	name := val[len(prefix):len(val)]
	if name == "" {
		return "", false
	}
	return name, true
}

func parseBinFormatDirective(val string) (string, bool) {
	prefix := "binfmt "
	if len(val) <= len(prefix) || val[0:len(prefix)] != prefix {
		return "", false
	}
	name := val[len(prefix):len(val)]
	if name == "" {
		return "", false
	}
	return name, true
}

func literalIntValue(node *frontend.Node) (int64, bool) {
	if node == nil {
		return 0, false
	}
	if node.Kind == frontend.NUnaryExpr && node.Name == "-" && node.X != nil {
		v, ok := literalIntValue(node.X)
		if !ok {
			return 0, false
		}
		return -v, true
	}
	if node.Kind != frontend.NIntLit {
		return 0, false
	}
	s := node.Name
	if s == "" {
		return 0, false
	}
	base := int64(10)
	i := 0
	if len(s) > 2 && s[0] == '0' {
		if s[1] == 'x' || s[1] == 'X' {
			base = 16
			i = 2
		} else if s[1] == 'b' || s[1] == 'B' {
			base = 2
			i = 2
		} else if s[1] == 'o' || s[1] == 'O' {
			base = 8
			i = 2
		}
	}
	var v int64
	for i < len(s) {
		ch := s[i]
		i = i + 1
		if ch == '_' {
			continue
		}
		d := int64(-1)
		if ch >= '0' && ch <= '9' {
			d = int64(ch - '0')
		} else if ch >= 'a' && ch <= 'f' {
			d = int64(ch-'a') + 10
		} else if ch >= 'A' && ch <= 'F' {
			d = int64(ch-'A') + 10
		}
		if d < 0 || d >= base {
			return 0, false
		}
		v = v*base + d
	}
	return v, true
}

func literalStringValue(node *frontend.Node) (string, bool) {
	if node == nil || node.Kind != frontend.NStringLit {
		return "", false
	}
	return node.Name, true
}

func literalBoolValue(node *frontend.Node) (bool, bool) {
	if node == nil {
		return false, false
	}
	if node.Kind == frontend.NBasicLit {
		if node.Name == "true" {
			return true, true
		}
		if node.Name == "false" {
			return false, true
		}
	}
	if node.Kind == frontend.NIdent {
		if node.Name == "true" {
			return true, true
		}
		if node.Name == "false" {
			return false, true
		}
	}
	return false, false
}

func keyName(node *frontend.Node) string {
	if node == nil {
		return ""
	}
	if node.Kind == frontend.NIdent || node.Kind == frontend.NStringLit {
		return node.Name
	}
	return ""
}

func expectSingleReturnExpr(fn *frontend.Node) (*frontend.Node, error) {
	if fn == nil || fn.Kind != frontend.NFunc || fn.Body == nil || fn.Body.Kind != frontend.NBlock {
		return nil, fmt.Errorf("directive function must have a body")
	}
	if len(fn.Body.Nodes) != 1 {
		return nil, fmt.Errorf("directive function must contain exactly one return statement")
	}
	ret := fn.Body.Nodes[0]
	if ret == nil || ret.Kind != frontend.NReturn {
		return nil, fmt.Errorf("directive function must contain exactly one return statement")
	}
	if ret.X == nil || len(ret.Nodes) != 0 {
		return nil, fmt.Errorf("directive function must return exactly one value")
	}
	return ret.X, nil
}

func parseDefaultsLiteral(expr *frontend.Node) (Defaults, error) {
	var out Defaults
	if expr == nil || expr.Kind != frontend.NCompositeLit {
		return out, fmt.Errorf("Defaults must be a composite literal")
	}
	for _, elem := range expr.Nodes {
		if elem == nil || elem.Kind != frontend.NKeyValue {
			return out, fmt.Errorf("Defaults requires keyed fields")
		}
		key := keyName(elem.X)
		switch key {
		case "GOOS":
			v, ok := literalStringValue(elem.Y)
			if !ok {
				return out, fmt.Errorf("Defaults.GOOS must be a string literal")
			}
			out.GOOS = v
		case "GOARCH":
			v, ok := literalStringValue(elem.Y)
			if !ok {
				return out, fmt.Errorf("Defaults.GOARCH must be a string literal")
			}
			out.GOARCH = v
		case "Backend":
			v, ok := literalStringValue(elem.Y)
			if !ok {
				return out, fmt.Errorf("Defaults.Backend must be a string literal")
			}
			out.Backend = v
		case "PtrSize":
			v, ok := literalIntValue(elem.Y)
			if !ok {
				return out, fmt.Errorf("Defaults.PtrSize must be an integer literal")
			}
			out.PtrSize = int(v)
		case "WordSize":
			v, ok := literalIntValue(elem.Y)
			if !ok {
				return out, fmt.Errorf("Defaults.WordSize must be an integer literal")
			}
			out.WordSize = int(v)
		}
	}
	return out, nil
}

func parseSpecLiteral(expr *frontend.Node) (Spec, error) {
	var spec Spec
	if expr == nil || expr.Kind != frontend.NCompositeLit {
		return spec, fmt.Errorf("target spec must be a composite literal")
	}
	for _, elem := range expr.Nodes {
		if elem == nil || elem.Kind != frontend.NKeyValue {
			return spec, fmt.Errorf("target spec requires keyed fields")
		}
		key := keyName(elem.X)
		switch key {
		case "Triple":
			v, ok := literalStringValue(elem.Y)
			if !ok {
				return spec, fmt.Errorf("Spec.Triple must be a string literal")
			}
			spec.Triple = v
		case "PackagePath":
			v, ok := literalStringValue(elem.Y)
			if !ok {
				return spec, fmt.Errorf("Spec.PackagePath must be a string literal")
			}
			spec.PackagePath = v
		case "Assembler":
			v, ok := literalStringValue(elem.Y)
			if !ok {
				return spec, fmt.Errorf("Spec.Assembler must be a string literal")
			}
			spec.Assembler = v
		case "BinFormat":
			v, ok := literalStringValue(elem.Y)
			if !ok {
				return spec, fmt.Errorf("Spec.BinFormat must be a string literal")
			}
			spec.BinFormat = v
		case "Defaults":
			def, err := parseDefaultsLiteral(elem.Y)
			if err != nil {
				return spec, err
			}
			spec.Defaults = def
		case "Driver":
			return spec, fmt.Errorf("Spec.Driver is not supported in single-file target definitions")
		}
	}
	return spec, nil
}

func parseStringMapLiteral(expr *frontend.Node) (map[string]string, error) {
	out := make(map[string]string)
	if expr == nil || expr.Kind != frontend.NCompositeLit {
		return out, fmt.Errorf("map value must be a composite literal")
	}
	for _, elem := range expr.Nodes {
		if elem == nil || elem.Kind != frontend.NKeyValue {
			return nil, fmt.Errorf("map requires keyed elements")
		}
		k, ok := literalStringValue(elem.X)
		if !ok {
			return nil, fmt.Errorf("map key must be a string literal")
		}
		v, ok := literalStringValue(elem.Y)
		if !ok {
			return nil, fmt.Errorf("map value must be a string literal")
		}
		out[k] = v
	}
	return out, nil
}

func parseBoolMapLiteral(expr *frontend.Node) (map[string]bool, error) {
	out := make(map[string]bool)
	if expr == nil || expr.Kind != frontend.NCompositeLit {
		return out, fmt.Errorf("map value must be a composite literal")
	}
	for _, elem := range expr.Nodes {
		if elem == nil || elem.Kind != frontend.NKeyValue {
			return nil, fmt.Errorf("map requires keyed elements")
		}
		k, ok := literalStringValue(elem.X)
		if !ok {
			return nil, fmt.Errorf("map key must be a string literal")
		}
		v, ok := literalBoolValue(elem.Y)
		if !ok {
			return nil, fmt.Errorf("map value must be a bool literal")
		}
		out[k] = v
	}
	return out, nil
}

func parseInt64MapLiteral(expr *frontend.Node) (map[string]int64, error) {
	out := make(map[string]int64)
	if expr == nil || expr.Kind != frontend.NCompositeLit {
		return out, fmt.Errorf("map value must be a composite literal")
	}
	for _, elem := range expr.Nodes {
		if elem == nil || elem.Kind != frontend.NKeyValue {
			return nil, fmt.Errorf("map requires keyed elements")
		}
		k, ok := literalStringValue(elem.X)
		if !ok {
			return nil, fmt.Errorf("map key must be a string literal")
		}
		v, ok := literalIntValue(elem.Y)
		if !ok {
			return nil, fmt.Errorf("map value must be an integer literal")
		}
		out[k] = v
	}
	return out, nil
}

func parseUint64MapLiteral(expr *frontend.Node) (map[string]uint64, error) {
	out := make(map[string]uint64)
	if expr == nil || expr.Kind != frontend.NCompositeLit {
		return out, fmt.Errorf("map value must be a composite literal")
	}
	for _, elem := range expr.Nodes {
		if elem == nil || elem.Kind != frontend.NKeyValue {
			return nil, fmt.Errorf("map requires keyed elements")
		}
		k, ok := literalStringValue(elem.X)
		if !ok {
			return nil, fmt.Errorf("map key must be a string literal")
		}
		v, ok := literalIntValue(elem.Y)
		if !ok || v < 0 {
			return nil, fmt.Errorf("map value must be a non-negative integer literal")
		}
		out[k] = uint64(v)
	}
	return out, nil
}

func parseABILiteral(expr *frontend.Node) (ABIProvider, error) {
	if expr == nil {
		return ABIProvider{Kind: "none"}, nil
	}
	if expr.Kind == frontend.NBasicLit && expr.Name == "nil" {
		return ABIProvider{Kind: "none"}, nil
	}
	if expr.Kind != frontend.NCompositeLit {
		return ABIProvider{}, fmt.Errorf("ABI payload must be nil or a composite literal")
	}
	abi := ABIProvider{
		U64:  make(map[string]uint64),
		I64:  make(map[string]int64),
		Str:  make(map[string]string),
		Bool: make(map[string]bool),
	}
	for _, elem := range expr.Nodes {
		if elem == nil || elem.Kind != frontend.NKeyValue {
			return ABIProvider{}, fmt.Errorf("ABI payload requires keyed fields")
		}
		key := keyName(elem.X)
		switch key {
		case "Kind":
			v, ok := literalStringValue(elem.Y)
			if !ok {
				return ABIProvider{}, fmt.Errorf("ABI Kind must be a string literal")
			}
			abi.Kind = v
		case "U64":
			m, err := parseUint64MapLiteral(elem.Y)
			if err != nil {
				return ABIProvider{}, err
			}
			abi.U64 = m
		case "I64":
			m, err := parseInt64MapLiteral(elem.Y)
			if err != nil {
				return ABIProvider{}, err
			}
			abi.I64 = m
		case "Str":
			m, err := parseStringMapLiteral(elem.Y)
			if err != nil {
				return ABIProvider{}, err
			}
			abi.Str = m
		case "Bool":
			m, err := parseBoolMapLiteral(elem.Y)
			if err != nil {
				return ABIProvider{}, err
			}
			abi.Bool = m
		}
	}
	if abi.Kind == "" {
		abi.Kind = "generic"
	}
	return abi, nil
}

func LoadTargetFile(path string) error {
	file := frontend.ParseFile(path)
	if file == nil {
		return fmt.Errorf("parse failed for target file %s", path)
	}
	loaded := false

	for _, node := range file.Nodes {
		base, directives := unwrapDirectiveNode(node)
		if base == nil || base.Kind != frontend.NFunc {
			continue
		}
		expr, err := expectSingleReturnExpr(base)
		if err != nil {
			continue
		}
		for _, d := range directives {
			if triple, ok := parseTargetDirective(d); ok {
				spec, err := parseSpecLiteral(expr)
				if err != nil {
					return fmt.Errorf("%s: invalid //rtg:target %s: %v", path, triple, err)
				}
				if spec.Triple == "" {
					spec.Triple = triple
				}
				if spec.Triple != triple {
					return fmt.Errorf("%s: //rtg:target %s conflicts with Spec.Triple %s", path, triple, spec.Triple)
				}
				if spec.PackagePath == "" {
					spec.PackagePath = "targetfile:" + path
				}
				RegisterExternal(spec)
				loaded = true
			}
			if triple, ok := parseTargetABIDirective(d); ok {
				abi, err := parseABILiteral(expr)
				if err != nil {
					return fmt.Errorf("%s: invalid //rtg:targetabi %s: %v", path, triple, err)
				}
				RegisterExternalABI(triple, abi)
				loaded = true
			}
			if name, ok := parseAssemblerDirective(d); ok {
				provider, ok := literalStringValue(expr)
				if !ok {
					return fmt.Errorf("%s: //rtg:assembler %s requires a string literal return", path, name)
				}
				RegisterAssembler(name, provider)
				loaded = true
			}
			if name, ok := parseBinFormatDirective(d); ok {
				provider, ok := literalStringValue(expr)
				if !ok {
					return fmt.Errorf("%s: //rtg:binfmt %s requires a string literal return", path, name)
				}
				RegisterBinFormat(name, provider)
				loaded = true
			}
		}
	}

	if !loaded {
		return fmt.Errorf("%s: no target directives found", path)
	}
	return nil
}

func LoadTargetFiles(paths []string) error {
	for _, path := range paths {
		if err := LoadTargetFile(path); err != nil {
			return err
		}
	}
	return nil
}

func walkTargetFiles(root string, out []string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	for _, entry := range entries {
		name := entry.Name()
		path := root + "/" + name
		if entry.IsDir() {
			out = walkTargetFiles(path, out)
			continue
		}
		if len(name) >= 3 && name[len(name)-3:len(name)] == ".go" {
			out = append(out, path)
		}
	}
	return out
}

func LoadTargetRoot(root string) error {
	files := walkTargetFiles(root, nil)
	if len(files) == 0 {
		return fmt.Errorf("%s: no .go files found", root)
	}
	return LoadTargetFiles(files)
}

func LoadTargetRoots(roots []string) error {
	for _, root := range roots {
		if err := LoadTargetRoot(root); err != nil {
			return err
		}
	}
	return nil
}
