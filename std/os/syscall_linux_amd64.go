//go:build linux && amd64

package os

type Errno int32

const (
	SYS_READ       int32 = 0
	SYS_WRITE      int32 = 1
	SYS_OPEN       int32 = 2
	SYS_CLOSE      int32 = 3
	SYS_STAT       int32 = 4
	SYS_DUP2       int32 = 33
	SYS_FORK       int32 = 57
	SYS_EXECVE     int32 = 59
	SYS_WAIT4      int32 = 61
	SYS_GETCWD     int32 = 79
	SYS_MKDIR      int32 = 83
	SYS_RMDIR      int32 = 84
	SYS_UNLINK     int32 = 87
	SYS_CHMOD      int32 = 90
	SYS_GETDENTS64 int32 = 217
	SYS_EXIT_GROUP int32 = 231
	SYS_PIPE2      int32 = 293

	O_RDONLY int32 = 0
	O_WRONLY int32 = 1
	O_RDWR   int32 = 2
	O_CREAT  int32 = 64
	O_TRUNC  int32 = 512
	O_CREATE int32 = 64
)

func (e Errno) Error() string {
	return "syscall error"
}
