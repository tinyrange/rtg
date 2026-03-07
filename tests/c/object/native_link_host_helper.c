#include <stdio.h>

int call_host(int x);

int host_mul2(int x) {
    return x * 2;
}

int main(void) {
    int v = call_host(21);
    printf("%d\n", v);
    return v != 43;
}
