#ifndef PASS2
#define PASS2
#define VALUE 7
#include __FILE__
int main(void) { return VALUE != 3; }
#else
#undef VALUE
#define VALUE 3
#endif
