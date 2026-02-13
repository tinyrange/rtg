//go:build c && c16

package runtime

const (
	SYS_MMAP       int32 = 10
	SYS_WRITE      int32 = 1
	SYS_EXIT_GROUP int32 = 9
	PtrSize              = 2
	SliceHdrSize         = 8
	StringHdrSize        = 4
	IfaceBoxSize         = 4
	SliceOffLen          = 2
	SliceOffCap          = 4
	SliceOffEsz          = 6
	MapEntrySize         = 4
	MapEntryOffVal       = 2
	MmapAnonFlags        = 34 // MAP_PRIVATE(0x02) | MAP_ANONYMOUS(0x20)
)

var GOOS string = "c"
var GOARCH string = "c16"
