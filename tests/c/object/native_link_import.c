int host_mul2(int x);

int call_host(int x) {
    return host_mul2(x) + 1;
}
