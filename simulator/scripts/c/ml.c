#include <stdlib.h>

int main(void) {
    const size_t size = 1024 * 1024;
    while (1) {
        void *block = malloc(size);
        if (!block) {
            return 0;
        }
    }
    return 0;
}
