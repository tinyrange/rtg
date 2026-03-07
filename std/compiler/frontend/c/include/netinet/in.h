#ifndef __RTG_NETINET_IN_H
#define __RTG_NETINET_IN_H

#include <stdint.h>
#include <sys/types.h>

typedef uint16_t in_port_t;
typedef uint32_t in_addr_t;

struct in_addr {
  in_addr_t s_addr;
};

struct sockaddr_in {
  uint8_t sin_len;
  sa_family_t sin_family;
  in_port_t sin_port;
  struct in_addr sin_addr;
  unsigned char sin_zero[8];
};

#define AF_INET 2
#define INADDR_ANY ((in_addr_t)0)
#define IPPROTO_TCP 6

#endif
