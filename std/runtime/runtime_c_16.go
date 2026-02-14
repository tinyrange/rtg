//go:build c && c16

package runtime

const (
	PtrSize              = 2
	SliceHdrSize         = 8
	StringHdrSize        = 4
	IfaceBoxSize         = 4
	SliceOffLen          = 2
	SliceOffCap          = 4
	SliceOffEsz          = 6
	MapEntrySize         = 4
	MapEntryOffVal       = 2
	MmapAnonFlags        = 34
)

var GOOS string = "c"
var GOARCH string = "c16"

//rtg:internal SysRead
func SysRead(fd, buf, count uintptr) (uintptr, uintptr, int32)

//rtg:internal SysWrite
func SysWrite(fd, buf, count uintptr) (uintptr, uintptr, int32)

//rtg:internal SysOpen
func SysOpen(path, flags, mode uintptr) (uintptr, uintptr, int32)

//rtg:internal SysClose
func SysClose(fd uintptr) (uintptr, uintptr, int32)

//rtg:internal SysStat
func SysStat(path, buf uintptr) (uintptr, uintptr, int32)

//rtg:internal SysMkdir
func SysMkdir(path, mode uintptr) (uintptr, uintptr, int32)

//rtg:internal SysRmdir
func SysRmdir(path uintptr) (uintptr, uintptr, int32)

//rtg:internal SysUnlink
func SysUnlink(path uintptr) (uintptr, uintptr, int32)

//rtg:internal SysGetcwd
func SysGetcwd(buf, size uintptr) (uintptr, uintptr, int32)

//rtg:internal SysExit
func SysExit(code uintptr)

//rtg:internal SysMmap
func SysMmap(addr, length, prot, flags, fd, offset uintptr) (uintptr, uintptr, int32)

//rtg:internal SysChmod
func SysChmod(path, mode uintptr) (uintptr, uintptr, int32)

//rtg:internal SysGetargc
func SysGetargc() (uintptr, uintptr, int32)

//rtg:internal SysGetargv
func SysGetargv(index uintptr) (uintptr, uintptr, int32)

//rtg:internal SysGetenv
func SysGetenv(key uintptr) (uintptr, uintptr, int32)

//rtg:internal SysOpendir
func SysOpendir(path uintptr) (uintptr, uintptr, int32)

//rtg:internal SysReaddir
func SysReaddir(handle, nameBuf, nameBufSize uintptr) (uintptr, uintptr, int32)

//rtg:internal SysReaddirWithType
func SysReaddirWithType(handle, nameBuf, nameBufSize, isDirBuf uintptr) (uintptr, uintptr, int32)

//rtg:internal SysClosedir
func SysClosedir(handle uintptr) (uintptr, uintptr, int32)

//rtg:internal SysSystem
func SysSystem(cmd uintptr) (uintptr, uintptr, int32)

//rtg:internal SysPopen
func SysPopen(cmd uintptr) (uintptr, uintptr, int32)

//rtg:internal SysPclose
func SysPclose(fd uintptr) (uintptr, uintptr, int32)

//rtg:internal SysGetpid
func SysGetpid() (uintptr, uintptr, int32)
