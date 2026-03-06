#ifndef __RTG_DIRENT_H
#define __RTG_DIRENT_H

#include <sys/types.h>

typedef struct __rtg_DIR DIR;

struct dirent {
  ino_t d_ino;
  unsigned short d_reclen;
  unsigned char d_type;
  char d_name[256];
};

DIR *opendir(const char *path);
struct dirent *readdir(DIR *dirp);
int closedir(DIR *dirp);
void rewinddir(DIR *dirp);

#endif
