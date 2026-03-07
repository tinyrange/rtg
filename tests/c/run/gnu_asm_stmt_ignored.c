int f(void) {
  asm volatile("" : : : "memory");
  return 7;
}

int main(void) {
  return f() == 7 ? 0 : 1;
}
