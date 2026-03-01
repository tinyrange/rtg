//go:build alloc_debug

package runtime

const allocDebugEnabled = true

var allocDebugCalls int
var allocDebugReqBytes int
var allocDebugMmapCalls int
var allocDebugMmapBytes int
var allocDebugMapHashMetaBytes int
var allocDebugMapHashMetaAllocCalls int
var allocDebugMapHashRebuilds int
var allocDebugMapHashFallbacks int

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

//rtg:noprofile
func allocDebugRecordMapHashMeta(bytes int) {
	if bytes < 0 {
		bytes = 0
	}
	allocDebugMapHashMetaBytes = allocDebugMapHashMetaBytes + bytes
}

//rtg:noprofile
func allocDebugRecordMapHashMetaAlloc(bytes int) {
	if bytes < 0 {
		bytes = 0
	}
	allocDebugMapHashMetaAllocCalls = allocDebugMapHashMetaAllocCalls + 1
	allocDebugMapHashMetaBytes = allocDebugMapHashMetaBytes + bytes
}

//rtg:noprofile
func allocDebugRecordMapHashRebuild() {
	allocDebugMapHashRebuilds = allocDebugMapHashRebuilds + 1
}

//rtg:noprofile
func allocDebugRecordMapHashFallback() {
	allocDebugMapHashFallbacks = allocDebugMapHashFallbacks + 1
}

// AllocDebugReset clears allocator debug counters.
//
//rtg:noprofile
func AllocDebugReset() {
	allocDebugCalls = 0
	allocDebugReqBytes = 0
	allocDebugMmapCalls = 0
	allocDebugMmapBytes = 0
	allocDebugMapHashMetaBytes = 0
	allocDebugMapHashMetaAllocCalls = 0
	allocDebugMapHashRebuilds = 0
	allocDebugMapHashFallbacks = 0
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

//rtg:noprofile
func allocDebugMaybePrintSummary() {
	if profileLookupEnv("RTG_ALLOC_DEBUG_SUMMARY") == "" {
		return
	}
	allocCalls, reqBytes, mmapCalls, mmapBytes, _, _, _ := AllocDebugSnapshot()
	msg := "alloc_calls=" + IntToString(allocCalls) +
		" req_bytes=" + IntToString(reqBytes) +
		" mmap_calls=" + IntToString(mmapCalls) +
		" mmap_bytes=" + IntToString(mmapBytes) +
		" map_hash_meta_bytes=" + IntToString(allocDebugMapHashMetaBytes) +
		" map_hash_meta_alloc_calls=" + IntToString(allocDebugMapHashMetaAllocCalls) +
		" map_hash_rebuilds=" + IntToString(allocDebugMapHashRebuilds) +
		" map_hash_fallbacks=" + IntToString(allocDebugMapHashFallbacks) +
		"\n"
	SysWrite(2, Stringptr(msg), uintptr(len(msg)))
}
