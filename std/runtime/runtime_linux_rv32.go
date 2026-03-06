//go:build linux && rv32

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
	MmapAnonFlags  = 34
)

var GOOS string = "linux"
var GOARCH string = "rv32"

var atFdcwd uintptr = ^uintptr(99)

//rtg:internal Syscall
func Syscall(num int32, a0, a1, a2, a3, a4, a5 uintptr) (r1 uintptr, r2 uintptr, err int32)

func SysRead(fd, buf, count uintptr) (uintptr, uintptr, int32) { return Syscall(63, fd, buf, count, 0, 0, 0) }
func SysWrite(fd, buf, count uintptr) (uintptr, uintptr, int32) { return Syscall(64, fd, buf, count, 0, 0, 0) }
func SysOpen(path, flags, mode uintptr) (uintptr, uintptr, int32) {
	return Syscall(56, atFdcwd, path, flags, mode, 0, 0)
}
func SysClose(fd uintptr) (uintptr, uintptr, int32) { return Syscall(57, fd, 0, 0, 0, 0, 0) }
func SysStat(path, buf uintptr) (uintptr, uintptr, int32) {
	return Syscall(79, atFdcwd, path, buf, 0, 0, 0)
}
func SysDup2(old, new_ uintptr) (uintptr, uintptr, int32) { return Syscall(24, old, new_, 0, 0, 0, 0) }
func SysFork() (uintptr, uintptr, int32)                  { return Syscall(220, 17, 0, 0, 0, 0, 0) }
func SysExecve(path, argv, envp uintptr) (uintptr, uintptr, int32) {
	return Syscall(221, path, argv, envp, 0, 0, 0)
}
func SysWait4(pid, status, opts, rusage uintptr) (uintptr, uintptr, int32) {
	return Syscall(260, pid, status, opts, rusage, 0, 0)
}
func SysGetcwd(buf, size uintptr) (uintptr, uintptr, int32) { return Syscall(17, buf, size, 0, 0, 0, 0) }
func SysMkdir(path, mode uintptr) (uintptr, uintptr, int32) {
	return Syscall(34, atFdcwd, path, mode, 0, 0, 0)
}
func SysRmdir(path uintptr) (uintptr, uintptr, int32) { return Syscall(35, atFdcwd, path, 0x200, 0, 0, 0) }
func SysUnlink(path uintptr) (uintptr, uintptr, int32) { return Syscall(35, atFdcwd, path, 0, 0, 0, 0) }
func SysChmod(path, mode uintptr) (uintptr, uintptr, int32) {
	return Syscall(53, atFdcwd, path, mode, 0, 0, 0)
}
func SysGetdents64(fd, buf, size uintptr) (uintptr, uintptr, int32) {
	return Syscall(61, fd, buf, size, 0, 0, 0)
}
func SysExit(code uintptr) { Syscall(94, code, 0, 0, 0, 0, 0) }
func SysMmap(addr, length, prot, flags, fd, offset uintptr) (uintptr, uintptr, int32) {
	return Syscall(222, addr, length, prot, flags, fd, offset)
}
func SysPipe(fds uintptr) (uintptr, uintptr, int32) { return Syscall(59, fds, 0, 0, 0, 0, 0) }
func SysGetpid() (uintptr, uintptr, int32)          { return Syscall(172, 0, 0, 0, 0, 0, 0) }
