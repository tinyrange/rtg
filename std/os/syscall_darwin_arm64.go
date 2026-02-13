//go:build darwin && arm64

package os

type Errno int32

const (
	SYS_READ       int32 = 0
	SYS_WRITE      int32 = 1
	SYS_OPEN       int32 = 2
	SYS_CLOSE      int32 = 3
	SYS_STAT       int32 = 4
	SYS_MKDIR      int32 = 5
	SYS_RMDIR      int32 = 6
	SYS_UNLINK     int32 = 7
	SYS_GETCWD     int32 = 8
	SYS_EXIT_GROUP int32 = 9
	SYS_MMAP       int32 = 10
	SYS_OPENDIR    int32 = 14
	SYS_READDIR    int32 = 15
	SYS_CLOSEDIR   int32 = 16
	SYS_DUP2       int32 = 17
	SYS_FORK       int32 = 18
	SYS_EXECVE     int32 = 19
	SYS_WAIT4      int32 = 20
	SYS_PIPE2      int32 = 21
	SYS_CHMOD      int32 = 22
	SYS_GETARGC    int32 = 100
	SYS_GETARGV    int32 = 101
	SYS_GETENVP    int32 = 102

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
