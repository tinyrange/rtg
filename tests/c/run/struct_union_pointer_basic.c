struct Node;
union Cell;
union Number { int i; long l; };

struct Pair { int a; int b; };

struct Pair g_pair = {5, 8};
union Number g_num = {21};

int both_null(struct Node *n, union Cell *c) {
  if (n != 0) {
    return 0;
  }
  if (c != 0) {
    return 0;
  }
  return 1;
}

int main(void) {
  struct Node *n = 0;
  union Cell *c = 0;
  if (both_null(n, c) != 1) {
    return 1;
  }
  struct Node **pp = &n;
  if (*pp != 0) {
    return 2;
  }
  if (sizeof(struct Node *) != sizeof(union Cell *)) {
    return 3;
  }

  if (g_pair.a != 5 || g_pair.b != 8) {
    return 4;
  }
  struct Pair *p = &g_pair;
  if (p->a + p->b != 13) {
    return 5;
  }
  if (g_num.i != 21) {
    return 6;
  }
  union Number local_num = {34};
  if (local_num.i != 34) {
    return 7;
  }

  return 0;
}
