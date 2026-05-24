package executor

import (
	"errors"
	"fmt"
	"io"
	"os"
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

func (e Executor) RunLine(line parser.Line) error {
	if len(line.Commands) == 0 {
		return nil
	}
	if len(line.Commands) == 1 && line.InputRedirect == "" && line.OutputRedirect == "" {
		return e.Run(line.Commands[0])
	}

	var input io.Reader = e.In
	var output io.Writer = e.Out
	var inputFile *os.File
	var outputFile *os.File

	if line.InputRedirect != "" {
		file, err := os.Open(line.InputRedirect)
		if err != nil {
			return fmt.Errorf("open input redirect: %w", err)
		}
		defer file.Close()
		inputFile = file
		input = inputFile
	}

	if line.OutputRedirect != "" {
		flag := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		if line.AppendOutput {
			flag = os.O_CREATE | os.O_WRONLY | os.O_APPEND
		}
		file, err := os.OpenFile(line.OutputRedirect, flag, 0644)
		if err != nil {
			return fmt.Errorf("open output redirect: %w", err)
		}
		defer file.Close()
		outputFile = file
		output = outputFile
	}

	processes := make([]*exec.Cmd, len(line.Commands))
	for i, command := range line.Commands {
		processes[i] = commandProcess(command)
		processes[i].Stderr = e.Err
	}

	processes[0].Stdin = input
	processes[len(processes)-1].Stdout = output

	readers := make([]*io.PipeReader, 0, len(processes)-1)
	writers := make([]*io.PipeWriter, 0, len(processes)-1)
	for i := 0; i < len(processes)-1; i++ {
		reader, writer := io.Pipe()
		processes[i].Stdout = writer
		processes[i+1].Stdin = reader
		readers = append(readers, reader)
		writers = append(writers, writer)
	}

	for _, process := range processes {
		if err := process.Start(); err != nil {
			return commandStartError(process, err)
		}
	}

	errs := make(chan error, len(processes))
	for i, process := range processes {
		go func(i int, process *exec.Cmd) {
			err := process.Wait()
			if i < len(writers) {
				_ = writers[i].Close()
			}
			if i > 0 {
				_ = readers[i-1].Close()
			}
			errs <- err
		}(i, process)
	}

	var firstErr error
	for range processes {
		if err := <-errs; err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

func commandProcess(cmd parser.Command) *exec.Cmd {
	if shouldFallbackToCMD(cmd.Name) {
		return exec.Command("cmd", "/C", cmd.Raw)
	}
	return exec.Command(cmd.Name, cmd.Args...)
}

func commandStartError(process *exec.Cmd, err error) error {
	var notFound *exec.Error
	if errors.As(err, &notFound) && errors.Is(notFound.Err, exec.ErrNotFound) {
		return fmt.Errorf("%s: command not found", process.Args[0])
	}
	return err
}

func shouldFallbackToCMD(name string) bool {
	switch strings.ToLower(name) {
	case "dir", "cls", "copy", "del", "type":
		return true
	default:
		return false
	}
}
