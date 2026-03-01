//go:build alloc_debug

package runtime

const allocDebugEnabled = true

var allocDebugCalls int
var allocDebugReqBytes int
var allocDebugMmapCalls int
var allocDebugMmapBytes int

//rtg:noprofile
func allocDebugRecord(reqSize int, mmapChunk int) {
	if reqSize < 0 {
		reqSize = 0
	}
	if mmapChunk < 0 {
		mmapChunk = 0
	}
	allocDebugCalls = allocDebugCalls + 1
	allocDebugReqBytes = allocDebugReqBytes + reqSize
	if mmapChunk > 0 {
		allocDebugMmapCalls = allocDebugMmapCalls + 1
		allocDebugMmapBytes = allocDebugMmapBytes + mmapChunk
	}
}

// AllocDebugReset clears allocator debug counters.
//
//rtg:noprofile
func AllocDebugReset() {
	allocDebugCalls = 0
	allocDebugReqBytes = 0
	allocDebugMmapCalls = 0
	allocDebugMmapBytes = 0
}

// AllocDebugSnapshot reports allocator counters and current allocator state.
//
//rtg:noprofile
func AllocDebugSnapshot() (allocCalls int, reqBytes int, mmapCalls int, mmapBytes int, nextChunk int, chunkMax int, heapAvail int) {
	avail := 0
	if heapEnd >= heapPtr {
		avail = int(heapEnd - heapPtr)
	}
	return allocDebugCalls, allocDebugReqBytes, allocDebugMmapCalls, allocDebugMmapBytes, heapChunk, heapChunkMax, avail
}
