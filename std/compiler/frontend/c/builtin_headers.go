package frontend

import (
	"os"
	"path/filepath"
	"strings"

	"j5.nz/rtg/std/compiler/stdlib"
)

const builtinIncludeEmbedRoot = "std/compiler/frontend/c/include"

var builtinIncludeDiskPaths = discoverBuiltinIncludeDiskPaths()
var builtinIncludeEmbedded = loadBuiltinIncludeMap()

func dirExists(path string) bool {
	if path == "" {
		return false
	}
	if _, err := os.ReadDir(path); err != nil {
		return false
	}
	return true
}

func appendBuiltinIncludeCandidates(dst []string, start string) []string {
	start = cleanPath(start)
	if start == "" {
		return dst
	}
	cur := start
	suffix := filepath.Join("std", "compiler")
	suffix = filepath.Join(suffix, "frontend")
	suffix = filepath.Join(suffix, "c")
	suffix = filepath.Join(suffix, "include")
	for {
		direct := cleanPath(filepath.Join(cur, "include"))
		if dirExists(direct) {
			dst = appendUniqueString(dst, direct)
		}
		repoRelative := cleanPath(filepath.Join(cur, suffix))
		if dirExists(repoRelative) {
			dst = appendUniqueString(dst, repoRelative)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return dst
}

func appendUniqueString(dst []string, value string) []string {
	for _, existing := range dst {
		if existing == value {
			return dst
		}
	}
	return append(dst, value)
}

func discoverBuiltinIncludeDiskPaths() []string {
	var out []string
	if wd, err := os.Getwd(); err == nil {
		out = appendBuiltinIncludeCandidates(out, wd)
	}
	if len(os.Args) > 0 && os.Args[0] != "" {
		exeDir := filepath.Dir(os.Args[0])
		out = appendBuiltinIncludeCandidates(out, exeDir)
	}
	return out
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
	return append([]string{}, builtinIncludeDiskPaths...)
}

func readBuiltinInclude(path string) (string, bool) {
	path = cleanPath(path)
	if stdlib.HasEmbeddedStd() {
		src := builtinIncludeEmbedded[path]
		if src != "" {
			return src, true
		}
	}
	return "", false
}
