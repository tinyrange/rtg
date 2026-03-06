#ifndef __RTG_UNISTD_H
#define __RTG_UNISTD_H

#include <stddef.h>

typedef __PTRDIFF_TYPE__ ssize_t;
typedef int pid_t;

#define STDIN_FILENO 0
#define STDOUT_FILENO 1
#define STDERR_FILENO 2

unsigned int sleep(unsigned int seconds);
int usleep(unsigned int usec);
ssize_t read(int fd, void *buf, size_t count);
ssize_t write(int fd, const void *buf, size_t count);
int close(int fd);
long lseek(int fd, long offset, int whence);
pid_t fork(void);
int isatty(int fd);

#endif
