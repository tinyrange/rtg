#define ARG_A 8
#define ARG_B 12

    .text
    .globl asm_add
asm_add:
    pushl %ebp
    movl %esp, %ebp
    movl ARG_A(%ebp), %eax
    addl ARG_B(%ebp), %eax
    popl %ebp
    ret
