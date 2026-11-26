//go:build windows

package follow

import "syscall"

const detachedProcess = 0x00000008

// detached starts the follower outside the step's console and process group,
// so it is not stopped with the step.
func detached() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess}
}
