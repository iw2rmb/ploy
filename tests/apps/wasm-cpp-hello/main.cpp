#include <iostream>
#include <emscripten.h>

extern "C" {
    EMSCRIPTEN_KEEPALIVE
    int add_numbers(int a, int b) {
        return a + b;
    }
    
    EMSCRIPTEN_KEEPALIVE
    void hello_world() {
        std::cout << "Hello from C++ WASM!" << std::endl;
    }
}

int main() {
    std::cout << "C++ WASM module initialized" << std::endl;
    return 0;
}