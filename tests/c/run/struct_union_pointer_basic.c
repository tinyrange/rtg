struct Node;
union Cell;

struct Pair { int a; int b; };

struct Pair g_pair;

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

  if (g_pair.a != 0 || g_pair.b != 0) {
    return 4;
  }
  g_pair.a = 5;
  g_pair.b = 8;
  struct Pair *p = &g_pair;
  if (p->a + p->b != 13) {
    return 5;
  }

  return 0;
}
