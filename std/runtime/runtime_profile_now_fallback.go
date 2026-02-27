//go:build !((linux && (amd64 || 386 || arm64)) || (windows && (amd64 || 386 || arm64)) || (darwin && arm64) || c || (wasi && wasm32))

package runtime

func profileNow() int {
	return 0
}
