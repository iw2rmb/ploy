cmake_minimum_required(VERSION 3.13)
project(wasm_cpp_hello)

set(CMAKE_CXX_STANDARD 17)

add_executable(wasm_cpp_hello main.cpp)

set_target_properties(wasm_cpp_hello PROPERTIES 
    COMPILE_FLAGS "-s USE_SDL=0"
    LINK_FLAGS "-s EXPORTED_FUNCTIONS='[\"_main\"]' -s EXPORTED_RUNTIME_METHODS='[\"ccall\",\"cwrap\"]'"
)