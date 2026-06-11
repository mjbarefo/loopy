//go:build windows

package tui

import "syscall"

// detachedProcAttr detaches a spawned engine from the monitor's console.
func detachedProcAttr() *syscall.SysProcAttr {
	// CREATE_NEW_PROCESS_GROUP | DETACHED_PROCESS
	return &syscall.SysProcAttr{CreationFlags: 0x00000200 | 0x00000008}
}
