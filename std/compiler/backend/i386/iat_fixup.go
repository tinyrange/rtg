package i386

import "strings"

const (
	winDefaultImportLibrary = "kernel32.dll"
	iatFixupPrefix          = "$iat$"
	winImportSeparator      = "|"
)

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
