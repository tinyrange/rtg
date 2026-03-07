typedef struct {
  int value;
} item_t;

typedef item_t item_array_t[2];

int main(void) {
  item_array_t items = {{3}, {5}};
  return items->value != 3;
}
