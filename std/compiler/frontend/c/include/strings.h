#ifndef __RTG_STRINGS_H
#define __RTG_STRINGS_H

#include <stddef.h>

int bcmp(const void *lhs, const void *rhs, size_t n);
void bcopy(const void *src, void *dst, size_t n);
void bzero(void *dst, size_t n);
int ffs(int value);
char *index(const char *s, int c);
char *rindex(const char *s, int c);
int strcasecmp(const char *lhs, const char *rhs);
int strncasecmp(const char *lhs, const char *rhs, size_t n);

#endif
