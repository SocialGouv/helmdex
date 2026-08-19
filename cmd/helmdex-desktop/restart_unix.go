//go:build desktop && !windows

package main

import (
	"os/exec"
	"syscall"
)

// detachProcess makes the relaunched app survive this process's exit
// (own session, no inherited controlling terminal).
func detachProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
