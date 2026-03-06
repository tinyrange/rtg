//go:build !alloc_debug

package runtime

const allocDebugEnabled = false

//rtg:noprofile
//rtg:zerocall
func allocDebugRecord(_ int, _ int) {
}

//rtg:noprofile
//rtg:zerocall
func allocDebugRecordMapHashMeta(_ int) {
}

//rtg:noprofile
//rtg:zerocall
func allocDebugRecordMapHashMetaAlloc(_ int) {
}

//rtg:noprofile
//rtg:zerocall
func allocDebugRecordMapHashRebuild() {
}

//rtg:noprofile
//rtg:zerocall
func allocDebugRecordMapHashFallback() {
}

// AllocDebugReset clears allocator debug counters.
// In default builds this is a no-op.
//
//rtg:noprofile
//rtg:zerocall
func AllocDebugReset() {
}

// AllocDebugSnapshot reports allocator counters and current allocator state.
// In default builds counters are zeroed.
//
//rtg:noprofile
func AllocDebugSnapshot() (allocCalls int, reqBytes int, mmapCalls int, mmapBytes int, nextChunk int, chunkMax int, heapAvail int) {
	avail := 0
	if heapEnd >= heapPtr {
		avail = int(heapEnd - heapPtr)
	}
	return 0, 0, 0, 0, heapChunk, heapChunkMax, avail
}

//rtg:noprofile
//rtg:zerocall
func allocDebugMaybePrintSummary() {
}
