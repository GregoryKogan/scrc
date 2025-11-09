#include <iostream>

int main() {
    long long value;
    long long total = 0;
    while (std::cin >> value) {
        total += value;
    }
    std::cout << (total + 1) << "\n";
    return 0;
}
