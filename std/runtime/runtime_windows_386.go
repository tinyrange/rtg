//go:build windows && 386

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
	MmapAnonFlags  = 0 // not used on Windows
)

var GOOS string = "windows"
var GOARCH string = "386"

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
