typedef int *intptr;
typedef intptr *intptrptr;

int main(void) {
  int x = 4, y = 3;
  intptr p = &x, q = &y;
  intptrptr pp = &p;

  if (**pp != 4) {
    return 1;
  }
  {
    typedef intptr alias_intptr;
    alias_intptr t = p;
    typedef int (*adder_t)(int, int);
    adder_t cb = 0;
    if (*t != 4) {
      return 3;
    }
    if (cb != 0) {
      return 4;
    }
  }

  *q = *q + **pp;
  if (y != 7) {
    return 2;
  }
  return 0;
}
