typedef int myint;
typedef unsigned char u8;
typedef myint (*binop_t)(myint, myint);

extern myint add2(myint a, myint b);

int main(void) {
  myint lhs = 2;
  myint rhs = 5;
  u8 clipped = (u8)258;
  binop_t op = 0;
  if (add2(lhs, rhs) != 7) {
    return 1;
  }
  if (clipped != 2) {
    return 2;
  }
  if (op != 0) {
    return 3;
  }
  return 0;
}

myint add2(myint a, myint b) {
  return a + b;
}
