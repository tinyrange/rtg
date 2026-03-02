int main(void) {
  int i = 0;

  goto check;
loop:
  i = i + 1;

check:
  if (i < 3) {
    goto loop;
  }
  if (i != 3) {
    return 1;
  }
  return 0;
}
