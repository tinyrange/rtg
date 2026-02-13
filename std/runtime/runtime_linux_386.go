//go:build linux && 386

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
	MmapAnonFlags  = 34 // MAP_PRIVATE(0x02) | MAP_ANONYMOUS(0x20)
)

var GOOS string = "linux"
var GOARCH string = "386"
