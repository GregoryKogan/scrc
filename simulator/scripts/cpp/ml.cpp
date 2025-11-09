#include <vector>

int main() {
    std::vector<std::vector<char>> blocks;
    const std::size_t size = 1024 * 1024;
    while (true) {
        blocks.emplace_back(size, 0);
    }
    return 0;
}
