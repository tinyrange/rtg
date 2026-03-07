#ifndef __RTG_SYS_IOCTL_H
#define __RTG_SYS_IOCTL_H

#include <sys/types.h>

#define TIOCGWINSZ 0x5413

int ioctl(int fd, unsigned long request, ...);

#endif
