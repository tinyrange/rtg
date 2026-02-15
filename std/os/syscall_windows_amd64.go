//go:build windows && amd64

package os

type Errno int32

const (
	O_RDONLY int32 = 0
	O_WRONLY int32 = 1
	O_RDWR   int32 = 2
	O_CREAT  int32 = 64
	O_TRUNC  int32 = 512
	O_CREATE int32 = 64

	ERROR_NO_MORE_FILES      int32 = 18
	FILE_ATTRIBUTE_DIRECTORY int32 = 16
)

func (e Errno) Error() string {
	return "syscall error"
}
