#ifndef PASS2
#define PASS2
int id(int x);
int main(void) {
	return id(
#include __FILE__
	);
}
int id(int x) { return x - 7; }
#else
7
#endif
