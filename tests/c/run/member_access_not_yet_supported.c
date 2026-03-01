struct Point { int x; };

int sum_point(struct Point *p) {
  return p->x + (*p).x;
}

int main(void) {
  int mem[2];
  struct Point *p = (struct Point *)mem;
  p->x = 7;
  if (p->x != 7) {
    return 1;
  }
  if ((*p).x != 7) {
    return 2;
  }
  if (sum_point(p) != 14) {
    return 3;
  }
  return 0;
}
