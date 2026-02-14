//go:build darwin && arm64

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
	MmapAnonFlags  = 0x1002 // MAP_PRIVATE(0x02) | MAP_ANON(0x1000)
)

var GOOS string = "darwin"
var GOARCH string = "arm64"

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

//rtg:internal SysDup2
func SysDup2(old, new_ uintptr) (uintptr, uintptr, int32)

//rtg:internal SysFork
func SysFork() (uintptr, uintptr, int32)

//rtg:internal SysExecve
func SysExecve(path, argv, envp uintptr) (uintptr, uintptr, int32)

//rtg:internal SysWait4
func SysWait4(pid, status, opts, rusage uintptr) (uintptr, uintptr, int32)

//rtg:internal SysPipe
func SysPipe(fds uintptr) (uintptr, uintptr, int32)

//rtg:internal SysOpendir
func SysOpendir(path uintptr) (uintptr, uintptr, int32)

//rtg:internal SysReaddir
func SysReaddir(dirp uintptr) (uintptr, uintptr, int32)

//rtg:internal SysClosedir
func SysClosedir(dirp uintptr) (uintptr, uintptr, int32)

//rtg:internal SysGetargc
func SysGetargc() (uintptr, uintptr, int32)

//rtg:internal SysGetargv
func SysGetargv() (uintptr, uintptr, int32)

//rtg:internal SysGetenvp
func SysGetenvp() (uintptr, uintptr, int32)

//rtg:internal SysGetpid
func SysGetpid() (uintptr, uintptr, int32)
