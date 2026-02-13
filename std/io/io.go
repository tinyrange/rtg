package io

type Writer interface {
	Write(p []byte) (n int, err error)
}

type Reader interface {
	Read(p []byte) (n int, err error)
}

func Copy(dst Writer, src Reader) (int64, error) {
	var written int64
	buf := make([]byte, 4096)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			nw, werr := dst.Write(buf[0:n])
			written = written + int64(nw)
			if werr != nil {
				return written, werr
			}
		}
		if err != nil {
			return written, err
		}
		if n == 0 {
			break
		}
	}
	return written, nil
}
