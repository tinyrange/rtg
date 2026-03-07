enum Color {
  RED,
  GREEN = 4,
  BLUE
};

int main(void) {
  if (RED != 0) {
    return 1;
  }
  if (GREEN != 4) {
    return 2;
  }
  if (BLUE != 5) {
    return 3;
  }
  int x = BLUE;
  if (x != 5) {
    return 4;
  }
  return 0;
}
