package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println("GoLinx " + Version)
		return
	}
	if err := Run(); err != nil {
		fmt.Fprintf(os.Stderr, "golinx: %v\n", err)
		os.Exit(1)
	}
}
