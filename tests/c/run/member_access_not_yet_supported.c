struct Point { int x; int y; };
union Number { int i; long l; };

int sum_point(struct Point *p) {
  return p->x + p->y;
}

int main(void) {
  struct Point p = {7, 9};
  struct Point z = {4};

  struct Point *q = &p;
  if (q->x != 7) {
    return 1;
  }
  if ((*q).y != 9) {
    return 2;
  }
  if (z.x != 4 || z.y != 0) {
    return 3;
  }
  if (sum_point(&p) != 16) {
    return 4;
  }
  union Number u = {13};
  if (u.i != 13) {
    return 5;
  }
  return 0;
}
