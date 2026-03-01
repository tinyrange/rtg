typedef int myint;
typedef unsigned char u8;
typedef myint (*binop_t)(myint, myint);

extern myint add2(myint a, myint b);

myint apply(binop_t fn, myint lhs, myint rhs) {
  return fn(lhs, rhs);
}

int main(void) {
  myint lhs = 2;
  myint rhs = 5;
  u8 clipped = (u8)258;
  binop_t op = &add2;
  if (add2(lhs, rhs) != 7) {
    return 1;
  }
  if (op(lhs, rhs) != 7) {
    return 4;
  }
  if ((*op)(lhs, rhs) != 7) {
    return 5;
  }
  if (apply(op, lhs, rhs) != 7) {
    return 6;
  }
  if (apply(add2, lhs, rhs) != 7) {
    return 7;
  }
  op = 0;
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
