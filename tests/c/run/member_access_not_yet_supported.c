struct Point { int x; int y; };

int sum_point(struct Point *p) {
  return p->x + p->y;
}

int main(void) {
  struct Point p;
  if (p.x != 0 || p.y != 0) {
    return 1;
  }

  p.x = 7;
  p.y = 9;

  struct Point *q = &p;
  if (q->x != 7) {
    return 2;
  }
  if ((*q).y != 9) {
    return 3;
  }
  if (sum_point(&p) != 16) {
    return 4;
  }
  return 0;
}
