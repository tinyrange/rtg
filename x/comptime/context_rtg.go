//go:build rtg

package comptime

func ReadFile(path string) (string, bool) {
	return hostReadFile(path)
}

//rtg:internal ComptimeReadFile
func hostReadFile(path string) (string, bool)
