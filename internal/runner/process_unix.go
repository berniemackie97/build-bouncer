//go:build !windows

package runner

import (
	"os/exec"
	"syscall"
	"time"
)

type checkProcess struct {
	groupID int
}

func startCheckProcess(command *exec.Cmd) (*checkProcess, error) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		return nil, err
	}
	return &checkProcess{groupID: command.Process.Pid}, nil
}

func (process *checkProcess) terminate() {
	_ = syscall.Kill(-process.groupID, syscall.SIGTERM)
	time.Sleep(200 * time.Millisecond)
	process.close()
}

func (process *checkProcess) close() {
	_ = syscall.Kill(-process.groupID, syscall.SIGKILL)
}
