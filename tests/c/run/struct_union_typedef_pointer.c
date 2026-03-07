typedef struct Packet { int x; } *PacketPtr;
typedef union Value { int x; long y; } *ValuePtr;

int same_width(PacketPtr p, ValuePtr v) {
  if (sizeof(PacketPtr) != sizeof(ValuePtr)) {
    return 0;
  }
  if (p != 0) {
    return 0;
  }
  if (v != 0) {
    return 0;
  }
  return 1;
}

int main(void) {
  PacketPtr p = 0;
  ValuePtr v = 0;
  if (same_width(p, v) != 1) {
    return 1;
  }
  p = (PacketPtr)v;
  if (p != 0) {
    return 2;
  }
  return 0;
}
