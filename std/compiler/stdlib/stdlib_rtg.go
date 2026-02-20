//go:build rtg && !no_embed_std

package main

import (
	"embed"
)

//go:embed ..
var embeddedStd embed.FS

func HasEmbeddedStd() bool {
	return true
}

func WalkEmbedFromFS(embedDir string) ([]string, []string) {
	return embeddedStd.WalkDir(embedDir)
}
