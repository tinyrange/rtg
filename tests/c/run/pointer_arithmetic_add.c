int main(void) {
  int a[4];
  int *p = a;
  int *q = p + 3;

  *(p + 1) = 7;
  *(1 + p) = *(p + 1) + 2;

  if (a[1] != 9) {
    return 1;
  }
  if ((q - p) != 3) {
    return 2;
  }
  return 0;
}
