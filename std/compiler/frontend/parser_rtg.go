//go:build rtg && !no_embed_std

package frontend

import (
	"j5.nz/rtg/std/compiler/stdlib"
)

func (p *Preprocessor) parsePackageFromEmbed(importPath string) *Package {
	// List files in the embedded std directory for this import path
	files := stdlib.ReadDirFromEmbed(importPath)
	if len(files) == 0 {
		return nil
	}

	// Filter and sort .go files
	var goFiles []string
	i := 0
	for i < len(files) {
		name := files[i]
		if isGoFile(name) {
			content := stdlib.ReadFileFromEmbed(importPath + "/" + name)
			if p.shouldIncludeContent(content, name) {
				goFiles = append(goFiles, name)
			}
		}
		i = i + 1
	}
	sortStrings(goFiles)

	pkg := &Package{
		Path:    importPath,
		Dir:     importPath,
		Symbols: make(map[string]*Symbol),
	}

	i = 0
	for i < len(goFiles) {
		name := goFiles[i]
		content := stdlib.ReadFileFromEmbed(importPath + "/" + name)
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
