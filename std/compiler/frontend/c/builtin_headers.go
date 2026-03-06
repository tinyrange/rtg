package frontend

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"j5.nz/rtg/std/compiler/stdlib"
)

const builtinIncludeEmbedRoot = "std/compiler/frontend/c/include"

var builtinIncludeDiskRoot = discoverBuiltinIncludeDiskRoot()
var builtinIncludeEmbedded = loadBuiltinIncludeMap()

func discoverBuiltinIncludeDiskRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	return cleanPath(filepath.Join(filepath.Dir(file), "include"))
}

func loadBuiltinIncludeMap() map[string]string {
	if !stdlib.HasEmbeddedStd() {
		return nil
	}
	names, data := stdlib.WalkEmbedFromFS(".")
	if len(names) == 0 {
		return nil
	}
	out := make(map[string]string)
	prefix := builtinIncludeEmbedRoot + "/"
	for i := 0; i < len(names) && i < len(data); i++ {
		name := cleanPath(names[i])
		if strings.HasPrefix(name, prefix) {
			out[name] = data[i]
		}
	}
	return out
}

func builtinIncludeSearchPaths(enable bool) []string {
	if !enable {
		return nil
	}
	if stdlib.HasEmbeddedStd() && len(builtinIncludeEmbedded) > 0 {
		return []string{builtinIncludeEmbedRoot}
	}
	if builtinIncludeDiskRoot == "" {
		return nil
	}
	if _, err := os.ReadDir(builtinIncludeDiskRoot); err == nil {
		return []string{builtinIncludeDiskRoot}
	}
	return nil
}

func readBuiltinInclude(path string) (string, bool) {
	path = cleanPath(path)
	if stdlib.HasEmbeddedStd() {
		src, ok := builtinIncludeEmbedded[path]
		return src, ok
	}
	return "", false
}
