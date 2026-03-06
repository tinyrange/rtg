#ifndef __RTG_FCNTL_H
#define __RTG_FCNTL_H

#define O_RDONLY 0
#define O_WRONLY 1
#define O_RDWR 2
#define O_NONBLOCK 4
#define O_BINARY 0x8000
#define _O_BINARY O_BINARY

int fcntl(int fd, int cmd, ...);
int _setmode(int fd, int mode);

#endif
