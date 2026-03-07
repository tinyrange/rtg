int main(void) {
  int a[2][3];

  a[0][1] = 7;
  a[1][2] = 9;

  if (a[0][1] != 7) return 1;
  if (a[1][2] != 9) return 2;
  if (sizeof(a) != 6 * sizeof(int)) return 3;
  if (sizeof(a[0]) != 3 * sizeof(int)) return 4;
  return 0;
}
