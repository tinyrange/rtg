int main(void) {
  int a[4];
  int *p = a;
  unsigned char uc = (unsigned char)257;
  short s = (short)65535;
  char *msg = "hi";

  p++;
  *p = 5;
  ++p;
  *p = 8;

  p--;
  if (*p != 5) {
    return 1;
  }
  if (a[2] != 8) {
    return 2;
  }

  p--;
  if (*p != 0) {
    return 3;
  }
  if (uc != 1) {
    return 4;
  }
  if (s != -1) {
    return 5;
  }
  if (sizeof(char) != 1 || sizeof(short) != 2 || sizeof(int) != 4) {
    return 6;
  }
  if (sizeof(uc) != 1 || sizeof(a) != 16) {
    return 7;
  }
  if (msg[1] != 'i') {
    return 8;
  }
  return 0;
}
