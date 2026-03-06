#ifndef __RTG_SYS_SELECT_H
#define __RTG_SYS_SELECT_H

#include <sys/types.h>
#include <sys/time.h>

#define FD_SETSIZE 1024

typedef struct {
  unsigned long fds_bits[32];
} fd_set;

#define FD_ZERO(set) ((void)(set))
#define FD_SET(fd, set) ((void)(fd), (void)(set))
#define FD_CLR(fd, set) ((void)(fd), (void)(set))
#define FD_ISSET(fd, set) (0)

int select();

#endif
