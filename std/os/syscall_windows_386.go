//go:build windows && 386

package os

type Errno int32

const (
	SYS_READ       int32 = 3
	SYS_WRITE      int32 = 4
	SYS_OPEN       int32 = 5
	SYS_CLOSE      int32 = 6
	SYS_STAT       int32 = 509
	SYS_UNLINK     int32 = 10
	SYS_MKDIR      int32 = 39
	SYS_RMDIR      int32 = 40
	SYS_GETCWD     int32 = 183
	SYS_EXIT_GROUP int32 = 252

	SYS_GET_CMDLINE     int32 = 500
	SYS_GET_ENV_STRINGS int32 = 501
	SYS_FIND_FIRST_FILE int32 = 502
	SYS_FIND_NEXT_FILE  int32 = 503
	SYS_FIND_CLOSE      int32 = 504
	SYS_CREATE_PROCESS  int32 = 505
	SYS_WAIT_PROCESS    int32 = 506
	SYS_CREATE_PIPE     int32 = 507
	SYS_SET_STD_HANDLE  int32 = 508

	O_RDONLY int32 = 0
	O_WRONLY int32 = 1
	O_RDWR   int32 = 2
	O_CREAT  int32 = 64
	O_TRUNC  int32 = 512
	O_CREATE int32 = 64

	ERROR_NO_MORE_FILES int32 = 18
	FILE_ATTRIBUTE_DIRECTORY int32 = 16
)

func (e Errno) Error() string {
	return "syscall error"
}
