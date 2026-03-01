int load_ptr_arg(int seed, ...) {
  int *p = (int *)__builtin_va_arg(1);
  return *p + seed;
}

int main(void) {
  int x = 12;
  if (load_ptr_arg(3, 0, &x) != 15) {
    return 1;
  }
  return 0;
}
