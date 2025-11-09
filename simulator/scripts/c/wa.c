#include <stdio.h>

int main(void) {
    long long value;
    long long total = 0;
    while (scanf("%lld", &value) == 1) {
        total += value;
    }
    printf("%lld\n", total + 1);
    return 0;
}
