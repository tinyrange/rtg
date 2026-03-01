enum Outer {
  ITEM = 7
};

int main(void) {
  if (ITEM != 7) {
    return 1;
  }
  {
    enum Inner {
      ITEM = 3,
      NEXT = ITEM + 2
    } x = NEXT;
    if (ITEM != 3) {
      return 2;
    }
    if (NEXT != 5) {
      return 3;
    }
    if (x != 5) {
      return 4;
    }
  }
  if (ITEM != 7) {
    return 5;
  }
  return 0;
}
