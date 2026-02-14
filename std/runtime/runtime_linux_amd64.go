//go:build linux && amd64

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
	MmapAnonFlags  = 34 // MAP_PRIVATE(0x02) | MAP_ANONYMOUS(0x20)
)

var GOOS string = "linux"
var GOARCH string = "amd64"

//rtg:internal Syscall
func Syscall(num int32, a0, a1, a2, a3, a4, a5 uintptr) (r1 uintptr, r2 uintptr, err int32)

func SysRead(fd, buf, count uintptr) (uintptr, uintptr, int32)                { return Syscall(0, fd, buf, count, 0, 0, 0) }
func SysWrite(fd, buf, count uintptr) (uintptr, uintptr, int32)               { return Syscall(1, fd, buf, count, 0, 0, 0) }
func SysOpen(path, flags, mode uintptr) (uintptr, uintptr, int32)             { return Syscall(2, path, flags, mode, 0, 0, 0) }
func SysClose(fd uintptr) (uintptr, uintptr, int32)                           { return Syscall(3, fd, 0, 0, 0, 0, 0) }
func SysStat(path, buf uintptr) (uintptr, uintptr, int32)                     { return Syscall(4, path, buf, 0, 0, 0, 0) }
func SysDup2(old, new_ uintptr) (uintptr, uintptr, int32)                     { return Syscall(33, old, new_, 0, 0, 0, 0) }
func SysFork() (uintptr, uintptr, int32)                                      { return Syscall(57, 0, 0, 0, 0, 0, 0) }
func SysExecve(path, argv, envp uintptr) (uintptr, uintptr, int32)            { return Syscall(59, path, argv, envp, 0, 0, 0) }
func SysWait4(pid, status, opts, rusage uintptr) (uintptr, uintptr, int32)    { return Syscall(61, pid, status, opts, rusage, 0, 0) }
func SysGetcwd(buf, size uintptr) (uintptr, uintptr, int32)                   { return Syscall(79, buf, size, 0, 0, 0, 0) }
func SysMkdir(path, mode uintptr) (uintptr, uintptr, int32)                   { return Syscall(83, path, mode, 0, 0, 0, 0) }
func SysRmdir(path uintptr) (uintptr, uintptr, int32)                         { return Syscall(84, path, 0, 0, 0, 0, 0) }
func SysUnlink(path uintptr) (uintptr, uintptr, int32)                        { return Syscall(87, path, 0, 0, 0, 0, 0) }
func SysChmod(path, mode uintptr) (uintptr, uintptr, int32)                   { return Syscall(90, path, mode, 0, 0, 0, 0) }
func SysGetdents64(fd, buf, size uintptr) (uintptr, uintptr, int32)           { return Syscall(217, fd, buf, size, 0, 0, 0) }
func SysExit(code uintptr)                                                    { Syscall(231, code, 0, 0, 0, 0, 0) }
func SysMmap(addr, length, prot, flags, fd, offset uintptr) (uintptr, uintptr, int32) { return Syscall(9, addr, length, prot, flags, fd, offset) }
func SysPipe(fds uintptr) (uintptr, uintptr, int32)                           { return Syscall(293, fds, 0, 0, 0, 0, 0) }
func SysGetpid() (uintptr, uintptr, int32)                                    { return Syscall(39, 0, 0, 0, 0, 0, 0) }
