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

//rtg:linkstatic libSystem.dylib,_read
func SysRead(fd, buf, count uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_write
func SysWrite(fd, buf, count uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_open
func SysOpen(path, flags, mode uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_close
func SysClose(fd uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_stat
func SysStat(path, buf uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_mkdir
func SysMkdir(path, mode uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_rmdir
func SysRmdir(path uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_unlink
func SysUnlink(path uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_getcwd,ptr
func SysGetcwd(buf, size uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_exit,noreturn
func SysExit(code uintptr)

//rtg:linkstatic libSystem.dylib,_mmap,ptr
func SysMmap(addr, length, prot, flags, fd, offset uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_chmod
func SysChmod(path, mode uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_dup2
func SysDup2(old, new_ uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_fork
func SysFork() (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_execve
func SysExecve(path, argv, envp uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_wait4
func SysWait4(pid, status, opts, rusage uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_pipe
func SysPipe(fds uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_opendir,ptr
func SysOpendir(path uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_readdir,rawptr
func SysReaddir(dirp uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_closedir
func SysClosedir(dirp uintptr) (uintptr, uintptr, int32)

//rtg:internal SysGetargc
func SysGetargc() (uintptr, uintptr, int32)

//rtg:internal SysGetargv
func SysGetargv() (uintptr, uintptr, int32)

//rtg:internal SysGetenvp
func SysGetenvp() (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_getpid
func SysGetpid() (uintptr, uintptr, int32)
