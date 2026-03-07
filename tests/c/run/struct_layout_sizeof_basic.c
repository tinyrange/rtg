struct Pair { int a; int b; };
struct Mix { char c; int x; };

int main(void) {
  if (sizeof(struct Pair) != 8) {
    return 1;
  }
  if (sizeof(struct Mix) != 8) {
    return 2;
  }
  if (sizeof(struct Pair *) != sizeof(char *)) {
    return 3;
  }
  return 0;
}
