package common

import "os"

type Target struct {
	Triple                string
	GOOS                  string
	GOARCH                string
	PtrSize               int
	Backend               string // native, c, ir, or vm
	CModel                int    // 16/32/64 when targetBackend==c
	WordSize              int    // word size in bytes
	BuildTags             []string
	Defines               map[string]string
	Strict                bool
	Profile               bool
	CompilerDebug         bool
	StripBinary           bool
	StdlibIncludePaths    []string
	StdlibIncludeExplicit bool
	StdlibIncludeEmbedded bool
	TestMode              bool
}

func HexDigit(v byte) byte {
	if v < 10 {
		return '0' + v
	}
	return 'a' + v - 10
}

func AppendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

// NormalizePath replaces backslashes with forward slashes for Windows compatibility.
func NormalizePath(path string) string {
	buf := make([]byte, len(path))
	i := 0
	for i < len(path) {
		if path[i] == '\\' {
			buf[i] = '/'
		} else {
			buf[i] = path[i]
		}
		i = i + 1
	}
	return string(buf)
}

// DirName returns the directory portion of a path.
func DirName(path string) string {
	i := len(path) - 1
	for i >= 0 {
		if path[i] == '/' {
			if i == 0 {
				return "/"
			}
			return path[0:i]
		}
		i = i - 1
	}
	return "."
}

func TrimTrailingSlash(path string) string {
	for len(path) > 1 && path[len(path)-1] == '/' {
		path = path[0 : len(path)-1]
	}
	return path
}

func PathExists(path string) bool {
	_, err := os.ReadFile(path)
	return err == nil
}

// WalkDirectory recursively collects all files under dir, returning
// relative paths (relative to base) and their contents.
func WalkDirectory(base string, dir string) ([]string, []string) {
	var names []string
	var data []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return names, data
	}
	for _, entry := range entries {
		path := dir + "/" + entry.Name()
		if entry.IsDir() {
			subNames, subData := WalkDirectory(base, path)
			names = append(names, subNames...)
			data = append(data, subData...)
		} else {
			content, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			// Compute relative path from base
			rel := path[len(base)+1 : len(path)]
			names = append(names, rel)
			data = append(data, string(content))
		}
	}
	return names, data
}

func PutU64(buf []byte, v uint64) {
	buf[0] = byte(v)
	buf[1] = byte(v >> 8)
	buf[2] = byte(v >> 16)
	buf[3] = byte(v >> 24)
	buf[4] = byte(v >> 32)
	buf[5] = byte(v >> 40)
	buf[6] = byte(v >> 48)
	buf[7] = byte(v >> 56)
}

func GetU64(b []byte) uint64 {
	return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
		uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
}

func PutU16(b []byte, v uint16) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
}

func PutU32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}

func GetU32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}
