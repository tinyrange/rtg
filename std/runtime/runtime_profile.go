package runtime

const (
	profileHeaderSize      = 8
	profileRecordSize      = 16
	profileBufferRecords   = 16384
	profileBufferSize      = profileRecordSize * profileBufferRecords
	profileFilePerm        = 0644
	profileRecordKindTime  = 1
	profileRecordKindAlloc = 2
)

var profileInitDone bool
var profileIniting bool
var profileTouched bool
var profileEnabled bool
var profileBypass bool
var profileFD uintptr
var profileFDOpen bool
var profileBuf []byte
var profileBufUsed int

// Exit flushes profiling data (when enabled) and terminates the process.
func Exit(code uintptr) {
	ProfileFlush()
	SysExit(code)
}

// Profile records a method execution duration in nanoseconds.
// Records are packed as:
// method_hash(uint32), parent_hash(uint32), value(uint32), kind(uint32), little-endian.
// kind=1 stores duration nanoseconds; kind=2 stores allocation bytes.
func Profile(methodName string, executionTime int) {
	if methodName == "" {
		return
	}
	ProfileHash(profileHash32(methodName), 0, executionTime)
}

// ProfileHash records a method execution duration by pre-hashed method id.
func ProfileHash(methodHash uint32, parentHash uint32, executionTime int) {
	profileRecord(profileRecordKindTime, methodHash, parentHash, profileDurationToU32(executionTime))
}

// ProfileHashNow records a duration using runtime.Now()-startTime.
func ProfileHashNow(methodHash uint32, parentHash uint32, startTime int) {
	profileRecord(profileRecordKindTime, methodHash, parentHash, profileDurationToU32(profileNow()-startTime))
}

// ProfileAllocHash records an allocation sample in bytes for a call-tree edge.
func ProfileAllocHash(size int, methodHash uint32, parentHash uint32) {
	profileRecord(profileRecordKindAlloc, methodHash, parentHash, profileSizeToU32(size))
}

func profileRecord(kind uint32, methodHash uint32, parentHash uint32, value uint32) {
	if profileBypass {
		return
	}
	profileBypass = true
	profileTouched = true
	profileEnsureInit()
	if !profileEnabled {
		profileBypass = false
		return
	}
	if profileBufUsed+profileRecordSize > profileBufferSize {
		profileFlushBuffer()
		if !profileEnabled {
			profileBypass = false
			return
		}
	}
	off := profileBufUsed
	profileBuf[off+0] = byte(methodHash)
	profileBuf[off+1] = byte(methodHash >> 8)
	profileBuf[off+2] = byte(methodHash >> 16)
	profileBuf[off+3] = byte(methodHash >> 24)
	profileBuf[off+4] = byte(parentHash)
	profileBuf[off+5] = byte(parentHash >> 8)
	profileBuf[off+6] = byte(parentHash >> 16)
	profileBuf[off+7] = byte(parentHash >> 24)
	profileBuf[off+8] = byte(value)
	profileBuf[off+9] = byte(value >> 8)
	profileBuf[off+10] = byte(value >> 16)
	profileBuf[off+11] = byte(value >> 24)
	profileBuf[off+12] = byte(kind)
	profileBuf[off+13] = byte(kind >> 8)
	profileBuf[off+14] = byte(kind >> 16)
	profileBuf[off+15] = byte(kind >> 24)
	profileBufUsed = off + profileRecordSize
	if profileBufUsed == profileBufferSize {
		profileFlushBuffer()
	}
	profileBypass = false
}

// ProfileFlush flushes pending profile records and closes the profile output.
func ProfileFlush() {
	if !profileTouched && !profileInitDone {
		return
	}
	if profileBypass {
		return
	}
	profileBypass = true
	profileEnsureInit()
	if !profileEnabled {
		profileBypass = false
		return
	}
	profileFlushBuffer()
	allocDebugMaybePrintSummary()
	profileDisable()
	profileBypass = false
}

func profileEnsureInit() {
	if profileInitDone {
		return
	}
	if profileIniting {
		return
	}
	profileIniting = true
	profileBypass = true
	profileInitDone = true
	if len(profileBuf) == 0 {
		profileBuf = make([]byte, profileBufferSize)
	}

	path := profileLookupEnv("RTG_PROFILE")
	if path == "" {
		profileBypass = false
		profileIniting = false
		return
	}
	cpath := profileMakeCString(path)
	fd, _, errn := SysOpen(Sliceptr(cpath), uintptr(profileOpenFlags), uintptr(profileFilePerm))
	if errn != 0 {
		profileBypass = false
		profileIniting = false
		return
	}
	// Some targets may create the file with restrictive default mode despite O_CREAT mode.
	// Normalize permissions so external tools can read profile output.
	profileNormalizePermissions(cpath)
	var header [profileHeaderSize]byte
	header[0] = 'R'
	header[1] = 'T'
	header[2] = 'P'
	header[3] = '2'
	if !profileWriteAll(fd, header[:]) {
		SysClose(fd)
		profileBypass = false
		profileIniting = false
		return
	}
	profileFD = fd
	profileFDOpen = true
	profileEnabled = true
	profileBypass = false
	profileIniting = false
}

func profileDisable() {
	if profileFDOpen {
		SysClose(profileFD)
	}
	profileFD = 0
	profileFDOpen = false
	profileEnabled = false
	profileBufUsed = 0
}

func profileFlushBuffer() {
	if !profileEnabled || !profileFDOpen || profileBufUsed == 0 {
		return
	}
	remaining := profileBufUsed
	offset := 0
	for remaining > 0 {
		chunk := profileBuf[offset : offset+remaining]
		wrote, _, errn := SysWrite(profileFD, Sliceptr(chunk), uintptr(remaining))
		if errn != 0 || wrote == 0 {
			profileDisable()
			return
		}
		n := int(wrote)
		offset += n
		remaining -= n
	}
	profileBufUsed = 0
}

func profileWriteAll(fd uintptr, data []byte) bool {
	remaining := len(data)
	offset := 0
	for remaining > 0 {
		chunk := data[offset : offset+remaining]
		wrote, _, errn := SysWrite(fd, Sliceptr(chunk), uintptr(remaining))
		if errn != 0 || wrote == 0 {
			return false
		}
		n := int(wrote)
		offset += n
		remaining -= n
	}
	return true
}

func profileHash32(name string) uint32 {
	var h uint32 = (uint32(0x811c) << 16) | uint32(0x9dc5)
	i := 0
	for i < len(name) {
		h = h ^ uint32(name[i])
		h = h * 16777619
		i++
	}
	return h
}

func profileDurationToU32(ns int) uint32 {
	if ns <= 0 {
		return 0
	}
	if PtrSize > 4 {
		if (ns >> 32) > 0 {
			return ^uint32(0)
		}
	}
	return uint32(ns)
}

func profileSizeToU32(size int) uint32 {
	if size <= 0 {
		return 0
	}
	if PtrSize > 4 {
		if (size >> 32) > 0 {
			return ^uint32(0)
		}
	}
	return uint32(size)
}

func profileMakeCString(s string) []byte {
	buf := make([]byte, len(s)+1)
	i := 0
	for i < len(s) {
		buf[i] = s[i]
		i++
	}
	buf[len(s)] = 0
	return buf
}

func profileSplitEnvEntry(entry string, key string) (string, bool) {
	if entry == "" || key == "" {
		return "", false
	}
	i := 0
	for i < len(entry) && entry[i] != '=' {
		i++
	}
	if i == len(entry) {
		return "", false
	}
	if i != len(key) {
		return "", false
	}
	j := 0
	for j < len(key) {
		if entry[j] != key[j] {
			return "", false
		}
		j++
	}
	return entry[i+1 : len(entry)], true
}

func profileLookupEnvFromBlock(data []byte, key string) string {
	start := 0
	i := 0
	for i <= len(data) {
		if i == len(data) || data[i] == 0 {
			if i > start {
				entry := string(data[start:i])
				if value, ok := profileSplitEnvEntry(entry, key); ok {
					return value
				}
			}
			start = i + 1
		}
		i++
	}
	return ""
}
