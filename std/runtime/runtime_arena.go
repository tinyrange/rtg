package runtime

const arenaMaxNodes = 262144
const arenaMaxStack = 262144

var arenaMethodHash []uint32
var arenaParent []uint32
var arenaFirstChild []uint32
var arenaNextSibling []uint32
var arenaEnters []uint64
var arenaAllocs []uint64
var arenaReqBytes []uint64
var arenaMmapBytes []uint64
var arenaName []string
var arenaStack []uint32

var arenaInitDone bool
var arenaIniting bool
var arenaNodeCount int
var arenaCurrent int
var arenaStackLen int
var arenaDropped bool
var arenaEnabled bool

//rtg:noprofile
func arenaEnsureInit() bool {
	if arenaInitDone {
		return true
	}
	if arenaIniting {
		return false
	}
	arenaIniting = true

	arenaMethodHash = make([]uint32, arenaMaxNodes)
	arenaParent = make([]uint32, arenaMaxNodes)
	arenaFirstChild = make([]uint32, arenaMaxNodes)
	arenaNextSibling = make([]uint32, arenaMaxNodes)
	arenaEnters = make([]uint64, arenaMaxNodes)
	arenaAllocs = make([]uint64, arenaMaxNodes)
	arenaReqBytes = make([]uint64, arenaMaxNodes)
	arenaMmapBytes = make([]uint64, arenaMaxNodes)
	arenaName = make([]string, arenaMaxNodes)
	arenaStack = make([]uint32, arenaMaxStack)

	arenaNodeCount = 1
	arenaCurrent = 1
	arenaStackLen = 0
	arenaDropped = false
	arenaName[1] = "<root>"
	arenaInitDone = true
	arenaIniting = false
	return true
}

// ArenaEnter switches allocation accounting to a child arena keyed by method hash.
//
//rtg:noprofile
func ArenaEnter(methodHash uint32, parentHash uint32, methodName string) {
	if methodHash == 0 {
		return
	}
	arenaEnabled = true
	if !arenaEnsureInit() {
		return
	}
	if arenaStackLen >= len(arenaStack) {
		arenaDropped = true
		return
	}
	arenaStack[arenaStackLen] = uint32(arenaCurrent)
	arenaStackLen++
	parent := arenaCurrent
	if parent <= 0 || parent > arenaNodeCount {
		parent = 1
	}
	if parentHash != 0 && arenaMethodHash[parent] != parentHash {
		probe := parent
		for probe > 0 && probe < arenaMaxNodes && arenaMethodHash[probe] != parentHash {
			probe = int(arenaParent[probe])
		}
		if probe > 0 {
			parent = probe
		} else {
			parent = 1
		}
	}
	if arenaMethodHash[parent] == methodHash {
		arenaCurrent = parent
		if arenaName[parent] == "" && methodName != "" {
			arenaName[parent] = methodName
		}
		arenaEnters[parent]++
		return
	}
	child := arenaFindChild(parent, methodHash, methodName)
	if child == 0 {
		child = arenaCreateChild(parent, methodHash, methodName)
	}
	if child == 0 {
		arenaDropped = true
		return
	}
	arenaEnabled = true
	arenaCurrent = child
	arenaEnters[child]++
}

// ArenaLeave returns allocation accounting to the parent arena.
//
//rtg:noprofile
func ArenaLeave() {
	if !arenaEnsureInit() {
		return
	}
	if arenaStackLen <= 0 {
		arenaCurrent = 1
		return
	}
	arenaStackLen--
	p := int(arenaStack[arenaStackLen])
	if p <= 0 || p > arenaNodeCount {
		arenaCurrent = 1
		return
	}
	arenaCurrent = p
}

func arenaFindChild(parent int, methodHash uint32, methodName string) int {
	if parent <= 0 || parent > arenaNodeCount {
		return 0
	}
	child := int(arenaFirstChild[parent])
	steps := 0
	for child != 0 {
		if child < 0 || child > arenaNodeCount {
			return 0
		}
		if arenaMethodHash[child] == methodHash {
			if arenaName[child] == "" && methodName != "" {
				arenaName[child] = methodName
			}
			return child
		}
		child = int(arenaNextSibling[child])
		steps++
		if steps > arenaNodeCount {
			return 0
		}
	}
	return 0
}

func arenaCreateChild(parent int, methodHash uint32, methodName string) int {
	if parent <= 0 || parent > arenaNodeCount {
		return 0
	}
	if arenaNodeCount+1 >= arenaMaxNodes {
		return 0
	}
	idx := arenaNodeCount + 1
	arenaNodeCount = idx
	arenaMethodHash[idx] = methodHash
	arenaName[idx] = methodName
	arenaParent[idx] = uint32(parent)
	arenaNextSibling[idx] = arenaFirstChild[parent]
	arenaFirstChild[parent] = uint32(idx)
	return idx
}

//rtg:noprofile
func arenaRecordAlloc(reqSize int, mmapChunk int) {
	if !arenaEnabled {
		return
	}
	if !arenaEnsureInit() {
		return
	}
	if reqSize < 0 {
		reqSize = 0
	}
	if mmapChunk < 0 {
		mmapChunk = 0
	}

	arenaAllocs[1]++
	arenaReqBytes[1] += uint64(reqSize)
	arenaMmapBytes[1] += uint64(mmapChunk)

	idx := arenaCurrent
	if idx <= 1 || idx > arenaNodeCount {
		return
	}
	arenaAllocs[idx]++
	arenaReqBytes[idx] += uint64(reqSize)
	arenaMmapBytes[idx] += uint64(mmapChunk)
}

// ArenaFlush writes arena accounting to RTG_ARENA_REPORT, when set.
//
//rtg:noprofile
func ArenaFlush() {
	if !arenaEnsureInit() {
		return
	}
	path := profileLookupEnv("RTG_ARENA_REPORT")
	if path == "" {
		return
	}
	cpath := profileMakeCString(path)
	fd, _, errn := SysOpen(Sliceptr(cpath), uintptr(profileOpenFlags), uintptr(profileFilePerm))
	if errn != 0 {
		return
	}
	profileNormalizePermissions(cpath)
	if !arenaWriteString(fd, "arena_report_v2\n") {
		SysClose(fd)
		return
	}
	if arenaDropped {
		if !arenaWriteString(fd, "note=dropped_nodes\n") {
			SysClose(fd)
			return
		}
	}
	if !arenaWriteString(fd, "id parent depth hash enters allocs req_bytes mmap_bytes name\n") {
		SysClose(fd)
		return
	}
	i := 1
	for i <= arenaNodeCount {
		if !arenaWriteU64(fd, uint64(i)) ||
			!arenaWriteString(fd, " ") ||
			!arenaWriteU64(fd, uint64(arenaParent[i])) ||
			!arenaWriteString(fd, " ") ||
			!arenaWriteU64(fd, uint64(arenaDepth(i))) ||
			!arenaWriteString(fd, " 0x") ||
			!arenaWriteHex32(fd, arenaMethodHash[i]) ||
			!arenaWriteString(fd, " ") ||
			!arenaWriteU64(fd, arenaEnters[i]) ||
			!arenaWriteString(fd, " ") ||
			!arenaWriteU64(fd, arenaAllocs[i]) ||
			!arenaWriteString(fd, " ") ||
			!arenaWriteU64(fd, arenaReqBytes[i]) ||
			!arenaWriteString(fd, " ") ||
			!arenaWriteU64(fd, arenaMmapBytes[i]) ||
			!arenaWriteString(fd, " ") ||
			!arenaWriteString(fd, arenaNodeName(i)) ||
			!arenaWriteString(fd, "\n") {
			SysClose(fd)
			return
		}
		i++
	}
	SysClose(fd)
}

func arenaDepth(id int) int {
	if id <= 0 || id > arenaNodeCount {
		return 0
	}
	depth := 0
	p := int(arenaParent[id])
	steps := 0
	for p > 0 {
		if p > arenaNodeCount {
			break
		}
		depth++
		p = int(arenaParent[p])
		steps++
		if steps > arenaNodeCount {
			break
		}
	}
	return depth
}

func arenaNodeName(id int) string {
	if id <= 0 || id > arenaNodeCount {
		return "<invalid>"
	}
	name := arenaName[id]
	if name != "" {
		return name
	}
	if arenaMethodHash[id] == 0 {
		return "<root>"
	}
	return "<unknown>"
}

func arenaWriteString(fd uintptr, s string) bool {
	if len(s) == 0 {
		return true
	}
	ptr := Stringptr(s)
	remaining := uintptr(len(s))
	for remaining > 0 {
		wrote, _, errn := SysWrite(fd, ptr, remaining)
		if errn != 0 || wrote == 0 {
			return false
		}
		ptr = ptr + wrote
		remaining = remaining - wrote
	}
	return true
}

func arenaWriteU64(fd uintptr, v uint64) bool {
	var buf [20]byte
	i := len(buf)
	if v == 0 {
		i--
		buf[i] = '0'
	} else {
		for v > 0 {
			i--
			buf[i] = byte('0' + (v % 10))
			v = v / 10
		}
	}
	return profileWriteAll(fd, buf[i:len(buf)])
}

func arenaWriteHex32(fd uintptr, v uint32) bool {
	const hexdigits = "0123456789abcdef"
	var buf [8]byte
	i := 0
	for i < 8 {
		shift := uint32(28 - i*4)
		buf[i] = hexdigits[(v>>shift)&0xF]
		i++
	}
	return profileWriteAll(fd, buf[0:8])
}
