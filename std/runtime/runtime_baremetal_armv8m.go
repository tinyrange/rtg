//go:build baremetal && armv8m

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
	MmapAnonFlags  = 0
)

var GOOS string = "baremetal"
var GOARCH string = "armv8m"

const (
	semiScratch0 uintptr = 0x80FFF000
	semiScratch1 uintptr = 0x80FFF080
)

var semiHeapCur uintptr = 0x80000000
var semiHeapEnd uintptr = 0x80FBF000
var semiHeapSramCur uintptr = 0
var semiHeapSramEnd uintptr = 0
var semiHeapAltCur uintptr = 0x28000000
var semiHeapAltEnd uintptr = 0x283BE000
var semiHeapLowCur uintptr = 0x00010000
var semiHeapLowEnd uintptr = 0x007FF000

func init() {
	// Keep mmap growth granular to avoid stranding free space near arena limits.
	heapChunk = 16384
	heapChunkMax = 65536
}

const (
	semiOpen         int32 = 0x01
	semiClose        int32 = 0x02
	semiWritec       int32 = 0x03
	semiWrite        int32 = 0x05
	semiRead         int32 = 0x06
	semiSeek         int32 = 0x0A
	semiFlen         int32 = 0x0C
	semiRemove       int32 = 0x0E
	semiRename       int32 = 0x0F
	semiSystem       int32 = 0x12
	semiErrno        int32 = 0x13
	semiGetCmdline   int32 = 0x15
	semiExitExtended int32 = 0x20
)

const (
	adpStoppedApplicationExit = 0x20026
)

//rtg:internal Semihost
func Semihost(op int32, arg uintptr) (r1 uintptr, r2 uintptr, err int32)

func semihostErrno() int32 {
	errv, _, err := Semihost(semiErrno, 0)
	if err != 0 {
		return err
	}
	return int32(errv)
}

func semihostResult(rv uintptr, apiErr int32, errWhen uintptr) (uintptr, uintptr, int32) {
	if apiErr != 0 {
		return 0, 0, apiErr
	}
	if rv == errWhen {
		e := semihostErrno()
		if e == 0 {
			e = 1
		}
		return 0, 0, e
	}
	return rv, 0, 0
}

func semihostOpenMode(flags uintptr) int32 {
	// Semihosting mode table (Arm semihosting):
	// 0 r, 1 rb, 2 r+, 3 r+b, 4 w, 5 wb, 6 w+, 7 w+b, 8 a, 9 ab, 10 a+, 11 a+b
	if flags == 0 {
		return 0
	}
	if flags&uintptr(2) != 0 { // O_RDWR
		if flags&uintptr(64) != 0 || flags&uintptr(512) != 0 {
			return 6 // w+
		}
		return 2 // r+
	}
	if flags&uintptr(1) != 0 { // O_WRONLY
		if flags&uintptr(64) != 0 || flags&uintptr(512) != 0 {
			return 4 // w
		}
		return 8 // a
	}
	return 0 // r
}

func SysRead(fd, buf, count uintptr) (uintptr, uintptr, int32) {
	params := semiScratch0
	WritePtr(params+0, fd)
	WritePtr(params+uintptr(PtrSize), buf)
	WritePtr(params+uintptr(2*PtrSize), count)
	notRead, _, err := Semihost(semiRead, params)
	if err != 0 {
		return 0, 0, err
	}
	if notRead > count {
		return 0, 0, 5
	}
	return count - notRead, 0, 0
}

func SysWrite(fd, buf, count uintptr) (uintptr, uintptr, int32) {
	// QEMU semihosting does not guarantee Unix-style fixed handles 1/2.
	// For stdout/stderr, use SYS_WRITEC directly so diagnostics are visible.
	if fd == 1 || fd == 2 {
		i := uintptr(0)
		for i < count {
			WriteByte(semiScratch1, byte(ReadPtr(buf+i)))
			_, _, err := Semihost(semiWritec, semiScratch1)
			if err != 0 {
				return i, 0, err
			}
			i = i + 1
		}
		return count, 0, 0
	}
	params := semiScratch0
	WritePtr(params+0, fd)
	WritePtr(params+uintptr(PtrSize), buf)
	WritePtr(params+uintptr(2*PtrSize), count)
	notWritten, _, err := Semihost(semiWrite, params)
	if err != 0 {
		return 0, 0, err
	}
	if notWritten > count {
		return 0, 0, 5
	}
	return count - notWritten, 0, 0
}

func SysOpen(path, flags, mode uintptr) (uintptr, uintptr, int32) {
	_ = mode
	plen := 0
	for {
		b := byte(ReadPtr(path + uintptr(plen)))
		if b == 0 {
			break
		}
		plen = plen + 1
	}
	params := semiScratch0
	WritePtr(params+0, path)
	WritePtr(params+uintptr(PtrSize), uintptr(semihostOpenMode(flags)))
	WritePtr(params+uintptr(2*PtrSize), uintptr(plen))
	rv, _, err := Semihost(semiOpen, params)
	return semihostResult(rv, err, ^uintptr(0))
}

func SysClose(fd uintptr) (uintptr, uintptr, int32) {
	rv, _, err := Semihost(semiClose, fd)
	return semihostResult(rv, err, ^uintptr(0))
}

func SysStat(path, buf uintptr) (uintptr, uintptr, int32) {
	_ = path
	_ = buf
	return 0, 0, 38
}

func SysDup2(old, new_ uintptr) (uintptr, uintptr, int32) {
	_ = old
	_ = new_
	return 0, 0, 38
}

func SysFork() (uintptr, uintptr, int32) {
	return 0, 0, 38
}

func SysExecve(path, argv, envp uintptr) (uintptr, uintptr, int32) {
	_ = path
	_ = argv
	_ = envp
	return 0, 0, 38
}

func SysWait4(pid, status, opts, rusage uintptr) (uintptr, uintptr, int32) {
	_ = pid
	_ = status
	_ = opts
	_ = rusage
	return 0, 0, 38
}

func SysGetcwd(buf, size uintptr) (uintptr, uintptr, int32) {
	_ = buf
	_ = size
	return 0, 0, 38
}

func SysMkdir(path, mode uintptr) (uintptr, uintptr, int32) {
	_ = path
	_ = mode
	return 0, 0, 38
}

func SysRmdir(path uintptr) (uintptr, uintptr, int32) {
	_ = path
	return 0, 0, 38
}

func SysUnlink(path uintptr) (uintptr, uintptr, int32) {
	rv, _, err := Semihost(semiRemove, path)
	return semihostResult(rv, err, ^uintptr(0))
}

func SysChmod(path, mode uintptr) (uintptr, uintptr, int32) {
	_ = path
	_ = mode
	return 0, 0, 38
}

func SysGetdents64(fd, buf, size uintptr) (uintptr, uintptr, int32) {
	_ = fd
	_ = buf
	_ = size
	return 0, 0, 38
}

func SysExit(code uintptr) {
	// Param block for SYS_EXIT_EXTENDED: [reason, subcode].
	params := semiScratch0
	WritePtr(params+0, adpStoppedApplicationExit)
	WritePtr(params+uintptr(PtrSize), code)
	Semihost(semiExitExtended, params)
	// Fallback: try legacy exit reason.
	Semihost(0x18, adpStoppedApplicationExit)
}

func SysMmap(addr, length, prot, flags, fd, offset uintptr) (uintptr, uintptr, int32) {
	_ = addr
	_ = prot
	_ = flags
	_ = fd
	_ = offset
	if length == 0 {
		length = 1
	}
	// 8-byte alignment for runtime allocator expectations.
	length = (length + 7) &^ uintptr(7)
	if semiHeapCur != 0 {
		p := semiHeapCur
		next := p + length
		if next >= p && next <= semiHeapEnd {
			semiHeapCur = next
			return p, 0, 0
		}
		semiHeapCur = 0
	}
	if semiHeapSramCur != 0 {
		p := semiHeapSramCur
		next := p + length
		if next >= p && next <= semiHeapSramEnd {
			semiHeapSramCur = next
			return p, 0, 0
		}
		semiHeapSramCur = 0
	}
	if semiHeapAltCur != 0 {
		p := semiHeapAltCur
		next := p + length
		if next >= p && next <= semiHeapAltEnd {
			semiHeapAltCur = next
			return p, 0, 0
		}
		semiHeapAltCur = 0
	}
	if semiHeapLowCur != 0 {
		p := semiHeapLowCur
		next := p + length
		if next >= p && next <= semiHeapLowEnd {
			semiHeapLowCur = next
			return p, 0, 0
		}
		semiHeapLowCur = 0
	}
	return 0, 0, 12
}

func SysPipe(fds uintptr) (uintptr, uintptr, int32) {
	_ = fds
	return 0, 0, 38
}

func SysGetpid() (uintptr, uintptr, int32) {
	return 1, 0, 0
}

func SysSystem(cmd uintptr) (uintptr, uintptr, int32) {
	rv, _, err := Semihost(semiSystem, cmd)
	if err != 0 {
		return 0, 0, err
	}
	return rv, 0, 0
}

func SysGetCommandLine() (uintptr, uintptr, int32) {
	// Param block: [buf_ptr, buf_size]
	buf := semiScratch1
	params := semiScratch0
	WritePtr(params+0, buf)
	WritePtr(params+uintptr(PtrSize), 1024)
	rv, _, err := Semihost(semiGetCmdline, params)
	if err != 0 || rv != 0 {
		return 0, 0, 38
	}
	return buf, 0, 0
}
