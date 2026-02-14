//go:build linux && 386

package runtime

const (
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

//rtg:internal Syscall
func Syscall(num int32, a0, a1, a2, a3, a4, a5 uintptr) (r1 uintptr, r2 uintptr, err int32)

func SysRead(fd, buf, count uintptr) (uintptr, uintptr, int32)                { return Syscall(3, fd, buf, count, 0, 0, 0) }
func SysWrite(fd, buf, count uintptr) (uintptr, uintptr, int32)               { return Syscall(4, fd, buf, count, 0, 0, 0) }
func SysOpen(path, flags, mode uintptr) (uintptr, uintptr, int32)             { return Syscall(5, path, flags, mode, 0, 0, 0) }
func SysClose(fd uintptr) (uintptr, uintptr, int32)                           { return Syscall(6, fd, 0, 0, 0, 0, 0) }
func SysStat(path, buf uintptr) (uintptr, uintptr, int32)                     { return Syscall(106, path, buf, 0, 0, 0, 0) }
func SysDup2(old, new_ uintptr) (uintptr, uintptr, int32)                     { return Syscall(63, old, new_, 0, 0, 0, 0) }
func SysFork() (uintptr, uintptr, int32)                                      { return Syscall(2, 0, 0, 0, 0, 0, 0) }
func SysExecve(path, argv, envp uintptr) (uintptr, uintptr, int32)            { return Syscall(11, path, argv, envp, 0, 0, 0) }
func SysWait4(pid, status, opts, rusage uintptr) (uintptr, uintptr, int32)    { return Syscall(114, pid, status, opts, rusage, 0, 0) }
func SysGetcwd(buf, size uintptr) (uintptr, uintptr, int32)                   { return Syscall(183, buf, size, 0, 0, 0, 0) }
func SysMkdir(path, mode uintptr) (uintptr, uintptr, int32)                   { return Syscall(39, path, mode, 0, 0, 0, 0) }
func SysRmdir(path uintptr) (uintptr, uintptr, int32)                         { return Syscall(40, path, 0, 0, 0, 0, 0) }
func SysUnlink(path uintptr) (uintptr, uintptr, int32)                        { return Syscall(10, path, 0, 0, 0, 0, 0) }
func SysChmod(path, mode uintptr) (uintptr, uintptr, int32)                   { return Syscall(15, path, mode, 0, 0, 0, 0) }
func SysGetdents64(fd, buf, size uintptr) (uintptr, uintptr, int32)           { return Syscall(220, fd, buf, size, 0, 0, 0) }
func SysExit(code uintptr)                                                    { Syscall(252, code, 0, 0, 0, 0, 0) }
func SysMmap(addr, length, prot, flags, fd, offset uintptr) (uintptr, uintptr, int32) { return Syscall(192, addr, length, prot, flags, fd, offset) }
func SysPipe(fds uintptr) (uintptr, uintptr, int32)                           { return Syscall(331, fds, 0, 0, 0, 0, 0) }
func SysGetpid() (uintptr, uintptr, int32)                                    { return Syscall(20, 0, 0, 0, 0, 0, 0) }
