package main

import (
	"os"

	"workspaces/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
