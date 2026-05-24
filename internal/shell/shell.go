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
	aliases        map[string]string
}

func New(in io.Reader, out io.Writer, errOut io.Writer) *Shell {
	aliases := loadAliases()
	return &Shell{
		in:             in,
		out:            out,
		err:            errOut,
		builtins:       builtin.NewRegistry(),
		executor:       executor.Executor{In: in, Out: out, Err: errOut},
		promptTemplate: promptFormat(),
		aliases:        aliases,
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
		AutoComplete:      newCompleter(s.builtins.Names(), s.aliasNames()),
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
		fmt.Fprintln(s.err, parser.FormatError(line, err))
		return true
	}
	if len(parsed.Commands) == 0 {
		return true
	}

	if err := s.expandAliases(&parsed); err != nil {
		fmt.Fprintln(s.err, err)
		return true
	}

	if len(parsed.Commands) == 1 && !parsed.Background && parsed.InputRedirect == "" {
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

func (s *Shell) expandAliases(line *parser.Line) error {
	for i, command := range line.Commands {
		replacement, ok := s.aliases[strings.ToLower(command.Name)]
		if !ok {
			continue
		}

		aliasLine, err := parser.ParseLine(replacement)
		if err != nil {
			return fmt.Errorf("alias %s: %w", command.Name, err)
		}
		if len(aliasLine.Commands) != 1 || aliasLine.InputRedirect != "" || aliasLine.OutputRedirect != "" || aliasLine.Background {
			return fmt.Errorf("alias %s: aliases must expand to one simple command", command.Name)
		}

		expanded := aliasLine.Commands[0]
		expanded.Args = append(expanded.Args, command.Args...)
		expanded.Raw = parser.QuoteForRaw(append([]string{expanded.Name}, expanded.Args...))
		line.Commands[i] = expanded
	}
	return nil
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

func configDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return os.TempDir()
	}
	path := filepath.Join(dir, "gosh")
	_ = os.MkdirAll(path, 0755)
	return path
}

func historyFile() string {
	return filepath.Join(configDir(), "history")
}

func aliasesFile() string {
	if value := os.Getenv("GOSH_ALIASES_FILE"); value != "" {
		return value
	}
	return filepath.Join(configDir(), "aliases")
}

func loadAliases() map[string]string {
	aliases := make(map[string]string)
	loadAliasPairs(aliases, os.Getenv("GOSH_ALIASES"), ";")

	content, err := os.ReadFile(aliasesFile())
	if err == nil {
		loadAliasPairs(aliases, string(content), "\n")
	}
	return aliases
}

func loadAliasPairs(aliases map[string]string, content string, separator string) {
	for _, pair := range strings.Split(content, separator) {
		pair = strings.TrimSpace(pair)
		if pair == "" || strings.HasPrefix(pair, "#") {
			continue
		}

		name, command, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		name = strings.ToLower(strings.TrimSpace(name))
		command = strings.TrimSpace(command)
		if name == "" || command == "" || strings.ContainsAny(name, " \t|<>&") {
			continue
		}
		aliases[name] = command
	}
}

type completer struct {
	builtins []string
	aliases  []string
}

func newCompleter(builtins []string, aliases []string) readline.AutoCompleter {
	return completer{builtins: builtins, aliases: aliases}
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
		for _, name := range c.aliases {
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

func (s *Shell) aliasNames() []string {
	names := make([]string, 0, len(s.aliases))
	for name := range s.aliases {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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
