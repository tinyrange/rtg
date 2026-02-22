//go:build rtg

package comptime

type hostContext struct{}

func Host() Context {
	return hostContext{}
}

func (h hostContext) ReadFile(path string) (string, bool) {
	_ = h
	return hostReadFile(path)
}

//rtg:internal ComptimeReadFile
func hostReadFile(path string) (string, bool)
