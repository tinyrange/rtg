struct Pair { int a; int b; };

int main(void) {
  int mem[4];
  struct Pair *p = (struct Pair *)mem;
  struct Pair *q = p + 1;

  p->a = 1;
  p->b = 2;
  q->a = 5;
  q->b = 6;

  if (p->a + p->b != 3) {
    return 1;
  }
  if (q->a + q->b != 11) {
    return 2;
  }
  return 0;
}
