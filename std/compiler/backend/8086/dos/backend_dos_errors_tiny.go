//go:build !no_backend_dos_i386 && tiny_dos_backend

package dos

type tinyBackendError string

//rtg:profile
func (e tinyBackendError) Error() string { return string(e) }

func reportUnresolvedCalls(unresolved []string) {
	_ = unresolved
}

func errUnresolvedCalls(count int) error {
	_ = count
	return tinyBackendError("dos backend: unresolved calls")
}

func errWriteOutput(err error) error {
	_ = err
	return tinyBackendError("dos backend: write output failed")
}

func errCOMTooLarge(total int, max int, text int, rodata int, data int) error {
	_ = total
	_ = max
	_ = text
	_ = rodata
	_ = data
	return tinyBackendError("dos backend: COM image too large")
}
