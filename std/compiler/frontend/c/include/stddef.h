#ifndef __RTG_STDDEF_H
#define __RTG_STDDEF_H

typedef __SIZE_TYPE__ size_t;
typedef __PTRDIFF_TYPE__ ptrdiff_t;
typedef __WCHAR_TYPE__ wchar_t;

#define NULL ((void *)0)
#define SIZE_MAX __SIZE_MAX__
#define offsetof(type, member) ((size_t)&(((type *)0)->member))

#endif
