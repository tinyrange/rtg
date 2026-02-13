//go:build c && c32

package runtime

const (
	SYS_MMAP       int32 = 10
	SYS_WRITE      int32 = 1
	SYS_EXIT_GROUP int32 = 9
	PtrSize              = 4
	SliceHdrSize         = 16
	StringHdrSize        = 8
	IfaceBoxSize         = 8
	SliceOffLen          = 4
	SliceOffCap          = 8
	SliceOffEsz          = 12
	MapEntrySize         = 8
	MapEntryOffVal       = 4
	MmapAnonFlags        = 34 // MAP_PRIVATE(0x02) | MAP_ANONYMOUS(0x20)
)

var GOOS string = "c"
var GOARCH string = "c32"
