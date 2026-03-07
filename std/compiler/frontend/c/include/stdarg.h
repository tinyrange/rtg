#ifndef __RTG_STDARG_H
#define __RTG_STDARG_H

typedef int va_list;

#define va_start(ap, last) ap = 0
#define va_end(ap) ap = ap
#define va_copy(dst, src) dst = src
#define va_arg(ap, type) ((type)__builtin_va_arg((((ap) = (ap) + 1) - 1)))

#endif
