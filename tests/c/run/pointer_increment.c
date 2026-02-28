int main(void) {
  int a[4];
  int *p = a;

  p++;
  *p = 5;
  ++p;
  *p = 8;

  p--;
  if (*p != 5) {
    return 1;
  }
  if (a[2] != 8) {
    return 2;
  }

  p--;
  if (*p != 0) {
    return 3;
  }
  return 0;
}
