//go:build js && wasm

package main

import (
    "fmt"
    "syscall/js"
)

func helloHandler(this js.Value, args []js.Value) interface{} {
    name := args[0].String()
    message := fmt.Sprintf("Hello, %s from Go WASM!", name)
    return js.ValueOf(message)
}

func main() {
    fmt.Println("Go WASM module started")
    
    js.Global().Set("hello", js.FuncOf(helloHandler))
    
    // Keep the program running
    select {}
}