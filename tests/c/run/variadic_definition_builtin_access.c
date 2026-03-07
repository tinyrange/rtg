int sum_tail(int base, ...) {
  int total = base;
  int i = 0;
  int n = __builtin_va_count();
  while (i < n) {
    total = total + __builtin_va_arg(i);
    i = i + 1;
  }
  return total;
}

int main(void) {
  if (sum_tail(5) != 5) {
    return 1;
  }
  if (sum_tail(5, 7, 11) != 23) {
    return 2;
  }
  return 0;
}
