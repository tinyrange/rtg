struct Pair { int a; };

int main(void) {
  int mem[2];
  struct Pair *p = (struct Pair *)mem;
  p->a = 5;
  if (p->a != 5) {
    return 1;
  }
  if ((*p).a != 5) {
    return 2;
  }
  return 0;
}
