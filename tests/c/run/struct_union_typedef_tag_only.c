typedef struct Node Node;
typedef union Payload Payload;

struct Node { int x; };
union Payload { int y; };

int main(void) {
  Node *n = 0;
  Payload *p = 0;
  if (n != 0) {
    return 1;
  }
  if (p != 0) {
    return 2;
  }
  if (sizeof(Node *) != sizeof(struct Node *)) {
    return 3;
  }
  if (sizeof(Payload *) != sizeof(union Payload *)) {
    return 4;
  }
  return 0;
}
