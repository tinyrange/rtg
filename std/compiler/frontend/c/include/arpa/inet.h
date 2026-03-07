#ifndef __RTG_ARPA_INET_H
#define __RTG_ARPA_INET_H

#include <netinet/in.h>
#include <sys/socket.h>

unsigned long inet_addr(const char *cp);
char *inet_ntoa();
int inet_aton(const char *cp, struct in_addr *inp);
const char *inet_ntop(int af, const void *src, char *dst, socklen_t size);
int inet_pton(int af, const char *src, void *dst);

#endif
