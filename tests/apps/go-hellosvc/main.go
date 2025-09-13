package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok")) })
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprintln(w, "hello from go-hellosvc") })
	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
