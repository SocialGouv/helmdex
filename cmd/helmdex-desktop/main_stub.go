//go:build !desktop

package main

import (
	"fmt"
	"os"
)

// Entrypoint when helmdex-desktop is built without the `desktop` build tag,
// so plain `go build ./...` / `go test ./...` never need the Wails/WebKit
// toolchain. The real desktop binary is built via `task desktop:build:*`
// (wails build -tags desktop).
func main() {
	fmt.Fprintln(os.Stderr, "helmdex-desktop must be built with `-tags desktop` (Wails); use `task desktop:build:<platform>`.")
	os.Exit(1)
}
