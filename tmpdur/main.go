package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println((5 * time.Minute).String())
	fmt.Println((60 * time.Second).String())
	fmt.Println((1*time.Hour + 0*time.Minute + 0*time.Second).String())
	fmt.Println((1*time.Hour + 2*time.Minute).String())
}
