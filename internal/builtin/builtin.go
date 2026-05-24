package builtin

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go-shell/internal/parser"
)

var ErrExit = errors.New("exit shell")

type Func func(Context, parser.Command) error

type Context struct {
	Out io.Writer
	Err io.Writer
}

type Registry struct {
	commands map[string]Func
}

func NewRegistry() *Registry {
	r := &Registry{commands: make(map[string]Func)}
	r.Register("cd", cd)
	r.Register("pwd", pwd)
	r.Register("exit", exit)
	r.Register("help", r.help)
	r.Register("echo", echo)
	r.Register("clear", clear)
	r.Register("cls", clear)
	return r
}

func (r *Registry) Register(name string, fn Func) {
	r.commands[strings.ToLower(name)] = fn
}

func (r *Registry) Run(ctx Context, cmd parser.Command) (bool, error) {
	fn, ok := r.commands[strings.ToLower(cmd.Name)]
	if !ok {
		return false, nil
	}
	return true, fn(ctx, cmd)
}

func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.commands))
	for name := range r.commands {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func cd(ctx Context, cmd parser.Command) error {
	if len(cmd.Args) > 1 {
		return fmt.Errorf("cd: too many arguments")
	}

	target := ""
	if len(cmd.Args) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cd: cannot find home directory: %w", err)
		}
		target = home
	} else {
		target = cmd.Args[0]
	}

	if err := os.Chdir(target); err != nil {
		return fmt.Errorf("cd: %w", err)
	}
	return nil
}

func pwd(ctx Context, cmd parser.Command) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("pwd: %w", err)
	}
	_, err = fmt.Fprintln(ctx.Out, wd)
	return err
}

func exit(ctx Context, cmd parser.Command) error {
	return ErrExit
}

func (r *Registry) help(ctx Context, cmd parser.Command) error {
	_, err := fmt.Fprintf(ctx.Out, "Builtins: %s\n", strings.Join(r.Names(), ", "))
	return err
}

func echo(ctx Context, cmd parser.Command) error {
	_, err := fmt.Fprintln(ctx.Out, strings.Join(cmd.Args, " "))
	return err
}

func clear(ctx Context, cmd parser.Command) error {
	return clearScreen(ctx.Out, ctx.Err)
}

func ShortWorkingDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return "gosh"
	}

	base := filepath.Base(wd)
	if base == "." || base == string(filepath.Separator) {
		return wd
	}
	return base
}
