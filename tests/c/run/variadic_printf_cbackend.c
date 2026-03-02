typedef int (*variadic_fn_t)(int first, ...);

static int first_arg(int first, ...) {
  return first;
}

int main(void) {
  int n1 = first_arg(7, 100);
  variadic_fn_t fn = first_arg;
  int n2 = fn(8, 200, 300);
  if (n1 != 7) {
    return 1;
  }
  if (n2 != 8) {
    return 2;
  }
  return 0;
}
