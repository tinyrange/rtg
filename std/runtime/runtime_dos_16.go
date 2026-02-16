//go:build dos && dos16

package runtime

const (
	PtrSize        = 2
	SliceHdrSize   = 8
	StringHdrSize  = 4
	IfaceBoxSize   = 4
	SliceOffLen    = 2
	SliceOffCap    = 4
	SliceOffEsz    = 6
	MapEntrySize   = 4
	MapEntryOffVal = 2
	MmapAnonFlags  = 34
)

var GOOS string = "dos"
var GOARCH string = "dos16"

func init() {
	// Keep allocator chunk size representable in 16-bit uintptr arithmetic.
	heapChunk = 4096
}

//rtg:internal Syscall
func Syscall(num, a0, a1, a2, a3, a4, a5 uintptr) (r1 uintptr, r2 uintptr, err uintptr)

func SysRead(fd, buf, count uintptr) (uintptr, uintptr, uintptr) {
	return Syscall(3, fd, buf, count, 0, 0, 0)
}
func SysWrite(fd, buf, count uintptr) (uintptr, uintptr, uintptr) {
	return Syscall(4, fd, buf, count, 0, 0, 0)
}
func SysOpen(path, flags, mode uintptr) (uintptr, uintptr, uintptr) {
	return Syscall(5, path, flags, mode, 0, 0, 0)
}
func SysClose(fd uintptr) (uintptr, uintptr, uintptr) { return Syscall(6, fd, 0, 0, 0, 0, 0) }
func SysStat(path, buf uintptr) (uintptr, uintptr, uintptr) {
	return Syscall(106, path, buf, 0, 0, 0, 0)
}
func SysDup2(old, new_ uintptr) (uintptr, uintptr, uintptr) {
	return Syscall(63, old, new_, 0, 0, 0, 0)
}
func SysFork() (uintptr, uintptr, uintptr) { return Syscall(2, 0, 0, 0, 0, 0, 0) }
func SysExecve(path, argv, envp uintptr) (uintptr, uintptr, uintptr) {
	return Syscall(11, path, argv, envp, 0, 0, 0)
}
func SysWait4(pid, status, opts, rusage uintptr) (uintptr, uintptr, uintptr) {
	return Syscall(114, pid, status, opts, rusage, 0, 0)
}
func SysGetcwd(buf, size uintptr) (uintptr, uintptr, uintptr) {
	return Syscall(183, buf, size, 0, 0, 0, 0)
}
func SysMkdir(path, mode uintptr) (uintptr, uintptr, uintptr) {
	return Syscall(39, path, mode, 0, 0, 0, 0)
}
func SysRmdir(path uintptr) (uintptr, uintptr, uintptr)  { return Syscall(40, path, 0, 0, 0, 0, 0) }
func SysUnlink(path uintptr) (uintptr, uintptr, uintptr) { return Syscall(10, path, 0, 0, 0, 0, 0) }
func SysChmod(path, mode uintptr) (uintptr, uintptr, uintptr) {
	return Syscall(15, path, mode, 0, 0, 0, 0)
}
func SysGetdents64(fd, buf, size uintptr) (uintptr, uintptr, uintptr) {
	return Syscall(220, fd, buf, size, 0, 0, 0)
}
func SysExit(code uintptr) { Syscall(252, code, 0, 0, 0, 0, 0) }
func SysMmap(addr, length, prot, flags, fd, offset uintptr) (uintptr, uintptr, uintptr) {
	return Syscall(192, addr, length, prot, flags, fd, offset)
}
func SysPipe(fds uintptr) (uintptr, uintptr, uintptr) { return Syscall(331, fds, 0, 0, 0, 0, 0) }
func SysGetpid() (uintptr, uintptr, uintptr)          { return Syscall(20, 0, 0, 0, 0, 0, 0) }
