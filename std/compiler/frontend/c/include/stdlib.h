#ifndef __RTG_STDLIB_H
#define __RTG_STDLIB_H

#include <stddef.h>

#define EXIT_SUCCESS 0
#define EXIT_FAILURE 1
#define RAND_MAX 2147483647

typedef struct {
  int quot;
  int rem;
} div_t;

void *malloc(size_t size);
void *calloc(size_t count, size_t size);
void *realloc(void *ptr, size_t size);
void free(void *ptr);

int atoi(const char *s);
long atol(const char *s);
long strtol(const char *s, char **endptr, int base);
unsigned long strtoul(const char *s, char **endptr, int base);
int atof(const char *s);

int abs(int v);
long labs(long v);

void abort(void);
void exit(int status);
int system(const char *cmd);
char *getenv(const char *name);

int rand(void);
void srand(unsigned int seed);

void qsort(void);
void *bsearch(void);

#endif
