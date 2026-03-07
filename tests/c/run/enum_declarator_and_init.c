enum Flags {
  FLAG_A = 1,
  FLAG_B = FLAG_A << 2
} g = FLAG_B;

int main(void) {
  if (g != 4) {
    return 1;
  }
  enum More {
    LOCAL_A = FLAG_B + 1,
    LOCAL_B
  } v = LOCAL_B;
  if (LOCAL_A != 5) {
    return 2;
  }
  if (LOCAL_B != 6) {
    return 3;
  }
  if (v != 6) {
    return 4;
  }
  return 0;
}
