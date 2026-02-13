//go:build c && c64

package runtime

const (
	SYS_MMAP       int32 = 10
	SYS_WRITE      int32 = 1
	SYS_EXIT_GROUP int32 = 9
	PtrSize              = 8
	SliceHdrSize         = 32
	StringHdrSize        = 16
	IfaceBoxSize         = 16
	SliceOffLen          = 8
	SliceOffCap          = 16
	SliceOffEsz          = 24
	MapEntrySize         = 16
	MapEntryOffVal       = 8
	MmapAnonFlags        = 34 // MAP_PRIVATE(0x02) | MAP_ANONYMOUS(0x20)
)

var GOOS string = "c"
var GOARCH string = "c64"
