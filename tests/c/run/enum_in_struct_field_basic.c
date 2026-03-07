struct S {
  enum { SA = 3, SB = 5 } tag;
};

int main(void) {
  return SB == 5 ? 0 : 1;
}
