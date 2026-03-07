#define PREFIX f(1,
#define f(a, b) ((a) + (b))

int main(void) {
	return PREFIX 2) != 3;
}
