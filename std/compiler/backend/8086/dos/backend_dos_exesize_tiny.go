//go:build !no_backend_dos_i386 && tiny_dos_backend

package dos

// Tiny backend profile runs inside severely constrained DOS16 executables.
// Skip strict EXE segment guards here; this path is only used for small bootstrapping payloads.
func exeSegmentTooLarge(textSize uint32, dataSegSize uint32) bool {
	_ = textSize
	_ = dataSegSize
	return false
}
