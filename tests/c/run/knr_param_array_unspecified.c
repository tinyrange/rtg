int sum(argv) char *argv[]; {
	return argv[0][0] + argv[1][0];
}

int main(void) {
	char a[] = "A";
	char b[] = "B";
	char *argv[] = {a, b};
	return sum(argv) != ('A' + 'B');
}
