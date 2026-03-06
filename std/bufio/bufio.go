package bufio

import "io"

const defaultBufSize = 4096

type Reader struct {
	rd  io.Reader
	buf []byte
	r   int
	w   int
	err error
}

func NewReader(rd io.Reader) *Reader {
	return NewReaderSize(rd, defaultBufSize)
}

func NewReaderSize(rd io.Reader, size int) *Reader {
	if size < 16 {
		size = 16
	}
	return &Reader{
		rd:  rd,
		buf: make([]byte, size),
	}
}

func (b *Reader) Buffered() int {
	return b.w - b.r
}

func (b *Reader) fill() error {
	if b.r > 0 {
		if b.r < b.w {
			copy(b.buf, b.buf[b.r:b.w])
			b.w = b.w - b.r
			b.r = 0
		} else {
			b.r = 0
			b.w = 0
		}
	}
	if b.err != nil {
		return b.err
	}
	r := b.rd
	n, err := r.Read(b.buf[b.w:len(b.buf)])
	if n > 0 {
		b.w = b.w + n
	}
	if err != nil {
		b.err = err
	} else if n == 0 {
		b.err = io.EOF
	}
	return b.err
}

func (b *Reader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	total := 0
	for len(p) > 0 {
		if b.r < b.w {
			n := copy(p, b.buf[b.r:b.w])
			b.r = b.r + n
			p = p[n:]
			total = total + n
			continue
		}
		if b.err != nil {
			if total > 0 {
				return total, nil
			}
			return 0, b.err
		}
		if total == 0 && len(p) >= len(b.buf) {
			r := b.rd
			n, err := r.Read(p)
			if n > 0 {
				return n, nil
			}
			if err != nil {
				return 0, err
			}
			return 0, io.EOF
		}
		b.fill()
	}
	return total, nil
}

func (b *Reader) ReadByte() (byte, error) {
	if b.r >= b.w {
		if b.err != nil {
			return 0, b.err
		}
		b.fill()
		if b.r >= b.w {
			if b.err != nil {
				return 0, b.err
			}
			return 0, io.EOF
		}
	}
	ch := b.buf[b.r]
	b.r++
	return ch, nil
}

func (b *Reader) ReadBytes(delim byte) ([]byte, error) {
	var out []byte
	for {
		ch, err := b.ReadByte()
		if err != nil {
			if len(out) > 0 {
				return out, err
			}
			return nil, err
		}
		out = append(out, ch)
		if ch == delim {
			return out, nil
		}
	}
}

func (b *Reader) ReadString(delim byte) (string, error) {
	out, err := b.ReadBytes(delim)
	if err != nil && len(out) == 0 {
		return "", err
	}
	return string(out), err
}

type Writer struct {
	wr  io.Writer
	buf []byte
	n   int
	err error
}

func NewWriter(w io.Writer) *Writer {
	return NewWriterSize(w, defaultBufSize)
}

func NewWriterSize(w io.Writer, size int) *Writer {
	if size < 16 {
		size = 16
	}
	return &Writer{
		wr:  w,
		buf: make([]byte, size),
	}
}

func (b *Writer) Buffered() int {
	return b.n
}

func (b *Writer) Flush() error {
	if b.err != nil {
		return b.err
	}
	wrote := 0
	w := b.wr
	for wrote < b.n {
		nw, err := w.Write(b.buf[wrote:b.n])
		wrote = wrote + nw
		if err != nil {
			if wrote < b.n {
				copy(b.buf, b.buf[wrote:b.n])
				b.n = b.n - wrote
			} else {
				b.n = 0
			}
			b.err = err
			return err
		}
		if nw == 0 {
			b.err = io.EOF
			return b.err
		}
	}
	b.n = 0
	return nil
}

func (b *Writer) Write(p []byte) (int, error) {
	if b.err != nil {
		return 0, b.err
	}
	total := 0
	for len(p) > 0 {
		if b.n == 0 && len(p) >= len(b.buf) {
			w := b.wr
			nw, err := w.Write(p)
			total = total + nw
			p = p[nw:]
			if err != nil {
				b.err = err
				return total, err
			}
			if nw == 0 {
				b.err = io.EOF
				return total, b.err
			}
			continue
		}
		space := len(b.buf) - b.n
		if space == 0 {
			if err := b.Flush(); err != nil {
				return total, err
			}
			space = len(b.buf) - b.n
		}
		n := space
		if len(p) < n {
			n = len(p)
		}
		copy(b.buf[b.n:b.n+n], p[:n])
		b.n = b.n + n
		total = total + n
		p = p[n:]
	}
	return total, nil
}

func (b *Writer) WriteByte(c byte) error {
	if b.err != nil {
		return b.err
	}
	if b.n >= len(b.buf) {
		if err := b.Flush(); err != nil {
			return err
		}
	}
	b.buf[b.n] = c
	b.n++
	return nil
}

func (b *Writer) WriteString(s string) (int, error) {
	return b.Write([]byte(s))
}
