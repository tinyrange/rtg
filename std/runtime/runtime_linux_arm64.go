//go:build linux && arm64

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
var GOARCH string = "arm64"

// AT_FDCWD = -100; computed at runtime to avoid constant overflow
var atFdcwd uintptr

func init() {
	var zero uintptr
	atFdcwd = zero - 100
}

//rtg:internal Syscall
func Syscall(num int32, a0, a1, a2, a3, a4, a5 uintptr) (r1 uintptr, r2 uintptr, err int32)

// ARM64 Linux only has *at variants for file syscalls.

func SysRead(fd, buf, count uintptr) (uintptr, uintptr, int32)    { return Syscall(63, fd, buf, count, 0, 0, 0) }
func SysWrite(fd, buf, count uintptr) (uintptr, uintptr, int32)   { return Syscall(64, fd, buf, count, 0, 0, 0) }
func SysOpen(path, flags, mode uintptr) (uintptr, uintptr, int32) { return Syscall(56, atFdcwd, path, flags, mode, 0, 0) }
func SysClose(fd uintptr) (uintptr, uintptr, int32)               { return Syscall(57, fd, 0, 0, 0, 0, 0) }
func SysStat(path, buf uintptr) (uintptr, uintptr, int32)         { return Syscall(79, atFdcwd, path, buf, 0, 0, 0) }
func SysDup2(old, new_ uintptr) (uintptr, uintptr, int32)         { return Syscall(24, old, new_, 0, 0, 0, 0) }
func SysFork() (uintptr, uintptr, int32)                          { return Syscall(220, 17, 0, 0, 0, 0, 0) }
func SysExecve(path, argv, envp uintptr) (uintptr, uintptr, int32) { return Syscall(221, path, argv, envp, 0, 0, 0) }
func SysWait4(pid, status, opts, rusage uintptr) (uintptr, uintptr, int32) { return Syscall(260, pid, status, opts, rusage, 0, 0) }
func SysGetcwd(buf, size uintptr) (uintptr, uintptr, int32)       { return Syscall(17, buf, size, 0, 0, 0, 0) }
func SysMkdir(path, mode uintptr) (uintptr, uintptr, int32)       { return Syscall(34, atFdcwd, path, mode, 0, 0, 0) }
func SysRmdir(path uintptr) (uintptr, uintptr, int32)             { return Syscall(35, atFdcwd, path, 0x200, 0, 0, 0) }
func SysUnlink(path uintptr) (uintptr, uintptr, int32)            { return Syscall(35, atFdcwd, path, 0, 0, 0, 0) }
func SysChmod(path, mode uintptr) (uintptr, uintptr, int32)       { return Syscall(53, atFdcwd, path, mode, 0, 0, 0) }
func SysGetdents64(fd, buf, size uintptr) (uintptr, uintptr, int32) { return Syscall(61, fd, buf, size, 0, 0, 0) }
func SysExit(code uintptr)                                        { Syscall(94, code, 0, 0, 0, 0, 0) }
func SysMmap(addr, length, prot, flags, fd, offset uintptr) (uintptr, uintptr, int32) { return Syscall(222, addr, length, prot, flags, fd, offset) }
func SysPipe(fds uintptr) (uintptr, uintptr, int32)               { return Syscall(59, fds, 0, 0, 0, 0, 0) }
func SysGetpid() (uintptr, uintptr, int32)                        { return Syscall(172, 0, 0, 0, 0, 0, 0) }
