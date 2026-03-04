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

// Alloc allocates size bytes via mmap, using a bump allocator over
// growth-sized regions to avoid per-allocation syscall overhead while
// keeping startup memory lower.
func Alloc(size int) uintptr {
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
		ptr, _, err := SysMmap(0, uintptr(chunk), 3, MmapAnonFlags, ^uintptr(0), 0)
		if err != 0 || ptr == 0 {
			runtimePanic("out of memory")
		}
		mmapChunk = chunk
		heapPtr = ptr
		heapEnd = ptr + uintptr(chunk)
	}
	arenaRecordAlloc(size, mmapChunk)

	result := heapPtr
	heapPtr = heapPtr + uintptr(size)
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
	if n < 0 {
		neg = true
		n = 0 - n
	}
	// Build digits in reverse
	buf := make([]byte, 20)
	i := 19
	for n > 0 {
		buf[i] = byte(n%10) + '0'
		n = n / 10
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
	total := alen + blen
	if total == 0 {
		return Makestring(0, 0)
	}
	ptr := Alloc(total)
	if alen > 0 {
		Memcopy(ptr, Stringptr(a), alen)
	}
	if blen > 0 {
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

// SliceMake allocates a new slice with the given length and element size.
func SliceMake(length int, elemSize int) uintptr {
	byteSize := length * elemSize
	var dataPtr uintptr
	if byteSize > 0 {
		dataPtr = Alloc(byteSize)
		Memzero(dataPtr, byteSize)
	}
	header := Alloc(SliceHdrSize)
	WritePtr(header, dataPtr)
	WritePtr(header+uintptr(SliceOffLen), uintptr(length))
	WritePtr(header+uintptr(SliceOffCap), uintptr(length))
	WritePtr(header+uintptr(SliceOffEsz), uintptr(elemSize))
	return header
}

func SliceMakeCap(length int, capacity int, elemSize int) uintptr {
	byteSize := capacity * elemSize
	var dataPtr uintptr
	if byteSize > 0 {
		dataPtr = Alloc(byteSize)
		Memzero(dataPtr, byteSize)
	}
	header := Alloc(SliceHdrSize)
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
		hdr = Alloc(SliceHdrSize)
		dataPtr := Alloc(8 * elemSize)
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
		dst = Alloc(SliceHdrSize)
		elemSize := int(ReadPtr(src + uintptr(SliceOffEsz)))
		dataPtr := Alloc(srcLen * elemSize)
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
	ab := Makeslice(aptr, alen, alen)
	bb := Makeslice(bptr, blen, blen)
	j := 0
	for j < alen {
		if ab[j] != bb[j] {
			return false
		}
		j = j + 1
	}
	return true
}

// MapMake allocates an empty map header. keyKind: 0=int, 1=string.
func MapMake(keyKind int) uintptr {
	hdr := Alloc(SliceHdrSize)
	data := Alloc(8 * MapEntrySize)
	WritePtr(hdr, data)
	WritePtr(hdr+uintptr(SliceOffLen), 0)
	WritePtr(hdr+uintptr(SliceOffCap), 8)
	WritePtr(hdr+uintptr(SliceOffEsz), uintptr(keyKind))
	return hdr
}

// MapGet looks up a key in the map. Returns (value, found).
func MapGet(hdr uintptr, key uintptr) (uintptr, bool) {
	if hdr == 0 {
		return 0, false
	}
	mlen := int(ReadPtr(hdr + uintptr(SliceOffLen)))
	keyKind := int(ReadPtr(hdr + uintptr(SliceOffEsz)))
	data := ReadPtr(hdr)
	i := 0
	for i < mlen {
		entryAddr := data + uintptr(i*MapEntrySize)
		entryKey := ReadPtr(entryAddr)
		if keyKind == 1 {
			if mapStrEqual(entryKey, key) {
				return ReadPtr(entryAddr + uintptr(MapEntryOffVal)), true
			}
		} else {
			if entryKey == key {
				return ReadPtr(entryAddr + uintptr(MapEntryOffVal)), true
			}
		}
		i = i + 1
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
	// Search for existing key
	i := 0
	for i < mlen {
		entryAddr := data + uintptr(i*MapEntrySize)
		entryKey := ReadPtr(entryAddr)
		if keyKind == 1 {
			if mapStrEqual(entryKey, key) {
				WritePtr(entryAddr+uintptr(MapEntryOffVal), value)
				return hdr
			}
		} else {
			if entryKey == key {
				WritePtr(entryAddr+uintptr(MapEntryOffVal), value)
				return hdr
			}
		}
		i = i + 1
	}
	// Not found — append
	mcap := int(ReadPtr(hdr + uintptr(SliceOffCap)))
	if mlen >= mcap {
		newCap := mcap * 2
		if newCap < 8 {
			newCap = 8
		}
		newData := Alloc(newCap * MapEntrySize)
		if mlen > 0 {
			Memcopy(newData, data, mlen*MapEntrySize)
		}
		WritePtr(hdr, newData)
		WritePtr(hdr+uintptr(SliceOffCap), uintptr(newCap))
		data = newData
	}
	entryAddr := data + uintptr(mlen*MapEntrySize)
	WritePtr(entryAddr, key)
	WritePtr(entryAddr+uintptr(MapEntryOffVal), value)
	WritePtr(hdr+uintptr(SliceOffLen), uintptr(mlen+1))
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
