//go:build no_embed_std

package frontend

//rtg:profile
func (p *Preprocessor) parsePackageFromEmbed(importPath string) *Package {
	return nil
}
