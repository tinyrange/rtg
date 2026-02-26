//go:build bare && armv8m

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

var GOOS string = "bare"
var GOARCH string = "armv8m"

const (
	// Renode/UART mapping for bare armv8m runs.
	bareUART0Base = uintptr(0x40004000)
	bareUARTTHR   = uintptr(0x00) // Transmit Holding Register
	bareUARTLSR   = uintptr(0x05) // Line Status Register
	bareUARTTHRE  = uintptr(0x20) // LSR bit 5: THR empty
)

var bareHeapCur uintptr = 0
var bareHeapEnd uintptr = 0
var bareHeapAltCur uintptr = 0
var bareHeapAltEnd uintptr = 0
var bareHeapHiCur uintptr = 0
var bareHeapHiEnd uintptr = 0

func init() {
	// Keep mmap growth granular to avoid stranding free space near arena limits.
	heapChunk = 16384
	heapChunkMax = 65536
	// RP2350-style constrained heap budget (~520KB total RAM across stack+heap+data).
	// Keep heap in the 0x2800_0000 bank; stack/data live in 0x2000_0000 bank.
	bareHeapCur = 0x28001000
	bareHeapEnd = 0x2806F000
	bareHeapAltCur = 0
	bareHeapAltEnd = 0
	bareHeapHiCur = 0
	bareHeapHiEnd = 0
}

func SysRead(fd, buf, count uintptr) (uintptr, uintptr, int32) {
	_ = fd
	_ = buf
	_ = count
	return 0, 0, 0
}

func SysWrite(fd, buf, count uintptr) (uintptr, uintptr, int32) {
	if fd == 1 || fd == 2 {
		if count == 0 {
			return 0, 0, 0
		}
		b := Makeslice(buf, int(count), int(count))
		i := 0
		for i < len(b) {
			for (ReadPtr(bareUART0Base+bareUARTLSR) & bareUARTTHRE) == 0 {
			}
			WriteByte(bareUART0Base+bareUARTTHR, uintptr(b[i]))
			i++
		}
		return uintptr(len(b)), 0, 0
	}
	return 0, 0, 9
}

func SysOpen(path, flags, mode uintptr) (uintptr, uintptr, int32) {
	_ = path
	_ = flags
	_ = mode
	return 0, 0, 38
}

func SysClose(fd uintptr) (uintptr, uintptr, int32) {
	_ = fd
	return 0, 0, 0
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
	_ = path
	return 0, 0, 38
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
	_ = code
	for {
	}
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
	if bareHeapEnd == 0 {
		bareHeapCur = 0x28001000
		bareHeapEnd = 0x2806F000
	}
	if bareHeapAltEnd == 0 {
		bareHeapAltCur = 0
		bareHeapAltEnd = 0
	}
	if bareHeapHiEnd == 0 {
		bareHeapHiCur = 0
		bareHeapHiEnd = 0
	}

	if bareHeapCur != 0 {
		p := bareHeapCur
		next := p + length
		if next >= p && next <= bareHeapEnd {
			bareHeapCur = next
			return p, 0, 0
		}
		bareHeapCur = 0
	}
	if bareHeapAltCur != 0 {
		p := bareHeapAltCur
		next := p + length
		if next >= p && next <= bareHeapAltEnd {
			bareHeapAltCur = next
			return p, 0, 0
		}
		bareHeapAltCur = 0
	}
	if bareHeapHiCur != 0 {
		p := bareHeapHiCur
		next := p + length
		if next >= p && next <= bareHeapHiEnd {
			bareHeapHiCur = next
			return p, 0, 0
		}
		bareHeapHiCur = 0
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
	_ = cmd
	return 0, 0, 38
}

func SysGetCommandLine() (uintptr, uintptr, int32) {
	return 0, 0, 38
}
