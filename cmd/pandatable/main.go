package main

import (
	"fmt"
	"os"
)

var version = "0.0.0-dev"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: pandatable <version|serve>")
		return 2
	}
	switch args[0] {
	case "version":
		fmt.Println(version)
		return 0
	case "serve":
		fmt.Fprintln(os.Stderr, "not implemented yet (T0.2/T0.4)")
		return 1
	default:
		fmt.Fprintln(os.Stderr, "usage: pandatable <version|serve>")
		return 2
	}
}
