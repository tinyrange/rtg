char *words[] = {"ab", "cd", "ef"};

int main(void) {
	char *local[] = {words[2], words[0]};

	if (sizeof(words) / sizeof(words[0]) != 3) return 1;
	if (words[1][1] != 'd') return 2;
	if (local[0][0] != 'e') return 3;
	if (local[1][1] != 'b') return 4;
	return 0;
}
