char *g = "global";

int main(void) {
  char *s = "hello";
  char *t = "ab" "cd";
  if (g == 0) {
    return 1;
  }
  if (s == 0) {
    return 2;
  }
  if (t == 0) {
    return 3;
  }
  return 0;
}
