package shell

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"go-shell/internal/builtin"
	"go-shell/internal/executor"
	"go-shell/internal/parser"
)

type Shell struct {
	in       io.Reader
	out      io.Writer
	err      io.Writer
	builtins *builtin.Registry
	executor executor.Executor
}

func New(in io.Reader, out io.Writer, errOut io.Writer) *Shell {
	return &Shell{
		in:       in,
		out:      out,
		err:      errOut,
		builtins: builtin.NewRegistry(),
		executor: executor.Executor{In: in, Out: out, Err: errOut},
	}
}

func (s *Shell) Run() error {
	reader := bufio.NewReader(s.in)

	for {
		fmt.Fprint(s.out, s.prompt())

		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("read input: %w", err)
		}
		if errors.Is(err, io.EOF) && strings.TrimSpace(line) == "" {
			return nil
		}

		keepRunning := s.ExecuteLine(line)
		if !keepRunning {
			return nil
		}

		if errors.Is(err, io.EOF) {
			return nil
		}
	}
}

func (s *Shell) ExecuteLine(line string) bool {
	cmd, err := parser.Parse(line)
	if err != nil {
		fmt.Fprintln(s.err, "parse error:", err)
		return true
	}
	if cmd.Name == "" {
		return true
	}

	ctx := builtin.Context{Out: s.out, Err: s.err}
	handled, err := s.builtins.Run(ctx, cmd)
	if handled {
		if errors.Is(err, builtin.ErrExit) {
			return false
		}
		if err != nil {
			fmt.Fprintln(s.err, err)
		}
		return true
	}

	if err := s.executor.Run(cmd); err != nil {
		fmt.Fprintln(s.err, err)
	}
	return true
}

func (s *Shell) prompt() string {
	return fmt.Sprintf("%s> ", builtin.ShortWorkingDir())
}
