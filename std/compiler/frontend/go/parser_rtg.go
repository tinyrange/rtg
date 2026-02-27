//go:build !no_embed_std

package frontend

import (
	"j5.nz/rtg/std/compiler/stdlib"
)

func (p *Preprocessor) parsePackageFromEmbed(importPath string) *Package {
	// Enumerate all embedded files and filter by package prefix. Some embed
	// runtimes are unreliable for non-dot walk roots.
	names, data := stdlib.WalkEmbedFromFS(".")
	if len(names) == 0 {
		return nil
	}
	contentsByPath := make(map[string]string, len(names))
	for i := 0; i < len(names) && i < len(data); i++ {
		contentsByPath[names[i]] = data[i]
	}

	// Filter and sort .go files in this package directory.
	var goFiles []string
	for _, full := range names {
		if len(full) <= len(importPath)+1 {
			continue
		}
		if full[0:len(importPath)] != importPath || full[len(importPath)] != '/' {
			continue
		}
		name := full[len(importPath)+1:]
		// Ignore nested files; package files must be direct children.
		if len(name) == 0 || stringsIndexByte(name, '/') >= 0 {
			continue
		}
		if !isGoFile(name) {
			continue
		}
		content := contentsByPath[full]
		if p.shouldIncludeContent(content, name) {
			goFiles = append(goFiles, name)
		}
	}
	sortStrings(goFiles)
	if len(goFiles) == 0 {
		return nil
	}

	pkg := &Package{
		Path:    importPath,
		Dir:     importPath,
		Symbols: make(map[string]*Symbol),
	}

	i := 0
	for i < len(goFiles) {
		name := goFiles[i]
		content := contentsByPath[importPath+"/"+name]
		node := parseSource(importPath+"/"+name, content)
		if node != nil {
			if pkg.Name == "" {
				pkg.Name = node.Name
			}
			pkg.Files = append(pkg.Files, node)
		}
		i = i + 1
	}

	if len(pkg.Files) == 0 {
		return nil
	}

	pkg.Imports = collectImports(pkg)
	return pkg
}

func stringsIndexByte(s string, c byte) int {
	i := 0
	for i < len(s) {
		if s[i] == c {
			return i
		}
		i = i + 1
	}
	return -1
}
