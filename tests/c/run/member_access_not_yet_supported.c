struct Point { int x; int y; };

int sum_point(struct Point *p) {
  return p->x + p->y;
}

int main(void) {
  int mem[2];
  struct Point *p = (struct Point *)mem;
  p->x = 7;
  (*p).y = 9;
  if (p->x != 7) {
    return 1;
  }
  if (p->y != 9) {
    return 2;
  }
  if ((*p).x + (*p).y != 16) {
    return 3;
  }
  if (sum_point(p) != 16) {
    return 4;
  }
  return 0;
}
