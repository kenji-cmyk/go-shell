package shell

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chzyer/readline"

	"go-shell/internal/builtin"
	"go-shell/internal/executor"
	"go-shell/internal/parser"
)

type Shell struct {
	in             io.Reader
	out            io.Writer
	err            io.Writer
	builtins       *builtin.Registry
	executor       executor.Executor
	promptTemplate string
}

func New(in io.Reader, out io.Writer, errOut io.Writer) *Shell {
	return &Shell{
		in:             in,
		out:            out,
		err:            errOut,
		builtins:       builtin.NewRegistry(),
		executor:       executor.Executor{In: in, Out: out, Err: errOut},
		promptTemplate: promptFormat(),
	}
}

func (s *Shell) Run() error {
	if s.isInteractive() {
		return s.runInteractive()
	}
	return s.runBuffered()
}

func (s *Shell) runBuffered() error {
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

func (s *Shell) runInteractive() error {
	rl, err := readline.NewEx(&readline.Config{
		Prompt:            s.prompt(),
		HistoryFile:       historyFile(),
		HistoryLimit:      500,
		HistorySearchFold: true,
		AutoComplete:      newCompleter(s.builtins.Names()),
		Stdin:             readline.NewCancelableStdin(os.Stdin),
		Stdout:            s.out,
		Stderr:            s.err,
	})
	if err != nil {
		return fmt.Errorf("start readline: %w", err)
	}
	defer rl.Close()

	for {
		rl.SetPrompt(s.prompt())
		line, err := rl.Readline()
		if errors.Is(err, readline.ErrInterrupt) {
			if strings.TrimSpace(line) == "" {
				continue
			}
		} else if errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return fmt.Errorf("read input: %w", err)
		}

		if !s.ExecuteLine(line) {
			return nil
		}
	}
}

func (s *Shell) ExecuteLine(line string) bool {
	parsed, err := parser.ParseLine(line)
	if err != nil {
		fmt.Fprintln(s.err, "parse error:", err)
		return true
	}
	if len(parsed.Commands) == 0 {
		return true
	}

	if len(parsed.Commands) == 1 {
		cmd := parsed.Commands[0]
		out, closeOut, err := s.builtinOutput(parsed)
		if err != nil {
			fmt.Fprintln(s.err, err)
			return true
		}
		defer closeOut()

		ctx := builtin.Context{Out: out, Err: s.err}
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
	}

	if err := s.executor.RunLine(parsed); err != nil {
		fmt.Fprintln(s.err, err)
	}
	return true
}

func (s *Shell) builtinOutput(line parser.Line) (io.Writer, func(), error) {
	if line.OutputRedirect == "" {
		return s.out, func() {}, nil
	}

	flag := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if line.AppendOutput {
		flag = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	file, err := os.OpenFile(line.OutputRedirect, flag, 0644)
	if err != nil {
		return nil, func() {}, fmt.Errorf("open output redirect: %w", err)
	}
	return file, func() { _ = file.Close() }, nil
}

func (s *Shell) prompt() string {
	wd, err := os.Getwd()
	if err != nil {
		wd = "gosh"
	}
	base := filepath.Base(wd)
	if base == "." || base == string(filepath.Separator) {
		base = wd
	}

	prompt := strings.ReplaceAll(s.promptTemplate, "{cwd}", wd)
	prompt = strings.ReplaceAll(prompt, "{base}", base)
	return prompt
}

func (s *Shell) isInteractive() bool {
	in, inOK := s.in.(*os.File)
	out, outOK := s.out.(*os.File)
	return inOK && outOK && isTerminalFile(in) && isTerminalFile(out)
}

func isTerminalFile(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func promptFormat() string {
	if value := os.Getenv("GOSH_PROMPT"); value != "" {
		return value
	}
	return "{base}> "
}

func historyFile() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "gosh_history")
	}
	path := filepath.Join(dir, "gosh")
	_ = os.MkdirAll(path, 0755)
	return filepath.Join(path, "history")
}

type completer struct {
	builtins []string
}

func newCompleter(builtins []string) readline.AutoCompleter {
	return completer{builtins: builtins}
}

func (c completer) Do(line []rune, pos int) ([][]rune, int) {
	prefix := currentToken(string(line[:pos]))
	candidates := c.candidates(prefix, isCommandPosition(string(line[:pos])))
	out := make([][]rune, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.HasPrefix(strings.ToLower(candidate), strings.ToLower(prefix)) {
			out = append(out, []rune(candidate[len(prefix):]))
		}
	}
	return out, len([]rune(prefix))
}

func (c completer) candidates(prefix string, commandPosition bool) []string {
	seen := make(map[string]struct{})
	var candidates []string
	add := func(value string) {
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		candidates = append(candidates, value)
	}

	if commandPosition {
		for _, name := range c.builtins {
			add(name)
		}
	}

	pattern := prefix + "*"
	if prefix == "" {
		pattern = "*"
	}
	matches, _ := filepath.Glob(pattern)
	for _, match := range matches {
		info, err := os.Stat(match)
		if err == nil && info.IsDir() {
			match += string(filepath.Separator)
		}
		add(match)
	}
	sort.Strings(candidates)
	return candidates
}

func currentToken(line string) string {
	index := strings.LastIndexAny(line, " \t|<>")
	if index < 0 {
		return line
	}
	return line[index+1:]
}

func isCommandPosition(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return true
	}
	last := trimmed[len(trimmed)-1]
	if last == '|' {
		return true
	}
	fields := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '|' || r == '<' || r == '>'
	})
	return len(fields) <= 1 && !strings.ContainsAny(trimmed, " <>")
}
