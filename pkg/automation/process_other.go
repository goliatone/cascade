//go:build windows

package automation

import (
	"os"
	osexec "os/exec"
)

func configureCommandForCancel(cmd *osexec.Cmd) {}

func terminateProcessTree(process *os.Process) error {
	if process == nil {
		return nil
	}
	return process.Kill()
}

func killProcessTree(process *os.Process) error {
	return terminateProcessTree(process)
}
