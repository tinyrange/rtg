#ifndef __RTG_SETJMP_H
#define __RTG_SETJMP_H

typedef char *jmp_buf[1];

int setjmp(jmp_buf env);
void longjmp(jmp_buf env, int value);

#endif
