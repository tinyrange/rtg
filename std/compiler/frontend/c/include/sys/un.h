#ifndef __RTG_SYS_UN_H
#define __RTG_SYS_UN_H

#include <sys/types.h>

struct sockaddr_un {
  sa_family_t sun_family;
  char sun_path[104];
};

#endif
