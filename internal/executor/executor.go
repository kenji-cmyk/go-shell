package executor

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"go-shell/internal/parser"
)

type Executor struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

func (e Executor) Run(cmd parser.Command) error {
	if cmd.Name == "" {
		return nil
	}

	process := exec.Command(cmd.Name, cmd.Args...)
	if shouldFallbackToCMD(cmd.Name) {
		process = exec.Command("cmd", "/C", cmd.Raw)
	}

	process.Stdin = e.In
	process.Stdout = e.Out
	process.Stderr = e.Err

	if err := process.Run(); err != nil {
		var notFound *exec.Error
		if errors.As(err, &notFound) && errors.Is(notFound.Err, exec.ErrNotFound) {
			return fmt.Errorf("%s: command not found", cmd.Name)
		}
		return err
	}
	return nil
}

func shouldFallbackToCMD(name string) bool {
	switch strings.ToLower(name) {
	case "dir", "cls", "copy", "del", "type":
		return true
	default:
		return false
	}
}
