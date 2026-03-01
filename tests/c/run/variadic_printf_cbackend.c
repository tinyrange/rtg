extern long open(const char *path, long flags, ...);
typedef long (*open_t)(const char *path, long flags, ...);

int main(void) {
  long fd1 = open("/dev/null", 0, 0);
  open_t of = open;
  long fd2 = of("/dev/null", 0, 0);
  if (fd1 < 0) {
    return 1;
  }
  if (fd2 < 0) {
    return 2;
  }
  return 0;
}
