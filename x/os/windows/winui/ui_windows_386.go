package winui

import "runtime"

const (
	ptrSize        = 4
	sizeWNDCLASSEX = 48
	sizeMSG        = 28

	// WNDCLASSEXA field offsets (i386)
	offWCE_cbSize      = 0
	offWCE_style       = 4
	offWCE_lpfnWndProc = 8
	offWCE_cbClsExtra  = 12
	offWCE_cbWndExtra  = 16
	offWCE_hInstance   = 20
	offWCE_hIcon       = 24
	offWCE_hCursor     = 28
	offWCE_hbrBg       = 32
	offWCE_lpszMenu    = 36
	offWCE_lpszClass   = 40
	offWCE_hIconSm     = 44

	// MSG.wParam offset (i386)
	offMSG_wParam = 8
)

// rtg:zerocall
func readPtr(addr uintptr) uintptr {
	return uintptr(readU32(addr))
}

// rtg:zerocall
func writePtr(addr uintptr, v uintptr) {
	writeU32(addr, uint32(v))
}

// allocMsg allocates and zeroes a MSG struct.
// rtg:zerocall
func allocMsg() uintptr {
	msg := runtime.Alloc(sizeMSG)
	runtime.Memzero(msg, sizeMSG)
	return msg
}
