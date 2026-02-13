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

// ReadPtr reads 8 bytes at the given address and returns them as uintptr.
//
//rtg:internal ReadPtr
func ReadPtr(addr uintptr) uintptr

// WritePtr writes an 8-byte value to the given address.
//
//rtg:internal WritePtr
func WritePtr(addr uintptr, val uintptr)

// WriteByte writes a single byte to the given address.
//
//rtg:internal WriteByte
func WriteByte(addr uintptr, val byte)

// === Memory allocator ===

func runtimePanic(msg string) {
	if len(msg) > 0 {
		SysWrite(2, Stringptr(msg), uintptr(len(msg)))
	}
	SysWrite(2, Stringptr("\n"), 1)
	SysExit(2)
}

var heapPtr uintptr
var heapEnd uintptr

// Alloc allocates size bytes via mmap, using a bump allocator over
// a 1MB region to avoid per-allocation syscall overhead.
func Alloc(size int) uintptr {
	// Round up to 8-byte alignment
	size = (size + 7) / 8 * 8

	if heapPtr == 0 || heapPtr+uintptr(size) > heapEnd {
		chunk := 1048576
		if size > chunk {
			chunk = size
		}
		// mmap(0, chunk, PROT_READ|PROT_WRITE, MAP_PRIVATE|MAP_ANONYMOUS, -1, 0)
		ptr, _, _ := SysMmap(0, uintptr(chunk), 3, MmapAnonFlags, 0, 0)
		heapPtr = ptr
		heapEnd = ptr + uintptr(chunk)
	}

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
	d := Makeslice(dst, n, n)
	s := Makeslice(src, n, n)
	i := 0
	for i < n {
		d[i] = s[i]
		i++
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
	b := Makeslice(ptr, n, n)
	i := 0
	for i < n {
		b[i] = 0
		i++
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

// ByteToString converts a single byte into a 1-character string.
func ByteToString(b byte) string {
	ptr := Alloc(1)
	buf := Makeslice(ptr, 1, 1)
	buf[0] = b
	return Makestring(ptr, 1)
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
