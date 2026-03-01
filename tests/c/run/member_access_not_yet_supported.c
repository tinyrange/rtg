struct Point { int x; int y; };

int main(void) {
  int mem[2];
  struct Point *p = (struct Point *)mem;
  p->x = 7;
  return 0;
}
