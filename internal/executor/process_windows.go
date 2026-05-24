//go:build windows

package executor

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

var errJobControlUnsupported = errors.New("job stop/resume is not supported on this OS")

func prepareCommand(cmd *exec.Cmd, foreground bool) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func processGroupID(cmd *exec.Cmd) int {
	if cmd.Process == nil {
		return 0
	}
	return cmd.Process.Pid
}

func forwardSignal(processes []*exec.Cmd, sig os.Signal) {
	if sig != os.Interrupt {
		return
	}
	seen := make(map[int]struct{})
	for _, process := range processes {
		pgid := processGroupID(process)
		if pgid <= 0 {
			continue
		}
		if _, ok := seen[pgid]; ok {
			continue
		}
		seen[pgid] = struct{}{}
		if err := generateConsoleCtrlEvent(ctrlBreakEvent, uint32(pgid)); err != nil {
			_ = process.Process.Kill()
		}
	}
}

func forwardedSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}

func isStopSignal(sig os.Signal) bool {
	return false
}

func SupportsJobStop() bool {
	return false
}

func stopJob(job JobSnapshot) error {
	return errJobControlUnsupported
}

func resumeJob(job JobSnapshot) error {
	return errJobControlUnsupported
}

const ctrlBreakEvent = 1

var (
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	procGenerateCtrlEvent = kernel32.NewProc("GenerateConsoleCtrlEvent")
)

func generateConsoleCtrlEvent(event uint32, processGroupID uint32) error {
	result, _, err := procGenerateCtrlEvent.Call(uintptr(event), uintptr(processGroupID))
	if result == 0 {
		if err != syscall.Errno(0) {
			return err
		}
		return syscall.EINVAL
	}
	return nil
}
