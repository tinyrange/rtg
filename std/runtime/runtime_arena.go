package runtime

const arenaMaxNodes = 8192
const arenaMaxStack = 8192
const arenaBlockMinSize = 4096

const (
	arenaBlockOffNext = 0
	arenaBlockOffEnd  = arenaBlockOffNext + PtrSize
	arenaBlockHdrSize = arenaBlockOffEnd + PtrSize
)

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
var arenaReuseBlock []uintptr

var arenaFrameNode []uint32
var arenaFrameBlockHead []uintptr
var arenaFrameCurrentBlock []uintptr
var arenaFrameParentAlloc []int
var arenaFramePtr []uintptr
var arenaFrameEnd []uintptr
var arenaFrameRetained []byte
var arenaAllocRestore []int

var arenaInitDone bool
var arenaIniting bool
var arenaNodeCount int
var arenaCurrent int
var arenaAllocFrame int
var arenaAllocRestoreLen int
var arenaStackLen int
var arenaDropped bool
var arenaEnabled bool
var arenaBypassAlloc bool

//rtg:noprofile
func arenaEnsureInit() bool {
	if arenaInitDone {
		return true
	}
	if arenaIniting {
		return false
	}
	arenaIniting = true
	arenaBypassAlloc = true

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
	arenaReuseBlock = make([]uintptr, arenaMaxNodes)
	arenaFrameNode = make([]uint32, arenaMaxStack)
	arenaFrameBlockHead = make([]uintptr, arenaMaxStack)
	arenaFrameCurrentBlock = make([]uintptr, arenaMaxStack)
	arenaFrameParentAlloc = make([]int, arenaMaxStack)
	arenaFramePtr = make([]uintptr, arenaMaxStack)
	arenaFrameEnd = make([]uintptr, arenaMaxStack)
	arenaFrameRetained = make([]byte, arenaMaxStack)
	arenaAllocRestore = make([]int, arenaMaxStack)

	arenaNodeCount = 1
	arenaCurrent = 1
	arenaAllocFrame = -1
	arenaAllocRestoreLen = 0
	arenaStackLen = 0
	arenaDropped = false
	arenaName[1] = "<root>"
	arenaInitDone = true
	arenaIniting = false
	arenaBypassAlloc = false
	return true
}

func arenaBlockCapacity(block uintptr) int {
	if block == 0 {
		return 0
	}
	end := ReadPtr(block + uintptr(arenaBlockOffEnd))
	if end <= block+uintptr(arenaBlockHdrSize) {
		return 0
	}
	return int(end - (block + uintptr(arenaBlockHdrSize)))
}

func arenaAllocNewBlock(size int) (uintptr, int) {
	chunk := arenaBlockMinSize
	need := size + arenaBlockHdrSize
	if chunk < need {
		chunk = need
	}
	ptr, _, err := SysMmap(0, uintptr(chunk), 3, MmapAnonFlags, 0, 0)
	if err != 0 || ptr == 0 {
		runtimePanic("out of memory")
	}
	WritePtr(ptr+uintptr(arenaBlockOffNext), 0)
	WritePtr(ptr+uintptr(arenaBlockOffEnd), ptr+uintptr(chunk))
	return ptr, chunk
}

func arenaAcquireReusableBlock(node int, size int) uintptr {
	if node <= 0 || node > arenaNodeCount {
		return 0
	}
	prev := uintptr(0)
	block := arenaReuseBlock[node]
	for block != 0 {
		next := ReadPtr(block + uintptr(arenaBlockOffNext))
		if arenaBlockCapacity(block) >= size {
			if prev == 0 {
				arenaReuseBlock[node] = next
			} else {
				WritePtr(prev+uintptr(arenaBlockOffNext), next)
			}
			WritePtr(block+uintptr(arenaBlockOffNext), 0)
			return block
		}
		prev = block
		block = next
	}
	return 0
}

func arenaPushBlockListToReuse(node int, block uintptr) {
	if node <= 0 || node > arenaNodeCount || block == 0 {
		return
	}
	tail := block
	for {
		next := ReadPtr(tail + uintptr(arenaBlockOffNext))
		if next == 0 {
			break
		}
		tail = next
	}
	WritePtr(tail+uintptr(arenaBlockOffNext), arenaReuseBlock[node])
	arenaReuseBlock[node] = block
}

func arenaMergeBlocksIntoParent(parentFrame int, childBlockHead uintptr, childCurrent uintptr, childPtr uintptr, childEnd uintptr) {
	if parentFrame < 0 || parentFrame >= len(arenaFrameBlockHead) || childBlockHead == 0 {
		return
	}
	tail := childBlockHead
	for {
		next := ReadPtr(tail + uintptr(arenaBlockOffNext))
		if next == 0 {
			break
		}
		tail = next
	}
	WritePtr(tail+uintptr(arenaBlockOffNext), arenaFrameBlockHead[parentFrame])
	arenaFrameBlockHead[parentFrame] = childBlockHead
	if arenaFrameCurrentBlock[parentFrame] == 0 && childCurrent != 0 {
		arenaFrameCurrentBlock[parentFrame] = childCurrent
		arenaFramePtr[parentFrame] = childPtr
		arenaFrameEnd[parentFrame] = childEnd
	}
}

func arenaAlloc(size int) (uintptr, int) {
	if size <= 0 {
		size = PtrSize
	}
	size = (size + 7) &^ 7
	if arenaBypassAlloc || !arenaEnabled || arenaAllocFrame < 0 {
		return allocHeap(size)
	}
	idx := arenaAllocFrame
	if idx < 0 || idx >= len(arenaFramePtr) {
		return allocHeap(size)
	}
	ptr := arenaFramePtr[idx]
	end := arenaFrameEnd[idx]
	if ptr != 0 && end >= ptr+uintptr(size) {
		arenaFramePtr[idx] = ptr + uintptr(size)
		return ptr, 0
	}
	node := int(arenaFrameNode[idx])
	block := arenaAcquireReusableBlock(node, size)
	mmapChunk := 0
	if block == 0 {
		block, mmapChunk = arenaAllocNewBlock(size)
	}
	WritePtr(block+uintptr(arenaBlockOffNext), arenaFrameBlockHead[idx])
	arenaFrameBlockHead[idx] = block
	arenaFrameCurrentBlock[idx] = block
	ptr = block + uintptr(arenaBlockHdrSize)
	end = ReadPtr(block + uintptr(arenaBlockOffEnd))
	arenaFramePtr[idx] = ptr + uintptr(size)
	arenaFrameEnd[idx] = end
	return ptr, mmapChunk
}

// ArenaRetainCurrent promotes the current arena frame to its parent instead of
// recycling it when the function exits.
//
//rtg:noprofile
func ArenaRetainCurrent() {
	if !arenaEnsureInit() || arenaStackLen <= 0 {
		return
	}
	idx := arenaStackLen - 1
	if idx < 0 || idx >= len(arenaFrameRetained) {
		return
	}
	arenaFrameRetained[idx] = 1
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
		arenaEnters[parent] = arenaEnters[parent] + 1
		frameIdx := arenaStackLen - 1
		if frameIdx >= 0 && frameIdx < len(arenaFrameNode) {
			arenaFrameNode[frameIdx] = uint32(parent)
			arenaFrameBlockHead[frameIdx] = 0
			arenaFrameCurrentBlock[frameIdx] = 0
			arenaFrameParentAlloc[frameIdx] = arenaAllocFrame
			arenaFramePtr[frameIdx] = 0
			arenaFrameEnd[frameIdx] = 0
			arenaFrameRetained[frameIdx] = 0
			arenaAllocFrame = frameIdx
		}
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
	arenaEnters[child] = arenaEnters[child] + 1
	frameIdx := arenaStackLen - 1
	if frameIdx >= 0 && frameIdx < len(arenaFrameNode) {
		arenaFrameNode[frameIdx] = uint32(child)
		arenaFrameBlockHead[frameIdx] = 0
		arenaFrameCurrentBlock[frameIdx] = 0
		arenaFrameParentAlloc[frameIdx] = arenaAllocFrame
		arenaFramePtr[frameIdx] = 0
		arenaFrameEnd[frameIdx] = 0
		arenaFrameRetained[frameIdx] = 0
		arenaAllocFrame = frameIdx
	}
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
	frameIdx := arenaStackLen - 1
	if frameIdx >= 0 && frameIdx < len(arenaFrameNode) {
		if arenaAllocFrame == frameIdx {
			arenaAllocFrame = arenaFrameParentAlloc[frameIdx]
		}
		node := int(arenaFrameNode[frameIdx])
		blockHead := arenaFrameBlockHead[frameIdx]
		currentBlock := arenaFrameCurrentBlock[frameIdx]
		currentPtr := arenaFramePtr[frameIdx]
		currentEnd := arenaFrameEnd[frameIdx]
		retained := arenaFrameRetained[frameIdx] != 0
		if retained {
			parentFrame := frameIdx - 1
			if parentFrame >= 0 {
				arenaMergeBlocksIntoParent(parentFrame, blockHead, currentBlock, currentPtr, currentEnd)
			}
		} else {
			arenaPushBlockListToReuse(node, blockHead)
		}
		arenaFrameNode[frameIdx] = 0
		arenaFrameBlockHead[frameIdx] = 0
		arenaFrameCurrentBlock[frameIdx] = 0
		arenaFrameParentAlloc[frameIdx] = -1
		arenaFramePtr[frameIdx] = 0
		arenaFrameEnd[frameIdx] = 0
		arenaFrameRetained[frameIdx] = 0
	}
	arenaStackLen--
	p := int(arenaStack[arenaStackLen])
	if p <= 0 || p > arenaNodeCount {
		arenaCurrent = 1
		return
	}
	arenaCurrent = p
}

// ArenaUseParent temporarily routes allocations to the parent arena frame.
//
//rtg:noprofile
func ArenaUseParent() {
	if !arenaEnsureInit() {
		return
	}
	if arenaAllocFrame < 0 || arenaAllocRestoreLen >= len(arenaAllocRestore) {
		return
	}
	arenaAllocRestore[arenaAllocRestoreLen] = arenaAllocFrame
	arenaAllocRestoreLen = arenaAllocRestoreLen + 1
	if arenaAllocFrame < len(arenaFrameParentAlloc) {
		arenaAllocFrame = arenaFrameParentAlloc[arenaAllocFrame]
	} else {
		arenaAllocFrame = -1
	}
}

// ArenaRestore restores the allocation target saved by ArenaUseParent.
//
//rtg:noprofile
func ArenaRestore() {
	if !arenaEnsureInit() || arenaAllocRestoreLen <= 0 {
		return
	}
	arenaAllocRestoreLen = arenaAllocRestoreLen - 1
	arenaAllocFrame = arenaAllocRestore[arenaAllocRestoreLen]
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

	arenaAllocs[1] = arenaAllocs[1] + 1
	arenaReqBytes[1] = arenaReqBytes[1] + uint64(reqSize)
	arenaMmapBytes[1] = arenaMmapBytes[1] + uint64(mmapChunk)

	idx := arenaCurrent
	if idx <= 1 || idx > arenaNodeCount {
		return
	}
	arenaAllocs[idx] = arenaAllocs[idx] + 1
	arenaReqBytes[idx] = arenaReqBytes[idx] + uint64(reqSize)
	arenaMmapBytes[idx] = arenaMmapBytes[idx] + uint64(mmapChunk)
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
