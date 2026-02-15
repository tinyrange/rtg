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

//rtg:internal SysExit
func SysExit(code uintptr)

//rtg:internal SysMmap
func SysMmap(addr, length, prot, flags, fd, offset uintptr) (uintptr, uintptr, int32)

//rtg:internal SysMkdir
func SysMkdir(path, mode uintptr) (uintptr, uintptr, int32)

//rtg:internal SysRmdir
func SysRmdir(path uintptr) (uintptr, uintptr, int32)

//rtg:internal SysUnlink
func SysUnlink(path uintptr) (uintptr, uintptr, int32)

//rtg:internal SysGetcwd
func SysGetcwd(buf, size uintptr) (uintptr, uintptr, int32)

//rtg:internal SysGetCommandLine
func SysGetCommandLine() (uintptr, uintptr, int32)

//rtg:internal SysGetEnvStrings
func SysGetEnvStrings() (uintptr, uintptr, int32)

//rtg:internal SysFindFirstFile
func SysFindFirstFile(pattern, findData uintptr) (uintptr, uintptr, int32)

//rtg:internal SysFindNextFile
func SysFindNextFile(handle, findData uintptr) (uintptr, uintptr, int32)

//rtg:internal SysFindClose
func SysFindClose(handle uintptr) (uintptr, uintptr, int32)

//rtg:internal SysCreateProcess
func SysCreateProcess(appName, cmdLine, startupInfo, processInfo, envp uintptr) (uintptr, uintptr, int32)

//rtg:internal SysWaitProcess
func SysWaitProcess(handle, exitCodeBuf uintptr) (uintptr, uintptr, int32)

//rtg:internal SysCreatePipe
func SysCreatePipe(readBuf, writeBuf uintptr) (uintptr, uintptr, int32)

//rtg:internal SysSetStdHandle
func SysSetStdHandle(stdHandle, handle uintptr) (uintptr, uintptr, int32)

//rtg:internal SysGetpid
func SysGetpid() (uintptr, uintptr, int32)
