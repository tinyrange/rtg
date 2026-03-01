enum Mode {
  MODE_A = 5,
  MODE_B = 9
};

enum Mode bump(enum Mode v) {
  return (enum Mode)(v + 1);
}

int main(void) {
  enum Mode x = MODE_A;
  if (sizeof(enum Mode) != 4) {
    return 1;
  }
  if (x != 5) {
    return 2;
  }
  if (bump(x) != 6) {
    return 3;
  }
  x = (enum Mode)(MODE_B - MODE_A);
  if (x != 4) {
    return 4;
  }
  return 0;
}
