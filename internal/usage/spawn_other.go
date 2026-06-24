//go:build !unix

package usage

import "syscall"

// detachSysProcAttr is a no-op on non-unix platforms (keeps the package building
// everywhere; the tool itself targets unix).
func detachSysProcAttr() *syscall.SysProcAttr {
	return nil
}
