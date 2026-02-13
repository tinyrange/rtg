//go:build linux && 386

package os

type Errno int32

const (
	SYS_READ       int32 = 3
	SYS_WRITE      int32 = 4
	SYS_OPEN       int32 = 5
	SYS_CLOSE      int32 = 6
	SYS_STAT       int32 = 106
	SYS_DUP2       int32 = 63
	SYS_FORK       int32 = 2
	SYS_EXECVE     int32 = 11
	SYS_WAIT4      int32 = 114
	SYS_GETCWD     int32 = 183
	SYS_MKDIR      int32 = 39
	SYS_RMDIR      int32 = 40
	SYS_UNLINK     int32 = 10
	SYS_CHMOD      int32 = 15
	SYS_GETDENTS64 int32 = 220
	SYS_EXIT_GROUP int32 = 252
	SYS_PIPE2      int32 = 331

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
