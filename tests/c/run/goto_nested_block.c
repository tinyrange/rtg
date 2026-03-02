int main(void) {
  int x = 1;
  {
    x = 2;
    goto done;
    x = 3;
  }

done:
  if (x != 2) {
    return 1;
  }
  return 0;
}
