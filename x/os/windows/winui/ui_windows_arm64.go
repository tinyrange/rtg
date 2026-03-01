package winui

import "runtime"

const (
	ptrSize        = 8
	sizeWNDCLASSEX = 80
	sizeMSG        = 48

	// WNDCLASSEXA field offsets (arm64, Win64 ABI)
	offWCE_cbSize      = 0
	offWCE_style       = 4
	offWCE_lpfnWndProc = 8
	offWCE_cbClsExtra  = 16
	offWCE_cbWndExtra  = 20
	offWCE_hInstance   = 24
	offWCE_hIcon       = 32
	offWCE_hCursor     = 40
	offWCE_hbrBg       = 48
	offWCE_lpszMenu    = 56
	offWCE_lpszClass   = 64
	offWCE_hIconSm     = 72

	// MSG.wParam offset (arm64, Win64 ABI)
	offMSG_wParam = 16
)

// rtg:zerocall
func readPtr(addr uintptr) uintptr {
	return uintptr(readU64(addr))
}

// rtg:zerocall
func writePtr(addr uintptr, v uintptr) {
	writeU64(addr, uint64(v))
}

// allocMsg allocates and zeroes a MSG struct.
func allocMsg() uintptr {
	msg := runtime.Alloc(sizeMSG)
	runtime.Memzero(msg, sizeMSG)
	return msg
}
