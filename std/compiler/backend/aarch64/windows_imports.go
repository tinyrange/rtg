package aarch64

import "strings"

const (
	winDefaultImportLibraryArm64 = "kernel32.dll"
	iatFixupPrefixArm64          = "$iat$"
	winImportSeparatorArm64      = "|"
)

type winImportArm64 struct {
	Library string
	Symbol  string
}

type winImportGroupArm64 struct {
	Library string
	Symbols []string
}

func canonicalWinImportLibraryArm64(lib string) string {
	lib = strings.TrimSpace(lib)
	if lib == "" {
		return winDefaultImportLibraryArm64
	}
	return lib
}

func winImportKeyArm64(lib string, sym string) string {
	return canonicalWinImportLibraryArm64(lib) + winImportSeparatorArm64 + sym
}

func encodeIATFixupTargetArm64(lib string, sym string) string {
	return iatFixupPrefixArm64 + winImportKeyArm64(lib, sym)
}

func decodeIATFixupTargetArm64(target string) (string, string, bool) {
	if len(target) <= len(iatFixupPrefixArm64) || target[0:len(iatFixupPrefixArm64)] != iatFixupPrefixArm64 {
		return "", "", false
	}
	raw := target[len(iatFixupPrefixArm64):len(target)]
	sep := strings.Index(raw, winImportSeparatorArm64)
	if sep < 0 {
		if raw == "" {
			return "", "", false
		}
		return winDefaultImportLibraryArm64, raw, true
	}
	lib := canonicalWinImportLibraryArm64(raw[0:sep])
	sym := raw[sep+len(winImportSeparatorArm64) : len(raw)]
	if sym == "" {
		return "", "", false
	}
	return lib, sym, true
}

func collectWinImportsFromFixupsArm64(callFixups []CallFixup) []winImportArm64 {
	var imports []winImportArm64
	seen := make(map[string]bool)
	for _, fix := range callFixups {
		lib, sym, ok := decodeIATFixupTargetArm64(fix.Target)
		if !ok {
			continue
		}
		key := winImportKeyArm64(lib, sym)
		if seen[key] {
			continue
		}
		seen[key] = true
		imports = append(imports, winImportArm64{Library: lib, Symbol: sym})
	}

	i := 1
	for i < len(imports) {
		j := i
		for j > 0 {
			prev := winImportKeyArm64(imports[j-1].Library, imports[j-1].Symbol)
			cur := winImportKeyArm64(imports[j].Library, imports[j].Symbol)
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

func groupWinImportsArm64(imports []winImportArm64) []winImportGroupArm64 {
	var groups []winImportGroupArm64
	for _, imp := range imports {
		if len(groups) == 0 || groups[len(groups)-1].Library != imp.Library {
			groups = append(groups, winImportGroupArm64{
				Library: imp.Library,
				Symbols: []string{imp.Symbol},
			})
			continue
		}
		groups[len(groups)-1].Symbols = append(groups[len(groups)-1].Symbols, imp.Symbol)
	}
	return groups
}
