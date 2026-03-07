#include <stdio.h>

int add(int a, int b);

int main(void) {
    int v = add(20, 22);
    printf("%d\n", v);
    return v != 42;
}
