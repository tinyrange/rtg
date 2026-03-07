#include <stdio.h>

int main(void) {
  int sum = 0;
  _Pragma("omp parallel for")
  for (int i = 0; i < 4; i++) {
    sum += i;
  }
  return sum != 6;
}
