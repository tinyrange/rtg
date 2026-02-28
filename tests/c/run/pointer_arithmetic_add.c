int main(void) {
  int a[4];
  int *p = a;

  *(p + 1) = 7;
  *(1 + p) = *(p + 1) + 2;

  if (a[1] != 9) {
    return 1;
  }
  return 0;
}
