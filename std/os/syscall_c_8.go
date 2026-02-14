//go:build c && c8

package os

type Errno int32

const (
	O_RDONLY int32 = 0
	O_WRONLY int32 = 1
	O_RDWR   int32 = 2
	O_CREAT  int32 = 64
	O_TRUNC  int32 = 512
	O_CREATE int32 = 64
)

func (e Errno) Error() string { return "syscall error" }
