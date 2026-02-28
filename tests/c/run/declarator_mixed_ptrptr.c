int main(void) {
  int x = 4, y = 3;
  int *p = &x, *q = &y;
  int **pp = &p;

  if (**pp != 4) {
    return 1;
  }

  *q = *q + **pp;
  if (y != 7) {
    return 2;
  }
  return 0;
}
