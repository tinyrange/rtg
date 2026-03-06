#ifndef __RTG_SYS_SOCKET_H
#define __RTG_SYS_SOCKET_H

#include <sys/types.h>

struct sockaddr {
  sa_family_t sa_family;
  char sa_data[14];
};

struct sockaddr_storage {
  sa_family_t ss_family;
  char __ss_padding[126];
};

#define AF_UNSPEC 0
#define AF_UNIX 1
#define AF_INET 2
#define SOCK_STREAM 1
#define SOCK_DGRAM 2
#define SOCK_RAW 3
#define SOCK_SEQPACKET 5
#define SOL_SOCKET 0xffff
#define SO_REUSEADDR 0x0004
#define SO_ERROR 0x1007
#define SHUT_RD 0
#define SHUT_WR 1
#define SHUT_RDWR 2

int socket(int domain, int type, int protocol);
int socketpair(int domain, int type, int protocol, int sv[2]);
int bind(int fd, const struct sockaddr *addr, socklen_t len);
int connect(int fd, const struct sockaddr *addr, socklen_t len);
int listen(int fd, int backlog);
int accept(int fd, struct sockaddr *addr, socklen_t *len);
int shutdown(int fd, int how);
int getsockname(int fd, struct sockaddr *addr, socklen_t *len);
int getpeername(int fd, struct sockaddr *addr, socklen_t *len);
int setsockopt(int fd, int level, int optname, const void *optval, socklen_t optlen);
int getsockopt(int fd, int level, int optname, void *optval, socklen_t *optlen);
long send(int fd, const void *buf, size_t len, int flags);
long recv(int fd, void *buf, size_t len, int flags);

#endif
