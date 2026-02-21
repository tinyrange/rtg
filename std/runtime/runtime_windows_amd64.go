//go:build windows && amd64

package runtime

const (
	PtrSize        = 8
	SliceHdrSize   = 32
	StringHdrSize  = 16
	IfaceBoxSize   = 16
	SliceOffLen    = 8
	SliceOffCap    = 16
	SliceOffEsz    = 24
	MapEntrySize   = 16
	MapEntryOffVal = 8
	MmapAnonFlags  = 0 // not used on Windows
)

var GOOS string = "windows"
var GOARCH string = "amd64"
