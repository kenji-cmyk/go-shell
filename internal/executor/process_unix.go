//go:build !windows

package executor

import (
	"os"
	"os/exec"
	"syscall"
)

func prepareCommand(cmd *exec.Cmd, foreground bool) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func processGroupID(cmd *exec.Cmd) int {
	if cmd.Process == nil {
		return 0
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		return cmd.Process.Pid
	}
	return pgid
}

func forwardSignal(processes []*exec.Cmd, sig os.Signal) {
	sysSig, ok := sig.(syscall.Signal)
	if !ok {
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
		_ = syscall.Kill(-pgid, sysSig)
	}
}

func forwardedSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGTSTP}
}

func isStopSignal(sig os.Signal) bool {
	return sig == syscall.SIGTSTP
}

func SupportsJobStop() bool {
	return true
}

func stopJob(job JobSnapshot) error {
	return signalJobGroups(job, syscall.SIGSTOP)
}

func resumeJob(job JobSnapshot) error {
	return signalJobGroups(job, syscall.SIGCONT)
}

func signalJobGroups(job JobSnapshot, sig syscall.Signal) error {
	ids := job.PGIDs
	if len(ids) == 0 {
		ids = job.PIDs
	}
	for _, id := range ids {
		if id > 0 {
			if err := syscall.Kill(-id, sig); err != nil {
				return err
			}
		}
	}
	return nil
}
