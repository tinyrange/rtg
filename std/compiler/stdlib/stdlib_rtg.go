//go:build rtg && !no_embed_std

package stdlib

import (
	"embed"
)

//go:embed ../..
var embeddedStd embed.FS

func HasEmbeddedStd() bool {
	return true
}

func WalkEmbedFromFS(embedDir string) ([]string, []string) {
	return embeddedStd.WalkDir(embedDir)
}

func ReadDirFromEmbed(path string) []string {
	return embeddedStd.ReadDir(path)
}

func ReadFileFromEmbed(path string) string {
	return embeddedStd.ReadFile(path)
}
