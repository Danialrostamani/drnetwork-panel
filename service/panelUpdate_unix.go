//go:build !windows

package service

import (
	"os/exec"
	"syscall"
)

// startDetached starts a command in a session of its own, so that it outlives
// the panel.
func startDetached(name string, args []string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Reaps it if the panel is still running when it ends.
	go func() { _ = cmd.Wait() }()
	return nil
}
