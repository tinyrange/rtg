#ifndef __RTG_ERR_H
#define __RTG_ERR_H

#include <stdarg.h>

void warn(const char *fmt, ...);
void vwarn(const char *fmt, va_list ap);
void warnx(const char *fmt, ...);
void vwarnx(const char *fmt, va_list ap);
void err(int eval, const char *fmt, ...);
void verr(int eval, const char *fmt, va_list ap);
void errx(int eval, const char *fmt, ...);
void verrx(int eval, const char *fmt, va_list ap);

#endif
