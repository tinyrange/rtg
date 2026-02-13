//go:build c

package os

import "runtime"

type FileMode int

type File struct {
	fd int
}

var Stdout *File = &File{fd: 1}
var Stderr *File = &File{fd: 2}
var Stdin *File = &File{fd: 0}

var Args []string

func (f *File) Write(p []byte) (n int, err error) {
	wrote, _, errn := runtime.Syscall(SYS_WRITE, uintptr(f.fd), runtime.Sliceptr(p), uintptr(len(p)), 0, 0, 0)
	if errn != 0 {
		return int(wrote), Errno(errn)
	}
	return int(wrote), nil
}

func (f *File) Read(p []byte) (int, error) {
	n, _, errn := runtime.Syscall(SYS_READ, uintptr(f.fd), runtime.Sliceptr(p), uintptr(len(p)), 0, 0, 0)
	if errn != 0 {
		return int(n), Errno(errn)
	}
	return int(n), nil
}

func (f *File) Close() error {
	_, _, errn := runtime.Syscall(SYS_CLOSE, uintptr(f.fd), 0, 0, 0, 0, 0)
	if errn != 0 {
		return Errno(errn)
	}
	return nil
}

func (f *File) Fd() int {
	return f.fd
}

func NewFile(fd int) *File {
	return &File{fd: fd}
}

func Write(f *File, p []byte) (int, error) {
	wrote, _, errn := runtime.Syscall(SYS_WRITE, uintptr(f.fd), runtime.Sliceptr(p), uintptr(len(p)), 0, 0, 0)
	if errn != 0 {
		return int(wrote), Errno(errn)
	}
	return int(wrote), nil
}

func Exit(code int) {
	runtime.Syscall(SYS_EXIT_GROUP, uintptr(code), 0, 0, 0, 0, 0)
}

func makeCString(s string) []byte {
	buf := make([]byte, len(s)+1)
	i := 0
	for i < len(s) {
		buf[i] = s[i]
		i++
	}
	return buf
}

func readCString(ptr uintptr) string {
	if ptr == 0 {
		return ""
	}
	n := 0
	for {
		b := runtime.ReadPtr(ptr + uintptr(n))
		if b&0xff == 0 {
			break
		}
		n++
	}
	if n == 0 {
		return ""
	}
	buf := make([]byte, n)
	i := 0
	for i < n {
		b := runtime.ReadPtr(ptr + uintptr(i))
		buf[i] = byte(b & 0xff)
		i++
	}
	return string(buf)
}

func Open(name string) (*File, error) {
	return OpenFile(name, int(O_RDONLY), FileMode(0))
}

func OpenFile(name string, flag int, perm FileMode) (*File, error) {
	buf := makeCString(name)
	fd, _, errn := runtime.Syscall(SYS_OPEN, runtime.Sliceptr(buf), uintptr(flag), uintptr(perm), 0, 0, 0)
	if errn != 0 {
		return nil, Errno(errn)
	}
	return &File{fd: int(fd)}, nil
}

func ReadFile(filename string) ([]byte, error) {
	buf := makeCString(filename)

	fd, _, errn := runtime.Syscall(SYS_OPEN, runtime.Sliceptr(buf), uintptr(O_RDONLY), 0, 0, 0, 0)
	if errn != 0 {
		return nil, Errno(errn)
	}

	var data []byte
	chunk := make([]byte, 4096)
	for {
		n, _, errn := runtime.Syscall(SYS_READ, fd, runtime.Sliceptr(chunk), 4096, 0, 0, 0)
		if errn != 0 {
			runtime.Syscall(SYS_CLOSE, fd, 0, 0, 0, 0, 0)
			return nil, Errno(errn)
		}
		if n == 0 {
			break
		}
		data = append(data, chunk[0:int(n)]...)
	}

	runtime.Syscall(SYS_CLOSE, fd, 0, 0, 0, 0, 0)
	return data, nil
}

func WriteFile(name string, data []byte, perm int) error {
	buf := makeCString(name)
	flags := O_WRONLY + O_CREAT + O_TRUNC
	fd, _, errn := runtime.Syscall(SYS_OPEN, runtime.Sliceptr(buf), uintptr(flags), uintptr(perm), 0, 0, 0)
	if errn != 0 {
		return Errno(errn)
	}
	for len(data) > 0 {
		n, _, errn := runtime.Syscall(SYS_WRITE, fd, runtime.Sliceptr(data), uintptr(len(data)), 0, 0, 0)
		if errn != 0 {
			runtime.Syscall(SYS_CLOSE, fd, 0, 0, 0, 0, 0)
			return Errno(errn)
		}
		data = data[int(n):len(data)]
	}
	runtime.Syscall(SYS_CLOSE, fd, 0, 0, 0, 0, 0)
	return nil
}

func MkdirAll(path string, perm FileMode) error {
	buf := makeCString(path)
	_, _, errn := runtime.Syscall(SYS_MKDIR, runtime.Sliceptr(buf), uintptr(perm), 0, 0, 0, 0)
	if errn == 0 || errn == 17 {
		return nil
	}

	i := 0
	if len(path) > 0 && path[0] == '/' {
		i = 1
	}
	for i < len(path) {
		j := i
		for j < len(path) && path[j] != '/' {
			j++
		}
		prefix := path[0:j]
		pbuf := makeCString(prefix)
		_, _, errn = runtime.Syscall(SYS_MKDIR, runtime.Sliceptr(pbuf), uintptr(perm), 0, 0, 0, 0)
		if errn != 0 && errn != 17 {
			return Errno(errn)
		}
		i = j + 1
	}
	return nil
}

func RemoveAll(path string) error {
	buf := makeCString(path)
	_, _, errn := runtime.Syscall(SYS_UNLINK, runtime.Sliceptr(buf), 0, 0, 0, 0, 0)
	if errn == 0 {
		return nil
	}

	entries, err := ListDir(path)
	if err != nil {
		return nil
	}

	i := 0
	for i < len(entries) {
		child := path + "/" + entries[i]
		rerr := RemoveAll(child)
		if rerr != nil {
			return rerr
		}
		i++
	}

	buf = makeCString(path)
	_, _, errn = runtime.Syscall(SYS_RMDIR, runtime.Sliceptr(buf), 0, 0, 0, 0, 0)
	if errn != 0 {
		return Errno(errn)
	}
	return nil
}

func Getenv(key string) string {
	buf := makeCString(key)
	ptr, _, errn := runtime.Syscall(SYS_GETENV, runtime.Sliceptr(buf), 0, 0, 0, 0, 0)
	if errn != 0 || ptr == 0 {
		return ""
	}
	return readCString(ptr)
}

func Environ() []string {
	return nil
}

func ListDir(dirname string) ([]string, error) {
	buf := makeCString(dirname)
	handle, _, errn := runtime.Syscall(SYS_OPENDIR, runtime.Sliceptr(buf), 0, 0, 0, 0, 0)
	if errn != 0 {
		return nil, Errno(errn)
	}

	var names []string
	nameBuf := make([]byte, 512)
	for {
		n, _, errn := runtime.Syscall(SYS_READDIR, handle, runtime.Sliceptr(nameBuf), uintptr(len(nameBuf)), 0, 0, 0)
		if errn != 0 || n == 0 {
			break
		}
		name := string(nameBuf[0:int(n)])
		if name != "." && name != ".." {
			names = append(names, name)
		}
	}

	runtime.Syscall(SYS_CLOSEDIR, handle, 0, 0, 0, 0, 0)
	return names, nil
}

type DirEntry struct {
	name  string
	isDir bool
}

func (d DirEntry) Name() string {
	return d.name
}

func (d DirEntry) IsDir() bool {
	return d.isDir
}

func ReadDir(dirname string) ([]DirEntry, error) {
	buf := makeCString(dirname)
	handle, _, errn := runtime.Syscall(SYS_OPENDIR, runtime.Sliceptr(buf), 0, 0, 0, 0, 0)
	if errn != 0 {
		return nil, Errno(errn)
	}

	var entries []DirEntry
	nameBuf := make([]byte, 512)
	isDirBuf := make([]byte, 1)
	for {
		n, _, errn := runtime.Syscall(SYS_READDIR, handle, runtime.Sliceptr(nameBuf), uintptr(len(nameBuf)), runtime.Sliceptr(isDirBuf), 0, 0)
		if errn != 0 || n == 0 {
			break
		}
		name := string(nameBuf[0:int(n)])
		if name != "." && name != ".." {
			entries = append(entries, DirEntry{name: name, isDir: isDirBuf[0] != 0})
		}
	}

	runtime.Syscall(SYS_CLOSEDIR, handle, 0, 0, 0, 0, 0)
	return entries, nil
}

func Getwd() (string, error) {
	buf := make([]byte, 4096)
	n, _, errn := runtime.Syscall(SYS_GETCWD, runtime.Sliceptr(buf), 4096, 0, 0, 0, 0)
	if errn != 0 {
		return "", Errno(errn)
	}
	return string(buf[0:int(n)]), nil
}

func Chmod(name string, mode int) error {
	buf := makeCString(name)
	_, _, errn := runtime.Syscall(SYS_CHMOD, runtime.Sliceptr(buf), uintptr(mode), 0, 0, 0, 0)
	if errn != 0 {
		return Errno(errn)
	}
	return nil
}

func Stat(name string) error {
	buf := makeCString(name)
	_, _, errn := runtime.Syscall(SYS_STAT, runtime.Sliceptr(buf), 0, 0, 0, 0, 0)
	if errn != 0 {
		return Errno(errn)
	}
	return nil
}

func init() {
	argc, _, _ := runtime.Syscall(SYS_GETARGC, 0, 0, 0, 0, 0, 0)
	i := 0
	for i < int(argc) {
		ptr, _, _ := runtime.Syscall(SYS_GETARGV, uintptr(i), 0, 0, 0, 0, 0)
		Args = append(Args, readCString(ptr))
		i++
	}
}
