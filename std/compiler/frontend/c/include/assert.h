#ifndef __RTG_ASSERT_H
#define __RTG_ASSERT_H

#include <stdlib.h>

#define assert(expr) ((expr) ? (void)0 : abort())

#endif
