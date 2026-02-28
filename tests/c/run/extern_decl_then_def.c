extern int add2(int a, int b);

int main(void) {
  if (add2(2, 5) != 7) {
    return 1;
  }
  return 0;
}

int add2(int a, int b) {
  return a + b;
}
