//go:build darwin

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

//rtg:linkstatic libSystem.dylib,_read
func sysRead(fd, buf, count uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_write
func sysWrite(fd, buf, count uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_open
func sysOpen(path, flags, mode uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_close
func sysClose(fd uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_stat
func sysStat(path, buf uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_mkdir
func sysMkdir(path, mode uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_rmdir
func sysRmdir(path uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_unlink
func sysUnlink(path uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_getcwd,ptr
func sysGetcwd(buf, size uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_exit,noreturn
func sysExit(code uintptr)

//rtg:linkstatic libSystem.dylib,_opendir,ptr
func sysOpendir(path uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_readdir,rawptr
func sysReaddir(dirp uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_closedir
func sysClosedir(dirp uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_chmod
func sysChmod(path, mode uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_getpid
func sysGetpid() (uintptr, uintptr, int32)

func (f *File) Write(p []byte) (n int, err error) {
	wrote, _, errn := sysWrite(uintptr(f.fd), runtime.Sliceptr(p), uintptr(len(p)))
	if errn != 0 {
		return int(wrote), Errno(errn)
	}
	return int(wrote), nil
}

func (f *File) Read(p []byte) (int, error) {
	n, _, errn := sysRead(uintptr(f.fd), runtime.Sliceptr(p), uintptr(len(p)))
	if errn != 0 {
		return int(n), Errno(errn)
	}
	return int(n), nil
}

func (f *File) Close() error {
	_, _, errn := sysClose(uintptr(f.fd))
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
	wrote, _, errn := sysWrite(uintptr(f.fd), runtime.Sliceptr(p), uintptr(len(p)))
	if errn != 0 {
		return int(wrote), Errno(errn)
	}
	return int(wrote), nil
}

func Exit(code int) {
	sysExit(uintptr(code))
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
	fd, _, errn := sysOpen(runtime.Sliceptr(buf), uintptr(flag), uintptr(perm))
	if errn != 0 {
		return nil, Errno(errn)
	}
	return &File{fd: int(fd)}, nil
}

func ReadFile(filename string) ([]byte, error) {
	buf := makeCString(filename)

	fd, _, errn := sysOpen(runtime.Sliceptr(buf), uintptr(O_RDONLY), 0)
	if errn != 0 {
		return nil, Errno(errn)
	}

	var data []byte
	chunk := make([]byte, 4096)
	for {
		n, _, errn := sysRead(fd, runtime.Sliceptr(chunk), 4096)
		if errn != 0 {
			sysClose(fd)
			return nil, Errno(errn)
		}
		if n == 0 {
			break
		}
		data = append(data, chunk[0:int(n)]...)
	}

	sysClose(fd)
	return data, nil
}

func WriteFile(name string, data []byte, perm int) error {
	buf := makeCString(name)
	flags := O_WRONLY + O_CREAT + O_TRUNC
	fd, _, errn := sysOpen(runtime.Sliceptr(buf), uintptr(flags), uintptr(perm))
	if errn != 0 {
		return Errno(errn)
	}
	for len(data) > 0 {
		n, _, errn := sysWrite(fd, runtime.Sliceptr(data), uintptr(len(data)))
		if errn != 0 {
			sysClose(fd)
			return Errno(errn)
		}
		data = data[int(n):len(data)]
	}
	sysClose(fd)
	return nil
}

func MkdirAll(path string, perm FileMode) error {
	buf := makeCString(path)
	_, _, errn := sysMkdir(runtime.Sliceptr(buf), uintptr(perm))
	if errn == 0 || errn == 17 {
		// 17 = EEXIST
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
		_, _, errn = sysMkdir(runtime.Sliceptr(pbuf), uintptr(perm))
		if errn != 0 && errn != 17 {
			return Errno(errn)
		}
		i = j + 1
	}
	return nil
}

func RemoveAll(path string) error {
	buf := makeCString(path)
	_, _, errn := sysUnlink(runtime.Sliceptr(buf))
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
	_, _, errn = sysRmdir(runtime.Sliceptr(buf))
	if errn != 0 {
		return Errno(errn)
	}
	return nil
}

func Getenv(key string) string {
	envp, _, _ := runtime.SysGetenvp()
	if envp == 0 {
		return ""
	}

	ptr := envp
	for {
		entryPtr := uintptr(runtime.ReadPtr(ptr))
		if entryPtr == 0 {
			break
		}
		var entry []byte
		p := entryPtr
		for {
			cb := byte(runtime.ReadPtr(p))
			if cb == 0 {
				break
			}
			entry = append(entry, cb)
			p = p + 1
		}
		ptr = ptr + 8 // next pointer in envp array

		s := string(entry)
		eq := 0
		for eq < len(s) && s[eq] != '=' {
			eq++
		}
		if eq < len(s) {
			k := s[0:eq]
			if k == key {
				return s[eq+1 : len(s)]
			}
		}
	}
	return ""
}

func Environ() []string {
	envp, _, _ := runtime.SysGetenvp()
	if envp == 0 {
		return nil
	}

	var result []string
	ptr := envp
	for {
		entryPtr := uintptr(runtime.ReadPtr(ptr))
		if entryPtr == 0 {
			break
		}
		var entry []byte
		p := entryPtr
		for {
			cb := byte(runtime.ReadPtr(p))
			if cb == 0 {
				break
			}
			entry = append(entry, cb)
			p = p + 1
		}
		ptr = ptr + 8
		if len(entry) > 0 {
			result = append(result, string(entry))
		}
	}
	return result
}

func ListDir(dirname string) ([]string, error) {
	buf := makeCString(dirname)
	dirp, _, errn := sysOpendir(runtime.Sliceptr(buf))
	if errn != 0 || dirp == 0 {
		if errn == 0 {
			errn = 1
		}
		return nil, Errno(errn)
	}

	var names []string
	for {
		entryPtr, _, errn := sysReaddir(dirp)
		if entryPtr == 0 {
			break
		}
		_ = errn
		// macOS arm64 struct dirent: d_ino(8) + d_seekoff(8) + d_reclen(2) + d_namlen(2) + d_type(1) + d_name(...)
		// d_name starts at offset 21
		nameStart := entryPtr + 21
		p := nameStart
		var nameBytes []byte
		for {
			cb := byte(runtime.ReadPtr(p))
			if cb == 0 {
				break
			}
			nameBytes = append(nameBytes, cb)
			p = p + 1
		}
		name := string(nameBytes)
		if name != "." && name != ".." {
			names = append(names, name)
		}
	}

	sysClosedir(dirp)
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
	dirp, _, errn := sysOpendir(runtime.Sliceptr(buf))
	if errn != 0 || dirp == 0 {
		if errn == 0 {
			errn = 1
		}
		return nil, Errno(errn)
	}

	var entries []DirEntry
	for {
		entryPtr, _, errn := sysReaddir(dirp)
		if entryPtr == 0 {
			break
		}
		_ = errn
		// d_type at offset 20
		dtype := byte(runtime.ReadPtr(entryPtr + 20))
		// d_name starts at offset 21
		nameStart := entryPtr + 21
		p := nameStart
		var nameBytes []byte
		for {
			cb := byte(runtime.ReadPtr(p))
			if cb == 0 {
				break
			}
			nameBytes = append(nameBytes, cb)
			p = p + 1
		}
		name := string(nameBytes)
		if name != "." && name != ".." {
			entries = append(entries, DirEntry{name: name, isDir: dtype == 4})
		}
	}

	sysClosedir(dirp)
	return entries, nil
}

func Getwd() (string, error) {
	buf := make([]byte, 4096)
	ret, _, errn := sysGetcwd(runtime.Sliceptr(buf), 4096)
	if errn != 0 || ret == 0 {
		if errn == 0 {
			errn = 1
		}
		return "", Errno(errn)
	}
	// Find null terminator
	n := 0
	for n < len(buf) && buf[n] != 0 {
		n++
	}
	return string(buf[0:n]), nil
}

func Chmod(name string, mode int) error {
	buf := makeCString(name)
	_, _, errn := sysChmod(runtime.Sliceptr(buf), uintptr(mode))
	if errn != 0 {
		return Errno(errn)
	}
	return nil
}

func Stat(name string) error {
	buf := makeCString(name)
	// macOS arm64 stat buf is 144 bytes
	statbuf := make([]byte, 144)
	_, _, errn := sysStat(runtime.Sliceptr(buf), runtime.Sliceptr(statbuf))
	if errn != 0 {
		return Errno(errn)
	}
	return nil
}

func Getpid() int {
	pid, _, _ := sysGetpid()
	return int(pid)
}

func init() {
	// Get argc, argv from saved globals
	argc, _, _ := runtime.SysGetargc()
	argv, _, _ := runtime.SysGetargv()

	if argc == 0 || argv == 0 {
		return
	}

	i := uintptr(0)
	for i < argc {
		// argv[i] is a pointer to a C string
		argPtr := uintptr(runtime.ReadPtr(argv + i*8))
		if argPtr == 0 {
			break
		}
		var argBytes []byte
		p := argPtr
		for {
			cb := byte(runtime.ReadPtr(p))
			if cb == 0 {
				break
			}
			argBytes = append(argBytes, cb)
			p = p + 1
		}
		Args = append(Args, string(argBytes))
		i++
	}
}
