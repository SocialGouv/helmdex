package main

import (
	"fmt"
	"os"

	"helmdex/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
