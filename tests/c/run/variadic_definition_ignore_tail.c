typedef int (*sum2_fn)(int, int, ...);

int side = 0;

int bump(void) {
  side = side + 1;
  return 9;
}

int sum2(int a, int b, ...) {
  return a + b;
}

int main(void) {
  sum2_fn fn = sum2;
  if (sum2(2, 3, bump()) != 5) {
    return 1;
  }
  if (fn(4, 6, bump(), 123) != 10) {
    return 2;
  }
  if (side != 2) {
    return 3;
  }
  return 0;
}
