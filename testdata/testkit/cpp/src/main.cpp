#include <iostream>
#include <thread>
#include "../include/entity.hpp"

int main() {
    Entity e("1", "test");
    std::cout << "Entity: " << e.name() << " (" << e.id() << ")\n";
    return 0;
}
