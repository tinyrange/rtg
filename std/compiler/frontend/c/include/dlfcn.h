#ifndef __RTG_DLFCN_H
#define __RTG_DLFCN_H

#define RTLD_LAZY 0x1
#define RTLD_NOW 0x2
#define RTLD_GLOBAL 0x8
#define RTLD_LOCAL 0x4

void *dlopen(const char *path, int mode);
int dlclose(void *handle);
void *dlsym(void *handle, const char *symbol);
char *dlerror(void);

#endif
