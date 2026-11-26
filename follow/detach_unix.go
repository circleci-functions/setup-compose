//go:build !windows

package follow

import "syscall"

// detached starts the follower in a session of its own, so it is not killed
// with the step's process group.
func detached() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
