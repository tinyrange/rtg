//go:build wasi && wasm32

package runtime

const (
	SYS_MMAP       int32 = 9
	SYS_WRITE      int32 = 1
	SYS_EXIT_GROUP int32 = 231
	PtrSize        = 4
	SliceHdrSize   = 16
	StringHdrSize  = 8
	IfaceBoxSize   = 8
	SliceOffLen    = 4
	SliceOffCap    = 8
	SliceOffEsz    = 12
	MapEntrySize   = 8
	MapEntryOffVal = 4
	MmapAnonFlags  = 0 // not applicable on WASI
)

var GOOS string = "wasi"
var GOARCH string = "wasm32"
