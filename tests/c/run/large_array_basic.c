int global_big[1 << 15];

int main(void) {
  int local_big[1 << 15];

  global_big[123] = 7;
  local_big[456] = 9;

  if (global_big[123] != 7) {
    return 1;
  }
  if (local_big[456] != 9) {
    return 2;
  }
  if (sizeof(global_big) != ((1 << 15) * 4)) {
    return 3;
  }
  if (sizeof(local_big) != ((1 << 15) * 4)) {
    return 4;
  }
  return 0;
}
