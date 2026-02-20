//go:build !rtg || no_embed_std

package stdlib

func HasEmbeddedStd() bool {
	return false
}

func WalkEmbedFromFS(embedDir string) ([]string, []string) {
	return nil, nil
}
