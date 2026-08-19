//go:build desktop && windows

package main

import "os/exec"

// detachProcess is a no-op on Windows: a started process is independent of
// its parent by default.
func detachProcess(cmd *exec.Cmd) {}
