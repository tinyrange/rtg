package runtime

const (
	profileRecordSize    = 8
	profileBufferRecords = 16384
	profileBufferSize    = profileRecordSize * profileBufferRecords
	profileFilePerm      = 0644
)

var profileInitDone bool
var profileTouched bool
var profileEnabled bool
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
// Records are packed as: hash(uint32), duration_ns(uint32), little-endian.
func Profile(methodName string, executionTime int) {
	if methodName == "" {
		return
	}
	ProfileHash(profileHash32(methodName), executionTime)
}

// ProfileHash records a method execution duration by pre-hashed method id.
func ProfileHash(methodHash uint32, executionTime int) {
	profileRecord(methodHash, executionTime)
}

// ProfileHashNow records a duration using runtime.Now()-startTime.
func ProfileHashNow(methodHash uint32, startTime int) {
	profileRecord(methodHash, profileNow()-startTime)
}

func profileRecord(methodHash uint32, executionTime int) {
	profileTouched = true
	profileEnsureInit()
	if !profileEnabled {
		return
	}
	if profileBufUsed+profileRecordSize > profileBufferSize {
		profileFlushBuffer()
		if !profileEnabled {
			return
		}
	}
	hash := methodHash
	duration := profileDurationToU32(executionTime)
	off := profileBufUsed
	profileBuf[off+0] = byte(hash)
	profileBuf[off+1] = byte(hash >> 8)
	profileBuf[off+2] = byte(hash >> 16)
	profileBuf[off+3] = byte(hash >> 24)
	profileBuf[off+4] = byte(duration)
	profileBuf[off+5] = byte(duration >> 8)
	profileBuf[off+6] = byte(duration >> 16)
	profileBuf[off+7] = byte(duration >> 24)
	profileBufUsed = off + profileRecordSize
	if profileBufUsed == profileBufferSize {
		profileFlushBuffer()
	}
}

// ProfileFlush flushes pending profile records and closes the profile output.
func ProfileFlush() {
	if !profileTouched && !profileInitDone {
		return
	}
	profileEnsureInit()
	if !profileEnabled {
		return
	}
	profileFlushBuffer()
	profileDisable()
}

func profileEnsureInit() {
	if profileInitDone {
		return
	}
	profileInitDone = true
	if len(profileBuf) == 0 {
		profileBuf = make([]byte, profileBufferSize)
	}

	path := profileLookupEnv("RTG_PROFILE")
	if path == "" {
		return
	}
	cpath := profileMakeCString(path)
	fd, _, errn := SysOpen(Sliceptr(cpath), uintptr(profileOpenFlags), uintptr(profileFilePerm))
	if errn != 0 {
		return
	}
	// Some targets may create the file with restrictive default mode despite O_CREAT mode.
	// Normalize permissions so external tools can read profile output.
	profileNormalizePermissions(cpath)
	profileFD = fd
	profileFDOpen = true
	profileEnabled = true
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

func profileHash32(name string) uint32 {
	var h uint32 = 2166136261
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
