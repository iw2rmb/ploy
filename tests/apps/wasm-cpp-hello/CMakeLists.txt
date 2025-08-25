cmake_minimum_required(VERSION 3.13)
project(wasm_cpp_hello)

# Emscripten WebAssembly configuration
set(CMAKE_CXX_STANDARD 17)

# Check if building for WebAssembly with Emscripten
if(EMSCRIPTEN)
    message(STATUS "Building for WebAssembly with Emscripten")
    set(CMAKE_EXECUTABLE_SUFFIX ".wasm")
endif()

add_executable(wasm_cpp_hello main.cpp)

# Emscripten-specific flags for WASM output
set_target_properties(wasm_cpp_hello PROPERTIES 
    COMPILE_FLAGS "-s USE_SDL=0"
    LINK_FLAGS "-s EXPORTED_FUNCTIONS='[\"_main\"]' -s EXPORTED_RUNTIME_METHODS='[\"ccall\",\"cwrap\"]' -s WASM=1"
)