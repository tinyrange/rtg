//go:build wasi

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
	// WASI doesn't have /proc/self/environ, but we still use the same
	// syscall interface. The WASM backend handles getcwd specially.
	// For environ, we'll return empty for now.
	// TODO: use WASI environ_get
	return ""
}

func Environ() []string {
	return nil
}

func ListDir(dirname string) ([]string, error) {
	f, err := OpenFile(dirname, int(O_RDONLY+O_DIRECTORY), FileMode(0))
	if err != nil {
		return nil, err
	}
	// WASI fd_readdir returns entries differently from Linux getdents64
	// For now, use the same syscall interface and the backend translates
	buf := make([]byte, 4096)
	n, _, errn := runtime.Syscall(SYS_GETDENTS64, uintptr(f.fd), runtime.Sliceptr(buf), 4096, 0, 0, 0)
	if errn != 0 {
		f.Close()
		return nil, Errno(errn)
	}
	f.Close()

	// Parse WASI fd_readdir entries:
	// Each entry: d_next(8) + d_ino(8) + d_namlen(4) + d_type(1) + name(d_namlen)
	var names []string
	offset := 0
	for offset+24 < int(n) {
		// d_namlen at offset+16 (4 bytes LE)
		namlen := int(buf[offset+16]) + int(buf[offset+17])*256
		// d_type at offset+20
		nameStart := offset + 24
		nameEnd := nameStart + namlen
		if nameEnd > int(n) {
			break
		}
		name := string(buf[nameStart:nameEnd])
		if name != "." && name != ".." {
			names = append(names, name)
		}
		offset = nameEnd
	}
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
	f, err := OpenFile(dirname, int(O_RDONLY+O_DIRECTORY), FileMode(0))
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 4096)
	n, _, errn := runtime.Syscall(SYS_GETDENTS64, uintptr(f.fd), runtime.Sliceptr(buf), 4096, 0, 0, 0)
	if errn != 0 {
		f.Close()
		return nil, Errno(errn)
	}
	f.Close()

	var entries []DirEntry
	offset := 0
	for offset+24 < int(n) {
		namlen := int(buf[offset+16]) + int(buf[offset+17])*256
		dtype := buf[offset+20]
		nameStart := offset + 24
		nameEnd := nameStart + namlen
		if nameEnd > int(n) {
			break
		}
		name := string(buf[nameStart:nameEnd])
		if name != "." && name != ".." {
			entries = append(entries, DirEntry{name: name, isDir: dtype == 3})
		}
		offset = nameEnd
	}
	return entries, nil
}

func Getwd() (string, error) {
	return ".", nil
}

func Chmod(name string, mode int) error {
	return nil
}

func Stat(name string) error {
	// Try to open and close the file to check if it exists
	f, err := Open(name)
	if err != nil {
		return err
	}
	f.Close()
	return nil
}

func init() {
	// WASI: args are retrieved via args_sizes_get + args_get
	// These are called via the WASI imports. For now, the backend
	// handles this. We use a simpler approach: the init function
	// for os doesn't need to populate Args here because the _start
	// in the WASM backend will handle it.
	// Actually, the os.init needs to populate Args somehow.
	// Since we can't call WASI directly from Go code (it goes through
	// the Syscall intrinsic), we'll leave Args empty for now and
	// populate it through a special init mechanism.
	//
	// For self-hosting, Args is populated by reading from a WASI-provided
	// mechanism. Let's use the same syscall approach but we need the
	// backend to handle a special "get args" syscall.
	//
	// Simplest approach: don't populate here; instead, have the WASM
	// _start function call a special init that populates Args using WASI.
}
