package runtime

// === Compiler intrinsics ===
// These cannot be implemented in Go. The compiler must provide them.

// Sliceptr returns the data pointer of a byte slice.
//
//rtg:internal Sliceptr
func Sliceptr(s []byte) uintptr

// Makeslice constructs a byte slice from a raw pointer, length, and capacity.
//
//rtg:internal Makeslice
func Makeslice(ptr uintptr, slen int, scap int) []byte

// Stringptr returns the data pointer of a string.
//
//rtg:internal Stringptr
func Stringptr(s string) uintptr

// Makestring constructs a string from a raw pointer and length.
//
//rtg:internal Makestring
func Makestring(ptr uintptr, slen int) string

// Tostring converts any value to its string representation.
// Requires compiler type dispatch since we have no type assertions.
//
//rtg:internal Tostring
func Tostring(v interface{}) string

// === Memory allocator ===

func runtimePanic(msg string) {
	if len(msg) > 0 {
		SysWrite(2, Stringptr(msg), uintptr(len(msg)))
	}
	SysWrite(2, Stringptr("\n"), 1)
	Exit(2)
}

var panicActive bool
var panicRecovered bool
var panicRecoverArmed bool
var panicRecoverDepth int
var panicValue interface{}
var panicMessage string

// PanicBegin records the current panic payload and starts panic-unwind mode.
func PanicBegin(v interface{}, msg string) {
	panicActive = true
	panicRecovered = false
	panicRecoverArmed = false
	panicRecoverDepth = 0
	panicValue = v
	panicMessage = msg
}

// PanicValueToString converts the active panic payload to text for OP_PANIC.
func PanicValueToString() string {
	if !panicActive {
		return "panic"
	}
	return panicMessage
}

// PanicWasRecovered reports whether recover() consumed the active panic.
func PanicWasRecovered() bool {
	return panicRecovered
}

// PanicShouldUnwind reports whether callers should branch into panic unwind.
func PanicShouldUnwind() bool {
	return panicActive && !panicRecoverArmed
}

// PanicReset clears panic bookkeeping after a recovered panic returns.
func PanicReset() {
	panicActive = false
	panicRecovered = false
	panicRecoverArmed = false
	panicRecoverDepth = 0
	panicValue = nil
	panicMessage = ""
}

// DeferRecoverEnter marks entry into a deferred callsite.
func DeferRecoverEnter() {
	panicRecoverArmed = panicActive
	panicRecoverDepth = 0
}

// DeferRecoverExit marks exit from a deferred callsite.
func DeferRecoverExit() {
	panicRecoverArmed = false
	panicRecoverDepth = 0
}

// DeferRecoverBeforeCall tracks nested calls from a deferred frame.
func DeferRecoverBeforeCall() {
	if panicRecoverArmed {
		panicRecoverDepth = panicRecoverDepth + 1
	}
}

// DeferRecoverAfterCall tracks return from nested calls in deferred frames.
func DeferRecoverAfterCall() {
	if panicRecoverArmed && panicRecoverDepth > 0 {
		panicRecoverDepth = panicRecoverDepth - 1
	}
}

// Recover implements the recover builtin contract for defer-unwind.
func Recover() interface{} {
	if !panicActive || !panicRecoverArmed || panicRecoverDepth != 0 {
		return nil
	}
	v := panicValue
	panicActive = false
	panicRecovered = true
	panicRecoverArmed = false
	panicRecoverDepth = 0
	panicValue = nil
	panicMessage = ""
	return v
}

var heapPtr uintptr
var heapEnd uintptr
var heapChunk int = 65536

var heapChunkMax int = 1048576

const mapReuseMaxClass = 62

const (
	mapReuseNodeOffNext       = 0
	mapReuseNodeOffClass      = mapReuseNodeOffNext + PtrSize
	mapReuseNodeOffEntryHead  = mapReuseNodeOffClass + PtrSize
	mapReuseNodeOffPtrHead    = mapReuseNodeOffEntryHead + PtrSize
	mapReuseNodeOffStringHead = mapReuseNodeOffPtrHead + PtrSize
	mapReuseNodeSize          = mapReuseNodeOffStringHead + PtrSize
)

var mapReuseBins uintptr

func mapReuseClassForCap(cap int) int {
	if cap <= 0 {
		return -1
	}
	if cap&(cap-1) != 0 {
		return -1
	}
	class := 0
	for cap > 1 {
		cap = cap >> 1
		class++
	}
	if class > mapReuseMaxClass {
		return -1
	}
	return class
}

func mapReuseFindBin(class int) uintptr {
	bin := mapReuseBins
	for bin != 0 {
		if int(ReadPtr(bin+uintptr(mapReuseNodeOffClass))) == class {
			return bin
		}
		bin = ReadPtr(bin + uintptr(mapReuseNodeOffNext))
	}
	bin = allocHeapTracked(mapReuseNodeSize)
	WritePtr(bin+uintptr(mapReuseNodeOffNext), mapReuseBins)
	WritePtr(bin+uintptr(mapReuseNodeOffClass), uintptr(class))
	WritePtr(bin+uintptr(mapReuseNodeOffEntryHead), 0)
	WritePtr(bin+uintptr(mapReuseNodeOffPtrHead), 0)
	WritePtr(bin+uintptr(mapReuseNodeOffStringHead), 0)
	mapReuseBins = bin
	return bin
}

func mapAllocEntryBlock(mcap int) uintptr {
	class := mapReuseClassForCap(mcap)
	if class >= 0 {
		bin := mapReuseFindBin(class)
		ptr := ReadPtr(bin + uintptr(mapReuseNodeOffEntryHead))
		if ptr != 0 {
			WritePtr(bin+uintptr(mapReuseNodeOffEntryHead), ReadPtr(ptr))
			return ptr
		}
	}
	return Alloc(mcap * MapEntrySize)
}

func mapFreeEntryBlock(ptr uintptr, mcap int) {
	class := mapReuseClassForCap(mcap)
	if class < 0 {
		return
	}
	bin := mapReuseFindBin(class)
	WritePtr(ptr, ReadPtr(bin+uintptr(mapReuseNodeOffEntryHead)))
	WritePtr(bin+uintptr(mapReuseNodeOffEntryHead), ptr)
}

func mapAllocPtrBlock(count int) uintptr {
	class := mapReuseClassForCap(count)
	if class >= 0 {
		bin := mapReuseFindBin(class)
		ptr := ReadPtr(bin + uintptr(mapReuseNodeOffPtrHead))
		if ptr != 0 {
			WritePtr(bin+uintptr(mapReuseNodeOffPtrHead), ReadPtr(ptr))
			return ptr
		}
	}
	return Alloc(count * PtrSize)
}

func mapFreePtrBlock(ptr uintptr, count int) {
	class := mapReuseClassForCap(count)
	if class < 0 {
		return
	}
	bin := mapReuseFindBin(class)
	WritePtr(ptr, ReadPtr(bin+uintptr(mapReuseNodeOffPtrHead)))
	WritePtr(bin+uintptr(mapReuseNodeOffPtrHead), ptr)
}

func mapAllocStringBlock(mcap int) uintptr {
	class := mapReuseClassForCap(mcap)
	if class >= 0 {
		bin := mapReuseFindBin(class)
		ptr := ReadPtr(bin + uintptr(mapReuseNodeOffStringHead))
		if ptr != 0 {
			WritePtr(bin+uintptr(mapReuseNodeOffStringHead), ReadPtr(ptr))
			return ptr
		}
	}
	return Alloc(mcap * (MapEntrySize + PtrSize))
}

func mapFreeStringBlock(ptr uintptr, mcap int) {
	class := mapReuseClassForCap(mcap)
	if class < 0 {
		return
	}
	bin := mapReuseFindBin(class)
	WritePtr(ptr, ReadPtr(bin+uintptr(mapReuseNodeOffStringHead)))
	WritePtr(bin+uintptr(mapReuseNodeOffStringHead), ptr)
}

// allocHeap allocates size bytes from the process-lifetime heap.
func allocHeap(size int) (uintptr, int) {
	// Round up to 8-byte alignment without division (important on no-div backends).
	size = (size + 7) &^ 7

	needGrow := heapPtr == 0
	mmapChunk := 0
	if !needGrow {
		avail := heapEnd - heapPtr
		if uintptr(size) > avail {
			needGrow = true
		}
	}
	if needGrow {
		chunk := heapChunk
		if size > chunk {
			chunk = size
		}
		if heapChunk < heapChunkMax {
			next := heapChunk * 2
			if next > heapChunkMax {
				next = heapChunkMax
			}
			heapChunk = next
		}
		// mmap(0, chunk, PROT_READ|PROT_WRITE, MAP_PRIVATE|MAP_ANONYMOUS, -1, 0)
		ptr, _, err := SysMmap(0, uintptr(chunk), 3, MmapAnonFlags, 0, 0)
		if err != 0 || ptr == 0 {
			runtimePanic("out of memory")
		}
		mmapChunk = chunk
		heapPtr = ptr
		heapEnd = ptr + uintptr(chunk)
	}
	result := heapPtr
	heapPtr = heapPtr + uintptr(size)
	return result, mmapChunk
}

// Alloc allocates size bytes, preferring the current arena frame when one is active.
func allocWithZeroInfo(size int) (uintptr, bool) {
	result, mmapChunk, freshZero := arenaAlloc(size)
	arenaRecordAlloc(size, mmapChunk)
	if allocDebugEnabled {
		allocDebugRecord(size, mmapChunk)
	}
	return result, freshZero
}

// Alloc allocates size bytes, preferring the current arena frame when one is active.
func Alloc(size int) uintptr {
	result, _ := allocWithZeroInfo(size)
	return result
}

// allocHeapTracked bypasses arena routing for runtime-owned storage that may
// be mutated or reused independently of the caller's arena lifetime.
func allocHeapTracked(size int) uintptr {
	result, mmapChunk := allocHeap(size)
	if allocDebugEnabled {
		allocDebugRecord(size, mmapChunk)
	}
	return result
}

// === Memory operations ===

// Memcopy copies n bytes from src to dst.
func Memcopy(dst uintptr, src uintptr, n int) {
	if n <= 0 {
		return
	}
	if dst == 0 {
		runtimePanic("Memcopy: nil dst")
	}
	if src == 0 {
		runtimePanic("Memcopy: nil src")
	}
	// Copy pointer-sized words first, then tail bytes.
	step := PtrSize
	for n >= step {
		WritePtr(dst, ReadPtr(src))
		dst = dst + uintptr(step)
		src = src + uintptr(step)
		n = n - step
	}
	if n > 0 {
		d := Makeslice(dst, n, n)
		s := Makeslice(src, n, n)
		i := 0
		for i < n {
			d[i] = s[i]
			i++
		}
	}
}

// Memzero zeroes n bytes starting at ptr.
func Memzero(ptr uintptr, n int) {
	if n <= 0 {
		return
	}
	if ptr == 0 {
		runtimePanic("Memzero: nil ptr")
	}
	step := PtrSize
	for n >= step {
		WritePtr(ptr, 0)
		ptr = ptr + uintptr(step)
		n = n - step
	}
	if n > 0 {
		b := Makeslice(ptr, n, n)
		i := 0
		for i < n {
			b[i] = 0
			i++
		}
	}
}

// readByte reads one byte from an arbitrary address without requiring alignment.
//rtg:zerocall
func readByte(ptr uintptr) byte {
	aligned := ptr &^ uintptr(PtrSize-1)
	shift := (ptr - aligned) * uintptr(8)
	return byte(ReadPtr(aligned) >> shift)
}

// === Type conversion helpers ===
// The compiler emits calls to these for string <-> []byte conversions.

// BytesToString copies a byte slice into a new string.
func BytesToString(b []byte) string {
	n := len(b)
	if n == 0 {
		return Makestring(0, 0)
	}
	ptr := Alloc(n)
	Memcopy(ptr, Sliceptr(b), n)
	return Makestring(ptr, n)
}

// StringToBytes copies a string into a new byte slice.
func StringToBytes(s string) []byte {
	n := len(s)
	if n == 0 {
		return Makeslice(0, 0, 0)
	}
	ptr := Alloc(n)
	Memcopy(ptr, Stringptr(s), n)
	return Makeslice(ptr, n, n)
}

// StringFromPtrZ builds a string from a NUL-terminated byte sequence.
// Used by bringup backends when interface string values arrive as raw data pointers.
// ByteToString converts a single byte into a 1-character string.
func ByteToString(b byte) string {
	ptr := Alloc(1)
	buf := Makeslice(ptr, 1, 1)
	buf[0] = b
	return Makestring(ptr, 1)
}

// RuneToString converts a Unicode code point to a UTF-8 string.
func RuneToString(r int) string {
	if r < 0 {
		r = 0xFFFD
	}
	if r <= 0x7F {
		return ByteToString(byte(r))
	}
	if r <= 0x7FF {
		ptr := Alloc(2)
		buf := Makeslice(ptr, 2, 2)
		buf[0] = byte(0xC0 | ((r >> 6) & 0x1F))
		buf[1] = byte(0x80 | (r & 0x3F))
		return Makestring(ptr, 2)
	}
	if r <= 0xFFFF {
		ptr := Alloc(3)
		buf := Makeslice(ptr, 3, 3)
		buf[0] = byte(0xE0 | ((r >> 12) & 0x0F))
		buf[1] = byte(0x80 | ((r >> 6) & 0x3F))
		buf[2] = byte(0x80 | (r & 0x3F))
		return Makestring(ptr, 3)
	}
	if r > 0x10FFFF {
		r = 0xFFFD
	}
	ptr := Alloc(4)
	buf := Makeslice(ptr, 4, 4)
	buf[0] = byte(0xF0 | ((r >> 18) & 0x07))
	buf[1] = byte(0x80 | ((r >> 12) & 0x3F))
	buf[2] = byte(0x80 | ((r >> 6) & 0x3F))
	buf[3] = byte(0x80 | (r & 0x3F))
	return Makestring(ptr, 4)
}

// StringDecodeRune decodes a UTF-8 rune from s at byte index i.
// Returns (rune, width). Invalid encodings return U+FFFD width 1.
func StringDecodeRune(s string, i int) (int, int) {
	if i < 0 || i >= len(s) {
		return 0, 0
	}
	b0 := int(s[i])
	if b0 < 0x80 {
		return b0, 1
	}
	if b0 < 0xC2 {
		return 0xFFFD, 1
	}
	if b0 < 0xE0 {
		if i+1 >= len(s) {
			return 0xFFFD, 1
		}
		b1 := int(s[i+1])
		if (b1 & 0xC0) != 0x80 {
			return 0xFFFD, 1
		}
		r := ((b0 & 0x1F) << 6) | (b1 & 0x3F)
		return r, 2
	}
	if b0 < 0xF0 {
		if i+2 >= len(s) {
			return 0xFFFD, 1
		}
		b1 := int(s[i+1])
		b2 := int(s[i+2])
		if (b1&0xC0) != 0x80 || (b2&0xC0) != 0x80 {
			return 0xFFFD, 1
		}
		r := ((b0 & 0x0F) << 12) | ((b1 & 0x3F) << 6) | (b2 & 0x3F)
		return r, 3
	}
	if b0 < 0xF8 {
		if i+3 >= len(s) {
			return 0xFFFD, 1
		}
		b1 := int(s[i+1])
		b2 := int(s[i+2])
		b3 := int(s[i+3])
		if (b1&0xC0) != 0x80 || (b2&0xC0) != 0x80 || (b3&0xC0) != 0x80 {
			return 0xFFFD, 1
		}
		r := ((b0 & 0x07) << 18) | ((b1 & 0x3F) << 12) | ((b2 & 0x3F) << 6) | (b3 & 0x3F)
		return r, 4
	}
	return 0xFFFD, 1
}

// IntToString converts an integer to its decimal string representation.
func IntToString(n int) string {
	if n == 0 {
		return Makestring(Stringptr("0"), 1)
	}
	neg := false
	var u uint
	if n < 0 {
		neg = true
		// Avoid signed overflow for the minimum int value on 32/64-bit targets.
		u = uint(0) - uint(n)
	} else {
		u = uint(n)
	}
	// Build digits in reverse
	buf := make([]byte, 20)
	i := 19
	for u > 0 {
		buf[i] = byte(u%10) + '0'
		u = u / 10
		i = i - 1
	}
	if neg {
		buf[i] = '-'
		i = i - 1
	}
	start := i + 1
	slen := 20 - start
	ptr := Alloc(slen)
	Memcopy(ptr, Sliceptr(buf[start:20]), slen)
	return Makestring(ptr, slen)
}

// StringSlice returns a substring s[low:high] without copying.
func StringSlice(s string, low int, high int) string {
	newLen := high - low
	if newLen <= 0 {
		return Makestring(0, 0)
	}
	ptr := Stringptr(s)
	if ptr == 0 && low > 0 {
		runtimePanic("string slice: nil ptr")
	}
	return Makestring(ptr+uintptr(low), newLen)
}

// StringConcat concatenates two strings and returns a new string.
func StringConcat(a string, b string) string {
	alen := len(a)
	blen := len(b)
	if alen == 0 {
		return b
	}
	if blen == 0 {
		return a
	}
	total := alen + blen
	if total == 0 {
		return Makestring(0, 0)
	}
	ptr := Alloc(total)
	if total <= 32 {
		aptr := Stringptr(a)
		bptr := Stringptr(b)
		i := 0
		for i < alen {
			WriteByte(ptr+uintptr(i), uintptr(readByte(aptr+uintptr(i))))
			i = i + 1
		}
		j := 0
		for j < blen {
			WriteByte(ptr+uintptr(alen+j), uintptr(readByte(bptr+uintptr(j))))
			j = j + 1
		}
	} else {
		Memcopy(ptr, Stringptr(a), alen)
		Memcopy(ptr+uintptr(alen), Stringptr(b), blen)
	}
	return Makestring(ptr, total)
}

// StringEqual returns true if two strings have equal content.
func StringEqual(a string, b string) bool {
	alen := len(a)
	blen := len(b)
	if alen != blen {
		return false
	}
	if alen == 0 {
		return true
	}
	aptr := Stringptr(a)
	bptr := Stringptr(b)
	if aptr == 0 {
		return false
	}
	if bptr == 0 {
		return false
	}
	i := 0
	for i < alen {
		ab := Makeslice(aptr+uintptr(i), 1, 1)
		bb := Makeslice(bptr+uintptr(i), 1, 1)
		if ab[0] != bb[0] {
			return false
		}
		i = i + 1
	}
	return true
}

// === Slice operations ===
// These replace assembly builtins with Go code.
// Slice headers: {data_ptr, len, cap, elem_size} - size is SliceHdrSize

func allocSliceStorage(byteSize int) (uintptr, uintptr, bool) {
	if byteSize <= 0 {
		header, freshZero := allocWithZeroInfo(SliceHdrSize)
		return header, 0, freshZero
	}
	header, freshZero := allocWithZeroInfo(SliceHdrSize + byteSize)
	dataPtr := header + uintptr(SliceHdrSize)
	return header, dataPtr, freshZero
}

// SliceMake allocates a new slice with the given length and element size.
func SliceMake(length int, elemSize int) uintptr {
	byteSize := length * elemSize
	header, dataPtr, freshZero := allocSliceStorage(byteSize)
	if byteSize > 0 && !freshZero {
		Memzero(dataPtr, byteSize)
	}
	WritePtr(header, dataPtr)
	WritePtr(header+uintptr(SliceOffLen), uintptr(length))
	WritePtr(header+uintptr(SliceOffCap), uintptr(length))
	WritePtr(header+uintptr(SliceOffEsz), uintptr(elemSize))
	return header
}

func SliceMakeCap(length int, capacity int, elemSize int) uintptr {
	byteSize := capacity * elemSize
	header, dataPtr, freshZero := allocSliceStorage(byteSize)
	if byteSize > 0 && !freshZero {
		Memzero(dataPtr, byteSize)
	}
	WritePtr(header, dataPtr)
	WritePtr(header+uintptr(SliceOffLen), uintptr(length))
	WritePtr(header+uintptr(SliceOffCap), uintptr(capacity))
	WritePtr(header+uintptr(SliceOffEsz), uintptr(elemSize))
	return header
}

// SliceAppend appends one element to a slice, growing if necessary.
// Returns the (possibly updated) header pointer.
func SliceAppend(hdr uintptr, elem uintptr, elemSize int) uintptr {
	if hdr == 0 {
		var dataPtr uintptr
		var freshZero bool
		hdr, dataPtr, freshZero = allocSliceStorage(8 * elemSize)
		if elemSize > 0 && !freshZero {
			Memzero(dataPtr, 8*elemSize)
		}
		WritePtr(hdr, dataPtr)
		WritePtr(hdr+uintptr(SliceOffLen), 0)
		WritePtr(hdr+uintptr(SliceOffCap), 8)
		WritePtr(hdr+uintptr(SliceOffEsz), uintptr(elemSize))
	}
	slen := int(ReadPtr(hdr + uintptr(SliceOffLen)))
	scap := int(ReadPtr(hdr + uintptr(SliceOffCap)))
	elemSize = int(ReadPtr(hdr + uintptr(SliceOffEsz)))
	if slen >= scap {
		newCap := scap * 2
		if newCap == 0 {
			newCap = 8
		}
		newData := Alloc(newCap * elemSize)
		oldData := ReadPtr(hdr)
		if slen > 0 {
			Memcopy(newData, oldData, slen*elemSize)
		}
		WritePtr(hdr, newData)
		WritePtr(hdr+uintptr(SliceOffCap), uintptr(newCap))
	}
	dataPtr := ReadPtr(hdr)
	addr := dataPtr + uintptr(slen*elemSize)
	if elemSize == 1 {
		WriteByte(addr, byte(elem))
	} else {
		WritePtr(addr, elem)
	}
	WritePtr(hdr+uintptr(SliceOffLen), uintptr(slen+1))
	return hdr
}

// SliceAppendU32LE appends a uint32 value to a []byte-like slice as four
// little-endian bytes in one grow/check pass.
func SliceAppendU32LE(hdr uintptr, v uintptr) uintptr {
	if hdr == 0 {
		var dataPtr uintptr
		var freshZero bool
		hdr, dataPtr, freshZero = allocSliceStorage(8)
		if !freshZero {
			Memzero(dataPtr, 8)
		}
		WritePtr(hdr, dataPtr)
		WritePtr(hdr+uintptr(SliceOffLen), 0)
		WritePtr(hdr+uintptr(SliceOffCap), 8)
		WritePtr(hdr+uintptr(SliceOffEsz), 1)
	}

	slen := int(ReadPtr(hdr + uintptr(SliceOffLen)))
	scap := int(ReadPtr(hdr + uintptr(SliceOffCap)))
	elemSize := int(ReadPtr(hdr + uintptr(SliceOffEsz)))

	// Defensive fallback: keep behavior correct if called for non-byte slices.
	if elemSize != 1 {
		hdr = SliceAppend(hdr, uintptr(byte(v)), 1)
		hdr = SliceAppend(hdr, uintptr(byte(v>>8)), 1)
		hdr = SliceAppend(hdr, uintptr(byte(v>>16)), 1)
		hdr = SliceAppend(hdr, uintptr(byte(v>>24)), 1)
		return hdr
	}

	needed := slen + 4
	if needed > scap {
		newCap := scap * 2
		if newCap < needed {
			newCap = needed
		}
		if newCap == 0 {
			newCap = 8
		}
		newData := Alloc(newCap)
		oldData := ReadPtr(hdr)
		if slen > 0 {
			Memcopy(newData, oldData, slen)
		}
		WritePtr(hdr, newData)
		WritePtr(hdr+uintptr(SliceOffCap), uintptr(newCap))
	}

	dataPtr := ReadPtr(hdr)
	addr := dataPtr + uintptr(slen)
	WriteByte(addr, byte(v))
	WriteByte(addr+1, byte(v>>8))
	WriteByte(addr+2, byte(v>>16))
	WriteByte(addr+3, byte(v>>24))
	WritePtr(hdr+uintptr(SliceOffLen), uintptr(needed))
	return hdr
}

// SliceAppendU64LE appends a uint64 value to a []byte-like slice as eight
// little-endian bytes in one grow/check pass.
func SliceAppendU64LE(hdr uintptr, v uintptr) uintptr {
	if hdr == 0 {
		var dataPtr uintptr
		var freshZero bool
		hdr, dataPtr, freshZero = allocSliceStorage(8)
		if !freshZero {
			Memzero(dataPtr, 8)
		}
		WritePtr(hdr, dataPtr)
		WritePtr(hdr+uintptr(SliceOffLen), 0)
		WritePtr(hdr+uintptr(SliceOffCap), 8)
		WritePtr(hdr+uintptr(SliceOffEsz), 1)
	}

	slen := int(ReadPtr(hdr + uintptr(SliceOffLen)))
	scap := int(ReadPtr(hdr + uintptr(SliceOffCap)))
	elemSize := int(ReadPtr(hdr + uintptr(SliceOffEsz)))

	// Defensive fallback: keep behavior correct if called for non-byte slices.
	if elemSize != 1 {
		hdr = SliceAppend(hdr, uintptr(byte(v)), 1)
		hdr = SliceAppend(hdr, uintptr(byte(v>>8)), 1)
		hdr = SliceAppend(hdr, uintptr(byte(v>>16)), 1)
		hdr = SliceAppend(hdr, uintptr(byte(v>>24)), 1)
		hdr = SliceAppend(hdr, uintptr(byte(v>>32)), 1)
		hdr = SliceAppend(hdr, uintptr(byte(v>>40)), 1)
		hdr = SliceAppend(hdr, uintptr(byte(v>>48)), 1)
		hdr = SliceAppend(hdr, uintptr(byte(v>>56)), 1)
		return hdr
	}

	needed := slen + 8
	if needed > scap {
		newCap := scap * 2
		if newCap < needed {
			newCap = needed
		}
		if newCap == 0 {
			newCap = 8
		}
		newData := Alloc(newCap)
		oldData := ReadPtr(hdr)
		if slen > 0 {
			Memcopy(newData, oldData, slen)
		}
		WritePtr(hdr, newData)
		WritePtr(hdr+uintptr(SliceOffCap), uintptr(newCap))
	}

	dataPtr := ReadPtr(hdr)
	addr := dataPtr + uintptr(slen)
	WriteByte(addr, byte(v))
	WriteByte(addr+1, byte(v>>8))
	WriteByte(addr+2, byte(v>>16))
	WriteByte(addr+3, byte(v>>24))
	WriteByte(addr+4, byte(v>>32))
	WriteByte(addr+5, byte(v>>40))
	WriteByte(addr+6, byte(v>>48))
	WriteByte(addr+7, byte(v>>56))
	WritePtr(hdr+uintptr(SliceOffLen), uintptr(needed))
	return hdr
}

// SliceAppendSlice appends all elements from src slice to dst slice.
// Returns the (possibly updated) dst header pointer.
func SliceAppendSlice(dst uintptr, src uintptr) uintptr {
	if src == 0 {
		return dst
	}
	srcLen := int(ReadPtr(src + uintptr(SliceOffLen)))
	if srcLen == 0 {
		return dst
	}
	if dst == 0 {
		elemSize := int(ReadPtr(src + uintptr(SliceOffEsz)))
		var dataPtr uintptr
		var freshZero bool
		dst, dataPtr, freshZero = allocSliceStorage(srcLen * elemSize)
		if srcLen > 0 && elemSize > 0 && !freshZero {
			Memzero(dataPtr, srcLen*elemSize)
		}
		WritePtr(dst, dataPtr)
		WritePtr(dst+uintptr(SliceOffLen), 0)
		WritePtr(dst+uintptr(SliceOffCap), uintptr(srcLen))
		WritePtr(dst+uintptr(SliceOffEsz), uintptr(elemSize))
	}
	dstLen := int(ReadPtr(dst + uintptr(SliceOffLen)))
	dstCap := int(ReadPtr(dst + uintptr(SliceOffCap)))
	elemSize := int(ReadPtr(dst + uintptr(SliceOffEsz)))
	needed := dstLen + srcLen
	if needed > dstCap {
		newCap := dstCap * 2
		if newCap < needed {
			newCap = needed
		}
		newData := Alloc(newCap * elemSize)
		oldData := ReadPtr(dst)
		if dstLen > 0 {
			Memcopy(newData, oldData, dstLen*elemSize)
		}
		WritePtr(dst, newData)
		WritePtr(dst+uintptr(SliceOffCap), uintptr(newCap))
	}
	dstData := ReadPtr(dst)
	srcData := ReadPtr(src)
	Memcopy(dstData+uintptr(dstLen*elemSize), srcData, srcLen*elemSize)
	WritePtr(dst+uintptr(SliceOffLen), uintptr(needed))
	return dst
}

// SliceCopy copies elements from src to dst, returning the number copied.
func SliceCopy(dst uintptr, src uintptr) int {
	if dst == 0 || src == 0 {
		return 0
	}
	dstLen := int(ReadPtr(dst + uintptr(SliceOffLen)))
	srcLen := int(ReadPtr(src + uintptr(SliceOffLen)))
	n := dstLen
	if srcLen < n {
		n = srcLen
	}
	if n > 0 {
		elemSize := int(ReadPtr(dst + uintptr(SliceOffEsz)))
		dstData := ReadPtr(dst)
		srcData := ReadPtr(src)
		Memcopy(dstData, srcData, n*elemSize)
	}
	return n
}

// SliceClone makes a deep copy of a slice header and its backing storage.
func SliceClone(hdr uintptr) uintptr {
	if hdr == 0 {
		return 0
	}
	srcData := ReadPtr(hdr)
	slen := int(ReadPtr(hdr + uintptr(SliceOffLen)))
	scap := int(ReadPtr(hdr + uintptr(SliceOffCap)))
	elemSize := int(ReadPtr(hdr + uintptr(SliceOffEsz)))
	if elemSize <= 0 {
		elemSize = PtrSize
	}
	if scap < slen {
		scap = slen
	}
	newHdr := Alloc(SliceHdrSize)
	newData := uintptr(0)
	if scap > 0 {
		newData = Alloc(scap * elemSize)
		if slen > 0 && srcData != 0 {
			Memcopy(newData, srcData, slen*elemSize)
		}
	}
	WritePtr(newHdr, newData)
	WritePtr(newHdr+uintptr(SliceOffLen), uintptr(slen))
	WritePtr(newHdr+uintptr(SliceOffCap), uintptr(scap))
	WritePtr(newHdr+uintptr(SliceOffEsz), uintptr(elemSize))
	return newHdr
}

// SliceCloneArray clones a fixed-array header/backing store and recursively
// clones nested array elements when nestedDepth > 0.
func SliceCloneArray(hdr uintptr, nestedDepth int) uintptr {
	cloned := SliceClone(hdr)
	if cloned == 0 || nestedDepth <= 0 {
		return cloned
	}
	elemSize := int(ReadPtr(cloned + uintptr(SliceOffEsz)))
	if elemSize != PtrSize {
		return cloned
	}
	data := ReadPtr(cloned)
	if data == 0 {
		return cloned
	}
	slen := int(ReadPtr(cloned + uintptr(SliceOffLen)))
	i := 0
	for i < slen {
		slot := data + uintptr(i*PtrSize)
		elemHdr := ReadPtr(slot)
		if elemHdr != 0 {
			WritePtr(slot, SliceCloneArray(elemHdr, nestedDepth-1))
		}
		i++
	}
	return cloned
}

// SliceReslice creates a new slice header for s[low:high].
func SliceReslice(hdr uintptr, low int, high int) uintptr {
	if hdr == 0 {
		if low == 0 && high == 0 {
			return 0
		}
		runtimePanic("slice of nil slice")
	}
	elemSize := int(ReadPtr(hdr + uintptr(SliceOffEsz)))
	oldData := ReadPtr(hdr)
	oldCap := int(ReadPtr(hdr + uintptr(SliceOffCap)))
	newData := oldData + uintptr(low*elemSize)
	newLen := high - low
	newCap := oldCap - low
	newHdr := Alloc(SliceHdrSize)
	WritePtr(newHdr, newData)
	WritePtr(newHdr+uintptr(SliceOffLen), uintptr(newLen))
	WritePtr(newHdr+uintptr(SliceOffCap), uintptr(newCap))
	WritePtr(newHdr+uintptr(SliceOffEsz), uintptr(elemSize))
	return newHdr
}

// SliceResliceFull creates a new slice header for s[low:high:max].
func SliceResliceFull(hdr uintptr, low int, high int, max int) uintptr {
	if hdr == 0 {
		if low == 0 && high == 0 && max == 0 {
			return 0
		}
		runtimePanic("slice of nil slice")
	}
	elemSize := int(ReadPtr(hdr + uintptr(SliceOffEsz)))
	oldData := ReadPtr(hdr)
	newData := oldData + uintptr(low*elemSize)
	newLen := high - low
	newCap := max - low
	newHdr := Alloc(SliceHdrSize)
	WritePtr(newHdr, newData)
	WritePtr(newHdr+uintptr(SliceOffLen), uintptr(newLen))
	WritePtr(newHdr+uintptr(SliceOffCap), uintptr(newCap))
	WritePtr(newHdr+uintptr(SliceOffEsz), uintptr(elemSize))
	return newHdr
}

// === Map operations ===
// Maps use a simple linear-scan table.
// Map header (SliceHdrSize bytes): {data_ptr, len, cap, keyKind}
// Each entry is MapEntrySize bytes: {key, value}

// mapStrEqual compares two string header pointers by content.
func mapStrEqual(a uintptr, b uintptr) bool {
	if a == b {
		return true
	}
	if a == 0 || b == 0 {
		return false
	}
	alen := int(ReadPtr(a + uintptr(PtrSize)))
	blen := int(ReadPtr(b + uintptr(PtrSize)))
	if alen != blen {
		return false
	}
	if alen == 0 {
		return true
	}
	aptr := ReadPtr(a)
	bptr := ReadPtr(b)
	// Fast path for aligned string data: compare by machine words, then tail bytes.
	if ((aptr | bptr) & uintptr(PtrSize-1)) == 0 {
		nwords := alen / PtrSize
		w := 0
		for w < nwords {
			off := uintptr(w * PtrSize)
			if ReadPtr(aptr+off) != ReadPtr(bptr+off) {
				return false
			}
			w = w + 1
		}
		i := nwords * PtrSize
		for i < alen {
			if readByte(aptr+uintptr(i)) != readByte(bptr+uintptr(i)) {
				return false
			}
			i = i + 1
		}
		return true
	}
	// Conservative fallback for unaligned data.
	i := 0
	for i < alen {
		if readByte(aptr+uintptr(i)) != readByte(bptr+uintptr(i)) {
			return false
		}
		i = i + 1
	}
	return true
}

func mapKeyEqual(keyKind int, lhs uintptr, rhs uintptr) bool {
	if keyKind == 1 {
		return mapStrEqual(lhs, rhs)
	}
	return lhs == rhs
}

func mapFindKey(data uintptr, mlen int, keyKind int, key uintptr) int {
	i := 0
	for i < mlen {
		entryAddr := data + uintptr(i*MapEntrySize)
		if mapKeyEqual(keyKind, ReadPtr(entryAddr), key) {
			return i
		}
		i = i + 1
	}
	return -1
}

func mapFindIntKey(data uintptr, mlen int, key uintptr) int {
	i := 0
	for i < mlen {
		if ReadPtr(data+uintptr(i*MapEntrySize)) == key {
			return i
		}
		i = i + 1
	}
	return -1
}

func mapFindStringKey(data uintptr, mlen int, key uintptr) int {
	i := 0
	for i < mlen {
		if mapStrEqual(ReadPtr(data+uintptr(i*MapEntrySize)), key) {
			return i
		}
		i = i + 1
	}
	return -1
}

const (
	mapHdrOffHashSlots = (SliceHdrSize + PtrSize - 1) &^ (PtrSize - 1)
	mapHdrOffHashCap   = mapHdrOffHashSlots + PtrSize
	mapHdrOffHashes    = mapHdrOffHashCap + PtrSize
	mapHdrSize         = mapHdrOffHashes + PtrSize
	mapHashMinLen      = 16
)

func mapHashCapForEntries(entryCap int) int {
	need := entryCap * 2
	if need < 64 {
		need = 64
	}
	cap := 1
	for cap < need {
		cap = cap * 2
	}
	return cap
}

func mapIntHashKey(key uintptr) uintptr {
	h := key * uintptr(2654435761)
	h = h ^ (h >> 16)
	if PtrSize > 4 {
		h = h ^ (h >> 32)
	}
	return h
}

func mapStringHashKey(key uintptr) uintptr {
	if key == 0 {
		return 0
	}
	sptr := ReadPtr(key)
	slen := int(ReadPtr(key + uintptr(PtrSize)))
	h := (uintptr(0x811c) << 16) | uintptr(0x9dc5)
	i := 0
	for i < slen {
		h = h ^ uintptr(readByte(sptr+uintptr(i)))
		h = h * uintptr(16777619)
		i = i + 1
	}
	h = h ^ (h >> 16)
	if PtrSize > 4 {
		h = h ^ (h >> 32)
	}
	return h
}

func mapInitHashSlots(hdr uintptr, hashCap int) uintptr {
	slots := mapAllocPtrBlock(hashCap)
	if allocDebugEnabled {
		allocDebugRecordMapHashMetaAlloc(hashCap * PtrSize)
	}
	Memzero(slots, hashCap*PtrSize)
	WritePtr(hdr+uintptr(mapHdrOffHashSlots), slots)
	WritePtr(hdr+uintptr(mapHdrOffHashCap), uintptr(hashCap))
	return slots
}

func mapIntRebuildHashSlots(hdr uintptr, data uintptr, mlen int) {
	slots := ReadPtr(hdr + uintptr(mapHdrOffHashSlots))
	hashCap := int(ReadPtr(hdr + uintptr(mapHdrOffHashCap)))
	if slots == 0 || hashCap <= 0 {
		return
	}
	Memzero(slots, hashCap*PtrSize)
	i := 0
	for i < mlen {
		entryAddr := data + uintptr(i*MapEntrySize)
		key := ReadPtr(entryAddr)
		mapInsertIndex(slots, hashCap, i, mapIntHashKey(key))
		i = i + 1
	}
}

func mapStringRebuildHashSlots(hdr uintptr, data uintptr, mlen int) {
	slots := ReadPtr(hdr + uintptr(mapHdrOffHashSlots))
	hashCap := int(ReadPtr(hdr + uintptr(mapHdrOffHashCap)))
	hashes := ReadPtr(hdr + uintptr(mapHdrOffHashes))
	if slots == 0 || hashes == 0 || hashCap <= 0 {
		return
	}
	if allocDebugEnabled {
		allocDebugRecordMapHashRebuild()
	}
	Memzero(slots, hashCap*PtrSize)
	mask := hashCap - 1
	i := 0
	for i < mlen {
		h := ReadPtr(hashes + uintptr(i*PtrSize))
		if h == 0 {
			key := ReadPtr(data + uintptr(i*MapEntrySize))
			h = mapStringHashKey(key)
			WritePtr(hashes+uintptr(i*PtrSize), h)
		}
		slot := int(h & uintptr(mask))
		probes := 0
		for probes < hashCap {
			slotAddr := slots + uintptr(slot*PtrSize)
			if ReadPtr(slotAddr) == 0 {
				WritePtr(slotAddr, uintptr(i+1))
				break
			}
			slot = (slot + 1) & mask
			probes = probes + 1
		}
		i = i + 1
	}
}

func mapIntFindIndex(data uintptr, mlen int, slots uintptr, hashCap int, key uintptr, keyHash uintptr) (int, int, bool) {
	if slots == 0 || hashCap <= 0 {
		return -1, -1, false
	}
	mask := hashCap - 1
	slot := int(keyHash & uintptr(mask))
	probes := 0
	for probes < hashCap {
		slotAddr := slots + uintptr(slot*PtrSize)
		entry := ReadPtr(slotAddr)
		if entry == 0 {
			return -1, slot, false
		}
		idx := int(entry) - 1
		if idx >= 0 && idx < mlen {
			entryAddr := data + uintptr(idx*MapEntrySize)
			if ReadPtr(entryAddr) == key {
				return idx, slot, true
			}
		}
		slot = (slot + 1) & mask
		probes = probes + 1
	}
	return -1, -1, false
}

func mapStringFindIndex(data uintptr, mlen int, slots uintptr, hashes uintptr, hashCap int, key uintptr, keyHash uintptr) (int, int, bool) {
	if slots == 0 || hashes == 0 || hashCap <= 0 {
		return -1, -1, false
	}
	mask := hashCap - 1
	slot := int(keyHash & uintptr(mask))
	probes := 0
	for probes < hashCap {
		slotAddr := slots + uintptr(slot*PtrSize)
		entry := ReadPtr(slotAddr)
		if entry == 0 {
			return -1, slot, false
		}
		idx := int(entry) - 1
		if idx >= 0 && idx < mlen {
			if ReadPtr(hashes+uintptr(idx*PtrSize)) == keyHash {
				entryAddr := data + uintptr(idx*MapEntrySize)
				if mapStrEqual(ReadPtr(entryAddr), key) {
					return idx, slot, true
				}
			}
		}
		slot = (slot + 1) & mask
		probes = probes + 1
	}
	return -1, -1, false
}

func mapInsertIndex(slots uintptr, hashCap int, idx int, keyHash uintptr) {
	mask := hashCap - 1
	slot := int(keyHash & uintptr(mask))
	probes := 0
	for probes < hashCap {
		slotAddr := slots + uintptr(slot*PtrSize)
		if ReadPtr(slotAddr) == 0 {
			WritePtr(slotAddr, uintptr(idx+1))
			return
		}
		slot = (slot + 1) & mask
		probes = probes + 1
	}
	runtimePanic("map hash table full")
}

func mapStringAllocDataHashes(mcap int) (data uintptr, hashes uintptr) {
	dataBytes := mcap * MapEntrySize
	hashBytes := mcap * PtrSize
	block := mapAllocStringBlock(mcap)
	if allocDebugEnabled {
		allocDebugRecordMapHashMeta(hashBytes)
	}
	return block, block + uintptr(dataBytes)
}

func mapStringUsesCombinedBlock(data uintptr, hashes uintptr, mcap int) bool {
	if data == 0 || hashes == 0 || mcap <= 0 {
		return false
	}
	return hashes == data+uintptr(mcap*MapEntrySize)
}

func mapIntInitHashState(hdr uintptr, mcap int) {
	hashCap := mapHashCapForEntries(mcap)
	mapInitHashSlots(hdr, hashCap)
	WritePtr(hdr+uintptr(mapHdrOffHashes), 0)
}

func mapStringInitHashState(hdr uintptr, mcap int, hashes uintptr) {
	hashCap := mapHashCapForEntries(mcap)
	mapInitHashSlots(hdr, hashCap)
	if hashes == 0 {
		hashes = mapAllocPtrBlock(mcap)
		if allocDebugEnabled {
			allocDebugRecordMapHashMetaAlloc(mcap * PtrSize)
		}
	} else {
		if allocDebugEnabled {
			allocDebugRecordMapHashMeta(mcap * PtrSize)
		}
	}
	Memzero(hashes, mcap*PtrSize)
	WritePtr(hdr+uintptr(mapHdrOffHashes), hashes)
}

func mapMakeWithKeyKind(keyKind int) uintptr {
	capHint := 32
	hdr := Alloc(mapHdrSize)
	data := mapAllocEntryBlock(capHint)
	WritePtr(hdr, data)
	WritePtr(hdr+uintptr(SliceOffLen), 0)
	WritePtr(hdr+uintptr(SliceOffCap), uintptr(capHint))
	WritePtr(hdr+uintptr(SliceOffEsz), uintptr(keyKind))
	WritePtr(hdr+uintptr(mapHdrOffHashSlots), 0)
	WritePtr(hdr+uintptr(mapHdrOffHashCap), 0)
	WritePtr(hdr+uintptr(mapHdrOffHashes), 0)
	return hdr
}

// MapMake allocates an empty map header. keyKind: 0=int, 1=string.
func MapMake(keyKind int) uintptr {
	return mapMakeWithKeyKind(keyKind)
}

// MapGet looks up a key in the map. Returns (value, found).
func MapGet(hdr uintptr, key uintptr) (uintptr, bool) {
	if hdr == 0 {
		return 0, false
	}
	mlen := int(ReadPtr(hdr + uintptr(SliceOffLen)))
	keyKind := int(ReadPtr(hdr + uintptr(SliceOffEsz)))
	data := ReadPtr(hdr)
	slots := ReadPtr(hdr + uintptr(mapHdrOffHashSlots))
	hashCap := int(ReadPtr(hdr + uintptr(mapHdrOffHashCap)))
	if keyKind == 1 {
		hashes := ReadPtr(hdr + uintptr(mapHdrOffHashes))
		if slots != 0 && hashes != 0 && hashCap > 0 {
			keyHash := mapStringHashKey(key)
			idx, _, found := mapStringFindIndex(data, mlen, slots, hashes, hashCap, key, keyHash)
			if found {
				entryAddr := data + uintptr(idx*MapEntrySize)
				return ReadPtr(entryAddr + uintptr(MapEntryOffVal)), true
			}
			return 0, false
		}
	} else if slots != 0 && hashCap > 0 {
		keyHash := mapIntHashKey(key)
		idx, _, found := mapIntFindIndex(data, mlen, slots, hashCap, key, keyHash)
		if found {
			entryAddr := data + uintptr(idx*MapEntrySize)
			return ReadPtr(entryAddr + uintptr(MapEntryOffVal)), true
		}
		return 0, false
	}
	idx := mapFindKey(data, mlen, keyKind, key)
	if idx >= 0 {
		entryAddr := data + uintptr(idx*MapEntrySize)
		return ReadPtr(entryAddr + uintptr(MapEntryOffVal)), true
	}
	return 0, false
}

// MapSet inserts or updates a key-value pair in the map.
// Returns the (possibly updated) header pointer.
func MapSet(hdr uintptr, key uintptr, value uintptr) uintptr {
	if hdr == 0 {
		hdr = MapMake(0)
	}
	mlen := int(ReadPtr(hdr + uintptr(SliceOffLen)))
	keyKind := int(ReadPtr(hdr + uintptr(SliceOffEsz)))
	data := ReadPtr(hdr)
	mcap := int(ReadPtr(hdr + uintptr(SliceOffCap)))
	slots := ReadPtr(hdr + uintptr(mapHdrOffHashSlots))
	hashCap := int(ReadPtr(hdr + uintptr(mapHdrOffHashCap)))
	if keyKind == 1 {
		hashes := ReadPtr(hdr + uintptr(mapHdrOffHashes))
		if slots != 0 && hashes != 0 && hashCap > 0 {
			keyHash := mapStringHashKey(key)
			idx, _, found := mapStringFindIndex(data, mlen, slots, hashes, hashCap, key, keyHash)
			if found {
				entryAddr := data + uintptr(idx*MapEntrySize)
				WritePtr(entryAddr+uintptr(MapEntryOffVal), value)
				return hdr
			}
			if mlen >= mcap {
				oldData := data
				oldHashes := hashes
				oldSlots := slots
				oldCap := mcap
				oldHashCap := hashCap
				newCap := mcap * 2
				if newCap < 8 {
					newCap = 8
				}
				newData, newHashes := mapStringAllocDataHashes(newCap)
				if mlen > 0 {
					Memcopy(newData, data, mlen*MapEntrySize)
					Memcopy(newHashes, hashes, mlen*PtrSize)
				}
				if mlen < newCap {
					Memzero(newHashes+uintptr(mlen*PtrSize), (newCap-mlen)*PtrSize)
				}
				data = newData
				hashes = newHashes
				mcap = newCap
				WritePtr(hdr, data)
				WritePtr(hdr+uintptr(SliceOffCap), uintptr(mcap))
				WritePtr(hdr+uintptr(mapHdrOffHashes), hashes)

				needHashCap := mapHashCapForEntries(mcap)
				if hashCap != needHashCap {
					hashCap = needHashCap
					mapInitHashSlots(hdr, hashCap)
				}
				mapStringRebuildHashSlots(hdr, data, mlen)
				slots = ReadPtr(hdr + uintptr(mapHdrOffHashSlots))
				hashCap = int(ReadPtr(hdr + uintptr(mapHdrOffHashCap)))
				if mapStringUsesCombinedBlock(oldData, oldHashes, oldCap) {
					mapFreeStringBlock(oldData, oldCap)
				} else {
					mapFreeEntryBlock(oldData, oldCap)
					mapFreePtrBlock(oldHashes, oldCap)
				}
				if oldSlots != 0 && oldHashCap != hashCap {
					mapFreePtrBlock(oldSlots, oldHashCap)
				}
			}
			entryAddr := data + uintptr(mlen*MapEntrySize)
			WritePtr(entryAddr, key)
			WritePtr(entryAddr+uintptr(MapEntryOffVal), value)
			WritePtr(hashes+uintptr(mlen*PtrSize), keyHash)
			WritePtr(hdr+uintptr(SliceOffLen), uintptr(mlen+1))
			mapInsertIndex(slots, hashCap, mlen, keyHash)
			return hdr
		}
		// Small string maps stay on linear scan until a minimum size.
		idx := mapFindKey(data, mlen, keyKind, key)
		if idx >= 0 {
			entryAddr := data + uintptr(idx*MapEntrySize)
			WritePtr(entryAddr+uintptr(MapEntryOffVal), value)
			return hdr
		}
		if mlen >= mcap {
			oldData := data
			oldCap := mcap
			newCap := mcap * 2
			if newCap < 8 {
				newCap = 8
			}
			newData := mapAllocEntryBlock(newCap)
			if mlen > 0 {
				Memcopy(newData, data, mlen*MapEntrySize)
			}
			data = newData
			mcap = newCap
			WritePtr(hdr, data)
			WritePtr(hdr+uintptr(SliceOffCap), uintptr(mcap))
			mapFreeEntryBlock(oldData, oldCap)
		}
		entryAddr := data + uintptr(mlen*MapEntrySize)
		WritePtr(entryAddr, key)
		WritePtr(entryAddr+uintptr(MapEntryOffVal), value)
		newLen := mlen + 1
		WritePtr(hdr+uintptr(SliceOffLen), uintptr(newLen))
		if newLen >= mapHashMinLen {
			mapStringInitHashState(hdr, mcap, 0)
			mapStringRebuildHashSlots(hdr, data, newLen)
		}
		return hdr
	}
	if slots != 0 && hashCap > 0 {
		keyHash := mapIntHashKey(key)
		idx, _, found := mapIntFindIndex(data, mlen, slots, hashCap, key, keyHash)
		if found {
			entryAddr := data + uintptr(idx*MapEntrySize)
			WritePtr(entryAddr+uintptr(MapEntryOffVal), value)
			return hdr
		}
		if mlen >= mcap {
			oldData := data
			oldSlots := slots
			oldCap := mcap
			oldHashCap := hashCap
			newCap := mcap * 2
			if newCap < 8 {
				newCap = 8
			}
			newData := mapAllocEntryBlock(newCap)
			if mlen > 0 {
				Memcopy(newData, data, mlen*MapEntrySize)
			}
			data = newData
			mcap = newCap
			WritePtr(hdr, data)
			WritePtr(hdr+uintptr(SliceOffCap), uintptr(mcap))

			needHashCap := mapHashCapForEntries(mcap)
			if hashCap != needHashCap {
				hashCap = needHashCap
				mapInitHashSlots(hdr, hashCap)
			}
			mapIntRebuildHashSlots(hdr, data, mlen)
			slots = ReadPtr(hdr + uintptr(mapHdrOffHashSlots))
			hashCap = int(ReadPtr(hdr + uintptr(mapHdrOffHashCap)))
			mapFreeEntryBlock(oldData, oldCap)
			if oldSlots != 0 && oldHashCap != hashCap {
				mapFreePtrBlock(oldSlots, oldHashCap)
			}
		}
		entryAddr := data + uintptr(mlen*MapEntrySize)
		WritePtr(entryAddr, key)
		WritePtr(entryAddr+uintptr(MapEntryOffVal), value)
		WritePtr(hdr+uintptr(SliceOffLen), uintptr(mlen+1))
		mapInsertIndex(slots, hashCap, mlen, keyHash)
		return hdr
	}
	// Search for existing key
	idx := mapFindKey(data, mlen, keyKind, key)
	if idx >= 0 {
		entryAddr := data + uintptr(idx*MapEntrySize)
		WritePtr(entryAddr+uintptr(MapEntryOffVal), value)
		return hdr
	}
	// Not found — append
	if mlen >= mcap {
		oldData := data
		oldCap := mcap
		newCap := mcap * 2
		if newCap < 8 {
			newCap = 8
		}
		newData := mapAllocEntryBlock(newCap)
		if mlen > 0 {
			Memcopy(newData, data, mlen*MapEntrySize)
		}
		WritePtr(hdr, newData)
		WritePtr(hdr+uintptr(SliceOffCap), uintptr(newCap))
		data = newData
		mcap = newCap
		mapFreeEntryBlock(oldData, oldCap)
	}
	entryAddr := data + uintptr(mlen*MapEntrySize)
	WritePtr(entryAddr, key)
	WritePtr(entryAddr+uintptr(MapEntryOffVal), value)
	newLen := mlen + 1
	WritePtr(hdr+uintptr(SliceOffLen), uintptr(newLen))
	if newLen >= mapHashMinLen {
		mapIntInitHashState(hdr, mcap)
		mapIntRebuildHashSlots(hdr, data, newLen)
	}
	return hdr
}

// MapDelete removes a key from the map.
func MapDelete(hdr uintptr, key uintptr) {
	if hdr == 0 {
		return
	}
	mlen := int(ReadPtr(hdr + uintptr(SliceOffLen)))
	keyKind := int(ReadPtr(hdr + uintptr(SliceOffEsz)))
	data := ReadPtr(hdr)
	slots := ReadPtr(hdr + uintptr(mapHdrOffHashSlots))
	hashCap := int(ReadPtr(hdr + uintptr(mapHdrOffHashCap)))
	if keyKind == 1 {
		hashes := ReadPtr(hdr + uintptr(mapHdrOffHashes))
		if slots != 0 && hashes != 0 && hashCap > 0 {
			keyHash := mapStringHashKey(key)
			idx, _, found := mapStringFindIndex(data, mlen, slots, hashes, hashCap, key, keyHash)
			if !found {
				return
			}
			lastIdx := mlen - 1
			if idx < lastIdx {
				entryAddr := data + uintptr(idx*MapEntrySize)
				lastAddr := data + uintptr(lastIdx*MapEntrySize)
				WritePtr(entryAddr, ReadPtr(lastAddr))
				WritePtr(entryAddr+uintptr(MapEntryOffVal), ReadPtr(lastAddr+uintptr(MapEntryOffVal)))
				WritePtr(hashes+uintptr(idx*PtrSize), ReadPtr(hashes+uintptr(lastIdx*PtrSize)))
			}
			WritePtr(hashes+uintptr(lastIdx*PtrSize), 0)
			WritePtr(hdr+uintptr(SliceOffLen), uintptr(lastIdx))
			mapStringRebuildHashSlots(hdr, data, lastIdx)
			return
		}
	} else if slots != 0 && hashCap > 0 {
		keyHash := mapIntHashKey(key)
		idx, _, found := mapIntFindIndex(data, mlen, slots, hashCap, key, keyHash)
		if !found {
			return
		}
		lastIdx := mlen - 1
		if idx < lastIdx {
			entryAddr := data + uintptr(idx*MapEntrySize)
			lastAddr := data + uintptr(lastIdx*MapEntrySize)
			WritePtr(entryAddr, ReadPtr(lastAddr))
			WritePtr(entryAddr+uintptr(MapEntryOffVal), ReadPtr(lastAddr+uintptr(MapEntryOffVal)))
		}
		WritePtr(hdr+uintptr(SliceOffLen), uintptr(lastIdx))
		mapIntRebuildHashSlots(hdr, data, lastIdx)
		return
	}
	i := 0
	for i < mlen {
		entryAddr := data + uintptr(i*MapEntrySize)
		entryKey := ReadPtr(entryAddr)
		found := false
		if keyKind == 1 {
			found = mapStrEqual(entryKey, key)
		} else {
			found = entryKey == key
		}
		if found {
			lastIdx := mlen - 1
			if i < lastIdx {
				lastAddr := data + uintptr(lastIdx*MapEntrySize)
				WritePtr(entryAddr, ReadPtr(lastAddr))
				WritePtr(entryAddr+uintptr(MapEntryOffVal), ReadPtr(lastAddr+uintptr(MapEntryOffVal)))
			}
			WritePtr(hdr+uintptr(SliceOffLen), uintptr(lastIdx))
			return
		}
		i = i + 1
	}
}

// MapLen returns the number of entries in the map.
func MapLen(hdr uintptr) int {
	if hdr == 0 {
		return 0
	}
	return int(ReadPtr(hdr + uintptr(SliceOffLen)))
}

// MapEntryKey returns the key at index i.
func MapEntryKey(hdr uintptr, i int) uintptr {
	if hdr == 0 {
		return 0
	}
	data := ReadPtr(hdr)
	return ReadPtr(data + uintptr(i*MapEntrySize))
}

// MapEntryValue returns the value at index i.
func MapEntryValue(hdr uintptr, i int) uintptr {
	if hdr == 0 {
		return 0
	}
	data := ReadPtr(hdr)
	return ReadPtr(data + uintptr(i*MapEntrySize) + uintptr(MapEntryOffVal))
}

// === String comparison ===

// StringLess returns true if a < b lexicographically.
func StringLess(a string, b string) bool {
	alen := len(a)
	blen := len(b)
	n := alen
	if blen < n {
		n = blen
	}
	aptr := Stringptr(a)
	bptr := Stringptr(b)
	if n > 0 {
		if aptr == 0 {
			return alen < blen
		}
		if bptr == 0 {
			return alen < blen
		}
	}
	i := 0
	for i < n {
		ab := Makeslice(aptr+uintptr(i), 1, 1)
		bb := Makeslice(bptr+uintptr(i), 1, 1)
		if ab[0] < bb[0] {
			return true
		}
		if ab[0] > bb[0] {
			return false
		}
		i = i + 1
	}
	return alen < blen
}
