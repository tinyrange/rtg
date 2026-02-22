package main

//rtg:internal Syscall
func Syscall(num, a0, a1, a2, a3, a4, a5 uintptr) (r1 uintptr, r2 uintptr, err uintptr)

//rtg:internal Sliceptr
func Sliceptr(b []byte) uintptr

//rtg:internal Stringptr
func Stringptr(s string) uintptr

type FileWriter struct {
	fd uintptr
}

func (w *FileWriter) OpenCreate(path []byte) {
	fd, _, _ := Syscall(5, Sliceptr(path), 1, 0, 0, 0, 0)
	w.fd = fd
}

func (w *FileWriter) WriteString(s string) {
	Syscall(4, w.fd, Stringptr(s), uintptr(len(s)), 0, 0, 0)
}

func (w *FileWriter) Close() {
	Syscall(6, w.fd, 0, 0, 0, 0, 0)
}

func main() {
	path := [10]byte{'O', 'U', 'T', '.', 'T', 'X', 'T', 0, 0, 0}
	w := FileWriter{}
	w.OpenCreate(path[:])
	w.WriteString("hello")
	w.Close()

	fd, _, _ := Syscall(5, Sliceptr(path[:]), 0, 0, 0, 0, 0)
	buf := [32]byte{}
	n, _, _ := Syscall(3, fd, Sliceptr(buf[:]), 32, 0, 0, 0)
	Syscall(6, fd, 0, 0, 0, 0, 0)

	if n > 0 {
		print("PASS")
	} else {
		print("FAIL")
	}
}
