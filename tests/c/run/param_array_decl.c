int sum2(int vals[2]) {
  return vals[0] + vals[1];
}

int main(void) {
  int a[2];
  a[0] = 5;
  a[1] = 6;
  if (sum2(a) != 11) {
    return 1;
  }
  return 0;
}
