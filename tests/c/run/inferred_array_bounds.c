#include <stddef.h>

char global_text[] = "abc";
char padded_text[5] = "xy";
int global_nums[] = {10, 20, 30};

int main(void) {
	char local_text[] = "ok";
	int local_nums[] = {4, 5};

	if (sizeof(global_text) != 4) return 1;
	if (sizeof(padded_text) != 5) return 2;
	if (sizeof(global_nums) / sizeof(global_nums[0]) != 3) return 3;
	if (sizeof(local_text) != 3) return 4;
	if (sizeof(local_nums) / sizeof(local_nums[0]) != 2) return 5;
	if (global_text[3] != 0 || padded_text[2] != 0 || local_text[2] != 0) return 6;
	if (global_nums[2] != 30 || local_nums[1] != 5) return 7;
	return 0;
}
