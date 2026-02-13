//go:build linux

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
	wrote, _, errn := runtime.SysWrite(uintptr(f.fd), runtime.Sliceptr(p), uintptr(len(p)))
	if errn != 0 {
		return int(wrote), Errno(errn)
	}
	return int(wrote), nil
}

func (f *File) Read(p []byte) (int, error) {
	n, _, errn := runtime.SysRead(uintptr(f.fd), runtime.Sliceptr(p), uintptr(len(p)))
	if errn != 0 {
		return int(n), Errno(errn)
	}
	return int(n), nil
}

func (f *File) Close() error {
	_, _, errn := runtime.SysClose(uintptr(f.fd))
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
	wrote, _, errn := runtime.SysWrite(uintptr(f.fd), runtime.Sliceptr(p), uintptr(len(p)))
	if errn != 0 {
		return int(wrote), Errno(errn)
	}
	return int(wrote), nil
}

func Exit(code int) {
	runtime.SysExit(uintptr(code))
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
	fd, _, errn := runtime.SysOpen(runtime.Sliceptr(buf), uintptr(flag), uintptr(perm))
	if errn != 0 {
		return nil, Errno(errn)
	}
	return &File{fd: int(fd)}, nil
}

func ReadFile(filename string) ([]byte, error) {
	buf := makeCString(filename)

	fd, _, errn := runtime.SysOpen(runtime.Sliceptr(buf), uintptr(O_RDONLY), 0)
	if errn != 0 {
		return nil, Errno(errn)
	}

	var data []byte
	chunk := make([]byte, 4096)
	for {
		n, _, errn := runtime.SysRead(fd, runtime.Sliceptr(chunk), 4096)
		if errn != 0 {
			runtime.SysClose(fd)
			return nil, Errno(errn)
		}
		if n == 0 {
			break
		}
		data = append(data, chunk[0:int(n)]...)
	}

	runtime.SysClose(fd)
	return data, nil
}

func WriteFile(name string, data []byte, perm int) error {
	buf := makeCString(name)
	flags := O_WRONLY + O_CREAT + O_TRUNC
	fd, _, errn := runtime.SysOpen(runtime.Sliceptr(buf), uintptr(flags), uintptr(perm))
	if errn != 0 {
		return Errno(errn)
	}
	for len(data) > 0 {
		n, _, errn := runtime.SysWrite(fd, runtime.Sliceptr(data), uintptr(len(data)))
		if errn != 0 {
			runtime.SysClose(fd)
			return Errno(errn)
		}
		data = data[int(n):len(data)]
	}
	runtime.SysClose(fd)
	return nil
}

func MkdirAll(path string, perm FileMode) error {
	// Try mkdir first - if it works, we're done
	buf := makeCString(path)
	_, _, errn := runtime.SysMkdir(runtime.Sliceptr(buf), uintptr(perm))
	if errn == 0 || errn == 17 {
		// 17 = EEXIST
		return nil
	}

	// Walk the path creating directories as needed
	i := 0
	if len(path) > 0 && path[0] == '/' {
		i = 1
	}
	for i < len(path) {
		// Find next /
		j := i
		for j < len(path) && path[j] != '/' {
			j++
		}
		// Create this prefix
		prefix := path[0:j]
		pbuf := makeCString(prefix)
		_, _, errn = runtime.SysMkdir(runtime.Sliceptr(pbuf), uintptr(perm))
		if errn != 0 && errn != 17 {
			return Errno(errn)
		}
		i = j + 1
	}
	return nil
}

func RemoveAll(path string) error {
	// Try unlink first (works for files)
	buf := makeCString(path)
	_, _, errn := runtime.SysUnlink(runtime.Sliceptr(buf))
	if errn == 0 {
		return nil
	}

	// Try as directory - list contents and remove recursively
	entries, err := ListDir(path)
	if err != nil {
		// Path doesn't exist - not an error for RemoveAll
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

	// Now rmdir the empty directory
	buf = makeCString(path)
	_, _, errn = runtime.SysRmdir(runtime.Sliceptr(buf))
	if errn != 0 {
		return Errno(errn)
	}
	return nil
}

func Getenv(key string) string {
	data, err := ReadFile("/proc/self/environ")
	if err != nil {
		return ""
	}
	// Environment is null-separated KEY=VALUE pairs
	start := 0
	i := 0
	for i <= len(data) {
		if i == len(data) || data[i] == 0 {
			if i > start {
				entry := string(data[start:i])
				// Find '='
				eq := 0
				for eq < len(entry) && entry[eq] != '=' {
					eq++
				}
				if eq < len(entry) {
					k := entry[0:eq]
					if k == key {
						return entry[eq+1 : len(entry)]
					}
				}
			}
			start = i + 1
		}
		i++
	}
	return ""
}

func Environ() []string {
	data, err := ReadFile("/proc/self/environ")
	if err != nil {
		return nil
	}
	var result []string
	start := 0
	i := 0
	for i <= len(data) {
		if i == len(data) || data[i] == 0 {
			if i > start {
				result = append(result, string(data[start:i]))
			}
			start = i + 1
		}
		i++
	}
	return result
}

func ListDir(dirname string) ([]string, error) {
	buf := makeCString(dirname)
	fd, _, errn := runtime.SysOpen(runtime.Sliceptr(buf), uintptr(O_RDONLY), 0)
	if errn != 0 {
		return nil, Errno(errn)
	}
	var names []string
	dbuf := make([]byte, 4096)
	for {
		n, _, errn := runtime.SysGetdents64(fd, runtime.Sliceptr(dbuf), 4096)
		if errn != 0 {
			runtime.SysClose(fd)
			return nil, Errno(errn)
		}
		if n == 0 {
			break
		}
		offset := 0
		for offset < int(n) {
			// d_reclen is at offset 16, 2 bytes little-endian
			reclen := int(dbuf[offset+16]) + int(dbuf[offset+17])*256
			// d_name starts at offset 19
			nameStart := offset + 19
			nameEnd := nameStart
			for nameEnd < offset+reclen && dbuf[nameEnd] != 0 {
				nameEnd++
			}
			name := string(dbuf[nameStart:nameEnd])
			if name != "." && name != ".." {
				names = append(names, name)
			}
			offset = offset + reclen
		}
	}
	runtime.SysClose(fd)
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
	fd, _, errn := runtime.SysOpen(runtime.Sliceptr(buf), uintptr(O_RDONLY), 0)
	if errn != 0 {
		return nil, Errno(errn)
	}
	var entries []DirEntry
	dbuf := make([]byte, 4096)
	for {
		n, _, errn := runtime.SysGetdents64(fd, runtime.Sliceptr(dbuf), 4096)
		if errn != 0 {
			runtime.SysClose(fd)
			return nil, Errno(errn)
		}
		if n == 0 {
			break
		}
		offset := 0
		for offset < int(n) {
			reclen := int(dbuf[offset+16]) + int(dbuf[offset+17])*256
			dtype := dbuf[offset+18]
			nameStart := offset + 19
			nameEnd := nameStart
			for nameEnd < offset+reclen && dbuf[nameEnd] != 0 {
				nameEnd++
			}
			name := string(dbuf[nameStart:nameEnd])
			if name != "." && name != ".." {
				entries = append(entries, DirEntry{name: name, isDir: dtype == 4})
			}
			offset = offset + reclen
		}
	}
	runtime.SysClose(fd)
	return entries, nil
}

func Getwd() (string, error) {
	buf := make([]byte, 4096)
	n, _, errn := runtime.SysGetcwd(runtime.Sliceptr(buf), 4096)
	if errn != 0 {
		return "", Errno(errn)
	}
	// n includes the null terminator
	if n > 0 {
		n = n - 1
	}
	return string(buf[0:int(n)]), nil
}

func Chmod(name string, mode int) error {
	buf := makeCString(name)
	_, _, errn := runtime.SysChmod(runtime.Sliceptr(buf), uintptr(mode))
	if errn != 0 {
		return Errno(errn)
	}
	return nil
}

func Stat(name string) error {
	buf := makeCString(name)
	// stat buf needs 144 bytes on linux amd64
	statbuf := make([]byte, 144)
	_, _, errn := runtime.SysStat(runtime.Sliceptr(buf), runtime.Sliceptr(statbuf))
	if errn != 0 {
		return Errno(errn)
	}
	return nil
}

func init() {
	data, err := ReadFile("/proc/self/cmdline")
	if err != nil {
		return
	}
	var current []byte
	i := 0
	for i < len(data) {
		if data[i] == 0 {
			if len(current) > 0 {
				Args = append(Args, string(current))
			}
			current = nil
		} else {
			current = append(current, data[i])
		}
		i++
	}
	if len(current) > 0 {
		Args = append(Args, string(current))
	}
}
