#define SUM(a, b) a /* comment
spans a newline */ + b

int main(void) {
	return SUM(1, 2) != 3;
}
