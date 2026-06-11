//go:build !windows

package tui

import "syscall"

// detachedProcAttr detaches a spawned engine from the monitor's session so
// quitting the monitor never kills a running loop.
func detachedProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
