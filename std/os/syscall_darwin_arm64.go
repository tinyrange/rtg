//go:build darwin && arm64

package os

type Errno int32

const (
	O_RDONLY int32 = 0
	O_WRONLY int32 = 1
	O_RDWR   int32 = 2
	O_CREAT  int32 = 512
	O_TRUNC  int32 = 1024
	O_CREATE int32 = 512
)

func (e Errno) Error() string {
	return "syscall error"
}
