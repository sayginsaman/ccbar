//go:build unix

package usage

import "syscall"

// detachSysProcAttr fully detaches the background refresher into its own session
// so it outlives the short-lived statusline render process.
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
