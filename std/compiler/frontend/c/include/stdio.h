#ifndef __RTG_STDIO_H
#define __RTG_STDIO_H

#include <stddef.h>
#include <stdarg.h>

typedef struct __rtg_FILE FILE;

#define EOF (-1)
#define SEEK_SET 0
#define SEEK_CUR 1
#define SEEK_END 2

extern FILE *stdin;
extern FILE *stdout;
extern FILE *stderr;

int printf(const char *fmt, ...);
int fprintf(FILE *stream, const char *fmt, ...);
int sprintf(char *dst, const char *fmt, ...);
int snprintf(char *dst, size_t n, const char *fmt, ...);
int scanf(const char *fmt, ...);
int sscanf(const char *src, const char *fmt, ...);
int vprintf(const char *fmt, va_list ap);
int putchar(int ch);
int getchar(void);
int puts(const char *s);
void perror(const char *s);
FILE *fopen(const char *path, const char *mode);
FILE *popen(const char *cmd, const char *mode);
int fclose(FILE *stream);
int pclose(FILE *stream);
int fflush(FILE *stream);
char *fgets(char *s, int n, FILE *stream);
size_t fread(void *ptr, size_t size, size_t count, FILE *stream);
size_t fwrite(const void *ptr, size_t size, size_t count, FILE *stream);
void setbuf(FILE *stream, char *buf);
int getdelim(char **lineptr, size_t *n, int delim, FILE *stream);

#endif
