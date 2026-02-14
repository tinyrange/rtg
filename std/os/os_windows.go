//go:build windows

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
	buf := makeCString(path)
	_, _, errn := runtime.SysMkdir(runtime.Sliceptr(buf), uintptr(perm))
	if errn == 0 || errn == 183 {
		// 183 = ERROR_ALREADY_EXISTS
		return nil
	}

	i := 0
	if len(path) > 0 && (path[0] == '/' || path[0] == '\\') {
		i = 1
	}
	// Skip drive letter (e.g., C:\)
	if len(path) >= 3 && path[1] == ':' && (path[2] == '/' || path[2] == '\\') {
		i = 3
	}
	for i < len(path) {
		j := i
		for j < len(path) && path[j] != '/' && path[j] != '\\' {
			j++
		}
		prefix := path[0:j]
		pbuf := makeCString(prefix)
		_, _, errn = runtime.SysMkdir(runtime.Sliceptr(pbuf), uintptr(perm))
		if errn != 0 && errn != 183 {
			return Errno(errn)
		}
		i = j + 1
	}
	return nil
}

func RemoveAll(path string) error {
	buf := makeCString(path)
	_, _, errn := runtime.SysUnlink(runtime.Sliceptr(buf))
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
	_, _, errn = runtime.SysRmdir(runtime.Sliceptr(buf))
	if errn != 0 {
		return Errno(errn)
	}
	return nil
}

func Getenv(key string) string {
	// Get environment block
	envPtr, _, _ := runtime.SysGetEnvStrings()
	if envPtr == 0 {
		return ""
	}

	// Environment block is double-null-terminated: KEY=VALUE\0KEY=VALUE\0\0
	ptr := envPtr
	for {
		// Read first byte to check for double-null terminator
		fb := byte(runtime.ReadPtr(ptr))
		if fb == 0 {
			break
		}
		// Read the whole entry up to null
		var entry []byte
		p := ptr
		for {
			cb := byte(runtime.ReadPtr(p))
			if cb == 0 {
				break
			}
			entry = append(entry, cb)
			p = p + 1
		}
		ptr = p + 1 // skip null

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
	envPtr, _, _ := runtime.SysGetEnvStrings()
	if envPtr == 0 {
		return nil
	}

	var result []string
	ptr := envPtr
	for {
		fb := byte(runtime.ReadPtr(ptr))
		if fb == 0 {
			break
		}
		var entry []byte
		p := ptr
		for {
			cb := byte(runtime.ReadPtr(p))
			if cb == 0 {
				break
			}
			entry = append(entry, cb)
			p = p + 1
		}
		ptr = p + 1
		if len(entry) > 0 {
			result = append(result, string(entry))
		}
	}
	return result
}

func ListDir(dirname string) ([]string, error) {
	// Build search pattern: dirname\*
	pattern := dirname + "\\*"
	pbuf := makeCString(pattern)

	// WIN32_FIND_DATAA is 320 bytes
	findData := make([]byte, 320)

	handle, _, errn := runtime.SysFindFirstFile(runtime.Sliceptr(pbuf), runtime.Sliceptr(findData))
	if errn != 0 {
		return nil, Errno(errn)
	}

	var names []string
	for {
		// Filename is at offset 44 in WIN32_FIND_DATAA, null-terminated
		nameStart := 44
		nameEnd := nameStart
		for nameEnd < 320 && findData[nameEnd] != 0 {
			nameEnd++
		}
		name := string(findData[nameStart:nameEnd])
		if name != "." && name != ".." {
			names = append(names, name)
		}

		// Clear findData for next iteration
		i := 0
		for i < 320 {
			findData[i] = 0
			i++
		}

		_, _, errn = runtime.SysFindNextFile(handle, runtime.Sliceptr(findData))
		if errn != 0 {
			break
		}
	}

	runtime.SysFindClose(handle)
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
	pattern := dirname + "\\*"
	pbuf := makeCString(pattern)

	findData := make([]byte, 320)

	handle, _, errn := runtime.SysFindFirstFile(runtime.Sliceptr(pbuf), runtime.Sliceptr(findData))
	if errn != 0 {
		return nil, Errno(errn)
	}

	var entries []DirEntry
	for {
		// dwFileAttributes at offset 0
		attrs := int(findData[0]) + int(findData[1])*256 + int(findData[2])*65536 + int(findData[3])*16777216

		nameStart := 44
		nameEnd := nameStart
		for nameEnd < 320 && findData[nameEnd] != 0 {
			nameEnd++
		}
		name := string(findData[nameStart:nameEnd])
		if name != "." && name != ".." {
			entries = append(entries, DirEntry{name: name, isDir: (attrs & int(FILE_ATTRIBUTE_DIRECTORY)) != 0})
		}

		i := 0
		for i < 320 {
			findData[i] = 0
			i++
		}

		_, _, errn = runtime.SysFindNextFile(handle, runtime.Sliceptr(findData))
		if errn != 0 {
			break
		}
	}

	runtime.SysFindClose(handle)
	return entries, nil
}

func Getwd() (string, error) {
	buf := make([]byte, 4096)
	n, _, errn := runtime.SysGetcwd(runtime.Sliceptr(buf), 4096)
	if errn != 0 {
		return "", Errno(errn)
	}
	// n is the number of chars written (not including null)
	return string(buf[0:int(n)]), nil
}

func Chmod(name string, mode int) error {
	return nil
}

func Stat(name string) error {
	buf := makeCString(name)
	_, _, errn := runtime.SysStat(runtime.Sliceptr(buf), 0)
	if errn != 0 {
		return Errno(errn)
	}
	return nil
}

func Getpid() int {
	pid, _, _ := runtime.SysGetpid()
	return int(pid)
}

func init() {
	// Get command line string
	cmdLinePtr, _, _ := runtime.SysGetCommandLine()
	if cmdLinePtr == 0 {
		return
	}

	// Parse command line into args (simple quote-aware splitting)
	// Read the command line into a byte slice
	var cmdLine []byte
	p := cmdLinePtr
	for {
		b := byte(runtime.ReadPtr(p))
		if b == 0 {
			break
		}
		cmdLine = append(cmdLine, b)
		p = p + 1
	}

	i := 0
	for i < len(cmdLine) {
		// Skip whitespace
		for i < len(cmdLine) && (cmdLine[i] == ' ' || cmdLine[i] == '\t') {
			i++
		}
		if i >= len(cmdLine) {
			break
		}

		var arg []byte
		if cmdLine[i] == '"' {
			// Quoted argument
			i++ // skip opening quote
			for i < len(cmdLine) && cmdLine[i] != '"' {
				if cmdLine[i] == '\\' && i+1 < len(cmdLine) {
					if cmdLine[i+1] == '"' || cmdLine[i+1] == '\\' {
						arg = append(arg, cmdLine[i+1])
						i = i + 2
						continue
					}
				}
				arg = append(arg, cmdLine[i])
				i++
			}
			if i < len(cmdLine) {
				i++ // skip closing quote
			}
		} else {
			// Unquoted argument
			for i < len(cmdLine) && cmdLine[i] != ' ' && cmdLine[i] != '\t' {
				arg = append(arg, cmdLine[i])
				i++
			}
		}

		if len(arg) > 0 {
			Args = append(Args, string(arg))
		}
	}
}
