#ifndef __RTG_WCHAR_H
#define __RTG_WCHAR_H

#include <stddef.h>
#include <stdarg.h>

typedef unsigned int wint_t;

typedef struct {
  unsigned int __state;
} mbstate_t;

#define WEOF ((wint_t)-1)

size_t wcslen(const wchar_t *s);
int wcscmp(const wchar_t *lhs, const wchar_t *rhs);
wchar_t *wcscpy(wchar_t *dst, const wchar_t *src);
wchar_t *wmemcpy(wchar_t *dst, const wchar_t *src, size_t n);
wchar_t *wmemset(wchar_t *dst, wchar_t c, size_t n);
wint_t fgetwc(void *stream);
wint_t fputwc(wchar_t wc, void *stream);
int swprintf(wchar_t *buf, size_t n, const wchar_t *fmt, ...);
int vswprintf(wchar_t *buf, size_t n, const wchar_t *fmt, va_list ap);

#endif
