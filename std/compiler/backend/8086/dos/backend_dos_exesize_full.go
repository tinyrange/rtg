//go:build !no_backend_dos_i386 && !tiny_dos_backend

package dos

func exeSegmentTooLarge(textSize uint32, dataSegSize uint32) bool {
	return textSize >= segLimitU32 || dataSegSize >= segLimitU32
}
