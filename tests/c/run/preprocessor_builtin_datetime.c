int main(void) {
	char *d = __DATE__;
	char *t = __TIME__;
	if (!d || !t) return 1;
	if (d[0] == 0 || t[0] == 0) return 2;
	if (t[2] != ':' || t[5] != ':') return 3;
	return 0;
}
