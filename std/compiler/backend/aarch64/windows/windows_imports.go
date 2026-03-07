package windows

import (
	"strings"

	"j5.nz/rtg/std/compiler/backend/aarch64"
)

const (
	winDefaultImportLibrary = "kernel32.dll"
	iatFixupPrefix          = "$iat$"
	winImportSeparator      = "|"
)

type winImport struct {
	Library string
	Symbol  string
}

type winImportGroup struct {
	Library string
	Symbols []string
}

func canonicalWinImportLibrary(lib string) string {
	lib = strings.TrimSpace(lib)
	if lib == "" {
		return winDefaultImportLibrary
	}
	return lib
}

func winImportKey(lib string, sym string) string {
	return canonicalWinImportLibrary(lib) + winImportSeparator + sym
}

func encodeIATFixupTarget(lib string, sym string) string {
	return iatFixupPrefix + winImportKey(lib, sym)
}

func decodeIATFixupTarget(target string) (string, string, bool) {
	if len(target) <= len(iatFixupPrefix) || target[0:len(iatFixupPrefix)] != iatFixupPrefix {
		return "", "", false
	}
	raw := target[len(iatFixupPrefix):len(target)]
	sep := strings.Index(raw, winImportSeparator)
	if sep < 0 {
		if raw == "" {
			return "", "", false
		}
		return winDefaultImportLibrary, raw, true
	}
	lib := canonicalWinImportLibrary(raw[0:sep])
	sym := raw[sep+len(winImportSeparator) : len(raw)]
	if sym == "" {
		return "", "", false
	}
	return lib, sym, true
}

func collectWinImportsFromFixups(g *aarch64.CodeGen) []winImport {
	var imports []winImport
	seen := make(map[string]bool)
	for i := 0; i < g.CallFixupCount(); i++ {
		_, target, _ := g.CallFixupAt(i)
		lib, sym, ok := decodeIATFixupTarget(target)
		if !ok {
			continue
		}
		key := winImportKey(lib, sym)
		if seen[key] {
			continue
		}
		seen[key] = true
		imports = append(imports, winImport{lib, sym})
	}

	i := 1
	for i < len(imports) {
		j := i
		for j > 0 {
			prev := winImportKey(imports[j-1].Library, imports[j-1].Symbol)
			cur := winImportKey(imports[j].Library, imports[j].Symbol)
			if cur >= prev {
				break
			}
			tmp := imports[j]
			imports[j] = imports[j-1]
			imports[j-1] = tmp
			j = j - 1
		}
		i = i + 1
	}
	return imports
}

func groupWinImports(imports []winImport) []winImportGroup {
	var groups []winImportGroup
	for _, imp := range imports {
		if len(groups) == 0 || groups[len(groups)-1].Library != imp.Library {
			groups = append(groups, winImportGroup{imp.Library, []string{imp.Symbol}})
			continue
		}
		groups[len(groups)-1].Symbols = append(groups[len(groups)-1].Symbols, imp.Symbol)
	}
	return groups
}
