package bytes

type eofError struct{}

func (e eofError) Error() string {
	return "EOF"
}

func Equal(a []byte, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	i := 0
	for i < len(a) {
		if a[i] != b[i] {
			return false
		}
		i++
	}
	return true
}

func Compare(a []byte, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	i := 0
	for i < n {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
		i++
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return 0
}

func Index(s []byte, sep []byte) int {
	if len(sep) == 0 {
		return 0
	}
	if len(sep) > len(s) {
		return -1
	}
	i := 0
	for i <= len(s)-len(sep) {
		match := true
		j := 0
		for j < len(sep) {
			if s[i+j] != sep[j] {
				match = false
				break
			}
			j++
		}
		if match {
			return i
		}
		i++
	}
	return -1
}

func Contains(s []byte, sep []byte) bool {
	return Index(s, sep) >= 0
}

func HasPrefix(s []byte, prefix []byte) bool {
	if len(prefix) > len(s) {
		return false
	}
	i := 0
	for i < len(prefix) {
		if s[i] != prefix[i] {
			return false
		}
		i++
	}
	return true
}

func HasSuffix(s []byte, suffix []byte) bool {
	if len(suffix) > len(s) {
		return false
	}
	start := len(s) - len(suffix)
	i := 0
	for i < len(suffix) {
		if s[start+i] != suffix[i] {
			return false
		}
		i++
	}
	return true
}

type Buffer struct {
	buf []byte
	off int
}

func NewBuffer(buf []byte) *Buffer {
	return &Buffer{buf: buf}
}

func NewBufferString(s string) *Buffer {
	return &Buffer{buf: []byte(s)}
}

func (b *Buffer) Bytes() []byte {
	if b.off >= len(b.buf) {
		return nil
	}
	return b.buf[b.off:]
}

func (b *Buffer) String() string {
	return string(b.Bytes())
}

func (b *Buffer) Len() int {
	if b.off >= len(b.buf) {
		return 0
	}
	return len(b.buf) - b.off
}

func (b *Buffer) Cap() int {
	return cap(b.buf)
}

func (b *Buffer) Reset() {
	b.buf = nil
	b.off = 0
}

func (b *Buffer) grow(n int) {
	if n <= 0 {
		return
	}
	if b.off == len(b.buf) {
		b.Reset()
	}
	need := len(b.buf) + n
	if need <= cap(b.buf) {
		return
	}
	newCap := cap(b.buf) * 2
	if newCap < need {
		newCap = need
	}
	if newCap == 0 {
		newCap = n
	}
	newBuf := make([]byte, len(b.buf)-b.off, newCap)
	copy(newBuf, b.buf[b.off:])
	b.buf = newBuf
	b.off = 0
}

func (b *Buffer) Write(p []byte) (int, error) {
	b.grow(len(p))
	if b.off > 0 && b.off < len(b.buf) {
		newBuf := make([]byte, len(b.buf)-b.off, cap(b.buf))
		copy(newBuf, b.buf[b.off:])
		b.buf = newBuf
		b.off = 0
	}
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *Buffer) WriteString(s string) (int, error) {
	return b.Write([]byte(s))
}

func (b *Buffer) WriteByte(c byte) error {
	_, err := b.Write([]byte{c})
	return err
}

func (b *Buffer) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if b.off >= len(b.buf) {
		return 0, eofError{}
	}
	n := copy(p, b.buf[b.off:])
	b.off += n
	if b.off >= len(b.buf) {
		b.Reset()
	}
	return n, nil
}

func (b *Buffer) ReadByte() (byte, error) {
	if b.off >= len(b.buf) {
		return 0, eofError{}
	}
	c := b.buf[b.off]
	b.off++
	if b.off >= len(b.buf) {
		b.Reset()
	}
	return c, nil
}

func (b *Buffer) Next(n int) []byte {
	if n <= 0 {
		return nil
	}
	avail := b.Len()
	if n > avail {
		n = avail
	}
	out := b.buf[b.off : b.off+n]
	b.off += n
	if b.off >= len(b.buf) {
		b.Reset()
	}
	return out
}
