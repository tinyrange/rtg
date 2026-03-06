#define PREFIX(a) f(a,
#define f(a, b) ((a) + (b))

int main(void) {
	return PREFIX(1) 2) != 3;
}
