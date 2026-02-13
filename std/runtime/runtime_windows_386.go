//go:build windows && 386

package runtime

const (
	SYS_MMAP       int32 = 192
	SYS_WRITE      int32 = 4
	SYS_EXIT_GROUP int32 = 252
	PtrSize        = 4
	SliceHdrSize   = 16
	StringHdrSize  = 8
	IfaceBoxSize   = 8
	SliceOffLen    = 4
	SliceOffCap    = 8
	SliceOffEsz    = 12
	MapEntrySize   = 8
	MapEntryOffVal = 4
	MmapAnonFlags  = 0 // not used on Windows
)

var GOOS string = "windows"
var GOARCH string = "386"
